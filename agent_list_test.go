package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newAgentListBoard builds a loaded board (one column "Column A" with cards
// #42 and #7), a wired FakeExecutor, an active watcher, and the given agent
// windows stored as the current snapshot.
func newAgentListBoard(t *testing.T, fe *action.FakeExecutor, windows []cenciwatch.WindowState) Board {
	t.Helper()
	cards := []provider.Card{
		{Number: 42, Title: "Card A"},
		{Number: 7, Title: "Card B"},
	}
	b := newBoardWithInlineCardsAndExecutor(t, cards, fe)
	b.cenciWatcher = &cenciwatch.FakeWatcher{}
	b.agentSnapshot = &cenciwatch.StateSnapshot{Windows: windows}
	return b
}

// threeWindows returns a snapshot fixture covering the three row shapes: a
// window joined to a board card, an unmatched numbered window, and a
// non-numbered window.
func threeWindows() []cenciwatch.WindowState {
	return []cenciwatch.WindowState{
		{Session: "dev", WindowIndex: "3", WindowName: "42-implement", Status: agentStatusRunning, Agent: "claude"},
		{Session: "dev", WindowIndex: "5", WindowName: "999-research", Status: "idle"},
		{Session: "ops", WindowIndex: "1", WindowName: "scratch", Status: "done"},
	}
}

// --- Opening the modal ---

func TestNormalMode_W_OpensAgentListModal(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())

	m, cmd := b.Update(keyMsg("w"))
	b = m.(Board)

	if b.mode != agentListMode {
		t.Errorf("mode after w = %d, want agentListMode (%d)", b.mode, agentListMode)
	}
	if cmd != nil {
		t.Errorf("cmd after w = %v, want nil (modal reads the stored snapshot, no fetch)", cmd)
	}
	if b.agentList.cursor != 0 {
		t.Errorf("cursor after open = %d, want 0", b.agentList.cursor)
	}
}

// --- Entries: every window is listed, matched or not ---

func TestAgentList_Entries_ListAllWindowsWithCardRefs(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())

	entries := b.agentListEntries()

	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3 (all windows, matched or not)", len(entries))
	}
	if entries[0].cardNumber != 42 || entries[0].columnTitle != "Column A" {
		t.Errorf("entry 0 card ref = (%d, %q), want (42, \"Column A\")", entries[0].cardNumber, entries[0].columnTitle)
	}
	if entries[1].cardNumber != 0 {
		t.Errorf("entry 1 (window 999-research, no board card) cardNumber = %d, want 0", entries[1].cardNumber)
	}
	if entries[2].cardNumber != 0 {
		t.Errorf("entry 2 (non-numbered window) cardNumber = %d, want 0", entries[2].cardNumber)
	}
}

func TestAgentList_Entries_NumberBoundaryDoesNotCrossMatch(t *testing.T) {
	fe := &action.FakeExecutor{}
	// Window 420-x must not join card #42.
	b := newAgentListBoard(t, fe, []cenciwatch.WindowState{
		{Session: "dev", WindowIndex: "1", WindowName: "420-other", Status: "idle"},
	})

	entries := b.agentListEntries()

	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].cardNumber != 0 {
		t.Errorf("window 420-other joined card #%d, want no join", entries[0].cardNumber)
	}
}

func TestAgentList_Entries_NonCanonicalNumberDoesNotJoin(t *testing.T) {
	fe := &action.FakeExecutor{}
	// agentStatusForNumber matches window names by exact string ("42" or
	// "42-" prefix), so spellings Atoi would normalize must not join here
	// either — otherwise the modal and the card badge disagree.
	b := newAgentListBoard(t, fe, []cenciwatch.WindowState{
		{Session: "dev", WindowIndex: "1", WindowName: "042-implement", Status: "idle"},
		{Session: "dev", WindowIndex: "2", WindowName: "+42-implement", Status: "idle"},
	})

	entries := b.agentListEntries()

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(entries))
	}
	for i, e := range entries {
		if e.cardNumber != 0 {
			t.Errorf("entry %d (%q) joined card #%d, want no join", i, e.window.WindowName, e.cardNumber)
		}
	}
}

// --- Session scoping: the agents list is scoped to the lazyboards instance's
// own tmux session (#410). ---

