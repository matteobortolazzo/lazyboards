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

	// Status bar should show only the "New" hint in normalMode.
	// Keys and descriptions are styled separately, so check each part individually.
	expectedKeys := []string{"n"}
	expectedDescs := []string{"New"}
	for i, key := range expectedKeys {
		if !strings.Contains(view, key) {
			t.Errorf("View() does not contain status bar key %q", key)
		}
		if !strings.Contains(view, expectedDescs[i]) {
			t.Errorf("View() does not contain status bar desc %q", expectedDescs[i])
		}
	}

	// Column, Quit, Config, and Refresh hints must NOT appear in the status bar.
	statusBarView := b.statusBar.View()
	for _, absent := range []string{"Column", "Quit", "Config", "Refresh"} {
		if strings.Contains(statusBarView, absent) {
			t.Errorf("statusBar.View() should NOT contain %q, but it does", absent)
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
	board := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: longTitle, Labels: []string{"test"}},
	}, 80, 30)

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
	board := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: longTitle, Labels: []string{"test"}},
		{Number: 2, Title: "Short", Labels: []string{"test"}},
	}, 80, 30)

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
	board := newBoardWithGeneratedCards(t, 10,
		"Card %d with a very long title that wraps to take more vertical space", 60, 15)

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

// --- PR Indicator Tests ---

func TestView_CardList_ShowsPRIndicator(t *testing.T) {
	b := newBoardWithPRs(t)
	view := b.View()

	// Cards with LinkedPRs should have the PR indicator symbol in the view.
	if !strings.Contains(view, "\ue728") {
		t.Error("View() should contain PR indicator \ue728 for cards with linked PRs")
	}
}

func TestView_CardList_NoPRIndicator_WhenNoPRs(t *testing.T) {
	b := newBoardWithPRs(t)
	view := b.View()

	// Find the line(s) for card 1 ("No PRs") and verify it does NOT have the indicator.
	// The card with no PRs is "#1 No PRs" -- its line should not contain the indicator.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "No PRs") {
			if strings.Contains(line, "\ue728") {
				t.Errorf("card with 0 PRs should NOT have PR indicator, but line %q contains it", line)
			}
		}
	}
}

// --- Working Indicator Tests ---

func TestView_CardList_ShowsWorkingIndicator(t *testing.T) {
	b := newBoardWithWorkingLabel(t)
	view := b.View()

	// Cards with the "Working" label should have the spinner icon in the view.
	if !strings.Contains(view, "\uf110") {
		t.Error("View() should contain Working indicator \uf110 for cards with 'Working' label")
	}
}

func TestView_CardList_NoWorkingIndicator_WhenNoWorkingLabel(t *testing.T) {
	b := newBoardWithWorkingLabel(t)
	view := b.View()

	// Find the line(s) for card 1 ("No indicators" with label "bug") and verify
	// it does NOT have the Working indicator.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "No indicators") {
			if strings.Contains(line, "\uf110") {
				t.Errorf("card without 'Working' label should NOT have Working indicator, but line %q contains it", line)
			}
		}
	}
}

func TestView_CardList_BothPRAndWorkingIndicators(t *testing.T) {
	b := newBoardWithWorkingLabel(t)
	view := b.View()

	// Card 4 has both a linked PR and the "Working" label.
	// The view should contain both indicator icons.
	if !strings.Contains(view, "\ue728") {
		t.Error("View() should contain PR indicator \ue728 for card with linked PR")
	}
	if !strings.Contains(view, "\uf110") {
		t.Error("View() should contain Working indicator \uf110 for card with 'Working' label")
	}

	// Verify ordering: PR icon (\ue728) should appear before Working icon (\uf110)
	// on the same line for card 4.
	lines := strings.Split(view, "\n")
	foundBothOnSameLine := false
	for _, line := range lines {
		prIdx := strings.Index(line, "\ue728")
		workIdx := strings.Index(line, "\uf110")
		if prIdx >= 0 && workIdx >= 0 {
			foundBothOnSameLine = true
			if prIdx >= workIdx {
				t.Errorf("PR indicator should appear before Working indicator, but PR at index %d, Working at index %d in line %q", prIdx, workIdx, line)
			}
		}
	}
	if !foundBothOnSameLine {
		t.Error("expected at least one line containing both PR indicator and Working indicator for card with both")
	}
}

func TestView_CardList_WorkingIndicator_CaseSensitive(t *testing.T) {
	// A card with "working" (lowercase) should NOT trigger the Working indicator.
	b := newBoardWithCustomCard(t, "Lowercase working", []string{"working"}, "")
	view := b.View()

	if strings.Contains(view, "\uf110") {
		t.Error("View() should NOT contain Working indicator \uf110 for card with lowercase 'working' label")
	}
}

// --- Background Refresh View ---

func TestView_BackgroundRefresh_ShowsRefreshingIndicator(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Set refreshing flag.
	b.refreshing = true

	view := b.View()

	// View should contain "Refreshing..." indicator in the help bar area.
	if !strings.Contains(view, "Refreshing...") {
		t.Errorf("View() with refreshing=true should contain %q, got:\n%s", "Refreshing...", view)
	}
}

