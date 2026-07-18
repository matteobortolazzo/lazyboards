package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
	"github.com/muesli/termenv"
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
	statusBarView := b.statusBar.View(200, 0, 0)
	for _, absent := range []string{"Column", "Quit", "Config", "Refresh"} {
		if strings.Contains(statusBarView, absent) {
			t.Errorf("statusBar.View(, 0, 0) should NOT contain %q, but it does", absent)
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

func TestView_LeavesBottomRowFreeForOuterBorderVisibility(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	view := b.View()
	lines := strings.Split(view, "\n")

	if got, want := len(lines), b.Height-1; got != want {
		t.Fatalf("View() rendered %d lines, want %d so the terminal bottom row stays free", got, want)
	}
	lastLine := lines[len(lines)-1]
	if lipgloss.Width(lastLine) != b.Width {
		t.Fatalf("bottom border width = %d, want %d", lipgloss.Width(lastLine), b.Width)
	}
	if !strings.Contains(lastLine, "\u2570") || !strings.Contains(lastLine, "\u256f") {
		t.Fatalf("last rendered line should be the outer bottom border, got %q", lastLine)
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
		{Number: 1, Title: longTitle, Labels: []provider.Label{{Name: "test"}}},
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
		{Number: 1, Title: longTitle, Labels: []provider.Label{{Name: "test"}}},
		{Number: 2, Title: "Short", Labels: []provider.Label{{Name: "test"}}},
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

// TestView_CardList_BothPRAndWorkingIndicators verifies a card with both a
// linked PR and the "Working" label renders both indicators, but on
// separate lines (#439): the Working spinner stays inline on the title
// line, while the linked PR now renders as its own status line beneath the
// title (prStatusPrefix(status) + "#N") instead of sharing the title line
// with an inline glyph.
func TestView_CardList_BothPRAndWorkingIndicators(t *testing.T) {
	b := newBoardWithWorkingLabel(t)
	view := b.View()

	// Card 4 has both a linked PR (#20, zero-value Mergeable/MergeStateStatus
	// -> "unknown" status) and the "Working" label.
	if !strings.Contains(view, "\uf110") {
		t.Error("View() should contain Working indicator \uf110 for card with 'Working' label")
	}
	wantPRLine := prStatusPrefix("unknown") + "#20"
	if !strings.Contains(view, wantPRLine) {
		t.Errorf("View() should contain PR status line %q for card's linked PR; got:\n%s", wantPRLine, view)
	}

	// The Working indicator's title line and the PR status line must be
	// separate lines, not merged together the way the old inline glyph was.
	lines := strings.Split(view, "\n")
	for _, line := range lines {
		if strings.Contains(line, "\uf110") && strings.Contains(line, wantPRLine) {
			t.Errorf("Working indicator and PR status line should render on separate lines, but found together in %q", line)
		}
	}
}

func TestView_CardList_WorkingIndicator_CaseInsensitive(t *testing.T) {
	// The default working label "Working" should match case-insensitively.
	// A card with "working" (lowercase) SHOULD trigger the Working indicator.
	b := newBoardWithCustomCard(t, "Lowercase working", []provider.Label{{Name: "working"}}, "")
	view := b.View()

	if !strings.Contains(view, "\uf110") {
		t.Error("View() should contain Working indicator \uf110 for card with lowercase 'working' label (case-insensitive match)")
	}
}

// --- Configurable Working Label Tests (#113) ---

func TestView_CardList_CustomWorkingLabel_ShowsSpinner(t *testing.T) {
	// When workingLabel is set to "In Progress", cards with that label show
	// the spinner icon, and cards with the default "Working" label do NOT.
	cards := []provider.Card{
		{Number: 1, Title: "Active task", Labels: []provider.Label{{Name: "In Progress"}}},
		{Number: 2, Title: "Old style", Labels: []provider.Label{{Name: "Working"}}},
		{Number: 3, Title: "Baseline", Labels: []provider.Label{{Name: "bug"}}},
	}
	b := newBoardWithCustomWorkingLabel(t, "In Progress", cards)
	view := b.View()

	// Card 1 ("Active task" with "In Progress") should show spinner.
	lines := strings.Split(view, "\n")
	card1HasSpinner := false
	card2HasSpinner := false
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "Active task") {
			if strings.Contains(line, "\uf110") {
				card1HasSpinner = true
			}
		}
		if strings.Contains(line, "#2") && strings.Contains(line, "Old style") {
			if strings.Contains(line, "\uf110") {
				card2HasSpinner = true
			}
		}
	}

	if !card1HasSpinner {
		t.Error("card with 'In Progress' label should show spinner when workingLabel='In Progress'")
	}
	if card2HasSpinner {
		t.Error("card with 'Working' label should NOT show spinner when workingLabel='In Progress'")
	}
}

func TestView_CardList_CustomWorkingLabel_HidesDot(t *testing.T) {
	// When workingLabel is set to "In Progress", the "In Progress" label
	// should be hidden from the colored dot display (same as "Working" is today).
	cards := []provider.Card{
		{Number: 1, Title: "Active task", Labels: []provider.Label{{Name: "In Progress"}, {Name: "bug"}}},
	}
	b := newBoardWithCustomWorkingLabel(t, "In Progress", cards)
	view := b.View()

	// Only 1 dot should appear (for "bug"), not 2.
	lines := strings.Split(view, "\n")
	dotCount := 0
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "Active task") {
			dotCount += strings.Count(line, "\u25cf")
		}
	}
	if dotCount != 1 {
		t.Errorf("expected 1 label dot (for 'bug' only, 'In Progress' hidden as workingLabel), got %d", dotCount)
	}
}

