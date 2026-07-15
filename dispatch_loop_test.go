package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
)

// Ticket #313: the dispatch panel becomes a pure reader of the daemon-owned
// "loop" object returned by `cenci dispatch status --json`. All
// in-app pidfile-based process management (start/stop of the dispatch loop)
// is deleted outright -- starting/stopping the loop becomes a
// user-configured custom shell action (README-documented only, no code).
//
// These tests drive the real path: script a FakeExecutor's RunShellOutput
// (version probe, then dispatch status --json), press 'd' to open the
// dispatch panel, execute the resulting Cmd(s) via collectMsgs, feed the
// resulting dispatchStatusMsg back into Update, then assert on View()
// content.

// driveDispatchStatus opens the dispatch panel (press 'd'), executes every
// Cmd fired as a result, and feeds every resulting message back through
// Update -- so the final Board reflects a completed dispatch status query
// against the scripted FakeExecutor.
func driveDispatchStatus(t *testing.T, fe *action.FakeExecutor) Board {
	t.Helper()
	b := newDispatchTestBoardWithExecutor(t, fe)

	m, cmd := b.Update(keyMsg("d"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("expected pressing 'd' to fire a Cmd, got nil")
	}

	msgs := collectMsgs(cmd)
	for _, msg := range msgs {
		m2, _ := b2.Update(msg)
		updated, ok := m2.(Board)
		if !ok {
			t.Fatalf("Update returned %T, want Board", m2)
		}
		b2 = updated
	}
	return b2
}

// scriptedStatusExecutor builds a FakeExecutor scripted with a successful
// version probe followed by the given dispatch status --json stdout.
func scriptedStatusExecutor(stdout string) *action.FakeExecutor {
	return &action.FakeExecutor{
		RunShellOutputResults: []action.RunShellOutputResult{
			{}, // cenci version probe succeeds
			{Stdout: stdout},
		},
	}
}

// --- Loop line rendering: on + daemon running + has run at least once ---

func TestDispatchView_Loop_OnAndRunning_ShowsIntervalTimeAndCounts(t *testing.T) {
	// Real captured wire sample from a live cenci v2.17.0 binary.
	sample := `{"repo":"matteobortolazzo/lazyboards","dir":"/workspace","enrolled":false,"loop":{"enabled":true,"daemon_running":true,"interval":"5m","pass_running":false,"last_run_at":"2026-07-14T10:09:50Z","last_dispatched":0,"last_skipped":8}}`
	fe := scriptedStatusExecutor(sample)

	b := driveDispatchStatus(t, fe)
	view := b.View()

	parsed, err := time.Parse(time.RFC3339, "2026-07-14T10:09:50Z")
	if err != nil {
		t.Fatalf("time.Parse failed on the sample's last_run_at: %v", err)
	}
	wantTime := parsed.Local().Format("15:04")
	want := fmt.Sprintf("Loop: on (5m) — last run %s, 0 dispatched / 8 skipped", wantTime)

	if !strings.Contains(view, want) {
		t.Errorf("dispatch view for on+running loop should contain %q, got:\n%s", want, view)
	}
}

// --- Loop line rendering: on but daemon process not running ---

func TestDispatchView_Loop_DaemonNotRunning(t *testing.T) {
	sample := `{"repo":"owner/repo","dir":"/tmp/x","enrolled":true,"loop":{"enabled":true,"daemon_running":false,"interval":"5m"}}`
	fe := scriptedStatusExecutor(sample)

	b := driveDispatchStatus(t, fe)
	view := b.View()

	want := "Loop: on (5m) — daemon not running"
	if !strings.Contains(view, want) {
		t.Errorf("dispatch view for on+daemon-down loop should contain %q, got:\n%s", want, view)
	}
}

// --- Loop line rendering: on + daemon running, but never run yet ---

func TestDispatchView_Loop_NeverRun(t *testing.T) {
	sample := `{"repo":"owner/repo","dir":"/tmp/x","enrolled":true,"loop":{"enabled":true,"daemon_running":true,"interval":"5m"}}`
	fe := scriptedStatusExecutor(sample)

	b := driveDispatchStatus(t, fe)
	view := b.View()

	want := "Loop: on (5m) — no runs yet"
	if !strings.Contains(view, want) {
		t.Errorf("dispatch view for on+never-run loop should contain %q, got:\n%s", want, view)
	}
}

// --- Loop line rendering: off ---

func TestDispatchView_Loop_Off(t *testing.T) {
	sample := `{"repo":"owner/repo","dir":"/tmp/x","enrolled":true,"loop":{"enabled":false}}`
	fe := scriptedStatusExecutor(sample)

	b := driveDispatchStatus(t, fe)
	view := b.View()

	if !strings.Contains(view, "Loop: off") {
		t.Errorf("dispatch view for a disabled loop should contain %q, got:\n%s", "Loop: off", view)
	}
	if strings.Contains(view, "Loop: off (") {
		t.Errorf("dispatch view for a disabled loop should not show parens/interval, got:\n%s", view)
	}
}

// --- Loop line rendering: last_error takes precedence over on/off ---

func TestDispatchView_Loop_ErrorTakesPrecedenceOverOnRendering(t *testing.T) {
	// enabled:true and daemon_running:true would otherwise render the
	// "no runs yet" on-state -- last_error must win regardless.
	sample := `{"repo":"owner/repo","dir":"/tmp/x","enrolled":true,"loop":{"enabled":true,"daemon_running":true,"interval":"5m","last_error":"boom"}}`
	fe := scriptedStatusExecutor(sample)

	b := driveDispatchStatus(t, fe)
	view := b.View()

	want := "Loop: error — boom"
	if !strings.Contains(view, want) {
		t.Errorf("dispatch view with last_error set should contain %q, got:\n%s", want, view)
	}
	if strings.Contains(view, "no runs yet") {
		t.Errorf("dispatch view with last_error set should not also render the on-state 'no runs yet' line, got:\n%s", view)
	}
}

// --- Old-binary guard: top-level "loop" key entirely absent ---

// TestDispatchView_Loop_OldBinaryGuard_MissingLoopKey covers a current
// cenci binary new enough to run `dispatch status --json` successfully,
// but whose response omits the "loop" key entirely (a binary that predates
// this feature). This must render a clear error line inside the loop
// section -- not crash the panel -- while repo/enrolled info (decoded from
// the same successful top-level parse) still renders normally. This is a
// distinct guard from the existing cenciTooOldMsg version-probe guard,
// which fires when the CLI verb itself doesn't exist.
func TestDispatchView_Loop_OldBinaryGuard_MissingLoopKey(t *testing.T) {
	sample := `{"repo":"owner/repo","dir":"/tmp/x","enrolled":true}`
	fe := scriptedStatusExecutor(sample)

	b := driveDispatchStatus(t, fe)
	view := b.View()

	if !strings.Contains(view, "owner/repo") {
		t.Errorf("dispatch view should still render repo info when the loop key is missing, got:\n%s", view)
	}

	lower := strings.ToLower(view)
	if !strings.Contains(lower, "loop") {
		t.Errorf("dispatch view should still render a loop section when the loop key is missing, got:\n%s", view)
	}
	if !strings.Contains(lower, "upgrade") && !strings.Contains(lower, "unavailable") && !strings.Contains(lower, "unsupported") {
		t.Errorf("dispatch view with a missing loop key should surface a clear error (mentioning upgrade/unavailable/unsupported), got:\n%s", view)
	}
}

// --- formatLoopRunTime: RFC3339 -> local HH:MM, malformed -> raw passthrough ---

func TestFormatLoopRunTime_ValidRFC3339_FormatsLocalHHMM(t *testing.T) {
	raw := "2026-07-14T10:09:50Z"

	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("time.Parse failed on the fixture value: %v", err)
	}
	want := parsed.Local().Format("15:04")

	got := formatLoopRunTime(raw)
	if got != want {
		t.Errorf("formatLoopRunTime(%q) = %q, want local HH:MM %q", raw, got, want)
	}
}

