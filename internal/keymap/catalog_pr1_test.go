package keymap

import (
	"strings"
	"testing"
)

// pr1Modes lists the ten resolvable Mode surfaces #508 PR 1 catalogues,
// spanning all three PR-1 command groups: the six navigable modals
// (modalCommands, command_modal.go), the git panel and dispatch modal
// (panelCommands, command_panel.go), and help/errorMode (systemCommands,
// command_system.go). ModeNormal/ModeDetail (#507) and the PR-2 confirm/
// text-input modes are deliberately excluded -- out of this PR's scope.
var pr1Modes = []Mode{
	ModePRList,
	ModeMilestoneList,
	ModeAgentList,
	ModeFilter,
	ModeAssign,
	ModePRPicker,
	ModeGitPanel,
	ModeDispatch,
	ModeHelp,
	ModeError,
}

// TestDefaults_ModalModesPopulated pins that Defaults() actually carries a
// non-empty default Table for every PR-1 mode -- without this, the
// mode-prefix and round-trip invariants below would pass vacuously (an empty
// table has nothing to violate them), masking a mode nobody ever wired up.
func TestDefaults_ModalModesPopulated(t *testing.T) {
	defaults := Defaults()
	for _, mode := range pr1Modes {
		if len(defaults.Modes[mode]) == 0 {
			t.Errorf("Defaults().Modes[%q] is empty, want a populated default table", mode)
		}
	}
}

// TestDefaults_ModalCommandIDsAreModePrefixed asserts that every command
// binding in a PR-1 mode's default table resolves to an id prefixed by that
// mode's own name ("pr_list.close", "git_panel.run", ...), except the
// reused app.quit id (CommandQuit) which help and errorMode both bind for
// "q" (allow-listed per Q3/Q5) -- inline BindingAction entries (the git
// panel's six lazygit-style keys) carry no CommandID and are skipped.
func TestDefaults_ModalCommandIDsAreModePrefixed(t *testing.T) {
	defaults := Defaults()
	for _, mode := range pr1Modes {
		prefix := string(mode) + "."
		for seq, binding := range defaults.Modes[mode] {
			if binding.Kind != BindingCommand {
				continue
			}
			if binding.Command == CommandQuit {
				continue
			}
			if !strings.HasPrefix(string(binding.Command), prefix) {
				t.Errorf("Defaults().Modes[%q][%q] resolves to %q, want it prefixed with %q or the allow-listed %q", mode, seq, binding.Command, prefix, CommandQuit)
			}
		}
	}
}

// TestDefaults_ModalKeysRoundTripCanonicalForm pins that every raw key
// bound in a PR-1 mode's default table is already in ParseSequence's
// canonical form -- a key stored some other way (e.g. "Esc" instead of
// "esc") would silently never match Lookup's canonicalized query.
func TestDefaults_ModalKeysRoundTripCanonicalForm(t *testing.T) {
	defaults := Defaults()
	for _, mode := range pr1Modes {
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

// TestCatalogue_ModalCommandIDsHaveDesc asserts every CommandID the PR-1
// default bindings resolve to (modalBindingCommandIDs, catalog_pr1_bindings_
// test.go) is catalogued via FindCommand with a non-empty Desc -- a default
// binding pointing at an uncatalogued id would resolve at runtime but render
// with no desc anywhere (help modal, which-key, #492).
func TestCatalogue_ModalCommandIDsHaveDesc(t *testing.T) {
	ids := modalBindingCommandIDs()
	if len(ids) == 0 {
		t.Fatal("modalBindingCommandIDs() returned no ids -- catalog_pr1_bindings_test.go's table is empty or broken")
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

// Whether the PR-1 default tables are internally consistent (no colliding
// raw keys, no invalid key notation) once merged with every other mode's
// defaults is already covered generically by TestDefaults_ResolveSucceeds
// (catalog_test.go), which resolves the whole Defaults() layer regardless
// of which modes are populated -- not repeated here.

// TestDefaults_ReturnsFreshCopyForModalModes asserts that mutating the Tables
// returned by one Defaults() call (deleting a PR-1 mode's table entirely)
// does not perturb a later call -- Defaults() must return a defensive deep
// copy, mirroring Commands()' contract (catalog.go).
func TestDefaults_ReturnsFreshCopyForModalModes(t *testing.T) {
	first := Defaults()
	for _, mode := range pr1Modes {
		delete(first.Modes, mode)
	}

	second := Defaults()
	for _, mode := range pr1Modes {
		if len(second.Modes[mode]) == 0 {
			t.Errorf("Defaults().Modes[%q] is empty on a later call after a previous caller mutated its own copy -- Defaults() is leaking shared state", mode)
		}
	}
}

// TestCommands_ReturnsFreshCopyAfterModalMutation asserts that mutating the
// slice returned by one Commands() call does not perturb a later call's
// entries for a PR-1 command id, mirroring TestDefaults_
// ReturnsFreshCopyForModalModes above.
func TestCommands_ReturnsFreshCopyAfterModalMutation(t *testing.T) {
	ids := modalBindingCommandIDs()
	if len(ids) == 0 {
		t.Fatal("modalBindingCommandIDs() returned no ids -- catalog_pr1_bindings_test.go's table is empty or broken")
	}
	target := ids[0]

	first := Commands()
	mutated := false
	for i := range first {
		if first[i].ID == target {
			first[i].Desc = "mutated-by-test"
			mutated = true
			break
		}
	}
	if !mutated {
		t.Fatalf("Commands() does not contain %q yet (catalogue not implemented) -- cannot exercise the fresh-copy guarantee", target)
	}

	second := Commands()
	cmd, ok := FindCommand(target)
	if !ok {
		t.Fatalf("FindCommand(%q) returned ok == false after a previous caller mutated its own Commands() copy", target)
	}
	if cmd.Desc == "mutated-by-test" {
		t.Errorf("FindCommand(%q).Desc = %q after a previous caller mutated its own Commands() slice -- Commands() is leaking shared state", target, cmd.Desc)
	}
	for _, c := range second {
		if c.ID == target && c.Desc == "mutated-by-test" {
			t.Errorf("Commands() entry %q carries a mutation made to a previous caller's slice -- Commands() is leaking shared state", target)
		}
	}
}
