package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newBoardWithFilterableCards creates a board with two columns, each containing
// cards with specific labels and assignees for filter testing.
//
// Column "Backlog" (5 cards):
//   - #1 "Bug fix"         labels=["bug"]     assignees=["alice"]
//   - #2 "Feature work"    labels=["feature"]  assignees=["bob"]
//   - #3 "Another bug"     labels=["bug"]      assignees=["alice"]
//   - #4 "Docs update"     labels=["docs"]     assignees=["charlie"]
//   - #5 "Specific bug"    labels=["bug"]      assignees=["bob"]
//
// Column "In Progress" (2 cards):
//   - #6 "Active feature"  labels=["feature"]  assignees=["alice"]
//   - #7 "Active bug"      labels=["bug"]      assignees=["bob"]
func newBoardWithFilterableCards(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Backlog", Cards: []provider.Card{
				{Number: 1, Title: "Bug fix", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
				{Number: 2, Title: "Feature work", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
				{Number: 3, Title: "Another bug", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
				{Number: 4, Title: "Docs update", Labels: []provider.Label{{Name: "docs"}}, Assignees: []provider.Assignee{{Login: "charlie"}}},
				{Number: 5, Title: "Specific bug", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
			}},
			{Title: "In Progress", Cards: []provider.Card{
				{Number: 6, Title: "Active feature", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
				{Number: 7, Title: "Active bug", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// --- filteredCards() with global filter ---

func TestFilter_LabelFilter_ShowsOnlyMatchingCards(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Apply a label filter for "bug".
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	filtered := b.filteredCards()

	// The Backlog column has 3 cards with the "bug" label (#1, #3, #5).
	expectedCount := 3
	if len(filtered) != expectedCount {
		t.Errorf("filteredCards() with label filter 'bug': got %d cards, want %d", len(filtered), expectedCount)
	}

	// Every returned card must have the "bug" label.
	for _, card := range filtered {
		hasBug := false
		for _, label := range card.Labels {
			if strings.EqualFold(label.Name, "bug") {
				hasBug = true
				break
			}
		}
		if !hasBug {
			t.Errorf("filteredCards() returned card #%d (%q) which lacks the 'bug' label", card.Number, card.Title)
		}
	}
}

func TestFilter_AssigneeFilter_ShowsOnlyMatchingCards(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Apply an assignee filter for "alice".
	b.activeFilterType = filterByAssignee
	b.activeFilterValue = "alice"

	filtered := b.filteredCards()

	// The Backlog column has 2 cards assigned to "alice" (#1, #3).
	expectedCount := 2
	if len(filtered) != expectedCount {
		t.Errorf("filteredCards() with assignee filter 'alice': got %d cards, want %d", len(filtered), expectedCount)
	}

	// Every returned card must have "alice" as an assignee.
	for _, card := range filtered {
		hasAlice := false
		for _, a := range card.Assignees {
			if strings.EqualFold(a.Login, "alice") {
				hasAlice = true
				break
			}
		}
		if !hasAlice {
			t.Errorf("filteredCards() returned card #%d (%q) which is not assigned to 'alice'", card.Number, card.Title)
		}
	}
}

func TestFilter_CaseInsensitive(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Test label filter with uppercase value against lowercase data.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "BUG" // uppercase
	labelFiltered := b.filteredCards()

	expectedLabelCount := 3 // cards #1, #3, #5 have "bug" label
	if len(labelFiltered) != expectedLabelCount {
		t.Errorf("filteredCards() with label filter 'BUG' (uppercase): got %d cards, want %d", len(labelFiltered), expectedLabelCount)
	}

	// Test assignee filter with mixed case value against lowercase data.
	b.activeFilterType = filterByAssignee
	b.activeFilterValue = "Alice" // mixed case
	assigneeFiltered := b.filteredCards()

	expectedAssigneeCount := 2 // cards #1, #3 assigned to "alice"
	if len(assigneeFiltered) != expectedAssigneeCount {
		t.Errorf("filteredCards() with assignee filter 'Alice' (mixed case): got %d cards, want %d", len(assigneeFiltered), expectedAssigneeCount)
	}
}

func TestFilter_NoFilter_ReturnsAllCards(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Ensure no filter is active.
	b.activeFilterType = filterTypeNone
	b.activeFilterValue = ""

	filtered := b.filteredCards()

	totalCards := len(b.Columns[b.ActiveTab].Cards)
	if len(filtered) != totalCards {
		t.Errorf("filteredCards() with filterTypeNone: got %d cards, want %d (all cards)", len(filtered), totalCards)
	}
}

func TestFilter_PlusSearch_Coexist(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Apply both a global label filter AND a search query.
	// "feature" search matches "Feature work" (#2) and "Active feature" (#6 in col 2).
	// "bug" label filter should exclude #2 which has "feature" label.
	// Only cards matching BOTH "bug" label AND "feature" in title would survive.
	// None of the "bug" cards have "feature" in their title, so the result
	// should be 0 cards if both filters are applied together.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"
	b.searchQuery = "feature" // matches "Feature work" (#2) by title

	filtered := b.filteredCards()

	// "Feature work" (#2) matches search "feature" but does NOT have "bug" label.
	// The 3 bug cards (#1, #3, #5) do NOT match search "feature" in their titles.
	// So with BOTH filters, zero cards should match.
	expectedCount := 0
	if len(filtered) != expectedCount {
		t.Errorf("filteredCards() with label 'bug' + search 'feature': got %d cards, want %d (search matches non-bug card, filter should exclude it)", len(filtered), expectedCount)
	}
}

// simulateRefreshWithCards sends a boardFetchedMsg with the given columns,
// simulating a refresh that returns specific card data (not the default FakeProvider data).
func simulateRefreshWithCards(t *testing.T, b Board, columns []provider.Column) Board {
	t.Helper()
	b.refreshing = true
	m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: columns}})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return updated
}

// refreshColumnsWithBugCards returns columns matching the filterable board layout
// but with cards that include "bug" labels, suitable for testing filter persistence.
func refreshColumnsWithBugCards() []provider.Column {
	return []provider.Column{
		{Title: "Backlog", Cards: []provider.Card{
			{Number: 1, Title: "Bug fix", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
			{Number: 2, Title: "Feature work", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
			{Number: 3, Title: "Another bug", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
			{Number: 4, Title: "Docs update", Labels: []provider.Label{{Name: "docs"}}, Assignees: []provider.Assignee{{Login: "charlie"}}},
			{Number: 5, Title: "Specific bug", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
		}},
		{Title: "In Progress", Cards: []provider.Card{
			{Number: 6, Title: "Active feature", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
			{Number: 7, Title: "Active bug", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
		}},
	}
}

func TestFilter_PersistsAcrossRefresh(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Set a label filter for "bug".
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Simulate a refresh with data that still contains "bug" cards.
	b = simulateRefreshWithCards(t, b, refreshColumnsWithBugCards())

	// The filter should persist — not be cleared.
	if b.activeFilterType != filterByLabel {
		t.Errorf("after refresh: activeFilterType = %d, want %d (filterByLabel)", b.activeFilterType, filterByLabel)
	}
	if b.activeFilterValue != "bug" {
		t.Errorf("after refresh: activeFilterValue = %q, want %q", b.activeFilterValue, "bug")
	}
}

func TestFilter_CursorResetsToZeroOnRefresh(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Set a label filter for "bug" (3 matching cards in Backlog: #1, #3, #5).
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Move cursor down within filtered list.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[b.ActiveTab].Cursor < 1 {
		t.Fatalf("precondition: cursor should be > 0 after j navigation, got %d", b.Columns[b.ActiveTab].Cursor)
	}

	// Simulate a refresh with same data.
	b = simulateRefreshWithCards(t, b, refreshColumnsWithBugCards())

	// After refresh, cursor and scroll offset should reset to 0 in each column.
	for i, col := range b.Columns {
		if col.Cursor != 0 {
			t.Errorf("column %d (%q): Cursor = %d after refresh, want 0", i, col.Title, col.Cursor)
		}
		if col.ScrollOffset != 0 {
			t.Errorf("column %d (%q): ScrollOffset = %d after refresh, want 0", i, col.Title, col.ScrollOffset)
		}
	}
}

func TestFilter_CursorClampedAfterRefreshShrinks(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Set a label filter for "bug" (3 matching cards in Backlog: #1, #3, #5).
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Move cursor to last filtered card (index 2).
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))

	// Refresh with data that has fewer "bug" cards — only 1 bug card in Backlog.
	shrunkColumns := []provider.Column{
		{Title: "Backlog", Cards: []provider.Card{
			{Number: 1, Title: "Bug fix", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
			{Number: 2, Title: "Feature work", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
			{Number: 4, Title: "Docs update", Labels: []provider.Label{{Name: "docs"}}, Assignees: []provider.Assignee{{Login: "charlie"}}},
		}},
		{Title: "In Progress", Cards: []provider.Card{
			{Number: 6, Title: "Active feature", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
		}},
	}
	b = simulateRefreshWithCards(t, b, shrunkColumns)

	// The filter must persist after refresh.
	if b.activeFilterType != filterByLabel {
		t.Fatalf("after refresh: activeFilterType = %d, want %d (filterByLabel) — filter should persist", b.activeFilterType, filterByLabel)
	}

	// After refresh, filtered list in Backlog has only 1 bug card.
	filtered := b.filteredCards()
	if len(filtered) == 0 {
		t.Fatal("precondition: filteredCards() should have at least 1 card after refresh")
	}

	// Cursor must be clamped to len(filteredCards()) - 1.
	col := b.Columns[b.ActiveTab]
	maxValid := len(filtered) - 1
	if col.Cursor > maxValid {
		t.Errorf("cursor = %d after refresh with shrunk filtered list, want <= %d (filtered count = %d)",
			col.Cursor, maxValid, len(filtered))
	}
}

func TestFilter_NoMatchesHintAfterRefresh(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Set a label filter for "bug".
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Refresh with data that has zero "bug" labels anywhere.
	noBugColumns := []provider.Column{
		{Title: "Backlog", Cards: []provider.Card{
			{Number: 1, Title: "Feature work", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
			{Number: 2, Title: "Docs update", Labels: []provider.Label{{Name: "docs"}}, Assignees: []provider.Assignee{{Login: "charlie"}}},
		}},
		{Title: "In Progress", Cards: []provider.Card{
			{Number: 3, Title: "Active feature", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
		}},
	}
	b = simulateRefreshWithCards(t, b, noBugColumns)

	// The status bar should show a hint about no matches.
	view := b.View()
	if !strings.Contains(view, "Filter has no matches") {
		t.Errorf("View() after refresh with zero filter matches should contain %q, got:\n%s", "Filter has no matches", view)
	}
}

func TestFilter_FilterItemsRebuiltOnRefresh(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Set a label filter.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Verify "urgent" label is NOT in the current filter items.
	b.filterItems = b.collectFilterItems()
	for _, item := range b.filterItems {
		if !item.isHeader && item.value == "urgent" {
			t.Fatal("precondition: 'urgent' label should not exist before refresh")
		}
	}

	// Refresh with data that introduces a new "urgent" label.
	columnsWithUrgent := []provider.Column{
		{Title: "Backlog", Cards: []provider.Card{
			{Number: 1, Title: "Bug fix", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
			{Number: 2, Title: "Urgent task", Labels: []provider.Label{{Name: "urgent"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
			{Number: 3, Title: "Another bug", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
		}},
		{Title: "In Progress", Cards: []provider.Card{
			{Number: 6, Title: "Active feature", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
		}},
	}
	b = simulateRefreshWithCards(t, b, columnsWithUrgent)

	// The filterItems should be rebuilt from the new data and include "urgent".
	foundUrgent := false
	for _, item := range b.filterItems {
		if !item.isHeader && item.value == "urgent" {
			foundUrgent = true
			break
		}
	}
	if !foundUrgent {
		t.Errorf("filterItems after refresh should include new label 'urgent', got items: %v", b.filterItems)
	}
}

func TestFilter_CursorClampedOnFilterApply(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Move cursor to position 4 (last card in Backlog, 5 total cards).
	for i := 0; i < 4; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.Columns[b.ActiveTab].Cursor != 4 {
		t.Fatalf("precondition: cursor = %d, want 4", b.Columns[b.ActiveTab].Cursor)
	}

	// Enter filter mode and select a filter that reduces the list to 2 cards.
	b.filterItems = b.collectFilterItems()
	// Find the "alice" assignee item (only 2 cards in Backlog assigned to alice: #1, #3).
	b.filterCursor = 0
	for i, item := range b.filterItems {
		if !item.isHeader && item.itemType == filterByAssignee && strings.EqualFold(item.value, "alice") {
			b.filterCursor = i
			break
		}
	}
	b.mode = filterMode

	// Press Enter to apply the filter.
	m, _ := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	// After applying filter: activeFilterType should be set to filterByAssignee.
	if b.activeFilterType != filterByAssignee {
		t.Fatalf("after Enter: activeFilterType = %d, want filterByAssignee", b.activeFilterType)
	}

	// After filtering by "alice", the Backlog column should show only 2 cards.
	// The cursor was at 4 which is beyond the filtered list (2 items).
	// handleFilterModeKey (or the view logic) must clamp the cursor to a valid range.
	// Expected: cursor < 2 (the filtered count).
	expectedMaxCursor := 1 // 2 alice-cards, so max valid cursor is 1
	if b.Columns[b.ActiveTab].Cursor > expectedMaxCursor {
		t.Errorf("cursor = %d after filter applied (2 matching cards), want <= %d", b.Columns[b.ActiveTab].Cursor, expectedMaxCursor)
	}
}

// --- View rendering with filter ---

func TestFilter_TabBar_ShowsIndicator_WhenFilterActive(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Activate a filter.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	view := b.View()

	// The border title (first line) should contain a filter indicator character
	// to visually signal that a global filter is active.
	borderLine := strings.SplitN(view, "\n", 2)[0]

	// Check for filter indicator in the border title. The implementation should
	// add a visual marker (like a filled circle or filter symbol) in the title bar
	// when a filter is active.
	if !strings.Contains(borderLine, "\u25cf") {
		t.Error("border title should contain filter indicator character when filter is active")
	}
}

func TestFilter_TabBar_NoIndicator_WhenNoFilter(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Ensure no filter is active.
	b.activeFilterType = filterTypeNone
	b.activeFilterValue = ""

	view := b.View()

	// The border title line (first line of View) should not contain the filter indicator.
	// Note: label dots in the card list may contain the same character, so check only
	// the first line (border title).
	lines := strings.SplitN(view, "\n", 2)
	if len(lines) > 0 && strings.Contains(lines[0], "\u25cf") {
		t.Error("border title line should NOT contain filter indicator when no filter is active")
	}
}

func TestFilter_TabBar_ShowsFilteredCounts(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Activate a filter.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	view := b.View()

	// When a global filter is active, the border title should show filtered counts
	// for the active column. The Backlog column has 5 total cards, 3 with "bug" label.
	// Expect a "3/5" or similar filtered/total format in the border title.
	totalBacklog := len(b.Columns[0].Cards)
	expectedBugCount := 0
	for _, card := range b.Columns[0].Cards {
		for _, label := range card.Labels {
			if strings.EqualFold(label.Name, "bug") {
				expectedBugCount++
				break
			}
		}
	}
	filteredCountStr := fmt.Sprintf("%d/%d", expectedBugCount, totalBacklog)
	if !strings.Contains(view, filteredCountStr) {
		t.Errorf("View() with active filter should show filtered count %q in border title, not found in view", filteredCountStr)
	}

	// Also check the second column "In Progress" shows its filtered count.
	totalInProgress := len(b.Columns[1].Cards)
	expectedInProgressBugCount := 0
	for _, card := range b.Columns[1].Cards {
		for _, label := range card.Labels {
			if strings.EqualFold(label.Name, "bug") {
				expectedInProgressBugCount++
				break
			}
		}
	}
	inProgressCountStr := fmt.Sprintf("%d/%d", expectedInProgressBugCount, totalInProgress)
	if !strings.Contains(view, inProgressCountStr) {
		t.Errorf("View() with active filter should show filtered count %q for In Progress column, not found in view", inProgressCountStr)
	}
}

func TestFilter_EmptyState_WhenNoCardsMatch(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Apply a filter that matches no cards in the active column.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "nonexistent-label"

	view := b.View()

	if !strings.Contains(view, "No matching cards") {
		t.Errorf("View() with filter matching zero cards should contain %q, not found in view", "No matching cards")
	}
}

// --- j/k navigation respects filter ---

func TestFilter_JK_NavigatesFilteredCards(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Apply a label filter for "bug" (3 matching cards in Backlog: #1, #3, #5).
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// The Backlog column has 5 total cards but only 3 match "bug".
	// Navigate down with j past the filtered card count.
	for i := 0; i < 10; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	// The cursor should be bounded by the expected filtered count (3),
	// not the total card count (5). Max valid cursor = 2 (third item, zero-indexed).
	expectedFilteredCount := 3 // cards #1, #3, #5
	expectedMaxCursor := expectedFilteredCount - 1
	col := b.Columns[b.ActiveTab]
	if col.Cursor > expectedMaxCursor {
		t.Errorf("cursor = %d after j navigation with active filter, want <= %d (filtered count = %d, total = %d)",
			col.Cursor, expectedMaxCursor, expectedFilteredCount, len(col.Cards))
	}
}

// --- Clear filter ---

func TestFilter_ClearFilter_RestoresAllCards(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Set a filter.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Press 'f' to clear the filter (toggle behavior: clears when active).
	m, cmd := b.Update(keyMsg("f"))
	b = m.(Board)

	// The cmd should be non-nil (timed message).
	if cmd == nil {
		t.Error("'f' key should return a non-nil cmd for timed message")
	}

	if b.activeFilterType != filterTypeNone {
		t.Errorf("after 'f': activeFilterType = %d, want %d (filterTypeNone)", b.activeFilterType, filterTypeNone)
	}
	if b.activeFilterValue != "" {
		t.Errorf("after 'f': activeFilterValue = %q, want empty", b.activeFilterValue)
	}

	// All cards should now be returned.
	totalCards := len(b.Columns[b.ActiveTab].Cards)
	filtered := b.filteredCards()
	if len(filtered) != totalCards {
		t.Errorf("after clearing filter: filteredCards() = %d, want %d (all cards)", len(filtered), totalCards)
	}
}

// --- Filter persists across tab switch ---

func TestFilter_FilterPersistsAcrossTabSwitch(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Set a filter.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Switch tabs with Tab key.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	// The filter state should persist.
	if b.activeFilterType != filterByLabel {
		t.Errorf("after tab switch: activeFilterType = %d, want %d (filterByLabel)", b.activeFilterType, filterByLabel)
	}
	if b.activeFilterValue != "bug" {
		t.Errorf("after tab switch: activeFilterValue = %q, want %q", b.activeFilterValue, "bug")
	}

	// The filtered cards in the new column should also respect the filter.
	filtered := b.filteredCards()
	for _, card := range filtered {
		hasBug := false
		for _, label := range card.Labels {
			if strings.EqualFold(label.Name, "bug") {
				hasBug = true
				break
			}
		}
		if !hasBug {
			t.Errorf("after tab switch: filteredCards() returned card #%d without 'bug' label", card.Number)
		}
	}
}

// --- selectedCard() respects filter ---

func TestFilter_SelectedCard_ReturnsFilteredCard(t *testing.T) {
	b := newBoardWithFilterableCards(t)

	// Apply a label filter for "bug" (cards #1, #3, #5 match in Backlog).
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Cursor at 0 should return the first bug card (#1).
	b.Columns[b.ActiveTab].Cursor = 0
	card := b.selectedCard()
	if card.Number != 1 {
		t.Errorf("selectedCard() at cursor 0 with bug filter: got card #%d, want #1", card.Number)
	}

	// Cursor at 1 should return the second bug card (#3), NOT the raw card at index 1 (#2 Feature work).
	b.Columns[b.ActiveTab].Cursor = 1
	card = b.selectedCard()
	if card.Number != 3 {
		t.Errorf("selectedCard() at cursor 1 with bug filter: got card #%d, want #3 (not raw index 1)", card.Number)
	}

	// Cursor at 2 should return the third bug card (#5).
	b.Columns[b.ActiveTab].Cursor = 2
	card = b.selectedCard()
	if card.Number != 5 {
		t.Errorf("selectedCard() at cursor 2 with bug filter: got card #%d, want #5", card.Number)
	}

	// Without filter, cursor 1 should return raw card at index 1 (#2 Feature work).
	b.activeFilterType = filterTypeNone
	b.activeFilterValue = ""
	b.Columns[b.ActiveTab].Cursor = 1
	card = b.selectedCard()
	if card.Number != 2 {
		t.Errorf("selectedCard() at cursor 1 without filter: got card #%d, want #2", card.Number)
	}
}
