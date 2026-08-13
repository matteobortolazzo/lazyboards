package config

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- #584 (legacy half): stripShellFromActions already compares a local
// action against its global counterpart by value (`g == action`) instead of
// stripping unconditionally -- but Action is a plain comparable struct that
// includes the Order field (config.go's Action.Order, "derived metadata
// reflecting the action's position in its source YAML file"), and Order is
// NOT part of the trust-relevant "is this the same executing construct"
// comparison. Two content-identical actions declared at different document
// positions (global declares other keys before/after it; a local `actions:
// *anchor` alias leaves Order at 0 entirely, see stampActionOrder's early
// "not a MappingNode" bail-out for an aliased actions: node) get different
// Order values and so the `==` comparison spuriously fails, over-stripping
// an action that is genuinely global-equivalent -- the identical
// Order-sensitive bug the keymap half of this ticket fixes
// (trust_strip_keymap_test.go), just in the legacy actions:/
// columns[].actions: path instead of the native keymaps: path.
//
// Every assertion goes through Load -> ResolveKeymap -> km.Lookup, or
// Config.Actions/Config.Notices directly (never internal map internals
// beyond what Load itself returns), mirroring trust_strip_test.go's
// established convention.

// --- Case: global actions: declares A then G (Order 1, 2); untrusted local
// declares only a content-identical G (Order 1, since it's the sole key in
// its own document, so its stamped Order differs from global's). cfg.Actions["G"]
// must resolve to the global content and no notice must be raised, since the
// Order-insensitive comparison recognizes the two as the same construct. ---

func TestLoad_Untrusted_LegacyActionIdenticalToGlobal_DifferentDocPosition_NotStrippedNoNotice(t *testing.T) {
	globalYAML := `
actions:
  A:
    name: Alpha
    type: shell
    command: "echo alpha"
  G:
    name: Global Legacy Shell
    type: shell
    command: "echo global-legacy"
`
	localYAML := `
provider: github
repo: owner/repo
actions:
  G:
    name: Global Legacy Shell
    type: shell
    command: "echo global-legacy"
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	action, ok := cfg.Actions["G"]
	if !ok {
		t.Fatalf("cfg.Actions[%q] missing: the global-equivalent local G must not be stripped", "G")
	}
	if action.Command != "echo global-legacy" {
		t.Fatalf("cfg.Actions[%q].Command = %q, want %q", "G", action.Command, "echo global-legacy")
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: local G is content-identical to global G, only its document position (Order) differs -- that must not count as a strip", cfg.Notices)
	}

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("G")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Command != "echo global-legacy" {
		t.Fatalf("Lookup(normal, \"\", G) = %+v, want the global-equivalent legacy shell action to resolve", result)
	}
}

// --- Case: untrusted local actions: *anchor whose aliased content is
// global-equivalent -> not stripped, no notice. Aliasing the whole actions:
// block leaves every entry's Order at 0 (stampActionOrder's nodeKeysInOrder
// bails out on a non-MappingNode), so this is the sharper, alias-driven
// version of the Order-mismatch case above. ---

func TestLoad_Untrusted_LegacyActionAliasedIdenticalToGlobal_NotStrippedNoNotice(t *testing.T) {
	globalYAML := `
actions:
  G:
    name: Global Legacy Shell
    type: shell
    command: "echo global-legacy"
`
	localYAML := `
provider: github
repo: owner/repo
.anchor: &anc
  G:
    name: Global Legacy Shell
    type: shell
    command: "echo global-legacy"
actions: *anc
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	action, ok := cfg.Actions["G"]
	if !ok || action.Command != "echo global-legacy" {
		t.Fatalf("cfg.Actions[%q] = %+v, ok=%v, want the aliased-but-content-identical global action to survive", "G", action, ok)
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: the aliased local G is content-identical to global G", cfg.Notices)
	}

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("G")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Command != "echo global-legacy" {
		t.Fatalf("Lookup(normal, \"\", G) = %+v, want the aliased global-equivalent legacy shell action to resolve", result)
	}
}

// --- Case: the aliased companion to the above -- when the aliased local
// content genuinely DIFFERS from global, it must still be stripped and
// counted. A merge-key/alias must never bypass stripping. ---

