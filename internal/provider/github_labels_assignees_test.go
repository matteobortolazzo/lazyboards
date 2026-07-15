package provider

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-github/v68/github"
)

// --- CreateLabel Tests ---

func TestGitHubCreateLabel_Success(t *testing.T) {
	labelName := "new-label"
	client := &mockGitHubClient{
		createdLabel: &github.Label{Name: github.Ptr(labelName)},
	}
	columns := []string{"New"}
	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	err := provider.CreateLabel(context.Background(), labelName)
	if err != nil {
		t.Fatalf("CreateLabel returned error: %v", err)
	}
}

func TestGitHubCreateLabel_APIError(t *testing.T) {
	apiErrMsg := "API request failed"
	client := &mockGitHubClient{
		createLabelErr: errors.New(apiErrMsg),
	}
	columns := []string{"New"}
	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	err := provider.CreateLabel(context.Background(), "some-label")
	if err == nil {
		t.Fatal("expected error from CreateLabel, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}
}

func TestGitHubCreateLabel_AlreadyExists(t *testing.T) {
	client := &mockGitHubClient{
		createLabelErr: &github.ErrorResponse{
			Response: &http.Response{StatusCode: 422},
			Message:  "Validation Failed",
		},
	}
	columns := []string{"New"}
	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	err := provider.CreateLabel(context.Background(), "existing-label")
	if err == nil {
		t.Fatal("expected error from CreateLabel for duplicate label, got nil")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "already exists")
	}
	if !errors.Is(err, ErrLabelExists) {
		t.Errorf("error = %v, want it to wrap ErrLabelExists", err)
	}
}

// pagedLabelsMockClient returns labels across multiple pages to exercise the
// pagination loop in ListLabels.
type pagedLabelsMockClient struct {
	mockGitHubClient
	pages [][]*github.Label
	call  int
}

func (m *pagedLabelsMockClient) ListLabels(
	_ context.Context,
	_ string,
	_ string,
	_ *github.ListOptions,
) ([]*github.Label, *github.Response, error) {
	if m.call >= len(m.pages) {
		return nil, &github.Response{}, nil
	}
	page := m.pages[m.call]
	m.call++
	resp := &github.Response{}
	if m.call < len(m.pages) {
		// More pages remain; advertise the next page number (1-indexed).
		resp.NextPage = m.call + 1
	}
	return page, resp, nil
}

func TestGitHubListLabels_PaginatesAndExtractsNames(t *testing.T) {
	client := &pagedLabelsMockClient{
		pages: [][]*github.Label{
			{{Name: github.Ptr("bug")}, {Name: github.Ptr("feature")}},
			{{Name: github.Ptr("planned")}},
		},
	}
	p := NewGitHubProvider(client, nil, "owner", "repo", []string{"New"})

	labels, err := p.ListLabels(context.Background())
	if err != nil {
		t.Fatalf("ListLabels returned error: %v", err)
	}

	want := []string{"bug", "feature", "planned"}
	if len(labels) != len(want) {
		t.Fatalf("got %d labels %v, want %d %v", len(labels), labels, len(want), want)
	}
	for i, name := range want {
		if labels[i] != name {
			t.Errorf("labels[%d] = %q, want %q", i, labels[i], name)
		}
	}
}

func TestGitHubListLabels_APIError(t *testing.T) {
	apiErrMsg := "boom"
	client := &mockGitHubClient{labelsErr: errors.New(apiErrMsg)}
	p := NewGitHubProvider(client, nil, "owner", "repo", []string{"New"})

	_, err := p.ListLabels(context.Background())
	if err == nil {
		t.Fatal("expected error from ListLabels, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}
}

// --- Assignee Extraction Tests ---

func TestExtractAssignees_MultipleAssignees(t *testing.T) {
	assigneeLogin1 := "alice"
	assigneeLogin2 := "bob"
	ghAssignees := []*github.User{
		{Login: github.Ptr(assigneeLogin1)},
		{Login: github.Ptr(assigneeLogin2)},
	}

	assignees := extractAssignees(ghAssignees)

	if len(assignees) != 2 {
		t.Fatalf("extractAssignees returned %d assignees, want 2", len(assignees))
	}
	if assignees[0].Login != assigneeLogin1 {
		t.Errorf("assignees[0].Login = %q, want %q", assignees[0].Login, assigneeLogin1)
	}
	if assignees[1].Login != assigneeLogin2 {
		t.Errorf("assignees[1].Login = %q, want %q", assignees[1].Login, assigneeLogin2)
	}
}

func TestExtractAssignees_NoAssignees(t *testing.T) {
	assignees := extractAssignees([]*github.User{})

	if len(assignees) != 0 {
		t.Errorf("extractAssignees returned %d assignees for empty input, want 0", len(assignees))
	}
}

func TestExtractAssignees_NilAssignees(t *testing.T) {
	assignees := extractAssignees(nil)

	if len(assignees) != 0 {
		t.Errorf("extractAssignees returned %d assignees for nil input, want 0", len(assignees))
	}
}

// --- FetchCollaborators Tests ---

func TestGitHubFetchCollaborators_Success(t *testing.T) {
	login1 := "alice"
	login2 := "bob"
	client := &mockGitHubClient{
		collaborators: []*github.User{
			{Login: github.Ptr(login1)},
			{Login: github.Ptr(login2)},
		},
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	collaborators, err := p.FetchCollaborators(context.Background())
	if err != nil {
		t.Fatalf("FetchCollaborators returned error: %v", err)
	}

	if len(collaborators) != 2 {
		t.Fatalf("got %d collaborators, want 2", len(collaborators))
	}
	if collaborators[0].Login != login1 {
		t.Errorf("collaborators[0].Login = %q, want %q", collaborators[0].Login, login1)
	}
	if collaborators[1].Login != login2 {
		t.Errorf("collaborators[1].Login = %q, want %q", collaborators[1].Login, login2)
	}
}

func TestGitHubFetchCollaborators_APIError(t *testing.T) {
	apiErrMsg := "API rate limit exceeded"
	client := &mockGitHubClient{
		collaboratorsErr: errors.New(apiErrMsg),
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	_, err := p.FetchCollaborators(context.Background())
	if err == nil {
		t.Fatal("expected error from FetchCollaborators, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}
}

func TestGitHubFetchCollaborators_EmptyList(t *testing.T) {
	client := &mockGitHubClient{
		collaborators: []*github.User{},
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	collaborators, err := p.FetchCollaborators(context.Background())
	if err != nil {
		t.Fatalf("FetchCollaborators returned error: %v", err)
	}

	if len(collaborators) != 0 {
		t.Errorf("got %d collaborators for empty repo, want 0", len(collaborators))
	}
}

// --- GetAuthenticatedUser Tests ---

func TestGitHubGetAuthenticatedUser_Success(t *testing.T) {
	expectedLogin := "testuser"
	client := &mockGitHubClient{
		authenticatedUser: &github.User{Login: github.Ptr(expectedLogin)},
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	login, err := p.GetAuthenticatedUser(context.Background())
	if err != nil {
		t.Fatalf("GetAuthenticatedUser returned error: %v", err)
	}

	if login != expectedLogin {
		t.Errorf("login = %q, want %q", login, expectedLogin)
	}
}

func TestGitHubGetAuthenticatedUser_APIError(t *testing.T) {
	apiErrMsg := "unauthorized"
	client := &mockGitHubClient{
		authUserErr: errors.New(apiErrMsg),
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	_, err := p.GetAuthenticatedUser(context.Background())
	if err == nil {
		t.Fatal("expected error from GetAuthenticatedUser, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}
}
