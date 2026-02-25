package config

import (
	"strings"
	"testing"
)

func TestLoad_ValidGlobalConfig(t *testing.T) {
	result := mustLoadConfig(t, "provider: github\nrepo: owner/repo\nproject: my-project\nsession_max_length: 50\n", "")

	// Identity fields from global config should be silently ignored.
	if result.Provider != "" {
		t.Errorf("Provider = %q, want empty (global identity fields are ignored)", result.Provider)
	}
	if result.Repo != "" {
		t.Errorf("Repo = %q, want empty (global identity fields are ignored)", result.Repo)
	}
	if result.Project != "" {
		t.Errorf("Project = %q, want empty (global identity fields are ignored)", result.Project)
	}
	// Non-identity fields from global config should still work.
	if result.SessionMaxLength != 50 {
		t.Errorf("SessionMaxLength = %d, want 50 (non-identity global fields preserved)", result.SessionMaxLength)
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

	// Global provider should be stripped (not carried through).
	if result.Provider != "" {
		t.Errorf("Provider = %q, want empty (global identity fields are ignored)", result.Provider)
	}

	// Local repo should be preserved.
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

// --- Global identity fields ignored (#197) ---

func TestLoad_GlobalIdentityFields_IgnoredWhenNoLocal(t *testing.T) {
	globalYAML := "provider: github\nrepo: owner/repo\nproject: my-project\nsession_max_length: 42\n"

	result := mustLoadConfig(t, globalYAML, "")

	// Identity fields (provider, repo, project) from global should be stripped.
	if result.Provider != "" {
		t.Errorf("Provider = %q, want empty (global identity fields are ignored)", result.Provider)
	}
	if result.Repo != "" {
		t.Errorf("Repo = %q, want empty (global identity fields are ignored)", result.Repo)
	}
	if result.Project != "" {
		t.Errorf("Project = %q, want empty (global identity fields are ignored)", result.Project)
	}
	// Non-identity fields from global should be preserved.
	if result.SessionMaxLength != 42 {
		t.Errorf("SessionMaxLength = %d, want 42 (non-identity global fields preserved)", result.SessionMaxLength)
	}
}

func TestLoad_LocalIdentityFields_Preserved(t *testing.T) {
	localYAML := "provider: ado\nrepo: my-org/my-repo\nproject: sprint-board\n"

	result := mustLoadConfig(t, "", localYAML)

	// Identity fields from local config should be preserved.
	if result.Provider != "ado" {
		t.Errorf("Provider = %q, want %q (local identity fields preserved)", result.Provider, "ado")
	}
	if result.Repo != "my-org/my-repo" {
		t.Errorf("Repo = %q, want %q (local identity fields preserved)", result.Repo, "my-org/my-repo")
	}
	if result.Project != "sprint-board" {
		t.Errorf("Project = %q, want %q (local identity fields preserved)", result.Project, "sprint-board")
	}
}

func TestLoad_GlobalIdentityIgnored_LocalIdentityPreserved(t *testing.T) {
	globalYAML := "provider: github\nrepo: global-owner/global-repo\nproject: global-project\nsession_max_length: 64\n"
	localYAML := "provider: ado\nrepo: local-owner/local-repo\nproject: local-project\n"

	result := mustLoadConfig(t, globalYAML, localYAML)

	// Only local identity fields should appear; global identity fields are stripped.
	if result.Provider != "ado" {
		t.Errorf("Provider = %q, want %q (local identity preserved, global ignored)", result.Provider, "ado")
	}
	if result.Repo != "local-owner/local-repo" {
		t.Errorf("Repo = %q, want %q (local identity preserved, global ignored)", result.Repo, "local-owner/local-repo")
	}
	if result.Project != "local-project" {
		t.Errorf("Project = %q, want %q (local identity preserved, global ignored)", result.Project, "local-project")
	}
	// Non-identity global fields should still merge through.
	if result.SessionMaxLength != 64 {
		t.Errorf("SessionMaxLength = %d, want 64 (non-identity global fields preserved)", result.SessionMaxLength)
	}
}

func TestLoad_MissingLocalFile_UsesGlobalOnly(t *testing.T) {
	result := mustLoadConfig(t, "provider: github\nrepo: org/repo\nproject: board-1\n", "")

	// Identity fields from global config should be silently ignored.
	if result.Provider != "" {
		t.Errorf("Provider = %q, want empty (global identity fields are ignored)", result.Provider)
	}
	if result.Repo != "" {
		t.Errorf("Repo = %q, want empty (global identity fields are ignored)", result.Repo)
	}
	if result.Project != "" {
		t.Errorf("Project = %q, want empty (global identity fields are ignored)", result.Project)
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
	// Identity fields in global config should also be ignored.
	yamlContent := "provider: github\nrepo: owner/repo\ntheme: dark\n"

	result := mustLoadConfig(t, yamlContent, "")

	if result.Provider != "" {
		t.Errorf("Provider = %q, want empty (global identity fields are ignored)", result.Provider)
	}
	if result.Repo != "" {
		t.Errorf("Repo = %q, want empty (global identity fields are ignored)", result.Repo)
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

// --- ActionRefreshDelay config tests (#119) ---

func TestLoad_ActionRefreshDelay_ParsesFromYAML(t *testing.T) {
	yamlContent := "provider: github\naction_refresh_delay: 10\n"

	result := mustLoadConfig(t, yamlContent, "")

	if result.ActionRefreshDelay == nil {
		t.Fatal("ActionRefreshDelay should not be nil when set in config")
	}
	if *result.ActionRefreshDelay != 10 {
		t.Errorf("ActionRefreshDelay = %d, want 10", *result.ActionRefreshDelay)
	}
}

func TestLoad_ActionRefreshDelay_DefaultsWhenOmitted(t *testing.T) {
	yamlContent := "provider: github\n"

	result := mustLoadConfig(t, yamlContent, "")

	// When omitted, ActionRefreshDelay should be nil (not set), and
	// ActionRefreshDelayValue() should return the default.
	if result.ActionRefreshDelay != nil {
		t.Errorf("ActionRefreshDelay should be nil when omitted, got %d", *result.ActionRefreshDelay)
	}
	if result.ActionRefreshDelayValue() != DefaultActionRefreshDelay {
		t.Errorf("ActionRefreshDelayValue() = %d, want %d (default)", result.ActionRefreshDelayValue(), DefaultActionRefreshDelay)
	}
}

func TestLoad_ActionRefreshDelay_LocalOverridesGlobal(t *testing.T) {
	globalYAML := "provider: github\naction_refresh_delay: 10\n"
	localYAML := "action_refresh_delay: 3\n"

	result := mustLoadConfig(t, globalYAML, localYAML)

	if result.ActionRefreshDelay == nil {
		t.Fatal("ActionRefreshDelay should not be nil when set in local config")
	}
	if *result.ActionRefreshDelay != 3 {
		t.Errorf("ActionRefreshDelay = %d, want 3 (local should override global)", *result.ActionRefreshDelay)
	}
}

func TestLoad_ActionRefreshDelay_NegativeBecomesZero(t *testing.T) {
	yamlContent := "provider: github\naction_refresh_delay: -5\n"

	result := mustLoadConfig(t, yamlContent, "")

	// Negative value stays as-is in the pointer, but ActionRefreshDelayValue() returns 0.
	if result.ActionRefreshDelay == nil {
		t.Fatal("ActionRefreshDelay should not be nil when set in config")
	}
	if *result.ActionRefreshDelay != -5 {
		t.Errorf("ActionRefreshDelay = %d, want -5 (raw value preserved)", *result.ActionRefreshDelay)
	}
	if result.ActionRefreshDelayValue() != 0 {
		t.Errorf("ActionRefreshDelayValue() = %d, want 0 (negative should become 0)", result.ActionRefreshDelayValue())
	}
}

func TestLoad_ActionRefreshDelay_ExplicitZeroDisablesFeature(t *testing.T) {
	yamlContent := "provider: github\naction_refresh_delay: 0\n"

	result := mustLoadConfig(t, yamlContent, "")

	// Explicitly set to 0 means "disable auto-refresh after shell actions".
	// ActionRefreshDelay should be a pointer to 0 (not nil).
	if result.ActionRefreshDelay == nil {
		t.Fatal("ActionRefreshDelay should not be nil when explicitly set to 0")
	}
	if *result.ActionRefreshDelay != 0 {
		t.Errorf("ActionRefreshDelay = %d, want 0", *result.ActionRefreshDelay)
	}
	if result.ActionRefreshDelayValue() != 0 {
		t.Errorf("ActionRefreshDelayValue() = %d, want 0 (disabled)", result.ActionRefreshDelayValue())
	}
}

func TestActionRefreshDelayValue_NilReturnsDefault(t *testing.T) {
	cfg := Config{}
	if cfg.ActionRefreshDelayValue() != DefaultActionRefreshDelay {
		t.Errorf("ActionRefreshDelayValue() = %d, want %d (default when nil)", cfg.ActionRefreshDelayValue(), DefaultActionRefreshDelay)
	}
}

func TestActionRefreshDelayValue_SetReturnsValue(t *testing.T) {
	delay := 10
	cfg := Config{ActionRefreshDelay: &delay}
	if cfg.ActionRefreshDelayValue() != 10 {
		t.Errorf("ActionRefreshDelayValue() = %d, want 10", cfg.ActionRefreshDelayValue())
	}
}

func TestActionRefreshDelayValue_ZeroReturnsZero(t *testing.T) {
	delay := 0
	cfg := Config{ActionRefreshDelay: &delay}
	if cfg.ActionRefreshDelayValue() != 0 {
		t.Errorf("ActionRefreshDelayValue() = %d, want 0", cfg.ActionRefreshDelayValue())
	}
}

func TestActionRefreshDelayValue_NegativeReturnsZero(t *testing.T) {
	delay := -3
	cfg := Config{ActionRefreshDelay: &delay}
	if cfg.ActionRefreshDelayValue() != 0 {
		t.Errorf("ActionRefreshDelayValue() = %d, want 0 (negative clamped)", cfg.ActionRefreshDelayValue())
	}
}

// --- Mouse config tests (#192) ---

func TestMouseValue_NilDefaultsToTrue(t *testing.T) {
	cfg := Config{}
	if !cfg.MouseValue() {
		t.Error("MouseValue() = false when Mouse is nil, want true (mouse enabled by default)")
	}
}

func TestMouseValue_ExplicitFalseReturnsFalse(t *testing.T) {
	enabled := false
	cfg := Config{Mouse: &enabled}
	if cfg.MouseValue() {
		t.Error("MouseValue() = true when Mouse is explicitly false, want false")
	}
}

func TestMouseValue_ExplicitTrueReturnsTrue(t *testing.T) {
	enabled := true
	cfg := Config{Mouse: &enabled}
	if !cfg.MouseValue() {
		t.Error("MouseValue() = false when Mouse is explicitly true, want true")
	}
}