func TestLoad_Untrusted_LegacyActionAliasedDiffering_StillStrippedAndCounted(t *testing.T) {
	globalYAML := `
actions:
  G:
    name: Global Legacy Shell
    type: shell
    command: "echo global-legacy"
`
	localYAML := `
provider: github
repo: owner/repo
.anchor: &anc
  G:
    name: Global Legacy Shell
    type: shell
    command: "rm -rf /"
actions: *anc
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	// Load()'s later merge step always re-populates a stripped key from
	// globalActions (see the "Merge actions" section in config.go), so
	// cfg.Actions["G"] existing here doesn't by itself prove anything -- the
	// content must be the global fallback, not the aliased-but-differing
	// local override, to prove the strip actually happened.
	action, ok := cfg.Actions["G"]
	if !ok || action.Command != "echo global-legacy" {
		t.Fatalf("cfg.Actions[%q] = %+v, ok=%v, want the aliased-but-differing local override stripped and replaced by the global fallback command", "G", action, ok)
	}
	if len(cfg.Notices) != 1 {
		t.Fatalf("cfg.Notices = %v, want exactly one notice for the single stripped aliased legacy action", cfg.Notices)
	}
	if !strings.Contains(cfg.Notices[0], "1 legacy shell action(s)") {
		t.Fatalf("cfg.Notices[0] = %q, want it to read %q", cfg.Notices[0], "1 legacy shell action(s)")
	}

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("G")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Command != "echo global-legacy" {
		t.Fatalf("Lookup(normal, \"\", G) = %+v, want fallback to the global command (aliased local override differs and must still be stripped)", result)
	}
}

// --- Case: per-column analog of the first case -- global column Refined
// declares A then G (Order 1, 2); untrusted local column Refined declares
// only a content-identical G (Order 1). Not stripped, no notice. ---

func TestLoad_Untrusted_LegacyColumnActionIdenticalToGlobal_DifferentDocPosition_NotStrippedNoNotice(t *testing.T) {
	globalYAML := `
columns:
  - name: Refined
    actions:
      A:
        name: Alpha
        type: shell
        command: "echo alpha"
      G:
        name: Global Column Legacy Shell
        type: shell
        command: "echo global-col-legacy"
`
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
    actions:
      G:
        name: Global Column Legacy Shell
        type: shell
        command: "echo global-col-legacy"
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	var refined *ColumnConfig
	for i := range cfg.Columns {
		if cfg.Columns[i].Name == "Refined" {
			refined = &cfg.Columns[i]
		}
	}
	if refined == nil {
		t.Fatalf("cfg.Columns has no %q entry: %+v", "Refined", cfg.Columns)
	}
	action, ok := refined.Actions["G"]
	if !ok || action.Command != "echo global-col-legacy" {
		t.Fatalf("Refined.Actions[%q] = %+v, ok=%v, want the global-equivalent local G to survive", "G", action, ok)
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: local column G is content-identical to global column G, only Order differs", cfg.Notices)
	}

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("G")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Command != "echo global-col-legacy" {
		t.Fatalf("Lookup(normal, Refined, G) = %+v, want the global-equivalent legacy column shell action to resolve", result)
	}
}

// --- Cross-cutting (#584 AC: notice pluralization/count aggregation stays
// correct alongside the other sinks): one config mixes a differing keymap
// binding, a differing legacy shell action, and a stripped cleanup: field.
// A single aggregated notice must name all three counts. ---

func TestLoad_Untrusted_MixedKeymapLegacyCleanupStrip_SingleAggregatedNotice(t *testing.T) {
	globalYAML := `
cleanup: "echo global-cleanup"
actions:
  G:
    name: Global Legacy Shell
    type: shell
    command: "echo global-legacy"
keymaps:
  normal:
    b: { name: Build, type: shell, scope: board, command: make }
`
	localYAML := `
provider: github
repo: owner/repo
cleanup: "rm -rf /"
actions:
  G:
    name: Local Evil Legacy
    type: shell
    command: "rm -rf /legacy"
keymaps:
  normal:
    b: { name: Build, type: shell, scope: board, command: "rm -rf /keymap" }
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if got := cfg.CleanupValue(); got != "echo global-cleanup" {
		t.Fatalf("cfg.CleanupValue() = %q, want the global cleanup after the differing local override is stripped", got)
	}
	if cfg.Actions["G"].Command != "echo global-legacy" {
		t.Fatalf("cfg.Actions[G].Command = %q, want the global legacy command after the differing local override is stripped", cfg.Actions["G"].Command)
	}

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Command != "make" {
		t.Fatalf("Lookup(normal, \"\", b) = %+v, want fallback to the global keymap command after the differing local override is stripped", result)
	}

	if len(cfg.Notices) != 1 {
		t.Fatalf("cfg.Notices = %v, want exactly one aggregated notice naming all three stripped sink kinds", cfg.Notices)
	}
	notice := cfg.Notices[0]
	for _, want := range []string{"1 keymap shell binding(s)", "1 legacy shell action(s)", "1 cleanup field(s)"} {
		if !strings.Contains(notice, want) {
			t.Fatalf("cfg.Notices[0] = %q, want it to contain %q", notice, want)
		}
	}
}
