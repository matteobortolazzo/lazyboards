package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// --- trustVerb (#568) ---
//
// trustVerb mirrors versionRequested's shape: it inspects os.Args-style
// arguments and reports which trust-store verb (if any) was requested. Only
// a bare "trust"/"untrust" as args[1] matches -- no flags are supported, so
// "trust --force" must NOT match.

func TestTrustVerb(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantVerb string
		wantOK   bool
	}{
		{"trust", []string{"lazyboards", "trust"}, "trust", true},
		{"untrust", []string{"lazyboards", "untrust"}, "untrust", true},
		{"trust with flag not supported", []string{"lazyboards", "trust", "--force"}, "", false},
		{"no args", []string{"lazyboards"}, "", false},
		{"unrelated first arg", []string{"lazyboards", "serve"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVerb, gotOK := trustVerb(tt.args)
			if gotVerb != tt.wantVerb || gotOK != tt.wantOK {
				t.Errorf("trustVerb(%v) = (%q, %v), want (%q, %v)", tt.args, gotVerb, gotOK, tt.wantVerb, tt.wantOK)
			}
		})
	}
}

// --- runTrustVerb (#568) ---

// writeCLITrustLocalConfig writes minimal-but-valid local config content to
// path, mirroring the "local config exists" precondition every trust/untrust
// flow assumes.
func writeCLITrustLocalConfig(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("provider: github\nrepo: owner/repo\n"), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}
}

func TestRunTrustVerb_TrustAddsEntry(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	trustPath := filepath.Join(dir, "trust.yml")
	writeCLITrustLocalConfig(t, localPath)

	var out bytes.Buffer
	code := runTrustVerb("trust", localPath, trustPath, "owner/repo", &out)
	if code != 0 {
		t.Fatalf("runTrustVerb(\"trust\", ...) = %d, want 0; output: %s", code, out.String())
	}

	store, err := config.LoadTrust(trustPath)
	if err != nil {
		t.Fatalf("LoadTrust() returned unexpected error: %v", err)
	}
	if len(store.Trusted) != 1 {
		t.Fatalf("Trusted count = %d, want 1", len(store.Trusted))
	}
	wantHash, err := config.HashLocalConfig(localPath)
	if err != nil {
		t.Fatalf("HashLocalConfig() returned unexpected error: %v", err)
	}
	if store.Trusted[0].Hash != wantHash {
		t.Errorf("Trusted[0].Hash = %q, want %q", store.Trusted[0].Hash, wantHash)
	}
	if store.Trusted[0].Note != "owner/repo" {
		t.Errorf("Trusted[0].Note = %q, want %q", store.Trusted[0].Note, "owner/repo")
	}
}

func TestRunTrustVerb_TrustTwice_NoDuplicateEntry(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	trustPath := filepath.Join(dir, "trust.yml")
	writeCLITrustLocalConfig(t, localPath)

	var out bytes.Buffer
	if code := runTrustVerb("trust", localPath, trustPath, "owner/repo", &out); code != 0 {
		t.Fatalf("first runTrustVerb(\"trust\", ...) = %d, want 0", code)
	}
	if code := runTrustVerb("trust", localPath, trustPath, "owner/repo", &out); code != 0 {
		t.Fatalf("second runTrustVerb(\"trust\", ...) = %d, want 0", code)
	}

	store, err := config.LoadTrust(trustPath)
	if err != nil {
		t.Fatalf("LoadTrust() returned unexpected error: %v", err)
	}
	// Exactly 1: re-running "trust" against the same content must be
	// idempotent, not append a duplicate entry every invocation.
	if len(store.Trusted) != 1 {
		t.Errorf("Trusted count = %d, want 1 (idempotent re-trust must not duplicate)", len(store.Trusted))
	}
}

