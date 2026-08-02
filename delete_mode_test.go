package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v68/github"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)
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

// failingDeleteProvider wraps a FakeProvider so DeleteCard always fails with a
// fixed error, while every other method (notably AddComment) behaves exactly
// like the embedded FakeProvider. This lets tests exercise the real
// deleteCommentPostedMsg -> deleteCardCmd production chain and observe
// exactly what commentPosted value that chain produces, without conflating a
// comment-post failure with a delete failure (a plain FakeProvider can't
// fail DeleteCard alone while leaving AddComment working, since both key off
// the same findCard lookup).
type failingDeleteProvider struct {
	*provider.FakeProvider
	deleteErr error
}

func (f *failingDeleteProvider) DeleteCard(_ context.Context, _ int) error {
	return f.deleteErr
}

// newDeleteTestBoardWithFailingDelete mirrors newDeleteTestBoard but backs
// the Board with a failingDeleteProvider, so DeleteCard always fails with
// deleteErr regardless of which card is targeted.
func newDeleteTestBoardWithFailingDelete(t *testing.T, deleteErr error) (Board, *failingDeleteProvider) {
	t.Helper()
	fp := provider.NewFakeProvider()
	fd := &failingDeleteProvider{FakeProvider: fp, deleteErr: deleteErr}
	b := NewBoard(fd, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)
	board, err := fp.FetchBoard(context.Background())
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
	return loaded, fd
}

// waitClearedWithin runs cmd (the tea.Cmd returned from a StatusBar
// SetTimedMessage call, e.g. via Update()) in a goroutine and reports whether
// it produced its clearStatusMsg within timeout. It never blocks the test for
// longer than timeout, so it can distinguish the short statusMessageDuration
// path from the long longStatusMessageDuration path without ever waiting out
// the full 30s duration: a timeout set just past statusMessageDuration is
// enough, since those are the only two durations this feature uses.
func waitClearedWithin(t *testing.T, cmd tea.Cmd, timeout time.Duration) bool {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a non-nil cmd carrying the timed-message clear tick")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		if _, ok := msg.(clearStatusMsg); !ok {
			t.Fatalf("expected cmd to produce a clearStatusMsg, got %T", msg)
		}
		return true
	case <-time.After(timeout):
		return false
	}
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

// TestDeleteMode_CommentStepPrompt_FlattensEmbeddedNewlineInTitle covers the
// delete modal's comment-step prompt (#500). Like the close-confirm helpBar,
// the prompt renders card.Title through fmt.Sprintf's %q verb, which already
// escapes a raw newline to the two-character `\n` literal -- so the
// meaningful assertion is the *flattened* form ("line one line two"), not
// merely "one physical line" (which %q already guarantees on its own).
func TestDeleteMode_CommentStepPrompt_FlattensEmbeddedNewlineInTitle(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].Title = "line one\nline two"

	m, _ := b.Update(keyMsg("t"))
	b = m.(Board)
	if b.delete.step != deleteStepComment {
		t.Fatalf("precondition: step = %d, want deleteStepComment", b.delete.step)
	}

	view := b.viewDeleteModal()
	if !strings.Contains(view, "line one line two") {
		t.Errorf("viewDeleteModal() = %q, want the comment-step prompt to render the flattened title \"line one line two\", not the %%q-escaped \\n literal", view)
	}
}

// TestDeleteMode_ConfirmStepPrompt_FlattensEmbeddedNewlineInTitle covers the
// same %q prompt gap for the retype-to-confirm step.
func TestDeleteMode_ConfirmStepPrompt_FlattensEmbeddedNewlineInTitle(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].Title = "line one\nline two"

	m, _ := b.Update(keyMsg("t"))
	b = m.(Board)
	m, _ = b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}

	view := b.viewDeleteModal()
	if !strings.Contains(view, "line one line two") {
		t.Errorf("viewDeleteModal() = %q, want the confirm-step prompt to render the flattened title \"line one line two\", not the %%q-escaped \\n literal", view)
	}
}

func TestDeleteMode_TKey_NoColumns_DoesNothing(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)
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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)
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

	// commentPosted is explicitly false: this is the direct no-comment path
	// (blank comment -> deleteCardCmd called straight from
	// handleDeleteModeKey), which must render the existing generic message
	// unchanged (#376).
	m, cmd := b.Update(cardDeleteErrorMsg{err: ghErr, commentPosted: false})
	b = m.(Board)

	if strings.Contains(b.statusBar.message, rawMessage) {
		t.Errorf("status bar message leaked raw internal error text, got: %q", b.statusBar.message)
	}
	wantMsg := "Delete error: " + provider.SanitizeError(ghErr)
	if b.statusBar.message != wantMsg {
		t.Errorf("statusBar.message = %q, want %q", b.statusBar.message, wantMsg)
	}
	if b.statusBar.level != StatusError {
		t.Errorf("statusBar.level = %d, want StatusError", b.statusBar.level)
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

	// commentPosted=false must keep the existing short duration, not the new
	// long duration reserved for the partial-success (comment-posted) path.
	threshold := statusMessageDuration + 500*time.Millisecond
	if !waitClearedWithin(t, cmd, threshold) {
		t.Errorf("expected the generic delete-error message to clear within statusMessageDuration (%s); commentPosted=false must not use longStatusMessageDuration (%s)", statusMessageDuration, longStatusMessageDuration)
	}
}

