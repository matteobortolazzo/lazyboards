package keymap

import "testing"

// TestUniversalCommands_ContainsExactlyCommandQuit pins universalCommands
// (capability.go) to a single-entry set: CommandQuit. The #577 per-mode
// capability index (commandModeIndex, built by buildCommandModeIndex)
// derives app.quit's allowed set from this predicate rather than
// hand-writing a mode list -- if a future id is ever
// added here without a corresponding dispatch seam case, this test's exact-
// membership assertion (not just "contains CommandQuit") is what would catch
// a stray addition no integration test could otherwise observe (an unwired
// universal id simply no-ops silently at the seam).
func TestUniversalCommands_ContainsExactlyCommandQuit(t *testing.T) {
	if got := len(universalCommands); got != 1 {
		t.Fatalf("len(universalCommands) = %d, want 1", got)
	}
	if !universalCommands[CommandQuit] {
		t.Errorf("universalCommands does not contain CommandQuit, want it to")
	}
}

func TestIsUniversalCommand_CommandQuit_ReturnsTrue(t *testing.T) {
	if !IsUniversalCommand(CommandQuit) {
		t.Errorf("IsUniversalCommand(CommandQuit) = false, want true")
	}
}

// TestIsUniversalCommand_ModeScopedID_ReturnsFalse uses CommandCardDelete (a
// normal/detail-mode-scoped id, command_board.go) as a representative
// non-universal command: it must not be treated as dispatchable outside its
// own mode's runner.
func TestIsUniversalCommand_ModeScopedID_ReturnsFalse(t *testing.T) {
	if IsUniversalCommand(CommandCardDelete) {
		t.Errorf("IsUniversalCommand(CommandCardDelete) = true, want false (mode-scoped id, not universal)")
	}
}

// --- #577: per-mode capability index (DispatchableCommands/CommandDispatchable) ---
//
// DispatchableCommands(mode) is the defaults-derived index internal/config's
// validateModeCapabilities consults to reject a
// keymaps.<mode>.<key> command id the mode can never actually dispatch. It
// must be built from three sources, not Defaults() alone:
//   - mode's own default Table (catalog.go's defaultModeTables), deduped by
//     CommandID (a command bound to two keys, e.g. nav.cursor_down at both
//     "j" and "down" in normalDefaults, counts once);
//   - universalCommands (capability.go, above) unioned into every mode,
//     since a universal id like app.quit is dispatchable everywhere
//     regardless of whether that mode's own table happens to bind it;
//   - two hand-specified widenings that don't fall out of Defaults() at
//     all: ModeDetail additionally inherits every ModeNormal command
//     (runDetailCommand falls through to runNormalCommand for any id it
//     doesn't handle itself, #588), and ModeColumns is derived as the
//     intersection ModeNormal ∩ ModeDetail (a keymaps.columns.<name> table
//     overlays both ModeNormal and ModeDetail, keymap.go's Resolve, so a
//     column binds into whichever of the two dispatchers is active --
//     mathematically this intersection equals normalDefaults's set in full
//     today, since ModeDetail's widened set already is a superset of
//     ModeNormal's).

// commandIDsFromTable returns the unique BindingCommand ids in table --
// exactly the "dedup by command id, not by key" shape DispatchableCommands must
// implement.
func commandIDsFromTable(table Table) map[CommandID]bool {
	set := make(map[CommandID]bool)
	for _, binding := range table {
		if binding.Kind == BindingCommand {
			set[binding.Command] = true
		}
	}
	return set
}

// assertCommandSetEqual asserts got (as returned by DispatchableCommands) has no
// duplicate entries and contains exactly the ids in want.
func assertCommandSetEqual(t *testing.T, got []CommandID, want map[CommandID]bool) {
	t.Helper()
	gotSet := make(map[CommandID]bool, len(got))
	for _, id := range got {
		if gotSet[id] {
			t.Errorf("result contains duplicate id %q", id)
		}
		gotSet[id] = true
	}
	if len(gotSet) != len(want) {
		t.Fatalf("result has %d unique ids, want %d\ngot:  %v\nwant: %v", len(gotSet), len(want), got, want)
	}
	for id := range want {
		if !gotSet[id] {
			t.Errorf("result missing expected id %q", id)
		}
	}
}

