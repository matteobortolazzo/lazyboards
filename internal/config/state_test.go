package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestLoadState_MissingFile_ReturnsZeroStateWithoutError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")

	st, err := LoadState(path)

	if err != nil {
		t.Fatalf("LoadState() on a missing file returned error %v, want nil (a first launch has no state file)", err)
	}
	if st.SortOrder != "" {
		t.Errorf("SortOrder = %q, want empty", st.SortOrder)
	}
}

// setSortOrder is an UpdateState mutation that sets only the sort order.
func setSortOrder(order string) func(*State) bool {
	return func(st *State) bool {
		st.SortOrder = order
		return true
	}
}

// setFilters is an UpdateState mutation that sets only one repo's filter entry.
func setFilters(key string, sels ...FilterSelection) func(*State) bool {
	return func(st *State) bool {
		if st.Filters == nil {
			st.Filters = map[string][]FilterSelection{}
		}
		st.Filters[key] = sels
		return true
	}
}

func TestUpdateState_LoadState_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")

	if err := UpdateState(path, setSortOrder(SortOrderNewest)); err != nil {
		t.Fatalf("UpdateState() returned error: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if st.SortOrder != SortOrderNewest {
		t.Errorf("SortOrder = %q, want %q", st.SortOrder, SortOrderNewest)
	}
}

// The state file lives next to the global config, whose directory may not
// exist yet on a machine that has never written one.
func TestUpdateState_CreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lazyboards", "state.yml")

	if err := UpdateState(path, setSortOrder(SortOrderOldest)); err != nil {
		t.Fatalf("UpdateState() returned error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file not written at %s: %v", path, err)
	}
}

func TestUpdateState_OverwritesPreviousValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")

	if err := UpdateState(path, setSortOrder(SortOrderNewest)); err != nil {
		t.Fatalf("first UpdateState() returned error: %v", err)
	}
	if err := UpdateState(path, setSortOrder(SortOrderOldest)); err != nil {
		t.Fatalf("second UpdateState() returned error: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if st.SortOrder != SortOrderOldest {
		t.Errorf("SortOrder = %q, want %q (the later save must win)", st.SortOrder, SortOrderOldest)
	}
}

// --- Filters: on-disk shape and validation ---

func TestUpdateState_LoadState_FiltersRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	key := FilterRepoKey("github", "acme", "widgets")
	want := []FilterSelection{
		{Category: FilterCategoryLabel, Value: "bug"},
		{Category: FilterCategoryAssignee, Value: "alice"},
		{Category: FilterCategoryMilestone, Value: "v1.0"},
		{Category: FilterCategoryHierarchy, Value: "Parents"},
	}

	if err := UpdateState(path, setFilters(key, want...)); err != nil {
		t.Fatalf("UpdateState() returned error: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if got := st.FiltersFor(key); !slices.Equal(got, want) {
		t.Errorf("FiltersFor(%q) = %+v, want %+v (order preserved)", key, got, want)
	}
}

// A selection value is kept exactly as saved, including an empty one.
func TestUpdateState_LoadState_KeepsEmptyValueAsSaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	key := FilterRepoKey("github", "acme", "widgets")
	want := []FilterSelection{{Category: FilterCategoryMilestone, Value: ""}}

	if err := UpdateState(path, setFilters(key, want...)); err != nil {
		t.Fatalf("UpdateState() returned error: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if got := st.FiltersFor(key); !slices.Equal(got, want) {
		t.Errorf("FiltersFor(%q) = %+v, want %+v", key, got, want)
	}
}

// One unrecognized category means a corrupted or hand-edited file, so the
// whole State is invalidated (all-or-nothing, like an invalid sort_order) --
// a valid sort_order in the same file must not leak through.
func TestLoadState_UnknownFilterCategory_InvalidatesWholeState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	content := "sort_order: newest\nfilters:\n  \"github:acme/widgets\":\n    - category: label\n      value: bug\n    - category: sprint\n      value: s1\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	st, err := LoadState(path)

	if err == nil {
		t.Fatal("LoadState() returned no error for an unknown filter category, want an error")
	}
	if st.SortOrder != "" || len(st.Filters) != 0 {
		t.Errorf("LoadState() = %+v on error, want the zero State", st)
	}
}

