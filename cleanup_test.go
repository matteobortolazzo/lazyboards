package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newCleanupTestBoardWith creates a Board with cleanup configured on col 0
// ("New"), a FakeExecutor, and a FakeProvider, wired with the given
// cenciwatch.Watcher (nil if the test doesn't need one). Initial load
// populates prevCards.
func newCleanupTestBoardWith(t *testing.T, cleanup string, watcher cenciwatch.Watcher) (Board, *action.FakeExecutor, *provider.FakeProvider) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	columnConfigs := []config.ColumnConfig{
		{Name: "New", Cleanup: &cleanup},
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b := NewBoard(p, nil, nil, columnConfigs, fe, "matteobortolazzo", "lazyboards", "github", 32, 0, 0, "Working", false, false, watcher, nil)
	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)
	b.Width = 120
	b.Height = 40
	return b, fe, p
}

// newCleanupTestBoard creates a Board with cleanup configured on col 0 ("New"),
// a FakeExecutor, and a FakeProvider. Initial load populates prevCards.
func newCleanupTestBoard(t *testing.T, cleanup string) (Board, *action.FakeExecutor, *provider.FakeProvider) {
	t.Helper()
	return newCleanupTestBoardWith(t, cleanup, nil)
}

// newCleanupTestBoardWithWatcher mirrors newCleanupTestBoard but wires a
// non-nil cenciwatch.Watcher (an empty FakeWatcher) so b.agentWatcher != nil,
// while leaving b.agentSnapshot nil (no snapshot delivered yet).
func newCleanupTestBoardWithWatcher(t *testing.T, cleanup string) (Board, *action.FakeExecutor, *provider.FakeProvider) {
	t.Helper()
	b, fe, p := newCleanupTestBoardWith(t, cleanup, &cenciwatch.FakeWatcher{})
	b.agentSnapshot = nil
	return b, fe, p
}

// fakeRefreshBoard builds a provider.Board based on FakeProvider data but with
// the given card numbers removed from col 0 ("New") and added to col 1 ("Refined").
// If a card number is negative, it is removed entirely (simulating closure).
func fakeRefreshBoard(movedCards ...int) provider.Board {
	movedSet := make(map[int]bool)
	removedSet := make(map[int]bool)
	for _, n := range movedCards {
		if n < 0 {
			removedSet[-n] = true
		} else {
			movedSet[n] = true
		}
	}

	// Start from FakeProvider's default data.
	base := provider.NewFakeProvider()
	original, _ := base.FetchBoard(context.TODO())

	var cols []provider.Column
	for i, col := range original.Columns {
		var filtered []provider.Card
		for _, c := range col.Cards {
			if i == 0 && (movedSet[c.Number] || removedSet[c.Number]) {
				continue
			}
			filtered = append(filtered, c)
		}
		// Add moved cards to col 1 ("Refined").
		if i == 1 {
			for _, c := range original.Columns[0].Cards {
				if movedSet[c.Number] {
					filtered = append([]provider.Card{c}, filtered...)
				}
			}
		}
		cols = append(cols, provider.Column{Title: col.Title, Cards: filtered})
	}
	return provider.Board{Columns: cols}
}

// refreshCleanupBoard simulates a background refresh delivering the given
// board, executing any resulting async commands (including cleanup).
func refreshCleanupBoard(t *testing.T, b Board, board provider.Board) Board {
	t.Helper()
	b.refreshing = true
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)
	return b
}

// withCardLabel returns the board with the given label added to card cardNum.
func withCardLabel(board provider.Board, cardNum int, label string) provider.Board {
	for i := range board.Columns {
		for j := range board.Columns[i].Cards {
			if board.Columns[i].Cards[j].Number == cardNum {
				board.Columns[i].Cards[j].Labels = append(board.Columns[i].Cards[j].Labels, provider.Label{Name: label})
			}
		}
	}
	return board
}

// cleanupSnapshot builds an agentwatch snapshot with a single window for card
// #1 ("Setup CI") in the given status. An empty status means no windows.
func cleanupSnapshot(status string) *cenciwatch.StateSnapshot {
	if status == "" {
		return &cenciwatch.StateSnapshot{}
	}
	session := action.BuildSessionName(1, "Setup CI", 32)
	return &cenciwatch.StateSnapshot{Windows: []cenciwatch.WindowState{
		{WindowName: session, Status: status, Agent: "claude"},
	}}
}

