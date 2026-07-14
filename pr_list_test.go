package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// errAnyOpenURL is a sentinel used to force the FakeExecutor's OpenURL to fail.
var errAnyOpenURL = errors.New("open failed")

// The shared newBoardWithPRsAndExecutor fixture has one column with three
// cards: card 1 (0 PRs), card 2 (1 PR #10), card 3 (2 PRs #20, #21) — three
// linked PRs across the board in card-then-PR order.

func TestNormalMode_V_OpensPRListModal(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	b = sendKey(t, b, keyMsg("v"))

	if b.mode != prListMode {
		t.Fatalf("mode = %d, want prListMode (%d)", b.mode, prListMode)
	}
	if len(b.prList.entries) != 3 {
		t.Fatalf("entries = %d, want 3 (aggregated across all cards)", len(b.prList.entries))
	}
	// Aggregation order is column, then card, then PR within card.
	wantNumbers := []int{10, 20, 21}
	for i, want := range wantNumbers {
		if got := b.prList.entries[i].pr.Number; got != want {
			t.Errorf("entries[%d].pr.Number = %d, want %d", i, got, want)
		}
	}
	// Each entry records its owning card so rows can be disambiguated.
	if b.prList.entries[0].cardNumber != 2 {
		t.Errorf("entries[0].cardNumber = %d, want 2", b.prList.entries[0].cardNumber)
	}
	if b.prList.entries[1].cardNumber != 3 || b.prList.entries[2].cardNumber != 3 {
		t.Errorf("entries[1..2].cardNumber = %d,%d, want 3,3",
			b.prList.entries[1].cardNumber, b.prList.entries[2].cardNumber)
	}
	if b.prList.entries[0].columnTitle != "Column A" {
		t.Errorf("entries[0].columnTitle = %q, want %q", b.prList.entries[0].columnTitle, "Column A")
	}
}

func TestPRList_Navigation_MovesAndClampsCursor(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))

	if b.prList.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", b.prList.cursor)
	}

	// k at the top stays clamped at 0.
	b = sendKey(t, b, keyMsg("k"))
	if b.prList.cursor != 0 {
		t.Errorf("cursor after k at top = %d, want 0", b.prList.cursor)
	}

	b = sendKey(t, b, keyMsg("j"))
	if b.prList.cursor != 1 {
		t.Errorf("cursor after j = %d, want 1", b.prList.cursor)
	}

	// Walk to the bottom and confirm it clamps at the last entry.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.prList.cursor != 2 {
		t.Errorf("cursor after walking past bottom = %d, want 2", b.prList.cursor)
	}
}

func TestPRList_Enter_OpensSelectedPR(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))

	// Select the second entry (PR #20) and open it.
	b = sendKey(t, b, keyMsg("j"))
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL called %d times, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != "https://github.com/owner/repo/pull/20" {
		t.Errorf("OpenURL = %q, want the PR #20 URL", fe.OpenURLCalls[0])
	}
	if !strings.Contains(b.statusBar.View(200, 0, 0), "Opened PR #20") {
		t.Errorf("status = %q, want it to contain %q", b.statusBar.View(200, 0, 0), "Opened PR #20")
	}
}

func TestPRList_Esc_ReturnsToNormal(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode after esc = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

func TestPRList_EmptyState_OpensAndEnterCloses(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newBoardWithInlineCardsAndExecutor(t, []provider.Card{
		{Number: 1, Title: "No PRs here"},
	}, fe)

	b = sendKey(t, b, keyMsg("v"))
	if b.mode != prListMode {
		t.Fatalf("mode = %d, want prListMode (%d)", b.mode, prListMode)
	}
	if len(b.prList.entries) != 0 {
		t.Fatalf("entries = %d, want 0", len(b.prList.entries))
	}

	// The modal communicates the empty state rather than showing a blank box.
	if !strings.Contains(b.viewPRListModal(), "No linked PRs") {
		t.Errorf("empty modal view = %q, want it to contain %q", b.viewPRListModal(), "No linked PRs")
	}

	// Enter on an empty list just closes; it must not attempt to open a URL.
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)
	if b.mode != normalMode {
		t.Errorf("mode after enter on empty list = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("OpenURL called %d times on empty list, want 0", len(fe.OpenURLCalls))
	}
}

func TestPRList_View_ListsAllPRsWithCardRefs(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))

	view := b.viewPRListModal()
	for _, want := range []string{"#10", "#20", "#21", "feat: one PR"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q\n%s", want, view)
		}
	}
	// Rows carry the owning card number so duplicate/similar PR titles are
	// still distinguishable across the board.
	if !strings.Contains(view, "#2") || !strings.Contains(view, "#3") {
		t.Errorf("view missing card references; got:\n%s", view)
	}
}

func TestPRList_DetailPanel_V_OpensPRListModal(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	// Focus the detail panel first, then press "v": the global PR list must be
	// reachable from detail focus too, like p/o/r/e/?.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatalf("expected detailFocused after l")
	}
	b = sendKey(t, b, keyMsg("v"))

	if b.mode != prListMode {
		t.Fatalf("mode after v from detail panel = %d, want prListMode (%d)", b.mode, prListMode)
	}
	if len(b.prList.entries) != 3 {
		t.Errorf("entries = %d, want 3", len(b.prList.entries))
	}
}

func TestPRList_Enter_OpenURLError_ShowsErrorAndReturns(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)
	fe.OpenURLErr = errAnyOpenURL
	b = sendKey(t, b, keyMsg("v"))

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after failed open = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if !strings.Contains(b.statusBar.View(200, 0, 0), "Error:") {
		t.Errorf("status = %q, want it to contain %q", b.statusBar.View(200, 0, 0), "Error:")
	}
}

// TestPRList_View_RowsFitModalContentWidth guards the invariant that a row can
// never wrap inside the modal. Wrapping is invisible in the fully rendered
// modal (lipgloss.Place pads to the full terminal), so this asserts on the
// pure row formatter directly, with realistic long column + PR titles.
func TestPRList_View_RowsFitModalContentWidth(t *testing.T) {
	entry := prListEntry{
		pr:          LinkedPR{Number: 12345, Title: strings.Repeat("very long pr title ", 6)},
		cardNumber:  6789,
		columnTitle: "In Progress / Waiting For Review",
	}
	const contentWidth = 60 - prListModalPadding
	row := formatPRListRow(entry, contentWidth)
	if w := lipgloss.Width(row); w > contentWidth {
		t.Errorf("row width %d exceeds content width %d: %q", w, contentWidth, row)
	}
	// The row must still carry its identifying number so it stays useful.
	if !strings.Contains(row, "#12345") {
		t.Errorf("row dropped the PR number: %q", row)
	}
}
