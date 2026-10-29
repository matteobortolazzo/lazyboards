package main

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
	"github.com/muesli/termenv"
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
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

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
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

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
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

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
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

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
// no labels or assignees, to verify collectFilterGroups
// accounts for milestones (not just labels/assignees).
func newBoardWithMilestonesOnly(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

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

// newBoardWithMixedCaseFilterItems creates a board whose labels, assignees,
// and milestones are deliberately encountered out of alphabetical order and
// in mixed case, to verify collectFilterGroups sorts each category
// case-insensitively (#477) while tab order (Labels -> Assignees
// -> Milestones) and existing dedup/column-exclusion/empty-milestone-skip
// behavior remain unchanged.
//
// Column "To Do":
//   - Card 1: labels ["zebra", "Apple"], assignees ["zack", "Amy"], milestone "Zeta"
//   - Card 2: labels ["mango"],          assignees ["bob"],        milestone "alpha"
func newBoardWithMixedCaseFilterItems(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "To Do", Cards: []provider.Card{
				{
					Number: 1,
					Title:  "Card One",
					Labels: []provider.Label{{Name: "zebra"}, {Name: "Apple"}},
					Assignees: []provider.Assignee{
						{Login: "zack"},
						{Login: "Amy"},
					},
					Milestone: "Zeta",
				},
				{
					Number:    2,
					Title:     "Card Two",
					Labels:    []provider.Label{{Name: "mango"}},
					Assignees: []provider.Assignee{{Login: "bob"}},
					Milestone: "alpha",
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

// sortFoldExpected sorts a copy of in using the same case-insensitive,
// stable fold collation the production code is expected to apply, so tests
// derive their expected order from the fixture's own data rather than a
// hand-typed literal copied from the implementation.
func sortFoldExpected(in []string) []string {
	want := make([]string, len(in))
	copy(want, in)
	sort.SliceStable(want, func(i, j int) bool {
		return strings.ToLower(want[i]) < strings.ToLower(want[j])
	})
	return want
}

// filterGroupTitles returns the picker's tab titles, in tab order.
func filterGroupTitles(groups []filterGroup) []string {
	titles := make([]string, len(groups))
	for i, g := range groups {
		titles[i] = g.title
	}
	return titles
}

func TestFilterMode_CollectFilterGroups_LabelsSortedAlphabeticallyCaseInsensitive(t *testing.T) {
	b := newBoardWithMixedCaseFilterItems(t)
	items := allFilterRows(b.collectFilterGroups())

	var labels []string
	for _, item := range items {
		if item.itemType == filterByLabel {
			labels = append(labels, item.value)
		}
	}
	if len(labels) == 0 {
		t.Fatal("precondition: expected label items in fixture")
	}

	want := sortFoldExpected(labels)
	for i := range labels {
		if labels[i] != want[i] {
			t.Errorf("label items = %v, want case-insensitive alphabetical order %v", labels, want)
			break
		}
	}
}

func TestFilterMode_CollectFilterGroups_AssigneesSortedAlphabeticallyCaseInsensitive(t *testing.T) {
	b := newBoardWithMixedCaseFilterItems(t)
	items := allFilterRows(b.collectFilterGroups())

	var assignees []string
	for _, item := range items {
		if item.itemType == filterByAssignee {
			assignees = append(assignees, item.value)
		}
	}
	if len(assignees) == 0 {
		t.Fatal("precondition: expected assignee items in fixture")
	}

	want := sortFoldExpected(assignees)
	for i := range assignees {
		if assignees[i] != want[i] {
			t.Errorf("assignee items = %v, want case-insensitive alphabetical order %v", assignees, want)
			break
		}
	}
}

func TestFilterMode_CollectFilterGroups_MilestonesSortedAlphabeticallyCaseInsensitive(t *testing.T) {
	b := newBoardWithMixedCaseFilterItems(t)
	items := allFilterRows(b.collectFilterGroups())

	var milestones []string
	for _, item := range items {
		if item.itemType == filterByMilestone {
			milestones = append(milestones, item.value)
		}
	}
	if len(milestones) == 0 {
		t.Fatal("precondition: expected milestone items in fixture")
	}

	want := sortFoldExpected(milestones)
	for i := range milestones {
		if milestones[i] != want[i] {
			t.Errorf("milestone items = %v, want case-insensitive alphabetical order %v", milestones, want)
			break
		}
	}
}

// TestFilterMode_CollectFilterGroups_MixedCaseFixture_TabOrderUnchanged
// asserts that sorting within categories does not disturb the tab order
// (Labels -> Assignees -> Milestones), using the same mixed-case fixture
// exercised by the per-category sort assertions above.
func TestFilterMode_CollectFilterGroups_MixedCaseFixture_TabOrderUnchanged(t *testing.T) {
	b := newBoardWithMixedCaseFilterItems(t)
	got := filterGroupTitles(b.collectFilterGroups())
	if want := []string{"Labels", "Assignees", "Milestones"}; !slices.Equal(got, want) {
		t.Errorf("tab titles = %v, want %v", got, want)
	}
}

// Every row of a group carries that group's own category.
func TestFilterMode_CollectFilterGroups_RowsMatchTheirTabCategory(t *testing.T) {
	b := newLoadedTestBoard(t)
	groups := b.collectFilterGroups()
	if len(groups) == 0 {
		t.Fatal("precondition: fake board yields no filter groups")
	}
	for _, g := range groups {
		if len(g.items) == 0 {
			t.Errorf("tab %q has no rows, want empty categories omitted", g.title)
		}
		for _, item := range g.items {
			if item.itemType != g.itemType {
				t.Errorf("tab %q holds row %+v of another category", g.title, item)
			}
		}
	}
}

// --- collectFilterGroups tests ---

func TestFilterMode_CollectFilterGroups_LabelsAndAssignees(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	groups := b.collectFilterGroups()

	// Labels: "bug", "feature", "docs" (3 unique, case-insensitive)
	// Assignees: "alice", "bob", "charlie" (3 unique, case-insensitive)
	if got, want := filterGroupTitles(groups), []string{"Labels", "Assignees"}; !slices.Equal(got, want) {
		t.Fatalf("tab titles = %v, want %v", got, want)
	}
	if n := len(groups[0].items); n != 3 {
		t.Errorf("label rows = %d, want 3 (bug, feature, docs deduplicated case-insensitively)", n)
	}
	if n := len(groups[1].items); n != 3 {
		t.Errorf("assignee rows = %d, want 3 (alice, bob, charlie deduplicated case-insensitively)", n)
	}
}

func TestFilterMode_CollectFilterGroups_LabelsDeduplicatedCaseInsensitively(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	items := allFilterRows(b.collectFilterGroups())

	// "bug" and "Bug" should appear only once.
	bugCount := 0
	for _, item := range items {
		if item.itemType == filterByLabel && strings.EqualFold(item.value, "bug") {
			bugCount++
		}
	}
	if bugCount != 1 {
		t.Errorf("bug label count = %d, want 1 (should be deduplicated case-insensitively)", bugCount)
	}
}

func TestFilterMode_CollectFilterGroups_AssigneesDeduplicatedCaseInsensitively(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	items := allFilterRows(b.collectFilterGroups())

	// "alice" and "Alice" should appear only once.
	aliceCount := 0
	for _, item := range items {
		if item.itemType == filterByAssignee && strings.EqualFold(item.value, "alice") {
			aliceCount++
		}
	}
	if aliceCount != 1 {
		t.Errorf("alice assignee count = %d, want 1 (should be deduplicated case-insensitively)", aliceCount)
	}
}

func TestFilterMode_CollectFilterGroups_LabelsOnly_NoAssigneesTab(t *testing.T) {
	b := newBoardWithLabelsOnly(t)
	groups := b.collectFilterGroups()

	if got, want := filterGroupTitles(groups), []string{"Labels"}; !slices.Equal(got, want) {
		t.Errorf("tab titles = %v, want %v (no Assignees tab when no assignees exist)", got, want)
	}
}

func TestFilterMode_CollectFilterGroups_NoLabelsOrAssignees_Empty(t *testing.T) {
	b := newBoardWithNoLabelsOrAssignees(t)
	groups := b.collectFilterGroups()

	if len(groups) != 0 {
		t.Errorf("collectFilterGroups with no labels or assignees = %v, want none", groups)
	}
}

func TestFilterMode_CollectFilterGroups_MilestonesTabAfterAssignees(t *testing.T) {
	b := newBoardWithMilestones(t)
	groups := b.collectFilterGroups()

	titles := filterGroupTitles(groups)
	assigneesIdx := slices.Index(titles, "Assignees")
	milestonesIdx := slices.Index(titles, "Milestones")
	if assigneesIdx == -1 || milestonesIdx == -1 {
		t.Fatalf("tab titles = %v, want both Assignees and Milestones", titles)
	}
	if milestonesIdx < assigneesIdx {
		t.Errorf("Milestones tab at %d, want after Assignees at %d", milestonesIdx, assigneesIdx)
	}
	if n := len(groups[milestonesIdx].items); n != 2 {
		t.Errorf("milestone rows = %d, want 2 (v1.0, To Do deduplicated case-insensitively)", n)
	}
}

func TestFilterMode_CollectFilterGroups_MilestonesDeduplicatedCaseInsensitively(t *testing.T) {
	b := newBoardWithMilestones(t)
	items := allFilterRows(b.collectFilterGroups())

	// "v1.0" and "V1.0" should appear only once.
	v1Count := 0
	for _, item := range items {
		if item.itemType == filterByMilestone && strings.EqualFold(item.value, "v1.0") {
			v1Count++
		}
	}
	if v1Count != 1 {
		t.Errorf("v1.0 milestone count = %d, want 1 (should be deduplicated case-insensitively)", v1Count)
	}
}

func TestFilterMode_CollectFilterGroups_MilestoneEqualsColumnName_NotExcluded(t *testing.T) {
	b := newBoardWithMilestones(t)
	items := allFilterRows(b.collectFilterGroups())

	// Milestone "To Do" matches the "To Do" column title, but milestones are a
	// distinct namespace from board columns (unlike labels) and must still appear.
	found := false
	for _, item := range items {
		if item.itemType == filterByMilestone && strings.EqualFold(item.value, "To Do") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected milestone \"To Do\" in items even though it matches a column title; milestones must not be excluded like labels")
	}
}

func TestFilterMode_CollectFilterGroups_EmptyMilestoneSkipped(t *testing.T) {
	b := newBoardWithMilestones(t)
	items := allFilterRows(b.collectFilterGroups())

	for _, item := range items {
		if item.itemType == filterByMilestone && item.value == "" {
			t.Error("collectFilterGroups should never produce an empty-string milestone item")
		}
	}
}

func TestFilterMode_CollectFilterGroups_NoMilestones_NoMilestonesTab(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	titles := filterGroupTitles(b.collectFilterGroups())

	if slices.Contains(titles, "Milestones") {
		t.Errorf("tab titles = %v, want no Milestones tab when no card has a milestone", titles)
	}
}

func TestFilterMode_CollectFilterGroups_MilestonesOnly(t *testing.T) {
	b := newBoardWithMilestonesOnly(t)
	titles := filterGroupTitles(b.collectFilterGroups())

	if want := []string{"Milestones"}; !slices.Equal(titles, want) {
		t.Errorf("tab titles = %v, want %v when the board has milestones but no labels or assignees", titles, want)
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

func TestFilterMode_FKeyPopulatesFilterGroups(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	if len(b.filterGroups) == 0 {
		t.Error("after pressing 'f': filterGroups is empty, expected populated groups")
	}
}

func TestFilterMode_FKeyOpensFirstTabAtFirstRow(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b.filterTab, b.filterCursor = 1, 2 // stale state from an earlier session

	b = sendKey(t, b, keyMsg("f"))

	if b.filterTab != 0 || b.filterCursor != 0 {
		t.Errorf("after 'f' with no active filter: tab = %d, cursor = %d; want tab 0, cursor 0", b.filterTab, b.filterCursor)
	}
}

func TestFilterMode_FKeyOpensFirstTabHoldingASelection(t *testing.T) {
	b := newBoardWithMilestones(t)
	setActiveFilters(&b,
		filterSelection{itemType: filterByMilestone, value: "v1.0"},
		filterSelection{itemType: filterByAssignee, value: "alice"})

	b = sendKey(t, b, keyMsg("f"))

	if got := b.filterGroups[b.filterTab].itemType; got != filterByAssignee || b.filterCursor != 0 {
		t.Errorf("after 'f': tab category = %d, cursor = %d; want the Assignees tab (first with a selection), cursor 0", got, b.filterCursor)
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

// TestFilterMode_EscapeDoesNotChangeFilter (#653: dropped the old 'f'-to-clear
// step now that 'f' never clears): pre-sets an active filter directly, opens
// the picker (reachable while filters are active), and presses Escape
// without toggling anything -- the selection set must survive unchanged,
// per AC3 ("esc closes the picker, leaving the current selection set
// applied").
func TestFilterMode_EscapeDoesNotChangeFilter(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	setActiveFilter(&b, filterByLabel, "bug")

	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("expected filterMode after 'f', got %d", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Fatalf("expected normalMode after Escape, got %d", b.mode)
	}
	if !hasFilter(&b, filterByLabel, "bug") {
		t.Errorf("after Escape: filters = %+v, want (filterByLabel, %q) still applied (Escape should not change filter)", b.filters, "bug")
	}
}

// TestFilterMode_EnterTogglesItemAndStaysInFilterMode (#653; formerly
// TestFilterMode_EnterSelectsItemAndReturnsToNormalMode): Enter toggles the
// cursor's item into the filter set, re-applies immediately, and leaves the
// modal open with the cursor unchanged (AC2).
func TestFilterMode_EnterTogglesItemAndStaysInFilterMode(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("expected filterMode after 'f', got %d", b.mode)
	}

	cursorBefore := b.filterCursor
	selected := currentFilterRow(t, b)

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != filterMode {
		t.Errorf("after Enter in filterMode: mode = %d, want filterMode (modal stays open)", b.mode)
	}
	if b.filterCursor != cursorBefore {
		t.Errorf("after Enter in filterMode: filterCursor = %d, want unchanged %d", b.filterCursor, cursorBefore)
	}
	if !hasFilter(&b, selected.itemType, selected.value) {
		t.Errorf("after Enter in filterMode: filters = %+v, want a selection for %+v", b.filters, selected)
	}
}

// --- Picker navigation tests ---

func TestFilterMode_JMovesDown(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))
	initialCursor := b.filterCursor

	b = sendKey(t, b, keyMsg("j"))

	if b.filterCursor <= initialCursor {
		t.Errorf("after 'j': filterCursor = %d, want > %d", b.filterCursor, initialCursor)
	}
}

func TestFilterMode_KMovesUp(t *testing.T) {
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

// j/k cycle within the active tab only: no number of presses reaches a row
// of another category or changes the tab.
func TestFilterMode_NavigationStaysWithinActiveTab(t *testing.T) {
	b := newBoardWithMilestones(t)
	b = sendKey(t, b, keyMsg("f"))
	tab := b.filterTab
	itemType := b.filterGroups[tab].itemType

	for _, key := range []string{"j", "k"} {
		for i := 0; i < len(allFilterRows(b.filterGroups))+2; i++ {
			b = sendKey(t, b, keyMsg(key))
			if b.filterTab != tab || currentFilterRow(t, b).itemType != itemType {
				t.Fatalf("after %d x %q: tab = %d, row = %+v; want tab %d rows only", i+1, key, b.filterTab, currentFilterRow(t, b), tab)
			}
		}
	}
}

// --- Wrap-around tests (#426 PR 2) ---
//
// The filter picker's cursor is built on the shared moveCursor wrap
// primitive: moving down from the active tab's last row wraps to its first
// row, and moving up from the first row wraps to the last.

func TestFilterMode_JWrapsFromLastRowToFirstRow(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	rowCount := len(b.visibleFilterItems())
	if rowCount <= 1 {
		t.Fatal("fixture needs more than one row in the first tab to test wraparound")
	}

	for i := 0; i < rowCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.filterCursor != rowCount-1 {
		t.Fatalf("precondition: cursor = %d, want the last row %d", b.filterCursor, rowCount-1)
	}

	b = sendKey(t, b, keyMsg("j"))
	if b.filterCursor != 0 {
		t.Errorf("cursor after 'j' past the last row = %d, want 0 (wrap to first)", b.filterCursor)
	}
}

func TestFilterMode_KWrapsFromFirstRowToLastRow(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	rowCount := len(b.visibleFilterItems())
	if rowCount <= 1 {
		t.Fatal("fixture needs more than one row in the first tab to test wraparound")
	}

	b = sendKey(t, b, keyMsg("k"))
	if b.filterCursor != rowCount-1 {
		t.Errorf("cursor after 'k' before the first row = %d, want %d (wrap to last)", b.filterCursor, rowCount-1)
	}
}

func TestFilterMode_DownArrowWrapsFromLastRowToFirstRow(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	rowCount := len(b.visibleFilterItems())
	if rowCount <= 1 {
		t.Fatal("fixture needs more than one row in the first tab to test wraparound")
	}

	for i := 0; i < rowCount; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyDown))
	}
	if b.filterCursor != 0 {
		t.Errorf("cursor after Down arrow past the last row = %d, want 0 (wrap to first)", b.filterCursor)
	}
}

func TestFilterMode_UpArrowWrapsFromFirstRowToLastRow(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	rowCount := len(b.visibleFilterItems())
	if rowCount <= 1 {
		t.Fatal("fixture needs more than one row in the first tab to test wraparound")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	if b.filterCursor != rowCount-1 {
		t.Errorf("cursor after Up arrow before the first row = %d, want %d (wrap to last)", b.filterCursor, rowCount-1)
	}
}

// TestFilterMode_ZeroSelectableItems_NoOp covers the moveCursor length<=1
// no-op guard for a filter picker forced into filterMode with no groups --
// j/k and the category keys must leave the cursor at 0 and must not panic.
func TestFilterMode_ZeroSelectableItems_NoOp(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b.mode = filterMode
	b.filterGroups = nil
	b.filterCursor = 0

	for _, key := range []string{"j", "k", "l", "h"} {
		b = sendKey(t, b, keyMsg(key))
		if b.filterCursor != 0 || b.filterTab != 0 {
			t.Errorf("after %q with no groups: cursor = %d, tab = %d; want 0, 0 (no-op)", key, b.filterCursor, b.filterTab)
		}
	}
}

// TestFilterMode_SingleSelectableItem_NoOp covers the moveCursor length<=1
// no-op guard for a filter picker with exactly one selectable item.
func TestFilterMode_SingleSelectableItem_NoOp(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

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
	if got := len(board.visibleFilterItems()); got != 1 {
		t.Fatalf("precondition: rows in the active tab = %d, want 1", got)
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

	// Cursor should be on the first label.
	selectedItem := currentFilterRow(t, b)
	if selectedItem.itemType != filterByLabel {
		t.Fatalf("expected first selectable item to be a label, got type %d", selectedItem.itemType)
	}

	expectedValue := selectedItem.value
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if !hasFilter(&b, filterByLabel, expectedValue) {
		t.Errorf("filters = %+v, want a (filterByLabel, %q) selection", b.filters, expectedValue)
	}
}

func TestFilterMode_SelectAssignee_SetsFilterByAssignee(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	b = focusFilterTab(t, b, filterByAssignee)

	selectedItem := currentFilterRow(t, b)
	expectedValue := selectedItem.value
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if !hasFilter(&b, filterByAssignee, expectedValue) {
		t.Errorf("filters = %+v, want a (filterByAssignee, %q) selection", b.filters, expectedValue)
	}
}

func TestFilterMode_SelectMilestone_SetsFilterByMilestone(t *testing.T) {
	b := newBoardWithMilestones(t)

	b = sendKey(t, b, keyMsg("f"))

	b = focusFilterTab(t, b, filterByMilestone)

	selectedItem := currentFilterRow(t, b)
	expectedValue := selectedItem.value
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if !hasFilter(&b, filterByMilestone, expectedValue) {
		t.Errorf("filters = %+v, want a (filterByMilestone, %q) selection", b.filters, expectedValue)
	}
}

// TestFilterMode_SelectMilestone_AccumulatesWithPriorLabelFilter (#653;
// formerly TestFilterMode_SelectMilestone_ClearsPriorLabelFilter): toggling a
// milestone item now accumulates onto a pre-existing selection from a
// different category rather than overwriting it -- the OR-within-category /
// AND-across-categories set semantics (#652) apply to every toggle.
func TestFilterMode_SelectMilestone_AccumulatesWithPriorLabelFilter(t *testing.T) {
	b := newBoardWithMilestones(t)

	// Simulate a pre-existing label filter, as if the user had previously
	// toggled a label before opening the picker again.
	setActiveFilter(&b, filterByLabel, "bug")

	b = sendKey(t, b, keyMsg("f"))
	b = focusFilterTab(t, b, filterByMilestone)
	expectedValue := currentFilterRow(t, b).value

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	// Toggling a filter item in filterMode is a synchronous state change with
	// no side effect to report, so Enter returns a nil cmd.
	if cmd != nil {
		t.Errorf("cmd after toggling a milestone filter item = %v, want nil", cmd)
	}

	if !hasFilter(&board, filterByMilestone, expectedValue) {
		t.Errorf("filters = %+v, want a (filterByMilestone, %q) selection", board.filters, expectedValue)
	}
	if !hasFilter(&board, filterByLabel, "bug") {
		t.Errorf("filters = %+v, want the prior (filterByLabel, %q) selection to remain (toggling accumulates, not replaces)", board.filters, "bug")
	}
	if filterCount(&board) != 2 {
		t.Errorf("filterCount = %d, want 2 (toggling a new category accumulates onto the existing selection)", filterCount(&board))
	}
}

// --- Clear filter tests ---

// TestFilterMode_FReopensPickerWhileFilterActive (#653; replaces
// TestFilterMode_FToggleClearsActiveMilestoneFilter/
// TestFilterMode_FToggleClearsActiveFilter/TestFilterMode_FToggleShowsTimedMessage):
// 'f' never clears (AC1) -- with a filter already active, it opens the
// picker and leaves the selection untouched.
func TestFilterMode_FReopensPickerWhileFilterActive(t *testing.T) {
	b := newBoardWithMilestones(t)
	setActiveFilter(&b, filterByMilestone, "v1.0")

	b = sendKey(t, b, keyMsg("f"))

	if b.mode != filterMode {
		t.Errorf("'f' with an active filter should open the picker, mode = %d, want filterMode", b.mode)
	}
	if !hasFilter(&b, filterByMilestone, "v1.0") {
		t.Errorf("'f' must never clear the filter; filters = %+v, want (filterByMilestone, %q) still applied", b.filters, "v1.0")
	}
}

// TestFilterMode_ClearAllClearsFiltersAndStaysOpen covers the new
// filter.clear_all command (default 'c'): clears every selection,
// re-applies (unfiltered), keeps the modal open, and reports a timed
// success message (AC5).
func TestFilterMode_ClearAllClearsFiltersAndStaysOpen(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	setActiveFilter(&b, filterByLabel, "bug")

	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("expected filterMode after 'f', got %d", b.mode)
	}

	m, cmd := b.Update(keyMsg("c"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if board.mode != filterMode {
		t.Errorf("after 'c' (filter.clear_all): mode = %d, want filterMode (modal stays open)", board.mode)
	}
	if board.hasActiveFilters() {
		t.Errorf("after 'c': hasActiveFilters() = true, want false")
	}
	if cmd == nil {
		t.Error("after 'c': expected a non-nil cmd for the timed 'Filters cleared' message")
	}
	if !strings.Contains(board.statusBar.message, "Filters cleared") {
		t.Errorf("statusBar.message = %q, want to contain %q", board.statusBar.message, "Filters cleared")
	}
}

// TestFilterMode_FOpensPickerWhenNoFilter (#653; formerly
// TestFilterMode_FToggleOpensPickerWhenNoFilter, renamed since 'f' no longer
// toggles).
func TestFilterMode_FOpensPickerWhenNoFilter(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	// Ensure no filter is active.
	if b.hasActiveFilters() {
		t.Fatal("precondition: expected no active filter")
	}

	b = sendKey(t, b, keyMsg("f"))

	if b.mode != filterMode {
		t.Errorf("after 'f' with no active filter: mode = %d, want filterMode", b.mode)
	}
}

// TestFilterMode_FOpensWithEmptyItemsWhenFiltersActive covers Q8's relaxed
// empty-items guard: a board whose own collectFilterGroups() yields no groups
// must still open the picker via 'f' when a filter selected from another
// entry point (e.g. the Milestones modal) is already active -- otherwise
// filter.clear_all would be unreachable.
func TestFilterMode_FOpensWithEmptyItemsWhenFiltersActive(t *testing.T) {
	b := newBoardWithNoLabelsOrAssignees(t)
	if got := b.collectFilterGroups(); len(got) != 0 {
		t.Fatalf("precondition: collectFilterGroups() = %v, want empty", got)
	}
	setActiveFilter(&b, filterByLabel, "some-other-repo-label")

	b = sendKey(t, b, keyMsg("f"))

	if b.mode != filterMode {
		t.Errorf("'f' with an active filter but zero collectFilterGroups() rows should still open the picker, mode = %d, want filterMode", b.mode)
	}
	if view := b.viewFilterModal(); !strings.Contains(view, "No filterable values") {
		t.Errorf("picker with no groups should say it has nothing to filter; got:\n%s", view)
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
	for _, item := range b.visibleFilterItems() {
		if item.itemType == filterByLabel {
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

func TestFilterMode_ViewContainsCategoryTabs(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	view := b.View()

	if !strings.Contains(view, "Labels") {
		t.Error("View() in filterMode should contain the 'Labels' tab")
	}
	if !strings.Contains(view, "Assignees") {
		t.Error("View() in filterMode should contain the 'Assignees' tab")
	}
}

func TestFilterMode_ViewHighlightsSelectedItem(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)

	b = sendKey(t, b, keyMsg("f"))

	// The selected item's value should appear in the view.
	selectedItem := currentFilterRow(t, b)
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
	b.filterGroups[b.filterTab].items[b.filterCursor].value = "\x1b[31mRED\x1b[0m"

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
	if !strings.Contains(view, "Clear all") {
		t.Error("View() in filterMode should contain hint 'Clear all'")
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
// card labels. This verifies that collectFilterGroups excludes labels that match
// column names.
//
// Columns: "To Do", "In Progress"
// Card 1 (in "To Do"):     labels ["bug", "To Do"],       assignees ["alice"]
// Card 2 (in "In Progress"): labels ["In Progress", "feature"], assignees ["bob"]
func newBoardWithColumnNameLabels(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

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

func TestFilterMode_CollectFilterGroups_ExcludesColumnNameLabels(t *testing.T) {
	b := newBoardWithColumnNameLabels(t)
	items := allFilterRows(b.collectFilterGroups())

	// Only "bug" and "feature" should appear as label items.
	// "To Do" and "In Progress" should be excluded because they match column titles.
	for _, item := range items {
		if item.itemType == filterByLabel {
			lower := strings.ToLower(item.value)
			if lower == "to do" || lower == "in progress" {
				t.Errorf("label %q should be excluded because it matches a column name", item.value)
			}
		}
	}

	labelCount := 0
	for _, item := range items {
		if item.itemType == filterByLabel {
			labelCount++
		}
	}
	if labelCount != 2 {
		t.Errorf("label items = %d, want 2 (bug, feature); column-name labels should be excluded", labelCount)
	}
}

func TestFilterMode_CollectFilterGroups_ExcludesColumnNamesCaseInsensitive(t *testing.T) {
	// Create a board where card labels use different casing than column titles.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

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

	items := allFilterRows(board.collectFilterGroups())

	// "to do" (lowercase) and "IN PROGRESS" (uppercase) should still be excluded
	// because column name matching is case-insensitive.
	for _, item := range items {
		if item.itemType == filterByLabel {
			lower := strings.ToLower(item.value)
			if lower == "to do" || lower == "in progress" {
				t.Errorf("label %q should be excluded (case-insensitive match to column name)", item.value)
			}
		}
	}

	labelCount := 0
	for _, item := range items {
		if item.itemType == filterByLabel {
			labelCount++
		}
	}
	if labelCount != 2 {
		t.Errorf("label items = %d, want 2 (bug, feature); case-variant column-name labels should be excluded", labelCount)
	}
}

func TestFilterMode_CollectFilterGroups_AllLabelsAreColumnNames_OmitsLabelsTab(t *testing.T) {
	// Board where every label is a column name — no labels should remain after exclusion.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

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

	// No Labels tab since all labels are column names; Assignees remains.
	if got, want := filterGroupTitles(board.collectFilterGroups()), []string{"Assignees"}; !slices.Equal(got, want) {
		t.Errorf("tab titles = %v, want %v when all labels match column names", got, want)
	}
}

// --- Multi-select '*' marker (#653) ---

// TestFilterMode_View_ToggledItemShowsAsteriskMarker asserts a toggled-on
// picker row carries the "* " prefix, mirroring viewAssignModal's existing
// convention (AC4).
func TestFilterMode_View_ToggledItemShowsAsteriskMarker(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	selectedItem := currentFilterRow(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyEnter)) // toggle on; picker stays open

	view := b.viewFilterModal()
	var markedLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, selectedItem.value) {
			markedLine = line
			break
		}
	}
	if markedLine == "" {
		t.Fatalf("view missing selected item row; got:\n%s", view)
	}
	if !strings.Contains(markedLine, "*") {
		t.Errorf("row for a toggled-on filter item = %q, want it to carry a '*' marker", markedLine)
	}
}

// TestFilterMode_View_UntoggledItemShowsNoAsteriskMarker asserts an
// untoggled row keeps the plain two-space prefix, with no '*' marker.
func TestFilterMode_View_UntoggledItemShowsNoAsteriskMarker(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	items := b.visibleFilterItems()
	if len(items) == 0 {
		t.Fatal("fixture needs at least one selectable filter item")
	}
	otherItem := items[0]

	view := b.viewFilterModal()
	var line string
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, otherItem.value) {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("view missing filter item row; got:\n%s", view)
	}
	if strings.Contains(line, "*") {
		t.Errorf("row for an untoggled filter item = %q, want no '*' marker", line)
	}
}

// TestFilterMode_View_AsteriskMarkerSurvivesCursorHighlight asserts the '*'
// marker still renders on a row that is both toggled-on AND under the
// cursor -- the marker must survive selectedRowStyle, mirroring
// viewAssignModal's precedent (AC4).
func TestFilterMode_View_AsteriskMarkerSurvivesCursorHighlight(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	selectedItem := currentFilterRow(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if currentFilterRow(t, b).value != selectedItem.value {
		t.Fatalf("precondition: cursor should remain on the toggled item")
	}

	view := b.viewFilterModal()
	var line string
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, selectedItem.value) {
			line = l
			break
		}
	}
	if line == "" {
		t.Fatalf("view missing cursor-selected row; got:\n%s", view)
	}
	if !strings.Contains(line, "*") {
		t.Errorf("cursor-highlighted + toggled-on row = %q, want it to still carry the '*' marker", line)
	}
}

// --- Mute non-selected rows to gray (#478) ---

// TestFilterMode_View_NonSelectedRowGray_SelectedRowBoldWhite asserts the
// filter picker's selected row stays bold-white (selectedCardStyle, the
// existing convention) while a non-selected row mutes to gray instead of
// rendering as bare unstyled text.
func TestFilterMode_View_NonSelectedRowGray_SelectedRowBoldWhite(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	selectedItem := currentFilterRow(t, b)
	items := b.visibleFilterItems()
	if len(items) < 2 {
		t.Fatal("fixture needs at least two selectable filter items")
	}
	otherItem := items[(b.filterCursor+1)%len(items)]

	view := b.viewFilterModal()
	var titleLine, selectedLine, nonSelectedLine string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Filter") {
			titleLine = line
		}
		if strings.Contains(line, selectedItem.value) {
			selectedLine = line
		}
		if strings.Contains(line, otherItem.value) {
			nonSelectedLine = line
		}
	}
	if titleLine == "" || selectedLine == "" || nonSelectedLine == "" {
		t.Fatalf("view missing expected filter rows; got:\n%s", view)
	}

	assertMutedRowStyle(t, "filter", titleLine, selectedLine, nonSelectedLine)
}

// TestFilterMode_View_NewlineInItemValue_RendersOneRow covers the filter
// picker's row render path (#500): a label/assignee/milestone value is
// untrusted GitHub content, so an embedded newline must not spill the row
// onto a second physical line.
func TestFilterMode_View_NewlineInItemValue_RendersOneRow(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.filterGroups = []filterGroup{{itemType: filterByLabel, title: "Labels", items: []filterItem{
		{itemType: filterByLabel, value: "line one\nline two"},
	}}}

	view := b.viewFilterModal()
	assertOneRow(t, view, "line one", "line two")
}

func TestFilterMode_CollectFilterGroups_AssigneesUnaffectedByColumnExclusion(t *testing.T) {
	b := newBoardWithColumnNameLabels(t)
	items := allFilterRows(b.collectFilterGroups())

	// Assignees should be unaffected by column-name label exclusion.
	// The board has "alice" and "bob" as assignees.
	assigneeCount := 0
	for _, item := range items {
		if item.itemType == filterByAssignee {
			assigneeCount++
		}
	}
	if assigneeCount != 2 {
		t.Errorf("assignee items = %d, want 2 (alice, bob); assignees should not be affected by column-name exclusion", assigneeCount)
	}
}

// filterLayout renders groups as "Title|Title|..." with itemType's tab
// followed by its row values, e.g. "Labels|Hierarchy|Parents".
func filterLayout(groups []filterGroup, itemType filterType) string {
	var parts []string
	for _, g := range groups {
		parts = append(parts, g.title)
		if g.itemType == itemType {
			for _, item := range g.items {
				parts = append(parts, item.value)
			}
		}
	}
	return strings.Join(parts, "|")
}

// hierarchyBoard: Backlog #1 parent, #2 sub-issue (unloaded parent, label bug),
// #3 parent and sub-issue, #4 standalone, #5 sub-issue labeled "Parents";
// In Progress #6 parent, #7 standalone.
func hierarchyBoard(t *testing.T) Board {
	t.Helper()
	b := NewBoard(provider.NewFakeProvider(), nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)
	m, cmd := b.Update(boardFetchedMsg{board: provider.Board{Columns: []provider.Column{
		{Title: "Backlog", Cards: []provider.Card{
			{Number: 1, Title: "Epic", SubIssueCount: 2},
			{Number: 2, Title: "Orphan", ParentNumber: 99, Labels: []provider.Label{{Name: "bug"}}},
			{Number: 3, Title: "Middle", SubIssueCount: 1, ParentNumber: 1},
			{Number: 4, Title: "Standalone"},
			{Number: 5, Title: "Task", ParentNumber: 1, Labels: []provider.Label{{Name: "Parents"}}},
		}},
		{Title: "In Progress", Cards: []provider.Card{
			{Number: 6, Title: "Other epic", SubIssueCount: 3, SubIssueCompleted: 3},
			{Number: 7, Title: "Plain"},
		}},
	}}})
	execCmds(cmd)
	board := m.(Board)
	board.Width, board.Height = 120, 40
	return board
}

// openFilterPickerOn opens the picker (if closed) and parks the cursor on the row.
func openFilterPickerOn(t *testing.T, b Board, itemType filterType, value string) Board {
	t.Helper()
	if b.mode != filterMode {
		b = sendKey(t, b, keyMsg("f"))
	}
	return focusFilterRow(t, b, itemType, value)
}

func TestFilterMode_CollectFilterGroups_HierarchySection(t *testing.T) {
	parent := provider.Card{Number: 1, Title: "P", SubIssueCount: 1, Milestone: "v1", Labels: []provider.Label{{Name: "bug"}}}
	child := provider.Card{Number: 2, Title: "C", ParentNumber: 1}
	plain := provider.Card{Number: 3, Title: "S", Labels: []provider.Label{{Name: "bug"}}}
	cases := []struct {
		name  string
		cards []provider.Card
		want  string
	}{
		{"parents only", []provider.Card{parent}, "Labels|Milestones|Hierarchy|Parents"},
		{"sub-issues only", []provider.Card{child, plain}, "Labels|Hierarchy|Sub-issues"},
		{"both, fixed order, last", []provider.Card{child, parent}, "Labels|Milestones|Hierarchy|Parents|Sub-issues|Leaves"},
		{"neither omits the tab", []provider.Card{plain}, "Labels"},
		{"parent beside a standalone card adds Leaves", []provider.Card{parent, plain}, "Labels|Milestones|Hierarchy|Parents|Leaves"},
		{"every card a leaf omits Leaves", []provider.Card{child, plain}, "Labels|Hierarchy|Sub-issues"},
		{"no card a leaf omits Leaves", []provider.Card{parent, {Number: 4, Title: "P2", SubIssueCount: 3, SubIssueCompleted: 2}}, "Labels|Milestones|Hierarchy|Parents"},
		{"an all-closed parent is a leaf", []provider.Card{parent, {Number: 4, Title: "Done", SubIssueCount: 2, SubIssueCompleted: 2}}, "Labels|Milestones|Hierarchy|Parents|Leaves"},
	}
	for _, tc := range cases {
		b := newBoardWithInlineCards(t, tc.cards, 120, 40)
		if got := filterLayout(b.collectFilterGroups(), filterByHierarchy); got != tc.want {
			t.Errorf("%s: tabs/rows = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestFilterMode_CollectFilterGroups_HierarchyRowsListedAcrossColumns(t *testing.T) {
	b := hierarchyBoard(t)
	b.ActiveTab = 1
	var got []string
	for _, item := range allFilterRows(b.collectFilterGroups()) {
		if item.itemType == filterByHierarchy {
			got = append(got, item.value)
		}
	}
	if want := hierarchyParentsValue + "|" + hierarchySubIssuesValue + "|" + hierarchyLeavesValue; strings.Join(got, "|") != want {
		t.Errorf("hierarchy rows = %v, want %s (sub-issues live only in another column)", got, want)
	}
}

func TestFilterMode_HierarchyRow_TogglesInPlaceWithGlyphMarkerAndClearAll(t *testing.T) {
	parentsRow := subIssueParentGlyph + " " + hierarchyParentsValue
	subsRow := subIssueChildGlyph + " " + hierarchySubIssuesValue
	b := openFilterPickerOn(t, hierarchyBoard(t), filterByHierarchy, hierarchyParentsValue)
	cursor := b.filterCursor

	view := b.viewFilterModal()
	if !strings.Contains(view, parentsRow) || strings.Index(view, parentsRow) > strings.Index(view, subsRow) {
		t.Fatalf("want glyph-prefixed Parents row before Sub-issues row; got:\n%s", view)
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	execCmds(cmd)
	b = m.(Board)
	if b.mode != filterMode || b.filterCursor != cursor {
		t.Errorf("after enter: mode=%d cursor=%d, want filterMode and cursor %d", b.mode, b.filterCursor, cursor)
	}
	if !b.filters.contains(filterByHierarchy, hierarchyParentsValue) || filterCount(&b) != 1 || b.totalFilteredCards() != 3 {
		t.Errorf("filters = %+v, total = %d, want only Parents and 3 parent cards", b.filters, b.totalFilteredCards())
	}
	view = b.viewFilterModal()
	if !strings.Contains(findLineContaining(t, view, parentsRow), "* "+parentsRow) {
		t.Errorf("selected Parents row want '* ' before the glyph; got:\n%s", view)
	}
	if seg := b.statusBar.filterStatus; !strings.Contains(seg, hierarchyParentsValue) || strings.Contains(seg, subIssueParentGlyph) {
		t.Errorf("status-bar segment = %q, want the plain label %q without the card glyph", seg, hierarchyParentsValue)
	}

	setActiveFilters(&b,
		filterSelection{itemType: filterByHierarchy, value: hierarchySubIssuesValue},
		filterSelection{itemType: filterByLabel, value: "bug"})
	m, cmd = b.Update(keyMsg("c"))
	b = m.(Board)
	if b.hasActiveFilters() || b.mode != filterMode || cmd == nil {
		t.Errorf("after 'c': filters = %+v, mode = %d, cmd nil = %v; want cleared, modal open, message cmd", b.filters, b.mode, cmd == nil)
	}
}

func TestFilterMode_FKeyOpensPickerWhenOnlyHierarchyRowsExist(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "P", SubIssueCount: 1},
		{Number: 2, Title: "C", ParentNumber: 1},
	}, 120, 40)

	m, cmd := b.Update(keyMsg("f"))
	execCmds(cmd)
	board := m.(Board)
	if got, want := filterLayout(board.filterGroups, filterByHierarchy), "Hierarchy|Parents|Sub-issues|Leaves"; board.mode != filterMode || got != want {
		t.Errorf("mode = %d, tabs/rows = %s; want filterMode with %s", board.mode, got, want)
	}
}

func TestFilterMode_LeavesRow_RendersPlainAndTogglesInPlace(t *testing.T) {
	b := openFilterPickerOn(t, hierarchyBoard(t), filterByHierarchy, hierarchyLeavesValue)
	cursor := b.filterCursor

	view := b.viewFilterModal()
	leavesLine := findLineContaining(t, view, hierarchyLeavesValue)
	if strings.Contains(leavesLine, subIssueParentGlyph) || strings.Contains(leavesLine, subIssueChildGlyph) || strings.Contains(leavesLine, blockedByGlyph) {
		t.Errorf("Leaves row = %q, want a plain row without any card glyph", leavesLine)
	}
	if strings.Index(view, hierarchySubIssuesValue) > strings.Index(view, hierarchyLeavesValue) {
		t.Errorf("want the Leaves row after the Sub-issues row; got:\n%s", view)
	}

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	execCmds(cmd)
	b = m.(Board)
	if b.mode != filterMode || b.filterCursor != cursor {
		t.Errorf("after enter: mode=%d cursor=%d, want filterMode and cursor %d", b.mode, b.filterCursor, cursor)
	}
	if !b.filters.contains(filterByHierarchy, hierarchyLeavesValue) || filterCount(&b) != 1 {
		t.Errorf("filters = %+v, want only the Leaves selection", b.filters)
	}
	// Of hierarchyBoard's 7 cards only #1 and #3 still have open sub-issues;
	// #6's are all closed, so it counts as a leaf.
	if got, want := b.totalFilteredCards(), 5; got != want {
		t.Errorf("total = %d, want %d leaf cards", got, want)
	}
	if !strings.Contains(findLineContaining(t, b.viewFilterModal(), hierarchyLeavesValue), "* ") {
		t.Errorf("selected Leaves row want a '* ' marker")
	}
	if seg := b.statusBar.filterStatus; !strings.Contains(seg, filterGlyph+" "+hierarchyLeavesValue) {
		t.Errorf("status-bar segment = %q, want the plain %q label", seg, hierarchyLeavesValue)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if visible := visibleCardNumbers(b); b.hasActiveFilters() || len(visible) != 7 {
		t.Errorf("second enter: filters = %+v, visible = %v, want the toggle undone and all 7 cards", b.filters, visible)
	}
}

func TestFilterMode_AllLeavesBoard_PickerStaysClosed(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "A"},
		{Number: 2, Title: "B"},
	}, 120, 40)

	m, cmd := b.Update(keyMsg("f"))
	execCmds(cmd)
	board := m.(Board)
	if got := filterLayout(board.collectFilterGroups(), filterByHierarchy); board.mode == filterMode {
		t.Errorf("mode = filterMode with tabs/rows %s, want the picker closed: a Leaves row matching every card filters nothing", got)
	}
}

func TestFilterMode_LabelNamedParents_IsIndependentOfHierarchyParentsRow(t *testing.T) {
	b := openFilterPickerOn(t, hierarchyBoard(t), filterByLabel, "Parents")
	hierarchyLine := subIssueParentGlyph + " " + hierarchyParentsValue

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if !b.filters.contains(filterByLabel, "Parents") || b.filters.contains(filterByHierarchy, hierarchyParentsValue) {
		t.Fatalf("toggling the label row: filters = %+v, want only the label selection", b.filters)
	}
	b = openFilterPickerOn(t, b, filterByHierarchy, hierarchyParentsValue)
	if strings.Contains(findLineContaining(t, b.viewFilterModal(), hierarchyLine), "*") {
		t.Errorf("Hierarchy Parents row must not be marked by a label named Parents")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if !b.filters.contains(filterByLabel, "Parents") || !b.filters.contains(filterByHierarchy, hierarchyParentsValue) || filterCount(&b) != 2 {
		t.Errorf("filters = %+v, want label and Hierarchy selections as separate entries", b.filters)
	}
}

func TestFilterMode_CollectFilterGroups_DependenciesSection(t *testing.T) {
	blocked := provider.Card{Number: 1, Title: "B", BlockedByCount: 1, SubIssueCount: 1}
	closedOnly := provider.Card{Number: 2, Title: "C", BlockedByCount: 0, TotalBlockedByCount: 2}
	plain := provider.Card{Number: 3, Title: "S", Labels: []provider.Label{{Name: "bug"}}}
	cases := []struct {
		name  string
		cards []provider.Card
		want  string
	}{
		{"blocked only", []provider.Card{blocked}, "Hierarchy|Dependencies|Blocked"},
		{"both, fixed order, last", []provider.Card{closedOnly, blocked}, "Hierarchy|Dependencies|Blocked|Unblocked"},
		{"nothing blocked omits the section", []provider.Card{plain, closedOnly}, "Labels"},
	}
	for _, tc := range cases {
		b := newBoardWithInlineCards(t, tc.cards, 120, 40)
		if got := filterLayout(b.collectFilterGroups(), filterByDependency); got != tc.want {
			t.Errorf("%s: tabs/rows = %s, want %s", tc.name, got, tc.want)
		}
	}
}

func TestFilterMode_NoBlockedCard_PickerStaysClosedWithoutOtherValues(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "A"},
		{Number: 2, Title: "B", TotalBlockedByCount: 1},
	}, 120, 40)

	m, cmd := b.Update(keyMsg("f"))
	execCmds(cmd)
	if board := m.(Board); board.mode == filterMode {
		t.Errorf("mode = filterMode with items %+v, want the picker closed: an Unblocked-only row filters nothing", board.filterGroups)
	}
}

func TestFilterMode_BlockedRow_TogglesWithGlyphAndPlainSegment(t *testing.T) {
	blockedRow := blockedByGlyph + " " + dependencyBlockedValue
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "A", BlockedByCount: 2},
		{Number: 2, Title: "B"},
	}, 120, 40)
	b = openFilterPickerOn(t, b, filterByDependency, dependencyBlockedValue)

	view := b.viewFilterModal()
	unblockedLine := findLineContaining(t, view, dependencyUnblockedValue)
	if !strings.Contains(view, blockedRow) || strings.Contains(unblockedLine, blockedByGlyph) {
		t.Fatalf("want a glyph-prefixed Blocked row and a plain Unblocked row; got:\n%s", view)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.mode != filterMode || !b.filters.contains(filterByDependency, dependencyBlockedValue) || b.totalFilteredCards() != 1 {
		t.Errorf("after enter: mode = %d, filters = %+v, total = %d; want picker open, Blocked active, 1 card", b.mode, b.filters, b.totalFilteredCards())
	}
	if !strings.Contains(findLineContaining(t, b.viewFilterModal(), blockedRow), "* "+blockedRow) {
		t.Errorf("selected Blocked row want '* ' before the glyph")
	}
	if seg := b.statusBar.filterStatus; !strings.Contains(seg, dependencyBlockedValue) || strings.Contains(seg, blockedByGlyph) {
		t.Errorf("status-bar segment = %q, want the plain label %q without the card glyph", seg, dependencyBlockedValue)
	}
}

