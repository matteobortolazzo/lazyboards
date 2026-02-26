package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// altKeyMsg builds a tea.KeyMsg for a single rune with the Alt modifier set.
func altKeyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key), Alt: true}
}

// --- Alt+key detection tests ---

func TestCommentMode_AltKeyEntersCommentMode(t *testing.T) {
	// An action whose template contains {comment} should enter commentMode when Alt is held.
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Press Alt+x — should enter commentMode because the action uses {comment}.
	b = sendKey(t, b, altKeyMsg("x"))

	if b.mode != commentMode {
		t.Errorf("after Alt+x on action with {comment}: mode = %d, want %d (commentMode)", b.mode, commentMode)
	}
}

func TestCommentMode_AltKeyWithoutComment_ExecutesNormally(t *testing.T) {
	// An action without {comment} should execute normally even with Alt held.
	actions := map[string]config.Action{
		"x": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Press Alt+x on an action without {comment} — should execute the action, not enter commentMode.
	b = sendKey(t, b, altKeyMsg("x"))

	if b.mode == commentMode {
		t.Error("Alt+key on action without {comment} should NOT enter commentMode")
	}
	if len(fe.OpenURLCalls) == 0 {
		t.Error("Alt+key on action without {comment} should execute the action normally")
	}
}

func TestCommentMode_RegularKeyOnCommentAction_ExecutesImmediately(t *testing.T) {
	// A regular key (no Alt) on an action with {comment} should execute immediately.
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Press x (no Alt) — should execute immediately, not enter commentMode.
	m, cmd := b.Update(keyMsg("x"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode == commentMode {
		t.Error("regular key on action with {comment} should NOT enter commentMode")
	}
}

func TestCommentMode_BoardScopeAction(t *testing.T) {
	// An Alt+key on a board-scope action with {comment} should set boardScope=true.
	actions := map[string]config.Action{
		"x": {Name: "Deploy Note", Type: "shell", Scope: "board", Command: "deploy --comment {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Press Alt+x on a board-scope action.
	b = sendKey(t, b, altKeyMsg("x"))

	if b.mode != commentMode {
		t.Errorf("after Alt+x on board-scope action with {comment}: mode = %d, want %d (commentMode)", b.mode, commentMode)
	}
	if !b.comment.boardScope {
		t.Error("board-scope action should set comment.boardScope = true")
	}
}

func TestCommentMode_StoresPendingAction(t *testing.T) {
	// commentState should store the correct pending action and card info.
	actionName := "Annotate"
	actions := map[string]config.Action{
		"x": {Name: actionName, Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Get the currently selected card before entering comment mode.
	col := b.Columns[b.ActiveTab]
	expectedCard := col.Cards[col.Cursor]

	// Press Alt+x.
	b = sendKey(t, b, altKeyMsg("x"))

	if b.comment.pendingAction.Name != actionName {
		t.Errorf("comment.pendingAction.Name = %q, want %q", b.comment.pendingAction.Name, actionName)
	}
	if b.comment.pendingCard.Number != expectedCard.Number {
		t.Errorf("comment.pendingCard.Number = %d, want %d", b.comment.pendingCard.Number, expectedCard.Number)
	}
	if b.comment.boardScope {
		t.Error("card-scope action should not set boardScope = true")
	}
}

// --- Modal UI tests ---

func TestCommentMode_ViewShowsModal(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)
	b.Width = 120
	b.Height = 40

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	view := b.View()
	if !strings.Contains(view, "Annotate") {
		t.Errorf("View() in commentMode should contain the action name %q", "Annotate")
	}
}

func TestCommentMode_ViewShowsHints(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)
	b.Width = 120
	b.Height = 40

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	view := b.View()
	if !strings.Contains(view, "Cancel") {
		t.Errorf("View() in commentMode should contain hint %q", "Cancel")
	}
	if !strings.Contains(view, "Submit") {
		t.Errorf("View() in commentMode should contain hint %q", "Submit")
	}
}

// --- Input handling tests ---

func TestCommentMode_Escape_ReturnsToNormalMode(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))
	if b.mode != commentMode {
		t.Fatalf("expected commentMode after Alt+x, got mode = %d", b.mode)
	}

	// Press Escape to cancel.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Escape in commentMode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestCommentMode_Enter_SubmitsAndReturnsToNormalMode(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	// Type a comment.
	for _, ch := range "my comment text" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to submit.
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("after Enter in commentMode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}

	// The shell command should have been called with the comment text.
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called after submitting comment, but no calls recorded")
	}
	// Verify the expanded command contains the comment.
	if !strings.Contains(fe.RunShellCalls[0], "my comment text") {
		t.Errorf("RunShell command = %q, want it to contain the comment text", fe.RunShellCalls[0])
	}
}

func TestCommentMode_TypingUpdatesInput(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	// Type characters.
	for _, ch := range "hello" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	if b.comment.input.Value() != "hello" {
		t.Errorf("comment.input.Value() = %q, want %q", b.comment.input.Value(), "hello")
	}
}

func TestCommentMode_InputResetOnReopen(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Enter comment mode and type something.
	b = sendKey(t, b, altKeyMsg("x"))
	for _, ch := range "old text" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Cancel with Escape.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	// Re-enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	if b.comment.input.Value() != "" {
		t.Errorf("comment.input.Value() after reopen = %q, want empty string (should reset on reopen)", b.comment.input.Value())
	}
}

// --- Comment text in template expansion ---

func TestCommentMode_Enter_URLAction_ExpandsComment(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate URL", Type: "url", URL: "https://example.com/{number}?comment={comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	// Type a comment.
	for _, ch := range "test comment" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to submit.
	sendKey(t, b, arrowMsg(tea.KeyEnter))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called after submitting comment URL action, but no calls recorded")
	}
	// The URL should contain the URL-escaped comment.
	if !strings.Contains(fe.OpenURLCalls[0], "comment=") {
		t.Errorf("OpenURL URL = %q, want it to contain the comment parameter", fe.OpenURLCalls[0])
	}
}

func TestCommentMode_Enter_ShellAction_ShellEscapesComment(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "echo {comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	// Type text that would be dangerous without shell escaping.
	dangerousText := "hello; rm -rf /"
	for _, ch := range dangerousText {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to submit.
	_, cmd := b.Update(arrowMsg(tea.KeyEnter))
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called, but no calls recorded")
	}
	// The comment should be shell-escaped (single-quoted), not injecting commands.
	expectedCmd := "echo " + action.ShellEscape(dangerousText)
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell command = %q, want %q (comment should be shell-escaped)", fe.RunShellCalls[0], expectedCmd)
	}
}

