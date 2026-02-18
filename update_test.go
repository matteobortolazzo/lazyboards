package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
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

func TestItemNavigation_K_ClampsAtStart(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	// Already at cursor 0, pressing k should stay at 0
	b = sendKey(t, b, keyMsg("k"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("'k' at cursor 0: cursor = %d, want 0", cursor)
	}
}

func TestItemNavigation_J_ClampsAtEnd(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	cardCount := len(b.Columns[b.ActiveTab].Cards)
	if cardCount == 0 {
		t.Fatal("active column has no cards; cannot test cursor clamping")
	}
	// Press j more times than there are cards
	for i := 0; i < cardCount+1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	cursor := b.Columns[b.ActiveTab].Cursor
	lastIndex := cardCount - 1
	if cursor != lastIndex {
		t.Errorf("pressing 'j' past end: cursor = %d, want %d", cursor, lastIndex)
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

// --- Quit ---

func TestQuit_Q_ReturnsQuitCmd(t *testing.T) {
	b := newLoadedTestBoard(t)
	_, cmd := b.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("'q' key should return a non-nil Cmd (tea.Quit)")
	}
}

func TestQuit_CtrlC_ReturnsQuitCmd(t *testing.T) {
	b := newLoadedTestBoard(t)
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C should return a non-nil Cmd (tea.Quit)")
	}
}

// --- Window Resize ---

func TestWindowResize_UpdatesDimensions(t *testing.T) {
	b := newLoadedTestBoard(t)
	wantWidth := 120
	wantHeight := 40
	m, _ := b.Update(tea.WindowSizeMsg{Width: wantWidth, Height: wantHeight})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if updated.Width != wantWidth {
		t.Errorf("Width = %d, want %d", updated.Width, wantWidth)
	}
	if updated.Height != wantHeight {
		t.Errorf("Height = %d, want %d", updated.Height, wantHeight)
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
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, false)

	var cards []provider.Card
	for i := 0; i < 15; i++ {
		cards = append(cards, provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf("Card %d with a long title that should wrap to multiple lines in the panel", i+1),
			Labels: []string{"test"},
		})
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 80
	board.Height = 20

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

// --- Refresh ---

func TestNormalMode_R_RefreshesBoard(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Press 'r' in normalMode to trigger a background refresh.
	m, cmd := b.Update(keyMsg("r"))
	updated := m.(Board)

	// Should stay in normalMode (background refresh, not full-screen loading).
	if updated.mode != normalMode {
		t.Errorf("mode = %d after 'r' in normalMode, want %d (normalMode)", updated.mode, normalMode)
	}

	// Should set the refreshing flag.
	if !updated.refreshing {
		t.Error("refreshing should be true after 'r' in normalMode")
	}

	// Should return a non-nil cmd (spinner tick + fetch).
	if cmd == nil {
		t.Error("'r' in normalMode should return a non-nil cmd for refresh")
	}
}

func TestBoardFetched_AfterRefresh_ShowsMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Press 'r' to trigger background refresh (stays in normalMode with refreshing=true).
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Simulate the board being fetched again (this is a refresh, not first load).
	board, err := provider.NewFakeProvider().FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ = b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// After refresh completes, refreshing flag should be cleared.
	if b.refreshing {
		t.Error("refreshing should be false after boardFetchedMsg")
	}

	view := b.View()
	if !strings.Contains(view, "Board refreshed") {
		t.Errorf("View() after refresh should contain %q, got:\n%s", "Board refreshed", view)
	}
}

func TestClearStatusMsg_ClearsTimedMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Press 'r' to trigger refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Simulate board fetched (triggers "Board refreshed" message).
	board, err := provider.NewFakeProvider().FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ = b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// Verify "Board refreshed" is visible before clearing (precondition).
	viewBefore := b.View()
	if !strings.Contains(viewBefore, "Board refreshed") {
		t.Fatalf("precondition: View() should contain %q before clearStatusMsg", "Board refreshed")
	}

	// Send clearStatusMsg to clear the timed message.
	m, _ = b.Update(clearStatusMsg{})
	b = m.(Board)

	view := b.View()
	if strings.Contains(view, "Board refreshed") {
		t.Errorf("View() after clearStatusMsg should NOT contain %q", "Board refreshed")
	}

	// Normal hints should be restored.
	if !strings.Contains(view, "New") {
		t.Errorf("View() after clearStatusMsg should contain hint desc %q (hints restored)", "New")
	}
}

