package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// NOTE (ticket #257, RED phase): NewBoard is expected to gain a new trailing
// `watcher cenciwatch.Watcher` parameter in the GREEN phase. These helpers are
// written against that intended signature; they intentionally do not compile
// until NewBoard (model.go) and its two call sites (main.go) are updated.
// See the ticket report for the full list of existing call sites that will
// also need the extra argument once NewBoard's signature changes.

// newCenciWatchTestBoard creates a Board (loadingMode) wired to the given
// watcher for cenci-watch subscription/backoff tests.
func newCenciWatchTestBoard(t *testing.T, watcher cenciwatch.Watcher) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	return NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, watcher, nil)
}

// newCenciWatchCardTestBoard creates a loaded Board with a single card in a
// single column, using sessionMaxLen for BuildSessionName-based join tests.
func newCenciWatchCardTestBoard(t *testing.T, cardNumber int, cardTitle string, sessionMaxLen int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", sessionMaxLen, 0, 0, "Working", false, false, nil, nil)

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

// --- agentStatusForNumber: ticket-number-prefix join (#257, <number>-<skill> names) ---

// A dispatched <number>-<skill> window joins to its card by ticket number.
func TestBoard_AgentStatusFor_SkillWindowMatchesByNumber(t *testing.T) {
	const cardNumber = 230
	b := newCenciWatchCardTestBoard(t, cardNumber, "Refine the thing", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "230-refine", Status: "running", Agent: "claude"},
		},
	}

	got := b.agentStatusForNumber(card.Number)
	if got == nil {
		t.Fatalf("agentStatusForNumber() = nil, want a match for window 230-refine")
	}
	if got.Status != "running" {
		t.Errorf("agentStatusForNumber().Status = %q, want %q", got.Status, "running")
	}
	if got.Agent != "claude" {
		t.Errorf("agentStatusForNumber().Agent = %q, want %q", got.Agent, "claude")
	}
}

// A legacy <number>-<title-slug> window still joins by number prefix, so
// lazyboards can ship before cenci changes its window naming.
func TestBoard_AgentStatusFor_LegacyTitleSlugWindowStillMatches(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newCenciWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	legacyName := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: legacyName, Status: "running", Agent: "claude"},
		},
	}

	if got := b.agentStatusForNumber(card.Number); got == nil {
		t.Fatalf("agentStatusForNumber() = nil, want a match for legacy window %q", legacyName)
	}
}

// A bare <number> window (no slug, no skill) joins.
func TestBoard_AgentStatusFor_BareNumberWindowMatches(t *testing.T) {
	const cardNumber = 42
	b := newCenciWatchCardTestBoard(t, cardNumber, "Whatever", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "42", Status: "need-input"},
		},
	}

	if got := b.agentStatusForNumber(card.Number); got == nil || got.Status != "need-input" {
		t.Errorf("agentStatusForNumber() = %+v, want the bare-number window (need_input)", got)
	}
}

// The trailing "-" boundary keeps card #23 from matching a 230-... window.
func TestBoard_AgentStatusFor_NumberPrefixBoundary(t *testing.T) {
	const cardNumber = 23
	b := newCenciWatchCardTestBoard(t, cardNumber, "Boundary", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "230-refine", Status: "running"},
		},
	}

	if got := b.agentStatusForNumber(card.Number); got != nil {
		t.Errorf("agentStatusForNumber() = %+v, want nil (23 must not match 230-refine)", got)
	}
}

// When several windows share the ticket number, an active one (running /
// need_input) wins over any other status, regardless of snapshot order.
func TestBoard_AgentStatusFor_PrefersActiveWindow(t *testing.T) {
	const cardNumber = 55
	b := newCenciWatchCardTestBoard(t, cardNumber, "Multi", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "55-implement", Status: "done"},
			{WindowName: "55-refine", Status: "running", Agent: "claude"},
		},
	}

	got := b.agentStatusForNumber(card.Number)
	if got == nil {
		t.Fatalf("agentStatusForNumber() = nil, want the active window")
	}
	if got.Status != "running" || got.WindowName != "55-refine" {
		t.Errorf("agentStatusForNumber() = %+v, want the running 55-refine window", got)
	}
}

func TestBoard_AgentStatusFor_NoMatchingWindowReturnsNil(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newCenciWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{
			{WindowName: "999-some-other-session", Status: "running"},
		},
	}

	if got := b.agentStatusForNumber(card.Number); got != nil {
		t.Errorf("agentStatusForNumber() = %+v, want nil (no matching window)", got)
	}
}

