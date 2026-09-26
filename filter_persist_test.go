package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// Persisted active filters (#664). Every test here drives the real
// Update() dispatch against a temp-dir state file and asserts on what
// config.LoadState reads back, never on hand-written YAML. Boards are built
// with provider/owner/repo AND statePath together: a board with an empty
// repo identity (most helpers in helpers_test.go) has no filter key and
// deliberately writes nothing, which would make a save test pass vacuously.

const (
	persistProvider = "github"
	// Mixed case on purpose: config.FilterRepoKey lower-cases, so the same
	// repo must be found under either spelling.
	persistOwner = "Acme"
	persistRepo  = "Widgets"

	persistOtherOwner = "Acme"
	persistOtherRepo  = "Gadgets"
)

func persistRepoKey() string {
	return config.FilterRepoKey(persistProvider, persistOwner, persistRepo)
}

func persistOtherRepoKey() string {
	return config.FilterRepoKey(persistProvider, persistOtherOwner, persistOtherRepo)
}

// persistBoardData is the fetched board every persistence test uses: labels
// bug/feature/docs and assignees alice/bob/charlie, two columns.
func persistBoardData() provider.Board {
	return provider.Board{
		Columns: []provider.Column{
			{Title: "Backlog", Cards: []provider.Card{
				{Number: 1, Title: "Bug fix", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
				{Number: 2, Title: "Feature work", Labels: []provider.Label{{Name: "feature"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
				{Number: 3, Title: "Another bug", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
				{Number: 4, Title: "Docs update", Labels: []provider.Label{{Name: "docs"}}, Assignees: []provider.Assignee{{Login: "charlie"}}},
			}},
			{Title: "In Progress", Cards: []provider.Card{
				{Number: 5, Title: "Active bug", Labels: []provider.Label{{Name: "bug"}}, Assignees: []provider.Assignee{{Login: "bob"}}},
			}},
		},
	}
}

// countLabeled returns how many cards on data carry the given label.
func countLabeled(data provider.Board, label string) int {
	n := 0
	for _, col := range data.Columns {
		for _, c := range col.Cards {
			for _, l := range c.Labels {
				if l.Name == label {
					n++
				}
			}
		}
	}
	return n
}

// newUnloadedPersistBoard builds a loading-mode board tracking owner/repo
// whose runtime state persists to statePath.
func newUnloadedPersistBoard(owner, repo, statePath string) Board {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, owner, repo, persistProvider, 0, 0, "Working", false, false, nil, nil, true)
	b.statePath = statePath
	b.Width = 120
	b.Height = 40
	return b
}

// newPersistBoardFor builds a loaded board tracking owner/repo with data.
func newPersistBoardFor(t *testing.T, statePath, owner, repo string, data provider.Board) Board {
	t.Helper()
	b := newUnloadedPersistBoard(owner, repo, statePath)
	m, _ := b.Update(boardFetchedMsg{board: data})
	loaded, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return loaded
}

// newPersistBoard is the common case: the default repo, the default data, a
// fresh temp-dir state file. It returns the board and the state path.
func newPersistBoard(t *testing.T) (Board, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state.yml")
	return newPersistBoardFor(t, path, persistOwner, persistRepo, persistBoardData()), path
}

// updateAndRun sends msg through Update, runs the returned cmd tree and
// returns the board plus every message the cmds produced.
func updateAndRun(t *testing.T, b Board, msg tea.Msg) (Board, []tea.Msg) {
	t.Helper()
	m, cmd := b.Update(msg)
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return updated, runCmd(cmd)
}

func loadPersistedState(t *testing.T, path string) config.State {
	t.Helper()
	st, err := config.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}
	return st
}

// seedPersistedFilters writes sels as key's entry, standing in for an
// earlier run of lazyboards.
func seedPersistedFilters(t *testing.T, path, key string, sels ...config.FilterSelection) {
	t.Helper()
	err := config.UpdateState(path, func(st *config.State) bool {
		if st.Filters == nil {
			st.Filters = map[string][]config.FilterSelection{}
		}
		st.Filters[key] = sels
		return true
	})
	if err != nil {
		t.Fatalf("seeding state file: %v", err)
	}
}

// openPickerAndToggle presses 'f' then Enter (toggling the row under the
// cursor) and returns the board, the toggled row and the messages the save
// cmds produced.
func openPickerAndToggle(t *testing.T, b Board) (Board, filterItem, []tea.Msg) {
	t.Helper()
	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("mode = %d after 'f', want filterMode (%d)", b.mode, filterMode)
	}
	item := b.filterItems[b.filterCursor]
	b, msgs := updateAndRun(t, b, arrowMsg(tea.KeyEnter))
	return b, item, msgs
}

// categoryOf maps a filterType to the on-disk category the test expects. It
// is deliberately written out here rather than borrowed from production.
func categoryOf(t *testing.T, ft filterType) string {
	t.Helper()
	switch ft {
	case filterByLabel:
		return config.FilterCategoryLabel
	case filterByAssignee:
		return config.FilterCategoryAssignee
	case filterByMilestone:
		return config.FilterCategoryMilestone
	case filterByHierarchy:
		return config.FilterCategoryHierarchy
	}
	t.Fatalf("no on-disk category for filterType %d", ft)
	return ""
}

// --- Save: one test per filter-change site ---

func TestFilterPersist_PickerToggleOn_SavesSetForCurrentRepo(t *testing.T) {
	b, path := newPersistBoard(t)

	b, item, _ := openPickerAndToggle(t, b)

	if item.itemType != filterByLabel {
		t.Fatalf("precondition: first selectable picker row = %+v, want a label row", item)
	}
	if !hasFilter(&b, item.itemType, item.value) {
		t.Fatalf("precondition: %+v not active in memory after Enter", item)
	}
	// Read back under the lower-cased spelling: the mixed-case board must
	// share the entry with its lower-cased twin.
	got := loadPersistedState(t, path).FiltersFor(config.FilterRepoKey(persistProvider, "acme", "widgets"))
	want := []config.FilterSelection{{Category: categoryOf(t, item.itemType), Value: item.value}}
	if !slices.Equal(got, want) {
		t.Errorf("persisted filters = %+v, want %+v", got, want)
	}
}

func TestFilterPersist_PickerToggleOff_RemovesTheEntry(t *testing.T) {
	b, path := newPersistBoard(t)
	b, _, _ = openPickerAndToggle(t, b)

	b, _ = updateAndRun(t, b, arrowMsg(tea.KeyEnter)) // same row again: toggles off

	if b.hasActiveFilters() {
		t.Fatalf("precondition: filters = %+v after toggling off, want none", b.filters)
	}
	st := loadPersistedState(t, path)
	if got := st.FiltersFor(persistRepoKey()); len(got) != 0 {
		t.Errorf("persisted filters = %+v after toggling the last one off, want none", got)
	}
	if _, present := st.Filters[persistRepoKey()]; present {
		t.Errorf("state file still holds an entry for the repo, want it deleted rather than written as an empty list")
	}
}

// Saves write the FULL set, not just the toggled selection.
func TestFilterPersist_SecondCategoryToggle_SavesFullSet(t *testing.T) {
	b, path := newPersistBoard(t)
	b, first, _ := openPickerAndToggle(t, b)
	for b.filterItems[b.filterCursor].itemType != filterByAssignee {
		b = sendKey(t, b, keyMsg("j"))
		if b.filterCursor >= len(b.filterItems)-1 {
			t.Fatal("could not reach an assignee row")
		}
	}
	second := b.filterItems[b.filterCursor]

	b, _ = updateAndRun(t, b, arrowMsg(tea.KeyEnter))

	got := loadPersistedState(t, path).FiltersFor(persistRepoKey())
	want := []config.FilterSelection{
		{Category: categoryOf(t, first.itemType), Value: first.value},
		{Category: categoryOf(t, second.itemType), Value: second.value},
	}
	if !slices.Equal(got, want) {
		t.Errorf("persisted filters = %+v, want the full set %+v in label-then-assignee order", got, want)
	}
	if filterCount(&b) != len(want) {
		t.Errorf("in-memory selections = %d, want %d (disk must match memory)", filterCount(&b), len(want))
	}
}

func TestFilterPersist_ClearAll_SavesEmptySet(t *testing.T) {
	b, path := newPersistBoard(t)
	b, _, _ = openPickerAndToggle(t, b)
	if len(loadPersistedState(t, path).FiltersFor(persistRepoKey())) == 0 {
		t.Fatal("precondition: the toggle did not persist a selection")
	}

	b, _ = updateAndRun(t, b, keyMsg("c")) // filter.clear_all, modal stays open

	if b.hasActiveFilters() {
		t.Fatalf("precondition: filters = %+v after clear-all, want none", b.filters)
	}
	if got := loadPersistedState(t, path).FiltersFor(persistRepoKey()); len(got) != 0 {
		t.Errorf("persisted filters = %+v after clear-all, want none", got)
	}
}

func TestFilterPersist_MilestoneToggle_SavesAndRemovesSelection(t *testing.T) {
	b, path := newPersistBoard(t)
	fixture := []provider.Milestone{{Title: "v1.0", URL: "https://github.com/acme/widgets/milestone/2"}}
	b = openMilestoneListWithResult(t, b, fixture)

	b, _ = updateAndRun(t, b, arrowMsg(tea.KeyEnter))

	got := loadPersistedState(t, path).FiltersFor(persistRepoKey())
	want := []config.FilterSelection{{Category: config.FilterCategoryMilestone, Value: fixture[0].Title}}
	if !slices.Equal(got, want) {
		t.Fatalf("persisted filters after toggling on = %+v, want %+v", got, want)
	}

	b, _ = updateAndRun(t, b, arrowMsg(tea.KeyEnter))

	if got := loadPersistedState(t, path).FiltersFor(persistRepoKey()); len(got) != 0 {
		t.Errorf("persisted filters after toggling off = %+v, want none", got)
	}
	if b.hasActiveFilters() {
		t.Errorf("filters = %+v in memory after toggling off, want none", b.filters)
	}
}

func TestFilterPersist_ReferenceJumpClear_SavesEmptySet(t *testing.T) {
	data := provider.Board{Columns: []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #3", URL: "https://github.com/acme/widgets/issues/1", Labels: []provider.Label{{Name: "bug"}}},
			{Number: 2, Title: "Filler", Labels: []provider.Label{{Name: "bug"}}},
			{Number: 3, Title: "Target", Labels: []provider.Label{{Name: "feature"}}},
		}},
	}}
	path := filepath.Join(t.TempDir(), "state.yml")
	b := newPersistBoardFor(t, path, persistOwner, persistRepo, data)
	b, item, _ := openPickerAndToggle(t, b) // label "bug" hides #3
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	if len(loadPersistedState(t, path).FiltersFor(persistRepoKey())) == 0 {
		t.Fatalf("precondition: selecting %q did not persist", item.value)
	}

	b = sendKeys(t, b, "g", "r")
	b, _ = updateAndRun(t, b, keyMsg("a")) // jump to the hidden #3, clearing the filter

	if b.hasActiveFilters() {
		t.Fatalf("precondition: filters = %+v after the jump, want cleared", b.filters)
	}
	if got := loadPersistedState(t, path).FiltersFor(persistRepoKey()); len(got) != 0 {
		t.Errorf("persisted filters = %+v after the reference-jump clear, want none", got)
	}
}

