package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// The shared newBoardWithPRsAndExecutor fixture has one column with three
// cards: card 1 (0 PRs), card 2 (1 PR #10), card 3 (2 PRs #20, #21) — three
// linked PRs across the board in card-then-PR order. Opening the modal shows
// these card-linked PRs immediately as a fallback while the repo-wide
// open-PR fetch is in flight; openPRsMsg then replaces them with the full
// repo-wide list.

func TestNormalMode_V_OpensPRListModal(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	m, cmd := b.Update(keyMsg("v"))
	b = m.(Board)

	if b.mode != prListMode {
		t.Fatalf("mode = %d, want prListMode (%d)", b.mode, prListMode)
	}
	if !b.prList.loading {
		t.Error("prList.loading = false, want true (repo-wide fetch in flight)")
	}
	if cmd == nil {
		t.Fatal("Update(v) returned nil cmd, want the repo-wide open-PR fetch command")
	}
	if len(b.prList.entries) != 3 {
		t.Fatalf("entries = %d, want 3 (card-linked fallback aggregated across all cards)", len(b.prList.entries))
	}
	// Fallback aggregation order is column, then card, then PR within card.
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

// TestNormalMode_V_FallbackExcludesClosedAndMergedLinkedPRs asserts the
// instant fallback list built from card.LinkedPRs while the repo-wide fetch
// is in flight (#449) excludes PRs whose State is CLOSED or MERGED --
// closedByPullRequestsReferences includes linked PRs regardless of state, so
// a merged/closed linked PR must not briefly flash in the "Open Pull
// Requests" modal. The filter is exclusive: an entry with no State set (the
// vast majority of existing fixtures) must still be kept, and once the real
// open-PR fetch lands via openPRsMsg, the fallback's filtering must not
// affect the replaced (already open-only) repo-wide list.
func TestNormalMode_V_FallbackExcludesClosedAndMergedLinkedPRs(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newBoardWithInlineCardsAndExecutor(t, []provider.Card{
		{Number: 1, Title: "Mixed-state PR card", LinkedPRs: []provider.LinkedPR{
			{Number: 11, Title: "feat: still open", URL: "https://github.com/owner/repo/pull/11"},
			{Number: 32, Title: "fix: already merged", URL: "https://github.com/owner/repo/pull/32", State: "MERGED"},
		}},
	}, fe)

	m, cmd := b.Update(keyMsg("v"))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("Update(v) returned nil cmd, want the repo-wide open-PR fetch command")
	}

	if len(b.prList.entries) != 1 {
		t.Fatalf("entries = %d, want 1 (merged linked PR excluded from fallback)", len(b.prList.entries))
	}
	if got := b.prList.entries[0].pr.Number; got != 11 {
		t.Errorf("entries[0].pr.Number = %d, want 11 (unset-State open PR kept)", got)
	}
	for _, e := range b.prList.entries {
		if e.pr.Number == 32 {
			t.Errorf("fallback entries contain merged PR #32, want it excluded: %+v", b.prList.entries)
		}
	}

	// Feeding the real (open-only) fetch result must replace the fallback
	// unchanged -- the exclusion only applies to the LinkedPRs-derived
	// fallback, never to rows that already came from the repo-wide fetch.
	msg, ok := cmd().(openPRsMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want openPRsMsg", msg)
	}
	msg.prs = []provider.LinkedPR{
		{Number: 11, Title: "feat: still open", URL: "https://github.com/owner/repo/pull/11"},
	}
	m, _ = b.Update(msg)
	b = m.(Board)

	if len(b.prList.entries) != 1 {
		t.Fatalf("entries after openPRsMsg = %d, want 1", len(b.prList.entries))
	}
	if got := b.prList.entries[0].pr.Number; got != 11 {
		t.Errorf("entries[0].pr.Number after openPRsMsg = %d, want 11 (unfiltered repo-wide result)", got)
	}
}

