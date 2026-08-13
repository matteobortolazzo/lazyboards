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

// --- Per-column cleanup inheritance tests ---

func TestLoad_ColumnCleanup_OmittedInheritsTopLevelDefault(t *testing.T) {
	yamlContent := `provider: github
cleanup: "tmux kill-window -t ={session}"
columns:
  - name: Implementing
`

	result := mustLoadConfig(t, yamlContent, "")

	col := result.Columns[0]
	if col.Cleanup == nil {
		t.Fatal("Columns[0].Cleanup should not be nil (should inherit top-level default)")
	}
	if *col.Cleanup != "tmux kill-window -t ={session}" {
		t.Errorf("Columns[0].Cleanup = %q, want top-level default", *col.Cleanup)
	}
}

func TestLoad_ColumnCleanup_ExplicitValueOverridesTopLevelDefault(t *testing.T) {
	yamlContent := `provider: github
cleanup: "tmux kill-window -t ={session}"
columns:
  - name: Implementing
    cleanup: "docker stop {session}"
`

	result := mustLoadConfig(t, yamlContent, "")

	col := result.Columns[0]
	if col.Cleanup == nil || *col.Cleanup != "docker stop {session}" {
		t.Errorf("Columns[0].Cleanup = %v, want column-level override to win over top-level default", col.Cleanup)
	}
}

func TestLoad_ColumnCleanup_ExplicitEmptyStringDisablesInheritance(t *testing.T) {
	yamlContent := `provider: github
cleanup: "tmux kill-window -t ={session}"
columns:
  - name: Implementing
    cleanup: ""
`

	result := mustLoadConfig(t, yamlContent, "")

	col := result.Columns[0]
	if col.Cleanup == nil {
		t.Fatal("Columns[0].Cleanup should not be nil (explicit empty string is a set value)")
	}
	if *col.Cleanup != "" {
		t.Errorf("Columns[0].Cleanup = %q, want empty string (explicit disable should not inherit default)", *col.Cleanup)
	}
}

func TestLoad_ColumnCleanup_NoTopLevelDefault_ColumnStaysEmpty(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: Implementing
`

	result := mustLoadConfig(t, yamlContent, "")

	col := result.Columns[0]
	if col.Cleanup == nil || *col.Cleanup != "" {
		t.Errorf("Columns[0].Cleanup = %v, want pointer to empty string (no top-level default configured)", col.Cleanup)
	}
}

func TestLoad_ColumnCleanup_DefaultColumns_InheritTopLevelCleanup(t *testing.T) {
	yamlContent := "provider: github\ncleanup: \"tmux kill-window -t ={session}\"\n"

	result := mustLoadConfig(t, yamlContent, "")

	if len(result.Columns) != len(DefaultColumns) {
		t.Fatalf("Columns count = %d, want %d (defaults)", len(result.Columns), len(DefaultColumns))
	}
	for _, col := range result.Columns {
		if col.Cleanup == nil || *col.Cleanup != "tmux kill-window -t ={session}" {
			t.Errorf("Column %q Cleanup = %v, want top-level default inherited into defaulted columns", col.Name, col.Cleanup)
		}
	}
}

func TestLoad_ColumnCleanup_LocalOmittedInheritsMatchingGlobalColumn(t *testing.T) {
	globalYAML := `provider: github
columns:
  - name: Implementing
    cleanup: "docker stop {session}"
`
	// Local redefines the column (e.g. to reorder it) but doesn't set cleanup.
	localYAML := `columns:
  - name: Implementing
`

	result := mustLoadConfig(t, globalYAML, localYAML)

	col := result.Columns[0]
	if col.Cleanup == nil || *col.Cleanup != "docker stop {session}" {
		t.Errorf("Columns[0].Cleanup = %v, want inherited from matching global column", col.Cleanup)
	}
}

func TestLoad_ColumnCleanup_LocalExplicitOverridesMatchingGlobalColumn(t *testing.T) {
	globalYAML := `provider: github
columns:
  - name: Implementing
    cleanup: "docker stop {session}"
`
	localYAML := `columns:
  - name: Implementing
    cleanup: "tmux kill-window -t ={session}"
`

	result := mustLoadConfig(t, globalYAML, localYAML)

	col := result.Columns[0]
	if col.Cleanup == nil || *col.Cleanup != "tmux kill-window -t ={session}" {
		t.Errorf("Columns[0].Cleanup = %v, want local column value (local should override global column)", col.Cleanup)
	}
}
