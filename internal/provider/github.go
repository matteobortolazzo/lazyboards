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
			}

			// Find the first label that matches a column.
			matched := false
			for _, label := range issue.Labels {
				labelName := label.GetName()
				if idx, ok := colIndex[strings.ToLower(labelName)]; ok {
					card.Label = labelName
					columns[idx].Cards = append(columns[idx].Cards, card)
					matched = true
					break
				}
			}

			// No matching label: place in first column with empty label.
			if !matched {
				columns[0].Cards = append(columns[0].Cards, card)
			}
		}

		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return Board{Columns: columns}, nil
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
	}
	if label != "" {
		card.Label = label
	}

	return card, nil
}
