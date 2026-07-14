package debuglog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- New(path) instance-level logger (#333) ---

// TestNew_WithPath_WritesTimestampedErrorLine verifies a file-backed logger
// (path set) appends one line per Errorf call in "<RFC3339 timestamp>
// <error text>\n" format.
func TestNew_WithPath_WritesTimestampedErrorLine(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.log")

	lg, err := New(path)
	if err != nil {
		t.Fatalf("New(%q) error = %v, want nil", path, err)
	}
	if lg == nil {
		t.Fatal("New() returned a nil logger for a valid, writable path")
	}

	lg.Errorf("watcher error: %s", "connection refused")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file %q: %v", path, err)
	}
	content := string(data)
	if !strings.Contains(content, "connection refused") {
		t.Errorf("log file content = %q, want it to contain the logged error text", content)
	}

	line := strings.TrimRight(content, "\n")
	fields := strings.SplitN(line, " ", 2)
	if len(fields) != 2 {
		t.Fatalf("log line = %q, want '<timestamp> <error text>' format", line)
	}
	if _, err := time.Parse(time.RFC3339, fields[0]); err != nil {
		t.Errorf("log line timestamp = %q, want a valid RFC3339 timestamp: %v", fields[0], err)
	}
}

// TestNew_WithPath_AppendsAcrossMultipleWrites verifies each Errorf call
// appends a new line rather than truncating the file, so a log-write test
// against an env-var-provided path accumulates a full history of watcher
// errors across the process lifetime.
func TestNew_WithPath_AppendsAcrossMultipleWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.log")

	lg, err := New(path)
	if err != nil {
		t.Fatalf("New(%q) error = %v, want nil", path, err)
	}

	lg.Errorf("first blip")
	lg.Errorf("second blip")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file %q: %v", path, err)
	}
	content := string(data)
	if !strings.Contains(content, "first blip") || !strings.Contains(content, "second blip") {
		t.Errorf("log file content = %q, want both entries present (append, not overwrite)", content)
	}

	lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("log file has %d lines, want exactly 2 (one per Errorf call)", len(lines))
	}
}

// TestNew_EmptyPath_ReturnsNoOpLoggerThatCreatesNoFile verifies path=="" is a
// complete no-op: no file is created anywhere, and calling Errorf on the
// disabled logger is harmless.
func TestNew_EmptyPath_ReturnsNoOpLoggerThatCreatesNoFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	lg, err := New("")
	if err != nil {
		t.Fatalf("New(\"\") error = %v, want nil (disabled logger, not an error)", err)
	}

	lg.Errorf("should not be written anywhere")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("New(\"\") + Errorf created %d file(s) in the working directory, want a complete no-op (zero files)", len(entries))
	}
}

// TestNew_UnwritablePath_ReturnsErrorWithoutPanicking verifies a path whose
// parent directory does not exist fails to open gracefully: New returns a
// non-nil error, and the returned logger (if any) never panics.
func TestNew_UnwritablePath_ReturnsErrorWithoutPanicking(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-such-parent-dir", "debug.log")

	lg, err := New(path)
	if err == nil {
		t.Fatalf("New(%q) error = nil, want non-nil (parent directory does not exist)", path)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Errorf on a logger from a failed New() panicked: %v", r)
		}
	}()
	lg.Errorf("should not panic even though construction failed")
}

// --- Nil-receiver safety (#333) ---

// TestLogger_NilReceiver_ErrorfDoesNotPanic verifies a zero-value (nil)
// *Logger's Errorf method no-ops safely, so call sites never need a nil
// check before logging.
func TestLogger_NilReceiver_ErrorfDoesNotPanic(t *testing.T) {
	var lg *Logger

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil-receiver Errorf panicked: %v", r)
		}
	}()
	lg.Errorf("boom: %s", "detail")
}

// TestLogger_NilReceiver_LogDoesNotPanic verifies the lower-level Log method
// is equally nil-safe.
func TestLogger_NilReceiver_LogDoesNotPanic(t *testing.T) {
	var lg *Logger

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil-receiver Log panicked: %v", r)
		}
	}()
	lg.Log("boom")
}

// --- Package-level Init/Errorf (#333) ---

// TestInit_WithPath_PackageErrorfWritesToFile verifies the package-level
// convenience API (Init + free-function Errorf) delegates to a file-backed
// logger once Init is given a path.
func TestInit_WithPath_PackageErrorfWritesToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "debug.log")

	if err := Init(path); err != nil {
		t.Fatalf("Init(%q) error = %v, want nil", path, err)
	}
	t.Cleanup(func() { std = nil })

	Errorf("dispatch watcher unreachable: %s", "connection refused")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file %q: %v", path, err)
	}
	if !strings.Contains(string(data), "connection refused") {
		t.Errorf("log file content = %q, want it to contain the logged error text", string(data))
	}
}

// TestInit_EmptyPath_PackageErrorfIsNoOp verifies Init("") (the unset-env-var
// case) makes the package-level Errorf a complete no-op: no file is created.
func TestInit_EmptyPath_PackageErrorfIsNoOp(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	if err := Init(""); err != nil {
		t.Fatalf("Init(\"\") error = %v, want nil", err)
	}
	t.Cleanup(func() { std = nil })

	Errorf("should not be logged anywhere")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("Init(\"\") + Errorf created %d file(s), want a complete no-op (zero files)", len(entries))
	}
}

// TestPackageErrorf_BeforeInit_DoesNotPanic verifies calling the free
// function Errorf before Init has ever been called (std is nil) is safe,
// since std defaults to a nil *Logger.
func TestPackageErrorf_BeforeInit_DoesNotPanic(t *testing.T) {
	std = nil
	t.Cleanup(func() { std = nil })

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("package-level Errorf panicked before Init was ever called: %v", r)
		}
	}()
	Errorf("boom")
}