func TestAgentList_Entries_ScopedToInstanceSession(t *testing.T) {
	fe := &action.FakeExecutor{}
	// threeWindows spans "dev" (dev:3, dev:5) and "ops" (ops:1).
	b := newAgentListBoard(t, fe, threeWindows())
	b.tmuxSession = "dev"

	entries := b.agentListEntries()

	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2 (only the dev-session windows)", len(entries))
	}
	for i, e := range entries {
		if e.window.Session != "dev" {
			t.Errorf("entry %d session = %q, want dev (out-of-session window leaked)", i, e.window.Session)
		}
	}
}

func TestAgentList_Entries_UnknownInstanceSessionShowsAll(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())
	b.tmuxSession = "" // not running inside tmux: nothing to scope to

	if got := len(b.agentListEntries()); got != 3 {
		t.Fatalf("entries with unknown instance session = %d, want 3 (all windows)", got)
	}
}

// TestAgentCounts_MatchesAgentListEntries_AcrossSessions verifies the
// status-bar agent tally (agentCounts) and the agents modal (agentListEntries)
// agree on population scope for a snapshot spanning multiple tmux sessions:
// summing agentCounts' six per-status tallies must equal the modal's entry
// count, and out-of-session windows (here carrying done/stopped statuses,
// distinct from the in-session statuses) must not leak into either (#420).
func TestAgentCounts_MatchesAgentListEntries_AcrossSessions(t *testing.T) {
	fe := &action.FakeExecutor{}
	windows := []cenciwatch.WindowState{
		{Session: "dev", WindowIndex: "1", WindowName: "42-implement", Status: agentStatusRunning},
		{Session: "dev", WindowIndex: "2", WindowName: "999-research", Status: "idle"},
		{Session: "dev", WindowIndex: "3", WindowName: "7-fix", Status: agentStatusNeedInput},
		{Session: "ops", WindowIndex: "1", WindowName: "scratch", Status: "done"},
		{Session: "ops", WindowIndex: "2", WindowName: "other", Status: "stopped"},
	}
	b := newAgentListBoard(t, fe, windows)
	b.tmuxSession = "dev"

	entries := b.agentListEntries()
	running, needInput, done, failed, stopped, idle := b.agentCounts()
	total := running + needInput + done + failed + stopped + idle

	if len(entries) != 3 {
		t.Fatalf("agentListEntries() = %d entries, want 3 (dev-session windows only)", len(entries))
	}
	if total != len(entries) {
		t.Errorf("agentCounts total = %d, want %d to match agentListEntries() (same session scope)", total, len(entries))
	}
	if running != 1 || idle != 1 || needInput != 1 {
		t.Errorf("agentCounts() dev-session tallies = (running=%d, idle=%d, needInput=%d), want (1, 1, 1)", running, idle, needInput)
	}
	if done != 0 || stopped != 0 || failed != 0 {
		t.Errorf("agentCounts() done/stopped/failed = (%d, %d, %d), want (0, 0, 0): the ops-session windows must not leak into the dev-scoped tally", done, stopped, failed)
	}
}

func TestAgentList_View_ShowsSessionIndexPrefix(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())
	b.tmuxSession = "dev"
	b = sendKey(t, b, keyMsg("w"))

	view := b.View()

	if !strings.Contains(view, "dev:3") {
		t.Errorf("view missing the session:index prefix %q", "dev:3")
	}
	if strings.Contains(view, "scratch") || strings.Contains(view, "ops:1") {
		t.Errorf("view leaked an out-of-session window (ops:1 scratch)")
	}
}

// TestNormalMode_S_OutOfSessionAgent_NotJumped: the s jump is scoped to the
// instance's session too, so a card whose only agent runs in another tmux
// session reports no windows rather than jumping across sessions.
func TestNormalMode_S_OutOfSessionAgent_NotJumped(t *testing.T) {
	fe := &action.FakeExecutor{}
	// Card #42's only agent runs in "ops"; this instance is "dev".
	b := newAgentListBoard(t, fe, []cenciwatch.WindowState{
		{Session: "ops", WindowIndex: "2", WindowName: "42-implement", Status: agentStatusRunning},
	})
	b.tmuxSession = "dev"

	m, cmd := b.Update(keyMsg("s")) // card #42 selected (cursor 0)
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (no in-session window to jump to)", b.mode)
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShell called %d times for an out-of-session agent, want 0", len(fe.RunShellCalls))
	}
	if !strings.Contains(b.statusBar.View(200, 0, 0), "No agent windows for #42") {
		t.Errorf("status = %q, want the no-windows-for-card message", b.statusBar.View(200, 0, 0))
	}
}

