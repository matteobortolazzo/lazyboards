package main

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Mode Transitions ---

func TestSearchMode_Slash_EntersSearchMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("/"))
	if b.mode != searchMode {
		t.Errorf("after '/': mode = %d, want %d (searchMode)", b.mode, searchMode)
	}
}

func TestSearchMode_Slash_IgnoredWhenDetailFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Focus the detail panel.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	// Press '/' — should be ignored.
	b = sendKey(t, b, keyMsg("/"))
	if b.mode != normalMode {
		t.Errorf("'/' with detailFocused: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestSearchMode_Escape_ExitsAndClearsQuery(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Enter search mode and type a query.
	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Escape.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Esc in search mode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if b.searchQuery != "" {
		t.Errorf("after Esc in search mode: searchQuery = %q, want empty string", b.searchQuery)
	}
}

func TestSearchMode_Enter_ExitsAndPreservesQuery(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != normalMode {
		t.Errorf("after Enter in search mode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if b.searchQuery != "bug" {
		t.Errorf("after Enter in search mode: searchQuery = %q, want %q", b.searchQuery, "bug")
	}
	if b.searchInput.Focused() {
		t.Error("after Enter in search mode: search input is still focused")
	}
}

func TestSearchMode_Tab_ExitsAndSwitchesColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	initialTab := b.ActiveTab

	// Enter search mode.
	b = sendKey(t, b, keyMsg("/"))
	if b.mode != searchMode {
		t.Fatalf("precondition: mode = %d, want %d (searchMode)", b.mode, searchMode)
	}

	// Press Tab.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	if b.mode != normalMode {
		t.Errorf("after Tab in search mode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	expectedTab := (initialTab + 1) % len(b.Columns)
	if b.ActiveTab != expectedTab {
		t.Errorf("after Tab in search mode: ActiveTab = %d, want %d", b.ActiveTab, expectedTab)
	}
}

func TestSearchMode_ShiftTab_ExitsAndSwitchesColumnBackward(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Move to column 1 first so Shift+Tab can go back to 0.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 1 {
		t.Fatalf("precondition: ActiveTab = %d, want 1", b.ActiveTab)
	}

	// Enter search mode.
	b = sendKey(t, b, keyMsg("/"))

	// Press Shift+Tab.
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))

	if b.mode != normalMode {
		t.Errorf("after Shift+Tab in search mode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if b.ActiveTab != 0 {
		t.Errorf("after Shift+Tab in search mode: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestSearchMode_DigitsTypeIntoQuery(t *testing.T) {
	// Search supports card-number matching (see TestSearchMode_MatchesCardNumber),
	// so digits must reach the query instead of switching columns.
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	requireColumns(t, b)

	b = sendKey(t, b, keyMsg("/"))
	b = sendKey(t, b, keyMsg("4"))
	b = sendKey(t, b, keyMsg("2"))

	if b.mode != searchMode {
		t.Errorf("after typing digits in search mode: mode = %d, want %d (searchMode)", b.mode, searchMode)
	}
	if b.searchQuery != "42" {
		t.Errorf("searchQuery = %q, want %q", b.searchQuery, "42")
	}
	if b.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d, want 0 (digits must not switch columns)", b.ActiveTab)
	}
}

func TestSearchMode_JKTypeIntoQuery(t *testing.T) {
	// j/k must be typeable in queries ("jwt", "kafka", ...); result
	// navigation lives on arrows and ctrl+n/ctrl+p.
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	requireColumns(t, b)

	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "jwt" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	if b.searchQuery != "jwt" {
		t.Errorf("searchQuery = %q, want %q (j/k must reach the textinput)", b.searchQuery, "jwt")
	}
}

// TestSearchMode_BareJK_DoesNotMoveCursor is a regression guard for the
// card-list wrap migration (#426 PR 2): bare j/k must keep reaching the
// search textinput and must never move the card cursor, even though j/k
// route through moveCursor everywhere else the card list navigates.
func TestSearchMode_BareJK_DoesNotMoveCursor(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Bug: login fails", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Bug: crash on load", Labels: []provider.Label{{Name: "bug"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b = sendKey(t, b, keyMsg("/"))
	cursorBefore := b.Columns[b.ActiveTab].Cursor

	b = sendKey(t, b, keyMsg("j"))

	if b.searchQuery != "j" {
		t.Errorf("searchQuery = %q after typing 'j' in search mode, want %q (bare j/k must reach the textinput)", b.searchQuery, "j")
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != cursorBefore {
		t.Errorf("cursor = %d after typing 'j' in search mode, want %d (unchanged; bare j/k must not navigate)", got, cursorBefore)
	}
}

// --- Filter Matching (filteredCards method) ---

func TestSearchMode_EmptyQuery_ReturnsAllCards(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	requireColumns(t, b)

	b.searchQuery = ""
	filtered := b.filteredCards()

	col := b.Columns[b.ActiveTab]
	if len(filtered) != len(col.Cards) {
		t.Errorf("filteredCards() with empty query: got %d cards, want %d (all cards)", len(filtered), len(col.Cards))
	}
}

func TestSearchMode_MatchesTitleCaseInsensitive(t *testing.T) {
	// Build a board with cards that have distinct titles.
	cards := []provider.Card{
		{Number: 1, Title: "Fix Login Bug", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Add Dashboard", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 3, Title: "Update README", Labels: []provider.Label{{Name: "docs"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Search for "login" (lowercase) should match "Fix Login Bug".
	b.searchQuery = "login"
	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() for 'login': got %d cards, want 1", len(filtered))
	}
	if filtered[0].Title != "Fix Login Bug" {
		t.Errorf("filteredCards() matched %q, want %q", filtered[0].Title, "Fix Login Bug")
	}
}

func TestSearchMode_MatchesLabelCaseInsensitive(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Card A", Labels: []provider.Label{{Name: "Critical"}}},
		{Number: 2, Title: "Card B", Labels: []provider.Label{{Name: "minor"}}},
		{Number: 3, Title: "Card C", Labels: []provider.Label{{Name: "Enhancement"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Search for "critical" (lowercase) should match card with "Critical" label.
	b.searchQuery = "critical"
	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() for 'critical': got %d cards, want 1", len(filtered))
	}
	if filtered[0].Number != 1 {
		t.Errorf("filteredCards() matched card #%d, want #1", filtered[0].Number)
	}
}

func TestSearchMode_MatchesCardNumber(t *testing.T) {
	cards := []provider.Card{
		{Number: 42, Title: "Some task", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 99, Title: "Another task", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 7, Title: "Third task", Labels: []provider.Label{{Name: "docs"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Search for "42" should match card #42.
	b.searchQuery = "42"
	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() for '42': got %d cards, want 1", len(filtered))
	}
	if filtered[0].Number != 42 {
		t.Errorf("filteredCards() matched card #%d, want #42", filtered[0].Number)
	}
}

func TestSearchMode_NoMatches_ReturnsEmpty(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	requireColumns(t, b)

	b.searchQuery = "zzzznonexistent"
	filtered := b.filteredCards()

	if len(filtered) != 0 {
		t.Errorf("filteredCards() for non-matching query: got %d cards, want 0", len(filtered))
	}
}

// --- Cursor and Scroll Reset ---

func TestSearchMode_TypingResetsCursor(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	requireColumns(t, b)

	// Move cursor down to a non-zero position.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[b.ActiveTab].Cursor == 0 {
		t.Fatal("precondition: cursor should be > 0 after navigating down")
	}

	// Enter search mode and type a character.
	b = sendKey(t, b, keyMsg("/"))
	b = sendKey(t, b, keyMsg("a"))

	if b.Columns[b.ActiveTab].Cursor != 0 {
		t.Errorf("cursor = %d after typing in search mode, want 0 (should reset)", b.Columns[b.ActiveTab].Cursor)
	}
}

func TestSearchMode_TypingResetsScrollOffset(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll down to build up scroll offset.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.Columns[b.ActiveTab].ScrollOffset == 0 {
		t.Fatal("precondition: ScrollOffset should be > 0 after scrolling down")
	}

	// Enter search mode and type a character.
	b = sendKey(t, b, keyMsg("/"))
	b = sendKey(t, b, keyMsg("a"))

	if b.Columns[b.ActiveTab].ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d after typing in search mode, want 0 (should reset)", b.Columns[b.ActiveTab].ScrollOffset)
	}
}

// --- Navigation on Filtered List ---

// TestSearchMode_CtrlNP_WrapsPastFilteredEnds asserts ctrl+n/ctrl+p cycle
// through the search-filtered card list (#426 PR 2): moving past the last
// filtered card wraps to the first, and moving before the first wraps to the
// last, mirroring the four modal lists already on moveCursor.
func TestSearchMode_CtrlNP_WrapsPastFilteredEnds(t *testing.T) {
	// Create cards where a search query matches exactly 3 cards.
	cards := []provider.Card{
		{Number: 1, Title: "Bug: login fails", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Feature: dashboard", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 3, Title: "Bug: crash on load", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 4, Title: "Feature: profile", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 5, Title: "Bug: memory leak", Labels: []provider.Label{{Name: "bug"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Enter search mode and type "Bug" to filter to 3 cards.
	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "Bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	filtered := b.filteredCards()
	if len(filtered) != 3 {
		t.Fatalf("precondition: filteredCards() for 'Bug' = %d, want 3", len(filtered))
	}
	lastIndex := len(filtered) - 1

	// Walk to the last filtered card via ctrl+n.
	for i := 0; i < lastIndex; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyCtrlN))
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != lastIndex {
		t.Fatalf("precondition: cursor = %d, want %d (last filtered card)", got, lastIndex)
	}

	// One more ctrl+n from the last filtered card should wrap to the first.
	b = sendKey(t, b, arrowMsg(tea.KeyCtrlN))

	// Navigation must not leave search mode or touch the query.
	if b.mode != searchMode {
		t.Fatalf("after ctrl+n: mode = %d, want %d (searchMode)", b.mode, searchMode)
	}
	if b.searchQuery != "Bug" {
		t.Errorf("searchQuery = %q after ctrl+n, want %q (navigation must not edit the query)", b.searchQuery, "Bug")
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("cursor = %d after ctrl+n past end of filtered list, want 0 (wrap to first)", got)
	}

	// ctrl+p from the first filtered card should wrap back to the last.
	b = sendKey(t, b, arrowMsg(tea.KeyCtrlP))
	if got := b.Columns[b.ActiveTab].Cursor; got != lastIndex {
		t.Errorf("cursor = %d after ctrl+p at first filtered card, want %d (wrap to last)", got, lastIndex)
	}
}

// TestSearchMode_ArrowKeys_WrapPastFilteredEnds mirrors
// TestSearchMode_CtrlNP_WrapsPastFilteredEnds for Down/Up arrows, which share
// the same key-match branch as ctrl+n/ctrl+p in handleSearchModeKey.
func TestSearchMode_ArrowKeys_WrapPastFilteredEnds(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Bug: login fails", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Bug: crash on load", Labels: []provider.Label{{Name: "bug"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b = sendKey(t, b, keyMsg("/"))
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if got := b.Columns[b.ActiveTab].Cursor; got != 1 {
		t.Errorf("cursor = %d after down arrow, want 1", got)
	}

	// One more Down arrow from the last matching card should wrap to the first.
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("cursor = %d after down arrow past last matching card, want 0 (wrap to first)", got)
	}

	// Up arrow from the first matching card should wrap to the last.
	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	if got := b.Columns[b.ActiveTab].Cursor; got != 1 {
		t.Errorf("cursor = %d after up arrow at first matching card, want 1 (wrap to last)", got)
	}
}

func TestSearchMode_Enter_AllowsNormalNavigationAndTicketOpen(t *testing.T) {
	firstURL := "https://github.com/owner/repo/issues/1"
	secondURL := "https://github.com/owner/repo/issues/3"
	cards := []provider.Card{
		{Number: 1, Title: "Bug: first", Labels: []provider.Label{{Name: "bug"}}, URL: firstURL},
		{Number: 2, Title: "Feature: skipped", Labels: []provider.Label{{Name: "feature"}}, URL: "https://github.com/owner/repo/issues/2"},
		{Number: 3, Title: "Bug: second", Labels: []provider.Label{{Name: "bug"}}, URL: secondURL},
	}
	b, fe := newActionTestBoardWithColumns(t, nil, []provider.Column{
		{Title: "Column A", Cards: cards},
	})

	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "Bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	b = sendKey(t, b, keyMsg("j"))

	if got := b.selectedCard().Number; got != 3 {
		t.Fatalf("after applying search and pressing j: selected card #%d, want #3", got)
	}

	b = sendKey(t, b, keyMsg("o"))

	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL called %d times, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != secondURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], secondURL)
	}
}

func TestSearchMode_DetailPanelShowsFilteredCard(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Alpha task", Labels: []provider.Label{{Name: "feature"}}, Body: "Alpha body content"},
		{Number: 2, Title: "Beta task", Labels: []provider.Label{{Name: "bug"}}, Body: "Beta body content"},
		{Number: 3, Title: "Gamma task", Labels: []provider.Label{{Name: "feature"}}, Body: "Gamma body content"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Enter search mode and filter to "Beta".
	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "Beta" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	filtered := b.filteredCards()
	if len(filtered) != 1 {
		t.Fatalf("precondition: filteredCards() for 'Beta' = %d, want 1", len(filtered))
	}

	// The detail panel should show the filtered card (Beta task).
	view := b.View()
	if len(view) == 0 {
		t.Fatal("View() returned empty string")
	}
	// The card shown in detail should be the first filtered card, not the
	// original card at cursor 0 of the unfiltered list.
	expectedTitle := fmt.Sprintf("#%d %s", filtered[0].Number, filtered[0].Title)
	if !strings.Contains(view, expectedTitle) {
		t.Errorf("detail panel should show filtered card title %q, not found in view", expectedTitle)
	}
}

// --- Search Input Rendering ---

func TestSearchMode_View_ContainsSearchInput(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Fix login bug", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Add dashboard", Labels: []provider.Label{{Name: "feature"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Enter search mode.
	b = sendKey(t, b, keyMsg("/"))

	view := b.View()
	// The search input's placeholder "Search..." should appear in the card list panel.
	if !strings.Contains(view, "Search...") {
		t.Errorf("View() in search mode should contain search input placeholder %q, not found in view", "Search...")
	}
}

func TestSearchMode_View_NoSearchInputInNormalMode(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Fix login bug", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Add dashboard", Labels: []provider.Label{{Name: "feature"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Stay in normal mode (do NOT enter search mode).
	view := b.View()
	// The search input placeholder should NOT appear when not in search mode.
	if strings.Contains(view, "Search...") {
		t.Errorf("View() in normal mode should NOT contain search placeholder %q", "Search...")
	}
}

// --- Empty State ---

func TestSearchMode_View_ShowsNoMatchingCards(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Fix login bug", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Add dashboard", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 3, Title: "Update docs", Labels: []provider.Label{{Name: "docs"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Enter search mode and type a query that matches nothing.
	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "zzzznonexistent" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Verify precondition: no cards match.
	filtered := b.filteredCards()
	if len(filtered) != 0 {
		t.Fatalf("precondition: filteredCards() = %d, want 0", len(filtered))
	}

	view := b.View()
	if !strings.Contains(view, "No matching cards") {
		t.Errorf("View() with zero matching cards should contain %q, not found in view", "No matching cards")
	}
}

func TestSearchMode_View_NoEmptyMessage_WhenCardsMatch(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Fix login bug", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Add dashboard", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 3, Title: "Update docs", Labels: []provider.Label{{Name: "docs"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Enter search mode and type a query that matches some cards.
	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Verify precondition: some cards match.
	filtered := b.filteredCards()
	if len(filtered) == 0 {
		t.Fatalf("precondition: filteredCards() = 0, want > 0")
	}

	view := b.View()
	if strings.Contains(view, "No matching cards") {
		t.Errorf("View() with matching cards should NOT contain %q", "No matching cards")
	}
}

// --- Filtered Count in Border Title ---

func TestSearchMode_View_BorderTitleShowsFilteredCount(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Fix login bug", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Add dashboard", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 3, Title: "Fix crash bug", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 4, Title: "Update docs", Labels: []provider.Label{{Name: "docs"}}},
		{Number: 5, Title: "Fix timeout bug", Labels: []provider.Label{{Name: "bug"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// Enter search mode and type "bug" to match cards with "bug" in the title.
	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Verify precondition: 3 of 5 cards match.
	filtered := b.filteredCards()
	totalCards := len(b.Columns[b.ActiveTab].Cards)
	if len(filtered) != 3 || totalCards != 5 {
		t.Fatalf("precondition: filtered = %d (want 3), total = %d (want 5)", len(filtered), totalCards)
	}

	view := b.View()
	// The border title should show "3/5" (filtered/total) format.
	expectedCount := fmt.Sprintf("%d/%d", len(filtered), totalCards)
	if !strings.Contains(view, expectedCount) {
		t.Errorf("View() border title should contain filtered count %q, not found in view", expectedCount)
	}
}

func TestSearchMode_View_BorderTitleShowsTotalCount_WhenNoSearch(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Fix login bug", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Add dashboard", Labels: []provider.Label{{Name: "feature"}}},
		{Number: 3, Title: "Update docs", Labels: []provider.Label{{Name: "docs"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	// No search active -- the border title should show the total count in (N) format.
	view := b.View()
	totalCards := len(b.Columns[b.ActiveTab].Cards)
	totalCount := fmt.Sprintf("(%d)", totalCards)
	if !strings.Contains(view, totalCount) {
		t.Errorf("View() border title without search should contain total count %q, not found in view", totalCount)
	}

	// The view should NOT contain a slash-separated count format.
	slashCount := fmt.Sprintf("/%d)", totalCards)
	if strings.Contains(view, slashCount) {
		t.Errorf("View() border title without search should NOT contain filtered count format %q", slashCount)
	}
}

// --- Status Bar Hints ---

func TestSearchMode_View_StatusBarShowsSearchHints(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Enter search mode.
	b = sendKey(t, b, keyMsg("/"))

	view := b.View()
	// The status bar should show the search mode hints: "esc" key and "Clear" description.
	if !strings.Contains(view, "esc") {
		t.Errorf("View() in search mode should contain hint key %q in status bar", "esc")
	}
	if !strings.Contains(view, "Clear") {
		t.Errorf("View() in search mode should contain hint desc %q in status bar", "Clear")
	}
}

// --- Search Input Prompt ---

func TestSearchMode_SearchInputPrompt(t *testing.T) {
	b := newLoadedTestBoard(t)

	// The search input should have "/ " as its Prompt value.
	expectedPrompt := "/ "
	if b.searchInput.Prompt != expectedPrompt {
		t.Errorf("searchInput.Prompt = %q, want %q", b.searchInput.Prompt, expectedPrompt)
	}
}

// --- Registry routing (#540 PR 2/2) ---
//
// handleSearchModeKey cuts over from a hardcoded switch msg.Type to
// b.textBinding(keymap.ModeSearch, msg) (keymap_text.go), mirroring
// handleCreateModeKey/handleConfigModeKey (#540 PR 1/2): a resolved
// search.* command dispatches through runSearchCommand; anything else --
// no match, OutcomePending, a non-command binding, or a foreign command id
// -- falls through to the searchInput textinput, since search is one of
// keymap.Mode.ConsumesPrintableRunes()'s modes.

// TestSearchMode_PrintableRunesReachInput_NotInterceptedAsCommands extends
// the existing digit/j/k passthrough coverage (TestSearchMode_
// DigitsTypeIntoQuery, TestSearchMode_JKTypeIntoQuery) with "y"/"n" -- the
// letters close_confirm/label_confirm bind as commands elsewhere in the
// catalog -- to guard against a future default-table change accidentally
// binding a printable rune into ModeSearch: textBinding's
// ConsumesPrintableRunes guard must keep refusing bare printable runes here
// regardless.
func TestSearchMode_PrintableRunesReachInput_NotInterceptedAsCommands(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	requireColumns(t, b)

	b = sendKey(t, b, keyMsg("/"))
	for _, ch := range "jkyn42" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	if b.searchQuery != "jkyn42" {
		t.Errorf("searchQuery = %q, want %q (printable runes, including y/n, must reach the textinput)", b.searchQuery, "jkyn42")
	}
	if b.mode != searchMode {
		t.Errorf("mode after typing printable runes = %d, want %d (searchMode; none of them may dispatch a command)", b.mode, searchMode)
	}
}

// TestSearchMode_RemapCancelKey_DispatchStaysInSync mirrors
// TestCreateMode_RemapCancelKey_DispatchAndHintStaySync (create_mode_test.go,
// #540 PR 1/2): unbinding "esc" from search.cancel and rebinding it onto
// "f1" must make the old key a no-op and the new key cancel the search.
func TestSearchMode_RemapCancelKey_DispatchStaysInSync(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("/"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeSearch: {
			"esc": keymap.UnboundBinding(),
			"f1":  keymap.CommandBinding(keymap.CommandSearchCancel),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("b"))

	before := b.mode
	b2 := sendKey(t, b, arrowMsg(tea.KeyEsc))
	if b2.mode != before {
		t.Errorf("mode after unbound (now-old) esc = %v, want unchanged %v", b2.mode, before)
	}
	if b2.searchQuery == "" {
		t.Error("unbound esc should not clear the search query")
	}

	b3 := sendKey(t, b, arrowMsg(tea.KeyF1))
	if b3.mode != normalMode {
		t.Errorf("mode after remapped 'f1' = %v, want normalMode", b3.mode)
	}
	if b3.searchQuery != "" {
		t.Errorf("searchQuery after cancelling via remapped 'f1' = %q, want empty (search.cancel clears the query)", b3.searchQuery)
	}
}

// TestSearchMode_RemapNavigateKeys_OldKeysNoLongerNavigateNewKeysDo is
// TestSearchMode_RemapCancelKey_DispatchStaysInSync's Navigate sibling,
// unbinding search's default up/down/ctrl+p/ctrl+n from
// search.prev_result/search.next_result and rebinding onto ctrl+k/ctrl+j
// (non-printable, per .claude/rules/testing.md's "Keymap Remap Tests for
// Text-Input Modes" lesson).
func TestSearchMode_RemapNavigateKeys_OldKeysNoLongerNavigateNewKeysDo(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Bug: login fails", Labels: []provider.Label{{Name: "bug"}}},
		{Number: 2, Title: "Bug: crash on load", Labels: []provider.Label{{Name: "bug"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	b = sendKey(t, b, keyMsg("/"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeSearch: {
			"up":     keymap.UnboundBinding(),
			"down":   keymap.UnboundBinding(),
			"ctrl+p": keymap.UnboundBinding(),
			"ctrl+n": keymap.UnboundBinding(),
			"ctrl+k": keymap.CommandBinding(keymap.CommandSearchPrevResult),
			"ctrl+j": keymap.CommandBinding(keymap.CommandSearchNextResult),
		},
	}, nil)
	before := b.Columns[b.ActiveTab].Cursor

	b2 := sendKey(t, b, arrowMsg(tea.KeyDown))
	if got := b2.Columns[b2.ActiveTab].Cursor; got != before {
		t.Errorf("cursor after old (now unbound) down = %d, want unchanged %d", got, before)
	}
	b3 := sendKey(t, b, arrowMsg(tea.KeyCtrlN))
	if got := b3.Columns[b3.ActiveTab].Cursor; got != before {
		t.Errorf("cursor after old (now unbound) ctrl+n = %d, want unchanged %d", got, before)
	}

	b4 := sendKey(t, b, arrowMsg(tea.KeyCtrlJ))
	if got := b4.Columns[b4.ActiveTab].Cursor; got == before {
		t.Error("cursor unchanged after remapped 'ctrl+j', want it to advance to the next result")
	}
}

// TestSearchMode_EscBoundToAction_FallsThroughToTextinputNotDispatched and
// TestSearchMode_ForeignCommandIDBoundToKey_FallsThroughToTextinputNotDispatched
// cover two of handleSearchModeKey's documented fall-through outcomes,
// mirroring TestCreateMode_EscBoundToAction_FallsThroughToTextinputNotDispatched
// / TestCreateMode_ForeignCommandIDBoundToKey_FallsThroughToTextinputNotDispatched
// (create_mode_test.go, #540 PR 1/2): a resolved non-command BindingAction,
// and a command id valid elsewhere in the catalog but foreign to
// ModeSearch's own six ids. Neither may dispatch.
func TestSearchMode_EscBoundToAction_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("/"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeSearch: {
			"esc": keymap.ActionBinding(keymap.Action{Name: "Noop", Type: "url", URL: "https://example.com/{number}"}),
		},
	}, nil)
	before := b.searchInput.Value()

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != searchMode {
		t.Errorf("mode after esc resolved to a non-command action = %v, want unchanged searchMode (must not dispatch the action)", b2.mode)
	}
	if got := b2.searchInput.Value(); got != before {
		t.Errorf("searchInput.Value() = %q after esc resolved to a non-command action, want unchanged %q (falls through to the textinput)", got, before)
	}
}

func TestSearchMode_ForeignCommandIDBoundToKey_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("/"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeSearch: {
			"f1": keymap.CommandBinding(keymap.CommandCloseConfirmConfirm),
		},
	}, nil)
	before := b.searchInput.Value()

	m, _ := b.Update(arrowMsg(tea.KeyF1))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != searchMode {
		t.Errorf("mode after a foreign command id (close_confirm.confirm) bound into ModeSearch = %v, want unchanged searchMode", b2.mode)
	}
	if got := b2.searchInput.Value(); got != before {
		t.Errorf("searchInput.Value() = %q after a foreign command id bound into ModeSearch, want unchanged %q (falls through to the textinput, not dispatched)", got, before)
	}
}

// --- searchHints(): byte-identity, arrow-preferred Navigate glyph, remap ---

// TestSearchHints_DefaultParityMatchesTodaysLiteral asserts
// b.searchHints() renders byte-identically to the deleted searchModeHints
// package var (pre-#540 model.go) under the default table, including the
// "↑/↓" Navigate glyph (Q3 of the #540 plan: search's ctrl+n/ctrl+p are
// aliases for up/down, the inverse of normal mode's j/k-primary
// convention, which is why the arrow-preferred tie-break is needed here).
func TestSearchHints_DefaultParityMatchesTodaysLiteral(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("/"))

	hints := b.searchHints()
	want := []Hint{
		{Key: "enter", Desc: "Apply"},
		{Key: "esc", Desc: "Clear"},
		{Key: "↑/↓", Desc: "Navigate"},
	}
	if !reflect.DeepEqual(hints, want) {
		t.Errorf("searchHints() = %+v, want %+v (today's inline literal, byte-identical)", hints, want)
	}
}

// TestSearchHints_RemapNavigateKeysOffArrows_ReflectsRawKeyNames is the Q3
// arrow-preferred/glyph-substituted case: once search.prev_result/
// search.next_result are remapped onto keys with no arrow glyph
// (ctrl+k/ctrl+j), the Navigate hint must show the raw key names instead of
// a stale "↑/↓" glyph.
func TestSearchHints_RemapNavigateKeysOffArrows_ReflectsRawKeyNames(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("/"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeSearch: {
			"up":     keymap.UnboundBinding(),
			"down":   keymap.UnboundBinding(),
			"ctrl+p": keymap.UnboundBinding(),
			"ctrl+n": keymap.UnboundBinding(),
			"ctrl+k": keymap.CommandBinding(keymap.CommandSearchPrevResult),
			"ctrl+j": keymap.CommandBinding(keymap.CommandSearchNextResult),
		},
	}, nil)

	hints := b.searchHints()
	if got := hintDesc(t, hints, "ctrl+k/ctrl+j"); got != "Navigate" {
		t.Errorf("searchHints() after remap onto ctrl+k/ctrl+j: Desc for %q = %q, want %q", "ctrl+k/ctrl+j", got, "Navigate")
	}
	for _, h := range hints {
		if h.Key == "↑/↓" {
			t.Errorf("searchHints() still advertises the stale '↑/↓' glyph after remapping off up/down/ctrl+p/ctrl+n, got %+v", hints)
		}
	}
}

// TestSearchMode_PerKeystrokeHintRefresh_ReflectsRemapAppliedAfterEntry
// asserts the per-keystroke hint refresh (mode_handlers.go's default branch
// in handleSearchModeKey) re-derives hints from the *current* keymap on
// every rune keypress, not just at mode entry: remapping after entering
// search mode, then typing a rune, must be reflected in b.statusBar's
// rendered hints -- asserted via the hints slice itself
// (.claude/rules/testing.md forbids a call-count assertion here).
func TestSearchMode_PerKeystrokeHintRefresh_ReflectsRemapAppliedAfterEntry(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("/"))
	if hintDesc(t, b.statusBar.hints, "esc") != "Clear" {
		t.Fatalf("precondition: entering search mode should show the default 'esc' Clear hint")
	}

	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeSearch: {
			"esc": keymap.UnboundBinding(),
			"f1":  keymap.CommandBinding(keymap.CommandSearchCancel),
		},
	}, nil)

	b = sendKey(t, b, keyMsg("a"))

	for _, h := range b.statusBar.hints {
		if h.Key == "esc" {
			t.Errorf("statusBar hints after typing a rune still advertise the unbound 'esc' key, want the per-keystroke refresh to pick up the remap: %+v", b.statusBar.hints)
		}
	}
	if got := hintDesc(t, b.statusBar.hints, "f1"); got != "Clear" {
		t.Errorf("statusBar hints after typing a rune: Desc for remapped 'f1' = %q, want %q", got, "Clear")
	}
}

// --- hint<->dispatch invariant across default and remapped tables ---

// TestSearchHints_KeysAlwaysDispatch_DefaultAndRemappedTables is the
// hint<->dispatch invariant test named in the #540 plan's Explicit Risk
// Coverage, mirroring TestCreateModalHints_KeysAlwaysDispatch_
// AllFocusPositionsDefaultAndRemappedTables (create_mode_test.go): every key
// b.searchHints() advertises must resolve through b.textBinding against
// ModeSearch -- the real dispatch path handleSearchModeKey uses.
func TestSearchHints_KeysAlwaysDispatch_DefaultAndRemappedTables(t *testing.T) {
	glyphToKey := map[string]string{"↑": "up", "↓": "down"}
	keyMsgForLabel := func(label string) tea.KeyMsg {
		if raw, ok := glyphToKey[label]; ok {
			label = raw
		}
		switch label {
		case "enter":
			return arrowMsg(tea.KeyEnter)
		case "esc":
			return arrowMsg(tea.KeyEsc)
		case "up":
			return arrowMsg(tea.KeyUp)
		case "down":
			return arrowMsg(tea.KeyDown)
		case "ctrl+n":
			return arrowMsg(tea.KeyCtrlN)
		case "ctrl+p":
			return arrowMsg(tea.KeyCtrlP)
		case "ctrl+j":
			return arrowMsg(tea.KeyCtrlJ)
		case "ctrl+k":
			return arrowMsg(tea.KeyCtrlK)
		case "f1":
			return arrowMsg(tea.KeyF1)
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
				keymap.ModeSearch: {
					"esc":    keymap.UnboundBinding(),
					"f1":     keymap.CommandBinding(keymap.CommandSearchCancel),
					"up":     keymap.UnboundBinding(),
					"down":   keymap.UnboundBinding(),
					"ctrl+p": keymap.UnboundBinding(),
					"ctrl+n": keymap.UnboundBinding(),
					"ctrl+k": keymap.CommandBinding(keymap.CommandSearchPrevResult),
					"ctrl+j": keymap.CommandBinding(keymap.CommandSearchNextResult),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newLoadedTestBoard(t)
			b = sendKey(t, b, keyMsg("/"))
			if tc.modes != nil {
				b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			}

			for _, h := range b.searchHints() {
				if h.Key == "" {
					continue
				}
				for _, key := range strings.Split(h.Key, "/") {
					if _, ok := b.textBinding(keymap.ModeSearch, keyMsgForLabel(key)); !ok {
						t.Errorf("hint %+v: textBinding(ModeSearch, %q) not found, want a match (every advertised key must actually dispatch through the real handler)", h, key)
					}
				}
			}
		})
	}
}

// --- ctrl+c always quits, regardless of user keymap config ---

func TestSearchMode_CtrlCQuits_EvenWithOverriddenKeymap(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("/"))
	// Attempt to steal ctrl+c for something else -- update.go's global
	// ctrl+c-always-quits check runs before any mode dispatch, so this must
	// have no effect.
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeSearch: {"ctrl+c": keymap.CommandBinding(keymap.CommandSearchApply)},
	}, nil)

	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in searchMode should return a non-nil Cmd (tea.Quit), even with an overridden keymap")
	}
}

// --- runNormalCommand's search-entry hint set derives from the registry ---

// TestSearchMode_EnterFromNormalMode_HintsDeriveFromRegistry covers the
// #540 plan's shared bullet: runNormalCommand's CommandBoardSearch case
// (mode_handlers.go, pre-#540 line 243) must set the status bar's hints via
// b.searchHints() rather than the deleted searchModeHints package var, so a
// keymap remap made *before* ever entering search mode is reflected the
// very first time it's entered -- not just after the per-keystroke refresh
// covered separately above.
func TestSearchMode_EnterFromNormalMode_HintsDeriveFromRegistry(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeSearch: {
			"esc": keymap.UnboundBinding(),
			"f1":  keymap.CommandBinding(keymap.CommandSearchCancel),
		},
	}, nil)

	b = sendKey(t, b, keyMsg("/"))

	if b.mode != searchMode {
		t.Fatalf("precondition: mode = %d, want %d (searchMode)", b.mode, searchMode)
	}
	for _, h := range b.statusBar.hints {
		if h.Key == "esc" {
			t.Errorf("statusBar hints right after entering search mode still show the unbound 'esc' key, want the registry-derived hint set: %+v", b.statusBar.hints)
		}
	}
	if got := hintDesc(t, b.statusBar.hints, "f1"); got != "Clear" {
		t.Errorf("statusBar hints right after entering search mode: Desc for remapped 'f1' = %q, want %q", got, "Clear")
	}
}

// --- #637: sub-issue relatives surfaced by search ---
//
// Extends matchesSearch/filteredCards so a direct match (title, label, own
// number, or ParentNumber) also surfaces that card's immediate sub-issue
// relatives in both directions, one hop only, seeds detected board-wide.
// See /workspace/.plans/637-search-subissue-relatives.md.

// newMultiColumnSearchTestBoard builds a loaded Board from explicit
// provider.Column data spanning more than one column, with ActiveTab left at
// its default (0). The existing newBoardWithInlineCards (helpers_test.go) is
// single-column only, and newActionTestBoardWithColumns drags in a
// FakeExecutor this work does not need (plan Q&A #3) -- board-wide seed
// detection and cross-column expansion require cards split across columns
// while the active column stays fixed.
func newMultiColumnSearchTestBoard(t *testing.T, columns []provider.Column) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: columns}})
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = 120
	board.Height = 40
	return board
}

// --- Query normalization: '#' stripping ---

func TestSearchMode_HashPrefixQuery_MatchesLikeBareNumber(t *testing.T) {
	cards := []provider.Card{
		{Number: 637, Title: "Some task"},
		{Number: 99, Title: "Other task"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.searchQuery = "#637"
	filteredHash := b.filteredCards()

	b.searchQuery = "637"
	filteredBare := b.filteredCards()

	if len(filteredHash) != 1 || filteredHash[0].Number != 637 {
		t.Fatalf("filteredCards() for '#637': got %+v, want exactly card #637", filteredHash)
	}
	if len(filteredBare) != len(filteredHash) || filteredBare[0].Number != filteredHash[0].Number {
		t.Errorf("filteredCards() for '#637' = %+v, want the same result as bare '637' = %+v", filteredHash, filteredBare)
	}
}

func TestSearchMode_DoubleHashPrefixQuery_DoesNotMatchByNumber(t *testing.T) {
	cards := []provider.Card{
		{Number: 637, Title: "Some task"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.searchQuery = "##637"
	filtered := b.filteredCards()

	if len(filtered) != 0 {
		t.Errorf("filteredCards() for '##637': got %d cards, want 0 (only a single leading '#' is stripped, so '##637' must not match #637 by number or as raw title/label text)", len(filtered))
	}
}

func TestSearchMode_BareHashQuery_MatchesNothingByNumberButMatchesLiteralHashText(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Fix login bug"},
		{Number: 2, Title: "Handle the #hashtag feature"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.searchQuery = "#"
	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() for bare '#': got %d cards, want exactly 1 (an empty stripped number form must skip all number comparisons; only the title with a literal '#' should match)", len(filtered))
	}
	if filtered[0].Number != 2 {
		t.Errorf("filteredCards() for bare '#' matched card #%d, want #2 (title containing literal '#')", filtered[0].Number)
	}
}

func TestSearchMode_RawQueryMatchesLiteralHashInLabel(t *testing.T) {
	cards := []provider.Card{
		{Number: 500, Title: "Task one", Labels: []provider.Label{{Name: "needs-review"}}},
		{Number: 700, Title: "Task two", Labels: []provider.Label{{Name: "priority-#1"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.searchQuery = "#1"
	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() for '#1': got %d cards, want exactly 1 (label text match only; numbers 500/700 don't contain the stripped form '1')", len(filtered))
	}
	if filtered[0].Number != 700 {
		t.Errorf("filteredCards() for '#1' matched card #%d, want #700 (label %q contains the literal raw query text)", filtered[0].Number, "priority-#1")
	}
}

func TestSearchMode_TitleContainingHash_MatchesRawQuery(t *testing.T) {
	cards := []provider.Card{
		{Number: 500, Title: "Support C#interop"},
		{Number: 700, Title: "Other task"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.searchQuery = "c#interop"
	filtered := b.filteredCards()

	if len(filtered) != 1 || filtered[0].Number != 500 {
		t.Fatalf("filteredCards() for 'c#interop': got %+v, want exactly card #500 (title contains the literal '#', raw query must still match text)", filtered)
	}
}

// --- ParentNumber direct-match rule ---

func TestSearchMode_ParentNumberMatch_SurfacesCardWithOffBoardParent(t *testing.T) {
	cards := []provider.Card{
		{Number: 55, Title: "Sub task", ParentNumber: 900}, // parent #900 is not present on the board
		{Number: 56, Title: "Unrelated task"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.searchQuery = "900"
	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() for '900': got %d cards, want exactly 1 (ParentNumber match works even though #900 is not itself on the board)", len(filtered))
	}
	if filtered[0].Number != 55 {
		t.Errorf("filteredCards() for '900' matched card #%d, want #55 (ParentNumber == 900)", filtered[0].Number)
	}
}

func TestSearchMode_NumberSubstring_MatchesOwnNumberAndParentNumber(t *testing.T) {
	cards := []provider.Card{
		{Number: 637, Title: "Epic task"},
		{Number: 40, Title: "Sub task", ParentNumber: 637},
		{Number: 41, Title: "Unrelated task"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.searchQuery = "63"
	filtered := b.filteredCards()

	got := map[int]bool{}
	for _, c := range filtered {
		got[c.Number] = true
	}
	if !got[637] {
		t.Errorf("filteredCards() for '63' should include card #637 (own-number substring match), got %+v", filtered)
	}
	if !got[40] {
		t.Errorf("filteredCards() for '63' should include card #40 (ParentNumber 637 contains the substring '63'), got %+v", filtered)
	}
	if got[41] {
		t.Errorf("filteredCards() for '63' should not include card #41 (neither its own number nor a relationship matches), got %+v", filtered)
	}
}

func TestSearchMode_ZeroQuery_DoesNotMatchParentlessCardsViaSentinel(t *testing.T) {
	cards := []provider.Card{
		{Number: 5, Title: "Parentless task"}, // ParentNumber defaults to the zero-value sentinel
		{Number: 10, Title: "Task with number containing zero"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.searchQuery = "0"
	filtered := b.filteredCards()

	for _, c := range filtered {
		if c.Number == 5 {
			t.Errorf("filteredCards() for '0' should not match card #5 merely because its ParentNumber is the zero-value sentinel (no parent), got %+v", filtered)
		}
	}
	found := false
	for _, c := range filtered {
		if c.Number == 10 {
			found = true
		}
	}
	if !found {
		t.Errorf("filteredCards() for '0' should still match card #10 by its own number substring, got %+v", filtered)
	}
}

// --- Bidirectional one-hop expansion ---

func TestSearchMode_ChildPulledIn_ByParentMatchingInDifferentColumn(t *testing.T) {
	columns := []provider.Column{
		{Title: "Active Column", Cards: []provider.Card{
			{Number: 20, Title: "Child task", ParentNumber: 10},
			{Number: 21, Title: "Unrelated task"},
		}},
		{Title: "Other Column", Cards: []provider.Card{
			{Number: 10, Title: "Epic Feature"},
		}},
	}
	b := newMultiColumnSearchTestBoard(t, columns)

	b.searchQuery = "epic"
	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() for 'epic': got %d cards in the active column, want exactly 1 (child pulled in by its matching parent in another column)", len(filtered))
	}
	if filtered[0].Number != 20 {
		t.Errorf("filteredCards() for 'epic' matched card #%d in the active column, want #20 (child whose ParentNumber is the matching parent's number)", filtered[0].Number)
	}
}

func TestSearchMode_ParentPulledIn_ByChildMatchingInNonActiveColumn(t *testing.T) {
	columns := []provider.Column{
		{Title: "Active Column", Cards: []provider.Card{
			{Number: 10, Title: "Epic Feature"},
			{Number: 11, Title: "Unrelated epic"},
		}},
		{Title: "Other Column", Cards: []provider.Card{
			{Number: 30, Title: "Login bug fix", ParentNumber: 10},
		}},
	}
	b := newMultiColumnSearchTestBoard(t, columns)

	b.searchQuery = "login"
	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() for 'login': got %d cards in the active column, want exactly 1 (parent pulled in by its matching child in a non-active column)", len(filtered))
	}
	if filtered[0].Number != 10 {
		t.Errorf("filteredCards() for 'login' matched card #%d in the active column, want #10 (parent whose number equals the matching child's ParentNumber)", filtered[0].Number)
	}
}

// TestSearchMode_ExpansionTriggeredByAnyMatchKind is the table-style
// coverage requested by the plan: expansion must behave identically whether
// the seed matched by title, label, own number, or the retained ParentNumber
// direct-match rule.
func TestSearchMode_ExpansionTriggeredByAnyMatchKind(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{name: "seed matches by title", query: "gamma widget"},
		{name: "seed matches by label", query: "urgent-flag"},
		{name: "seed matches by own number", query: "888"},
		{name: "seed matches by ParentNumber", query: "999"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			columns := []provider.Column{
				{Title: "Active Column", Cards: []provider.Card{
					{Number: 500, Title: "Child of gamma widget", ParentNumber: 888},
				}},
				{Title: "Other Column", Cards: []provider.Card{
					{Number: 888, Title: "Gamma Widget", Labels: []provider.Label{{Name: "urgent-flag"}}, ParentNumber: 999},
				}},
			}
			b := newMultiColumnSearchTestBoard(t, columns)
			b.searchQuery = tc.query
			filtered := b.filteredCards()

			found := false
			for _, c := range filtered {
				if c.Number == 500 {
					found = true
				}
			}
			if !found {
				t.Errorf("filteredCards() for %q: child #500 not pulled into the active column, want expansion regardless of how the seed (#888) itself matched; got %+v", tc.query, filtered)
			}
		})
	}
}

// TestSearchMode_OneHopBoundary_GrandparentMatchPullsParentNotGrandchild
// covers the "One hop only" AC: with a grandparent -> parent -> child chain,
// a query matching only the grandparent must pull in the parent but not the
// grandchild.
func TestSearchMode_OneHopBoundary_GrandparentMatchPullsParentNotGrandchild(t *testing.T) {
	columns := []provider.Column{
		{Title: "Active Column", Cards: []provider.Card{
			{Number: 2, Title: "Middle task", ParentNumber: 1},
			{Number: 3, Title: "Leaf task", ParentNumber: 2},
		}},
		{Title: "Other Column", Cards: []provider.Card{
			{Number: 1, Title: "Root Epic"},
		}},
	}
	b := newMultiColumnSearchTestBoard(t, columns)

	b.searchQuery = "root epic"
	filtered := b.filteredCards()

	got := map[int]bool{}
	for _, c := range filtered {
		got[c.Number] = true
	}
	if !got[2] {
		t.Errorf("filteredCards() for 'root epic' should include card #2 (one hop: parent of the matching grandparent #1), got %+v", filtered)
	}
	if got[3] {
		t.Errorf("filteredCards() for 'root epic' should NOT include card #3 (two hops from the matching grandparent #1; expansion is one hop only), got %+v", filtered)
	}
}

// TestSearchMode_SeedsDetectedAcrossAllColumns covers the "Seeds are
// board-wide" AC with a seed placed in a third, non-adjacent column.
func TestSearchMode_SeedsDetectedAcrossAllColumns(t *testing.T) {
	columns := []provider.Column{
		{Title: "Active Column", Cards: []provider.Card{
			{Number: 50, Title: "Child task", ParentNumber: 40},
		}},
		{Title: "Middle Column", Cards: []provider.Card{
			{Number: 999, Title: "Unrelated"},
		}},
		{Title: "Far Column", Cards: []provider.Card{
			{Number: 40, Title: "Distant Epic"},
		}},
	}
	b := newMultiColumnSearchTestBoard(t, columns)

	b.searchQuery = "distant epic"
	filtered := b.filteredCards()

	if len(filtered) != 1 || filtered[0].Number != 50 {
		t.Fatalf("filteredCards() for 'distant epic': got %+v, want exactly card #50 (seed detected in a third, non-adjacent column)", filtered)
	}
}

// TestSearchMode_GlobalFilter_GatesDisplayButNotSeeding covers "the active
// global filter is a hard gate on what's displayed, but does NOT gate
// seeding": a label filter excludes the matching epic from the results even
// though it still seeds its child's inclusion, and the existing
// filter-then-search order (filteredCards) is preserved.
func TestSearchMode_GlobalFilter_GatesDisplayButNotSeeding(t *testing.T) {
	cards := []provider.Card{
		{Number: 100, Title: "Sprint Planning"}, // no "keep" label; excluded by the active filter
		{Number: 101, Title: "Subtask A", ParentNumber: 100, Labels: []provider.Label{{Name: "keep"}}},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "keep"
	b.searchQuery = "sprint planning"

	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() with filter=keep, query='sprint planning': got %d cards, want exactly 1", len(filtered))
	}
	if filtered[0].Number != 101 {
		t.Errorf("filteredCards() matched card #%d, want #101 (the child passes the filter; the filtered-out epic seed must not itself display)", filtered[0].Number)
	}
	for _, c := range filtered {
		if c.Number == 100 {
			t.Errorf("filteredCards() should not display card #100 (fails the active label filter) even though it seeded the expansion that surfaced #101")
		}
	}
}

// TestSearchMode_SeedWithZeroParentNumber_ContributesNoParentSideLookup
// guards the sentinel handling in the parent-side expansion lookup itself
// (distinct from the direct-match sentinel guard above, per the plan's
// Risks section: "ParentNumber == 0 must be guarded at three sites"): a
// seed whose own ParentNumber is the zero-value sentinel must not seed a
// phantom "parent" relationship to any card numbered 0.
func TestSearchMode_SeedWithZeroParentNumber_ContributesNoParentSideLookup(t *testing.T) {
	cards := []provider.Card{
		{Number: 5, Title: "Parentless matching card"}, // seeds via title match; ParentNumber == 0
		{Number: 0, Title: "Should never surface via a phantom ParentNumber==0 relationship"},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	b.searchQuery = "parentless matching card"
	filtered := b.filteredCards()

	if len(filtered) != 1 {
		t.Fatalf("filteredCards() for 'parentless matching card': got %d cards, want exactly 1 (the seed itself only; a ParentNumber == 0 sentinel must not seed a phantom relationship to a card numbered 0)", len(filtered))
	}
	if filtered[0].Number != 5 {
		t.Errorf("filteredCards() matched card #%d, want #5", filtered[0].Number)
	}
}

// TestSearchMode_CursorAndScrollStayInBoundsAsExpansionGrowsResults drives
// keystrokes directly through b.Update, capturing both return values at
// every step (per .claude/rules/testing.md -- never discard either with
// "_"), asserting the active column's cursor/scroll stay valid as one-hop
// expansion balloons the filtered set from a single direct match (the epic)
// to all 20 cards (the epic plus its 19 children), then exercises ctrl+n
// wraparound across that expanded set.
func TestSearchMode_CursorAndScrollStayInBoundsAsExpansionGrowsResults(t *testing.T) {
	cards := []provider.Card{{Number: 1, Title: "Epic Rollout"}}
	for i := 2; i <= 20; i++ {
		cards = append(cards, provider.Card{Number: i, Title: fmt.Sprintf("Rollout child %d", i), ParentNumber: 1})
	}
	b := newBoardWithInlineCards(t, cards, 40, 12)

	m, cmd := b.Update(keyMsg("/"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	b = board
	if cmd != nil {
		execCmds(cmd)
	}

	for _, ch := range "epic" {
		m, cmd = b.Update(keyMsg(string(ch)))
		board, ok = m.(Board)
		if !ok {
			t.Fatalf("Update returned %T, want Board", m)
		}
		b = board
		if cmd != nil {
			execCmds(cmd)
		}

		filtered := b.filteredCards()
		col := b.Columns[b.ActiveTab]
		if col.Cursor != 0 {
			t.Errorf("after typing %q: Cursor = %d, want 0 (typing resets cursor even as expansion grows the set)", b.searchQuery, col.Cursor)
		}
		if col.ScrollOffset != 0 {
			t.Errorf("after typing %q: ScrollOffset = %d, want 0", b.searchQuery, col.ScrollOffset)
		}
		if len(filtered) > 0 && col.Cursor >= len(filtered) {
			t.Fatalf("after typing %q: Cursor = %d out of bounds for filtered set of %d cards", b.searchQuery, col.Cursor, len(filtered))
		}
	}

	filtered := b.filteredCards()
	if len(filtered) != 20 {
		t.Fatalf("precondition: filteredCards() for 'epic' = %d, want 20 (seed #1 plus all 19 children via one-hop expansion)", len(filtered))
	}

	// Navigate to the end of the now-expanded (20-card) result set and
	// confirm ctrl+n wraps back to the first entry rather than leaving the
	// cursor pointing past the grown slice.
	for i := 0; i < len(filtered)-1; i++ {
		m, cmd = b.Update(arrowMsg(tea.KeyCtrlN))
		board, ok = m.(Board)
		if !ok {
			t.Fatalf("Update returned %T, want Board", m)
		}
		b = board
		if cmd != nil {
			execCmds(cmd)
		}
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != len(filtered)-1 {
		t.Fatalf("precondition: cursor = %d, want %d (last card of the expanded set)", got, len(filtered)-1)
	}

	m, cmd = b.Update(arrowMsg(tea.KeyCtrlN))
	board, ok = m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	b = board
	if cmd != nil {
		execCmds(cmd)
	}

	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("Cursor after ctrl+n past the end of the expanded (20-card) result set = %d, want 0 (wrap to first)", got)
	}
}