// sensitiveFieldError is a test-local error type standing in for a
// non-GitHub error whose Error() string is safe to display but which may
// carry additional sensitive fields (e.g. a token embedded in a URL) that
// must never be surfaced by SanitizeError's raw-fallback path (#389).
type sensitiveFieldError struct {
	safe   string // returned by Error() — safe to display
	secret string // sensitive detail (e.g. token-in-URL) NOT surfaced by Error()
}

func (e sensitiveFieldError) Error() string { return e.safe }

// TestDeleteMode_CardDeleteErrorMsg_SanitizesNonGitHubError_NoFieldLeak locks
// in SanitizeError's raw-fallback path (non-GitHub errors) for the
// commentPosted=false rendering: the status bar must route the error through
// provider.SanitizeError and must never leak fields beyond what Error()
// returns (#389).
func TestDeleteMode_CardDeleteErrorMsg_SanitizesNonGitHubError_NoFieldLeak(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()

	raw := sensitiveFieldError{
		safe:   "delete failed",
		secret: "https://ghp_SECRETTOKEN@api.github.com/repos/x",
	}

	m, cmd := b.Update(cardDeleteErrorMsg{err: raw, commentPosted: false})
	b = m.(Board)

	if strings.Contains(b.statusBar.message, "ghp_SECRETTOKEN") {
		t.Errorf("status bar message leaked the sensitive field, got: %q", b.statusBar.message)
	}
	wantMsg := "Delete error: " + provider.SanitizeError(raw)
	if b.statusBar.message != wantMsg {
		t.Errorf("statusBar.message = %q, want %q", b.statusBar.message, wantMsg)
	}
	if strings.Contains(b.statusBar.message, "API error") {
		t.Errorf("expected the non-GitHub fallback branch, not the GitHub API mapping, got: %q", b.statusBar.message)
	}
	if b.statusBar.level != StatusError {
		t.Errorf("statusBar.level = %d, want StatusError", b.statusBar.level)
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

	threshold := statusMessageDuration + 500*time.Millisecond
	if !waitClearedWithin(t, cmd, threshold) {
		t.Errorf("expected the generic delete-error message to clear within statusMessageDuration (%s); commentPosted=false must not use longStatusMessageDuration (%s)", statusMessageDuration, longStatusMessageDuration)
	}
}

// TestDeleteMode_CardDeleteErrorMsg_CommentPosted_SanitizesNonGitHubError_NoFieldLeak
// is the commentPosted=true sibling of the test above: it locks in the same
// raw-fallback sanitization guarantee for the partial-success rendering path
// (#389).
func TestDeleteMode_CardDeleteErrorMsg_CommentPosted_SanitizesNonGitHubError_NoFieldLeak(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()

	raw := sensitiveFieldError{
		safe:   "delete failed",
		secret: "https://ghp_SECRETTOKEN@api.github.com/repos/x",
	}

	m, cmd := b.Update(cardDeleteErrorMsg{err: raw, commentPosted: true})
	b = m.(Board)

	if strings.Contains(b.statusBar.message, "ghp_SECRETTOKEN") {
		t.Errorf("status bar message leaked the sensitive field, got: %q", b.statusBar.message)
	}
	wantMsg := "Comment posted, but delete failed: " + provider.SanitizeError(raw)
	if b.statusBar.message != wantMsg {
		t.Errorf("statusBar.message = %q, want %q", b.statusBar.message, wantMsg)
	}
	if b.statusBar.level != StatusError {
		t.Errorf("statusBar.level = %d, want StatusError", b.statusBar.level)
	}
	if b.mode != normalMode {
		t.Errorf("mode = %d after commentPosted cardDeleteErrorMsg, want normalMode", b.mode)
	}
	found := false
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			found = true
		}
	}
	if !found {
		t.Error("expected the card to remain in Columns after a partial-success delete failure")
	}

	threshold := statusMessageDuration + 500*time.Millisecond
	if waitClearedWithin(t, cmd, threshold) {
		t.Errorf("expected the partial-success message to persist past statusMessageDuration (%s); commentPosted=true must use longStatusMessageDuration (%s), not the short default", statusMessageDuration, longStatusMessageDuration)
	}
}

