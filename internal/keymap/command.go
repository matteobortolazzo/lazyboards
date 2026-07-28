package keymap

// CommandID names a built-in, non-user-configurable command a key can
// resolve to (as opposed to an inline Action). #507 fills in the catalogue
// and default bindings for the normal-mode and detail-panel surfaces; #508
// covers every remaining mode.
type CommandID string

// CommandQuit is the app-quit command. Lookup resolves it unconditionally
// whenever the last key of a sequence is "ctrl+c", regardless of table
// contents -- the engine-level half of the guarantee update.go already
// enforces ahead of mode dispatch.
const CommandQuit CommandID = "app.quit"

// Command is one catalogued entry: a stable id plus the human-readable
// description surfaced in help/which-key rendering. Every CommandID a
// default (or user) table can resolve to must have a matching Command in
// the catalogue Commands() returns.
type Command struct {
	ID   CommandID
	Desc string
}
