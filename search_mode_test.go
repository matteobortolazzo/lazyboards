package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

func TestSearchMode_NumberKey_ExitsAndSwitchesColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	requireColumns(t, b)

	// Enter search mode.
	b = sendKey(t, b, keyMsg("/"))

	// Press '2' to switch to column index 1.
	b = sendKey(t, b, keyMsg("2"))

	if b.mode != normalMode {
		t.Errorf("after '2' in search mode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if b.ActiveTab != 1 {
		t.Errorf("after '2' in search mode: ActiveTab = %d, want 1", b.ActiveTab)
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

func TestSearchMode_JK_NavigatesFilteredCards(t *testing.T) {
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

	// Cursor starts at 0 of the filtered list.
	// Press j to move to second filtered card.
	b = sendKey(t, b, keyMsg("j"))
	// Press j again to move to third (last) filtered card.
	b = sendKey(t, b, keyMsg("j"))
	// Press j again — should clamp at the last filtered card.
	b = sendKey(t, b, keyMsg("j"))

	// Cursor should not exceed the filtered count minus 1.
	col := b.Columns[b.ActiveTab]
	filteredCount := len(b.filteredCards())
	if col.Cursor >= filteredCount {
		t.Errorf("cursor = %d after j past end of filtered list, want < %d", col.Cursor, filteredCount)
	}

	// Press k to go back up.
	b = sendKey(t, b, keyMsg("k"))
	if b.Columns[b.ActiveTab].Cursor < 0 {
		t.Errorf("cursor = %d after k, want >= 0", b.Columns[b.ActiveTab].Cursor)
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
