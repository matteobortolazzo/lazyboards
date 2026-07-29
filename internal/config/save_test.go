package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"gopkg.in/yaml.v3"
)

// --- Save and LocalExists tests ---

func TestSave_WritesValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	err := Save(path, "github", "owner/repo")
	if err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	// Read back and verify.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("saved file is not valid YAML: %v", err)
	}
	if cfg.Provider != "github" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "github")
	}
	if cfg.Repo != "owner/repo" {
		t.Errorf("Repo = %q, want %q", cfg.Repo, "owner/repo")
	}
}

func TestSave_OverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	// Write initial config.
	err := Save(path, "github", "old-owner/old-repo")
	if err != nil {
		t.Fatalf("first Save() returned unexpected error: %v", err)
	}

	// Overwrite with new values.
	err = Save(path, "ado", "new-owner/new-repo")
	if err != nil {
		t.Fatalf("second Save() returned unexpected error: %v", err)
	}

	// Read back and verify the new values.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("saved file is not valid YAML: %v", err)
	}
	if cfg.Provider != "ado" {
		t.Errorf("Provider = %q, want %q after overwrite", cfg.Provider, "ado")
	}
	if cfg.Repo != "new-owner/new-repo" {
		t.Errorf("Repo = %q, want %q after overwrite", cfg.Repo, "new-owner/new-repo")
	}
}

func TestSave_PreservesExistingActions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	// Write initial config with actions.
	initialYAML := `provider: github
repo: owner/repo
actions:
  o:
    name: Open in browser
    type: url
    url: "https://example.com/{number}"
`
	if err := os.WriteFile(path, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	// Save with new provider/repo values.
	err := Save(path, "ado", "new-owner/new-repo")
	if err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	// Read back and verify actions are preserved.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("saved file is not valid YAML: %v", err)
	}

	// Provider and repo should be updated.
	if cfg.Provider != "ado" {
		t.Errorf("Provider = %q, want %q", cfg.Provider, "ado")
	}
	if cfg.Repo != "new-owner/new-repo" {
		t.Errorf("Repo = %q, want %q", cfg.Repo, "new-owner/new-repo")
	}

	// Actions should be preserved.
	if len(cfg.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1 (actions should be preserved)", len(cfg.Actions))
	}
	act, ok := cfg.Actions["o"]
	if !ok {
		t.Fatal("Actions missing key 'o' (should be preserved)")
	}
	if act.Name != "Open in browser" {
		t.Errorf("Actions[o].Name = %q, want %q (should be preserved)", act.Name, "Open in browser")
	}
	if act.Type != "url" {
		t.Errorf("Actions[o].Type = %q, want %q (should be preserved)", act.Type, "url")
	}
}

func TestSave_PreservesExistingCleanup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	initialYAML := `provider: github
repo: owner/repo
cleanup: "tmux kill-window -t ={session} 2>/dev/null || true"
columns:
  - name: Implementing
    cleanup: "docker stop {session}"
`
	if err := os.WriteFile(path, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	err := Save(path, "ado", "new-owner/new-repo")
	if err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("saved file is not valid YAML: %v", err)
	}

	if cfg.Cleanup == nil || *cfg.Cleanup != "tmux kill-window -t ={session} 2>/dev/null || true" {
		t.Errorf("Cleanup = %v, want top-level cleanup to be preserved", cfg.Cleanup)
	}
	if len(cfg.Columns) != 1 || cfg.Columns[0].Cleanup == nil || *cfg.Columns[0].Cleanup != "docker stop {session}" {
		t.Errorf("Columns[0].Cleanup = %v, want per-column cleanup to be preserved", cfg.Columns)
	}
}