func TestView_BackgroundRefresh_BoardStillVisible(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Set refreshing flag.
	b.refreshing = true

	view := b.View()

	// Board content (column titles) should still be visible, not replaced with a loading screen.
	for _, title := range expectedColumnTitles {
		if !strings.Contains(view, title) {
			t.Errorf("View() with refreshing=true should still contain column title %q (board visible)", title)
		}
	}

	// Should NOT show the full loading screen.
	if strings.Contains(view, "Loading board...") {
		t.Error("View() with refreshing=true should NOT show full 'Loading board...' screen")
	}
}

// --- Label Dot Tests ---

func TestView_CardList_ShowsLabelDots(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()

	// Cards have labels, so the card list should contain the dot character ●.
	if !strings.Contains(view, "\u25cf") {
		t.Error("View() card list should contain label dot \u25cf for cards with labels")
	}
}

func TestView_CardList_HidesWorkingLabelDot(t *testing.T) {
	// A card with labels ["Working", "bug"] should show the spinner icon
	// but only 1 dot (for "bug"), not 2. The "Working" label dot is hidden.
	b := newBoardWithCustomCard(t, "Fix crash", []string{"Working", "bug"}, "")
	view := b.View()

	// Spinner icon must still be present.
	if !strings.Contains(view, "\uf110") {
		t.Error("View() should contain Working spinner icon for card with 'Working' label")
	}

	// Count dots on the line(s) containing the card.
	lines := strings.Split(view, "\n")
	dotCount := 0
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "Fix crash") {
			dotCount += strings.Count(line, "\u25cf")
		}
	}
	if dotCount != 1 {
		t.Errorf("expected 1 label dot (for 'bug' only), got %d", dotCount)
	}
}

func TestView_CardList_HidesColumnNameLabelDot(t *testing.T) {
	// When a card has a label matching its column name, that label's dot
	// should be hidden. Only non-column-name labels get dots.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Backlog", Cards: []provider.Card{
				{Number: 1, Title: "Add feature", Labels: []string{"Backlog", "enhancement"}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 80
	board.Height = 20
	view := board.View()

	// Count dots on the line(s) containing the card.
	lines := strings.Split(view, "\n")
	dotCount := 0
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "Add feature") {
			dotCount += strings.Count(line, "\u25cf")
		}
	}
	if dotCount != 1 {
		t.Errorf("expected 1 label dot (for 'enhancement' only, not 'Backlog'), got %d", dotCount)
	}
}

func TestView_CardList_HidesWorkingCaseInsensitive(t *testing.T) {
	// The "working" label (lowercase) should also be hidden from the dot display,
	// even though it does NOT trigger the spinner icon (spinner is case-sensitive).
	b := newBoardWithCustomCard(t, "Some task", []string{"working", "bug"}, "")
	view := b.View()

	// Count dots on the line(s) containing the card.
	lines := strings.Split(view, "\n")
	dotCount := 0
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "Some task") {
			dotCount += strings.Count(line, "\u25cf")
		}
	}
	if dotCount != 1 {
		t.Errorf("expected 1 label dot (for 'bug' only, 'working' hidden case-insensitively), got %d", dotCount)
	}
}

func TestView_CardList_NoDots_WhenOnlyHiddenLabels(t *testing.T) {
	// A card with only ["Working"] should show the spinner icon but NO dots at all.
	b := newBoardWithCustomCard(t, "Solo working", []string{"Working"}, "")
	view := b.View()

	// Spinner icon must still be present.
	if !strings.Contains(view, "\uf110") {
		t.Error("View() should contain Working spinner icon for card with 'Working' label")
	}

	// No dots should appear on the card's line.
	lines := strings.Split(view, "\n")
	dotCount := 0
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "Solo working") {
			dotCount += strings.Count(line, "\u25cf")
		}
	}
	if dotCount != 0 {
		t.Errorf("expected 0 label dots when only hidden labels present, got %d", dotCount)
	}
}

func TestView_CardList_MixedHiddenLabels(t *testing.T) {
	// Column "To Do" with a card that has labels ["Working", "To Do", "bug", "urgent"].
	// "Working" and "To Do" (column name) should be hidden; only "bug" and "urgent" get dots.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "To Do", Cards: []provider.Card{
				{Number: 1, Title: "Important task", Labels: []string{"Working", "To Do", "bug", "urgent"}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	view := board.View()

	// Count dots on the line(s) containing the card.
	lines := strings.Split(view, "\n")
	dotCount := 0
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "Important task") {
			dotCount += strings.Count(line, "\u25cf")
		}
	}
	if dotCount != 2 {
		t.Errorf("expected 2 label dots (for 'bug' and 'urgent'), got %d", dotCount)
	}
}

func TestView_DetailPanel_StillShowsHiddenLabels(t *testing.T) {
	// The detail panel must display ALL labels including "Working" and column-name
	// labels, even though they are hidden from the card list dots.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Backlog", Cards: []provider.Card{
				{Number: 1, Title: "Test card", Labels: []string{"Working", "Backlog", "bug"}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	view := board.View()

	// All three labels should appear as text in the detail panel (right side).
	for _, label := range []string{"Working", "Backlog", "bug"} {
		if !strings.Contains(view, label) {
			t.Errorf("detail panel should contain label %q, but it was not found in view", label)
		}
	}
}
