package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func loadConfigFromStrings(t *testing.T, globalYAML, localYAML string) (Config, error) {
	t.Helper()
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	if globalYAML != "" {
		if err := os.WriteFile(globalPath, []byte(globalYAML), 0644); err != nil {
			t.Fatalf("failed to write global config: %v", err)
		}
	} else {
		globalPath = filepath.Join(dir, "nonexistent-global.yml")
	}

	if localYAML != "" {
		if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
			t.Fatalf("failed to write local config: %v", err)
		}
	} else {
		localPath = filepath.Join(dir, "nonexistent-local.yml")
	}

	return Load(globalPath, localPath)
}

func mustLoadConfig(t *testing.T, globalYAML, localYAML string) Config {
	t.Helper()
	cfg, err := loadConfigFromStrings(t, globalYAML, localYAML)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	return cfg
}

func TestLoad_ValidGlobalConfig(t *testing.T) {
	result := mustLoadConfig(t, "provider: github\nrepo: owner/repo\nproject: my-project\n", "")

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
	result := mustLoadConfig(t, "", "")

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
	globalYAML := "provider: github\nrepo: owner/repo\nproject: my-project\n"
	localYAML := "repo: other-owner/other-repo\n"

	result := mustLoadConfig(t, globalYAML, localYAML)

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
	globalYAML := "provider: github\n"
	localYAML := "provider: ado\n"

	result := mustLoadConfig(t, globalYAML, localYAML)

	if result.Provider != "ado" {
		t.Errorf("Provider = %q, want %q (local should override global)", result.Provider, "ado")
	}
}

func TestLoad_MissingLocalFile_UsesGlobalOnly(t *testing.T) {
	result := mustLoadConfig(t, "provider: github\nrepo: org/repo\nproject: board-1\n", "")

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
	result := mustLoadConfig(t, "", "")

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
	malformed := "provider: github\n  bad indent: [this is: not valid\n"

	_, err := loadConfigFromStrings(t, malformed, "")
	if err == nil {
		t.Error("Load() returned nil error for invalid YAML, want non-nil error")
	}
}

func TestLoad_InvalidLocalYAML_ReturnsError(t *testing.T) {
	globalYAML := "provider: github\n"
	malformed := "columns: [this is: not valid\n  bad indent\n"

	_, err := loadConfigFromStrings(t, globalYAML, malformed)
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
	// A config file with unknown fields (e.g., "theme") should load successfully.
	yamlContent := "provider: github\nrepo: owner/repo\ntheme: dark\n"

	result := mustLoadConfig(t, yamlContent, "")

	if result.Provider != "github" {
		t.Errorf("Provider = %q, want %q", result.Provider, "github")
	}
	if result.Repo != "owner/repo" {
		t.Errorf("Repo = %q, want %q", result.Repo, "owner/repo")
	}
}

// --- Column parsing and validation tests (ColumnConfig object format) ---

func TestLoad_ParsesColumnConfigFromYAML(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: Todo
  - name: Doing
  - name: Done
`

	result := mustLoadConfig(t, yamlContent, "")

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

	result := mustLoadConfig(t, yamlContent, "")

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
	yamlContent := `provider: github
columns:
  - name: Implementing
    actions:
      b:
        name: Bad action
        type: webhook
        command: "echo hello"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for invalid column action type")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "type") {
		t.Errorf("error = %q, want it to contain 'type'", err.Error())
	}
}

func TestLoad_ColumnActionConflictsWithBuiltinKey_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: Implementing
    actions:
      j:
        name: Conflicting action
        type: url
        url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for column action with built-in key")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "conflict") && !strings.Contains(errLower, "built-in") {
		t.Errorf("error = %q, want it to contain 'conflict' or 'built-in'", err.Error())
	}
}