// TestNormalMode_V_FetchesRepoWideOpenPRs exercises the full wiring: the cmd
// returned by pressing v queries the provider's open-PR list, and feeding its
// message back through Update replaces the fallback entries with the
// repo-wide list (FakeProvider returns PRs #40, #31, #30, #20 newest-first).
func TestNormalMode_V_FetchesRepoWideOpenPRs(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)

	m, cmd := b.Update(keyMsg("v"))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("Update(v) returned nil cmd, want the repo-wide open-PR fetch command")
	}

	msg, ok := cmd().(openPRsMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want openPRsMsg", msg)
	}
	m, _ = b.Update(msg)
	b = m.(Board)

	if b.prList.loading {
		t.Error("prList.loading = true after openPRsMsg, want false")
	}
	wantNumbers := []int{40, 31, 30, 20}
	if len(b.prList.entries) != len(wantNumbers) {
		t.Fatalf("entries = %d, want %d (repo-wide open PRs)", len(b.prList.entries), len(wantNumbers))
	}
	for i, want := range wantNumbers {
		if got := b.prList.entries[i].pr.Number; got != want {
			t.Errorf("entries[%d].pr.Number = %d, want %d (provider order preserved)", i, got, want)
		}
	}
	// PR #20 is linked to card 3 on the loaded board; unlinked PRs carry no
	// card reference (cardNumber 0).
	last := b.prList.entries[len(b.prList.entries)-1]
	if last.cardNumber != 3 || last.columnTitle != "Column A" {
		t.Errorf("linked entry #20 ref = %q #%d, want %q #3", last.columnTitle, last.cardNumber, "Column A")
	}
	if b.prList.entries[0].cardNumber != 0 {
		t.Errorf("unlinked entry #40 cardNumber = %d, want 0 (no card link)", b.prList.entries[0].cardNumber)
	}
}

// TestPRList_OpenPRsFetched_ClampsCursor asserts the cursor is re-clamped
// when the repo-wide list replaces a longer fallback list (list-cursor
// invariant: clamp at the mutation site).
func TestPRList_OpenPRsFetched_ClampsCursor(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.prList.cursor != 2 {
		t.Fatalf("cursor = %d, want 2 before fetch lands", b.prList.cursor)
	}

	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, prs: []provider.LinkedPR{
		{Number: 40, Title: "chore: only PR", URL: "https://github.com/owner/repo/pull/40"},
	}})

	if len(b.prList.entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(b.prList.entries))
	}
	if b.prList.cursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped to new list length)", b.prList.cursor)
	}
}

// TestPRList_OpenPRsError_KeepsFallbackEntries asserts a failed repo-wide
// fetch degrades to the card-linked fallback instead of blanking the modal,
// and surfaces the failure in the rendered view.
func TestPRList_OpenPRsError_KeepsFallbackEntries(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))

	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, err: errors.New("boom")})

	if b.prList.loading {
		t.Error("prList.loading = true after error, want false")
	}
	if b.prList.err == "" {
		t.Error("prList.err = \"\", want the fetch error recorded")
	}
	if len(b.prList.entries) != 3 {
		t.Fatalf("entries = %d, want 3 (card-linked fallback kept on error)", len(b.prList.entries))
	}
	view := b.viewPRListModal()
	if !strings.Contains(view, "linked PRs only") {
		t.Errorf("error view = %q, want it to explain the fallback to linked PRs only", view)
	}
}

// TestPRList_OpenPRsMsg_IgnoredAfterModalClosed asserts a fetch result that
// lands after the user closed the modal is dropped rather than mutating
// stale modal state.
func TestPRList_OpenPRsMsg_IgnoredAfterModalClosed(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, prs: []provider.LinkedPR{
		{Number: 40, Title: "chore: late arrival", URL: "https://github.com/owner/repo/pull/40"},
	}})

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(b.prList.entries) != 3 {
		t.Errorf("entries = %d, want 3 (stale result must not replace closed modal's state)", len(b.prList.entries))
	}
}

func TestPRList_OpenPRsMsg_IgnoredAfterModalReopened(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))
	staleGeneration := b.prList.generation
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	b = sendKey(t, b, keyMsg("v"))

	b = sendKey(t, b, openPRsMsg{
		generation: staleGeneration,
		prs: []provider.LinkedPR{
			{Number: 40, Title: "stale result", URL: "https://github.com/owner/repo/pull/40"},
		},
	})

	if !b.prList.loading {
		t.Error("prList.loading = false after stale result, want current request to remain loading")
	}
	if len(b.prList.entries) != 3 {
		t.Errorf("entries = %d, want the reopened modal's 3 fallback entries", len(b.prList.entries))
	}
}