// TestDeleteMode_CardDeleteErrorMsg_CommentPosted_SanitizesAndShowsPartialSuccessMessage
// is the commentPosted=true sibling of the sanitization test above: when the
// comment successfully posted before DeleteCard failed, the status bar must
// show a distinct "partial success" message (not the generic "Delete error:"
// text), still sanitized, and held for the long duration so the user has time
// to notice the comment landed but the card was not removed (#376).
func TestDeleteMode_CardDeleteErrorMsg_CommentPosted_SanitizesAndShowsPartialSuccessMessage(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()

	rawMessage := "internal trace: panic at /srv/app/internal/db.go:99 token=SECRET"
	ghErr := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusForbidden},
		Message:  rawMessage,
	}

	m, cmd := b.Update(cardDeleteErrorMsg{err: ghErr, commentPosted: true})
	b = m.(Board)

	if strings.Contains(b.statusBar.message, rawMessage) {
		t.Errorf("status bar message leaked raw internal error text, got: %q", b.statusBar.message)
	}
	wantMsg := "Comment posted, but delete failed: " + provider.SanitizeError(ghErr)
	if b.statusBar.message != wantMsg {
		t.Errorf("statusBar.message = %q, want %q", b.statusBar.message, wantMsg)
	}
	if b.statusBar.message == "Delete error: "+provider.SanitizeError(ghErr) {
		t.Error("expected the commentPosted=true message to be distinct from the generic 'Delete error:' text")
	}
	if b.statusBar.level != StatusError {
		t.Errorf("statusBar.level = %d, want StatusError", b.statusBar.level)
	}
	if b.mode != normalMode {
		t.Errorf("mode = %d after commentPosted cardDeleteErrorMsg, want normalMode", b.mode)
	}
	found := false
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			found = true
		}
	}
	if !found {
		t.Error("expected the card to remain in Columns after a partial-success delete failure")
	}

	// commentPosted=true must use the long duration, not the default 3s.
	threshold := statusMessageDuration + 500*time.Millisecond
	if waitClearedWithin(t, cmd, threshold) {
		t.Errorf("expected the partial-success message to persist past statusMessageDuration (%s); commentPosted=true must use longStatusMessageDuration (%s), not the short default", statusMessageDuration, longStatusMessageDuration)
	}
}

// --- Partial-success: comment posts, then delete fails (full production chain) ---
//
// These two tests exercise the actual handleDeleteModeKey -> [commentCmd ->]
// deleteCardCmd production wiring (not a hand-constructed message), to verify
// the real code sets cardDeleteErrorMsg.commentPosted correctly depending on
// which path was taken (#376).

func TestDeleteMode_HappyPath_WithComment_DeleteFailsAfterCommentPosted_ShowsPartialSuccess(t *testing.T) {
	deleteErr := errSentinel("delete failed after comment posted")
	b, fd := newDeleteTestBoardWithFailingDelete(t, deleteErr)
	card := b.selectedCard()

	b = sendKey(t, b, keyMsg("t"))
	const commentText = "cleaning up before delete"
	for _, ch := range commentText {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter)) // -> confirm step
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
	if len(fd.Comments[card.Number]) == 0 {
		t.Fatalf("expected the comment to actually post before the delete attempt, got: %v", fd.Comments[card.Number])
	}

	m, cmd2 := b.Update(posted)
	b = m.(Board)
	if cmd2 == nil {
		t.Fatal("expected deleteCommentPostedMsg to chain to deleteCardCmd")
	}

	msgs2 := collectMsgs(cmd2)
	if len(msgs2) == 0 {
		t.Fatal("expected deleteCardCmd to produce a message")
	}
	deleteErrMsg, ok := msgs2[0].(cardDeleteErrorMsg)
	if !ok {
		t.Fatalf("expected deleteCardCmd's result to be cardDeleteErrorMsg (DeleteCard always fails in this test), got %T", msgs2[0])
	}
	if !deleteErrMsg.commentPosted {
		t.Error("expected cardDeleteErrorMsg.commentPosted = true when the failure is reached via the comment-then-delete chain")
	}

	m, cmd3 := b.Update(deleteErrMsg)
	b = m.(Board)

	wantMsg := "Comment posted, but delete failed: " + provider.SanitizeError(deleteErrMsg.err)
	if b.statusBar.message != wantMsg {
		t.Errorf("statusBar.message = %q, want %q", b.statusBar.message, wantMsg)
	}
	if b.statusBar.level != StatusError {
		t.Errorf("statusBar.level = %d, want StatusError", b.statusBar.level)
	}
	if b.mode != normalMode {
		t.Errorf("mode = %d after partial-success cardDeleteErrorMsg, want normalMode", b.mode)
	}
	found := false
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			found = true
		}
	}
	if !found {
		t.Error("expected the card to remain in Columns after a partial-success delete failure (comment landed but delete failed)")
	}

	threshold := statusMessageDuration + 500*time.Millisecond
	if waitClearedWithin(t, cmd3, threshold) {
		t.Errorf("expected the partial-success message to persist past statusMessageDuration (%s); it must use longStatusMessageDuration (%s), not the short default", statusMessageDuration, longStatusMessageDuration)
	}
}

