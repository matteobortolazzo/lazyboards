package config

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- keymaps.<mode>.<key> parsing (#509) ---

func TestLoad_KeymapCommandString_ParsesAsCommandBinding(t *testing.T) {
	localYAML := `keymaps:
  normal:
    n: card.create
`
	result := mustLoadConfig(t, "", localYAML)

	if result.Keymaps == nil {
		t.Fatal("Keymaps is nil, want non-nil after a keymaps: block is configured")
	}
	table, ok := result.Keymaps.Modes[keymap.ModeNormal]
	if !ok {
		t.Fatal("Keymaps.Modes missing \"normal\" entry")
	}
	binding, ok := table["n"]
	if !ok {
		t.Fatal("Keymaps.Modes[normal] missing key \"n\"")
	}
	if binding.Kind != keymap.BindingCommand {
		t.Fatalf("Modes[normal][n].Kind = %v, want BindingCommand", binding.Kind)
	}
	if binding.Command != "card.create" {
		t.Errorf("Modes[normal][n].Command = %q, want %q", binding.Command, "card.create")
	}
}

func TestLoad_KeymapMapping_ParsesAsInlineActionBinding(t *testing.T) {
	localYAML := `keymaps:
  normal:
    W:
      name: Custom widget
      type: shell
      command: "echo widget"
`
	result := mustLoadConfig(t, "", localYAML)

	binding := result.Keymaps.Modes[keymap.ModeNormal]["W"]
	if binding.Kind != keymap.BindingAction {
		t.Fatalf("Modes[normal][W].Kind = %v, want BindingAction", binding.Kind)
	}
	if binding.Action.Name != "Custom widget" || binding.Action.Type != "shell" || binding.Action.Command != "echo widget" {
		t.Errorf("Modes[normal][W].Action = %+v, want inline action fields to round-trip", binding.Action)
	}
}

func TestLoad_KeymapTildeUnbind_ParsesAsExplicitUnbind(t *testing.T) {
	localYAML := `keymaps:
  normal:
    n: ~
`
	result := mustLoadConfig(t, "", localYAML)

	binding := result.Keymaps.Modes[keymap.ModeNormal]["n"]
	if binding.Kind != keymap.BindingUnbound {
		t.Errorf("Modes[normal][n].Kind = %v, want BindingUnbound for a tilde value", binding.Kind)
	}
}

func TestLoad_KeymapNullUnbind_ParsesAsExplicitUnbind(t *testing.T) {
	localYAML := `keymaps:
  normal:
    n: null
`
	result := mustLoadConfig(t, "", localYAML)

	binding := result.Keymaps.Modes[keymap.ModeNormal]["n"]
	if binding.Kind != keymap.BindingUnbound {
		t.Errorf("Modes[normal][n].Kind = %v, want BindingUnbound for a null value", binding.Kind)
	}
}

func TestLoad_KeymapUnknownMode_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  bogus_mode:
    n: card.create
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for unknown keymap mode \"bogus_mode\"")
	}
	if !strings.Contains(err.Error(), "bogus_mode") {
		t.Errorf("error = %q, want it to name the offending mode %q", err.Error(), "bogus_mode")
	}
}

func TestLoad_KeymapUnparseableRHS_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    zseq:
      - one
      - two
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an unparseable keymap binding")
	}
	if !strings.Contains(err.Error(), "normal") || !strings.Contains(err.Error(), "zseq") {
		t.Errorf("error = %q, want it to name both the offending mode %q and key %q", err.Error(), "normal", "zseq")
	}
}

func TestLoad_KeymapDuplicateKeyInTable_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    n: card.create
    n: card.close
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a duplicate key within one keymap table")
	}
	if !strings.Contains(err.Error(), "n") {
		t.Errorf("error = %q, want it to name the offending key %q", err.Error(), "n")
	}
}

func TestLoad_KeymapColumnsDuplicateNameCaseInsensitive_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  columns:
    Backlog:
      n: card.create.in.column
    backlog:
      e: card.edit.in.column
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for keymap columns whose names collide case-insensitively")
	}
	if !strings.Contains(err.Error(), "Backlog") || !strings.Contains(err.Error(), "backlog") {
		t.Errorf("error = %q, want it to name both colliding column names %q and %q", err.Error(), "Backlog", "backlog")
	}
}

