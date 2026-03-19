package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/google/go-github/v68/github"
)

// hexColorRE matches a valid 6-character hex color string (e.g., "d73a4a").
var hexColorRE = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

// maxTimelineConcurrency limits the number of concurrent timeline API calls.
const maxTimelineConcurrency = 10

// cardLocation records where a card was placed so concurrent timeline results
// can be assigned back without reordering.
type cardLocation struct {
	colIdx   int
	cardIdx  int
	issueNum int
}

// Compile-time check: *GitHubProvider implements BoardProvider.
var _ BoardProvider = (*GitHubProvider)(nil)

// GitHubClient abstracts the GitHub API for testing.
type GitHubClient interface {
	ListByRepo(ctx context.Context, owner string, repo string, opts *github.IssueListByRepoOptions) ([]*github.Issue, *github.Response, error)
	Create(ctx context.Context, owner string, repo string, issue *github.IssueRequest) (*github.Issue, *github.Response, error)
	Edit(ctx context.Context, owner string, repo string, number int, issue *github.IssueRequest) (*github.Issue, *github.Response, error)
	CreateLabel(ctx context.Context, owner string, repo string, label *github.Label) (*github.Label, *github.Response, error)
	ListIssueTimeline(ctx context.Context, owner string, repo string, number int, opts *github.ListOptions) ([]*github.Timeline, *github.Response, error)
	ListCollaborators(ctx context.Context, owner string, repo string, opts *github.ListCollaboratorsOptions) ([]*github.User, *github.Response, error)
	GetUser(ctx context.Context, user string) (*github.User, *github.Response, error)
}

// GitHubProvider fetches board data from GitHub Issues.
type GitHubProvider struct {
	client  GitHubClient
	owner   string
	repo    string
	columns []string
}

// NewGitHubProvider creates a GitHubProvider with the given client, repository, and column names.
func NewGitHubProvider(client GitHubClient, owner, repo string, columns []string) *GitHubProvider {
	return &GitHubProvider{
		client:  client,
		owner:   owner,
		repo:    repo,
		columns: columns,
	}
}

