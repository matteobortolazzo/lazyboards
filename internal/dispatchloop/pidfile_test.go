package dispatchloop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRawFile writes raw content to path for test setup, without going
// through WritePid's pid-formatting logic.
func writeRawFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

func TestPidPath_JoinsDirWithFixedFilename(t *testing.T) {
	dir := "/tmp/some-state-dir"
	got := PidPath(dir)
	want := filepath.Join(dir, "dispatch-loop.pid")
	if got != want {
		t.Errorf("PidPath(%q) = %q, want %q", dir, got, want)
	}
}

func TestLogPath_JoinsDirWithFixedFilename(t *testing.T) {
	dir := "/tmp/some-state-dir"
	got := LogPath(dir)
	want := filepath.Join(dir, "dispatch-loop.log")
	if got != want {
		t.Errorf("LogPath(%q) = %q, want %q", dir, got, want)
	}
}

func TestWriteReadPid_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := PidPath(dir)

	if err := WritePid(path, 4242); err != nil {
		t.Fatalf("WritePid error = %v, want nil", err)
	}

	got, err := ReadPid(path)
	if err != nil {
		t.Fatalf("ReadPid error = %v, want nil", err)
	}
	if got != 4242 {
		t.Errorf("ReadPid = %d, want 4242", got)
	}
}

func TestWritePid_CreatesParentDirs(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "state", "dir")
	path := PidPath(dir)

	if err := WritePid(path, 99); err != nil {
		t.Fatalf("WritePid error = %v, want nil (should mkdir parents)", err)
	}

	got, err := ReadPid(path)
	if err != nil {
		t.Fatalf("ReadPid error = %v, want nil", err)
	}
	if got != 99 {
		t.Errorf("ReadPid = %d, want 99", got)
	}
}

func TestReadPid_MissingFileMeansStopped(t *testing.T) {
	dir := t.TempDir()
	path := PidPath(dir)

	pid, err := ReadPid(path)
	if err != nil {
		t.Fatalf("ReadPid error = %v, want nil for missing file", err)
	}
	if pid != 0 {
		t.Errorf("ReadPid = %d, want 0 for missing file", pid)
	}
}

func TestReadPid_MalformedContentIsAnError(t *testing.T) {
	dir := t.TempDir()
	path := PidPath(dir)
	if err := writeRawFile(path, "not-a-pid"); err != nil {
		t.Fatalf("writeRawFile setup error = %v", err)
	}

	pid, err := ReadPid(path)
	if err == nil {
		t.Fatalf("ReadPid error = nil, want non-nil for malformed content (got pid=%d)", pid)
	}
}

func TestRemovePid_IdempotentOnMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := PidPath(dir)

	if err := RemovePid(path); err != nil {
		t.Fatalf("RemovePid on missing file error = %v, want nil (idempotent)", err)
	}
}

func TestRemovePid_RemovesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := PidPath(dir)
	if err := WritePid(path, 1); err != nil {
		t.Fatalf("WritePid setup error = %v", err)
	}

	if err := RemovePid(path); err != nil {
		t.Fatalf("RemovePid error = %v, want nil", err)
	}

	pid, err := ReadPid(path)
	if err != nil {
		t.Fatalf("ReadPid after remove error = %v, want nil", err)
	}
	if pid != 0 {
		t.Errorf("ReadPid after remove = %d, want 0 (stopped)", pid)
	}
}

func TestDefaultDir_HonorsXDGStateHome(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "/custom/state")

	got := DefaultDir()
	want := filepath.Join("/custom/state", "lazyboards")
	if got != want {
		t.Errorf("DefaultDir() = %q, want %q", got, want)
	}
}

func TestDefaultDir_FallsBackToHomeLocalStateWhenUnset(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", "")

	got := DefaultDir()
	if !strings.HasSuffix(got, filepath.Join(".local", "state", "lazyboards")) {
		t.Errorf("DefaultDir() = %q, want suffix %q", got, filepath.Join(".local", "state", "lazyboards"))
	}
}
