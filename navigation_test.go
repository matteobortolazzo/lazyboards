package main

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Tab Navigation ---

func TestTabNavigation_Tab_MovesRight(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 1 {
		t.Errorf("after Tab: ActiveTab = %d, want 1", b.ActiveTab)
	}
}

func TestTabNavigation_ShiftTab_MovesLeft(t *testing.T) {
	b := newLoadedTestBoard(t)
	// Move right first so we can move left
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	if b.ActiveTab != 0 {
		t.Errorf("after Tab then Shift+Tab: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestNormalMode_LKey_DoesNotChangeTab(t *testing.T) {
	b := newLoadedTestBoard(t)
	tabBefore := b.ActiveTab
	b = sendKey(t, b, keyMsg("l"))
	if b.ActiveTab != tabBefore {
		t.Errorf("after 'l': ActiveTab = %d, want %d (l should not change tab)", b.ActiveTab, tabBefore)
	}
	if !b.detailFocused {
		t.Error("after 'l': detailFocused should be true")
	}
}

func TestNormalMode_RightArrow_FocusesDetail(t *testing.T) {
	b := newLoadedTestBoard(t)
	tabBefore := b.ActiveTab
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.ActiveTab != tabBefore {
		t.Errorf("after Right arrow: ActiveTab = %d, want %d (right should not change tab)", b.ActiveTab, tabBefore)
	}
	if !b.detailFocused {
		t.Error("after Right arrow: detailFocused should be true")
	}
}

func TestNormalMode_LeftArrow_DoesNotChangeTab(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // move to column 1
	tabBefore := b.ActiveTab
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.ActiveTab != tabBefore {
		t.Errorf("after Left arrow: ActiveTab = %d, want %d (left should not change tab)", b.ActiveTab, tabBefore)
	}
}

func TestTabNavigation_ShiftTab_WrapsToLastColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	lastCol := len(b.Columns) - 1
	// At column 0, pressing Shift+Tab should wrap to last column
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	if b.ActiveTab != lastCol {
		t.Errorf("Shift+Tab at column 0: ActiveTab = %d, want %d (should wrap to last)", b.ActiveTab, lastCol)
	}
}

func TestTabNavigation_Tab_WrapsToFirstColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	if len(b.Columns) < 2 {
		t.Fatal("board must have at least 2 columns for this test")
	}
	lastColumn := len(b.Columns) - 1
	// Move to the last column
	for i := 0; i < lastColumn; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyTab))
	}
	if b.ActiveTab != lastColumn {
		t.Fatalf("precondition: ActiveTab = %d, want %d (last column)", b.ActiveTab, lastColumn)
	}
	// One more Tab should wrap to first column
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 0 {
		t.Errorf("Tab past last column: ActiveTab = %d, want 0 (should wrap to first)", b.ActiveTab)
	}
}

func TestTabNavigation_FullTraversal(t *testing.T) {
	b := newLoadedTestBoard(t)
	if len(b.Columns) < 2 {
		t.Fatal("board must have at least 2 columns for this test")
	}
	lastColumn := len(b.Columns) - 1

	// Move all the way right
	for i := 0; i < lastColumn; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyTab))
	}
	if b.ActiveTab != lastColumn {
		t.Errorf("after traversing right: ActiveTab = %d, want %d", b.ActiveTab, lastColumn)
	}

	// Move all the way back left
	for i := 0; i < lastColumn; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	}
	if b.ActiveTab != 0 {
		t.Errorf("after traversing back left: ActiveTab = %d, want 0", b.ActiveTab)
	}

	// Tab past last should wrap to first
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 1 {
		t.Errorf("after Tab at column 0: ActiveTab = %d, want 1", b.ActiveTab)
	}
	// Navigate to last column and Tab again to wrap
	for i := 1; i < lastColumn; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyTab))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 0 {
		t.Errorf("Tab past last column: ActiveTab = %d, want 0 (should wrap)", b.ActiveTab)
	}

	// Shift+Tab past first should wrap to last
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	if b.ActiveTab != lastColumn {
		t.Errorf("Shift+Tab past first column: ActiveTab = %d, want %d (should wrap to last)", b.ActiveTab, lastColumn)
	}
}

