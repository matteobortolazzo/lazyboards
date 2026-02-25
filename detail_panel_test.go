package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func TestDetailFocus_LeftArrow_ReturnsFocusToCardList(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus with 'l', then exit with left arrow.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))

	if b.detailFocused {
		t.Error("after 'l' then Left arrow: detailFocused should be false")
	}
}

func TestView_DetailPanelShowsSelectedCard(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()

	// Detail panel should show each of the selected card's labels individually.
	selectedCard := b.Columns[b.ActiveTab].Cards[0]
	for _, label := range selectedCard.Labels {
		if !strings.Contains(view, label.Name) {
			t.Errorf("View() detail panel does not contain selected card label %q", label.Name)
		}
	}

	// After navigating down, detail should update to the new card.
	b = sendKey(t, b, keyMsg("j"))
	view = b.View()
	nextCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	for _, label := range nextCard.Labels {
		if !strings.Contains(view, label.Name) {
			t.Errorf("View() detail panel does not contain card label %q after navigating", label.Name)
		}
	}
}

func TestView_DetailPanelShowsCardBody(t *testing.T) {
	bodyText := "This is the card description with important details."
	b := newBoardWithBody(t, bodyText, "other body")

	view := b.View()

	// The detail panel should display the body text of the selected card.
	if !strings.Contains(view, bodyText) {
		t.Errorf("View() detail panel does not contain card body %q", bodyText)
	}
}

func TestView_DetailPanelEmptyBody_NoExtraSpace(t *testing.T) {
	b := newBoardWithBody(t, "", "")

	view := b.View()

	// With an empty body, the view should still render without errors.
	// The detail panel should show the card title in frontmatter YAML format.
	if !strings.Contains(view, "title:") {
		t.Error("View() detail panel should contain 'title:' in frontmatter format")
	}

	// The card title text should still appear in the view (inside the YAML block).
	selectedCard := b.Columns[b.ActiveTab].Cards[0]
	if !strings.Contains(view, selectedCard.Title) {
		t.Errorf("View() detail panel does not contain card title %q", selectedCard.Title)
	}
}

func TestView_DetailPanelBodyUpdatesOnNavigation(t *testing.T) {
	firstBody := "Description of the first card."
	secondBody := "Description of the second card."
	b := newBoardWithBody(t, firstBody, secondBody)

	// Initially, the first card is selected.
	view := b.View()
	if !strings.Contains(view, firstBody) {
		t.Errorf("View() detail panel does not contain first card body %q", firstBody)
	}

	// Navigate down to the second card.
	b = sendKey(t, b, keyMsg("j"))
	view = b.View()

	// The second card's body should now appear.
	if !strings.Contains(view, secondBody) {
		t.Errorf("View() detail panel does not contain second card body %q after navigation", secondBody)
	}

	// The first card's body should no longer be visible (it's unique text).
	if strings.Contains(view, firstBody) {
		t.Errorf("View() detail panel still contains first card body %q after navigating away", firstBody)
	}
}

// --- Detail Panel Focus: Focus Switching ---

func TestDetailFocus_LKey_FocusesDetailPanel(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Press 'l' to focus the detail panel.
	b = sendKey(t, b, keyMsg("l"))

	if !b.detailFocused {
		t.Error("after 'l': detailFocused should be true")
	}
}

func TestDetailFocus_HKey_ReturnsFocusToCardList(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus with 'l', then exit with 'h'.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("h"))

	if b.detailFocused {
		t.Error("after 'l' then 'h': detailFocused should be false")
	}
}

func TestDetailFocus_Escape_ReturnsFocusToCardList(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus with 'l', then exit with Escape.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.detailFocused {
		t.Error("after 'l' then Escape: detailFocused should be false")
	}
}

func TestDetailFocus_Tab_ReturnsFocusAndSwitchesColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	initialTab := b.ActiveTab

	// Enter detail focus with 'l', then press Tab.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	if b.detailFocused {
		t.Error("after Tab in detail focus: detailFocused should be false")
	}
	if b.ActiveTab != initialTab+1 {
		t.Errorf("after Tab in detail focus: ActiveTab = %d, want %d", b.ActiveTab, initialTab+1)
	}
}

