package provider

import (
	"context"
	"time"

	"github.com/shurcooL/githubv4"
)

// graphQLBoardClient is a narrow, typed-result seam over the GitHub GraphQL
// API used to fetch issues (and their linked PRs -- the open PRs GitHub
// recognizes as closing them, unioned with open PRs that merely mention
// them, #441) in a single paginated query.
//
// It intentionally does NOT expose githubv4's raw Query(ctx, q interface{},
// vars) reflection-based API: a fake implementing that signature would need
// to reflect into the caller's query struct to populate it, which is
// brittle. Returning plain result structs keeps fakes trivial to write.
type graphQLBoardClient interface {
	fetchIssuePage(ctx context.Context, owner, repo, afterCursor string) (issuePage, error)

	// fetchIssueClosingPRPage fetches a bounded follow-up page of a single
	// issue's closedByPullRequestsReferences connection.
	fetchIssueClosingPRPage(ctx context.Context, owner, repo string, issueNumber int, cursor string) (closingPRPage, error)

	// fetchOpenPRPage fetches one page of the repository's open pull
	// requests (repo-wide, independent of any issue).
	fetchOpenPRPage(ctx context.Context, owner, repo, afterCursor string) (openPRPage, error)

	// fetchMilestonePage fetches one page of the repository's open
	// milestones (repo-wide, independent of any issue).
	fetchMilestonePage(ctx context.Context, owner, repo, afterCursor string) (milestonePage, error)

	// deleteIssue permanently deletes the given issue via GraphQL's
	// deleteIssue mutation (REST has no delete-issue endpoint). Number-based
	// like the other seam methods; implementations resolve the issue's
	// GraphQL node ID internally.
	deleteIssue(ctx context.Context, owner, repo string, number int) error
}

// maxClosingPRFollowupPages bounds the number of per-issue closing-PR follow-up
// queries fetchIssueClosingPRPage can be called for a single issue (100
// initial + 500 follow-up = 600 closing PRs/issue max). At the cap, callers
// keep whatever LinkedPRs were collected so far and continue rather than
// erroring the whole board fetch.
const maxClosingPRFollowupPages = 5

// issuePage is one page of issues returned by a GraphQL query, decoupled
// from any githubv4-specific types.
type issuePage struct {
	issues      []issueNode
	hasNextPage bool
	endCursor   string
}

// issueNode is a single issue and its linked PRs, as mapped from a GraphQL
// response. hasMoreClosingPRs/closingPREndCursor support a bounded per-issue
// follow-up query for issues with more than 100 closing PRs.
//
// parentNumber/subIssueCount/subIssueCompleted carry GitHub's native
// sub-issue relationship (#460, #475), mapped from the parent{number} and
// subIssuesSummary{total completed} GraphQL fields; all default to 0
// ("none") for issues without the relationship.
//
// blockedByCount/totalBlockedByCount/blockingCount/totalBlockingCount/
// blockers carry GitHub's native issue-dependency relationship (#628, #630),
// mapped from issueDependenciesSummary and the bounded blockedBy(first: 10)
// connection; all default to 0/nil ("none") for issues without the
// relationship.
type issueNode struct {
	number            int
	title             string
	body              string
	url               string
	labels            []Label
	assignees         []Assignee
	linkedPRs         []LinkedPR
	milestone         string
	createdAt         time.Time
	parentNumber      int
	subIssueCount     int
	subIssueCompleted int

	blockedByCount      int
	totalBlockedByCount int
	blockingCount       int
	totalBlockingCount  int
	blockers            []Blocker

	hasMoreClosingPRs  bool
	closingPREndCursor string
}