func TestView_CardList_DisabledWorkingLabel_NoSpinner(t *testing.T) {
	// When workingLabel is explicitly set to "" (empty string), the working
	// indicator feature is disabled entirely. No spinner icon should appear,
	// even for cards with the default "Working" label.
	cards := []provider.Card{
		{Number: 1, Title: "Has working label", Labels: []provider.Label{{Name: "Working"}}},
		{Number: 2, Title: "Normal card", Labels: []provider.Label{{Name: "bug"}}},
	}
	b := newBoardWithCustomWorkingLabel(t, "", cards)
	view := b.View()

	if strings.Contains(view, "\uf110") {
		t.Error("View() should NOT contain spinner icon when workingLabel is empty (feature disabled)")
	}
}

func TestView_CardList_DisabledWorkingLabel_ShowsDot(t *testing.T) {
	// When workingLabel is empty (disabled), the "Working" label should NOT be
	// hidden from dots -- it should render as a normal label dot.
	cards := []provider.Card{
		{Number: 1, Title: "Has labels", Labels: []provider.Label{{Name: "Working"}, {Name: "bug"}}},
	}
	b := newBoardWithCustomWorkingLabel(t, "", cards)
	view := b.View()

	// Both "Working" and "bug" should produce dots (2 total) since Working
	// is no longer a special label when the feature is disabled.
	lines := strings.Split(view, "\n")
	dotCount := 0
	for _, line := range lines {
		if strings.Contains(line, "#1") && strings.Contains(line, "Has labels") {
			dotCount += strings.Count(line, "\u25cf")
		}
	}
	if dotCount != 2 {
		t.Errorf("expected 2 label dots ('Working' and 'bug') when workingLabel disabled, got %d", dotCount)
	}
}

