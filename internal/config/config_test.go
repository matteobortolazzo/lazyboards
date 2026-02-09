package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestLoad_ValidGlobalConfig(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")

	cfg := Config{
		Provider: "github",
		Repo:     "owner/repo",
		Project:  "my-project",
		Columns:  []string{"Todo", "In Progress", "Done"},
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}
	if err := os.WriteFile(globalPath, data, 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	result, err := Load(globalPath, filepath.Join(dir, "nonexistent.yml"))
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if result.Provider != "github" {
		t.Errorf("Provider = %q, want %q", result.Provider, "github")
	}
	if result.Repo != "owner/repo" {
		t.Errorf("Repo = %q, want %q", result.Repo, "owner/repo")
	}
	if result.Project != "my-project" {
		t.Errorf("Project = %q, want %q", result.Project, "my-project")
	}
	if len(result.Columns) != 3 {
		t.Fatalf("Columns length = %d, want 3", len(result.Columns))
	}
	if result.Columns[0] != "Todo" || result.Columns[1] != "In Progress" || result.Columns[2] != "Done" {
		t.Errorf("Columns = %v, want [Todo, In Progress, Done]", result.Columns)
	}
}

func TestLoad_MissingGlobalFile_ReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "no-such-global.yml")
	localPath := filepath.Join(dir, "no-such-local.yml")

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if result.Provider != "fake" {
		t.Errorf("Provider = %q, want %q", result.Provider, "fake")
	}
	if result.Repo != "" {
		t.Errorf("Repo = %q, want empty string", result.Repo)
	}
	if result.Project != "" {
		t.Errorf("Project = %q, want empty string", result.Project)
	}
	if result.Columns != nil {
		t.Errorf("Columns = %v, want nil", result.Columns)
	}
}

func TestLoad_LocalOverridesGlobal(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalCfg := Config{
		Provider: "github",
		Repo:     "owner/repo",
		Project:  "my-project",
		Columns:  []string{"Backlog", "Active", "Review"},
	}
	globalData, err := yaml.Marshal(globalCfg)
	if err != nil {
		t.Fatalf("failed to marshal global config: %v", err)
	}
	if err := os.WriteFile(globalPath, globalData, 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	// Local only sets columns
	localYAML := "columns:\n  - Todo\n  - Done\n"
	if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	// Global values should be preserved
	if result.Provider != "github" {
		t.Errorf("Provider = %q, want %q (from global)", result.Provider, "github")
	}
	if result.Repo != "owner/repo" {
		t.Errorf("Repo = %q, want %q (from global)", result.Repo, "owner/repo")
	}

	// Local columns should override global
	if len(result.Columns) != 2 {
		t.Fatalf("Columns length = %d, want 2", len(result.Columns))
	}
	if result.Columns[0] != "Todo" || result.Columns[1] != "Done" {
		t.Errorf("Columns = %v, want [Todo, Done] (from local)", result.Columns)
	}
}

func TestLoad_LocalOverridesProvider(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := "provider: github\n"
	if err := os.WriteFile(globalPath, []byte(globalYAML), 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	localYAML := "provider: ado\n"
	if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if result.Provider != "ado" {
		t.Errorf("Provider = %q, want %q (local should override global)", result.Provider, "ado")
	}
}

func TestLoad_EmptyColumnsInLocalClearsGlobal(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalCfg := Config{
		Columns: []string{"A", "B", "C"},
	}
	globalData, err := yaml.Marshal(globalCfg)
	if err != nil {
		t.Fatalf("failed to marshal global config: %v", err)
	}
	if err := os.WriteFile(globalPath, globalData, 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	localYAML := "columns: []\n"
	if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Columns) != 0 {
		t.Errorf("Columns = %v, want empty slice (local empty should clear global)", result.Columns)
	}
}

func TestLoad_MissingLocalFile_UsesGlobalOnly(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")

	globalCfg := Config{
		Provider: "github",
		Repo:     "org/repo",
		Project:  "board-1",
		Columns:  []string{"Open", "Closed"},
	}
	globalData, err := yaml.Marshal(globalCfg)
	if err != nil {
		t.Fatalf("failed to marshal global config: %v", err)
	}
	if err := os.WriteFile(globalPath, globalData, 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	result, err := Load(globalPath, filepath.Join(dir, "missing-local.yml"))
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if result.Provider != "github" {
		t.Errorf("Provider = %q, want %q", result.Provider, "github")
	}
	if result.Repo != "org/repo" {
		t.Errorf("Repo = %q, want %q", result.Repo, "org/repo")
	}
	if result.Project != "board-1" {
		t.Errorf("Project = %q, want %q", result.Project, "board-1")
	}
	if len(result.Columns) != 2 {
		t.Fatalf("Columns length = %d, want 2", len(result.Columns))
	}
	if result.Columns[0] != "Open" || result.Columns[1] != "Closed" {
		t.Errorf("Columns = %v, want [Open, Closed]", result.Columns)
	}
}

func TestLoad_BothMissing_ReturnsDefaults(t *testing.T) {
	dir := t.TempDir()

	result, err := Load(
		filepath.Join(dir, "absent-global.yml"),
		filepath.Join(dir, "absent-local.yml"),
	)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if result.Provider != "fake" {
		t.Errorf("Provider = %q, want %q", result.Provider, "fake")
	}
	if result.Repo != "" {
		t.Errorf("Repo = %q, want empty string", result.Repo)
	}
	if result.Project != "" {
		t.Errorf("Project = %q, want empty string", result.Project)
	}
	if result.Columns != nil {
		t.Errorf("Columns = %v, want nil", result.Columns)
	}
}

func TestLoad_InvalidYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "bad.yml")

	malformed := "provider: github\n  bad indent: [this is: not valid\n"
	if err := os.WriteFile(globalPath, []byte(malformed), 0644); err != nil {
		t.Fatalf("failed to write malformed config: %v", err)
	}

	_, err := Load(globalPath, filepath.Join(dir, "nonexistent.yml"))
	if err == nil {
		t.Error("Load() returned nil error for invalid YAML, want non-nil error")
	}
}

func TestLoad_InvalidLocalYAML_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "bad-local.yml")

	globalYAML := "provider: github\n"
	if err := os.WriteFile(globalPath, []byte(globalYAML), 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	malformed := "columns: [this is: not valid\n  bad indent\n"
	if err := os.WriteFile(localPath, []byte(malformed), 0644); err != nil {
		t.Fatalf("failed to write malformed local config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Error("Load() returned nil error for invalid local YAML, want non-nil error")
	}
}

func TestDefaultGlobalPath_ContainsExpectedSuffix(t *testing.T) {
	path, err := DefaultGlobalPath()
	if err != nil {
		t.Fatalf("DefaultGlobalPath() returned unexpected error: %v", err)
	}

	expectedSuffix := ".config/lazyboards/config.yml"
	if !strings.HasSuffix(path, expectedSuffix) {
		t.Errorf("DefaultGlobalPath() = %q, want suffix %q", path, expectedSuffix)
	}
}
