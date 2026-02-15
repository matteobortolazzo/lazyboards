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
	// Columns should fall back to defaults when config files are missing.
	if len(result.Columns) != len(DefaultColumns) {
		t.Fatalf("Columns count = %d, want %d (defaults)", len(result.Columns), len(DefaultColumns))
	}
	for i, col := range result.Columns {
		if col.Name != DefaultColumns[i].Name {
			t.Errorf("Columns[%d].Name = %q, want %q", i, col.Name, DefaultColumns[i].Name)
		}
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
	// Columns should fall back to defaults when both config files are missing.
	if len(result.Columns) != len(DefaultColumns) {
		t.Fatalf("Columns count = %d, want %d (defaults)", len(result.Columns), len(DefaultColumns))
	}
	for i, col := range result.Columns {
		if col.Name != DefaultColumns[i].Name {
			t.Errorf("Columns[%d].Name = %q, want %q", i, col.Name, DefaultColumns[i].Name)
		}
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

	// A config file with unknown fields (e.g., "theme") should load successfully.
	yamlContent := "provider: github\nrepo: owner/repo\ntheme: dark\n"
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

// --- Column parsing and validation tests (ColumnConfig object format) ---

func TestLoad_ParsesColumnConfigFromYAML(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
columns:
  - name: Todo
  - name: Doing
  - name: Done
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	expectedNames := []string{"Todo", "Doing", "Done"}
	if len(result.Columns) != len(expectedNames) {
		t.Fatalf("Columns count = %d, want %d", len(result.Columns), len(expectedNames))
	}
	for i, col := range result.Columns {
		if col.Name != expectedNames[i] {
			t.Errorf("Columns[%d].Name = %q, want %q", i, col.Name, expectedNames[i])
		}
	}
}

func TestLoad_ParsesColumnConfigWithActions(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
columns:
  - name: New
  - name: Implementing
    actions:
      b:
        name: Create branch
        type: shell
        command: "git checkout -b {title}"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Columns) != 2 {
		t.Fatalf("Columns count = %d, want 2", len(result.Columns))
	}

	// First column: no actions.
	if result.Columns[0].Name != "New" {
		t.Errorf("Columns[0].Name = %q, want %q", result.Columns[0].Name, "New")
	}
	if len(result.Columns[0].Actions) != 0 {
		t.Errorf("Columns[0].Actions count = %d, want 0", len(result.Columns[0].Actions))
	}

	// Second column: has actions.
	if result.Columns[1].Name != "Implementing" {
		t.Errorf("Columns[1].Name = %q, want %q", result.Columns[1].Name, "Implementing")
	}
	if len(result.Columns[1].Actions) != 1 {
		t.Fatalf("Columns[1].Actions count = %d, want 1", len(result.Columns[1].Actions))
	}
	act, ok := result.Columns[1].Actions["b"]
	if !ok {
		t.Fatal("Columns[1].Actions missing key 'b'")
	}
	if act.Name != "Create branch" {
		t.Errorf("Columns[1].Actions[b].Name = %q, want %q", act.Name, "Create branch")
	}
	if act.Type != "shell" {
		t.Errorf("Columns[1].Actions[b].Type = %q, want %q", act.Type, "shell")
	}
	if act.Command != "git checkout -b {title}" {
		t.Errorf("Columns[1].Actions[b].Command = %q, want %q", act.Command, "git checkout -b {title}")
	}
}

func TestLoad_ColumnActionInvalidType_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
columns:
  - name: Implementing
    actions:
      b:
        name: Bad action
        type: webhook
        command: "echo hello"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for invalid column action type")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "type") {
		t.Errorf("error = %q, want it to contain 'type'", err.Error())
	}
}

func TestLoad_ColumnActionConflictsWithBuiltinKey_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
columns:
  - name: Implementing
    actions:
      j:
        name: Conflicting action
        type: url
        url: "https://example.com"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for column action with built-in key")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "conflict") && !strings.Contains(errLower, "built-in") {
		t.Errorf("error = %q, want it to contain 'conflict' or 'built-in'", err.Error())
	}
}