func TestPRList_Navigation_MovesAndWrapsCursor(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))

	if b.prList.cursor != 0 {
		t.Fatalf("initial cursor = %d, want 0", b.prList.cursor)
	}
	lastIndex := len(b.prList.entries) - 1

	// k at the top wraps to the last entry.
	b = sendKey(t, b, keyMsg("k"))
	if b.prList.cursor != lastIndex {
		t.Errorf("cursor after k at top = %d, want %d (wrap to last)", b.prList.cursor, lastIndex)
	}

	// j from the last entry wraps back to the first.
	b = sendKey(t, b, keyMsg("j"))
	if b.prList.cursor != 0 {
		t.Errorf("cursor after j at bottom = %d, want 0 (wrap to first)", b.prList.cursor)
	}

	b = sendKey(t, b, keyMsg("j"))
	if b.prList.cursor != 1 {
		t.Errorf("cursor after j = %d, want 1", b.prList.cursor)
	}

	// Walk to the bottom and confirm one more j wraps past it, back to 0.
	b = sendKey(t, b, keyMsg("j"))
	if b.prList.cursor != lastIndex {
		t.Fatalf("cursor after walking to bottom = %d, want %d", b.prList.cursor, lastIndex)
	}
	b = sendKey(t, b, keyMsg("j"))
	if b.prList.cursor != 0 {
		t.Errorf("cursor after walking past bottom = %d, want 0 (wrap to first)", b.prList.cursor)
	}
}

// TestPRList_Navigation_ArrowKeys_WrapsCursor confirms Up/Down arrow keys
// wrap identically to j/k: both route through the shared moveCursor helper.
func TestPRList_Navigation_ArrowKeys_WrapsCursor(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))
	lastIndex := len(b.prList.entries) - 1

	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	if b.prList.cursor != lastIndex {
		t.Errorf("cursor after Up at top = %d, want %d (wrap to last)", b.prList.cursor, lastIndex)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if b.prList.cursor != 0 {
		t.Errorf("cursor after Down at bottom = %d, want 0 (wrap to first)", b.prList.cursor)
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

// TestPRList_Enter_OpensUnlinkedPR asserts an unlinked repo-wide PR row is
// just as actionable as a linked one.
func TestPRList_Enter_OpensUnlinkedPR(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))
	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, prs: []provider.LinkedPR{
		{Number: 40, Title: "chore: unlinked cleanup", URL: "https://github.com/owner/repo/pull/40"},
	}})

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL called %d times, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != "https://github.com/owner/repo/pull/40" {
		t.Errorf("OpenURL = %q, want the unlinked PR #40 URL", fe.OpenURLCalls[0])
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

// TestPRList_EmptyStates_LoadingThenNoOpenPRs walks the modal's empty-list
// precedence on a board with no linked PRs: while the fetch is in flight it
// says it's loading (not a misleading "no PRs"), and a successful empty
// result renders the repo-wide empty state. Enter on an empty list just
// closes without attempting to open a URL.
func TestPRList_EmptyStates_LoadingThenNoOpenPRs(t *testing.T) {
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
	if !strings.Contains(b.viewPRListModal(), "Loading") {
		t.Errorf("loading modal view = %q, want it to say it is loading", b.viewPRListModal())
	}

	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation})
	if !strings.Contains(b.viewPRListModal(), "No open PRs") {
		t.Errorf("empty modal view = %q, want it to contain %q", b.viewPRListModal(), "No open PRs")
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

// TestPRList_View_UnlinkedPRsCarryNoCardRef asserts the loaded repo-wide view
// renders linked rows with their card reference and unlinked rows without one.
func TestPRList_View_UnlinkedPRsCarryNoCardRef(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))
	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, prs: []provider.LinkedPR{
		{Number: 40, Title: "chore: unlinked cleanup", URL: "https://github.com/owner/repo/pull/40"},
		{Number: 20, Title: "feat: first PR", URL: "https://github.com/owner/repo/pull/20"},
	}})

	view := b.viewPRListModal()
	var linkedLine, unlinkedLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "#40") {
			unlinkedLine = line
		}
		if strings.Contains(line, "#20") {
			linkedLine = line
		}
	}
	if unlinkedLine == "" || linkedLine == "" {
		t.Fatalf("view missing PR rows; got:\n%s", view)
	}
	if !strings.Contains(linkedLine, "Column A") || !strings.Contains(linkedLine, "#3") {
		t.Errorf("linked row = %q, want it to carry the owning column and card", linkedLine)
	}
	if strings.Contains(unlinkedLine, "Column A") {
		t.Errorf("unlinked row = %q, want no card reference", unlinkedLine)
	}
}

