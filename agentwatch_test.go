package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/agent-stack/agentwatch/pkg/watch"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/agentwatch"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// NOTE (ticket #257, RED phase): NewBoard is expected to gain a new trailing
// `watcher agentwatch.Watcher` parameter in the GREEN phase. These helpers are
// written against that intended signature; they intentionally do not compile
// until NewBoard (model.go) and its two call sites (main.go) are updated.
// See the ticket report for the full list of existing call sites that will
// also need the extra argument once NewBoard's signature changes.

// newAgentWatchTestBoard creates a Board (loadingMode) wired to the given
// watcher for agentwatch subscription/backoff tests.
func newAgentWatchTestBoard(t *testing.T, watcher agentwatch.Watcher) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	return NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, watcher)
}

// newAgentWatchCardTestBoard creates a loaded Board with a single card in a
// single column, using sessionMaxLen for BuildSessionName-based join tests.
func newAgentWatchCardTestBoard(t *testing.T, cardNumber int, cardTitle string, sessionMaxLen int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", sessionMaxLen, 0, 0, "Working", false, false, nil)

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
	return board
}

// collectCmdMsgs recursively executes a tea.Cmd, collecting every resulting
// message (including nested tea.BatchMsg). Mirrors execCmds in helpers_test.go
// but returns the messages instead of discarding them, so tests can assert on
// what a Cmd (e.g. Init()'s subscription) actually delivers. Uses a goroutine +
// timeout to avoid blocking on tea.Tick per lessons-learned.
func collectCmdMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(100 * time.Millisecond):
		return nil // Skip blocking commands (e.g., tea.Tick).
	}
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		var all []tea.Msg
		for _, subCmd := range batchMsg {
			all = append(all, collectCmdMsgs(subCmd)...)
		}
		return all
	}
	return []tea.Msg{msg}
}

// --- agentStatusFor: exact WindowName join (#257) ---

func TestBoard_AgentStatusFor_ExactWindowNameMatch(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newAgentWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	expectedName := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	snap := &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{WindowName: expectedName, Status: "running", Agent: "claude"},
		},
	}
	b.agentSnapshot = snap

	got := b.agentStatusFor(card)

	if got == nil {
		t.Fatalf("agentStatusFor() = nil, want a match for window %q", expectedName)
	}
	if got.Status != "running" {
		t.Errorf("agentStatusFor().Status = %q, want %q", got.Status, "running")
	}
	if got.Agent != "claude" {
		t.Errorf("agentStatusFor().Agent = %q, want %q", got.Agent, "claude")
	}
}

func TestBoard_AgentStatusFor_NoMatchingWindowReturnsNil(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newAgentWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{WindowName: "999-some-other-session", Status: "running"},
		},
	}

	if got := b.agentStatusFor(card); got != nil {
		t.Errorf("agentStatusFor() = %+v, want nil (no matching window)", got)
	}
}

func TestBoard_AgentStatusFor_CaseMismatchReturnsNil(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix Flaky Test"
	b := newAgentWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	expectedName := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	upper := strings.ToUpper(expectedName)
	if upper == expectedName {
		t.Fatalf("test setup: expected session name %q has no case-sensitive characters", expectedName)
	}

	// A window name differing only by case must NOT match: the join uses
	// exact string equality, not strings.EqualFold.
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{WindowName: upper, Status: "running"},
		},
	}

	if got := b.agentStatusFor(card); got != nil {
		t.Errorf("agentStatusFor() = %+v, want nil (case-insensitive match must not count)", got)
	}
}