func TestBoard_AgentStatusFor_NilSnapshotReturnsNil(t *testing.T) {
	b := newCenciWatchCardTestBoard(t, 7, "Fix flaky test", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	if b.agentSnapshot != nil {
		t.Fatal("test setup: agentSnapshot should be nil by default")
	}

	if got := b.agentStatusForNumber(card.Number); got != nil {
		t.Errorf("agentStatusForNumber() = %+v, want nil (no snapshot stored yet)", got)
	}
}

// --- Update: cenciWatchErrorMsg backoff growth (#257) ---

func TestBoard_Update_CenciWatchError_GrowsBackoffExponentiallyAndCaps(t *testing.T) {
	b := newCenciWatchTestBoard(t, &cenciwatch.FakeWatcher{})
	initialStatusMessage := b.statusBar.message

	expectedBackoffs := []time.Duration{
		cenciWatchInitialBackoff,
		2 * cenciWatchInitialBackoff,
		4 * cenciWatchInitialBackoff,
		8 * cenciWatchInitialBackoff,
		16 * cenciWatchInitialBackoff,
		cenciWatchMaxBackoff, // 32x initial would exceed the cap.
		cenciWatchMaxBackoff, // stays capped on further consecutive errors.
	}

	someErr := errors.New("connection refused")
	for i, want := range expectedBackoffs {
		m, cmd := b.Update(cenciWatchErrorMsg{err: someErr})
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
		t.Errorf("statusBar.message = %q after cenciWatchErrorMsg, want unchanged %q (errors must be silent)", b.statusBar.message, initialStatusMessage)
	}
}

// --- Update: agentSnapshotMsg stores state, resets backoff, re-subscribes (#257) ---

func TestBoard_Update_AgentSnapshotMsg_StoresSnapshotAndResetsBackoff(t *testing.T) {
	b := newCenciWatchTestBoard(t, &cenciwatch.FakeWatcher{})

	// Grow backoff past the initial value first, so we can observe the reset.
	m, _ := b.Update(cenciWatchErrorMsg{err: errors.New("boom")})
	b = m.(Board)
	m, _ = b.Update(cenciWatchErrorMsg{err: errors.New("boom")})
	b = m.(Board)
	if b.agentBackoff == cenciWatchInitialBackoff {
		t.Fatal("test setup: agentBackoff should have grown past initial before the snapshot arrives")
	}

	snap := &cenciwatch.StateSnapshot{Timestamp: "2026-07-11T00:00:00Z"}
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
	m, _ = updated.Update(cenciWatchErrorMsg{err: errors.New("boom")})
	afterReset := m.(Board)
	if afterReset.agentBackoff != cenciWatchInitialBackoff {
		t.Errorf("agentBackoff after reset then error = %v, want %v (ladder restarts at initial)", afterReset.agentBackoff, cenciWatchInitialBackoff)
	}
}

// --- Update: cenciWatchRetryMsg re-subscribes (#257) ---

func TestBoard_Update_CenciWatchRetryMsg_ReSubscribes(t *testing.T) {
	b := newCenciWatchTestBoard(t, &cenciwatch.FakeWatcher{
		Results: []cenciwatch.FakeWatcherResult{{Snap: &cenciwatch.StateSnapshot{}}},
	})

	m, cmd := b.Update(cenciWatchRetryMsg{})
	if _, ok := m.(Board); !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Error("cmd = nil, want non-nil re-subscribe cmd")
	}
}

// --- Init gating: nil watcher means no subscription (#257) ---

func TestBoard_Init_NilWatcher_NoCenciWatchSubscription(t *testing.T) {
	b := newCenciWatchTestBoard(t, nil)

	cmd := b.Init()
	msgs := collectCmdMsgs(cmd)

	for _, msg := range msgs {
		switch msg.(type) {
		case agentSnapshotMsg, cenciWatchErrorMsg:
			t.Fatalf("Init() with a nil watcher produced %T, want no cenci-watch subscription messages", msg)
		}
	}
}

// --- Init gating: a configured watcher subscribes and delivers a snapshot (#257) ---

