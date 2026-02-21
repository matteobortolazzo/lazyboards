package config

import (
	"strings"
	"testing"
)

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

// --- WorkingLabel config tests (#113) ---

func TestLoad_WorkingLabel_ParsesFromYAML(t *testing.T) {
	yamlContent := "provider: github\nworking_label: In Progress\n"

	result := mustLoadConfig(t, yamlContent, "")

	if result.WorkingLabel == nil {
		t.Fatal("WorkingLabel should not be nil when set in config")
	}
	if *result.WorkingLabel != "In Progress" {
		t.Errorf("WorkingLabel = %q, want %q", *result.WorkingLabel, "In Progress")
	}
}

func TestLoad_WorkingLabel_DefaultsToWorkingWhenOmitted(t *testing.T) {
	yamlContent := "provider: github\n"

	result := mustLoadConfig(t, yamlContent, "")

	// When omitted, WorkingLabel should be nil (not set), and
	// WorkingLabelValue() should return the default "Working".
	if result.WorkingLabel != nil {
		t.Errorf("WorkingLabel should be nil when omitted, got %q", *result.WorkingLabel)
	}
	if result.WorkingLabelValue() != "Working" {
		t.Errorf("WorkingLabelValue() = %q, want %q (default)", result.WorkingLabelValue(), "Working")
	}
}

func TestLoad_WorkingLabel_EmptyStringDisablesFeature(t *testing.T) {
	yamlContent := "provider: github\nworking_label: \"\"\n"

	result := mustLoadConfig(t, yamlContent, "")

	// Explicitly set to empty string means "disable the feature".
	// WorkingLabel should be a pointer to "" (not nil).
	if result.WorkingLabel == nil {
		t.Fatal("WorkingLabel should not be nil when explicitly set to empty string")
	}
	if *result.WorkingLabel != "" {
		t.Errorf("WorkingLabel = %q, want empty string", *result.WorkingLabel)
	}
	if result.WorkingLabelValue() != "" {
		t.Errorf("WorkingLabelValue() = %q, want empty string (disabled)", result.WorkingLabelValue())
	}
}

func TestLoad_WorkingLabel_LocalOverridesGlobal(t *testing.T) {
	globalYAML := "provider: github\nworking_label: Active\n"
	localYAML := "working_label: In Progress\n"

	result := mustLoadConfig(t, globalYAML, localYAML)

	if result.WorkingLabel == nil {
		t.Fatal("WorkingLabel should not be nil when set in local config")
	}
	if *result.WorkingLabel != "In Progress" {
		t.Errorf("WorkingLabel = %q, want %q (local should override global)", *result.WorkingLabel, "In Progress")
	}
}

func TestWorkingLabelValue_NilReturnsDefault(t *testing.T) {
	cfg := Config{}
	if cfg.WorkingLabelValue() != "Working" {
		t.Errorf("WorkingLabelValue() = %q, want %q (default when nil)", cfg.WorkingLabelValue(), "Working")
	}
}

func TestWorkingLabelValue_SetReturnsValue(t *testing.T) {
	label := "In Review"
	cfg := Config{WorkingLabel: &label}
	if cfg.WorkingLabelValue() != "In Review" {
		t.Errorf("WorkingLabelValue() = %q, want %q", cfg.WorkingLabelValue(), "In Review")
	}
}

func TestWorkingLabelValue_EmptyStringReturnsEmpty(t *testing.T) {
	label := ""
	cfg := Config{WorkingLabel: &label}
	if cfg.WorkingLabelValue() != "" {
		t.Errorf("WorkingLabelValue() = %q, want empty string", cfg.WorkingLabelValue())
	}
}