// --- Category tabs ---

// fiveCategoryCards carries a value for every filter category.
func fiveCategoryCards() []provider.Card {
	return []provider.Card{
		{Number: 1, Title: "A", Labels: []provider.Label{{Name: "bug"}, {Name: "ui"}}, Assignees: []provider.Assignee{{Login: "alice"}}, Milestone: "v1", SubIssueCount: 1},
		{Number: 2, Title: "B", ParentNumber: 1, BlockedByCount: 1},
		{Number: 3, Title: "C", Labels: []provider.Label{{Name: "docs"}}},
	}
}

func TestFilterMode_CategoryKeys_SwitchTabsWrapAndResetCursor(t *testing.T) {
	b := newBoardWithInlineCards(t, fiveCategoryCards(), 120, 40)
	b = sendKey(t, b, keyMsg("f"))
	last := len(b.filterGroups) - 1
	if last < 1 {
		t.Fatalf("precondition: want several tabs, got %v", filterGroupTitles(b.filterGroups))
	}

	next := []tea.KeyMsg{keyMsg("l"), arrowMsg(tea.KeyRight), arrowMsg(tea.KeyTab)}
	prev := []tea.KeyMsg{keyMsg("h"), arrowMsg(tea.KeyLeft), arrowMsg(tea.KeyShiftTab)}
	for i := range next {
		b.filterTab, b.filterCursor = 0, 1
		b = sendKey(t, b, next[i])
		if b.filterTab != 1 || b.filterCursor != 0 {
			t.Errorf("%q from tab 0: tab = %d, cursor = %d; want tab 1, cursor 0", next[i].String(), b.filterTab, b.filterCursor)
		}
		b.filterTab = last
		b = sendKey(t, b, next[i])
		if b.filterTab != 0 {
			t.Errorf("%q from the last tab: tab = %d, want 0 (wrap)", next[i].String(), b.filterTab)
		}
		b = sendKey(t, b, prev[i])
		if b.filterTab != last || b.filterCursor != 0 {
			t.Errorf("%q from tab 0: tab = %d, cursor = %d; want the last tab %d (wrap), cursor 0", prev[i].String(), b.filterTab, b.filterCursor, last)
		}
	}
	if b.mode != filterMode {
		t.Errorf("mode = %d after switching tabs, want filterMode", b.mode)
	}
}

