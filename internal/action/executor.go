package action

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// Executor defines methods for executing actions.
type Executor interface {
	OpenURL(url string) error
	RunShell(command string) (stderr string, err error)
	RunShellOutput(command string) (stdout, stderr string, err error)
	SwitchToWindow(session, windowIndex string) error
}

// DefaultExecutor executes actions using real OS calls.
type DefaultExecutor struct{}

// OpenURL opens a URL in the system browser.
func (d DefaultExecutor) OpenURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("cmd", "/c", "start", "", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// RunShell executes a command via sh -c and returns stderr and error.
func (d DefaultExecutor) RunShell(command string) (string, error) {
	cmd := exec.Command("sh", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), err
	}
	return "", nil
}

// RunShellOutput executes a command via sh -c and returns stdout, stderr, and error.
func (d DefaultExecutor) RunShellOutput(command string) (string, string, error) {
	cmd := exec.Command("sh", "-c", command)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// SwitchToWindow selects and switches to the tmux window identified by
// session and windowIndex ("<session>:<windowIndex>"). It runs
// `tmux select-window` followed by `tmux switch-client`, using discrete
// exec.Command args (never a shell string) since session/windowIndex values
// ultimately derive from untrusted ticket data.
func (d DefaultExecutor) SwitchToWindow(session, windowIndex string) error {
	target := session + ":" + windowIndex
	if output, err := exec.Command("tmux", "select-window", "-t", target).CombinedOutput(); err != nil {
		return fmt.Errorf("select-window: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("tmux", "switch-client", "-t", target).CombinedOutput(); err != nil {
		return fmt.Errorf("switch-client: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// SwitchWindowCall records a single SwitchToWindow invocation.
type SwitchWindowCall struct {
	Session     string
	WindowIndex string
}

// FakeExecutor records calls for testing.
type FakeExecutor struct {
	OpenURLCalls         []string
	RunShellCalls        []string
	SwitchWindowCalls    []SwitchWindowCall
	OpenURLErr           error
	RunShellErr          error
	RunShellStderr       string
	RunShellOutputCalls  []string
	RunShellOutputStdout string
	RunShellOutputStderr string
	RunShellOutputErr    error
	SwitchWindowErr      error
}

// OpenURL records the call and returns the configured error.
func (f *FakeExecutor) OpenURL(url string) error {
	f.OpenURLCalls = append(f.OpenURLCalls, url)
	return f.OpenURLErr
}

// RunShell records the call and returns the configured stderr and error.
func (f *FakeExecutor) RunShell(command string) (string, error) {
	f.RunShellCalls = append(f.RunShellCalls, command)
	return f.RunShellStderr, f.RunShellErr
}

// RunShellOutput records the call and returns the configured stdout, stderr, and error.
func (f *FakeExecutor) RunShellOutput(command string) (string, string, error) {
	f.RunShellOutputCalls = append(f.RunShellOutputCalls, command)
	return f.RunShellOutputStdout, f.RunShellOutputStderr, f.RunShellOutputErr
}

// SwitchToWindow records the call and returns the configured error.
func (f *FakeExecutor) SwitchToWindow(session, windowIndex string) error {
	f.SwitchWindowCalls = append(f.SwitchWindowCalls, SwitchWindowCall{Session: session, WindowIndex: windowIndex})
	return f.SwitchWindowErr
}
