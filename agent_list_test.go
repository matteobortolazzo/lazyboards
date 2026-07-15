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

// --- Help ---

func TestHelp_ListsAgentListKeybindings(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())

	content := b.buildHelpContent()

	agentsIdx := strings.Index(content, "\nAgents\n")
	if agentsIdx == -1 {
		t.Fatal("buildHelpContent() should contain an 'Agents' section header")
	}
	sectionContent := content[agentsIdx:]
	if nextSection := strings.Index(sectionContent[1:], "\n\n"); nextSection != -1 {
		sectionContent = sectionContent[:nextSection+1]
	}
	for _, want := range []string{"w", "Go to tmux window", "Navigate", "Cancel"} {
		if !strings.Contains(sectionContent, want) {
			t.Errorf("Agents help section missing %q", want)
		}
	}
}