func TestLoad_ColumnActionValidKey_AcceptsGlobalActionOverlap(t *testing.T) {
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

	result := mustLoadConfig(t, yamlContent, "")

	// Both global and column actions should be parsed successfully.
	if len(result.Actions) != 1 {
		t.Errorf("global Actions count = %d, want 1", len(result.Actions))
	}
	if len(result.Columns[0].Actions) != 1 {
		t.Errorf("Columns[0].Actions count = %d, want 1", len(result.Columns[0].Actions))
	}
}

func TestLoad_ColumnWithoutActions_ParsesCorrectly(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: Backlog
  - name: In Progress
  - name: Done
`

	result := mustLoadConfig(t, yamlContent, "")

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
	yamlContent := "provider: github\nrepo: owner/repo\n"

	result := mustLoadConfig(t, yamlContent, "")

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
	yamlContent := "provider: github\ncolumns: []\n"

	result := mustLoadConfig(t, yamlContent, "")

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
	// Case-insensitive duplicate: "Todo" and "todo"
	yamlContent := `provider: github
columns:
  - name: Todo
  - name: Doing
  - name: todo
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for duplicate columns")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "duplicate") {
		t.Errorf("error = %q, want it to contain 'duplicate'", err.Error())
	}
}

func TestLoad_SingleColumn_Valid(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: Backlog
`

	result := mustLoadConfig(t, yamlContent, "")

	if len(result.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(result.Columns))
	}
	if result.Columns[0].Name != "Backlog" {
		t.Errorf("Columns[0].Name = %q, want %q", result.Columns[0].Name, "Backlog")
	}
}

func TestLoad_LocalColumnsOverrideGlobal(t *testing.T) {
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

	result := mustLoadConfig(t, globalYAML, localYAML)

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
	yamlContent := `provider: github
columns:
  - name: Todo
  - name: "  "
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for whitespace-only column name")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "empty") && !strings.Contains(errLower, "whitespace") {
		t.Errorf("error = %q, want it to contain 'empty' or 'whitespace'", err.Error())
	}
}

func TestLoad_ColumnNameWithWhitespace_Trimmed(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: " Todo "
  - name: "Doing "
`

	result := mustLoadConfig(t, yamlContent, "")

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

	result := mustLoadConfig(t, yamlContent, "")

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
	yamlContent := `provider: github
repo: owner/repo
`

	result := mustLoadConfig(t, yamlContent, "")

	if len(result.Actions) != 0 {
		t.Errorf("Actions count = %d, want 0 for config without actions", len(result.Actions))
	}
}

func TestLoad_ActionMissingName_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  o:
    type: url
    url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for action missing name")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "name") {
		t.Errorf("error = %q, want it to contain 'name'", err.Error())
	}
}

func TestLoad_ActionInvalidType_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  o:
    name: Bad action
    type: webhook
    url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for invalid action type")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "type") {
		t.Errorf("error = %q, want it to contain 'type'", err.Error())
	}
}

func TestLoad_ActionMissingType_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  o:
    name: No type action
    url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for action missing type")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "type") {
		t.Errorf("error = %q, want it to contain 'type'", err.Error())
	}
}

func TestLoad_ActionURLType_MissingURL_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  o:
    name: Open
    type: url
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for url type missing url field")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "url") {
		t.Errorf("error = %q, want it to contain 'url'", err.Error())
	}
}

func TestLoad_ActionShellType_MissingCommand_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  x:
    name: Run tests
    type: shell
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for shell type missing command field")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "command") {
		t.Errorf("error = %q, want it to contain 'command'", err.Error())
	}
}

func TestLoad_ActionKeyMultipleChars_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  open:
    name: Open
    type: url
    url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for multi-character action key")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "single character") {
		t.Errorf("error = %q, want it to contain 'single character'", err.Error())
	}
}

func TestLoad_ActionConflictsWithBuiltinKey_ReturnsError(t *testing.T) {
	builtinKeys := []string{"j", "k", "q", "r", "n", "p"}

	for _, key := range builtinKeys {
		t.Run("key_"+key, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  ` + key + `:
    name: Conflicting action
    type: url
    url: "https://example.com"
`

			_, err := loadConfigFromStrings(t, yamlContent, "")
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
	yamlContent := `provider: github
actions:
  o:
    name: Open in browser
    type: url
    url: "https://example.com/{id}"
`

	result := mustLoadConfig(t, yamlContent, "")

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	action := result.Actions["o"]
	if action.Type != "url" {
		t.Errorf("Actions[o].Type = %q, want %q", action.Type, "url")
	}
}