func TestFilterPersist_CardCreatedClear_SavesEmptySet(t *testing.T) {
	b, path := newPersistBoard(t)
	b, _, _ = openPickerAndToggle(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	if len(loadPersistedState(t, path).FiltersFor(persistRepoKey())) == 0 {
		t.Fatal("precondition: the toggle did not persist a selection")
	}

	b, _ = updateAndRun(t, b, cardCreatedMsg{card: provider.Card{Number: 99, Title: "New card"}})

	if b.hasActiveFilters() {
		t.Fatalf("precondition: filters = %+v after card creation, want cleared", b.filters)
	}
	if got := loadPersistedState(t, path).FiltersFor(persistRepoKey()); len(got) != 0 {
		t.Errorf("persisted filters = %+v after the card-created clear, want none", got)
	}
}

// The automatic clears save every time they fire, even on an already-empty
// set, so a disk entry left behind by an earlier failed save is repaired.
func TestFilterPersist_CardCreatedClear_RepairsStaleDiskEntryEvenWhenMemoryIsEmpty(t *testing.T) {
	b, path := newPersistBoard(t)
	seedPersistedFilters(t, path, persistRepoKey(), config.FilterSelection{Category: config.FilterCategoryLabel, Value: "bug"})
	if b.hasActiveFilters() {
		t.Fatal("precondition: the in-memory set should be empty")
	}

	_, _ = updateAndRun(t, b, cardCreatedMsg{card: provider.Card{Number: 99, Title: "New card"}})

	if got := loadPersistedState(t, path).FiltersFor(persistRepoKey()); len(got) != 0 {
		t.Errorf("persisted filters = %+v, want the stale entry cleared to match the empty in-memory set", got)
	}
}

// --- Isolation between repositories and between state keys ---

func TestFilterPersist_SavingRepoB_LeavesRepoAEntryUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	a := newPersistBoardFor(t, path, persistOwner, persistRepo, persistBoardData())
	_, aItem, _ := openPickerAndToggle(t, a)
	wantA := loadPersistedState(t, path).FiltersFor(persistRepoKey())
	if len(wantA) == 0 {
		t.Fatalf("precondition: repo A's toggle of %+v did not persist", aItem)
	}

	b := newPersistBoardFor(t, path, persistOtherOwner, persistOtherRepo, persistBoardData())
	b = sendKey(t, b, keyMsg("f"))
	for b.filterItems[b.filterCursor].itemType != filterByAssignee {
		b = sendKey(t, b, keyMsg("j"))
		if b.filterCursor >= len(b.filterItems)-1 {
			t.Fatal("could not reach an assignee row")
		}
	}
	bItem := b.filterItems[b.filterCursor]
	_, _ = updateAndRun(t, b, arrowMsg(tea.KeyEnter))

	st := loadPersistedState(t, path)
	if got := st.FiltersFor(persistRepoKey()); !slices.Equal(got, wantA) {
		t.Errorf("repo A filters = %+v after repo B saved, want %+v unchanged", got, wantA)
	}
	wantB := []config.FilterSelection{{Category: config.FilterCategoryAssignee, Value: bItem.value}}
	if got := st.FiltersFor(persistOtherRepoKey()); !slices.Equal(got, wantB) {
		t.Errorf("repo B filters = %+v, want %+v", got, wantB)
	}
}