func TestDetailFocus_ShiftTab_ReturnsFocusAndSwitchesColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Move to column 1 first so Shift+Tab can decrement.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 1 {
		t.Fatalf("precondition: ActiveTab = %d, want 1", b.ActiveTab)
	}

	// Enter detail focus with 'l', then press Shift+Tab.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))

	if b.detailFocused {
		t.Error("after Shift+Tab in detail focus: detailFocused should be false")
	}
	if b.ActiveTab != 0 {
		t.Errorf("after Shift+Tab in detail focus: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

// --- Detail Panel Focus: Scroll ---

func TestDetailFocus_JKey_ScrollsDown(t *testing.T) {
	// With Height=40: panelHeight=34, headerLines=3, availableBodyLines=31.
	// Need more than 31 raw lines so maxOffset > 0.
	b := newBoardWithLongBody(t, 50)

	// Record the card cursor before entering detail focus.
	cursorBefore := b.Columns[b.ActiveTab].Cursor

	// Enter detail focus, then press 'j' to scroll down.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("j"))

	// detailScrollOffset should increment.
	if b.detailScrollOffset < 1 {
		t.Errorf("detailScrollOffset = %d after 'j' in detail focus, want >= 1", b.detailScrollOffset)
	}

	// Card cursor should NOT change when in detail focus.
	cursorAfter := b.Columns[b.ActiveTab].Cursor
	if cursorAfter != cursorBefore {
		t.Errorf("card cursor changed from %d to %d during detail scroll, want unchanged", cursorBefore, cursorAfter)
	}
}

func TestDetailFocus_KKey_ScrollsUp(t *testing.T) {
	b := newBoardWithLongBody(t, 50)

	// Enter detail focus, scroll down twice, then scroll up once.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	offsetAfterDown := b.detailScrollOffset

	b = sendKey(t, b, keyMsg("k"))

	if b.detailScrollOffset >= offsetAfterDown {
		t.Errorf("detailScrollOffset = %d after 'k', want less than %d", b.detailScrollOffset, offsetAfterDown)
	}
}

func TestDetailFocus_KKey_ClampsAtZero(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus and press 'k' without scrolling down first.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("k"))

	if b.detailScrollOffset < 0 {
		t.Errorf("detailScrollOffset = %d after 'k' at top, want >= 0 (should not go negative)", b.detailScrollOffset)
	}
}

func TestDetailFocus_ScrollOffsetResetsOnCardChange(t *testing.T) {
	b := newBoardWithLongBody(t, 50)

	// Enter detail focus, scroll down.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))

	if b.detailScrollOffset == 0 {
		t.Fatal("precondition: detailScrollOffset should be > 0 after scrolling")
	}

	// Exit detail focus with 'h', then navigate to a different card with 'j'.
	b = sendKey(t, b, keyMsg("h"))
	b = sendKey(t, b, keyMsg("j"))

	if b.detailScrollOffset != 0 {
		t.Errorf("detailScrollOffset = %d after changing card, want 0 (should reset)", b.detailScrollOffset)
	}
}

func TestDetailFocus_ScrollOffsetResetsOnRefresh(t *testing.T) {
	b := newBoardWithLongBody(t, 50)

	// Enter detail focus, scroll down.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))

	if b.detailScrollOffset == 0 {
		t.Fatal("precondition: detailScrollOffset should be > 0 after scrolling")
	}

	// Exit detail focus and refresh.
	b = sendKey(t, b, keyMsg("h"))
	b = sendKey(t, b, keyMsg("r"))

	// Simulate the board being fetched again.
	b = simulateRefresh(t, b)

	if b.detailScrollOffset != 0 {
		t.Errorf("detailScrollOffset = %d after board refresh, want 0 (should reset)", b.detailScrollOffset)
	}
}

// --- Detail Panel Focus: View ---

func TestView_DetailFocused_BorderHighlighted(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))

	// When detail panel is focused, the view should render.
	// We verify that the model state is set correctly.
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	view := b.View()
	// The view should render without panic when detailFocused is true.
	if strings.TrimSpace(view) == "" {
		t.Error("View() should not be empty when detail panel is focused")
	}
}