func TestDeleteMode_HappyPath_NoComment_DeleteFails_ShowsGenericErrorWithShortDuration(t *testing.T) {
	deleteErr := errSentinel("delete failed, no comment involved")
	b, _ := newDeleteTestBoardWithFailingDelete(t, deleteErr)
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
	deleteErrMsg, ok := msgs[0].(cardDeleteErrorMsg)
	if !ok {
		t.Fatalf("expected the no-comment confirm cmd to call deleteCardCmd directly and fail with cardDeleteErrorMsg, got %T", msgs[0])
	}
	if deleteErrMsg.commentPosted {
		t.Error("expected cardDeleteErrorMsg.commentPosted = false on the direct no-comment path")
	}

	m, cmd2 := b.Update(deleteErrMsg)
	b = m.(Board)

	wantMsg := "Delete error: " + provider.SanitizeError(deleteErrMsg.err)
	if b.statusBar.message != wantMsg {
		t.Errorf("statusBar.message = %q, want %q", b.statusBar.message, wantMsg)
	}
	if b.statusBar.level != StatusError {
		t.Errorf("statusBar.level = %d, want StatusError", b.statusBar.level)
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
		t.Error("expected the card to remain in Columns after a delete failure")
	}

	threshold := statusMessageDuration + 500*time.Millisecond
	if !waitClearedWithin(t, cmd2, threshold) {
		t.Errorf("expected the generic delete-error message to clear within statusMessageDuration (%s), unchanged existing behavior", statusMessageDuration)
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

func TestDeleteMode_CardDeleted_CenciEnabledSnapshotNotYetDelivered_SkipsCleanupButRemovesCard(t *testing.T) {
	b, fe, _ := newCleanupTestBoardWithWatcher(t, "tmux kill-window -t {session}")

	card := b.Columns[0].Cards[0]
	m, cmd := b.Update(cardDeletedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected cleanup blocked while cenci snapshot is nil, got: %v", fe.RunShellCalls)
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

// --- Sanitized rendering of untrusted card.Title ---

// TestDeleteMode_ViewSanitizesControlSequencesInTitle covers the same
// GitHub-sourced-untrusted-content gap for the deleteMode modal prompts
// (both the comment step and the confirm/retype step): card.Title is
// rendered via fmt.Sprintf's %q verb, which currently escapes control bytes
// to visible literal text, but must still route through
// sanitizeControlSequences for consistency with every other card.Title
// render site (cardDisplayText, composeDetailMarkdown). A malicious title
// must not leak raw ESC/BEL control bytes into either rendered prompt, while
// the visible text is retained.
func TestDeleteMode_ViewSanitizesControlSequencesInTitle(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].Title = "\x1b[31mRED\x1b[0m title\x07"

	// Comment step (deleteStepComment).
	b = sendKey(t, b, keyMsg("t"))
	if b.delete.step != deleteStepComment {
		t.Fatalf("precondition: step = %d, want deleteStepComment", b.delete.step)
	}
	view := b.View()
	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("View() at deleteStepComment = %q, want no ESC (0x1b) byte", view)
	}
	if strings.ContainsRune(view, '\x07') {
		t.Errorf("View() at deleteStepComment = %q, want no BEL (0x07) byte", view)
	}
	if !strings.Contains(view, "RED title") {
		t.Errorf("View() at deleteStepComment = %q, want visible title text %q retained", view, "RED title")
	}

	// Confirm/retype step (deleteStepConfirm).
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}
	view = b.View()
	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("View() at deleteStepConfirm = %q, want no ESC (0x1b) byte", view)
	}
	if strings.ContainsRune(view, '\x07') {
		t.Errorf("View() at deleteStepConfirm = %q, want no BEL (0x07) byte", view)
	}
	if !strings.Contains(view, "RED title") {
		t.Errorf("View() at deleteStepConfirm = %q, want visible title text %q retained", view, "RED title")
	}
}

