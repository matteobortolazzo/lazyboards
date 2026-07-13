// Package dispatchloop provides pure primitives for tracking a background
// `agentwatch dispatch --interval` process via a pidfile. It has no
// dependency on BubbleTea or internal/action so it can be used, and tested,
// in complete isolation from the TUI.
package dispatchloop

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	pidFileName = "dispatch-loop.pid"
	logFileName = "dispatch-loop.log"
)

// DefaultDir returns the directory used to store the dispatch loop's pidfile
// and log file: $XDG_STATE_HOME/lazyboards, falling back to
// ~/.local/state/lazyboards when XDG_STATE_HOME is unset or empty.
func DefaultDir() string {
	if stateHome := os.Getenv("XDG_STATE_HOME"); stateHome != "" {
		return filepath.Join(stateHome, "lazyboards")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// Fall back to a relative path rather than failing outright; callers
		// that need a guaranteed-absolute path can still join it themselves.
		home = "."
	}
	return filepath.Join(home, ".local", "state", "lazyboards")
}

// PidPath returns the path to the dispatch loop pidfile within dir.
func PidPath(dir string) string {
	return filepath.Join(dir, pidFileName)
}

// LogPath returns the path to the dispatch loop log file within dir.
func LogPath(dir string) string {
	return filepath.Join(dir, logFileName)
}

// ReadPid reads the pid stored at path. A missing file means the loop is
// stopped and returns (0, nil) -- not an error. Malformed content (present
// but not a valid integer) is a distinct, ambiguous state and returns an
// error rather than being folded into "stopped".
func ReadPid(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read pidfile %s: %w", path, err)
	}
	text := strings.TrimSpace(string(data))
	pid, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("parse pidfile %s: content %q is not a valid pid: %w", path, text, err)
	}
	return pid, nil
}

// WritePid writes pid to path atomically (temp file + rename), creating any
// missing parent directories first.
func WritePid(path string, pid int) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".dispatch-loop.pid.*.tmp")
	if err != nil {
		return fmt.Errorf("create temp pidfile in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.WriteString(strconv.Itoa(pid)); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp pidfile %s: %w", tmpPath, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp pidfile %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename %s to %s: %w", tmpPath, path, err)
	}
	return nil
}

// RemovePid deletes the pidfile at path. It is idempotent: a missing file is
// not an error.
func RemovePid(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove pidfile %s: %w", path, err)
	}
	return nil
}
