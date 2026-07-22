// Package debuglog provides an optional, file-backed debug logger for
// transient errors (e.g. watcher reconnect blips) that must never be
// written to stdout/stderr while a BubbleTea program owns the terminal.
package debuglog

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime/debug"
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

// crashPath is the destination file for panic reports written by
// RecoverCrash. Empty (the default) disables crash-file writing; RecoverCrash
// still re-panics so terminal restoration is never affected. It is separate
// from the LAZYBOARDS_DEBUG_LOG-backed std logger on purpose: crash reporting
// must be on by default (you cannot predict a crash), whereas the debug log is
// an opt-in diagnostic trail.
var crashPath string

// InitCrash sets the file that RecoverCrash writes panic reports to. An empty
// path disables crash-file writing. The parent directory is created lazily on
// the first crash, so the path need not exist yet.
func InitCrash(path string) {
	crashPath = path
}

// RecoverCrash is a deferred panic handler that writes a timestamped crash
// report (panic value + full stack trace) to the file configured via
// InitCrash, then RE-PANICS. Re-panicking is essential: a BubbleTea program
// owns the terminal, and its own recovery is what restores it — swallowing the
// panic here would leave the terminal in raw/altscreen mode. `where` labels
// the call site (e.g. "Update", "View") in the report. With no panic in
// flight, it is a no-op.
//
// Writing is best-effort: any error opening or writing the report is
// swallowed so a logging failure can never mask, or take precedence over, the
// original crash.
func RecoverCrash(where string) {
	r := recover()
	if r == nil {
		return
	}
	// debug.Stack() here still captures the original panic site: deferred
	// functions run before the stack unwinds past the panicking frame.
	writeCrashReport(crashPath, where, r, debug.Stack(), time.Now())
	panic(r)
}

// writeCrashReport appends one crash report to path. It is split out from
// RecoverCrash so the timestamp is injectable for tests. An empty path, or any
// I/O failure, is a silent no-op — crash logging must never itself panic.
func writeCrashReport(path, where string, r any, stack []byte, now time.Time) {
	if path == "" {
		return
	}
	if dir := filepath.Dir(path); dir != "" {
		_ = os.MkdirAll(dir, 0o700)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = fmt.Fprintf(f, "%s panic in %s: %v\n%s\n",
		now.Format(time.RFC3339), where, r, stack)
}
