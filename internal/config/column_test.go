package config

import (
	"strings"
	"testing"
)

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