func TestLoad_KeymapColumnsOverlay_ParsesPerColumnTable(t *testing.T) {
	localYAML := `keymaps:
  columns:
    Implementing:
      n: card.create.in.column
`
	result := mustLoadConfig(t, "", localYAML)

	if result.Keymaps == nil {
		t.Fatal("Keymaps is nil, want non-nil after a keymaps.columns block is configured")
	}
	table, ok := result.Keymaps.Columns["Implementing"]
	if !ok {
		t.Fatal("Keymaps.Columns missing \"Implementing\" entry")
	}
	binding, ok := table["n"]
	if !ok {
		t.Fatal("Keymaps.Columns[\"Implementing\"] missing key \"n\"")
	}
	if binding.Kind != keymap.BindingCommand || binding.Command != "card.create.in.column" {
		t.Errorf("Keymaps.Columns[Implementing][n] = %+v, want CommandBinding(%q)", binding, "card.create.in.column")
	}
}

// --- Order stamping (mirrors action_order_test.go's convention) ---

func TestLoad_KeymapOrder_SequentialFromYAMLPosition(t *testing.T) {
	localYAML := `keymaps:
  normal:
    z: card.zebra
    a: card.apple
    m: card.mango
`
	result := mustLoadConfig(t, "", localYAML)

	table := result.Keymaps.Modes[keymap.ModeNormal]
	if table["z"].Order >= table["a"].Order {
		t.Errorf("Modes[normal][z].Order = %d, want < Modes[normal][a].Order = %d", table["z"].Order, table["a"].Order)
	}
	if table["a"].Order >= table["m"].Order {
		t.Errorf("Modes[normal][a].Order = %d, want < Modes[normal][m].Order = %d", table["a"].Order, table["m"].Order)
	}
}

// --- Global/local merge: per-mode tables ---

func TestLoad_KeymapModeMerge_LocalOverridesGlobalKey(t *testing.T) {
	globalYAML := `provider: github
keymaps:
  normal:
    n: card.create.global
`
	localYAML := `keymaps:
  normal:
    n: card.create.local
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	binding := result.Keymaps.Modes[keymap.ModeNormal]["n"]
	if binding.Command != "card.create.local" {
		t.Errorf("Modes[normal][n].Command = %q, want %q (local should override global)", binding.Command, "card.create.local")
	}
}

func TestLoad_KeymapModeMerge_GlobalOnlyKeyPreserved(t *testing.T) {
	globalYAML := `provider: github
keymaps:
  normal:
    n: card.create
    e: card.edit
`
	localYAML := `keymaps:
  normal:
    n: card.create.override
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	table := result.Keymaps.Modes[keymap.ModeNormal]
	if len(table) != 2 {
		t.Fatalf("Modes[normal] entry count = %d, want 2 (local override + global-only key)", len(table))
	}
	if binding, ok := table["e"]; !ok || binding.Command != "card.edit" {
		t.Errorf("Modes[normal][e] = %+v, want the global-only binding preserved", binding)
	}
}

func TestLoad_KeymapModeMerge_OmittedModeTable_InheritsGlobalEntries(t *testing.T) {
	globalYAML := `provider: github
keymaps:
  normal:
    n: card.create
`
	// Local declares a keymaps: block but never mentions "normal" at all.
	localYAML := `keymaps:
  detail:
    e: card.edit
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	table, ok := result.Keymaps.Modes[keymap.ModeNormal]
	if !ok || len(table) != 1 {
		t.Fatalf("Modes[normal] = %v, want the inherited global entry (mode omitted locally)", table)
	}
	if table["n"].Kind != keymap.BindingCommand || table["n"].Command != "card.create" {
		t.Errorf("Modes[normal][n] = %+v, want the inherited global command binding", table["n"])
	}
}

func TestLoad_KeymapModeMerge_ExplicitEmptyModeTable_ClearsInheritedGlobalKeys(t *testing.T) {
	globalYAML := `provider: github
keymaps:
  normal:
    n: card.create
`
	localYAML := `keymaps:
  normal: {}
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	table, ok := result.Keymaps.Modes[keymap.ModeNormal]
	if !ok {
		t.Fatal("Modes missing \"normal\" entry after an explicit empty mode table")
	}
	if len(table) != 0 {
		t.Errorf("Modes[normal] = %v, want empty (explicit {} must not inherit global entries)", table)
	}
}

