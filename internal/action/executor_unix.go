//go:build !windows

package action

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// StartDetached spawns command via "sh -c" fully detached from the current
// process: a new session (via Setsid) so it survives this process exiting,
// stdin from /dev/null, and stdout+stderr appended to logPath. It calls
// cmd.Start() (never Run()/Wait()) and immediately releases the process
// handle so the caller does not keep the child as a wait-dependent.
func (d DefaultExecutor) StartDetached(command, logPath string) (int, error) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", os.DevNull, err)
	}
	defer devNull.Close()

	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return 0, fmt.Errorf("open log file %s: %w", logPath, err)
	}
	defer logFile.Close()

	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = devNull
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start detached command: %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Process.Release(); err != nil {
		return pid, fmt.Errorf("release detached process %d: %w", pid, err)
	}
	return pid, nil
}

// ProcessAlive probes whether pid is alive via a signal-0 kill (which sends
// no signal, only checks existence/permission). ESRCH ("no such process")
// means dead; any other error -- including EPERM, which can mean the
// process exists but is owned by another user -- conservatively reports the
// process as alive. A false "dead" result here would let a caller spawn a
// duplicate dispatch loop.
func (d DefaultExecutor) ProcessAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if err == syscall.ESRCH {
		return false
	}
	return true
}

// SignalProcess sends SIGTERM to pid.
func (d DefaultExecutor) SignalProcess(pid int) error {
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return fmt.Errorf("signal pid %d: %w", pid, err)
	}
	return nil
}
