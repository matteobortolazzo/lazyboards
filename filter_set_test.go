package main

import (
	"slices"
	"testing"
)

// --- filterSet.matches / cardMatchesSelection truth table (#652) ---
//
// These are pure unit tests over the new matching algebra itself: OR within
// a category, AND across categories, case-insensitive comparison at every
// comparison site, the empty-milestone early return, an empty-value
// selection matching nothing, and the empty-set/none-category degenerate
// cases. Per the plan's Test Strategy, an integration test through
// filteredCards cannot cheaply enumerate this combinatorial matrix (and
// can't reach a multi-selection set at all until #653 ships the toggle UI).
//
// None of filterSet.matches, cardMatchesSelection, or Board.hasActiveFilters
// exist in production yet (they land in model.go during #652's GREEN
// phase) -- this file, together with helpers_test.go's re-pointed
// setActiveFilter/new setActiveFilters/hasFilter/filterCount helpers, is
// expected to fail to compile until then.

// cardFixture builds a minimal Card for matching-algebra tests: a nil/empty
// slice argument means "no labels"/"no assignees" (not a single empty-name
// entry), matching how a real fetched card looks when it has none.
func cardFixture(number int, labels []string, assignees []string, milestone string) Card {
	var ls []Label
	for _, l := range labels {
		ls = append(ls, Label{Name: l})
	}
	var as []Assignee
	for _, a := range assignees {
		as = append(as, Assignee{Login: a})
	}
	return Card{Number: number, Labels: ls, Assignees: as, Milestone: milestone}
}

func TestFilterSet_EmptySet_MatchesEverything(t *testing.T) {
	var fs filterSet // nil set -- no selections at all

	populated := cardFixture(1, []string{"bug"}, []string{"alice"}, "v1.0")
	if !fs.matches(populated) {
		t.Error("empty filterSet should match a fully-populated card")
	}

	blank := cardFixture(2, nil, nil, "")
	if !fs.matches(blank) {
		t.Error("empty filterSet should match a card with no labels/assignees/milestone too")
	}
}

func TestFilterSet_OrWithinCategory_Label(t *testing.T) {
	fs := filterSet{
		{itemType: filterByLabel, value: "bug"},
		{itemType: filterByLabel, value: "feature"},
	}
	bugCard := cardFixture(1, []string{"bug"}, nil, "")
	featureCard := cardFixture(2, []string{"feature"}, nil, "")
	docsCard := cardFixture(3, []string{"docs"}, nil, "")

	if !fs.matches(bugCard) {
		t.Error("OR-within-category: 'bug' selection should match a card labeled 'bug'")
	}
	if !fs.matches(featureCard) {
		t.Error("OR-within-category: 'feature' selection should match a card labeled 'feature'")
	}
	if fs.matches(docsCard) {
		t.Error("OR-within-category: a card matching neither selected label should not match")
	}
}

func TestFilterSet_AndAcrossCategories(t *testing.T) {
	fs := filterSet{
		{itemType: filterByLabel, value: "bug"},
		{itemType: filterByAssignee, value: "alice"},
	}
	both := cardFixture(1, []string{"bug"}, []string{"alice"}, "")
	labelOnly := cardFixture(2, []string{"bug"}, []string{"bob"}, "")
	assigneeOnly := cardFixture(3, []string{"feature"}, []string{"alice"}, "")
	neither := cardFixture(4, []string{"docs"}, []string{"bob"}, "")

	if !fs.matches(both) {
		t.Error("AND-across-categories: a card matching both the label and assignee selections should match")
	}
	if fs.matches(labelOnly) {
		t.Error("AND-across-categories: a card matching only the label selection should not match")
	}
	if fs.matches(assigneeOnly) {
		t.Error("AND-across-categories: a card matching only the assignee selection should not match")
	}
	if fs.matches(neither) {
		t.Error("AND-across-categories: a card matching neither selection should not match")
	}
}

func TestFilterSet_CaseInsensitive_AtEveryComparison(t *testing.T) {
	cases := []struct {
		name string
		sel  filterSelection
		card Card
	}{
		{"label", filterSelection{itemType: filterByLabel, value: "bug"}, cardFixture(1, []string{"Bug"}, nil, "")},
		{"assignee", filterSelection{itemType: filterByAssignee, value: "alice"}, cardFixture(2, nil, []string{"Alice"}, "")},
		{"milestone", filterSelection{itemType: filterByMilestone, value: "v1.0"}, cardFixture(3, nil, nil, "V1.0")},
	}
	for _, tc := range cases {
		fs := filterSet{tc.sel}
		if !fs.matches(tc.card) {
			t.Errorf("%s: case-insensitive match should succeed (selection %q vs stored value with different case)", tc.name, tc.sel.value)
		}
	}
}

