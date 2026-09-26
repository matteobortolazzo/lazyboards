package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newSortPersistBoard returns a loaded board whose sort direction is persisted
// to a state file in a temp dir, plus that file's path.
func newSortPersistBoard(t *testing.T) (Board, string) {
	t.Helper()
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	path := filepath.Join(t.TempDir(), "state.yml")
	b.statePath = path
	return b, path
}

func TestNormalMode_S_PersistsNewSortOrder(t *testing.T) {
	b, path := newSortPersistBoard(t)

	m, cmd := b.Update(keyMsg("s"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("'s' toggle returned a nil cmd, want a cmd that persists the new sort order")
	}
	if !updated.sortNewestFirst {
		t.Fatalf("precondition: sortNewestFirst = false after toggle, want true")
	}
	execCmds(cmd)

	st, err := config.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}
	if st.SortOrder != config.SortOrderNewest {
		t.Errorf("persisted sort_order = %q, want %q", st.SortOrder, config.SortOrderNewest)
	}
}

func TestNormalMode_S_PersistsBothDirections(t *testing.T) {
	b, path := newSortPersistBoard(t)

	m, cmd := b.Update(keyMsg("s"))
	b, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	execCmds(cmd)

	m, cmd = b.Update(keyMsg("s"))
	b, ok = m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("second 's' toggle returned a nil cmd, want a cmd that persists the restored sort order")
	}
	execCmds(cmd)

	st, err := config.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}
	if st.SortOrder != config.SortOrderOldest {
		t.Errorf("persisted sort_order = %q, want %q (toggling back must persist too)", st.SortOrder, config.SortOrderOldest)
	}
	if b.sortNewestFirst {
		t.Error("sortNewestFirst = true after toggling twice, want false")
	}
}

// A board with no resolvable state path (e.g. no home directory) must still
// toggle — it just can't remember the choice.
func TestNormalMode_S_WithoutStatePath_TogglesWithoutPersisting(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	m, cmd := b.Update(keyMsg("s"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("'s' toggle returned a non-nil cmd with no state path configured, want nil (nothing to persist to)")
	}
	if !updated.sortNewestFirst {
		t.Error("sortNewestFirst = false after toggle, want true (the toggle must work even when it can't be saved)")
	}
	assertCardOrder(t, updated.Columns[0].Cards, []int{2, 1})
}

// A failed write must surface, not disappear: the sort still flips, but the
// user is told the choice won't survive a restart.
func TestSortOrderSaveError_ShowsStatusMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	m, cmd := b.Update(sortOrderSaveErrorMsg{err: errors.New("permission denied")})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Error("sortOrderSaveErrorMsg returned a nil cmd, want the status-bar message timeout cmd")
	}

	view := updated.View()
	if !strings.Contains(view, "sort order") {
		t.Errorf("View() after a failed sort-order save does not mention the failure, got:\n%s", view)
	}
}

func TestSortOrderSaveErrorCmd_ReportsUnwritablePath(t *testing.T) {
	// A path whose parent is a regular file can't be created as a directory.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	seq := newTestBoard(t).stateSaves
	cmd := saveSortOrderCmd(filepath.Join(file, "state.yml"), seq, seq.ticket(sortOrderGateKey("")), "", true)
	if cmd == nil {
		t.Fatal("saveSortOrderCmd() returned nil, want a cmd")
	}
	msg := cmd()

	if _, ok := msg.(sortOrderSaveErrorMsg); !ok {
		t.Errorf("saveSortOrderCmd() msg = %T, want sortOrderSaveErrorMsg", msg)
	}
}

func TestSaveSortOrderCmd_SuccessReportsSavedMsg(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")

	seq := newTestBoard(t).stateSaves
	msg := saveSortOrderCmd(path, seq, seq.ticket(sortOrderGateKey("")), "", false)()

	if _, ok := msg.(sortOrderSavedMsg); !ok {
		t.Fatalf("saveSortOrderCmd() msg = %T, want sortOrderSavedMsg", msg)
	}
	st, err := config.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}
	if st.SortOrder != config.SortOrderOldest {
		t.Errorf("persisted sort_order = %q, want %q", st.SortOrder, config.SortOrderOldest)
	}
}

