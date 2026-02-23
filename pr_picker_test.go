package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
	if !strings.Contains(b.statusBar.View(200), "No linked PRs") {
		t.Errorf("statusBar.View() = %q, want it to contain %q", b.statusBar.View(200), "No linked PRs")
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

	if !strings.Contains(b.statusBar.View(200), "Opened PR #10") {
		t.Errorf("statusBar.View() = %q, want it to contain %q", b.statusBar.View(200), "Opened PR #10")
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

	if !strings.Contains(b.statusBar.View(200), "Opened PR #21") {
		t.Errorf("statusBar.View() = %q, want it to contain %q", b.statusBar.View(200), "Opened PR #21")
	}
}

func TestNormalMode_HintShowsOpenPR(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	// Navigate to card 1 which has a linked PR.
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) == 0 {
		t.Fatalf("test setup: expected card at cursor to have LinkedPRs")
	}

	view := b.statusBar.View(200)
	if !strings.Contains(view, "Open PR") {
		t.Errorf("statusBar.View() = %q, want it to contain %q", view, "Open PR")
	}
}

func TestNormalMode_HintHidesOpenPR_NoPRs(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	// Cursor starts on card 0 which has no linked PRs.
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 0 {
		t.Fatalf("test setup: expected card at cursor to have 0 LinkedPRs, got %d", len(card.LinkedPRs))
	}

	view := b.statusBar.View(200)
	if strings.Contains(view, "Open PR") {
		t.Errorf("statusBar.View() = %q, should NOT contain %q when card has no linked PRs", view, "Open PR")
	}
}

func TestNormalMode_HintUpdatesOnCursorMove(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	// Card 0 (cursor start): no linked PRs — hint should be absent.
	view := b.statusBar.View(200)
	if strings.Contains(view, "Open PR") {
		t.Errorf("card with no PRs: statusBar.View() = %q, should NOT contain %q", view, "Open PR")
	}

	// Move to card 1: has linked PRs — hint should appear.
	b = sendKey(t, b, keyMsg("j"))
	view = b.statusBar.View(200)
	if !strings.Contains(view, "Open PR") {
		t.Errorf("card with PRs: statusBar.View() = %q, want it to contain %q", view, "Open PR")
	}

	// Move back to card 0: no linked PRs — hint should disappear.
	b = sendKey(t, b, keyMsg("k"))
	view = b.statusBar.View(200)
	if strings.Contains(view, "Open PR") {
		t.Errorf("back to card with no PRs: statusBar.View() = %q, should NOT contain %q", view, "Open PR")
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
