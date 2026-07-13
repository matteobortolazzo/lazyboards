//go:build !windows

package action

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// reapChild blocks (with a bounded timeout) until pid exits and is reaped.
//
// StartDetached's contract never calls Wait() -- that's intentional, since
// in production the parent (lazyboards) exits soon after spawning and the
// kernel reparents the child to init, which reaps it. Within a single
// long-lived test binary, though, the test process remains the real parent,
// so an exited child sits as a zombie (still visible to a signal-0
// ProcessAlive probe) until something calls wait() on it. This helper
// stands in for that eventual reap so tests can observe the transition to
// "dead" deterministically.
func reapChild(t *testing.T, pid int) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		var ws syscall.WaitStatus
		_, err := syscall.Wait4(pid, &ws, 0, nil)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("reap pid %d: %v", pid, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out reaping pid %d", pid)
	}
}

func TestDefaultExecutor_StartDetached_WritesToLogThenBecomesDeadAfterReap(t *testing.T) {
	d := DefaultExecutor{}
	logPath := filepath.Join(t.TempDir(), "detached.log")

	pid, err := d.StartDetached("echo hi", logPath)
	if err != nil {
		t.Fatalf("StartDetached error = %v, want nil", err)
	}
	if pid <= 0 {
		t.Fatalf("StartDetached pid = %d, want > 0", pid)
	}

	// Poll briefly for the log content -- the child runs asynchronously.
	deadline := time.Now().Add(2 * time.Second)
	var content string
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(logPath)
		if readErr == nil && strings.Contains(string(data), "hi") {
			content = string(data)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(content, "hi") {
		t.Fatalf("log content = %q, want to contain %q", content, "hi")
	}

	reapChild(t, pid)

	if d.ProcessAlive(pid) {
		t.Errorf("ProcessAlive(%d) = true after exit+reap, want false", pid)
	}
}

func TestDefaultExecutor_ProcessAlive_CurrentProcessIsAlive(t *testing.T) {
	d := DefaultExecutor{}
	if !d.ProcessAlive(os.Getpid()) {
		t.Error("ProcessAlive(os.Getpid()) = false, want true")
	}
}

func TestDefaultExecutor_SignalProcess_TerminatesLongRunningChild(t *testing.T) {
	d := DefaultExecutor{}
	logPath := filepath.Join(t.TempDir(), "long.log")

	pid, err := d.StartDetached("sleep 30", logPath)
	if err != nil {
		t.Fatalf("StartDetached error = %v, want nil", err)
	}

	// Give the child a brief moment to actually start before signalling.
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) && !d.ProcessAlive(pid) {
		time.Sleep(10 * time.Millisecond)
	}
	if !d.ProcessAlive(pid) {
		t.Fatalf("ProcessAlive(%d) = false before signalling, want true", pid)
	}

	if err := d.SignalProcess(pid); err != nil {
		t.Fatalf("SignalProcess error = %v, want nil", err)
	}

	reapChild(t, pid)

	if d.ProcessAlive(pid) {
		t.Errorf("ProcessAlive(%d) = true after SIGTERM+reap, want false", pid)
	}
}
