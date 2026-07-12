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

func TestFakeExecutor_RecordsRunShellOutputCalls(t *testing.T) {
	fe := &FakeExecutor{}
	cmd := "echo hello-world"
	_, _, _ = fe.RunShellOutput(cmd)

	if len(fe.RunShellOutputCalls) != 1 {
		t.Fatalf("RunShellOutputCalls length = %d, want 1", len(fe.RunShellOutputCalls))
	}
	if fe.RunShellOutputCalls[0] != cmd {
		t.Errorf("RunShellOutputCalls[0] = %q, want %q", fe.RunShellOutputCalls[0], cmd)
	}
}

func TestFakeExecutor_ReturnsConfiguredRunShellOutput(t *testing.T) {
	expectedErr := errors.New("command failed")
	expectedStdout := "partial output"
	expectedStderr := "permission denied"
	fe := &FakeExecutor{
		RunShellOutputStdout: expectedStdout,
		RunShellOutputStderr: expectedStderr,
		RunShellOutputErr:    expectedErr,
	}

	stdout, stderr, err := fe.RunShellOutput("failing-command")
	if !errors.Is(err, expectedErr) {
		t.Errorf("RunShellOutput error = %v, want %v", err, expectedErr)
	}
	if stdout != expectedStdout {
		t.Errorf("RunShellOutput stdout = %q, want %q", stdout, expectedStdout)
	}
	if stderr != expectedStderr {
		t.Errorf("RunShellOutput stderr = %q, want %q", stderr, expectedStderr)
	}
}

func TestDefaultExecutor_RunShellOutput_Success(t *testing.T) {
	d := DefaultExecutor{}
	stdout, stderr, err := d.RunShellOutput("echo hi")

	if err != nil {
		t.Fatalf("RunShellOutput error = %v, want nil", err)
	}
	if stdout == "" {
		t.Error("RunShellOutput stdout is empty, want non-empty")
	}
	if stderr != "" {
		t.Errorf("RunShellOutput stderr = %q, want empty string", stderr)
	}
}

func TestDefaultExecutor_RunShellOutput_Failure(t *testing.T) {
	d := DefaultExecutor{}
	stdout, stderr, err := d.RunShellOutput("printf out; printf err >&2; exit 3")

	if err == nil {
		t.Fatal("RunShellOutput error = nil, want non-nil")
	}
	if stdout == "" {
		t.Error("RunShellOutput stdout is empty, want non-empty")
	}
	if stderr == "" {
		t.Error("RunShellOutput stderr is empty, want non-empty")
	}
}

func TestDefaultExecutor_RunShellOutput_StderrOnly(t *testing.T) {
	d := DefaultExecutor{}
	stdout, stderr, err := d.RunShellOutput("echo boom >&2")

	if err != nil {
		t.Fatalf("RunShellOutput error = %v, want nil", err)
	}
	if stdout != "" {
		t.Errorf("RunShellOutput stdout = %q, want empty string", stdout)
	}
	if stderr == "" {
		t.Error("RunShellOutput stderr is empty, want non-empty")
	}
}