func TestCleanup_FirstLoad_NoPrevCards(t *testing.T) {
	_, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls on first load, got %d: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}

func TestCleanup_CardMovesColumn(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session} 2>/dev/null || true")

	// The move-debounce (#363) defers a column-change departure to the second
	// consecutive fetch, so two refreshes are needed before cleanup fires.
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call for card leaving column, got none")
	}
	found := false
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "tmux kill-window") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected cleanup command containing 'tmux kill-window', got: %v", fe.RunShellCalls)
	}
}

// TestCleanup_CardMovesColumn_DebouncedToSecondFetch asserts the move-debounce
// explicitly: a single fetch observing a column change must never fire
// cleanup on its own, since a single bad fetch that misplaces cards (e.g. a
// dropped-label fallback moving everything to column 0) must not trigger a
// board-wide cleanup in one shot.
func TestCleanup_CardMovesColumn_DebouncedToSecondFetch(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session} 2>/dev/null || true")

	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected no cleanup after a single fetch observing the move, got: %v", fe.RunShellCalls)
	}

	refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call after card moved on two consecutive fetches, got none")
	}
}

// TestCleanup_CardMovesBackBeforeSecondFetch_NoCleanup asserts that a card
// which reverts to its original column before the second fetch resets the
// move-debounce entirely -- cleanup must never fire for it.
func TestCleanup_CardMovesBackBeforeSecondFetch_NoCleanup(t *testing.T) {
	b, fe, p := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected cleanup deferred on first move, got: %v", fe.RunShellCalls)
	}

	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	b = refreshCleanupBoard(t, b, board)
	refreshCleanupBoard(t, b, board)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no cleanup for card that moved back before second fetch, got: %v", fe.RunShellCalls)
	}
}

func TestCleanup_CardDisappears_DebouncedToSecondFetch(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	// Card #1 disappears entirely (negative = removed, not moved). A single
	// fetch without the card can be a transient glitch (e.g. pagination shift
	// while issues close), so no cleanup runs yet.
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(-1))
	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected no cleanup after a single fetch without the card, got: %v", fe.RunShellCalls)
	}

	// Still missing on the second consecutive fetch — now it's a real departure.
	refreshCleanupBoard(t, b, fakeRefreshBoard(-1))
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call after card missing on two consecutive fetches, got none")
	}
}

func TestCleanup_CardMissingOnceThenReappears_NoCleanup(t *testing.T) {
	b, fe, p := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	// Transient glitch: card #1 missing on one fetch, back in its original
	// column on the next. Cleanup must never run.
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(-1))
	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	b = refreshCleanupBoard(t, b, board)
	refreshCleanupBoard(t, b, board)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no cleanup for card that reappeared, got: %v", fe.RunShellCalls)
	}
}

func TestCleanup_CardStaysSameColumn(t *testing.T) {
	b, fe, p := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	b.refreshing = true
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls when no cards moved, got %d: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}

func TestCleanup_NoCleanupConfigured(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "") // empty cleanup

	b.refreshing = true
	m, cmd := b.Update(boardFetchedMsg{board: fakeRefreshBoard(1)})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls when no cleanup configured, got %d: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}

func TestCleanup_MultipleCards(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	// Cards #1 and #2 both leave col 0. The move-debounce (#363) defers to
	// the second consecutive fetch.
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1, 2))
	refreshCleanupBoard(t, b, fakeRefreshBoard(1, 2))

	if len(fe.RunShellCalls) < 2 {
		t.Errorf("expected at least 2 RunShell calls for 2 cards leaving column, got %d: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}

func TestCleanup_TemplateVarsExpanded(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "cleanup {number} {session}")

	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call, got none")
	}

	call := fe.RunShellCalls[0]
	expectedSession := action.BuildSessionName(1, "Setup CI", 32)
	if !strings.Contains(call, "'1'") {
		t.Errorf("cleanup command should contain shell-escaped card number '1', got: %s", call)
	}
	if !strings.Contains(call, action.ShellEscape(expectedSession)) {
		t.Errorf("cleanup command should contain shell-escaped session %q, got: %s", action.ShellEscape(expectedSession), call)
	}
}

