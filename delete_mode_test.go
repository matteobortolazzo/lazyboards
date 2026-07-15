package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v68/github"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newDeleteTestBoard creates a loaded Board backed by a fresh FakeProvider
// (no cleanup configured), exposing the FakeProvider so tests can assert
// AddComment/DeleteCard side effects (the Comments map, remaining Columns).
// Card #1 "Setup CI" (the default selected card) has 0 LinkedPRs, so it is a
// valid delete target under the PR-linked gate.
func newDeleteTestBoard(t *testing.T) (Board, *provider.FakeProvider) {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil)
	board, err := p.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	loaded, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	loaded.Width = 120
	loaded.Height = 40
	return loaded, p
}

// --- PR-linked gating ---

func TestDeleteMode_TKey_PRCardGated_StaysNormalModeShowsError(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)
	// Card #2 "One PR" has 1 LinkedPR -- must block deletion.
	b.Columns[b.ActiveTab].Cursor = 1
	gatedCard := b.selectedCard()
	if len(gatedCard.LinkedPRs) == 0 {
		t.Fatal("test precondition: selected card must have >=1 LinkedPR")
	}

	m, cmd := b.Update(keyMsg("t"))
	b = m.(Board)

	if b.mode != normalMode {
		t.Fatalf("mode = %d after 't' on a card with linked PRs, want normalMode", b.mode)
	}
	if b.delete.card.Number != 0 {
		t.Errorf("delete.card.Number = %d, want 0 (delete state untouched by gated attempt)", b.delete.card.Number)
	}
	execCmds(cmd)
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "pr") {
		t.Errorf("View() should show a PR-related gating message, got:\n%s", view)
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no shell calls from a gated delete attempt, got: %v", fe.RunShellCalls)
	}
}

// --- Entering deleteMode via 't' ---

func TestDeleteMode_TKey_EntersDeleteModeWithCommentStep(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	wantCard := b.selectedCard()
	if len(wantCard.LinkedPRs) != 0 {
		t.Fatal("test precondition: selected card must have 0 LinkedPRs")
	}

	m, _ := b.Update(keyMsg("t"))
	b = m.(Board)

	if b.mode != deleteMode {
		t.Fatalf("mode = %d, want deleteMode", b.mode)
	}
	if b.delete.step != deleteStepComment {
		t.Errorf("delete.step = %d, want deleteStepComment", b.delete.step)
	}
	if b.delete.card.Number != wantCard.Number {
		t.Errorf("delete.card.Number = %d, want %d", b.delete.card.Number, wantCard.Number)
	}
	if !b.delete.commentInput.Focused() {
		t.Error("delete.commentInput should be focused when entering the comment step")
	}
}

func TestDeleteMode_TKey_NoColumns_DoesNothing(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil)
	msg := boardFetchedMsg{board: provider.Board{Columns: nil}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	m, cmd := b.Update(keyMsg("t"))
	b = m.(Board)

	if cmd != nil {
		t.Error("expected nil cmd from 't' key when board has no columns")
	}
	if b.mode == deleteMode {
		t.Error("should not enter deleteMode when board has no columns")
	}
}

func TestDeleteMode_TKey_NoVisibleCards_DoesNothing(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil)
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Empty", Cards: nil},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	m, cmd := b.Update(keyMsg("t"))
	b = m.(Board)

	if cmd != nil {
		t.Error("expected nil cmd from 't' key when the active column has no cards")
	}
	if b.mode == deleteMode {
		t.Error("should not enter deleteMode when the active column has no cards")
	}
}

func TestDeleteMode_TKey_DetailFocused_IsNoOp(t *testing.T) {
	// 't' is a normal-mode-only keybinding (per CLAUDE.md convention, mirroring
	// 'x'/Close) and must not be duplicated into the detail-focused sub-mode.
	b, _ := newDeleteTestBoard(t)
	b.detailFocused = true

	m, _ := b.Update(keyMsg("t"))
	b = m.(Board)

	if b.mode == deleteMode {
		t.Error("'t' pressed while detail-focused should not enter deleteMode")
	}
	if !b.detailFocused {
		t.Error("'t' pressed while detail-focused should leave detailFocused unchanged (unhandled key)")
	}
}

func TestDeleteMode_TKey_FilterActive_TargetsFilteredCard(t *testing.T) {
	// Regression guard for #234: card resolution under an active filter must
	// go through selectedCard(), not the raw (unfiltered) column index.
	b := newBoardWithFilterableCards(t)

	// Cards #1, #3, #5 in "Backlog" carry the "bug" label; none of the fixture
	// cards carry LinkedPRs, so the gate always passes here.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"
	b.Columns[b.ActiveTab].Cursor = 1 // second bug card in the filtered list -> #3

	m, _ := b.Update(keyMsg("t"))
	b = m.(Board)

	if b.mode != deleteMode {
		t.Fatalf("mode = %d, want deleteMode", b.mode)
	}
	if b.delete.card.Number != 3 {
		t.Errorf("delete.card.Number = %d, want #3 (filtered selection), not raw index 1 (#2)", b.delete.card.Number)
	}
}