// issuesQuery is the githubv4 struct-tag-based query DSL representation of:
//
//	query($owner: String!, $name: String!, $issueCursor: String) {
//	  repository(owner: $owner, name: $name) {
//	    issues(states: [OPEN], orderBy: {field: CREATED_AT, direction: ASC}, first: 100, after: $issueCursor) {
//	      nodes {
//	        number
//	        title
//	        body
//	        url
//	        createdAt
//	        milestone { title }
//	        labels(first: 50) { nodes { name color } }
//	        assignees(first: 20) { nodes { login } }
//	        closedByPullRequestsReferences(first: 100) {
//	          nodes { number title url headRefName }
//	          pageInfo { hasNextPage endCursor }
//	        }
//	        timelineItems(first: 100, itemTypes: [CROSS_REFERENCED_EVENT]) {
//	          nodes {
//	            ... on CrossReferencedEvent {
//	              source {
//	                ... on PullRequest { number title url headRefName state }
//	              }
//	            }
//	          }
//	        }
//	        parent { number }
//	        subIssuesSummary { total completed }
//	        issueDependenciesSummary { blockedBy totalBlockedBy blocking totalBlocking }
//	        blockedBy(first: 10) {
//	          nodes { number state url repository { nameWithOwner } }
//	        }
//	      }
//	      pageInfo { hasNextPage endCursor }
//	    }
//	  }
//	}
type issuesQuery struct {
	Repository struct {
		Issues struct {
			Nodes    []issueQueryNode
			PageInfo pageInfoFragment
		} `graphql:"issues(states: [OPEN], orderBy: {field: CREATED_AT, direction: ASC}, first: 100, after: $issueCursor)"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

// pageInfoFragment mirrors GraphQL's standard Relay PageInfo shape.
type pageInfoFragment struct {
	HasNextPage githubv4.Boolean
	EndCursor   githubv4.String
}

type issueQueryNode struct {
	Number    githubv4.Int
	Title     githubv4.String
	Body      githubv4.String
	URL       githubv4.String
	CreatedAt githubv4.DateTime
	Milestone struct {
		Title githubv4.String
	}
	Labels struct {
		Nodes []labelQueryNode
	} `graphql:"labels(first: 50)"`
	Assignees struct {
		Nodes []assigneeQueryNode
	} `graphql:"assignees(first: 20)"`
	ClosedByPullRequestsReferences struct {
		Nodes    []pullRequestQueryNode
		PageInfo pageInfoFragment
	} `graphql:"closedByPullRequestsReferences(first: 100)"`
	// TimelineItems captures the first 100 cross-referenced mentions of this
	// issue (#441). Unlike ClosedByPullRequestsReferences, there is no
	// follow-up pagination for mentions beyond this page -- an issue with
	// over 100 distinct PR mentions is rare enough that a bounded first page
	// is an acceptable, deliberate scope cut rather than an oversight;
	// follow-up pagination can be added the same way fetchIssueClosingPRPage
	// was if this proves insufficient in practice.
	TimelineItems struct {
		Nodes []timelineItemQueryNode
	} `graphql:"timelineItems(first: 100, itemTypes: [CROSS_REFERENCED_EVENT])"`
	// Parent and SubIssuesSummary request GitHub's native sub-issue
	// relationship (#460, #475): Parent.Number is this issue's parent (0 if
	// none), SubIssuesSummary.Total is this issue's sub-issue count (0 if
	// none), and SubIssuesSummary.Completed is how many of those sub-issues
	// are closed (0 if none).
	Parent struct {
		Number githubv4.Int
	} `graphql:"parent"`
	SubIssuesSummary struct {
		Total     githubv4.Int
		Completed githubv4.Int
	} `graphql:"subIssuesSummary"`
	// IssueDependenciesSummary and BlockedBy request GitHub's native
	// issue-dependency relationship (#628, #630): the four counts are
	// verbatim GraphQL fields (BlockedBy/Blocking are open-only counts,
	// TotalBlockedBy/TotalBlocking include closed issues too), and
	// BlockedBy.Nodes is a bounded first page (first: 10) of the issues
	// blocking this one -- no follow-up pagination, mirroring TimelineItems'
	// deliberate scope cut above.
	IssueDependenciesSummary struct {
		BlockedBy      githubv4.Int
		TotalBlockedBy githubv4.Int
		Blocking       githubv4.Int
		TotalBlocking  githubv4.Int
	} `graphql:"issueDependenciesSummary"`
	BlockedBy struct {
		Nodes []blockerQueryNode
	} `graphql:"blockedBy(first: 10)"`
}

type labelQueryNode struct {
	Name  githubv4.String
	Color githubv4.String
}

type assigneeQueryNode struct {
	Login githubv4.String
}

// blockerQueryNode represents a single issue from an issue's blockedBy
// connection (#628, #630) -- the issues blocking this one.
type blockerQueryNode struct {
	Number     githubv4.Int
	State      githubv4.IssueState
	URL        githubv4.String
	Repository struct {
		NameWithOwner githubv4.String
	}
}

// pullRequestQueryNode represents a PR from GitHub's
// closedByPullRequestsReferences, pullRequests, and cross-referenced
// timeline-mention connections. State is consumed by both the mention path
// (mapMentionedPRs, which filters out stale mentions of PRs that have since
// closed or merged so a dead link doesn't resurrect on a still-open issue)
// and the closing-PR path (mapLinkedPRs, which carries State through to
// LinkedPR so consumers like the global PR list can exclude closed/merged
// entries from open-PR fallbacks).
type pullRequestQueryNode struct {
	Number           githubv4.Int
	Title            githubv4.String
	URL              githubv4.String
	HeadRefName      githubv4.String
	IsDraft          githubv4.Boolean
	Mergeable        githubv4.MergeableState
	MergeStateStatus githubv4.MergeStateStatus
	State            githubv4.PullRequestState
}

// timelineItemQueryNode represents one CROSS_REFERENCED_EVENT timeline item
// on an issue -- a PR (or another issue) that mentions this issue somewhere
// in its body/title, whether or not the mention uses a closing keyword
// (e.g. this project's own stacked-PR convention, "Stack: 2/3 -- depends on
// #<prev>" per docs/git-workflow.md, is a non-closing mention). Source is a
// GraphQL union (Issue or PullRequest); only the "... on PullRequest" inline
// fragment is requested, so a cross-reference sourced from a plain Issue
// leaves PullRequest zero-valued (Number == 0), which mapMentionedPRs
// filters out.
type timelineItemQueryNode struct {
	CrossReferencedEvent struct {
		Source struct {
			PullRequest pullRequestQueryNode `graphql:"... on PullRequest"`
		}
	} `graphql:"... on CrossReferencedEvent"`
}

// closingPRPage is one follow-up page of an issue's closing PRs
// connection, decoupled from any githubv4-specific types. It mirrors
// issuePage's pagination shape but for the nested per-issue closing PR
// connection rather than the outer issues connection.
type closingPRPage struct {
	linkedPRs   []LinkedPR
	hasNextPage bool
	endCursor   string
}

// issueClosingPRQuery is the githubv4 struct-tag-based query DSL
// representation of a bounded follow-up query for a single issue's
// closedByPullRequestsReferences connection:
//
//	query($owner: String!, $name: String!, $issueNumber: Int!, $closingPRCursor: String) {
//	  repository(owner: $owner, name: $name) {
//	    issue(number: $issueNumber) {
//	      closedByPullRequestsReferences(first: 100, after: $closingPRCursor) {
//	        nodes { number title url headRefName isDraft mergeable mergeStateStatus }
//	        pageInfo { hasNextPage endCursor }
//	      }
//	    }
//	  }
//	}
type issueClosingPRQuery struct {
	Repository struct {
		Issue struct {
			ClosedByPullRequestsReferences struct {
				Nodes    []pullRequestQueryNode
				PageInfo pageInfoFragment
			} `graphql:"closedByPullRequestsReferences(first: 100, after: $closingPRCursor)"`
		} `graphql:"issue(number: $issueNumber)"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

// openPRPage is one page of the repository's open pull requests, decoupled
// from any githubv4-specific types. Mirrors issuePage's pagination shape but
// for the repo-wide pullRequests connection rather than the issues connection.
type openPRPage struct {
	prs         []LinkedPR
	hasNextPage bool
	endCursor   string
}

// openPRsQuery is the githubv4 struct-tag-based query DSL representation of:
//
//	query($owner: String!, $name: String!, $prCursor: String) {
//	  repository(owner: $owner, name: $name) {
//	    pullRequests(states: [OPEN], orderBy: {field: CREATED_AT, direction: DESC}, first: 100, after: $prCursor) {
//	      nodes { number title url headRefName isDraft mergeable mergeStateStatus }
//	      pageInfo { hasNextPage endCursor }
//	    }
//	  }
//	}
type openPRsQuery struct {
	Repository struct {
		PullRequests struct {
			Nodes    []pullRequestQueryNode
			PageInfo pageInfoFragment
		} `graphql:"pullRequests(states: [OPEN], orderBy: {field: CREATED_AT, direction: DESC}, first: 100, after: $prCursor)"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

// mapOpenPRsQuery converts a githubv4 openPRsQuery response into a plain
// openPRPage. It reuses mapLinkedPRs so rows carry the same shape and
// within-page dedup semantics as the closing-PR connections.
func mapOpenPRsQuery(q openPRsQuery) openPRPage {
	items := q.Repository.PullRequests
	return openPRPage{
		prs:         mapLinkedPRs(items.Nodes),
		hasNextPage: bool(items.PageInfo.HasNextPage),
		endCursor:   string(items.PageInfo.EndCursor),
	}
}

// milestonePage is one page of the repository's open milestones, decoupled
// from any githubv4-specific types. Mirrors openPRPage's pagination shape
// but for the repo-wide milestones connection.
type milestonePage struct {
	milestones  []Milestone
	hasNextPage bool
	endCursor   string
}

// milestonesQuery is the githubv4 struct-tag-based query DSL representation of:
//
//	query($owner: String!, $name: String!, $milestoneCursor: String) {
//	  repository(owner: $owner, name: $name) {
//	    milestones(states: [OPEN], first: 100, after: $milestoneCursor, orderBy: {field: DUE_DATE, direction: ASC}) {
//	      nodes { title url dueOn openIssueCount closedIssueCount progressPercentage }
//	      pageInfo { hasNextPage endCursor }
//	    }
//	  }
//	}
type milestonesQuery struct {
	Repository struct {
		Milestones struct {
			Nodes    []milestoneQueryNode
			PageInfo pageInfoFragment
		} `graphql:"milestones(states: [OPEN], first: 100, after: $milestoneCursor, orderBy: {field: DUE_DATE, direction: ASC})"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

// milestoneQueryNode is a single milestone as requested by milestonesQuery.
// DueOn is a pointer because GraphQL's dueOn is nullable: a non-pointer
// githubv4.DateTime would decode a null dueOn into the zero time.Time,
// destroying the "no due date" distinction.
type milestoneQueryNode struct {
	Title              githubv4.String
	URL                githubv4.String
	DueOn              *githubv4.DateTime
	OpenIssueCount     githubv4.Int
	ClosedIssueCount   githubv4.Int
	ProgressPercentage githubv4.Float
}

// mapMilestonesQuery converts a githubv4 milestonesQuery response into a
// plain milestonePage, decoupled from any githubv4-specific types.
// ProgressPercentage is carried through verbatim from GitHub's field, never
// derived from the two counts (see the Milestone doc comment in provider.go).
func mapMilestonesQuery(q milestonesQuery) milestonePage {
	nodes := q.Repository.Milestones.Nodes
	milestones := make([]Milestone, 0, len(nodes))
	for _, n := range nodes {
		var dueOn *time.Time
		if n.DueOn != nil {
			t := n.DueOn.Time
			dueOn = &t
		}
		milestones = append(milestones, Milestone{
			Title:              string(n.Title),
			URL:                string(n.URL),
			DueOn:              dueOn,
			OpenIssueCount:     int(n.OpenIssueCount),
			ClosedIssueCount:   int(n.ClosedIssueCount),
			ProgressPercentage: float64(n.ProgressPercentage),
		})
	}
	return milestonePage{
		milestones:  milestones,
		hasNextPage: bool(q.Repository.Milestones.PageInfo.HasNextPage),
		endCursor:   string(q.Repository.Milestones.PageInfo.EndCursor),
	}
}

// mapIssueClosingPRQuery converts a githubv4 issueClosingPRQuery response into
// a plain closingPRPage, decoupled from any githubv4-specific types. It
// reuses mapLinkedPRs for the same dedup semantics as the outer query.
func mapIssueClosingPRQuery(q issueClosingPRQuery) closingPRPage {
	items := q.Repository.Issue.ClosedByPullRequestsReferences
	return closingPRPage{
		linkedPRs:   mapLinkedPRs(items.Nodes),
		hasNextPage: bool(items.PageInfo.HasNextPage),
		endCursor:   string(items.PageInfo.EndCursor),
	}
}

// Compile-time check: *GitHubV4Adapter implements graphQLBoardClient.
var _ graphQLBoardClient = (*GitHubV4Adapter)(nil)

// GitHubV4Adapter implements graphQLBoardClient by running issuesQuery
// against a real GitHub GraphQL API v4 client and mapping the response into
// plain issuePage/issueNode values.
type GitHubV4Adapter struct {
	client *githubv4.Client
}

// NewGitHubV4Adapter creates a GitHubV4Adapter wrapping the given githubv4.Client.
func NewGitHubV4Adapter(client *githubv4.Client) *GitHubV4Adapter {
	return &GitHubV4Adapter{client: client}
}

func (a *GitHubV4Adapter) fetchIssuePage(ctx context.Context, owner, repo, afterCursor string) (issuePage, error) {
	variables := map[string]interface{}{
		"owner":       githubv4.String(owner),
		"name":        githubv4.String(repo),
		"issueCursor": (*githubv4.String)(nil),
	}
	if afterCursor != "" {
		cursor := githubv4.String(afterCursor)
		variables["issueCursor"] = &cursor
	}

	var q issuesQuery
	if err := a.client.Query(ctx, &q, variables); err != nil {
		return issuePage{}, err
	}

	return mapIssuesQuery(q), nil
}

func (a *GitHubV4Adapter) fetchIssueClosingPRPage(ctx context.Context, owner, repo string, issueNumber int, cursor string) (closingPRPage, error) {
	variables := map[string]interface{}{
		"owner":           githubv4.String(owner),
		"name":            githubv4.String(repo),
		"issueNumber":     githubv4.Int(issueNumber),
		"closingPRCursor": (*githubv4.String)(nil),
	}
	if cursor != "" {
		c := githubv4.String(cursor)
		variables["closingPRCursor"] = &c
	}

	var q issueClosingPRQuery
	if err := a.client.Query(ctx, &q, variables); err != nil {
		return closingPRPage{}, err
	}

	return mapIssueClosingPRQuery(q), nil
}

func (a *GitHubV4Adapter) fetchOpenPRPage(ctx context.Context, owner, repo, afterCursor string) (openPRPage, error) {
	variables := map[string]interface{}{
		"owner":    githubv4.String(owner),
		"name":     githubv4.String(repo),
		"prCursor": (*githubv4.String)(nil),
	}
	if afterCursor != "" {
		cursor := githubv4.String(afterCursor)
		variables["prCursor"] = &cursor
	}

	var q openPRsQuery
	if err := a.client.Query(ctx, &q, variables); err != nil {
		return openPRPage{}, err
	}

	return mapOpenPRsQuery(q), nil
}

func (a *GitHubV4Adapter) fetchMilestonePage(ctx context.Context, owner, repo, afterCursor string) (milestonePage, error) {
	variables := map[string]interface{}{
		"owner":           githubv4.String(owner),
		"name":            githubv4.String(repo),
		"milestoneCursor": (*githubv4.String)(nil),
	}
	if afterCursor != "" {
		cursor := githubv4.String(afterCursor)
		variables["milestoneCursor"] = &cursor
	}

	var q milestonesQuery
	if err := a.client.Query(ctx, &q, variables); err != nil {
		return milestonePage{}, err
	}

	return mapMilestonesQuery(q), nil
}

// issueLookupQuery resolves an issue's GraphQL global node ID by number.
// GitHub's deleteIssue mutation requires a node ID (DeleteIssueInput.IssueID),
// not owner/repo/number, and issueQueryNode (used by FetchBoard's issuesQuery)
// carries no ID field, so a preliminary lookup query is needed:
//
//	query($owner: String!, $name: String!, $number: Int!) {
//	  repository(owner: $owner, name: $name) {
//	    issue(number: $number) { id }
//	  }
//	}
type issueLookupQuery struct {
	Repository struct {
		Issue struct {
			ID githubv4.ID
		} `graphql:"issue(number: $number)"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

// deleteIssueMutation is the githubv4 struct-tag-based mutation DSL
// representation of:
//
//	mutation($input: DeleteIssueInput!) {
//	  deleteIssue(input: $input) {
//	    clientMutationId
//	  }
//	}
type deleteIssueMutation struct {
	DeleteIssue struct {
		ClientMutationID githubv4.String
	} `graphql:"deleteIssue(input: $input)"`
}

// deleteIssue permanently deletes the issue identified by owner/repo/number.
// It first resolves the issue's GraphQL node ID via a lookup query, then runs
// the deleteIssue mutation with that ID. Not unit-tested (like
// fetchIssuePage, this is a real-network adapter path); the fakeGraphQLClient
// test double covers the number-based seam contract instead.
func (a *GitHubV4Adapter) deleteIssue(ctx context.Context, owner, repo string, number int) error {
	var lookup issueLookupQuery
	lookupVars := map[string]interface{}{
		"owner":  githubv4.String(owner),
		"name":   githubv4.String(repo),
		"number": githubv4.Int(number),
	}
	if err := a.client.Query(ctx, &lookup, lookupVars); err != nil {
		return err
	}

	input := githubv4.DeleteIssueInput{
		IssueID: lookup.Repository.Issue.ID,
	}
	var mutation deleteIssueMutation
	return a.client.Mutate(ctx, &mutation, input, nil)
}

// mapIssuesQuery converts a githubv4 issuesQuery response into a plain
// issuePage, decoupled from any githubv4-specific types.
func mapIssuesQuery(q issuesQuery) issuePage {
	nodes := q.Repository.Issues.Nodes
	issues := make([]issueNode, 0, len(nodes))
	for _, n := range nodes {
		issues = append(issues, mapIssueQueryNode(n))
	}
	return issuePage{
		issues:      issues,
		hasNextPage: bool(q.Repository.Issues.PageInfo.HasNextPage),
		endCursor:   string(q.Repository.Issues.PageInfo.EndCursor),
	}
}

func mapIssueQueryNode(n issueQueryNode) issueNode {
	labels := make([]Label, 0, len(n.Labels.Nodes))
	for _, l := range n.Labels.Nodes {
		labels = append(labels, Label{Name: string(l.Name), Color: normalizeLabelColor(string(l.Color))})
	}

	assignees := make([]Assignee, 0, len(n.Assignees.Nodes))
	for _, a := range n.Assignees.Nodes {
		assignees = append(assignees, Assignee{Login: string(a.Login)})
	}

	return issueNode{
		number:            int(n.Number),
		title:             string(n.Title),
		body:              string(n.Body),
		url:               string(n.URL),
		labels:            labels,
		assignees:         assignees,
		linkedPRs:         mergeLinkedPRs(mapLinkedPRs(n.ClosedByPullRequestsReferences.Nodes), mapMentionedPRs(n.TimelineItems.Nodes)),
		milestone:         string(n.Milestone.Title),
		createdAt:         n.CreatedAt.Time,
		parentNumber:      int(n.Parent.Number),
		subIssueCount:     int(n.SubIssuesSummary.Total),
		subIssueCompleted: int(n.SubIssuesSummary.Completed),

		blockedByCount:      int(n.IssueDependenciesSummary.BlockedBy),
		totalBlockedByCount: int(n.IssueDependenciesSummary.TotalBlockedBy),
		blockingCount:       int(n.IssueDependenciesSummary.Blocking),
		totalBlockingCount:  int(n.IssueDependenciesSummary.TotalBlocking),
		blockers:            mapBlockers(n.BlockedBy.Nodes),

		hasMoreClosingPRs:  bool(n.ClosedByPullRequestsReferences.PageInfo.HasNextPage),
		closingPREndCursor: string(n.ClosedByPullRequestsReferences.PageInfo.EndCursor),
	}
}

// mapBlockers converts a bounded page of blockedBy connection nodes into
// plain Blocker values, decoupled from any githubv4-specific types.
//
// Deliberately no state filtering and no dedup: per this package's
// raw-values-verbatim convention (see the LinkedPR doc comment in
// provider.go), a CLOSED blocker must survive the mapping unfiltered so
// TotalBlockedByCount's closed-inclusive count stays meaningful and a
// consumer can defensively filter to open blockers itself -- unlike
// mapLinkedPRs/mapMentionedPRs, which dedup/filter because their consumers
// (the linked-PR list) expect only currently-relevant open PRs.
func mapBlockers(nodes []blockerQueryNode) []Blocker {
	if len(nodes) == 0 {
		return nil
	}
	blockers := make([]Blocker, 0, len(nodes))
	for _, n := range nodes {
		// A node GitHub returns as null (e.g. a blocker in a repo the token
		// can't read) unmarshals to a zero-valued blockerQueryNode rather
		// than being omitted from the slice. Skip it here rather than
		// carrying through a phantom Blocker{Number: 0, URL: ""} that would
		// render as a bogus "#0" blocker an "open blocker" action would fail
		// to open. This is a data-hygiene guard, not state filtering -- it
		// deliberately doesn't touch State (see the doc comment above).
		if n.Number == 0 || n.URL == "" {
			continue
		}
		blockers = append(blockers, Blocker{
			Number:            int(n.Number),
			State:             string(n.State),
			URL:               string(n.URL),
			RepoNameWithOwner: string(n.Repository.NameWithOwner),
		})
	}
	return blockers
}

// mapMentionedPRs extracts open pull requests from an issue's
// CROSS_REFERENCED_EVENT timeline items -- PRs that reference this issue
// without necessarily closing it. A cross-reference whose source is an
// Issue rather than a PullRequest leaves the inline fragment zero-valued
// (Number == 0), filtered out here. Closed/merged PRs are filtered by State
// so a stale mention of a long-dead PR can't resurrect a link on an issue
// that's still open -- the same staleness problem #373 fixed for the
// closing-PR connection. Reuses mapLinkedPRs for identical dedup/mapping
// semantics.
func mapMentionedPRs(items []timelineItemQueryNode) []LinkedPR {
	var candidates []pullRequestQueryNode
	for _, item := range items {
		pr := item.CrossReferencedEvent.Source.PullRequest
		if pr.Number == 0 || pr.State != githubv4.PullRequestStateOpen {
			continue
		}
		candidates = append(candidates, pr)
	}
	return mapLinkedPRs(candidates)
}

// mergeLinkedPRs unions LinkedPR lists -- closing PRs first, then
// cross-referenced mentions -- deduping by PR number so a PR satisfying
// both (e.g. "Fixes #123" is also technically a cross-reference of #123) is
// only listed once.
func mergeLinkedPRs(lists ...[]LinkedPR) []LinkedPR {
	seen := make(map[int]bool)
	var merged []LinkedPR
	for _, list := range lists {
		for _, pr := range list {
			if seen[pr.Number] {
				continue
			}
			seen[pr.Number] = true
			merged = append(merged, pr)
		}
	}
	return merged
}

// mapLinkedPRs maps GitHub-recognized closing PRs and dedupes them by number.
func mapLinkedPRs(items []pullRequestQueryNode) []LinkedPR {
	seen := make(map[int]bool)
	var linkedPRs []LinkedPR
	for _, item := range items {
		pr := item
		if pr.Number == 0 {
			continue
		}
		number := int(pr.Number)
		if seen[number] {
			continue
		}
		seen[number] = true
		linkedPRs = append(linkedPRs, LinkedPR{
			Number:           number,
			Title:            string(pr.Title),
			URL:              string(pr.URL),
			Branch:           string(pr.HeadRefName),
			IsDraft:          bool(pr.IsDraft),
			Mergeable:        string(pr.Mergeable),
			MergeStateStatus: string(pr.MergeStateStatus),
			State:            string(pr.State),
		})
	}
	return linkedPRs
}