// A filter save keeps the sort order, and a sort toggle keeps the filters.
// This board tracks a real repo, so the sort toggle writes the per-repo
// sort_orders entry, not the legacy global sort_order.
func TestFilterPersist_SortAndFilterSaves_DoNotEraseEachOther(t *testing.T) {
	b, path := newPersistBoard(t)

	b, _ = updateAndRun(t, b, keyMsg("s")) // sort newest first
	b, item, _ := openPickerAndToggle(t, b)
	st := loadPersistedState(t, path)
	if got := st.SortOrderForRepo(persistRepoKey()); got != config.SortOrderNewest {
		t.Errorf("per-repo sort order = %q after a filter save, want %q kept", got, config.SortOrderNewest)
	}
	if len(st.FiltersFor(persistRepoKey())) != 1 {
		t.Fatalf("filters = %+v, want the toggled selection", st.FiltersFor(persistRepoKey()))
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	_, _ = updateAndRun(t, b, keyMsg("s")) // sort oldest first again

	st = loadPersistedState(t, path)
	if got := st.SortOrderForRepo(persistRepoKey()); got != config.SortOrderOldest {
		t.Errorf("per-repo sort order = %q, want %q", got, config.SortOrderOldest)
	}
	want := []config.FilterSelection{{Category: categoryOf(t, item.itemType), Value: item.value}}
	if got := st.FiltersFor(persistRepoKey()); !slices.Equal(got, want) {
		t.Errorf("filters = %+v after a sort save, want %+v kept", got, want)
	}
}

// Filter saves for one repo are ordered by generation like sort saves: when
// the older Cmd runs last, the newer snapshot stays on disk.
func TestFilterPersist_OutOfOrderSaves_KeepNewestSnapshot(t *testing.T) {
	b, path := newPersistBoard(t)
	cmdOlder := b.toggleFilter(filterByLabel, "bug")
	cmdNewer := b.toggleFilter(filterByLabel, "feature")
	if cmdOlder == nil || cmdNewer == nil {
		t.Fatal("toggleFilter returned a nil cmd with a state path and repo identity configured")
	}

	cmdNewer()
	cmdOlder()

	got := loadPersistedState(t, path).FiltersFor(persistRepoKey())
	want := []config.FilterSelection{
		{Category: config.FilterCategoryLabel, Value: "bug"},
		{Category: config.FilterCategoryLabel, Value: "feature"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("persisted filters = %+v, want the newer snapshot %+v", got, want)
	}
}

// --- Nothing to persist to / nothing to key by ---

func TestFilterPersist_NoStatePath_ReturnsNilCmdAndStillFilters(t *testing.T) {
	b := newPersistBoardFor(t, "", persistOwner, persistRepo, persistBoardData())

	cmd := b.toggleFilter(filterByLabel, "bug")

	if cmd != nil {
		t.Error("toggleFilter returned a non-nil cmd with no state path, want nil (session-only)")
	}
	if !hasFilter(&b, filterByLabel, "bug") {
		t.Errorf("filters = %+v, want the toggle applied even though it cannot be saved", b.filters)
	}
	if cmd := b.clearFilter(); cmd != nil {
		t.Error("clearFilter returned a non-nil cmd with no state path, want nil")
	}

	b = sendKey(t, b, keyMsg("f"))
	m, pickerCmd := b.Update(arrowMsg(tea.KeyEnter))
	if _, ok := m.(Board); !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if pickerCmd != nil {
		t.Error("picker Enter returned a non-nil cmd with no state path, want nil")
	}
}

func TestFilterPersist_EmptyRepoIdentity_WritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	b := newPersistBoardFor(t, path, "", "", persistBoardData())

	cmd := b.toggleFilter(filterByLabel, "bug")

	if cmd != nil {
		t.Error("toggleFilter returned a non-nil cmd for a board with no repo identity, want nil")
	}
	if !hasFilter(&b, filterByLabel, "bug") {
		t.Errorf("filters = %+v, want the toggle applied in memory", b.filters)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("state file stat error = %v, want it not to exist (no repo key, nothing written)", err)
	}
}

// --- Save result messages ---

func TestFilterPersist_SaveError_ShowsStatusMessageAndKeepsFilters(t *testing.T) {
	// A state path whose parent is a regular file cannot be created.
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	b := newPersistBoardFor(t, filepath.Join(blocker, "state.yml"), persistOwner, persistRepo, persistBoardData())
	b, item, msgs := openPickerAndToggle(t, b)

	var errMsg tea.Msg
	for _, msg := range msgs {
		if _, ok := msg.(filtersSaveErrorMsg); ok {
			errMsg = msg
		}
	}
	if errMsg == nil {
		t.Fatalf("save cmd produced %v, want a filtersSaveErrorMsg for an unwritable path", msgs)
	}
	b = sendKey(t, b, errMsg)

	if !hasFilter(&b, item.itemType, item.value) {
		t.Errorf("filters = %+v after a failed save, want the in-memory selection kept", b.filters)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	if view := b.View(); !strings.Contains(view, "Could not save filters") {
		t.Errorf("View() after a failed filter save does not show the failure, got:\n%s", view)
	}
}

func TestFilterPersist_SaveErrorMsg_ReturnsTimeoutCmdAndSanitizesText(t *testing.T) {
	b, _ := newPersistBoard(t)
	setActiveFilter(&b, filterByLabel, "bug")

	m, cmd := b.Update(filtersSaveErrorMsg{err: errors.New("disk\nfull\x1b[31m")})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if cmd == nil {
		t.Error("filtersSaveErrorMsg returned a nil cmd, want the status-bar message timeout cmd")
	}
	if !hasFilter(&updated, filterByLabel, "bug") {
		t.Errorf("filters = %+v, want them unchanged by a failed save", updated.filters)
	}
	if strings.ContainsAny(updated.statusBar.message, "\n\x1b") {
		t.Errorf("status message = %q, want it flattened (no newline or escape byte)", updated.statusBar.message)
	}
	if !strings.Contains(updated.statusBar.message, "Could not save filters") {
		t.Errorf("status message = %q, want it to start with %q", updated.statusBar.message, "Could not save filters")
	}
}

func TestFilterPersist_SavedMsg_IsSilent(t *testing.T) {
	b, _ := newPersistBoard(t)
	b.Width = 120
	b.Height = 40
	before := b.View()

	m, cmd := b.Update(filtersSavedMsg{})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if cmd != nil {
		t.Error("filtersSavedMsg returned a non-nil cmd, want nil (a successful save is silent)")
	}
	if updated.View() != before {
		t.Error("filtersSavedMsg changed the view, want no visible effect")
	}
}

func TestSaveFiltersCmd_ReportsSavedAndErrorMessages(t *testing.T) {
	seq := newTestBoard(t).stateSaves
	sels := []config.FilterSelection{{Category: config.FilterCategoryLabel, Value: "bug"}}
	path := filepath.Join(t.TempDir(), "state.yml")

	msg := saveFiltersCmd(path, seq, seq.ticket(filtersGateKey(persistRepoKey())), persistRepoKey(), sels)()
	if _, ok := msg.(filtersSavedMsg); !ok {
		t.Fatalf("saveFiltersCmd() msg = %T, want filtersSavedMsg", msg)
	}

	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	msg = saveFiltersCmd(filepath.Join(blocker, "state.yml"), seq, seq.ticket(filtersGateKey(persistRepoKey())), persistRepoKey(), sels)()
	if _, ok := msg.(filtersSaveErrorMsg); !ok {
		t.Errorf("saveFiltersCmd() on an unwritable path msg = %T, want filtersSaveErrorMsg", msg)
	}
}

// --- Startup restore ---

func TestFilterPersist_StartupRestore_ActiveBeforeFetchAndAppliedAfter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	data := persistBoardData()
	seedPersistedFilters(t, path, persistRepoKey(), config.FilterSelection{Category: config.FilterCategoryLabel, Value: "bug"})
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)

	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))

	if !hasFilter(&b, filterByLabel, "bug") {
		t.Fatalf("filters = %+v before any fetch, want the saved label active", b.filters)
	}
	if b.statusBar.filterStatus == "" || !strings.Contains(b.statusBar.filterStatus, "bug") {
		t.Errorf("statusBar.filterStatus = %q before the fetch, want the restored set shown via refreshFilterStatus", b.statusBar.filterStatus)
	}

	m, _ := b.Update(boardFetchedMsg{board: data})
	b = m.(Board)

	if want := countLabeled(data, "bug"); b.totalFilteredCards() != want {
		t.Errorf("totalFilteredCards() = %d after the first fetch, want %d (the label's cards)", b.totalFilteredCards(), want)
	}
	if view := b.View(); !strings.Contains(view, "⚑") {
		t.Errorf("View() after the first fetch lacks the filter glyph, got:\n%s", view)
	}
}

