package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestListMilestones_PagesThroughAllPages asserts ListMilestones follows the
// milestones connection's cursors to the end, preserving the connection's
// page order verbatim. Unlike ListOpenPRs, there is deliberately NO
// cross-page dedup here (milestones have no closing-PR duplication problem
// and Milestone carries no numeric key to dedup on) -- a title repeated
// across the page boundary must appear twice, pinning "no dedup" as
// behavior rather than an accidental omission.
func TestListMilestones_PagesThroughAllPages(t *testing.T) {
	repeated := Milestone{Title: "v1.0", URL: "https://github.com/o/r/milestone/2"}
	gql := &fakeGraphQLClient{
		milestonePages: map[string]milestonePage{
			"": {
				milestones: []Milestone{
					{Title: "v0.9", URL: "https://github.com/o/r/milestone/1"},
					repeated,
				},
				hasNextPage: true,
				endCursor:   "ms-cursor-A",
			},
			"ms-cursor-A": {
				milestones: []Milestone{
					repeated,
					{Title: "Backlog", URL: "https://github.com/o/r/milestone/5"},
				},
			},
		},
	}
	p := NewGitHubProvider(nil, gql, "o", "r", []string{"New"})

	got, err := p.ListMilestones(context.Background())
	if err != nil {
		t.Fatalf("ListMilestones() unexpected error: %v", err)
	}

	wantTitles := []string{"v0.9", "v1.0", "v1.0", "Backlog"}
	if len(got) != len(wantTitles) {
		t.Fatalf("ListMilestones() returned %d milestones, want %d (no cross-page dedup): %+v", len(got), len(wantTitles), got)
	}
	for i, want := range wantTitles {
		if got[i].Title != want {
			t.Errorf("ListMilestones()[%d].Title = %q, want %q", i, got[i].Title, want)
		}
	}

	wantCursors := []string{"", "ms-cursor-A"}
	if len(gql.calledMilestoneCursors) != len(wantCursors) {
		t.Fatalf("calledMilestoneCursors = %v, want %v", gql.calledMilestoneCursors, wantCursors)
	}
	for i, want := range wantCursors {
		if gql.calledMilestoneCursors[i] != want {
			t.Fatalf("calledMilestoneCursors = %v, want %v", gql.calledMilestoneCursors, wantCursors)
		}
	}
}

// TestListMilestones_ReturnsErrorFromGQL asserts a first-page GraphQL
// failure propagates to the caller instead of silently returning a
// partial/empty list.
func TestListMilestones_ReturnsErrorFromGQL(t *testing.T) {
	wantErr := errors.New("graphql: rate limited")
	gql := &fakeGraphQLClient{err: wantErr}
	p := NewGitHubProvider(nil, gql, "o", "r", []string{"New"})

	_, err := p.ListMilestones(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ListMilestones() error = %v, want %v", err, wantErr)
	}
}

// milestoneSecondPageErrorFakeClient wraps a fakeGraphQLClient to succeed on
// the first fetchMilestonePage call (cursor "", using the embedded fake's
// scripted first page) but return a distinct, scripted error on every
// subsequent call. fakeGraphQLClient's single shared err field can't
// express "first page succeeds, second page fails" (setting it fails both
// calls), so this wrapper isolates the second-page-only failure, mirroring
// closingPRErrorFakeClient (github_fetchboard_test.go:1130).
type milestoneSecondPageErrorFakeClient struct {
	*fakeGraphQLClient
	secondPageErr error
}

func (f *milestoneSecondPageErrorFakeClient) fetchMilestonePage(ctx context.Context, owner, repo, afterCursor string) (milestonePage, error) {
	if afterCursor == "" {
		return f.fakeGraphQLClient.fetchMilestonePage(ctx, owner, repo, afterCursor)
	}
	f.calledMilestoneCursors = append(f.calledMilestoneCursors, afterCursor)
	return milestonePage{}, f.secondPageErr
}

// TestListMilestones_SecondPageErrorAbortsListing asserts that when the
// first page succeeds but a later page errors, ListMilestones returns the
// error and a nil result -- never a silently-partial list.
func TestListMilestones_SecondPageErrorAbortsListing(t *testing.T) {
	wantErr := errors.New("graphql: second page failed")
	gql := &milestoneSecondPageErrorFakeClient{
		fakeGraphQLClient: &fakeGraphQLClient{
			milestonePages: map[string]milestonePage{
				"": {
					milestones:  []Milestone{{Title: "v0.9", URL: "https://github.com/o/r/milestone/1"}},
					hasNextPage: true,
					endCursor:   "ms-cursor-A",
				},
			},
		},
		secondPageErr: wantErr,
	}
	p := NewGitHubProvider(nil, gql, "o", "r", []string{"New"})

	got, err := p.ListMilestones(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("ListMilestones() error = %v, want %v", err, wantErr)
	}
	if got != nil {
		t.Fatalf("ListMilestones() = %+v, want nil result when a later page errors (no silently-partial list)", got)
	}
}

// TestListMilestones_CarriesFieldsVerbatim asserts a page containing a
// nil-DueOn milestone and a 0-open/0-closed milestone survives the paging
// loop with every field untouched -- no derivation, no divide-by-zero.
func TestListMilestones_CarriesFieldsVerbatim(t *testing.T) {
	due := time.Date(2024, 12, 31, 0, 0, 0, 0, time.UTC)
	withDue := Milestone{
		Title:              "Icebox",
		URL:                "https://github.com/o/r/milestone/4",
		DueOn:              &due,
		OpenIssueCount:     0,
		ClosedIssueCount:   0,
		ProgressPercentage: 0,
	}
	noDue := Milestone{
		Title:              "Backlog",
		URL:                "https://github.com/o/r/milestone/5",
		DueOn:              nil,
		OpenIssueCount:     5,
		ClosedIssueCount:   0,
		ProgressPercentage: 0,
	}
	gql := &fakeGraphQLClient{
		milestonePages: map[string]milestonePage{
			"": {milestones: []Milestone{withDue, noDue}},
		},
	}
	p := NewGitHubProvider(nil, gql, "o", "r", []string{"New"})

	got, err := p.ListMilestones(context.Background())
	if err != nil {
		t.Fatalf("ListMilestones() unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListMilestones() returned %d milestones, want 2", len(got))
	}

	byTitle := make(map[string]Milestone, len(got))
	for _, m := range got {
		byTitle[m.Title] = m
	}

	icebox, ok := byTitle["Icebox"]
	if !ok {
		t.Fatalf("ListMilestones() missing %q", "Icebox")
	}
	if icebox.DueOn == nil || !icebox.DueOn.Equal(due) {
		t.Errorf("Icebox.DueOn = %v, want %v", icebox.DueOn, due)
	}
	if icebox.OpenIssueCount != 0 || icebox.ClosedIssueCount != 0 {
		t.Errorf("Icebox counts = open=%d closed=%d, want open=0 closed=0 (no derivation, no divide-by-zero)", icebox.OpenIssueCount, icebox.ClosedIssueCount)
	}

	backlog, ok := byTitle["Backlog"]
	if !ok {
		t.Fatalf("ListMilestones() missing %q", "Backlog")
	}
	if backlog.DueOn != nil {
		t.Errorf("Backlog.DueOn = %v, want nil", backlog.DueOn)
	}
	if backlog.OpenIssueCount != 5 {
		t.Errorf("Backlog.OpenIssueCount = %d, want 5", backlog.OpenIssueCount)
	}
	if backlog.URL != noDue.URL {
		t.Errorf("Backlog.URL = %q, want %q", backlog.URL, noDue.URL)
	}
}
