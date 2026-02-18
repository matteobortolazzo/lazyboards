package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-github/v68/github"
)

// mockIssuesClient implements GitHubIssuesClient with configurable return values.
type mockIssuesClient struct {
	issues         []*github.Issue
	err            error
	createdIssue   *github.Issue // returned by Create
	createErr      error         // returned by Create
	timelineEvents map[int][]*github.Timeline // keyed by issue number
	timelineErr    error
	capturedOpts   *github.IssueListByRepoOptions // captured from ListByRepo
}

func (m *mockIssuesClient) ListByRepo(
	_ context.Context,
	_ string,
	_ string,
	opts *github.IssueListByRepoOptions,
) ([]*github.Issue, *github.Response, error) {
	m.capturedOpts = opts
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

func (m *mockIssuesClient) ListIssueTimeline(
	_ context.Context,
	_ string,
	_ string,
	number int,
	_ *github.ListOptions,
) ([]*github.Timeline, *github.Response, error) {
	return m.timelineEvents[number], nil, m.timelineErr
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
	if len(card.Labels) == 0 || card.Labels[0] != issueLabel {
		t.Errorf("card.Labels = %v, want [%q]", card.Labels, issueLabel)
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
	if len(card.Labels) != 1 || card.Labels[0] != "nonexistent" {
		t.Errorf("card.Labels = %v, want [\"nonexistent\"] (all issue labels collected)", card.Labels)
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

func TestGitHubFetchBoard_MultipleLabels_FurthestColumnWins(t *testing.T) {
	columns := []string{"New", "In Progress", "Done"}
	// Issue has labels ["In Progress", "Done"] — the closer column label is listed
	// first in the API response. With furthest-match semantics, the card should
	// land in "Done" (column index 2), not "In Progress" (column index 1).
	issues := []*github.Issue{
		makeIssue(5, "Multi-labeled issue", "In Progress", "Done"),
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

	// The card should be in "Done" (the furthest matching column), not "In Progress".
	doneCol := board.Columns[2]
	if len(doneCol.Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", doneCol.Title, len(doneCol.Cards))
	}
	if doneCol.Cards[0].Title != "Multi-labeled issue" {
		t.Errorf("Done column card = %q, want %q", doneCol.Cards[0].Title, "Multi-labeled issue")
	}

	// The card's Labels field should contain ALL labels from the issue.
	cardLabels := doneCol.Cards[0].Labels
	if len(cardLabels) != 2 {
		t.Fatalf("card.Labels has %d entries, want 2", len(cardLabels))
	}
	if cardLabels[0] != "In Progress" || cardLabels[1] != "Done" {
		t.Errorf("card.Labels = %v, want [\"In Progress\", \"Done\"]", cardLabels)
	}

	// Other columns should be empty.
	if len(board.Columns[0].Cards) != 0 {
		t.Errorf("column %q has %d cards, want 0", columns[0], len(board.Columns[0].Cards))
	}
	if len(board.Columns[1].Cards) != 0 {
		t.Errorf("column %q has %d cards, want 0", columns[1], len(board.Columns[1].Cards))
	}
}

func TestGitHubFetchBoard_MultipleLabels_NonMatchingLabelsIgnored(t *testing.T) {
	columns := []string{"New", "In Progress", "Done"}
	// Issue has labels ["unrelated-label", "New", "Done"] — a mix of non-matching
	// and matching labels. "New" matches column index 0 and "Done" matches column
	// index 2. The non-matching label should be ignored for placement, and the card
	// should land in "Done" (the furthest matching column).
	issues := []*github.Issue{
		makeIssue(6, "Mixed labels issue", "unrelated-label", "New", "Done"),
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

	// The card should be in "Done" (the furthest matching column).
	doneCol := board.Columns[2]
	if len(doneCol.Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", doneCol.Title, len(doneCol.Cards))
	}
	if doneCol.Cards[0].Title != "Mixed labels issue" {
		t.Errorf("Done column card = %q, want %q", doneCol.Cards[0].Title, "Mixed labels issue")
	}

	// The card's Labels field should contain ALL labels from the issue.
	cardLabels := doneCol.Cards[0].Labels
	if len(cardLabels) != 3 {
		t.Fatalf("card.Labels has %d entries, want 3", len(cardLabels))
	}
	if cardLabels[0] != "unrelated-label" || cardLabels[1] != "New" || cardLabels[2] != "Done" {
		t.Errorf("card.Labels = %v, want [\"unrelated-label\", \"New\", \"Done\"]", cardLabels)
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

func TestGitHubCreateCard_WithLabel(t *testing.T) {
	expectedNumber := 42
	expectedTitle := "New feature"
	expectedLabel := "bug"

	client := &mockIssuesClient{
		createdIssue: makeIssue(expectedNumber, expectedTitle, expectedLabel),
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, "owner", "repo", columns)

	card, err := provider.CreateCard(context.Background(), expectedTitle, expectedLabel)
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	if card.Number != expectedNumber {
		t.Errorf("card.Number = %d, want %d", card.Number, expectedNumber)
	}
	if card.Title != expectedTitle {
		t.Errorf("card.Title = %q, want %q", card.Title, expectedTitle)
	}
	if len(card.Labels) == 0 || card.Labels[0] != expectedLabel {
		t.Errorf("card.Labels = %v, want [%q]", card.Labels, expectedLabel)
	}
}

func TestGitHubCreateCard_WithoutLabel(t *testing.T) {
	expectedNumber := 7
	expectedTitle := "No label issue"

	client := &mockIssuesClient{
		createdIssue: makeIssue(expectedNumber, expectedTitle),
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, "owner", "repo", columns)

	card, err := provider.CreateCard(context.Background(), expectedTitle, "")
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	if card.Number != expectedNumber {
		t.Errorf("card.Number = %d, want %d", card.Number, expectedNumber)
	}
	if card.Title != expectedTitle {
		t.Errorf("card.Title = %q, want %q", card.Title, expectedTitle)
	}
	if len(card.Labels) != 0 {
		t.Errorf("card.Labels = %v, want empty slice", card.Labels)
	}
}

func TestGitHubCreateCard_InvalidLabel_ReturnsFriendlyError(t *testing.T) {
	invalidLabel := "nonexistent"
	client := &mockIssuesClient{
		createErr: &github.ErrorResponse{
			Response: &http.Response{StatusCode: 422},
			Message:  "Validation Failed",
		},
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, "owner", "repo", columns)

	_, err := provider.CreateCard(context.Background(), "title", invalidLabel)
	if err == nil {
		t.Fatal("expected error from CreateCard with invalid label, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, invalidLabel) {
		t.Errorf("error = %q, want it to contain the invalid label %q", errMsg, invalidLabel)
	}
	if !strings.Contains(errMsg, "does not exist") {
		t.Errorf("error = %q, want it to contain %q", errMsg, "does not exist")
	}
}

func TestGitHubCreateCard_GenericAPIError_PassesThrough(t *testing.T) {
	apiErrMsg := "server error"
	client := &mockIssuesClient{
		createErr: errors.New(apiErrMsg),
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, "owner", "repo", columns)

	_, err := provider.CreateCard(context.Background(), "title", "label")
	if err == nil {
		t.Fatal("expected error from CreateCard, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}
}

// --- Card Body from GitHub Issue Body ---

func TestGitHubFetchBoard_CardBodyPopulatedFromIssueBody(t *testing.T) {
	columns := []string{"Todo"}
	issueBody := "This is the issue description.\nIt has multiple lines."
	issue := makeIssue(1, "Issue with body", "Todo")
	issue.Body = github.Ptr(issueBody)

	client := &mockIssuesClient{issues: []*github.Issue{issue}}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if card.Body != issueBody {
		t.Errorf("card.Body = %q, want %q", card.Body, issueBody)
	}
}

func TestGitHubFetchBoard_NilIssueBody_ResultsInEmptyCardBody(t *testing.T) {
	columns := []string{"Todo"}
	// makeIssue does not set Body, so issue.Body is nil.
	issue := makeIssue(1, "Issue without body", "Todo")

	client := &mockIssuesClient{issues: []*github.Issue{issue}}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if card.Body != "" {
		t.Errorf("card.Body = %q, want empty string for nil issue body", card.Body)
	}
}

// --- Linked PRs from Timeline API ---

// makeCrossReferencedPREvent builds a Timeline event that represents
// a cross-referenced pull request (a PR that references the issue).
func makeCrossReferencedPREvent(prNumber int, prTitle, prHTMLURL string) *github.Timeline {
	return &github.Timeline{
		Event: github.Ptr("cross-referenced"),
		Source: &github.Source{
			Issue: &github.Issue{
				Number:           github.Ptr(prNumber),
				Title:            github.Ptr(prTitle),
				HTMLURL:          github.Ptr(prHTMLURL),
				PullRequestLinks: &github.PullRequestLinks{URL: github.Ptr(prHTMLURL)},
			},
		},
	}
}

// makeCrossReferencedIssueEvent builds a Timeline event that represents
// a cross-referenced regular issue (not a PR).
func makeCrossReferencedIssueEvent(issueNumber int, issueTitle string) *github.Timeline {
	return &github.Timeline{
		Event: github.Ptr("cross-referenced"),
		Source: &github.Source{
			Issue: &github.Issue{
				Number: github.Ptr(issueNumber),
				Title:  github.Ptr(issueTitle),
				// No PullRequestLinks — this is a regular issue, not a PR.
			},
		},
	}
}

func TestGitHubFetchBoard_LinkedPRsPopulatedFromTimeline(t *testing.T) {
	columns := []string{"Todo"}
	issue := makeIssue(10, "Bug report", "Todo")

	prNumber := 89
	prTitle := "Fix bug"
	prURL := "https://github.com/owner/repo/pull/89"

	client := &mockIssuesClient{
		issues: []*github.Issue{issue},
		timelineEvents: map[int][]*github.Timeline{
			10: {makeCrossReferencedPREvent(prNumber, prTitle, prURL)},
		},
	}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if len(card.LinkedPRs) != 1 {
		t.Fatalf("card.LinkedPRs has %d entries, want 1", len(card.LinkedPRs))
	}

	linkedPR := card.LinkedPRs[0]
	if linkedPR.Number != prNumber {
		t.Errorf("LinkedPR.Number = %d, want %d", linkedPR.Number, prNumber)
	}
	if linkedPR.Title != prTitle {
		t.Errorf("LinkedPR.Title = %q, want %q", linkedPR.Title, prTitle)
	}
	if linkedPR.URL != prURL {
		t.Errorf("LinkedPR.URL = %q, want %q", linkedPR.URL, prURL)
	}
}

func TestGitHubFetchBoard_NoLinkedPRs_EmptyTimeline(t *testing.T) {
	columns := []string{"Todo"}
	issue := makeIssue(20, "Solo issue", "Todo")

	client := &mockIssuesClient{
		issues: []*github.Issue{issue},
		timelineEvents: map[int][]*github.Timeline{
			20: {}, // empty timeline
		},
	}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if len(card.LinkedPRs) != 0 {
		t.Errorf("card.LinkedPRs has %d entries, want 0", len(card.LinkedPRs))
	}
}

func TestGitHubFetchBoard_CrossReferencedNonPR_Ignored(t *testing.T) {
	columns := []string{"Todo"}
	issue := makeIssue(30, "Referenced issue", "Todo")

	client := &mockIssuesClient{
		issues: []*github.Issue{issue},
		timelineEvents: map[int][]*github.Timeline{
			30: {makeCrossReferencedIssueEvent(31, "Related issue")},
		},
	}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if len(card.LinkedPRs) != 0 {
		t.Errorf("card.LinkedPRs has %d entries, want 0 (non-PR cross-reference should be ignored)", len(card.LinkedPRs))
	}
}

func TestGitHubFetchBoard_MultipleLinkedPRs(t *testing.T) {
	columns := []string{"Todo"}
	issue := makeIssue(40, "Complex issue", "Todo")

	client := &mockIssuesClient{
		issues: []*github.Issue{issue},
		timelineEvents: map[int][]*github.Timeline{
			40: {
				makeCrossReferencedPREvent(50, "First fix", "https://github.com/owner/repo/pull/50"),
				makeCrossReferencedPREvent(51, "Second fix", "https://github.com/owner/repo/pull/51"),
			},
		},
	}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if len(card.LinkedPRs) != 2 {
		t.Fatalf("card.LinkedPRs has %d entries, want 2", len(card.LinkedPRs))
	}

	// Verify both PRs are present (order should match timeline order).
	if card.LinkedPRs[0].Number != 50 {
		t.Errorf("LinkedPRs[0].Number = %d, want 50", card.LinkedPRs[0].Number)
	}
	if card.LinkedPRs[1].Number != 51 {
		t.Errorf("LinkedPRs[1].Number = %d, want 51", card.LinkedPRs[1].Number)
	}
}

func TestGitHubFetchBoard_DuplicateLinkedPRs_Deduplicated(t *testing.T) {
	columns := []string{"Todo"}
	issue := makeIssue(45, "Issue with duplicate cross-refs", "Todo")

	// Same PR cross-referenced multiple times in timeline.
	prEvent := makeCrossReferencedPREvent(60, "Fix", "https://github.com/owner/repo/pull/60")

	client := &mockIssuesClient{
		issues: []*github.Issue{issue},
		timelineEvents: map[int][]*github.Timeline{
			45: {prEvent, prEvent, prEvent},
		},
	}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	card := board.Columns[0].Cards[0]
	if len(card.LinkedPRs) != 1 {
		t.Fatalf("card.LinkedPRs has %d entries, want 1 (duplicates should be deduplicated)", len(card.LinkedPRs))
	}
	if card.LinkedPRs[0].Number != 60 {
		t.Errorf("LinkedPR.Number = %d, want 60", card.LinkedPRs[0].Number)
	}
}

func TestGitHubFetchBoard_RequestsIssuesSortedByCreatedAsc(t *testing.T) {
	columns := []string{"Todo"}
	client := &mockIssuesClient{issues: []*github.Issue{}}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	_, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if client.capturedOpts == nil {
		t.Fatal("expected ListByRepo to be called with options, got nil")
	}
	if client.capturedOpts.Sort != "created" {
		t.Errorf("opts.Sort = %q, want %q", client.capturedOpts.Sort, "created")
	}
	if client.capturedOpts.Direction != "asc" {
		t.Errorf("opts.Direction = %q, want %q", client.capturedOpts.Direction, "asc")
	}
}

func TestGitHubFetchBoard_TimelineAPIError_ReturnsError(t *testing.T) {
	columns := []string{"Todo"}
	issue := makeIssue(50, "Issue", "Todo")

	apiErr := errors.New("timeline API rate limit exceeded")
	client := &mockIssuesClient{
		issues:      []*github.Issue{issue},
		timelineErr: apiErr,
	}
	provider := NewGitHubProvider(client, "owner", "repo", columns)

	_, err := provider.FetchBoard(context.Background())
	if err == nil {
		t.Fatal("expected error from FetchBoard when timeline API fails, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "rate limit")
	}
}
