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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

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

// newBoardWithMilestones creates a board with 2 columns, cards carrying
// labels, assignees, AND milestones, for testing the "Milestones" filter
// picker section (#462). Deliberately not built on top of
// newBoardWithLabelsAndAssignees per lessons-learned: that fixture's cards
// intentionally have empty milestones and other tests rely on its counts
// being unaffected.
//
// Column "To Do":
//   - Card 1: labels ["bug"], assignees ["alice"], milestone "v1.0"
//   - Card 2: labels ["feature"], assignees ["bob"], milestone "V1.0" (case-dup of v1.0)
//
// Column "Done":
//   - Card 3: labels ["docs"], assignees ["alice"], milestone "To Do" (same text as the "To Do" column title)
//   - Card 4: labels [], assignees [], milestone "" (empty — must never appear as a filter item)
//
// This provides:
//   - Duplicate milestones: "v1.0"/"V1.0" (case-insensitive dedup)
//   - A milestone whose value equals a column title ("To Do") — milestones are
//     a distinct namespace from board columns and must NOT be excluded like labels are.
//   - An empty milestone that must be skipped entirely.
func newBoardWithMilestones(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "To Do", Cards: []provider.Card{
				{
					Number:    1,
					Title:     "Card One",
					Labels:    []provider.Label{{Name: "bug"}},
					Assignees: []provider.Assignee{{Login: "alice"}},
					Milestone: "v1.0",
				},
				{
					Number:    2,
					Title:     "Card Two",
					Labels:    []provider.Label{{Name: "feature"}},
					Assignees: []provider.Assignee{{Login: "bob"}},
					Milestone: "V1.0",
				},
			}},
			{Title: "Done", Cards: []provider.Card{
				{
					Number:    3,
					Title:     "Card Three",
					Labels:    []provider.Label{{Name: "docs"}},
					Assignees: []provider.Assignee{{Login: "alice"}},
					Milestone: "To Do",
				},
				{
					Number:    4,
					Title:     "Card Four",
					Milestone: "",
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

// newBoardWithMilestonesOnly creates a board where cards have a milestone but
// no labels or assignees, to verify collectFilterItems' early-return guard
// accounts for milestones (not just labels/assignees).
func newBoardWithMilestonesOnly(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{
					Number:    1,
					Title:     "Card One",
					Milestone: "v1.0",
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

func TestFilterMode_CollectFilterItems_MilestonesSectionAfterAssignees(t *testing.T) {
	b := newBoardWithMilestones(t)
	items := b.collectFilterItems()

	assigneesIdx := -1
	milestonesIdx := -1
	for i, item := range items {
		if item.isHeader && item.value == "Assignees" {
			assigneesIdx = i
		}
		if item.isHeader && item.value == "Milestones" {
			milestonesIdx = i
		}
	}
	if assigneesIdx == -1 {
		t.Fatal("expected an Assignees header in items, but not found")
	}
	if milestonesIdx == -1 {
		t.Fatal("expected a Milestones header in items, but not found")
	}
	if milestonesIdx < assigneesIdx {
		t.Errorf("Milestones header at index %d, want after Assignees header at index %d", milestonesIdx, assigneesIdx)
	}

	milestoneCount := 0
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByMilestone {
			milestoneCount++
		}
	}
	if milestoneCount != 2 {
		t.Errorf("milestone items = %d, want 2 (v1.0, To Do deduplicated case-insensitively)", milestoneCount)
	}
}

func TestFilterMode_CollectFilterItems_MilestonesDeduplicatedCaseInsensitively(t *testing.T) {
	b := newBoardWithMilestones(t)
	items := b.collectFilterItems()

	// "v1.0" and "V1.0" should appear only once.
	v1Count := 0
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByMilestone && strings.EqualFold(item.value, "v1.0") {
			v1Count++
		}
	}
	if v1Count != 1 {
		t.Errorf("v1.0 milestone count = %d, want 1 (should be deduplicated case-insensitively)", v1Count)
	}
}

func TestFilterMode_CollectFilterItems_MilestoneEqualsColumnName_NotExcluded(t *testing.T) {
	b := newBoardWithMilestones(t)
	items := b.collectFilterItems()

	// Milestone "To Do" matches the "To Do" column title, but milestones are a
	// distinct namespace from board columns (unlike labels) and must still appear.
	found := false
	for _, item := range items {
		if !item.isHeader && item.itemType == filterByMilestone && strings.EqualFold(item.value, "To Do") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected milestone \"To Do\" in items even though it matches a column title; milestones must not be excluded like labels")
	}
}

func TestFilterMode_CollectFilterItems_EmptyMilestoneSkipped(t *testing.T) {
	b := newBoardWithMilestones(t)
	items := b.collectFilterItems()

	for _, item := range items {
		if !item.isHeader && item.itemType == filterByMilestone && item.value == "" {
			t.Error("collectFilterItems should never produce an empty-string milestone item")
		}
	}
}

func TestFilterMode_CollectFilterItems_NoMilestones_NoMilestonesHeader(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	items := b.collectFilterItems()

	for _, item := range items {
		if item.isHeader && item.value == "Milestones" {
			t.Error("expected no Milestones header when no card has a milestone")
		}
		if item.itemType == filterByMilestone {
			t.Error("expected no milestone items when no card has a milestone")
		}
	}
}

func TestFilterMode_CollectFilterItems_MilestonesOnly_GuardIncludesItems(t *testing.T) {
	b := newBoardWithMilestonesOnly(t)
	items := b.collectFilterItems()

	// The early-return guard must account for milestones, not just labels/assignees.
	if len(items) == 0 {
		t.Fatal("collectFilterItems returned empty list, expected milestone items (guard should not ignore milestones)")
	}

	hasMilestonesHeader := false
	for _, item := range items {
		if item.isHeader && item.value == "Milestones" {
			hasMilestonesHeader = true
			break
		}
	}
	if !hasMilestonesHeader {
		t.Error("expected a Milestones header when the board has milestones but no labels or assignees")
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

	// Enter filter mode first (no active filter).
	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("expected filterMode after 'f', got %d", b.mode)
	}

	// Select a filter item to set an active filter.
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.activeFilterValue == "" {
		t.Fatal("expected active filter after selecting an item")
	}
	savedValue := b.activeFilterValue
	savedType := b.activeFilterType

	// Now clear filter with 'f' to get back to no-filter state.
	b = sendKey(t, b, keyMsg("f"))

	// Enter filter mode again (no active filter).
	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("expected filterMode after second 'f', got %d", b.mode)
	}

	// Press Escape to leave filter mode without selecting.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	// Filter should remain cleared (Escape does not change filter).
	if b.activeFilterValue != "" {
		t.Errorf("after Escape: activeFilterValue = %q, want empty (Escape should not change filter)", b.activeFilterValue)
	}
	_ = savedValue
	_ = savedType
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

func TestFilterMode_NavigationSkipsMilestonesHeader(t *testing.T) {
	b := newBoardWithMilestones(t)

	b = sendKey(t, b, keyMsg("f"))

	// Navigate through all items. At no point should cursor be on a header,
	// including the new "Milestones" header.
	for i := 0; i < len(b.filterItems); i++ {
		if b.filterItems[b.filterCursor].isHeader {
			t.Errorf("cursor at index %d is on a header item, should skip headers", b.filterCursor)
		}
		b = sendKey(t, b, keyMsg("j"))
	}
}

// --- Wrap-around tests (#426 PR 2) ---
//
// The filter picker's header-skipping cursor is rebuilt on the shared
// moveCursor wrap primitive: moving down from the last selectable item wraps
// to the first selectable item (skipping the leading "Labels" header), and
// moving up from the first selectable item wraps to the last selectable item
// -- never landing on a header in either direction.

// selectableFilterCount counts the non-header items in b.filterItems.
func selectableFilterCount(b Board) int {
	count := 0
	for _, item := range b.filterItems {
		if !item.isHeader {
			count++
		}
	}
	return count
}

func TestFilterMode_JWrapsFromLastSelectableToFirstSelectable(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	firstSelectable := b.filterCursor
	selectableCount := selectableFilterCount(b)
	if selectableCount <= 1 {
		t.Fatal("fixture needs more than one selectable filter item to test wraparound")
	}

	// Walk down to the last selectable item.
	for i := 0; i < selectableCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.filterItems[b.filterCursor].isHeader {
		t.Fatalf("precondition: cursor landed on a header at index %d", b.filterCursor)
	}

	// One more 'j' from the last selectable item should wrap to the first
	// selectable item, skipping the leading "Labels" header.
	b = sendKey(t, b, keyMsg("j"))
	if b.filterCursor != firstSelectable {
		t.Errorf("cursor after 'j' past last selectable item = %d, want %d (wrap to first selectable)", b.filterCursor, firstSelectable)
	}
	if b.filterItems[b.filterCursor].isHeader {
		t.Error("cursor after wrap-down landed on a header item")
	}
}

func TestFilterMode_KWrapsFromFirstSelectableToLastSelectable(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	firstSelectable := b.filterCursor
	selectableCount := selectableFilterCount(b)
	if selectableCount <= 1 {
		t.Fatal("fixture needs more than one selectable filter item to test wraparound")
	}

	// Walk down to the last selectable item to know its index.
	for i := 0; i < selectableCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	lastSelectable := b.filterCursor

	// Walk back up to the first selectable item.
	for i := 0; i < selectableCount-1; i++ {
		b = sendKey(t, b, keyMsg("k"))
	}
	if b.filterCursor != firstSelectable {
		t.Fatalf("precondition: cursor = %d, want %d (first selectable)", b.filterCursor, firstSelectable)
	}

	// One more 'k' from the first selectable item should wrap to the last
	// selectable item, never landing on the leading "Labels" header.
	b = sendKey(t, b, keyMsg("k"))
	if b.filterCursor != lastSelectable {
		t.Errorf("cursor after 'k' before first selectable item = %d, want %d (wrap to last selectable)", b.filterCursor, lastSelectable)
	}
	if b.filterItems[b.filterCursor].isHeader {
		t.Error("cursor after wrap-up landed on a header item")
	}
}

func TestFilterMode_DownArrowWrapsFromLastSelectableToFirstSelectable(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	firstSelectable := b.filterCursor
	selectableCount := selectableFilterCount(b)
	if selectableCount <= 1 {
		t.Fatal("fixture needs more than one selectable filter item to test wraparound")
	}

	for i := 0; i < selectableCount-1; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyDown))
	}

	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if b.filterCursor != firstSelectable {
		t.Errorf("cursor after Down arrow past last selectable item = %d, want %d (wrap to first selectable)", b.filterCursor, firstSelectable)
	}
	if b.filterItems[b.filterCursor].isHeader {
		t.Error("cursor after wrap-down landed on a header item")
	}
}

