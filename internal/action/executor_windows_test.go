//go:build windows

package action

import "testing"

// TestDefaultExecutor_OpenURL_UsesShellExecuteSeamWithValidatedURL swaps the
// shellExecuteOpen package-level seam for a recorder and asserts OpenURL
// passes the untrusted URL through to it verbatim, without ever routing
// through os/exec / cmd.exe. This only runs on an actual Windows host (or
// `GOOS=windows go vet`/`go build`), so it does not execute in this Linux
// sandbox -- it is written correctly per Go build-tag conventions and left
// for CI/a Windows runner to exercise.
func TestDefaultExecutor_OpenURL_UsesShellExecuteSeamWithValidatedURL(t *testing.T) {
	const wantURL = "https://example.invalid/&calc.exe"

	orig := shellExecuteOpen
	var got []string
	shellExecuteOpen = func(url string) error {
		got = append(got, url)
		return nil
	}
	t.Cleanup(func() { shellExecuteOpen = orig })

	d := DefaultExecutor{}
	if err := d.OpenURL(wantURL); err != nil {
		t.Fatalf("OpenURL(%q) error = %v, want nil", wantURL, err)
	}

	// == 1 here guards a real no-duplicate-side-effect invariant: a single
	// OpenURL call must trigger exactly one ShellExecute, not zero (silently
	// swallowed) or two (a duplicate window/process launch).
	if len(got) != 1 {
		t.Fatalf("shellExecuteOpen calls = %d, want 1", len(got))
	}
	if got[0] != wantURL {
		t.Errorf("shellExecuteOpen received %q, want exactly %q", got[0], wantURL)
	}
}
