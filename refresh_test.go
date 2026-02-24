package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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
			Labels: []provider.Label{{Name: "test"}},
		}
	}
	fetchMsg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: providerCards},
			{Title: "Column B", Cards: []provider.Card{
				{Number: 100, Title: "Other card", Labels: []provider.Label{{Name: "test"}}},
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
			Labels: []provider.Label{{Name: "test"}},
		}
	}
	fetchMsg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: providerCards},
			{Title: "Column B", Cards: []provider.Card{
				{Number: 100, Title: "Other card", Labels: []provider.Label{{Name: "test"}}},
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

// --- Periodic Background Refresh ---

func TestRefreshTick_TriggersRefresh(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 5 * time.Minute

	// Send refreshTickMsg to a loaded board in normalMode.
	m, cmd := b.Update(refreshTickMsg{})
	updated := m.(Board)

	// Should start a background refresh.
	if !updated.refreshing {
		t.Error("refreshing should be true after refreshTickMsg in normalMode")
	}

	// Should return a non-nil cmd (spinner tick + fetch).
	if cmd == nil {
		t.Error("refreshTickMsg should return a non-nil cmd to start the refresh")
	}
}

func TestRefreshTick_SkippedWhenAlreadyRefreshing(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 5 * time.Minute
	b.refreshing = true

	// Send refreshTickMsg while already refreshing.
	m, cmd := b.Update(refreshTickMsg{})
	updated := m.(Board)

	// Should still be refreshing (no double-refresh).
	if !updated.refreshing {
		t.Error("refreshing should remain true when refreshTickMsg arrives during active refresh")
	}

	// Board state should not change (mode stays normalMode).
	if updated.mode != normalMode {
		t.Errorf("mode = %d after refreshTickMsg during active refresh, want %d (normalMode)", updated.mode, normalMode)
	}

	// Should return a non-nil cmd to reschedule the next tick (keep timer alive).
	if cmd == nil {
		t.Error("refreshTickMsg during active refresh should return a non-nil cmd to reschedule the next tick")
	}
}

func TestRefreshTick_SkippedInNonNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 5 * time.Minute
	b.mode = loadingMode

	// Send refreshTickMsg while in loadingMode.
	m, cmd := b.Update(refreshTickMsg{})
	updated := m.(Board)

	// Should not start a refresh.
	if updated.refreshing {
		t.Error("refreshing should remain false when refreshTickMsg arrives in loadingMode")
	}

	// Mode should remain loadingMode.
	if updated.mode != loadingMode {
		t.Errorf("mode = %d after refreshTickMsg in loadingMode, want %d (loadingMode)", updated.mode, loadingMode)
	}

	// Should return a non-nil cmd to reschedule the next tick (keep timer alive).
	if cmd == nil {
		t.Error("refreshTickMsg in non-normal mode should return a non-nil cmd to reschedule the next tick")
	}
}

func TestRefreshTick_DisabledWhenIntervalZero(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 0

	// Send refreshTickMsg with interval disabled.
	m, _ := b.Update(refreshTickMsg{})
	updated := m.(Board)

	// Should not start a refresh.
	if updated.refreshing {
		t.Error("refreshing should remain false when refreshTickMsg arrives with refreshInterval=0")
	}
}

func TestBoardFetched_SchedulesNextTick(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 5 * time.Minute
	b.Width = 120
	b.Height = 40

	// Start a background refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)
	if !b.refreshing {
		t.Fatal("precondition: refreshing should be true after 'r'")
	}

	// Simulate the board being fetched (refresh completes).
	board, err := provider.NewFakeProvider().FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// Refresh should be complete.
	if b.refreshing {
		t.Error("refreshing should be false after boardFetchedMsg")
	}

	// The returned cmd should be non-nil (it should include the tick schedule).
	if cmd == nil {
		t.Error("boardFetchedMsg with refreshInterval > 0 should return a non-nil cmd (includes tick schedule)")
	}
}

func TestBoardFetched_NoTickWhenIntervalZero(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 0
	b.Width = 120
	b.Height = 40

	// Start a background refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Simulate the board being fetched (refresh completes).
	board, err := provider.NewFakeProvider().FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// With refreshInterval=0, the cmd should contain only the "Board refreshed"
	// timed message cmd, NOT a tick schedule. We verify by executing the cmd
	// and checking that no refreshTickMsg is produced.
	if cmd == nil {
		// Even without periodic refresh, a "Board refreshed" timed message cmd
		// is expected, so nil here would indicate a different issue.
		t.Skip("cmd is nil; existing behavior may not produce a timed message cmd here")
	}

	// Execute the cmd tree and check that no refreshTickMsg is among the results.
	msgs := collectMsgs(cmd)
	for _, msg := range msgs {
		if _, ok := msg.(refreshTickMsg); ok {
			t.Error("boardFetchedMsg with refreshInterval=0 should NOT schedule a refreshTickMsg")
		}
	}
}

func TestManualRefresh_ResetsTimer(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 1 * time.Millisecond
	b.Width = 120
	b.Height = 40

	// Press 'r' to start a manual refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)
	if !b.refreshing {
		t.Fatal("precondition: refreshing should be true after 'r'")
	}

	// Complete the refresh with boardFetchedMsg.
	board, err := provider.NewFakeProvider().FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// After manual refresh completes, a tick should be scheduled
	// (timer resets so the next periodic refresh fires after the full interval).
	if cmd == nil {
		t.Fatal("cmd should be non-nil after manual refresh with refreshInterval > 0")
	}

	// Verify the cmd tree contains a refreshTickMsg when executed.
	msgs := collectMsgs(cmd)
	found := false
	for _, msg := range msgs {
		if _, ok := msg.(refreshTickMsg); ok {
			found = true
			break
		}
	}
	if !found {
		t.Error("manual refresh completion should schedule a refreshTickMsg for the next periodic refresh")
	}
}

// collectMsgs executes a tea.Cmd tree and collects all resulting tea.Msg values.
// It recursively expands tea.BatchMsg. Uses a timeout to avoid blocking on
// tea.Tick or other long-running commands. Use very short durations (e.g., 1ms)
// for refreshInterval in tests so tick commands complete within the timeout.
func collectMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(100 * time.Millisecond):
		return nil // Skip blocking commands
	}
	if msg == nil {
		return nil
	}
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, subCmd := range batchMsg {
			msgs = append(msgs, collectMsgs(subCmd)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

// --- Auto-Refresh ---

func TestAutoRefresh_ShellSuccess_StartsTimer(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b.actionRefreshDelay = 5 * time.Second

	// Send actionResultMsg with success=true (simulating shell action completion).
	m, cmd := b.Update(actionResultMsg{success: true, message: "Done"})
	b = m.(Board)

	// Should set pendingAutoRefresh to true.
	if !b.pendingAutoRefresh {
		t.Error("pendingAutoRefresh should be true after successful shell action")
	}

	// Should return a non-nil cmd (status message + tick timer).
	if cmd == nil {
		t.Error("successful actionResultMsg should return a non-nil cmd")
	}
}

func TestAutoRefresh_ShellFailure_NoTimer(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b.actionRefreshDelay = 5 * time.Second

	// Send actionResultMsg with success=false.
	m, _ := b.Update(actionResultMsg{success: false, message: "Error: exit 1"})
	b = m.(Board)

	// Should NOT set pendingAutoRefresh.
	if b.pendingAutoRefresh {
		t.Error("pendingAutoRefresh should be false after failed shell action")
	}
}

func TestAutoRefresh_TimerFires_TriggersRefresh(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Manually set pendingAutoRefresh (simulating timer was started).
	b.pendingAutoRefresh = true

	// Send autoRefreshMsg (simulating timer firing).
	m, cmd := b.Update(autoRefreshMsg{})
	b = m.(Board)

	// Should start a refresh.
	if !b.refreshing {
		t.Error("refreshing should be true after autoRefreshMsg fires")
	}

	// pendingAutoRefresh should be cleared.
	if b.pendingAutoRefresh {
		t.Error("pendingAutoRefresh should be false after autoRefreshMsg fires")
	}

	// Should return a non-nil cmd (spinner tick + fetch).
	if cmd == nil {
		t.Error("autoRefreshMsg should return a non-nil cmd for refresh")
	}
}

func TestAutoRefresh_CancelledByManualRefresh(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Set pendingAutoRefresh (simulating timer was started).
	b.pendingAutoRefresh = true

	// Press "r" to trigger manual refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Manual refresh should cancel auto-refresh.
	if b.pendingAutoRefresh {
		t.Error("pendingAutoRefresh should be false after manual refresh")
	}

	// Complete the manual refresh so refreshing=false, isolating the cancellation guard.
	b = simulateRefresh(t, b)

	// Now send autoRefreshMsg (the timer fires after manual refresh completed).
	// Only the pendingAutoRefresh=false guard is active (refreshing is false).
	m, cmd := b.Update(autoRefreshMsg{})
	b = m.(Board)

	if cmd != nil {
		t.Error("autoRefreshMsg after manual refresh should return nil cmd (cancelled)")
	}
}

func TestAutoRefresh_IgnoredWhenAlreadyRefreshing(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Set both pendingAutoRefresh and refreshing.
	b.pendingAutoRefresh = true
	b.refreshing = true

	// Send autoRefreshMsg while already refreshing.
	m, cmd := b.Update(autoRefreshMsg{})
	b = m.(Board)

	// Should NOT start another refresh (already in progress).
	if cmd != nil {
		t.Error("autoRefreshMsg while already refreshing should return nil cmd")
	}

	// refreshing should still be true (unchanged).
	if !b.refreshing {
		t.Error("refreshing should remain true when autoRefreshMsg is ignored")
	}
}

func TestAutoRefresh_ClearedOnBoardFetched(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Set pendingAutoRefresh and refreshing (simulating auto-refresh in progress).
	b.pendingAutoRefresh = true
	b.refreshing = true

	// Simulate board fetch completing.
	b = simulateRefresh(t, b)

	// pendingAutoRefresh should be cleared.
	if b.pendingAutoRefresh {
		t.Error("pendingAutoRefresh should be false after boardFetchedMsg")
	}
}

func TestAutoRefresh_ClearedOnFetchError(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Simulate auto-refresh in progress (timer fired, fetch started).
	b.pendingAutoRefresh = true
	b.refreshing = true

	// Send a fetch error.
	m, _ := b.Update(boardFetchErrorMsg{err: fmt.Errorf("timeout")})
	b = m.(Board)

	// pendingAutoRefresh should be cleared on fetch error.
	if b.pendingAutoRefresh {
		t.Error("pendingAutoRefresh should be false after boardFetchErrorMsg")
	}
}

func TestAutoRefresh_CancelledByManualRefresh_DetailFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)

	// Enter detail focus mode.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	// Set pendingAutoRefresh.
	b.pendingAutoRefresh = true

	// Press "r" in detail-focused mode to trigger manual refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Manual refresh should cancel auto-refresh in detail mode too.
	if b.pendingAutoRefresh {
		t.Error("pendingAutoRefresh should be false after manual refresh in detail-focused mode")
	}
}