// An old file that only ever knew sort_order still loads, with no filters.
func TestLoadState_SortOrderOnlyFile_LoadsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	if err := os.WriteFile(path, []byte("sort_order: newest\n"), 0600); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	st, err := LoadState(path)

	if err != nil {
		t.Fatalf("LoadState() returned error for a sort_order-only file: %v", err)
	}
	if st.SortOrder != SortOrderNewest {
		t.Errorf("SortOrder = %q, want %q", st.SortOrder, SortOrderNewest)
	}
	if len(st.Filters) != 0 {
		t.Errorf("Filters = %+v, want none", st.Filters)
	}
}

func TestFilterRepoKey_LowerCasesAndIsCaseInsensitive(t *testing.T) {
	a := FilterRepoKey("GitHub", "Acme", "Widgets")
	b := FilterRepoKey("github", "acme", "widgets")

	if a == "" {
		t.Fatal("FilterRepoKey() returned an empty key for a fully specified repo")
	}
	if a != b {
		t.Errorf("FilterRepoKey differs by case: %q vs %q, want one shared key", a, b)
	}
	if a != strings.ToLower(a) {
		t.Errorf("FilterRepoKey() = %q, want it lower-cased", a)
	}
}

func TestFilterRepoKey_DistinguishesProviderOwnerAndRepo(t *testing.T) {
	base := FilterRepoKey("github", "acme", "widgets")
	others := []string{
		FilterRepoKey("azure-devops", "acme", "widgets"),
		FilterRepoKey("github", "other", "widgets"),
		FilterRepoKey("github", "acme", "gadgets"),
	}

	for _, o := range others {
		if o == base {
			t.Errorf("FilterRepoKey collided with the base key %q, want distinct keys per provider/owner/repo", base)
		}
	}
}

func TestFilterRepoKey_EmptyPartYieldsEmptyKey(t *testing.T) {
	cases := map[string]string{
		"provider": FilterRepoKey("", "acme", "widgets"),
		"owner":    FilterRepoKey("github", "", "widgets"),
		"repo":     FilterRepoKey("github", "acme", ""),
	}

	for part, key := range cases {
		if key != "" {
			t.Errorf("FilterRepoKey with empty %s = %q, want \"\" (no repo identity, nothing to persist)", part, key)
		}
	}
}

// setSortOrderForRepo is an UpdateState mutation that sets only one repo's
// per-repo sort-order entry.
func setSortOrderForRepo(key, order string) func(*State) bool {
	return func(st *State) bool {
		if st.SortOrders == nil {
			st.SortOrders = map[string]string{}
		}
		st.SortOrders[key] = order
		return true
	}
}

func TestUpdateState_LoadState_SortOrdersRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	key := FilterRepoKey("github", "acme", "widgets")

	if err := UpdateState(path, setSortOrderForRepo(key, SortOrderNewest)); err != nil {
		t.Fatalf("UpdateState() returned error: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if got := st.SortOrderForRepo(key); got != SortOrderNewest {
		t.Errorf("SortOrderForRepo(%q) = %q, want %q", key, got, SortOrderNewest)
	}
}

func TestState_SortOrderForRepo(t *testing.T) {
	keyA := FilterRepoKey("github", "acme", "a")
	keyB := FilterRepoKey("github", "acme", "b")
	st := State{SortOrders: map[string]string{keyA: SortOrderNewest}}

	if got := st.SortOrderForRepo(keyA); got != SortOrderNewest {
		t.Errorf("SortOrderForRepo(known key) = %q, want %q", got, SortOrderNewest)
	}
	if got := st.SortOrderForRepo(keyB); got != "" {
		t.Errorf("SortOrderForRepo(unknown key) = %q, want empty", got)
	}
	if got := (State{}).SortOrderForRepo(keyA); got != "" {
		t.Errorf("SortOrderForRepo on a State with a nil map = %q, want empty", got)
	}
}

// An unrecognized per-repo sort-order value invalidates the whole state file,
// exactly like an unrecognized legacy global sort_order.
func TestLoadState_InvalidPerRepoSortOrder_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	content := "sort_orders:\n  \"github:acme/widgets\": sideways\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	_, err := LoadState(path)

	if err == nil {
		t.Fatal("LoadState() returned no error for an unrecognized per-repo sort_orders value, want an error")
	}
	if !strings.Contains(err.Error(), "sort_orders") {
		t.Errorf("error = %q, want it to name the sort_orders field", err.Error())
	}
}