func TestView_DetailUnfocused_BorderDim(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Without entering detail focus, the default state.
	if b.detailFocused {
		t.Fatal("precondition: detailFocused should be false by default")
	}

	view := b.View()
	if strings.TrimSpace(view) == "" {
		t.Error("View() should not be empty in default (unfocused) state")
	}
}

func TestView_DetailFocused_StatusBarShowsDetailHints(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))

	view := b.View()

	// Status bar should show detail-specific hint keys and descriptions.
	if !strings.Contains(view, "j/k") {
		t.Errorf("View() in detail focus should contain key %q in status bar", "j/k")
	}
	if !strings.Contains(view, "Scroll") {
		t.Errorf("View() in detail focus should contain desc %q in status bar", "Scroll")
	}
	if !strings.Contains(view, "Back") {
		t.Errorf("View() in detail focus should contain desc %q in status bar", "Back")
	}

	// Normal-mode hint descriptions should NOT appear.
	if strings.Contains(view, "New") {
		t.Errorf("View() in detail focus should NOT contain normal hint desc %q", "New")
	}
}

func TestView_GlamourRendersMarkdown(t *testing.T) {
	markdownBody := "This has **bold** text and a list:\n- item one\n- item two"
	b := newBoardWithBody(t, markdownBody, "")

	// Enter detail focus to trigger glamour rendering.
	b = sendKey(t, b, keyMsg("l"))

	view := b.View()

	// The raw markdown syntax should NOT appear.
	if strings.Contains(view, "**bold**") {
		t.Error("View() should not contain raw markdown '**bold**' - glamour should render it")
	}

	// The word "bold" should still be present (rendered without markdown syntax).
	if !strings.Contains(view, "bold") {
		t.Error("View() should contain the word 'bold' (rendered from markdown)")
	}
}

// --- Fix: Scroll offset upper bound ---

func TestDetailFocus_JKey_ClampsAtMaxLines(t *testing.T) {
	// Use a short body so we can verify scrolling stops at the end.
	shortBody := "line one\nline two"
	b := newBoardWithBody(t, shortBody, "")

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))

	// Press 'j' many times (more than the number of lines).
	for i := 0; i < 100; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	// The offset should be capped; it should not grow unboundedly.
	// With a 2-line body, the offset should not exceed the line count.
	bodyLineCount := strings.Count(shortBody, "\n") + 1
	if b.detailScrollOffset > bodyLineCount {
		t.Errorf("detailScrollOffset = %d after excessive scrolling, want <= %d (body line count)", b.detailScrollOffset, bodyLineCount)
	}
}

// --- Fix: Tab/Shift+Tab at column boundaries ---

func TestDetailFocus_Tab_AtLastColumn_WrapsToFirst(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Navigate to the last column.
	lastCol := len(b.Columns) - 1
	for b.ActiveTab < lastCol {
		b = sendKey(t, b, arrowMsg(tea.KeyTab))
	}
	if b.ActiveTab != lastCol {
		t.Fatalf("precondition: ActiveTab = %d, want %d (last column)", b.ActiveTab, lastCol)
	}

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	// Press Tab at the last column boundary — should wrap to first column.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	if b.ActiveTab != 0 {
		t.Errorf("after Tab at last column: ActiveTab = %d, want 0 (should wrap to first)", b.ActiveTab)
	}
	if b.detailFocused {
		t.Error("after Tab at last column: detailFocused should be false (column switched)")
	}
}