// --- Config Hint ---

func TestNormalMode_StatusBarShowsConfigHint(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()
	if !strings.Contains(view, "Config") {
		t.Errorf("View() status bar does not contain hint desc %q", "Config")
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

	// Pressing "3" should set ActiveTab to 2 (third column).
	b = sendKey(t, b, keyMsg("3"))
	if b.ActiveTab != 2 {
		t.Errorf("after '3': ActiveTab = %d, want 2", b.ActiveTab)
	}

	// Pressing "4" should set ActiveTab to 3 (fourth column).
	b = sendKey(t, b, keyMsg("4"))
	if b.ActiveTab != 3 {
		t.Errorf("after '4': ActiveTab = %d, want 3", b.ActiveTab)
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

	// Press a number beyond the column count (e.g., "5" on a 4-column board).
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

// --- Number Hint ---

func TestNumberHint_UpdatesOnSubsequentFetch(t *testing.T) {
	// Start with a board loaded from FakeProvider (4 columns).
	b := newLoadedTestBoard(t)
	initialColCount := len(b.Columns)
	initialHint := b.normalHints[0]
	expectedInitialKey := fmt.Sprintf("1-%d", initialColCount)
	if initialHint.Key != expectedInitialKey {
		t.Fatalf("initial hint key = %q, want %q", initialHint.Key, expectedInitialKey)
	}

	// Simulate a second fetch that returns a different number of columns.
	newBoard := provider.Board{
		Columns: []provider.Column{
			{Title: "Todo", Cards: nil},
			{Title: "Done", Cards: nil},
		},
	}
	m, _ := b.Update(boardFetchedMsg{board: newBoard})
	updated := m.(Board)

	// The number hint (first element) should reflect the new column count.
	newColCount := len(updated.Columns)
	expectedNewKey := fmt.Sprintf("1-%d", newColCount)
	if updated.normalHints[0].Key != expectedNewKey {
		t.Errorf("after re-fetch, hint key = %q, want %q", updated.normalHints[0].Key, expectedNewKey)
	}

	// The number of hints should not grow (no duplicate number hints prepended).
	if len(updated.normalHints) != len(b.normalHints) {
		t.Errorf("normalHints length changed: before=%d, after=%d (should stay same)", len(b.normalHints), len(updated.normalHints))
	}
}

// --- Status Bar ---

func TestStatusBar_HintsUpdateOnColumnSwitch(t *testing.T) {
	globalActions := map[string]config.Action{
		"o": {Name: "Global Open", Type: "url", URL: "https://global.com/{number}"},
	}
	columnConfigs := []config.ColumnConfig{
		{Name: "New"}, // No column-level actions.
		{
			Name: "Refined",
			Actions: map[string]config.Action{
				"o": {Name: "Deploy", Type: "url", URL: "https://deploy.com/{number}"},
			},
		},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, _ := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// On column 0: should show global action hint.
	view0 := b.View()
	if !strings.Contains(view0, "Global Open") {
		t.Errorf("on column 0, View() should contain %q, got:\n%s", "Global Open", view0)
	}

	// Tab to column 1: should show column-level action hint overriding global.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	view1 := b.View()
	if !strings.Contains(view1, "Deploy") {
		t.Errorf("on column 1, View() should contain %q, got:\n%s", "Deploy", view1)
	}
	if strings.Contains(view1, "Global Open") {
		t.Errorf("on column 1, View() should NOT contain %q", "Global Open")
	}

	// Shift+tab back to column 0: should show global hint again.
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	view0again := b.View()
	if !strings.Contains(view0again, "Global Open") {
		t.Errorf("back on column 0, View() should contain %q, got:\n%s", "Global Open", view0again)
	}
}

// --- Background Refresh ---

func TestBackgroundRefresh_DuplicateIgnored(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Simulate already refreshing.
	b.refreshing = true

	// Press 'r' again while already refreshing.
	_, cmd := b.Update(keyMsg("r"))

	// Should return nil cmd (duplicate refresh ignored).
	if cmd != nil {
		t.Error("pressing 'r' while already refreshing should return nil cmd")
	}
}

func TestBackgroundRefresh_NavigationDuringRefresh(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	if len(b.Columns) < 2 {
		t.Fatal("board must have at least 2 columns for this test")
	}

	// Start a background refresh.
	b.refreshing = true

	// Navigate between columns with Tab.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d after Tab during refresh, want 1", b.ActiveTab)
	}

	// Navigate cards with j.
	b = sendKey(t, b, keyMsg("j"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 1 {
		t.Errorf("Cursor = %d after 'j' during refresh, want 1", cursor)
	}

	// Mode should still be normalMode.
	if b.mode != normalMode {
		t.Errorf("mode = %d during navigation while refreshing, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestBackgroundRefresh_PreservesColumnIndex(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	if len(b.Columns) < 2 {
		t.Fatal("board must have at least 2 columns for this test")
	}
	b.Width = 120
	b.Height = 40

	// Navigate to column index 1.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 1 {
		t.Fatalf("precondition: ActiveTab = %d, want 1", b.ActiveTab)
	}

	// Press 'r' to start background refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Simulate the board being fetched again with same columns.
	board, err := provider.NewFakeProvider().FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ = b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// ActiveTab should be preserved at 1.
	if b.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d after background refresh, want 1 (should be preserved)", b.ActiveTab)
	}
}

func TestBackgroundRefresh_PreservesCardByNumber(t *testing.T) {
	b := newBoardWithCards(t, 5, 40)
	requireColumns(t, b)

	// Navigate to card at index 2 (Card #3).
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	targetNumber := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].Number

	// Start background refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Build a new board with same cards (same Numbers) for the fetch result.
	providerCards := make([]provider.Card, 5)
	for i := range providerCards {
		providerCards[i] = provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf("Card %d", i+1),
			Labels: []string{"test"},
		}
	}
	fetchMsg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: providerCards},
			{Title: "Column B", Cards: []provider.Card{
				{Number: 100, Title: "Other card", Labels: []string{"test"}},
			}},
		},
	}}

	m, _ = b.Update(fetchMsg)
	b = m.(Board)

	// Cursor should still point to the card with the same Number.
	col := b.Columns[b.ActiveTab]
	cursorCard := col.Cards[col.Cursor]
	if cursorCard.Number != targetNumber {
		t.Errorf("cursor card Number = %d after refresh, want %d (should be preserved by Number)", cursorCard.Number, targetNumber)
	}
}

