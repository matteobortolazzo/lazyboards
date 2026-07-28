package keymap

// CommandID names a built-in, non-user-configurable command a key can
// resolve to (as opposed to an inline Action). The full catalogue of
// command ids lands in #507/#508 alongside their default bindings; this
// ticket defines only the one command the engine itself hard-wires.
type CommandID string

// CommandQuit is the app-quit command. Lookup resolves it unconditionally
// whenever the last key of a sequence is "ctrl+c", regardless of table
// contents -- the engine-level half of the guarantee update.go already
// enforces ahead of mode dispatch.
const CommandQuit CommandID = "app.quit"