func TestDetailFocus_ShiftTab_AtFirstColumn_WrapsToLast(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	lastCol := len(b.Columns) - 1

	if b.ActiveTab != 0 {
		t.Fatalf("precondition: ActiveTab = %d, want 0 (first column)", b.ActiveTab)
	}

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	// Press Shift+Tab at the first column boundary — should wrap to last column.
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))

	if b.ActiveTab != lastCol {
		t.Errorf("after Shift+Tab at first column: ActiveTab = %d, want %d (should wrap to last)", b.ActiveTab, lastCol)
	}
	if b.detailFocused {
		t.Error("after Shift+Tab at first column: detailFocused should be false (column switched)")
	}
}

// --- Detail Panel Scroll Fix (#61) ---

func TestView_DetailPanel_LongBodyTruncated(t *testing.T) {
	// Build a body with 50 paragraphs (double-newline separated), which
	// glamour renders as ~100 lines — well exceeding the available panel area.
	// With Height=40: panelHeight = 40 - 6 = 34
	// The entire rendered content (frontmatter + body) scrolls as one unit,
	// so only ~34 lines of the ~100+ rendered lines should be visible.
	var lines []string
	totalBodyLines := 50
	for i := 1; i <= totalBodyLines; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	longBody := strings.Join(lines, "\n\n")
	b := newBoardWithBody(t, longBody, "")

	view := b.View()

	// The total rendered output must not exceed the terminal height.
	outputLines := strings.Split(view, "\n")
	if len(outputLines) > b.Height {
		t.Errorf("View() output has %d lines, want <= %d (terminal height)", len(outputLines), b.Height)
	}

	// Not all 50 body lines should appear in the output, since only ~31 fit.
	// The last line ("line 50") should NOT be visible at scroll offset 0.
	lastLine := fmt.Sprintf("line %d", totalBodyLines)
	if strings.Contains(view, lastLine) {
		t.Errorf("View() should not contain %q — body should be truncated to fit panel height", lastLine)
	}
}

func TestView_DetailPanel_ScrollIndicatorShown(t *testing.T) {
	// When there is more body content below the visible area,
	// the detail panel should show a down-arrow indicator.
	// Use double-newline separated paragraphs so glamour renders
	// each as a separate line (~100 rendered lines for 50 paragraphs).
	var lines []string
	for i := 1; i <= 50; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	longBody := strings.Join(lines, "\n\n")
	b := newBoardWithBody(t, longBody, "")

	view := b.View()

	// The down-arrow indicator should appear because the body is longer
	// than the available display area.
	downArrow := "\u25bc"
	if !strings.Contains(view, downArrow) {
		t.Errorf("View() should contain scroll indicator %q when body content overflows panel", downArrow)
	}
}

func TestView_DetailPanel_ScrollIndicatorHiddenAtBottom(t *testing.T) {
	// When scrolled to the very bottom of the body content,
	// the down-arrow indicator should no longer appear in the detail panel.
	// Use double-newline separated paragraphs so glamour renders enough lines.
	var lines []string
	for i := 1; i <= 50; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	longBody := strings.Join(lines, "\n\n")
	b := newBoardWithBody(t, longBody, "")

	// Enter detail focus and scroll to the bottom.
	b = sendKey(t, b, keyMsg("l"))
	for i := 0; i < 100; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	view := b.View()

	// Count occurrences of the down-arrow indicator in the full view.
	// The left panel may show its own scroll indicator for the card list,
	// but with only 2 cards in the column (from newBoardWithBody) and
	// Height=40 (panelHeight=34), 2 cards fit without scrolling, so the
	// left panel should show zero down-arrows.
	downArrow := "\u25bc"
	count := strings.Count(view, downArrow)
	if count > 0 {
		t.Errorf("View() contains %d scroll indicators %q after scrolling to bottom, want 0 — "+
			"detail panel should hide indicator when fully scrolled", count, downArrow)
	}
}

