package keymap

// Command ids for the help modal and errorMode surfaces cataloged by #508
// PR 1. Both reuse CommandQuit (command.go, "app.quit") for their own "q"
// binding rather than minting a mode-specific id -- an allow-listed
// exception to the mode-prefix convention, since the handler behavior is
// identical (Q3/Q5). See defaults_system.go for the matching default Table
// entries.
const (
	CommandHelpClose      CommandID = "help.close"
	CommandHelpScrollDown CommandID = "help.scroll_down"
	CommandHelpScrollUp   CommandID = "help.scroll_up"

	CommandErrorRetry CommandID = "error.retry"
)

// systemCommands is the Command catalogue entries for the help modal and
// errorMode, sourced from helpModeHints (model.go:212) and the "Error"
// helpSections entry (view.go:1311) so hint/help wording matches today's
// text exactly. CommandQuit's own "Quit" desc (catalog.go) already covers
// both modes' reused "q" binding, so it is not re-registered here. catalog.go
// merges this into the package-level catalog.
var systemCommands = []Command{
	{CommandHelpClose, "Close"},
	{CommandHelpScrollDown, "Scroll"},
	{CommandHelpScrollUp, "Scroll"},

	{CommandErrorRetry, "Retry"},
}
