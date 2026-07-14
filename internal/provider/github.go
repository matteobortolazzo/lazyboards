package provider

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/go-github/v68/github"
)

// hexColorRE matches a valid 6-character hex color string (e.g., "d73a4a").
var hexColorRE = regexp.MustCompile(`^[0-9a-fA-F]{6}$`)

// Compile-time check: *GitHubProvider implements BoardProvider.
var _ BoardProvider = (*GitHubProvider)(nil)

// GitHubClient abstracts the GitHub API for testing.
type GitHubClient interface {
	ListByRepo(ctx context.Context, owner string, repo string, opts *github.IssueListByRepoOptions) ([]*github.Issue, *github.Response, error)
	Create(ctx context.Context, owner string, repo string, issue *github.IssueRequest) (*github.Issue, *github.Response, error)
	Edit(ctx context.Context, owner string, repo string, number int, issue *github.IssueRequest) (*github.Issue, *github.Response, error)
	CreateLabel(ctx context.Context, owner string, repo string, label *github.Label) (*github.Label, *github.Response, error)
	ListLabels(ctx context.Context, owner string, repo string, opts *github.ListOptions) ([]*github.Label, *github.Response, error)
	ListCollaborators(ctx context.Context, owner string, repo string, opts *github.ListCollaboratorsOptions) ([]*github.User, *github.Response, error)
	GetUser(ctx context.Context, user string) (*github.User, *github.Response, error)
}

// GitHubProvider fetches board data from GitHub Issues.
type GitHubProvider struct {
	client  GitHubClient
	gql     graphQLBoardClient
	owner   string
	repo    string
	columns []string
}

// NewGitHubProvider creates a GitHubProvider with the given clients, repository, and column names.
//
// FetchBoard pages issues (and their linked PRs) exclusively through gql.
// client is still required for every other GitHubProvider method
// (CreateCard, UpdateCard, FetchCollaborators, etc.), which remain REST-based.
func NewGitHubProvider(client GitHubClient, gql graphQLBoardClient, owner, repo string, columns []string) *GitHubProvider {
	return &GitHubProvider{
		client:  client,
		gql:     gql,
		owner:   owner,
		repo:    repo,
		columns: columns,
	}
}

// FetchBoard retrieves open issues and maps them to board columns, paging
// through gql.fetchIssuePage (a single GraphQL query per page fetches each
// issue's core fields plus its first 100 cross-referenced linked PRs).
//
// GraphQL's `issues` connection excludes pull requests by construction
// (unlike the REST Issues API, which mixes them in and requires a runtime
// PullRequestLinks check), so no PR-skip step is needed here.
//
// Issues with more than 100 cross-references (issue.hasMoreTimelineItems)
// trigger a bounded per-issue follow-up query via fetchTimelineFollowups.
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

	cursor := ""
	for {
		page, err := g.gql.fetchIssuePage(ctx, g.owner, g.repo, cursor)
		if err != nil {
			return Board{}, err
		}

		for _, issue := range page.issues {
			linkedPRs := issue.linkedPRs
			if issue.hasMoreTimelineItems {
				linkedPRs, err = g.fetchTimelineFollowups(ctx, issue)
				if err != nil {
					return Board{}, err
				}
			}

			// Find the furthest (rightmost) column matching any label.
			bestIdx := -1
			for _, label := range issue.labels {
				if idx, ok := colIndex[strings.ToLower(label.Name)]; ok {
					if idx > bestIdx {
						bestIdx = idx
					}
				}
			}

			// No matching label: place in first column.
			if bestIdx < 0 {
				bestIdx = 0
			}

			card := Card{
				Number:    issue.number,
				Title:     issue.title,
				Body:      issue.body,
				URL:       issue.url,
				Labels:    issue.labels,
				Assignees: issue.assignees,
				LinkedPRs: linkedPRs,
			}
			columns[bestIdx].Cards = append(columns[bestIdx].Cards, card)
		}

		if !page.hasNextPage {
			break
		}
		cursor = page.endCursor
	}

	return Board{Columns: columns}, nil
}

// fetchTimelineFollowups pages a single issue's timelineItems connection
// beyond its first page, merging follow-up LinkedPRs onto the issue's
// already-collected list. PR-number dedup mirrors mapLinkedPRs' pattern
// (graphql.go) so a PR cross-referenced on both the initial and a follow-up
// page is only counted once.
//
// Bounded by maxTimelineFollowupPages: if the cap is reached, the loop stops
// and returns whatever LinkedPRs were collected so far WITHOUT an error --
// one pathological issue must not fail the whole board fetch. A genuine
// error from fetchIssueTimelinePage (network/API failure) is returned
// immediately and DOES fail the board fetch (via FetchBoard's caller), since
// it means we can no longer trust that issue's linked-PR list is complete.
func (g *GitHubProvider) fetchTimelineFollowups(ctx context.Context, issue issueNode) ([]LinkedPR, error) {
	linkedPRs := issue.linkedPRs
	seen := make(map[int]bool, len(linkedPRs))
	for _, pr := range linkedPRs {
		seen[pr.Number] = true
	}

	cursor := issue.timelineEndCursor
	hasNext := true
	for page := 0; hasNext && page < maxTimelineFollowupPages; page++ {
		tp, err := g.gql.fetchIssueTimelinePage(ctx, g.owner, g.repo, issue.number, cursor)
		if err != nil {
			return nil, err
		}
		for _, pr := range tp.linkedPRs {
			if seen[pr.Number] {
				continue
			}
			seen[pr.Number] = true
			linkedPRs = append(linkedPRs, pr)
		}
		hasNext = tp.hasNextPage
		cursor = tp.endCursor
	}

	return linkedPRs, nil
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

// ListLabels returns the names of every label defined in the repository,
// paginating through all pages.
func (g *GitHubProvider) ListLabels(ctx context.Context) ([]string, error) {
	var allLabels []string
	opts := &github.ListOptions{PerPage: 100}
	for {
		labels, resp, err := g.client.ListLabels(ctx, g.owner, g.repo, opts)
		if err != nil {
			return nil, err
		}
		for _, label := range labels {
			allLabels = append(allLabels, label.GetName())
		}
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	if allLabels == nil {
		allLabels = []string{}
	}
	return allLabels, nil
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
			return fmt.Errorf("label %q already exists: %w", name, ErrLabelExists)
		}
		return err
	}
	return nil
}
