package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

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
	b = simulateRefresh(t, b)

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
	b = simulateRefresh(t, b)

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
	b = simulateRefresh(t, b)

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
	b = simulateRefresh(t, b)

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
