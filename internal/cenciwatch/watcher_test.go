package cenciwatch

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestSnapshot builds a minimal StateSnapshot with a single window, used
// to assert that decoding round-trips the fields our join logic depends on.
func newTestSnapshot(windowName, status string) StateSnapshot {
	return StateSnapshot{
		Timestamp: "2026-07-11T00:00:00Z",
		Windows: []WindowState{
			{
				Session:     "main",
				WindowIndex: "0",
				WindowName:  windowName,
				TaskName:    "Fix bug",
				Status:      status,
				Agent:       "claude",
			},
		},
	}
}

// acceptAndWriteOnce accepts a single connection on ln, writes the marshalled
// snapshot as one NDJSON line, and delivers the accepted conn on the returned
// channel so the test can later close it to simulate a daemon disconnect.
func acceptAndWriteOnce(t *testing.T, ln net.Listener, snap StateSnapshot) <-chan net.Conn {
	t.Helper()
	line, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("failed to marshal snapshot: %v", err)
	}
	connCh := make(chan net.Conn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = conn.Write(append(line, '\n'))
		connCh <- conn
	}()
	return connCh
}

// --- defaultSocketPath ---

// A valid, non-loosely-permissioned XDG_RUNTIME_DIR is used as the base for
// the nested cenci/cenci.sock path the daemon listens on.
func TestDefaultSocketPath_UsesNestedPathUnderValidXDGRuntimeDir(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatalf("failed to chmod temp dir: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", dir)

	got := defaultSocketPath()
	want := filepath.Join(dir, "cenci", "cenci.sock")
	if got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// A loosely-permissioned XDG_RUNTIME_DIR (group/other writable) is rejected,
// falling back to the private per-uid tmp directory, mirroring the daemon's
// own secureSocketDir() fallback rule.
func TestDefaultSocketPath_FallsBackWhenXDGRuntimeDirIsLooselyPermissioned(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0777); err != nil {
		t.Fatalf("failed to chmod temp dir: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", dir)

	got := defaultSocketPath()
	want := filepath.Join(os.TempDir(), fmt.Sprintf("cenci-%d", os.Getuid()), "cenci", "cenci.sock")
	if got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// An unset XDG_RUNTIME_DIR falls back to the private per-uid tmp directory.
func TestDefaultSocketPath_FallsBackWhenXDGRuntimeDirUnset(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")

	got := defaultSocketPath()
	want := filepath.Join(os.TempDir(), fmt.Sprintf("cenci-%d", os.Getuid()), "cenci", "cenci.sock")
	if got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// --- FakeWatcher ---

func TestFakeWatcher_ReadNext_ReturnsScriptedResultsInOrder(t *testing.T) {
	first := &StateSnapshot{Timestamp: "t1"}
	second := &StateSnapshot{Timestamp: "t2"}
	boom := errors.New("boom")

	fw := &FakeWatcher{
		Results: []FakeWatcherResult{
			{Snap: first},
			{Err: boom},
			{Snap: second},
		},
	}

	gotSnap, gotErr := fw.ReadNext()
	if gotErr != nil || gotSnap != first {
		t.Fatalf("ReadNext() #1 = (%v, %v), want (%v, nil)", gotSnap, gotErr, first)
	}

	gotSnap, gotErr = fw.ReadNext()
	if gotErr != boom || gotSnap != nil {
		t.Fatalf("ReadNext() #2 = (%v, %v), want (nil, %v)", gotSnap, gotErr, boom)
	}

	gotSnap, gotErr = fw.ReadNext()
	if gotErr != nil || gotSnap != second {
		t.Fatalf("ReadNext() #3 = (%v, %v), want (%v, nil)", gotSnap, gotErr, second)
	}
}

func TestFakeWatcher_Close_SetsClosedAndReturnsNil(t *testing.T) {
	fw := &FakeWatcher{}

	if fw.closed {
		t.Fatal("closed should be false before Close()")
	}

	err := fw.Close()

	if err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}
	if !fw.closed {
		t.Error("closed should be true after Close()")
	}
}

// --- socketWatcher (integration, real unix socket) ---

func TestSocketWatcher_ReadNext_DecodesSnapshot(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "cenci.sock")

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to listen on unix socket: %v", err)
	}
	defer func() { _ = ln.Close() }()

	snap := newTestSnapshot("42-fix-the-bug", "running")
	connCh := acceptAndWriteOnce(t, ln, snap)

	w := newSocketWatcher(socketPath)
	defer func() { _ = w.Close() }()

	got, err := w.ReadNext()
	if err != nil {
		t.Fatalf("ReadNext() error = %v, want nil", err)
	}
	if got == nil || len(got.Windows) != 1 {
		t.Fatalf("ReadNext() = %+v, want one window", got)
	}
	if got.Windows[0].WindowName != snap.Windows[0].WindowName {
		t.Errorf("WindowName = %q, want %q", got.Windows[0].WindowName, snap.Windows[0].WindowName)
	}
	if got.Windows[0].Status != snap.Windows[0].Status {
		t.Errorf("Status = %q, want %q", got.Windows[0].Status, snap.Windows[0].Status)
	}

	select {
	case conn := <-connCh:
		_ = conn.Close()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for accepted connection")
	}
}

// TestSocketWatcher_ReadNext_ReconnectsAfterServerClose verifies the
// reconnect-after-close contract: once the daemon drops the connection, the
// next ReadNext() surfaces an error, but the watcher transparently re-dials
// on a subsequent call once a fresh listener is accepting at the same path.
func TestSocketWatcher_ReadNext_ReconnectsAfterServerClose(t *testing.T) {
	dir := t.TempDir()
	socketPath := filepath.Join(dir, "cenci.sock")

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to listen on unix socket: %v", err)
	}

	firstSnap := newTestSnapshot("10-first-task", "running")
	connCh := acceptAndWriteOnce(t, ln, firstSnap)

	w := newSocketWatcher(socketPath)
	defer func() { _ = w.Close() }()

	if _, err := w.ReadNext(); err != nil {
		t.Fatalf("first ReadNext() error = %v, want nil", err)
	}

	// Simulate the daemon dropping the connection, then tearing down its listener.
	select {
	case conn := <-connCh:
		_ = conn.Close()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for accepted connection")
	}
	_ = ln.Close()

	if _, err := w.ReadNext(); err == nil {
		t.Fatal("ReadNext() after server close, want error, got nil")
	}

	// A fresh listener at the same path should allow the watcher to
	// transparently reconnect on the next ReadNext() call.
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("failed to remove stale socket file: %v", err)
	}
	ln2, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("failed to re-listen on unix socket: %v", err)
	}
	defer func() { _ = ln2.Close() }()

	secondSnap := newTestSnapshot("20-second-task", "done")
	connCh2 := acceptAndWriteOnce(t, ln2, secondSnap)

	got, err := w.ReadNext()
	if err != nil {
		t.Fatalf("ReadNext() after reconnect error = %v, want nil", err)
	}
	if got == nil || len(got.Windows) != 1 || got.Windows[0].WindowName != secondSnap.Windows[0].WindowName {
		t.Errorf("ReadNext() after reconnect = %+v, want window name %q", got, secondSnap.Windows[0].WindowName)
	}

	select {
	case conn := <-connCh2:
		_ = conn.Close()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for second accepted connection")
	}
}