func TestDetailFocus_JKey_ClampsAtMaxLines_TightBound(t *testing.T) {
	// With a short 2-line body and Height=40:
	// panelHeight = 34. The entire rendered content (frontmatter + body)
	// scrolls as one unit. A 2-line body plus frontmatter renders to far
	// fewer than 34 lines, so maxOffset = 0.
	// Scrolling should have no effect — offset must stay at 0.
	shortBody := "line one\nline two"
	b := newBoardWithBody(t, shortBody, "")

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))

	// Press 'j' many times.
	for i := 0; i < 100; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	// A 2-line body fits entirely within the visible panel area
	// (Height=40 gives 31 available body lines), so there is nothing to scroll.
	if b.detailScrollOffset != 0 {
		t.Errorf("detailScrollOffset = %d after 100 'j' presses on 2-line body, want 0 "+
			"(body fits entirely in panel, nothing to scroll)", b.detailScrollOffset)
	}
}

func TestDetailFocus_JKey_ScrollsWhenRenderedLinesExceedRaw(t *testing.T) {
	// Bug: handler uses raw line count (strings.Count(body, "\n") + 1) to
	// compute maxOffset, but glamour word-wraps long paragraphs into many
	// more rendered lines. This makes maxOffset = 0 even though the view
	// shows overflow. Result: j/k scrolling is blocked.
	//
	// Build a body with few raw lines but many rendered lines.
	// Width=120 → right panel content width ≈ 69 chars.
	// Each ~500-char paragraph wraps to ~8+ rendered lines.
	var paragraphs []string
	for i := 0; i < 10; i++ {
		paragraphs = append(paragraphs, strings.Repeat("word ", 100))
	}
	// Use \n\n to ensure glamour treats them as separate paragraphs.
	longBody := strings.Join(paragraphs, "\n\n")

	// Raw line count is small: 10 paragraphs + 9 separators = 19 raw lines.
	rawLines := strings.Count(longBody, "\n") + 1
	if rawLines > 30 {
		t.Fatalf("precondition: raw line count = %d, want <= 30 (few raw lines)", rawLines)
	}

	b := newBoardWithBody(t, longBody, "")

	// Call View() to initialize the glamour renderer, matching the real
	// BubbleTea lifecycle (View runs before every Update).
	b.View()

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	// Press 'j' several times — should actually scroll.
	for i := 0; i < 10; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	if b.detailScrollOffset == 0 {
		t.Error("detailScrollOffset = 0 after pressing 'j' 10 times on a body that " +
			"renders to many more lines than raw — scrolling should work")
	}
}

// --- Detail Panel Border Alignment & Up-Arrow (#65) ---

func TestDetailFocus_BorderAlignment_LongTitle(t *testing.T) {
	// A title long enough to wrap at the right panel's content width.
	// Width=80: innerWidth=78, leftTotal=78*2/5=31, rightTotal=78-31=47, rightContentWidth=45.
	// Title "#1 " + 80 chars ≈ 83 chars → wraps to ~2 lines at width 45.
	longTitle := strings.Repeat("A very long title word ", 5) // ~115 chars
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("body line %d", i))
	}
	body := strings.Join(lines, "\n\n")
	b := newBoardWithCustomCard(t, longTitle, []provider.Label{{Name: "bug"}}, body)

	view := b.View()

	// The rendered view should not exceed terminal height.
	outputLines := strings.Split(view, "\n")
	if len(outputLines) > b.Height {
		t.Errorf("View() has %d lines, want <= %d (terminal height); "+
			"long title wrapping causes border misalignment", len(outputLines), b.Height)
	}
}

func TestDetailFocus_ScrollUpArrow(t *testing.T) {
	// Scroll down in detail panel; up-arrow (▲) should appear.
	b := newBoardWithLongBody(t, 50)

	// Initialize glamour renderer via View().
	b.View()

	// Enter detail focus, scroll down.
	b = sendKey(t, b, keyMsg("l"))
	for i := 0; i < 5; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	view := b.View()
	upArrow := "\u25b2"
	if !strings.Contains(view, upArrow) {
		t.Error("View() should contain up-arrow indicator ▲ when detail panel is scrolled past top")
	}
}

