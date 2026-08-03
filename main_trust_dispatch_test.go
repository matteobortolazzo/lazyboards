package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// TestMain_TrustDispatch_EndToEnd builds the real lazyboards binary and runs
// it as a subprocess, proving that main() actually dispatches "trust"/
// "untrust" (#568) -- the unit tests in cli_trust_test.go exercise
// trustVerb/runTrustVerb directly and would stay green even if main() never
// called them, which is exactly the bug this test guards against.
func TestMain_TrustDispatch_EndToEnd(t *testing.T) {
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "lazyboards")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir, _ = os.Getwd()
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("failed to build lazyboards binary: %v\n%s", err, out)
	}

	repoDir := t.TempDir()
	localConfigPath := filepath.Join(repoDir, config.DefaultLocalPath)
	if err := os.WriteFile(localConfigPath, []byte("provider: github\nrepo: owner/repo\n"), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	home := t.TempDir()
	trustPath := filepath.Join(home, ".config", "lazyboards", "trust.yml")

	run := func(verb string) (int, string) {
		t.Helper()
		cmd := exec.Command(binPath, verb)
		cmd.Dir = repoDir
		cmd.Env = []string{"HOME=" + home}
		out, err := cmd.CombinedOutput()
		exitCode := 0
		if err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				t.Fatalf("failed to run %q: %v", verb, err)
			}
		}
		return exitCode, string(out)
	}

	wantHash, err := config.HashLocalConfig(localConfigPath)
	if err != nil {
		t.Fatalf("HashLocalConfig() returned unexpected error: %v", err)
	}

	// "trust" once: creates the trust store with one matching entry.
	if code, out := run("trust"); code != 0 {
		t.Fatalf("first `lazyboards trust` exited %d, want 0; output: %s", code, out)
	}
	store, err := config.LoadTrust(trustPath)
	if err != nil {
		t.Fatalf("LoadTrust(%q) returned unexpected error: %v", trustPath, err)
	}
	if len(store.Trusted) != 1 {
		t.Fatalf("Trusted count after first trust = %d, want 1", len(store.Trusted))
	}
	if store.Trusted[0].Hash != wantHash {
		t.Errorf("Trusted[0].Hash = %q, want %q", store.Trusted[0].Hash, wantHash)
	}

	// "trust" again: idempotent, still exactly one entry.
	if code, out := run("trust"); code != 0 {
		t.Fatalf("second `lazyboards trust` exited %d, want 0; output: %s", code, out)
	}
	store, err = config.LoadTrust(trustPath)
	if err != nil {
		t.Fatalf("LoadTrust(%q) returned unexpected error: %v", trustPath, err)
	}
	if len(store.Trusted) != 1 {
		t.Errorf("Trusted count after idempotent re-trust = %d, want 1", len(store.Trusted))
	}

	// "untrust": removes the entry.
	if code, out := run("untrust"); code != 0 {
		t.Fatalf("`lazyboards untrust` exited %d, want 0; output: %s", code, out)
	}
	store, err = config.LoadTrust(trustPath)
	if err != nil {
		t.Fatalf("LoadTrust(%q) returned unexpected error: %v", trustPath, err)
	}
	if store.Trusts(wantHash) {
		t.Errorf("store still trusts %q after `lazyboards untrust`", wantHash)
	}

	// "untrust" in a directory with no local config: non-zero exit.
	emptyDir := t.TempDir()
	cmd := exec.Command(binPath, "untrust")
	cmd.Dir = emptyDir
	cmd.Env = []string{"HOME=" + home}
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("`lazyboards untrust` with no local config succeeded, want non-zero exit; output: %s", out)
	}
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("`lazyboards untrust` failed to run: %v", err)
	}
}
