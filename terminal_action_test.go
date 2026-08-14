package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// --- #623: `terminal: true` shell actions ---
//
// A plain type: shell action runs through action.Executor.RunShell, which
// buffers the command's output via CombinedOutput() and discards it on
// success -- nothing the command prints is ever visible. A terminal: true
// action instead hands the real terminal to the command via tea.ExecProcess,
// so a test suite / dev server / device run can actually be watched.
//
// The two paths are distinguishable at dispatch time without running any
// process: the terminal path calls Executor.ShellCommand eagerly (it must
// build the *exec.Cmd to hand to tea.ExecProcess), while the buffered path
// only calls Executor.RunShell from inside the returned tea.Cmd's closure.
// Every test below therefore asserts on BOTH recorders -- the intended one
// received the expanded command, and the other was never called -- so a
// regression that silently routes a terminal action back through the
// buffered path (or vice versa) fails rather than passing on a half-match.

// expandedEchoTitle returns the command "echo {title}" as it must look after
// template expansion and shell escaping for b's currently selected card. It
// derives the value from the fixture card through the same public helpers
// production code uses, so no expected command string is hardcoded.
func expandedEchoTitle(b Board) string {
	selected := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	return "echo " + action.ShellEscape(action.Slugify(selected.Title))
}

func TestTerminalAction_DispatchHandsTerminalToCommand(t *testing.T) {
	actions := map[string]config.Action{
		"S": {Name: "Run tests", Type: "shell", Command: "echo {title}", Terminal: true},
	}
	b, fe := newActionTestBoard(t, actions)
	want := expandedEchoTitle(b)

	m, cmd := b.Update(keyMsg("S"))
	b = m.(Board)
	if cmd == nil {
		t.Fatal("dispatching a terminal: true action returned a nil tea.Cmd, want the tea.ExecProcess command")
	}
	if b.mode != normalMode {
		t.Errorf("mode after dispatching a terminal action = %v, want normalMode: suspending the board is BubbleTea's job, not a mode change", b.mode)
	}
	if len(fe.ShellCommandCalls) != 1 {
		t.Fatalf("ShellCommand calls = %v, want exactly one (the command handed to tea.ExecProcess)", fe.ShellCommandCalls)
	}
	if fe.ShellCommandCalls[0] != want {
		t.Errorf("ShellCommand called with %q, want %q (same expansion/escaping as a buffered shell action)", fe.ShellCommandCalls[0], want)
	}

	// Running the returned command must not fall back to the buffered path:
	// tea.ExecProcess's Cmd only yields a message the BubbleTea runtime acts
	// on, it never executes the process itself.
	execCmds(cmd)
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShell calls = %v, want none -- a terminal action must never go through the output-swallowing buffered path", fe.RunShellCalls)
	}
}