// FetchBoard retrieves open issues and maps them to board columns.
// It fetches issues sequentially, then fetches linked PRs concurrently
// with bounded concurrency to reduce total latency.
func (g *GitHubProvider) FetchBoard(ctx context.Context) (Board, error) {
	if len(g.columns) == 0 {
		return Board{}, errors.New("at least one column is required")
	}

	// Build columns from configured names.
	columns := make([]Column, len(g.columns))
	for i, name := range g.columns {
		columns[i] = Column{Title: name}
	}

	// Build a lookup map from lowercase column name to column index.
	colIndex := make(map[string]int, len(g.columns))
	for i, name := range g.columns {
		colIndex[strings.ToLower(name)] = i
	}

	// --- Phase 1: Collect issues and build cards (sequential) ---

	var locations []cardLocation

	opts := &github.IssueListByRepoOptions{
		State:       "open",
		Sort:        "created",
		Direction:   "asc",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		issues, resp, err := g.client.ListByRepo(ctx, g.owner, g.repo, opts)
		if err != nil {
			return Board{}, err
		}

		for _, issue := range issues {
			// Skip pull requests (GitHub's Issues API returns them too).
			if issue.PullRequestLinks != nil {
				continue
			}

			card := issueToCard(issue)

			// Find the furthest (rightmost) column matching any label.
			bestIdx := -1
			for _, label := range issue.Labels {
				if idx, ok := colIndex[strings.ToLower(label.GetName())]; ok {
					if idx > bestIdx {
						bestIdx = idx
					}
				}
			}

			// No matching label: place in first column.
			if bestIdx < 0 {
				bestIdx = 0
			}
			columns[bestIdx].Cards = append(columns[bestIdx].Cards, card)

			// Record location for Phase 2.
			locations = append(locations, cardLocation{
				colIdx:   bestIdx,
				cardIdx:  len(columns[bestIdx].Cards) - 1,
				issueNum: issue.GetNumber(),
			})
		}

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// --- Phase 2: Fetch linked PRs concurrently ---

	if len(locations) == 0 {
		return Board{Columns: columns}, nil
	}

	type timelineResult struct {
		loc       cardLocation
		linkedPRs []LinkedPR
		err       error
	}

	parentCtx := ctx
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sem := make(chan struct{}, maxTimelineConcurrency)
	results := make(chan timelineResult, len(locations))
	var wg sync.WaitGroup

	for _, loc := range locations {
		wg.Add(1)
		go func(loc cardLocation) {
			defer wg.Done()

			// Check for cancellation before acquiring semaphore.
			select {
			case <-ctx.Done():
				results <- timelineResult{loc: loc, err: ctx.Err()}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			// Check again after acquiring semaphore.
			if ctx.Err() != nil {
				results <- timelineResult{loc: loc, err: ctx.Err()}
				return
			}

			linkedPRs, err := g.fetchLinkedPRs(ctx, loc.issueNum)
			if err != nil {
				cancel()
			}
			results <- timelineResult{loc: loc, linkedPRs: linkedPRs, err: err}
		}(loc)
	}

	// Close results channel once all goroutines complete.
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results; return the first real (non-cancellation) error.
	var firstErr error
	for res := range results {
		if res.err != nil {
			if firstErr == nil && !errors.Is(res.err, context.Canceled) {
				firstErr = res.err
			}
			continue
		}
		columns[res.loc.colIdx].Cards[res.loc.cardIdx].LinkedPRs = res.linkedPRs
	}

	if firstErr != nil {
		return Board{}, firstErr
	}
	if parentCtx.Err() != nil {
		return Board{}, parentCtx.Err()
	}

	return Board{Columns: columns}, nil
}

// extractLabels converts GitHub API labels to provider Labels, stripping the
// optional "#" prefix from color values and validating 6-character hex format.
func extractLabels(ghLabels []*github.Label) []Label {
	labels := make([]Label, 0, len(ghLabels))
	for _, l := range ghLabels {
		color := strings.TrimPrefix(l.GetColor(), "#")
		if !hexColorRE.MatchString(color) {
			color = ""
		}
		labels = append(labels, Label{Name: l.GetName(), Color: color})
	}
	return labels
}

// extractAssignees converts GitHub API users to provider Assignees.
func extractAssignees(ghAssignees []*github.User) []Assignee {
	assignees := make([]Assignee, 0, len(ghAssignees))
	for _, u := range ghAssignees {
		assignees = append(assignees, Assignee{Login: u.GetLogin()})
	}
	return assignees
}

// issueToCard converts a github.Issue to a provider.Card, extracting labels and assignees.
func issueToCard(issue *github.Issue) Card {
	return Card{
		Number:    issue.GetNumber(),
		Title:     issue.GetTitle(),
		Body:      issue.GetBody(),
		URL:       issue.GetHTMLURL(),
		Labels:    extractLabels(issue.Labels),
		Assignees: extractAssignees(issue.Assignees),
	}
}

// fetchLinkedPRs retrieves cross-referenced pull requests from the issue timeline.
func (g *GitHubProvider) fetchLinkedPRs(ctx context.Context, issueNumber int) ([]LinkedPR, error) {
	opts := &github.ListOptions{PerPage: 100}
	seen := make(map[int]bool)
	var linkedPRs []LinkedPR

	for {
		events, resp, err := g.client.ListIssueTimeline(ctx, g.owner, g.repo, issueNumber, opts)
		if err != nil {
			return nil, err
		}

		for _, event := range events {
			if event.GetEvent() != "cross-referenced" {
				continue
			}
			if event.Source == nil || event.Source.Issue == nil || event.Source.Issue.PullRequestLinks == nil {
				continue
			}
			prNumber := event.Source.Issue.GetNumber()
			if seen[prNumber] {
				continue
			}
			seen[prNumber] = true
			linkedPRs = append(linkedPRs, LinkedPR{
				Number: prNumber,
				Title:  event.Source.Issue.GetTitle(),
				URL:    event.Source.Issue.GetHTMLURL(),
			})
		}

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return linkedPRs, nil
}

// CreateCard creates a GitHub issue with the given title and optional label.
func (g *GitHubProvider) CreateCard(ctx context.Context, title string, label string) (Card, error) {
	req := &github.IssueRequest{
		Title: github.Ptr(title),
	}
	if label != "" {
		req.Labels = &[]string{label}
	}

	issue, _, err := g.client.Create(ctx, g.owner, g.repo, req)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == 422 {
			return Card{}, fmt.Errorf("label %q does not exist in the repository", label)
		}
		return Card{}, err
	}

	card := issueToCard(issue)
	if len(card.Labels) == 0 && label != "" {
		card.Labels = []Label{{Name: label}}
	}

	return card, nil
}

// UpdateCard updates a GitHub issue's title, body, and labels.
func (g *GitHubProvider) UpdateCard(ctx context.Context, number int, title string, body string, labels []string) (Card, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Card{}, errors.New("title is required")
	}

	req := &github.IssueRequest{
		Title:  github.Ptr(title),
		Body:   github.Ptr(body),
		Labels: &labels,
	}

	issue, _, err := g.client.Edit(ctx, g.owner, g.repo, number, req)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == 422 {
			return Card{}, fmt.Errorf("validation failed: one or more labels may not exist")
		}
		return Card{}, err
	}

	return issueToCard(issue), nil
}

// FetchCollaborators retrieves all collaborators for the repository.
func (g *GitHubProvider) FetchCollaborators(ctx context.Context) ([]Assignee, error) {
	var allCollaborators []Assignee
	opts := &github.ListCollaboratorsOptions{
		ListOptions: github.ListOptions{PerPage: 100},
	}
	for {
		users, resp, err := g.client.ListCollaborators(ctx, g.owner, g.repo, opts)
		if err != nil {
			return nil, err
		}
		allCollaborators = append(allCollaborators, extractAssignees(users)...)
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	if allCollaborators == nil {
		allCollaborators = []Assignee{}
	}
	return allCollaborators, nil
}

// SetAssignees atomically replaces the assignees on a GitHub issue.
func (g *GitHubProvider) SetAssignees(ctx context.Context, number int, logins []string) (Card, error) {
	req := &github.IssueRequest{
		Assignees: &logins,
	}
	issue, _, err := g.client.Edit(ctx, g.owner, g.repo, number, req)
	if err != nil {
		return Card{}, err
	}
	return issueToCard(issue), nil
}

// GetAuthenticatedUser returns the login of the currently authenticated user.
func (g *GitHubProvider) GetAuthenticatedUser(ctx context.Context) (string, error) {
	user, _, err := g.client.GetUser(ctx, "")
	if err != nil {
		return "", err
	}
	return user.GetLogin(), nil
}

// CreateLabel creates a new label in the GitHub repository.
func (g *GitHubProvider) CreateLabel(ctx context.Context, name string) error {
	label := &github.Label{
		Name:  github.Ptr(name),
		Color: github.Ptr("ededed"),
	}

	_, _, err := g.client.CreateLabel(ctx, g.owner, g.repo, label)
	if err != nil {
		var ghErr *github.ErrorResponse
		if errors.As(err, &ghErr) && ghErr.Response != nil && ghErr.Response.StatusCode == 422 {
			return fmt.Errorf("label %q already exists", name)
		}
		return err
	}
	return nil
}
