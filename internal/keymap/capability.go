package keymap

import "sort"

// universalCommands is the set of command ids that are valid and
// dispatchable in every resolvable Mode (Modes()), not just the mode(s)
// that bind them by default. Today it holds exactly one entry, CommandQuit
// ("app.quit"): quit is a system-level command, not a mode feature, and
// Lookup's unconditional "ctrl+c" short-circuit (lookup.go) already encodes
// the same idea at the key-resolution layer -- #589's dispatch-layer seam
// (package main's universalDispatch, keymap_dispatch.go) generalizes it to
// any key a user binds to app.quit, in any of the 19 resolvable modes.
//
// commandModeIndex (below) consults this set for app.quit's allowed-mode
// set rather than deriving it from Defaults() or a hand-written per-mode
// list -- Defaults() only reports the four modes that bind app.quit by
// default (normal, detail, help, error), which would wrongly reject a
// user's keymaps.<mode>.<key>: app.quit in every other mode (see #589's
// ticket body for the full "validation wall" rationale).
var universalCommands = map[CommandID]bool{
	CommandQuit: true,
}

// IsUniversalCommand reports whether id is a universal command -- one whose
// semantics are mode-independent and thus valid/dispatchable in any
// resolvable Mode, per universalCommands above.
func IsUniversalCommand(id CommandID) bool {
	return universalCommands[id]
}

// commandModeIndex is the #577 per-mode capability index: for every Mode in
// Modes() plus ModeColumns, the set of CommandIDs that mode can actually
// dispatch. internal/config's validateModeCapabilities consults it (via
// CommandDispatchable/DispatchableCommands) to reject a keymaps.<mode>.<key>
// command id the mode's runner could never reach.
//
// It is built by buildCommandModeIndex (called from catalog.go's init, see
// that file's comment for why) from three sources, not defaultModeTables
// alone:
//   - the mode's own default Table, deduped by CommandID (a command bound
//     to two keys, e.g. nav.cursor_down at both "j" and "down" in
//     normalDefaults, counts once);
//   - universalCommands, unioned into every mode, since a universal id like
//     app.quit is dispatchable everywhere regardless of whether that mode's
//     own table happens to bind it by default;
//   - two hand-specified widenings that don't fall out of defaultModeTables
//     at all: ModeDetail additionally inherits every ModeNormal command
//     (runDetailCommand falls through to runNormalCommand for any id it
//     doesn't handle itself, #588), and ModeColumns is derived as the
//     intersection ModeNormal ∩ ModeDetail (a keymaps.columns.<name> table
//     overlays both ModeNormal and ModeDetail, keymap.go's Resolve, so a
//     column binds into whichever of the two dispatchers is active --
//     mathematically this intersection equals normalDefaults's set in
//     full today, since ModeDetail's widened set already is a superset of
//     ModeNormal's).
var commandModeIndex map[Mode][]CommandID

// buildCommandModeIndex populates commandModeIndex. It must run after
// defaultModeTables (catalog.go) is populated -- see catalog.go's init for
// why this function is called from there rather than having its own init in
// this file.
func buildCommandModeIndex() {
	sets := make(map[Mode]map[CommandID]bool, len(modes)+1)
	for _, mode := range modes {
		set := make(map[CommandID]bool)
		for _, binding := range defaultModeTables[mode] {
			if binding.Kind == BindingCommand {
				set[binding.Command] = true
			}
		}
		for id := range universalCommands {
			set[id] = true
		}
		sets[mode] = set
	}

	// ModeDetail additionally inherits every ModeNormal command (#588's
	// runDetailCommand fall-through).
	for id := range sets[ModeNormal] {
		sets[ModeDetail][id] = true
	}

	// ModeColumns is derived as the intersection ModeNormal ∩ ModeDetail,
	// computed AFTER ModeDetail's widening above so it self-corrects if
	// #588's fall-through ever changes.
	columnsSet := make(map[CommandID]bool)
	for id := range sets[ModeNormal] {
		if sets[ModeDetail][id] {
			columnsSet[id] = true
		}
	}
	sets[ModeColumns] = columnsSet

	index := make(map[Mode][]CommandID, len(sets))
	for mode, set := range sets {
		ids := make([]CommandID, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		index[mode] = ids
	}
	commandModeIndex = index
}

// DispatchableCommands returns every CommandID mode can dispatch, per
// commandModeIndex, sorted and as a defensive copy -- mirroring Commands()/
// Modes()/Defaults()'s existing defensive-copy convention (catalog.go,
// mode.go).
func DispatchableCommands(mode Mode) []CommandID {
	src := commandModeIndex[mode]
	out := make([]CommandID, len(src))
	copy(out, src)
	return out
}

// CommandDispatchable reports whether id is in mode's dispatchable-command
// set, per commandModeIndex.
func CommandDispatchable(mode Mode, id CommandID) bool {
	for _, candidate := range commandModeIndex[mode] {
		if candidate == id {
			return true
		}
	}
	return false
}
