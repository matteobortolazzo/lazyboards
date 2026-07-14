package provider

import (
	"context"
)

// graphQLBoardClient is a narrow, typed-result seam over the GitHub GraphQL
// API used to fetch issues (and their linked PRs) in a single paginated
// query, instead of one REST timeline call per issue.
//
// It intentionally does NOT expose githubv4's raw Query(ctx, q interface{},
// vars) reflection-based API: a fake implementing that signature would need
// to reflect into the caller's query struct to populate it, which is
// brittle. Returning plain result structs keeps fakes trivial to write.
type graphQLBoardClient interface {
	fetchIssuePage(ctx context.Context, owner, repo, afterCursor string) (issuePage, error)
}

// issuePage is one page of issues returned by a GraphQL query, decoupled
// from any githubv4-specific types.
type issuePage struct {
	issues      []issueNode
	hasNextPage bool
	endCursor   string
}

// issueNode is a single issue and its linked PRs, as mapped from a GraphQL
// response. hasMoreTimelineItems/timelineEndCursor support a future bounded
// per-issue follow-up query for issues with more than 100 cross-referenced
// timeline items (a rare case a single page can't fully capture).
type issueNode struct {
	number    int
	title     string
	body      string
	url       string
	labels    []Label
	assignees []Assignee
	linkedPRs []LinkedPR

	hasMoreTimelineItems bool
	timelineEndCursor    string
}