func TestFilterMode_EnterTogglesTheActiveTabsRow(t *testing.T) {
	b := newBoardWithInlineCards(t, fiveCategoryCards(), 120, 40)
	b = sendKey(t, b, keyMsg("f"))
	b = focusFilterTab(t, b, filterByMilestone)

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if !hasFilter(&b, filterByMilestone, "v1") || filterCount(&b) != 1 {
		t.Errorf("filters = %+v, want only the Milestones tab's row v1", b.filters)
	}
}

func TestFilterMode_View_TabRowShowsSelectedCountBadges(t *testing.T) {
	b := newBoardWithInlineCards(t, fiveCategoryCards(), 120, 40)
	setActiveFilters(&b,
		filterSelection{itemType: filterByLabel, value: "bug"},
		filterSelection{itemType: filterByLabel, value: "ui"},
		filterSelection{itemType: filterByDependency, value: dependencyBlockedValue})
	b = sendKey(t, b, keyMsg("f"))

	view := b.viewFilterModal()
	for _, want := range []string{"Labels (2)", "Dependencies (1)", "Assignees", "Milestones", "Hierarchy"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing tab %q; got:\n%s", want, view)
		}
	}
	for _, unwanted := range []string{"Assignees (", "Milestones (", "Hierarchy ("} {
		if strings.Contains(view, unwanted) {
			t.Errorf("view shows a badge %q for a tab with no selection", unwanted)
		}
	}
}

