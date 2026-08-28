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

// --- Default sort order on load (#412, default flipped in #503) ---

func TestSortColumns_DefaultOldestFirstOnLoad(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
		{Number: 3, Title: "Middle", CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	assertCardOrder(t, b.Columns[0].Cards, []int{1, 3, 2})
}

func TestSortColumns_DefaultsSortNewestFirstFalse(t *testing.T) {
	b := newTestBoard(t)

	if b.sortNewestFirst {
		t.Error("NewBoard() should default sortNewestFirst = false (oldest-created-first is the default order, #503)")
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
	assertCardOrder(t, b.Columns[0].Cards, []int{1, 2})

	b.sortNewestFirst = true
	b.sortColumns()

	assertCardOrder(t, b.Columns[0].Cards, []int{2, 1})
}

// --- 's' toggle (#412) ---
//
// The 's' key itself stays a built-in normal-mode command with a persistent
// sort-order effect, but its hint is intentionally omitted from the status
// bar (#443) to reduce bottom-bar clutter; it remains documented in the '?'
// help modal (generated from the registry -- see keymap_help.go).

func TestNormalMode_S_HintHiddenFromStatusBar(t *testing.T) {
	b := newLoadedTestBoard(t)

	if idx := hintIndex(b.normalHints, "s"); idx != -1 {
		t.Errorf("normalHints should not contain a %q hint (#443), got: %+v", "s", b.normalHints)
	}
}

func TestNormalMode_S_TogglesSortOrder_FlipsOrder(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
		{Number: 3, Title: "Middle", CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	assertCardOrder(t, b.Columns[0].Cards, []int{1, 3, 2}) // precondition: oldest-first

	m, cmd := b.Update(keyMsg("s"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("'s' toggle should return a nil cmd when no state path is configured (the re-sort is synchronous; only persistence is async, #503)")
	}

	assertCardOrder(t, updated.Columns[0].Cards, []int{2, 3, 1}) // newest-first

	if idx := hintIndex(updated.normalHints, "s"); idx != -1 {
		t.Errorf("normalHints should not contain a %q hint after toggle (#443), got: %+v", "s", updated.normalHints)
	}
}

func TestNormalMode_S_TogglingTwiceRestoresOldestFirst(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b = sendKey(t, b, keyMsg("s"))
	b = sendKey(t, b, keyMsg("s"))

	assertCardOrder(t, b.Columns[0].Cards, []int{1, 2})
	if idx := hintIndex(b.normalHints, "s"); idx != -1 {
		t.Errorf("after toggling twice, normalHints should still not contain a %q hint (#443), got: %+v", "s", b.normalHints)
	}
}

func TestNormalMode_S_PreservesCursorIdentity_Unfiltered(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
		{Number: 3, Title: "Middle", CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	// Default order: [1, 3, 2]. Move cursor to card #2 (last row).
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[0].Cards[b.Columns[0].Cursor].Number != 2 {
		t.Fatalf("precondition: cursor card = %d, want 2", b.Columns[0].Cards[b.Columns[0].Cursor].Number)
	}

	m, cmd := b.Update(keyMsg("s"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("'s' toggle should return a nil cmd when no state path is configured (the re-sort is synchronous; only persistence is async, #503)")
	}

	col := updated.Columns[updated.ActiveTab]
	if col.Cards[col.Cursor].Number != 2 {
		t.Errorf("cursor card = %d after 's' toggle, want 2 (cursor should follow the same card by identity)", col.Cards[col.Cursor].Number)
	}
}

func TestNormalMode_S_PreservesCursorIdentity_Filtered(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Bug old", Labels: []provider.Label{{Name: "bug"}}, CreatedAt: sortTestOlder},
		{Number: 2, Title: "Feature newest", Labels: []provider.Label{{Name: "feature"}}, CreatedAt: sortTestNewest},
		{Number: 3, Title: "Bug new", Labels: []provider.Label{{Name: "bug"}}, CreatedAt: sortTestNewer},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"
	// Filtered, oldest-first: [#1, #3]. Move cursor to filtered index 1 (card #3).
	b = sendKey(t, b, keyMsg("j"))
	visible := b.visibleCards()
	if len(visible) != 2 || visible[b.Columns[0].Cursor].Number != 3 {
		t.Fatalf("precondition: filtered visible cards = %+v, cursor = %d, want cursor on card #3", visible, b.Columns[0].Cursor)
	}

	m, cmd := b.Update(keyMsg("s"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("'s' toggle should return a nil cmd when no state path is configured (the re-sort is synchronous; only persistence is async, #503)")
	}

	col := updated.Columns[updated.ActiveTab]
	newVisible := updated.visibleCards()
	if col.Cursor >= len(newVisible) || newVisible[col.Cursor].Number != 3 {
		t.Errorf("filtered visible cards after 's' toggle = %+v, cursor = %d, want cursor on card #3 (identity preserved under active filter)", newVisible, col.Cursor)
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
	// Default order: [1, 3, 2]. Move cursor to card #2.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[0].Cards[b.Columns[0].Cursor].Number != 2 {
		t.Fatalf("precondition: cursor card = %d, want 2", b.Columns[0].Cards[b.Columns[0].Cursor].Number)
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

	assertCardOrder(t, b.Columns[0].Cards, []int{1, 3, 2})
	col := b.Columns[b.ActiveTab]
	if col.Cards[col.Cursor].Number != 2 {
		t.Errorf("cursor card = %d after refresh with sort active, want 2 (identity preserved through re-sort)", col.Cards[col.Cursor].Number)
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

// --- New card placement (created cards land at their sorted position) ---
//
// handleCardCreated appends the created card and re-sorts the column, so the
// new card lands where the active sort order says it belongs rather than
// always at the tail. The cursor is then resolved by Number, not by index,
// because the sort has just moved everything around.

// newCardPlacementCards is the shared three-card fixture for the placement
// tests: oldest (#1), middle (#3), newest (#2).
func newCardPlacementCards() []provider.Card {
	return []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
		{Number: 3, Title: "Middle", CreatedAt: sortTestNewer},
	}
}

// createCard drives a cardCreatedMsg through Update and returns the resulting
// Board, capturing both return values per .claude/rules/testing.md.
func createCard(t *testing.T, b Board, card provider.Card) Board {
	t.Helper()
	m, cmd := b.Update(cardCreatedMsg{card: card})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cardCreatedMsg with no pending assignee should return a nil cmd, got %T", cmd)
	}
	return updated
}

func TestCardCreated_NewestFirst_PlacesNewCardAtTop(t *testing.T) {
	b := newBoardWithInlineCards(t, newCardPlacementCards(), 120, 40)
	b.sortNewestFirst = true
	b.sortColumns()
	assertCardOrder(t, b.Columns[0].Cards, []int{2, 3, 1}) // precondition

	updated := createCard(t, b, provider.Card{Number: 99, Title: "Brand new", CreatedAt: time.Now()})

	assertCardOrder(t, updated.Columns[0].Cards, []int{99, 2, 3, 1})
	col := updated.Columns[0]
	if col.Cursor != 0 {
		t.Errorf("Cursor = %d after creating a card under newest-first, want 0 (the new card sits at the top)", col.Cursor)
	}
}

func TestCardCreated_OldestFirst_PlacesNewCardAtBottom(t *testing.T) {
	b := newBoardWithInlineCards(t, newCardPlacementCards(), 120, 40)
	assertCardOrder(t, b.Columns[0].Cards, []int{1, 3, 2}) // precondition: default oldest-first

	updated := createCard(t, b, provider.Card{Number: 99, Title: "Brand new", CreatedAt: time.Now()})

	assertCardOrder(t, updated.Columns[0].Cards, []int{1, 3, 2, 99})
	col := updated.Columns[0]
	if col.Cursor != len(col.Cards)-1 {
		t.Errorf("Cursor = %d after creating a card under oldest-first, want %d (the new card sits at the bottom)", col.Cursor, len(col.Cards)-1)
	}
}

// TestCardCreated_ZeroCreatedAt_LandsAtNewestEnd covers the fail-safe for
// providers that omit the creation timestamp (FakeProvider.CreateCard does):
// the card was just created, so it must sort to the newest end in either
// direction, never the oldest. Without the fallback a zero timestamp would
// silently invert the placement.
func TestCardCreated_ZeroCreatedAt_LandsAtNewestEnd(t *testing.T) {
	tests := []struct {
		name            string
		sortNewestFirst bool
		want            []int
	}{
		{name: "newest first", sortNewestFirst: true, want: []int{99, 2, 3, 1}},
		{name: "oldest first", sortNewestFirst: false, want: []int{1, 3, 2, 99}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newBoardWithInlineCards(t, newCardPlacementCards(), 120, 40)
			b.sortNewestFirst = tt.sortNewestFirst
			b.sortColumns()

			// No CreatedAt: the FakeProvider.CreateCard shape.
			updated := createCard(t, b, provider.Card{Number: 99, Title: "No timestamp"})

			assertCardOrder(t, updated.Columns[0].Cards, tt.want)
			col := updated.Columns[0]
			if col.Cards[col.Cursor].Number != 99 {
				t.Errorf("cursor card = %d, want 99 (the newly created card)", col.Cards[col.Cursor].Number)
			}
		})
	}
}

// TestCardCreated_RealCreatedAt_SortsIntoTheMiddle proves the fallback only
// fires for a zero timestamp: a provider-supplied CreatedAt is honored as-is
// and is not overwritten with time.Now().
func TestCardCreated_RealCreatedAt_SortsIntoTheMiddle(t *testing.T) {
	b := newBoardWithInlineCards(t, newCardPlacementCards(), 120, 40)
	assertCardOrder(t, b.Columns[0].Cards, []int{1, 3, 2}) // precondition: oldest-first

	// Between sortTestOlder (#1) and sortTestNewer (#3).
	between := sortTestOlder.Add(sortTestNewer.Sub(sortTestOlder) / 2)
	updated := createCard(t, b, provider.Card{Number: 99, Title: "Backdated", CreatedAt: between})

	assertCardOrder(t, updated.Columns[0].Cards, []int{1, 99, 3, 2})
	col := updated.Columns[0]
	if col.Cards[col.Cursor].Number != 99 {
		t.Errorf("cursor card = %d, want 99 (cursor follows the new card to its sorted position)", col.Cards[col.Cursor].Number)
	}
}

// TestCardCreated_CursorResolvesByNumberNotIndex pins the cursor-restore
// contract: after the re-sort the appended index is stale, so the cursor is
// resolved by the created card's Number.
func TestCardCreated_CursorResolvesByNumberNotIndex(t *testing.T) {
	b := newBoardWithInlineCards(t, newCardPlacementCards(), 120, 40)
	b.sortNewestFirst = true
	b.sortColumns()

	created := provider.Card{Number: 99, Title: "Brand new", CreatedAt: time.Now()}
	updated := createCard(t, b, created)

	col := updated.Columns[0]
	if col.Cursor < 0 || col.Cursor >= len(col.Cards) {
		t.Fatalf("Cursor = %d out of bounds for %d cards", col.Cursor, len(col.Cards))
	}
	if col.Cards[col.Cursor].Number != created.Number {
		t.Errorf("cursor card = %d, want %d (cursor must track the new card by Number, not by the pre-sort append index)", col.Cards[col.Cursor].Number, created.Number)
	}
	if updated.selectedCard().Number != created.Number {
		t.Errorf("selectedCard().Number = %d, want %d", updated.selectedCard().Number, created.Number)
	}
}

// TestCardCreated_NewestFirst_LongListKeepsNewCardOnScreen is the newest-first
// counterpart of create_mode_test.go's AC4 scroll test: at the top of a long
// list the scroll offset must go back to 0 so the new card is on screen.
func TestCardCreated_NewestFirst_LongListKeepsNewCardOnScreen(t *testing.T) {
	cardCount := 30
	b := newBoardWithGeneratedCards(t, cardCount, "Card %d", 120, 15)
	b.sortNewestFirst = true
	b.sortColumns()
	// Scroll away from the top so a reset to 0 is observable.
	b.Columns[0].Cursor = cardCount - 1
	b.clampScrollOffset()
	if b.Columns[0].ScrollOffset == 0 {
		t.Fatalf("precondition: ScrollOffset = 0, want > 0 after moving the cursor to the end of a long list")
	}

	created := provider.Card{Number: cardCount + 1, Title: "Top-of-list task", CreatedAt: time.Now()}
	updated := createCard(t, b, created)

	col := updated.Columns[0]
	if col.Cards[0].Number != created.Number {
		t.Errorf("first card = %d, want %d (newest-first puts the new card at the top)", col.Cards[0].Number, created.Number)
	}
	if col.Cursor != 0 {
		t.Errorf("Cursor = %d, want 0", col.Cursor)
	}
	if col.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 (the new card sits at the top and must be on screen)", col.ScrollOffset)
	}
}