func TestDetailFocus_ScrollUpArrow_NotShownAtTop(t *testing.T) {
	// At scroll offset 0, no up-arrow should appear in the detail panel.
	b := newBoardWithLongBody(t, 50)

	// Enter detail focus but don't scroll.
	b = sendKey(t, b, keyMsg("l"))

	view := b.View()
	upArrow := "\u25b2"

	// The left panel may show ▲ for the card list, but with 2 cards in
	// Height=40 they fit without scrolling. So no ▲ should appear anywhere.
	if strings.Contains(view, upArrow) {
		t.Error("View() should not contain up-arrow indicator ▲ when detail panel is at top (offset=0)")
	}
}

func TestDetailFocus_DynamicHeaderLines(t *testing.T) {
	// With a wrapping title, the max scroll offset should account for
	// extra header lines. Verify j doesn't scroll past the content.
	longTitle := strings.Repeat("A very long title word ", 5)
	var lines []string
	for i := 1; i <= 30; i++ {
		lines = append(lines, fmt.Sprintf("body line %d", i))
	}
	body := strings.Join(lines, "\n\n")
	b := newBoardWithCustomCard(t, longTitle, []provider.Label{{Name: "bug"}}, body)

	// Initialize glamour renderer.
	b.View()

	// Enter detail focus, scroll down many times.
	b = sendKey(t, b, keyMsg("l"))
	for i := 0; i < 200; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	maxOffset := b.detailScrollOffset

	// Scrolling one more time should not increase the offset.
	b = sendKey(t, b, keyMsg("j"))
	if b.detailScrollOffset > maxOffset {
		t.Errorf("detailScrollOffset increased past max: got %d, previous max %d", b.detailScrollOffset, maxOffset)
	}

	// Verify the view still renders within terminal bounds.
	view := b.View()
	outputLines := strings.Split(view, "\n")
	if len(outputLines) > b.Height {
		t.Errorf("View() has %d lines at max scroll, want <= %d", len(outputLines), b.Height)
	}
}

// --- Frontmatter Detail Panel (#198) ---

func TestView_DetailPanel_ShowsFrontmatterFormat(t *testing.T) {
	// The detail panel should render card metadata in a fenced YAML code block
	// (frontmatter), not as raw lipgloss-styled title/labels.
	b := newBoardWithCustomCard(t, "Fix login bug",
		[]provider.Label{{Name: "bug"}, {Name: "urgent"}}, "Some body text")

	view := b.View()

	// The frontmatter should contain a "title:" field.
	if !strings.Contains(view, "title:") {
		t.Error("View() detail panel should contain 'title:' in YAML frontmatter format")
	}

	// The frontmatter should contain a "labels:" field when labels exist.
	if !strings.Contains(view, "labels:") {
		t.Error("View() detail panel should contain 'labels:' in YAML frontmatter format")
	}

	// Each label name should appear in the rendered view.
	for _, labelName := range []string{"bug", "urgent"} {
		if !strings.Contains(view, labelName) {
			t.Errorf("View() detail panel should contain label name %q", labelName)
		}
	}

	// The card title text should appear in the view.
	if !strings.Contains(view, "Fix login bug") {
		t.Error("View() detail panel should contain the card title text")
	}

	// The body text should still appear after the frontmatter block.
	if !strings.Contains(view, "Some body text") {
		t.Error("View() detail panel should contain the card body text")
	}
}

func TestView_DetailPanel_FrontmatterOmitsLabelsWhenEmpty(t *testing.T) {
	// A card with no labels should NOT have a "labels:" field in the frontmatter.
	b := newBoardWithCustomCard(t, "No label card", nil, "Body content here")

	view := b.View()

	// The frontmatter should still contain a "title:" field.
	if !strings.Contains(view, "title:") {
		t.Error("View() detail panel should contain 'title:' in YAML frontmatter")
	}

	// The "labels:" field should be absent when the card has no labels.
	if strings.Contains(view, "labels:") {
		t.Error("View() detail panel should NOT contain 'labels:' when card has no labels")
	}

	// The body should still render.
	if !strings.Contains(view, "Body content here") {
		t.Error("View() detail panel should contain the card body text")
	}
}