func TestFilterMode_View_TabRowWrapsWithinModalWidth(t *testing.T) {
	b := newBoardWithInlineCards(t, fiveCategoryCards(), 120, 40)
	setActiveFilters(&b,
		filterSelection{itemType: filterByLabel, value: "bug"},
		filterSelection{itemType: filterByMilestone, value: "v1"})
	b = sendKey(t, b, keyMsg("f"))
	width := modalContentWidth(62)

	lines := b.filterTabLines(width)

	joined := strings.Join(lines, "\n")
	for _, title := range filterGroupTitles(b.filterGroups) {
		if !strings.Contains(joined, title) {
			t.Errorf("tab lines %q missing %q", joined, title)
		}
	}
	if len(lines) < 2 {
		t.Errorf("tab lines = %q, want the five tabs wrapped onto more than one line at width %d", lines, width)
	}
	for _, line := range lines {
		if w := lipgloss.Width(line); w > width {
			t.Errorf("tab line %q is %d cells, want <= %d", line, w, width)
		}
	}
}

// --- Scrolling ---

func manyLabelsBoard(t *testing.T, count, height int) Board {
	t.Helper()
	cards := make([]provider.Card, count)
	for i := range cards {
		cards[i] = provider.Card{Number: i + 1, Title: "Card", Labels: []provider.Label{{Name: fmt.Sprintf("label-%02d", i)}}}
	}
	return newBoardWithInlineCards(t, cards, 120, height)
}