func TestBackgroundRefresh_CardRemoved_ClampsCursor(t *testing.T) {
	b := newBoardWithCards(t, 5, 40)
	requireColumns(t, b)

	// Navigate to the last card (index 4).
	for i := 0; i < 4; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.Columns[b.ActiveTab].Cursor != 4 {
		t.Fatalf("precondition: Cursor = %d, want 4", b.Columns[b.ActiveTab].Cursor)
	}

	// Start background refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Send boardFetchedMsg with fewer cards (only 3 cards instead of 5).
	providerCards := make([]provider.Card, 3)
	for i := range providerCards {
		providerCards[i] = provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf("Card %d", i+1),
			Labels: []string{"test"},
		}
	}
	fetchMsg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: providerCards},
			{Title: "Column B", Cards: []provider.Card{
				{Number: 100, Title: "Other card", Labels: []string{"test"}},
			}},
		},
	}}

	m, _ = b.Update(fetchMsg)
	b = m.(Board)

	// Cursor should be clamped to the last valid index (2).
	col := b.Columns[b.ActiveTab]
	lastValidIndex := len(col.Cards) - 1
	if col.Cursor > lastValidIndex {
		t.Errorf("Cursor = %d after refresh with fewer cards, want <= %d (should be clamped)", col.Cursor, lastValidIndex)
	}
}

