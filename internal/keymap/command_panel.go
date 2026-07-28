package keymap

// Command ids for the git panel and dispatch modal surfaces cataloged by
// #508 PR 1. The git panel's six lazygit-style action keys (P/p/f/m/s/S) are
// not commands -- they resolve to inline Action bindings seeded from
// config.DefaultGitActions(), duplicated as literals in defaults_panel.go
// since internal/keymap must not import internal/config (binding.go).
const (
	CommandGitPanelClose CommandID = "git_panel.close"
	CommandGitPanelRun   CommandID = "git_panel.run"
	CommandGitPanelNext  CommandID = "git_panel.next"
	CommandGitPanelPrev  CommandID = "git_panel.prev"

	CommandDispatchClose        CommandID = "dispatch.close"
	CommandDispatchToggleEnroll CommandID = "dispatch.toggle_enroll"
	CommandDispatchOnce         CommandID = "dispatch.once"
	CommandDispatchToggleLoop   CommandID = "dispatch.toggle_loop"
	CommandDispatchConfirmLoop  CommandID = "dispatch.confirm_loop"
	CommandDispatchCancelLoop   CommandID = "dispatch.cancel_loop"
)

// panelCommands is the Command catalogue entries for the git panel and
// dispatch modal, sourced from gitPanelModeHints (model.go:630) and the
// "Dispatch (cenci)" helpSections entry (view.go:1298) so hint/help wording
// matches today's status bar/help text exactly. catalog.go merges this into
// the package-level catalog.
var panelCommands = []Command{
	{CommandGitPanelClose, "Cancel"},
	{CommandGitPanelRun, "Run"},
	{CommandGitPanelNext, "Navigate"},
	{CommandGitPanelPrev, "Navigate"},

	{CommandDispatchClose, "Close"},
	{CommandDispatchToggleEnroll, "Enroll/Unenroll"},
	{CommandDispatchOnce, "Dispatch once"},
	{CommandDispatchToggleLoop, "Toggle loop on/off (all enrolled repos)"},
	{CommandDispatchConfirmLoop, "Confirm loop toggle"},
	{CommandDispatchCancelLoop, "Cancel loop toggle"},
}
