package main

import (
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// TestShippedConfig_MigratedToKeymaps guards this repo's own .lazyboards.yml
// (test cwd is the repo root, so the relative path resolves directly)
// against a regression back to the deprecated columns[].actions/top-level
// actions: blocks (#552). It must load with zero legacy deprecations and
// resolve every migrated surface through the real keymap.Keymap pipeline:
// a column-scoped key (Refined/I), a scope: pr column key (In Review/W), and
// a top-level board action duplicated into ModeDetail (C) -- the last one
// locks in the plan's Q3 decision that top-level actions: must be mirrored
// into both keymaps.normal and keymaps.detail, not just normal.
func TestShippedConfig_MigratedToKeymaps(t *testing.T) {
	cfg, err := config.Load("", ".lazyboards.yml")
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}

	if len(cfg.Deprecations) != 0 {
		t.Fatalf("cfg.Deprecations = %v, want empty (shipped .lazyboards.yml should use keymaps:, not legacy actions:/columns[].actions)", cfg.Deprecations)
	}

	km, err := config.ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}

	// Column-scoped key: Refined column's "I" (Implement).
	result := km.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{"I"})
	if result.Outcome != keymap.OutcomeMatch {
		t.Fatalf("Lookup(normal, Refined, I) outcome = %v, want OutcomeMatch", result.Outcome)
	}
	if result.Binding.Kind != keymap.BindingAction {
		t.Fatalf("Lookup(normal, Refined, I) binding kind = %v, want BindingAction", result.Binding.Kind)
	}

	// scope: pr column key: In Review column's "W" (Open worktree).
	result = km.Lookup(keymap.ModeNormal, "In Review", keymap.Sequence{"W"})
	if result.Outcome != keymap.OutcomeMatch {
		t.Fatalf("Lookup(normal, In Review, W) outcome = %v, want OutcomeMatch", result.Outcome)
	}
	if result.Binding.Kind != keymap.BindingAction {
		t.Fatalf("Lookup(normal, In Review, W) binding kind = %v, want BindingAction", result.Binding.Kind)
	}
	if result.Binding.Action.Scope != "pr" {
		t.Fatalf("Lookup(normal, In Review, W) action scope = %q, want %q", result.Binding.Action.Scope, "pr")
	}

	// Board action duplicated into ModeDetail (Q3): top-level "C" (Claude)
	// must dispatch while the detail panel is focused too, not just normal.
	result = km.Lookup(keymap.ModeDetail, "", keymap.Sequence{"C"})
	if result.Outcome != keymap.OutcomeMatch {
		t.Fatalf("Lookup(detail, \"\", C) outcome = %v, want OutcomeMatch", result.Outcome)
	}
	if result.Binding.Kind != keymap.BindingAction {
		t.Fatalf("Lookup(detail, \"\", C) binding kind = %v, want BindingAction", result.Binding.Kind)
	}
}