func TestBoard_Init_WithWatcher_SubscriptionDeliversSnapshot(t *testing.T) {
	snap := &cenciwatch.StateSnapshot{
		Timestamp: "2026-07-11T00:00:00Z",
		Windows: []cenciwatch.WindowState{
			{WindowName: "7-fix-flaky-test", Status: "running"},
		},
	}
	fw := &cenciwatch.FakeWatcher{
		Results: []cenciwatch.FakeWatcherResult{{Snap: snap}},
	}
	b := newCenciWatchTestBoard(t, fw)

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

// --- Board-scoped agent counts (#259) ---

// newAgentCountsBoard creates a loaded Board with the given cards in a single
// column, using DefaultSessionMaxLength for BuildSessionName-based joins.
func newAgentCountsBoard(t *testing.T, cards []provider.Card) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", config.DefaultSessionMaxLength, 0, 0, "Working", false, false, nil, nil)
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{Title: "Column A", Cards: cards}},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return board
}

// TestBoard_AgentCounts_CountsAllWindows verifies agentCounts tallies every
// window in the snapshot — matched to a board card or not — matching the
// agents list modal's all-windows-in-scope behavior. Unlike the pre-#420
// two-state tally, idle now counts too (#420 extends agentCounts to all six
// states), and the tally is a count (not a boolean) so multiple active
// windows accumulate. b.tmuxSession is unset here, so sessionScopedWindows
// falls back to "every tracked window" (see TestBoard_AgentCounts_ScopedToInstanceSession
// for the scoped case).
func TestBoard_AgentCounts_CountsAllWindows(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "First running"},
		{Number: 2, Title: "Needs input"},
		{Number: 3, Title: "Idle card"},
		{Number: 4, Title: "Second running"},
	}
	b := newAgentCountsBoard(t, cards)

	name := func(n int, title string) string {
		return action.BuildSessionName(n, title, config.DefaultSessionMaxLength)
	}
	b.agentSnapshot = &cenciwatch.StateSnapshot{Windows: []cenciwatch.WindowState{
		{WindowName: name(1, "First running"), Status: "running"},
		{WindowName: name(2, "Needs input"), Status: "need-input"},
		{WindowName: name(3, "Idle card"), Status: "idle"}, // counted: idle is one of the six states
		{WindowName: name(4, "Second running"), Status: "running"},
		{WindowName: "999-no-such-card", Status: "running"},    // counted: no card joins it
		{WindowName: "888-no-such-card", Status: "need-input"}, // counted: no card joins it
	}}

	running, needInput, done, failed, stopped, idle := b.agentCounts()

	if running != 3 {
		t.Errorf("agentCounts() running = %d, want 3 (two matched + one unmatched running window)", running)
	}
	if needInput != 2 {
		t.Errorf("agentCounts() needInput = %d, want 2 (one matched + one unmatched need_input window)", needInput)
	}
	if idle != 1 {
		t.Errorf("agentCounts() idle = %d, want 1 (the idle card's window)", idle)
	}
	if done != 0 || failed != 0 || stopped != 0 {
		t.Errorf("agentCounts() done/failed/stopped = (%d, %d, %d), want (0, 0, 0): no windows in those states", done, failed, stopped)
	}
}

// TestBoard_AgentCounts_CountsAllSixStates verifies agentCounts surfaces every
// one of the six window statuses the daemon reports (running, need-input,
// done, failed, stopped, idle) individually, and excludes an unrecognized
// status from all six tallies (#420 acceptance: "surface all six states").
func TestBoard_AgentCounts_CountsAllSixStates(t *testing.T) {
	b := newAgentCountsBoard(t, []provider.Card{{Number: 1, Title: "A card"}})
	b.agentSnapshot = &cenciwatch.StateSnapshot{Windows: []cenciwatch.WindowState{
		{WindowName: "100-a", Status: agentStatusRunning},
		{WindowName: "101-b", Status: agentStatusNeedInput},
		{WindowName: "102-c", Status: "done"},
		{WindowName: "103-d", Status: agentStatusFailed},
		{WindowName: "104-e", Status: "stopped"},
		{WindowName: "105-f", Status: "idle"},
		{WindowName: "106-g", Status: "banana"}, // unrecognized status: excluded from every tally
	}}

	running, needInput, done, failed, stopped, idle := b.agentCounts()

	if running != 1 {
		t.Errorf("agentCounts() running = %d, want 1", running)
	}
	if needInput != 1 {
		t.Errorf("agentCounts() needInput = %d, want 1", needInput)
	}
	if done != 1 {
		t.Errorf("agentCounts() done = %d, want 1", done)
	}
	if failed != 1 {
		t.Errorf("agentCounts() failed = %d, want 1", failed)
	}
	if stopped != 1 {
		t.Errorf("agentCounts() stopped = %d, want 1", stopped)
	}
	if idle != 1 {
		t.Errorf("agentCounts() idle = %d, want 1", idle)
	}
}

