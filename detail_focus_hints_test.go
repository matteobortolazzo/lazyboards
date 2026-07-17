package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// Detail-focused hint bar (#419)
//
// The detail panel's status-bar hints (b.statusBar.hints, set whenever
// b.detailFocused becomes true) are currently a hardcoded static list
// (detailFocusHints in model.go): "e" Edit, "j/k" Scroll, "h"/"esc" Back --
// missing the "?" Help pointer and never reflecting the user's configured
// custom actions the way the card-list hint bar (b.normalHints, built by
// rebuildNormalHints()) does.
//
// These tests assert the corrected behavior: the detail-focused hint bar
// must carry the same "?" Help pointer, the same scope-gated custom-action
// merge (global overlaid by the active column's per-column actions), and the
// same truncation-safe ordering (help first, built-ins next, custom actions
// last) as the card-list bar -- restored consistently at every point the app
// sets the detail-focused hint bar.
//
// Tests read b.statusBar.hints directly (same package) rather than a new
// production field, so they describe the required *behavior* without
// dictating the shape of the fix.

// --- AC1: "?" Help present alongside the built-in detail-panel hints ---

func TestDetailFocusedHints_IncludesHelpAndBuiltins(t *testing.T) {
	b := newLoadedTestBoard(t)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	hints := b.statusBar.hints

	help := hintIndex(hints, "?")
	if help == -1 {
		t.Fatalf("detail-focused hints should contain a %q hint, got: %+v", "?", hints)
	}
	if hints[help].Desc != "Help" {
		t.Errorf("? hint Desc = %q, want %q", hints[help].Desc, "Help")
	}

	e := hintIndex(hints, "e")
	if e == -1 || hints[e].Desc != "Edit" {
		t.Errorf("detail-focused hints should contain an %q hint with Desc %q, got: %+v", "e", "Edit", hints)
	}

	jk := hintIndex(hints, "j/k")
	if jk == -1 || hints[jk].Desc != "Scroll" {
		t.Errorf("detail-focused hints should contain a %q hint with Desc %q, got: %+v", "j/k", "Scroll", hints)
	}

	backKeyFound := false
	for _, key := range []string{"h", "esc"} {
		if i := hintIndex(hints, key); i != -1 && hints[i].Desc == "Back" {
			backKeyFound = true
		}
	}
	if !backKeyFound {
		t.Errorf("detail-focused hints should contain an %q-described %q hint, got: %+v", "Back", "h/esc", hints)
	}
}

// --- AC4: ordering -- "?" leftmost, built-ins next, custom actions last ---

func TestDetailFocusedHints_OrderingSurvivesTruncation(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Deploy App", Type: "url", Scope: "board", URL: "https://example.com"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	hints := b.statusBar.hints

	help := hintIndex(hints, "?")
	if help != 0 {
		t.Fatalf("? hint should be leftmost (index 0) so it survives truncation, got index %d: %+v", help, hints)
	}

	custom := hintIndex(hints, "X")
	if custom == -1 {
		t.Fatalf("detail-focused hints should contain the custom action hint %q, got: %+v", "X", hints)
	}

	for _, key := range []string{"e", "j/k", "h"} {
		if i := hintIndex(hints, key); i != -1 && i > custom {
			t.Errorf("built-in hint %q (index %d) should appear before custom action hint (index %d): %+v", key, i, custom, hints)
		}
	}
}

// --- Custom-action hint bar follows config file order (#435/#437) ---

func TestDetailFocusedHints_OrderMatchesConfigOrder(t *testing.T) {
	localYAML := `provider: github
repo: matteobortolazzo/lazyboards
actions:
  Z:
    name: Zebra action
    type: shell
    scope: board
    command: "echo z"
  A:
    name: Apple action
    type: shell
    scope: board
    command: "echo a"
  M:
    name: Mango action
    type: shell
    scope: board
    command: "echo m"
`
	b, _ := newConfigLoadedActionTestBoard(t, localYAML)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	hints := b.statusBar.hints
	z := hintIndex(hints, "Z")
	a := hintIndex(hints, "A")
	m := hintIndex(hints, "M")
	if z == -1 || a == -1 || m == -1 {
		t.Fatalf("expected hints for Z, A, M; got: %+v", hints)
	}
	if !(z < a && a < m) {
		t.Errorf("detail-focused hint order should match the config file order Z, A, M; got indices Z=%d A=%d M=%d in %+v", z, a, m, hints)
	}
}

// --- AC2: scope gating mirrors rebuildNormalHints / dispatch ---