// TestDispatchableCommands_PopulatedForEveryResolvableModeAndColumns pins that
// every Mode in Modes() (all 19) plus ModeColumns has a non-empty entry in
// the capability index -- at minimum, universalCommands's union guarantees
// this even for a mode whose own default table were ever empty.
func TestDispatchableCommands_PopulatedForEveryResolvableModeAndColumns(t *testing.T) {
	for _, mode := range Modes() {
		t.Run(string(mode), func(t *testing.T) {
			if got := DispatchableCommands(mode); len(got) == 0 {
				t.Errorf("DispatchableCommands(%q) is empty, want at least the universal command set", mode)
			}
		})
	}
	t.Run("columns", func(t *testing.T) {
		if got := DispatchableCommands(ModeColumns); len(got) == 0 {
			t.Errorf("DispatchableCommands(ModeColumns) is empty, want normalDefaults' command set")
		}
	})
}

// TestDispatchableCommands_Normal_DedupesByCommandID_NotByKey pins ModeNormal's
// allowed set to normalDefaults' unique command ids (35, per AGENTS.md's
// count of normalDefaults's distinct CommandIDs) unioned with
// universalCommands, and separately pins that nav.cursor_down -- bound to
// both "j" and "down" in normalDefaults -- appears exactly once in the
// result, not once per key it's bound to.
func TestDispatchableCommands_Normal_DedupesByCommandID_NotByKey(t *testing.T) {
	want := commandIDsFromTable(normalDefaults)
	for id := range universalCommands {
		want[id] = true
	}
	got := DispatchableCommands(ModeNormal)
	assertCommandSetEqual(t, got, want)

	count := 0
	for _, id := range got {
		if id == CommandNavCursorDown {
			count++
		}
	}
	if count != 1 {
		t.Errorf("DispatchableCommands(ModeNormal) contains nav.cursor_down %d times, want exactly 1 (bound to both \"j\" and \"down\" in normalDefaults)", count)
	}
}

// TestDispatchableCommands_ReturnsSortedDefensiveCopy pins two properties every
// caller (error-message rendering, in particular) depends on: the result is
// sorted (so an error listing valid ids is deterministic, not Go's
// randomized map-iteration order), and it is a defensive copy (mutating one
// call's result must never perturb a later call's), mirroring Commands()/
// Modes()/Defaults()'s existing defensive-copy convention (catalog.go,
// mode.go).
func TestDispatchableCommands_ReturnsSortedDefensiveCopy(t *testing.T) {
	first := DispatchableCommands(ModeNormal)
	for i := 1; i < len(first); i++ {
		if first[i-1] > first[i] {
			t.Fatalf("DispatchableCommands(ModeNormal) not sorted: %q came before %q", first[i-1], first[i])
		}
	}

	if len(first) == 0 {
		t.Fatal("DispatchableCommands(ModeNormal) returned no ids, cannot exercise the defensive-copy guard")
	}
	original := first[0]
	first[0] = "mutated.sentinel"

	second := DispatchableCommands(ModeNormal)
	if second[0] != original {
		t.Errorf("DispatchableCommands(ModeNormal)[0] = %q after mutating a previous call's result, want %q (result must be a defensive copy)", second[0], original)
	}
}

// TestDispatchableCommands_UniversalCommandUnionApplied uses ModeFilter --
// filterDefaults never binds CommandQuit -- as the representative mode
// proving DispatchableCommands unions in universalCommands rather than deriving
// app.quit's allowed-mode set from Defaults() alone (the exact regression
// #589's capability.go doc comment warns against).
func TestDispatchableCommands_UniversalCommandUnionApplied(t *testing.T) {
	if _, bound := filterDefaults["q"]; bound {
		t.Fatal("test fixture invariant broken: filterDefaults now binds \"q\", pick a different representative mode/key")
	}
	found := false
	for _, id := range DispatchableCommands(ModeFilter) {
		if id == CommandQuit {
			found = true
		}
	}
	if !found {
		t.Errorf("DispatchableCommands(ModeFilter) does not contain CommandQuit, want it unioned in via universalCommands even though filterDefaults never binds it")
	}
}

// TestDispatchableCommands_ModeDetail_EqualsUnionOfDetailAndNormalDefaults pins
// the #588 inheritance widening: because runDetailCommand falls through to
// runNormalCommand for any id it doesn't handle itself, ModeDetail's
// allowed set must be detailDefaults UNION normalDefaults, not
// detailDefaults alone.
func TestDispatchableCommands_ModeDetail_EqualsUnionOfDetailAndNormalDefaults(t *testing.T) {
	want := commandIDsFromTable(detailDefaults)
	for id := range commandIDsFromTable(normalDefaults) {
		want[id] = true
	}
	for id := range universalCommands {
		want[id] = true
	}
	assertCommandSetEqual(t, DispatchableCommands(ModeDetail), want)
}

