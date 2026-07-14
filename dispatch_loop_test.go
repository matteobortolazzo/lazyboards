package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/matteobortolazzo/lazyboards/internal/action"
)

// Ticket #313: the dispatch panel becomes a pure reader of the daemon-owned
// "loop" object returned by `agentwatch dispatch status --json`. All
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
			{}, // agentwatch version probe succeeds
			{Stdout: stdout},
		},
	}
}

// --- Loop line rendering: on + daemon running + has run at least once ---

func TestDispatchView_Loop_OnAndRunning_ShowsIntervalTimeAndCounts(t *testing.T) {
	// Real captured wire sample from a live agentwatch v2.17.0 binary.
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
// agentwatch binary new enough to run `dispatch status --json` successfully,
// but whose response omits the "loop" key entirely (a binary that predates
// this feature). This must render a clear error line inside the loop
// section -- not crash the panel -- while repo/enrolled info (decoded from
// the same successful top-level parse) still renders normally. This is a
// distinct guard from the existing agentwatchTooOldMsg version-probe guard,
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
// test for agent-stack's own frontend script bug: a malformed/unparseable
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
