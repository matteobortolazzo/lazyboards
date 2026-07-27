package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSaveState_LoadState_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")

	if err := SaveState(path, State{SortOrder: SortOrderNewest}); err != nil {
		t.Fatalf("SaveState() returned error: %v", err)
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
func TestSaveState_CreatesParentDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lazyboards", "state.yml")

	if err := SaveState(path, State{SortOrder: SortOrderOldest}); err != nil {
		t.Fatalf("SaveState() returned error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("state file not written at %s: %v", path, err)
	}
}

func TestSaveState_OverwritesPreviousValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")

	if err := SaveState(path, State{SortOrder: SortOrderNewest}); err != nil {
		t.Fatalf("first SaveState() returned error: %v", err)
	}
	if err := SaveState(path, State{SortOrder: SortOrderOldest}); err != nil {
		t.Fatalf("second SaveState() returned error: %v", err)
	}
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}

	if st.SortOrder != SortOrderOldest {
		t.Errorf("SortOrder = %q, want %q (the later save must win)", st.SortOrder, SortOrderOldest)
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

// --- Precedence: state file > config > built-in default ---

func TestResolveSortNewestFirst_StateOverridesConfig(t *testing.T) {
	oldest := SortOrderOldest
	cfg := Config{SortOrder: &oldest}

	if !ResolveSortNewestFirst(cfg, State{SortOrder: SortOrderNewest}) {
		t.Error("ResolveSortNewestFirst() = false, want true (a persisted toggle must beat the configured default)")
	}
}

func TestResolveSortNewestFirst_StateOldestOverridesConfigNewest(t *testing.T) {
	newest := SortOrderNewest
	cfg := Config{SortOrder: &newest}

	if ResolveSortNewestFirst(cfg, State{SortOrder: SortOrderOldest}) {
		t.Error("ResolveSortNewestFirst() = true, want false (a persisted toggle must beat the configured default in both directions)")
	}
}

func TestResolveSortNewestFirst_EmptyStateFallsBackToConfig(t *testing.T) {
	newest := SortOrderNewest
	cfg := Config{SortOrder: &newest}

	if !ResolveSortNewestFirst(cfg, State{}) {
		t.Error("ResolveSortNewestFirst() = false, want true (with no persisted state, config decides)")
	}
}

func TestResolveSortNewestFirst_EmptyStateAndConfigUsesDefault(t *testing.T) {
	if ResolveSortNewestFirst(Config{}, State{}) {
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
