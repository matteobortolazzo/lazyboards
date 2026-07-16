package provider

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/shurcooL/githubv4"
)

// Compile-time check: *fakeGraphQLClient implements graphQLBoardClient.
var _ graphQLBoardClient = (*fakeGraphQLClient)(nil)

// fakeGraphQLClient is a hand-written graphQLBoardClient for tests: no
// network, no reflection. Pages are scripted by the afterCursor argument
// that would request them, so tests can script multi-page sequences.
type fakeGraphQLClient struct {
	pages map[string]issuePage
	err   error

	calledCursors []string

	// closingPRPages scripts fetchIssueClosingPRPage results, keyed by issue
	// number and then by the afterCursor that would request them, mirroring
	// the pages/calledCursors pattern above but for the nested per-issue
	// closing PR follow-up query.
	closingPRPages map[int]map[string]closingPRPage

	calledClosingPRCursors []closingPRCursorCall

	// openPRPages scripts fetchOpenPRPage results, keyed by the afterCursor
	// that would request them, mirroring the pages/calledCursors pattern for
	// the repo-wide open pull-request connection.
	openPRPages map[string]openPRPage

	calledOpenPRCursors []string

	// deleteIssueErr scripts the error DeleteIssue returns, letting tests
	// exercise GitHubProvider.DeleteCard's success / not-found / generic-error
	// mapping without a real GraphQL round-trip.
	deleteIssueErr error
}

// closingPRCursorCall records one fetchIssueClosingPRPage call so tests can
// assert call order for multi-page follow-up sequences.
type closingPRCursorCall struct {
	issueNumber int
	cursor      string
}

func (f *fakeGraphQLClient) fetchIssuePage(_ context.Context, _, _, afterCursor string) (issuePage, error) {
	f.calledCursors = append(f.calledCursors, afterCursor)
	if f.err != nil {
		return issuePage{}, f.err
	}
	return f.pages[afterCursor], nil
}

// fetchIssueClosingPRPage returns the scripted closingPRPage for the given
// issue number and afterCursor, recording each call in calledClosingPRCursors
// so tests can assert call order for multi-page follow-up sequences.
func (f *fakeGraphQLClient) fetchIssueClosingPRPage(_ context.Context, _, _ string, issueNumber int, afterCursor string) (closingPRPage, error) {
	f.calledClosingPRCursors = append(f.calledClosingPRCursors, closingPRCursorCall{issueNumber: issueNumber, cursor: afterCursor})
	if f.err != nil {
		return closingPRPage{}, f.err
	}
	return f.closingPRPages[issueNumber][afterCursor], nil
}

// fetchOpenPRPage returns the scripted openPRPage for the given afterCursor,
// recording each call in calledOpenPRCursors so tests can assert multi-page
// sequences.
func (f *fakeGraphQLClient) fetchOpenPRPage(_ context.Context, _, _, afterCursor string) (openPRPage, error) {
	f.calledOpenPRCursors = append(f.calledOpenPRCursors, afterCursor)
	if f.err != nil {
		return openPRPage{}, f.err
	}
	return f.openPRPages[afterCursor], nil
}

// deleteIssue returns the scripted deleteIssueErr, letting tests script
// success (nil), not-found, or an arbitrary generic-error scenario.
func (f *fakeGraphQLClient) deleteIssue(_ context.Context, _, _ string, _ int) error {
	return f.deleteIssueErr
}

func TestNewGitHubV4Adapter_WrapsGivenClient(t *testing.T) {
	client := githubv4.NewClient(nil)

	adapter := NewGitHubV4Adapter(client)

	if adapter.client != client {
		t.Fatalf("NewGitHubV4Adapter().client = %p, want the same client instance %p passed in", adapter.client, client)
	}
}

