package provider

import (
	"context"
	"errors"
	"testing"
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
