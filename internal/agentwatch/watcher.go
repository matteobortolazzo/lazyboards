// Package agentwatch wires lazyboards to the agentwatch daemon's unix socket,
// providing tmux window/agent status snapshots for the board's session join.
package agentwatch

import (
	"github.com/matteobortolazzo/agent-stack/agentwatch/pkg/watch"
)

// Watcher reads successive state snapshots from the agentwatch daemon.
type Watcher interface {
	// ReadNext blocks until the next snapshot is available (or an error occurs).
	ReadNext() (*watch.StateSnapshot, error)
	// Close releases any underlying connection.
	Close() error
}

// socketWatcher is a Watcher backed by a unix socket connection to the
// agentwatch daemon. It lazily dials on the first ReadNext call, and
// transparently re-dials on the next call after a read error.
type socketWatcher struct {
	socketPath string
	client     *watch.Client
}

var _ Watcher = (*socketWatcher)(nil)

// newSocketWatcher creates a socketWatcher for the given socket path.
// It does not dial immediately; dialing happens lazily on the first ReadNext call.
func newSocketWatcher(path string) *socketWatcher {
	return &socketWatcher{socketPath: path}
}

// NewSocketWatcher creates a Watcher connected to the default agentwatch socket path.
func NewSocketWatcher() Watcher {
	return newSocketWatcher(watch.DefaultSocketPath())
}

// ReadNext dials the agentwatch socket if not already connected, then reads
// the next snapshot. On error, the connection is closed so the next call
// re-dials from scratch.
func (w *socketWatcher) ReadNext() (*watch.StateSnapshot, error) {
	if w.client == nil {
		client, err := watch.Dial(w.socketPath)
		if err != nil {
			return nil, err
		}
		w.client = client
	}

	snap, err := w.client.ReadSnapshot()
	if err != nil {
		_ = w.client.Close()
		w.client = nil
		return nil, err
	}
	return snap, nil
}

// Close closes the underlying connection, if any.
func (w *socketWatcher) Close() error {
	if w.client == nil {
		return nil
	}
	err := w.client.Close()
	w.client = nil
	return err
}
