package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newBoardWithLabelsAndAssignees creates a board with 2 columns, cards with
// various labels and assignees for testing the filter picker.
// Column A cards:
//   - Card 1: labels ["bug", "feature"], assignees ["alice", "bob"]
//   - Card 2: labels ["Bug", "docs"], assignees ["Alice", "charlie"]
//
// Column B cards:
//   - Card 3: labels ["feature"], assignees ["bob"]
//
// This provides:
//   - Duplicate labels: "bug"/"Bug", "feature"/"feature" (case-insensitive dedup)
//   - Unique label: "docs"
//   - Duplicate assignees: "alice"/"Alice", "bob"/"bob" (case-insensitive dedup)
//   - Unique assignee: "charlie"
func newBoardWithLabelsAndAssignees(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{
					Number: 1,
					Title:  "Card One",
					Labels: []provider.Label{{Name: "bug"}, {Name: "feature"}},
					Assignees: []provider.Assignee{
						{Login: "alice"},
						{Login: "bob"},
					},
				},
				{
					Number: 2,
					Title:  "Card Two",
					Labels: []provider.Label{{Name: "Bug"}, {Name: "docs"}},
					Assignees: []provider.Assignee{
						{Login: "Alice"},
						{Login: "charlie"},
					},
				},
			}},
			{Title: "Column B", Cards: []provider.Card{
				{
					Number: 3,
					Title:  "Card Three",
					Labels: []provider.Label{{Name: "feature"}},
					Assignees: []provider.Assignee{
						{Login: "bob"},
					},
				},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// newBoardWithLabelsOnly creates a board with labels but no assignees.
func newBoardWithLabelsOnly(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{
					Number: 1,
					Title:  "Card One",
					Labels: []provider.Label{{Name: "bug"}, {Name: "feature"}},
				},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// newBoardWithNoLabelsOrAssignees creates a board with cards that have no labels or assignees.
func newBoardWithNoLabelsOrAssignees(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{
					Number: 1,
					Title:  "Card One",
				},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// --- collectFilterItems tests ---

func TestFilterMode_CollectFilterItems_LabelsAndAssignees(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	items := b.collectFilterItems()

	// Should have: Labels header + deduplicated labels + Assignees header + deduplicated assignees.
	// Labels: "bug", "feature", "docs" (3 unique, case-insensitive)
	// Assignees: "alice", "bob", "charlie" (3 unique, case-insensitive)
	// Total: 1 header + 3 labels + 1 header + 3 assignees = 8
	if len(items) == 0 {
		t.Fatal("collectFilterItems returned empty list, expected items")
	}

	// First item should be the "Labels" header.
	if !items[0].isHeader {
		t.Errorf("items[0].isHeader = false, want true (Labels header)")
	}
	if items[0].value != "Labels" {
		t.Errorf("items[0].value = %q, want %q", items[0].value, "Labels")
	}

	// Count non-header label items.
	labelCount := 0
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByLabel {
			labelCount++
		}
	}
	if labelCount != 3 {
		t.Errorf("label items = %d, want 3 (bug, feature, docs deduplicated case-insensitively)", labelCount)
	}

	// Check that an assignees header exists.
	hasAssigneesHeader := false
	for _, item := range items {
		if item.isHeader && item.value == "Assignees" {
			hasAssigneesHeader = true
			break
		}
	}
	if !hasAssigneesHeader {
		t.Error("expected Assignees header in items, but not found")
	}

	// Count non-header assignee items.
	assigneeCount := 0
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByAssignee {
			assigneeCount++
		}
	}
	if assigneeCount != 3 {
		t.Errorf("assignee items = %d, want 3 (alice, bob, charlie deduplicated case-insensitively)", assigneeCount)
	}
}

func TestFilterMode_CollectFilterItems_LabelsDeduplicatedCaseInsensitively(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	items := b.collectFilterItems()

	// "bug" and "Bug" should appear only once.
	bugCount := 0
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByLabel && strings.EqualFold(item.value, "bug") {
			bugCount++
		}
	}
	if bugCount != 1 {
		t.Errorf("bug label count = %d, want 1 (should be deduplicated case-insensitively)", bugCount)
	}
}

func TestFilterMode_CollectFilterItems_AssigneesDeduplicatedCaseInsensitively(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	items := b.collectFilterItems()

	// "alice" and "Alice" should appear only once.
	aliceCount := 0
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByAssignee && strings.EqualFold(item.value, "alice") {
			aliceCount++
		}
	}
	if aliceCount != 1 {
		t.Errorf("alice assignee count = %d, want 1 (should be deduplicated case-insensitively)", aliceCount)
	}
}

