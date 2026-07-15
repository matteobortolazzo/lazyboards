package provider

import (
	"context"
	"errors"
	"testing"

	"github.com/shurcooL/githubv4"
)

// TestMapOpenPRsQuery_MapsNodesAndPageInfo pins the repo-wide open-PR query
// mapping: nodes map through mapLinkedPRs (same shape/dedup as the closing-PR
// connections) and pageInfo carries through for pagination.
func TestMapOpenPRsQuery_MapsNodesAndPageInfo(t *testing.T) {
	var q openPRsQuery
	q.Repository.PullRequests.Nodes = []pullRequestQueryNode{
		buildClosingPRItem(42, "feat: repo-wide PR", "https://github.com/o/r/pull/42"),
		buildClosingPRItem(42, "feat: repo-wide PR", "https://github.com/o/r/pull/42"),
	}
	q.Repository.PullRequests.Nodes[0].HeadRefName = githubv4.String("feature/repo-wide")
	q.Repository.PullRequests.PageInfo = pageInfoFragment{HasNextPage: true, EndCursor: "pr-cursor-A"}

	got := mapOpenPRsQuery(q)

	if len(got.prs) != 1 {
		t.Fatalf("mapOpenPRsQuery().prs has %d entries, want 1 (duplicate PR must be deduped): %+v", len(got.prs), got.prs)
	}
	pr := got.prs[0]
	if pr.Number != 42 || pr.Title != "feat: repo-wide PR" || pr.URL != "https://github.com/o/r/pull/42" || pr.Branch != "feature/repo-wide" {
		t.Fatalf("mapOpenPRsQuery().prs[0] = %+v, want {Number:42 Title:%q URL:%q Branch:%q}",
			pr, "feat: repo-wide PR", "https://github.com/o/r/pull/42", "feature/repo-wide")
	}
	if !got.hasNextPage || got.endCursor != "pr-cursor-A" {
		t.Fatalf("mapOpenPRsQuery() pagination = %+v, want hasNextPage=true endCursor=pr-cursor-A", got)
	}
}

// TestListOpenPRs_PagesThroughAllPagesAndDedupes asserts that ListOpenPRs
// follows the pullRequests connection's cursors to the end, preserves the
// provider's page order, and returns a PR repeated across a page boundary
// only once.
func TestListOpenPRs_PagesThroughAllPagesAndDedupes(t *testing.T) {
	gql := &fakeGraphQLClient{
		openPRPages: map[string]openPRPage{
			"": {
				prs: []LinkedPR{
					{Number: 12, Title: "third PR", URL: "https://github.com/o/r/pull/12"},
					{Number: 11, Title: "second PR", URL: "https://github.com/o/r/pull/11"},
				},
				hasNextPage: true,
				endCursor:   "pr-cursor-A",
			},
			"pr-cursor-A": {
				prs: []LinkedPR{
					{Number: 11, Title: "second PR", URL: "https://github.com/o/r/pull/11"},
					{Number: 10, Title: "first PR", URL: "https://github.com/o/r/pull/10"},
				},
			},
		},
	}
	p := NewGitHubProvider(nil, gql, "o", "r", []string{"New"})

	got, err := p.ListOpenPRs(context.Background())
	if err != nil {
		t.Fatalf("ListOpenPRs() unexpected error: %v", err)
	}

	wantNumbers := []int{12, 11, 10}
	if len(got) != len(wantNumbers) {
		t.Fatalf("ListOpenPRs() returned %d PRs, want %d (cross-page duplicate must be deduped): %+v", len(got), len(wantNumbers), got)
	}
	for i, want := range wantNumbers {
		if got[i].Number != want {
			t.Errorf("ListOpenPRs()[%d].Number = %d, want %d", i, got[i].Number, want)
		}
	}

	wantCursors := []string{"", "pr-cursor-A"}
	if len(gql.calledOpenPRCursors) != len(wantCursors) {
		t.Fatalf("calledOpenPRCursors = %v, want %v", gql.calledOpenPRCursors, wantCursors)
	}
	for i, want := range wantCursors {
		if gql.calledOpenPRCursors[i] != want {
			t.Fatalf("calledOpenPRCursors = %v, want %v", gql.calledOpenPRCursors, wantCursors)
		}
	}
}

// TestListOpenPRs_ReturnsErrorFromGQL asserts a GraphQL failure propagates to
// the caller instead of silently returning a partial/empty list.
func TestListOpenPRs_ReturnsErrorFromGQL(t *testing.T) {
	wantErr := errors.New("graphql: rate limited")
	gql := &fakeGraphQLClient{err: wantErr}
	p := NewGitHubProvider(nil, gql, "o", "r", []string{"New"})

	_, err := p.ListOpenPRs(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ListOpenPRs() error = %v, want %v", err, wantErr)
	}
}