func TestFakeGraphQLClient_ReturnsScriptedPageForCursor(t *testing.T) {
	firstPage := issuePage{
		issues:      []issueNode{{number: 1, title: "first page issue"}},
		hasNextPage: true,
		endCursor:   "cursor-A",
	}
	secondPage := issuePage{
		issues:      []issueNode{{number: 2, title: "second page issue"}},
		hasNextPage: false,
	}
	fake := &fakeGraphQLClient{
		pages: map[string]issuePage{
			"":         firstPage,
			"cursor-A": secondPage,
		},
	}

	got, err := fake.fetchIssuePage(context.Background(), "owner", "repo", "")
	if err != nil {
		t.Fatalf("fetchIssuePage(first page): unexpected error: %v", err)
	}
	if len(got.issues) != 1 || got.issues[0].number != firstPage.issues[0].number {
		t.Fatalf("fetchIssuePage(first page) = %+v, want issue number %d", got, firstPage.issues[0].number)
	}
	if !got.hasNextPage || got.endCursor != "cursor-A" {
		t.Fatalf("fetchIssuePage(first page) pagination = %+v, want hasNextPage=true endCursor=cursor-A", got)
	}

	got, err = fake.fetchIssuePage(context.Background(), "owner", "repo", got.endCursor)
	if err != nil {
		t.Fatalf("fetchIssuePage(second page): unexpected error: %v", err)
	}
	if len(got.issues) != 1 || got.issues[0].number != secondPage.issues[0].number {
		t.Fatalf("fetchIssuePage(second page) = %+v, want issue number %d", got, secondPage.issues[0].number)
	}
	if got.hasNextPage {
		t.Fatalf("fetchIssuePage(second page).hasNextPage = true, want false (last page)")
	}

	wantCursors := []string{"", "cursor-A"}
	if len(fake.calledCursors) != len(wantCursors) {
		t.Fatalf("calledCursors = %v, want %v", fake.calledCursors, wantCursors)
	}
	for i, c := range wantCursors {
		if fake.calledCursors[i] != c {
			t.Fatalf("calledCursors = %v, want %v", fake.calledCursors, wantCursors)
		}
	}
}

func TestFakeGraphQLClient_ReturnsScriptedError(t *testing.T) {
	wantErr := errors.New("graphql: rate limited")
	fake := &fakeGraphQLClient{err: wantErr}

	_, err := fake.fetchIssuePage(context.Background(), "owner", "repo", "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("fetchIssuePage() error = %v, want %v", err, wantErr)
	}
}

// TestFakeGraphQLClient_FetchIssueClosingPRPage_ReturnsScriptedError mirrors
// TestFakeGraphQLClient_ReturnsScriptedError for fetchIssueClosingPRPage's own
// err branch, so the closing PR follow-up query's error path is exercised
// directly rather than only indirectly via the sibling fetchIssuePage test.
func TestFakeGraphQLClient_FetchIssueClosingPRPage_ReturnsScriptedError(t *testing.T) {
	wantErr := errors.New("graphql: rate limited")
	fake := &fakeGraphQLClient{err: wantErr}

	_, err := fake.fetchIssueClosingPRPage(context.Background(), "owner", "repo", 55, "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("fetchIssueClosingPRPage() error = %v, want %v", err, wantErr)
	}
}

// buildClosingPRItem constructs a node returned by GitHub's
// closedByPullRequestsReferences connection.
func buildClosingPRItem(number int, title, url string) pullRequestQueryNode {
	var item pullRequestQueryNode
	item.Number = githubv4.Int(number)
	item.Title = githubv4.String(title)
	item.URL = githubv4.String(url)
	return item
}

// TestMapLinkedPRs_PopulatesBranchFromHeadRefName asserts that mapLinkedPRs
// carries the PullRequest's headRefName GraphQL field through to the plain
// LinkedPR.Branch field. The node is built directly (rather than via
// buildClosingPRItem, which has no Branch parameter) since HeadRefName is new.
func TestMapLinkedPRs_PopulatesBranchFromHeadRefName(t *testing.T) {
	wantBranch := "feature/widget-support"
	var item pullRequestQueryNode
	item.Number = githubv4.Int(99)
	item.Title = githubv4.String("feat: add widget support")
	item.URL = githubv4.String("https://github.com/o/r/pull/99")
	item.HeadRefName = githubv4.String(wantBranch)

	got := mapLinkedPRs([]pullRequestQueryNode{item})

	if len(got) != 1 {
		t.Fatalf("mapLinkedPRs() returned %d PRs, want 1", len(got))
	}
	if got[0].Branch != wantBranch {
		t.Errorf("mapLinkedPRs()[0].Branch = %q, want %q", got[0].Branch, wantBranch)
	}
}

func TestIssueQuery_UsesGitHubClosingPRConnection(t *testing.T) {
	field, ok := reflect.TypeOf(issueQueryNode{}).FieldByName("ClosedByPullRequestsReferences")
	if !ok {
		t.Fatal("issueQueryNode is missing ClosedByPullRequestsReferences")
	}
	if got, want := field.Tag.Get("graphql"), "closedByPullRequestsReferences(first: 100)"; got != want {
		t.Fatalf("closing PR GraphQL field = %q, want %q", got, want)
	}
}