// --- Registry dispatch seam (#539 PR 2/2) ---
//
// handleDeleteModeKey cuts over from a hardcoded `msg.Type == tea.KeyEsc`
// pre-check + per-step `msg.Type == tea.KeyEnter` branch to
// keymap.Keymap.Lookup against ModeDelete (textBinding, keymap_text.go),
// mirroring close_confirm/label_confirm's PR1 cutover. delete diverges from
// that seam in one deliberate way: every key that is NOT a recognized
// command (delete.submit/delete.cancel) -- whether genuinely unrecognized OR
// resolved to a non-command (BindingAction) binding -- must fall through to
// the active step's textinput.Update(msg), not no-op, byte-identical to
// today's behavior where everything except esc/enter reaches the textinput.

// --- Remap: rebinding delete.submit's key changes dispatch AND the hint bar, at both steps ---

func TestDeleteMode_RemapSubmitKey_CommentStep_DispatchAndHintStaySync(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {
			"enter": keymap.UnboundBinding(),
			"tab":   keymap.CommandBinding(keymap.CommandDeleteSubmit),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("t"))
	if b.mode != deleteMode || b.delete.step != deleteStepComment {
		t.Fatalf("precondition: mode=%d step=%d, want deleteMode/deleteStepComment", b.mode, b.delete.step)
	}

	foundNew, foundOld := false, false
	for _, h := range b.statusBar.hints {
		if h.Key == "tab" && h.Desc == "Continue" {
			foundNew = true
		}
		if h.Key == "enter" {
			foundOld = true
		}
	}
	if !foundNew {
		t.Errorf("statusBar hints missing remapped 'tab'/Continue entry, got %+v", b.statusBar.hints)
	}
	if foundOld {
		t.Errorf("statusBar hints still advertise the old 'enter' key after remap, got %+v", b.statusBar.hints)
	}

	// Old 'enter' (now unbound) must no longer advance the step.
	before := b.mode
	beforeStep := b.delete.step
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before || b2.delete.step != beforeStep {
		t.Errorf("mode/step after old (now unbound) enter = %d/%d, want unchanged %d/%d", b2.mode, b2.delete.step, before, beforeStep)
	}
	if cmd != nil {
		t.Error("unbound enter should not fire a cmd")
	}

	// New 'tab' key advances comment -> confirm.
	m, cmd = b.Update(arrowMsg(tea.KeyTab))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.delete.step != deleteStepConfirm {
		t.Errorf("step after remapped 'tab' = %d, want deleteStepConfirm", b3.delete.step)
	}
	if cmd == nil {
		t.Error("remapped 'tab' advancing to the confirm step should focus confirmInput (non-nil cmd)")
	}
}

func TestDeleteMode_RemapSubmitKey_ConfirmStep_DispatchAndHintStaySync(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {
			"enter": keymap.UnboundBinding(),
			"tab":   keymap.CommandBinding(keymap.CommandDeleteSubmit),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("t"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // advance comment -> confirm via the remapped key
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}

	foundNew, foundOld := false, false
	for _, h := range b.statusBar.hints {
		if h.Key == "tab" && h.Desc == "Confirm" {
			foundNew = true
		}
		if h.Key == "enter" {
			foundOld = true
		}
	}
	if !foundNew {
		t.Errorf("statusBar hints missing remapped 'tab'/Confirm entry, got %+v", b.statusBar.hints)
	}
	if foundOld {
		t.Errorf("statusBar hints still advertise the old 'enter' key after remap, got %+v", b.statusBar.hints)
	}

	for _, ch := range strconv.Itoa(card.Number) {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Old 'enter' (now unbound) must no longer confirm.
	before := b.mode
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after old (now unbound) enter = %v, want unchanged (%v)", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unbound enter should not fire a cmd")
	}

	// New 'tab' key confirms the delete.
	m, cmd = b.Update(arrowMsg(tea.KeyTab))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.mode != normalMode {
		t.Errorf("mode after remapped 'tab' confirm = %v, want normalMode", b3.mode)
	}
	if cmd == nil {
		t.Fatal("remapped 'tab' should fire the delete cmd")
	}
}

// --- Unbound enter/esc: fall through to the textinput, not a no-op-only path ---

func TestDeleteMode_CommentStep_UnboundEnter_FallsThroughToTextinput(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {"enter": keymap.UnboundBinding()},
	}, nil)
	b = sendKey(t, b, keyMsg("t"))
	if b.delete.step != deleteStepComment {
		t.Fatalf("precondition: step = %d, want deleteStepComment", b.delete.step)
	}

	before := b.delete.commentInput.Value()
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != deleteMode || b2.delete.step != deleteStepComment {
		t.Errorf("mode/step after unbound enter = %d/%d, want unchanged deleteMode/deleteStepComment", b2.mode, b2.delete.step)
	}
	if cmd != nil {
		t.Error("unbound enter should not fire a cmd")
	}
	if b2.delete.commentInput.Value() != before {
		t.Errorf("commentInput.Value() = %q after unbound enter, want unchanged %q (Enter carries no runes, so textinput passthrough is a genuine no-op, byte-identical to today)", b2.delete.commentInput.Value(), before)
	}
}

