package action

import "strings"

// TmuxNewWindow builds the shell command that opens a new tmux window
// running command, backing the `window:` action field (#624). name, cwd, and
// command are raw (already template-expanded) values: every one of them can
// carry provider data -- a card title, a branch name, a worktree path -- so
// each is escaped here into exactly one shell token. Callers must NOT
// pre-escape them; passing values from BuildShellSafeVars would double-quote
// them.
//
// The window is detached (-d) unless focus is set, mirroring the default
// every hand-written `tmux new-window -d` action used before this field
// existed. An empty cwd omits -c (tmux inherits its own default), and an
// empty command omits the command argument entirely, which opens the window
// running the user's default shell.
//
// tmux is the only multiplexer this builds for today. It is the single place
// that knowledge lives: a zellij/wezterm backend would be a sibling function
// selected here, not a second command string assembled at a dispatch site.
func TmuxNewWindow(name, cwd, command string, focus bool) string {
	parts := []string{"tmux", "new-window"}
	if !focus {
		parts = append(parts, "-d")
	}
	parts = append(parts, "-n", ShellEscape(name))
	if cwd != "" {
		parts = append(parts, "-c", ShellEscape(cwd))
	}
	if command != "" {
		parts = append(parts, ShellEscape(command))
	}
	return strings.Join(parts, " ")
}

// WithDir composes command so it runs in dir, backing the `cwd:` action
// field for the shell paths that have no window of their own to start in
// (buffered and terminal: true actions). dir is a raw value and is escaped
// here as one shell token.
//
// The composition is `cd <dir> && <command>`, deliberately && and not ;: a
// missing or unreadable directory must abort the action, never silently run
// the command wherever lazyboards happens to be -- which for a destructive
// command would be the user's repo.
//
// An empty dir returns command unchanged, so callers can pass an unset cwd
// through without a conditional.
func WithDir(dir, command string) string {
	if dir == "" {
		return command
	}
	return "cd " + ShellEscape(dir) + " && " + command
}
