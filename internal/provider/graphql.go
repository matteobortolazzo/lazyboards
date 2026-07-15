package provider

import (
	"context"

	"github.com/shurcooL/githubv4"
)

// graphQLBoardClient is a narrow, typed-result seam over the GitHub GraphQL
// API used to fetch issues (and the open PRs that GitHub recognizes as
// closing them) in a single paginated query.
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
type issueNode struct {
	number    int
	title     string
	body      string
	url       string
	labels    []Label
	assignees []Assignee
	linkedPRs []LinkedPR

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
//	        labels(first: 50) { nodes { name color } }
//	        assignees(first: 20) { nodes { login } }
//	        closedByPullRequestsReferences(first: 100) {
//	          nodes { number title url headRefName }
//	          pageInfo { hasNextPage endCursor }
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
	Number githubv4.Int
	Title  githubv4.String
	Body   githubv4.String
	URL    githubv4.String
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
}

type labelQueryNode struct {
	Name  githubv4.String
	Color githubv4.String
}

type assigneeQueryNode struct {
	Login githubv4.String
}

// pullRequestQueryNode represents a PR from GitHub's
// closedByPullRequestsReferences connection.
type pullRequestQueryNode struct {
	Number      githubv4.Int
	Title       githubv4.String
	URL         githubv4.String
	HeadRefName githubv4.String
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
//	        nodes { number title url headRefName }
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
//	      nodes { number title url headRefName }
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
		number:             int(n.Number),
		title:              string(n.Title),
		body:               string(n.Body),
		url:                string(n.URL),
		labels:             labels,
		assignees:          assignees,
		linkedPRs:          mapLinkedPRs(n.ClosedByPullRequestsReferences.Nodes),
		hasMoreClosingPRs:  bool(n.ClosedByPullRequestsReferences.PageInfo.HasNextPage),
		closingPREndCursor: string(n.ClosedByPullRequestsReferences.PageInfo.EndCursor),
	}
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
			Number: number,
			Title:  string(pr.Title),
			URL:    string(pr.URL),
			Branch: string(pr.HeadRefName),
		})
	}
	return linkedPRs
}
