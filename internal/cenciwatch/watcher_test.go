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

// shortTempDir creates a temp directory with a deliberately short name: the
// resolved socket paths built on top of it ("<dir>/.local/state/cenci/run/
// cenci.sock") must stay inside the ~108-byte sun_path bound, which a
// t.TempDir() path named after a long test function would blow past.
func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cw")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// isolateSocketEnv neutralizes every environment input the tier chain reads,
// so a test never sees the developer machine's real cenci socket directories.
// TMPDIR is redirected too, since the tmp tier is otherwise a machine-global
// path that a locally running daemon may already occupy.
func isolateSocketEnv(t *testing.T) {
	t.Helper()
	t.Setenv("CENCI_SOCKET_DIR", "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", shortTempDir(t))
	t.Setenv("TMPDIR", shortTempDir(t))
}

// listenAt creates dir at 0700 and, when withSocket is true, binds a real
// listening unix socket at <dir>/cenci.sock so the resolver's liveness probe
// has a genuine socket to find rather than a plain file.
func listenAt(t *testing.T, dir string, withSocket bool) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("failed to create socket dir %q: %v", dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		t.Fatalf("failed to chmod socket dir %q: %v", dir, err)
	}
	path := filepath.Join(dir, socketBasename)
	if withSocket {
		ln, err := net.Listen("unix", path)
		if err != nil {
			t.Fatalf("failed to listen at %q: %v", path, err)
		}
		t.Cleanup(func() { _ = ln.Close() })
	}
	return path
}

// stateTierDir is the tier-2 directory the daemon resolves from a given HOME
// when XDG_STATE_HOME is unset, spelled out here from the daemon's documented
// layout rather than read back from the implementation under test.
func stateTierDir(home string) string {
	return filepath.Join(home, ".local", "state", "cenci", "run")
}

// $CENCI_SOCKET_DIR is the top of the daemon's chain and is used verbatim,
// with no appended path segment.
func TestDefaultSocketPath_OverrideTierWinsVerbatim(t *testing.T) {
	isolateSocketEnv(t)
	override := shortTempDir(t)
	want := listenAt(t, override, true)
	t.Setenv("CENCI_SOCKET_DIR", override)

	if got := defaultSocketPath(); got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// A relative $CENCI_SOCKET_DIR is not a usable socket directory; resolution
// skips it rather than dialing a path relative to the board's working
// directory.
func TestDefaultSocketPath_RelativeOverrideIsSkipped(t *testing.T) {
	isolateSocketEnv(t)
	home := os.Getenv("HOME")
	want := listenAt(t, stateTierDir(home), true)
	t.Setenv("CENCI_SOCKET_DIR", "relative/cenci")

	if got := defaultSocketPath(); got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// With no override, the state tier ($XDG_STATE_HOME/cenci/run) wins — this is
// where a default `cenci daemon start` actually listens.
func TestDefaultSocketPath_StateTierUsesXDGStateHome(t *testing.T) {
	isolateSocketEnv(t)
	base := shortTempDir(t)
	want := listenAt(t, filepath.Join(base, "cenci", "run"), true)
	t.Setenv("XDG_STATE_HOME", base)

	if got := defaultSocketPath(); got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// An unset $XDG_STATE_HOME resolves the state tier under $HOME/.local/state.
func TestDefaultSocketPath_StateTierDefaultsToHomeLocalState(t *testing.T) {
	isolateSocketEnv(t)
	home := os.Getenv("HOME")
	want := listenAt(t, stateTierDir(home), true)

	if got := defaultSocketPath(); got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// When no higher tier holds a socket, the per-uid tmp tier is used — the
// daemon's final always-available fallback.
func TestDefaultSocketPath_FallsBackToTmpTierHoldingTheSocket(t *testing.T) {
	isolateSocketEnv(t)
	tmpBase := filepath.Join(os.TempDir(), fmt.Sprintf("cenci-%d", os.Getuid()), "cenci")
	want := listenAt(t, tmpBase, true)

	if got := defaultSocketPath(); got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// A tier whose directory is group/other-writable is skipped even when a
// socket sits in it: an attacker able to write the directory could plant the
// socket the board would then trust.
func TestDefaultSocketPath_SkipsLooselyPermissionedTierHoldingASocket(t *testing.T) {
	isolateSocketEnv(t)
	override := shortTempDir(t)
	listenAt(t, override, true)
	if err := os.Chmod(override, 0777); err != nil {
		t.Fatalf("failed to loosen permissions: %v", err)
	}
	t.Setenv("CENCI_SOCKET_DIR", override)

	home := os.Getenv("HOME")
	want := listenAt(t, stateTierDir(home), true)

	if got := defaultSocketPath(); got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// A tier whose directory is a symlink is skipped rather than followed.
func TestDefaultSocketPath_SkipsSymlinkedTier(t *testing.T) {
	isolateSocketEnv(t)
	real := shortTempDir(t)
	listenAt(t, real, true)
	link := filepath.Join(shortTempDir(t), "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("failed to symlink: %v", err)
	}
	t.Setenv("CENCI_SOCKET_DIR", link)

	home := os.Getenv("HOME")
	want := listenAt(t, stateTierDir(home), true)

	if got := defaultSocketPath(); got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// A path that exists but is a regular file, not a socket, does not count as a
// live tier: resolution moves on to the tier that actually holds a socket.
func TestDefaultSocketPath_IgnoresNonSocketFile(t *testing.T) {
	isolateSocketEnv(t)
	override := shortTempDir(t)
	decoy := listenAt(t, override, false)
	if err := os.WriteFile(decoy, []byte("not a socket"), 0600); err != nil {
		t.Fatalf("failed to write decoy: %v", err)
	}
	t.Setenv("CENCI_SOCKET_DIR", override)

	home := os.Getenv("HOME")
	want := listenAt(t, stateTierDir(home), true)

	if got := defaultSocketPath(); got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// With no socket anywhere (daemon not started yet), resolution still returns
// the highest usable tier's path, so the watcher's next re-dial targets the
// location the daemon will occupy once it starts.
func TestDefaultSocketPath_NoLiveSocketReturnsHighestUsableTier(t *testing.T) {
	isolateSocketEnv(t)
	override := shortTempDir(t)
	want := listenAt(t, override, false)
	t.Setenv("CENCI_SOCKET_DIR", override)

	if got := defaultSocketPath(); got != want {
		t.Errorf("defaultSocketPath() = %q, want %q", got, want)
	}
}

// The socket path is re-resolved on every dial, not frozen when the board is
// constructed: a daemon started after launch — or restarted into a different
// tier — is picked up without restarting lazyboards.
func TestSocketWatcher_ResolvesPathOnEveryDial(t *testing.T) {
	isolateSocketEnv(t)
	w := NewSocketWatcher().(*socketWatcher)
	t.Cleanup(func() { _ = w.Close() })

	if _, err := w.ReadNext(); err == nil {
		t.Fatal("expected a dial error while no daemon is listening")
	}

	home := os.Getenv("HOME")
	dir := stateTierDir(home)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}
	ln, err := net.Listen("unix", filepath.Join(dir, socketBasename))
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	snap := newTestSnapshot("42-implement", "running")
	connCh := acceptAndWriteOnce(t, ln, snap)

	got, err := w.ReadNext()
	if err != nil {
		t.Fatalf("ReadNext() after the daemon started = %v, want a snapshot", err)
	}
	if len(got.Windows) != 1 || got.Windows[0].WindowName != "42-implement" {
		t.Errorf("ReadNext() = %+v, want the daemon's snapshot", got)
	}
	if c := <-connCh; c != nil {
		_ = c.Close()
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