// A Hierarchy row (#663) persists like any other category and survives a
// restart: the picker toggle writes it, and seedFromState restores it.
func TestFilterPersist_HierarchyToggle_SavesAndRestores(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	data := persistBoardData()
	data.Columns[0].Cards[0].SubIssueCount = 2
	data.Columns[0].Cards[1].ParentNumber = 1
	b := newPersistBoardFor(t, path, persistOwner, persistRepo, data)
	b = sendKey(t, b, keyMsg("f"))
	for b.filterItems[b.filterCursor].itemType != filterByHierarchy {
		if b.filterCursor >= len(b.filterItems)-1 {
			t.Fatal("could not reach a hierarchy row")
		}
		b = sendKey(t, b, keyMsg("j"))
	}
	item := b.filterItems[b.filterCursor]

	_, _ = updateAndRun(t, b, arrowMsg(tea.KeyEnter))

	got := loadPersistedState(t, path).FiltersFor(persistRepoKey())
	want := []config.FilterSelection{{Category: config.FilterCategoryHierarchy, Value: item.value}}
	if !slices.Equal(got, want) {
		t.Fatalf("persisted filters = %+v, want %+v", got, want)
	}

	restored := seedFromState(newUnloadedPersistBoard(persistOwner, persistRepo, path), config.Config{}, loadPersistedState(t, path))
	if !hasFilter(&restored, filterByHierarchy, item.value) {
		t.Errorf("filters = %+v after restart, want the hierarchy row %q active", restored.filters, item.value)
	}
}