func TestDeleteMode_CommentStep_UnboundEsc_FallsThroughToTextinputNotCancel(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {"esc": keymap.UnboundBinding()},
	}, nil)
	b = sendKey(t, b, keyMsg("t"))
	if b.delete.step != deleteStepComment {
		t.Fatalf("precondition: step = %d, want deleteStepComment", b.delete.step)
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != deleteMode || b2.delete.step != deleteStepComment {
		t.Errorf("mode/step after unbound esc = %d/%d, want unchanged deleteMode/deleteStepComment (esc must no longer cancel once unbound)", b2.mode, b2.delete.step)
	}
	if cmd != nil {
		t.Error("unbound esc should not fire a cmd")
	}
}

func TestDeleteMode_ConfirmStep_UnboundEnter_FallsThroughToTextinputNotConfirm(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()

	// Reach the confirm step via the default table (enter is still bound),
	// then unbind it -- rather than constructing deleteState mid-flow -- so
	// this exercises the real handler's step-advance path too.
	b = sendKey(t, b, keyMsg("t"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {"enter": keymap.UnboundBinding()},
	}, nil)

	for _, ch := range strconv.Itoa(card.Number) {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	before := b.mode
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before || b2.delete.step != deleteStepConfirm {
		t.Errorf("mode/step after unbound enter = %v/%d, want unchanged %v/deleteStepConfirm", b2.mode, b2.delete.step, before)
	}
	if cmd != nil {
		t.Error("unbound enter should not fire a cmd (the delete must not proceed)")
	}
}

func TestDeleteMode_ConfirmStep_UnboundEsc_FallsThroughToTextinputNotCancel(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {"esc": keymap.UnboundBinding()},
	}, nil)
	b = sendKey(t, b, keyMsg("t"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter)) // enter is still bound by default -> confirm step
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != deleteMode || b2.delete.step != deleteStepConfirm {
		t.Errorf("mode/step after unbound esc = %d/%d, want unchanged deleteMode/deleteStepConfirm (esc must no longer cancel once unbound)", b2.mode, b2.delete.step)
	}
	if cmd != nil {
		t.Error("unbound esc should not fire a cmd")
	}
}

// --- Resolved-but-non-command binding: falls through to the textinput too ---
//
// This is the seam's deliberate divergence from close_confirm/label_confirm:
// there, a resolved BindingAction result is a plain no-op (mode_handlers.go's
// `!ok || binding.Kind != keymap.BindingCommand` guard returns b, nil).
// delete must instead still route the keypress to the active step's
// textinput, since delete's ConsumesPrintableRunes()==true handler owns
// every keystroke that isn't one of its two commands.

func TestDeleteMode_CommentStep_EscBoundToAction_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {
			"esc": keymap.ActionBinding(keymap.Action{Name: "Noop", Type: "url", URL: "https://example.com/{number}"}),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("t"))
	if b.delete.step != deleteStepComment {
		t.Fatalf("precondition: step = %d, want deleteStepComment", b.delete.step)
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != deleteMode || b2.delete.step != deleteStepComment {
		t.Errorf("mode/step after esc resolved to a non-command action = %d/%d, want unchanged deleteMode/deleteStepComment", b2.mode, b2.delete.step)
	}
	if cmd != nil {
		t.Error("esc resolved to a non-command action should not fire a cmd (must fall through to the textinput, not dispatch the action)")
	}
}

func TestDeleteMode_ConfirmStep_EnterBoundToAction_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b, _ := newDeleteTestBoard(t)

	// Reach the confirm step via the default table (enter is still bound to
	// delete.submit), then rebind enter to a non-command action -- rather
	// than constructing deleteState mid-flow -- so this exercises the real
	// handler's step-advance path too.
	b = sendKey(t, b, keyMsg("t"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {
			"enter": keymap.ActionBinding(keymap.Action{Name: "Noop", Type: "url", URL: "https://example.com/{number}"}),
		},
	}, nil)

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != deleteMode || b2.delete.step != deleteStepConfirm {
		t.Errorf("mode/step after enter resolved to a non-command action = %d/%d, want unchanged deleteMode/deleteStepConfirm", b2.mode, b2.delete.step)
	}
	if cmd != nil {
		t.Error("enter resolved to a non-command action should not fire a cmd (must fall through to the textinput, not dispatch the action, and must not confirm the delete)")
	}
}

// --- Foreign command id bound into ModeDelete: falls through, not dispatched ---
//
// A misconfigured/creative user config can bind a command id that is valid
// elsewhere in the catalog (e.g. close_confirm.confirm) into ModeDelete's
// table. handleDeleteModeKey only dispatches when the resolved binding's
// command id is exactly delete.submit or delete.cancel, so a foreign id must
// fall through to the textinput like any other non-dispatchable binding.
// Bound to a named key (f1), not a bare printable rune, since ModeDelete's
// ConsumesPrintableRunes guard makes bare-rune bindings unreachable via
// textBinding before Lookup ever sees them.