func TestCleanup_CleanupResultMsg_ShowsStatusMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	m, _ := b.Update(cleanupResultMsg{count: 2})
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, "Cleaned up") {
		t.Errorf("View() after cleanupResultMsg should contain 'Cleaned up', got:\n%s", view)
	}
}

func TestCleanup_CleanupResultMsg_ZeroCount_NoMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	_, cmd := b.Update(cleanupResultMsg{count: 0})

	if cmd != nil {
		t.Error("cleanupResultMsg with count=0 should not return a cmd")
	}
}

// --- Liveness guards (#285): cleanup must never kill a live agent window ---

func TestCleanup_DeferredWhileAgentRunning(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t ={session}")

	// Card #1 moves column while agentwatch reports its window as running.
	b.agentSnapshot = cleanupSnapshot("running")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected cleanup deferred while agent running, got: %v", fe.RunShellCalls)
	}

	// The agent finishes; the next refresh runs the deferred cleanup.
	b.agentSnapshot = cleanupSnapshot("done")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call after agent finished, got none")
	}

	// Cleanup runs once, then the departure is settled — no re-fire.
	calls := len(fe.RunShellCalls)
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	if len(fe.RunShellCalls) != calls {
		t.Errorf("expected no further cleanup after departure settled, got: %v", fe.RunShellCalls[calls:])
	}
}

func TestCleanup_DeferredWhileAgentNeedsInput(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t ={session}")

	b.agentSnapshot = cleanupSnapshot("need-input")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected cleanup deferred while agent needs input, got: %v", fe.RunShellCalls)
	}

	// The window disappears from the snapshot; cleanup proceeds.
	b.agentSnapshot = cleanupSnapshot("")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call after agent window gone, got none")
	}
}

func TestCleanup_DeferredWhileAgentRunning_CardMissing(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t ={session}")

	// Card #1 vanishes entirely while its agent is still running: deferred
	// past the missing-card debounce for as long as the agent is alive.
	b.agentSnapshot = cleanupSnapshot("running")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(-1))
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(-1))
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(-1))
	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected cleanup deferred while agent running on missing card, got: %v", fe.RunShellCalls)
	}

	b.agentSnapshot = cleanupSnapshot("done")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(-1))
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call after agent finished, got none")
	}
}

func TestCleanup_DeferredWhileWorkingLabelSet(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t ={session}")

	// Card #1 moves column but still carries the working label (case-insensitive).
	b = refreshCleanupBoard(t, b, withCardLabel(fakeRefreshBoard(1), 1, "working"))
	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected cleanup deferred while working label set, got: %v", fe.RunShellCalls)
	}

	// The label is removed; the next refresh runs the deferred cleanup.
	refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call after working label removed, got none")
	}
}

// --- {window} template variable (#309) ---
//
// agentwatch names dispatched windows "{number}-{skill}" (e.g. "1-refine"),
// not the reconstructed "{number}-{title-slug}" that BuildSessionName
// produces. The cleanup hook's kill-window target must resolve to the LIVE
// window name so `tmux kill-window -t {window}` actually matches, falling
// back to {session} only when no live window is available.

// cleanupSnapshotWithWindow builds an agentwatch snapshot with a single
// window using an explicit window name (distinct from BuildSessionName's
// reconstructed name), so tests can distinguish live-window resolution from
// the {session} fallback.
func cleanupSnapshotWithWindow(windowName, status string) *cenciwatch.StateSnapshot {
	return &cenciwatch.StateSnapshot{Windows: []cenciwatch.WindowState{
		{WindowName: windowName, Status: status, Agent: "claude"},
	}}
}

func TestCleanup_WindowVariable_UsesLiveWindowName(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {window} 2>/dev/null || true")

	// "done" is a non-busy status so the liveness guard lets cleanup proceed
	// (mirrors TestCleanup_DeferredWhileAgentRunning's "done" step), while the
	// window is still present in the snapshot at cleanup time.
	liveWindow := "1-refine"
	b.agentSnapshot = cleanupSnapshotWithWindow(liveWindow, "done")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call, got none")
	}
	call := fe.RunShellCalls[len(fe.RunShellCalls)-1]
	if !strings.Contains(call, action.ShellEscape(liveWindow)) {
		t.Errorf("cleanup command should target the live agentwatch window %q, got: %s", liveWindow, call)
	}
}