// Toggling the sort order must only touch sort_order: an existing filters
// entry (any repo's) has to survive the save (#664).
func TestSaveSortOrderCmd_KeepsExistingFiltersEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	key := config.FilterRepoKey("github", "acme", "widgets")
	saved := []config.FilterSelection{{Category: config.FilterCategoryLabel, Value: "bug"}}
	err := config.UpdateState(path, func(st *config.State) bool {
		st.Filters = map[string][]config.FilterSelection{key: saved}
		return true
	})
	if err != nil {
		t.Fatalf("seeding filters: %v", err)
	}

	seq := newTestBoard(t).stateSaves
	msg := saveSortOrderCmd(path, seq, seq.ticket(sortOrderGateKey("")), "", true)()

	if _, ok := msg.(sortOrderSavedMsg); !ok {
		t.Fatalf("saveSortOrderCmd() msg = %T, want sortOrderSavedMsg", msg)
	}
	st, err := config.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}
	if st.SortOrder != config.SortOrderNewest {
		t.Errorf("persisted sort_order = %q, want %q", st.SortOrder, config.SortOrderNewest)
	}
	if got := st.FiltersFor(key); !slices.Equal(got, saved) {
		t.Errorf("filters = %+v after a sort save, want %+v kept", got, saved)
	}
}

// BubbleTea runs Cmds concurrently, so an older save's Cmd can execute after a
// newer one's. The generation gate must keep the newer snapshot on disk. Only
// a direct Cmd call can force this order: Update always issues gens in order.
func TestSaveSortOrderCmd_OutOfOrderGenerationsKeepNewest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	seq := newTestBoard(t).stateSaves
	older := seq.ticket(sortOrderGateKey(""))
	newer := seq.ticket(sortOrderGateKey(""))

	newerMsg := saveSortOrderCmd(path, seq, newer, "", true)()
	olderMsg := saveSortOrderCmd(path, seq, older, "", false)()

	if _, ok := newerMsg.(sortOrderSavedMsg); !ok {
		t.Fatalf("newer save msg = %T, want sortOrderSavedMsg", newerMsg)
	}
	if _, ok := olderMsg.(sortOrderSaveErrorMsg); ok {
		t.Errorf("superseded save reported %T, want it dropped silently (not an error)", olderMsg)
	}
	st, err := config.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}
	if st.SortOrder != config.SortOrderNewest {
		t.Errorf("persisted sort_order = %q, want %q (the newer generation must win)", st.SortOrder, config.SortOrderNewest)
	}
}

// A successful save is silent — no status-bar noise on every 's' press.
func TestSortOrderSavedMsg_ShowsNoStatusMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	before := b.View()

	m, cmd := b.Update(sortOrderSavedMsg{})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("sortOrderSavedMsg returned a non-nil cmd, want nil (a successful save is silent)")
	}
	if updated.View() != before {
		t.Error("sortOrderSavedMsg changed the view, want no visible effect")
	}
}

// --- Per-repository sort order ---
//
// These tests reuse filter_persist_test.go's repo-identity fixtures
// (persistProvider/persistOwner/persistRepo, newPersistBoard(For),
// newUnloadedPersistBoard, savedMsgForWithSortDefault, loadPersistedState):
// a board with no repo identity (newSortPersistBoard above) has no sort key
// and deliberately writes/reads the legacy global sort_order, which the
// tests above already cover.

// sortPersistBoardData is like filter_persist_test.go's persistBoardData but
// with distinguishable CreatedAt values, so a re-sort is observable.
func sortPersistBoardData() provider.Board {
	return provider.Board{Columns: []provider.Column{
		{Title: "Backlog", Cards: []provider.Card{
			{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
			{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
		}},
	}}
}

// seedPersistedSortOrder writes order as key's per-repo sort_orders entry,
// standing in for an earlier run of lazyboards.
func seedPersistedSortOrder(t *testing.T, path, key, order string) {
	t.Helper()
	err := config.UpdateState(path, func(st *config.State) bool {
		if st.SortOrders == nil {
			st.SortOrders = map[string]string{}
		}
		st.SortOrders[key] = order
		return true
	})
	if err != nil {
		t.Fatalf("seeding per-repo sort order: %v", err)
	}
}

func TestNormalMode_S_WithRepoIdentity_WritesOnlyThisReposEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	otherKey := persistOtherRepoKey()
	otherFilterSel := config.FilterSelection{Category: config.FilterCategoryLabel, Value: "bug"}
	if err := config.UpdateState(path, func(st *config.State) bool {
		st.SortOrder = config.SortOrderNewest // legacy global, must survive untouched
		st.SortOrders = map[string]string{otherKey: config.SortOrderNewest}
		st.Filters = map[string][]config.FilterSelection{persistRepoKey(): {otherFilterSel}}
		return true
	}); err != nil {
		t.Fatalf("seeding state: %v", err)
	}
	b := newPersistBoardFor(t, path, persistOwner, persistRepo, persistBoardData())

	m, cmd := b.Update(keyMsg("s"))
	if _, ok := m.(Board); !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("'s' toggle returned a nil cmd, want a cmd that persists the new sort order")
	}
	execCmds(cmd)

	st := loadPersistedState(t, path)
	if got := st.SortOrderForRepo(persistRepoKey()); got != config.SortOrderNewest {
		t.Errorf("this repo's per-repo sort order = %q, want %q", got, config.SortOrderNewest)
	}
	if st.SortOrder != config.SortOrderNewest {
		t.Errorf("legacy global sort_order = %q after a per-repo toggle, want %q untouched", st.SortOrder, config.SortOrderNewest)
	}
	if got := st.SortOrderForRepo(otherKey); got != config.SortOrderNewest {
		t.Errorf("other repo's per-repo sort order = %q after this repo's toggle, want %q untouched", got, config.SortOrderNewest)
	}
	if got := st.FiltersFor(persistRepoKey()); !slices.Equal(got, []config.FilterSelection{otherFilterSel}) {
		t.Errorf("filters = %+v after a sort toggle, want %+v untouched", got, []config.FilterSelection{otherFilterSel})
	}
}