func TestLoad_KeymapModeMerge_LocalExplicitUnbindOverridesGlobalCommand(t *testing.T) {
	globalYAML := `provider: github
keymaps:
  normal:
    n: card.create
`
	localYAML := `keymaps:
  normal:
    n: ~
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	table := result.Keymaps.Modes[keymap.ModeNormal]
	binding, ok := table["n"]
	if !ok {
		t.Fatal("Modes[normal] missing key \"n\" after merge")
	}
	if binding.Kind != keymap.BindingUnbound {
		t.Errorf("Modes[normal][n].Kind = %v, want BindingUnbound (local explicit unbind must override the inherited global command)", binding.Kind)
	}
}

// --- Global/local merge: keymaps.columns.<name> overlay tables ---

func TestLoad_KeymapColumnsMerge_NilInheritsGlobalColumnTable(t *testing.T) {
	globalYAML := `provider: github
keymaps:
  columns:
    Implementing:
      n: card.create.implementing
`
	// Local declares keymaps but no columns.Implementing entry at all.
	localYAML := `keymaps:
  normal:
    e: card.edit
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	table, ok := result.Keymaps.Columns["Implementing"]
	if !ok || len(table) != 1 {
		t.Fatalf("Columns[Implementing] = %v, want the inherited global entry", table)
	}
	if table["n"].Command != "card.create.implementing" {
		t.Errorf("Columns[Implementing][n].Command = %q, want %q", table["n"].Command, "card.create.implementing")
	}
}

func TestLoad_KeymapColumnsMerge_ExplicitEmptyMapGetsNone(t *testing.T) {
	globalYAML := `provider: github
keymaps:
  columns:
    Implementing:
      n: card.create.implementing
`
	localYAML := `keymaps:
  columns:
    Implementing: {}
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	table, ok := result.Keymaps.Columns["Implementing"]
	if !ok {
		t.Fatal("Columns missing \"Implementing\" entry")
	}
	if len(table) != 0 {
		t.Errorf("Columns[Implementing] = %v, want empty (explicit {} must not inherit global entries)", table)
	}
}

func TestLoad_KeymapColumnsMerge_NonEmptyMergesGlobalOnlyKeysAfterLocal(t *testing.T) {
	globalYAML := `provider: github
keymaps:
  columns:
    Implementing:
      d: card.delete.implementing
`
	localYAML := `keymaps:
  columns:
    Implementing:
      b: card.branch.implementing
`
	result := mustLoadConfig(t, globalYAML, localYAML)

	table := result.Keymaps.Columns["Implementing"]
	if len(table) != 2 {
		t.Fatalf("Columns[Implementing] entry count = %d, want 2 (local 'b' + global-only 'd')", len(table))
	}
	if _, ok := table["b"]; !ok {
		t.Error("Columns[Implementing] missing local key \"b\"")
	}
	if _, ok := table["d"]; !ok {
		t.Error("Columns[Implementing] missing global-only key \"d\"")
	}
	if table["b"].Order >= table["d"].Order {
		t.Errorf("Columns[Implementing][b].Order = %d, want < Columns[Implementing][d].Order = %d (local key must precede global-only key)",
			table["b"].Order, table["d"].Order)
	}
}

// --- Back-compat: no keymaps: block at all ---

func TestLoad_NoKeymapsBlock_KeymapsFieldStaysNil(t *testing.T) {
	yamlContent := `provider: github
repo: owner/repo
`
	result := mustLoadConfig(t, yamlContent, "")

	if result.Keymaps != nil {
		t.Errorf("Keymaps = %+v, want nil when no keymaps: block is configured", result.Keymaps)
	}
}
