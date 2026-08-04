package action

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
)

// Executor defines methods for executing actions.
type Executor interface {
	OpenURL(url string) error
	RunShell(command string) (stderr string, err error)
	RunShellOutput(command string) (stdout, stderr string, err error)
}

// DefaultExecutor executes actions using real OS calls.
type DefaultExecutor struct{}

// errInvalidURLScheme is returned by OpenURL when the URL does not use the
// http or https scheme, or does not carry a non-empty host.
var errInvalidURLScheme = errors.New("invalid URL scheme")

// validateOpenURL parses raw and requires an exact (case-insensitive)
// http or https scheme plus a non-empty hostname. Action templates can
// embed untrusted values (e.g. GitHub issue/PR URLs), so OpenURL must
// reject anything else before handing the URL to the platform opener. On
// success it returns raw unmodified -- never u.String(), which would
// re-encode IDN hosts and rewrite spaces, regressing Unicode domains.
func validateOpenURL(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%w: %q", errInvalidURLScheme, raw)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return "", fmt.Errorf("%w: %q", errInvalidURLScheme, raw)
	}
	return raw, nil
}

// OpenURL opens a URL in the system browser.
func (d DefaultExecutor) OpenURL(rawURL string) error {
	validated, err := validateOpenURL(rawURL)
	if err != nil {
		return err
	}
	return openBrowser(validated)
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

// RunShellOutputResult is one scripted result returned by
// FakeExecutor.RunShellOutput.
type RunShellOutputResult struct {
	Stdout string
	Stderr string
	Err    error
}

// FakeExecutor records calls for testing.
type FakeExecutor struct {
	OpenURLCalls   []string
	RunShellCalls  []string
	OpenURLErr     error
	RunShellErr    error
	RunShellStderr string

	// RunShellOutputResults, when non-empty, scripts successive
	// RunShellOutput calls in order (mirroring FakeWatcher.Results in
	// internal/cenciwatch/fake.go). Once exhausted, RunShellOutput falls
	// back to the single canned RunShellOutputStdout/Stderr/Err fields
	// below, keeping this fully backward-compatible with existing tests
	// that only set the canned fields.
	RunShellOutputResults []RunShellOutputResult
	runShellOutputIndex   int

	RunShellOutputCalls  []string
	RunShellOutputStdout string
	RunShellOutputStderr string
	RunShellOutputErr    error
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

// RunShellOutput records the call and returns the next scripted result from
// RunShellOutputResults, in order; once exhausted (or if never scripted), it
// falls back to the canned RunShellOutputStdout/Stderr/Err fields.
func (f *FakeExecutor) RunShellOutput(command string) (string, string, error) {
	f.RunShellOutputCalls = append(f.RunShellOutputCalls, command)
	if f.runShellOutputIndex < len(f.RunShellOutputResults) {
		result := f.RunShellOutputResults[f.runShellOutputIndex]
		f.runShellOutputIndex++
		return result.Stdout, result.Stderr, result.Err
	}
	return f.RunShellOutputStdout, f.RunShellOutputStderr, f.RunShellOutputErr
}