func TestCleanup_WindowVariable_FallsBackToSessionWhenSnapshotNil(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {window} 2>/dev/null || true")

	// No agentwatch snapshot at all (agentwatch off/absent).
	b.agentSnapshot = nil
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call, got none")
	}
	call := fe.RunShellCalls[len(fe.RunShellCalls)-1]
	expectedSession := action.BuildSessionName(1, "Setup CI", 32)
	if !strings.Contains(call, action.ShellEscape(expectedSession)) {
		t.Errorf("cleanup command should fall back to session name %q when no snapshot exists, got: %s", expectedSession, call)
	}
}

func TestCleanup_WindowVariable_FallsBackToSessionWhenNoWindowMatches(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {window} 2>/dev/null || true")

	// Snapshot exists but its only window belongs to a different ticket.
	b.agentSnapshot = cleanupSnapshotWithWindow("99-refine", "done")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call, got none")
	}
	call := fe.RunShellCalls[len(fe.RunShellCalls)-1]
	expectedSession := action.BuildSessionName(1, "Setup CI", 32)
	if !strings.Contains(call, action.ShellEscape(expectedSession)) {
		t.Errorf("cleanup command should fall back to session name %q when no window matches card #1, got: %s", expectedSession, call)
	}
}

// --- Fail-closed guard (#362): agentwatch enabled but no snapshot delivered yet ---

func TestCleanup_DeferredWhenAgentwatchEnabledButSnapshotNotYetDelivered(t *testing.T) {
	b, fe, _ := newCleanupTestBoardWithWatcher(t, "tmux kill-window -t {session}")

	// Two fetches while agentwatch is enabled but no snapshot has arrived yet:
	// the fail-closed guard must defer cleanup both times, same as a live agent.
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls while agentwatch snapshot is nil, got %d: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}

	// Snapshot arrives (empty — no windows in it): the guard no longer fails
	// closed, so the deferred departure fires on the next fetch.
	b.agentSnapshot = cleanupSnapshot("")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) == 0 {
		t.Error("expected cleanup RunShell call once snapshot is delivered, got none")
	}
}

// --- Circuit breaker (#363): skip implausible mass cleanups ---

func TestCleanupCircuitBreakerTripped_BelowBothThresholds_NotTripped(t *testing.T) {
	if cleanupCircuitBreakerTripped(2, 10) {
		t.Error("expected not tripped: 2 cleanups on a 10-card board is plausible")
	}
}

func TestCleanupCircuitBreakerTripped_AtOrAboveAbsoluteFloor_Tripped(t *testing.T) {
	if !cleanupCircuitBreakerTripped(cleanupCircuitBreakerMinCount, 1000) {
		t.Error("expected tripped: hitting the absolute floor count trips regardless of board size")
	}
}

func TestCleanupCircuitBreakerTripped_AboveFractionOnSmallBoard_Tripped(t *testing.T) {
	// 3 cleanups on a 4-card board is 75% of tracked cards -- implausible
	// even though 3 is below the absolute floor.
	if !cleanupCircuitBreakerTripped(3, 4) {
		t.Error("expected tripped: 3 of 4 tracked cards cleaning up in one fetch is implausible")
	}
}

// newCircuitBreakerTestBoard creates a Board with cardCount synthetic cards
// (numbered 1..cardCount) all placed in column 0 ("New", cleanup-configured)
// and loaded into prevCards.
func newCircuitBreakerTestBoard(t *testing.T, cleanup string, cardCount int) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	columnConfigs := []config.ColumnConfig{
		{Name: "New", Cleanup: &cleanup},
		{Name: "Refined"},
	}
	b := NewBoard(p, nil, nil, columnConfigs, fe, "matteobortolazzo", "lazyboards", "github", 32, 0, 0, "Working", false, false, nil, nil)
	m, cmd := b.Update(boardFetchedMsg{board: circuitBreakerBoard(cardCount, nil)})
	b = m.(Board)
	execCmds(cmd)
	b.Width = 120
	b.Height = 40
	return b, fe
}

