package config

import (
	"reflect"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- Keymaps.Tables() feeding keymap.Resolve/Lookup (#509) ---

func TestKeymaps_Tables_ConvertsCommandBinding(t *testing.T) {
	localYAML := `keymaps:
  normal:
    n: card.create
`
	cfg := mustLoadConfig(t, "", localYAML)
	tables := cfg.Keymaps.Tables()

	km, err := keymap.Resolve(keymap.Tables{}, tables)
	if err != nil {
		t.Fatalf("keymap.Resolve() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{"n"})
	if result.Outcome != keymap.OutcomeMatch {
		t.Fatalf("Lookup() outcome = %v, want OutcomeMatch", result.Outcome)
	}
	if result.Binding.Kind != keymap.BindingCommand || result.Binding.Command != "card.create" {
		t.Errorf("Lookup() binding = %+v, want CommandBinding(%q)", result.Binding, "card.create")
	}
}

func TestKeymaps_Tables_ConvertsInlineActionBinding(t *testing.T) {
	localYAML := `keymaps:
  normal:
    W:
      name: Widget
      type: shell
      command: "echo widget"
`
	cfg := mustLoadConfig(t, "", localYAML)
	tables := cfg.Keymaps.Tables()

	km, err := keymap.Resolve(keymap.Tables{}, tables)
	if err != nil {
		t.Fatalf("keymap.Resolve() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{"W"})
	if result.Outcome != keymap.OutcomeMatch {
		t.Fatalf("Lookup() outcome = %v, want OutcomeMatch", result.Outcome)
	}
	if result.Binding.Kind != keymap.BindingAction {
		t.Fatalf("Lookup() binding.Kind = %v, want BindingAction", result.Binding.Kind)
	}
	if result.Binding.Action.Name != "Widget" || result.Binding.Action.Command != "echo widget" {
		t.Errorf("Lookup() binding.Action = %+v, want the inline action fields to survive conversion", result.Binding.Action)
	}
}

// TestKeymaps_Tables_DropsActionOrderMetadata pins the #492 deferral: Order
// is derived config-layer metadata (document position, used for hint
// ordering) that keymap.Binding has no concept of yet. Tables() must not
// leak it into the engine layer even though keymap.Action happens to have an
// Order field for cross-package tag-compatibility with config.Action.
func TestKeymaps_Tables_DropsActionOrderMetadata(t *testing.T) {
	localYAML := `keymaps:
  normal:
    W:
      name: Widget
      type: shell
      command: "echo widget"
    X:
      name: Extra
      type: shell
      command: "echo extra"
`
	cfg := mustLoadConfig(t, "", localYAML)
	if cfg.Keymaps.Modes[keymap.ModeNormal]["W"].Order == cfg.Keymaps.Modes[keymap.ModeNormal]["X"].Order {
		t.Fatal("test setup invalid: Load() did not assign distinct Order to the two inline-action bindings")
	}

	tables := cfg.Keymaps.Tables()
	km, err := keymap.Resolve(keymap.Tables{}, tables)
	if err != nil {
		t.Fatalf("keymap.Resolve() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{"W"})
	if result.Outcome != keymap.OutcomeMatch {
		t.Fatalf("Lookup() outcome = %v, want OutcomeMatch", result.Outcome)
	}
	if result.Binding.Action.Order != 0 {
		t.Errorf("Lookup() binding.Action.Order = %d, want 0 (Tables() must drop Order metadata, deferred to #492)", result.Binding.Action.Order)
	}
}

func TestKeymaps_Tables_ConvertsColumnsOverlay(t *testing.T) {
	localYAML := `keymaps:
  columns:
    Implementing:
      n: card.create.implementing
`
	cfg := mustLoadConfig(t, "", localYAML)
	tables := cfg.Keymaps.Tables()

	km, err := keymap.Resolve(keymap.Tables{}, tables)
	if err != nil {
		t.Fatalf("keymap.Resolve() returned unexpected error: %v", err)
	}
	// Resolve's column matching is case-insensitive.
	result := km.Lookup(keymap.ModeNormal, "implementing", keymap.Sequence{"n"})
	if result.Outcome != keymap.OutcomeMatch {
		t.Fatalf("Lookup() outcome = %v, want OutcomeMatch for the column overlay", result.Outcome)
	}
	if result.Binding.Command != "card.create.implementing" {
		t.Errorf("Lookup() binding.Command = %q, want %q", result.Binding.Command, "card.create.implementing")
	}
}

// TestKeymaps_Tables_ExplicitUnbindOverridesDefault pins the merge precedence
// the lessons-learned entry on combined-axis precedence warns about: a
// user's explicit unbind must survive Tables()+Resolve and win over a
// defaults-layer command binding for the same key, not get silently
// re-bound by the default.
func TestKeymaps_Tables_ExplicitUnbindOverridesDefault(t *testing.T) {
	localYAML := `keymaps:
  normal:
    n: ~
`
	cfg := mustLoadConfig(t, "", localYAML)
	tables := cfg.Keymaps.Tables()

	defaults := keymap.Tables{Modes: map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"n": keymap.CommandBinding("card.create")},
	}}
	km, err := keymap.Resolve(defaults, tables)
	if err != nil {
		t.Fatalf("keymap.Resolve() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{"n"})
	if result.Outcome != keymap.OutcomeNoMatch {
		t.Errorf("Lookup() outcome = %v, want OutcomeNoMatch (the user's explicit unbind must override the default binding)", result.Outcome)
	}
}

// TestConfigAction_KeymapAction_FieldsStayInSync pins the hand-kept
// cross-package contract binding.go documents: config.Action and
// keymap.Action must stay field-name/type/yaml-tag identical so #509's
// conversion is a straight field copy. internal/keymap cannot import
// internal/config (that would invert the config -> keymap edge), so this
// test lives here, the one package allowed to import both.
func TestConfigAction_KeymapAction_FieldsStayInSync(t *testing.T) {
	configType := reflect.TypeOf(Action{})
	keymapType := reflect.TypeOf(keymap.Action{})

	if configType.NumField() != keymapType.NumField() {
		t.Fatalf("config.Action has %d fields, keymap.Action has %d; they must stay field-for-field in sync",
			configType.NumField(), keymapType.NumField())
	}
	for i := 0; i < configType.NumField(); i++ {
		cf := configType.Field(i)
		kf, ok := keymapType.FieldByName(cf.Name)
		if !ok {
			t.Errorf("keymap.Action is missing field %q present on config.Action", cf.Name)
			continue
		}
		if cf.Type != kf.Type {
			t.Errorf("field %q: config.Action type = %v, keymap.Action type = %v, want identical", cf.Name, cf.Type, kf.Type)
		}
		if cf.Tag.Get("yaml") != kf.Tag.Get("yaml") {
			t.Errorf("field %q: config.Action yaml tag = %q, keymap.Action yaml tag = %q, want identical",
				cf.Name, cf.Tag.Get("yaml"), kf.Tag.Get("yaml"))
		}
	}
}
