package provider

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-github/v68/github"
)

// mockIssuesClient implements GitHubIssuesClient with configurable return values.
type mockIssuesClient struct {
	issues       []*github.Issue
	err          error
	createdIssue *github.Issue
	createErr    error
}

func (m *mockIssuesClient) ListByRepo(
	_ context.Context,
	_ string,
	_ string,
	_ *github.IssueListByRepoOptions,
) ([]*github.Issue, *github.Response, error) {
	return m.issues, nil, m.err
}

func (m *mockIssuesClient) Create(
	_ context.Context,
	_ string,
	_ string,
	_ *github.IssueRequest,
) (*github.Issue, *github.Response, error) {
	return m.createdIssue, nil, m.createErr
}

// makeIssue builds a github.Issue with the given number, title, and label names.
func makeIssue(number int, title string, labels ...string) *github.Issue {
	ghLabels := make([]*github.Label, len(labels))
	for i, name := range labels {
		ghLabels[i] = &github.Label{Name: github.Ptr(name)}
	}
	return &github.Issue{
		Number: github.Ptr(number),
		Title:  github.Ptr(title),
		Labels: ghLabels,
	}
}

func TestGitHubFetchBoard_SortsIssuesToMatchingColumns(t *testing.T) {
	columns := []string{"New", "In Progress", "Done"}
	issues := []*github.Issue{
		makeIssue(1, "Setup project", "New"),
		makeIssue(2, "Implement feature", "In Progress"),
		makeIssue(3, "Deploy to prod", "Done"),
	}

	client := &mockIssuesClient{issues: issues}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns) != len(columns) {
		t.Fatalf("got %d columns, want %d", len(board.Columns), len(columns))
	}

	// Verify column titles match configured columns.
	for i, col := range board.Columns {
		if col.Title != columns[i] {
			t.Errorf("column %d title = %q, want %q", i, col.Title, columns[i])
		}
	}

	// "New" column should have issue #1
	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}
	if board.Columns[0].Cards[0].Title != "Setup project" {
		t.Errorf("column %q card title = %q, want %q", columns[0], board.Columns[0].Cards[0].Title, "Setup project")
	}

	// "In Progress" column should have issue #2
	if len(board.Columns[1].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[1], len(board.Columns[1].Cards))
	}
	if board.Columns[1].Cards[0].Title != "Implement feature" {
		t.Errorf("column %q card title = %q, want %q", columns[1], board.Columns[1].Cards[0].Title, "Implement feature")
	}

	// "Done" column should have issue #3
	if len(board.Columns[2].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[2], len(board.Columns[2].Cards))
	}
	if board.Columns[2].Cards[0].Title != "Deploy to prod" {
		t.Errorf("column %q card title = %q, want %q", columns[2], board.Columns[2].Cards[0].Title, "Deploy to prod")
	}
}

func TestGitHubFetchBoard_UnlabeledIssuesGoToFirstColumn(t *testing.T) {
	columns := []string{"Backlog", "Working", "Review"}
	issues := []*github.Issue{
		makeIssue(10, "No labels at all"),                     // no labels
		makeIssue(11, "Unrecognized label", "nonexistent"),    // label doesn't match any column
		makeIssue(12, "Recognized label goes elsewhere", "Review"), // should go to "Review"
	}

	client := &mockIssuesClient{issues: issues}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns) != len(columns) {
		t.Fatalf("got %d columns, want %d", len(board.Columns), len(columns))
	}

	// First column ("Backlog") should contain the unlabeled and unrecognized issues.
	backlog := board.Columns[0]
	if len(backlog.Cards) != 2 {
		t.Fatalf("column %q has %d cards, want 2", backlog.Title, len(backlog.Cards))
	}

	// Verify the specific issues landed in the first column.
	titles := map[string]bool{}
	for _, card := range backlog.Cards {
		titles[card.Title] = true
	}
	if !titles["No labels at all"] {
		t.Error("issue with no labels not found in first column")
	}
	if !titles["Unrecognized label"] {
		t.Error("issue with non-matching label not found in first column")
	}

	// "Review" column should have the recognized issue.
	review := board.Columns[2]
	if len(review.Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", review.Title, len(review.Cards))
	}
	if review.Cards[0].Title != "Recognized label goes elsewhere" {
		t.Errorf("review card title = %q, want %q", review.Cards[0].Title, "Recognized label goes elsewhere")
	}
}