func TestMapLinkedPRs_DedupesByPRNumberWithinIssue(t *testing.T) {
	items := []pullRequestQueryNode{
		buildClosingPRItem(7, "feat: shared PR", "https://github.com/o/r/pull/7"),
		buildClosingPRItem(7, "feat: shared PR", "https://github.com/o/r/pull/7"),
	}

	got := mapLinkedPRs(items)

	if len(got) != 1 {
		t.Fatalf("mapLinkedPRs() returned %d PRs, want 1 (duplicate PR number must be deduped): %+v", len(got), got)
	}
}

func TestMapIssueQueryNode_MapsFieldsLabelsAndAssignees(t *testing.T) {
	var n issueQueryNode
	n.Number = githubv4.Int(101)
	n.Title = githubv4.String("Add dark mode")
	n.Body = githubv4.String("Users want a dark theme.")
	n.URL = githubv4.String("https://github.com/o/r/issues/101")
	n.Labels.Nodes = []labelQueryNode{
		{Name: githubv4.String("ui"), Color: githubv4.String("d73a4a")},
		{Name: githubv4.String("bad-color"), Color: githubv4.String("not-hex")},
	}
	n.Assignees.Nodes = []assigneeQueryNode{
		{Login: githubv4.String("alice")},
	}

	got := mapIssueQueryNode(n)

	if got.number != 101 || got.title != "Add dark mode" || got.body != "Users want a dark theme." || got.url != "https://github.com/o/r/issues/101" {
		t.Fatalf("mapIssueQueryNode() core fields = %+v, want number=101 title=%q body=%q url=%q", got, "Add dark mode", "Users want a dark theme.", "https://github.com/o/r/issues/101")
	}
	if len(got.labels) != 2 || got.labels[0].Color != "d73a4a" {
		t.Fatalf("mapIssueQueryNode().labels = %+v, want valid hex color preserved", got.labels)
	}
	if got.labels[1].Color != "" {
		t.Fatalf("mapIssueQueryNode().labels[1].Color = %q, want empty (invalid hex must be stripped, mirroring extractLabels)", got.labels[1].Color)
	}
	if len(got.assignees) != 1 || got.assignees[0].Login != "alice" {
		t.Fatalf("mapIssueQueryNode().assignees = %+v, want [{alice}]", got.assignees)
	}
}

// TestMapIssueQueryNode_LabelColor_StripsHashPrefix,
// _InvalidHex_FallsBackToEmpty, and _ShortHex_FallsBackToEmpty pin
// mapIssueQueryNode's label-color validation (relocated here from the
// REST-era FetchBoard-level tests during the GraphQL migration -- hex
// validation now happens once, in this mapping function, before an
// issueNode ever reaches FetchBoard).

func TestMapIssueQueryNode_LabelColor_StripsHashPrefix(t *testing.T) {
	var n issueQueryNode
	// Some GitHub API responses may include a "#" prefix in the color field.
	n.Labels.Nodes = []labelQueryNode{{Name: githubv4.String("feature"), Color: githubv4.String("#0075ca")}}

	got := mapIssueQueryNode(n)

	if len(got.labels) != 1 {
		t.Fatalf("mapIssueQueryNode().labels has %d entries, want 1", len(got.labels))
	}
	if got.labels[0].Color != "0075ca" {
		t.Errorf("labels[0].Color = %q, want %q (should strip # prefix)", got.labels[0].Color, "0075ca")
	}
}

func TestMapIssueQueryNode_LabelColor_InvalidHex_FallsBackToEmpty(t *testing.T) {
	var n issueQueryNode
	n.Labels.Nodes = []labelQueryNode{{Name: githubv4.String("bug"), Color: githubv4.String("xxxxxx")}}

	got := mapIssueQueryNode(n)

	if got.labels[0].Color != "" {
		t.Errorf("labels[0].Color = %q, want empty string for invalid hex color", got.labels[0].Color)
	}
}

func TestMapIssueQueryNode_LabelColor_ShortHex_FallsBackToEmpty(t *testing.T) {
	var n issueQueryNode
	n.Labels.Nodes = []labelQueryNode{{Name: githubv4.String("bug"), Color: githubv4.String("fff")}}

	got := mapIssueQueryNode(n)

	if got.labels[0].Color != "" {
		t.Errorf("labels[0].Color = %q, want empty string for short hex color", got.labels[0].Color)
	}
}