// TestPRList_View_SanitizesControlSequencesInPRTitle covers the global PR
// list modal render path (#469): entry.pr.Title is untrusted (any GitHub
// user can open a PR against a tracked repo), so a malicious title
// containing raw terminal control sequences must not leak ESC/BEL bytes
// into the modal while the visible text is retained. truncateOutput alone
// does not strip control bytes, so this must go through sanitization first.
func TestPRList_View_SanitizesControlSequencesInPRTitle(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))
	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, prs: []provider.LinkedPR{
		{Number: 40, Title: "\x1b[31mRED\x1b[0m", URL: "https://github.com/owner/repo/pull/40"},
	}})

	view := b.viewPRListModal()

	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("viewPRListModal() = %q, want no ESC (0x1b) byte", view)
	}
	if strings.ContainsRune(view, '\x07') {
		t.Errorf("viewPRListModal() = %q, want no BEL (0x07) byte", view)
	}
	if !strings.Contains(view, "RED") {
		t.Errorf("viewPRListModal() should still contain visible PR title text %q", "RED")
	}
}

func TestPRList_View_TitleFitsModalWidth(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = sendKey(t, b, keyMsg("v"))

	for _, line := range strings.Split(b.viewPRListModal(), "\n") {
		if w := lipgloss.Width(line); w > b.Width {
			t.Errorf("modal line wider than terminal (%d > %d): %q", w, b.Width, line)
		}
	}
}

func TestPRList_View_KeepsSelectedRowVisibleWithinTerminal(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b.Height = 12
	b = sendKey(t, b, keyMsg("v"))
	prs := make([]provider.LinkedPR, 20)
	for i := range prs {
		prs[i] = provider.LinkedPR{Number: i + 1, Title: "open PR"}
	}
	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, prs: prs})
	b.prList.cursor = len(prs) - 1

	view := b.viewPRListModal()
	if !strings.Contains(view, "#20") {
		t.Errorf("view does not contain selected final row:\n%s", view)
	}
	if strings.Contains(view, "#1  ") {
		t.Errorf("view still contains first row instead of a cursor-relative window:\n%s", view)
	}
	if lipgloss.Height(view) > b.Height {
		t.Errorf("modal height = %d, want at most terminal height %d", lipgloss.Height(view), b.Height)
	}
	if !strings.Contains(view, "▲") {
		t.Errorf("view does not indicate rows above the visible window:\n%s", view)
	}
}

// --- Custom actions inside the PR list modal ---

// prListActionFixture builds a board from prFixtureColumns wired with the
// given global custom actions, opens the PR list, and returns board+executor.
func prListActionFixture(t *testing.T, actions map[string]config.Action) (Board, *action.FakeExecutor) {
	t.Helper()
	b, fe := newActionTestBoardWithColumns(t, actions, prFixtureColumns())
	b = sendKey(t, b, keyMsg("v"))
	return b, fe
}