func TestLoad_ValidShellAction_NoError(t *testing.T) {
	yamlContent := `provider: github
actions:
  x:
    name: Run tests
    type: shell
    command: "go test ./..."
`

	result := mustLoadConfig(t, yamlContent, "")

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	action := result.Actions["x"]
	if action.Type != "shell" {
		t.Errorf("Actions[x].Type = %q, want %q", action.Type, "shell")
	}
}

func TestLoad_ActionURLType_WithExtraCommand_NoError(t *testing.T) {
	yamlContent := `provider: github
actions:
  o:
    name: Open and run
    type: url
    url: "https://example.com"
    command: "echo extra"
`

	result := mustLoadConfig(t, yamlContent, "")

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
}

func TestLoad_LocalActionsOverrideGlobal(t *testing.T) {
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

	result := mustLoadConfig(t, globalYAML, localYAML)

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

	result := mustLoadConfig(t, globalYAML, localYAML)

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
	globalYAML := `provider: github
actions:
  o:
    name: Open
    type: url
    url: "https://example.com"
`

	result := mustLoadConfig(t, globalYAML, "")

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	if _, ok := result.Actions["o"]; !ok {
		t.Error("Actions missing key 'o' from global config")
	}
}

func TestLoad_LocalOnlyActions(t *testing.T) {
	localYAML := `provider: github
actions:
  x:
    name: Execute
    type: shell
    command: "make test"
`

	result := mustLoadConfig(t, "", localYAML)

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

func TestLoad_ActionConflictsWithConfigKey_ReturnsError(t *testing.T) {
	// "c" is now a built-in key for config popup, so it should conflict.
	yamlContent := `provider: github
actions:
  c:
    name: Config conflict
    type: url
    url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
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

	result := mustLoadConfig(t, globalYAML, localYAML)

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

	result := mustLoadConfig(t, globalYAML, localYAML)

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

	result := mustLoadConfig(t, globalYAML, localYAML)

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

	result := mustLoadConfig(t, globalYAML, localYAML)

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

	result := mustLoadConfig(t, globalYAML, localYAML)

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

	result := mustLoadConfig(t, globalYAML, localYAML)

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

	result := mustLoadConfig(t, globalYAML, localYAML)

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

// --- SessionMaxLength config tests ---

func TestLoad_SessionMaxLength_ParsesFromYAML(t *testing.T) {
	yamlContent := "provider: github\nsession_max_length: 50\n"

	result := mustLoadConfig(t, yamlContent, "")

	if result.SessionMaxLength != 50 {
		t.Errorf("SessionMaxLength = %d, want 50", result.SessionMaxLength)
	}
}

func TestLoad_SessionMaxLength_DefaultsWhenOmitted(t *testing.T) {
	yamlContent := "provider: github\n"

	result := mustLoadConfig(t, yamlContent, "")

	if result.SessionMaxLength != DefaultSessionMaxLength {
		t.Errorf("SessionMaxLength = %d, want %d (default)", result.SessionMaxLength, DefaultSessionMaxLength)
	}
}

func TestLoad_SessionMaxLength_LocalOverridesGlobal(t *testing.T) {
	globalYAML := "provider: github\nsession_max_length: 40\n"
	localYAML := "session_max_length: 20\n"

	result := mustLoadConfig(t, globalYAML, localYAML)

	if result.SessionMaxLength != 20 {
		t.Errorf("SessionMaxLength = %d, want 20 (local should override global)", result.SessionMaxLength)
	}
}
