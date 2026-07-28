package main

import (
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// TestGitPanelDefaults_MatchDefaultGitActions is the cross-package drift
// guard named in the #508 plan (Q4/Assumptions): internal/keymap cannot
// import internal/config (binding.go), so the git_panel default table's six
// action entries are hand-duplicated literals in
// internal/keymap/defaults_git_panel.go. This test lives in package main --
// the only package that can import internal/keymap *and* internal/config
// *and* see gitPanelBuiltinOrder (model.go:1088) -- so it can assert the
// duplicated literals against the real producer values rather than
// hardcoding a second copy of its own.
//
// It asserts, for every key in gitPanelBuiltinOrder: the git_panel default
// table has a BindingAction entry for that key; its Name/Type/Command/Scope
// match config.DefaultGitActions()[key] field-for-field; and its Order is
// the key's 1-based position in gitPanelBuiltinOrder. It also asserts the
// git_panel table's full action-key set equals DefaultGitActions()'s key
// set, catching an action added or removed on either side.
func TestGitPanelDefaults_MatchDefaultGitActions(t *testing.T) {
	want := config.DefaultGitActions()
	defaults := keymap.Defaults()
	table := defaults.Modes[keymap.ModeGitPanel]

	gotActionKeys := make(map[string]bool)
	for key, binding := range table {
		if binding.Kind == keymap.BindingAction {
			gotActionKeys[key] = true
		}
	}

	wantKeys := make(map[string]bool, len(want))
	for key := range want {
		wantKeys[key] = true
	}

	for key := range wantKeys {
		if !gotActionKeys[key] {
			t.Errorf("git_panel default table has no action entry for key %q, which is present in config.DefaultGitActions()", key)
		}
	}
	for key := range gotActionKeys {
		if !wantKeys[key] {
			t.Errorf("git_panel default table has an action entry for key %q, which is absent from config.DefaultGitActions()", key)
		}
	}

	for i, key := range gitPanelBuiltinOrder {
		wantAction, ok := want[key]
		if !ok {
			t.Fatalf("gitPanelBuiltinOrder[%d] = %q, which is absent from config.DefaultGitActions() -- gitPanelBuiltinOrder and DefaultGitActions have drifted", i, key)
		}

		binding, ok := table[key]
		if !ok {
			t.Errorf("git_panel default table has no entry for key %q (gitPanelBuiltinOrder[%d])", key, i)
			continue
		}
		if binding.Kind != keymap.BindingAction {
			t.Errorf("git_panel default table entry for key %q has Kind %v, want BindingAction", key, binding.Kind)
			continue
		}

		got := binding.Action
		if got.Name != wantAction.Name || got.Type != wantAction.Type || got.Command != wantAction.Command || got.Scope != wantAction.Scope {
			t.Errorf("git_panel default table action for key %q = %+v, want Name/Type/Command/Scope matching config.DefaultGitActions()[%q] = %+v", key, got, key, wantAction)
		}

		wantOrder := i + 1
		if got.Order != wantOrder {
			t.Errorf("git_panel default table action for key %q has Order %d, want %d (its 1-based position in gitPanelBuiltinOrder)", key, got.Order, wantOrder)
		}
	}
}
