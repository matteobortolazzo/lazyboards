package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The pre-0.73 top-level `actions:` and per-column `columns[].actions:`
// blocks are no longer a supported config syntax: `keymaps:` is the only one.
// A config still declaring either fails to load with an error naming the
// offending file and key, rather than being silently ignored (which would
// leave a user's keybindings quietly dead) or translated behind their back.

// legacyActionsYAML is a populated top-level actions: block.
const legacyActionsYAML = `provider: github
actions:
  O:
    name: Open
    type: url
    url: "https://example.com"
`

// legacyColumnActionsYAML is a populated per-column actions: block.
const legacyColumnActionsYAML = `provider: github
columns:
  - name: New
    actions:
      O:
        name: Open
        type: url
        url: "https://example.com"
`

func TestLoad_RejectsTopLevelActionsBlock(t *testing.T) {
	_, err := loadConfigFromStrings(t, "", legacyActionsYAML)
	if err == nil {
		t.Fatal("Load() returned nil error for a config declaring a top-level actions: block, want a rejection")
	}
	assertLegacyRejection(t, err, "actions:")
}

func TestLoad_RejectsColumnActionsBlock(t *testing.T) {
	_, err := loadConfigFromStrings(t, "", legacyColumnActionsYAML)
	if err == nil {
		t.Fatal("Load() returned nil error for a config declaring a columns[].actions: block, want a rejection")
	}
	assertLegacyRejection(t, err, `columns["New"].actions:`)
}

// TestLoad_RejectsValuelessLegacyBlocks covers the presence-based rule: the
// key's mere presence is the rejection trigger, whatever its value. An empty
// mapping or an explicit null is still a stale block the user should delete,
// not a request for "no actions".
func TestLoad_RejectsValuelessLegacyBlocks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		localYML string
		wantKey  string
	}{
		{"empty top-level mapping", "provider: github\nactions: {}\n", "actions:"},
		{"null top-level", "provider: github\nactions: ~\n", "actions:"},
		{"valueless top-level", "provider: github\nactions:\n", "actions:"},
		{"empty column mapping", "provider: github\ncolumns:\n  - name: New\n    actions: {}\n", `columns["New"].actions:`},
		{"null column", "provider: github\ncolumns:\n  - name: New\n    actions: ~\n", `columns["New"].actions:`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadConfigFromStrings(t, "", tc.localYML)
			if err == nil {
				t.Fatalf("Load() returned nil error for %s, want a rejection", tc.name)
			}
			assertLegacyRejection(t, err, tc.wantKey)
		})
	}
}

// TestLoad_RejectionNamesTheOffendingFile pins the decision that a user
// locked out by a legacy block can tell which of the two config files to
// edit: the error carries that file's own path, not the other one's.
func TestLoad_RejectionNamesTheOffendingFile(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	for _, tc := range []struct {
		name        string
		globalYML   string
		localYML    string
		wantPath    string
		notWantPath string
	}{
		{"local file", "provider: github\n", legacyActionsYAML, localPath, globalPath},
		{"global file", legacyActionsYAML, "provider: github\n", globalPath, localPath},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(globalPath, []byte(tc.globalYML), 0644); err != nil {
				t.Fatalf("failed to write global config: %v", err)
			}
			if err := os.WriteFile(localPath, []byte(tc.localYML), 0644); err != nil {
				t.Fatalf("failed to write local config: %v", err)
			}

			_, err := Load(globalPath, localPath, Trust{Trusted: []TrustEntry{{Hash: hashConfigBytes([]byte(tc.localYML))}}})
			if err == nil {
				t.Fatalf("Load() returned nil error for a legacy block in the %s, want a rejection", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantPath) {
				t.Errorf("error %q does not name the offending file %q", err, tc.wantPath)
			}
			if strings.Contains(err.Error(), tc.notWantPath) {
				t.Errorf("error %q names the innocent file %q", err, tc.notWantPath)
			}
		})
	}
}

// TestLoad_KeymapsOnlyConfigStillLoads is the positive control: nothing about
// the rejection may reject a config that expresses the same bindings the
// supported way.
func TestLoad_KeymapsOnlyConfigStillLoads(t *testing.T) {
	cfg := mustLoadConfig(t, "", `provider: github
columns:
  - name: New
    cleanup: "echo done"
keymaps:
  normal:
    O:
      name: Open
      type: url
      url: "https://example.com"
  columns:
    New:
      R:
        name: Review
        type: url
        url: "https://example.com/review"
`)

	if len(cfg.Keymaps.Modes) == 0 {
		t.Fatal("cfg.Keymaps.Modes is empty, want the normal-mode table the config declares")
	}
	if _, ok := cfg.Keymaps.Columns["New"]; !ok {
		t.Errorf("cfg.Keymaps.Columns missing the %q overlay the config declares, got %v", "New", cfg.Keymaps.Columns)
	}
	if _, err := ResolveKeymap(&cfg); err != nil {
		t.Errorf("ResolveKeymap() returned unexpected error for a keymaps:-only config: %v", err)
	}
}

// assertLegacyRejection checks the rejection is the legacy-block one (naming
// the offending key and pointing at keymaps:), not some unrelated load error
// that happens to be non-nil.
func assertLegacyRejection(t *testing.T, err error, wantKey string) {
	t.Helper()
	msg := err.Error()
	if !strings.Contains(msg, wantKey) {
		t.Errorf("error %q does not name the offending key %q", msg, wantKey)
	}
	if !strings.Contains(msg, "keymaps:") {
		t.Errorf("error %q does not point at the keymaps: namespace", msg)
	}
}