// --- Esc-cancel at either step ---

func TestDeleteMode_EscAtCommentStep_CancelsToNormalModeNoProviderCalls(t *testing.T) {
	b, p := newDeleteTestBoard(t)
	card := b.selectedCard()

	b = sendKey(t, b, keyMsg("t"))
	if b.mode != deleteMode {
		t.Fatalf("precondition: mode = %d, want deleteMode", b.mode)
	}

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after esc at comment step, want normalMode", b.mode)
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "cancel") {
		t.Errorf("View() after esc should contain a cancel message, got:\n%s", view)
	}
	found := false
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			found = true
		}
	}
	if !found {
		t.Error("expected card to remain in Columns after esc-cancel")
	}
	if len(p.Comments[card.Number]) != 0 {
		t.Errorf("expected no comment posted after esc-cancel, got: %v", p.Comments[card.Number])
	}
}

func TestDeleteMode_EnterAtCommentStep_BlankComment_AdvancesToConfirmStep(t *testing.T) {
	b, _ := newDeleteTestBoard(t)

	b = sendKey(t, b, keyMsg("t"))
	m, _ := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	if b.mode != deleteMode {
		t.Fatalf("mode = %d after Enter at comment step, want deleteMode (still in flow)", b.mode)
	}
	if b.delete.step != deleteStepConfirm {
		t.Errorf("delete.step = %d, want deleteStepConfirm", b.delete.step)
	}
	if !b.delete.confirmInput.Focused() {
		t.Error("delete.confirmInput should be focused when entering the confirm step")
	}
}

func TestDeleteMode_EscAtConfirmStep_DiscardsCommentAndCancels(t *testing.T) {
	b, p := newDeleteTestBoard(t)
	card := b.selectedCard()

	b = sendKey(t, b, keyMsg("t"))
	for _, ch := range "a comment that should be discarded" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after esc at confirm step, want normalMode", b.mode)
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "cancel") {
		t.Errorf("View() after esc should contain a cancel message, got:\n%s", view)
	}
	if len(p.Comments[card.Number]) != 0 {
		t.Errorf("expected the step-1 comment to be discarded (never posted), got: %v", p.Comments[card.Number])
	}
	found := false
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			found = true
		}
	}
	if !found {
		t.Error("expected card to remain in Columns after esc-cancel at confirm step")
	}
}

// --- Retype-to-confirm: mismatch stays in step, correct retry proceeds ---

func TestDeleteMode_ConfirmStep_MismatchStaysInStepThenCorrectRetryProceeds(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()

	b = sendKey(t, b, keyMsg("t"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter)) // blank comment -> confirm step

	wrong := strconv.Itoa(card.Number + 999)
	for _, ch := range wrong {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	m, _ := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	if b.mode != deleteMode || b.delete.step != deleteStepConfirm {
		t.Fatalf("mode=%d step=%d after mismatch, want deleteMode/deleteStepConfirm (stay in step)", b.mode, b.delete.step)
	}
	if b.delete.mismatchMsg == "" {
		t.Error("expected a non-empty mismatchMsg after a wrong retype")
	}
	if !strings.Contains(b.delete.confirmInput.Value(), wrong) {
		t.Error("expected the confirm input to retain the mismatched text (not cleared)")
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "match") {
		t.Errorf("View() should show a mismatch indication, got:\n%s", view)
	}

	// Clear the wrong input and retype the correct card number.
	for range wrong {
		b = sendKey(t, b, arrowMsg(tea.KeyBackspace))
	}
	for _, ch := range strconv.Itoa(card.Number) {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after correct retype, want normalMode (optimistic transition)", b.mode)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil cmd after a correct retype confirms the delete")
	}
}

// --- Happy path: no comment ---

func TestDeleteMode_HappyPath_NoComment_RemovesCardAndShowsSuccess(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()

	b = sendKey(t, b, keyMsg("t"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter)) // blank comment -> confirm step
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}
	for _, ch := range strconv.Itoa(card.Number) {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("expected a non-nil cmd after confirming delete with no comment")
	}

	msgs := collectMsgs(cmd)
	if len(msgs) == 0 {
		t.Fatal("expected the confirm cmd to produce at least one message")
	}
	for _, msg := range msgs {
		m, _ = b.Update(msg)
		b = m.(Board)
	}

	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			t.Fatal("expected card removed from Columns after successful delete")
		}
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "delete") {
		t.Errorf("View() after successful delete should show a deletion status message, got:\n%s", view)
	}
}