// TestPRList_CustomAction_RunsAgainstSelectedLinkedPR asserts that an
// uppercase key inside the PR list dispatches the global scope: pr action
// against the selected row, expanding both PR and owning-card variables —
// the same variables a normal-mode scope: pr dispatch gets.
func TestPRList_CustomAction_RunsAgainstSelectedLinkedPR(t *testing.T) {
	b, fe := prListActionFixture(t, map[string]config.Action{
		"W": {Name: "Review", Type: "url", URL: "https://example.com/{number}/{pr_number}", Scope: "pr"},
	})

	// Fallback entry 0 is PR #10, linked to card 2.
	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != prListMode {
		t.Errorf("mode after action = %d, want prListMode (%d) (modal stays open)", b.mode, prListMode)
	}
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL called %d times, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != "https://example.com/2/10" {
		t.Errorf("OpenURL = %q, want card #2 and PR #10 expanded", fe.OpenURLCalls[0])
	}
}

// TestPRList_CustomAction_UnlinkedPR_ExpandsEmptyCardVars asserts a scope: pr
// action still runs on a repo-wide PR with no linked card, with card-derived
// variables expanding to empty strings rather than misleading zero values.
func TestPRList_CustomAction_UnlinkedPR_ExpandsEmptyCardVars(t *testing.T) {
	b, fe := prListActionFixture(t, map[string]config.Action{
		"W": {Name: "Review", Type: "url", URL: "https://example.com/{number}/{pr_number}", Scope: "pr"},
	})
	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation, prs: []provider.LinkedPR{
		{Number: 40, Title: "chore: unlinked cleanup", URL: "https://github.com/owner/repo/pull/40"},
	}})

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != prListMode {
		t.Errorf("mode after action = %d, want prListMode (%d) (modal stays open)", b.mode, prListMode)
	}
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL called %d times, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != "https://example.com//40" {
		t.Errorf("OpenURL = %q, want empty card number and PR #40 expanded", fe.OpenURLCalls[0])
	}
}

// TestPRList_CustomAction_NonPRScopeIgnored asserts card- and board-scope
// actions do not fire from the PR list — a PR row is not a card/board target.
func TestPRList_CustomAction_NonPRScopeIgnored(t *testing.T) {
	b, fe := prListActionFixture(t, map[string]config.Action{
		"C": {Name: "Card thing", Type: "url", URL: "https://example.com/{number}", Scope: "card"},
		"B": {Name: "Board thing", Type: "url", URL: "https://example.com/board", Scope: "board"},
	})

	for _, key := range []string{"C", "B"} {
		m, cmd := b.Update(keyMsg(key))
		b = m.(Board)
		execCmds(cmd)
	}

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("OpenURL called %d times from non-pr-scope actions, want 0: %v", len(fe.OpenURLCalls), fe.OpenURLCalls)
	}
	if b.mode != prListMode {
		t.Errorf("mode = %d, want prListMode (%d)", b.mode, prListMode)
	}
}

// TestPRList_CustomAction_EmptyListNoOp asserts an action key on an empty
// list is a safe no-op.
func TestPRList_CustomAction_EmptyListNoOp(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newBoardWithInlineCardsAndExecutor(t, []provider.Card{{Number: 1, Title: "No PRs here"}}, fe)
	b.actions = map[string]config.Action{
		"W": {Name: "Review", Type: "url", URL: "https://example.com/{pr_number}", Scope: "pr"},
	}
	b = sendKey(t, b, keyMsg("v"))
	b = sendKey(t, b, openPRsMsg{generation: b.prList.generation})

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != prListMode {
		t.Errorf("mode = %d, want prListMode (%d) (no-op must not leave the modal)", b.mode, prListMode)
	}
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("OpenURL called %d times on empty list, want 0", len(fe.OpenURLCalls))
	}
}

// TestPRList_Hints_IncludePRScopedActions asserts the modal surfaces the
// user's scope: pr actions by name, so the dispatch capability is
// discoverable (view-state consistency with the key handler).
func TestPRList_Hints_IncludePRScopedActions(t *testing.T) {
	b, _ := prListActionFixture(t, map[string]config.Action{
		"W": {Name: "Review", Type: "url", URL: "https://example.com/{pr_number}", Scope: "pr"},
		"C": {Name: "Card thing", Type: "url", URL: "https://example.com/{number}", Scope: "card"},
	})

	view := b.viewPRListModal()
	if !strings.Contains(view, "Review") {
		t.Errorf("modal view = %q, want it to hint the scope: pr action %q", view, "Review")
	}
	if strings.Contains(view, "Card thing") {
		t.Errorf("modal view hints non-pr-scope action %q; card-scope actions cannot fire here", "Card thing")
	}
}