func TestView_CardList_CustomWorkingLabel_CaseInsensitive(t *testing.T) {
	// Working label matching should be case-insensitive. A workingLabel of
	// "ACTIVE" should match a card label "active" (lowercase).
	cards := []provider.Card{
		{Number: 1, Title: "Lowercase active", Labels: []provider.Label{{Name: "active"}}},
		{Number: 2, Title: "Uppercase active", Labels: []provider.Label{{Name: "ACTIVE"}}},
		{Number: 3, Title: "Mixed case", Labels: []provider.Label{{Name: "Active"}}},
	}
	b := newBoardWithCustomWorkingLabel(t, "ACTIVE", cards)
	view := b.View()

	// All three cards should show the spinner icon (case-insensitive match).
	lines := strings.Split(view, "\n")
	spinnerCount := 0
	for _, line := range lines {
		if strings.Contains(line, "\uf110") {
			spinnerCount++
		}
	}
	if spinnerCount < 3 {
		t.Errorf("expected spinner on all 3 cards (case-insensitive match with workingLabel='ACTIVE'), got %d spinners", spinnerCount)
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
	b := newBoardWithCustomCard(t, "Fix crash", []provider.Label{{Name: "Working"}, {Name: "bug"}}, "")
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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Backlog", Cards: []provider.Card{
				{Number: 1, Title: "Add feature", Labels: []provider.Label{{Name: "Backlog"}, {Name: "enhancement"}}},
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
	b := newBoardWithCustomCard(t, "Some task", []provider.Label{{Name: "working"}, {Name: "bug"}}, "")
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
	b := newBoardWithCustomCard(t, "Solo working", []provider.Label{{Name: "Working"}}, "")
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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "To Do", Cards: []provider.Card{
				{Number: 1, Title: "Important task", Labels: []provider.Label{{Name: "Working"}, {Name: "To Do"}, {Name: "bug"}, {Name: "urgent"}}},
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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Backlog", Cards: []provider.Card{
				{Number: 1, Title: "Test card", Labels: []provider.Label{{Name: "Working"}, {Name: "Backlog"}, {Name: "bug"}}},
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

func TestView_DetailPanel_RendersWithGitHubLabelColors(t *testing.T) {
	// A card with labels should render them in YAML frontmatter format.
	// Per-label lipgloss colors are no longer used in the detail panel;
	// labels appear in the YAML code block rendered through glamour.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Backlog", Cards: []provider.Card{
				{Number: 1, Title: "Colored labels", Labels: []provider.Label{
					{Name: "bug", Color: "d73a4a"},
					{Name: "enhancement", Color: "a2eeef"},
					{Name: "no-color-label"},
				}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	view := board.View()

	// The detail panel should use YAML frontmatter format with "labels:" field.
	if !strings.Contains(view, "labels:") {
		t.Error("detail panel should contain 'labels:' in YAML frontmatter format")
	}

	// All three label names should appear in the detail panel (inside the YAML block).
	for _, labelName := range []string{"bug", "enhancement", "no-color-label"} {
		if !strings.Contains(view, labelName) {
			t.Errorf("detail panel should contain label %q in YAML frontmatter, but it was not found in view", labelName)
		}
	}

	// The view should render without being empty (no panics from rendering).
	if strings.TrimSpace(view) == "" {
		t.Error("View() should not be empty when rendering cards with labels")
	}
}

// --- Help Modal Key Binding Tests ---

func TestHelpModal_ShowsTicketOpenBinding(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Enter help mode.
	b = sendKey(t, b, keyMsg("?"))

	view := b.View()
	// The help modal should show "o" mapped to "Open ticket" in the Normal Mode section.
	if !strings.Contains(view, "Open ticket") {
		t.Errorf("help modal should contain %q key binding, got:\n%s", "Open ticket", view)
	}
}

func TestHelpModal_ShowsFilterToggle(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Enter help mode.
	b = sendKey(t, b, keyMsg("?"))

	view := b.View()
	// The help modal should show "f" mapped to "Filter (toggle)" in the Normal Mode section.
	if !strings.Contains(view, "Filter (toggle)") {
		t.Errorf("help modal should contain %q key binding, got:\n%s", "Filter (toggle)", view)
	}
}

// --- Card list PR status lines (#439) ---
//
// PR status moved off the title line's single collapsed glyph onto its own
// dedicated line per linked PR (prStatusPrefix(status) + "#N"), so a card
// with multiple linked PRs shows each one's own status instead of only the
// worst of them (worstPRStatus's old inline-glyph collapse).

// TestViewCardList_PRStatusLine_ConflictingStyled asserts a card with a
// single conflicting linked PR renders a dedicated status line carrying the
// prConflictingStyle-rendered glyph and the PR's number.
func TestViewCardList_PRStatusLine_ConflictingStyled(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Has conflicting PR", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: PR", URL: "https://github.com/o/r/pull/10", Mergeable: "CONFLICTING", MergeStateStatus: "DIRTY"},
		}},
	}, 120, 40)

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	want := prStatusPrefix("conflicting") + "#10"
	if !strings.Contains(out, want) {
		t.Errorf("rendered card list missing conflicting PR status line %q; got:\n%s", want, out)
	}
}