func TestMapIssueQueryNode_CapturesClosingPRContinuationCursor(t *testing.T) {
	var n issueQueryNode
	n.ClosedByPullRequestsReferences.PageInfo.HasNextPage = githubv4.Boolean(true)
	n.ClosedByPullRequestsReferences.PageInfo.EndCursor = githubv4.String("closing-pr-cursor-1")

	got := mapIssueQueryNode(n)

	if !got.hasMoreClosingPRs {
		t.Fatalf("mapIssueQueryNode().hasMoreClosingPRs = false, want true when closing PR pageInfo.hasNextPage is true")
	}
	if got.closingPREndCursor != "closing-pr-cursor-1" {
		t.Fatalf("mapIssueQueryNode().closingPREndCursor = %q, want %q", got.closingPREndCursor, "closing-pr-cursor-1")
	}
}

// TestMapIssueClosingPRQuery_MapsLinkedPRsAndPageInfo pins the nested closing
// PR connection's mapping, deduplication, and pagination behavior.
func TestMapIssueClosingPRQuery_MapsLinkedPRsAndPageInfo(t *testing.T) {
	var q issueClosingPRQuery
	q.Repository.Issue.ClosedByPullRequestsReferences.Nodes = []pullRequestQueryNode{
		buildClosingPRItem(42, "fix: real linked PR", "https://github.com/o/r/pull/42"),
		buildClosingPRItem(42, "fix: real linked PR", "https://github.com/o/r/pull/42"),
	}
	q.Repository.Issue.ClosedByPullRequestsReferences.PageInfo.HasNextPage = githubv4.Boolean(true)
	q.Repository.Issue.ClosedByPullRequestsReferences.PageInfo.EndCursor = githubv4.String("closing-pr-cursor-2")

	got := mapIssueClosingPRQuery(q)

	if len(got.linkedPRs) != 1 {
		t.Fatalf("mapIssueClosingPRQuery().linkedPRs has %d entries, want 1 (duplicate PR must be deduped): %+v", len(got.linkedPRs), got.linkedPRs)
	}
	if got.linkedPRs[0].Number != 42 || got.linkedPRs[0].Title != "fix: real linked PR" || got.linkedPRs[0].URL != "https://github.com/o/r/pull/42" {
		t.Fatalf("mapIssueClosingPRQuery().linkedPRs[0] = %+v, want {Number:42 Title:%q URL:%q}", got.linkedPRs[0], "fix: real linked PR", "https://github.com/o/r/pull/42")
	}
	if !got.hasNextPage || got.endCursor != "closing-pr-cursor-2" {
		t.Fatalf("mapIssueClosingPRQuery() pagination = hasNextPage=%v endCursor=%q, want hasNextPage=true endCursor=%q", got.hasNextPage, got.endCursor, "closing-pr-cursor-2")
	}
}