func TestTabNavigation_Tab_SingleColumn_NoChange(t *testing.T) {
	b := newBoardWithBody(t, "body", "body2")
	if len(b.Columns) != 1 {
		t.Fatalf("precondition: expected 1 column, got %d", len(b.Columns))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 0 {
		t.Errorf("Tab on single-column board: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestTabNavigation_ShiftTab_SingleColumn_NoChange(t *testing.T) {
	b := newBoardWithBody(t, "body", "body2")
	if len(b.Columns) != 1 {
		t.Fatalf("precondition: expected 1 column, got %d", len(b.Columns))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	if b.ActiveTab != 0 {
		t.Errorf("Shift+Tab on single-column board: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

// --- Item Navigation ---

func TestItemNavigation_J_MovesCursorDown(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	b = sendKey(t, b, keyMsg("j"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 1 {
		t.Errorf("after 'j': cursor = %d, want 1", cursor)
	}
}

func TestItemNavigation_K_MovesCursorUp(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	// Move down first so we can move up
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("k"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("after 'j' then 'k': cursor = %d, want 0", cursor)
	}
}

func TestItemNavigation_DownArrow_MovesCursorDown(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 1 {
		t.Errorf("after Down arrow: cursor = %d, want 1", cursor)
	}
}

func TestItemNavigation_UpArrow_MovesCursorUp(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("after Down then Up arrow: cursor = %d, want 0", cursor)
	}
}

// TestItemNavigation_K_WrapsToLastAtStart, TestItemNavigation_J_WrapsToFirstAtEnd,
// and TestItemNavigation_ArrowKeys_WrapAtBounds cover the card list's
// migration onto the shared moveCursor wrap primitive (#426 PR 2): the
// per-column card list no longer clamps at the first/last card, it cycles,
// matching the four modal lists (PR list, agents list, assignee picker, git
// menu) that already wrap via moveCursor.

func TestItemNavigation_K_WrapsToLastAtStart(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	lastIndex := len(b.Columns[b.ActiveTab].Cards) - 1
	if lastIndex <= 0 {
		t.Fatal("active column needs more than one card to test wraparound")
	}

	// At cursor 0, pressing 'k' should wrap to the last card.
	b = sendKey(t, b, keyMsg("k"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != lastIndex {
		t.Errorf("'k' at cursor 0: cursor = %d, want %d (wrap to last card)", cursor, lastIndex)
	}
}

func TestItemNavigation_J_WrapsToFirstAtEnd(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	cardCount := len(b.Columns[b.ActiveTab].Cards)
	if cardCount <= 1 {
		t.Fatal("active column needs more than one card to test wraparound")
	}

	// Walk to the last card.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	lastIndex := cardCount - 1
	if got := b.Columns[b.ActiveTab].Cursor; got != lastIndex {
		t.Fatalf("precondition: cursor = %d, want %d (last card)", got, lastIndex)
	}

	// One more 'j' from the last card should wrap to the first.
	b = sendKey(t, b, keyMsg("j"))
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("'j' past last card: cursor = %d, want 0 (wrap to first)", got)
	}
}

// TestItemNavigation_ArrowKeys_WrapAtBounds confirms Up/Down arrow keys wrap
// identically to j/k in the card list (both route through moveCursor).
func TestItemNavigation_ArrowKeys_WrapAtBounds(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	lastIndex := len(b.Columns[b.ActiveTab].Cards) - 1
	if lastIndex <= 0 {
		t.Fatal("active column needs more than one card to test wraparound")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	if got := b.Columns[b.ActiveTab].Cursor; got != lastIndex {
		t.Errorf("Up arrow at cursor 0: cursor = %d, want %d (wrap to last card)", got, lastIndex)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("Down arrow at last card: cursor = %d, want 0 (wrap to first card)", got)
	}
}

// TestItemNavigation_SingleCard_JK_NoOp and TestItemNavigation_EmptyColumn_JK_NoOp
// cover the length<=1 guard for the card list specifically (moveCursor's own
// empty/single coverage landed in PR 1 for the four modal lists; the plan
// requires the same coverage for the card list and filter picker in PR 2).

func TestItemNavigation_SingleCard_JK_NoOp(t *testing.T) {
	b := newBoardWithCards(t, 1, 40)
	if got := len(b.Columns[b.ActiveTab].Cards); got != 1 {
		t.Fatalf("precondition: active column has %d cards, want 1", got)
	}

	b = sendKey(t, b, keyMsg("j"))
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("'j' on single-card column: cursor = %d, want 0 (no-op)", got)
	}

	b = sendKey(t, b, keyMsg("k"))
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("'k' on single-card column: cursor = %d, want 0 (no-op)", got)
	}
}

func TestItemNavigation_EmptyColumn_JK_NoOp(t *testing.T) {
	b, _ := newBoardWithEmptyColumn(t, nil)

	b = sendKey(t, b, keyMsg("j"))
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("'j' on empty column: cursor = %d, want 0 (no-op, no panic)", got)
	}

	b = sendKey(t, b, keyMsg("k"))
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("'k' on empty column: cursor = %d, want 0 (no-op, no panic)", got)
	}
}

// TestItemNavigation_KWrapsToLastFilteredCard_WhenFilterActive and
// TestItemNavigation_JWrapsToFirstFilteredCard_WhenFilterActive assert the
// card list's wrap length comes from b.visibleCards() (the search/filter-aware
// list), not the column's raw, unfiltered card count.
func TestItemNavigation_KWrapsToLastFilteredCard_WhenFilterActive(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Bug one", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Feature one", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 3, Title: "Bug two", Labels: []provider.Label{{Name: "bug"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	filtered := b.filteredCards()
	if len(filtered) != 2 {
		t.Fatalf("precondition: filteredCards() = %d, want 2", len(filtered))
	}

	// 'k' at cursor 0 with an active filter should wrap to the last
	// *filtered* card (index 1), not clamp in place, and never land on the
	// unfiltered column's excluded "Feature one" card.
	b = sendKey(t, b, keyMsg("k"))
	if got := b.Columns[b.ActiveTab].Cursor; got != len(filtered)-1 {
		t.Errorf("cursor after 'k' with active filter = %d, want %d (wrap to last filtered card)", got, len(filtered)-1)
	}
}

func TestItemNavigation_JWrapsToFirstFilteredCard_WhenFilterActive(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Bug one", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Feature one", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 3, Title: "Bug two", Labels: []provider.Label{{Name: "bug"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	filtered := b.filteredCards()
	if len(filtered) != 2 {
		t.Fatalf("precondition: filteredCards() = %d, want 2", len(filtered))
	}

	// Walk to the last filtered card.
	b = sendKey(t, b, keyMsg("j"))
	if got := b.Columns[b.ActiveTab].Cursor; got != len(filtered)-1 {
		t.Fatalf("precondition: cursor = %d, want %d (last filtered card)", got, len(filtered)-1)
	}

	// One more 'j' should wrap to the first filtered card instead of
	// clamping at the last filtered card's index.
	b = sendKey(t, b, keyMsg("j"))
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("'j' past last filtered card: cursor = %d, want 0 (wrap to first)", got)
	}
}

func TestItemNavigation_CursorIsPerColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	if len(b.Columns) < 2 {
		t.Fatal("board must have at least 2 columns for this test")
	}
	// Move cursor down in column 0
	b = sendKey(t, b, keyMsg("j"))
	// Switch to column 1
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	// Column 1 cursor should still be at 0
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("column 1 cursor after switching = %d, want 0 (cursor should be per-column)", cursor)
	}
}

// --- Card List Scroll ---

func TestScroll_AllCardsFit_NoScrollNeeded(t *testing.T) {
	cardCount := 5
	// Height large enough: panelHeight = Height - 6, each card ~1 line.
	// Use a tall terminal so all cards fit.
	height := cardCount + 6 + 10 // plenty of room
	b := newBoardWithCards(t, cardCount, height)

	// Navigate to the last card.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	col := b.Columns[b.ActiveTab]
	if col.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 when all cards fit in the viewport", col.ScrollOffset)
	}
}

func TestScroll_CursorDownScrollsViewport(t *testing.T) {
	cardCount := 30
	height := 15 // panelHeight = 15 - 6 = 9, far fewer than 30 cards
	b := newBoardWithCards(t, cardCount, height)

	// Navigate cursor well past the visible area.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	col := b.Columns[b.ActiveTab]
	if col.ScrollOffset <= 0 {
		t.Errorf("ScrollOffset = %d after scrolling past visible area, want > 0", col.ScrollOffset)
	}

	// Cursor should be at the last card.
	if col.Cursor != cardCount-1 {
		t.Errorf("Cursor = %d, want %d (last card)", col.Cursor, cardCount-1)
	}
}

func TestScroll_CursorUpScrollsViewport(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll all the way down.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	// Record the offset after scrolling down.
	offsetAfterDown := b.Columns[b.ActiveTab].ScrollOffset
	if offsetAfterDown <= 0 {
		t.Fatalf("expected ScrollOffset > 0 after scrolling down, got %d", offsetAfterDown)
	}

	// Now scroll all the way back up.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("k"))
	}

	col := b.Columns[b.ActiveTab]
	if col.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d after scrolling back to top, want 0", col.ScrollOffset)
	}
	if col.Cursor != 0 {
		t.Errorf("Cursor = %d after scrolling back to top, want 0", col.Cursor)
	}
}

func TestScroll_OffsetResetsOnTabSwitch(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll down in column A.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	// Switch to column B.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	// Column B should have ScrollOffset appropriate for its cursor position.
	// Since Column B cursor starts at 0 and has only 1 card, offset should be 0.
	col := b.Columns[b.ActiveTab]
	if col.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d after tab switch, want 0 (Column B has only 1 card)", col.ScrollOffset)
	}
}

// --- Wrapped Title Scroll ---

func TestScroll_WrappedTitles_CursorCardFullyVisible(t *testing.T) {
	// Create cards with long titles that will wrap, filling more visual lines.
	board := newBoardWithGeneratedCards(t, 15,
		"Card %d with a long title that should wrap to multiple lines in the panel", 80, 20)

	// Navigate down to a card near the bottom.
	for i := 0; i < 10; i++ {
		board = sendKey(t, board, keyMsg("j"))
	}

	col := board.Columns[board.ActiveTab]
	// Cursor should be at the card we navigated to.
	if col.Cursor != 10 {
		t.Errorf("Cursor = %d, want 10", col.Cursor)
	}

	// ScrollOffset should have adjusted (non-zero, since wrapped titles take more space).
	if col.ScrollOffset <= 0 {
		t.Errorf("ScrollOffset = %d, want > 0 when navigating to card 10 with wrapped titles", col.ScrollOffset)
	}
}

// --- Resize Clamp ---

func TestScroll_ResizeClampsOffset(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll down.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	offsetBefore := b.Columns[b.ActiveTab].ScrollOffset
	if offsetBefore <= 0 {
		t.Fatalf("expected ScrollOffset > 0 before resize, got %d", offsetBefore)
	}

	// Resize to a much larger height (all cards fit now).
	largeHeight := cardCount + 6 + 10
	m, _ := b.Update(tea.WindowSizeMsg{Width: 120, Height: largeHeight})
	b = m.(Board)

	col := b.Columns[b.ActiveTab]
	// With all cards fitting, the scroll offset should be clamped so we don't
	// have unnecessary blank space at the top.
	newPanelHeight := largeHeight - 6
	maxOffset := cardCount - newPanelHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if col.ScrollOffset > maxOffset {
		t.Errorf("ScrollOffset = %d after resize to large height, want <= %d (clamped)", col.ScrollOffset, maxOffset)
	}
}

// --- Number Key ---

func TestNumberKey_SwitchesToColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)

	// Pressing "1" should set ActiveTab to 0 (first column).
	b = sendKey(t, b, keyMsg("1"))
	if b.ActiveTab != 0 {
		t.Errorf("after '1': ActiveTab = %d, want 0", b.ActiveTab)
	}

	// Pressing "2" should set ActiveTab to 1 (second column).
	b = sendKey(t, b, keyMsg("2"))
	if b.ActiveTab != 1 {
		t.Errorf("after '2': ActiveTab = %d, want 1", b.ActiveTab)
	}

	// Pressing "3" should set ActiveTab to 2 (third and last column).
	b = sendKey(t, b, keyMsg("3"))
	if b.ActiveTab != 2 {
		t.Errorf("after '3': ActiveTab = %d, want 2", b.ActiveTab)
	}
}

func TestNumberKey_OutOfRange_NoChange(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)

	// Start at column 1 (ActiveTab=1) so we can detect changes.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 1 {
		t.Fatalf("precondition: ActiveTab = %d, want 1", b.ActiveTab)
	}

	columnCount := len(b.Columns)

	// Press a number beyond the column count (e.g., "4" on a 3-column board).
	outOfRange := fmt.Sprintf("%d", columnCount+1)
	b = sendKey(t, b, keyMsg(outOfRange))
	if b.ActiveTab != 1 {
		t.Errorf("after pressing %q (out of range): ActiveTab = %d, want 1 (unchanged)", outOfRange, b.ActiveTab)
	}

	// Press "0" which is not a valid column number (columns are 1-indexed).
	b = sendKey(t, b, keyMsg("0"))
	if b.ActiveTab != 1 {
		t.Errorf("after pressing '0': ActiveTab = %d, want 1 (unchanged)", b.ActiveTab)
	}
}

