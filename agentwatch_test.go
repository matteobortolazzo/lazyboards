package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	return NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, watcher, nil, "", "")
}

// newAgentWatchCardTestBoard creates a loaded Board with a single card in a
// single column, using sessionMaxLen for BuildSessionName-based join tests.
func newAgentWatchCardTestBoard(t *testing.T, cardNumber int, cardTitle string, sessionMaxLen int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", sessionMaxLen, 0, 0, "Working", false, false, nil, nil, "", "")

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

// --- agentStatusFor: ticket-number-prefix join (#257, <number>-<skill> names) ---

// A dispatched <number>-<skill> window joins to its card by ticket number.
func TestBoard_AgentStatusFor_SkillWindowMatchesByNumber(t *testing.T) {
	const cardNumber = 230
	b := newAgentWatchCardTestBoard(t, cardNumber, "Refine the thing", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{WindowName: "230-refine", Status: "running", Agent: "claude"},
		},
	}

	got := b.agentStatusFor(card)
	if got == nil {
		t.Fatalf("agentStatusFor() = nil, want a match for window 230-refine")
	}
	if got.Status != "running" {
		t.Errorf("agentStatusFor().Status = %q, want %q", got.Status, "running")
	}
	if got.Agent != "claude" {
		t.Errorf("agentStatusFor().Agent = %q, want %q", got.Agent, "claude")
	}
}

// A legacy <number>-<title-slug> window still joins by number prefix, so
// lazyboards can ship before agentwatch changes its window naming.
func TestBoard_AgentStatusFor_LegacyTitleSlugWindowStillMatches(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newAgentWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	legacyName := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{WindowName: legacyName, Status: "running", Agent: "claude"},
		},
	}

	if got := b.agentStatusFor(card); got == nil {
		t.Fatalf("agentStatusFor() = nil, want a match for legacy window %q", legacyName)
	}
}

// A bare <number> window (no slug, no skill) joins.
func TestBoard_AgentStatusFor_BareNumberWindowMatches(t *testing.T) {
	const cardNumber = 42
	b := newAgentWatchCardTestBoard(t, cardNumber, "Whatever", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{WindowName: "42", Status: "need-input"},
		},
	}

	if got := b.agentStatusFor(card); got == nil || got.Status != "need-input" {
		t.Errorf("agentStatusFor() = %+v, want the bare-number window (need_input)", got)
	}
}

// The trailing "-" boundary keeps card #23 from matching a 230-... window.
func TestBoard_AgentStatusFor_NumberPrefixBoundary(t *testing.T) {
	const cardNumber = 23
	b := newAgentWatchCardTestBoard(t, cardNumber, "Boundary", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{WindowName: "230-refine", Status: "running"},
		},
	}

	if got := b.agentStatusFor(card); got != nil {
		t.Errorf("agentStatusFor() = %+v, want nil (23 must not match 230-refine)", got)
	}
}