func TestBoard_AgentStatusFor_NilSnapshotReturnsNil(t *testing.T) {
	b := newAgentWatchCardTestBoard(t, 7, "Fix flaky test", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	if b.agentSnapshot != nil {
		t.Fatal("test setup: agentSnapshot should be nil by default")
	}

	if got := b.agentStatusFor(card); got != nil {
		t.Errorf("agentStatusFor() = %+v, want nil (no snapshot stored yet)", got)
	}
}

// --- Update: agentWatchErrorMsg backoff growth (#257) ---

func TestBoard_Update_AgentWatchError_GrowsBackoffExponentiallyAndCaps(t *testing.T) {
	b := newAgentWatchTestBoard(t, &agentwatch.FakeWatcher{})
	initialStatusMessage := b.statusBar.message

	expectedBackoffs := []time.Duration{
		agentWatchInitialBackoff,
		2 * agentWatchInitialBackoff,
		4 * agentWatchInitialBackoff,
		8 * agentWatchInitialBackoff,
		16 * agentWatchInitialBackoff,
		agentWatchMaxBackoff, // 32x initial would exceed the cap.
		agentWatchMaxBackoff, // stays capped on further consecutive errors.
	}

	someErr := errors.New("connection refused")
	for i, want := range expectedBackoffs {
		m, cmd := b.Update(agentWatchErrorMsg{err: someErr})
		updated, ok := m.(Board)
		if !ok {
			t.Fatalf("Update returned %T, want Board", m)
		}
		b = updated

		if b.agentBackoff != want {
			t.Errorf("after error #%d: agentBackoff = %v, want %v", i+1, b.agentBackoff, want)
		}
		if cmd == nil {
			t.Errorf("after error #%d: cmd = nil, want non-nil retry cmd", i+1)
		}
	}

	// Acceptance criterion: connection errors are silent -- no status bar message.
	if b.statusBar.message != initialStatusMessage {
		t.Errorf("statusBar.message = %q after agentWatchErrorMsg, want unchanged %q (errors must be silent)", b.statusBar.message, initialStatusMessage)
	}
}

// --- Update: agentSnapshotMsg stores state, resets backoff, re-subscribes (#257) ---

func TestBoard_Update_AgentSnapshotMsg_StoresSnapshotAndResetsBackoff(t *testing.T) {
	b := newAgentWatchTestBoard(t, &agentwatch.FakeWatcher{})

	// Grow backoff past the initial value first, so we can observe the reset.
	m, _ := b.Update(agentWatchErrorMsg{err: errors.New("boom")})
	b = m.(Board)
	m, _ = b.Update(agentWatchErrorMsg{err: errors.New("boom")})
	b = m.(Board)
	if b.agentBackoff == agentWatchInitialBackoff {
		t.Fatal("test setup: agentBackoff should have grown past initial before the snapshot arrives")
	}

	snap := &watch.StateSnapshot{Timestamp: "2026-07-11T00:00:00Z"}
	m, cmd := b.Update(agentSnapshotMsg{snapshot: snap})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.agentSnapshot != snap {
		t.Errorf("agentSnapshot = %v, want %v (stored snapshot)", updated.agentSnapshot, snap)
	}
	if cmd == nil {
		t.Error("cmd = nil, want non-nil re-subscribe cmd")
	}

	// A successful snapshot resets the backoff ladder: the next error must
	// retry at the initial delay (1s), not a value doubled from before the reset.
	m, _ = updated.Update(agentWatchErrorMsg{err: errors.New("boom")})
	afterReset := m.(Board)
	if afterReset.agentBackoff != agentWatchInitialBackoff {
		t.Errorf("agentBackoff after reset then error = %v, want %v (ladder restarts at initial)", afterReset.agentBackoff, agentWatchInitialBackoff)
	}
}

// --- Update: agentWatchRetryMsg re-subscribes (#257) ---

func TestBoard_Update_AgentWatchRetryMsg_ReSubscribes(t *testing.T) {
	b := newAgentWatchTestBoard(t, &agentwatch.FakeWatcher{
		Results: []agentwatch.FakeWatcherResult{{Snap: &watch.StateSnapshot{}}},
	})

	m, cmd := b.Update(agentWatchRetryMsg{})
	if _, ok := m.(Board); !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Error("cmd = nil, want non-nil re-subscribe cmd")
	}
}

// --- Init gating: nil watcher means no subscription (#257) ---

func TestBoard_Init_NilWatcher_NoAgentWatchSubscription(t *testing.T) {
	b := newAgentWatchTestBoard(t, nil)

	cmd := b.Init()
	msgs := collectCmdMsgs(cmd)

	for _, msg := range msgs {
		switch msg.(type) {
		case agentSnapshotMsg, agentWatchErrorMsg:
			t.Fatalf("Init() with a nil watcher produced %T, want no agentwatch subscription messages", msg)
		}
	}
}

// --- Init gating: a configured watcher subscribes and delivers a snapshot (#257) ---

func TestBoard_Init_WithWatcher_SubscriptionDeliversSnapshot(t *testing.T) {
	snap := &watch.StateSnapshot{
		Timestamp: "2026-07-11T00:00:00Z",
		Windows: []watch.WindowState{
			{WindowName: "7-fix-flaky-test", Status: "running"},
		},
	}
	fw := &agentwatch.FakeWatcher{
		Results: []agentwatch.FakeWatcherResult{{Snap: snap}},
	}
	b := newAgentWatchTestBoard(t, fw)

	cmd := b.Init()
	msgs := collectCmdMsgs(cmd)

	var found bool
	for _, msg := range msgs {
		if snapMsg, ok := msg.(agentSnapshotMsg); ok {
			found = true
			if snapMsg.snapshot != snap {
				t.Errorf("agentSnapshotMsg.snapshot = %v, want %v", snapMsg.snapshot, snap)
			}
		}
	}
	if !found {
		t.Fatal("Init() with a scripted FakeWatcher did not deliver an agentSnapshotMsg")
	}
}