func TestShellAction_WithoutTerminal_StaysBuffered(t *testing.T) {
	actions := map[string]config.Action{
		"S": {Name: "Shell", Type: "shell", Command: "echo {title}"},
	}
	b, fe := newActionTestBoard(t, actions)
	want := expandedEchoTitle(b)

	m, cmd := b.Update(keyMsg("S"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after dispatching a shell action = %v, want normalMode", b.mode)
	}
	if len(fe.RunShellCalls) != 1 || fe.RunShellCalls[0] != want {
		t.Errorf("RunShell calls = %v, want exactly [%q]", fe.RunShellCalls, want)
	}
	if len(fe.ShellCommandCalls) != 0 {
		t.Errorf("ShellCommand calls = %v, want none -- an action without terminal: true must not take over the terminal", fe.ShellCommandCalls)
	}
}

// TestTerminalAction_FromLoadedConfig covers the whole real wiring main.go
// uses -- config.Load -> ResolveKeymap -> withKeymap -> registry dispatch --
// so the terminal flag surviving both Action conversions (config.Action ->
// keymap.Action in internal/config, keymap.Action -> config.Action in
// keymap_dispatch.go's configActionFromKeymap) is asserted end-to-end, not
// just on a hand-built fixture that bypasses them.
func TestTerminalAction_FromLoadedConfig(t *testing.T) {
	localYAML := `provider: github
repo: matteobortolazzo/lazyboards
keymaps:
  normal:
    T:
      name: Run tests
      type: shell
      terminal: true
      command: "echo {title}"
`
	b, fe := newConfigLoadedActionTestBoard(t, localYAML)
	want := expandedEchoTitle(b)

	m, cmd := b.Update(keyMsg("T"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after dispatching a terminal action = %v, want normalMode", b.mode)
	}
	if len(fe.ShellCommandCalls) != 1 || fe.ShellCommandCalls[0] != want {
		t.Errorf("ShellCommand calls = %v, want exactly [%q] -- terminal: true must survive config.Load and both Action conversions", fe.ShellCommandCalls, want)
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShell calls = %v, want none", fe.RunShellCalls)
	}
}

// TestTerminalAction_AltCommentOverload covers the deferred-execution path:
// Alt+key on a {comment} action opens comment mode and only dispatches on
// submit, through handleActionKeyWithComment rather than the immediate
// dispatch above. The terminal flag must be honored there too.
func TestTerminalAction_AltCommentOverload(t *testing.T) {
	actions := map[string]config.Action{
		"S": {Name: "Run with note", Type: "shell", Command: "echo {comment}", Terminal: true},
	}
	b, fe := newActionTestBoard(t, actions)

	m, _ := b.Update(altKeyMsg("S"))
	b = m.(Board)
	if b.mode != commentMode {
		t.Fatalf("mode after alt+S = %v, want commentMode", b.mode)
	}
	for _, ch := range "run it" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after submitting the comment = %v, want normalMode", b.mode)
	}
	want := "echo " + action.ShellEscape("run it")
	if len(fe.ShellCommandCalls) != 1 || fe.ShellCommandCalls[0] != want {
		t.Errorf("ShellCommand calls = %v, want exactly [%q]", fe.ShellCommandCalls, want)
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShell calls = %v, want none", fe.RunShellCalls)
	}
}

// TestTerminalAction_BoardScope pins that scope gating is unchanged: a
// board-scope terminal action runs with no card selected at all, where a
// card-scope action would silently refuse.
func TestTerminalAction_BoardScope(t *testing.T) {
	localYAML := `provider: github
repo: matteobortolazzo/lazyboards
keymaps:
  normal:
    T:
      name: Build
      type: shell
      scope: board
      terminal: true
      command: "make build"
`
	b, fe := newConfigLoadedEmptyColumnBoard(t, localYAML)

	m, cmd := b.Update(keyMsg("T"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after dispatching a board-scope terminal action = %v, want normalMode", b.mode)
	}
	if len(fe.ShellCommandCalls) != 1 || fe.ShellCommandCalls[0] != "make build" {
		t.Errorf("ShellCommand calls = %v, want exactly [%q]", fe.ShellCommandCalls, "make build")
	}
}

// --- terminalActionResult: what the board reports once the process exits ---
//
// The command's own output already went to the terminal, so the status bar
// only reports the outcome. These assert the ExecCallback tea.ExecProcess
// invokes on resume, which no test can reach through the returned tea.Cmd
// (BubbleTea's execMsg is unexported and only its runtime unwraps it).

func TestTerminalActionResult_CleanExit_ReportsSuccess(t *testing.T) {
	msg, ok := terminalActionResult(nil).(actionResultMsg)
	if !ok {
		t.Fatalf("terminalActionResult(nil) = %T, want actionResultMsg", terminalActionResult(nil))
	}
	if !msg.success {
		t.Errorf("success = false for a clean exit, want true (message %q)", msg.message)
	}
	if msg.message == "" {
		t.Error("message is empty, want the shared success message a buffered shell action reports")
	}
}

func TestTerminalActionResult_NonZeroExit_ReportsError(t *testing.T) {
	msg, ok := terminalActionResult(errors.New("exit status 1")).(actionResultMsg)
	if !ok {
		t.Fatal("terminalActionResult(err) did not return an actionResultMsg")
	}
	if msg.success {
		t.Error("success = true for a non-zero exit, want false")
	}
	if !strings.Contains(msg.message, "exit status 1") {
		t.Errorf("message = %q, want it to surface the process's exit error", msg.message)
	}
}

// TestTerminalActionResult_SanitizesHostileErrorText mirrors #547: an error
// string is untrusted text (it can carry whatever the spawned process wrote
// to its own error path) and must reach the one-line status bar flattened,
// with no newline and no ANSI escape byte.
func TestTerminalActionResult_SanitizesHostileErrorText(t *testing.T) {
	hostile := errors.New("exit status 1\n\x1b[31mfake board row\x1b[0m")
	msg, ok := terminalActionResult(hostile).(actionResultMsg)
	if !ok {
		t.Fatal("terminalActionResult(err) did not return an actionResultMsg")
	}
	if strings.ContainsAny(msg.message, "\n\r") {
		t.Errorf("message = %q, want no newline/carriage return", msg.message)
	}
	if strings.Contains(msg.message, "\x1b") {
		t.Errorf("message = %q, want no ANSI escape byte", msg.message)
	}
}

// terminalActionResult must satisfy tea.ExecCallback so it can be passed
// straight to tea.ExecProcess; a signature drift would otherwise only show
// up as a compile error inside commands.go.
var _ tea.ExecCallback = terminalActionResult