func TestFilterPersist_StartupRestore_OtherReposEntryIsNotApplied(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	seedPersistedFilters(t, path, persistOtherRepoKey(), config.FilterSelection{Category: config.FilterCategoryLabel, Value: "bug"})
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)

	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))

	if b.hasActiveFilters() {
		t.Errorf("filters = %+v for a repo with no saved entry, want none", b.filters)
	}
}

func TestFilterPersist_StartupRestore_StillSeedsSortDirection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	if err := config.UpdateState(path, func(st *config.State) bool {
		st.SortOrder = config.SortOrderNewest
		return true
	}); err != nil {
		t.Fatalf("seeding sort order: %v", err)
	}
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)

	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))

	if !b.sortNewestFirst {
		t.Error("sortNewestFirst = false after seeding from a newest-first state, want true")
	}
}

// A saved value no card carries (e.g. a deleted label) is restored exactly as
// saved, keeps counting in the segment, and is only removed by clear-all --
// which also persists.
func TestFilterPersist_StartupRestore_StaleValueKeptUntilClearAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	const stale = "deleted-label"
	saved := config.FilterSelection{Category: config.FilterCategoryLabel, Value: stale}
	seedPersistedFilters(t, path, persistRepoKey(), saved)
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)
	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))
	m, _ := b.Update(boardFetchedMsg{board: persistBoardData()})
	b = m.(Board)

	if !hasFilter(&b, filterByLabel, stale) {
		t.Fatalf("filters = %+v after the fetch, want the stale selection kept as saved", b.filters)
	}
	if b.totalFilteredCards() != 0 {
		t.Errorf("totalFilteredCards() = %d, want 0 (no card carries %q)", b.totalFilteredCards(), stale)
	}
	if got := loadPersistedState(t, path).FiltersFor(persistRepoKey()); !slices.Equal(got, []config.FilterSelection{saved}) {
		t.Errorf("state file = %+v after the fetch, want the stale entry untouched", got)
	}

	b = sendKey(t, b, keyMsg("f"))
	b, _ = updateAndRun(t, b, keyMsg("c"))

	if b.hasActiveFilters() {
		t.Errorf("filters = %+v after clear-all, want none", b.filters)
	}
	if got := loadPersistedState(t, path).FiltersFor(persistRepoKey()); len(got) != 0 {
		t.Errorf("state file = %+v after clear-all, want the stale entry removed", got)
	}
}

