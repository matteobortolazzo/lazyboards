package config

import (
	"strings"
	"testing"
)

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