// A sort save for one repo must not erase another repo's sort entry or the
// legacy global sort_order.
func TestUpdateState_SortOrdersMergeKeepsOtherRepos(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	keyA := FilterRepoKey("github", "acme", "a")
	keyB := FilterRepoKey("github", "acme", "b")

	if err := UpdateState(path, setSortOrder(SortOrderNewest)); err != nil {
		t.Fatalf("seed legacy global sort order: %v", err)
	}
	if err := UpdateState(path, setSortOrderForRepo(keyA, SortOrderNewest)); err != nil {
		t.Fatalf("seed repo A: %v", err)
	}
	if err := UpdateState(path, setSortOrderForRepo(keyB, SortOrderOldest)); err != nil {
		t.Fatalf("save repo B: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if st.SortOrder != SortOrderNewest {
		t.Errorf("legacy SortOrder = %q after per-repo saves, want %q kept", st.SortOrder, SortOrderNewest)
	}
	if got := st.SortOrderForRepo(keyA); got != SortOrderNewest {
		t.Errorf("repo A sort order = %q after saving repo B, want %q unchanged", got, SortOrderNewest)
	}
	if got := st.SortOrderForRepo(keyB); got != SortOrderOldest {
		t.Errorf("repo B sort order = %q, want %q", got, SortOrderOldest)
	}
}

// --- Precedence: per-repo state > legacy global state > config > built-in default ---

func TestResolveSortNewestFirst_PerRepoWinsOverLegacyGlobalAndConfig(t *testing.T) {
	key := FilterRepoKey("github", "acme", "widgets")
	st := State{SortOrder: SortOrderOldest, SortOrders: map[string]string{key: SortOrderNewest}}

	if !ResolveSortNewestFirst(st, key, false) {
		t.Error("ResolveSortNewestFirst() = false, want true (a per-repo override must beat both the legacy global value and the config default)")
	}
}

func TestResolveSortNewestFirst_MissingPerRepoFallsBackToLegacyGlobal(t *testing.T) {
	key := FilterRepoKey("github", "acme", "widgets")
	st := State{SortOrder: SortOrderNewest}

	if !ResolveSortNewestFirst(st, key, false) {
		t.Error("ResolveSortNewestFirst() = false, want true (with no per-repo entry, the legacy global value decides)")
	}
}

func TestResolveSortNewestFirst_MissingBothFallsBackToConfigDefault(t *testing.T) {
	key := FilterRepoKey("github", "acme", "widgets")

	if !ResolveSortNewestFirst(State{}, key, true) {
		t.Error("ResolveSortNewestFirst() = false, want true (with no persisted state at all, the config default decides)")
	}
	if ResolveSortNewestFirst(State{}, key, false) {
		t.Error("ResolveSortNewestFirst() = true, want false (config default false must be honored too)")
	}
}

// An empty key means no repo identity to key by: the per-repo layer is
// skipped entirely, even if SortOrders happens to hold an entry for "".
func TestResolveSortNewestFirst_EmptyKeySkipsPerRepoLayer(t *testing.T) {
	st := State{SortOrder: SortOrderOldest, SortOrders: map[string]string{"": SortOrderNewest}}

	if ResolveSortNewestFirst(st, "", false) {
		t.Error("ResolveSortNewestFirst() = true, want false (an empty key must not consult SortOrders at all, falling to the legacy global value)")
	}
}

func TestResolveSortNewestFirst_UsesConfigSortNewestFirstValueAsFinalFallback(t *testing.T) {
	newest := SortOrderNewest
	cfg := Config{SortOrder: &newest}
	key := FilterRepoKey("github", "acme", "widgets")

	if !ResolveSortNewestFirst(State{}, key, cfg.SortNewestFirstValue()) {
		t.Error("ResolveSortNewestFirst() = false, want true (with no persisted state, the config's sort_order field decides)")
	}
}

func TestState_FiltersFor(t *testing.T) {
	keyA := FilterRepoKey("github", "acme", "a")
	keyB := FilterRepoKey("github", "acme", "b")
	sel := FilterSelection{Category: FilterCategoryLabel, Value: "bug"}
	st := State{Filters: map[string][]FilterSelection{keyA: {sel}}}

	if got := st.FiltersFor(keyA); !slices.Equal(got, []FilterSelection{sel}) {
		t.Errorf("FiltersFor(known key) = %+v, want [%+v]", got, sel)
	}
	if got := st.FiltersFor(keyB); len(got) != 0 {
		t.Errorf("FiltersFor(unknown key) = %+v, want none", got)
	}
	if got := (State{}).FiltersFor(keyA); len(got) != 0 {
		t.Errorf("FiltersFor on a State with a nil map = %+v, want none", got)
	}
}

// --- UpdateState: merge, repair, atomicity ---

// A save changes only the keys its mutation touches: the sort order, other
// repositories' entries and this repo's own other content all survive.
func TestUpdateState_MergeKeepsOtherKeysAndOtherRepos(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	keyA := FilterRepoKey("github", "acme", "a")
	keyB := FilterRepoKey("github", "acme", "b")
	selA := FilterSelection{Category: FilterCategoryLabel, Value: "bug"}
	selB := FilterSelection{Category: FilterCategoryAssignee, Value: "alice"}

	if err := UpdateState(path, setSortOrder(SortOrderNewest)); err != nil {
		t.Fatalf("seed sort order: %v", err)
	}
	if err := UpdateState(path, setFilters(keyA, selA)); err != nil {
		t.Fatalf("seed repo A: %v", err)
	}
	if err := UpdateState(path, setFilters(keyB, selB)); err != nil {
		t.Fatalf("save repo B: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if st.SortOrder != SortOrderNewest {
		t.Errorf("SortOrder = %q after a filter save, want %q kept", st.SortOrder, SortOrderNewest)
	}
	if got := st.FiltersFor(keyA); !slices.Equal(got, []FilterSelection{selA}) {
		t.Errorf("repo A filters = %+v after saving repo B, want [%+v] unchanged", got, selA)
	}
	if got := st.FiltersFor(keyB); !slices.Equal(got, []FilterSelection{selB}) {
		t.Errorf("repo B filters = %+v, want [%+v]", got, selB)
	}
}

// A sort save must not erase saved filters.
func TestUpdateState_SortSaveKeepsFilters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	key := FilterRepoKey("github", "acme", "a")
	sel := FilterSelection{Category: FilterCategoryLabel, Value: "bug"}
	if err := UpdateState(path, setFilters(key, sel)); err != nil {
		t.Fatalf("seed filters: %v", err)
	}

	if err := UpdateState(path, setSortOrder(SortOrderOldest)); err != nil {
		t.Fatalf("sort save: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if got := st.FiltersFor(key); !slices.Equal(got, []FilterSelection{sel}) {
		t.Errorf("filters = %+v after a sort save, want [%+v] kept", got, sel)
	}
	if st.SortOrder != SortOrderOldest {
		t.Errorf("SortOrder = %q, want %q", st.SortOrder, SortOrderOldest)
	}
}

// Deleting a repo's entry (an empty set) leaves the others alone.
func TestUpdateState_MutationCanDeleteOneReposEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	keyA := FilterRepoKey("github", "acme", "a")
	keyB := FilterRepoKey("github", "acme", "b")
	sel := FilterSelection{Category: FilterCategoryLabel, Value: "bug"}
	if err := UpdateState(path, setFilters(keyA, sel)); err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if err := UpdateState(path, setFilters(keyB, sel)); err != nil {
		t.Fatalf("seed B: %v", err)
	}

	err := UpdateState(path, func(st *State) bool {
		delete(st.Filters, keyB)
		return true
	})
	if err != nil {
		t.Fatalf("delete B: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if _, ok := st.Filters[keyB]; ok {
		t.Errorf("repo B entry still present after deletion: %+v", st.Filters)
	}
	if got := st.FiltersFor(keyA); !slices.Equal(got, []FilterSelection{sel}) {
		t.Errorf("repo A filters = %+v, want [%+v] unchanged", got, sel)
	}
}

func TestUpdateState_MissingFile_IsCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	key := FilterRepoKey("github", "acme", "a")

	if err := UpdateState(path, setFilters(key, FilterSelection{Category: FilterCategoryLabel, Value: "bug"})); err != nil {
		t.Fatalf("UpdateState() on a missing file returned error: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if len(st.FiltersFor(key)) != 1 {
		t.Errorf("filters = %+v, want the one saved selection", st.FiltersFor(key))
	}
}

// A file that fails to parse/validate was already ignored at startup, so the
// save repairs it with a fresh State rather than failing forever.
func TestUpdateState_MalformedFile_IsRepaired(t *testing.T) {
	cases := map[string]string{
		"malformed yaml":   "sort_order: [unclosed\n",
		"bad sort order":   "sort_order: sideways\n",
		"unknown category": "filters:\n  k:\n    - category: sprint\n      value: x\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "state.yml")
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatalf("failed to write state file: %v", err)
			}
			key := FilterRepoKey("github", "acme", "a")
			sel := FilterSelection{Category: FilterCategoryLabel, Value: "bug"}

			if err := UpdateState(path, setFilters(key, sel)); err != nil {
				t.Fatalf("UpdateState() on a %s file returned error: %v, want it repaired", name, err)
			}
			st, err := LoadState(path)
			if err != nil {
				t.Fatalf("LoadState() after repair returned error: %v", err)
			}

			if got := st.FiltersFor(key); !slices.Equal(got, []FilterSelection{sel}) {
				t.Errorf("filters = %+v after repair, want [%+v]", got, sel)
			}
		})
	}
}