// Restoring never writes: only a user-driven change does.
func TestFilterPersist_StartupRestore_DoesNotWriteTheStateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	seedPersistedFilters(t, path, persistRepoKey(), config.FilterSelection{Category: config.FilterCategoryLabel, Value: "bug"})
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)

	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))
	_, _ = updateAndRun(t, b, boardFetchedMsg{board: persistBoardData()})

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("state file changed by a restore:\nbefore: %q\nafter:  %q", before, after)
	}
}

// --- Q1: the no-matches warning on the very first fetch ---

func TestFilterPersist_FirstFetch_RestoredFilterMatchingNothing_ShowsNoMatchesWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	seedPersistedFilters(t, path, persistRepoKey(), config.FilterSelection{Category: config.FilterCategoryLabel, Value: "deleted-label"})
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)
	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))

	m, cmd := b.Update(boardFetchedMsg{board: persistBoardData()})
	b = m.(Board)

	if cmd == nil {
		t.Error("first fetch returned a nil cmd, want the status message timeout cmd")
	}
	if want := b.filterNoMatchesMessage(); b.statusBar.message != want {
		t.Errorf("status message = %q on the first fetch, want %q", b.statusBar.message, want)
	}
	if strings.Contains(b.statusBar.message, "Board refreshed") {
		t.Errorf("status message = %q, want no %q on a first load", b.statusBar.message, "Board refreshed")
	}
}

