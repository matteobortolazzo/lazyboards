package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func TestNormalMode_P_NoPRs_ShowsMessage(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	// Card 0 (cursor starts here) has 0 LinkedPRs.
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 0 {
		t.Fatalf("test setup: expected card at cursor to have 0 LinkedPRs, got %d", len(card.LinkedPRs))
	}

	// Press "p" — should stay in normalMode and show a message.
	m, cmd := b.Update(keyMsg("p"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if !strings.Contains(b.statusBar.View(200, 0, 0), "No linked PRs") {
		t.Errorf("statusBar.View(, 0, 0) = %q, want it to contain %q", b.statusBar.View(200, 0, 0), "No linked PRs")
	}
}

func TestNormalMode_P_SinglePR_OpensBrowser(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)

	// Navigate to card 1 which has exactly 1 LinkedPR.
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 1 {
		t.Fatalf("test setup: expected card at cursor to have 1 LinkedPR, got %d", len(card.LinkedPRs))
	}

	// Press "p" — should open the PR URL in the browser and stay in normalMode.
	b = sendKey(t, b, keyMsg("p"))

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL called %d times, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != "https://github.com/owner/repo/pull/10" {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], "https://github.com/owner/repo/pull/10")
	}
}

func TestNormalMode_P_SinglePR_ShowsStatusMessage(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	// Navigate to card 1 which has exactly 1 LinkedPR.
	b = sendKey(t, b, keyMsg("j"))

	// Press "p" — status bar should show "Opened PR #10".
	m, cmd := b.Update(keyMsg("p"))
	b = m.(Board)
	execCmds(cmd)

	if !strings.Contains(b.statusBar.View(200, 0, 0), "Opened PR #10") {
		t.Errorf("statusBar.View(, 0, 0) = %q, want it to contain %q", b.statusBar.View(200, 0, 0), "Opened PR #10")
	}
}

func TestNormalMode_P_MultiplePRs_EntersPicker(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	// Navigate to card 2 which has 2 LinkedPRs.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) < 2 {
		t.Fatalf("test setup: expected card at cursor to have 2+ LinkedPRs, got %d", len(card.LinkedPRs))
	}

	// Press "p" — should enter prPickerMode.
	b = sendKey(t, b, keyMsg("p"))

	if b.mode != prPickerMode {
		t.Errorf("mode = %d, want prPickerMode (%d)", b.mode, prPickerMode)
	}
	if b.prPickerIndex != 0 {
		t.Errorf("prPickerIndex = %d, want 0", b.prPickerIndex)
	}
}

func TestPRPicker_LeftRight_CyclesPRs(t *testing.T) {
	b := newBoardWithPRs(t)

	// Navigate to card 2 (2 LinkedPRs) and enter picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))

	prCount := len(b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].LinkedPRs)

	// Right once: index should advance to 1.
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.prPickerIndex != 1 {
		t.Errorf("after Right: prPickerIndex = %d, want 1", b.prPickerIndex)
	}

	// Right again: should wrap to 0.
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.prPickerIndex != 0 {
		t.Errorf("after Right×2: prPickerIndex = %d, want 0 (wrap from %d)", b.prPickerIndex, prCount)
	}

	// Left once: should wrap to last index.
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.prPickerIndex != prCount-1 {
		t.Errorf("after Left: prPickerIndex = %d, want %d (last)", b.prPickerIndex, prCount-1)
	}

	// Left again: should return to 0.
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.prPickerIndex != 0 {
		t.Errorf("after Left×2: prPickerIndex = %d, want 0", b.prPickerIndex)
	}
}

func TestPRPicker_Escape_ReturnsToNormal(t *testing.T) {
	b := newBoardWithPRs(t)

	// Navigate to card 2 and enter picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))

	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}

	// Press Escape — should return to normalMode.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

func TestPRPicker_LinkedPRsShrinkToZeroWhileOpen_ReturnsToNormal(t *testing.T) {
	b := newBoardWithPRs(t)

	// Navigate to card 2 (2 LinkedPRs) and enter picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))

	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}

	// Simulate an async board refresh (boardFetchedMsg) shrinking the
	// selected card's LinkedPRs to 0 while the picker is still open.
	col := &b.Columns[b.ActiveTab]
	col.Cards[col.Cursor].LinkedPRs = nil

	// Any picker key (e.g. Right) must not panic or divide by zero; it
	// should fall back to the same cleanup as Escape.
	b = sendKey(t, b, arrowMsg(tea.KeyRight))

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if b.pendingPRAction != nil {
		t.Errorf("pendingPRAction = %+v, want nil", b.pendingPRAction)
	}
}

