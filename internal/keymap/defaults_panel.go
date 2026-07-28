package keymap

// gitPanelDefaults is the default ModeGitPanel table, transcribed from
// handleGitPanelKey (mode_handlers.go:526). The six action entries mirror
// config.DefaultGitActions() (internal/config/config.go:161) field-for-
// field, with Order set to each key's 1-based position in
// gitPanelBuiltinOrder (model.go:1088, ["P","p","f","m","s","S"]).
// internal/keymap must not import internal/config (binding.go), so these
// literals are duplicated by hand; the root git_keymap_defaults_test.go
// drift guard holds them in sync with the real config values.
var gitPanelDefaults = Table{
	"esc":   CommandBinding(CommandGitPanelClose),
	"enter": CommandBinding(CommandGitPanelRun),
	"j":     CommandBinding(CommandGitPanelNext),
	"down":  CommandBinding(CommandGitPanelNext),
	"k":     CommandBinding(CommandGitPanelPrev),
	"up":    CommandBinding(CommandGitPanelPrev),

	"P": ActionBinding(Action{Name: "Push", Type: "shell", Command: "git push", Scope: "board", Order: 1}),
	"p": ActionBinding(Action{Name: "Pull (rebase)", Type: "shell", Command: "git pull --rebase", Scope: "board", Order: 2}),
	"f": ActionBinding(Action{Name: "Fetch", Type: "shell", Command: "git fetch", Scope: "board", Order: 3}),
	"m": ActionBinding(Action{Name: "Mergetool", Type: "shell", Command: "git mergetool", Scope: "board", Order: 4}),
	"s": ActionBinding(Action{Name: "Stash push", Type: "shell", Command: "git stash push", Scope: "board", Order: 5}),
	"S": ActionBinding(Action{Name: "Stash pop", Type: "shell", Command: "git stash pop", Scope: "board", Order: 6}),
}

// dispatchDefaults is the default ModeDispatch table, transcribed from
// handleDispatchModeKey (mode_handlers.go:561). y/n resolve unconditionally
// at this layer -- the confirmingLoop state gate that scopes them to the
// loop confirm stays in the handler (docs/view-state-consistency.md), not
// the table.
var dispatchDefaults = Table{
	"esc":   CommandBinding(CommandDispatchClose),
	"enter": CommandBinding(CommandDispatchToggleEnroll),
	"o":     CommandBinding(CommandDispatchOnce),
	"l":     CommandBinding(CommandDispatchToggleLoop),
	"y":     CommandBinding(CommandDispatchConfirmLoop),
	"n":     CommandBinding(CommandDispatchCancelLoop),
}

// panelDefaultTables aggregates the git panel and dispatch default tables by
// Mode. catalog.go merges this into the package-level defaultModeTables.
var panelDefaultTables = map[Mode]Table{
	ModeGitPanel: gitPanelDefaults,
	ModeDispatch: dispatchDefaults,
}