func TestLoad_ColumnActionValidKey_AcceptsGlobalActionOverlap(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	// Column action key "o" overlaps with a global action key "o".
	// This should be allowed -- column overrides at runtime (PR 2).
	yamlContent := `provider: github
actions:
  o:
    name: Global open
    type: url
    url: "https://global.example.com"
columns:
  - name: Implementing
    actions:
      o:
        name: Column open
        type: url
        url: "https://column.example.com"
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	// Both global and column actions should be parsed successfully.
	if len(result.Actions) != 1 {
		t.Errorf("global Actions count = %d, want 1", len(result.Actions))
	}
	if len(result.Columns[0].Actions) != 1 {
		t.Errorf("Columns[0].Actions count = %d, want 1", len(result.Columns[0].Actions))
	}
}

func TestLoad_ColumnWithoutActions_ParsesCorrectly(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
columns:
  - name: Backlog
  - name: In Progress
  - name: Done
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Columns) != 3 {
		t.Fatalf("Columns count = %d, want 3", len(result.Columns))
	}
	for _, col := range result.Columns {
		if len(col.Actions) != 0 {
			t.Errorf("Column %q has %d actions, want 0", col.Name, len(col.Actions))
		}
	}
}

func TestLoad_OmittedColumns_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := "provider: github\nrepo: owner/repo\n"
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Columns) != len(DefaultColumns) {
		t.Fatalf("Columns count = %d, want %d (defaults)", len(result.Columns), len(DefaultColumns))
	}
	for i, col := range result.Columns {
		if col.Name != DefaultColumns[i].Name {
			t.Errorf("Columns[%d].Name = %q, want %q", i, col.Name, DefaultColumns[i].Name)
		}
	}
}

func TestLoad_EmptyColumnsList_UsesDefaults(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := "provider: github\ncolumns: []\n"
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Columns) != len(DefaultColumns) {
		t.Fatalf("Columns count = %d, want %d (defaults)", len(result.Columns), len(DefaultColumns))
	}
	for i, col := range result.Columns {
		if col.Name != DefaultColumns[i].Name {
			t.Errorf("Columns[%d].Name = %q, want %q", i, col.Name, DefaultColumns[i].Name)
		}
	}
}

func TestLoad_DuplicateColumns_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	// Case-insensitive duplicate: "Todo" and "todo"
	yamlContent := `provider: github
columns:
  - name: Todo
  - name: Doing
  - name: todo
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for duplicate columns")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "duplicate") {
		t.Errorf("error = %q, want it to contain 'duplicate'", err.Error())
	}
}

func TestLoad_SingleColumn_Valid(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
columns:
  - name: Backlog
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}
	if result.Columns[0].Name != "Backlog" {
		t.Errorf("Columns[0].Name = %q, want %q", result.Columns[0].Name, "Backlog")
	}
}

func TestLoad_LocalColumnsOverrideGlobal(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := `provider: github
columns:
  - name: Global1
  - name: Global2
  - name: Global3
`
	localYAML := `columns:
  - name: Local1
  - name: Local2
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

	// Local columns should completely replace global columns.
	expectedNames := []string{"Local1", "Local2"}
	if len(result.Columns) != len(expectedNames) {
		t.Fatalf("Columns count = %d, want %d (local should replace global)", len(result.Columns), len(expectedNames))
	}
	for i, col := range result.Columns {
		if col.Name != expectedNames[i] {
			t.Errorf("Columns[%d].Name = %q, want %q", i, col.Name, expectedNames[i])
		}
	}
}

func TestLoad_WhitespaceOnlyColumnName_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
columns:
  - name: Todo
  - name: "  "
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := Load(globalPath, localPath)
	if err == nil {
		t.Fatal("Load() returned nil error, want error for whitespace-only column name")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "empty") && !strings.Contains(errLower, "whitespace") {
		t.Errorf("error = %q, want it to contain 'empty' or 'whitespace'", err.Error())
	}
}

func TestLoad_ColumnNameWithWhitespace_Trimmed(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "nonexistent.yml")

	yamlContent := `provider: github
columns:
  - name: " Todo "
  - name: "Doing "
`
	if err := os.WriteFile(globalPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	result, err := Load(globalPath, localPath)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if result.Columns[0].Name != "Todo" {
		t.Errorf("Columns[0].Name = %q, want %q (should be trimmed)", result.Columns[0].Name, "Todo")
	}
	if result.Columns[1].Name != "Doing" {
		t.Errorf("Columns[1].Name = %q, want %q (should be trimmed)", result.Columns[1].Name, "Doing")
	}
}

// --- ColumnNames() helper ---

func TestColumnNames_ExtractsNamesFromColumnConfigs(t *testing.T) {
	cfg := Config{
		Columns: []ColumnConfig{
			{Name: "New"},
			{Name: "Implementing"},
			{Name: "Done"},
		},
	}

	names := cfg.ColumnNames()
	expectedNames := []string{"New", "Implementing", "Done"}

	if len(names) != len(expectedNames) {
		t.Fatalf("ColumnNames() returned %d names, want %d", len(names), len(expectedNames))
	}
	for i, name := range names {
		if name != expectedNames[i] {
			t.Errorf("ColumnNames()[%d] = %q, want %q", i, name, expectedNames[i])
		}
	}
}

func TestColumnNames_EmptyColumns_ReturnsEmptySlice(t *testing.T) {
	cfg := Config{}

	names := cfg.ColumnNames()
	if len(names) != 0 {
		t.Errorf("ColumnNames() returned %d names, want 0 for empty columns", len(names))
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
	builtinKeys := []string{"j", "k", "q", "r", "n"}

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

// --- Per-column action merging tests (#71) ---

func TestLoad_ColumnActionsMerge_LocalOverridesGlobal(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := `provider: github
columns:
  - name: Implementing
    actions:
      b:
        name: Create branch
        type: shell
        command: "git checkout -b {title}"
`
	localYAML := `columns:
  - name: Implementing
    actions:
      b:
        name: Local branch
        type: shell
        command: "git switch -c {title}"
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

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}

	col := result.Columns[0]
	if len(col.Actions) != 1 {
		t.Fatalf("Implementing actions count = %d, want 1", len(col.Actions))
	}
	act, ok := col.Actions["b"]
	if !ok {
		t.Fatal("Implementing actions missing key 'b'")
	}
	// Local "b" should win over global "b".
	if act.Name != "Local branch" {
		t.Errorf("Actions[b].Name = %q, want %q (local should override global)", act.Name, "Local branch")
	}
	if act.Command != "git switch -c {title}" {
		t.Errorf("Actions[b].Command = %q, want %q (local should override global)", act.Command, "git switch -c {title}")
	}
}