func TestPRPicker_LinkedPRsShrinkWhileOpen_ClampsIndexAndActsOnCorrectPR(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)

	// Navigate to card 3 ("Two PRs": #20, #21) and enter the picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}

	// Move to the second PR (index 1, PR #21).
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.prPickerIndex != 1 {
		t.Fatalf("test setup: expected prPickerIndex 1, got %d", b.prPickerIndex)
	}

	// Simulate an in-flight background refresh completing while the picker
	// is still open: mark the board as mid-refresh (handleBoardFetched's
	// refreshing branch preserves mode/cursor instead of forcing
	// normalMode) and deliver a boardFetchedMsg where the same card's
	// LinkedPRs shrank from 2 to 1 (only PR #20 remains). b.prPickerIndex
	// (still 1) now points past the end of the new slice.
	b.refreshing = true
	shrunkPR := provider.LinkedPR{Number: 20, Title: "feat: first PR", URL: "https://github.com/owner/repo/pull/20", Branch: "feature/first-pr"}
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "No PRs", Labels: []provider.Label{{Name: "bug"}}},
				{Number: 2, Title: "One PR", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "feat: one PR", URL: "https://github.com/owner/repo/pull/10", Branch: "feature/one-pr"},
				}},
				{Number: 3, Title: "Two PRs", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{shrunkPR}},
			}},
		},
	}}

	m, cmd := b.Update(msg)
	b = m.(Board)
	execCmds(cmd)

	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected mode to remain prPickerMode after in-flight refresh, got %d", b.mode)
	}
	card := b.selectedCard()
	if len(card.LinkedPRs) != 1 {
		t.Fatalf("test setup: expected shrunk card to have 1 LinkedPR, got %d", len(card.LinkedPRs))
	}

	// Press Enter without re-navigating: prPickerIndex is still 1, past the
	// new length of 1. This must not panic, and must act on the clamped
	// (last-valid) PR rather than an out-of-range index.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Enter panicked after LinkedPRs shrank while the picker was open: %v", r)
		}
	}()
	m, cmd = b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL called %d times, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != shrunkPR.URL {
		t.Errorf("OpenURL called with %q, want %q (the clamped, remaining PR)", fe.OpenURLCalls[0], shrunkPR.URL)
	}
}

func TestPRPicker_Enter_OpensBrowser(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)

	// Navigate to card 2 (2 LinkedPRs) and enter picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))

	// Move to second PR (index 1).
	b = sendKey(t, b, arrowMsg(tea.KeyRight))

	// Press Enter — should open the selected PR URL in the browser.
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL called %d times, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != "https://github.com/owner/repo/pull/21" {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], "https://github.com/owner/repo/pull/21")
	}
}

func TestPRPicker_Enter_ShowsStatusMessage(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	// Navigate to card 2 (2 LinkedPRs) and enter picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))

	// Move to second PR and press Enter.
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if !strings.Contains(b.statusBar.View(200, 0, 0), "Opened PR #21") {
		t.Errorf("statusBar.View(, 0, 0) = %q, want it to contain %q", b.statusBar.View(200, 0, 0), "Opened PR #21")
	}
}

func TestPRPicker_ViewShowsModal(t *testing.T) {
	b := newBoardWithPRs(t)

	// Navigate to card 2 and enter picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))

	// Get the first PR's data from the card for assertions.
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	firstPR := card.LinkedPRs[0]

	view := b.View()

	if !strings.Contains(view, "Select PR") {
		t.Errorf("View() should contain modal title %q", "Select PR")
	}
	if !strings.Contains(view, fmt.Sprintf("#%d", firstPR.Number)) {
		t.Errorf("View() should contain PR number %q", fmt.Sprintf("#%d", firstPR.Number))
	}
	if !strings.Contains(view, firstPR.Title) {
		t.Errorf("View() should contain PR title %q", firstPR.Title)
	}
}

func TestPRPicker_BlocksNavigation(t *testing.T) {
	b := newBoardWithPRs(t)

	// Navigate to card 2 and enter picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))

	initialTab := b.ActiveTab
	initialCursor := b.Columns[b.ActiveTab].Cursor

	// Keys that should be no-ops in prPickerMode.
	blockedKeys := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"j", keyMsg("j")},
		{"k", keyMsg("k")},
		{"tab", arrowMsg(tea.KeyTab)},
		{"q", keyMsg("q")},
	}

	for _, bk := range blockedKeys {
		b = sendKey(t, b, bk.msg)

		if b.mode != prPickerMode {
			t.Errorf("after %q: mode = %d, want prPickerMode (%d)", bk.name, b.mode, prPickerMode)
		}
		if b.ActiveTab != initialTab {
			t.Errorf("after %q: ActiveTab = %d, want %d", bk.name, b.ActiveTab, initialTab)
		}
		if b.Columns[b.ActiveTab].Cursor != initialCursor {
			t.Errorf("after %q: Cursor = %d, want %d", bk.name, b.Columns[b.ActiveTab].Cursor, initialCursor)
		}
	}
}

// --- Dual-purpose picker: scope: pr custom actions (#340) ---

func TestPRPicker_Enter_WithPendingPRAction_RunsActionAndClearsPending(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	// Move to card 3 (2 linked PRs) and trigger the pr-scope action to open the picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("W"))
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode after pr-scope action on a 2+ PR card, got %d", b.mode)
	}

	// Move to the second PR and confirm.
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	selectedPR := card.LinkedPRs[b.prPickerIndex]

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if b.pendingPRAction != nil {
		t.Error("pendingPRAction should be cleared after running")
	}
	// The pending pr-scope action, not the default open-URL behavior, must run.
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls (pending pr-scope action should run instead of opening the URL), got %d", len(fe.OpenURLCalls))
	}
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called against the selected PR, but no calls recorded")
	}
	expectedCmd := "cd " + action.ShellEscape(selectedPR.Branch)
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
}