func TestSaveSortOrderCmd_TwoDifferentRepos_BothSavesLand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	seq := newTestBoard(t).stateSaves
	keyA := persistRepoKey()
	keyB := persistOtherRepoKey()

	msgA := saveSortOrderCmd(path, seq, seq.ticket(sortOrderGateKey(keyA)), keyA, true)()
	msgB := saveSortOrderCmd(path, seq, seq.ticket(sortOrderGateKey(keyB)), keyB, false)()

	if _, ok := msgA.(sortOrderSavedMsg); !ok {
		t.Fatalf("repo A save msg = %T, want sortOrderSavedMsg", msgA)
	}
	if _, ok := msgB.(sortOrderSavedMsg); !ok {
		t.Fatalf("repo B save msg = %T, want sortOrderSavedMsg", msgB)
	}
	st := loadPersistedState(t, path)
	if got := st.SortOrderForRepo(keyA); got != config.SortOrderNewest {
		t.Errorf("repo A sort order = %q, want %q", got, config.SortOrderNewest)
	}
	if got := st.SortOrderForRepo(keyB); got != config.SortOrderOldest {
		t.Errorf("repo B sort order = %q, want %q", got, config.SortOrderOldest)
	}
}

// BubbleTea runs Cmds concurrently, so within one repository's own saves an
// older Cmd can still execute after a newer one's; the per-repo gate key must
// keep ordering per repo, not just globally.
func TestSaveSortOrderCmd_PerRepo_OutOfOrderGenerationsKeepNewest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	seq := newTestBoard(t).stateSaves
	key := persistRepoKey()
	older := seq.ticket(sortOrderGateKey(key))
	newer := seq.ticket(sortOrderGateKey(key))

	newerMsg := saveSortOrderCmd(path, seq, newer, key, true)()
	olderMsg := saveSortOrderCmd(path, seq, older, key, false)()

	if _, ok := newerMsg.(sortOrderSavedMsg); !ok {
		t.Fatalf("newer save msg = %T, want sortOrderSavedMsg", newerMsg)
	}
	if _, ok := olderMsg.(sortOrderSaveErrorMsg); ok {
		t.Errorf("superseded save reported %T, want it dropped silently (not an error)", olderMsg)
	}
	if got := loadPersistedState(t, path).SortOrderForRepo(key); got != config.SortOrderNewest {
		t.Errorf("persisted per-repo sort order = %q, want %q (the newer generation must win)", got, config.SortOrderNewest)
	}
}

// --- Startup precedence via seedFromState ---

func TestSeedFromState_PerRepoSortWinsOverLegacyGlobalAndConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	seedPersistedSortOrder(t, path, persistRepoKey(), config.SortOrderNewest)
	if err := config.UpdateState(path, func(st *config.State) bool {
		st.SortOrder = config.SortOrderOldest
		return true
	}); err != nil {
		t.Fatalf("seeding legacy sort order: %v", err)
	}
	cfgOldest := config.SortOrderOldest
	cfg := config.Config{SortOrder: &cfgOldest}
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)

	b = seedFromState(b, cfg, loadPersistedState(t, path))

	if !b.sortNewestFirst {
		t.Error("sortNewestFirst = false after seeding, want true (the per-repo entry must win over both the legacy global value and the config default)")
	}
}

func TestSeedFromState_MissingPerRepoFallsBackToLegacyGlobal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	if err := config.UpdateState(path, func(st *config.State) bool {
		st.SortOrder = config.SortOrderNewest
		return true
	}); err != nil {
		t.Fatalf("seeding legacy sort order: %v", err)
	}
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)

	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))

	if !b.sortNewestFirst {
		t.Error("sortNewestFirst = false with no per-repo entry, want true (the legacy global value decides)")
	}
}