func TestDetailFocusedHints_BoardScopeAction_VisibleEvenOnEmptyColumn(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://example.com"},
	}
	b, _ := newBoardWithEmptyColumn(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l', even with an empty column")
	}

	hints := b.statusBar.hints
	if hintIndex(hints, "B") == -1 {
		t.Errorf("board-scope action hint should be visible in detail-focused hints on an empty column, got: %+v", hints)
	}
}

func TestDetailFocusedHints_CardScopeAction_HiddenOnEmptyColumn(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Card action", Type: "url", URL: "https://example.com/{number}"},
	}
	b, _ := newBoardWithEmptyColumn(t, actions)

	b = sendKey(t, b, keyMsg("l"))

	hints := b.statusBar.hints
	if hintIndex(hints, "X") != -1 {
		t.Errorf("card-scope action hint should be hidden in detail-focused hints when the column has no cards, got: %+v", hints)
	}
}

func TestDetailFocusedHints_CardScopeAction_VisibleWithCards(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Card action", Type: "url", URL: "https://example.com/{number}"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))

	hints := b.statusBar.hints
	if hintIndex(hints, "X") == -1 {
		t.Errorf("card-scope action hint should be visible in detail-focused hints when the column has cards, got: %+v", hints)
	}
}

func TestDetailFocusedHints_PRScopeAction_HiddenWithoutLinkedPR(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch}"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	// Cursor starts on card 1 (0 linked PRs).
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 0 {
		t.Fatalf("test setup: expected card at cursor to have 0 LinkedPRs, got %d", len(card.LinkedPRs))
	}

	b = sendKey(t, b, keyMsg("l"))

	hints := b.statusBar.hints
	if hintIndex(hints, "W") != -1 {
		t.Errorf("pr-scope action hint should be hidden in detail-focused hints on a card with 0 linked PRs, got: %+v", hints)
	}
}

func TestDetailFocusedHints_PRScopeAction_VisibleWithLinkedPR(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch}"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	// Move to card 2 (1 linked PR).
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) == 0 {
		t.Fatalf("test setup: expected card at cursor to have a linked PR")
	}

	b = sendKey(t, b, keyMsg("l"))

	hints := b.statusBar.hints
	if hintIndex(hints, "W") == -1 {
		t.Errorf("pr-scope action hint should be visible in detail-focused hints on a card with a linked PR, got: %+v", hints)
	}
}

// --- AC3: active column's per-column actions override same-keyed globals ---