func TestSave_RejectsNonYAMLExtension(t *testing.T) {
	cases := []struct {
		name     string
		filename string
	}{
		{"txt extension", "config.txt"},
		{"sh extension", "script.sh"},
		{"json extension", "data.json"},
		{"no extension", "noext"},
		{"dotfile no real extension", ".bashrc"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.filename)

			err := Save(path, "github", "owner/repo")
			if err == nil {
				t.Fatalf("Save(%q) returned nil error, want error for non-YAML extension", tc.filename)
			}

			// Error message should contain the path so the user knows which file was rejected.
			if !strings.Contains(err.Error(), path) {
				t.Errorf("error = %q, want it to contain the path %q", err.Error(), path)
			}

			// The file should NOT have been created (no side effects on rejection).
			if _, statErr := os.Stat(path); statErr == nil {
				t.Errorf("Save(%q) rejected the extension but still created the file", tc.filename)
			}
		})
	}
}

func TestSave_AcceptsYAMLExtension(t *testing.T) {
	cases := []struct {
		name     string
		filename string
	}{
		{"yml extension", "config.yml"},
		{"yaml extension", "config.yaml"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.filename)

			err := Save(path, "github", "owner/repo")
			if err != nil {
				t.Fatalf("Save(%q) returned unexpected error: %v", tc.filename, err)
			}

			// File should exist and contain valid YAML.
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("failed to read saved config: %v", err)
			}

			var cfg Config
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				t.Fatalf("saved file is not valid YAML: %v", err)
			}
			if cfg.Provider != "github" {
				t.Errorf("Provider = %q, want %q", cfg.Provider, "github")
			}
			if cfg.Repo != "owner/repo" {
				t.Errorf("Repo = %q, want %q", cfg.Repo, "owner/repo")
			}
		})
	}
}

func TestLocalExists_ReturnsTrueForExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	// Create the file.
	if err := os.WriteFile(path, []byte("provider: github\n"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if !LocalExists(path) {
		t.Error("LocalExists() = false, want true for existing file")
	}
}

func TestLocalExists_ReturnsFalseForMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.yml")

	if LocalExists(path) {
		t.Error("LocalExists() = true, want false for missing file")
	}
}

// --- Save() keymaps: round-trip preservation (#509) ---

// TestSave_PreservesKeymapsBlock is the AC4 regression test: Save() must
// preserve an existing keymaps: block through the unmarshal/re-marshal round
// trip instead of dropping it.
func TestSave_PreservesKeymapsBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	initialYAML := `provider: github
repo: owner/repo
keymaps:
  normal:
    n: card.create
`
	if err := os.WriteFile(path, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := Save(path, "ado", "new-owner/new-repo"); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("saved file is not valid YAML: %v", err)
	}

	if cfg.Keymaps == nil {
		t.Fatal("Keymaps is nil after Save(), want the keymaps: block preserved")
	}
	binding, ok := cfg.Keymaps.Modes[keymap.ModeNormal]["n"]
	if !ok {
		t.Fatal("Keymaps.Modes[normal] missing key \"n\" after Save()")
	}
	if binding.Kind != keymap.BindingCommand || binding.Command != "card.create" {
		t.Errorf("Keymaps.Modes[normal][n] = %+v, want the original command binding preserved", binding)
	}
}

// TestSave_KeymapsKeyOrderSurvivesSave is the AC5 regression test: it walks
// the written file as a raw yaml.Node tree (not the unmarshaled Config) so
// the assertion is against the file's actual declared key order, which a
// plain map re-marshal would otherwise randomize/alphabetize away.
func TestSave_KeymapsKeyOrderSurvivesSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	initialYAML := `provider: github
repo: owner/repo
keymaps:
  normal:
    z: card.zebra
    a: card.apple
    m: card.mango
`
	if err := os.WriteFile(path, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := Save(path, "ado", "new-owner/new-repo"); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		t.Fatalf("saved file is not valid YAML: %v", err)
	}
	normalNode := findMappingValue(t, findMappingValue(t, root.Content[0], "keymaps"), "normal")

	positions := make(map[string]int, len(normalNode.Content)/2)
	for i := 0; i+1 < len(normalNode.Content); i += 2 {
		positions[normalNode.Content[i].Value] = i
	}
	if positions["z"] >= positions["a"] {
		t.Errorf("saved key position z=%d, a=%d, want z < a (declared order must survive Save())", positions["z"], positions["a"])
	}
	if positions["a"] >= positions["m"] {
		t.Errorf("saved key position a=%d, m=%d, want a < m (declared order must survive Save())", positions["a"], positions["m"])
	}
}