// The relaxed gate must not add "Board refreshed" to a first load whose
// restored filter does match.
func TestFilterPersist_FirstFetch_RestoredFilterMatching_ShowsNoRefreshedMessage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	seedPersistedFilters(t, path, persistRepoKey(), config.FilterSelection{Category: config.FilterCategoryLabel, Value: "bug"})
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)
	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))

	m, _ := b.Update(boardFetchedMsg{board: persistBoardData()})
	b = m.(Board)

	if b.statusBar.message != "" {
		t.Errorf("status message = %q on a first load with matching filters, want none", b.statusBar.message)
	}
}

// --- Repo-switch restore ---

// newRepoSwitchPersistFixture returns a board tracking repo A with A's saved
// filters restored, a state file that also holds repo B's entry, and B's
// entry. Its providerFactory serves a fake provider so a retarget succeeds.
func newRepoSwitchPersistFixture(t *testing.T) (b Board, statePath string, entryB []config.FilterSelection) {
	t.Helper()
	dir := t.TempDir()
	statePath = filepath.Join(dir, "state.yml")
	entryA := []config.FilterSelection{{Category: config.FilterCategoryLabel, Value: "bug"}}
	entryB = []config.FilterSelection{{Category: config.FilterCategoryAssignee, Value: "alice"}}
	seedPersistedFilters(t, statePath, persistRepoKey(), entryA...)
	seedPersistedFilters(t, statePath, persistOtherRepoKey(), entryB...)

	b = newUnloadedPersistBoard(persistOwner, persistRepo, statePath)
	b = seedFromState(b, config.Config{}, loadPersistedState(t, statePath))
	m, _ := b.Update(boardFetchedMsg{board: persistBoardData()})
	b = m.(Board)
	b.providerFactory = func(providerName, owner, repo string) (provider.BoardProvider, error) {
		return provider.NewFakeProvider(), nil
	}
	return b, statePath, entryB
}

// savedMsgFor runs saveConfigCmd for repo against statePath (with a false
// config-file sort default, irrelevant to the filter-focused tests that call
// this) and returns the configSavedMsg it produced.
func savedMsgFor(t *testing.T, statePath, repo string) configSavedMsg {
	t.Helper()
	return savedMsgForWithSortDefault(t, statePath, repo, false)
}