func TestFilterMode_CollectFilterItems_LabelsOnly_NoAssigneesHeader(t *testing.T) {
	b := newBoardWithLabelsOnly(t)
	items := b.collectFilterItems()

	// Should have labels but no "Assignees" header.
	if len(items) == 0 {
		t.Fatal("collectFilterItems returned empty list, expected label items")
	}

	for _, item := range items {
		if item.isHeader && item.value == "Assignees" {
			t.Error("expected no Assignees header when no assignees exist")
		}
		if item.itemType == filterByAssignee {
			t.Error("expected no assignee items when no assignees exist")
		}
	}
}

func TestFilterMode_CollectFilterItems_NoLabelsOrAssignees_EmptyList(t *testing.T) {
	b := newBoardWithNoLabelsOrAssignees(t)
	items := b.collectFilterItems()

	if len(items) != 0 {
		t.Errorf("collectFilterItems with no labels or assignees = %d items, want 0", len(items))
	}
}

// --- Mode transition tests ---

func TestFilterMode_FKeyEntersFilterMode(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	if b.mode != filterMode {
		t.Errorf("after pressing 'f': mode = %d, want filterMode", b.mode)
	}
}

func TestFilterMode_FKeyPopulatesFilterItems(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	if len(b.filterItems) == 0 {
		t.Error("after pressing 'f': filterItems is empty, expected populated items")
	}
}

func TestFilterMode_FKeySetsFilterCursorToFirstSelectableItem(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// The first item is a header (index 0), so cursor should be at index 1 (first selectable item).
	if b.filterCursor == 0 {
		t.Error("filterCursor should skip the header at index 0")
	}
	if b.filterCursor >= len(b.filterItems) {
		t.Fatal("filterCursor out of bounds")
	}
	if b.filterItems[b.filterCursor].isHeader {
		t.Errorf("filterCursor points to a header item, should point to a selectable item")
	}
}

func TestFilterMode_EscapeReturnsToNormalMode(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("expected filterMode after 'f', got %d", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Escape in filterMode: mode = %d, want normalMode", b.mode)
	}
}

func TestFilterMode_EscapeDoesNotChangeFilter(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	// Set an active filter first.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Enter filter mode and press Escape.
	b = sendKey(t, b, keyMsg("f"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	// Filter should remain unchanged.
	if b.activeFilterValue != "bug" {
		t.Errorf("after Escape: activeFilterValue = %q, want %q (should not change)", b.activeFilterValue, "bug")
	}
	if b.activeFilterType != filterByLabel {
		t.Errorf("after Escape: activeFilterType changed, should not change")
	}
}

func TestFilterMode_EnterSelectsItemAndReturnsToNormalMode(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("expected filterMode after 'f', got %d", b.mode)
	}

	// Cursor should be on the first selectable label item.
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != normalMode {
		t.Errorf("after Enter in filterMode: mode = %d, want normalMode", b.mode)
	}
	if b.activeFilterValue == "" {
		t.Error("after Enter in filterMode: activeFilterValue is empty, expected a selected value")
	}
}

// --- Picker navigation tests ---

func TestFilterMode_JMovesDown_SkipsHeaders(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))
	initialCursor := b.filterCursor

	b = sendKey(t, b, keyMsg("j"))

	if b.filterCursor <= initialCursor {
		t.Errorf("after 'j': filterCursor = %d, want > %d", b.filterCursor, initialCursor)
	}
	if b.filterItems[b.filterCursor].isHeader {
		t.Error("after 'j': cursor should not land on a header item")
	}
}

func TestFilterMode_KMovesUp_SkipsHeaders(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// Move down first so we can move up.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	cursorAfterDown := b.filterCursor

	b = sendKey(t, b, keyMsg("k"))

	if b.filterCursor >= cursorAfterDown {
		t.Errorf("after 'k': filterCursor = %d, want < %d", b.filterCursor, cursorAfterDown)
	}
	if b.filterItems[b.filterCursor].isHeader {
		t.Error("after 'k': cursor should not land on a header item")
	}
}

