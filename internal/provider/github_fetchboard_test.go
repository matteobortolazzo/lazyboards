package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v68/github"
)

// singlePageGQL wraps the given issueNodes in a fakeGraphQLClient that
// returns them as a single, non-paginated page -- the common case for
// FetchBoard tests that don't need to exercise outer pagination itself.
func singlePageGQL(issues ...issueNode) *fakeGraphQLClient {
	return &fakeGraphQLClient{pages: map[string]issuePage{"": {issues: issues}}}
}

func TestGitHubFetchBoard_SortsIssuesToMatchingColumns(t *testing.T) {
	columns := []string{"New", "In Progress", "Done"}
	issues := []issueNode{
		buildIssueNode(1, "Setup project", "New"),
		buildIssueNode(2, "Implement feature", "In Progress"),
		buildIssueNode(3, "Deploy to prod", "Done"),
	}

	gql := singlePageGQL(issues...)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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
	issues := []issueNode{
		buildIssueNode(10, "No labels at all"),                          // no labels
		buildIssueNode(11, "Unrecognized label", "nonexistent"),         // label doesn't match any column
		buildIssueNode(12, "Recognized label goes elsewhere", "Review"), // should go to "Review"
	}

	gql := singlePageGQL(issues...)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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

	issues := []issueNode{
		buildIssueNode(issueNumber, issueTitle, issueLabel),
	}

	gql := singlePageGQL(issues...)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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
	if len(card.Labels) == 0 || card.Labels[0].Name != issueLabel {
		t.Errorf("card.Labels = %v, want [%q]", card.Labels, issueLabel)
	}
}

func TestGitHubFetchBoard_CardFieldsPopulated_UnmatchedLabel(t *testing.T) {
	columns := []string{"Todo", "Doing"}
	issues := []issueNode{
		buildIssueNode(7, "Orphan issue", "nonexistent"),
	}

	gql := singlePageGQL(issues...)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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
	if len(card.Labels) != 1 || card.Labels[0].Name != "nonexistent" {
		t.Errorf("card.Labels = %v, want [\"nonexistent\"] (all issue labels collected)", card.Labels)
	}
}