// TestViewCardList_PRStatusLine_MultiplePRs_EachShowsOwnStatus asserts a card
// with a draft PR and a blocked PR renders BOTH as their own status lines --
// per-PR status (prStatus applied individually), not the single
// worst-of-all-linked-PRs status the old inline glyph collapsed to
// (worstPRStatus). This is the ticket's core "stacked PRs" scenario: neither
// status may be dropped in favor of the other.
//
// The comparison forces an ANSI256 color profile for the duration of the
// test: prDraftStyle/prBlockedStyle are plain lipgloss styles (no dedicated
// renderer, matching the agent-badge convention), so lipgloss's default
// global renderer would otherwise auto-detect "no color support" in a
// non-TTY `go test` run and render every style as identical plain text,
// making draft- and blocked-styled output indistinguishable regardless of
// which one the code actually picked.
func TestViewCardList_PRStatusLine_MultiplePRs_EachShowsOwnStatus(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Two PRs, one blocked one draft", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "draft PR", URL: "https://github.com/o/r/pull/10", IsDraft: true, Mergeable: "MERGEABLE", MergeStateStatus: "DRAFT"},
			{Number: 11, Title: "blocked PR", URL: "https://github.com/o/r/pull/11", Mergeable: "MERGEABLE", MergeStateStatus: "BLOCKED"},
		}},
	}, 120, 40)

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	wantDraft := prStatusPrefix("draft") + "#10"
	wantBlocked := prStatusPrefix("blocked") + "#11"
	if !strings.Contains(out, wantDraft) {
		t.Errorf("rendered card list missing draft PR status line %q (per-PR status must not be dropped in favor of the worse one); got:\n%s", wantDraft, out)
	}
	if !strings.Contains(out, wantBlocked) {
		t.Errorf("rendered card list missing blocked PR status line %q; got:\n%s", wantBlocked, out)
	}
}

// TestViewCardList_PRStatusLine_UnknownKeepsNeutralColor is the Q1
// regression test carried over from the inline-glyph era: a linked PR's
// status line keeps the neutral prIndicatorStyle fallback color
// (prStatusStyle's default case) when its status is UNKNOWN. This is a
// deliberate divergence from the PR list/picker modals (which render no
// glyph at all on UNKNOWN, see pr_list_test.go /
// TestPRList_View_UnknownStatusRendersNoGlyph) -- the board's per-PR line
// must keep signaling "this is a linked PR" even when its status can't yet
// be determined.
func TestViewCardList_PRStatusLine_UnknownKeepsNeutralColor(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Has PR with unresolved mergeability", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: PR", URL: "https://github.com/o/r/pull/10", Mergeable: "UNKNOWN", MergeStateStatus: "UNKNOWN"},
		}},
	}, 120, 40)

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	want := prStatusPrefix("unknown") + "#10"
	if !strings.Contains(out, want) {
		t.Errorf("rendered card list should keep the neutral prIndicatorStyle status line %q; got:\n%s", want, out)
	}
}

// --- cardStatusLines (#439) ---
//
// Agent and PR status render as dedicated lines under the card title instead
// of a single collapsed inline glyph. Agent lines are derived from
// cardAgentWindows (every session-scoped window joined to the card, not just
// the single "best" one the now-removed agentBadgeFor/agentStatusFor picked),
// and PR lines are derived per-PR from prStatus (not the now-removed
// worstPRStatus's single collapsed status). Agent lines render before PR
// lines (Q3); idle/badge-less agent windows are skipped entirely, with no
// vertical cost (Q2).

// TestCardStatusLines_AgentOnly_SkipsIdleRendersRunning verifies a single
// non-idle agent window renders as one status line carrying its styled
// badge, and that a card with only an agent window (no linked PRs) yields
// exactly one line.
func TestCardStatusLines_AgentOnly_SkipsIdleRendersRunning(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 7, Title: "Fix flaky test"},
	}, 120, 40)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "7", Status: "running", Agent: "claude"},
		},
	}
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth)
	if len(lines) != 1 {
		t.Fatalf("cardStatusLines() = %d lines, want 1 (one running agent window, no linked PRs); got %v", len(lines), lines)
	}
	wantBadge := agentBadgeStyle("running").Render(agentBadgeText("running", "claude"))
	if !strings.Contains(lines[0], wantBadge) {
		t.Errorf("agent status line %q missing styled badge %q", lines[0], wantBadge)
	}
}

// TestCardStatusLines_IdleWindow_SkippedEntirely verifies an idle-only
// window produces no status line and no vertical cost (Q2).
func TestCardStatusLines_IdleWindow_SkippedEntirely(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 7, Title: "Idle card"},
	}, 120, 40)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "7", Status: "idle", Agent: "claude"},
		},
	}
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth)
	if len(lines) != 0 {
		t.Errorf("cardStatusLines() = %v, want no lines for an idle-only agent window", lines)
	}
}

