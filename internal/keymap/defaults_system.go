package keymap

// helpDefaults is the default ModeHelp table, transcribed from
// handleHelpModeKey (mode_handlers.go). "q" reuses CommandQuit
// (command.go) rather than a help-specific id (Q5).
var helpDefaults = Table{
	"esc":  CommandBinding(CommandHelpClose),
	"?":    CommandBinding(CommandHelpClose),
	"j":    CommandBinding(CommandHelpScrollDown),
	"down": CommandBinding(CommandHelpScrollDown),
	"k":    CommandBinding(CommandHelpScrollUp),
	"up":   CommandBinding(CommandHelpScrollUp),
	"q":    CommandBinding(CommandQuit),
}

// errorDefaults is the default ModeError table, transcribed from
// errorMode's key switch (update.go). "q" reuses CommandQuit
// (command.go) rather than an error-specific id (Q3/Q5).
var errorDefaults = Table{
	"q": CommandBinding(CommandQuit),
	"r": CommandBinding(CommandErrorRetry),
}

// systemDefaultTables aggregates the help and error default tables by Mode.
// catalog.go merges this into the package-level defaultModeTables.
var systemDefaultTables = map[Mode]Table{
	ModeHelp:  helpDefaults,
	ModeError: errorDefaults,
}
