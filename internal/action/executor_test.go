package action

import (
	"errors"
	"testing"
)

func TestFakeExecutor_RecordsOpenURLCalls(t *testing.T) {
	fe := &FakeExecutor{}
	url := "https://example.com/issues/42"
	_ = fe.OpenURL(url)

	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURLCalls length = %d, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != url {
		t.Errorf("OpenURLCalls[0] = %q, want %q", fe.OpenURLCalls[0], url)
	}
}

func TestFakeExecutor_RecordsRunShellCalls(t *testing.T) {
	fe := &FakeExecutor{}
	cmd := "echo hello-world"
	_, _ = fe.RunShell(cmd)

	if len(fe.RunShellCalls) != 1 {
		t.Fatalf("RunShellCalls length = %d, want 1", len(fe.RunShellCalls))
	}
	if fe.RunShellCalls[0] != cmd {
		t.Errorf("RunShellCalls[0] = %q, want %q", fe.RunShellCalls[0], cmd)
	}
}

func TestFakeExecutor_ReturnsConfiguredOpenURLError(t *testing.T) {
	expectedErr := errors.New("open failed")
	fe := &FakeExecutor{OpenURLErr: expectedErr}

	err := fe.OpenURL("https://example.com")
	if !errors.Is(err, expectedErr) {
		t.Errorf("OpenURL error = %v, want %v", err, expectedErr)
	}
}

func TestFakeExecutor_ReturnsConfiguredRunShellError(t *testing.T) {
	expectedErr := errors.New("command failed")
	expectedStderr := "permission denied"
	fe := &FakeExecutor{
		RunShellErr:    expectedErr,
		RunShellStderr: expectedStderr,
	}

	stderr, err := fe.RunShell("failing-command")
	if !errors.Is(err, expectedErr) {
		t.Errorf("RunShell error = %v, want %v", err, expectedErr)
	}
	if stderr != expectedStderr {
		t.Errorf("RunShell stderr = %q, want %q", stderr, expectedStderr)
	}
}

func TestFakeExecutor_DefaultsToNilError(t *testing.T) {
	fe := &FakeExecutor{}

	if err := fe.OpenURL("https://example.com"); err != nil {
		t.Errorf("OpenURL error = %v, want nil (default success)", err)
	}

	stderr, err := fe.RunShell("echo ok")
	if err != nil {
		t.Errorf("RunShell error = %v, want nil (default success)", err)
	}
	if stderr != "" {
		t.Errorf("RunShell stderr = %q, want empty string (default)", stderr)
	}
}