// TestCardStatusLines_MultipleAgentWindows_OneLineEach verifies a card
// joined to more than one live agent window (the rare case agentBadgeFor's
// single "best window" pick used to silently lose) renders one status line
// per non-idle window, in snapshot order.
func TestCardStatusLines_MultipleAgentWindows_OneLineEach(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 7, Title: "Two windows"},
	}, 120, 40)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "7-implement", Status: "running", Agent: "claude"},
			{WindowName: "7-review", Status: "failed", Agent: "codex"},
		},
	}
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth)
	if len(lines) != 2 {
		t.Fatalf("cardStatusLines() = %d lines, want 2 (one per non-idle window); got %v", len(lines), lines)
	}
	wantRunning := agentBadgeStyle("running").Render(agentBadgeText("running", "claude"))
	wantFailed := agentBadgeStyle("failed").Render(agentBadgeText("failed", "codex"))
	if !strings.Contains(lines[0], wantRunning) {
		t.Errorf("first agent line %q missing running badge %q (snapshot order)", lines[0], wantRunning)
	}
	if !strings.Contains(lines[1], wantFailed) {
		t.Errorf("second agent line %q missing failed badge %q (snapshot order)", lines[1], wantFailed)
	}
}

// TestCardStatusLines_PROnly_ShowsNumberAndStatus verifies a single linked
// PR (no agent windows) renders as one status line: prStatusPrefix(status) +
// "#N" (Q1: number + status only, no title).
func TestCardStatusLines_PROnly_ShowsNumberAndStatus(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Has PR", LinkedPRs: []provider.LinkedPR{
			{Number: 11, Title: "feat: PR", URL: "https://github.com/o/r/pull/11", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
		}},
	}, 120, 40)
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth)
	if len(lines) != 1 {
		t.Fatalf("cardStatusLines() = %d lines, want 1 (one linked PR, no agent windows); got %v", len(lines), lines)
	}
	want := prStatusPrefix("mergeable") + "#11"
	if !strings.Contains(lines[0], want) {
		t.Errorf("PR status line %q missing expected content %q", lines[0], want)
	}
}

// TestCardStatusLines_MultiplePRs_EachShowsOwnStatus_NotWorst is the direct
// unit-level counterpart of TestViewCardList_PRStatusLine_MultiplePRs_EachShowsOwnStatus:
// a card with a draft PR and a blocked PR must yield two lines, each with
// its own PR's status -- never collapsed to the single worst status.
func TestCardStatusLines_MultiplePRs_EachShowsOwnStatus_NotWorst(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Stacked PRs", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "draft PR", URL: "https://github.com/o/r/pull/10", IsDraft: true, Mergeable: "MERGEABLE", MergeStateStatus: "DRAFT"},
			{Number: 11, Title: "blocked PR", URL: "https://github.com/o/r/pull/11", Mergeable: "MERGEABLE", MergeStateStatus: "BLOCKED"},
		}},
	}, 120, 40)
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth)
	if len(lines) != 2 {
		t.Fatalf("cardStatusLines() = %d lines, want 2 (one per linked PR); got %v", len(lines), lines)
	}
	wantDraft := prStatusPrefix("draft") + "#10"
	wantBlocked := prStatusPrefix("blocked") + "#11"
	if !strings.Contains(lines[0], wantDraft) {
		t.Errorf("first PR line %q missing draft content %q (per-PR status, not worst-of-all)", lines[0], wantDraft)
	}
	if !strings.Contains(lines[1], wantBlocked) {
		t.Errorf("second PR line %q missing blocked content %q (per-PR status, not worst-of-all)", lines[1], wantBlocked)
	}
}

