package main

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v68/github"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Entering closeConfirmMode via 'x' ---

func TestCloseMode_XKey_EntersCloseConfirmModeWithTargetCard(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	wantCard := b.selectedCard()

	m, _ := b.Update(keyMsg("x"))
	b = m.(Board)

	if b.mode != closeConfirmMode {
		t.Fatalf("mode = %d, want closeConfirmMode", b.mode)
	}
	if b.closeConfirm.card.Number != wantCard.Number {
		t.Errorf("closeConfirm.card.Number = %d, want %d", b.closeConfirm.card.Number, wantCard.Number)
	}
	if b.closeConfirm.card.Title != wantCard.Title {
		t.Errorf("closeConfirm.card.Title = %q, want %q", b.closeConfirm.card.Title, wantCard.Title)
	}
}

func TestCloseMode_XKey_NoColumns_DoesNothing(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)
	msg := boardFetchedMsg{board: provider.Board{Columns: nil}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	m, cmd := b.Update(keyMsg("x"))
	b = m.(Board)

	if cmd != nil {
		t.Error("expected nil cmd from 'x' key when board has no columns")
	}
	if b.mode == closeConfirmMode {
		t.Error("should not enter closeConfirmMode when board has no columns")
	}
}

func TestCloseMode_XKey_NoVisibleCards_DoesNothing(t *testing.T) {
	b, _ := newBoardWithEmptyColumn(t, nil)

	m, cmd := b.Update(keyMsg("x"))
	b = m.(Board)

	if cmd != nil {
		t.Error("expected nil cmd from 'x' key when the active column has no cards")
	}
	if b.mode == closeConfirmMode {
		t.Error("should not enter closeConfirmMode when the active column has no cards")
	}
}

func TestCloseMode_XKey_DetailFocused_IsNoOp(t *testing.T) {
	// 'x' is a normal-mode-only keybinding (per CLAUDE.md convention) and must
	// not be duplicated into the detail-focused sub-mode.
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b.detailFocused = true

	m, _ := b.Update(keyMsg("x"))
	b = m.(Board)

	if b.mode == closeConfirmMode {
		t.Error("'x' pressed while detail-focused should not enter closeConfirmMode")
	}
	if !b.detailFocused {
		t.Error("'x' pressed while detail-focused should leave detailFocused unchanged (unhandled key)")
	}
}

func TestCloseMode_XKey_FilterActive_TargetsFilteredCard(t *testing.T) {
	// Regression guard for #234: card resolution under an active filter must
	// go through selectedCard(), not the raw (unfiltered) column index.
	b := newBoardWithFilterableCards(t)
	b.Width = 120
	b.Height = 40

	// Cards #1, #3, #5 in "Backlog" carry the "bug" label.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"
	b.Columns[b.ActiveTab].Cursor = 1 // second bug card in the filtered list -> #3

	m, _ := b.Update(keyMsg("x"))
	b = m.(Board)

	if b.mode != closeConfirmMode {
		t.Fatalf("mode = %d, want closeConfirmMode", b.mode)
	}
	if b.closeConfirm.card.Number != 3 {
		t.Errorf("closeConfirm.card.Number = %d, want #3 (filtered selection), not raw index 1 (#2)", b.closeConfirm.card.Number)
	}
}

// --- View rendering of the confirmation prompt ---

func TestCloseMode_ViewShowsPromptWithCardNumberAndTitle(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	card := b.selectedCard()

	m, _ := b.Update(keyMsg("x"))
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, fmt.Sprintf("#%d", card.Number)) {
		t.Errorf("View() in closeConfirmMode should show card number, got:\n%s", view)
	}
	if !strings.Contains(view, card.Title) {
		t.Errorf("View() in closeConfirmMode should show card title, got:\n%s", view)
	}
	if !strings.Contains(view, "y/n") {
		t.Errorf("View() in closeConfirmMode should show a y/n prompt, got:\n%s", view)
	}
}