func TestBackgroundRefresh_FetchError_ShowsTimedMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Start background refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Send a fetch error.
	m, _ = b.Update(boardFetchErrorMsg{err: fmt.Errorf("connection failed")})
	b = m.(Board)

	// Mode should stay normalMode (not enter errorMode).
	if b.mode != normalMode {
		t.Errorf("mode = %d after fetch error during refresh, want %d (normalMode)", b.mode, normalMode)
	}

	// Refreshing flag should be cleared.
	if b.refreshing {
		t.Error("refreshing should be false after fetch error")
	}

	// View should contain "Refresh failed:" with the error detail.
	view := b.View()
	if !strings.Contains(view, "Refresh failed:") {
		t.Errorf("View() after fetch error during refresh should contain %q, got:\n%s", "Refresh failed:", view)
	}
	if !strings.Contains(view, "connection failed") {
		t.Errorf("View() after fetch error during refresh should contain the error detail %q, got:\n%s", "connection failed", view)
	}
}

func TestBackgroundRefresh_DetailFocused_PreservesHints(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	requireColumns(t, b)

	// Enter detail focus mode.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	// Press 'r' to start background refresh while in detail focus.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)
	if !b.refreshing {
		t.Fatal("precondition: refreshing should be true after 'r'")
	}

	// Simulate the board being fetched again (refresh completes).
	board, err := provider.NewFakeProvider().FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ = b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// Detail focus should be preserved.
	if !b.detailFocused {
		t.Error("detailFocused should still be true after background refresh completes")
	}

	// View should show "Board refreshed" timed message while active.
	view := b.View()
	if !strings.Contains(view, "Board refreshed") {
		t.Errorf("View() after refresh should contain %q, got:\n%s", "Board refreshed", view)
	}

	// Clear the timed message to reveal the underlying hints.
	m, _ = b.Update(clearStatusMsg{})
	b = m.(Board)

	// After clearing, detail-focus hints (e.g., "Back", "Scroll") should be visible, not normal hints.
	viewAfterClear := b.View()
	if !strings.Contains(viewAfterClear, "Back") {
		t.Errorf("View() after clearing timed message in detail focus should contain detail hint %q, got:\n%s", "Back", viewAfterClear)
	}
	if !strings.Contains(viewAfterClear, "Scroll") {
		t.Errorf("View() after clearing timed message in detail focus should contain detail hint %q, got:\n%s", "Scroll", viewAfterClear)
	}
	// Should NOT show normal-mode-only hints like "Quit".
	if strings.Contains(viewAfterClear, "Quit") {
		t.Errorf("View() after clearing timed message in detail focus should NOT contain normal hint %q", "Quit")
	}
}

func TestBackgroundRefresh_SpinnerTickPropagated(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Set refreshing in normalMode.
	b.refreshing = true

	// Send a spinner tick message.
	_, cmd := b.Update(b.spinner.Tick())

	// Spinner tick should be propagated (cmd is non-nil for the next tick).
	if cmd == nil {
		t.Error("spinner tick should return non-nil cmd when refreshing is true in normalMode")
	}
}

func TestStatusBar_ColumnOnlyActionAppearsOnlyInColumn(t *testing.T) {
	// No global action for "x".
	globalActions := map[string]config.Action{}
	columnConfigs := []config.ColumnConfig{
		{
			Name: "New",
			Actions: map[string]config.Action{
				"x": {Name: "Special", Type: "url", URL: "https://special.com/{number}"},
			},
		},
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, _ := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// On column 0: should show the column-only action hint.
	view0 := b.View()
	if !strings.Contains(view0, "Special") {
		t.Errorf("on column 0, View() should contain %q, got:\n%s", "Special", view0)
	}

	// Tab to column 1: should NOT show the column-only action hint.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	view1 := b.View()
	if strings.Contains(view1, "Special") {
		t.Errorf("on column 1, View() should NOT contain %q", "Special")
	}
}