func TestFilterSet_MilestoneSelection_EmptyCardMilestoneNeverMatches(t *testing.T) {
	// A card with no milestone must never match a milestone selection --
	// preserving today's card.Milestone == "" early return in
	// matchesGlobalFilter.
	noMilestone := cardFixture(1, nil, nil, "")
	sel := filterSelection{itemType: filterByMilestone, value: "v1.0"}
	if cardMatchesSelection(noMilestone, sel) {
		t.Error("a card with no milestone should never match a non-empty milestone selection")
	}
}

func TestFilterSet_EmptyValueSelection_MatchesNothing(t *testing.T) {
	// An empty-value selection stores verbatim (Q5) and must match nothing
	// -- not even a card whose own field is also empty.
	cases := []struct {
		name string
		sel  filterSelection
		card Card
	}{
		{"label", filterSelection{itemType: filterByLabel, value: ""}, cardFixture(1, []string{""}, nil, "")},
		{"assignee", filterSelection{itemType: filterByAssignee, value: ""}, cardFixture(2, nil, []string{""}, "")},
		{"milestone", filterSelection{itemType: filterByMilestone, value: ""}, cardFixture(3, nil, nil, "")},
	}
	for _, tc := range cases {
		if cardMatchesSelection(tc.card, tc.sel) {
			t.Errorf("%s: an empty-value selection should match nothing, even a card whose own %s field is also empty", tc.name, tc.name)
		}
	}
}

func TestFilterSet_FilterTypeNoneSelectionInSet_MatchesNothing(t *testing.T) {
	// Risk: cardMatchesSelection's default branch must return false for an
	// unknown/filterTypeNone category placed directly in the set -- distinct
	// from the *empty set* returning true at the filterSet.matches level.
	// Getting this backwards fails open (every card would match a
	// none-category selection sitting in a non-empty set).
	sel := filterSelection{itemType: filterTypeNone, value: ""}
	card := cardFixture(1, []string{"bug"}, []string{"alice"}, "v1.0")

	if cardMatchesSelection(card, sel) {
		t.Error("cardMatchesSelection must return false for a filterTypeNone selection, not true")
	}

	fs := filterSet{sel}
	if fs.matches(card) {
		t.Error("a filterSet containing only a filterTypeNone selection must match nothing, even though the empty set matches everything")
	}
}

// TestFilterSet_ClearFilter_ClearsTheSet (formerly
// TestFilterSet_ApplyFilterTypeNone_ClearsTheSet, #653): applyFilter is
// deleted -- clearFilter is the surviving choke point that must clear the
// set entirely (b.filters = nil), symmetric with setActiveFilter's
// filterTypeNone convention.
func TestFilterSet_ClearFilter_ClearsTheSet(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	setActiveFilters(&b, filterSelection{itemType: filterByLabel, value: "bug"})

	b.clearFilter()

	if b.hasActiveFilters() {
		t.Error("clearFilter() should clear the filter set entirely")
	}
	if filterCount(&b) != 0 {
		t.Errorf("filterCount after clearFilter() = %d, want 0", filterCount(&b))
	}
}

// --- filterSet.contains / filterSet.toggled (#653) ---
//
// Pure unit tests over the new toggle-set algebra: neither method exists in
// production yet (they land in model.go during the GREEN phase) -- this
// section is expected to fail to compile until then.

func TestFilterSet_Contains_MatchesExactSelection(t *testing.T) {
	fs := filterSet{{itemType: filterByLabel, value: "bug"}}
	if !fs.contains(filterByLabel, "bug") {
		t.Error("contains should report true for a selection present in the set")
	}
	if fs.contains(filterByLabel, "feature") {
		t.Error("contains should report false for a value not present in the set")
	}
}

// TestFilterSet_Contains_CaseInsensitive pins the EqualFold membership
// comparison convention (Q7): a milestone toggled from the Milestones modal
// and the same milestone's picker row must resolve to one selection
// regardless of case.
func TestFilterSet_Contains_CaseInsensitive(t *testing.T) {
	fs := filterSet{{itemType: filterByLabel, value: "bug"}}

	if !fs.contains(filterByLabel, "BUG") {
		t.Error("contains should match case-insensitively (strings.EqualFold), like cardMatchesSelection")
	}
	if fs.contains(filterByAssignee, "bug") {
		t.Error("contains should not match a different itemType even with the same value")
	}
}

