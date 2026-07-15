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

// NewSocketWatcher creates a Watcher connected to the default cenci socket path.
func NewSocketWatcher() Watcher {
	return newSocketWatcher(defaultSocketPath())
}

// defaultSocketPath resolves the cenci-watch daemon's broadcast socket path:
// <runtime-dir>/cenci/cenci.sock. This replicates the daemon's own
// secureSocketDir() resolution so the client and daemon agree on the socket
// location without needing to import the daemon's package.
func defaultSocketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir != "" {
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || info.Mode().Perm()&0022 != 0 {
			dir = ""
		}
	}
	if dir == "" {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("cenci-%d", os.Getuid()))
	}
	return filepath.Join(dir, "cenci", "cenci.sock")
}

// ReadNext dials the cenci socket if not already connected, then reads
// and decodes the next NDJSON snapshot line. On error, the connection is
// closed so the next call re-dials from scratch.
func (w *socketWatcher) ReadNext() (*StateSnapshot, error) {
	if w.conn == nil {
		conn, err := net.Dial("unix", w.socketPath)
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