func TestGitHubFetchBoard_CardFieldsPopulated(t *testing.T) {
	columns := []string{"Todo", "Doing"}
	issueNumber := 42
	issueTitle := "Fix the widget"
	issueLabel := "Doing"

	issues := []*github.Issue{
		makeIssue(issueNumber, issueTitle, issueLabel),
	}

	client := &mockIssuesClient{issues: issues}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns) != len(columns) {
		t.Fatalf("got %d columns, want %d", len(board.Columns), len(columns))
	}

	// Issue should be in "Doing" column (index 1).
	doingCol := board.Columns[1]
	if len(doingCol.Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", doingCol.Title, len(doingCol.Cards))
	}

	card := doingCol.Cards[0]
	if card.Number != issueNumber {
		t.Errorf("card.Number = %d, want %d", card.Number, issueNumber)
	}
	if card.Title != issueTitle {
		t.Errorf("card.Title = %q, want %q", card.Title, issueTitle)
	}
	if card.Label != issueLabel {
		t.Errorf("card.Label = %q, want %q", card.Label, issueLabel)
	}
}

func TestGitHubFetchBoard_CardFieldsPopulated_UnmatchedLabel(t *testing.T) {
	columns := []string{"Todo", "Doing"}
	issues := []*github.Issue{
		makeIssue(7, "Orphan issue", "nonexistent"),
	}

	client := &mockIssuesClient{issues: issues}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns) != len(columns) {
		t.Fatalf("got %d columns, want %d", len(board.Columns), len(columns))
	}

	// Unmatched label issue goes to first column.
	todoCol := board.Columns[0]
	if len(todoCol.Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", todoCol.Title, len(todoCol.Cards))
	}

	card := todoCol.Cards[0]
	if card.Label != "" {
		t.Errorf("card.Label = %q, want empty string for unmatched label", card.Label)
	}
}

func TestGitHubFetchBoard_NoIssues_ReturnsEmptyColumns(t *testing.T) {
	columns := []string{"New", "In Progress", "Done"}

	client := &mockIssuesClient{issues: []*github.Issue{}}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns) != len(columns) {
		t.Fatalf("got %d columns, want %d", len(board.Columns), len(columns))
	}

	for i, col := range board.Columns {
		if col.Title != columns[i] {
			t.Errorf("column %d title = %q, want %q", i, col.Title, columns[i])
		}
		if len(col.Cards) != 0 {
			t.Errorf("column %q has %d cards, want 0", col.Title, len(col.Cards))
		}
	}
}

func TestGitHubFetchBoard_APIError_ReturnsError(t *testing.T) {
	apiErr := errors.New("GitHub API rate limit exceeded")
	client := &mockIssuesClient{err: apiErr}
	columns := []string{"New", "Done"}

	provider := NewGitHubProvider(client, "owner", "repo", columns)

	_, err := provider.FetchBoard(context.Background())
	if err == nil {
		t.Fatal("expected error from FetchBoard, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "rate limit")
	}
}

func TestGitHubFetchBoard_LabelMatchingIsCaseInsensitive(t *testing.T) {
	columns := []string{"bug", "feature", "docs"}
	issues := []*github.Issue{
		makeIssue(1, "Uppercase label", "BUG"),
		makeIssue(2, "Mixed case label", "Feature"),
		makeIssue(3, "Lowercase label", "docs"),
	}

	client := &mockIssuesClient{issues: issues}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns) != len(columns) {
		t.Fatalf("got %d columns, want %d", len(board.Columns), len(columns))
	}

	// "bug" column should have issue #1 (label "BUG").
	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}
	if board.Columns[0].Cards[0].Title != "Uppercase label" {
		t.Errorf("bug column card = %q, want %q", board.Columns[0].Cards[0].Title, "Uppercase label")
	}

	// "feature" column should have issue #2 (label "Feature").
	if len(board.Columns[1].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[1], len(board.Columns[1].Cards))
	}
	if board.Columns[1].Cards[0].Title != "Mixed case label" {
		t.Errorf("feature column card = %q, want %q", board.Columns[1].Cards[0].Title, "Mixed case label")
	}

	// "docs" column should have issue #3 (label "docs").
	if len(board.Columns[2].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[2], len(board.Columns[2].Cards))
	}
	if board.Columns[2].Cards[0].Title != "Lowercase label" {
		t.Errorf("docs column card = %q, want %q", board.Columns[2].Cards[0].Title, "Lowercase label")
	}
}

