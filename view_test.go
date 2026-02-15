package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func TestView_TabBarShowsAllTabNames(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()
	for _, title := range expectedColumnTitles {
		if !strings.Contains(view, title) {
			t.Errorf("View() does not contain tab name %q", title)
		}
	}
}

func TestView_ContainsActiveTabCardData(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()
	// Only the active tab's cards should appear in the view
	activeCol := b.Columns[b.ActiveTab]
	for _, card := range activeCol.Cards {
		if !strings.Contains(view, card.Title) {
			t.Errorf("View() does not contain active tab card title %q", card.Title)
		}
	}
}

func TestView_OnlyActiveTabCardsVisible(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Switch to tab 1 (Refined)
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	view := b.View()

	// Cards from tab 0 (New) should NOT be visible
	for _, card := range b.Columns[0].Cards {
		if strings.Contains(view, card.Title) {
			// Only fail if this card title doesn't also appear in the active tab
			found := false
			for _, activeCard := range b.Columns[b.ActiveTab].Cards {
				if activeCard.Title == card.Title {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("View() contains card title %q from inactive tab", card.Title)
			}
		}
	}
}

func TestView_ContainsHelpBar(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()

	// Status bar should show contextual hints for normalMode.
	expectedHints := []string{"n: New", "r: Refresh", "q: Quit"}
	for _, hint := range expectedHints {
		if !strings.Contains(view, hint) {
			t.Errorf("View() does not contain status bar hint %q", hint)
		}
	}

	// Old-style combined key hints should NOT appear.
	oldHints := []string{"h/l", "j/k"}
	for _, old := range oldHints {
		if strings.Contains(view, old) {
			t.Errorf("View() still contains old help text %q, want new status bar format", old)
		}
	}
}

func TestView_IsNotEmpty(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()
	if strings.TrimSpace(view) == "" {
		t.Error("View() returned empty string, want rendered board content")
	}
}

func TestView_NoScrollIndicators_WhenAllCardsFit(t *testing.T) {
	cardCount := 3
	height := cardCount + 6 + 10 // plenty of room
	b := newBoardWithCards(t, cardCount, height)

	view := b.View()
	if strings.Contains(view, "\u25b2") {
		t.Error("View should not contain up arrow indicator when all cards fit")
	}
	if strings.Contains(view, "\u25bc") {
		t.Error("View should not contain down arrow indicator when all cards fit")
	}
}

func TestView_DownIndicator_WhenCardsBelow(t *testing.T) {
	cardCount := 30
	height := 15 // panelHeight = 9, far fewer than 30 cards
	b := newBoardWithCards(t, cardCount, height)

	// ScrollOffset starts at 0, so there are cards below.
	view := b.View()
	if !strings.Contains(view, "\u25bc") {
		t.Error("View should contain down arrow indicator when more cards are below the viewport")
	}
	if strings.Contains(view, "\u25b2") {
		t.Error("View should not contain up arrow indicator when at the top")
	}
}

func TestView_UpIndicator_WhenCardsAbove(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll all the way down.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	view := b.View()
	if !strings.Contains(view, "\u25b2") {
		t.Error("View should contain up arrow indicator when cards are above the viewport")
	}
}

func TestView_BothIndicators_WhenMiddle(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Navigate to somewhere in the middle.
	panelHeight := height - 6
	for i := 0; i < panelHeight+2; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	view := b.View()
	if !strings.Contains(view, "\u25b2") {
		t.Error("View should contain up arrow indicator when scrolled to middle")
	}
	if !strings.Contains(view, "\u25bc") {
		t.Error("View should contain down arrow indicator when scrolled to middle")
	}
}

func TestView_WrapsLongTitle(t *testing.T) {
	longTitle := "This is a very long title that should definitely be wrapped in the card list panel"
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: longTitle, Labels: []string{"test"}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 80
	board.Height = 30

	view := board.View()

	// The full long title text should appear in the view (wrapped, not truncated).
	// Check that all words from the title are present somewhere in the view.
	for _, word := range strings.Fields(longTitle) {
		if !strings.Contains(view, word) {
			t.Errorf("View should contain word %q from the long title, but it does not", word)
		}
	}

	// There should be no truncation marker.
	if strings.Contains(view, "...") {
		t.Error("View should NOT contain '...' — titles should be wrapped, not truncated")
	}
}

func TestView_WrappedTitles_SelectedCardAllLinesStyled(t *testing.T) {
	longTitle := "This is a card title that is long enough to require wrapping across lines"
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: longTitle, Labels: []string{"test"}},
				{Number: 2, Title: "Short", Labels: []string{"test"}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 80
	board.Height = 30

	view := board.View()

	// All words from the wrapped title of the selected card should appear in the view.
	for _, word := range strings.Fields(longTitle) {
		if !strings.Contains(view, word) {
			t.Errorf("View should contain word %q from the selected card's wrapped title", word)
		}
	}
}

func TestView_WrappedTitles_PartialCardHidden(t *testing.T) {
	// Create a board where cards have titles long enough to wrap.
	// With limited height, the last card that would only partially fit should be hidden.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", false)

	var cards []provider.Card
	for i := 0; i < 10; i++ {
		cards = append(cards, provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf("Card %d with a very long title that wraps to take more vertical space", i+1),
			Labels: []string{"test"},
		})
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 60  // narrow width to force wrapping
	board.Height = 15 // short height to force some cards off-screen

	view := board.View()

	// With wrapping enabled and limited height, not all 10 cards should be fully visible.
	// Count how many card numbers appear in the view.
	visibleCount := 0
	for i := 0; i < 10; i++ {
		marker := fmt.Sprintf("#%d ", i+1)
		if strings.Contains(view, marker) {
			visibleCount++
		}
	}

	if visibleCount >= 10 {
		t.Errorf("expected fewer than 10 cards visible with wrapped titles in limited height, got %d", visibleCount)
	}
}

func TestView_BorderTitleShowsNumberPrefixes(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()

	// The view should contain number prefixes like "[1]" and "[2]" for column tab names.
	for i := range b.Columns {
		prefix := fmt.Sprintf("[%d]", i+1)
		if !strings.Contains(view, prefix) {
			t.Errorf("View() does not contain number prefix %q for column %d", prefix, i)
		}
	}
}

func TestView_HelpBarShowsNumberHint(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()

	// The status bar / help bar should contain a hint about number key navigation.
	// The exact format may vary (e.g., "1-4: Columns" or "1-4" as a key hint).
	columnCount := len(b.Columns)
	numberRange := fmt.Sprintf("1-%d", columnCount)
	if !strings.Contains(view, numberRange) {
		t.Errorf("View() help bar does not contain number key hint %q", numberRange)
	}
}

func TestBuildBorderTitle_WideTerm_ShowsFullTitles(t *testing.T) {
	columns := []Column{
		{Title: "New"},
		{Title: "Refined"},
		{Title: "Implementing"},
		{Title: "Implemented"},
	}
	title := buildBorderTitle(columns, 0, 120)
	titleWidth := lipgloss.Width(title)

	// All full column titles should appear.
	for _, col := range columns {
		if !strings.Contains(title, col.Title) {
			t.Errorf("buildBorderTitle() missing full title %q at width 120", col.Title)
		}
	}

	// Total rendered width must not exceed the requested width.
	if titleWidth > 120 {
		t.Errorf("buildBorderTitle() width = %d, want <= 120", titleWidth)
	}
}

func TestBuildBorderTitle_NarrowTerm_TruncatesTitles(t *testing.T) {
	columns := []Column{
		{Title: "New"},
		{Title: "Refined"},
		{Title: "Implementing"},
		{Title: "Implemented"},
	}
	// Width too narrow for all full titles but enough for truncated ones.
	// Full labels with card counts: "[1] New (0) ─ [2] Refined (0) ─ [3] Implementing (0) ─ [4] Implemented (0)"
	// That's ~74 chars of labels alone, plus prefix/suffix/fill.
	// At width 56, titles must be truncated.
	title := buildBorderTitle(columns, 0, 56)
	titleWidth := lipgloss.Width(title)

	// Total rendered width must not exceed the requested width.
	if titleWidth > 56 {
		t.Errorf("buildBorderTitle() width = %d, want <= 56", titleWidth)
	}

	// Number prefixes should still be present.
	for i := range columns {
		prefix := fmt.Sprintf("[%d]", i+1)
		if !strings.Contains(title, prefix) {
			t.Errorf("buildBorderTitle() missing number prefix %q at narrow width", prefix)
		}
	}
}

func TestBuildBorderTitle_VeryNarrowTerm_FallsBackToNumbersOnly(t *testing.T) {
	columns := []Column{
		{Title: "New"},
		{Title: "Refined"},
		{Title: "Implementing"},
		{Title: "Implemented"},
	}
	// Use a width where numbers-only fits but truncated titles do not.
	// Numbers-only with card counts: "[1] (0) ─ [2] (0) ─ [3] (0) ─ [4] (0)" + prefix (3) + suffix (1) + fill (2) ~ 44.
	// So width 45 should trigger numbers-only mode.
	title := buildBorderTitle(columns, 0, 45)
	titleWidth := lipgloss.Width(title)

	// Total rendered width must not exceed the requested width.
	if titleWidth > 45 {
		t.Errorf("buildBorderTitle() width = %d, want <= 45", titleWidth)
	}

	// Number prefixes should still be present in numbers-only mode.
	for i := range columns {
		prefix := fmt.Sprintf("[%d]", i+1)
		if !strings.Contains(title, prefix) {
			t.Errorf("buildBorderTitle() missing number prefix %q at narrow width", prefix)
		}
	}
}

func TestBuildBorderTitle_ExtremelyNarrowTerm_StillFitsWidth(t *testing.T) {
	columns := []Column{
		{Title: "New"},
		{Title: "Refined"},
		{Title: "Implementing"},
		{Title: "Implemented"},
	}
	// At width 15, even numbers-only can't fit. Should degrade gracefully.
	title := buildBorderTitle(columns, 0, 15)
	titleWidth := lipgloss.Width(title)

	if titleWidth > 15 {
		t.Errorf("buildBorderTitle() width = %d, want <= 15", titleWidth)
	}
}

func TestBuildBorderTitle_AlwaysWithinTotalWidth(t *testing.T) {
	columns := []Column{
		{Title: "New"},
		{Title: "Refined"},
		{Title: "Implementing"},
		{Title: "Implemented"},
	}
	// Test a range of widths to ensure the border title never exceeds totalWidth.
	for width := 15; width <= 150; width++ {
		title := buildBorderTitle(columns, 0, width)
		titleWidth := lipgloss.Width(title)
		if titleWidth > width {
			t.Errorf("buildBorderTitle() at totalWidth=%d: rendered width = %d, exceeds limit", width, titleWidth)
		}
	}
}

func TestBuildBorderTitle_WideTerm_ShowsCardCounts(t *testing.T) {
	columns := []Column{
		{Title: "New", Cards: []Card{{Number: 1, Title: "A"}, {Number: 2, Title: "B"}, {Number: 3, Title: "C"}}},
		{Title: "Refined", Cards: []Card{{Number: 4, Title: "D"}, {Number: 5, Title: "E"}, {Number: 6, Title: "F"}, {Number: 7, Title: "G"}}},
		{Title: "Implementing", Cards: []Card{{Number: 8, Title: "H"}, {Number: 9, Title: "I"}, {Number: 10, Title: "J"}, {Number: 11, Title: "K"}, {Number: 12, Title: "L"}}},
		{Title: "Implemented", Cards: []Card{{Number: 13, Title: "M"}, {Number: 14, Title: "N"}, {Number: 15, Title: "O"}, {Number: 16, Title: "P"}}},
	}

	title := buildBorderTitle(columns, 0, 120)

	expectedCounts := []struct {
		colTitle string
		count    string
	}{
		{"New", "(3)"},
		{"Refined", "(4)"},
		{"Implementing", "(5)"},
		{"Implemented", "(4)"},
	}
	for _, ec := range expectedCounts {
		if !strings.Contains(title, ec.count) {
			t.Errorf("buildBorderTitle() missing card count %s for column %q", ec.count, ec.colTitle)
		}
	}
}

func TestBuildBorderTitle_NumbersOnly_ShowsCardCounts(t *testing.T) {
	columns := []Column{
		{Title: "New", Cards: []Card{{Number: 1, Title: "A"}, {Number: 2, Title: "B"}, {Number: 3, Title: "C"}}},
		{Title: "Refined", Cards: []Card{{Number: 4, Title: "D"}, {Number: 5, Title: "E"}, {Number: 6, Title: "F"}, {Number: 7, Title: "G"}}},
		{Title: "Implementing", Cards: []Card{{Number: 8, Title: "H"}, {Number: 9, Title: "I"}, {Number: 10, Title: "J"}, {Number: 11, Title: "K"}, {Number: 12, Title: "L"}}},
		{Title: "Implemented", Cards: []Card{{Number: 13, Title: "M"}, {Number: 14, Title: "N"}, {Number: 15, Title: "O"}, {Number: 16, Title: "P"}}},
	}

	// Width ~45 should trigger numbers-only mode (accounting for card count suffixes).
	title := buildBorderTitle(columns, 0, 45)

	expectedCounts := []struct {
		colTitle string
		count    string
	}{
		{"New", "(3)"},
		{"Refined", "(4)"},
		{"Implementing", "(5)"},
		{"Implemented", "(4)"},
	}
	for _, ec := range expectedCounts {
		if !strings.Contains(title, ec.count) {
			t.Errorf("buildBorderTitle() at width 45, missing card count %s for column %q", ec.count, ec.colTitle)
		}
	}
}

func TestBuildBorderTitle_NoLabels_HidesCardCounts(t *testing.T) {
	columns := []Column{
		{Title: "New", Cards: []Card{{Number: 1, Title: "A"}, {Number: 2, Title: "B"}, {Number: 3, Title: "C"}}},
		{Title: "Refined", Cards: []Card{{Number: 4, Title: "D"}, {Number: 5, Title: "E"}, {Number: 6, Title: "F"}, {Number: 7, Title: "G"}}},
		{Title: "Implementing", Cards: []Card{{Number: 8, Title: "H"}, {Number: 9, Title: "I"}, {Number: 10, Title: "J"}, {Number: 11, Title: "K"}, {Number: 12, Title: "L"}}},
		{Title: "Implemented", Cards: []Card{{Number: 13, Title: "M"}, {Number: 14, Title: "N"}, {Number: 15, Title: "O"}, {Number: 16, Title: "P"}}},
	}

	// Extremely narrow: no labels should appear, and no card counts either.
	title := buildBorderTitle(columns, 0, 15)

	absentCounts := []string{"(3)", "(4)", "(5)"}
	for _, count := range absentCounts {
		if strings.Contains(title, count) {
			t.Errorf("buildBorderTitle() at width 15 should not contain card count %s, but it does", count)
		}
	}
}

func TestBuildBorderTitle_ZeroCards_ShowsZero(t *testing.T) {
	columns := []Column{
		{Title: "New", Cards: []Card{{Number: 1, Title: "A"}, {Number: 2, Title: "B"}, {Number: 3, Title: "C"}}},
		{Title: "Refined", Cards: nil},
		{Title: "Implementing", Cards: []Card{{Number: 8, Title: "H"}}},
		{Title: "Implemented", Cards: []Card{{Number: 13, Title: "M"}, {Number: 14, Title: "N"}}},
	}

	title := buildBorderTitle(columns, 0, 120)

	// Column with zero cards should show (0).
	if !strings.Contains(title, "(0)") {
		t.Errorf("buildBorderTitle() missing card count (0) for empty column %q", "Refined")
	}

	// Other counts should also be present.
	if !strings.Contains(title, "(3)") {
		t.Errorf("buildBorderTitle() missing card count (3) for column %q", "New")
	}
	if !strings.Contains(title, "(1)") {
		t.Errorf("buildBorderTitle() missing card count (1) for column %q", "Implementing")
	}
	if !strings.Contains(title, "(2)") {
		t.Errorf("buildBorderTitle() missing card count (2) for column %q", "Implemented")
	}
}

func TestBuildBorderTitle_CardCountUpdatesAfterChange(t *testing.T) {
	columns := []Column{
		{Title: "New", Cards: []Card{{Number: 1, Title: "A"}}},
		{Title: "Done", Cards: []Card{{Number: 2, Title: "B"}}},
	}

	titleBefore := buildBorderTitle(columns, 0, 120)
	if !strings.Contains(titleBefore, "(1)") {
		t.Fatalf("buildBorderTitle() before adding card: expected (1) to appear")
	}

	// Add a card to the first column.
	columns[0].Cards = append(columns[0].Cards, Card{Number: 3, Title: "C"})
	titleAfter := buildBorderTitle(columns, 0, 120)

	if !strings.Contains(titleAfter, "(2)") {
		t.Errorf("buildBorderTitle() after adding card: expected (2) to appear for column %q", "New")
	}
	// Column "Done" still has 1 card, so (1) should still appear.
	if !strings.Contains(titleAfter, "(1)") {
		t.Errorf("buildBorderTitle() after adding card: expected (1) to still appear for column %q", "Done")
	}
}

func TestBuildBorderTitle_AlwaysWithinTotalWidth_WithCards(t *testing.T) {
	columns := []Column{
		{Title: "New", Cards: []Card{{Number: 1, Title: "A"}, {Number: 2, Title: "B"}, {Number: 3, Title: "C"}}},
		{Title: "Refined", Cards: []Card{{Number: 4, Title: "D"}, {Number: 5, Title: "E"}, {Number: 6, Title: "F"}, {Number: 7, Title: "G"}}},
		{Title: "Implementing", Cards: []Card{{Number: 8, Title: "H"}, {Number: 9, Title: "I"}, {Number: 10, Title: "J"}, {Number: 11, Title: "K"}, {Number: 12, Title: "L"}}},
		{Title: "Implemented", Cards: []Card{{Number: 13, Title: "M"}, {Number: 14, Title: "N"}, {Number: 15, Title: "O"}, {Number: 16, Title: "P"}}},
	}

	for width := 15; width <= 150; width++ {
		title := buildBorderTitle(columns, 0, width)
		titleWidth := lipgloss.Width(title)
		if titleWidth > width {
			t.Errorf("buildBorderTitle() with cards at totalWidth=%d: rendered width = %d, exceeds limit", width, titleWidth)
		}
	}
}