// When several windows share the ticket number, an active one (running /
// need_input) wins over any other status, regardless of snapshot order.
func TestBoard_AgentStatusFor_PrefersActiveWindow(t *testing.T) {
	const cardNumber = 55
	b := newAgentWatchCardTestBoard(t, cardNumber, "Multi", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{
			{WindowName: "55-implement", Status: "done"},
			{WindowName: "55-refine", Status: "running", Agent: "claude"},
		},
	}

	got := b.agentStatusFor(card)
	if got == nil {
		t.Fatalf("agentStatusFor() = nil, want the active window")
	}
	if got.Status != "running" || got.WindowName != "55-refine" {
		t.Errorf("agentStatusFor() = %+v, want the running 55-refine window", got)
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

// --- Board-scoped agent counts (#259) ---

// newAgentCountsBoard creates a loaded Board with the given cards in a single
// column, using DefaultSessionMaxLength for BuildSessionName-based joins.
func newAgentCountsBoard(t *testing.T, cards []provider.Card) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", config.DefaultSessionMaxLength, 0, 0, "Working", false, false, nil, nil, "", "")
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

// TestBoard_AgentCounts_BoardScoped verifies agentCounts tallies only running /
// need_input windows that join to a card on the board: idle statuses and
// windows with no matching card are excluded, and the tally is a count (not a
// boolean) so multiple running cards accumulate.
func TestBoard_AgentCounts_BoardScoped(t *testing.T) {
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
	b.agentSnapshot = &watch.StateSnapshot{Windows: []watch.WindowState{
		{WindowName: name(1, "First running"), Status: "running"},
		{WindowName: name(2, "Needs input"), Status: "need-input"},
		{WindowName: name(3, "Idle card"), Status: "idle"}, // excluded: idle
		{WindowName: name(4, "Second running"), Status: "running"},
		{WindowName: "999-no-such-card", Status: "running"},    // excluded: unmatched
		{WindowName: "888-no-such-card", Status: "need-input"}, // excluded: unmatched
	}}

	running, needInput := b.agentCounts()

	if running != 2 {
		t.Errorf("agentCounts() running = %d, want 2 (two matched running cards)", running)
	}
	if needInput != 1 {
		t.Errorf("agentCounts() needInput = %d, want 1 (one matched need_input card)", needInput)
	}
}

// TestBoard_AgentCounts_NilSnapshotIsZero verifies that with no snapshot stored
// (agentwatch off/absent) both counts are zero.
func TestBoard_AgentCounts_NilSnapshotIsZero(t *testing.T) {
	b := newAgentCountsBoard(t, []provider.Card{{Number: 1, Title: "A card"}})
	if b.agentSnapshot != nil {
		t.Fatal("test setup: agentSnapshot should be nil by default")
	}

	running, needInput := b.agentCounts()
	if running != 0 || needInput != 0 {
		t.Errorf("agentCounts() = (%d, %d), want (0, 0) when no snapshot is stored", running, needInput)
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
		{"need-input", "need-input", "claude", "‼"},
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
// the exact shape the agentwatch daemon broadcasts over its socket — including
// the "need-input" (hyphen) status the daemon's detect.StatusNeedInput.String()
// emits — and asserts a need-input agent badges its card and counts toward the
// status-bar summary. Constructing the snapshot from the raw NDJSON (rather than
// a hand-built WindowState) pins the status token to the daemon's real wire
// value: if lazyboards drifts back to the "need_input" underscore spelling, the
// badge silently disappears and this test fails.
func TestBoard_AgentBadge_NeedInputFromDaemonWireFormat(t *testing.T) {
	const cardNumber = 42
	b := newAgentWatchCardTestBoard(t, cardNumber, "Waiting for input", config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]

	// A single NDJSON line as emitted by the daemon's broadcast socket.
	const wireLine = `{"timestamp":"2026-07-13T22:09:50Z","windows":[{"session":"lazyboards","window_index":"2","window_name":"42-implement","task_name":"Waiting for input","status":"need-input","agent":"claude","manually_named":false}],"summary":{"total":1,"need_input":1}}`

	var snap watch.StateSnapshot
	if err := json.Unmarshal([]byte(wireLine), &snap); err != nil {
		t.Fatalf("failed to decode daemon wire line: %v", err)
	}
	b.agentSnapshot = &snap

	ws := b.agentStatusFor(card)
	if ws == nil {
		t.Fatalf("agentStatusFor() = nil, want the need-input window for card #%d", cardNumber)
	}
	if badge := b.agentBadgeFor(card); badge == "" {
		t.Errorf("agentBadgeFor() = %q, want a non-empty badge for a need-input agent", badge)
	}
	if _, needInput := b.agentCounts(); needInput != 1 {
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

// TestBoard_AgentBadgeFor_AppendedToDisplayText verifies a matching non-idle
// window causes cardDisplayText to append the badge.
func TestBoard_AgentBadgeFor_AppendedToDisplayText(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newAgentWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	card := b.Columns[0].Cards[0]
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{{WindowName: name, Status: "running", Agent: "claude"}},
	}

	badge := b.agentBadgeFor(card)
	if badge == "" {
		t.Fatal("agentBadgeFor() returned empty for a matching running window")
	}
	text, _ := cardDisplayText(card, []string{"Column A"}, b.workingLabel, badge)
	if !strings.Contains(text, badge) {
		t.Errorf("cardDisplayText did not append badge %q; got %q", badge, text)
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
		snap *watch.StateSnapshot
	}{
		{"idle status", &watch.StateSnapshot{Windows: []watch.WindowState{{WindowName: name, Status: "idle", Agent: "claude"}}}},
		{"non-matching window", &watch.StateSnapshot{Windows: []watch.WindowState{{WindowName: "999-other", Status: "running"}}}},
		{"nil snapshot", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newAgentWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
			b.agentSnapshot = tt.snap
			card := b.Columns[0].Cards[0]
			if badge := b.agentBadgeFor(card); badge != "" {
				t.Errorf("agentBadgeFor() = %q, want empty", badge)
			}
		})
	}
}

// TestViewCardList_RunningBadgeRendered verifies the running badge (kind +
// symbol) appears in the rendered card list.
func TestViewCardList_RunningBadgeRendered(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newAgentWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{{WindowName: name, Status: "running", Agent: "claude"}},
	}

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	if !strings.Contains(out, "▶") {
		t.Errorf("rendered card list missing running symbol; got:\n%s", out)
	}
	if !strings.Contains(out, "claude") {
		t.Errorf("rendered card list missing agent kind; got:\n%s", out)
	}
}

// TestViewCardList_NeedInputIsLoudest verifies need_input renders with the
// reverse/bold style — the loudest attention badge.
func TestViewCardList_NeedInputIsLoudest(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newAgentWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{{WindowName: name, Status: "need-input", Agent: "claude"}},
	}

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	if !strings.Contains(out, agentNeedInputStyle.Render("‼")) {
		t.Errorf("need_input badge not rendered with agentNeedInputStyle; got:\n%s", out)
	}
}

// TestViewCardList_IdleRendersNoSymbol verifies an idle (or non-matching) window
// leaves the card free of any status symbol.
func TestViewCardList_IdleRendersNoSymbol(t *testing.T) {
	const cardNumber = 7
	const cardTitle = "Fix flaky test"
	b := newAgentWatchCardTestBoard(t, cardNumber, cardTitle, config.DefaultSessionMaxLength)
	name := action.BuildSessionName(cardNumber, cardTitle, config.DefaultSessionMaxLength)
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{{WindowName: name, Status: "idle", Agent: "claude"}},
	}

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	for _, sym := range []string{"▶", "✓", "■", "‼", "✗"} {
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
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", config.DefaultSessionMaxLength, 0, 0, "Working", false, false, nil, nil, "", "")
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
	b.agentSnapshot = &watch.StateSnapshot{
		Windows: []watch.WindowState{{WindowName: name, Status: "running", Agent: "claude"}},
	}

	out := b.viewCardList(b.Columns[0], 20, 60, leftPanelStyle)
	if !strings.Contains(out, "") {
		t.Errorf("working spinner icon missing; got:\n%s", out)
	}
	if !strings.Contains(out, "▶") {
		t.Errorf("running badge symbol missing; got:\n%s", out)
	}
}