// --- View ---

func TestAgentList_View_ShowsWindowsWithCardRefs(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())
	b = sendKey(t, b, keyMsg("w"))

	view := b.View()

	for _, want := range []string{"42-implement", "999-research", "scratch", "Column A #42"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

// TestAgentList_View_StatePrecedence locks the full cenciwatch state
// precedence: watcher disabled -> daemon not connected -> no windows ->
// list (with a stale note once the error threshold that clears the
// status-bar dispatch segment is reached).
func TestAgentList_View_StatePrecedence(t *testing.T) {
	open := func(t *testing.T, mutate func(*Board)) Board {
		fe := &action.FakeExecutor{}
		b := newAgentListBoard(t, fe, threeWindows())
		mutate(&b)
		return sendKey(t, b, keyMsg("w"))
	}

	t.Run("watcher disabled", func(t *testing.T) {
		b := open(t, func(b *Board) { b.cenciWatcher = nil; b.agentSnapshot = nil })
		if view := b.View(); !strings.Contains(view, "cenci-watch is not enabled") {
			t.Errorf("view = %q, want the watcher-disabled message", view)
		}
	})

	t.Run("daemon not connected", func(t *testing.T) {
		b := open(t, func(b *Board) { b.agentSnapshot = nil })
		if view := b.View(); !strings.Contains(view, "Waiting for cenci-watch daemon") {
			t.Errorf("view = %q, want the not-connected message", view)
		}
	})

	t.Run("no agent windows", func(t *testing.T) {
		b := open(t, func(b *Board) { b.agentSnapshot = &cenciwatch.StateSnapshot{} })
		if view := b.View(); !strings.Contains(view, "No agent windows") {
			t.Errorf("view = %q, want the empty message", view)
		}
	})

	t.Run("healthy list has no stale note", func(t *testing.T) {
		b := open(t, func(b *Board) {})
		if view := b.View(); strings.Contains(view, "disconnected") {
			t.Errorf("healthy view unexpectedly shows the disconnected note")
		}
	})

	t.Run("disconnected keeps last known list with note", func(t *testing.T) {
		b := open(t, func(b *Board) { b.cenciWatchConsecutiveErrors = cenciWatchClearThreshold })
		view := b.View()
		if !strings.Contains(view, "42-implement") {
			t.Errorf("disconnected view dropped the last known windows")
		}
		if !strings.Contains(view, "disconnected") {
			t.Errorf("disconnected view missing the stale note")
		}
	})
}

func TestAgentList_View_KeepsSelectedRowVisibleWithinTerminal(t *testing.T) {
	fe := &action.FakeExecutor{}
	windows := make([]cenciwatch.WindowState, 0, 40)
	for i := 0; i < 40; i++ {
		windows = append(windows, cenciwatch.WindowState{
			Session:     "dev",
			WindowIndex: "1",
			WindowName:  "w" + strings.Repeat("x", i%3) + "-" + string(rune('a'+i%26)),
			Status:      "idle",
		})
	}
	windows[0].WindowName = "first-window"
	windows[39].WindowName = "last-window"
	b := newAgentListBoard(t, fe, windows)
	b.Height = 20
	b = sendKey(t, b, keyMsg("w"))
	b.agentList.cursor = 39

	view := b.View()

	if !strings.Contains(view, "last-window") {
		t.Errorf("selected bottom row not visible in a short terminal")
	}
	if strings.Contains(view, "first-window") {
		t.Errorf("top row still visible; expected it scrolled out with ▲ indicator")
	}
	if !strings.Contains(view, "▲") {
		t.Errorf("view missing the ▲ scroll indicator")
	}
}

// --- Navigation ---

func TestAgentList_Navigation_MovesAndClampsCursor(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())
	b = sendKey(t, b, keyMsg("w"))

	for _, k := range []string{"j", "j", "j", "j"} {
		b = sendKey(t, b, keyMsg(k))
	}
	if b.agentList.cursor != 2 {
		t.Errorf("cursor after walking past bottom = %d, want 2", b.agentList.cursor)
	}

	for _, k := range []string{"k", "k", "k", "k"} {
		b = sendKey(t, b, keyMsg(k))
	}
	if b.agentList.cursor != 0 {
		t.Errorf("cursor after walking past top = %d, want 0", b.agentList.cursor)
	}
}