// --- PR status glyphs in the global PR list (#431) ---
//
// Each row is prefixed with prStatusSymbol(prStatus(entry.pr)) styled via
// prStatusStyle, mirroring the codebase's existing agent-badge convention.
// UNKNOWN renders a blank placeholder (no glyph) here -- unlike the board
// glyph's neutral-color fallback (view_test.go's Q1 regression test) -- since
// these rows already show the PR number/title and don't need a "has a PR"
// signal from the glyph itself.

// TestPRList_View_RowShowsStatusSymbolStyledPerEntry asserts a row for a
// conflicting PR renders prStatusSymbol("conflicting") styled via
// prConflictingStyle.
func TestPRList_View_RowShowsStatusSymbolStyledPerEntry(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newBoardWithInlineCardsAndExecutor(t, []provider.Card{
		{Number: 1, Title: "Conflicting PR card", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: conflicting", URL: "https://github.com/o/r/pull/10", Mergeable: "CONFLICTING", MergeStateStatus: "DIRTY"},
		}},
	}, fe)
	b = sendKey(t, b, keyMsg("v"))

	view := b.viewPRListModal()
	want := prConflictingStyle.Render(prStatusSymbol("conflicting"))
	if !strings.Contains(view, want) {
		t.Errorf("PR list view missing conflicting status symbol styled %q; got:\n%s", want, view)
	}
}

// TestPRList_View_UnknownStatusRendersNoGlyph asserts a row whose PR status
// is UNKNOWN shows no status glyph at all (blank placeholder), diverging
// intentionally from the board glyph's neutral-color-but-still-colored
// behavior on UNKNOWN.
func TestPRList_View_UnknownStatusRendersNoGlyph(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newBoardWithInlineCardsAndExecutor(t, []provider.Card{
		{Number: 1, Title: "Unresolved PR card", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: unresolved", URL: "https://github.com/o/r/pull/10", Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN"},
		}},
	}, fe)
	b = sendKey(t, b, keyMsg("v"))

	view := b.viewPRListModal()
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "#10") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("PR list view missing row for #10; got:\n%s", view)
	}
	for _, status := range []string{"draft", "mergeable", "conflicting", "blocked"} {
		if sym := prStatusSymbol(status); sym != "" && strings.Contains(row, sym) {
			t.Errorf("row for unknown-status PR contains a known-state glyph %q (status %s), want no glyph; row = %q", sym, status, row)
		}
	}
}

// TestPRList_View_ColumnAlignment_UnknownAndKnownStatusRowsMatch asserts the
// "#NN" column lands at the same rendered column for a row with no status
// glyph (unknown) as for a row with a styled glyph (known status) -- the
// glyph slot must occupy a fixed rendered width whether or not a glyph is
// actually shown, or the "#NN" column jitters left/right per row.
func TestPRList_View_ColumnAlignment_UnknownAndKnownStatusRowsMatch(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newBoardWithInlineCardsAndExecutor(t, []provider.Card{
		{Number: 1, Title: "Mixed status card", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: unresolved", URL: "https://github.com/o/r/pull/10", Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN"},
			{Number: 20, Title: "feat: conflicting", URL: "https://github.com/o/r/pull/20", Mergeable: "CONFLICTING", MergeStateStatus: "DIRTY"},
			{Number: 30, Title: "feat: unstable", URL: "https://github.com/o/r/pull/30", Mergeable: "MERGEABLE", MergeStateStatus: "UNSTABLE"},
		}},
	}, fe)
	b = sendKey(t, b, keyMsg("v"))

	view := b.viewPRListModal()
	var unknownRow, knownRow, unstableRow string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "#10") {
			unknownRow = line
		}
		if strings.Contains(line, "#20") {
			knownRow = line
		}
		if strings.Contains(line, "#30") {
			unstableRow = line
		}
	}
	if unknownRow == "" || knownRow == "" || unstableRow == "" {
		t.Fatalf("view missing expected PR rows; got:\n%s", view)
	}

	unknownPrefixWidth := lipgloss.Width(unknownRow[:strings.Index(unknownRow, "#10")])
	knownPrefixWidth := lipgloss.Width(knownRow[:strings.Index(knownRow, "#20")])
	unstablePrefixWidth := lipgloss.Width(unstableRow[:strings.Index(unstableRow, "#30")])
	if unknownPrefixWidth != knownPrefixWidth {
		t.Errorf("column widths before # differ: unknown-status row = %d, known-status row = %d\nunknown row: %q\nknown row:   %q",
			unknownPrefixWidth, knownPrefixWidth, unknownRow, knownRow)
	}
	if unstablePrefixWidth != knownPrefixWidth {
		t.Errorf("column widths before # differ: unstable-status row = %d, known-status row = %d\nunstable row: %q\nknown row:    %q",
			unstablePrefixWidth, knownPrefixWidth, unstableRow, knownRow)
	}
}