// TestBoard_AgentCounts_ScopedToInstanceSession verifies agentCounts iterates
// sessionScopedWindows() rather than the raw, unfiltered snapshot: windows in
// a different tmux session than this lazyboards instance must not contribute
// to any of the six tallies (#420 acceptance: population scope must match
// agentListEntries).
func TestBoard_AgentCounts_ScopedToInstanceSession(t *testing.T) {
	b := newAgentCountsBoard(t, []provider.Card{{Number: 1, Title: "A card"}})
	b.tmuxSession = "dev"
	b.agentSnapshot = &cenciwatch.StateSnapshot{Windows: []cenciwatch.WindowState{
		{Session: "dev", WindowName: "1-a", Status: agentStatusRunning},
		{Session: "dev", WindowName: "2-b", Status: "done"},
		{Session: "ops", WindowName: "3-c", Status: agentStatusRunning},   // different session: excluded
		{Session: "ops", WindowName: "4-d", Status: agentStatusNeedInput}, // different session: excluded
		{Session: "ops", WindowName: "5-e", Status: "stopped"},            // different session: excluded
	}}

	running, needInput, done, failed, stopped, idle := b.agentCounts()

	if running != 1 {
		t.Errorf("agentCounts() running = %d, want 1 (only the dev-session running window)", running)
	}
	if done != 1 {
		t.Errorf("agentCounts() done = %d, want 1 (only the dev-session done window)", done)
	}
	if needInput != 0 || failed != 0 || stopped != 0 || idle != 0 {
		t.Errorf("agentCounts() needInput/failed/stopped/idle = (%d, %d, %d, %d), want all 0: those windows are all in the out-of-session \"ops\" session",
			needInput, failed, stopped, idle)
	}
}

// TestBoard_AgentCounts_NilSnapshotIsZero verifies that with no snapshot stored
// (cenci off/absent) all six counts are zero.
func TestBoard_AgentCounts_NilSnapshotIsZero(t *testing.T) {
	b := newAgentCountsBoard(t, []provider.Card{{Number: 1, Title: "A card"}})
	if b.agentSnapshot != nil {
		t.Fatal("test setup: agentSnapshot should be nil by default")
	}

	running, needInput, done, failed, stopped, idle := b.agentCounts()
	if running != 0 || needInput != 0 || done != 0 || failed != 0 || stopped != 0 || idle != 0 {
		t.Errorf("agentCounts() = (%d, %d, %d, %d, %d, %d), want all zero when no snapshot is stored",
			running, needInput, done, failed, stopped, idle)
	}
}

// --- Card status badges (#258) ---

// TestAgentBadgeText_StatusSymbolAndKind verifies the badge text encodes the
// state as a symbol and includes the agent kind, and that idle/unknown statuses
// produce no badge.
func TestAgentBadgeText_StatusSymbolAndKind(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		agent      string
		wantSymbol string // "" means no badge expected
	}{
		{"running", "running", "claude", "▶"},
		{"done", "done", "claude", "✓"},
		{"stopped", "stopped", "claude", "■"},
		{"need-input", "need-input", "claude", "!"},
		{"failed", "failed", "claude", "✗"},
		{"idle has no badge", "idle", "claude", ""},
		{"unknown has no badge", "banana", "claude", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := agentBadgeText(tt.status, tt.agent)
			if tt.wantSymbol == "" {
				if got != "" {
					t.Errorf("agentBadgeText(%q, %q) = %q, want empty", tt.status, tt.agent, got)
				}
				return
			}
			if !strings.Contains(got, tt.wantSymbol) {
				t.Errorf("agentBadgeText(%q, %q) = %q, want to contain symbol %q", tt.status, tt.agent, got, tt.wantSymbol)
			}
			if !strings.Contains(got, tt.agent) {
				t.Errorf("agentBadgeText(%q, %q) = %q, want to contain kind %q", tt.status, tt.agent, got, tt.agent)
			}
		})
	}
}

