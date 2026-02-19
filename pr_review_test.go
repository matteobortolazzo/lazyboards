package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// enterPRReview creates a board with PRs, navigates to the card with exactly
// 1 LinkedPR (card index 1), and presses "p" to enter prReviewMode directly
// (single PR skips the picker). Returns the board in prReviewMode.
func enterPRReview(t *testing.T) Board {
	t.Helper()
	b := newBoardWithPRs(t)

	// Navigate to card 1 which has exactly 1 LinkedPR.
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 1 {
		t.Fatalf("enterPRReview: expected card with 1 LinkedPR, got %d", len(card.LinkedPRs))
	}

	// Press "p" to enter prReviewMode (single PR skips picker).
	b = sendKey(t, b, keyMsg("p"))
	if b.mode != prReviewMode {
		t.Fatalf("enterPRReview: expected prReviewMode (%d), got %d", prReviewMode, b.mode)
	}
	return b
}

func TestPRReview_Escape_ReturnsToNormal(t *testing.T) {
	b := enterPRReview(t)

	// Press Escape to exit prReviewMode.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("mode = %d after Escape, want normalMode (%d)", b.mode, normalMode)
	}
}

func TestPRReview_Q_Quits(t *testing.T) {
	b := enterPRReview(t)

	// Press "q" — should return a tea.Quit cmd.
	_, cmd := b.Update(keyMsg("q"))

	if cmd == nil {
		t.Error("'q' in prReviewMode should return a non-nil Cmd (tea.Quit)")
	}
}

func TestPRReview_JK_ScrollsOffset(t *testing.T) {
	b := enterPRReview(t)

	// Press "j" multiple times — prScrollOffset should increment.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))

	if b.prScrollOffset < 3 {
		t.Errorf("prScrollOffset = %d after 3 'j' presses, want >= 3", b.prScrollOffset)
	}

	// Press "k" — prScrollOffset should decrement.
	offsetBefore := b.prScrollOffset
	b = sendKey(t, b, keyMsg("k"))

	if b.prScrollOffset >= offsetBefore {
		t.Errorf("prScrollOffset = %d after 'k', want less than %d", b.prScrollOffset, offsetBefore)
	}
}

func TestPRReview_K_ClampsAtZero(t *testing.T) {
	b := enterPRReview(t)

	// prScrollOffset starts at 0. Press "k" — should stay at 0.
	b = sendKey(t, b, keyMsg("k"))

	if b.prScrollOffset != 0 {
		t.Errorf("prScrollOffset = %d after 'k' at offset 0, want 0 (should not go negative)", b.prScrollOffset)
	}
}

func TestPRReview_HL_TogglesFocus(t *testing.T) {
	b := enterPRReview(t)

	// Press "l" — should set prFocusRight to true.
	b = sendKey(t, b, keyMsg("l"))

	if !b.prFocusRight {
		t.Error("prFocusRight should be true after 'l' in prReviewMode")
	}

	// Press "h" — should set prFocusRight to false.
	b = sendKey(t, b, keyMsg("h"))

	if b.prFocusRight {
		t.Error("prFocusRight should be false after 'h' in prReviewMode")
	}
}

func TestPRReview_BlocksCreateAndConfig(t *testing.T) {
	b := enterPRReview(t)

	// These keys should be no-ops in prReviewMode — mode should not change.
	blockedKeys := []struct {
		name string
		msg  tea.KeyMsg
	}{
		{"n", keyMsg("n")},
		{"c", keyMsg("c")},
		{"p", keyMsg("p")},
		{"1", keyMsg("1")},
		{"2", keyMsg("2")},
		{"9", keyMsg("9")},
	}

	for _, bk := range blockedKeys {
		b = sendKey(t, b, bk.msg)
		if b.mode != prReviewMode {
			t.Errorf("after %q: mode = %d, want prReviewMode (%d)", bk.name, b.mode, prReviewMode)
		}
	}
}