func TestFilterSet_Toggled_AddsNewSelection(t *testing.T) {
	fs := filterSet{{itemType: filterByLabel, value: "bug"}}
	sel := filterSelection{itemType: filterByAssignee, value: "alice"}

	got := fs.toggled(sel)

	if !got.contains(filterByAssignee, "alice") {
		t.Errorf("toggled(%+v) = %+v, want it to contain the new selection", sel, got)
	}
	if !got.contains(filterByLabel, "bug") {
		t.Errorf("toggled(%+v) = %+v, want the pre-existing selection to remain", sel, got)
	}
	if len(got) != 2 {
		t.Errorf("len(toggled(...)) = %d, want 2", len(got))
	}
}

// TestFilterSet_Toggled_RemovesExistingSelection is the toggle-off case
// named in the plan's Test Strategy.
func TestFilterSet_Toggled_RemovesExistingSelection(t *testing.T) {
	fs := filterSet{
		{itemType: filterByLabel, value: "bug"},
		{itemType: filterByAssignee, value: "alice"},
	}
	sel := filterSelection{itemType: filterByLabel, value: "bug"}

	got := fs.toggled(sel)

	if got.contains(filterByLabel, "bug") {
		t.Errorf("toggled(%+v) = %+v, want the existing selection removed", sel, got)
	}
	if !got.contains(filterByAssignee, "alice") {
		t.Errorf("toggled(%+v) = %+v, want the other selection to remain", sel, got)
	}
	if len(got) != 1 {
		t.Errorf("len(toggled(...)) = %d, want 1", len(got))
	}
}

// TestFilterSet_Toggled_CaseInsensitiveMembership_RemovesRegardlessOfCase
// pins toggled's use of the same EqualFold membership check as contains: a
// case-variant of an already-selected value must toggle it OFF, not add a
// second, distinct-cased entry.
func TestFilterSet_Toggled_CaseInsensitiveMembership_RemovesRegardlessOfCase(t *testing.T) {
	fs := filterSet{{itemType: filterByMilestone, value: "v1.0"}}
	sel := filterSelection{itemType: filterByMilestone, value: "V1.0"}

	got := fs.toggled(sel)

	if got.contains(filterByMilestone, "v1.0") {
		t.Errorf("toggled(%+v) = %+v, want the case-variant existing selection removed (EqualFold membership)", sel, got)
	}
	if len(got) != 0 {
		t.Errorf("len(toggled(...)) = %d, want 0 (toggle-off, not a second case-distinct entry)", len(got))
	}
}

// TestFilterSet_Toggled_ReturnsFreshSlice_NoAliasing guards the slice-aliasing
// hazard named in the plan's Risks section: toggled must allocate a new
// backing array rather than appending in place, since Board is copied by
// value through every Update() handler and a pre-Update snapshot of b.filters
// must never observe a later mutation.
func TestFilterSet_Toggled_ReturnsFreshSlice_NoAliasing(t *testing.T) {
	original := filterSet{{itemType: filterByLabel, value: "bug"}}
	snapshot := original

	_ = original.toggled(filterSelection{itemType: filterByAssignee, value: "alice"})

	if len(snapshot) != 1 || snapshot[0].value != "bug" {
		t.Errorf("pre-toggle snapshot mutated: got %+v, want unchanged single 'bug' selection", snapshot)
	}
}

func TestBoard_HasActiveFilters_TrueWithOneSelection(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	setActiveFilter(&b, filterByLabel, "bug")
	if !b.hasActiveFilters() {
		t.Error("hasActiveFilters() should be true with one selection in the set")
	}
}

func TestBoard_HasActiveFilters_FalseWithEmptySet(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	if b.hasActiveFilters() {
		t.Error("hasActiveFilters() should be false with an empty filter set")
	}
}

func TestBoard_HasActiveFilters_TrueWithEmptyValueMilestoneSelection(t *testing.T) {
	// Q5 / plan Risks: an empty-value milestone selection is still "active"
	// even though it matches nothing -- hasActiveFilters must not conflate
	// "no selections" with "a selection that happens to match nothing".
	// TestFilter_MilestoneFilter_EmptyActiveValueMatchesNothing (filter_test.go)
	// must keep passing unmodified alongside this.
	b := newBoardWithFilterableCards(t)
	setActiveFilter(&b, filterByMilestone, "")
	if !b.hasActiveFilters() {
		t.Error("hasActiveFilters() should be true for a set containing an empty-value milestone selection")
	}
}

// --- Integration: two-category set through filteredCards/totalFilteredCards/borderTitleCounts ---
//
// Uses newBoardWithFilterableCards' fixture (filter_test.go):
//   Backlog:     #1 bug/alice, #2 feature/bob, #3 bug/alice, #4 docs/charlie, #5 bug/bob
//   In Progress: #6 feature/alice, #7 bug/bob