func TestPRPicker_Enter_WithoutPendingPRAction_StillOpensURL(t *testing.T) {
	// Regression test: plain "open PR" via the built-in 'p' key must still
	// work when no pr-scope custom action is pending (dual-purpose picker).
	b, fe := newBoardWithPRsAndExecutor(t)

	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}
	if b.pendingPRAction != nil {
		t.Fatalf("test setup: expected no pendingPRAction for the built-in 'p' picker path")
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called (fallback open-URL behavior), but no calls recorded")
	}
}

// --- PR status glyph in the PR picker (#431) ---
//
// viewPRPickerModal prepends the same prStatusSymbol(prStatus(pr)) +
// prStatusStyle prefix used by the global PR list (pr_list_test.go) to the
// single shown PR row. UNKNOWN renders no glyph, matching the PR list's
// (and diverging from the board glyph's neutral-color) behavior.

// TestPRPicker_View_ShowsStatusSymbolStyledForSelectedPR asserts the
// currently-selected PR's row renders prStatusSymbol("conflicting") styled
// via prConflictingStyle.
func TestPRPicker_View_ShowsStatusSymbolStyledForSelectedPR(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Two PRs", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: conflicting", URL: "https://github.com/o/r/pull/10", Mergeable: "CONFLICTING", MergeStateStatus: "DIRTY"},
			{Number: 11, Title: "feat: unresolved", URL: "https://github.com/o/r/pull/11", Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN"},
		}},
	}, 120, 40)
	b = sendKey(t, b, keyMsg("p"))
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}

	view := b.viewPRPickerModal()
	want := prConflictingStyle.Render(prStatusSymbol("conflicting"))
	if !strings.Contains(view, want) {
		t.Errorf("PR picker view missing conflicting status symbol styled %q; got:\n%s", want, view)
	}
}

// TestPRPicker_View_UnknownStatusRendersNoGlyph asserts the picker shows no
// status glyph for a PR whose mergeable state is UNKNOWN.
func TestPRPicker_View_UnknownStatusRendersNoGlyph(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Two PRs", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: conflicting", URL: "https://github.com/o/r/pull/10", Mergeable: "CONFLICTING", MergeStateStatus: "DIRTY"},
			{Number: 11, Title: "feat: unresolved", URL: "https://github.com/o/r/pull/11", Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN"},
		}},
	}, 120, 40)
	b = sendKey(t, b, keyMsg("p"))
	b = sendKey(t, b, arrowMsg(tea.KeyRight)) // move to the second (unresolved) PR
	if b.prPickerIndex != 1 {
		t.Fatalf("test setup: expected prPickerIndex 1, got %d", b.prPickerIndex)
	}

	view := b.viewPRPickerModal()
	for _, status := range []string{"draft", "mergeable", "conflicting", "blocked"} {
		if sym := prStatusSymbol(status); sym != "" && strings.Contains(view, sym) {
			t.Errorf("picker view for unknown-status PR contains a known-state glyph %q (status %s), want none; got:\n%s", sym, status, view)
		}
	}
}

// --- PR Picker selected-row styling matches other lists (#450) ---
//
// Every other list-like UI element in the app (card list, PR list, agents
// list) highlights its current/selected row with selectedCardStyle (bold,
// bright white). The PR Picker modal renders its PR text in the terminal's
// default foreground color instead -- it should style the "#NN Title"
// segment with selectedCardStyle just like the other lists.

// TestPRPicker_View_StylesSelectedPRTextWithSelectedRowStyle asserts the
// picker's focused/current PR text is rendered with selectedCardStyle,
// matching the selected-row convention used by every other list in the app.
func TestPRPicker_View_StylesSelectedPRTextWithSelectedRowStyle(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Two PRs", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: first PR", URL: "https://github.com/o/r/pull/10"},
			{Number: 11, Title: "feat: second PR", URL: "https://github.com/o/r/pull/11"},
		}},
	}, 120, 40)

	m, cmd := b.Update(keyMsg("p"))
	b = m.(Board)
	execCmds(cmd)
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}

	pr := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].LinkedPRs[b.prPickerIndex]

	view := b.viewPRPickerModal()
	want := selectedCardStyle.Render(fmt.Sprintf("#%d %s", pr.Number, pr.Title))
	if !strings.Contains(view, want) {
		t.Errorf("PR picker view missing selected-row-styled PR text %q; got:\n%s", want, view)
	}
}

func TestPRPicker_Escape_ClearsPendingPRAction(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch}"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("W"))
	if b.pendingPRAction == nil {
		t.Fatalf("test setup: expected pendingPRAction to be set")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if b.pendingPRAction != nil {
		t.Error("pendingPRAction should be cleared on Escape")
	}
}
