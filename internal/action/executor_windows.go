//go:build windows

package action

import "errors"

// errDispatchLoopUnsupported is returned by StartDetached and SignalProcess
// on Windows: syscall.SysProcAttr has no Setsid field on this platform, so
// the detached-session process model the dispatch loop relies on cannot be
// implemented here.
var errDispatchLoopUnsupported = errors.New("dispatch loop is not supported on Windows")

// StartDetached is unsupported on Windows.
func (d DefaultExecutor) StartDetached(command, logPath string) (int, error) {
	return 0, errDispatchLoopUnsupported
}

// ProcessAlive always reports false on Windows, consistent with
// StartDetached never having successfully spawned a tracked process here.
func (d DefaultExecutor) ProcessAlive(pid int) bool {
	return false
}

// SignalProcess is unsupported on Windows.
func (d DefaultExecutor) SignalProcess(pid int) error {
	return errDispatchLoopUnsupported
}