func TestNumberKey_ResetsScrollAndDetailOffset(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll down in column A to build up scroll offset.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.Columns[0].ScrollOffset <= 0 {
		t.Fatal("precondition: ScrollOffset should be > 0 after scrolling down")
	}

	// Set a nonzero detailScrollOffset manually.
	b.detailScrollOffset = 5

	// Press "1" to switch to column 0 (same column, but should reset offsets).
	b = sendKey(t, b, keyMsg("1"))

	col := b.Columns[b.ActiveTab]
	if col.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d after pressing '1', want 0 (should reset)", col.ScrollOffset)
	}
	if b.detailScrollOffset != 0 {
		t.Errorf("detailScrollOffset = %d after pressing '1', want 0 (should reset)", b.detailScrollOffset)
	}
}

func TestNumberKey_InDetailMode_SwitchesAndUnfocuses(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	requireColumns(t, b)

	// Enter detail focus with 'l'.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	// Press "2" to switch to column 1 while detail is focused.
	b = sendKey(t, b, keyMsg("2"))

	// Should switch to column 1.
	if b.ActiveTab != 1 {
		t.Errorf("after '2' in detail focus: ActiveTab = %d, want 1", b.ActiveTab)
	}

	// Should unfocus detail panel.
	if b.detailFocused {
		t.Error("after '2' in detail focus: detailFocused should be false")
	}

	// Scroll offsets should be reset.
	if b.detailScrollOffset != 0 {
		t.Errorf("detailScrollOffset = %d after '2' in detail focus, want 0", b.detailScrollOffset)
	}
}