// TestCardStatusLines_AgentThenPR_AgentLinesBeforePRLines verifies the Q3
// ordering: when a card has both agent windows and linked PRs, every agent
// line precedes every PR line.
func TestCardStatusLines_AgentThenPR_AgentLinesBeforePRLines(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 7, Title: "Agent and PR", LinkedPRs: []provider.LinkedPR{
			{Number: 11, Title: "feat: PR", URL: "https://github.com/o/r/pull/11", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
		}},
	}, 120, 40)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "7", Status: "running", Agent: "claude"},
		},
	}
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth)
	if len(lines) != 2 {
		t.Fatalf("cardStatusLines() = %d lines, want 2 (1 agent + 1 PR); got %v", len(lines), lines)
	}
	wantAgent := agentBadgeStyle("running").Render(agentBadgeText("running", "claude"))
	wantPR := prStatusPrefix("mergeable") + "#11"
	if !strings.Contains(lines[0], wantAgent) {
		t.Errorf("first line %q should be the agent line %q (agent lines precede PR lines)", lines[0], wantAgent)
	}
	if !strings.Contains(lines[1], wantPR) {
		t.Errorf("second line %q should be the PR line %q (agent lines precede PR lines)", lines[1], wantPR)
	}
}

// TestCardStatusLines_NoAgentNoPR_ReturnsNoLines verifies the common-case
// card (no agent windows, no linked PRs) costs zero status lines.
func TestCardStatusLines_NoAgentNoPR_ReturnsNoLines(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Plain card"},
	}, 120, 40)
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth)
	if len(lines) != 0 {
		t.Errorf("cardStatusLines() = %v, want no lines for a card with no agent windows and no linked PRs", lines)
	}
}

// TestCardStatusLines_Indentation_MatchesGivenIndentWidth verifies every
// status line is prefixed with exactly indentWidth spaces -- the same
// continuation indent wrapTitle uses for a wrapped title's "#N " prefix, so
// status lines visually align under the title text rather than the "#N "
// number prefix.
func TestCardStatusLines_Indentation_MatchesGivenIndentWidth(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 42, Title: "Double-digit number", LinkedPRs: []provider.LinkedPR{
			{Number: 11, Title: "feat: PR", URL: "https://github.com/o/r/pull/11", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
		}},
	}, 120, 40)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "42", Status: "running", Agent: "claude"},
		},
	}
	card := b.Columns[0].Cards[0]
	// "#42 " is 4 runes -- wrapTitle's continuation indent for this card.
	indentWidth := cardTitlePrefixWidth(card)
	if indentWidth != 4 {
		t.Fatalf("test setup: indentWidth = %d, want 4 for card #42", indentWidth)
	}

	lines := b.cardStatusLines(card, indentWidth)
	if len(lines) != 2 {
		t.Fatalf("cardStatusLines() = %d lines, want 2; got %v", len(lines), lines)
	}
	wantIndent := strings.Repeat(" ", indentWidth)
	for i, line := range lines {
		if !strings.HasPrefix(line, wantIndent) {
			t.Errorf("status line %d %q does not start with the %d-space indent matching wrapTitle's continuation indent", i, line, indentWidth)
		}
	}
}

// --- cardLineCount (Board method, #439) ---
//
// cardLineCount becomes a Board method so it can call b.cardStatusLines
// directly; its returned count must include the card's status lines, not
// just its (possibly wrapped) title lines, since that count is the single
// source of truth clampScrollOffset, viewCardList, and handleCardClick all
// share (docs/list-cursor-invariants.md).

// TestCardLineCount_IncludesAgentAndPRStatusLines verifies the count is
// title lines + agent status lines + PR status lines, not just the title.
func TestCardLineCount_IncludesAgentAndPRStatusLines(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Card with agent and PR", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "feat: PR", URL: "https://github.com/o/r/pull/10", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
		}},
	}, 120, 40)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "1", Status: "running", Agent: "claude"},
		},
	}
	card := b.Columns[0].Cards[0]
	columnNames := []string{"Column A"}

	// A wide contentWidth keeps the title on a single line, isolating the
	// count to title(1) + agent(1) + PR(1).
	got := b.cardLineCount(card, 80, columnNames)
	want := 3
	if got != want {
		t.Errorf("cardLineCount() = %d, want %d (1 title line + 1 agent status line + 1 PR status line)", got, want)
	}
}

// TestCardLineCount_NoAgentNoPR_IsJustTitleLines verifies the common-case
// card (no agent windows, no linked PRs) still costs only its title line(s).
func TestCardLineCount_NoAgentNoPR_IsJustTitleLines(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "Plain card"},
	}, 120, 40)
	card := b.Columns[0].Cards[0]
	columnNames := []string{"Column A"}

	got := b.cardLineCount(card, 80, columnNames)
	if got != 1 {
		t.Errorf("cardLineCount() = %d, want 1 for a card with no agent windows and no linked PRs", got)
	}
}

