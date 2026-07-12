package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// newDispatchTestBoard creates a loaded Board with Width/Height set so View()
// can render normal-mode hints for assertion.
func newDispatchTestBoard(t *testing.T) Board {
	t.Helper()
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	return b
}

// --- Mode transition ---

func TestDispatch_PressD_EntersDispatchMode(t *testing.T) {
	b := newDispatchTestBoard(t)

	b = sendKey(t, b, keyMsg("d"))

	if b.mode != dispatchMode {
		t.Errorf("after pressing 'd': mode = %v, want dispatchMode", b.mode)
	}
}

func TestDispatch_PressD_ResetsStateToLoading(t *testing.T) {
	b := newDispatchTestBoard(t)

	b = sendKey(t, b, keyMsg("d"))

	if !b.dispatch.loading {
		t.Error("after pressing 'd': dispatch.loading = false, want true")
	}
	if b.dispatch.err != "" {
		t.Errorf("after pressing 'd': dispatch.err = %q, want empty", b.dispatch.err)
	}
	if b.dispatch.running {
		t.Error("after pressing 'd': dispatch.running = true, want false")
	}
}

func TestDispatch_Escape_ReturnsToNormalMode(t *testing.T) {
	b := newDispatchTestBoard(t)

	b = sendKey(t, b, keyMsg("d"))
	if b.mode != dispatchMode {
		t.Fatalf("expected dispatchMode after 'd', got %v", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Escape in dispatchMode: mode = %v, want normalMode", b.mode)
	}
}

func TestDispatch_Escape_RestoresNormalHints(t *testing.T) {
	b := newDispatchTestBoard(t)

	b = sendKey(t, b, keyMsg("d"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	view := b.View()
	if !strings.Contains(view, "Edit") {
		t.Error("after Escape from dispatchMode, normal hint 'Edit' should be visible")
	}
}

// --- Noop while loading/error/running ---

func TestDispatch_EnterNoop_WhileLoading(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch.loading = true

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != dispatchMode {
		t.Errorf("Enter while dispatch.loading=true: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}

func TestDispatch_ONoop_WhileLoading(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch.loading = true

	b = sendKey(t, b, keyMsg("o"))

	if b.mode != dispatchMode {
		t.Errorf("'o' while dispatch.loading=true: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}

func TestDispatch_EnterNoop_WhileError(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch.loading = false
	b.dispatch.err = "x"

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != dispatchMode {
		t.Errorf("Enter while dispatch.err set: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}

func TestDispatch_EnterNoop_WhileRunning(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch.loading = false
	b.dispatch.running = true

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != dispatchMode {
		t.Errorf("Enter while dispatch.running=true: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}

// TestDispatch_EnterAndO_NoopWhenClean exercises the fall-through no-op path:
// with dispatch fully zeroed (guard condition false), enter/"o" are still
// no-ops in this ticket's scope (#283 fills that branch with real behavior).
func TestDispatch_EnterAndO_NoopWhenClean(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = sendKey(t, b, keyMsg("d"))
	b.dispatch = dispatchState{} // loading=false, err="", running=false

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.mode != dispatchMode {
		t.Errorf("Enter with clean dispatch state: mode = %v, want dispatchMode (no-op)", b.mode)
	}

	b = sendKey(t, b, keyMsg("o"))
	if b.mode != dispatchMode {
		t.Errorf("'o' with clean dispatch state: mode = %v, want dispatchMode (no-op)", b.mode)
	}
}