func TestDetailFocusedHints_ColumnActionOverridesGlobal(t *testing.T) {
	globalActions := map[string]config.Action{
		"X": {Name: "Global Open", Type: "url", URL: "https://global.com/{number}"},
	}
	columnConfigs := []config.ColumnConfig{
		{Name: "New"}, // No column-level actions.
		{
			Name: "Refined",
			Actions: map[string]config.Action{
				"X": {Name: "Deploy", Type: "url", URL: "https://deploy.com/{number}"},
			},
		},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, _ := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// On column 0: should show the global action hint.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}
	hints := b.statusBar.hints
	if i := hintIndex(hints, "X"); i == -1 || hints[i].Desc != "Global Open" {
		t.Errorf("on column 0, detail-focused hints should show the global action %q, got: %+v", "Global Open", hints)
	}

	// Back to card list, tab to column 1, refocus detail: column action should
	// override the global one.
	b = sendKey(t, b, keyMsg("h"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b = sendKey(t, b, keyMsg("l"))
	hints = b.statusBar.hints
	if i := hintIndex(hints, "X"); i == -1 || hints[i].Desc != "Deploy" {
		t.Errorf("on column 1, detail-focused hints should show the column override %q, got: %+v", "Deploy", hints)
	}
}

// --- AC5: the corrected hint bar is shown/restored at every call site ---

func TestDetailFocusedHints_SurviveBoardRefresh(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://example.com"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	// Trigger a manual refresh ('r') and simulate its completion.
	b = sendKey(t, b, keyMsg("r"))
	b = simulateRefresh(t, b)

	if !b.detailFocused {
		t.Fatal("detailFocused should remain true across a board refresh")
	}

	hints := b.statusBar.hints
	if hintIndex(hints, "?") == -1 {
		t.Errorf("after a board refresh while detail-focused, hints should still contain %q, got: %+v", "?", hints)
	}
	if hintIndex(hints, "B") == -1 {
		t.Errorf("after a board refresh while detail-focused, hints should still contain the custom action %q, got: %+v", "B", hints)
	}
	if i := hintIndex(hints, "h"); i == -1 {
		t.Errorf("after a board refresh while detail-focused, hints should still contain the built-in %q hint, got: %+v", "h", hints)
	}
}

func TestDetailFocusedHints_ConfigReload_ResetsToCorrectedNormalHints(t *testing.T) {
	// A config reload (configSavedMsg -> loadingMode -> fetchBoardCmd ->
	// boardFetchedMsg with b.refreshing == false) unconditionally resets
	// detailFocused to false (update.go's non-refreshing handleBoardFetched
	// branch). The resulting hint bar must be the corrected, dynamic
	// b.normalHints -- not a stale detail-focused bar -- and must include the
	// same custom actions/help hint the (now non-detail-focused) card list
	// would show.
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://example.com"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	// Simulate the config-reload completion path: a boardFetchedMsg arriving
	// while b.refreshing is false (the real trigger is configSavedMsg ->
	// mode=loadingMode -> fetchBoardCmd -> this message) hits
	// handleBoardFetched's non-refreshing branch, which unconditionally
	// resets detailFocused. simulateRefresh sends the message directly
	// without setting b.refreshing=true first, exercising that branch.
	b = simulateRefresh(t, b)

	if b.detailFocused {
		t.Error("a config-reload boardFetchedMsg should reset detailFocused to false")
	}

	hints := b.statusBar.hints
	if hintIndex(hints, "?") == -1 {
		t.Errorf("after a config reload, hints should contain %q, got: %+v", "?", hints)
	}
	if hintIndex(hints, "B") == -1 {
		t.Errorf("after a config reload, hints should contain the custom action %q, got: %+v", "B", hints)
	}
	if hintIndex(hints, "j/k") != -1 {
		t.Errorf("after a config reload (no longer detail-focused), hints should NOT contain the detail-only %q hint, got: %+v", "j/k", hints)
	}
}

func TestDetailFocusedHints_RestoreAfterCommentMode(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, altKeyMsg("X"))
	if b.mode != commentMode {
		t.Fatalf("precondition: mode should be commentMode after Alt+X, got %d", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be restored after Escape from comment mode")
	}

	hints := b.statusBar.hints
	if hintIndex(hints, "?") == -1 {
		t.Errorf("after returning from comment mode to detail focus, hints should contain %q, got: %+v", "?", hints)
	}
	if hintIndex(hints, "X") == -1 {
		t.Errorf("after returning from comment mode to detail focus, hints should contain the custom action %q, got: %+v", "X", hints)
	}
}

func TestDetailFocusedHints_RestoreAfterHelpModal(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://example.com"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("?"))
	if b.mode != helpMode {
		t.Fatalf("precondition: mode should be helpMode after '?', got %d", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be restored after Escape from help modal")
	}

	hints := b.statusBar.hints
	if hintIndex(hints, "?") == -1 {
		t.Errorf("after returning from the help modal to detail focus, hints should contain %q, got: %+v", "?", hints)
	}
	if hintIndex(hints, "B") == -1 {
		t.Errorf("after returning from the help modal to detail focus, hints should contain the custom action %q, got: %+v", "B", hints)
	}
}

func TestDetailFocusedHints_RestoreAfterPRPicker(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://example.com"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	// Move to card 3 (2 linked PRs) so 'p' opens the picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	b = sendKey(t, b, keyMsg("p"))
	if b.mode != prPickerMode {
		t.Fatalf("precondition: mode should be prPickerMode after 'p' on a 2-PR card, got %d", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be restored after Escape from the PR picker")
	}

	hints := b.statusBar.hints
	if hintIndex(hints, "?") == -1 {
		t.Errorf("after returning from the PR picker to detail focus, hints should contain %q, got: %+v", "?", hints)
	}
	if hintIndex(hints, "B") == -1 {
		t.Errorf("after returning from the PR picker to detail focus, hints should contain the custom action %q, got: %+v", "B", hints)
	}
}

// --- AC6: dispatch behavior itself is unaffected (regression guard) ---

func TestDetailFocusedHints_DispatchStillFiresRegardlessOfHintBarContent(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://example.com"},
	}
	b, fe := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("B"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called for a board-scope action dispatched from detail focus, but no calls recorded")
	}
	if !b.detailFocused {
		t.Error("dispatching a board-scope action from detail focus should not drop detailFocused")
	}
}

// --- View() rendering: hints ultimately reach the rendered status bar ---

func TestView_DetailFocused_ShowsHelpAndCustomActionHints(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://example.com"},
	}
	b, _ := newActionTestBoard(t, actions)
	b.Width = 200
	b.Height = 40

	b = sendKey(t, b, keyMsg("l"))

	view := b.View()
	if !strings.Contains(view, "Help") {
		t.Errorf("View() in detail focus should contain the %q hint desc in the status bar, got:\n%s", "Help", view)
	}
	if !strings.Contains(view, "Open board") {
		t.Errorf("View() in detail focus should contain the custom action hint %q in the status bar, got:\n%s", "Open board", view)
	}
}
