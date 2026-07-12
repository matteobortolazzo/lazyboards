package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// gitPanelKeyOrder is the fixed display/dispatch order of the git panel's
// built-in shortcuts, per the approved plan: Push, Pull, Mergetool, Fetch,
// Stash push, Stash pop. This must hold regardless of Go map iteration order
// over defaultActions.
var gitPanelKeyOrder = []string{"P", "L", "M", "F", "S", "X"}

// newGitPanelTestBoard creates a loaded Board seeded with the built-in git
// default actions (config.DefaultGitActions()) plus any user-provided
// overrides, a FakeExecutor for asserting dispatched shell commands, and an
// optional git status reader (nil disables the git status feature).
func newGitPanelTestBoard(t *testing.T, userActions map[string]config.Action, reader gitdetect.Reader) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, userActions, config.DefaultGitActions(), nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, reader)

	// Load a board with an empty column so board-scope actions (and the git
	// panel, which is board-scope with no active-card requirement) can dispatch.
	m, _ := b.Update(boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{Title: "Empty", Cards: nil}},
	}})
	loaded := m.(Board)
	loaded.Width = 120
	loaded.Height = 40
	return loaded, fe
}

// gitPanelItemIndex returns the index of the item with the given key in
// b.gitPanel.items, or -1 if not found.
func gitPanelItemIndex(b Board, key string) int {
	for i, item := range b.gitPanel.items {
		if item.key == key {
			return i
		}
	}
	return -1
}

// --- Mode transition tests ---

func TestGitPanel_PressG_OpensPanel_WhenDefaultActionsPresent(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))

	if b.mode != gitPanelMode {
		t.Errorf("after pressing 'g' with default git actions available: mode = %d, want gitPanelMode (%d)", b.mode, gitPanelMode)
	}
}

func TestGitPanel_PressG_Noop_WhenNoDefaultActions(t *testing.T) {
	// Simulate being outside a git repo: defaultActions is empty/nil.
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil)
	m, _ := b.Update(boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{Title: "Empty", Cards: nil}},
	}})
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("g"))

	if b.mode != normalMode {
		t.Errorf("after pressing 'g' with no default git actions: mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

func TestGitPanel_Escape_ReturnsToNormalMode(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Escape in gitPanelMode: mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

func TestGitPanel_Escape_RestoresNormalHints(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	view := b.View()
	if !strings.Contains(view, "Edit") {
		t.Error("after Escape from gitPanelMode, normal hint 'Edit' should be visible")
	}
}

// --- Ordered items ---

func TestGitPanel_ItemsFixedOrder_RegardlessOfMapIteration(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	// Reopen the panel many times: Go randomizes map iteration order on each
	// range, so repeating this within a single test run will surface a
	// naive "range defaultActions" implementation that doesn't sort/fix order.
	for i := 0; i < 20; i++ {
		b = sendKey(t, b, keyMsg("g"))
		if b.mode != gitPanelMode {
			t.Fatalf("iteration %d: expected gitPanelMode after 'g', got %d", i, b.mode)
		}
		if len(b.gitPanel.items) != len(gitPanelKeyOrder) {
			t.Fatalf("iteration %d: len(gitPanel.items) = %d, want %d", i, len(b.gitPanel.items), len(gitPanelKeyOrder))
		}
		for j, wantKey := range gitPanelKeyOrder {
			if b.gitPanel.items[j].key != wantKey {
				t.Fatalf("iteration %d: gitPanel.items[%d].key = %q, want %q (fixed order: Push, Pull, Mergetool, Fetch, Stash push, Stash pop)", i, j, b.gitPanel.items[j].key, wantKey)
			}
		}
		b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	}
}

// --- Navigation ---

func TestGitPanel_JK_Navigation(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	initialCursor := b.gitPanel.cursor

	b = sendKey(t, b, keyMsg("j"))
	if b.gitPanel.cursor <= initialCursor {
		t.Errorf("after 'j': gitPanel.cursor = %d, want > %d", b.gitPanel.cursor, initialCursor)
	}

	afterDown := b.gitPanel.cursor

	b = sendKey(t, b, keyMsg("k"))
	if b.gitPanel.cursor >= afterDown {
		t.Errorf("after 'k': gitPanel.cursor = %d, want < %d", b.gitPanel.cursor, afterDown)
	}
}

func TestGitPanel_CursorClampsAtBounds(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	for i := 0; i < 20; i++ {
		b = sendKey(t, b, keyMsg("k"))
	}
	if b.gitPanel.cursor < 0 {
		t.Errorf("cursor went below 0: %d", b.gitPanel.cursor)
	}

	for i := 0; i < 20; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.gitPanel.cursor >= len(b.gitPanel.items) {
		t.Errorf("cursor went past items: cursor=%d, len=%d", b.gitPanel.cursor, len(b.gitPanel.items))
	}
}

// --- Dispatch ---

func TestGitPanel_Enter_DefaultKey_DispatchesBuiltinAction(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	idx := gitPanelItemIndex(b, "F")
	if idx == -1 {
		t.Fatal("expected a Fetch (key F) entry in the git panel items")
	}
	b.gitPanel.cursor = idx

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("pressing Enter on a git panel item should return a non-nil cmd")
	}
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called for the Fetch entry, got no calls")
	}
	if fe.RunShellCalls[0] != "git fetch" {
		t.Errorf("RunShellCalls[0] = %q, want %q", fe.RunShellCalls[0], "git fetch")
	}
}

func TestGitPanel_Enter_UserOverride_DispatchesOverriddenAction(t *testing.T) {
	userActions := map[string]config.Action{
		"S": {Name: "Custom Stash", Type: "shell", Command: "echo custom-stash", Scope: "board"},
	}
	b, fe := newGitPanelTestBoard(t, userActions, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	idx := gitPanelItemIndex(b, "S")
	if idx == -1 {
		t.Fatal("expected a Stash push (key S) entry in the git panel items")
	}
	b.gitPanel.cursor = idx

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("pressing Enter on a git panel item should return a non-nil cmd")
	}
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called for the overridden Stash push entry, got no calls")
	}
	if fe.RunShellCalls[0] != "echo custom-stash" {
		t.Errorf("RunShellCalls[0] = %q, want the user-overridden command %q (not the built-in %q); Enter must dispatch via resolveAction so overrides win", fe.RunShellCalls[0], "echo custom-stash", "git stash push")
	}
}