// TestFormatLoopRunTime_Malformed_ReturnsRawInputUnchanged is a regression
// test for cenci's own frontend script bug: a malformed/unparseable
// last_run_at value must come back verbatim, never "NaN:NaN" and never a
// panic.
func TestFormatLoopRunTime_Malformed_ReturnsRawInputUnchanged(t *testing.T) {
	raw := "not-a-valid-timestamp"

	got := formatLoopRunTime(raw)
	if got != raw {
		t.Errorf("formatLoopRunTime(%q) = %q, want the raw input returned unchanged", raw, got)
	}
	if strings.Contains(strings.ToLower(got), "nan") {
		t.Errorf("formatLoopRunTime(%q) = %q, must never produce NaN-style output", raw, got)
	}
}

// --- Live-preferred Loop line (ticket #403) ---
//
// The Loop line prefers the live dispatch state pushed over the daemon
// socket and falls back to the CLI-queried state when the watcher is
// considered disconnected. These tests drive the real path: open the modal
// against a scripted CLI status, then feed socket snapshots (decoded from a
// real daemon wire line, per the testing.md wire-boundary rule) and watcher
// errors through Update, asserting on View().

// liveLoopWireLine is a real captured daemon-format NDJSON line (the
// snapshot_test.go sample, which pins the producer shape of cenci's
// watch/pkg/watch/snapshot.go) carrying a live "dispatch" object.
const liveLoopWireLine = `{"timestamp":"2026-07-14T10:09:50Z","windows":[],"summary":{"total":0},"dispatch":{"enabled":true,"daemon_running":true,"interval":"5m","pass_running":false,"last_run_at":"2026-07-14T10:09:50Z","last_dispatched":0,"last_skipped":8}}`

// noDispatchWireLine is the same daemon wire format with no "dispatch" key
// at all (a pre-#219 daemon).
const noDispatchWireLine = `{"timestamp":"2026-07-14T10:09:50Z","windows":[],"summary":{"total":0}}`

// cliLoopOffSample is a `cenci dispatch status --json` response whose loop
// is off — deliberately distinct from liveLoopWireLine's enabled loop so a
// source-preference assertion cannot pass coincidentally.
const cliLoopOffSample = `{"repo":"owner/repo","dir":"/tmp/x","enrolled":true,"loop":{"enabled":false}}`