func TestLoad_ColumnActionsMerge_GlobalOnlyKeysPreserved(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := `provider: github
columns:
  - name: Implementing
    actions:
      b:
        name: Create branch
        type: shell
        command: "git checkout -b {title}"
      d:
        name: Delete branch
        type: shell
        command: "git branch -d {title}"
`
	localYAML := `columns:
  - name: Implementing
    actions:
      b:
        name: Local branch
        type: shell
        command: "git switch -c {title}"
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

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}

	col := result.Columns[0]
	if len(col.Actions) != 2 {
		t.Fatalf("Implementing actions count = %d, want 2 (local 'b' + global 'd')", len(col.Actions))
	}

	// "b" from local should win.
	actB, ok := col.Actions["b"]
	if !ok {
		t.Fatal("Implementing actions missing key 'b'")
	}
	if actB.Name != "Local branch" {
		t.Errorf("Actions[b].Name = %q, want %q (local should override)", actB.Name, "Local branch")
	}

	// "d" from global should be preserved.
	actD, ok := col.Actions["d"]
	if !ok {
		t.Fatal("Implementing actions missing key 'd' (global-only key should be preserved)")
	}
	if actD.Name != "Delete branch" {
		t.Errorf("Actions[d].Name = %q, want %q (global-only key should be preserved)", actD.Name, "Delete branch")
	}
}

func TestLoad_ColumnActionsMerge_NilActionsInheritsGlobal(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := `provider: github
columns:
  - name: Implementing
    actions:
      b:
        name: Create branch
        type: shell
        command: "git checkout -b {title}"
      d:
        name: Delete branch
        type: shell
        command: "git branch -d {title}"
`
	// Local column "Implementing" omits the actions field entirely (nil).
	localYAML := `columns:
  - name: Implementing
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

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}

	col := result.Columns[0]
	// Nil actions in local should inherit all global actions.
	if len(col.Actions) != 2 {
		t.Fatalf("Implementing actions count = %d, want 2 (should inherit both global actions)", len(col.Actions))
	}
	if _, ok := col.Actions["b"]; !ok {
		t.Error("Implementing actions missing key 'b' (should be inherited from global)")
	}
	if _, ok := col.Actions["d"]; !ok {
		t.Error("Implementing actions missing key 'd' (should be inherited from global)")
	}
}