func TestFilterMode_View_LongListScrollsWithinTerminalHeight(t *testing.T) {
	const height = 20
	b := manyLabelsBoard(t, 40, height)
	b = sendKey(t, b, keyMsg("f"))
	rows := b.visibleFilterItems()

	view := b.viewFilterModal()
	if !strings.Contains(view, "▼") || strings.Contains(view, "▲") || !strings.Contains(view, rows[0].value) {
		t.Errorf("at the top: want ▼ only and the first row visible; got:\n%s", view)
	}
	if h := lipgloss.Height(view); h > height {
		t.Errorf("modal height = %d at the top, want <= %d", h, height)
	}

	b = sendKey(t, b, keyMsg("k")) // wraps to the last row
	lastRow := rows[len(rows)-1].value
	view = b.viewFilterModal()
	if !strings.Contains(view, "▲") || strings.Contains(view, "▼") {
		t.Errorf("at the bottom: want ▲ only; got:\n%s", view)
	}
	if !strings.Contains(view, lastRow) || strings.Contains(view, rows[0].value) {
		t.Errorf("at the bottom: want the selected last row %q visible and the first row scrolled away; got:\n%s", lastRow, view)
	}
	if h := lipgloss.Height(view); h > height {
		t.Errorf("modal height = %d at the bottom, want <= %d", h, height)
	}
}