func TestGitHubFetchBoard_MultipleLabels_FirstMatchWins(t *testing.T) {
	columns := []string{"New", "In Progress", "Done"}
	// Issue has labels ["Done", "In Progress"] — iterating the issue's labels,
	// "Done" matches column index 2, and "In Progress" matches column index 1.
	// The first matching label encountered ("Done") should win.
	issues := []*github.Issue{
		makeIssue(5, "Multi-labeled issue", "Done", "In Progress"),
	}

	client := &mockIssuesClient{issues: issues}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns) != len(columns) {
		t.Fatalf("got %d columns, want %d", len(board.Columns), len(columns))
	}

	// "Done" is the first matching label, so the issue should be in "Done" column.
	doneCol := board.Columns[2]
	if len(doneCol.Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", doneCol.Title, len(doneCol.Cards))
	}
	if doneCol.Cards[0].Title != "Multi-labeled issue" {
		t.Errorf("Done column card = %q, want %q", doneCol.Cards[0].Title, "Multi-labeled issue")
	}

	// The card's Label field should be "Done" (the first matching label).
	if doneCol.Cards[0].Label != "Done" {
		t.Errorf("card.Label = %q, want %q", doneCol.Cards[0].Label, "Done")
	}

	// Other columns should be empty.
	if len(board.Columns[0].Cards) != 0 {
		t.Errorf("column %q has %d cards, want 0", columns[0], len(board.Columns[0].Cards))
	}
	if len(board.Columns[1].Cards) != 0 {
		t.Errorf("column %q has %d cards, want 0", columns[1], len(board.Columns[1].Cards))
	}
}

func TestGitHubFetchBoard_EmptyColumnsReturnsError(t *testing.T) {
	client := &mockIssuesClient{}
	provider := NewGitHubProvider(client, "owner", "repo", nil)

	_, err := provider.FetchBoard(context.Background())
	if err == nil {
		t.Fatal("expected error from FetchBoard with no columns, got nil")
	}
	if !strings.Contains(err.Error(), "column") {
		t.Errorf("error = %q, want it to mention columns", err.Error())
	}
}

func TestGitHubFetchBoard_SkipsPullRequests(t *testing.T) {
	columns := []string{"Open"}
	prURL := "https://api.github.com/repos/owner/repo/pulls/2"
	issues := []*github.Issue{
		makeIssue(1, "Real issue", "Open"),
		{
			Number:           github.Ptr(2),
			Title:            github.Ptr("A pull request"),
			PullRequestLinks: &github.PullRequestLinks{URL: &prURL},
		},
	}

	client := &mockIssuesClient{issues: issues}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	// Only the real issue should appear, not the PR.
	totalCards := 0
	for _, col := range board.Columns {
		totalCards += len(col.Cards)
	}
	if totalCards != 1 {
		t.Fatalf("got %d total cards, want 1 (PR should be filtered out)", totalCards)
	}
	if board.Columns[0].Cards[0].Title != "Real issue" {
		t.Errorf("card title = %q, want %q", board.Columns[0].Cards[0].Title, "Real issue")
	}
}

func TestGitHubCreateCard_Success_WithLabel(t *testing.T) {
	client := &mockIssuesClient{
		createdIssue: makeIssue(42, "New feature", "backlog"),
	}
	columns := []string{"Backlog", "Done"}

	provider := NewGitHubProvider(client, "owner", "repo", columns)

	card, err := provider.CreateCard(context.Background(), "New feature", "backlog")
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	if card.Number != 42 {
		t.Errorf("card.Number = %d, want 42", card.Number)
	}
	if card.Title != "New feature" {
		t.Errorf("card.Title = %q, want %q", card.Title, "New feature")
	}
	if card.Label != "backlog" {
		t.Errorf("card.Label = %q, want %q", card.Label, "backlog")
	}
}

func TestGitHubCreateCard_Success_WithoutLabel(t *testing.T) {
	client := &mockIssuesClient{
		createdIssue: makeIssue(7, "Quick fix"),
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, "owner", "repo", columns)

	card, err := provider.CreateCard(context.Background(), "Quick fix", "")
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}
	if card.Number != 7 {
		t.Errorf("card.Number = %d, want 7", card.Number)
	}
	if card.Title != "Quick fix" {
		t.Errorf("card.Title = %q, want %q", card.Title, "Quick fix")
	}
	if card.Label != "" {
		t.Errorf("card.Label = %q, want empty string", card.Label)
	}
}

func TestGitHubCreateCard_APIError_ReturnsError(t *testing.T) {
	client := &mockIssuesClient{
		createErr: errors.New("authentication required"),
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, "owner", "repo", columns)

	_, err := provider.CreateCard(context.Background(), "Some title", "some-label")
	if err == nil {
		t.Fatal("expected error from CreateCard, got nil")
	}
	if !strings.Contains(err.Error(), "authentication") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "authentication")
	}
}
