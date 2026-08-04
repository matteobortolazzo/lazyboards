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

func TestDefaultExecutor_OpenURL_RejectsInvalidScheme(t *testing.T) {
	d := DefaultExecutor{}

	invalidURLs := []string{
		"javascript:alert(1)",
		"not-a-url",
		"file:///etc/passwd",
		"-rf",
	}

	for _, u := range invalidURLs {
		err := d.OpenURL(u)
		if err == nil {
			t.Errorf("OpenURL(%q) error = nil, want non-nil (invalid scheme)", u)
			continue
		}
		if !errors.Is(err, errInvalidURLScheme) {
			t.Errorf("OpenURL(%q) error = %v, want errInvalidURLScheme", u, err)
		}
	}
}

func TestDefaultExecutor_OpenURL_AcceptsHTTPAndHTTPSSchemes(t *testing.T) {
	// Asserts against the validator directly, not OpenURL: calling OpenURL
	// with a real accepted URL actually spawns xdg-open (or the platform
	// equivalent) and pops a real browser tab on a dev machine.
	validURLs := []string{
		"http://example.com",
		"https://example.com/issues/42",
	}

	for _, u := range validURLs {
		got, err := validateOpenURL(u)
		if err != nil {
			t.Errorf("validateOpenURL(%q) error = %v, want nil", u, err)
			continue
		}
		if got != u {
			t.Errorf("validateOpenURL(%q) = %q, want unmodified input %q", u, got, u)
		}
	}
}

// TestValidateOpenURL is the table-driven validator matrix for
// validateOpenURL: exact case-insensitive http/https scheme, non-empty
// hostname, and fail-closed on anything net/url.Parse can't make sense of.
// This is the guard for the ticket's actual repro -- a repo-local keymap
// action binding a URL containing shell metacharacters (&, |, ^, %, quotes)
// must still be treated as inert data, never re-interpreted by a shell.
func TestValidateOpenURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Accept: valid http/https URLs, including ones carrying shell
		// metacharacters as inert path/query data.
		{name: "http scheme", input: "http://example.com", wantErr: false},
		{name: "https scheme with path", input: "https://example.com/issues/42", wantErr: false},
		{name: "uppercase HTTPS scheme", input: "HTTPS://example.com", wantErr: false},
		{name: "mixed-case scheme and host", input: "HtTp://Example.COM/x", wantErr: false},
		{name: "ticket repro: shell metacharacters as inert path data", input: "https://example.invalid/&calc.exe", wantErr: false},
		{name: "percent-encoded path and query", input: "https://example.com/p%20q?a=1&b=2#frag", wantErr: false},
		{name: "shell metacharacters in path", input: "https://example.com/^|&", wantErr: false},
		{name: "quotes in path", input: `https://example.com/"q"`, wantErr: false},
		{name: "raw space in path", input: "https://example.com/a b", wantErr: false},
		{name: "valid percent-escape reaching as data", input: "https://example.invalid/?x=%25TEMP%25", wantErr: false},
		{name: "IDN unicode host", input: "https://exämple.de/ünicode", wantErr: false},
		{name: "IDN punycode host", input: "https://xn--exmple-cua.de/x", wantErr: false},
		{name: "userinfo in authority", input: "https://user:pass@example.com/x", wantErr: false},

		// Reject: wrong/missing scheme, missing host, or malformed syntax.
		{name: "file scheme", input: "file:///etc/passwd", wantErr: true},
		{name: "javascript scheme", input: "javascript:alert(1)", wantErr: true},
		{name: "protocol-relative", input: "//example.com", wantErr: true},
		{name: "http missing host", input: "http://", wantErr: true},
		{name: "https missing host", input: "https://", wantErr: true},
		{name: "port-only authority", input: "http://:8080", wantErr: true},
		{name: "opaque https (no authority)", input: "https:example.com", wantErr: true},
		{name: "not a url", input: "not-a-url", wantErr: true},
		{name: "flag-like string", input: "-rf", wantErr: true},
		{name: "leading whitespace", input: " https://example.com", wantErr: true},
		{name: "malformed percent-escape", input: "https://example.invalid/%TEMP%", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateOpenURL(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateOpenURL(%q) error = nil, want non-nil", tt.input)
				}
				return
			}

			if err != nil {
				t.Fatalf("validateOpenURL(%q) error = %v, want nil", tt.input, err)
			}
			// Locks in no-normalization: the validator must pass the URL
			// through byte-for-byte, not rewrite/re-encode it.
			if got != tt.input {
				t.Errorf("validateOpenURL(%q) = %q, want unmodified input %q", tt.input, got, tt.input)
			}
		})
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