func TestFilterMode_DownArrowMovesDown(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))
	initialCursor := b.filterCursor

	b = sendKey(t, b, arrowMsg(tea.KeyDown))

	if b.filterCursor <= initialCursor {
		t.Errorf("after down arrow: filterCursor = %d, want > %d", b.filterCursor, initialCursor)
	}
}

func TestFilterMode_UpArrowMovesUp(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// Move down first.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	cursorAfterDown := b.filterCursor

	b = sendKey(t, b, arrowMsg(tea.KeyUp))

	if b.filterCursor >= cursorAfterDown {
		t.Errorf("after up arrow: filterCursor = %d, want < %d", b.filterCursor, cursorAfterDown)
	}
}

func TestFilterMode_CursorDoesNotGoPastLastSelectableItem(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// Move cursor down many times to try to go past the last item.
	for i := 0; i < len(b.filterItems)+5; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	if b.filterCursor >= len(b.filterItems) {
		t.Errorf("filterCursor = %d, should be within bounds (len = %d)", b.filterCursor, len(b.filterItems))
	}
	if b.filterItems[b.filterCursor].isHeader {
		t.Error("cursor should not land on a header item")
	}
}

func TestFilterMode_CursorDoesNotGoBeforeFirstSelectableItem(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// Move cursor up many times to try to go before the first selectable item.
	for i := 0; i < len(b.filterItems)+5; i++ {
		b = sendKey(t, b, keyMsg("k"))
	}

	// Cursor should still be on a selectable item (not a header).
	if b.filterItems[b.filterCursor].isHeader {
		t.Error("cursor should not land on a header item after pressing 'k' many times")
	}
}

func TestFilterMode_NavigationSkipsAssigneesHeader(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// Navigate through all items. At no point should cursor be on a header.
	for i := 0; i < len(b.filterItems); i++ {
		if b.filterItems[b.filterCursor].isHeader {
			t.Errorf("cursor at index %d is on a header item, should skip headers", b.filterCursor)
		}
		b = sendKey(t, b, keyMsg("j"))
	}
}

// --- Selection tests ---

func TestFilterMode_SelectLabel_SetsFilterByLabel(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// Cursor should be on the first label (after the header).
	selectedItem := b.filterItems[b.filterCursor]
	if selectedItem.itemType != filterByLabel {
		t.Fatalf("expected first selectable item to be a label, got type %d", selectedItem.itemType)
	}

	expectedValue := selectedItem.value
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.activeFilterType != filterByLabel {
		t.Errorf("activeFilterType = %d, want filterByLabel", b.activeFilterType)
	}
	if b.activeFilterValue != expectedValue {
		t.Errorf("activeFilterValue = %q, want %q", b.activeFilterValue, expectedValue)
	}
}

func TestFilterMode_SelectAssignee_SetsFilterByAssignee(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// Navigate past all labels to reach an assignee item.
	for b.filterItems[b.filterCursor].itemType != filterByAssignee {
		b = sendKey(t, b, keyMsg("j"))
		if b.filterCursor >= len(b.filterItems)-1 {
			t.Fatal("could not reach an assignee item")
		}
	}

	selectedItem := b.filterItems[b.filterCursor]
	expectedValue := selectedItem.value
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.activeFilterType != filterByAssignee {
		t.Errorf("activeFilterType = %d, want filterByAssignee", b.activeFilterType)
	}
	if b.activeFilterValue != expectedValue {
		t.Errorf("activeFilterValue = %q, want %q", b.activeFilterValue, expectedValue)
	}
}

// --- Clear filter tests ---

func TestFilterMode_ShiftFClearsFilter(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	// Set an active filter.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	b = sendKey(t, b, keyMsg("F"))

	if b.activeFilterValue != "" {
		t.Errorf("after 'F': activeFilterValue = %q, want empty", b.activeFilterValue)
	}
}

func TestFilterMode_ShiftFShowsTimedMessage(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	// Set an active filter.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	m, cmd := b.Update(keyMsg("F"))
	b = m.(Board)

	// The command should be non-nil (timed message for "Filter cleared").
	if cmd == nil {
		t.Error("after 'F': expected non-nil cmd for timed message")
	}

	if b.statusBar.message == "" {
		t.Error("after 'F': expected a status bar message about filter cleared")
	}
}

