package provider

import (
	"context"
	"strings"

	"github.com/shurcooL/githubv4"
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

	// fetchIssueTimelinePage fetches a bounded follow-up page of a single
	// issue's timelineItems connection, used when issueNode.hasMoreTimelineItems
	// is true (i.e. an issue has more than 100 cross-referenced timeline items).
	fetchIssueTimelinePage(ctx context.Context, owner, repo string, issueNumber int, cursor string) (timelinePage, error)
}

// maxTimelineFollowupPages bounds the number of per-issue timeline follow-up
// queries fetchIssueTimelinePage can be called for a single issue (100
// initial + 500 follow-up = 600 cross-refs/issue max). At the cap, callers
// keep whatever LinkedPRs were collected so far and continue rather than
// erroring the whole board fetch.
//
// Unused in this PR by design: FetchBoard's nested-pagination follow-up loop
// that consumes this constant lands in a stacked follow-up PR (#323 Part B).
//
//nolint:unused
const maxTimelineFollowupPages = 5

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
//	        timelineItems(first: 100, itemTypes: [CROSS_REFERENCED_EVENT]) {
//	          nodes {
//	            ... on CrossReferencedEvent {
//	              source {
//	                ... on PullRequest { number title url headRefName }
//	              }
//	            }
//	          }
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
	TimelineItems struct {
		Nodes    []timelineItemQueryNode
		PageInfo pageInfoFragment
	} `graphql:"timelineItems(first: 100, itemTypes: [CROSS_REFERENCED_EVENT])"`
}

type labelQueryNode struct {
	Name  githubv4.String
	Color githubv4.String
}

type assigneeQueryNode struct {
	Login githubv4.String
}

// timelineItemQueryNode represents one CROSS_REFERENCED_EVENT timeline item.
// CrossReferencedEvent.source is a GraphQL union (Issue or PullRequest); only
// the "... on PullRequest" inline fragment is requested here, so a
// cross-reference sourced from a plain Issue leaves PullRequest zero-valued.
type timelineItemQueryNode struct {
	CrossReferencedEvent struct {
		Source struct {
			PullRequest struct {
				Number      githubv4.Int
				Title       githubv4.String
				URL         githubv4.String
				HeadRefName githubv4.String
			} `graphql:"... on PullRequest"`
		}
	} `graphql:"... on CrossReferencedEvent"`
}

// timelinePage is one follow-up page of an issue's timelineItems
// connection, decoupled from any githubv4-specific types. It mirrors
// issuePage's pagination shape but for the nested per-issue timeline
// connection rather than the outer issues connection.
type timelinePage struct {
	linkedPRs   []LinkedPR
	hasNextPage bool
	endCursor   string
}

// issueTimelineQuery is the githubv4 struct-tag-based query DSL
// representation of a bounded follow-up query for a single issue's
// timelineItems connection, used when issueNode.hasMoreTimelineItems is true
// (i.e. an issue has more than 100 cross-referenced timeline items):
//
//	query($owner: String!, $name: String!, $issueNumber: Int!, $timelineCursor: String) {
//	  repository(owner: $owner, name: $name) {
//	    issue(number: $issueNumber) {
//	      timelineItems(itemTypes: [CROSS_REFERENCED_EVENT], first: 100, after: $timelineCursor) {
//	        nodes {
//	          ... on CrossReferencedEvent {
//	            source {
//	              ... on PullRequest { number title url headRefName }
//	            }
//	          }
//	        }
//	        pageInfo { hasNextPage endCursor }
//	      }
//	    }
//	  }
//	}
type issueTimelineQuery struct {
	Repository struct {
		Issue struct {
			TimelineItems struct {
				Nodes    []timelineItemQueryNode
				PageInfo pageInfoFragment
			} `graphql:"timelineItems(first: 100, itemTypes: [CROSS_REFERENCED_EVENT], after: $timelineCursor)"`
		} `graphql:"issue(number: $issueNumber)"`
	} `graphql:"repository(owner: $owner, name: $name)"`
}

// mapIssueTimelineQuery converts a githubv4 issueTimelineQuery response into
// a plain timelinePage, decoupled from any githubv4-specific types. It
// reuses mapLinkedPRs for the same skip/dedup semantics as the outer
// issuesQuery timeline mapping (mapIssueQueryNode).
func mapIssueTimelineQuery(q issueTimelineQuery) timelinePage {
	items := q.Repository.Issue.TimelineItems
	return timelinePage{
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

func (a *GitHubV4Adapter) fetchIssueTimelinePage(ctx context.Context, owner, repo string, issueNumber int, cursor string) (timelinePage, error) {
	variables := map[string]interface{}{
		"owner":          githubv4.String(owner),
		"name":           githubv4.String(repo),
		"issueNumber":    githubv4.Int(issueNumber),
		"timelineCursor": (*githubv4.String)(nil),
	}
	if cursor != "" {
		c := githubv4.String(cursor)
		variables["timelineCursor"] = &c
	}

	var q issueTimelineQuery
	if err := a.client.Query(ctx, &q, variables); err != nil {
		return timelinePage{}, err
	}

	return mapIssueTimelineQuery(q), nil
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
		color := strings.TrimPrefix(string(l.Color), "#")
		if !hexColorRE.MatchString(color) {
			color = ""
		}
		labels = append(labels, Label{Name: string(l.Name), Color: color})
	}

	assignees := make([]Assignee, 0, len(n.Assignees.Nodes))
	for _, a := range n.Assignees.Nodes {
		assignees = append(assignees, Assignee{Login: string(a.Login)})
	}

	return issueNode{
		number:               int(n.Number),
		title:                string(n.Title),
		body:                 string(n.Body),
		url:                  string(n.URL),
		labels:               labels,
		assignees:            assignees,
		linkedPRs:            mapLinkedPRs(n.TimelineItems.Nodes),
		hasMoreTimelineItems: bool(n.TimelineItems.PageInfo.HasNextPage),
		timelineEndCursor:    string(n.TimelineItems.PageInfo.EndCursor),
	}
}

// mapLinkedPRs extracts linked PRs from an issue's cross-referenced timeline
// items, skipping cross-references whose source is a plain Issue (the
// "... on PullRequest" union fragment didn't match, so PullRequest.Number is
// zero-valued) and deduping by PR number within the issue's timeline.
func mapLinkedPRs(items []timelineItemQueryNode) []LinkedPR {
	seen := make(map[int]bool)
	var linkedPRs []LinkedPR
	for _, item := range items {
		pr := item.CrossReferencedEvent.Source.PullRequest
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