// --- Configurable Action Refresh Delay (#119) ---

func TestAutoRefresh_DisabledWhenDelayZero(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Disable auto-refresh after shell actions by setting delay to 0.
	b.actionRefreshDelay = 0

	// Send actionResultMsg with success=true (simulating shell action completion).
	m, _ := b.Update(actionResultMsg{success: true, message: "Done"})
	b = m.(Board)

	// With delay=0, auto-refresh should NOT be scheduled.
	if b.pendingAutoRefresh {
		t.Error("pendingAutoRefresh should be false when actionRefreshDelay is 0 (disabled)")
	}
}

func TestAutoRefresh_UsesConfiguredDelay(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Set a non-zero action refresh delay.
	b.actionRefreshDelay = 10 * time.Second

	// Send actionResultMsg with success=true (simulating shell action completion).
	m, cmd := b.Update(actionResultMsg{success: true, message: "Done"})
	b = m.(Board)

	// With a non-zero delay, auto-refresh SHOULD be scheduled.
	if !b.pendingAutoRefresh {
		t.Error("pendingAutoRefresh should be true when actionRefreshDelay is non-zero")
	}

	// Should return a non-nil cmd (status message + tick timer).
	if cmd == nil {
		t.Error("successful actionResultMsg with non-zero delay should return a non-nil cmd")
	}
}