// A file that could not be READ (as opposed to parsed) must never be
// overwritten: the save fails and the content survives.
func TestUpdateState_UnreadableFile_ReturnsErrorAndIsNotOverwritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("file permissions are not enforced for root")
	}
	path := filepath.Join(t.TempDir(), "state.yml")
	original := []byte("sort_order: newest\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}
	if err := os.Chmod(path, 0000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0600) })

	err := UpdateState(path, setSortOrder(SortOrderOldest))

	if err == nil {
		t.Fatal("UpdateState() on an unreadable file returned nil, want an error")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatalf("chmod restore: %v", err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("state file = %q after a failed save, want the original %q", got, original)
	}
}

// Same contract via an I/O error that also holds for root: the state path is
// a directory, so reading it fails with something other than not-exist.
func TestUpdateState_PathIsDirectory_ReturnsErrorAndLeavesItIntact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	marker := filepath.Join(path, "keep")
	if err := os.WriteFile(marker, []byte("x"), 0600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	err := UpdateState(path, setSortOrder(SortOrderOldest))

	if err == nil {
		t.Fatal("UpdateState() on a directory path returned nil, want an error")
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Errorf("directory content lost after a failed save: %v", statErr)
	}
}

// The atomic write goes through a temp file that must not be left behind.
func TestUpdateState_LeavesOnlyStateFileInDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yml")

	for i := 0; i < 3; i++ {
		if err := UpdateState(path, setSortOrder(SortOrderFor(i%2 == 0))); err != nil {
			t.Fatalf("UpdateState() #%d returned error: %v", i, err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != filepath.Base(path) {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only %q", names, filepath.Base(path))
	}
}

// The write goes through a new file in the state directory, not into the
// existing file: where the directory refuses a new file, the save fails and
// the existing content is untouched. An in-place write would succeed here
// (the file itself stays writable) and could tear the file on a crash.
func TestUpdateState_UncreatableTempFile_ReturnsErrorAndKeepsOriginal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("directory permissions are not enforced for root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yml")
	original := []byte("sort_order: newest\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	err := UpdateState(path, setSortOrder(SortOrderOldest))

	if err == nil {
		t.Fatal("UpdateState() in a read-only directory returned nil, want an error")
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if !bytes.Equal(got, original) {
		t.Errorf("state file = %q after a failed save, want the original %q", got, original)
	}
}

func TestUpdateState_MutateReturningFalse_LeavesFileByteIdentical(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	if err := UpdateState(path, setSortOrder(SortOrderNewest)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	err = UpdateState(path, func(st *State) bool {
		st.SortOrder = SortOrderOldest // a change that must be discarded
		return false
	})
	if err != nil {
		t.Fatalf("UpdateState() returned error: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	if !bytes.Equal(before, after) {
		t.Errorf("file changed although mutate returned false:\nbefore: %q\nafter:  %q", before, after)
	}
}

// Saves from several goroutines (BubbleTea runs Cmds concurrently) each
// re-read under the lock, so none can lose another's key. Run under -race.
func TestUpdateState_ConcurrentWritersToDistinctKeys_AllLand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	const writers = 16

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := FilterRepoKey("github", "acme", fmt.Sprintf("repo%d", i))
			errs <- UpdateState(path, setFilters(key, FilterSelection{Category: FilterCategoryLabel, Value: fmt.Sprintf("label%d", i)}))
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent UpdateState() returned error: %v", err)
		}
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	for i := 0; i < writers; i++ {
		key := FilterRepoKey("github", "acme", fmt.Sprintf("repo%d", i))
		want := []FilterSelection{{Category: FilterCategoryLabel, Value: fmt.Sprintf("label%d", i)}}
		if got := st.FiltersFor(key); !slices.Equal(got, want) {
			t.Errorf("writer %d's entry = %+v, want %+v (lost update)", i, got, want)
		}
	}
}

// The state file is machine-written, so an unrecognized direction means a
// corrupted or hand-edited file. It reports an error rather than being read
// as a valid direction; callers decide whether that's fatal.
func TestLoadState_InvalidSortOrder_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	if err := os.WriteFile(path, []byte("sort_order: sideways\n"), 0600); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	_, err := LoadState(path)

	if err == nil {
		t.Fatal("LoadState() returned no error for an unrecognized sort_order, want an error")
	}
	if !strings.Contains(err.Error(), "sort_order") {
		t.Errorf("error = %q, want it to name the sort_order field", err.Error())
	}
}

func TestLoadState_MalformedYAML_ReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")
	if err := os.WriteFile(path, []byte("sort_order: [unclosed\n"), 0600); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}

	if _, err := LoadState(path); err == nil {
		t.Fatal("LoadState() returned no error for malformed YAML, want an error")
	}
}