func TestDeleteMode_CommentStep_ForeignCommandIDBoundToKey_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {
			"f1": keymap.CommandBinding(keymap.CommandCloseConfirmConfirm),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("t"))
	if b.delete.step != deleteStepComment {
		t.Fatalf("precondition: step = %d, want deleteStepComment", b.delete.step)
	}

	m, cmd := b.Update(arrowMsg(tea.KeyF1))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != deleteMode || b2.delete.step != deleteStepComment {
		t.Errorf("mode/step after a foreign command id (close_confirm.confirm) bound into ModeDelete = %d/%d, want unchanged deleteMode/deleteStepComment", b2.mode, b2.delete.step)
	}
	if cmd != nil {
		t.Error("a foreign command id bound into ModeDelete must fall through to the textinput, not dispatch as submit/cancel")
	}
}

func TestDeleteMode_ConfirmStep_ForeignCommandIDBoundToKey_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b, _ := newDeleteTestBoard(t)

	// Reach the confirm step via the default table (enter is still bound to
	// delete.submit), then rebind a named key to a foreign command id --
	// rather than constructing deleteState mid-flow -- so this exercises the
	// real handler's step-advance path too.
	b = sendKey(t, b, keyMsg("t"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {
			"f1": keymap.CommandBinding(keymap.CommandCloseConfirmConfirm),
		},
	}, nil)

	m, cmd := b.Update(arrowMsg(tea.KeyF1))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != deleteMode || b2.delete.step != deleteStepConfirm {
		t.Errorf("mode/step after a foreign command id (close_confirm.confirm) bound into ModeDelete = %d/%d, want unchanged deleteMode/deleteStepConfirm", b2.mode, b2.delete.step)
	}
	if cmd != nil {
		t.Error("a foreign command id bound into ModeDelete must fall through to the textinput, not dispatch as submit/cancel, and must not confirm the delete")
	}
}

// --- Textinput passthrough: command-bound-elsewhere runes insert literally ---

// TestDeleteMode_TypedCommentPassesThroughLiteralRunes_NoDispatch proves
// textBinding's ConsumesPrintableRunes guard end-to-end through the real
// handler: "y"/"n" are commands in close_confirm/label_confirm, and
// "d"/"x"/"g" are commands in normal mode, but none of that may leak into
// delete mode's comment textinput.
func TestDeleteMode_TypedCommentPassesThroughLiteralRunes_NoDispatch(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b = sendKey(t, b, keyMsg("t"))
	if b.delete.step != deleteStepComment {
		t.Fatalf("precondition: step = %d, want deleteStepComment", b.delete.step)
	}

	const comment = "y n d x g plain text"
	for _, ch := range comment {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	if b.delete.commentInput.Value() != comment {
		t.Errorf("commentInput.Value() = %q, want %q (every typed rune must reach the textinput literally)", b.delete.commentInput.Value(), comment)
	}
	if b.mode != deleteMode || b.delete.step != deleteStepComment {
		t.Errorf("mode/step changed after typing command-bound-elsewhere runes: mode=%d step=%d, want unchanged deleteMode/deleteStepComment", b.mode, b.delete.step)
	}
}

// --- Mismatch-retype: fires no command ---

func TestDeleteMode_ConfirmStep_MismatchThenEnter_FiresNoCommand(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	card := b.selectedCard()

	b = sendKey(t, b, keyMsg("t"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter)) // blank comment -> confirm step

	wrong := strconv.Itoa(card.Number + 999)
	for _, ch := range wrong {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if cmd != nil {
		t.Error("expected a nil cmd from a mismatched retype-and-enter (no delete/comment command should fire)")
	}
	if b2.mode != deleteMode || b2.delete.step != deleteStepConfirm {
		t.Errorf("mode=%d step=%d after mismatch, want deleteMode/deleteStepConfirm (stay in step)", b2.mode, b2.delete.step)
	}
	if b2.delete.mismatchMsg == "" {
		t.Error("expected a non-empty mismatchMsg after a wrong retype")
	}
}

// --- Esc-from-step-2: cancels the whole flow, restores b.normalHints ---

func TestDeleteMode_EscAtConfirmStep_RestoresNormalHints(t *testing.T) {
	b, _ := newDeleteTestBoard(t)

	b = sendKey(t, b, keyMsg("t"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter)) // -> confirm step
	if b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: step = %d, want deleteStepConfirm", b.delete.step)
	}

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if b2.mode != normalMode {
		t.Fatalf("mode = %d after esc-cancel from confirm step, want normalMode", b2.mode)
	}
	if len(b2.statusBar.hints) != len(b2.normalHints) {
		t.Fatalf("statusBar.hints len = %d after esc-cancel from confirm step, want normalHints len %d", len(b2.statusBar.hints), len(b2.normalHints))
	}
	for i, h := range b2.normalHints {
		if b2.statusBar.hints[i] != h {
			t.Errorf("statusBar.hints[%d] = %+v after esc-cancel from confirm step, want normalHints[%d] = %+v", i, b2.statusBar.hints[i], i, h)
		}
	}
}

// --- ctrl+c always quits, regardless of user keymap config ---

func TestDeleteMode_CtrlCQuits_EvenWithOverriddenKeymap(t *testing.T) {
	b, _ := newDeleteTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDelete: {"ctrl+c": keymap.CommandBinding(keymap.CommandDeleteSubmit)},
	}, nil)
	b = sendKey(t, b, keyMsg("t"))
	if b.mode != deleteMode {
		t.Fatalf("precondition: mode = %d, want deleteMode", b.mode)
	}

	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in deleteMode should return a non-nil Cmd (tea.Quit), even with an overridden keymap")
	}
}

