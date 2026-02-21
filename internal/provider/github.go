package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/go-github/v68/github"
)

// Compile-time check: *GitHubProvider implements BoardProvider.
var _ BoardProvider = (*GitHubProvider)(nil)

// GitHubIssuesClient abstracts the GitHub Issues API for testing.
type GitHubIssuesClient interface {
	ListByRepo(ctx context.Context, owner string, repo string, opts *github.IssueListByRepoOptions) ([]*github.Issue, *github.Response, error)
	Create(ctx context.Context, owner string, repo string, issue *github.IssueRequest) (*github.Issue, *github.Response, error)
	ListIssueTimeline(ctx context.Context, owner string, repo string, number int, opts *github.ListOptions) ([]*github.Timeline, *github.Response, error)
}

// GitHubProvider fetches board data from GitHub Issues.
type GitHubProvider struct {
	client  GitHubIssuesClient
	owner   string
	repo    string
	columns []string
}

// NewGitHubProvider creates a GitHubProvider with the given client, repository, and column names.
func NewGitHubProvider(client GitHubIssuesClient, owner, repo string, columns []string) *GitHubProvider {
	return &GitHubProvider{
		client:  client,
		owner:   owner,
		repo:    repo,
		columns: columns,
	}
}

// FetchBoard retrieves open issues and maps them to board columns.
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

	// Fetch all open issues with pagination.
	opts := &github.IssueListByRepoOptions{
		State:       "open",
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

			card := Card{
				Number: issue.GetNumber(),
				Title:  issue.GetTitle(),
				Body:   issue.GetBody(),
			}

			// Collect all label names.
			allLabels := make([]string, 0, len(issue.Labels))
			for _, label := range issue.Labels {
				allLabels = append(allLabels, label.GetName())
			}
			if len(allLabels) > 0 {
				card.Labels = allLabels
			}

			// Fetch timeline events to find linked PRs.
			linkedPRs, err := g.fetchLinkedPRs(ctx, issue.GetNumber())
			if err != nil {
				return Board{}, err
			}
			card.LinkedPRs = linkedPRs

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
		}

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return Board{Columns: columns}, nil
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
		if errors.As(err, &ghErr) && ghErr.Response.StatusCode == 422 {
			return Card{}, fmt.Errorf("label %q does not exist in the repository", label)
		}
		return Card{}, err
	}

	card := Card{
		Number: issue.GetNumber(),
		Title:  issue.GetTitle(),
		Body:   issue.GetBody(),
	}
	if label != "" {
		card.Labels = []string{label}
	}

	return card, nil
}
