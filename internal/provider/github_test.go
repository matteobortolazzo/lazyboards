package provider

import (
	"context"

	"github.com/google/go-github/v68/github"
)

// mockGitHubClient implements GitHubClient with configurable return values.
type mockGitHubClient struct {
	issues                []*github.Issue
	err                   error
	createdIssue          *github.Issue                  // returned by Create
	createErr             error                          // returned by Create
	editedIssue           *github.Issue                  // returned by Edit
	editErr               error                          // returned by Edit
	createdLabel          *github.Label                  // returned by CreateLabel
	createLabelErr        error                          // returned by CreateLabel
	capturedOpts          *github.IssueListByRepoOptions // captured from ListByRepo
	collaborators         []*github.User                 // returned by ListCollaborators
	collaboratorsErr      error                          // returned by ListCollaborators
	authenticatedUser     *github.User                   // returned by GetUser
	authUserErr           error                          // returned by GetUser
	capturedEditReq       *github.IssueRequest           // captured from Edit for assertion
	labels                []*github.Label                // returned by ListLabels
	labelsErr             error                          // returned by ListLabels
	createdComment        *github.IssueComment           // returned by CreateComment
	createCommentErr      error                          // returned by CreateComment
	capturedCommentNumber int                            // captured issue number from CreateComment
	capturedComment       *github.IssueComment           // captured comment body from CreateComment
}

func (m *mockGitHubClient) ListByRepo(
	_ context.Context,
	_ string,
	_ string,
	opts *github.IssueListByRepoOptions,
) ([]*github.Issue, *github.Response, error) {
	m.capturedOpts = opts
	return m.issues, nil, m.err
}

func (m *mockGitHubClient) Create(
	_ context.Context,
	_ string,
	_ string,
	_ *github.IssueRequest,
) (*github.Issue, *github.Response, error) {
	return m.createdIssue, nil, m.createErr
}

func (m *mockGitHubClient) Edit(
	_ context.Context,
	_ string,
	_ string,
	_ int,
	req *github.IssueRequest,
) (*github.Issue, *github.Response, error) {
	m.capturedEditReq = req
	return m.editedIssue, nil, m.editErr
}

func (m *mockGitHubClient) CreateLabel(
	_ context.Context,
	_ string,
	_ string,
	_ *github.Label,
) (*github.Label, *github.Response, error) {
	return m.createdLabel, nil, m.createLabelErr
}

func (m *mockGitHubClient) ListCollaborators(
	_ context.Context,
	_ string,
	_ string,
	_ *github.ListCollaboratorsOptions,
) ([]*github.User, *github.Response, error) {
	return m.collaborators, nil, m.collaboratorsErr
}

func (m *mockGitHubClient) GetUser(
	_ context.Context,
	_ string,
) (*github.User, *github.Response, error) {
	return m.authenticatedUser, nil, m.authUserErr
}

func (m *mockGitHubClient) ListLabels(
	_ context.Context,
	_ string,
	_ string,
	_ *github.ListOptions,
) ([]*github.Label, *github.Response, error) {
	return m.labels, nil, m.labelsErr
}

func (m *mockGitHubClient) CreateComment(
	_ context.Context,
	_ string,
	_ string,
	number int,
	comment *github.IssueComment,
) (*github.IssueComment, *github.Response, error) {
	m.capturedCommentNumber = number
	m.capturedComment = comment
	return m.createdComment, nil, m.createCommentErr
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

// buildIssueNode constructs an issueNode with the given number, title, and
// label names, mirroring makeIssue's REST-fixture shape but as the already-
// mapped GraphQL issueNode value FetchBoard now consumes (labels are plain
// provider.Label values, matching mapIssueQueryNode's output).
func buildIssueNode(number int, title string, labelNames ...string) issueNode {
	labels := make([]Label, len(labelNames))
	for i, name := range labelNames {
		labels[i] = Label{Name: name}
	}
	return issueNode{number: number, title: title, labels: labels}
}