// --- Scroll / click integration with variable card heights (#439) ---
//
// Single source of truth (docs/list-cursor-invariants.md): cardLineCount is
// shared by clampScrollOffset, viewCardList's visible-window loop, and
// handleCardClick. A card whose height is inflated by multiple PR status
// lines must be counted identically everywhere, or scroll math and click
// hit-detection silently disagree with what's rendered.

// TestHandleCardClick_TallMultiPRCard_ClickTargetsAccountForStatusLines
// verifies handleCardClick's line-count math correctly accounts for a card
// whose height is inflated by multiple PR status lines, not just its title
// line -- a click landing on any of that card's status lines selects it, and
// a click on the row just past its last status line selects the next card.
func TestHandleCardClick_TallMultiPRCard_ClickTargetsAccountForStatusLines(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", true, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "First"},
				{Number: 2, Title: "Second", LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "first PR", URL: "https://github.com/o/r/pull/10", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
					{Number: 11, Title: "second PR", URL: "https://github.com/o/r/pull/11", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
					{Number: 12, Title: "third PR", URL: "https://github.com/o/r/pull/12", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
				}},
				{Number: 3, Title: "Third"},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = 120
	board.Height = 40

	// Row layout (ScrollOffset=0, no up-arrow indicator):
	// Y=2:    card 0 ("First", 1 line)
	// Y=3..6: card 1 ("Second", 1 title line + 3 PR status lines = 4 lines)
	// Y=7:    card 2 ("Third", 1 line)

	// A click on one of card 1's PR status lines (Y=5) must select card 1,
	// not miscount past it.
	clicked := sendKey(t, board, tea.MouseMsg{
		X: leftPanelX(), Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	if got := clicked.Columns[0].Cursor; got != 1 {
		t.Errorf("click on card 1's PR status line (Y=5): cursor = %d, want 1", got)
	}

	// A click on the row just past card 1's status lines must select card 2,
	// confirming the tall card's full height (not just its title line) was
	// counted.
	clicked = sendKey(t, board, tea.MouseMsg{
		X: leftPanelX(), Y: 7, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress,
	})
	if got := clicked.Columns[0].Cursor; got != 2 {
		t.Errorf("click below card 1's status lines (Y=7): cursor = %d, want 2 (card 3)", got)
	}
}

// TestScroll_TallMultiPRCard_ScrollOffsetAccountsForStatusLines verifies
// clampScrollOffset's per-card cardLineCount summation counts a multi-PR
// card's full height (title + PR status lines), not a flat "1 line per
// card" assumption -- forcing scroll math that a flat-height bug would get
// wrong.
func TestScroll_TallMultiPRCard_ScrollOffsetAccountsForStatusLines(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

	cards := []provider.Card{
		{Number: 1, Title: "First"},
		{Number: 2, Title: "Second"},
		{Number: 3, Title: "Stacked PRs card", LinkedPRs: []provider.LinkedPR{
			{Number: 10, Title: "first", URL: "https://github.com/o/r/pull/10", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
			{Number: 11, Title: "second", URL: "https://github.com/o/r/pull/11", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
			{Number: 12, Title: "third", URL: "https://github.com/o/r/pull/12", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
		}},
		{Number: 4, Title: "Fourth"},
		{Number: 5, Title: "Fifth"},
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{Title: "Column A", Cards: cards}},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = 120
	// panelHeight = Height - 6. Total lines = 1+1+4+1+1 = 8. Height=13 gives
	// panelHeight=7, less than the 8 total lines -- forces scrolling only if
	// the tall card's 3 extra PR lines are actually counted.
	board.Height = 13

	// Navigate to the last card.
	for i := 0; i < len(cards)-1; i++ {
		board = sendKey(t, board, keyMsg("j"))
	}

	col := board.Columns[board.ActiveTab]
	if col.Cursor != len(cards)-1 {
		t.Fatalf("Cursor = %d, want %d (last card)", col.Cursor, len(cards)-1)
	}
	if col.ScrollOffset <= 0 {
		t.Errorf("ScrollOffset = %d after scrolling past the tall multi-PR card, want > 0 (its 3 PR status lines must count toward scroll math)", col.ScrollOffset)
	}
}
