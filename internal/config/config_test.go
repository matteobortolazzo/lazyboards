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
	if result.Project != "" {
		t.Errorf("Project = %q, want empty string", result.Project)
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
	}
	globalData, err := yaml.Marshal(globalCfg)
	if err != nil {
		t.Fatalf("failed to marshal global config: %v", err)
	}
	if err := os.WriteFile(globalPath, globalData, 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	// Local overrides repo
	localYAML := "repo: other-owner/other-repo\n"
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

	// Local repo should override global
	if result.Repo != "other-owner/other-repo" {
		t.Errorf("Repo = %q, want %q (from local)", result.Repo, "other-owner/other-repo")
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
		Project:  "board-1",
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
	if result.Project != "" {
		t.Errorf("Project = %q, want empty string", result.Project)
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

func TestLoad_UnknownYAMLFields_Ignored(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")

	// A config file with unknown fields (e.g., old "columns" field) should load successfully.
	yamlContent := "provider: github\nrepo: owner/repo\ncolumns:\n  - A\n  - B\n"
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, filepath.Join(dir, "nonexistent.yml"))
	if err != nil {
		t.Fatalf("Load() returned unexpected error for config with unknown fields: %v", err)
	}

	if result.Provider != "github" {
		t.Errorf("Provider = %q, want %q", result.Provider, "github")
	}
	if result.Repo != "owner/repo" {
		t.Errorf("Repo = %q, want %q", result.Repo, "owner/repo")
	}
}

// --- Action parsing and validation tests ---

func TestLoad_ParsesActionsFromYAML(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  o:
    name: Open in browser
    type: url
    url: "https://example.com/{id}"
  x:
    name: Run tests
    type: shell
    command: "go test ./..."
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Actions) != 2 {
		t.Fatalf("Actions count = %d, want 2", len(result.Actions))
	}

	urlAction, ok := result.Actions["o"]
	if !ok {
		t.Fatal("Actions missing key 'o'")
	}
	if urlAction.Name != "Open in browser" {
		t.Errorf("Actions[o].Name = %q, want %q", urlAction.Name, "Open in browser")
	}
	if urlAction.Type != "url" {
		t.Errorf("Actions[o].Type = %q, want %q", urlAction.Type, "url")
	}
	if urlAction.URL != "https://example.com/{id}" {
		t.Errorf("Actions[o].URL = %q, want %q", urlAction.URL, "https://example.com/{id}")
	}

	shellAction, ok := result.Actions["x"]
	if !ok {
		t.Fatal("Actions missing key 'x'")
	}
	if shellAction.Name != "Run tests" {
		t.Errorf("Actions[x].Name = %q, want %q", shellAction.Name, "Run tests")
	}
	if shellAction.Type != "shell" {
		t.Errorf("Actions[x].Type = %q, want %q", shellAction.Type, "shell")
	}
	if shellAction.Command != "go test ./..." {
		t.Errorf("Actions[x].Command = %q, want %q", shellAction.Command, "go test ./...")
	}
}

func TestLoad_NoActions_EmptyMap(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
repo: owner/repo
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Actions) != 0 {
		t.Errorf("Actions count = %d, want 0 for config without actions", len(result.Actions))
	}
}

func TestLoad_ActionMissingName_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  o:
    type: url
    url: "https://example.com"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for action missing name")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "name") {
		t.Errorf("error = %q, want it to contain 'name'", err.Error())
	}
}

func TestLoad_ActionInvalidType_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  o:
    name: Bad action
    type: webhook
    url: "https://example.com"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for invalid action type")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "type") {
		t.Errorf("error = %q, want it to contain 'type'", err.Error())
	}
}

func TestLoad_ActionMissingType_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  o:
    name: No type action
    url: "https://example.com"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for action missing type")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "type") {
		t.Errorf("error = %q, want it to contain 'type'", err.Error())
	}
}

func TestLoad_ActionURLType_MissingURL_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  o:
    name: Open
    type: url
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for url type missing url field")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "url") {
		t.Errorf("error = %q, want it to contain 'url'", err.Error())
	}
}

func TestLoad_ActionShellType_MissingCommand_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  x:
    name: Run tests
    type: shell
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for shell type missing command field")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "command") {
		t.Errorf("error = %q, want it to contain 'command'", err.Error())
	}
}

func TestLoad_ActionKeyMultipleChars_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  open:
    name: Open
    type: url
    url: "https://example.com"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for multi-character action key")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "single character") {
		t.Errorf("error = %q, want it to contain 'single character'", err.Error())
	}
}

