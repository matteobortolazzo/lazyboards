package keymap

import "testing"

// TestCommands_NonTrivialCount catches an accidentally empty or truncated
// Commands() slice, which would otherwise fail silently (every other test in
// this file iterates it, so an empty slice makes them vacuously pass) --
// mirrors TestModes_NonTrivialCount (mode_test.go). The catalogue transcribed
// from handleNormalModeKey/handleDetailFocusedKey (mode_handlers.go,
// update.go) holds 35 normal-derived ids (26 switch cases, three of which
// bind two keys to one id, plus nine nav.column_N ids) plus 3 detail-only
// ids (detail.blur, detail.scroll_down, detail.scroll_up) = 38 total.
func TestCommands_NonTrivialCount(t *testing.T) {
	if len(Commands()) < 30 {
		t.Fatalf("Commands() returned %d commands, want at least 30 (the normal+detail catalogue)", len(Commands()))
	}
}

// TestCommands_IDsAreUnique guards against a copy-pasted command id that
// compiles fine but silently shadows another command's catalogue entry.
func TestCommands_IDsAreUnique(t *testing.T) {
	seen := make(map[CommandID]bool)
	for _, c := range Commands() {
		if seen[c.ID] {
			t.Errorf("Commands() contains duplicate id %q", c.ID)
		}
		seen[c.ID] = true
	}
}

// TestDefaultModeTableGroups_NoModeCollisions guards catalog.go's init(),
// which merges modalDefaultTables (defaults_modal.go), panelDefaultTables
// (defaults_panel.go) and systemDefaultTables (defaults_system.go) into
// defaultModeTables with plain map-assignment and no collision check -- if
// two groups ever registered a Table for the same Mode, the later merge
// would silently overwrite the earlier one with no panic/error/log. This
// mirrors TestCommands_IDsAreUnique's guard on the parallel catalog slice
// merge, for the map-merge side.
func TestDefaultModeTableGroups_NoModeCollisions(t *testing.T) {
	groups := map[string]map[Mode]Table{
		"modalDefaultTables":  modalDefaultTables,
		"panelDefaultTables":  panelDefaultTables,
		"systemDefaultTables": systemDefaultTables,
	}

	seenIn := make(map[Mode]string)
	for groupName, group := range groups {
		for mode := range group {
			if mode == ModeNormal || mode == ModeDetail {
				t.Errorf("%s registers mode %q, which collides with the #507 board tables (normalDefaults/detailDefaults)", groupName, mode)
				continue
			}
			if prevGroup, exists := seenIn[mode]; exists {
				t.Errorf("mode %q is registered by both %s and %s -- catalog.go's init() would silently let one overwrite the other", mode, prevGroup, groupName)
				continue
			}
			seenIn[mode] = groupName
		}
	}
}

// TestCommands_DescNonEmpty pins that every catalogued command carries a
// human-readable description -- an empty Desc would render as a blank line
// in help/which-key output.
func TestCommands_DescNonEmpty(t *testing.T) {
	for _, c := range Commands() {
		if c.Desc == "" {
			t.Errorf("Commands() entry %q has empty Desc", c.ID)
		}
	}
}

// commandIDSet builds a membership set from Commands(), for tests that
// check every default-table id resolves against the catalogue.
func commandIDSet(t *testing.T) map[CommandID]bool {
	t.Helper()
	set := make(map[CommandID]bool, len(Commands()))
	for _, c := range Commands() {
		set[c.ID] = true
	}
	return set
}

// TestDefaults_EveryBoundCommandExistsInCatalogue asserts every CommandID
// appearing in Defaults()'s normal/detail tables has a matching Command
// entry in the catalogue -- a default binding pointing at an uncatalogued id
// would resolve at runtime but render with no desc anywhere (help modal,
// which-key).
func TestDefaults_EveryBoundCommandExistsInCatalogue(t *testing.T) {
	ids := commandIDSet(t)
	defaults := Defaults()
	for mode, table := range defaults.Modes {
		for seq, binding := range table {
			if binding.Kind != BindingCommand {
				continue
			}
			if !ids[binding.Command] {
				t.Errorf("Defaults().Modes[%q][%q] resolves to uncatalogued command %q", mode, seq, binding.Command)
			}
		}
	}
}

// TestDefaults_OnlyBoardAndModalModesPopulated pins this ticket's scope:
// #507 delivers the normal/detail default tables and #508 PR 1
// (pr1Modes, catalog_pr1_test.go) adds the six navigable modals, the git
// panel, the dispatch modal, help and errorMode -- Defaults() must not
// populate any mode beyond that set yet (PR 2 adds the confirm/text-input
// modes next).
func TestDefaults_OnlyBoardAndModalModesPopulated(t *testing.T) {
	populated := map[Mode]bool{ModeNormal: true, ModeDetail: true}
	for _, mode := range pr1Modes {
		populated[mode] = true
	}

	defaults := Defaults()
	for mode := range defaults.Modes {
		if !populated[mode] {
			t.Errorf("Defaults().Modes contains mode %q, want only the #507 board modes and #508 PR 1 modal modes", mode)
		}
	}
}

// TestDefaults_ResolveSucceeds pins that the default table itself is
// internally consistent (no colliding raw keys, no invalid key notation) --
// Resolve must accept it against an empty user layer without error.
func TestDefaults_ResolveSucceeds(t *testing.T) {
	if _, err := Resolve(Defaults(), Tables{}); err != nil {
		t.Fatalf("Resolve(Defaults(), Tables{}) returned unexpected error: %v", err)
	}
}

// TestFindCommand_ReturnsMatchingCommand asserts a catalogued id resolves to
// its Command entry (matching ID and a non-empty Desc) with ok == true.
func TestFindCommand_ReturnsMatchingCommand(t *testing.T) {
	cmd, ok := FindCommand(CommandQuit)
	if !ok {
		t.Fatalf("FindCommand(CommandQuit) returned ok == false, want true")
	}
	if cmd.ID != CommandQuit {
		t.Errorf("FindCommand(CommandQuit).ID = %q, want %q", cmd.ID, CommandQuit)
	}
	if cmd.Desc == "" {
		t.Errorf("FindCommand(CommandQuit).Desc is empty, want a human-readable description")
	}
}

// TestFindCommand_UnknownIDReturnsFalse asserts an id absent from the
// catalogue resolves to the zero-value Command with ok == false, rather than
// panicking or matching an unrelated entry.
func TestFindCommand_UnknownIDReturnsFalse(t *testing.T) {
	cmd, ok := FindCommand(CommandID("nonexistent.command"))
	if ok {
		t.Fatalf("FindCommand(nonexistent.command) returned ok == true, want false")
	}
	if cmd != (Command{}) {
		t.Errorf("FindCommand(nonexistent.command) = %+v, want zero-value Command", cmd)
	}
}

// TestDefaults_NoEntryBindsCtrlC pins that the default table never binds
// "ctrl+c" itself -- Lookup already hard-wires ctrl+c to CommandQuit ahead
// of any table (lookup.go), so a default table entry for it would be dead,
// misleading configuration surface.
func TestDefaults_NoEntryBindsCtrlC(t *testing.T) {
	defaults := Defaults()
	for mode, table := range defaults.Modes {
		if _, ok := table["ctrl+c"]; ok {
			t.Errorf("Defaults().Modes[%q] binds \"ctrl+c\", which Lookup already hard-wires to CommandQuit", mode)
		}
	}
}