// TestBoard_AgentBadge_NeedInputFromDaemonWireFormat decodes a snapshot line in
// the exact shape the cenci-watch daemon broadcasts over its socket — including
// the "need-input" (hyphen) status the daemon's detect.StatusNeedInput.String()
// emits — and asserts a need-input agent badges its card and counts toward the
// status-bar summary. Constructing the snapshot from the raw NDJSON (rather than
// a hand-built WindowState) pins the status token to the daemon's real wire
// value: if lazyboards drifts back to the "need_input" underscore spelling, the
// badge silently disappears and this test fails.
func TestBoard_AgentBadge_NeedInputFromDaemonWireFormat(t *testing.T) {
	const cardNumber = 42
	b := newCenciWatchCardTestBoard(t, cardNumber, "Waiting for input", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	// A single NDJSON line as emitted by the daemon's broadcast socket.
	const wireLine = `{"timestamp":"2026-07-13T22:09:50Z","windows":[{"session":"lazyboards","window_index":"2","window_name":"42-implement","task_name":"Waiting for input","status":"need-input","agent":"claude","manually_named":false}],"summary":{"total":1,"need_input":1}}`

	var snap cenciwatch.StateSnapshot
	if err := json.Unmarshal([]byte(wireLine), &snap); err != nil {
		t.Fatalf("failed to decode daemon wire line: %v", err)
	}
	b.agentSnapshot = &snap

	ws := b.agentStatusForNumber(card.Number)
	if ws == nil {
		t.Fatalf("agentStatusForNumber() = nil, want the need-input window for card #%d", cardNumber)
	}
	if badge := agentBadgeText(ws.Status, ws.Agent); badge == "" {
		t.Errorf("agentBadgeText() = %q, want a non-empty badge for a need-input agent", badge)
	}
	if _, needInput, _, _, _, _ := b.agentCounts(); needInput != 1 {
		t.Errorf("agentCounts() needInput = %d, want 1 for one matched need-input card", needInput)
	}
}

// TestAgentBadgeText_FixedWidth verifies the kind is padded/truncated to a
// stable cell width so badges align across cards regardless of agent length.
func TestAgentBadgeText_FixedWidth(t *testing.T) {
	short := agentBadgeText("running", "cl")
	long := agentBadgeText("running", "verylongagentname")
	if lipgloss.Width(short) != lipgloss.Width(long) {
		t.Errorf("badge width not stable: short=%q (%d) long=%q (%d)",
			short, lipgloss.Width(short), long, lipgloss.Width(long))
	}
	if !strings.Contains(long, "▶") {
		t.Errorf("truncated badge %q lost its symbol", long)
	}
}

// TestAgentBadgeText_EmptyAgentSymbolOnly verifies a symbol-only badge when the
// window has no detected agent kind.
func TestAgentBadgeText_EmptyAgentSymbolOnly(t *testing.T) {
	got := agentBadgeText("running", "")
	if got != "▶" {
		t.Errorf("agentBadgeText(running, \"\") = %q, want the bare symbol %q", got, "▶")
	}
}

// TestBoard_AgentBadgeFor_AppearsAsSeparateStatusLine verifies a matching
// non-idle window's badge renders as its own status line via
// cardStatusLines (#439) -- it no longer appends to cardDisplayText's
// returned title text.
func TestBoard_AgentBadgeFor_AppearsAsSeparateStatusLine(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newCenciWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{{WindowName: name, Status: "running", Agent: "claude"}},
	}

	ws := b.agentStatusForNumber(card.Number)
	if ws == nil {
		t.Fatal("agentStatusForNumber() = nil, want a match for the running window")
	}
	badge := agentBadgeText(ws.Status, ws.Agent)
	if badge == "" {
		t.Fatal("agentBadgeText() returned empty for a matching running window")
	}

	// cardDisplayText's title text must NOT contain the badge -- it moved to
	// its own status line.
	text, indentWidth := cardDisplayText(card, []string{"Column A"}, b.workingLabel)
	if strings.Contains(text, badge) {
		t.Errorf("cardDisplayText() title %q should not contain the agent badge %q (badge moved to its own status line)", text, badge)
	}

	// The badge appears instead as a line from cardStatusLines.
	lines := b.cardStatusLines(card, indentWidth)
	found := false
	for _, line := range lines {
		if strings.Contains(line, badge) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cardStatusLines() = %v, want a line containing the agent badge %q", lines, badge)
	}
}

// TestBoard_AgentBadgeFor_NoBadgeCases verifies idle status, a non-matching
// window, and a nil snapshot all yield no badge.
func TestBoard_AgentBadgeFor_NoBadgeCases(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)

	tests := []struct {
		name string
		snap *cenciwatch.StateSnapshot
	}{
		{"idle status", &cenciwatch.StateSnapshot{Windows: []cenciwatch.WindowState{{WindowName: name, Status: "idle", Agent: "claude"}}}},
		{"non-matching window", &cenciwatch.StateSnapshot{Windows: []cenciwatch.WindowState{{WindowName: "999-other", Status: "running"}}}},
		{"nil snapshot", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newCenciWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
			b.agentSnapshot = tt.snap
			card := b.Columns[0].Cards[0]
			badge := ""
			if ws := b.agentStatusForNumber(card.Number); ws != nil {
				badge = agentBadgeText(ws.Status, ws.Agent)
			}
			if badge != "" {
				t.Errorf("badge = %q, want empty", badge)
			}
		})
	}
}

