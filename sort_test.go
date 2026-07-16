package main

import (
	"testing"
	"time"

	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// Card creation timestamps shared across sort tests: older < newer < newest.
var (
	sortTestOlder  = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	sortTestNewer  = time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	sortTestNewest = time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)
)

// cardNumberOrder extracts the Number field of each card, in order, for
// concise "want this order" assertions.
func cardNumberOrder(cards []Card) []int {
	nums := make([]int, len(cards))
	for i, c := range cards {
		nums[i] = c.Number
	}
	return nums
}

// assertCardOrder fails the test if got's card numbers, in order, don't
// exactly match want.
func assertCardOrder(t *testing.T, got []Card, want []int) {
	t.Helper()
	gotNums := cardNumberOrder(got)
	if len(gotNums) != len(want) {
		t.Fatalf("card order = %v, want %v", gotNums, want)
	}
	for i, n := range want {
		if gotNums[i] != n {
			t.Fatalf("card order = %v, want %v", gotNums, want)
		}
	}
}

// --- Default sort order on load (#412) ---

func TestSortColumns_DefaultNewestFirstOnLoad(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
		{Number: 3, Title: "Middle", CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	assertCardOrder(t, b.Columns[0].Cards, []int{2, 3, 1})
}

func TestSortColumns_DefaultsSortNewestFirstTrue(t *testing.T) {
	b := newTestBoard(t)

	if !b.sortNewestFirst {
		t.Error("NewBoard() should default sortNewestFirst = true (newest-created-first is the default order, #412)")
	}
}

func TestSortColumns_StableTieBreak_PreservesProviderOrderForZeroTimestamps(t *testing.T) {
	// No card sets CreatedAt (zero value): a stable sort must preserve the
	// provider's original order among ties.
	cards := []provider.Card{
		{Number: 1, Title: "First"},
		{Number: 2, Title: "Second"},
		{Number: 3, Title: "Third"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	assertCardOrder(t, b.Columns[0].Cards, []int{1, 2, 3})
}

func TestSortColumns_StableTieBreak_PreservesProviderOrderForEqualTimestamps(t *testing.T) {
	same := sortTestNewer
	cards := []provider.Card{
		{Number: 5, Title: "First", CreatedAt: same},
		{Number: 6, Title: "Second", CreatedAt: same},
		{Number: 7, Title: "Third", CreatedAt: same},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	assertCardOrder(t, b.Columns[0].Cards, []int{5, 6, 7})
}

// --- Direct sortColumns unit test ---

func TestSortColumns_TogglingFieldFlipsOrder(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	assertCardOrder(t, b.Columns[0].Cards, []int{2, 1})

	b.sortNewestFirst = false
	b.sortColumns()

	assertCardOrder(t, b.Columns[0].Cards, []int{1, 2})
}

// --- 'u' toggle (#412) ---

func TestNormalMode_U_HintShowsNewestFirstByDefault(t *testing.T) {
	b := newLoadedTestBoard(t)

	idx := hintIndex(b.normalHints, "u")
	if idx == -1 {
		t.Fatalf("normalHints should contain a %q hint, got: %+v", "u", b.normalHints)
	}
	if b.normalHints[idx].Desc != "sort: newest" {
		t.Errorf("u hint Desc = %q, want %q", b.normalHints[idx].Desc, "sort: newest")
	}
}

func TestNormalMode_U_TogglesSortOrder_FlipsOrderAndHint(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
		{Number: 3, Title: "Middle", CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	assertCardOrder(t, b.Columns[0].Cards, []int{2, 3, 1}) // precondition: newest-first

	m, cmd := b.Update(keyMsg("u"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("'u' toggle should return a nil cmd (synchronous in-memory re-sort, no async work)")
	}

	assertCardOrder(t, updated.Columns[0].Cards, []int{1, 3, 2}) // oldest-first

	idx := hintIndex(updated.normalHints, "u")
	if idx == -1 {
		t.Fatalf("normalHints should contain a %q hint after toggle, got: %+v", "u", updated.normalHints)
	}
	if updated.normalHints[idx].Desc != "sort: oldest" {
		t.Errorf("u hint Desc after toggle = %q, want %q", updated.normalHints[idx].Desc, "sort: oldest")
	}
}

func TestNormalMode_U_TogglingTwiceRestoresNewestFirst(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b = sendKey(t, b, keyMsg("u"))
	b = sendKey(t, b, keyMsg("u"))

	assertCardOrder(t, b.Columns[0].Cards, []int{2, 1})
	idx := hintIndex(b.normalHints, "u")
	if idx == -1 || b.normalHints[idx].Desc != "sort: newest" {
		t.Errorf("after toggling twice, u hint = %+v, want Desc %q", b.normalHints, "sort: newest")
	}
}

func TestNormalMode_U_PreservesCursorIdentity_Unfiltered(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
		{Number: 3, Title: "Middle", CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	// Default order: [2, 3, 1]. Move cursor to card #1 (last row).
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[0].Cards[b.Columns[0].Cursor].Number != 1 {
		t.Fatalf("precondition: cursor card = %d, want 1", b.Columns[0].Cards[b.Columns[0].Cursor].Number)
	}

	m, cmd := b.Update(keyMsg("u"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("'u' toggle should return a nil cmd (synchronous in-memory re-sort, no async work)")
	}

	col := updated.Columns[updated.ActiveTab]
	if col.Cards[col.Cursor].Number != 1 {
		t.Errorf("cursor card = %d after 'u' toggle, want 1 (cursor should follow the same card by identity)", col.Cards[col.Cursor].Number)
	}
}

func TestNormalMode_U_PreservesCursorIdentity_Filtered(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Bug old", Labels: []provider.Label{{Name: "bug"}}, CreatedAt: sortTestOlder},
		{Number: 2, Title: "Feature newest", Labels: []provider.Label{{Name: "feature"}}, CreatedAt: sortTestNewest},
		{Number: 3, Title: "Bug new", Labels: []provider.Label{{Name: "bug"}}, CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"
	// Filtered, newest-first: [#3, #1]. Move cursor to filtered index 1 (card #1).
	b = sendKey(t, b, keyMsg("j"))
	visible := b.visibleCards()
	if len(visible) != 2 || visible[b.Columns[0].Cursor].Number != 1 {
		t.Fatalf("precondition: filtered visible cards = %+v, cursor = %d, want cursor on card #1", visible, b.Columns[0].Cursor)
	}

	m, cmd := b.Update(keyMsg("u"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("'u' toggle should return a nil cmd (synchronous in-memory re-sort, no async work)")
	}

	col := updated.Columns[updated.ActiveTab]
	newVisible := updated.visibleCards()
	if col.Cursor >= len(newVisible) || newVisible[col.Cursor].Number != 1 {
		t.Errorf("filtered visible cards after 'u' toggle = %+v, cursor = %d, want cursor on card #1 (identity preserved under active filter)", newVisible, col.Cursor)
	}
}

// --- Refresh interaction (#412) ---

func TestBackgroundRefresh_WithSort_PreservesCursorIdentity(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
		{Number: 3, Title: "Middle", CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	// Default order: [2, 3, 1]. Move cursor to card #1.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[0].Cards[b.Columns[0].Cursor].Number != 1 {
		t.Fatalf("precondition: cursor card = %d, want 1", b.Columns[0].Cards[b.Columns[0].Cursor].Number)
	}

	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Provider returns the same cards in raw (unsorted) order; sortColumns
	// must run again after the refresh for the assertions below to hold.
	fetchMsg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ = b.Update(fetchMsg)
	b = m.(Board)

	assertCardOrder(t, b.Columns[0].Cards, []int{2, 3, 1})
	col := b.Columns[b.ActiveTab]
	if col.Cards[col.Cursor].Number != 1 {
		t.Errorf("cursor card = %d after refresh with sort active, want 1 (identity preserved through re-sort)", col.Cards[col.Cursor].Number)
	}
}

func TestBackgroundRefresh_WithSort_FilteredResetsCursorToZero(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Bug old", Labels: []provider.Label{{Name: "bug"}}, CreatedAt: sortTestOlder},
		{Number: 2, Title: "Feature", Labels: []provider.Label{{Name: "feature"}}, CreatedAt: sortTestNewest},
		{Number: 3, Title: "Bug new", Labels: []provider.Label{{Name: "bug"}}, CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[0].Cursor != 1 {
		t.Fatalf("precondition: cursor = %d, want 1", b.Columns[0].Cursor)
	}

	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	fetchMsg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ = b.Update(fetchMsg)
	b = m.(Board)

	// Existing filtered-refresh reset behavior (docs/list-cursor-invariants.md
	// / resolved Q&A #1) must still hold when sorting is layered on top.
	if b.Columns[0].Cursor != 0 {
		t.Errorf("Cursor = %d after refresh with filter active, want 0 (existing filtered-refresh reset behavior preserved)", b.Columns[0].Cursor)
	}
}
