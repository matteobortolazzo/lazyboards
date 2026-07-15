package main

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// --- Custom-action key sequences (prefix keybindings) ---

// seqActions returns two sibling URL sequences sharing the "P" prefix.
func seqActions() map[string]config.Action {
	return map[string]config.Action{
		"Pf": {Name: "PR frontend", Type: "url", URL: "https://example.com/frontend/{number}"},
		"Pb": {Name: "PR backend", Type: "url", URL: "https://example.com/backend/{number}"},
	}
}

func TestKeySequence_PrefixKeyEntersPendingWithoutDispatch(t *testing.T) {
	b, fe := newActionTestBoard(t, seqActions())

	b = sendKey(t, b, keyMsg("P"))

	if len(fe.OpenURLCalls) != 0 {
		t.Fatalf("expected no OpenURL calls after prefix key only, got %d", len(fe.OpenURLCalls))
	}
	if b.pendingSeq != "P" {
		t.Errorf("pendingSeq = %q, want %q", b.pendingSeq, "P")
	}
}

func TestKeySequence_PendingHintsListCandidatesAndCancel(t *testing.T) {
	b, _ := newActionTestBoard(t, seqActions())

	b = sendKey(t, b, keyMsg("P"))

	wantHints := []Hint{
		{Key: "Pb", Desc: "PR backend"},
		{Key: "Pf", Desc: "PR frontend"},
		{Key: "esc", Desc: "cancel"},
	}
	hints := b.statusBar.hints
	if len(hints) != len(wantHints) {
		t.Fatalf("pending hints = %v, want %v", hints, wantHints)
	}
	for i, want := range wantHints {
		if hints[i] != want {
			t.Errorf("pending hint[%d] = %v, want %v", i, hints[i], want)
		}
	}
}

func TestKeySequence_FullSequenceDispatches(t *testing.T) {
	b, fe := newActionTestBoard(t, seqActions())

	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedURL := fmt.Sprintf("https://example.com/frontend/%d", selectedCard.Number)

	b = sendKey(t, b, keyMsg("P"))
	b = sendKey(t, b, keyMsg("f"))

	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("expected 1 OpenURL call after full sequence, got %d", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after dispatch, want empty", b.pendingSeq)
	}
}

func TestKeySequence_EscCancelsPending(t *testing.T) {
	b, fe := newActionTestBoard(t, seqActions())

	b = sendKey(t, b, keyMsg("P"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after esc, want empty", b.pendingSeq)
	}
	// The continuation key must no longer complete the cancelled sequence.
	b = sendKey(t, b, keyMsg("f"))
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls after cancelled sequence, got %d", len(fe.OpenURLCalls))
	}
	// Hints are restored to the normal-mode set.
	if len(b.statusBar.hints) == 0 || b.statusBar.hints[len(b.statusBar.hints)-1].Key == "esc" {
		t.Errorf("hints = %v, want normal-mode hints restored", b.statusBar.hints)
	}
}

func TestKeySequence_UnmatchedContinuationCancelsWithWarning(t *testing.T) {
	b, fe := newActionTestBoard(t, seqActions())

	b = sendKey(t, b, keyMsg("P"))
	b = sendKey(t, b, keyMsg("z"))

	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after unmatched continuation, want empty", b.pendingSeq)
	}
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls, got %d", len(fe.OpenURLCalls))
	}
	if b.statusBar.message == "" {
		t.Error("expected a status-bar message after unmatched continuation, got none")
	}
}

