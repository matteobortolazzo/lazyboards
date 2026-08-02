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
	// Uses "z" (unused by any default normal-mode binding, #502) rather than
	// "g" (the #502 go-prefix, itself a default prefix of "g a"/"g r") so
	// this generic prefix-conflict mechanism test stays isolated from the
	// specific default-remap collision scenarios (see
	// TestLoad_UserSequenceUnderRemappedDefaultPrefix_IsAnError below).
	yamlContent := `provider: github
keymaps:
  normal:
    z: board.refresh
    "z d": card.delete
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for key \"z\" being a prefix of key \"z d\" in normal mode")
	}
	if !strings.Contains(err.Error(), `"z"`) || !strings.Contains(err.Error(), `"z d"`) {
		t.Errorf("error = %q, want it to reference both conflicting keys \"z\" and \"z d\"", err.Error())
	}
	if !strings.Contains(err.Error(), "normal") {
		t.Errorf("error = %q, want it to name the mode \"normal\"", err.Error())
	}
}

func TestLoad_KeymapPrefixConflict_Column_ReturnsError(t *testing.T) {
	// See TestLoad_KeymapPrefixConflict_Normal_ReturnsError for why "z" is
	// used instead of "g".
	yamlContent := `provider: github
keymaps:
  columns:
    Doing:
      z: board.refresh
      "z d": card.delete
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for key \"z\" being a prefix of key \"z d\" scoped to column Doing")
	}
	if !strings.Contains(err.Error(), `"z"`) || !strings.Contains(err.Error(), `"z d"`) {
		t.Errorf("error = %q, want it to reference both conflicting keys \"z\" and \"z d\"", err.Error())
	}
	if !strings.Contains(err.Error(), "Doing") {
		t.Errorf("error = %q, want it to name the column \"Doing\"", err.Error())
	}
}

func TestLoad_KeymapPrefixConflict_UnboundPrefix_ClearsConflict(t *testing.T) {
	// Unbinding the prefix key with ~ removes it from consideration, so the
	// otherwise-conflicting continuation key loads clean. See
	// TestLoad_KeymapPrefixConflict_Normal_ReturnsError for why "z" is used
	// instead of "g".
	yamlContent := `provider: github
keymaps:
  normal:
    z: ~
    "z d": card.delete
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error after unbinding the prefix key: %v", err)
	}
}

// --- #502: user sequences colliding with the remapped default prefixes ---

// TestLoad_UserSequenceUnderRemappedDefaultPrefix_IsAnError covers the
// specific collision #502 introduces: "P", "D", "A", and "G" now ship as
// exact-match built-in defaults, so a user config binding a sequence under
// any of them as a prefix (without first unbinding the colliding default)
// must be rejected -- the default key can never dispatch once a longer
// sequence shares its prefix. Also covers the bare-"g" case: "g" is not
// itself a bound default, but it is a default PREFIX ("g a"/"g r"), so
// binding "g" directly to a command must be rejected the same way -- here
// the collision runs the other direction (the user's bare key collides with
// the pre-existing default sequences), so the config under test binds only
// the prefix key itself, not a new sibling sequence.
func TestLoad_UserSequenceUnderRemappedDefaultPrefix_IsAnError(t *testing.T) {
	cases := []struct {
		name        string
		prefixKey   string
		conflictKey string
		yamlBody    string
	}{
		{"P", "P", "P f", `"P f": card.new`},
		{"D", "D", "D f", `"D f": card.new`},
		{"A", "A", "A f", `"A f": card.new`},
		{"G", "G", "G f", `"G f": card.new`},
		{"bare g", "g", "g a", `g: card.new`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			yamlContent := `provider: github
keymaps:
  normal:
    ` + tc.yamlBody + `
`
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for key %q colliding with the default prefix %q", tc.conflictKey, tc.prefixKey)
			}
			if !strings.Contains(err.Error(), `"`+tc.prefixKey+`"`) || !strings.Contains(err.Error(), `"`+tc.conflictKey+`"`) {
				t.Errorf("error = %q, want it to reference both %q and %q", err.Error(), tc.prefixKey, tc.conflictKey)
			}
		})
	}
}

// TestLoad_UnbindingCollidingDefaultClearsPrefixConflict covers the
// documented fix for the collision above: explicitly unbinding the
// colliding default prefix key (e.g. "P: ~") lets the user's sequence load
// clean, and ResolveKeymap then resolves the sequence to the user's own
// action while the bare prefix key itself resolves to OutcomePending (its
// own default command is gone, but "P f" is still bound, so pressing bare
// "P" now waits for the continuation instead of dispatching immediately).
func TestLoad_UnbindingCollidingDefaultClearsPrefixConflict(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    P: ~
    "P f":
      name: Custom PR frontend
      type: url
      url: "https://example.com/frontend"
`
	cfg := mustLoadConfig(t, yamlContent, "")

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	seq, err := keymap.ParseSequence("P f")
	if err != nil {
		t.Fatalf("ParseSequence(\"P f\") error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", seq)
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Name != "Custom PR frontend" {
		t.Errorf("Lookup(ModeNormal, \"\", \"P f\") = %+v, want an OutcomeMatch BindingAction(\"Custom PR frontend\")", result)
	}

	bareSeq, err := keymap.ParseSequence("P")
	if err != nil {
		t.Fatalf("ParseSequence(\"P\") error: %v", err)
	}
	bareResult := km.Lookup(keymap.ModeNormal, "", bareSeq)
	if bareResult.Outcome != keymap.OutcomePending {
		t.Errorf("Lookup(ModeNormal, \"\", \"P\") outcome = %v after unbinding the default, want OutcomePending (\"P f\" is still bound)", bareResult.Outcome)
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
