package config

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- #643: trust_confirm keymap registry scaffolding ---
//
// trust_confirm is cataloged ahead of its runtime wiring (#644): nothing in
// package main produces this mode yet (see internal/keymap/mode.go's
// ModeTrustConfirm doc comment), so there is no Board/handler to dispatch
// through here -- these tests instead prove the config layer (Load,
// ResolveKeymap, validateModeCapabilities) treats keymaps.trust_confirm
// exactly like every other bindable mode's config surface: defaults
// resolve, a user override wins, an explicit unbind is honoured, and a
// command id foreign to the mode is rejected at load time rather than
// silently no-opped at runtime.

// trustConfirmKeymap loads localYAML (as the global config -- trust_confirm
// bindings are BindingCommand, never a shell sink, so global-vs-local is
// immaterial here, mirroring capabilityCommandYAML's own convention above)
// through the real Load -> ResolveKeymap pipeline, failing the test on any
// unexpected error.
func trustConfirmKeymap(t *testing.T, yamlContent string) *keymap.Keymap {
	t.Helper()
	cfg, err := loadConfigFromStrings(t, yamlContent, "")
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}
	return km
}

// TestLoad_TrustConfirm_DefaultsResolve pins the built-in t/s/esc table
// (internal/keymap/defaults_text.go's trustConfirmDefaults) with no user
// override present.
func TestLoad_TrustConfirm_DefaultsResolve(t *testing.T) {
	km := trustConfirmKeymap(t, "provider: github\n")

	cases := []struct {
		key  string
		want keymap.CommandID
	}{
		{"t", keymap.CommandTrustConfirmTrust},
		{"s", keymap.CommandTrustConfirmSkip},
		{"esc", keymap.CommandTrustConfirmSkip},
	}
	for _, tc := range cases {
		result := km.Lookup(keymap.ModeTrustConfirm, "", keymap.Sequence{keymap.Key(tc.key)})
		if result.Outcome != keymap.OutcomeMatch {
			t.Fatalf("Lookup(trust_confirm, %q) outcome = %v, want OutcomeMatch", tc.key, result.Outcome)
		}
		if result.Binding.Command != tc.want {
			t.Errorf("Lookup(trust_confirm, %q) = %q, want %q", tc.key, result.Binding.Command, tc.want)
		}
	}
}

// TestLoad_TrustConfirm_UserOverrideWins pins that a keymaps.trust_confirm
// entry replaces the default binding for that key.
func TestLoad_TrustConfirm_UserOverrideWins(t *testing.T) {
	km := trustConfirmKeymap(t, `provider: github
keymaps:
  trust_confirm:
    t: trust_confirm.skip
`)
	result := km.Lookup(keymap.ModeTrustConfirm, "", keymap.Sequence{keymap.Key("t")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Command != keymap.CommandTrustConfirmSkip {
		t.Errorf("Lookup(trust_confirm, \"t\") = %+v, want the override to trust_confirm.skip", result)
	}
}

// TestLoad_TrustConfirm_ExplicitUnbindHonoured pins that a "~" entry removes
// only that key -- the mode's other default keys stay intact.
func TestLoad_TrustConfirm_ExplicitUnbindHonoured(t *testing.T) {
	km := trustConfirmKeymap(t, `provider: github
keymaps:
  trust_confirm:
    s: ~
`)
	result := km.Lookup(keymap.ModeTrustConfirm, "", keymap.Sequence{keymap.Key("s")})
	if result.Outcome != keymap.OutcomeNoMatch {
		t.Errorf("Lookup(trust_confirm, \"s\") outcome = %v after unbind, want OutcomeNoMatch", result.Outcome)
	}

	result = km.Lookup(keymap.ModeTrustConfirm, "", keymap.Sequence{keymap.Key("esc")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Command != keymap.CommandTrustConfirmSkip {
		t.Errorf("Lookup(trust_confirm, \"esc\") = %+v, want it unaffected by the \"s\" unbind", result)
	}
}

// TestLoad_TrustConfirm_ForeignCommandRejectedAtLoad pins AC 3: a command id
// valid elsewhere in the catalog but foreign to trust_confirm must fail
// Load() itself, not resolve to a runtime no-op.
func TestLoad_TrustConfirm_ForeignCommandRejectedAtLoad(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  trust_confirm:
    z: nav.cursor_down
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a command foreign to trust_confirm")
	}
	assertCapabilityError(t, err, "keymaps.trust_confirm", `"z"`, "nav.cursor_down")
	if !strings.Contains(err.Error(), "trust_confirm") {
		t.Errorf("error = %q, want it to name mode %q", err.Error(), "trust_confirm")
	}
}