// --- View rendering tests ---

func TestFilterMode_ViewRendersModal(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	view := b.View()
	if view == "" {
		t.Fatal("View() returned empty string in filterMode")
	}
}

func TestFilterMode_ViewContainsFilterItems(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	view := b.View()

	// The view should contain label names.
	foundLabel := false
	for _, item := range b.filterItems {
		if !item.isHeader && item.itemType == filterByLabel {
			if strings.Contains(view, item.value) {
				foundLabel = true
				break
			}
		}
	}
	if !foundLabel {
		t.Error("View() in filterMode should contain at least one label name")
	}
}

func TestFilterMode_ViewContainsSectionHeaders(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	view := b.View()

	if !strings.Contains(view, "Labels") {
		t.Error("View() in filterMode should contain 'Labels' header")
	}
	if !strings.Contains(view, "Assignees") {
		t.Error("View() in filterMode should contain 'Assignees' header")
	}
}

func TestFilterMode_ViewHighlightsSelectedItem(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// The selected item's value should appear in the view.
	selectedItem := b.filterItems[b.filterCursor]
	view := b.View()

	if !strings.Contains(view, selectedItem.value) {
		t.Errorf("View() should contain the selected item value %q", selectedItem.value)
	}
}

// --- Status bar hints tests ---

func TestFilterMode_ShowsFilterModeHints(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	view := b.View()

	if !strings.Contains(view, "Cancel") {
		t.Error("View() in filterMode should contain hint 'Cancel'")
	}
	if !strings.Contains(view, "Navigate") {
		t.Error("View() in filterMode should contain hint 'Navigate'")
	}
	if !strings.Contains(view, "Select") {
		t.Error("View() in filterMode should contain hint 'Select'")
	}
}

func TestFilterMode_NormalModeHintsIncludeF(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	// Check that normal mode hints include a filter-related hint.
	foundFilter := false
	for _, hint := range b.normalHints {
		if hint.Key == "f" {
			foundFilter = true
			break
		}
	}
	if !foundFilter {
		t.Error("normalHints should include 'f' for filter")
	}
}

// --- Help content test ---

func TestFilterMode_HelpContentIncludesFilterSection(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	content := b.buildHelpContent()

	if !strings.Contains(content, "Filter") {
		t.Error("buildHelpContent() should include a 'Filter' section")
	}
	if !strings.Contains(content, "f") {
		t.Error("buildHelpContent() Filter section should mention 'f' key")
	}
	if !strings.Contains(content, "F") {
		t.Error("buildHelpContent() Filter section should mention 'F' key")
	}
}

// --- Edge case tests ---

func TestFilterMode_FKeyOnEmptyBoard_DoesNotEnterFilterMode(t *testing.T) {
	b := newBoardWithNoLabelsOrAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// Should not enter filter mode if there are no filter items.
	if b.mode == filterMode {
		t.Error("should not enter filterMode when no labels or assignees exist")
	}
}

func TestFilterMode_CtrlCQuits(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("expected filterMode after 'f', got %d", b.mode)
	}

	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in filterMode should return a non-nil Cmd (tea.Quit)")
	}
}