// TestCloseMode_ViewSanitizesControlSequencesInTitle covers the same
// GitHub-sourced-untrusted-content gap for the closeConfirmMode helpBar
// prompt: card.Title is rendered via fmt.Sprintf's %q verb, which currently
// escapes control bytes to visible literal text, but must still route
// through sanitizeControlSequences for consistency with every other
// card.Title render site (cardDisplayText, composeDetailMarkdown). A
// malicious title must not leak raw ESC/BEL control bytes into the rendered
// prompt, while the visible text is retained.
func TestCloseMode_ViewSanitizesControlSequencesInTitle(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].Title = "\x1b[31mRED\x1b[0m title\x07"

	m, _ := b.Update(keyMsg("x"))
	b = m.(Board)
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	view := b.View()
	promptLine := ""
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Close #") {
			promptLine = line
			break
		}
	}
	if promptLine == "" {
		t.Fatalf("View() in closeConfirmMode has no line containing the close prompt, got:\n%s", view)
	}
	if strings.ContainsRune(promptLine, '\x1b') {
		t.Errorf("close prompt line = %q, want no ESC (0x1b) byte", promptLine)
	}
	if strings.ContainsRune(promptLine, '\x07') {
		t.Errorf("close prompt line = %q, want no BEL (0x07) byte", promptLine)
	}
	if !strings.Contains(promptLine, "RED title") {
		t.Errorf("close prompt line = %q, want visible title text %q retained", promptLine, "RED title")
	}
}

// --- Key handling within closeConfirmMode ---

func TestCloseMode_YKey_FiresCloseCardCmdAndReturnsToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	m, _ := b.Update(keyMsg("x"))
	b = m.(Board)
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}

	m, cmd := b.Update(keyMsg("y"))
	b = m.(Board)

	if cmd == nil {
		t.Fatal("expected non-nil cmd from 'y' key (closeCardCmd)")
	}
	if b.mode != normalMode {
		t.Errorf("mode = %d after 'y', want normalMode (optimistic transition, mirrors assign-mode Enter)", b.mode)
	}
}

func TestCloseMode_NKey_CancelsToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	m, _ := b.Update(keyMsg("x"))
	b = m.(Board)

	m, _ = b.Update(keyMsg("n"))
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after 'n', want normalMode", b.mode)
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "cancel") {
		t.Errorf("View() after 'n' should contain a cancel message, got:\n%s", view)
	}
}

func TestCloseMode_EscKey_CancelsToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	m, _ := b.Update(keyMsg("x"))
	b = m.(Board)

	m, _ = b.Update(arrowMsg(tea.KeyEsc))
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after 'esc', want normalMode", b.mode)
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "cancel") {
		t.Errorf("View() after 'esc' should contain a cancel message, got:\n%s", view)
	}
}

// --- cardClosedMsg: optimistic local removal + guard-matrix cleanup ---

func TestCloseMode_CardClosed_NoGuard_RemovesCardAndCleansUp(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	card := b.Columns[0].Cards[0] // card #1 "Setup CI" in column "New" (cleanup configured)
	m, cmd := b.Update(cardClosedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call for closed card with no guard active, got none")
	}
	for _, c := range b.Columns[0].Cards {
		if c.Number == card.Number {
			t.Fatal("expected card removed from Columns after cardClosedMsg")
		}
	}
	if _, exists := b.prevCards[card.Number]; exists {
		t.Error("expected prevCards entry deleted after close")
	}
}

func TestCloseMode_CardClosed_AgentBusy_SkipsCleanupButRemovesCard(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")
	b.agentSnapshot = cleanupSnapshot("running")

	card := b.Columns[0].Cards[0]
	m, cmd := b.Update(cardClosedMsg{card: card})
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
	// Locked decision (#347 Q2): a guard-blocked close ALWAYS deletes the
	// prevCards entry -- unlike detectDepartures, which defers by re-inserting
	// it. Cleanup for this card is therefore permanently skipped, not deferred.
	if _, exists := b.prevCards[card.Number]; exists {
		t.Error("expected prevCards entry deleted after close, even though guard blocked cleanup")
	}
}

func TestCloseMode_CardClosed_AgentBusy_GuardBlockDoesNotRefireOnNextFetch(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")
	b.agentSnapshot = cleanupSnapshot("running")

	card := b.Columns[0].Cards[0]
	m, cmd := b.Update(cardClosedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)
	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("precondition: expected no cleanup yet, got: %v", fe.RunShellCalls)
	}

	// The agent finishes and a subsequent fetch cycle reflects the real close
	// (card genuinely gone from the provider). Since prevCards no longer
	// tracks this card, detectDepartures has nothing left to compare -- the
	// cleanup command must NOT fire again for it.
	b.agentSnapshot = cleanupSnapshot("done")
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(-1))
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no cleanup re-fire on subsequent fetch after guard-blocked close, got: %v", fe.RunShellCalls)
	}
}

func TestCloseMode_CardClosed_CenciEnabledSnapshotNotYetDelivered_SkipsCleanupButRemovesCard(t *testing.T) {
	b, fe, _ := newCleanupTestBoardWithWatcher(t, "tmux kill-window -t {session}")

	card := b.Columns[0].Cards[0]
	m, cmd := b.Update(cardClosedMsg{card: card})
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
	// Same locked decision (#347 Q2) as the agent-busy guard: a guard-blocked
	// close always deletes the prevCards entry, it never defers.
	if _, exists := b.prevCards[card.Number]; exists {
		t.Error("expected prevCards entry deleted after close, even though the fail-closed guard blocked cleanup")
	}
}