func TestLoad_ColumnActionsMerge_EmptyActionsGetsNone(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := `provider: github
columns:
  - name: Implementing
    actions:
      b:
        name: Create branch
        type: shell
        command: "git checkout -b {title}"
`
	// Local column "Implementing" has explicit empty actions map (not nil).
	localYAML := `columns:
  - name: Implementing
    actions: {}
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

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}

	col := result.Columns[0]
	// Empty map (not nil) means the local explicitly cleared actions.
	if len(col.Actions) != 0 {
		t.Errorf("Implementing actions count = %d, want 0 (explicit empty map should not inherit global)", len(col.Actions))
	}
}

func TestLoad_ColumnActionsMerge_NoGlobalMatch(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := `provider: github
columns:
  - name: Backlog
    actions:
      b:
        name: Global backlog action
        type: shell
        command: "echo backlog"
`
	// Local has a different column name; no match with global "Backlog".
	localYAML := `columns:
  - name: Custom
    actions:
      x:
        name: Custom action
        type: shell
        command: "echo custom"
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

	// Local columns replace global entirely, so only "Custom" should exist.
	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}

	col := result.Columns[0]
	if col.Name != "Custom" {
		t.Errorf("Columns[0].Name = %q, want %q", col.Name, "Custom")
	}
	// "Custom" keeps only its own action, no merge from global "Backlog".
	if len(col.Actions) != 1 {
		t.Fatalf("Custom actions count = %d, want 1", len(col.Actions))
	}
	if _, ok := col.Actions["x"]; !ok {
		t.Error("Custom actions missing key 'x'")
	}
	if _, ok := col.Actions["b"]; ok {
		t.Error("Custom actions should not have key 'b' from unmatched global column")
	}
}

func TestLoad_ColumnActionsMerge_CaseInsensitiveMatch(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	// Global uses lowercase "implementing".
	globalYAML := `provider: github
columns:
  - name: implementing
    actions:
      b:
        name: Global branch
        type: shell
        command: "git checkout -b {title}"
      d:
        name: Delete branch
        type: shell
        command: "git branch -d {title}"
`
	// Local uses title case "Implementing".
	localYAML := `columns:
  - name: Implementing
    actions:
      b:
        name: Local branch
        type: shell
        command: "git switch -c {title}"
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

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}

	col := result.Columns[0]
	// Should match case-insensitively and merge actions.
	if len(col.Actions) != 2 {
		t.Fatalf("Implementing actions count = %d, want 2 (local 'b' + global 'd' via case-insensitive match)", len(col.Actions))
	}

	actB, ok := col.Actions["b"]
	if !ok {
		t.Fatal("Implementing actions missing key 'b'")
	}
	if actB.Name != "Local branch" {
		t.Errorf("Actions[b].Name = %q, want %q (local should override)", actB.Name, "Local branch")
	}

	actD, ok := col.Actions["d"]
	if !ok {
		t.Fatal("Implementing actions missing key 'd' (global-only key should be preserved via case-insensitive match)")
	}
	if actD.Name != "Delete branch" {
		t.Errorf("Actions[d].Name = %q, want %q (global-only key should be preserved)", actD.Name, "Delete branch")
	}
}

func TestLoad_ColumnActionsMerge_GlobalColumnsWithActionsNoLocal(t *testing.T) {
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	globalYAML := `provider: github
columns:
  - name: Implementing
    actions:
      b:
        name: Create branch
        type: shell
        command: "git checkout -b {title}"
  - name: Done
`
	// Local has no columns field at all.
	localYAML := `repo: local-owner/local-repo
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

	// Global columns should be preserved when local has no columns.
	if len(result.Columns) != 2 {
		t.Fatalf("Columns count = %d, want 2 (global columns should be preserved)", len(result.Columns))
	}

	if result.Columns[0].Name != "Implementing" {
		t.Errorf("Columns[0].Name = %q, want %q", result.Columns[0].Name, "Implementing")
	}

	// Global column actions should be preserved as-is.
	if len(result.Columns[0].Actions) != 1 {
		t.Fatalf("Implementing actions count = %d, want 1 (global actions should be preserved)", len(result.Columns[0].Actions))
	}
	act, ok := result.Columns[0].Actions["b"]
	if !ok {
		t.Fatal("Implementing actions missing key 'b' (global actions should be preserved)")
	}
	if act.Name != "Create branch" {
		t.Errorf("Actions[b].Name = %q, want %q (global actions should be preserved)", act.Name, "Create branch")
	}

	if result.Columns[1].Name != "Done" {
		t.Errorf("Columns[1].Name = %q, want %q", result.Columns[1].Name, "Done")
	}
}