func TestFilterSet_Integration_ORWithinCategory_FilteredCards(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	setActiveFilters(&b,
		filterSelection{itemType: filterByLabel, value: "bug"},
		filterSelection{itemType: filterByLabel, value: "feature"},
	)

	filtered := b.filteredCards()
	// Backlog: #1, #2, #3, #5 match ("bug" or "feature"); #4 ("docs") excluded.
	want := 4
	if len(filtered) != want {
		t.Errorf("filteredCards() with OR-within-category label set {bug, feature}: got %d cards, want %d", len(filtered), want)
	}
	for _, card := range filtered {
		if card.Number == 4 {
			t.Error("filteredCards() should exclude card #4 ('docs' label), which matches neither selected label")
		}
	}
}

func TestFilterSet_Integration_ANDAcrossCategories_FilteredCards(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	setActiveFilters(&b,
		filterSelection{itemType: filterByLabel, value: "bug"},
		filterSelection{itemType: filterByAssignee, value: "alice"},
	)

	filtered := b.filteredCards()
	// Backlog: #1 (bug, alice) and #3 (bug, alice) match both selections;
	// #5 (bug, bob) fails the assignee selection.
	want := 2
	if len(filtered) != want {
		t.Errorf("filteredCards() with AND-across-category set {label:bug, assignee:alice}: got %d cards, want %d", len(filtered), want)
	}
	for _, card := range filtered {
		if card.Number != 1 && card.Number != 3 {
			t.Errorf("filteredCards() returned unexpected card #%d for AND-across-category set", card.Number)
		}
	}
}

func TestFilterSet_Integration_TotalFilteredCards_TwoCategorySet(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	setActiveFilters(&b,
		filterSelection{itemType: filterByLabel, value: "bug"},
		filterSelection{itemType: filterByAssignee, value: "alice"},
	)

	// Backlog: #1, #3 match (2). In Progress: #6 (feature, alice) fails the
	// label selection, #7 (bug, bob) fails the assignee selection -- 0.
	want := 2
	if got := b.totalFilteredCards(); got != want {
		t.Errorf("totalFilteredCards() with AND-across-category set = %d, want %d", got, want)
	}
}

func TestFilterSet_Integration_BorderTitleCounts_TwoCategorySet(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	setActiveFilters(&b,
		filterSelection{itemType: filterByLabel, value: "bug"},
		filterSelection{itemType: filterByAssignee, value: "alice"},
	)

	counts := b.borderTitleCounts()
	if len(counts) != len(b.Columns) {
		t.Fatalf("borderTitleCounts() length = %d, want %d (one per column)", len(counts), len(b.Columns))
	}
	// Backlog (index 0): 2 matches (#1, #3). In Progress (index 1): 0 matches.
	wantBacklog, wantInProgress := 2, 0
	if counts[0] != wantBacklog {
		t.Errorf("borderTitleCounts()[0] (Backlog) = %d, want %d", counts[0], wantBacklog)
	}
	if counts[1] != wantInProgress {
		t.Errorf("borderTitleCounts()[1] (In Progress) = %d, want %d", counts[1], wantInProgress)
	}
}

// TestFilterSet_PreUpdateSnapshotUnaffectedByLaterMutation guards against
// the slice-aliasing hazard named in the plan's Risks section: Board is
// copied by value through every Update() handler, so b.filters must only
// ever be replaced wholesale (a fresh slice or nil), never mutated in place
// through an existing element -- that constraint carries forward to #653's
// toggle. This snapshots the slice value (a header copy sharing the same
// backing array) before Update() runs and asserts it is still byte-for-byte
// the original two selections afterward.
// TestFilterSet_PreUpdateSnapshotUnaffectedByLaterMutation (#653: 'f' no
// longer clears, so the clearing dispatch now goes through the picker's
// filter.clear_all command ('c') instead of a direct normal-mode 'f' press).
func TestFilterSet_PreUpdateSnapshotUnaffectedByLaterMutation(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	setActiveFilters(&b,
		filterSelection{itemType: filterByLabel, value: "bug"},
		filterSelection{itemType: filterByAssignee, value: "alice"},
	)

	before := b.filters

	b = sendKey(t, b, keyMsg("f")) // open the picker; 'f' never clears
	m, _ := b.Update(keyMsg("c"))  // filter.clear_all
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if filterCount(&updated) != 0 {
		t.Fatalf("precondition: 'c' (filter.clear_all) should clear the filter set, got %d selections", filterCount(&updated))
	}

	want := filterSet{
		{itemType: filterByLabel, value: "bug"},
		{itemType: filterByAssignee, value: "alice"},
	}
	if !slices.Equal(before, want) {
		t.Errorf("pre-Update snapshot of b.filters was mutated in place: got %+v, want %+v", before, want)
	}
}
