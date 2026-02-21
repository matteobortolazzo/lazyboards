package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