// --- Happy path: with comment (message-chain ordering) ---

func TestDeleteMode_HappyPath_WithComment_PostsCommentBeforeDelete(t *testing.T) {
	b, p := newDeleteTestBoard(t)
	card := b.selectedCard()

	b = sendKey(t, b, keyMsg("t"))
	const commentText = "cleaning up before delete"
	for _, ch := range commentText {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}
	for _, ch := range strconv.Itoa(card.Number) {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("expected a non-nil cmd after confirming delete with a comment")
	}

	msgs := collectMsgs(cmd)
	if len(msgs) == 0 {
		t.Fatal("expected the confirm cmd to produce at least one message")
	}
	posted, ok := msgs[0].(deleteCommentPostedMsg)
	if !ok {
		t.Fatalf("expected the confirm cmd to first post the comment (deleteCommentPostedMsg), got %T", msgs[0])
	}

	m, cmd2 := b.Update(posted)
	b = m.(Board)
	if cmd2 == nil {
		t.Fatal("expected deleteCommentPostedMsg to chain to deleteCardCmd")
	}

	// The comment must be recorded, and the card must still be present --
	// delete has not fired yet at this point in the chain.
	found := false
	for _, c := range p.Comments[card.Number] {
		if strings.Contains(c, commentText) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected comment %q recorded for card #%d before delete, got: %v", commentText, card.Number, p.Comments[card.Number])
	}
	stillPresent := false
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			stillPresent = true
		}
	}
	if !stillPresent {
		t.Fatal("expected card to still be present immediately after the comment posts, before delete completes")
	}

	msgs2 := collectMsgs(cmd2)
	if len(msgs2) == 0 {
		t.Fatal("expected deleteCardCmd to produce a message")
	}
	deleted, ok := msgs2[0].(cardDeletedMsg)
	if !ok {
		t.Fatalf("expected deleteCardCmd's result to be cardDeletedMsg, got %T", msgs2[0])
	}

	m, _ = b.Update(deleted)
	b = m.(Board)
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			t.Fatal("expected card removed from Columns after cardDeletedMsg")
		}
	}
}

// --- Comment-post failure blocks delete ---

func TestDeleteMode_DeleteCommentErrorMsg_BlocksDeleteReturnsToNormalMode(t *testing.T) {
	b, p := newDeleteTestBoard(t)
	card := b.selectedCard()

	m, _ := b.Update(deleteCommentErrorMsg{err: errSentinel("comment failed")})
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after deleteCommentErrorMsg, want normalMode", b.mode)
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "error") {
		t.Errorf("View() after deleteCommentErrorMsg should contain an error message, got:\n%s", view)
	}
	found := false
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			found = true
		}
	}
	if !found {
		t.Error("expected card to remain in Columns after comment-post failure (delete must not proceed)")
	}
	if len(p.Comments[card.Number]) != 0 {
		t.Errorf("expected no comment recorded after simulated comment failure, got: %v", p.Comments[card.Number])
	}
}

// --- Delete-permission failure ---

func TestDeleteMode_CardDeleteErrorMsg_SanitizesGitHubAPIErrorCardUntouched(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()

	rawMessage := "internal trace: panic at /srv/app/internal/db.go:99 token=SECRET"
	ghErr := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusForbidden},
		Message:  rawMessage,
	}

	m, _ := b.Update(cardDeleteErrorMsg{err: ghErr})
	b = m.(Board)

	if strings.Contains(b.statusBar.message, rawMessage) {
		t.Errorf("status bar message leaked raw internal error text, got: %q", b.statusBar.message)
	}
	if b.statusBar.message == "" {
		t.Error("expected a sanitized status bar message after cardDeleteErrorMsg")
	}
	if b.mode != normalMode {
		t.Errorf("mode = %d after cardDeleteErrorMsg, want normalMode", b.mode)
	}
	found := false
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			found = true
		}
	}
	if !found {
		t.Error("expected card to remain in Columns after a delete-permission failure")
	}
}

// --- handleCardDeleted: mirrors handleCardClosed's full cleanup-guard precedence ---

func TestDeleteMode_CardDeleted_NoGuard_RemovesCardAndCleansUp(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	card := b.Columns[0].Cards[0] // card #1 "Setup CI" in column "New" (cleanup configured)
	m, cmd := b.Update(cardDeletedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call for deleted card with no guard active, got none")
	}
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			t.Fatal("expected card removed from Columns after cardDeletedMsg")
		}
	}
	if _, exists := b.prevCards[card.Number]; exists {
		t.Error("expected prevCards entry deleted after delete")
	}
}