func TestRunTrustVerb_UntrustAfterTrust_RemovesEntry(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	trustPath := filepath.Join(dir, "trust.yml")
	writeCLITrustLocalConfig(t, localPath)

	var out bytes.Buffer
	if code := runTrustVerb("trust", localPath, trustPath, "owner/repo", &out); code != 0 {
		t.Fatalf("runTrustVerb(\"trust\", ...) = %d, want 0", code)
	}

	code := runTrustVerb("untrust", localPath, trustPath, "owner/repo", &out)
	if code != 0 {
		t.Fatalf("runTrustVerb(\"untrust\", ...) = %d, want 0; output: %s", code, out.String())
	}

	store, err := config.LoadTrust(trustPath)
	if err != nil {
		t.Fatalf("LoadTrust() returned unexpected error: %v", err)
	}
	wantHash, err := config.HashLocalConfig(localPath)
	if err != nil {
		t.Fatalf("HashLocalConfig() returned unexpected error: %v", err)
	}
	if store.Trusts(wantHash) {
		t.Errorf("store still trusts %q after untrust", wantHash)
	}
}

func TestRunTrustVerb_UntrustWithNothingTrusted_Idempotent(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	trustPath := filepath.Join(dir, "trust.yml")
	writeCLITrustLocalConfig(t, localPath)

	var out bytes.Buffer
	code := runTrustVerb("untrust", localPath, trustPath, "owner/repo", &out)
	if code != 0 {
		t.Fatalf("runTrustVerb(\"untrust\", ...) = %d, want 0 (nothing to remove is not an error); output: %s", code, out.String())
	}
}

func TestRunTrustVerb_NoLocalConfig_NonZeroExitWithMessage(t *testing.T) {
	verbs := []string{"trust", "untrust"}
	for _, verb := range verbs {
		t.Run(verb, func(t *testing.T) {
			dir := t.TempDir()
			localPath := filepath.Join(dir, "nonexistent-local.yml")
			trustPath := filepath.Join(dir, "trust.yml")

			var out bytes.Buffer
			code := runTrustVerb(verb, localPath, trustPath, "owner/repo", &out)
			if code == 0 {
				t.Fatalf("runTrustVerb(%q, ...) = 0, want non-zero exit when no local config exists", verb)
			}
			if out.Len() == 0 {
				t.Errorf("runTrustVerb(%q, ...) wrote nothing to out, want a clear message explaining the missing local config", verb)
			}
		})
	}
}

func TestRunTrustVerb_MalformedTrustStore_FailsClosedByteIdentical(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	trustPath := filepath.Join(dir, "trust.yml")
	writeCLITrustLocalConfig(t, localPath)

	malformed := []byte("trusted: \"this is not a list\"\n")
	if err := os.WriteFile(trustPath, malformed, 0600); err != nil {
		t.Fatalf("failed to write malformed trust store: %v", err)
	}

	var out bytes.Buffer
	code := runTrustVerb("trust", localPath, trustPath, "owner/repo", &out)
	if code == 0 {
		t.Fatalf("runTrustVerb(\"trust\", ...) = 0, want non-zero exit for a malformed trust store")
	}

	after, err := os.ReadFile(trustPath)
	if err != nil {
		t.Fatalf("failed to read trust store after call: %v", err)
	}
	if !bytes.Equal(after, malformed) {
		t.Errorf("trust store bytes changed after a malformed-store call\nbefore: %q\nafter:  %q", malformed, after)
	}
}

func TestRunTrustVerb_NeverWritesSiblingGlobalConfigPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	trustPath := filepath.Join(dir, "trust.yml")
	writeCLITrustLocalConfig(t, localPath)

	globalPath, err := config.DefaultGlobalPath()
	if err != nil {
		t.Fatalf("DefaultGlobalPath() returned unexpected error: %v", err)
	}
	if _, statErr := os.Stat(globalPath); !os.IsNotExist(statErr) {
		t.Fatalf("precondition failed: global config path %q already exists", globalPath)
	}

	var out bytes.Buffer
	if code := runTrustVerb("trust", localPath, trustPath, "owner/repo", &out); code != 0 {
		t.Fatalf("runTrustVerb(\"trust\", ...) = %d, want 0; output: %s", code, out.String())
	}

	if _, statErr := os.Stat(globalPath); !os.IsNotExist(statErr) {
		t.Errorf("global config path %q was created/modified by runTrustVerb, want it untouched", globalPath)
	}
}
