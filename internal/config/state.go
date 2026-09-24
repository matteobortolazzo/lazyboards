package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	"gopkg.in/yaml.v3"
)

// On-disk filter categories. They are strings, never the board's internal
// filterType integers, so those values never become a file-format contract.
const (
	FilterCategoryLabel     = "label"
	FilterCategoryAssignee  = "assignee"
	FilterCategoryMilestone = "milestone"
)

// FilterSelection is one persisted filter selection: a category and the value
// exactly as the user selected it.
type FilterSelection struct {
	Category string `yaml:"category"`
	Value    string `yaml:"value"`
}

// State holds runtime UI state that lazyboards writes for itself and reloads
// on the next launch. It is deliberately separate from Config: config.yml is
// hand-authored (comments, fields a newer/older binary doesn't know about),
// and rewriting it through yaml.Marshal on every toggle would destroy that
// content (#503). Only lazyboards writes this file.
type State struct {
	SortOrder string `yaml:"sort_order,omitempty"`
	// Filters holds each repository's active filter set, keyed by
	// FilterRepoKey (#664).
	Filters map[string][]FilterSelection `yaml:"filters,omitempty"`
}

// FilterRepoKey returns the state-file key for a tracked repository: provider
// plus owner/repo, lower-cased so spellings that differ only by case share one
// entry. It returns "" when any part is empty (no repo identity to key by).
func FilterRepoKey(provider, owner, repo string) string {
	if provider == "" || owner == "" || repo == "" {
		return ""
	}
	return strings.ToLower(provider + ":" + owner + "/" + repo)
}

// FiltersFor returns the saved filter selections for key, or nil.
func (s State) FiltersFor(key string) []FilterSelection {
	return s.Filters[key]
}

// DefaultStatePath returns the default runtime-state file path, alongside the
// global config at ~/.config/lazyboards/state.yml. The parent directory is
// created on demand by UpdateState, so it need not exist yet.
func DefaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lazyboards", "state.yml"), nil
}

// LoadState reads persisted runtime state from path. A missing file is not an
// error — it just means nothing has been persisted yet — but a malformed file
// or an unrecognized value is reported, so callers can decide (lazyboards logs
// it and falls back to the configured default rather than failing startup).
func LoadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}

	st, err := parseState(data)
	if err != nil {
		return State{}, fmt.Errorf("state file %s: %w", path, err)
	}
	return st, nil
}

// parseState decodes and validates state-file content. Any invalid value
// (sort_order, an unknown filter category) rejects the whole file.
func parseState(data []byte) (State, error) {
	var st State
	if err := yaml.Unmarshal(data, &st); err != nil {
		return State{}, err
	}
	if st.SortOrder != "" && st.SortOrder != SortOrderOldest && st.SortOrder != SortOrderNewest {
		return State{}, fmt.Errorf("sort_order must be %q or %q, got %q", SortOrderOldest, SortOrderNewest, st.SortOrder)
	}
	for key, sels := range st.Filters {
		for _, sel := range sels {
			switch sel.Category {
			case FilterCategoryLabel, FilterCategoryAssignee, FilterCategoryMilestone:
			default:
				return State{}, fmt.Errorf("filters %q: unknown category %q", key, sel.Category)
			}
		}
	}
	return st, nil
}

// stateMu serializes UpdateState's read-modify-write within this process.
var stateMu sync.Mutex

// UpdateState re-reads the state file at path, applies mutate to it and, when
// mutate returns true, writes the result back atomically (temp file in the
// same directory, then rename). Because each save re-reads the file, it
// changes only the keys its mutation touches and keeps everything else,
// including other repositories' entries. A file that fails to parse or
// validate was already ignored at startup, so it is logged and replaced with a
// fresh State; a file that cannot be read at all fails the save instead of
// being overwritten. mutate runs under the lock, so it may safely make an
// ordering decision together with its write.
func UpdateState(path string, mutate func(*State) bool) error {
	stateMu.Lock()
	defer stateMu.Unlock()

	var st State
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return fmt.Errorf("state file %s: %w", path, err)
	default:
		if st, err = parseState(data); err != nil {
			debuglog.Errorf("state file %s is invalid, rewriting it: %v", path, err)
			st = State{}
		}
	}

	if !mutate(&st) {
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("state file %s: mkdir %s: %w", path, dir, err)
	}
	out, err := yaml.Marshal(st)
	if err != nil {
		return fmt.Errorf("state file %s: marshal: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".state-*.tmp")
	if err != nil {
		return fmt.Errorf("state file %s: create temp: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once the rename below succeeds
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("state file %s: write temp: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("state file %s: close temp: %w", path, err)
	}
	if err := os.Chmod(tmpPath, 0600); err != nil {
		return fmt.Errorf("state file %s: chmod temp: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("state file %s: rename: %w", path, err)
	}
	return nil
}

// SortOrderFor maps a board's sort direction to the value persisted in the
// state file.
func SortOrderFor(newestFirst bool) string {
	if newestFirst {
		return SortOrderNewest
	}
	return SortOrderOldest
}

// ResolveSortNewestFirst decides the startup sort direction: a direction the
// user toggled at runtime (persisted state) wins, then the sort_order config
// field, then the built-in default.
func ResolveSortNewestFirst(cfg Config, st State) bool {
	if st.SortOrder != "" {
		return st.SortOrder == SortOrderNewest
	}
	return cfg.SortNewestFirstValue()
}