func TestCloseMode_CardClosed_WorkingLabel_SkipsCleanupButRemovesCard(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	card := b.Columns[0].Cards[0]
	card.Labels = append(card.Labels, Label{Name: "working"}) // case-insensitive match to configured workingLabel "Working"
	m, cmd := b.Update(cardClosedMsg{card: card})
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
		t.Error("expected prevCards entry deleted after close, even though guard blocked cleanup")
	}
}

func TestCloseMode_CardClosed_BypassesMissingCardDebounce(t *testing.T) {
	// Unlike a card vanishing during a background fetch -- which requires two
	// consecutive misses before cleanup fires, see
	// TestCleanup_CardDisappears_DebouncedToSecondFetch in cleanup_test.go --
	// an explicit close via 'x'/'y' must clean up on the very first
	// cardClosedMsg, with no debounce.
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	card := b.Columns[0].Cards[0]
	m, cmd := b.Update(cardClosedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call on the very first close (no debounce), got none")
	}
}

func TestCloseMode_CardClosed_CursorOnLastCard_ClampsAndDoesNotPanic(t *testing.T) {
	// Regression guard: closing the card the cursor sits on, when it is the
	// last card in the column, must clamp Cursor to the new (shorter) length.
	// Column "New" in newCleanupTestBoard has 3 cards: #1, #2, #3.
	b, _, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	lastIdx := len(b.Columns[0].Cards) - 1
	b.Columns[0].Cursor = lastIdx
	card := b.Columns[0].Cards[lastIdx]

	m, cmd := b.Update(cardClosedMsg{card: card})
	b = m.(Board)
	execCmds(cmd)

	if b.Columns[0].Cursor >= len(b.Columns[0].Cards) {
		t.Fatalf("Cursor = %d after closing last card, want < %d (clamped)", b.Columns[0].Cursor, len(b.Columns[0].Cards))
	}
	if b.Columns[0].Cursor < 0 {
		t.Fatalf("Cursor = %d after closing last card, want >= 0", b.Columns[0].Cursor)
	}

	// This is the exact panic the reviewer found: viewCardDetail indexing
	// col.Cards[col.Cursor] with an out-of-bounds Cursor.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("b.View() panicked after closing last card with cursor on it: %v", r)
		}
	}()
	_ = b.View()
}

// --- cardCloseErrorMsg: sanitized error rendering ---

func TestCloseMode_CardCloseErrorMsg_SanitizesGitHubAPIError(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	rawMessage := "internal trace: panic at /srv/app/internal/db.go:42 token=SECRET"
	ghErr := &github.ErrorResponse{
		Response: &http.Response{StatusCode: http.StatusInternalServerError},
		Message:  rawMessage,
	}

	m, _ := b.Update(cardCloseErrorMsg{err: ghErr})
	b = m.(Board)

	if strings.Contains(b.statusBar.message, rawMessage) {
		t.Errorf("status bar message leaked raw internal error text, got: %q", b.statusBar.message)
	}
	if !strings.Contains(strings.ToLower(b.statusBar.message), "internal server error") {
		t.Errorf("status bar message = %q, want it to contain the sanitized description", b.statusBar.message)
	}
}

func TestCloseMode_CardCloseErrorMsg_ReturnsToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	m, _ := b.Update(keyMsg("x"))
	b = m.(Board)
	m, _ = b.Update(keyMsg("y"))
	b = m.(Board)

	m, _ = b.Update(cardCloseErrorMsg{err: errSentinel("close failed")})
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after cardCloseErrorMsg, want normalMode", b.mode)
	}
	view := b.View()
	if !strings.Contains(strings.ToLower(view), "error") {
		t.Errorf("View() after cardCloseErrorMsg should contain an error message, got:\n%s", view)
	}
}

// --- Keybinding hint registration (CLAUDE.md hard rule) ---

func TestHelpSections_NormalMode_ContainsCloseCardHint(t *testing.T) {
	for _, section := range helpSections {
		if section.title != "Normal Mode" {
			continue
		}
		for _, kv := range section.keys {
			if kv[0] == "x" {
				return
			}
		}
		t.Fatalf("helpSections[%q] does not contain an entry for key %q", "Normal Mode", "x")
	}
	t.Fatal(`helpSections has no "Normal Mode" section`)
}
