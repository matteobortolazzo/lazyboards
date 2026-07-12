package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/matteobortolazzo/agent-stack/agentwatch/pkg/watch"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Agent jump (G key) (#256) ---
//
// Mirrors the "o" (open ticket) keybinding pattern (see TestTicketOpen_* in
// actions_test.go): press "G" to focus the tmux window backing a card's live
// agent session ("g" is already bound to the git panel). Requires being
// inside tmux ($TMUX set) and a matching, non-failed agentwatch window state.

// newAgentJumpTestBoard creates a loaded Board with a single card in a single
// column, wired to a FakeExecutor for SwitchToWindow assertions.
func newAgentJumpTestBoard(t *testing.T, cardNumber int, cardTitle string) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", config.DefaultSessionMaxLength, 0, 0, "Working", false, false, nil, nil)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: cardNumber, Title: cardTitle},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = 120
	board.Height = 40
	return board, fe
}

func TestAgentJump_NormalMode_HasSession_SwitchesToWindow(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b, fe := newAgentJumpTestBoard(t, cardNumber, cardTitle)

	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{Session: "work", WindowIndex: "3", WindowName: name, Status: "running"},
		},
	}

	b = sendKey(t, b, keyMsg("G"))

	if len(fe.SwitchWindowCalls) != 1 {
		t.Fatalf("SwitchWindowCalls length = %d, want 1", len(fe.SwitchWindowCalls))
	}
	if fe.SwitchWindowCalls[0].Session != "work" {
		t.Errorf("SwitchWindowCalls[0].Session = %q, want %q", fe.SwitchWindowCalls[0].Session, "work")
	}
	if fe.SwitchWindowCalls[0].WindowIndex != "3" {
		t.Errorf("SwitchWindowCalls[0].WindowIndex = %q, want %q", fe.SwitchWindowCalls[0].WindowIndex, "3")
	}
}

func TestAgentJump_NormalMode_SwitchFails_ShowsError(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b, fe := newAgentJumpTestBoard(t, cardNumber, cardTitle)
	fe.SwitchWindowErr = errors.New("select-window: exit status 1: can't find session work")

	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{Session: "work", WindowIndex: "3", WindowName: name, Status: "running"},
		},
	}

	b = sendKey(t, b, keyMsg("G"))

	if b.statusBar.level != StatusError {
		t.Errorf("statusBar.level = %v, want StatusError", b.statusBar.level)
	}
	if !strings.Contains(b.statusBar.message, fe.SwitchWindowErr.Error()) {
		t.Errorf("statusBar.message = %q, want it to contain %q", b.statusBar.message, fe.SwitchWindowErr.Error())
	}
}

func TestAgentJump_NormalMode_NoSession_NoOpWithMessage(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b, fe := newAgentJumpTestBoard(t, cardNumber, cardTitle)

	// No agentSnapshot stored at all -- agentStatusFor returns nil for every card.
	b = sendKey(t, b, keyMsg("G"))

	if len(fe.SwitchWindowCalls) != 0 {
		t.Errorf("expected no SwitchToWindow calls when no agent session matches, got %d", len(fe.SwitchWindowCalls))
	}
	if b.statusBar.message == "" {
		t.Error("expected a status bar message when no agent session matches, got empty")
	}
}

func TestAgentJump_NormalMode_FailedStatus_NoOpWithMessage(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b, fe := newAgentJumpTestBoard(t, cardNumber, cardTitle)

	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			// A "failed" entry is agentwatch's synthetic no-window marker for a
			// dispatch-failed/plan-invalid ticket -- nothing to jump to.
			{Session: "work", WindowIndex: "3", WindowName: name, Status: "failed"},
		},
	}

	b = sendKey(t, b, keyMsg("G"))

	if len(fe.SwitchWindowCalls) != 0 {
		t.Errorf("expected no SwitchToWindow calls for a failed-status entry, got %d", len(fe.SwitchWindowCalls))
	}
	if b.statusBar.message == "" {
		t.Error("expected a status bar message for a failed-status entry, got empty")
	}
}