// TestViewCardList_RunningBadgeRendered verifies the running badge (kind +
// symbol) appears in the rendered card list.
func TestViewCardList_RunningBadgeRendered(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newCenciWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{{WindowName: name, Status: "running", Agent: "claude"}},
	}

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	if !strings.Contains(out, "▶") {
		t.Errorf("rendered card list missing running symbol; got:\n%s", out)
	}
	if !strings.Contains(out, "claude") {
		t.Errorf("rendered card list missing agent kind; got:\n%s", out)
	}
}

// TestViewCardList_NeedInputRendersSingleMarkInRed verifies need-input renders
// as a single "!" mark styled via agentNeedInputStyle — consistent with the
// other single-mark status badges, no reverse/background.
func TestViewCardList_NeedInputRendersSingleMarkInRed(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newCenciWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{{WindowName: name, Status: "need-input", Agent: "claude"}},
	}

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	if !strings.Contains(out, agentNeedInputStyle.Render("!")) {
		t.Errorf("need-input badge not rendered with agentNeedInputStyle; got:\n%s", out)
	}
	if agentNeedInputStyle.GetReverse() {
		t.Error("agentNeedInputStyle should not use Reverse (no background swap); want plain colored text")
	}
}

// TestViewCardList_IdleRendersNoSymbol verifies an idle (or non-matching) window
// leaves the card free of any status symbol.
func TestViewCardList_IdleRendersNoSymbol(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newCenciWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{{WindowName: name, Status: "idle", Agent: "claude"}},
	}

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	for _, sym := range []string{"▶", "✓", "■", "!", "✗"} {
		if strings.Contains(out, sym) {
			t.Errorf("idle card unexpectedly rendered status symbol %q; got:\n%s", sym, out)
		}
	}
}

// TestViewCardList_WorkingLabelAndBadgeCoexist verifies a card carrying the
// working label renders both the working spinner icon and the agent badge.
func TestViewCardList_WorkingLabelAndBadgeCoexist(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", config.DefaultSessionMaxLength, 0, 0, "Working", false, false, nil, nil)
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: cardNumber, Title: cardTitle, Labels: []provider.Label{{Name: "Working"}}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)

	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{{WindowName: name, Status: "running", Agent: "claude"}},
	}

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	if !strings.Contains(out, "") {
		t.Errorf("working spinner icon missing; got:\n%s", out)
	}
	if !strings.Contains(out, "▶") {
		t.Errorf("running badge symbol missing; got:\n%s", out)
	}
}

// TestAgentStatusFailed_ConstantMatchesLiteral guards the agentStatusFailed
// constant against drifting from the "failed" literal used across the
// cenci join (agentBadgeText, agentCounts, agentStatusForNumber callers).
func TestAgentStatusFailed_ConstantMatchesLiteral(t *testing.T) {
	if agentStatusFailed != "failed" {
		t.Errorf("agentStatusFailed = %q, want %q", agentStatusFailed, "failed")
	}
	if !strings.Contains(agentStatusFailed, "fail") {
		t.Errorf("agentStatusFailed = %q, want it to describe a failed dispatch", agentStatusFailed)
	}
}

// --- Update: agentSnapshotMsg / cenciWatchErrorMsg wire the dispatch status
// bar segment (#315) ---

// TestUpdate_AgentSnapshotMsg_DispatchEnabled_SetsDispatchStatusSegment
// verifies a snapshot carrying an enabled dispatch loop sets a non-empty
// status bar segment, mirroring the existing gitStatusMsg wiring test
// (TestUpdate_GitStatusMsg_Success_SetsGitStatusSegment in
// gitstatus_wiring_test.go).
func TestUpdate_AgentSnapshotMsg_DispatchEnabled_SetsDispatchStatusSegment(t *testing.T) {
	b := newCenciWatchTestBoard(t, &cenciwatch.FakeWatcher{})
	snap := &cenciwatch.StateSnapshot{
		Dispatch: &cenciwatch.DispatchState{Enabled: true, DaemonRunning: true},
	}

	m, _ := b.Update(agentSnapshotMsg{snapshot: snap})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.statusBar.dispatchStatus == "" {
		t.Fatal("agentSnapshotMsg with Dispatch.Enabled=true should set a non-empty dispatch status segment")
	}
}