// TestFakeGraphQLClient_FetchIssueClosingPRPage_ReturnsScriptedPageForIssueAndCursor
// pins fakeGraphQLClient's closing PR-scripting scaffolding, keyed by
// (issueNumber, cursor), so a follow-up green-phase FetchBoard loop test can
// script multi-page per-issue closing PR sequences.
func TestFakeGraphQLClient_FetchIssueClosingPRPage_ReturnsScriptedPageForIssueAndCursor(t *testing.T) {
	firstPage := closingPRPage{
		linkedPRs:   []LinkedPR{{Number: 101, Title: "first closing PR page PR", URL: "https://github.com/o/r/pull/101"}},
		hasNextPage: true,
		endCursor:   "closing-pr-cursor-A",
	}
	secondPage := closingPRPage{
		linkedPRs:   []LinkedPR{{Number: 102, Title: "second closing PR page PR", URL: "https://github.com/o/r/pull/102"}},
		hasNextPage: false,
	}
	const issueNumber = 55
	fake := &fakeGraphQLClient{
		closingPRPages: map[int]map[string]closingPRPage{
			issueNumber: {
				"":                    firstPage,
				"closing-pr-cursor-A": secondPage,
			},
		},
	}

	got, err := fake.fetchIssueClosingPRPage(context.Background(), "owner", "repo", issueNumber, "")
	if err != nil {
		t.Fatalf("fetchIssueClosingPRPage(first page): unexpected error: %v", err)
	}
	if len(got.linkedPRs) != 1 || got.linkedPRs[0].Number != firstPage.linkedPRs[0].Number {
		t.Fatalf("fetchIssueClosingPRPage(first page) = %+v, want linked PR number %d", got, firstPage.linkedPRs[0].Number)
	}
	if !got.hasNextPage || got.endCursor != "closing-pr-cursor-A" {
		t.Fatalf("fetchIssueClosingPRPage(first page) pagination = %+v, want hasNextPage=true endCursor=closing-pr-cursor-A", got)
	}

	got, err = fake.fetchIssueClosingPRPage(context.Background(), "owner", "repo", issueNumber, got.endCursor)
	if err != nil {
		t.Fatalf("fetchIssueClosingPRPage(second page): unexpected error: %v", err)
	}
	if len(got.linkedPRs) != 1 || got.linkedPRs[0].Number != secondPage.linkedPRs[0].Number {
		t.Fatalf("fetchIssueClosingPRPage(second page) = %+v, want linked PR number %d", got, secondPage.linkedPRs[0].Number)
	}
	if got.hasNextPage {
		t.Fatalf("fetchIssueClosingPRPage(second page).hasNextPage = true, want false (last page)")
	}

	wantCalls := []closingPRCursorCall{
		{issueNumber: issueNumber, cursor: ""},
		{issueNumber: issueNumber, cursor: "closing-pr-cursor-A"},
	}
	if len(fake.calledClosingPRCursors) != len(wantCalls) {
		t.Fatalf("calledClosingPRCursors = %+v, want %+v", fake.calledClosingPRCursors, wantCalls)
	}
	for i, want := range wantCalls {
		if fake.calledClosingPRCursors[i] != want {
			t.Fatalf("calledClosingPRCursors = %+v, want %+v", fake.calledClosingPRCursors, wantCalls)
		}
	}
}

// --- CreatedAt (date-based sorting, #412) ---

// TestIssueQueryNode_HasCreatedAtField pins that issueQueryNode selects the
// issue's creation date, needed for the board's date-based sort (#412).
// githubv4's default (no explicit graphql tag) field-name mapping
// lowercases only the first rune, so a Go field named CreatedAt maps to the
// GraphQL field "createdAt" without needing an explicit tag (unlike the
// paginated ClosedByPullRequestsReferences connection above).
func TestIssueQueryNode_HasCreatedAtField(t *testing.T) {
	field, ok := reflect.TypeOf(issueQueryNode{}).FieldByName("CreatedAt")
	if !ok {
		t.Fatal("issueQueryNode is missing a CreatedAt field (needed for date-based sorting, #412)")
	}
	if field.Type != reflect.TypeOf(githubv4.DateTime{}) {
		t.Fatalf("issueQueryNode.CreatedAt type = %v, want %v", field.Type, reflect.TypeOf(githubv4.DateTime{}))
	}
}

// TestMapIssueQueryNode_MapsCreatedAt asserts mapIssueQueryNode carries the
// GraphQL createdAt field through to issueNode.createdAt as a plain
// time.Time, decoupled from githubv4.DateTime.
func TestMapIssueQueryNode_MapsCreatedAt(t *testing.T) {
	want := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)
	var n issueQueryNode
	n.CreatedAt = githubv4.DateTime{Time: want}

	got := mapIssueQueryNode(n)

	if !got.createdAt.Equal(want) {
		t.Fatalf("mapIssueQueryNode().createdAt = %v, want %v", got.createdAt, want)
	}
}

func TestMapIssuesQuery_MapsOuterPageInfo(t *testing.T) {
	var q issuesQuery
	q.Repository.Issues.Nodes = []issueQueryNode{{Number: githubv4.Int(1)}, {Number: githubv4.Int(2)}}
	q.Repository.Issues.PageInfo.HasNextPage = githubv4.Boolean(true)
	q.Repository.Issues.PageInfo.EndCursor = githubv4.String("issue-cursor-1")

	got := mapIssuesQuery(q)

	if len(got.issues) != 2 {
		t.Fatalf("mapIssuesQuery().issues has %d entries, want 2", len(got.issues))
	}
	if !got.hasNextPage || got.endCursor != "issue-cursor-1" {
		t.Fatalf("mapIssuesQuery() pagination = hasNextPage=%v endCursor=%q, want hasNextPage=true endCursor=%q", got.hasNextPage, got.endCursor, "issue-cursor-1")
	}
}