// liveLoopWantLine returns the Loop line renderLoopLine produces for
// liveLoopWireLine's dispatch object.
func liveLoopWantLine(t *testing.T) string {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, "2026-07-14T10:09:50Z")
	if err != nil {
		t.Fatalf("time.Parse failed on the wire sample's last_run_at: %v", err)
	}
	return fmt.Sprintf("Loop: on (5m) — last run %s, 0 dispatched / 8 skipped", parsed.Local().Format("15:04"))
}

// feedWireSnapshot decodes a real daemon wire line into a StateSnapshot and
// feeds it through Update. The test boards here have no cenciWatcher, so the
// handler must not fire a re-subscribe Cmd.
func feedWireSnapshot(t *testing.T, b Board, wireLine string) Board {
	t.Helper()
	var snap cenciwatch.StateSnapshot
	if err := json.Unmarshal([]byte(wireLine), &snap); err != nil {
		t.Fatalf("failed to decode daemon wire line: %v", err)
	}
	m, cmd := b.Update(agentSnapshotMsg{snapshot: &snap})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Fatal("expected nil Cmd from agentSnapshotMsg on a board with no cenciWatcher, got non-nil")
	}
	return updated
}

// feedWatchError feeds one cenciWatchErrorMsg through Update, asserting the
// handler schedules its reconnect retry Cmd.
func feedWatchError(t *testing.T, b Board) Board {
	t.Helper()
	m, cmd := b.Update(cenciWatchErrorMsg{err: errors.New("connection refused")})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("expected a retry Cmd from cenciWatchErrorMsg, got nil")
	}
	return updated
}

func TestDispatchView_Loop_LiveSnapshotPreferredOverCLI(t *testing.T) {
	b := driveDispatchStatus(t, scriptedStatusExecutor(cliLoopOffSample))
	b = feedWireSnapshot(t, b, liveLoopWireLine)

	view := b.View()

	want := liveLoopWantLine(t)
	if !strings.Contains(view, want) {
		t.Errorf("dispatch view with a live snapshot should render the live loop line %q, got:\n%s", want, view)
	}
	if strings.Contains(view, "Loop: off") {
		t.Errorf("dispatch view with a live snapshot should not render the CLI-sourced %q line, got:\n%s", "Loop: off", view)
	}
}

func TestDispatchView_Loop_DisconnectedWatcher_FallsBackToCLI(t *testing.T) {
	b := driveDispatchStatus(t, scriptedStatusExecutor(cliLoopOffSample))
	b = feedWireSnapshot(t, b, liveLoopWireLine)

	// Reach the same consecutive-error threshold that clears the status-bar
	// segment and marks the agents modal disconnected.
	for range cenciWatchClearThreshold {
		b = feedWatchError(t, b)
	}

	view := b.View()

	if !strings.Contains(view, "Loop: off") {
		t.Errorf("dispatch view after %d consecutive watcher errors should fall back to the CLI-sourced %q line, got:\n%s", cenciWatchClearThreshold, "Loop: off", view)
	}
	if strings.Contains(view, "Loop: on") {
		t.Errorf("dispatch view after the watcher disconnected should not render the (stale) live loop line, got:\n%s", view)
	}
}

func TestDispatchView_Loop_SnapshotWithoutDispatch_RendersCLILine(t *testing.T) {
	// A connected pre-#219 daemon (snapshot without dispatch data) must not
	// shadow a good CLI-sourced loop, and must not trip the upgrade guard.
	b := driveDispatchStatus(t, scriptedStatusExecutor(cliLoopOffSample))
	b = feedWireSnapshot(t, b, noDispatchWireLine)

	view := b.View()

	if !strings.Contains(view, "Loop: off") {
		t.Errorf("dispatch view with a dispatch-less snapshot should render the CLI-sourced %q line, got:\n%s", "Loop: off", view)
	}
	if strings.Contains(strings.ToLower(view), "upgrade") {
		t.Errorf("dispatch view with a dispatch-less snapshot but a good CLI loop should not render the upgrade guard, got:\n%s", view)
	}
}

func TestDispatchView_Loop_ReconnectAfterFallback_RendersLiveAgain(t *testing.T) {
	b := driveDispatchStatus(t, scriptedStatusExecutor(cliLoopOffSample))
	b = feedWireSnapshot(t, b, liveLoopWireLine)
	for range cenciWatchClearThreshold {
		b = feedWatchError(t, b)
	}

	// A successful snapshot resets the consecutive-error counter (#333), so
	// the Loop line silently flips back to the live source.
	b = feedWireSnapshot(t, b, liveLoopWireLine)

	view := b.View()

	want := liveLoopWantLine(t)
	if !strings.Contains(view, want) {
		t.Errorf("dispatch view after a reconnect snapshot should render the live loop line %q again, got:\n%s", want, view)
	}
	if strings.Contains(view, "Loop: off") {
		t.Errorf("dispatch view after a reconnect snapshot should not keep the CLI-sourced %q line, got:\n%s", "Loop: off", view)
	}
}