// --- Precedence: legacy global state > config > built-in default (no repo identity, key "") ---

func TestResolveSortNewestFirst_StateOverridesConfig(t *testing.T) {
	oldest := SortOrderOldest
	cfg := Config{SortOrder: &oldest}

	if !ResolveSortNewestFirst(State{SortOrder: SortOrderNewest}, "", cfg.SortNewestFirstValue()) {
		t.Error("ResolveSortNewestFirst() = false, want true (a persisted toggle must beat the configured default)")
	}
}

func TestResolveSortNewestFirst_StateOldestOverridesConfigNewest(t *testing.T) {
	newest := SortOrderNewest
	cfg := Config{SortOrder: &newest}

	if ResolveSortNewestFirst(State{SortOrder: SortOrderOldest}, "", cfg.SortNewestFirstValue()) {
		t.Error("ResolveSortNewestFirst() = true, want false (a persisted toggle must beat the configured default in both directions)")
	}
}

func TestResolveSortNewestFirst_EmptyStateFallsBackToConfig(t *testing.T) {
	newest := SortOrderNewest
	cfg := Config{SortOrder: &newest}

	if !ResolveSortNewestFirst(State{}, "", cfg.SortNewestFirstValue()) {
		t.Error("ResolveSortNewestFirst() = false, want true (with no persisted state, config decides)")
	}
}