func TestNumberKey_IgnoredInCreateMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)

	// Enter createMode.
	b = sendKey(t, b, keyMsg("n"))
	if b.mode != createMode {
		t.Fatalf("precondition: mode = %d, want %d (createMode)", b.mode, createMode)
	}

	origTab := b.ActiveTab

	// Press "2" in createMode.
	b = sendKey(t, b, keyMsg("2"))

	// Should NOT change ActiveTab.
	if b.ActiveTab != origTab {
		t.Errorf("'2' in createMode changed ActiveTab from %d to %d, want unchanged", origTab, b.ActiveTab)
	}

	// Should still be in createMode.
	if b.mode != createMode {
		t.Errorf("mode = %d after '2' in createMode, want %d (createMode)", b.mode, createMode)
	}
}

func TestNumberKey_IgnoredInConfigMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)

	// Enter configMode.
	b = sendKey(t, b, keyMsg("c"))
	if b.mode != configMode {
		t.Fatalf("precondition: mode = %d, want %d (configMode)", b.mode, configMode)
	}

	origTab := b.ActiveTab

	// Press "2" in configMode.
	b = sendKey(t, b, keyMsg("2"))

	// Should NOT change ActiveTab.
	if b.ActiveTab != origTab {
		t.Errorf("'2' in configMode changed ActiveTab from %d to %d, want unchanged", origTab, b.ActiveTab)
	}

	// Should still be in configMode.
	if b.mode != configMode {
		t.Errorf("mode = %d after '2' in configMode, want %d (configMode)", b.mode, configMode)
	}
}