// TestUpdate_AgentSnapshotMsg_DispatchDisabled_ClearsDispatchStatusSegment
// verifies the segment is hidden (cleared) once a snapshot reports the loop
// disabled, even if a previous snapshot had it set.
func TestUpdate_AgentSnapshotMsg_DispatchDisabled_ClearsDispatchStatusSegment(t *testing.T) {
	b := newCenciWatchTestBoard(t, &cenciwatch.FakeWatcher{})
	b.statusBar.SetDispatchStatus("⟳ dispatch")

	snap := &cenciwatch.StateSnapshot{
		Dispatch: &cenciwatch.DispatchState{Enabled: false},
	}
	m, _ := b.Update(agentSnapshotMsg{snapshot: snap})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.statusBar.dispatchStatus != "" {
		t.Errorf("dispatchStatus = %q, want empty after a snapshot reports the loop disabled", updated.statusBar.dispatchStatus)
	}
}

// TestUpdate_AgentSnapshotMsg_NilDispatch_ClearsDispatchStatusSegment covers
// the pre-#219 daemon guard: a snapshot with no dispatch data at all also
// hides the segment.
func TestUpdate_AgentSnapshotMsg_NilDispatch_ClearsDispatchStatusSegment(t *testing.T) {
	b := newCenciWatchTestBoard(t, &cenciwatch.FakeWatcher{})
	b.statusBar.SetDispatchStatus("⟳ dispatch")

	snap := &cenciwatch.StateSnapshot{}
	m, _ := b.Update(agentSnapshotMsg{snapshot: snap})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.statusBar.dispatchStatus != "" {
		t.Errorf("dispatchStatus = %q, want empty after a snapshot with no dispatch data (pre-#219 daemon)", updated.statusBar.dispatchStatus)
	}
}

// TestUpdate_CenciWatchErrorMsg_SingleError_DoesNotClearDispatchStatusSegment
// verifies the grace-period rule added by #333: a single, isolated watcher
// error is tolerated (the reconnect backoff ladder self-heals within ~1s),
// so the dispatch segment must remain exactly as it was, not clear.
func TestUpdate_CenciWatchErrorMsg_SingleError_DoesNotClearDispatchStatusSegment(t *testing.T) {
	b := newCenciWatchTestBoard(t, &cenciwatch.FakeWatcher{})
	b.statusBar.SetDispatchStatus("⟳ dispatch")

	m, _ := b.Update(cenciWatchErrorMsg{err: errors.New("connection refused")})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.statusBar.dispatchStatus != "⟳ dispatch" {
		t.Errorf("dispatchStatus = %q after a single cenciWatchErrorMsg, want unchanged %q (a lone transient blip must not clear the segment, #333)", updated.statusBar.dispatchStatus, "⟳ dispatch")
	}
}

// TestUpdate_CenciWatchErrorMsg_SecondConsecutiveError_ClearsDispatchStatusSegment
// verifies the watcher-down visibility rule: only once a SECOND consecutive
// error arrives, with no intervening successful agentSnapshotMsg, does the
// dispatch segment clear live (#333's two-strike grace period).
func TestUpdate_CenciWatchErrorMsg_SecondConsecutiveError_ClearsDispatchStatusSegment(t *testing.T) {
	b := newCenciWatchTestBoard(t, &cenciwatch.FakeWatcher{})
	b.statusBar.SetDispatchStatus("⟳ dispatch")

	m, _ := b.Update(cenciWatchErrorMsg{err: errors.New("connection refused")})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if updated.statusBar.dispatchStatus == "" {
		t.Fatal("dispatchStatus cleared after the FIRST error (test setup expects the grace period to hold here)")
	}

	m, _ = updated.Update(cenciWatchErrorMsg{err: errors.New("connection refused")})
	updated, ok = m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.statusBar.dispatchStatus != "" {
		t.Errorf("dispatchStatus = %q after a second consecutive cenciWatchErrorMsg, want empty (watcher-down hides the segment live)", updated.statusBar.dispatchStatus)
	}
}

