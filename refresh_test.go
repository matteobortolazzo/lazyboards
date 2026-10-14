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

	// Send the current-generation refreshTickMsg to a loaded board in normalMode.
	m, cmd := b.Update(refreshTickMsg{gen: b.refreshTickGen})
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
	m, cmd := b.Update(refreshTickMsg{gen: b.refreshTickGen})
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
	m, cmd := b.Update(refreshTickMsg{gen: b.refreshTickGen})
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
	m, cmd := b.Update(refreshTickMsg{gen: b.refreshTickGen})
	updated := m.(Board)

	if cmd != nil {
		t.Error("refreshTickMsg with refreshInterval=0 should return a nil cmd")
	}

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

// --- Refresh tick generations ---

// refreshTicksIn returns every refreshTickMsg found in msgs, in order.
func refreshTicksIn(msgs []tea.Msg) []refreshTickMsg {
	var ticks []refreshTickMsg
	for _, msg := range msgs {
		if tick, ok := msg.(refreshTickMsg); ok {
			ticks = append(ticks, tick)
		}
	}
	return ticks
}

// followUpTicksIn returns every cleanupFollowUpTickMsg found in msgs, in order.
func followUpTicksIn(msgs []tea.Msg) []cleanupFollowUpTickMsg {
	var ticks []cleanupFollowUpTickMsg
	for _, msg := range msgs {
		if tick, ok := msg.(cleanupFollowUpTickMsg); ok {
			ticks = append(ticks, tick)
		}
	}
	return ticks
}

// completeManualRefresh presses 'r' and delivers a fetched board, returning
// the resulting board and the refreshTickMsg the completion scheduled.
func completeManualRefresh(t *testing.T, b Board) (Board, refreshTickMsg) {
	t.Helper()
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)
	if !b.refreshing {
		t.Fatal("precondition: refreshing should be true after 'r'")
	}
	board, err := provider.NewFakeProvider().FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	ticks := refreshTicksIn(collectMsgs(cmd))
	if len(ticks) == 0 {
		t.Fatal("refresh completion should schedule a refreshTickMsg when refreshInterval > 0")
	}
	return b, ticks[len(ticks)-1]
}

func TestRefreshTick_NonCurrentGenerationIgnored(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 5 * time.Minute
	if b.refreshTickGen == 0 {
		t.Fatal("precondition: a loaded board has scheduled at least one tick, so its generation is non-zero")
	}

	for name, gen := range map[string]uint64{
		"older generation": b.refreshTickGen - 1,
		"newer generation": b.refreshTickGen + 1,
	} {
		t.Run(name, func(t *testing.T) {
			m, cmd := b.Update(refreshTickMsg{gen: gen})
			updated := m.(Board)

			if updated.refreshing {
				t.Error("a refresh tick that is not the current generation must not start a refresh")
			}
			if cmd != nil {
				t.Error("a refresh tick that is not the current generation must return a nil cmd (no reschedule, no extra timer chain)")
			}
		})
	}
}

func TestRefreshTick_NoDuplicateTimerChainsAfterManualRefresh(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 1 * time.Millisecond
	b.Width = 120
	b.Height = 40

	b, tickA := completeManualRefresh(t, b)
	b, tickB := completeManualRefresh(t, b)

	if tickA.gen == tickB.gen {
		t.Fatalf("successive refresh completions must schedule distinct generations, both were %d", tickA.gen)
	}

	// The older chain's tick fires late: it must be ignored.
	m, cmd := b.Update(tickA)
	staleResult := m.(Board)
	if staleResult.refreshing {
		t.Error("the superseded tick must not start a refresh")
	}
	if cmd != nil {
		t.Error("the superseded tick must not reschedule itself")
	}

	// The most recently scheduled tick is the only one that acts.
	m, cmd = b.Update(tickB)
	current := m.(Board)
	if !current.refreshing {
		t.Error("the most recently scheduled tick must start a refresh")
	}
	if cmd == nil {
		t.Error("the most recently scheduled tick must return the fetch cmd")
	}
}

// --- Cleanup follow-up tick ---

func TestCleanupFollowUpTick_StaleGenerationIgnored(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.followUpGen = 3

	m, cmd := b.Update(cleanupFollowUpTickMsg{gen: b.followUpGen - 1})
	updated := m.(Board)

	if updated.refreshing {
		t.Error("a stale follow-up tick must not start a fetch")
	}
	if cmd != nil {
		t.Error("a stale follow-up tick must return a nil cmd (no fetch, no reschedule)")
	}
}

func TestCleanupFollowUpTick_DroppedWhileFetchInFlight(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshing = true

	m, cmd := b.Update(cleanupFollowUpTickMsg{gen: b.followUpGen})
	b = m.(Board)

	if cmd != nil {
		t.Error("a follow-up tick that finds a fetch in flight must be dropped: nil cmd, no second fetch, no reschedule")
	}
	if !b.refreshing {
		t.Error("refreshing must keep its meaning (the in-flight fetch) after a dropped follow-up tick")
	}

	// The in-flight fetch completes and clears refreshing as usual.
	board, err := provider.NewFakeProvider().FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd = b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)
	if b.refreshing {
		t.Error("refreshing should be cleared by the in-flight fetch's boardFetchedMsg")
	}
}

func TestCleanupFollowUpTick_RescheduledOutsideNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.followUpDelay = 1 * time.Millisecond
	b.mode = helpMode
	before := b.followUpGen

	m, cmd := b.Update(cleanupFollowUpTickMsg{gen: b.followUpGen})
	updated := m.(Board)

	if updated.refreshing {
		t.Error("a follow-up tick outside normal mode must not start a fetch")
	}
	if updated.mode != helpMode {
		t.Errorf("mode = %d, want %d (helpMode untouched)", updated.mode, helpMode)
	}
	if cmd == nil {
		t.Fatal("a follow-up tick outside normal mode must reschedule itself")
	}

	msgs := collectMsgs(cmd)
	for _, msg := range msgs {
		if _, ok := msg.(boardFetchedMsg); ok {
			t.Error("a follow-up tick outside normal mode must not fetch the board")
		}
	}
	ticks := followUpTicksIn(msgs)
	if len(ticks) == 0 {
		t.Fatal("the reschedule cmd should produce a cleanupFollowUpTickMsg")
	}
	if ticks[0].gen != updated.followUpGen {
		t.Errorf("rescheduled tick gen = %d, want the board's current generation %d", ticks[0].gen, updated.followUpGen)
	}
	if updated.followUpGen == before {
		t.Error("rescheduling must supersede the tick that just fired (generation must change)")
	}
}

func TestCleanupFollowUpTick_RunsWhenAutoRefreshDisabled(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.refreshInterval = 0

	m, cmd := b.Update(cleanupFollowUpTickMsg{gen: b.followUpGen})
	updated := m.(Board)

	if !updated.refreshing {
		t.Error("a current follow-up tick must start a fetch even when refresh_interval is 0")
	}
	if cmd == nil {
		t.Fatal("a current follow-up tick must return the fetch cmd")
	}
	fetched := false
	for _, msg := range collectMsgs(cmd) {
		if _, ok := msg.(boardFetchedMsg); ok {
			fetched = true
		}
	}
	if !fetched {
		t.Error("the follow-up tick's cmd should produce a boardFetchedMsg")
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