func TestFilterMode_View_ShortListRendersWithoutScrollIndicators(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = sendKey(t, b, keyMsg("f"))

	view := b.viewFilterModal()
	if strings.Contains(view, "▲") || strings.Contains(view, "▼") {
		t.Errorf("short list must not show scroll indicators; got:\n%s", view)
	}
	for _, item := range b.visibleFilterItems() {
		if !strings.Contains(view, item.value) {
			t.Errorf("view missing row %q", item.value)
		}
	}
}

// --- Refresh while open ---

func TestFilterMode_RefreshWhileOpen_KeepsTabAndClampsCursor(t *testing.T) {
	b := newBoardWithInlineCards(t, fiveCategoryCards(), 120, 40)
	b = sendKey(t, b, keyMsg("f"))
	b = focusFilterTab(t, b, filterByLabel)
	b = sendKey(t, b, keyMsg("k")) // last label row

	b = simulateRefreshWithCards(t, b, []provider.Column{{Title: "Working", Cards: []provider.Card{
		{Number: 1, Title: "A", Assignees: []provider.Assignee{{Login: "alice"}}, Labels: []provider.Label{{Name: "bug"}}},
	}}})

	if got := b.filterGroups[b.filterTab].itemType; got != filterByLabel || b.filterCursor != 0 {
		t.Errorf("after refresh: tab category = %d, cursor = %d; want the Labels tab kept and the cursor clamped to 0", got, b.filterCursor)
	}
	if b.mode != filterMode {
		t.Errorf("mode = %d after refresh, want filterMode", b.mode)
	}
}

func TestFilterMode_RefreshWhileOpen_VanishedTabFallsBackToFirst(t *testing.T) {
	b := newBoardWithInlineCards(t, fiveCategoryCards(), 120, 40)
	b = sendKey(t, b, keyMsg("f"))
	b = focusFilterTab(t, b, filterByMilestone)

	b = simulateRefreshWithCards(t, b, []provider.Column{{Title: "Working", Cards: []provider.Card{
		{Number: 1, Title: "A", Labels: []provider.Label{{Name: "bug"}}},
	}}})

	if b.filterTab != 0 || b.filterCursor != 0 || len(b.visibleFilterItems()) == 0 {
		t.Errorf("after the Milestones tab vanished: tab = %d, cursor = %d, rows = %v; want tab 0, cursor 0", b.filterTab, b.filterCursor, b.visibleFilterItems())
	}
}
