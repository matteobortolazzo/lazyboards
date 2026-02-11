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
}

func TestLoad_MissingGlobalFile_ReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "no-such-global.yml")
	localPath := filepath.Join(dir, "no-such-local.yml")

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if result.Provider != "" {
		t.Errorf("Provider = %q, want empty string", result.Provider)
	}
	if result.Repo != "" {
		t.Errorf("Repo = %q, want empty string", result.Repo)
	}
}

func TestLoad_LocalOverridesGlobal(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalCfg := Config{
		Provider: "github",
		Repo:     "owner/repo",
	}
	globalData, err := yaml.Marshal(globalCfg)
	if err != nil {
		t.Fatalf("failed to marshal global config: %v", err)
	}
	if err := os.WriteFile(globalPath, globalData, 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	// Local only sets repo
	localYAML := "repo: other/repo\n"
	if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	// Global provider should be preserved
	if result.Provider != "github" {
		t.Errorf("Provider = %q, want %q (from global)", result.Provider, "github")
	}
	// Local repo should override global
	if result.Repo != "other/repo" {
		t.Errorf("Repo = %q, want %q (from local)", result.Repo, "other/repo")
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

func TestLoad_MissingLocalFile_UsesGlobalOnly(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")

	globalCfg := Config{
		Provider: "github",
		Repo:     "org/repo",
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

	if result.Provider != "" {
		t.Errorf("Provider = %q, want empty string", result.Provider)
	}
	if result.Repo != "" {
		t.Errorf("Repo = %q, want empty string", result.Repo)
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

func TestSaveGlobal_WritesProviderOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "global.yml")

	cfg := Config{
		Provider: "github",
		Repo:     "owner/repo",
	}
	if err := cfg.SaveGlobal(path); err != nil {
		t.Fatalf("SaveGlobal() returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "provider:") {
		t.Error("saved file should contain 'provider:' key")
	}
	if strings.Contains(content, "repo:") {
		t.Error("saved file should not contain 'repo:' key, but it does")
	}

	// Verify the value round-trips correctly
	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal saved YAML: %v", err)
	}
	if result["provider"] != "github" {
		t.Errorf("provider = %q, want %q", result["provider"], "github")
	}
}

func TestSaveGlobal_CreatesParentDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeply", "global.yml")

	cfg := Config{Provider: "github"}
	if err := cfg.SaveGlobal(path); err != nil {
		t.Fatalf("SaveGlobal() returned unexpected error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist at %s, got error: %v", path, err)
	}
}

func TestSaveLocal_WritesRepoOnly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "local.yml")

	cfg := Config{
		Provider: "github",
		Repo:     "owner/repo",
	}
	if err := cfg.SaveLocal(path); err != nil {
		t.Fatalf("SaveLocal() returned unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "repo:") {
		t.Error("saved file should contain 'repo:' key")
	}
	if strings.Contains(content, "provider:") {
		t.Error("saved file should not contain 'provider:' key, but it does")
	}

	// Verify the value round-trips correctly
	var result map[string]string
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to unmarshal saved YAML: %v", err)
	}
	if result["repo"] != "owner/repo" {
		t.Errorf("repo = %q, want %q", result["repo"], "owner/repo")
	}
}

func TestExists_ReturnsTrueWhenGlobalExists(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	if err := os.WriteFile(globalPath, []byte("provider: github\n"), 0644); err != nil {
		t.Fatalf("failed to create global file: %v", err)
	}

	if !Exists(globalPath, localPath) {
		t.Error("Exists() = false, want true when global file exists")
	}
}

func TestExists_ReturnsTrueWhenLocalExists(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	if err := os.WriteFile(localPath, []byte("repo: owner/repo\n"), 0644); err != nil {
		t.Fatalf("failed to create local file: %v", err)
	}

	if !Exists(globalPath, localPath) {
		t.Error("Exists() = false, want true when local file exists")
	}
}

func TestExists_ReturnsFalseWhenBothMissing(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	if Exists(globalPath, localPath) {
		t.Error("Exists() = true, want false when both files are missing")
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
