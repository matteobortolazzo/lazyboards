package cenciwatch

import (
	"encoding/json"
	"testing"
)

// TestStateSnapshot_DecodesDispatchFromDaemonWireFormat decodes a real
// daemon-format NDJSON line carrying a "dispatch" object (per cenci's
// authoritative producer, watch/pkg/watch/snapshot.go) and asserts every
// field decodes via its exact wire tag. Per the #316/#317 lesson (cross-service
// tokens must be pinned to the producer's real wire sample, not a doc comment
// or a hand-built struct literal), this test starts from a raw NDJSON string
// rather than constructing a DispatchState value directly.
func TestStateSnapshot_DecodesDispatchFromDaemonWireFormat(t *testing.T) {
	const wireLine = `{"timestamp":"2026-07-14T10:09:50Z","windows":[],"summary":{"total":0},"dispatch":{"enabled":true,"daemon_running":true,"interval":"5m","pass_running":false,"last_run_at":"2026-07-14T10:09:50Z","last_dispatched":0,"last_skipped":8}}`

	var snap StateSnapshot
	if err := json.Unmarshal([]byte(wireLine), &snap); err != nil {
		t.Fatalf("failed to decode daemon wire line: %v", err)
	}

	if snap.Dispatch == nil {
		t.Fatalf("Dispatch = nil, want a decoded DispatchState from the wire's \"dispatch\" object")
	}
	if !snap.Dispatch.Enabled {
		t.Errorf("Dispatch.Enabled = false, want true")
	}
	if !snap.Dispatch.DaemonRunning {
		t.Errorf("Dispatch.DaemonRunning = false, want true")
	}
	if snap.Dispatch.Interval != "5m" {
		t.Errorf("Dispatch.Interval = %q, want %q", snap.Dispatch.Interval, "5m")
	}
	if snap.Dispatch.PassRunning {
		t.Errorf("Dispatch.PassRunning = true, want false")
	}
	if snap.Dispatch.LastRunAt != "2026-07-14T10:09:50Z" {
		t.Errorf("Dispatch.LastRunAt = %q, want %q", snap.Dispatch.LastRunAt, "2026-07-14T10:09:50Z")
	}
	if snap.Dispatch.LastDispatched != 0 {
		t.Errorf("Dispatch.LastDispatched = %d, want 0", snap.Dispatch.LastDispatched)
	}
	if snap.Dispatch.LastSkipped != 8 {
		t.Errorf("Dispatch.LastSkipped = %d, want 8", snap.Dispatch.LastSkipped)
	}
	if snap.Dispatch.LastError != "" {
		t.Errorf("Dispatch.LastError = %q, want empty (omitted from the wire sample)", snap.Dispatch.LastError)
	}
}

// TestStateSnapshot_DecodesDispatchLastError pins the last_error field's wire
// tag specifically, since it drives the status-bar segment's distinct (red)
// styling.
func TestStateSnapshot_DecodesDispatchLastError(t *testing.T) {
	const wireLine = `{"timestamp":"2026-07-14T10:09:50Z","windows":[],"summary":{"total":0},"dispatch":{"enabled":true,"daemon_running":true,"pass_running":false,"last_dispatched":3,"last_skipped":1,"last_error":"exit status 1: git push rejected"}}`

	var snap StateSnapshot
	if err := json.Unmarshal([]byte(wireLine), &snap); err != nil {
		t.Fatalf("failed to decode daemon wire line: %v", err)
	}

	if snap.Dispatch == nil {
		t.Fatalf("Dispatch = nil, want a decoded DispatchState")
	}
	if snap.Dispatch.LastError != "exit status 1: git push rejected" {
		t.Errorf("Dispatch.LastError = %q, want %q", snap.Dispatch.LastError, "exit status 1: git push rejected")
	}
}

// TestStateSnapshot_NoDispatchKey_DispatchIsNil guards the pre-#219 daemon
// compatibility case: a snapshot with no "dispatch" key at all (an older
// daemon that predates the dispatch loop feature) must decode with
// Dispatch == nil, so the status-bar segment stays hidden rather than
// rendering a zero-valued (falsely "disabled") segment.
func TestStateSnapshot_NoDispatchKey_DispatchIsNil(t *testing.T) {
	const wireLine = `{"timestamp":"2026-07-14T10:09:50Z","windows":[],"summary":{"total":0}}`

	var snap StateSnapshot
	if err := json.Unmarshal([]byte(wireLine), &snap); err != nil {
		t.Fatalf("failed to decode daemon wire line: %v", err)
	}

	if snap.Dispatch != nil {
		t.Errorf("Dispatch = %+v, want nil when the wire line omits the \"dispatch\" key (pre-#219 daemon)", snap.Dispatch)
	}
}