func TestGitHubFetchBoard_NoIssues_ReturnsEmptyColumns(t *testing.T) {
	columns := []string{"New", "In Progress", "Done"}

	gql := singlePageGQL()
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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
	gql := &fakeGraphQLClient{err: apiErr}
	columns := []string{"New", "Done"}

	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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
	issues := []issueNode{
		buildIssueNode(1, "Uppercase label", "BUG"),
		buildIssueNode(2, "Mixed case label", "Feature"),
		buildIssueNode(3, "Lowercase label", "docs"),
	}

	gql := singlePageGQL(issues...)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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
	issues := []issueNode{
		buildIssueNode(5, "Multi-labeled issue", "In Progress", "Done"),
	}

	gql := singlePageGQL(issues...)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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
	if cardLabels[0].Name != "In Progress" || cardLabels[1].Name != "Done" {
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
	issues := []issueNode{
		buildIssueNode(6, "Mixed labels issue", "unrelated-label", "New", "Done"),
	}

	gql := singlePageGQL(issues...)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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
	if cardLabels[0].Name != "unrelated-label" || cardLabels[1].Name != "New" || cardLabels[2].Name != "Done" {
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
	client := &mockGitHubClient{}
	provider := NewGitHubProvider(client, nil, "owner", "repo", nil)

	_, err := provider.FetchBoard(context.Background())
	if err == nil {
		t.Fatal("expected error from FetchBoard with no columns, got nil")
	}
	if !strings.Contains(err.Error(), "column") {
		t.Errorf("error = %q, want it to mention columns", err.Error())
	}
}

// NOTE: TestGitHubFetchBoard_SkipsPullRequests (the REST-era PR-skip test)
// was removed during the GraphQL migration. GraphQL's `issues` connection
// excludes pull requests by construction (unlike the REST Issues API, which
// mixes them in) -- issueNode has no PullRequestLinks-equivalent field, so
// there is no fixture that could represent "a PR leaked through" to assert
// against. See FetchBoard's doc comment for the structural explanation.

// --- Card Body from GitHub Issue Body ---

func TestGitHubFetchBoard_CardBodyPopulatedFromIssueBody(t *testing.T) {
	columns := []string{"Todo"}
	issueBody := "This is the issue description.\nIt has multiple lines."
	issue := buildIssueNode(1, "Issue with body", "Todo")
	issue.body = issueBody

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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
	// buildIssueNode does not set body, so issue.body is the zero value "".
	issue := buildIssueNode(1, "Issue without body", "Todo")

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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

// --- Linked PRs (GitHub closing-PR relationship) ---
//
// NOTE: TestGitHubFetchBoard_RequestsIssuesSortedByCreatedAsc and
// TestGitHubFetchBoard_ClosingPRAPIError_ReturnsError (the REST-era tests)
// were removed during the GraphQL migration. Sort order (CREATED_AT ASC) is
// now baked into issuesQuery's static GraphQL query string (graphql.go), not
// a runtime option fakeGraphQLClient can observe, so there's nothing left to
// assert through this boundary. And GraphQL fetches an issue and its first
// page of linked PRs in a single gql.fetchIssuePage call, so a "closing PR API
// error" is no longer distinguishable from the general
// TestGitHubFetchBoard_APIError_ReturnsError case above.

func TestGitHubFetchBoard_LinkedPRsPopulatedFromClosingPRs(t *testing.T) {
	columns := []string{"Todo"}
	issue := buildIssueNode(10, "Bug report", "Todo")

	prNumber := 89
	prTitle := "Fix bug"
	prURL := "https://github.com/owner/repo/pull/89"
	issue.linkedPRs = []LinkedPR{{Number: prNumber, Title: prTitle, URL: prURL}}

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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

func TestGitHubFetchBoard_NoLinkedPRs_EmptyClosingPRConnection(t *testing.T) {
	columns := []string{"Todo"}
	issue := buildIssueNode(20, "Solo issue", "Todo") // no linkedPRs set

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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

func TestGitHubFetchBoard_MultipleLinkedPRs(t *testing.T) {
	columns := []string{"Todo"}
	issue := buildIssueNode(40, "Complex issue", "Todo")
	issue.linkedPRs = []LinkedPR{
		{Number: 50, Title: "First fix", URL: "https://github.com/owner/repo/pull/50"},
		{Number: 51, Title: "Second fix", URL: "https://github.com/owner/repo/pull/51"},
	}

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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

	// Verify both PRs are present (order should match issueNode.linkedPRs order).
	if card.LinkedPRs[0].Number != 50 {
		t.Errorf("LinkedPRs[0].Number = %d, want 50", card.LinkedPRs[0].Number)
	}
	if card.LinkedPRs[1].Number != 51 {
		t.Errorf("LinkedPRs[1].Number = %d, want 51", card.LinkedPRs[1].Number)
	}
}

// TestGitHubFetchBoard_DuplicateLinkedPRs_Deduplicated confirms FetchBoard
// forwards issueNode.linkedPRs unchanged: the actual dedup by PR number now
// happens upstream in mapLinkedPRs, already pinned by
// TestMapLinkedPRs_DedupesByPRNumberWithinIssue (graphql_test.go).
func TestGitHubFetchBoard_DuplicateLinkedPRs_Deduplicated(t *testing.T) {
	columns := []string{"Todo"}
	issue := buildIssueNode(45, "Issue with duplicate cross-refs", "Todo")
	// mapLinkedPRs would have already deduped by PR number by the time
	// issueNode reaches FetchBoard, so only one entry survives here.
	issue.linkedPRs = []LinkedPR{{Number: 60, Title: "Fix", URL: "https://github.com/owner/repo/pull/60"}}

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

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

// --- Multi-Issue / Linked-PR Fetch Tests ---

func TestGitHubFetchBoard_MultipleIssues_AllLinkedPRsPopulated(t *testing.T) {
	columns := []string{"Todo", "Doing", "Done"}

	issue1 := buildIssueNode(1, "Issue one", "Todo")
	issue1.linkedPRs = []LinkedPR{{Number: 101, Title: "PR for issue 1", URL: "https://github.com/o/r/pull/101"}}

	issue2 := buildIssueNode(2, "Issue two", "Todo")
	issue2.linkedPRs = []LinkedPR{
		{Number: 102, Title: "PR for issue 2a", URL: "https://github.com/o/r/pull/102"},
		{Number: 103, Title: "PR for issue 2b", URL: "https://github.com/o/r/pull/103"},
	}

	issue3 := buildIssueNode(3, "Issue three", "Doing")
	issue3.linkedPRs = []LinkedPR{{Number: 104, Title: "PR for issue 3", URL: "https://github.com/o/r/pull/104"}}

	issue4 := buildIssueNode(4, "Issue four", "Doing") // no linked PRs

	issue5 := buildIssueNode(5, "Issue five", "Done")
	issue5.linkedPRs = []LinkedPR{{Number: 105, Title: "PR for issue 5", URL: "https://github.com/o/r/pull/105"}}

	gql := singlePageGQL(issue1, issue2, issue3, issue4, issue5)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	// Collect all cards across columns.
	cardsByNumber := make(map[int]Card)
	for _, col := range board.Columns {
		for _, card := range col.Cards {
			cardsByNumber[card.Number] = card
		}
	}

	if len(cardsByNumber) != 5 {
		t.Fatalf("got %d total cards, want 5", len(cardsByNumber))
	}

	// Issue 1: 1 linked PR.
	if len(cardsByNumber[1].LinkedPRs) != 1 || cardsByNumber[1].LinkedPRs[0].Number != 101 {
		t.Errorf("issue 1 LinkedPRs = %v, want [{Number:101 ...}]", cardsByNumber[1].LinkedPRs)
	}

	// Issue 2: 2 linked PRs.
	if len(cardsByNumber[2].LinkedPRs) != 2 {
		t.Errorf("issue 2 LinkedPRs has %d entries, want 2", len(cardsByNumber[2].LinkedPRs))
	}

	// Issue 3: 1 linked PR.
	if len(cardsByNumber[3].LinkedPRs) != 1 || cardsByNumber[3].LinkedPRs[0].Number != 104 {
		t.Errorf("issue 3 LinkedPRs = %v, want [{Number:104 ...}]", cardsByNumber[3].LinkedPRs)
	}

	// Issue 4: no linked PRs.
	if len(cardsByNumber[4].LinkedPRs) != 0 {
		t.Errorf("issue 4 LinkedPRs has %d entries, want 0", len(cardsByNumber[4].LinkedPRs))
	}

	// Issue 5: 1 linked PR.
	if len(cardsByNumber[5].LinkedPRs) != 1 || cardsByNumber[5].LinkedPRs[0].Number != 105 {
		t.Errorf("issue 5 LinkedPRs = %v, want [{Number:105 ...}]", cardsByNumber[5].LinkedPRs)
	}
}

// NOTE: TestGitHubFetchBoard_ClosingPRErrorPropagated (the REST-era
// concurrent-fetch error test, backed by perIssueErrorMockClient) was
// removed during the GraphQL migration: it is superseded by
// TestFetchBoard_GraphQL_ClosingPRFollowupError_FailsFetchBoard below, which
// pins the same "a linked-PR fetch failure fails the whole board fetch"
// behavior for the GraphQL follow-up path.

func TestGitHubFetchBoard_OrderPreserved(t *testing.T) {
	columns := []string{"Backlog"}

	// FetchBoard's GraphQL path has no concurrency (unlike the old REST
	// Phase 2 goroutine pool), but cards must still appear in the same order
	// gql.fetchIssuePage returned the issueNodes in.
	issueCount := 15
	issues := make([]issueNode, issueCount)
	for i := range issues {
		num := i + 1
		issues[i] = buildIssueNode(num, fmt.Sprintf("Issue %d", num), "Backlog")
	}

	gql := singlePageGQL(issues...)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	cards := board.Columns[0].Cards
	if len(cards) != issueCount {
		t.Fatalf("got %d cards, want %d", len(cards), issueCount)
	}

	// Cards must be in the same order the fake returned the issueNodes.
	for i, card := range cards {
		expectedNumber := i + 1
		if card.Number != expectedNumber {
			t.Errorf("card[%d].Number = %d, want %d (order not preserved)", i, card.Number, expectedNumber)
		}
	}
}

// --- Label Color Capture Tests ---
//
// NOTE: TestGitHubFetchBoard_LabelColor_StripsHashPrefix,
// _InvalidHex_FallsBackToEmpty, and _ShortHex_FallsBackToEmpty (the REST-era
// hex-validation tests) were relocated to graphql_test.go during the
// GraphQL migration. Hex color validation/stripping is performed once, in
// mapIssueQueryNode (graphql.go) before an issueNode ever reaches
// FetchBoard -- issueNode.labels already carries the validated Color, so
// FetchBoard itself no longer has hex-validation behavior to test. See
// TestMapIssueQueryNode_LabelColor_StripsHashPrefix,
// _InvalidHex_FallsBackToEmpty, and _ShortHex_FallsBackToEmpty.

func TestGitHubFetchBoard_CapturesLabelColor(t *testing.T) {
	columns := []string{"Todo"}
	issue := buildIssueNode(1, "Colored label issue")
	issue.labels = []Label{{Name: "bug", Color: "d73a4a"}}

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	card := board.Columns[0].Cards[0]
	if len(card.Labels) != 1 {
		t.Fatalf("card.Labels has %d entries, want 1", len(card.Labels))
	}
	if card.Labels[0].Name != "bug" {
		t.Errorf("card.Labels[0].Name = %q, want %q", card.Labels[0].Name, "bug")
	}
	if card.Labels[0].Color != "d73a4a" {
		t.Errorf("card.Labels[0].Color = %q, want %q", card.Labels[0].Color, "d73a4a")
	}
}

func TestGitHubFetchBoard_LabelWithoutColor_EmptyColorField(t *testing.T) {
	columns := []string{"Todo"}
	// buildIssueNode creates labels without a Color field.
	issue := buildIssueNode(1, "No color label", "Todo")

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	card := board.Columns[0].Cards[0]
	if len(card.Labels) != 1 {
		t.Fatalf("card.Labels has %d entries, want 1", len(card.Labels))
	}
	if card.Labels[0].Color != "" {
		t.Errorf("card.Labels[0].Color = %q, want empty string for label without color", card.Labels[0].Color)
	}
}

// --- Card CreatedAt Threading (date-based sorting, #412) ---

func TestGitHubFetchBoard_CardCreatedAtPopulated(t *testing.T) {
	columns := []string{"Todo"}
	wantCreatedAt := time.Date(2023, 5, 20, 8, 0, 0, 0, time.UTC)
	issue := buildIssueNode(1, "Timestamped issue", "Todo")
	issue.createdAt = wantCreatedAt

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if !card.CreatedAt.Equal(wantCreatedAt) {
		t.Errorf("card.CreatedAt = %v, want %v", card.CreatedAt, wantCreatedAt)
	}
}

// --- Milestone Threading (#461) ---

func TestGitHubFetchBoard_MilestonePopulated(t *testing.T) {
	columns := []string{"Todo"}
	wantMilestone := "v1.0"
	issue := buildIssueNode(1, "Milestoned issue", "Todo")
	issue.milestone = wantMilestone

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if card.Milestone != wantMilestone {
		t.Errorf("card.Milestone = %q, want %q", card.Milestone, wantMilestone)
	}
}

func TestGitHubFetchBoard_NoMilestone_EmptyString(t *testing.T) {
	columns := []string{"Todo"}
	// buildIssueNode does not set milestone, so issue.milestone is the zero value "".
	issue := buildIssueNode(1, "Unmilestoned issue", "Todo")

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if card.Milestone != "" {
		t.Errorf("card.Milestone = %q, want empty string for issue without a milestone", card.Milestone)
	}
}

// --- FetchBoard Assignee Integration Tests ---

func TestGitHubFetchBoard_AssigneesPopulated(t *testing.T) {
	columns := []string{"Todo"}
	assigneeLogin1 := "alice"
	assigneeLogin2 := "bob"
	issue := buildIssueNode(1, "Assigned issue", "Todo")
	issue.assignees = []Assignee{{Login: assigneeLogin1}, {Login: assigneeLogin2}}

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns[0].Cards) != 1 {
		t.Fatalf("column %q has %d cards, want 1", columns[0], len(board.Columns[0].Cards))
	}

	card := board.Columns[0].Cards[0]
	if len(card.Assignees) != 2 {
		t.Fatalf("card.Assignees has %d entries, want 2", len(card.Assignees))
	}
	if card.Assignees[0].Login != assigneeLogin1 {
		t.Errorf("card.Assignees[0].Login = %q, want %q", card.Assignees[0].Login, assigneeLogin1)
	}
	if card.Assignees[1].Login != assigneeLogin2 {
		t.Errorf("card.Assignees[1].Login = %q, want %q", card.Assignees[1].Login, assigneeLogin2)
	}
}

func TestGitHubFetchBoard_NoAssignees_EmptySlice(t *testing.T) {
	columns := []string{"Todo"}
	issue := buildIssueNode(1, "Unassigned issue", "Todo") // no assignees set

	gql := singlePageGQL(issue)
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	card := board.Columns[0].Cards[0]
	if len(card.Assignees) != 0 {
		t.Errorf("card.Assignees has %d entries for unassigned issue, want 0", len(card.Assignees))
	}
}

// --- GraphQL FetchBoard Tests (Part B) ---
//
// These tests exercise FetchBoard against a gql graphQLBoardClient fake.
// FetchBoard itself no longer reads the REST GitHubClient at all -- it pages
// exclusively through gql.fetchIssuePage/fetchIssueClosingPRPage -- but
// NewGitHubProvider still requires a non-nil GitHubClient for its other
// REST-backed methods (CreateCard, UpdateCard, etc.), so these tests pass
// emptyRESTClient() as an unused placeholder.

// emptyRESTClient is a non-nil, empty GitHubClient used as the required
// client param in GraphQL-path tests below. FetchBoard never reads it (it
// pages through gql instead); it exists solely to satisfy NewGitHubProvider's
// signature without a nil interface value.
func emptyRESTClient() *mockGitHubClient {
	return &mockGitHubClient{issues: []*github.Issue{}}
}

func TestFetchBoard_GraphQL_SingleCallFetchesIssuesAndLinkedPRs(t *testing.T) {
	columns := []string{"New", "In Progress", "Done"}

	issues := []issueNode{
		{
			number: 1,
			title:  "Multi-labeled issue lands in furthest column",
			labels: []Label{{Name: "In Progress"}, {Name: "Done"}},
		},
		{
			number: 2,
			title:  "No matching label goes to first column",
			labels: []Label{{Name: "unrelated-label"}},
		},
		{
			number: 3,
			title:  "Has linked PRs",
			labels: []Label{{Name: "New"}},
			// Represents mapLinkedPRs' already-deduped closing-PR output.
			linkedPRs: []LinkedPR{
				{Number: 50, Title: "Fix A", URL: "https://github.com/owner/repo/pull/50"},
				{Number: 51, Title: "Fix B", URL: "https://github.com/owner/repo/pull/51"},
			},
		},
	}

	gql := &fakeGraphQLClient{
		pages: map[string]issuePage{
			"": {issues: issues, hasNextPage: false},
		},
	}
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	if len(board.Columns) != len(columns) {
		t.Fatalf("got %d columns, want %d", len(board.Columns), len(columns))
	}

	// Furthest-matching-label column wins: issue #1 has labels for both
	// "In Progress" (index 1) and "Done" (index 2); it must land in "Done".
	doneCol := board.Columns[2]
	if len(doneCol.Cards) != 1 || doneCol.Cards[0].Number != 1 {
		t.Fatalf("Done column cards = %+v, want issue #1 (furthest matching column)", doneCol.Cards)
	}

	// Issue #2 (no matching label) and issue #3 (label "New") both belong in
	// the "New" column (index 0).
	newCol := board.Columns[0]
	cardsByNumber := map[int]Card{}
	for _, c := range newCol.Cards {
		cardsByNumber[c.Number] = c
	}
	if _, ok := cardsByNumber[2]; !ok {
		t.Fatalf("New column cards = %+v, want issue #2 present (no matching label -> first column)", newCol.Cards)
	}
	card3, ok := cardsByNumber[3]
	if !ok {
		t.Fatalf("New column cards = %+v, want issue #3 present (matches \"New\" label)", newCol.Cards)
	}

	// Issue #3's closing PRs must be forwarded intact and deduped by number.
	if len(card3.LinkedPRs) != 2 {
		t.Fatalf("issue #3 LinkedPRs = %+v, want 2 entries (deduped, non-PR sources excluded)", card3.LinkedPRs)
	}
	seenPRs := map[int]bool{}
	for _, pr := range card3.LinkedPRs {
		seenPRs[pr.Number] = true
	}
	if !seenPRs[50] || !seenPRs[51] {
		t.Fatalf("issue #3 LinkedPRs = %+v, want PR numbers 50 and 51", card3.LinkedPRs)
	}
}

func TestFetchBoard_GraphQL_Paginates(t *testing.T) {
	columns := []string{"Backlog"}

	firstPage := issuePage{
		issues:      []issueNode{{number: 1, title: "First page issue"}},
		hasNextPage: true,
		endCursor:   "issue-cursor-1",
	}
	secondPage := issuePage{
		issues:      []issueNode{{number: 2, title: "Second page issue"}},
		hasNextPage: false,
	}

	gql := &fakeGraphQLClient{
		pages: map[string]issuePage{
			"":               firstPage,
			"issue-cursor-1": secondPage,
		},
	}
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	if len(board.Columns) != 1 {
		t.Fatalf("got %d columns, want 1", len(board.Columns))
	}

	numbersSeen := map[int]bool{}
	for _, c := range board.Columns[0].Cards {
		numbersSeen[c.Number] = true
	}
	if !numbersSeen[1] || !numbersSeen[2] {
		t.Fatalf("board cards = %+v, want both issue #1 (first page) and issue #2 (second page) present", board.Columns[0].Cards)
	}
}

func TestFetchBoard_GraphQL_NestedClosingPRPagination(t *testing.T) {
	columns := []string{"Backlog"}
	const issueNumber = 55

	initialIssue := issueNode{
		number: issueNumber,
		title:  "Issue with >100 closing PRs",
		// First page's own linked PRs, as mapIssueQueryNode's closing-PR connection
		// mapping would have produced them.
		linkedPRs:          []LinkedPR{{Number: 900, Title: "First-page PR", URL: "https://github.com/owner/repo/pull/900"}},
		hasMoreClosingPRs:  true,
		closingPREndCursor: "closing-pr-cursor-1",
	}

	gql := &fakeGraphQLClient{
		pages: map[string]issuePage{
			"": {issues: []issueNode{initialIssue}, hasNextPage: false},
		},
		closingPRPages: map[int]map[string]closingPRPage{
			issueNumber: {
				"closing-pr-cursor-1": {
					linkedPRs:   []LinkedPR{{Number: 901, Title: "Follow-up PR", URL: "https://github.com/owner/repo/pull/901"}},
					hasNextPage: false,
				},
			},
		},
	}
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	if len(board.Columns) != 1 || len(board.Columns[0].Cards) != 1 {
		t.Fatalf("board columns = %+v, want 1 column with 1 card", board.Columns)
	}

	card := board.Columns[0].Cards[0]
	seenPRs := map[int]bool{}
	for _, pr := range card.LinkedPRs {
		seenPRs[pr.Number] = true
	}
	if !seenPRs[900] {
		t.Fatalf("card.LinkedPRs = %+v, want first page's PR #900 present", card.LinkedPRs)
	}
	if !seenPRs[901] {
		t.Fatalf("card.LinkedPRs = %+v, want follow-up page's PR #901 present (bounded nested pagination merge)", card.LinkedPRs)
	}
}

func TestFetchBoard_GraphQL_NestedClosingPRPagination_CapsFollowupQueries(t *testing.T) {
	columns := []string{"Backlog"}
	const issueNumber = 77

	initialIssue := issueNode{
		number:             issueNumber,
		title:              "Issue with a pathological number of closing PRs",
		hasMoreClosingPRs:  true,
		closingPREndCursor: "cursor-0",
	}

	// Script maxClosingPRFollowupPages worth of follow-up pages, each pointing
	// to the next, PLUS one extra page beyond the cap. If FetchBoard properly
	// bounds the follow-up loop at maxClosingPRFollowupPages, the extra page's
	// PR must never be requested/collected -- a correctness property that
	// can only be pinned by observing which scripted pages were consumed.
	closingPRPagesForIssue := map[string]closingPRPage{}
	for i := 0; i < maxClosingPRFollowupPages; i++ {
		cursor := fmt.Sprintf("cursor-%d", i)
		nextCursor := fmt.Sprintf("cursor-%d", i+1)
		closingPRPagesForIssue[cursor] = closingPRPage{
			linkedPRs:   []LinkedPR{{Number: 1000 + i, Title: fmt.Sprintf("Follow-up PR %d", i), URL: "https://github.com/owner/repo/pull/x"}},
			hasNextPage: true,
			endCursor:   nextCursor,
		}
	}
	// The one-past-the-cap page: must never be reached by a correctly-capped loop.
	pastCapCursor := fmt.Sprintf("cursor-%d", maxClosingPRFollowupPages)
	pastCapPRNumber := 1000 + maxClosingPRFollowupPages
	closingPRPagesForIssue[pastCapCursor] = closingPRPage{
		linkedPRs:   []LinkedPR{{Number: pastCapPRNumber, Title: "Past-cap PR", URL: "https://github.com/owner/repo/pull/x"}},
		hasNextPage: true,
		endCursor:   fmt.Sprintf("cursor-%d", maxClosingPRFollowupPages+1),
	}

	gql := &fakeGraphQLClient{
		pages: map[string]issuePage{
			"": {issues: []issueNode{initialIssue}, hasNextPage: false},
		},
		closingPRPages: map[int]map[string]closingPRPage{
			issueNumber: closingPRPagesForIssue,
		},
	}
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v, want no error (cap-exceeded degrades gracefully, does not fail the board fetch)", err)
	}
	if len(board.Columns) != 1 || len(board.Columns[0].Cards) != 1 {
		t.Fatalf("board columns = %+v, want 1 column with 1 card", board.Columns)
	}

	card := board.Columns[0].Cards[0]
	seenPRs := map[int]bool{}
	for _, pr := range card.LinkedPRs {
		seenPRs[pr.Number] = true
	}
	for i := 0; i < maxClosingPRFollowupPages; i++ {
		if !seenPRs[1000+i] {
			t.Fatalf("card.LinkedPRs = %+v, want PR #%d collected (within the follow-up cap)", card.LinkedPRs, 1000+i)
		}
	}
	if seenPRs[pastCapPRNumber] {
		t.Fatalf("card.LinkedPRs = %+v, want past-cap PR #%d NOT collected (follow-up loop must stop at maxClosingPRFollowupPages)", card.LinkedPRs, pastCapPRNumber)
	}

	// The cap itself is the behavior under test: the follow-up loop must
	// issue no more than maxClosingPRFollowupPages calls for this issue. This
	// call-count assertion is a direct guard of the ticket's cap contract,
	// not an incidental implementation detail (see lessons-learned.md's
	// carve-out for count assertions that guard observable cap behavior).
	if len(gql.calledClosingPRCursors) > maxClosingPRFollowupPages {
		t.Fatalf("fetchIssueClosingPRPage was called %d times, want at most maxClosingPRFollowupPages (%d)", len(gql.calledClosingPRCursors), maxClosingPRFollowupPages)
	}
}

// closingPRErrorFakeClient wraps a fakeGraphQLClient to return a distinct,
// scripted error only from fetchIssueClosingPRPage, leaving fetchIssuePage's
// success path untouched. fakeGraphQLClient's single shared err field can't
// express "outer page succeeds, follow-up fails" (setting it fails both
// calls), so this wrapper isolates the follow-up-only failure needed to
// distinguish FetchBoard's fail-whole-board branch from the cap-exceeded
// graceful-degradation branch (TestFetchBoard_GraphQL_NestedClosingPRPagination_CapsFollowupQueries).
type closingPRErrorFakeClient struct {
	*fakeGraphQLClient
	closingPRErr error
}

func (f *closingPRErrorFakeClient) fetchIssueClosingPRPage(_ context.Context, _, _ string, _ int, _ string) (closingPRPage, error) {
	return closingPRPage{}, f.closingPRErr
}

func TestFetchBoard_GraphQL_ClosingPRFollowupError_FailsFetchBoard(t *testing.T) {
	columns := []string{"Backlog"}
	const issueNumber = 88
	followupErr := errors.New("graphql: follow-up query failed")

	initialIssue := issueNode{
		number:             issueNumber,
		title:              "Issue whose follow-up query fails",
		hasMoreClosingPRs:  true,
		closingPREndCursor: "closing-pr-cursor-1",
	}

	gql := &closingPRErrorFakeClient{
		fakeGraphQLClient: &fakeGraphQLClient{
			pages: map[string]issuePage{
				"": {issues: []issueNode{initialIssue}, hasNextPage: false},
			},
		},
		closingPRErr: followupErr,
	}
	provider := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", columns)

	board, err := provider.FetchBoard(context.Background())
	if err == nil {
		t.Fatal("expected FetchBoard to return an error when a follow-up closing PR query fails, got nil")
	}
	if !errors.Is(err, followupErr) {
		t.Errorf("FetchBoard error = %v, want it to wrap %v", err, followupErr)
	}
	if len(board.Columns) != 0 {
		t.Errorf("board = %+v, want a zero-value Board{} on follow-up error (not a partial board)", board)
	}
}