// --- Composite hint<->dispatch invariant, for both steps ---

// TestDeleteMode_HintKeysAlwaysDispatch_BothStepsDefaultAndRemappedTables is
// the hint<->dispatch invariant test named in the #539 plan's Explicit Risk
// Coverage, adapted to delete's []Hint-slice hint bar (rather than
// close_confirm/label_confirm's single joined "(y/n)"-style string): for
// every key advertised by b.deleteCommentHints()/b.deleteConfirmHints(), it
// must resolve through b.textBinding -- the real dispatch path
// handleDeleteModeKey uses -- against ModeDelete. Run against both the
// default table and a remapped/unbound table, for both steps, mirroring
// TestCloseMode_PromptHintKeysAlwaysDispatch_DefaultAndRemappedTables
// (close_mode_test.go).
//
// The remapped table uses named keys (f1/f2), not bare printable runes:
// ModeDelete.ConsumesPrintableRunes()==true makes a bare-rune binding
// unreachable via textBinding (it never even reaches Lookup), so asserting
// through raw Lookup instead of textBinding would pass even for a hint that
// advertises a key that can never actually dispatch through the real
// handler. f1/f2 don't collide with deleteDefaults (internal/keymap/defaults_text.go),
// which only binds enter/esc.
func TestDeleteMode_HintKeysAlwaysDispatch_BothStepsDefaultAndRemappedTables(t *testing.T) {
	keyMsgForLabel := func(label string) tea.KeyMsg {
		switch label {
		case "enter":
			return arrowMsg(tea.KeyEnter)
		case "esc":
			return arrowMsg(tea.KeyEsc)
		case "f1":
			return arrowMsg(tea.KeyF1)
		case "f2":
			return arrowMsg(tea.KeyF2)
		default:
			return keyMsg(label)
		}
	}

	tests := []struct {
		name  string
		modes map[keymap.Mode]keymap.Table
	}{
		{name: "default table", modes: nil},
		{
			name: "remapped table",
			modes: map[keymap.Mode]keymap.Table{
				keymap.ModeDelete: {
					"enter": keymap.UnboundBinding(),
					"f1":    keymap.CommandBinding(keymap.CommandDeleteSubmit),
					"esc":   keymap.UnboundBinding(),
					"f2":    keymap.CommandBinding(keymap.CommandDeleteCancel),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := newDeleteTestBoard(t)
			if tc.modes != nil {
				b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			}

			for _, hints := range [][]Hint{b.deleteCommentHints(), b.deleteConfirmHints()} {
				for _, h := range hints {
					if h.Key == "" {
						continue
					}
					for _, key := range strings.Split(h.Key, "/") {
						if _, ok := b.textBinding(keymap.ModeDelete, keyMsgForLabel(key)); !ok {
							t.Errorf("hint %+v: textBinding(ModeDelete, %q) not found, want a match (every advertised key must actually dispatch through the real handler)", h, key)
						}
					}
				}
			}
		})
	}
}

// --- Keybinding hint registration (CLAUDE.md hard rule) ---

func TestHelpSections_NormalMode_ContainsDeleteCardHint(t *testing.T) {
	b := newLoadedTestBoard(t)
	content := b.buildHelpContent()
	section := helpSectionBody(t, content, "Normal Mode")

	desc := mustFindCommand(t, keymap.CommandCardDelete).Desc
	if !strings.Contains(section, desc) {
		t.Fatalf("generated Normal Mode section does not contain the card.delete desc %q, got:\n%s", desc, section)
	}
}
