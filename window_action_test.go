package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// --- #624: window:/cwd:/focus: actions ---
//
// The escaping itself is proven in internal/action/window_test.go, which runs
// the assembled command through a real shell against a recording tmux stub.
// What these tests own is the dispatch layer: which values get expanded into
// the window command, which run path a given field combination selects, and
// the guards around them.

// insideTmuxEnv makes the current test look like it is running inside a tmux
// client, which every window: action requires.
func insideTmuxEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
}

// selectedCardNumber returns the number of the card the board has selected,
// so expected window names/paths are derived from fixture data rather than
// hardcoded.
func selectedCardNumber(b Board) int {
	return b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].Number
}

func TestWindowAction_RunsCommandInDetachedWindow(t *testing.T) {
	insideTmuxEnv(t)
	actions := map[string]config.Action{
		"W": {Name: "Serve", Type: "shell", Command: "go run .", Window: "card-{number}", Cwd: "/srv/app"},
	}
	b, fe := newActionTestBoard(t, actions)
	wantName := fmt.Sprintf("card-%d", selectedCardNumber(b))

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after dispatching a window action = %v, want normalMode", b.mode)
	}
	if len(fe.RunShellCalls) != 1 {
		t.Fatalf("RunShellCalls = %v, want exactly one window command", fe.RunShellCalls)
	}
	want := action.TmuxNewWindow(wantName, "/srv/app", "go run .", false)
	if fe.RunShellCalls[0] != want {
		t.Errorf("dispatched %q, want %q", fe.RunShellCalls[0], want)
	}
}

func TestWindowAction_FocusSwitchesToTheWindow(t *testing.T) {
	insideTmuxEnv(t)
	actions := map[string]config.Action{
		"W": {Name: "Serve", Type: "shell", Scope: "board", Command: "go run .", Window: "srv", Focus: true},
	}
	b, fe := newActionTestBoard(t, actions)

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after dispatching a focus window action = %v, want normalMode", b.mode)
	}
	want := action.TmuxNewWindow("srv", "", "go run .", true)
	if len(fe.RunShellCalls) != 1 || fe.RunShellCalls[0] != want {
		t.Errorf("RunShellCalls = %v, want exactly [%q] (focus: true drops tmux's -d)", fe.RunShellCalls, want)
	}
}

// A window action carrying no command opens the window on the user's shell.
func TestWindowAction_WithoutCommandOpensAnEmptyWindow(t *testing.T) {
	insideTmuxEnv(t)
	actions := map[string]config.Action{
		"W": {Name: "Open worktree", Type: "shell", Scope: "board", Window: "wt", Cwd: "/srv/app"},
	}
	b, fe := newActionTestBoard(t, actions)

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after dispatching a command-less window action = %v, want normalMode", b.mode)
	}
	want := action.TmuxNewWindow("wt", "/srv/app", "", false)
	if len(fe.RunShellCalls) != 1 || fe.RunShellCalls[0] != want {
		t.Errorf("RunShellCalls = %v, want exactly [%q]", fe.RunShellCalls, want)
	}
}

// The window name and cwd are template-expanded from the same variables the
// command is, so an action can name its window after the card it acts on.
func TestWindowAction_ExpandsTemplateVariablesInNameAndCwd(t *testing.T) {
	insideTmuxEnv(t)
	actions := map[string]config.Action{
		"W": {Name: "Open", Type: "shell", Window: "card-{number}", Cwd: "/repos/{title}"},
	}
	b, fe := newActionTestBoard(t, actions)
	selected := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	want := action.TmuxNewWindow(
		fmt.Sprintf("card-%d", selected.Number),
		"/repos/"+action.Slugify(selected.Title),
		"", false,
	)

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 1 || fe.RunShellCalls[0] != want {
		t.Errorf("RunShellCalls = %v, want exactly [%q]", fe.RunShellCalls, want)
	}
}

// TestWindowAction_OutsideTmuxRefusesWithAMessage covers the guard: without a
// tmux client there is no window to create, and the user must be told that
// rather than shown tmux's own "no server running" stderr.
func TestWindowAction_OutsideTmuxRefusesWithAMessage(t *testing.T) {
	t.Setenv("TMUX", "")
	actions := map[string]config.Action{
		"W": {Name: "Serve", Type: "shell", Scope: "board", Command: "go run .", Window: "srv"},
	}
	b, fe := newActionTestBoard(t, actions)

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShellCalls = %v, want none: a window action outside tmux must not shell out at all", fe.RunShellCalls)
	}
	if !strings.Contains(strings.ToLower(b.statusBar.message), "tmux") {
		t.Errorf("status bar message = %q, want it to explain that a window action needs tmux", b.statusBar.message)
	}
}

