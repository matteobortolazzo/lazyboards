package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Compile-time check: *FakeProvider implements BoardProvider.
var _ BoardProvider = (*FakeProvider)(nil)

// FakeProvider is an in-memory BoardProvider for development and testing.
type FakeProvider struct {
	columns    []Column
	nextNumber int
	labels     []string

	// Comments records posted comments keyed by card number, in the order
	// they were added.
	Comments map[int][]string

	// Call counters for tests that need to assert which provider methods
	// were (or were not) invoked during a given operation, e.g. verifying
	// that metadata calls are skipped when gated behind a TTL.
	FetchBoardCalls           int
	FetchCollaboratorsCalls   int
	GetAuthenticatedUserCalls int
	ListLabelsCalls           int
	ListOpenPRsCalls          int
}

// fakeCreatedAtBase anchors the fixture cards' creation timestamps.
// fakeCreatedAt gives each fixture card a distinct value (one day apart,
// decreasing as the card number increases) so the board's newest-created-
// first default sort (#412) has an observable effect on the fake/dev-mode
// data while preserving the fixture's original by-number display order
// within each column (lower-numbered cards were "created" more recently,
// so they continue to sort first under the newest-first default).
var fakeCreatedAtBase = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// fakeCreatedAt returns a distinct creation timestamp for fixture card
// number n, decreasing with n.
func fakeCreatedAt(n int) time.Time {
	return fakeCreatedAtBase.AddDate(0, 0, -n)
}

