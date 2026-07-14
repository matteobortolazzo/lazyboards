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
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Press Alt+x — should enter commentMode because the action uses {comment}.
	b = sendKey(t, b, altKeyMsg("X"))

	if b.mode != commentMode {
		t.Errorf("after Alt+x on action with {comment}: mode = %d, want %d (commentMode)", b.mode, commentMode)
	}
}

func TestCommentMode_AltKeyWithoutComment_ExecutesNormally(t *testing.T) {
	// An action without {comment} should execute normally even with Alt held.
	actions := map[string]config.Action{
		"X": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Press Alt+x on an action without {comment} — should execute the action, not enter commentMode.
	b = sendKey(t, b, altKeyMsg("X"))

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
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Press X (no Alt) — should execute immediately, not enter commentMode.
	m, cmd := b.Update(keyMsg("X"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode == commentMode {
		t.Error("regular key on action with {comment} should NOT enter commentMode")
	}
}

func TestCommentMode_BoardScopeAction(t *testing.T) {
	// An Alt+key on a board-scope action with {comment} should set boardScope=true.
	actions := map[string]config.Action{
		"X": {Name: "Deploy Note", Type: "shell", Scope: "board", Command: "deploy --comment {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Press Alt+x on a board-scope action.
	b = sendKey(t, b, altKeyMsg("X"))

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
		"X": {Name: actionName, Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Get the currently selected card before entering comment mode.
	col := b.Columns[b.ActiveTab]
	expectedCard := col.Cards[col.Cursor]

	// Press Alt+x.
	b = sendKey(t, b, altKeyMsg("X"))

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
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)
	b.Width = 120
	b.Height = 40

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

	view := b.View()
	if !strings.Contains(view, "Annotate") {
		t.Errorf("View() in commentMode should contain the action name %q", "Annotate")
	}
}

func TestCommentMode_ViewShowsHints(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)
	b.Width = 120
	b.Height = 40

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

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
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))
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
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

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
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

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
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Enter comment mode and type something.
	b = sendKey(t, b, altKeyMsg("X"))
	for _, ch := range "old text" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Cancel with Escape.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	// Re-enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

	if b.comment.input.Value() != "" {
		t.Errorf("comment.input.Value() after reopen = %q, want empty string (should reset on reopen)", b.comment.input.Value())
	}
}

// --- Comment text in template expansion ---

func TestCommentMode_Enter_URLAction_ExpandsComment(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Annotate URL", Type: "url", URL: "https://example.com/{number}?comment={comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

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
		"X": {Name: "Annotate", Type: "shell", Command: "echo {comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

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
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

	// Ctrl+C should still quit.
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in commentMode should return a non-nil Cmd (tea.Quit)")
	}
}

func TestCommentMode_BlocksNavigation(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)
	requireColumns(t, b)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

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

// --- Comment mode triggered from the detail-focused panel ---

func TestCommentMode_AltKeyFromDetailFocus_EntersCommentModeAndUnfocusesDetail(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	b = sendKey(t, b, altKeyMsg("X"))

	if b.mode != commentMode {
		t.Fatalf("after Alt+x in detail focus: mode = %d, want %d (commentMode)", b.mode, commentMode)
	}
	if b.detailFocused {
		t.Error("entering commentMode should unfocus the detail panel while comment input is active, mirroring the help-mode pattern")
	}
}

func TestCommentMode_Escape_FromDetailFocus_RestoresDetailFocusAndHints(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, altKeyMsg("X"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Fatalf("after Escape in commentMode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if !b.detailFocused {
		t.Error("Escape from a comment triggered in detail focus should restore detailFocused")
	}

	view := b.View()
	if !strings.Contains(view, "Back") {
		t.Errorf("View() after returning from comment mode should show detail-focus hints, got:\n%s", view)
	}
	// "n: New" is the normal-mode hint rendering; check the hint form (not
	// bare "New") since a column in this fixture is itself titled "New".
	if strings.Contains(view, "n: New") {
		t.Errorf("View() after returning from comment mode should NOT show normal-mode hints, got:\n%s", view)
	}
}

func TestCommentMode_Enter_FromDetailFocus_RestoresDetailFocusAndHints(t *testing.T) {
	// Use a "url" action, not "shell": a shell submit shows a transient
	// "Running..." status message that would mask the hint-bar assertion
	// below without indicating a hint-restoration bug.
	actions := map[string]config.Action{
		"X": {Name: "Annotate", Type: "url", URL: "https://example.com/{number}?body={comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, altKeyMsg("X"))

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if !b.detailFocused {
		t.Error("Enter/submit from a comment triggered in detail focus should restore detailFocused")
	}
	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called after submitting comment from detail focus, but no calls recorded")
	}

	view := b.View()
	if !strings.Contains(view, "Back") {
		t.Errorf("View() after submitting comment from detail focus should show detail-focus hints, got:\n%s", view)
	}
}

func TestCommentMode_AltKeyFromNormalMode_DoesNotSetDetailFocused(t *testing.T) {
	// Regression: comment mode triggered from normal (non-focused) mode must
	// not leave detailFocused set on return.
	actions := map[string]config.Action{
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, altKeyMsg("X"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.detailFocused {
		t.Error("comment mode triggered from normal mode should not set detailFocused on return")
	}
}

func TestCommentMode_EmptyCommentStillSubmits(t *testing.T) {
	// An empty comment is valid — the user may want to pass an empty string.
	// The action should execute with {comment} expanded to an empty shell-escaped value.
	actions := map[string]config.Action{
		"X": {Name: "Annotate", Type: "shell", Command: "echo {comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))

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

// --- scope: pr comment-mode-first interaction (#340, Q2) ---

func TestCommentMode_PRScopeAltKey_EntersCommentModeWithPRScopeFlag(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve with note", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && echo {comment}"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	// Move to card 2 (1 linked PR) so resolveAction gates the pr-scope key through.
	b = sendKey(t, b, keyMsg("j"))

	b = sendKey(t, b, altKeyMsg("W"))

	if b.mode != commentMode {
		t.Fatalf("expected commentMode after Alt+W on pr-scope action with {comment}, got mode = %d", b.mode)
	}
	if !b.comment.prScope {
		t.Error("comment.prScope should be true for a pr-scope action")
	}
	if b.comment.boardScope {
		t.Error("comment.boardScope should be false for a pr-scope action")
	}
}

func TestCommentMode_PRScope_ZeroPRs_DoesNotEnterCommentMode(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve with note", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && echo {comment}"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	// Cursor starts on card 1 (0 linked PRs) -- resolveAction should gate the
	// key out entirely, so Alt+W must not enter commentMode.
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 0 {
		t.Fatalf("test setup: expected 0 linked PRs")
	}

	b = sendKey(t, b, altKeyMsg("W"))

	if b.mode == commentMode {
		t.Error("Alt+key on pr-scope action should NOT enter commentMode when the card has 0 linked PRs")
	}
}

func TestCommentMode_PRScope_SinglePR_SubmitRunsImmediatelyWithComment(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve with note", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && echo {comment}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	// Move to card 2 (1 linked PR).
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	pr := card.LinkedPRs[0]

	b = sendKey(t, b, altKeyMsg("W"))
	for _, ch := range "deploying" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called immediately for a pr-scope+comment action on a 1-PR card")
	}
	expectedCmd := "cd " + action.ShellEscape(pr.Branch) + " && echo " + action.ShellEscape("deploying")
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
}

func TestCommentMode_PRScope_MultiplePRs_SubmitOpensPickerWithPendingComment(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve with note", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && echo {comment}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	// Move to card 3 (2 linked PRs).
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))

	b = sendKey(t, b, altKeyMsg("W"))
	for _, ch := range "picked" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != prPickerMode {
		t.Fatalf("expected prPickerMode after submitting a comment for a pr-scope action on a 2+ PR card, got mode = %d", b.mode)
	}
	if b.pendingPRAction == nil {
		t.Fatal("expected pendingPRAction to be set")
	}
	if b.pendingPRAction.comment != "picked" {
		t.Errorf("pendingPRAction.comment = %q, want %q", b.pendingPRAction.comment, "picked")
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls yet (waiting on PR selection), got %d", len(fe.RunShellCalls))
	}

	// Now select a PR and confirm; the carried comment must be expanded.
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	selectedPR := card.LinkedPRs[b.prPickerIndex]
	m2, cmd2 := b.Update(arrowMsg(tea.KeyEnter))
	b = m2.(Board)
	execCmds(cmd2)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called after selecting a PR with a pending comment")
	}
	expectedCmd := "cd " + action.ShellEscape(selectedPR.Branch) + " && echo " + action.ShellEscape("picked")
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
}

func TestCommentMode_PRScope_PickerEscape_ClearsPendingCommentAndAction(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve with note", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && echo {comment}"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, altKeyMsg("W"))
	for _, ch := range "note" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if b.pendingPRAction != nil {
		t.Error("pendingPRAction should be cleared after Escape from the picker")
	}
}

// --- scope: pr comment mode triggered from the detail-focused panel (#349 x #340) ---

func TestCommentMode_PRScopeAltKeyFromDetailFocus_EntersCommentModeAndUnfocusesDetail(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve with note", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && echo {comment}"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	// Move to card 2 (1 linked PR) so resolveAction gates the pr-scope key through.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	b = sendKey(t, b, altKeyMsg("W"))

	if b.mode != commentMode {
		t.Fatalf("after Alt+W in detail focus: mode = %d, want %d (commentMode)", b.mode, commentMode)
	}
	if !b.comment.prScope {
		t.Error("comment.prScope should be true for a pr-scope action triggered from detail focus")
	}
	if !b.comment.fromDetailFocused {
		t.Error("comment.fromDetailFocused should be true when triggered from the detail-focused panel")
	}
	if b.detailFocused {
		t.Error("entering commentMode should unfocus the detail panel while comment input is active, mirroring the help-mode pattern")
	}
}

func TestCommentMode_PRScope_Escape_FromDetailFocus_RestoresDetailFocusAndHints(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve with note", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && echo {comment}"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, altKeyMsg("W"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Fatalf("after Escape in commentMode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if !b.detailFocused {
		t.Error("Escape from a pr-scope comment triggered in detail focus should restore detailFocused")
	}

	view := b.View()
	if !strings.Contains(view, "Back") {
		t.Errorf("View() after returning from comment mode should show detail-focus hints, got:\n%s", view)
	}
}

func TestCommentMode_PRScope_Enter_FromDetailFocus_RestoresDetailFocusAndHints(t *testing.T) {
	// Use a "url" action, not "shell": a shell submit shows a transient
	// "Running..." status message that would mask the hint-bar assertion
	// below without indicating a hint-restoration bug.
	actions := map[string]config.Action{
		"W": {Name: "Serve with note", Type: "url", Scope: "pr", URL: "https://example.com/{pr_number}?note={comment}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, altKeyMsg("W"))

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if !b.detailFocused {
		t.Error("Enter/submit from a pr-scope comment triggered in detail focus should restore detailFocused")
	}
	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called after submitting comment from detail focus, but no calls recorded")
	}

	view := b.View()
	if !strings.Contains(view, "Back") {
		t.Errorf("View() after submitting comment from detail focus should show detail-focus hints, got:\n%s", view)
	}
}