func TestResolveSortNewestFirst_EmptyStateAndConfigUsesDefault(t *testing.T) {
	if ResolveSortNewestFirst(State{}, "", Config{}.SortNewestFirstValue()) {
		t.Error("ResolveSortNewestFirst() = true, want false (oldest-first is the built-in default, #503)")
	}
}

func TestSortOrderFor_MapsDirectionToPersistedValue(t *testing.T) {
	if got := SortOrderFor(true); got != SortOrderNewest {
		t.Errorf("SortOrderFor(true) = %q, want %q", got, SortOrderNewest)
	}
	if got := SortOrderFor(false); got != SortOrderOldest {
		t.Errorf("SortOrderFor(false) = %q, want %q", got, SortOrderOldest)
	}
}

func TestDefaultStatePath_SitsBesideGlobalConfig(t *testing.T) {
	statePath, err := DefaultStatePath()
	if err != nil {
		t.Fatalf("DefaultStatePath() returned error: %v", err)
	}
	globalPath, err := DefaultGlobalPath()
	if err != nil {
		t.Fatalf("DefaultGlobalPath() returned error: %v", err)
	}

	if filepath.Dir(statePath) != filepath.Dir(globalPath) {
		t.Errorf("state path dir = %q, want it alongside the global config dir %q", filepath.Dir(statePath), filepath.Dir(globalPath))
	}
	if statePath == globalPath {
		t.Error("DefaultStatePath() must not be the config file itself — runtime state is written separately so config.yml is never rewritten")
	}
}