// --- Enter: switch to the agent's tmux window ---

func TestAgentList_Enter_SwitchesToSelectedWindow(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())
	b = sendKey(t, b, keyMsg("w"))
	b = sendKey(t, b, keyMsg("j")) // select 999-research (dev:5)

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.RunShellCalls) != 1 {
		t.Fatalf("RunShell called %d times, want 1", len(fe.RunShellCalls))
	}
	want := "tmux select-window -t 'dev:5' && tmux switch-client -t 'dev'"
	if fe.RunShellCalls[0] != want {
		t.Errorf("RunShell = %q, want %q", fe.RunShellCalls[0], want)
	}
	if !strings.Contains(b.statusBar.View(200, 0, 0), "999-research") {
		t.Errorf("status = %q, want it to mention the window", b.statusBar.View(200, 0, 0))
	}
}

func TestAgentList_Enter_EscapesShellMetacharacters(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, []cenciwatch.WindowState{
		{Session: "de'v; rm -rf /", WindowIndex: "2", WindowName: "evil", Status: "idle"},
	})
	b = sendKey(t, b, keyMsg("w"))

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.RunShellCalls) != 1 {
		t.Fatalf("RunShell called %d times, want 1", len(fe.RunShellCalls))
	}
	want := `tmux select-window -t 'de'\''v; rm -rf /:2' && tmux switch-client -t 'de'\''v; rm -rf /'`
	if fe.RunShellCalls[0] != want {
		t.Errorf("RunShell = %q, want %q", fe.RunShellCalls[0], want)
	}
}

func TestAgentList_Enter_EmptyList_NoShellCall(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, nil)
	b = sendKey(t, b, keyMsg("w"))

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter on empty list = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShell called %d times on an empty list, want 0", len(fe.RunShellCalls))
	}
}

func TestAgentList_Enter_ShellError_ShowsStderr(t *testing.T) {
	fe := &action.FakeExecutor{
		RunShellErr:    errors.New("exit status 1"),
		RunShellStderr: "no current client\n",
	}
	b := newAgentListBoard(t, fe, threeWindows())
	b = sendKey(t, b, keyMsg("w"))

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	status := b.statusBar.View(200, 0, 0)
	if !strings.Contains(status, "no current client") {
		t.Errorf("status = %q, want it to surface tmux's stderr", status)
	}
}

// --- Live snapshot updates while the modal is open ---

func TestAgentList_SnapshotUpdate_ClampsCursorWhileOpen(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())
	b = sendKey(t, b, keyMsg("w"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j")) // cursor on the last of 3 rows

	shrunk := &cenciwatch.StateSnapshot{Windows: threeWindows()[:1]}
	m, cmd := b.Update(agentSnapshotMsg{snapshot: shrunk})
	b = m.(Board)

	if cmd == nil {
		t.Errorf("agentSnapshotMsg with a live watcher must re-subscribe; got nil cmd")
	}
	if b.agentList.cursor != 0 {
		t.Errorf("cursor after list shrank to 1 = %d, want 0", b.agentList.cursor)
	}
	if b.mode != agentListMode {
		t.Errorf("mode after snapshot update = %d, want agentListMode (%d)", b.mode, agentListMode)
	}
	// The clamped view must render without panicking.
	if view := b.View(); !strings.Contains(view, "42-implement") {
		t.Errorf("view after shrink missing the remaining window")
	}
}

// TestAgentList_SnapshotUpdate_ShrinkToZero_SwitchesToEmptyHints: when every
// window of an open (card-scoped) modal closes, the view falls to its "No
// agent windows" branch, so the hints must stop advertising enter/j/k
// (docs/view-state-consistency.md).
func TestAgentList_SnapshotUpdate_ShrinkToZero_SwitchesToEmptyHints(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, cardJumpWindows())
	b = sendKey(t, b, keyMsg("s")) // card #42 scoped: two windows

	// Both of #42's windows close; only another card's window survives.
	m, cmd := b.Update(agentSnapshotMsg{snapshot: &cenciwatch.StateSnapshot{Windows: []cenciwatch.WindowState{
		{Session: "dev", WindowIndex: "4", WindowName: "7-fix", Status: agentStatusRunning},
	}}})
	b = m.(Board)
	if cmd == nil {
		t.Errorf("agentSnapshotMsg with a live watcher must re-subscribe; got nil cmd")
	}

	if !strings.Contains(b.View(), agentListMsgNoWindows) {
		t.Errorf("view after shrink-to-zero missing the empty message")
	}
	status := b.statusBar.View(200, 0, 0)
	if strings.Contains(status, "Go to window") {
		t.Errorf("hints = %q, still advertising enter on an empty modal", status)
	}
	if !strings.Contains(status, "Cancel") {
		t.Errorf("hints = %q, want esc/Cancel to remain", status)
	}
}