func TestKeySequence_BuiltinKeyServesAsContinuation(t *testing.T) {
	// While a sequence is pending, every key belongs to the sequence: "j"
	// must complete "Pj" instead of moving the cursor.
	actions := map[string]config.Action{
		"Pj": {Name: "PR jobs", Type: "url", URL: "https://example.com/jobs/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)
	cursorBefore := b.Columns[b.ActiveTab].Cursor

	b = sendKey(t, b, keyMsg("P"))
	b = sendKey(t, b, keyMsg("j"))

	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("expected 1 OpenURL call for sequence Pj, got %d", len(fe.OpenURLCalls))
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != cursorBefore {
		t.Errorf("cursor = %d, want %d (j inside a sequence must not navigate)", got, cursorBefore)
	}
}

func TestKeySequence_SingleKeyActionsStillDispatchImmediately(t *testing.T) {
	actions := map[string]config.Action{
		"X":  {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
		"Pf": {Name: "PR frontend", Type: "url", URL: "https://example.com/frontend/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("X"))

	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("expected 1 OpenURL call for single-key action, got %d", len(fe.OpenURLCalls))
	}
	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q, want empty after a single-key dispatch", b.pendingSeq)
	}
}

func TestKeySequence_WorksFromDetailFocus(t *testing.T) {
	b, fe := newActionTestBoard(t, seqActions())

	b = sendKey(t, b, keyMsg("l")) // focus detail panel
	if !b.detailFocused {
		t.Fatal("expected detailFocused after l")
	}

	b = sendKey(t, b, keyMsg("P"))
	b = sendKey(t, b, keyMsg("f"))

	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("expected 1 OpenURL call from detail focus, got %d", len(fe.OpenURLCalls))
	}
	if !b.detailFocused {
		t.Error("expected detailFocused to survive a sequence dispatch")
	}
}

func TestKeySequence_EscFromDetailFocusOnlyCancelsSequence(t *testing.T) {
	b, _ := newActionTestBoard(t, seqActions())

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("P"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after esc, want empty", b.pendingSeq)
	}
	if !b.detailFocused {
		t.Error("esc during a pending sequence must cancel the sequence, not leave the detail panel")
	}
}

func TestKeySequence_AltOnPrefixKeyTriggersCommentMode(t *testing.T) {
	actions := map[string]config.Action{
		"Pf": {Name: "PR frontend", Type: "shell", Command: "echo {number} {comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	b = sendKey(t, b, altKeyMsg("P"))
	b = sendKey(t, b, keyMsg("f"))

	if b.mode != commentMode {
		t.Fatalf("mode = %v, want commentMode after alt-prefixed sequence with {comment}", b.mode)
	}
	if b.comment.pendingAction.Name != "PR frontend" {
		t.Errorf("pendingAction.Name = %q, want %q", b.comment.pendingAction.Name, "PR frontend")
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls before comment submit, got %d", len(fe.RunShellCalls))
	}
}

func TestKeySequence_AltOnFinalKeyTriggersCommentMode(t *testing.T) {
	actions := map[string]config.Action{
		"Pf": {Name: "PR frontend", Type: "shell", Command: "echo {number} {comment}"},
	}
	b, _ := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("P"))
	b = sendKey(t, b, altKeyMsg("f"))

	if b.mode != commentMode {
		t.Fatalf("mode = %v, want commentMode after sequence with alt on final key", b.mode)
	}
}

func TestKeySequence_PRScopeGatedPrefixDoesNotEnterPending(t *testing.T) {
	actions := map[string]config.Action{
		"Pf": {Name: "PR frontend", Type: "url", Scope: "pr", URL: "https://example.com/pr/{pr_number}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	// Cursor starts on card 1 (0 linked PRs): the only "P…" candidate is
	// pr-scope and gated, so the prefix must not enter the pending state.
	b = sendKey(t, b, keyMsg("P"))
	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q on a card with no linked PRs, want empty", b.pendingSeq)
	}

	// Card 2 has exactly 1 linked PR: the sequence dispatches against it.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("P"))
	b = sendKey(t, b, keyMsg("f"))
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("expected 1 OpenURL call on the 1-PR card, got %d", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != "https://example.com/pr/10" {
		t.Errorf("OpenURL called with %q, want the URL expanded with the linked PR's number", fe.OpenURLCalls[0])
	}
}

func TestKeySequence_CardScopePrefixIgnoredWhenNoCards(t *testing.T) {
	b, fe := newBoardWithEmptyColumn(t, seqActions())

	b = sendKey(t, b, keyMsg("P"))

	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q with no cards, want empty (card-scope candidates are gated)", b.pendingSeq)
	}
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls, got %d", len(fe.OpenURLCalls))
	}
}

func TestKeySequence_ColumnActionCanExtendPrefix(t *testing.T) {
	columnConfigs := []config.ColumnConfig{
		{Name: "New", Actions: map[string]config.Action{
			"Pn": {Name: "Column sequence", Type: "url", URL: "https://example.com/col/{number}"},
		}},
	}
	b, fe := newColumnActionTestBoard(t, nil, columnConfigs)

	b = sendKey(t, b, keyMsg("P"))
	b = sendKey(t, b, keyMsg("n"))

	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("expected 1 OpenURL call for column-defined sequence, got %d", len(fe.OpenURLCalls))
	}
}

func TestKeySequence_BoardRefreshCancelsPending(t *testing.T) {
	b, fe := newActionTestBoard(t, seqActions())

	b = sendKey(t, b, keyMsg("P"))
	b = simulateRefresh(t, b)

	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after board refresh, want empty", b.pendingSeq)
	}
	// "f" after the refresh is the built-in filter key again, not a
	// continuation of the stale sequence.
	b = sendKey(t, b, keyMsg("f"))
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls after refresh cancelled the sequence, got %d", len(fe.OpenURLCalls))
	}
}

func TestKeySequence_ThreeKeySequenceDispatches(t *testing.T) {
	actions := map[string]config.Action{
		"Pfa": {Name: "Deep sequence", Type: "url", URL: "https://example.com/deep/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("P"))
	if b.pendingSeq != "P" {
		t.Fatalf("pendingSeq = %q, want %q", b.pendingSeq, "P")
	}
	b = sendKey(t, b, keyMsg("f"))
	if b.pendingSeq != "Pf" {
		t.Fatalf("pendingSeq = %q, want %q", b.pendingSeq, "Pf")
	}
	b = sendKey(t, b, keyMsg("a"))

	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("expected 1 OpenURL call for three-key sequence, got %d", len(fe.OpenURLCalls))
	}
}