// TestDispatchableCommands_ModeColumns_EqualsNormalDefaultsInFull pins that
// keymaps.columns.<name>'s admissible command set is normalDefaults's in
// full -- not an intersection with detailDefaults -- since a column table
// overlays ModeNormal/ModeDetail (keymap.go's Resolve), not just ModeNormal.
func TestDispatchableCommands_ModeColumns_EqualsNormalDefaultsInFull(t *testing.T) {
	want := commandIDsFromTable(normalDefaults)
	for id := range universalCommands {
		want[id] = true
	}
	assertCommandSetEqual(t, DispatchableCommands(ModeColumns), want)
}

// TestDispatchableCommands_ModeColumns_AcceptsNormalOnlyID pins the accept half
// of the columns matrix: nav.cursor_down (bound only in normalDefaults, not
// detailDefaults) must be admissible under keymaps.columns.<name>.
func TestDispatchableCommands_ModeColumns_AcceptsNormalOnlyID(t *testing.T) {
	found := false
	for _, id := range DispatchableCommands(ModeColumns) {
		if id == CommandNavCursorDown {
			found = true
		}
	}
	if !found {
		t.Error("DispatchableCommands(ModeColumns) does not contain nav.cursor_down, want a normal-only id to be accepted")
	}
}

// TestDispatchableCommands_ModeColumns_RejectsDetailOnlyIDs pins the reject half:
// the three detail-only ids (bound in detailDefaults, never in
// normalDefaults) must NOT be admissible under keymaps.columns.<name>.
func TestDispatchableCommands_ModeColumns_RejectsDetailOnlyIDs(t *testing.T) {
	for _, id := range []CommandID{CommandDetailBlur, CommandDetailScrollDown, CommandDetailScrollUp} {
		t.Run(string(id), func(t *testing.T) {
			for _, got := range DispatchableCommands(ModeColumns) {
				if got == id {
					t.Errorf("DispatchableCommands(ModeColumns) contains detail-only id %q, want it rejected", id)
				}
			}
		})
	}
}

// TestCommandDispatchable_MirrorsDispatchableCommands pins the boolean
// convenience predicate against three representative cases: a mode-scoped
// id in its own mode, the same id in a foreign mode, and a universal id in
// a mode that never binds it by default.
func TestCommandDispatchable_MirrorsDispatchableCommands(t *testing.T) {
	if !CommandDispatchable(ModeNormal, CommandBoardRefresh) {
		t.Error("CommandDispatchable(ModeNormal, board.refresh) = false, want true (normalDefaults binds it)")
	}
	if CommandDispatchable(ModeFilter, CommandBoardRefresh) {
		t.Error("CommandDispatchable(ModeFilter, board.refresh) = true, want false (foreign to filter)")
	}
	if !CommandDispatchable(ModeFilter, CommandQuit) {
		t.Error("CommandDispatchable(ModeFilter, app.quit) = false, want true (universal)")
	}
}

// --- #577: Mode.DispatchesInlineActions() ---

// TestMode_DispatchesInlineActions_Matrix is the hand-written predicate's
// full matrix across all 19 resolvable modes plus ModeColumns: only
// git_panel, normal, detail, columns and pr_list ever dispatch an inline
// action (git_panel accepts any scope; pr_list additionally requires the
// effective scope to be exactly "pr", enforced at the config layer since
// keymap.Mode has no notion of an Action's scope). Every other mode rejects
// inline actions entirely -- this predicate cannot be derived from
// Defaults() or DispatchableCommands() the way command-id capability can, since
// none of the rejecting modes' default tables say anything about actions
// one way or the other.
func TestMode_DispatchesInlineActions_Matrix(t *testing.T) {
	want := map[Mode]bool{
		ModeGitPanel: true,
		ModeNormal:   true,
		ModeDetail:   true,
		ModeColumns:  true,
		ModePRList:   true,
	}

	allModes := append(append([]Mode{}, Modes()...), ModeColumns)
	for _, mode := range allModes {
		t.Run(string(mode), func(t *testing.T) {
			if got := mode.DispatchesInlineActions(); got != want[mode] {
				t.Errorf("Mode(%q).DispatchesInlineActions() = %v, want %v", mode, got, want[mode])
			}
		})
	}
}