func TestAgentList_SnapshotUpdate_GrowFromZero_RestoresHints(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, nil)
	b = sendKey(t, b, keyMsg("w")) // opens empty: esc-only hints

	m, cmd := b.Update(agentSnapshotMsg{snapshot: &cenciwatch.StateSnapshot{Windows: threeWindows()}})
	b = m.(Board)
	if cmd == nil {
		t.Errorf("agentSnapshotMsg with a live watcher must re-subscribe; got nil cmd")
	}

	status := b.statusBar.View(200, 0, 0)
	if !strings.Contains(status, "Go to window") {
		t.Errorf("hints = %q, want enter hint restored once rows exist", status)
	}
}

// --- Closing ---

func TestAgentList_Esc_ReturnsToNormal(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())
	b = sendKey(t, b, keyMsg("w"))

	b = sendKey(t, b, arrowMsg(tea.KeyEscape))

	if b.mode != normalMode {
		t.Errorf("mode after esc = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

// --- Card-scoped jump (s) ---

// cardJumpWindows: card #42 has two agent windows, card #7 has one.
func cardJumpWindows() []cenciwatch.WindowState {
	return []cenciwatch.WindowState{
		{Session: "dev", WindowIndex: "1", WindowName: "42-plan", Status: "done"},
		{Session: "dev", WindowIndex: "2", WindowName: "42-implement", Status: agentStatusRunning, Agent: "claude"},
		{Session: "dev", WindowIndex: "4", WindowName: "7-fix", Status: agentStatusRunning},
	}
}

func TestNormalMode_S_SingleWindow_SwitchesDirectly(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, cardJumpWindows())
	b = sendKey(t, b, keyMsg("j")) // select card #7 (one window: 7-fix)

	m, cmd := b.Update(keyMsg("s"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after single-window jump = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.RunShellCalls) != 1 {
		t.Fatalf("RunShell called %d times, want 1", len(fe.RunShellCalls))
	}
	want := "tmux select-window -t 'dev:4' && tmux switch-client -t 'dev'"
	if fe.RunShellCalls[0] != want {
		t.Errorf("RunShell = %q, want %q", fe.RunShellCalls[0], want)
	}
	if !strings.Contains(b.statusBar.View(200, 0, 0), "7-fix") {
		t.Errorf("status = %q, want it to mention the window", b.statusBar.View(200, 0, 0))
	}
}

func TestNormalMode_S_MultipleWindows_OpensModalFilteredToCard(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, cardJumpWindows())

	m, cmd := b.Update(keyMsg("s")) // card #42 selected: two windows
	b = m.(Board)
	execCmds(cmd)

	if b.mode != agentListMode {
		t.Fatalf("mode after multi-window jump = %d, want agentListMode (%d)", b.mode, agentListMode)
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShell called %d times before a window was chosen, want 0", len(fe.RunShellCalls))
	}
	entries := b.agentListEntries()
	if len(entries) != 2 {
		t.Fatalf("filtered entries = %d, want 2 (only #42's windows)", len(entries))
	}
	for i, e := range entries {
		if e.cardNumber != 42 {
			t.Errorf("entry %d joined card #%d, want 42", i, e.cardNumber)
		}
	}
	view := b.View()
	if !strings.Contains(view, "Agents — #42") {
		t.Errorf("view missing the card-scoped title, got:\n%s", view)
	}
	if strings.Contains(view, "7-fix") {
		t.Errorf("card-scoped view leaked another card's window")
	}
}

func TestNormalMode_S_FilteredModal_EnterSwitches(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, cardJumpWindows())
	b = sendKey(t, b, keyMsg("s"))
	b = sendKey(t, b, keyMsg("j")) // select 42-implement (dev:2)

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.RunShellCalls) != 1 {
		t.Fatalf("RunShell called %d times, want 1", len(fe.RunShellCalls))
	}
	want := "tmux select-window -t 'dev:2' && tmux switch-client -t 'dev'"
	if fe.RunShellCalls[0] != want {
		t.Errorf("RunShell = %q, want %q", fe.RunShellCalls[0], want)
	}
}