func TestFilterMode_UpArrowWrapsFromFirstSelectableToLastSelectable(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	firstSelectable := b.filterCursor
	selectableCount := selectableFilterCount(b)
	if selectableCount <= 1 {
		t.Fatal("fixture needs more than one selectable filter item to test wraparound")
	}

	for i := 0; i < selectableCount-1; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyDown))
	}
	lastSelectable := b.filterCursor

	for i := 0; i < selectableCount-1; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyUp))
	}
	if b.filterCursor != firstSelectable {
		t.Fatalf("precondition: cursor = %d, want %d (first selectable)", b.filterCursor, firstSelectable)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	if b.filterCursor != lastSelectable {
		t.Errorf("cursor after Up arrow before first selectable item = %d, want %d (wrap to last selectable)", b.filterCursor, lastSelectable)
	}
	if b.filterItems[b.filterCursor].isHeader {
		t.Error("cursor after wrap-up landed on a header item")
	}
}

// TestFilterMode_ZeroSelectableItems_NoOp covers the moveCursor length<=1
// no-op guard for a filter picker forced into filterMode with zero
// selectable items -- j/k must leave the cursor at 0 and must not panic
// walking the header-skip loop.
func TestFilterMode_ZeroSelectableItems_NoOp(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b.mode = filterMode
	b.filterItems = nil
	b.filterCursor = 0

	b = sendKey(t, b, keyMsg("j"))
	if b.filterCursor != 0 {
		t.Errorf("cursor after 'j' with zero selectable items = %d, want 0 (no-op)", b.filterCursor)
	}

	b = sendKey(t, b, keyMsg("k"))
	if b.filterCursor != 0 {
		t.Errorf("cursor after 'k' with zero selectable items = %d, want 0 (no-op)", b.filterCursor)
	}
}

