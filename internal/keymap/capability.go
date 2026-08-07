package keymap

// universalCommands is the set of command ids that are valid and
// dispatchable in every resolvable Mode (Modes()), not just the mode(s)
// that bind them by default. Today it holds exactly one entry, CommandQuit
// ("app.quit"): quit is a system-level command, not a mode feature, and
// Lookup's unconditional "ctrl+c" short-circuit (lookup.go) already encodes
// the same idea at the key-resolution layer -- #589's dispatch-layer seam
// (package main's universalDispatch, keymap_dispatch.go) generalizes it to
// any key a user binds to app.quit, in any of the 19 resolvable modes.
//
// #577 (parent ticket, not yet implemented) will extend this file with a
// per-mode capability index used to validate keymaps.<mode>.<key> command
// ids at config-load time. That index MUST consult IsUniversalCommand for
// app.quit's allowed-mode set rather than deriving it from Defaults() or a
// hand-written per-mode list -- Defaults() only reports the four modes that
// bind app.quit by default (normal, detail, help, error), which would
// wrongly reject a user's keymaps.<mode>.<key>: app.quit in every other
// mode (see #589's ticket body for the full "validation wall" rationale).
var universalCommands = map[CommandID]bool{
	CommandQuit: true,
}

// IsUniversalCommand reports whether id is a universal command -- one whose
// semantics are mode-independent and thus valid/dispatchable in any
// resolvable Mode, per universalCommands above.
func IsUniversalCommand(id CommandID) bool {
	return universalCommands[id]
}
