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

func TestFakeExecutor_RecordsSwitchToWindowCalls(t *testing.T) {
	fe := &FakeExecutor{}
	session := "my-session"
	windowIndex := "3"
	_ = fe.SwitchToWindow(session, windowIndex)

	if len(fe.SwitchWindowCalls) != 1 {
		t.Fatalf("SwitchWindowCalls length = %d, want 1", len(fe.SwitchWindowCalls))
	}
	if fe.SwitchWindowCalls[0].Session != session {
		t.Errorf("SwitchWindowCalls[0].Session = %q, want %q", fe.SwitchWindowCalls[0].Session, session)
	}
	if fe.SwitchWindowCalls[0].WindowIndex != windowIndex {
		t.Errorf("SwitchWindowCalls[0].WindowIndex = %q, want %q", fe.SwitchWindowCalls[0].WindowIndex, windowIndex)
	}
}

func TestFakeExecutor_ReturnsConfiguredSwitchToWindowError(t *testing.T) {
	expectedErr := errors.New("switch failed")
	fe := &FakeExecutor{SwitchWindowErr: expectedErr}

	err := fe.SwitchToWindow("session", "1")
	if !errors.Is(err, expectedErr) {
		t.Errorf("SwitchToWindow error = %v, want %v", err, expectedErr)
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

	if err := fe.SwitchToWindow("session", "1"); err != nil {
		t.Errorf("SwitchToWindow error = %v, want nil (default success)", err)
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

func TestFakeExecutor_RunShellOutput_ScriptedResultsConsumedInOrderThenFallback(t *testing.T) {
	fe := &FakeExecutor{
		RunShellOutputResults: []RunShellOutputResult{
			{Stdout: "first-out", Stderr: "", Err: nil},
			{Stdout: "second-out", Stderr: "second-warn", Err: errors.New("second-boom")},
		},
		RunShellOutputStdout: "fallback-out",
		RunShellOutputStderr: "fallback-err",
	}

	stdout1, stderr1, err1 := fe.RunShellOutput("cmd1")
	if stdout1 != "first-out" || stderr1 != "" || err1 != nil {
		t.Errorf("call 1 = (%q, %q, %v), want (%q, %q, nil)", stdout1, stderr1, err1, "first-out", "")
	}

	stdout2, stderr2, err2 := fe.RunShellOutput("cmd2")
	if stdout2 != "second-out" || stderr2 != "second-warn" || err2 == nil || err2.Error() != "second-boom" {
		t.Errorf("call 2 = (%q, %q, %v), want (%q, %q, %q)", stdout2, stderr2, err2, "second-out", "second-warn", "second-boom")
	}

	// Script exhausted after 2 scripted results -- the 3rd call must fall back
	// to the single canned fields, preserving backward compatibility with
	// existing single-result tests.
	stdout3, stderr3, err3 := fe.RunShellOutput("cmd3")
	if stdout3 != "fallback-out" || stderr3 != "fallback-err" || err3 != nil {
		t.Errorf("call 3 (post-exhaustion) = (%q, %q, %v), want fallback (%q, %q, nil)", stdout3, stderr3, err3, "fallback-out", "fallback-err")
	}

	if len(fe.RunShellOutputCalls) != 3 {
		t.Fatalf("RunShellOutputCalls length = %d, want 3", len(fe.RunShellOutputCalls))
	}
	wantCalls := []string{"cmd1", "cmd2", "cmd3"}
	for i, want := range wantCalls {
		if fe.RunShellOutputCalls[i] != want {
			t.Errorf("RunShellOutputCalls[%d] = %q, want %q", i, fe.RunShellOutputCalls[i], want)
		}
	}
}

func TestFakeExecutor_RunShellOutput_EmptyScriptUsesCannedFields(t *testing.T) {
	// Backward compat: a FakeExecutor with no RunShellOutputResults scripted
	// must behave exactly as before -- every call returns the single canned
	// fields, regardless of how many times RunShellOutput is invoked.
	fe := &FakeExecutor{
		RunShellOutputStdout: "canned-out",
		RunShellOutputStderr: "canned-err",
	}

	for i := 0; i < 2; i++ {
		stdout, stderr, err := fe.RunShellOutput("cmd")
		if stdout != "canned-out" || stderr != "canned-err" || err != nil {
			t.Errorf("call %d = (%q, %q, %v), want (%q, %q, nil)", i, stdout, stderr, err, "canned-out", "canned-err")
		}
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
