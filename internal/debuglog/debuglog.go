// Package debuglog provides an optional, file-backed debug logger for
// transient errors (e.g. watcher reconnect blips) that must never be
// written to stdout/stderr while a BubbleTea program owns the terminal.
package debuglog

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Logger writes timestamped lines to an underlying io.Writer. A nil
// *Logger, or one with a nil writer, is always safe to call: all methods
// no-op instead of panicking.
type Logger struct {
	mu sync.Mutex
	w  io.Writer
}

// New creates a file-backed Logger for path. An empty path returns a
// disabled logger (non-nil, writer nil) with a nil error, creating no
// file. A non-empty path is opened in append/create mode; on failure New
// returns a nil *Logger and the error.
func New(path string) (*Logger, error) {
	if path == "" {
		return &Logger{}, nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	return &Logger{w: f}, nil
}

// Log writes a single timestamped line "<RFC3339 timestamp> <msg>\n" to
// the underlying writer. Safe to call on a nil receiver or a disabled
// logger (no-op).
func (l *Logger) Log(msg string) {
	if l == nil || l.w == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	// Best-effort sink: a write failure here must never propagate or panic.
	_, _ = fmt.Fprintf(l.w, "%s %s\n", time.Now().Format(time.RFC3339), msg)
}

// Errorf formats a message per format/args and logs it via Log. Safe to
// call on a nil receiver or a disabled logger (no-op).
func (l *Logger) Errorf(format string, args ...any) {
	if l == nil {
		return
	}
	l.Log(fmt.Sprintf(format, args...))
}

// std is the package-level logger used by the free-function Errorf. It
// defaults to nil, which Errorf's nil-receiver safety handles.
var std *Logger

// Init opens (or disables, for an empty path) the package-level logger
// used by Errorf. On failure, std is left nil so subsequent Errorf calls
// safely no-op rather than panicking.
func Init(path string) error {
	lg, err := New(path)
	if err != nil {
		std = nil
		return err
	}
	std = lg
	return nil
}

// Errorf delegates to the package-level logger. Safe to call before Init
// (std is nil) or after a failed Init.
func Errorf(format string, args ...any) {
	std.Errorf(format, args...)
}

// Log delegates to the package-level logger. Safe to call before Init (std
// is nil) or after a failed Init, matching Errorf's nil-safety.
func Log(msg string) {
	std.Log(msg)
}