// savedMsgForWithSortDefault is savedMsgFor's sibling for tests that also
// care about the resolved sort direction, threading the board's own
// config-file sort default through to saveConfigCmd.
func savedMsgForWithSortDefault(t *testing.T, statePath, repo string, cfgSortNewestFirst bool) configSavedMsg {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	for _, msg := range runCmd(saveConfigCmd(cfgPath, persistProvider, repo, "", statePath, cfgSortNewestFirst)) {
		if saved, ok := msg.(configSavedMsg); ok {
			return saved
		}
	}
	t.Fatalf("saveConfigCmd produced no configSavedMsg for %q", repo)
	return configSavedMsg{}
}

func TestFilterPersist_RepoSwitch_LoadsNewReposFiltersAndLeavesOldEntryAlone(t *testing.T) {
	b, statePath, entryB := newRepoSwitchPersistFixture(t)
	before, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	saved := savedMsgFor(t, statePath, persistOtherOwner+"/"+persistOtherRepo)
	if !slices.Equal(saved.savedFilters, entryB) {
		t.Fatalf("configSavedMsg.savedFilters = %+v, want repo B's entry %+v", saved.savedFilters, entryB)
	}
	b, _ = updateAndRun(t, b, saved)

	if !hasFilter(&b, filterByAssignee, "alice") || filterCount(&b) != len(entryB) {
		t.Errorf("filters = %+v after the switch, want exactly repo B's saved set", b.filters)
	}
	if b.statusBar.filterStatus == "" {
		t.Error("statusBar.filterStatus is empty after the switch, want B's restored set shown")
	}
	after, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("the switch itself rewrote the state file:\nbefore: %q\nafter:  %q", before, after)
	}
}

func TestFilterPersist_RepoSwitch_MissingEntry_LeavesNoFiltersAndKeepsOldEntry(t *testing.T) {
	b, statePath, _ := newRepoSwitchPersistFixture(t)
	wantA := loadPersistedState(t, statePath).FiltersFor(persistRepoKey())

	saved := savedMsgFor(t, statePath, "Acme/Unsaved")
	if len(saved.savedFilters) != 0 {
		t.Fatalf("configSavedMsg.savedFilters = %+v for a repo with no entry, want none", saved.savedFilters)
	}
	b, _ = updateAndRun(t, b, saved)

	if b.hasActiveFilters() {
		t.Errorf("filters = %+v after switching to a repo with no saved entry, want none", b.filters)
	}
	if got := loadPersistedState(t, statePath).FiltersFor(persistRepoKey()); !slices.Equal(got, wantA) {
		t.Errorf("repo A entry = %+v after the switch, want %+v untouched", got, wantA)
	}
}

// Reloading the same repo (e.g. a config modal save that changed nothing) is
// not a retarget: the in-memory set stays exactly as it is.
func TestFilterPersist_SameRepoReload_KeepsInMemoryFilters(t *testing.T) {
	b, _, _ := newRepoSwitchPersistFixture(t)
	other := []config.FilterSelection{{Category: config.FilterCategoryLabel, Value: "not-the-in-memory-one"}}

	b, _ = updateAndRun(t, b, configSavedMsg{
		provider:     persistProvider,
		repo:         persistOwner + "/" + persistRepo,
		savedFilters: other,
	})

	if !hasFilter(&b, filterByLabel, "bug") || filterCount(&b) != 1 {
		t.Errorf("filters = %+v after a same-repo reload, want the in-memory set kept", b.filters)
	}
}

// A malformed state file must not block or fail the config save: the switch
// proceeds with no filters.
func TestSaveConfigCmd_MalformedStateFile_StillReportsSavedWithNoFilters(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.yml")
	if err := os.WriteFile(statePath, []byte("sort_order: [unclosed\n"), 0600); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	saved := savedMsgFor(t, statePath, persistOtherOwner+"/"+persistOtherRepo)

	if len(saved.savedFilters) != 0 {
		t.Errorf("configSavedMsg.savedFilters = %+v from a malformed state file, want none", saved.savedFilters)
	}
}

// An empty statePath (no home directory) means nothing to read; the save
// still succeeds.
func TestSaveConfigCmd_NoStatePath_ReportsSavedWithNoFilters(t *testing.T) {
	saved := savedMsgFor(t, "", persistOtherOwner+"/"+persistOtherRepo)

	if len(saved.savedFilters) != 0 {
		t.Errorf("configSavedMsg.savedFilters = %+v with no state path, want none", saved.savedFilters)
	}
}