// --- cwd: on the two windowless run paths ---

func TestCwd_BufferedActionRunsInDirectory(t *testing.T) {
	actions := map[string]config.Action{
		"S": {Name: "Test", Type: "shell", Scope: "board", Command: "go test ./...", Cwd: "/srv/app"},
	}
	b, fe := newActionTestBoard(t, actions)

	m, cmd := b.Update(keyMsg("S"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after dispatching a cwd: action = %v, want normalMode", b.mode)
	}
	want := action.WithDir("/srv/app", "go test ./...")
	if len(fe.RunShellCalls) != 1 || fe.RunShellCalls[0] != want {
		t.Errorf("RunShellCalls = %v, want exactly [%q]", fe.RunShellCalls, want)
	}
}

func TestCwd_TerminalActionRunsInDirectory(t *testing.T) {
	actions := map[string]config.Action{
		"S": {Name: "Test", Type: "shell", Scope: "board", Command: "go test ./...", Cwd: "/srv/app", Terminal: true},
	}
	b, fe := newActionTestBoard(t, actions)

	m, cmd := b.Update(keyMsg("S"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after dispatching a cwd: terminal action = %v, want normalMode", b.mode)
	}
	want := action.WithDir("/srv/app", "go test ./...")
	if len(fe.ShellCommandCalls) != 1 || fe.ShellCommandCalls[0] != want {
		t.Errorf("ShellCommandCalls = %v, want exactly [%q] -- cwd applies to terminal: true actions too", fe.ShellCommandCalls, want)
	}
}

// TestCwd_PRWorktreeIsResolvedFromCwd is the AC7 regression case: the
// on-demand {pr_worktree} lookup is triggered by scanning the action's
// template, so a worktree referenced only from cwd: must still resolve
// instead of expanding to an empty path.
func TestCwd_PRWorktreeIsResolvedFromCwd(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve worktree", Type: "shell", Scope: "pr", Command: "ng serve", Cwd: "{pr_worktree}"},
	}
	b, fe := newPRActionTestBoard(t, actions)
	fe.RunShellOutputStdout = "worktree /repo/.worktrees/one-pr\nHEAD 1234567\nbranch refs/heads/feature/one-pr\n"

	b = sendKey(t, b, keyMsg("j"))
	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after dispatching a pr-scope cwd: action = %v, want normalMode", b.mode)
	}
	if len(fe.RunShellOutputCalls) != 1 || fe.RunShellOutputCalls[0] != "git worktree list --porcelain" {
		t.Fatalf("RunShellOutputCalls = %v, want the worktree lookup to have run", fe.RunShellOutputCalls)
	}
	want := action.WithDir("/repo/.worktrees/one-pr", "ng serve")
	if len(fe.RunShellCalls) != 1 || fe.RunShellCalls[0] != want {
		t.Errorf("RunShellCalls = %v, want exactly [%q]", fe.RunShellCalls, want)
	}
}

// A {comment} placeholder in cwd: must arm the Alt+key comment overload the
// same way one in command: does -- the overload is decided by scanning the
// whole action template.
func TestWindowAction_CommentOverloadSeesCwd(t *testing.T) {
	insideTmuxEnv(t)
	actions := map[string]config.Action{
		"W": {Name: "Open", Type: "shell", Scope: "board", Window: "w", Cwd: "/notes/{comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	m, _ := b.Update(altKeyMsg("W"))
	b = m.(Board)
	if b.mode != commentMode {
		t.Fatalf("mode after alt+W = %v, want commentMode: {comment} in cwd: must arm the overload", b.mode)
	}
	for _, ch := range "today" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	want := action.TmuxNewWindow("w", "/notes/today", "", false)
	if len(fe.RunShellCalls) != 1 || fe.RunShellCalls[0] != want {
		t.Errorf("RunShellCalls = %v, want exactly [%q]", fe.RunShellCalls, want)
	}
}