func TestDeleteMode_CardDeleted_AgentBusy_SkipsCleanupButRemovesCard(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")
	b.agentSnapshot = cleanupSnapshot("running")

	card := b.Columns[0].Cards[0]
	m, cmd := b.Update(cardDeletedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected cleanup blocked while agent busy, got: %v", fe.RunShellCalls)
	}
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			t.Fatal("expected card removed from Columns even when cleanup guard blocks")
		}
	}
	if _, exists := b.prevCards[card.Number]; exists {
		t.Error("expected prevCards entry deleted after delete, even though guard blocked cleanup")
	}
}

func TestDeleteMode_CardDeleted_AgentwatchEnabledSnapshotNotYetDelivered_SkipsCleanupButRemovesCard(t *testing.T) {
	b, fe, _ := newCleanupTestBoardWithWatcher(t, "tmux kill-window -t {session}")

	card := b.Columns[0].Cards[0]
	m, cmd := b.Update(cardDeletedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected cleanup blocked while agentwatch snapshot is nil, got: %v", fe.RunShellCalls)
	}
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			t.Fatal("expected card removed from Columns even when the fail-closed guard blocks cleanup")
		}
	}
	if _, exists := b.prevCards[card.Number]; exists {
		t.Error("expected prevCards entry deleted after delete, even though the fail-closed guard blocked cleanup")
	}
}

func TestDeleteMode_CardDeleted_WorkingLabel_SkipsCleanupButRemovesCard(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	card := b.Columns[0].Cards[0]
	card.Labels = append(card.Labels, Label{Name: "working"}) // case-insensitive match to configured workingLabel "Working"
	m, cmd := b.Update(cardDeletedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected cleanup blocked while working label present, got: %v", fe.RunShellCalls)
	}
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			t.Fatal("expected card removed from Columns even when cleanup guard blocks")
		}
	}
	if _, exists := b.prevCards[card.Number]; exists {
		t.Error("expected prevCards entry deleted after delete, even though guard blocked cleanup")
	}
}

func TestDeleteMode_CardDeleted_BypassesMissingCardDebounce(t *testing.T) {
	// Unlike a card vanishing during a background fetch -- which requires two
	// consecutive misses before cleanup fires -- an explicit delete via
	// 't'/retype-confirm must clean up on the very first cardDeletedMsg, with
	// no debounce.
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	card := b.Columns[0].Cards[0]
	m, cmd := b.Update(cardDeletedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call on the very first delete (no debounce), got none")
	}
}

func TestDeleteMode_CardDeleted_CursorOnLastCard_ClampsAndDoesNotPanic(t *testing.T) {
	// Regression guard: deleting the card the cursor sits on, when it is the
	// last card in the column, must clamp Cursor to the new (shorter) length.
	b, _, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	lastIdx := len(b.Columns[0].Cards) - 1
	b.Columns[0].Cursor = lastIdx
	card := b.Columns[0].Cards[lastIdx]

	m, cmd := b.Update(cardDeletedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if b.Columns[0].Cursor >= len(b.Columns[0].Cards) {
		t.Fatalf("Cursor = %d after deleting last card, want < %d (clamped)", b.Columns[0].Cursor, len(b.Columns[0].Cards))
	}
	if b.Columns[0].Cursor < 0 {
		t.Fatalf("Cursor = %d after deleting last card, want >= 0", b.Columns[0].Cursor)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("b.View() panicked after deleting last card with cursor on it: %v", r)
		}
	}()
	_ = b.View()
}

// --- View rendering ---

func TestDeleteMode_ViewShowsCardNumberAndStepPrompt(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()

	m, _ := b.Update(keyMsg("t"))
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, fmt.Sprintf("#%d", card.Number)) {
		t.Errorf("View() in deleteMode should show the card number, got:\n%s", view)
	}
	if !strings.Contains(strings.ToLower(view), "delete") {
		t.Errorf("View() in deleteMode should show delete-related prompt text, got:\n%s", view)
	}
}

// --- Keybinding hint registration (CLAUDE.md hard rule) ---

func TestHelpSections_NormalMode_ContainsDeleteCardHint(t *testing.T) {
	for _, section := range helpSections {
		if section.title != "Normal Mode" {
			continue
		}
		for _, kv := range section.keys {
			if kv[0] == "t" {
				return
			}
		}
		t.Fatalf("helpSections[%q] does not contain an entry for key %q", "Normal Mode", "t")
	}
	t.Fatal(`helpSections has no "Normal Mode" section`)
}
