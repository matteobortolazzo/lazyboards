package main

import (
	"errors"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/action"
)

// resolveTmuxSession returns the tmux session name this lazyboards instance
// runs in, used to scope the agents list to its own session (#410).

func TestResolveTmuxSession_NotInsideTmux_ReturnsEmpty(t *testing.T) {
	t.Setenv("TMUX", "")
	fe := &action.FakeExecutor{RunShellOutputStdout: "cenci\n"}

	if got := resolveTmuxSession(fe); got != "" {
		t.Errorf("resolveTmuxSession outside tmux = %q, want empty", got)
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("ran %d shell calls outside tmux, want 0 (no $TMUX, no query)", len(fe.RunShellOutputCalls))
	}
}

func TestResolveTmuxSession_InsideTmux_ReturnsSessionName(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	fe := &action.FakeExecutor{RunShellOutputStdout: "cenci\n"}

	if got := resolveTmuxSession(fe); got != "cenci" {
		t.Errorf("resolveTmuxSession = %q, want %q", got, "cenci")
	}
	if len(fe.RunShellOutputCalls) != 1 || fe.RunShellOutputCalls[0] != "tmux display-message -p '#S'" {
		t.Errorf("shell calls = %v, want a single 'tmux display-message -p #S'", fe.RunShellOutputCalls)
	}
}

func TestResolveTmuxSession_QueryError_ReturnsEmpty(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1234,0")
	fe := &action.FakeExecutor{RunShellOutputErr: errors.New("no server running")}

	if got := resolveTmuxSession(fe); got != "" {
		t.Errorf("resolveTmuxSession on query error = %q, want empty", got)
	}
}