// TestFilterMode_SingleSelectableItem_NoOp covers the moveCursor length<=1
// no-op guard for a filter picker with exactly one selectable item.
func TestFilterMode_SingleSelectableItem_NoOp(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "Card One", Labels: []provider.Label{{Name: "bug"}}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40

	board = sendKey(t, board, keyMsg("f"))
	if board.mode != filterMode {
		t.Fatalf("expected filterMode after 'f', got %d", board.mode)
	}
	if got := selectableFilterCount(board); got != 1 {
		t.Fatalf("precondition: selectableFilterCount = %d, want 1", got)
	}
	initialCursor := board.filterCursor

	board = sendKey(t, board, keyMsg("j"))
	if board.filterCursor != initialCursor {
		t.Errorf("cursor after 'j' with single selectable item = %d, want %d (no-op)", board.filterCursor, initialCursor)
	}

	board = sendKey(t, board, keyMsg("k"))
	if board.filterCursor != initialCursor {
		t.Errorf("cursor after 'k' with single selectable item = %d, want %d (no-op)", board.filterCursor, initialCursor)
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

func TestFilterMode_SelectMilestone_SetsFilterByMilestone(t *testing.T) {
	b := newBoardWithMilestones(t)

	b = sendKey(t, b, keyMsg("f"))

	// Navigate past all labels and assignees to reach a milestone item.
	for b.filterItems[b.filterCursor].itemType != filterByMilestone {
		b = sendKey(t, b, keyMsg("j"))
		if b.filterCursor >= len(b.filterItems)-1 {
			t.Fatal("could not reach a milestone item")
		}
	}

	selectedItem := b.filterItems[b.filterCursor]
	expectedValue := selectedItem.value
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.activeFilterType != filterByMilestone {
		t.Errorf("activeFilterType = %d, want filterByMilestone", b.activeFilterType)
	}
	if b.activeFilterValue != expectedValue {
		t.Errorf("activeFilterValue = %q, want %q", b.activeFilterValue, expectedValue)
	}
}

func TestFilterMode_SelectMilestone_ClearsPriorLabelFilter(t *testing.T) {
	b := newBoardWithMilestones(t)

	// Simulate a pre-existing label filter, as if the user had previously
	// selected a label before opening the picker again.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	b.filterItems = b.collectFilterItems()
	b.mode = filterMode

	// Move the filter cursor to a milestone item.
	milestoneCursor := -1
	for i, item := range b.filterItems {
		if !item.isHeader && item.itemType == filterByMilestone {
			milestoneCursor = i
			break
		}
	}
	if milestoneCursor == -1 {
		t.Fatal("precondition: could not find a milestone item in filterItems")
	}
	b.filterCursor = milestoneCursor
	expectedValue := b.filterItems[milestoneCursor].value

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	// Selecting a filter item in filterMode is a synchronous state change with
	// no side effect to report, so Enter returns a nil cmd.
	if cmd != nil {
		t.Errorf("cmd after selecting a milestone filter item = %v, want nil", cmd)
	}

	if board.activeFilterType != filterByMilestone {
		t.Errorf("activeFilterType = %d, want filterByMilestone (selecting a milestone should overwrite the prior label filter)", board.activeFilterType)
	}
	if board.activeFilterValue != expectedValue {
		t.Errorf("activeFilterValue = %q, want %q", board.activeFilterValue, expectedValue)
	}
}

// --- Clear filter tests ---

func TestFilterMode_FToggleClearsActiveMilestoneFilter(t *testing.T) {
	b := newBoardWithMilestones(t)

	// Set an active milestone filter.
	b.activeFilterType = filterByMilestone
	b.activeFilterValue = "v1.0"

	m, cmd := b.Update(keyMsg("f"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if cmd == nil {
		t.Error("after 'f' with active milestone filter: expected non-nil cmd for timed message")
	}
	if board.activeFilterValue != "" {
		t.Errorf("after 'f' with active milestone filter: activeFilterValue = %q, want empty", board.activeFilterValue)
	}
	if board.activeFilterType != filterTypeNone {
		t.Errorf("after 'f' with active milestone filter: activeFilterType = %d, want filterTypeNone", board.activeFilterType)
	}
	if board.statusBar.message == "" {
		t.Error("after 'f' with active milestone filter: expected a status bar message about filter cleared")
	}
	if !strings.Contains(board.statusBar.message, "Filter cleared") {
		t.Errorf("statusBar.message = %q, want to contain %q", board.statusBar.message, "Filter cleared")
	}
}

func TestFilterMode_FToggleClearsActiveFilter(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	// Set an active filter.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	b = sendKey(t, b, keyMsg("f"))

	if b.activeFilterValue != "" {
		t.Errorf("after 'f' with active filter: activeFilterValue = %q, want empty", b.activeFilterValue)
	}
}

func TestFilterMode_FToggleShowsTimedMessage(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	// Set an active filter.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	m, cmd := b.Update(keyMsg("f"))
	b = m.(Board)

	// The command should be non-nil (timed message for "Filter cleared").
	if cmd == nil {
		t.Error("after 'f' with active filter: expected non-nil cmd for timed message")
	}

	if b.statusBar.message == "" {
		t.Error("after 'f' with active filter: expected a status bar message about filter cleared")
	}
}

func TestFilterMode_FToggleOpensPickerWhenNoFilter(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	// Ensure no filter is active.
	if b.activeFilterType != filterTypeNone {
		t.Fatal("precondition: expected no active filter")
	}

	b = sendKey(t, b, keyMsg("f"))

	if b.mode != filterMode {
		t.Errorf("after 'f' with no active filter: mode = %d, want filterMode", b.mode)
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

// TestFilterMode_View_SanitizesControlSequencesInFilterItemValue covers the
// filter picker modal render path (#469): filterItem.value holds untrusted
// GitHub content (a label or milestone name), so a malicious value containing
// raw terminal control sequences must not leak ESC/BEL bytes into the modal
// while the visible text is retained.
func TestFilterMode_View_SanitizesControlSequencesInFilterItemValue(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))
	b.filterItems[b.filterCursor].value = "\x1b[31mRED\x1b[0m"

	view := b.View()

	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("View() = %q, want no ESC (0x1b) byte", view)
	}
	if strings.ContainsRune(view, '\x07') {
		t.Errorf("View() = %q, want no BEL (0x07) byte", view)
	}
	if !strings.Contains(view, "RED") {
		t.Errorf("View() should still contain visible filter item text %q", "RED")
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

func TestFilterMode_NormalBarExcludesF(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	// The 'f' keybinding remains functional (see other filter mode tests) but
	// should no longer appear in the always-visible normal-mode hint bar.
	foundFilter := false
	for _, hint := range b.normalHints {
		if hint.Key == "f" {
			foundFilter = true
			break
		}
	}
	if foundFilter {
		t.Error("normalHints should NOT include 'f' for filter; it stays available via the '?' Help popup")
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
	if !strings.Contains(content, "toggle") {
		t.Error("buildHelpContent() Filter section should mention 'toggle'")
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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)

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
