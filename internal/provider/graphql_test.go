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
}

func (f *fakeGraphQLClient) fetchIssuePage(_ context.Context, _, _, afterCursor string) (issuePage, error) {
	f.calledCursors = append(f.calledCursors, afterCursor)
	if f.err != nil {
		return issuePage{}, f.err
	}
	return f.pages[afterCursor], nil
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