// TestUpdate_AgentSnapshotMsg_ResetsConsecutiveErrorCounter verifies a
// successful agentSnapshotMsg between two errors resets the consecutive-error
// counter, mirroring the existing agentBackoff reset: error, snapshot, error
// must still be treated as a single (first) error, not a second strike.
func TestUpdate_AgentSnapshotMsg_ResetsConsecutiveErrorCounter(t *testing.T) {
	b := newCenciWatchTestBoard(t, &cenciwatch.FakeWatcher{})

	m, _ := b.Update(cenciWatchErrorMsg{err: errors.New("connection refused")})
	b = m.(Board)

	snap := &cenciwatch.StateSnapshot{
		Dispatch: &cenciwatch.DispatchState{Enabled: true, DaemonRunning: true},
	}
	m, _ = b.Update(agentSnapshotMsg{snapshot: snap})
	b = m.(Board)
	if b.statusBar.dispatchStatus == "" {
		t.Fatal("test setup: a snapshot with Dispatch.Enabled=true should set a non-empty dispatch segment")
	}
	segmentAfterSnapshot := b.statusBar.dispatchStatus

	m, _ = b.Update(cenciWatchErrorMsg{err: errors.New("connection refused")})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.statusBar.dispatchStatus != segmentAfterSnapshot {
		t.Errorf("dispatchStatus = %q after error, snapshot, error, want unchanged %q (snapshot must reset the consecutive-error counter, #333)", updated.statusBar.dispatchStatus, segmentAfterSnapshot)
	}
}

// TestBoard_DispatchStatusSegment_AppearsThenClearsLiveViaWatcher is the
// board-level integration test for the ticket's core acceptance criterion:
// using a real FakeWatcher subscription (driven the way Init() drives it,
// via subscribeCenciWatchCmd + collectCmdMsgs per the repo's established
// goroutine+timeout tea.Cmd testing pattern), a snapshot with an enabled
// dispatch loop makes the segment appear in the rendered View(); a single
// subsequent watcher-down error is tolerated (grace period, #333) and the
// segment stays visible; only a SECOND consecutive error clears it live,
// with no restart.
func TestBoard_DispatchStatusSegment_AppearsThenClearsLiveViaWatcher(t *testing.T) {
	snap := &cenciwatch.StateSnapshot{
		Dispatch: &cenciwatch.DispatchState{Enabled: true, DaemonRunning: true},
	}
	fw := &cenciwatch.FakeWatcher{
		Results: []cenciwatch.FakeWatcherResult{{Snap: snap}},
	}
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, fw, nil)
	b.Width = 120
	b.Height = 40

	boardMsg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{Title: "Column A", Cards: []provider.Card{{Number: 1, Title: "A card"}}}},
	}}
	m, _ := b.Update(boardMsg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	b = board

	// Drive the watcher subscription live, the way Init()/the self-chaining
	// Cmd would deliver it.
	msgs := collectCmdMsgs(subscribeCenciWatchCmd(fw))
	found := false
	for _, msg := range msgs {
		if snapMsg, ok := msg.(agentSnapshotMsg); ok {
			found = true
			m, _ := b.Update(snapMsg)
			board, ok := m.(Board)
			if !ok {
				t.Fatalf("Update returned %T, want Board", m)
			}
			b = board
		}
	}
	if !found {
		t.Fatal("subscribeCenciWatchCmd(fw) did not deliver an agentSnapshotMsg (test setup)")
	}

	view := b.View()
	if !strings.Contains(view, "dispatch") {
		t.Fatalf("View() after a live snapshot with Dispatch.Enabled=true = %q, want the dispatch segment visible", view)
	}

	// The daemon becomes unreachable: a single transient blip is tolerated per
	// the #333 grace period, so the segment must remain visible.
	m2, _ := b.Update(cenciWatchErrorMsg{err: errors.New("connection refused")})
	afterFirstError, ok := m2.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m2)
	}
	b = afterFirstError

	viewAfterFirstError := b.View()
	if !strings.Contains(viewAfterFirstError, "dispatch") {
		t.Fatalf("View() after a single cenciWatchErrorMsg = %q, want the dispatch segment still visible (grace period, #333)", viewAfterFirstError)
	}

	// A second consecutive error, with no intervening snapshot, clears it live.
	m3, _ := b.Update(cenciWatchErrorMsg{err: errors.New("connection refused")})
	afterSecondError, ok := m3.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m3)
	}
	b = afterSecondError

	view2 := b.View()
	if strings.Contains(view2, "dispatch") {
		t.Errorf("View() after a second consecutive cenciWatchErrorMsg = %q, want the dispatch segment cleared live (watcher-down hides it)", view2)
	}
}