func TestSeedFromState_MissingBothFallsBackToConfigDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	newest := config.SortOrderNewest
	cfg := config.Config{SortOrder: &newest}
	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)

	b = seedFromState(b, cfg, loadPersistedState(t, path))

	if !b.sortNewestFirst {
		t.Error("sortNewestFirst = false with no persisted state at all, want true (the config default decides)")
	}
	if !b.configSortNewestFirst {
		t.Error("configSortNewestFirst = false, want true (must be seeded from cfg.SortNewestFirstValue() for later reuse by a repo switch)")
	}
}

// A board with no repo identity must never consult SortOrders, even when
// some (irrelevant) key happens to hold an entry.
func TestSeedFromState_NoRepoIdentity_SkipsPerRepoLayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	if err := config.UpdateState(path, func(st *config.State) bool {
		st.SortOrder = config.SortOrderOldest
		st.SortOrders = map[string]string{persistRepoKey(): config.SortOrderNewest}
		return true
	}); err != nil {
		t.Fatalf("seeding state: %v", err)
	}
	b := newTestBoard(t)
	b.statePath = path

	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))

	if b.sortNewestFirst {
		t.Error("sortNewestFirst = true for a board with no repo identity, want false (the legacy global value, never a per-repo entry)")
	}
}

// --- Repo-switch restore ---

func TestSortPersist_RepoSwitch_AppliesTargetReposSortAndReSorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	seedPersistedSortOrder(t, path, persistRepoKey(), config.SortOrderOldest)
	seedPersistedSortOrder(t, path, persistOtherRepoKey(), config.SortOrderNewest)

	b := newUnloadedPersistBoard(persistOwner, persistRepo, path)
	b = seedFromState(b, config.Config{}, loadPersistedState(t, path))
	m, _ := b.Update(boardFetchedMsg{board: sortPersistBoardData()})
	b, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b.sortNewestFirst {
		t.Fatalf("precondition: repo A resolved to newest-first, want oldest-first")
	}
	assertCardOrder(t, b.Columns[0].Cards, []int{1, 2})
	b.providerFactory = func(providerName, owner, repo string) (provider.BoardProvider, error) {
		return provider.NewFakeProvider(), nil
	}

	saved := savedMsgForWithSortDefault(t, path, persistOtherOwner+"/"+persistOtherRepo, false)
	if !saved.savedSortNewestFirst {
		t.Fatalf("precondition: configSavedMsg.savedSortNewestFirst = false, want true (repo B's saved newest-first entry)")
	}
	b, _ = updateAndRun(t, b, saved)
	if !b.sortNewestFirst {
		t.Fatal("sortNewestFirst = false after switching to repo B, want true")
	}

	m, _ = b.Update(boardFetchedMsg{board: sortPersistBoardData()})
	b, ok = m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	assertCardOrder(t, b.Columns[0].Cards, []int{2, 1})

	if got := loadPersistedState(t, path).SortOrderForRepo(persistRepoKey()); got != config.SortOrderOldest {
		t.Errorf("repo A per-repo sort order = %q after the switch, want %q untouched", got, config.SortOrderOldest)
	}
}

func TestSortPersist_RepoSwitch_MissingEntry_FallsBackToConfigDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	b := newPersistBoardFor(t, path, persistOwner, persistRepo, sortPersistBoardData())
	b.configSortNewestFirst = true // simulate a config-file default of newest-first
	b.providerFactory = func(providerName, owner, repo string) (provider.BoardProvider, error) {
		return provider.NewFakeProvider(), nil
	}

	saved := savedMsgForWithSortDefault(t, path, "Acme/Unsaved", true)
	if !saved.savedSortNewestFirst {
		t.Fatalf("configSavedMsg.savedSortNewestFirst = false for a repo with no saved entry, want true (the config default)")
	}
	b, _ = updateAndRun(t, b, saved)

	if !b.sortNewestFirst {
		t.Error("sortNewestFirst = false after switching to a repo with no saved entry, want true (the config default applied)")
	}
}

// Reloading the same repo (e.g. a config-modal save that changed nothing) is
// not a retarget: the in-memory sort direction stays exactly as it is.
func TestSortPersist_SameRepoReload_KeepsInMemorySort(t *testing.T) {
	b := newPersistBoardFor(t, "", persistOwner, persistRepo, persistBoardData())
	b.sortNewestFirst = true

	b, _ = updateAndRun(t, b, configSavedMsg{
		provider:             persistProvider,
		repo:                 persistOwner + "/" + persistRepo,
		savedSortNewestFirst: false,
	})

	if !b.sortNewestFirst {
		t.Error("sortNewestFirst = false after a same-repo reload, want the in-memory true kept")
	}
}