func TestFilterMode_BlocksNavigation(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	requireColumns(t, b)

	origTab := b.ActiveTab
	origCursor := b.Columns[b.ActiveTab].Cursor

	b = sendKey(t, b, keyMsg("f"))

	// Tab should not switch columns.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != origTab {
		t.Errorf("Tab in filterMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}
	if b.Columns[origTab].Cursor != origCursor {
		t.Errorf("cursor changed in filterMode, should not change")
	}
}

// --- Column-name label exclusion tests ---

// newBoardWithColumnNameLabels creates a board where column titles overlap with
// card labels. This verifies that collectFilterItems excludes labels that match
// column names.
//
// Columns: "To Do", "In Progress"
// Card 1 (in "To Do"):     labels ["bug", "To Do"],       assignees ["alice"]
// Card 2 (in "In Progress"): labels ["In Progress", "feature"], assignees ["bob"]
func newBoardWithColumnNameLabels(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "To Do", Cards: []provider.Card{
				{
					Number: 1,
					Title:  "Card One",
					Labels: []provider.Label{{Name: "bug"}, {Name: "To Do"}},
					Assignees: []provider.Assignee{
						{Login: "alice"},
					},
				},
			}},
			{Title: "In Progress", Cards: []provider.Card{
				{
					Number: 2,
					Title:  "Card Two",
					Labels: []provider.Label{{Name: "In Progress"}, {Name: "feature"}},
					Assignees: []provider.Assignee{
						{Login: "bob"},
					},
				},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

func TestFilterMode_CollectFilterItems_ExcludesColumnNameLabels(t *testing.T) {
	b := newBoardWithColumnNameLabels(t)
	items := b.collectFilterItems()

	// Only "bug" and "feature" should appear as label items.
	// "To Do" and "In Progress" should be excluded because they match column titles.
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByLabel {
			lower := strings.ToLower(item.value)
			if lower == "to do" || lower == "in progress" {
				t.Errorf("label %q should be excluded because it matches a column name", item.value)
			}
		}
	}

	labelCount := 0
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByLabel {
			labelCount++
		}
	}
	if labelCount != 2 {
		t.Errorf("label items = %d, want 2 (bug, feature); column-name labels should be excluded", labelCount)
	}
}

func TestFilterMode_CollectFilterItems_ExcludesColumnNamesCaseInsensitive(t *testing.T) {
	// Create a board where card labels use different casing than column titles.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "To Do", Cards: []provider.Card{
				{
					Number: 1,
					Title:  "Card One",
					Labels: []provider.Label{{Name: "bug"}, {Name: "to do"}},
				},
			}},
			{Title: "In Progress", Cards: []provider.Card{
				{
					Number: 2,
					Title:  "Card Two",
					Labels: []provider.Label{{Name: "IN PROGRESS"}, {Name: "feature"}},
				},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40

	items := board.collectFilterItems()

	// "to do" (lowercase) and "IN PROGRESS" (uppercase) should still be excluded
	// because column name matching is case-insensitive.
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByLabel {
			lower := strings.ToLower(item.value)
			if lower == "to do" || lower == "in progress" {
				t.Errorf("label %q should be excluded (case-insensitive match to column name)", item.value)
			}
		}
	}

	labelCount := 0
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByLabel {
			labelCount++
		}
	}
	if labelCount != 2 {
		t.Errorf("label items = %d, want 2 (bug, feature); case-variant column-name labels should be excluded", labelCount)
	}
}

func TestFilterMode_CollectFilterItems_AllLabelsAreColumnNames_OmitsLabelSection(t *testing.T) {
	// Board where every label is a column name — no labels should remain after exclusion.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "To Do", Cards: []provider.Card{
				{
					Number:    1,
					Title:     "Card One",
					Labels:    []provider.Label{{Name: "To Do"}},
					Assignees: []provider.Assignee{{Login: "alice"}},
				},
			}},
			{Title: "Done", Cards: []provider.Card{
				{
					Number:    2,
					Title:     "Card Two",
					Labels:    []provider.Label{{Name: "Done"}},
					Assignees: []provider.Assignee{{Login: "bob"}},
				},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40

	items := board.collectFilterItems()

	// No "Labels" header should exist since all labels are column names.
	for _, item := range items {
		if item.isHeader && item.value == "Labels" {
			t.Error("expected no 'Labels' header when all labels match column names")
		}
		if !item.isHeader && item.itemType == filterByLabel {
			t.Errorf("unexpected label item %q; all labels should be excluded as column names", item.value)
		}
	}

	// Assignees should still be present.
	hasAssigneesHeader := false
	for _, item := range items {
		if item.isHeader && item.value == "Assignees" {
			hasAssigneesHeader = true
			break
		}
	}
	if !hasAssigneesHeader {
		t.Error("expected 'Assignees' header even when all labels are excluded")
	}
}

func TestFilterMode_CollectFilterItems_AssigneesUnaffectedByColumnExclusion(t *testing.T) {
	b := newBoardWithColumnNameLabels(t)
	items := b.collectFilterItems()

	// Assignees should be unaffected by column-name label exclusion.
	// The board has "alice" and "bob" as assignees.
	assigneeCount := 0
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByAssignee {
			assigneeCount++
		}
	}
	if assigneeCount != 2 {
		t.Errorf("assignee items = %d, want 2 (alice, bob); assignees should not be affected by column-name exclusion", assigneeCount)
	}
}
