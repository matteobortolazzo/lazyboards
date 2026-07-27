package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// State holds runtime UI state that lazyboards writes for itself and reloads
// on the next launch. It is deliberately separate from Config: config.yml is
// hand-authored (comments, fields a newer/older binary doesn't know about),
// and rewriting it through yaml.Marshal on every toggle would destroy that
// content (#503). Only lazyboards writes this file.
type State struct {
	SortOrder string `yaml:"sort_order,omitempty"`
}

// DefaultStatePath returns the default runtime-state file path, alongside the
// global config at ~/.config/lazyboards/state.yml. The parent directory is
// created on demand by SaveState, so it need not exist yet.
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

	var st State
	if err := yaml.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("state file %s: %w", path, err)
	}
	if st.SortOrder != "" && st.SortOrder != SortOrderOldest && st.SortOrder != SortOrderNewest {
		return State{}, fmt.Errorf("state file %s: sort_order must be %q or %q, got %q", path, SortOrderOldest, SortOrderNewest, st.SortOrder)
	}
	return st, nil
}

// SaveState writes st to path, creating the parent directory if needed.
func SaveState(path string, st State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	out, err := yaml.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0600)
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
