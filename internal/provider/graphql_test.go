package provider

import (
	"context"
	"errors"
	"testing"

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

	// timelinePages scripts fetchIssueTimelinePage results, keyed by issue
	// number and then by the afterCursor that would request them, mirroring
	// the pages/calledCursors pattern above but for the nested per-issue
	// timeline follow-up query.
	timelinePages map[int]map[string]timelinePage

	calledTimelineCursors []timelineCursorCall
}

// timelineCursorCall records one fetchIssueTimelinePage call so tests can
// assert call order for multi-page follow-up sequences.
type timelineCursorCall struct {
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

// fetchIssueTimelinePage returns the scripted timelinePage for the given
// issue number and afterCursor, recording each call in calledTimelineCursors
// so tests can assert call order for multi-page follow-up sequences.
func (f *fakeGraphQLClient) fetchIssueTimelinePage(_ context.Context, _, _ string, issueNumber int, afterCursor string) (timelinePage, error) {
	f.calledTimelineCursors = append(f.calledTimelineCursors, timelineCursorCall{issueNumber: issueNumber, cursor: afterCursor})
	if f.err != nil {
		return timelinePage{}, f.err
	}
	return f.timelinePages[issueNumber][afterCursor], nil
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

// TestFakeGraphQLClient_FetchIssueTimelinePage_ReturnsScriptedError mirrors
// TestFakeGraphQLClient_ReturnsScriptedError for fetchIssueTimelinePage's own
// err branch, so the timeline follow-up query's error path is exercised
// directly rather than only indirectly via the sibling fetchIssuePage test.
func TestFakeGraphQLClient_FetchIssueTimelinePage_ReturnsScriptedError(t *testing.T) {
	wantErr := errors.New("graphql: rate limited")
	fake := &fakeGraphQLClient{err: wantErr}

	_, err := fake.fetchIssueTimelinePage(context.Background(), "owner", "repo", 55, "")
	if !errors.Is(err, wantErr) {
		t.Fatalf("fetchIssueTimelinePage() error = %v, want %v", err, wantErr)
	}
}

// buildTimelineItem constructs a timelineItemQueryNode whose
// "... on CrossReferencedEvent { source { ... on PullRequest {...} } }"
// inline fragment matched, as it would when the cross-reference's source is
// an actual pull request.
func buildTimelineItem(number int, title, url string) timelineItemQueryNode {
	var item timelineItemQueryNode
	item.CrossReferencedEvent.Source.PullRequest.Number = githubv4.Int(number)
	item.CrossReferencedEvent.Source.PullRequest.Title = githubv4.String(title)
	item.CrossReferencedEvent.Source.PullRequest.URL = githubv4.String(url)
	return item
}

// TestMapLinkedPRs_PopulatesBranchFromHeadRefName asserts that mapLinkedPRs
// carries the PullRequest's headRefName GraphQL field through to the plain
// LinkedPR.Branch field. The node is built directly (rather than via
// buildTimelineItem, which has no Branch parameter) since HeadRefName is new.
func TestMapLinkedPRs_PopulatesBranchFromHeadRefName(t *testing.T) {
	wantBranch := "feature/widget-support"
	var item timelineItemQueryNode
	item.CrossReferencedEvent.Source.PullRequest.Number = githubv4.Int(99)
	item.CrossReferencedEvent.Source.PullRequest.Title = githubv4.String("feat: add widget support")
	item.CrossReferencedEvent.Source.PullRequest.URL = githubv4.String("https://github.com/o/r/pull/99")
	item.CrossReferencedEvent.Source.PullRequest.HeadRefName = githubv4.String(wantBranch)

	got := mapLinkedPRs([]timelineItemQueryNode{item})

	if len(got) != 1 {
		t.Fatalf("mapLinkedPRs() returned %d PRs, want 1", len(got))
	}
	if got[0].Branch != wantBranch {
		t.Errorf("mapLinkedPRs()[0].Branch = %q, want %q", got[0].Branch, wantBranch)
	}
}

func TestMapLinkedPRs_SkipsCrossReferenceSourcedFromPlainIssue(t *testing.T) {
	items := []timelineItemQueryNode{
		buildTimelineItem(42, "fix: real linked PR", "https://github.com/o/r/pull/42"),
		// Source was a plain Issue: the "... on PullRequest" fragment never
		// matched, so PullRequest is left zero-valued.
		{},
	}

	got := mapLinkedPRs(items)

	if len(got) != 1 {
		t.Fatalf("mapLinkedPRs() returned %d PRs, want 1 (the issue-sourced cross-reference must be skipped): %+v", len(got), got)
	}
	if got[0].Number != 42 {
		t.Fatalf("mapLinkedPRs()[0].Number = %d, want 42", got[0].Number)
	}
}

func TestMapLinkedPRs_DedupesByPRNumberWithinIssue(t *testing.T) {
	items := []timelineItemQueryNode{
		buildTimelineItem(7, "feat: shared PR", "https://github.com/o/r/pull/7"),
		buildTimelineItem(7, "feat: shared PR", "https://github.com/o/r/pull/7"),
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

func TestMapIssueQueryNode_CapturesTimelineContinuationCursor(t *testing.T) {
	var n issueQueryNode
	n.TimelineItems.PageInfo.HasNextPage = githubv4.Boolean(true)
	n.TimelineItems.PageInfo.EndCursor = githubv4.String("timeline-cursor-1")

	got := mapIssueQueryNode(n)

	if !got.hasMoreTimelineItems {
		t.Fatalf("mapIssueQueryNode().hasMoreTimelineItems = false, want true when timeline pageInfo.hasNextPage is true")
	}
	if got.timelineEndCursor != "timeline-cursor-1" {
		t.Fatalf("mapIssueQueryNode().timelineEndCursor = %q, want %q", got.timelineEndCursor, "timeline-cursor-1")
	}
}

// TestMapIssueTimelineQuery_MapsLinkedPRsAndPageInfo pins mapIssueTimelineQuery's
// expected behavior: it must reuse mapLinkedPRs' skip/dedup semantics (proven
// separately by TestMapLinkedPRs_SkipsCrossReferenceSourcedFromPlainIssue and
// TestMapLinkedPRs_DedupesByPRNumberWithinIssue) for the nested timelineItems
// nodes, plus carry through the connection's own pageInfo.
func TestMapIssueTimelineQuery_MapsLinkedPRsAndPageInfo(t *testing.T) {
	var q issueTimelineQuery
	q.Repository.Issue.TimelineItems.Nodes = []timelineItemQueryNode{
		buildTimelineItem(42, "fix: real linked PR", "https://github.com/o/r/pull/42"),
		buildTimelineItem(42, "fix: real linked PR", "https://github.com/o/r/pull/42"),
		{}, // cross-reference sourced from a plain Issue; must be skipped
	}
	q.Repository.Issue.TimelineItems.PageInfo.HasNextPage = githubv4.Boolean(true)
	q.Repository.Issue.TimelineItems.PageInfo.EndCursor = githubv4.String("timeline-cursor-2")

	got := mapIssueTimelineQuery(q)

	if len(got.linkedPRs) != 1 {
		t.Fatalf("mapIssueTimelineQuery().linkedPRs has %d entries, want 1 (duplicate PR must be deduped, issue-sourced cross-reference must be skipped): %+v", len(got.linkedPRs), got.linkedPRs)
	}
	if got.linkedPRs[0].Number != 42 || got.linkedPRs[0].Title != "fix: real linked PR" || got.linkedPRs[0].URL != "https://github.com/o/r/pull/42" {
		t.Fatalf("mapIssueTimelineQuery().linkedPRs[0] = %+v, want {Number:42 Title:%q URL:%q}", got.linkedPRs[0], "fix: real linked PR", "https://github.com/o/r/pull/42")
	}
	if !got.hasNextPage || got.endCursor != "timeline-cursor-2" {
		t.Fatalf("mapIssueTimelineQuery() pagination = hasNextPage=%v endCursor=%q, want hasNextPage=true endCursor=%q", got.hasNextPage, got.endCursor, "timeline-cursor-2")
	}
}

// TestFakeGraphQLClient_FetchIssueTimelinePage_ReturnsScriptedPageForIssueAndCursor
// pins fakeGraphQLClient's timeline-scripting scaffolding, keyed by
// (issueNumber, cursor), so a follow-up green-phase FetchBoard loop test can
// script multi-page per-issue timeline sequences.
func TestFakeGraphQLClient_FetchIssueTimelinePage_ReturnsScriptedPageForIssueAndCursor(t *testing.T) {
	firstPage := timelinePage{
		linkedPRs:   []LinkedPR{{Number: 101, Title: "first timeline page PR", URL: "https://github.com/o/r/pull/101"}},
		hasNextPage: true,
		endCursor:   "timeline-cursor-A",
	}
	secondPage := timelinePage{
		linkedPRs:   []LinkedPR{{Number: 102, Title: "second timeline page PR", URL: "https://github.com/o/r/pull/102"}},
		hasNextPage: false,
	}
	const issueNumber = 55
	fake := &fakeGraphQLClient{
		timelinePages: map[int]map[string]timelinePage{
			issueNumber: {
				"":                  firstPage,
				"timeline-cursor-A": secondPage,
			},
		},
	}

	got, err := fake.fetchIssueTimelinePage(context.Background(), "owner", "repo", issueNumber, "")
	if err != nil {
		t.Fatalf("fetchIssueTimelinePage(first page): unexpected error: %v", err)
	}
	if len(got.linkedPRs) != 1 || got.linkedPRs[0].Number != firstPage.linkedPRs[0].Number {
		t.Fatalf("fetchIssueTimelinePage(first page) = %+v, want linked PR number %d", got, firstPage.linkedPRs[0].Number)
	}
	if !got.hasNextPage || got.endCursor != "timeline-cursor-A" {
		t.Fatalf("fetchIssueTimelinePage(first page) pagination = %+v, want hasNextPage=true endCursor=timeline-cursor-A", got)
	}

	got, err = fake.fetchIssueTimelinePage(context.Background(), "owner", "repo", issueNumber, got.endCursor)
	if err != nil {
		t.Fatalf("fetchIssueTimelinePage(second page): unexpected error: %v", err)
	}
	if len(got.linkedPRs) != 1 || got.linkedPRs[0].Number != secondPage.linkedPRs[0].Number {
		t.Fatalf("fetchIssueTimelinePage(second page) = %+v, want linked PR number %d", got, secondPage.linkedPRs[0].Number)
	}
	if got.hasNextPage {
		t.Fatalf("fetchIssueTimelinePage(second page).hasNextPage = true, want false (last page)")
	}

	wantCalls := []timelineCursorCall{
		{issueNumber: issueNumber, cursor: ""},
		{issueNumber: issueNumber, cursor: "timeline-cursor-A"},
	}
	if len(fake.calledTimelineCursors) != len(wantCalls) {
		t.Fatalf("calledTimelineCursors = %+v, want %+v", fake.calledTimelineCursors, wantCalls)
	}
	for i, want := range wantCalls {
		if fake.calledTimelineCursors[i] != want {
			t.Fatalf("calledTimelineCursors = %+v, want %+v", fake.calledTimelineCursors, wantCalls)
		}
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
