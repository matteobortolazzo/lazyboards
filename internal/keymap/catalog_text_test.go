package keymap

import (
	"strings"
	"testing"
)

// textModes lists the seven confirm/text-input Mode surfaces #538
// catalogues: close_confirm, label_confirm, delete, create, config, search
// and comment -- the gap left when #508's PR 2/2 was never delivered.
// Mirrors pr1Modes (catalog_pr1_test.go).
var textModes = []Mode{
	ModeCloseConfirm,
	ModeLabelConfirm,
	ModeDelete,
	ModeCreate,
	ModeConfig,
	ModeSearch,
	ModeComment,
}

// TestDefaults_TextModesPopulated pins that Defaults() actually carries a
// non-empty default Table for every one of the seven text-input modes --
// without this, the mode-prefix and round-trip invariants below would pass
// vacuously (an empty table has nothing to violate them), masking a mode
// nobody ever wired up. Mirrors TestDefaults_ModalModesPopulated
// (catalog_pr1_test.go).
func TestDefaults_TextModesPopulated(t *testing.T) {
	defaults := Defaults()
	for _, mode := range textModes {
		if len(defaults.Modes[mode]) == 0 {
			t.Errorf("Defaults().Modes[%q] is empty, want a populated default table", mode)
		}
	}
}

// TestDefaults_TextCommandIDsAreModePrefixed asserts that every command
// binding in a text-input mode's default table resolves to an id prefixed
// by that mode's own name ("create.submit", "search.next_result", ...) --
// unlike the PR-1 modal modes, none of the seven text-input modes reuses
// app.quit or dispatches an inline Action, so there is no allow-listed
// exception here. Mirrors TestDefaults_ModalCommandIDsAreModePrefixed
// (catalog_pr1_test.go).
func TestDefaults_TextCommandIDsAreModePrefixed(t *testing.T) {
	defaults := Defaults()
	for _, mode := range textModes {
		prefix := string(mode) + "."
		for seq, binding := range defaults.Modes[mode] {
			if binding.Kind != BindingCommand {
				t.Errorf("Defaults().Modes[%q][%q] has Kind %v, want BindingCommand -- no text-input mode dispatches an inline Action", mode, seq, binding.Kind)
				continue
			}
			if !strings.HasPrefix(string(binding.Command), prefix) {
				t.Errorf("Defaults().Modes[%q][%q] resolves to %q, want it prefixed with %q", mode, seq, binding.Command, prefix)
			}
		}
	}
}

// TestDefaults_TextKeysRoundTripCanonicalForm pins that every raw key bound
// in a text-input mode's default table is already in ParseSequence's
// canonical form -- a key stored some other way (e.g. "Esc" instead of
// "esc", or "Shift+Tab" instead of "shift+tab") would silently never match
// Lookup's canonicalized query. Mirrors TestDefaults_ModalKeysRoundTripCanonicalForm
// (catalog_pr1_test.go).
func TestDefaults_TextKeysRoundTripCanonicalForm(t *testing.T) {
	defaults := Defaults()
	for _, mode := range textModes {
		for rawKey := range defaults.Modes[mode] {
			seq, err := ParseSequence(rawKey)
			if err != nil {
				t.Errorf("Defaults().Modes[%q] has key %q, which fails ParseSequence: %v", mode, rawKey, err)
				continue
			}
			if got := seq.String(); got != rawKey {
				t.Errorf("Defaults().Modes[%q] has key %q, which does not round-trip (ParseSequence(...).String() = %q)", mode, rawKey, got)
			}
		}
	}
}

// TestCatalogue_TextCommandIDsHaveDesc asserts every CommandID the
// text-input default bindings resolve to (textBindingCommandIDs,
// catalog_text_bindings_test.go) is catalogued via FindCommand with a
// non-empty Desc -- a default binding pointing at an uncatalogued id would
// resolve at runtime but render with no desc anywhere (help modal,
// which-key, #492). Mirrors TestCatalogue_ModalCommandIDsHaveDesc
// (catalog_pr1_test.go).
func TestCatalogue_TextCommandIDsHaveDesc(t *testing.T) {
	ids := textBindingCommandIDs()
	if len(ids) == 0 {
		t.Fatal("textBindingCommandIDs() returned no ids -- catalog_text_bindings_test.go's table is empty or broken")
	}
	for _, id := range ids {
		cmd, ok := FindCommand(id)
		if !ok {
			t.Errorf("FindCommand(%q) returned ok == false, want a catalogued Command", id)
			continue
		}
		if cmd.Desc == "" {
			t.Errorf("FindCommand(%q).Desc is empty, want a human-readable description", id)
		}
	}
}

// TestDefaults_EveryResolvableModeHasDefaults is the AC-3 full-coverage
// assertion: with #538 completing the catalogue that #508's missing PR 2/2
// left as a gap, Defaults() must carry a non-empty table for every mode in
// Modes() -- with no ModeError carve-out. The carve-out AC 3 originally
// described is stale: errorDefaults already landed in #508 PR 1
// (defaults_system.go), so the stronger, unconditional assertion holds.
func TestDefaults_EveryResolvableModeHasDefaults(t *testing.T) {
	defaults := Defaults()
	for _, mode := range Modes() {
		if len(defaults.Modes[mode]) == 0 {
			t.Errorf("Defaults().Modes[%q] is empty, want every mode in Modes() to have a non-empty default table", mode)
		}
	}
}
