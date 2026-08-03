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
	// Self-trusting: this test guards the repo's own shipped config resolving
	// as authored, so it loads with the trust its own hash would carry once a
	// maintainer has approved it (#567) -- untrusted-stripping is covered
	// separately by TestShippedConfig_Untrusted_StripsInlineShellBindings.
	hash, err := config.HashLocalConfig(".lazyboards.yml")
	if err != nil {
		t.Fatalf("config.HashLocalConfig() returned unexpected error: %v", err)
	}
	trust := config.Trust{Trusted: []config.TrustEntry{{Hash: hash}}}
	cfg, err := config.Load("", ".lazyboards.yml", trust)
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

// TestShippedConfig_Untrusted_StripsInlineShellBindings guards this repo's
// own .lazyboards.yml against #567's regression: loaded with an untrusted
// (zero-value) Trust, every inline keymaps: shell binding it declares must
// be absent from the resolved keymap.Keymap, while every non-executing
// field (columns:) still parses and applies. This is the RED-phase target
// for #567 -- it will not compile until config.Load grows its third Trust
// argument.
func TestShippedConfig_Untrusted_StripsInlineShellBindings(t *testing.T) {
	cfg, err := config.Load("", ".lazyboards.yml", config.Trust{})
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}

	wantColumns := []string{"New", "Refined", "Planned", "In Review"}
	names := cfg.ColumnNames()
	if len(names) != len(wantColumns) {
		t.Fatalf("cfg.ColumnNames() = %v, want %v (columns: must still apply -- untrusted only strips executing constructs)", names, wantColumns)
	}
	for i, want := range wantColumns {
		if names[i] != want {
			t.Fatalf("cfg.ColumnNames() = %v, want %v", names, wantColumns)
		}
	}

	km, err := config.ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}

	assertNotAction := func(t *testing.T, mode keymap.Mode, column string, key string) {
		t.Helper()
		result := km.Lookup(mode, column, keymap.Sequence{keymap.Key(key)})
		if result.Outcome == keymap.OutcomeMatch && result.Binding.Kind == keymap.BindingAction {
			t.Fatalf("Lookup(%v, %q, %q) = %+v, want the untrusted local shell binding stripped (built-in default or no match)", mode, column, key, result)
		}
	}

	// Refined/I ("Implement") is an untrusted local column-scoped shell
	// binding with no global fallback (this repo has no global config in
	// test): stripping it must leave no BindingAction match.
	assertNotAction(t, keymap.ModeNormal, "Refined", "I")
	// In Review/W ("Open worktree", scope: pr) is the same story at a
	// different column.
	assertNotAction(t, keymap.ModeNormal, "In Review", "W")
	// detail/C ("Claude") is a top-level keymaps.detail shell binding.
	assertNotAction(t, keymap.ModeDetail, "", "C")
}