// --- Edge case tests ---

func TestCommentMode_CtrlCQuits(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	// Ctrl+C should still quit.
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in commentMode should return a non-nil Cmd (tea.Quit)")
	}
}

func TestCommentMode_BlocksNavigation(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)
	requireColumns(t, b)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	origTab := b.ActiveTab
	origCursor := b.Columns[b.ActiveTab].Cursor

	// Navigation keys should not change ActiveTab or cursor.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != origTab {
		t.Errorf("Tab in commentMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("j in commentMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
	b = sendKey(t, b, keyMsg("k"))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("k in commentMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
}

func TestCommentMode_EmptyCommentStillSubmits(t *testing.T) {
	// An empty comment is valid — the user may want to pass an empty string.
	// The action should execute with {comment} expanded to an empty shell-escaped value.
	actions := map[string]config.Action{
		"x": {Name: "Annotate", Type: "shell", Command: "echo {comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("x"))

	// Press Enter without typing anything.
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("after Enter with empty comment: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called with empty comment, but no calls recorded")
	}
	// Empty comment should still be shell-escaped (results in '').
	expectedCmd := "echo " + action.ShellEscape("")
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell command = %q, want %q (empty comment should be shell-escaped)", fe.RunShellCalls[0], expectedCmd)
	}
}