// NewFakeProvider returns a FakeProvider pre-populated with hardcoded Kanban data.
func NewFakeProvider() *FakeProvider {
	return &FakeProvider{
		columns: []Column{
			{
				Title: "New",
				Cards: []Card{
					// Card #1 deliberately has no LinkedPRs: it's the default
					// selected card and several tests (e.g. newDeleteTestBoard)
					// rely on it being a valid target for PR-linked-gated flows.
					{Number: 1, Title: "Setup CI", Labels: []Label{{Name: "infra"}}, Body: "Configure GitHub Actions for CI pipeline.", Assignees: []Assignee{{Login: "alice"}}, CreatedAt: fakeCreatedAt(1)},
					{Number: 2, Title: "Data model", Labels: []Label{{Name: "design"}}, Body: "Design the core data model for boards and cards.", Assignees: []Assignee{{Login: "alice"}, {Login: "bob"}}, LinkedPRs: []LinkedPR{
						{Number: 20, Title: "feat: add data model", URL: "https://github.com/owner/repo/pull/20", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN", State: "OPEN"},
					}, CreatedAt: fakeCreatedAt(2)},
					// Card #3 links three PRs (one draft, one blocked, one merged)
					// to exercise each linked PR rendering its own status line
					// (#439) in dev-mode/manual verification. PR #32 is merged and
					// deliberately absent from ListOpenPRs below -- this
					// linked-but-not-open asymmetry reproduces the #449 bug where
					// the card-linked fallback list (shown while the repo-wide
					// open-PR fetch is in flight) must exclude closed/merged
					// entries rather than briefly flashing them.
					{Number: 3, Title: "Add README", Labels: []Label{{Name: "docs"}}, LinkedPRs: []LinkedPR{
						{Number: 30, Title: "docs: add README", URL: "https://github.com/owner/repo/pull/30", IsDraft: true, Mergeable: "MERGEABLE", MergeStateStatus: "DRAFT", State: "OPEN"},
						{Number: 31, Title: "docs: improve README", URL: "https://github.com/owner/repo/pull/31", Mergeable: "MERGEABLE", MergeStateStatus: "BLOCKED", State: "OPEN"},
						{Number: 32, Title: "docs: fix typo", URL: "https://github.com/owner/repo/pull/32", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN", State: "MERGED"},
					}, CreatedAt: fakeCreatedAt(3)},
				},
			},
			{
				Title: "Refined",
				Cards: []Card{
					// Cards #4 and #5 deliberately share the "v1.0" milestone, and
					// card #6 is deliberately left without one, so the milestone
					// filter dedup logic (#462) has both a shared value and an
					// empty case to exercise.
					{Number: 4, Title: "User auth", Labels: []Label{{Name: "feature"}}, Milestone: "v1.0", CreatedAt: fakeCreatedAt(4)},
					// Card #5 is a parent (has sub-issues, #460): SubIssueCount > 0,
					// no ParentNumber, exercising the parent-only sub-issue line.
					// SubIssueCompleted: 1 of its 2 sub-issues are closed (#475),
					// rendering "1/2".
					{Number: 5, Title: "API routes", Labels: []Label{{Name: "backend"}}, Milestone: "v1.0", CreatedAt: fakeCreatedAt(5), SubIssueCount: 2, SubIssueCompleted: 1},
					// Card #6 is a child of #5 (#460): ParentNumber > 0, no
					// SubIssueCount, exercising the child-only sub-issue line.
					{Number: 6, Title: "Error types", Labels: []Label{{Name: "backend"}}, CreatedAt: fakeCreatedAt(6), ParentNumber: 5},
					{Number: 7, Title: "DB migrate", Labels: []Label{{Name: "infra"}}, CreatedAt: fakeCreatedAt(7)},
				},
			},
			{
				Title: "Implementing",
				Cards: []Card{
					{Number: 8, Title: "Board view", Labels: []Label{{Name: "feature"}}, CreatedAt: fakeCreatedAt(8)},
					{Number: 9, Title: "Key binds", Labels: []Label{{Name: "feature"}}, CreatedAt: fakeCreatedAt(9)},
					{Number: 10, Title: "Col nav", Labels: []Label{{Name: "feature"}}, CreatedAt: fakeCreatedAt(10)},
					// Card #11 is both a parent and a child (#460), exercising the
					// combined-lines case, parent line before child line.
					// SubIssueCompleted: 1 of its 1 sub-issue is closed (#475),
					// rendering "1/1".
					{Number: 11, Title: "Lipgloss", Labels: []Label{{Name: "ui"}}, CreatedAt: fakeCreatedAt(11), SubIssueCount: 1, SubIssueCompleted: 1, ParentNumber: 8},
					{Number: 12, Title: "Config", Labels: []Label{{Name: "feature"}}, CreatedAt: fakeCreatedAt(12)},
				},
			},
		},
		nextNumber: 13,
		// Repo label set: every label attached to a card above, plus "planned"
		// which is intentionally not on any card (an existing repo label not
		// shown on the board).
		labels:   []string{"infra", "design", "docs", "feature", "backend", "ui", "planned"},
		Comments: make(map[int][]string),
	}
}

// FetchBoard returns a copy of the current board state.
func (f *FakeProvider) FetchBoard(_ context.Context) (Board, error) {
	f.FetchBoardCalls++
	cols := make([]Column, len(f.columns))
	for i, col := range f.columns {
		cards := make([]Card, len(col.Cards))
		copy(cards, col.Cards)
		cols[i] = Column{Title: col.Title, Cards: cards}
	}
	return Board{Columns: cols}, nil
}

// ListOpenPRs returns the fake repository's open pull requests: every PR
// linked to a card above, plus one (#40) no card links to, so the open-PR
// overview can be exercised against both linked and unlinked rows. PR #32
// (merged, linked to card #3) is deliberately NOT included here -- it only
// appears in card #3's LinkedPRs, mirroring how closedByPullRequestsReferences
// returns closing PRs regardless of state while this repo-wide query only
// returns OPEN ones (#449). Ordered newest-first to mirror the real
// provider's CREATED_AT DESC ordering. Status fields mirror each PR's
// card-linked counterpart so dev-mode/manual verification covers draft/
// mergeable/blocked plus one unresolved/unknown mergeability (#40, the
// unlinked PR). "Conflicting" is intentionally not represented here — it's
// already exercised in isolation by pr_status_test.go and the rendered-view
// tests in view_test.go/pr_list_test.go/pr_picker_test.go, and every card in
// this shared fixture is either a PR-gating test target (card #1, see
// NewFakeProvider) or already used above, so there is no free card to attach
// a 5th example to without risking collisions with other tests built on this
// fixture.
func (f *FakeProvider) ListOpenPRs(_ context.Context) ([]LinkedPR, error) {
	f.ListOpenPRsCalls++
	return []LinkedPR{
		{Number: 40, Title: "chore: unlinked cleanup", URL: "https://github.com/owner/repo/pull/40", Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN", State: "OPEN"},
		{Number: 31, Title: "docs: improve README", URL: "https://github.com/owner/repo/pull/31", Mergeable: "MERGEABLE", MergeStateStatus: "BLOCKED", State: "OPEN"},
		{Number: 30, Title: "docs: add README", URL: "https://github.com/owner/repo/pull/30", IsDraft: true, Mergeable: "MERGEABLE", MergeStateStatus: "DRAFT", State: "OPEN"},
		{Number: 20, Title: "feat: add data model", URL: "https://github.com/owner/repo/pull/20", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN", State: "OPEN"},
	}, nil
}

// CreateCard adds a new card to the first column with an auto-incremented number.
// Title must be non-empty after trimming whitespace. Empty label is allowed.
func (f *FakeProvider) CreateCard(_ context.Context, title string, label string) (Card, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Card{}, errors.New("title is required")
	}

	var labels []Label
	if label != "" {
		labels = []Label{{Name: label}}
	}
	card := Card{
		Number: f.nextNumber,
		Title:  title,
		Labels: labels,
	}
	f.columns[0].Cards = append(f.columns[0].Cards, card)
	f.nextNumber++

	return card, nil
}

// findCard searches f.columns for a card with the given number, returning
// its column and card indices. ok is false if no card with that number
// exists in any column.
func (f *FakeProvider) findCard(number int) (colIdx, cardIdx int, ok bool) {
	for ci := range f.columns {
		for i := range f.columns[ci].Cards {
			if f.columns[ci].Cards[i].Number == number {
				return ci, i, true
			}
		}
	}
	return 0, 0, false
}

// UpdateCard updates an existing card's title, body, and labels in memory.
// Title must be non-empty after trimming whitespace.
func (f *FakeProvider) UpdateCard(_ context.Context, number int, title string, body string, labels []string) (Card, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Card{}, errors.New("title is required")
	}

	ci, i, ok := f.findCard(number)
	if !ok {
		return Card{}, fmt.Errorf("card #%d not found", number)
	}
	f.columns[ci].Cards[i].Title = title
	f.columns[ci].Cards[i].Body = body
	cardLabels := make([]Label, len(labels))
	for j, name := range labels {
		cardLabels[j] = Label{Name: name}
	}
	f.columns[ci].Cards[i].Labels = cardLabels
	return f.columns[ci].Cards[i], nil
}

// CreateLabel is a no-op for the fake provider.
func (f *FakeProvider) CreateLabel(_ context.Context, _ string) error {
	return nil
}

// ListLabels returns a copy of the fake repository's label set.
func (f *FakeProvider) ListLabels(_ context.Context) ([]string, error) {
	f.ListLabelsCalls++
	labels := make([]string, len(f.labels))
	copy(labels, f.labels)
	return labels, nil
}

// FetchCollaborators returns a hardcoded list of collaborators for the fake provider.
func (f *FakeProvider) FetchCollaborators(_ context.Context) ([]Assignee, error) {
	f.FetchCollaboratorsCalls++
	return []Assignee{{Login: "alice"}, {Login: "bob"}, {Login: "charlie"}}, nil
}

// SetAssignees updates the assignees of a card in the fake provider.
func (f *FakeProvider) SetAssignees(_ context.Context, number int, logins []string) (Card, error) {
	ci, i, ok := f.findCard(number)
	if !ok {
		return Card{}, fmt.Errorf("card #%d not found", number)
	}
	assignees := make([]Assignee, len(logins))
	for j, login := range logins {
		assignees[j] = Assignee{Login: login}
	}
	f.columns[ci].Cards[i].Assignees = assignees
	return f.columns[ci].Cards[i], nil
}

// CloseCard finds a card by number and returns it. It does not remove or
// mutate the card in f.columns, so unlike the real GitHub provider, a
// subsequent FetchBoard() on the same fake instance still returns the
// "closed" card.
func (f *FakeProvider) CloseCard(_ context.Context, number int) (Card, error) {
	ci, i, ok := f.findCard(number)
	if !ok {
		return Card{}, fmt.Errorf("card #%d not found", number)
	}
	return f.columns[ci].Cards[i], nil
}

// AddComment records a comment against a card in the fake provider.
func (f *FakeProvider) AddComment(_ context.Context, number int, body string) error {
	if _, _, ok := f.findCard(number); !ok {
		return fmt.Errorf("card #%d not found", number)
	}
	f.Comments[number] = append(f.Comments[number], body)
	return nil
}

// GetAuthenticatedUser returns a hardcoded username for the fake provider.
func (f *FakeProvider) GetAuthenticatedUser(_ context.Context) (string, error) {
	f.GetAuthenticatedUserCalls++
	return "fake-user", nil
}

// DeleteCard finds a card by number and removes it from f.columns, unlike
// CloseCard, which only finds/returns without mutating. A subsequent
// FetchBoard() on the same fake instance no longer returns the deleted card.
func (f *FakeProvider) DeleteCard(_ context.Context, number int) error {
	ci, i, ok := f.findCard(number)
	if !ok {
		return fmt.Errorf("card #%d not found", number)
	}
	f.columns[ci].Cards = append(f.columns[ci].Cards[:i], f.columns[ci].Cards[i+1:]...)
	return nil
}
