// Package cenciwatch wires lazyboards to the cenci-watch daemon's unix
// socket, providing tmux window/agent status snapshots for the board's
// session join.
//
// This package has no Go module dependency on cenci/watch: it
// dials the daemon's unix socket directly and decodes its NDJSON
// StateSnapshot stream using only the standard library, the same way any
// other JSON-speaking integration in lazyboards works. The daemon is an
// optional external process — if it isn't running, dialing simply fails and
// the board falls back to showing no agent status badges.
package cenciwatch

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// snapshotMaxBytes bounds the size of a single StateSnapshot JSON line so a
// malformed or oversized stream cannot exhaust memory.
const snapshotMaxBytes = 65536

// Watcher reads successive state snapshots from the cenci-watch daemon.
type Watcher interface {
	// ReadNext blocks until the next snapshot is available (or an error occurs).
	ReadNext() (*StateSnapshot, error)
	// Close releases any underlying connection.
	Close() error
}

// socketWatcher is a Watcher backed by a unix socket connection to the
// cenci-watch daemon. It lazily dials on the first ReadNext call, and
// transparently re-dials on the next call after a read error.
type socketWatcher struct {
	// socketPath pins an explicit socket path. When empty, the path is
	// resolved fresh on every dial via defaultSocketPath, so a daemon started
	// after the board launched — or restarted into a different tier — is
	// picked up without restarting lazyboards.
	socketPath string
	conn       net.Conn
	scanner    *bufio.Scanner
}

var _ Watcher = (*socketWatcher)(nil)

// newSocketWatcher creates a socketWatcher for the given socket path.
// It does not dial immediately; dialing happens lazily on the first ReadNext call.
func newSocketWatcher(path string) *socketWatcher {
	return &socketWatcher{socketPath: path}
}

// NewSocketWatcher creates a Watcher that resolves the default cenci socket
// path on each dial.
func NewSocketWatcher() Watcher {
	return &socketWatcher{}
}

// dialPath returns the path the next dial should target: the pinned path if
// one was given, otherwise a freshly resolved one.
func (w *socketWatcher) dialPath() string {
	if w.socketPath != "" {
		return w.socketPath
	}
	return defaultSocketPath()
}

// socketBasename is the file name of the daemon's broadcast socket inside
// whichever tier directory wins resolution.
const socketBasename = "cenci.sock"

// defaultSocketPath resolves the cenci-watch daemon's broadcast socket path
// by replicating the daemon's own three-tier socket-directory chain, so the
// client and daemon agree on the location without importing the daemon's
// package:
//
//  1. $CENCI_SOCKET_DIR, used verbatim with no appended segment (absolute
//     paths only).
//  2. $XDG_STATE_HOME/cenci/run, defaulting to ~/.local/state/cenci/run.
//  3. /tmp/cenci-<uid>/cenci, the always-available fallback.
//
// Unlike the daemon, which creates and hardens the directory it is about to
// bind, the client only reads: it walks the chain and returns the first
// secure tier that actually holds a live socket. That liveness probe is what
// makes the chain work in practice — a daemon may sit in any tier, and only
// one of them is real at a time. When no tier holds a socket (the daemon is
// not running yet), the highest usable tier's path is returned so the
// watcher's next re-dial targets the location the daemon will occupy once it
// starts.
func defaultSocketPath() string {
	var firstUsable string
	for _, dir := range socketDirCandidates() {
		if !secureSocketDir(dir) {
			continue
		}
		path := filepath.Join(dir, socketBasename)
		if isSocket(path) {
			return path
		}
		if firstUsable == "" {
			firstUsable = path
		}
	}
	if firstUsable != "" {
		return firstUsable
	}
	return filepath.Join(tmpTierDir(), socketBasename)
}

// socketDirCandidates returns the tier directories to probe, in the daemon's
// own precedence order. A tier whose environment inputs are unresolvable is
// omitted rather than guessed at.
func socketDirCandidates() []string {
	var dirs []string
	// Tier 1: the explicit override, used verbatim. A relative path is not a
	// usable socket directory — dialing it would resolve against the board's
	// working directory — so it is dropped rather than made absolute.
	if override := os.Getenv("CENCI_SOCKET_DIR"); override != "" && filepath.IsAbs(override) {
		dirs = append(dirs, override)
	}
	// Tier 2: the state directory, where a default `cenci daemon start` listens.
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			base = filepath.Join(home, ".local", "state")
		}
	}
	if base != "" {
		dirs = append(dirs, filepath.Join(base, "cenci", "run"))
	}
	// Tier 3: the per-uid tmp fallback.
	return append(dirs, tmpTierDir())
}

// tmpTierDir returns the final fallback tier, /tmp/cenci-<uid>/cenci.
func tmpTierDir() string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("cenci-%d", os.Getuid()), "cenci")
}

// secureSocketDir reports whether dir is safe to read a socket out of: it must
// exist as a real directory (never a symlink, which is not followed), be owned
// by the current user, and not be group/other-writable. A directory failing any
// of these could let a local attacker plant a socket that the board would then
// trust as the daemon, so such a tier is skipped instead of dialed.
func secureSocketDir(dir string) bool {
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	if info.Mode().Perm()&0022 != 0 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == os.Getuid()
}

// isSocket reports whether path exists and is a unix socket, so a stale
// regular file left at a tier's socket name is never mistaken for a live
// daemon.
func isSocket(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

// ReadNext dials the cenci socket if not already connected, then reads
// and decodes the next NDJSON snapshot line. On error, the connection is
// closed so the next call re-dials from scratch.
func (w *socketWatcher) ReadNext() (*StateSnapshot, error) {
	if w.conn == nil {
		conn, err := net.Dial("unix", w.dialPath())
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(conn)
		scanner.Buffer(make([]byte, 4096), snapshotMaxBytes)
		w.conn = conn
		w.scanner = scanner
	}

	if !w.scanner.Scan() {
		err := w.scanner.Err()
		if err == nil {
			err = net.ErrClosed
		}
		_ = w.conn.Close()
		w.conn = nil
		w.scanner = nil
		return nil, err
	}

	var snap StateSnapshot
	if err := json.Unmarshal(w.scanner.Bytes(), &snap); err != nil {
		_ = w.conn.Close()
		w.conn = nil
		w.scanner = nil
		return nil, err
	}
	return &snap, nil
}

// Close closes the underlying connection, if any.
func (w *socketWatcher) Close() error {
	if w.conn == nil {
		return nil
	}
	err := w.conn.Close()
	w.conn = nil
	w.scanner = nil
	return err
}