func TestGitPanel_Enter_ClosesPanel(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("after Enter in gitPanelMode: mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

// --- Refresh wiring ---
//
// The git panel must dispatch through the same resolveAction/handleBoardActionKey
// path used by ordinary board-scope hotkeys, so that a successful action
// re-triggers the existing broad git-status refresh (see gitstatus_wiring_test.go's
// TestUpdate_ActionResultMsg_Success_RefreshesGitStatus) without any
// git-panel-specific duplicate wiring.
func TestGitPanel_Enter_SuccessfulAction_RefreshesGitStatusViaExistingWiring(t *testing.T) {
	reader := gitdetect.FakeReader{Status: gitdetect.Status{Branch: "main"}}
	b, fe := newGitPanelTestBoard(t, nil, reader)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	idx := gitPanelItemIndex(b, "P")
	if idx == -1 {
		t.Fatal("expected a Push (key P) entry in the git panel items")
	}
	b.gitPanel.cursor = idx

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("pressing Enter on a git panel item should return a non-nil cmd")
	}

	msgs := collectMsgs(cmd)
	var resultMsg *actionResultMsg
	for _, msg := range msgs {
		if am, ok := msg.(actionResultMsg); ok {
			resultMsg = &am
		}
	}
	if resultMsg == nil {
		t.Fatal("expected dispatching the git panel Push entry to eventually produce an actionResultMsg")
	}
	if !resultMsg.success {
		t.Fatalf("expected the Push action to succeed, got failure message %q", resultMsg.message)
	}
	if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != "git push" {
		t.Errorf("RunShellCalls = %v, want first call to be %q", fe.RunShellCalls, "git push")
	}

	// Feed the actionResultMsg back through the normal Update() dispatcher,
	// exercising the existing actionResultMsg-triggered refresh wiring.
	m2, cmd2 := b.Update(*resultMsg)
	b = m2.(Board)
	if cmd2 == nil {
		t.Fatal("actionResultMsg{success:true} should return a non-nil cmd (existing broad-refresh wiring)")
	}
	found := false
	for _, msg := range collectMsgs(cmd2) {
		if _, ok := msg.(gitStatusMsg); ok {
			found = true
		}
	}
	if !found {
		t.Error("a successful git panel action should trigger a git status refresh via the existing actionResultMsg wiring")
	}
}

// --- View rendering ---

func TestGitPanel_View_RendersModalWithItemNames(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	view := b.View()
	if view == "" {
		t.Fatal("View() returned empty string in gitPanelMode")
	}

	for _, key := range gitPanelKeyOrder {
		act, ok := b.defaultActions[key]
		if !ok {
			t.Fatalf("test setup: defaultActions missing key %q", key)
		}
		if !strings.Contains(view, act.Name) {
			t.Errorf("View() in gitPanelMode should contain the action name %q for key %q", act.Name, key)
		}
	}
}
