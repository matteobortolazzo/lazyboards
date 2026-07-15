package provider

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-github/v68/github"
)

func TestGitHubCreateCard_WithLabel(t *testing.T) {
	expectedNumber := 42
	expectedTitle := "New feature"
	expectedLabel := "bug"

	client := &mockGitHubClient{
		createdIssue: makeIssue(expectedNumber, expectedTitle, expectedLabel),
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	card, err := provider.CreateCard(context.Background(), expectedTitle, expectedLabel)
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	if card.Number != expectedNumber {
		t.Errorf("card.Number = %d, want %d", card.Number, expectedNumber)
	}
	if card.Title != expectedTitle {
		t.Errorf("card.Title = %q, want %q", card.Title, expectedTitle)
	}
	if len(card.Labels) == 0 || card.Labels[0].Name != expectedLabel {
		t.Errorf("card.Labels = %v, want [%q]", card.Labels, expectedLabel)
	}
}

func TestGitHubCreateCard_WithoutLabel(t *testing.T) {
	expectedNumber := 7
	expectedTitle := "No label issue"

	client := &mockGitHubClient{
		createdIssue: makeIssue(expectedNumber, expectedTitle),
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	card, err := provider.CreateCard(context.Background(), expectedTitle, "")
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	if card.Number != expectedNumber {
		t.Errorf("card.Number = %d, want %d", card.Number, expectedNumber)
	}
	if card.Title != expectedTitle {
		t.Errorf("card.Title = %q, want %q", card.Title, expectedTitle)
	}
	if len(card.Labels) != 0 {
		t.Errorf("card.Labels = %v, want empty slice", card.Labels)
	}
}

func TestGitHubCreateCard_InvalidLabel_ReturnsFriendlyError(t *testing.T) {
	invalidLabel := "nonexistent"
	client := &mockGitHubClient{
		createErr: &github.ErrorResponse{
			Response: &http.Response{StatusCode: 422},
			Message:  "Validation Failed",
		},
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	_, err := provider.CreateCard(context.Background(), "title", invalidLabel)
	if err == nil {
		t.Fatal("expected error from CreateCard with invalid label, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, invalidLabel) {
		t.Errorf("error = %q, want it to contain the invalid label %q", errMsg, invalidLabel)
	}
	if !strings.Contains(errMsg, "does not exist") {
		t.Errorf("error = %q, want it to contain %q", errMsg, "does not exist")
	}
}

func TestGitHubCreateCard_GenericAPIError_PassesThrough(t *testing.T) {
	apiErrMsg := "server error"
	client := &mockGitHubClient{
		createErr: errors.New(apiErrMsg),
	}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	_, err := provider.CreateCard(context.Background(), "title", "label")
	if err == nil {
		t.Fatal("expected error from CreateCard, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}
}

func TestGitHubCreateCard_CapturesLabelColorFromResponse(t *testing.T) {
	expectedLabel := "bug"
	expectedColor := "d73a4a"

	createdIssue := &github.Issue{
		Number: github.Ptr(42),
		Title:  github.Ptr("New feature"),
		Labels: []*github.Label{
			{Name: github.Ptr(expectedLabel), Color: github.Ptr(expectedColor)},
		},
	}
	client := &mockGitHubClient{createdIssue: createdIssue}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	card, err := provider.CreateCard(context.Background(), "New feature", expectedLabel)
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	if len(card.Labels) != 1 {
		t.Fatalf("card.Labels has %d entries, want 1", len(card.Labels))
	}
	if card.Labels[0].Name != expectedLabel {
		t.Errorf("card.Labels[0].Name = %q, want %q", card.Labels[0].Name, expectedLabel)
	}
	if card.Labels[0].Color != expectedColor {
		t.Errorf("card.Labels[0].Color = %q, want %q", card.Labels[0].Color, expectedColor)
	}
}

// --- UpdateCard Tests ---

func TestGitHubUpdateCard_Success(t *testing.T) {
	expectedNumber := 42
	expectedTitle := "Updated title"
	expectedBody := "Updated body content"
	expectedLabels := []struct{ Name, Color string }{
		{"bug", "d73a4a"},
		{"enhancement", "a2eeef"},
	}

	ghLabels := make([]*github.Label, len(expectedLabels))
	for i, l := range expectedLabels {
		ghLabels[i] = &github.Label{
			Name:  github.Ptr(l.Name),
			Color: github.Ptr(l.Color),
		}
	}

	client := &mockGitHubClient{
		editedIssue: &github.Issue{
			Number: github.Ptr(expectedNumber),
			Title:  github.Ptr(expectedTitle),
			Body:   github.Ptr(expectedBody),
			Labels: ghLabels,
		},
	}
	columns := []string{"New"}
	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	labelNames := make([]string, len(expectedLabels))
	for i, l := range expectedLabels {
		labelNames[i] = l.Name
	}

	card, err := provider.UpdateCard(context.Background(), expectedNumber, expectedTitle, expectedBody, labelNames)
	if err != nil {
		t.Fatalf("UpdateCard returned error: %v", err)
	}

	if card.Number != expectedNumber {
		t.Errorf("card.Number = %d, want %d", card.Number, expectedNumber)
	}
	if card.Title != expectedTitle {
		t.Errorf("card.Title = %q, want %q", card.Title, expectedTitle)
	}
	if card.Body != expectedBody {
		t.Errorf("card.Body = %q, want %q", card.Body, expectedBody)
	}

	if len(card.Labels) != len(expectedLabels) {
		t.Fatalf("card.Labels has %d entries, want %d", len(card.Labels), len(expectedLabels))
	}
	for i, expected := range expectedLabels {
		if card.Labels[i].Name != expected.Name {
			t.Errorf("card.Labels[%d].Name = %q, want %q", i, card.Labels[i].Name, expected.Name)
		}
		if card.Labels[i].Color != expected.Color {
			t.Errorf("card.Labels[%d].Color = %q, want %q", i, card.Labels[i].Color, expected.Color)
		}
	}
}

func TestGitHubUpdateCard_WithEmptyLabels(t *testing.T) {
	expectedNumber := 10
	expectedTitle := "No labels issue"
	expectedBody := "Body without labels"

	client := &mockGitHubClient{
		editedIssue: &github.Issue{
			Number: github.Ptr(expectedNumber),
			Title:  github.Ptr(expectedTitle),
			Body:   github.Ptr(expectedBody),
			Labels: []*github.Label{},
		},
	}
	columns := []string{"New"}
	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	card, err := provider.UpdateCard(context.Background(), expectedNumber, expectedTitle, expectedBody, []string{})
	if err != nil {
		t.Fatalf("UpdateCard returned error: %v", err)
	}

	if len(card.Labels) != 0 {
		t.Errorf("card.Labels has %d entries, want 0", len(card.Labels))
	}
}

func TestGitHubUpdateCard_APIError(t *testing.T) {
	apiErrMsg := "API request failed"
	client := &mockGitHubClient{
		editErr: errors.New(apiErrMsg),
	}
	columns := []string{"New"}
	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	_, err := provider.UpdateCard(context.Background(), 1, "title", "body", []string{"bug"})
	if err == nil {
		t.Fatal("expected error from UpdateCard, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}
}

// --- UpdateCard Validation Tests ---

func TestGitHubUpdateCard_EmptyTitle(t *testing.T) {
	client := &mockGitHubClient{}
	columns := []string{"New"}
	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	// Empty string title should return an error.
	_, err := provider.UpdateCard(context.Background(), 1, "", "body", []string{"bug"})
	if err == nil {
		t.Fatal("expected error for empty title, got nil")
	}
	if !strings.Contains(err.Error(), "title") {
		t.Errorf("error = %q, want it to mention title", err.Error())
	}

	// Whitespace-only title should also return an error.
	_, err = provider.UpdateCard(context.Background(), 1, "   ", "body", []string{"bug"})
	if err == nil {
		t.Fatal("expected error for whitespace-only title, got nil")
	}
}

func TestGitHubUpdateCard_ValidationError(t *testing.T) {
	client := &mockGitHubClient{
		editErr: &github.ErrorResponse{
			Response: &http.Response{StatusCode: 422},
			Message:  "Validation Failed",
		},
	}
	columns := []string{"New"}
	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	_, err := provider.UpdateCard(context.Background(), 1, "title", "body", []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error from UpdateCard with 422 response, got nil")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "validation failed") {
		t.Errorf("error = %q, want it to contain %q", errMsg, "validation failed")
	}
	if !strings.Contains(errMsg, "label") {
		t.Errorf("error = %q, want it to mention labels", errMsg)
	}
}

// --- FetchBoard/CreateCard Assignee Tests (individual-card mutations) ---

func TestGitHubCreateCard_AssigneesPopulated(t *testing.T) {
	assigneeLogin := "charlie"
	createdIssue := &github.Issue{
		Number: github.Ptr(42),
		Title:  github.Ptr("New feature"),
		Labels: []*github.Label{},
		Assignees: []*github.User{
			{Login: github.Ptr(assigneeLogin)},
		},
	}
	client := &mockGitHubClient{createdIssue: createdIssue}
	columns := []string{"New"}

	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	card, err := provider.CreateCard(context.Background(), "New feature", "")
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	if len(card.Assignees) != 1 {
		t.Fatalf("card.Assignees has %d entries, want 1", len(card.Assignees))
	}
	if card.Assignees[0].Login != assigneeLogin {
		t.Errorf("card.Assignees[0].Login = %q, want %q", card.Assignees[0].Login, assigneeLogin)
	}
}

func TestGitHubUpdateCard_AssigneesPreserved(t *testing.T) {
	assigneeLogin := "dana"
	editedIssue := &github.Issue{
		Number: github.Ptr(10),
		Title:  github.Ptr("Updated title"),
		Body:   github.Ptr("Updated body"),
		Labels: []*github.Label{},
		Assignees: []*github.User{
			{Login: github.Ptr(assigneeLogin)},
		},
	}
	client := &mockGitHubClient{editedIssue: editedIssue}
	columns := []string{"New"}
	provider := NewGitHubProvider(client, nil, "owner", "repo", columns)

	card, err := provider.UpdateCard(context.Background(), 10, "Updated title", "Updated body", []string{})
	if err != nil {
		t.Fatalf("UpdateCard returned error: %v", err)
	}

	if len(card.Assignees) != 1 {
		t.Fatalf("card.Assignees has %d entries, want 1", len(card.Assignees))
	}
	if card.Assignees[0].Login != assigneeLogin {
		t.Errorf("card.Assignees[0].Login = %q, want %q", card.Assignees[0].Login, assigneeLogin)
	}
}

// --- SetAssignees Tests ---

func TestGitHubSetAssignees_Success(t *testing.T) {
	assigneeLogin1 := "alice"
	assigneeLogin2 := "bob"
	issueNumber := 42

	client := &mockGitHubClient{
		editedIssue: &github.Issue{
			Number: github.Ptr(issueNumber),
			Title:  github.Ptr("Test issue"),
			Labels: []*github.Label{{Name: github.Ptr("bug")}},
			Assignees: []*github.User{
				{Login: github.Ptr(assigneeLogin1)},
				{Login: github.Ptr(assigneeLogin2)},
			},
		},
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	logins := []string{assigneeLogin1, assigneeLogin2}
	card, err := p.SetAssignees(context.Background(), issueNumber, logins)
	if err != nil {
		t.Fatalf("SetAssignees returned error: %v", err)
	}

	// Verify the returned card has the correct assignees.
	if len(card.Assignees) != 2 {
		t.Fatalf("card.Assignees has %d entries, want 2", len(card.Assignees))
	}
	if card.Assignees[0].Login != assigneeLogin1 {
		t.Errorf("card.Assignees[0].Login = %q, want %q", card.Assignees[0].Login, assigneeLogin1)
	}
	if card.Assignees[1].Login != assigneeLogin2 {
		t.Errorf("card.Assignees[1].Login = %q, want %q", card.Assignees[1].Login, assigneeLogin2)
	}

	// Verify the Edit call used Assignees field (atomic replace).
	if client.capturedEditReq == nil {
		t.Fatal("Edit was not called")
	}
	if client.capturedEditReq.Assignees == nil {
		t.Fatal("Edit request Assignees should be non-nil")
	}
	if len(*client.capturedEditReq.Assignees) != 2 {
		t.Errorf("Edit request Assignees has %d entries, want 2", len(*client.capturedEditReq.Assignees))
	}
}

func TestGitHubSetAssignees_APIError(t *testing.T) {
	apiErrMsg := "API request failed"
	client := &mockGitHubClient{
		editErr: errors.New(apiErrMsg),
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	_, err := p.SetAssignees(context.Background(), 1, []string{"alice"})
	if err == nil {
		t.Fatal("expected error from SetAssignees, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}
}

func TestGitHubSetAssignees_EmptyLogins(t *testing.T) {
	issueNumber := 10

	client := &mockGitHubClient{
		editedIssue: &github.Issue{
			Number:    github.Ptr(issueNumber),
			Title:     github.Ptr("Test issue"),
			Labels:    []*github.Label{},
			Assignees: []*github.User{}, // cleared
		},
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	card, err := p.SetAssignees(context.Background(), issueNumber, []string{})
	if err != nil {
		t.Fatalf("SetAssignees with empty logins returned error: %v", err)
	}

	// Setting empty logins should clear assignees.
	if len(card.Assignees) != 0 {
		t.Errorf("card.Assignees has %d entries after clearing, want 0", len(card.Assignees))
	}

	// Verify the Edit call sent an empty Assignees list (atomic replace to clear).
	if client.capturedEditReq == nil {
		t.Fatal("Edit was not called")
	}
	if client.capturedEditReq.Assignees == nil {
		t.Fatal("Edit request Assignees should be non-nil (explicit empty list)")
	}
	if len(*client.capturedEditReq.Assignees) != 0 {
		t.Errorf("Edit request Assignees has %d entries, want 0", len(*client.capturedEditReq.Assignees))
	}
}

// --- CloseCard Tests ---

func TestGitHubCloseCard_Success(t *testing.T) {
	issueNumber := 42

	client := &mockGitHubClient{
		editedIssue: &github.Issue{
			Number: github.Ptr(issueNumber),
			Title:  github.Ptr("Test issue"),
			State:  github.Ptr("closed"),
			Labels: []*github.Label{{Name: github.Ptr("bug")}},
		},
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	card, err := p.CloseCard(context.Background(), issueNumber)
	if err != nil {
		t.Fatalf("CloseCard returned error: %v", err)
	}

	if card.Number != issueNumber {
		t.Errorf("card.Number = %d, want %d", card.Number, issueNumber)
	}

	// The Edit request must set the issue state to "closed".
	if client.capturedEditReq == nil {
		t.Fatal("Edit was not called")
	}
	if client.capturedEditReq.State == nil {
		t.Fatal("Edit request State should be non-nil")
	}
	if *client.capturedEditReq.State != "closed" {
		t.Errorf("Edit request State = %q, want %q", *client.capturedEditReq.State, "closed")
	}
}

func TestGitHubCloseCard_APIError(t *testing.T) {
	apiErrMsg := "API request failed"
	client := &mockGitHubClient{
		editErr: errors.New(apiErrMsg),
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	_, err := p.CloseCard(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from CloseCard, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}
}

// --- AddComment Tests ---

func TestGitHubAddComment_Success(t *testing.T) {
	issueNumber := 42
	commentBody := "This has been resolved, closing now."

	client := &mockGitHubClient{
		createdComment: &github.IssueComment{Body: github.Ptr(commentBody)},
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	err := p.AddComment(context.Background(), issueNumber, commentBody)
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}

	if client.capturedCommentNumber != issueNumber {
		t.Errorf("CreateComment was called with issue number %d, want %d", client.capturedCommentNumber, issueNumber)
	}
	if client.capturedComment == nil || client.capturedComment.GetBody() != commentBody {
		t.Errorf("CreateComment was called with body %v, want %q", client.capturedComment, commentBody)
	}
}

func TestGitHubAddComment_IssueNotFound_MapsToCleanError(t *testing.T) {
	// Simulate GitHub's 404 response, including a raw API message that must
	// NOT leak into the sanitized, user-facing error string.
	rawAPIMessage := "Not Found"
	client := &mockGitHubClient{
		createCommentErr: &github.ErrorResponse{
			Response: &http.Response{StatusCode: http.StatusNotFound},
			Message:  rawAPIMessage,
		},
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	err := p.AddComment(context.Background(), 9999, "test comment")
	if err == nil {
		t.Fatal("expected error from AddComment for a non-existent issue, got nil")
	}

	clean := SanitizeError(err)
	if !strings.Contains(clean, "not found") {
		t.Errorf("SanitizeError(err) = %q, want it to mention %q", clean, "not found")
	}
	if strings.Contains(clean, rawAPIMessage) {
		t.Errorf("SanitizeError(err) = %q, must not leak the raw API message %q", clean, rawAPIMessage)
	}
}

func TestGitHubAddComment_GenericAPIError_PassesThrough(t *testing.T) {
	apiErrMsg := "connection reset by peer"
	client := &mockGitHubClient{
		createCommentErr: errors.New(apiErrMsg),
	}
	columns := []string{"New"}
	p := NewGitHubProvider(client, nil, "owner", "repo", columns)

	err := p.AddComment(context.Background(), 1, "test comment")
	if err == nil {
		t.Fatal("expected error from AddComment, got nil")
	}
	if !strings.Contains(err.Error(), apiErrMsg) {
		t.Errorf("error = %q, want it to contain %q", err.Error(), apiErrMsg)
	}

	// The sanitized message must be clean of stack traces / internal details --
	// for a plain error it is simply the error string itself, never more.
	clean := SanitizeError(err)
	if clean != apiErrMsg {
		t.Errorf("SanitizeError(err) = %q, want %q", clean, apiErrMsg)
	}
}

// --- DeleteCard Tests ---

func TestGitHubDeleteCard_Success(t *testing.T) {
	issueNumber := 42
	gql := &fakeGraphQLClient{}
	p := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", []string{"New"})

	if err := p.DeleteCard(context.Background(), issueNumber); err != nil {
		t.Fatalf("DeleteCard returned error: %v", err)
	}
}

// TestGitHubDeleteCard_NotFound scripts the lookup-query's real not-found
// wording ("Could not resolve to an Issue") -- the only not-found signal
// shurcooL/graphql exposes -- and asserts DeleteCard maps it to a clean,
// user-safe message that names the issue number, distinct from the generic
// permission-denied fallback.
func TestGitHubDeleteCard_NotFound(t *testing.T) {
	issueNumber := 404
	gql := &fakeGraphQLClient{deleteIssueErr: errors.New("Could not resolve to an Issue with the number of 404.")}
	p := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", []string{"New"})

	err := p.DeleteCard(context.Background(), issueNumber)
	if err == nil {
		t.Fatal("expected error for non-existent issue number, got nil")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(issueNumber)) {
		t.Errorf("error = %q, want it to mention issue number %d", err.Error(), issueNumber)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not found") {
		t.Errorf("error = %q, want a clean not-found message", err.Error())
	}
}

// TestGitHubDeleteCard_GenericErrorMapping scripts an arbitrary, opaque
// GraphQL error (a unique nonsense token unrelated to any real GitHub
// wording) and asserts DeleteCard maps ANY non-not-found mutation error to a
// single generic, non-leaking message: it must name the issue number and
// mention permission, but must NOT leak the raw scripted error text. This
// proves the mapping is generic (doesn't depend on matching GitHub's exact
// wording) without pinning any GitHub wire-format string.
func TestGitHubDeleteCard_GenericErrorMapping(t *testing.T) {
	issueNumber := 77
	nonsenseToken := "qzx-9f31-opaque-wire-blob-zzyx"
	gql := &fakeGraphQLClient{deleteIssueErr: errors.New(nonsenseToken)}
	p := NewGitHubProvider(emptyRESTClient(), gql, "owner", "repo", []string{"New"})

	err := p.DeleteCard(context.Background(), issueNumber)
	if err == nil {
		t.Fatal("expected error for scripted generic GraphQL failure, got nil")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(issueNumber)) {
		t.Errorf("error = %q, want it to mention issue number %d", err.Error(), issueNumber)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "permission") {
		t.Errorf("error = %q, want it to mention permission", err.Error())
	}
	if strings.Contains(err.Error(), nonsenseToken) {
		t.Errorf("error = %q, must not leak the raw scripted GraphQL error text %q", err.Error(), nonsenseToken)
	}
}