func TestNormalMode_S_NoWindows_ShowsStatusMessage(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, []cenciwatch.WindowState{
		{Session: "dev", WindowIndex: "9", WindowName: "999-other", Status: "idle"},
	})

	m, cmd := b.Update(keyMsg("s")) // card #42 selected: no windows
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after no-window jump = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShell called %d times, want 0", len(fe.RunShellCalls))
	}
	if !strings.Contains(b.statusBar.View(200, 0, 0), "No agent windows") {
		t.Errorf("status = %q, want a no-agent-windows message", b.statusBar.View(200, 0, 0))
	}
}

// TestNormalMode_S_StatePrecedence locks the full cenciwatch state precedence
// for the jump's zero-window branch: "no windows for this card" must only be
// claimed when a daemon snapshot is actually connected.
func TestNormalMode_S_StatePrecedence(t *testing.T) {
	press := func(t *testing.T, mutate func(*Board)) Board {
		t.Helper()
		fe := &action.FakeExecutor{}
		b := newAgentListBoard(t, fe, nil)
		mutate(&b)
		m, cmd := b.Update(keyMsg("s"))
		updated := m.(Board)
		execCmds(cmd)
		return updated
	}

	t.Run("watcher disabled", func(t *testing.T) {
		b := press(t, func(b *Board) { b.cenciWatcher = nil; b.agentSnapshot = nil })
		if status := b.statusBar.View(200, 0, 0); !strings.Contains(status, "not enabled") {
			t.Errorf("status = %q, want the watcher-disabled message", status)
		}
	})

	t.Run("daemon not connected", func(t *testing.T) {
		b := press(t, func(b *Board) { b.agentSnapshot = nil })
		if status := b.statusBar.View(200, 0, 0); !strings.Contains(status, "Waiting for cenci-watch") {
			t.Errorf("status = %q, want the not-connected message", status)
		}
	})

	t.Run("connected with no matching windows", func(t *testing.T) {
		b := press(t, func(b *Board) {})
		if status := b.statusBar.View(200, 0, 0); !strings.Contains(status, "No agent windows for #42") {
			t.Errorf("status = %q, want the no-windows-for-card message", status)
		}
	})
}

// TestNormalMode_S_RespectsActiveSearch locks the cursor-invariant rule: with
// a search active the cursor indexes the filtered list, so s must act on the
// filtered selection (via selectedCard), not the raw column card at the same
// index.
func TestNormalMode_S_RespectsActiveSearch(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, cardJumpWindows())
	b.searchQuery = "card b" // only card #7 visible; cursor 0 now means #7

	m, cmd := b.Update(keyMsg("s"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 1 {
		t.Fatalf("RunShell called %d times, want 1", len(fe.RunShellCalls))
	}
	want := "tmux select-window -t 'dev:4' && tmux switch-client -t 'dev'"
	if fe.RunShellCalls[0] != want {
		t.Errorf("RunShell = %q, want %q (card #7's window, not card #42's)", fe.RunShellCalls[0], want)
	}
}

// TestNormalMode_W_AfterCardScopedOpen_ListsAll guards the reset: a global
// open after a card-scoped one must not inherit the card filter.
func TestNormalMode_W_AfterCardScopedOpen_ListsAll(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, cardJumpWindows())
	b = sendKey(t, b, keyMsg("s")) // card-scoped (#42)
	b = sendKey(t, b, arrowMsg(tea.KeyEscape))

	b = sendKey(t, b, keyMsg("w"))

	if got := len(b.agentListEntries()); got != 3 {
		t.Errorf("global entries after a card-scoped open = %d, want 3", got)
	}
}

// --- Help ---

func TestHelp_ListsAgentListKeybindings(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())

	content := b.buildHelpContent()

	agentsIdx := strings.Index(content, "\nAgents (cenci)\n")
	if agentsIdx == -1 {
		t.Fatal("buildHelpContent() should contain an 'Agents' section header")
	}
	sectionContent := content[agentsIdx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}
	for _, want := range []string{"w", "Go to tmux window", "Navigate", "Cancel", "Go to agent"} {
		if !strings.Contains(sectionContent, want) {
			t.Errorf("Agents help section missing %q", want)
		}
	}
}