func TestAgentJump_NormalMode_OutsideTmux_NoOpWithMessage(t *testing.T) {
	t.Setenv("TMUX", "")

	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b, fe := newAgentJumpTestBoard(t, cardNumber, cardTitle)

	// A live, matching session exists, but lazyboards is not running inside tmux.
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{Session: "work", WindowIndex: "3", WindowName: name, Status: "running"},
		},
	}

	b = sendKey(t, b, keyMsg("G"))

	if len(fe.SwitchWindowCalls) != 0 {
		t.Errorf("expected no SwitchToWindow calls outside tmux, got %d", len(fe.SwitchWindowCalls))
	}
	if b.statusBar.message == "" {
		t.Error("expected a status bar message when outside tmux, got empty")
	}
}

func TestAgentJump_DetailFocused_HasSession_SwitchesToWindow(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	const cardNumber = 9
	const cardTitle = "Detail panel card"
	b, fe := newAgentJumpTestBoard(t, cardNumber, cardTitle)

	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{Session: "review", WindowIndex: "5", WindowName: name, Status: "need_input"},
		},
	}

	// Enter detail focus, then press "G" to jump to the agent session.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("G"))

	if len(fe.SwitchWindowCalls) != 1 {
		t.Fatalf("SwitchWindowCalls length = %d, want 1", len(fe.SwitchWindowCalls))
	}
	if fe.SwitchWindowCalls[0].Session != "review" {
		t.Errorf("SwitchWindowCalls[0].Session = %q, want %q", fe.SwitchWindowCalls[0].Session, "review")
	}
	if fe.SwitchWindowCalls[0].WindowIndex != "5" {
		t.Errorf("SwitchWindowCalls[0].WindowIndex = %q, want %q", fe.SwitchWindowCalls[0].WindowIndex, "5")
	}
}

func TestAgentJump_DetailFocused_NoSession_NoOpWithMessage(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	const cardNumber = 9
	const cardTitle = "Detail panel card"
	b, fe := newAgentJumpTestBoard(t, cardNumber, cardTitle)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("G"))

	if len(fe.SwitchWindowCalls) != 0 {
		t.Errorf("expected no SwitchToWindow calls when no agent session matches, got %d", len(fe.SwitchWindowCalls))
	}
	if b.statusBar.message == "" {
		t.Error("expected a status bar message when no agent session matches, got empty")
	}
}

func TestAgentJump_DetailFocused_FailedStatus_NoOpWithMessage(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux-1000/default,12345,0")

	const cardNumber = 9
	const cardTitle = "Detail panel card"
	b, fe := newAgentJumpTestBoard(t, cardNumber, cardTitle)

	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{Session: "review", WindowIndex: "5", WindowName: name, Status: "failed"},
		},
	}

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("G"))

	if len(fe.SwitchWindowCalls) != 0 {
		t.Errorf("expected no SwitchToWindow calls for a failed-status entry, got %d", len(fe.SwitchWindowCalls))
	}
	if b.statusBar.message == "" {
		t.Error("expected a status bar message for a failed-status entry, got empty")
	}
}

func TestAgentJump_DetailFocused_OutsideTmux_NoOpWithMessage(t *testing.T) {
	t.Setenv("TMUX", "")

	const cardNumber = 9
	const cardTitle = "Detail panel card"
	b, fe := newAgentJumpTestBoard(t, cardNumber, cardTitle)

	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{Session: "review", WindowIndex: "5", WindowName: name, Status: "running"},
		},
	}

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("G"))

	if len(fe.SwitchWindowCalls) != 0 {
		t.Errorf("expected no SwitchToWindow calls outside tmux, got %d", len(fe.SwitchWindowCalls))
	}
	if b.statusBar.message == "" {
		t.Error("expected a status bar message when outside tmux, got empty")
	}
}

// TestAgentStatusFailed_ConstantMatchesLiteral guards the agentStatusFailed
// constant against drifting from the "failed" literal used across the
// agentwatch join (agentBadgeText, agentCounts, agentStatusFor callers).
func TestAgentStatusFailed_ConstantMatchesLiteral(t *testing.T) {
	if agentStatusFailed != "failed" {
		t.Errorf("agentStatusFailed = %q, want %q", agentStatusFailed, "failed")
	}
	if !strings.Contains(agentStatusFailed, "fail") {
		t.Errorf("agentStatusFailed = %q, want it to describe a failed dispatch", agentStatusFailed)
	}
}
