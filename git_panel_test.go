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

// gitPanelKeyOrder is the fixed display/dispatch order of the git menu's
// built-in shortcuts (lazygit-style keys): Push, Pull, Fetch, Mergetool,
// Stash push, Stash pop. This must hold regardless of Go map iteration order
// over defaultActions.
var gitPanelKeyOrder = []string{"P", "p", "f", "m", "s", "S"}

// newGitPanelTestBoard creates a loaded Board seeded with the built-in git
// default actions (config.DefaultGitActions()) plus any user-provided
// overrides, a FakeExecutor for asserting dispatched shell commands, and an
// optional git status reader (nil disables the git status feature).
func newGitPanelTestBoard(t *testing.T, userActions map[string]config.Action, reader gitdetect.Reader) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, userActions, config.DefaultGitActions(), nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, reader, true)

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
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil, true)
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
				t.Fatalf("iteration %d: gitPanel.items[%d].key = %q, want %q (fixed order: Push, Pull, Fetch, Mergetool, Stash push, Stash pop)", i, j, b.gitPanel.items[j].key, wantKey)
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

func TestGitPanel_CursorWrapsAtBounds(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}
	lastIndex := len(b.gitPanel.items) - 1

	// k at the top wraps to the last item.
	b = sendKey(t, b, keyMsg("k"))
	if b.gitPanel.cursor != lastIndex {
		t.Errorf("cursor after k at top = %d, want %d (wrap to last)", b.gitPanel.cursor, lastIndex)
	}

	// j from the last item wraps back to the first.
	b = sendKey(t, b, keyMsg("j"))
	if b.gitPanel.cursor != 0 {
		t.Errorf("cursor after j at bottom = %d, want 0 (wrap to first)", b.gitPanel.cursor)
	}

	// A full round trip stays in bounds throughout.
	for i := 0; i < 20; i++ {
		b = sendKey(t, b, keyMsg("k"))
		if b.gitPanel.cursor < 0 || b.gitPanel.cursor > lastIndex {
			t.Fatalf("iteration %d: cursor out of bounds: %d (want [0, %d])", i, b.gitPanel.cursor, lastIndex)
		}
	}
	for i := 0; i < 20; i++ {
		b = sendKey(t, b, keyMsg("j"))
		if b.gitPanel.cursor < 0 || b.gitPanel.cursor > lastIndex {
			t.Fatalf("iteration %d: cursor out of bounds: %d (want [0, %d])", i, b.gitPanel.cursor, lastIndex)
		}
	}
}

// TestGitPanel_Navigation_ArrowKeys_WrapsCursor confirms Up/Down arrow keys
// wrap identically to j/k: both route through the shared moveCursor helper.
func TestGitPanel_Navigation_ArrowKeys_WrapsCursor(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)
	b = sendKey(t, b, keyMsg("g"))
	lastIndex := len(b.gitPanel.items) - 1

	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	if b.gitPanel.cursor != lastIndex {
		t.Errorf("cursor after Up at top = %d, want %d (wrap to last)", b.gitPanel.cursor, lastIndex)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if b.gitPanel.cursor != 0 {
		t.Errorf("cursor after Down at bottom = %d, want 0 (wrap to first)", b.gitPanel.cursor)
	}
}