func TestLoad_ActionConflictsWithBuiltinKey_ReturnsError(t *testing.T) {
	builtinKeys := []string{"h", "l", "j", "k", "q", "r", "n"}

	for _, key := range builtinKeys {
		t.Run("key_"+key, func(t *testing.T) {
			dir := t.TempDir()
			globalPath := filepath.Join(dir, "global.yml")
			localPath := filepath.Join(dir, "nonexistent.yml")

			yamlContent := `provider: github
actions:
  ` + key + `:
    name: Conflicting action
    type: url
    url: "https://example.com"
`
			if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
				t.Fatalf("failed to write config: %v", err)
			}

			_, err := Load(globalPath, localPath)
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for built-in key %q", key)
			}
			errLower := strings.ToLower(err.Error())
			if !strings.Contains(errLower, "conflict") && !strings.Contains(errLower, "built-in") {
				t.Errorf("error = %q, want it to contain 'conflict' or 'built-in'", err.Error())
			}
		})
	}
}

func TestLoad_ValidURLAction_NoError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  o:
    name: Open in browser
    type: url
    url: "https://example.com/{id}"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	action := result.Actions["o"]
	if action.Type != "url" {
		t.Errorf("Actions[o].Type = %q, want %q", action.Type, "url")
	}
}

func TestLoad_ValidShellAction_NoError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  x:
    name: Run tests
    type: shell
    command: "go test ./..."
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	action := result.Actions["x"]
	if action.Type != "shell" {
		t.Errorf("Actions[x].Type = %q, want %q", action.Type, "shell")
	}
}

func TestLoad_ActionURLType_WithExtraCommand_NoError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  o:
    name: Open and run
    type: url
    url: "https://example.com"
    command: "echo extra"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error for url type with extra command: %v", err)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
}

func TestLoad_LocalActionsOverrideGlobal(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := `provider: github
actions:
  o:
    name: Global open
    type: url
    url: "https://global.example.com"
`
	localYAML := `actions:
  o:
    name: Local open
    type: url
    url: "https://local.example.com"
`
	if err := os.WriteFile(globalPath, []byte(globalYAML), 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}
	if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}

	action := result.Actions["o"]
	if action.Name != "Local open" {
		t.Errorf("Actions[o].Name = %q, want %q (local should override global)", action.Name, "Local open")
	}
	if action.URL != "https://local.example.com" {
		t.Errorf("Actions[o].URL = %q, want %q (local should override global)", action.URL, "https://local.example.com")
	}
}

func TestLoad_GlobalAndLocalActionsMerge(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := `provider: github
actions:
  o:
    name: Open
    type: url
    url: "https://example.com"
`
	localYAML := `actions:
  x:
    name: Execute
    type: shell
    command: "make build"
`
	if err := os.WriteFile(globalPath, []byte(globalYAML), 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}
	if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Actions) != 2 {
		t.Fatalf("Actions count = %d, want 2 (merged global + local)", len(result.Actions))
	}

	if _, ok := result.Actions["o"]; !ok {
		t.Error("Actions missing key 'o' from global config")
	}
	if _, ok := result.Actions["x"]; !ok {
		t.Error("Actions missing key 'x' from local config")
	}
}

func TestLoad_GlobalOnlyActions(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	globalYAML := `provider: github
actions:
  o:
    name: Open
    type: url
    url: "https://example.com"
`
	if err := os.WriteFile(globalPath, []byte(globalYAML), 0644); err != nil {
		t.Fatalf("failed to write global config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	if _, ok := result.Actions["o"]; !ok {
		t.Error("Actions missing key 'o' from global config")
	}
}

func TestLoad_LocalOnlyActions(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "nonexistent-global.yml")
	localPath := filepath.Join(dir, "local.yml")

	localYAML := `provider: github
actions:
  x:
    name: Execute
    type: shell
    command: "make test"
`
	if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	if _, ok := result.Actions["x"]; !ok {
		t.Error("Actions missing key 'x' from local config")
	}
}

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

func TestLoad_ActionConflictsWithConfigKey_ReturnsError(t *testing.T) {
	// "c" is now a built-in key for config popup, so it should conflict.
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
actions:
  c:
    name: Config conflict
    type: url
    url: "https://example.com"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for built-in key \"c\"")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "conflict") && !strings.Contains(errLower, "built-in") {
		t.Errorf("error = %q, want it to contain 'conflict' or 'built-in'", err.Error())
	}
}
