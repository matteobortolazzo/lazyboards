package config

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- Defaults guard ---

func TestValidateKeymap_BuiltInDefaultsAlone_NoError(t *testing.T) {
	// The built-in default tables alone (no user keymaps: at all) must
	// always pass validateKeymap. This is the regression guard named in the
	// ticket: a naive raw strings.HasPrefix scan (instead of a
	// whitespace-boundary one) would wrongly flag e/esc (detail),
	// s/shift+tab, u/up, d/down (normal) as prefix conflicts.
	if err := validateKeymap(&Config{}); err != nil {
		t.Fatalf("validateKeymap(defaults only) returned error, want nil: %v", err)
	}
}

// --- Prefix conflicts ---

func TestLoad_KeymapPrefixConflict_Normal_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    g: board.refresh
    "g d": card.delete
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for key \"g\" being a prefix of key \"g d\" in normal mode")
	}
	if !strings.Contains(err.Error(), `"g"`) || !strings.Contains(err.Error(), `"g d"`) {
		t.Errorf("error = %q, want it to reference both conflicting keys \"g\" and \"g d\"", err.Error())
	}
	if !strings.Contains(err.Error(), "normal") {
		t.Errorf("error = %q, want it to name the mode \"normal\"", err.Error())
	}
}

func TestLoad_KeymapPrefixConflict_Column_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  columns:
    Doing:
      g: board.refresh
      "g d": card.delete
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for key \"g\" being a prefix of key \"g d\" scoped to column Doing")
	}
	if !strings.Contains(err.Error(), `"g"`) || !strings.Contains(err.Error(), `"g d"`) {
		t.Errorf("error = %q, want it to reference both conflicting keys \"g\" and \"g d\"", err.Error())
	}
	if !strings.Contains(err.Error(), "Doing") {
		t.Errorf("error = %q, want it to name the column \"Doing\"", err.Error())
	}
}

func TestLoad_KeymapPrefixConflict_UnboundPrefix_ClearsConflict(t *testing.T) {
	// Unbinding the prefix key with ~ removes it from consideration, so the
	// otherwise-conflicting continuation key loads clean.
	yamlContent := `provider: github
keymaps:
  normal:
    g: ~
    "g d": card.delete
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error after unbinding the prefix key: %v", err)
	}
}

// --- ctrl+c rejection ---

func TestLoad_KeymapCtrlC_DirectBinding_ReturnsError(t *testing.T) {
	// A non-normal mode, per the ticket's explicit "in a non-normal mode
	// too" requirement.
	yamlContent := `provider: github
keymaps:
  search:
    ctrl+c: board.refresh
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a direct ctrl+c binding")
	}
	if !strings.Contains(err.Error(), "ctrl+c") {
		t.Errorf("error = %q, want it to mention ctrl+c", err.Error())
	}
}

func TestLoad_KeymapCtrlC_SequenceContinuation_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    "g ctrl+c": board.refresh
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for ctrl+c as a sequence continuation key")
	}
	if !strings.Contains(err.Error(), "ctrl+c") {
		t.Errorf("error = %q, want it to mention ctrl+c", err.Error())
	}
}

func TestLoad_KeymapCtrlC_ExplicitUnbind_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  search:
    ctrl+c: ~
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an explicit ctrl+c unbind")
	}
	if !strings.Contains(err.Error(), "ctrl+c") {
		t.Errorf("error = %q, want it to mention ctrl+c", err.Error())
	}
}

// --- Lowercase custom / uppercase built-in coexistence ---

func TestLoad_LowercaseCustomActionAndUppercaseBuiltIn_LoadCleanly(t *testing.T) {
	// #510 removed the "custom action keys must start with A-Z" rule: a
	// lowercase key bound to a custom action and an uppercase key bound to
	// a built-in command must both load with no error.
	yamlContent := `provider: github
actions:
  z:
    name: Custom
    type: url
    url: "https://example.com"
keymaps:
  normal:
    Z: board.refresh
`
	result := mustLoadConfig(t, yamlContent, "")

	table, ok := result.Keymaps.Modes[keymap.ModeNormal]
	if !ok {
		t.Fatal("Keymaps.Modes missing \"normal\" entry")
	}
	if _, ok := table["z"]; !ok {
		t.Fatal("expected legacy-translated lowercase key \"z\" in keymaps.normal")
	}
	if _, ok := table["Z"]; !ok {
		t.Fatal("expected native uppercase key \"Z\" in keymaps.normal")
	}
}