// circuitBreakerBoard builds a synthetic provider.Board with cardCount cards
// numbered 1..cardCount. Cards whose number is in movedNumbers are placed in
// column 1 ("Refined"); all others stay in column 0 ("New").
func circuitBreakerBoard(cardCount int, movedNumbers []int) provider.Board {
	moved := make(map[int]bool, len(movedNumbers))
	for _, n := range movedNumbers {
		moved[n] = true
	}
	var col0, col1 []provider.Card
	for i := 1; i <= cardCount; i++ {
		card := provider.Card{Number: i, Title: fmt.Sprintf("Card %d", i)}
		if moved[i] {
			col1 = append(col1, card)
		} else {
			col0 = append(col0, card)
		}
	}
	return provider.Board{Columns: []provider.Column{
		{Title: "New", Cards: col0},
		{Title: "Refined", Cards: col1},
	}}
}

// circuitBreakerSnapshot builds an agentwatch snapshot with a running window
// for each of busyNumbers, so the liveness guard defers those cards
// indefinitely regardless of how many fetches occur.
func circuitBreakerSnapshot(busyNumbers []int) *cenciwatch.StateSnapshot {
	windows := make([]cenciwatch.WindowState, len(busyNumbers))
	for i, n := range busyNumbers {
		windows[i] = cenciwatch.WindowState{WindowName: fmt.Sprintf("%d-work", n), Status: "running", Agent: "claude"}
	}
	return &cenciwatch.StateSnapshot{Windows: windows}
}

func TestCleanupCircuitBreaker_TripsOnMassDeparture(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "debug.log")
	if err := debuglog.Init(logPath); err != nil {
		t.Fatalf("debuglog.Init(%q) error = %v, want nil", logPath, err)
	}
	t.Cleanup(func() { _ = debuglog.Init("") })

	b, fe := newCircuitBreakerTestBoard(t, "tmux kill-window -t {session}", 8)

	// All 8 cards move at once -- the move-debounce (#363) requires two
	// consecutive fetches before departures are confirmed, so drive both.
	moved := []int{1, 2, 3, 4, 5, 6, 7, 8}
	b = refreshCleanupBoard(t, b, circuitBreakerBoard(8, moved))
	b = refreshCleanupBoard(t, b, circuitBreakerBoard(8, moved))

	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected circuit breaker to block all cleanups for an implausible mass departure, got: %v", fe.RunShellCalls)
	}

	view := b.View()
	if !strings.Contains(view, "Cleanup skipped") {
		t.Errorf("expected status bar warning after circuit breaker trip, view:\n%s", view)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read debug log %q: %v", logPath, err)
	}
	if !strings.Contains(string(data), "circuit breaker") {
		t.Errorf("expected debug log to record the circuit breaker trip, got: %q", string(data))
	}
}

func TestCleanupCircuitBreaker_DoesNotTripOnSmallDeparture(t *testing.T) {
	b, fe := newCircuitBreakerTestBoard(t, "tmux kill-window -t {session}", 8)

	// Only 2 of 8 cards depart -- well under both thresholds.
	moved := []int{1, 2}
	b = refreshCleanupBoard(t, b, circuitBreakerBoard(8, moved))
	refreshCleanupBoard(t, b, circuitBreakerBoard(8, moved))

	if len(fe.RunShellCalls) < 2 {
		t.Errorf("expected circuit breaker not to trip for 2 of 8 departures, got %d RunShell calls: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}

func TestCleanupCircuitBreaker_DoesNotCountGuardDeferredCards(t *testing.T) {
	b, fe := newCircuitBreakerTestBoard(t, "tmux kill-window -t {session}", 10)

	// 8 of the 10 cards move while their agent is still running (deferred by
	// the liveness guard, never counted); the other 2 depart for real. The
	// circuit breaker must judge only the 2 real departures against the
	// 10 tracked cards, not the raw count of 10 cards that moved.
	busy := []int{1, 2, 3, 4, 5, 6, 7, 8}
	allMoved := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	b.agentSnapshot = circuitBreakerSnapshot(busy)
	b = refreshCleanupBoard(t, b, circuitBreakerBoard(10, allMoved))
	b = refreshCleanupBoard(t, b, circuitBreakerBoard(10, allMoved))

	if len(fe.RunShellCalls) < 2 {
		t.Errorf("expected the 2 non-busy departures to fire despite 8 busy cards also moving, got %d RunShell calls: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}