// TestGitPanel_SingleItem_NavigationIsNoOp covers the length<=1 guard for one
// of the four modal list handlers (docs/list-cursor-invariants.md): with a
// single default git action registered, j/k must never move the cursor off 0
// and must not panic.
func TestGitPanel_SingleItem_NavigationIsNoOp(t *testing.T) {
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	singleAction := map[string]config.Action{
		"P": {Name: "Push", Type: "shell", Command: "git push", Scope: "board"},
	}
	b := NewBoard(p, nil, singleAction, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil, true)
	m, _ := b.Update(boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{Title: "Empty", Cards: nil}},
	}})
	loaded := m.(Board)
	loaded.Width = 120
	loaded.Height = 40

	loaded = sendKey(t, loaded, keyMsg("g"))
	if loaded.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", loaded.mode)
	}
	if len(loaded.gitPanel.items) != 1 {
		t.Fatalf("len(gitPanel.items) = %d, want 1", len(loaded.gitPanel.items))
	}

	loaded = sendKey(t, loaded, keyMsg("j"))
	if loaded.gitPanel.cursor != 0 {
		t.Errorf("cursor after j on single-item list = %d, want 0 (no-op)", loaded.gitPanel.cursor)
	}
	loaded = sendKey(t, loaded, keyMsg("k"))
	if loaded.gitPanel.cursor != 0 {
		t.Errorf("cursor after k on single-item list = %d, want 0 (no-op)", loaded.gitPanel.cursor)
	}
}

// --- Dispatch ---

func TestGitPanel_Enter_DefaultKey_DispatchesBuiltinAction(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	idx := gitPanelItemIndex(b, "f")
	if idx == -1 {
		t.Fatal("expected a Fetch (key f) entry in the git panel items")
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

func TestGitPanel_MenuKeysAreScopedFromUserActions(t *testing.T) {
	// A normal-mode custom action on the same letter must not shadow the git
	// menu entry: menu keys dispatch from defaultActions, not resolveAction.
	userActions := map[string]config.Action{
		"S": {Name: "Custom S", Type: "shell", Command: "echo custom-s", Scope: "board"},
	}
	b, fe := newGitPanelTestBoard(t, userActions, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	idx := gitPanelItemIndex(b, "S")
	if idx == -1 {
		t.Fatal("expected a Stash pop (key S) entry in the git panel items")
	}
	b.gitPanel.cursor = idx

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("pressing Enter on a git panel item should return a non-nil cmd")
	}
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called for the Stash pop entry, got no calls")
	}
	if fe.RunShellCalls[0] != "git stash pop" {
		t.Errorf("RunShellCalls[0] = %q, want the built-in %q; a user action on the same letter must not shadow the menu entry", fe.RunShellCalls[0], "git stash pop")
	}
}

// --- Direct key dispatch (lazygit-style) ---

func TestGitPanel_DirectKey_DispatchesAndClosesPanel(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	m, cmd := b.Update(keyMsg("P"))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("pressing 'P' in the git menu should return a non-nil cmd")
	}
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("after direct key dispatch: mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != "git push" {
		t.Errorf("RunShellCalls = %v, want first call to be %q", fe.RunShellCalls, "git push")
	}
}

func TestGitPanel_DirectKey_LowercasePull(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	m, cmd := b.Update(keyMsg("p"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != "git pull --rebase" {
		t.Errorf("RunShellCalls = %v, want first call to be %q", fe.RunShellCalls, "git pull --rebase")
	}
	if b.mode != normalMode {
		t.Errorf("after direct key dispatch: mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

func TestGitPanel_UnboundKey_IsIgnored(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	b = sendKey(t, b, keyMsg("z"))

	if b.mode != gitPanelMode {
		t.Errorf("after unbound key 'z': mode = %d, want gitPanelMode (%d)", b.mode, gitPanelMode)
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("unbound key must not dispatch, got RunShell calls: %v", fe.RunShellCalls)
	}
}

func TestGitPanel_JK_NavigateWithoutDispatching(t *testing.T) {
	// j/k must stay pure navigation even though the menu dispatches on bare
	// letters — they are not menu keys.
	b, fe := newGitPanelTestBoard(t, nil, nil)

	b = sendKey(t, b, keyMsg("g"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("k"))

	if b.mode != gitPanelMode {
		t.Errorf("after j/k: mode = %d, want gitPanelMode (%d)", b.mode, gitPanelMode)
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("j/k must not dispatch, got RunShell calls: %v", fe.RunShellCalls)
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