// TestSave_NoKeymapsBlock_ProducesNoKeymapsKeyOnSave is the back-compat case:
// a config with no keymaps: block must not gain one on save.
func TestSave_NoKeymapsBlock_ProducesNoKeymapsKeyOnSave(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	initialYAML := `provider: github
repo: owner/repo
`
	if err := os.WriteFile(path, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := Save(path, "ado", "new-owner/new-repo"); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	if strings.Contains(string(data), "keymaps:") {
		t.Errorf("saved file = %q, want no \"keymaps:\" key when no keymaps were configured", string(data))
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("saved file is not valid YAML: %v", err)
	}
	if cfg.Keymaps != nil {
		t.Errorf("Keymaps = %+v, want nil after saving a config with no keymaps: block", cfg.Keymaps)
	}
}

// TestSave_StructurallyInvalidKeymaps_RefusesAndPreservesFile is the
// data-loss regression test: Save() must surface a structurally invalid
// existing keymaps: block as an error instead of silently rewriting a
// truncated file (today's `_ = yaml.Unmarshal(...)` swallows the error and
// overwrites the file with a config that dropped the invalid block).
func TestSave_StructurallyInvalidKeymaps_RefusesAndPreservesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	initialYAML := `provider: github
repo: owner/repo
keymaps:
  bogus_mode:
    n: card.create
`
	if err := os.WriteFile(path, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read initial config: %v", err)
	}

	err = Save(path, "ado", "new-owner/new-repo")
	if err == nil {
		t.Fatal("Save() returned nil error, want error for a structurally invalid existing keymaps: block")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read config after failed Save(): %v", err)
	}
	if string(after) != string(before) {
		t.Errorf("file was modified by a failed Save() call\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// --- Save() legacy actions: round-trip preservation (#510) ---

// TestSave_LegacyActionsBlock_RoundTripsWithoutSynthesizedKeymapsBlock is
// the #510 Risks-section regression test: Save() re-reads the config
// straight from disk and never calls Load() (and therefore never calls
// translateLegacyActions), so an existing legacy actions: block must
// round-trip byte-for-field intact and the file must not gain a
// synthesized keymaps: block as a side effect of PR 1's changes to Load().
func TestSave_LegacyActionsBlock_RoundTripsWithoutSynthesizedKeymapsBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")

	initialYAML := `provider: github
repo: owner/repo
actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
`
	if err := os.WriteFile(path, []byte(initialYAML), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	if err := Save(path, "ado", "new-owner/new-repo"); err != nil {
		t.Fatalf("Save() returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	if strings.Contains(string(data), "keymaps:") {
		t.Errorf("saved file = %q, want no synthesized \"keymaps:\" key for a config that only ever had a legacy actions: block", string(data))
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("saved file is not valid YAML: %v", err)
	}
	if cfg.Keymaps != nil {
		t.Errorf("Keymaps = %+v, want nil after Save() round-trips a legacy-only config", cfg.Keymaps)
	}
	if len(cfg.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1 (legacy actions: block must round-trip intact)", len(cfg.Actions))
	}
	action, ok := cfg.Actions["P"]
	if !ok {
		t.Fatal("Actions missing key \"P\" after Save() round-trip")
	}
	if action.Name != "Push" || action.Type != "shell" || action.Command != "git push" {
		t.Errorf("Actions[P] = %+v, want the original legacy action fields untouched", action)
	}
}

// findMappingValue returns the value node for key within a YAML mapping
// node, failing the test if key is not found or node is not a mapping.
func findMappingValue(t *testing.T, node *yaml.Node, key string) *yaml.Node {
	t.Helper()
	if node == nil || node.Kind != yaml.MappingNode {
		t.Fatalf("expected a mapping node while looking for key %q, got %v", key, node)
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	t.Fatalf("mapping node missing key %q", key)
	return nil
}