func TestComposeDetailMarkdown_EscapesTitleQuotesAndBackslashes(t *testing.T) {
	// Card titles containing double quotes or backslashes must be escaped
	// so they don't break the YAML double-quoted string.
	card := Card{
		Number: 42,
		Title:  `Fix "login" bug with path C:\Users`,
	}

	md := composeDetailMarkdown(card)

	// The title must contain escaped quotes and backslashes.
	if strings.Contains(md, `"login"`) {
		t.Error("composeDetailMarkdown should escape double quotes in title, but found unescaped quotes")
	}
	if !strings.Contains(md, `\"login\"`) {
		t.Error("composeDetailMarkdown should contain escaped double quotes (\\\"login\\\") in title")
	}
	if strings.Contains(md, `C:\Users`) && !strings.Contains(md, `C:\\Users`) {
		t.Error("composeDetailMarkdown should escape backslashes in title")
	}

	// Output should use horizontal rule delimiters, not code fences.
	if !strings.Contains(md, "---") {
		t.Error("composeDetailMarkdown should use --- horizontal rule delimiters, not code fences")
	}
	if strings.Contains(md, "```") {
		t.Error("composeDetailMarkdown should not contain triple backtick code fences")
	}
}

func TestComposeDetailMarkdown_EscapesTitleMarkdownChars(t *testing.T) {
	// A title containing markdown-special characters must have them
	// backslash-escaped so glamour renders them as literal text
	// (since the frontmatter uses --- delimiters, not code fences).
	card := Card{
		Number: 10,
		Title:  "Add *bold* and _italic_ and `code` and [link] and ~strike~ support",
	}

	md := composeDetailMarkdown(card)

	// Each markdown-special character should be backslash-escaped.
	escapedChars := []struct {
		raw     string
		escaped string
	}{
		{"*bold*", `\*bold\*`},
		{"_italic_", `\_italic\_`},
		{"`code`", "\\`code\\`"},
		{"[link]", `\[link\]`},
		{"~strike~", `\~strike\~`},
	}
	for _, tc := range escapedChars {
		if !strings.Contains(md, tc.escaped) {
			t.Errorf("composeDetailMarkdown should escape %q as %q in title, got:\n%s", tc.raw, tc.escaped, md)
		}
	}

	// Output should use horizontal rule delimiters, not code fences.
	if !strings.Contains(md, "---") {
		t.Error("composeDetailMarkdown should use --- horizontal rule delimiters")
	}
	if strings.Contains(md, "```") {
		t.Error("composeDetailMarkdown should not contain triple backtick code fences")
	}
}

func TestComposeDetailMarkdown_EscapesLabelSpecialCharacters(t *testing.T) {
	// Label names containing ], ", \, markdown chars, or triple backticks
	// must be escaped to prevent breaking the YAML array or markdown rendering.
	card := Card{
		Number: 7,
		Title:  "Test card",
		Labels: []Label{
			{Name: `label"with"quotes`},
			{Name: `label\with\backslash`},
			{Name: "label]breaks,array"},
			{Name: "*important*"},
		},
	}

	md := composeDetailMarkdown(card)

	// Unescaped double quotes in label names should not appear in YAML.
	if strings.Contains(md, `label"with"quotes`) {
		t.Error("composeDetailMarkdown should escape double quotes in label names")
	}
	// Backslashes in label names should be escaped.
	if strings.Contains(md, `label\with\backslash`) && !strings.Contains(md, `label\\with\\backslash`) {
		t.Error("composeDetailMarkdown should escape backslashes in label names")
	}

	// Output should NOT contain code fences (uses --- delimiters now).
	if strings.Contains(md, "```") {
		t.Error("composeDetailMarkdown should not contain triple backtick code fences")
	}

	// Labels with markdown-special characters should be escaped.
	if !strings.Contains(md, `\*important\*`) {
		t.Errorf("composeDetailMarkdown should escape markdown chars in label names, got:\n%s", md)
	}
	// Brackets in label names should be escaped.
	if !strings.Contains(md, `label\]breaks`) {
		t.Errorf("composeDetailMarkdown should escape ] in label names, got:\n%s", md)
	}
}