// --- purple linkedPRGlyph prefix on every PR list row (#447) ---
//
// Every row is now prefixed with the purple linkedPRGlyph marker (color 183,
// via prIndicatorStyle) in addition to the existing status glyph -- the same
// icon already used for the status-bar aggregate open-PR count.

// TestPRList_View_RowsPrefixedWithPurpleLinkedPRGlyph asserts a known-status
// row (mergeable) is prefixed with the purple linkedPRGlyph marker.
func TestPRList_View_RowsPrefixedWithPurpleLinkedPRGlyph(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	fe := &action.FakeExecutor{}
	b := newBoardWithInlineCardsAndExecutor(t, []provider.Card{
		{Number: 1, Title: "Mergeable PR card", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: mergeable", URL: "https://github.com/o/r/pull/10", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
		}},
	}, fe)
	b = sendKey(t, b, keyMsg("v"))

	view := b.viewPRListModal()
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "#10") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("PR list view missing row for #10; got:\n%s", view)
	}
	wantGlyph := prIndicatorStyle.Render(linkedPRGlyph)
	if !strings.Contains(row, wantGlyph) {
		t.Errorf("PR list row %q missing purple linkedPRGlyph prefix %q", row, wantGlyph)
	}
}

// TestPRList_View_UnknownStatusRow_StillPrefixedWithPurpleLinkedPRGlyph
// asserts the purple glyph prefix applies uniformly even to an
// "unknown"-status row, which renders no status glyph of its own -- the
// purple icon is a prefix in addition to the status glyph, not a substitute.
func TestPRList_View_UnknownStatusRow_StillPrefixedWithPurpleLinkedPRGlyph(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	fe := &action.FakeExecutor{}
	b := newBoardWithInlineCardsAndExecutor(t, []provider.Card{
		{Number: 1, Title: "Unresolved PR card", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: unresolved", URL: "https://github.com/o/r/pull/10", Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN"},
		}},
	}, fe)
	b = sendKey(t, b, keyMsg("v"))

	view := b.viewPRListModal()
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "#10") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("PR list view missing row for #10; got:\n%s", view)
	}
	wantGlyph := prIndicatorStyle.Render(linkedPRGlyph)
	if !strings.Contains(row, wantGlyph) {
		t.Errorf("unknown-status PR list row %q missing purple linkedPRGlyph prefix %q", row, wantGlyph)
	}
}

func TestPRList_ActionHints_AreSortedByKey(t *testing.T) {
	b, _ := prListActionFixture(t, map[string]config.Action{
		"Z": {Name: "Last", Scope: "pr"},
		"A": {Name: "First", Scope: "pr"},
		"M": {Name: "Middle", Scope: "pr"},
	})

	hints := b.prListActionHints()
	wantKeys := []string{"esc", "j/k", "enter", "A", "M", "Z"}
	if len(hints) != len(wantKeys) {
		t.Fatalf("hint count = %d, want %d: %+v", len(hints), len(wantKeys), hints)
	}
	for i, want := range wantKeys {
		if hints[i].Key != want {
			t.Errorf("hints[%d].Key = %q, want %q", i, hints[i].Key, want)
		}
	}
}
