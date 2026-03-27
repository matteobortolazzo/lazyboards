package config

import (
	"strings"
	"testing"
)

// --- Action parsing and validation tests ---

func TestLoad_ParsesActionsFromYAML(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Open in browser
    type: url
    url: "https://example.com/{id}"
  X:
    name: Run tests
    type: shell
    command: "go test ./..."
`

	result := mustLoadConfig(t, yamlContent, "")

	if len(result.Actions) != 2 {
		t.Fatalf("Actions count = %d, want 2", len(result.Actions))
	}

	urlAction, ok := result.Actions["B"]
	if !ok {
		t.Fatal("Actions missing key 'B'")
	}
	if urlAction.Name != "Open in browser" {
		t.Errorf("Actions[B].Name = %q, want %q", urlAction.Name, "Open in browser")
	}
	if urlAction.Type != "url" {
		t.Errorf("Actions[B].Type = %q, want %q", urlAction.Type, "url")
	}
	if urlAction.URL != "https://example.com/{id}" {
		t.Errorf("Actions[B].URL = %q, want %q", urlAction.URL, "https://example.com/{id}")
	}

	shellAction, ok := result.Actions["X"]
	if !ok {
		t.Fatal("Actions missing key 'X'")
	}
	if shellAction.Name != "Run tests" {
		t.Errorf("Actions[X].Name = %q, want %q", shellAction.Name, "Run tests")
	}
	if shellAction.Type != "shell" {
		t.Errorf("Actions[X].Type = %q, want %q", shellAction.Type, "shell")
	}
	if shellAction.Command != "go test ./..." {
		t.Errorf("Actions[X].Command = %q, want %q", shellAction.Command, "go test ./...")
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
  B:
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
  B:
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
  B:
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
  B:
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
  X:
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
  OPEN:
    name: Open
    type: url
    url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for multi-character action key")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "uppercase") {
		t.Errorf("error = %q, want it to contain 'uppercase'", err.Error())
	}
}

func TestLoad_ActionLowercaseKey_ReturnsError(t *testing.T) {
	lowercaseKeys := []string{"a", "b", "x", "z"}

	for _, key := range lowercaseKeys {
		t.Run("key_"+key, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  ` + key + `:
    name: Lowercase action
    type: url
    url: "https://example.com"
`

			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for lowercase key %q", key)
			}
			errLower := strings.ToLower(err.Error())
			if !strings.Contains(errLower, "uppercase") {
				t.Errorf("error = %q, want it to contain 'uppercase'", err.Error())
			}
		})
	}
}

func TestLoad_ActionNumberKey_ReturnsError(t *testing.T) {
	numberKeys := []string{"1", "5", "9"}

	for _, key := range numberKeys {
		t.Run("key_"+key, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  ` + key + `:
    name: Number action
    type: url
    url: "https://example.com"
`

			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for number key %q", key)
			}
			errLower := strings.ToLower(err.Error())
			if !strings.Contains(errLower, "uppercase") {
				t.Errorf("error = %q, want it to contain 'uppercase'", err.Error())
			}
		})
	}
}

func TestLoad_ActionSymbolKey_ReturnsError(t *testing.T) {
	symbolKeys := []string{"!", "@", "#"}

	for _, key := range symbolKeys {
		t.Run("key_"+key, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  "` + key + `":
    name: Symbol action
    type: url
    url: "https://example.com"
`

			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for symbol key %q", key)
			}
			errLower := strings.ToLower(err.Error())
			if !strings.Contains(errLower, "uppercase") {
				t.Errorf("error = %q, want it to contain 'uppercase'", err.Error())
			}
		})
	}
}

func TestLoad_ValidURLAction_NoError(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Open in browser
    type: url
    url: "https://example.com/{id}"
`

	result := mustLoadConfig(t, yamlContent, "")

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	action := result.Actions["B"]
	if action.Type != "url" {
		t.Errorf("Actions[B].Type = %q, want %q", action.Type, "url")
	}
}

func TestLoad_ValidShellAction_NoError(t *testing.T) {
	yamlContent := `provider: github
actions:
  X:
    name: Run tests
    type: shell
    command: "go test ./..."
`

	result := mustLoadConfig(t, yamlContent, "")

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	action := result.Actions["X"]
	if action.Type != "shell" {
		t.Errorf("Actions[X].Type = %q, want %q", action.Type, "shell")
	}
}

func TestLoad_ActionURLType_WithExtraCommand_NoError(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
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
  B:
    name: Global open
    type: url
    url: "https://global.example.com"
`
	localYAML := `actions:
  B:
    name: Local open
    type: url
    url: "https://local.example.com"
`

	result := mustLoadConfig(t, globalYAML, localYAML)

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}

	action := result.Actions["B"]
	if action.Name != "Local open" {
		t.Errorf("Actions[B].Name = %q, want %q (local should override global)", action.Name, "Local open")
	}
	if action.URL != "https://local.example.com" {
		t.Errorf("Actions[B].URL = %q, want %q (local should override global)", action.URL, "https://local.example.com")
	}
}

func TestLoad_GlobalAndLocalActionsMerge(t *testing.T) {
	globalYAML := `provider: github
actions:
  B:
    name: Open
    type: url
    url: "https://example.com"
`
	localYAML := `actions:
  X:
    name: Execute
    type: shell
    command: "make build"
`

	result := mustLoadConfig(t, globalYAML, localYAML)

	if len(result.Actions) != 2 {
		t.Fatalf("Actions count = %d, want 2 (merged global + local)", len(result.Actions))
	}

	if _, ok := result.Actions["B"]; !ok {
		t.Error("Actions missing key 'B' from global config")
	}
	if _, ok := result.Actions["X"]; !ok {
		t.Error("Actions missing key 'X' from local config")
	}
}

func TestLoad_GlobalOnlyActions(t *testing.T) {
	globalYAML := `provider: github
actions:
  B:
    name: Open
    type: url
    url: "https://example.com"
`

	result := mustLoadConfig(t, globalYAML, "")

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	if _, ok := result.Actions["B"]; !ok {
		t.Error("Actions missing key 'B' from global config")
	}
}

func TestLoad_LocalOnlyActions(t *testing.T) {
	localYAML := `provider: github
actions:
  X:
    name: Execute
    type: shell
    command: "make test"
`

	result := mustLoadConfig(t, "", localYAML)

	if len(result.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1", len(result.Actions))
	}
	if _, ok := result.Actions["X"]; !ok {
		t.Error("Actions missing key 'X' from local config")
	}
}

func TestLoad_ActionLowercaseConfigKey_ReturnsError(t *testing.T) {
	// "c" is lowercase, so it should be rejected as non-uppercase.
	yamlContent := `provider: github
actions:
  c:
    name: Config conflict
    type: url
    url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for lowercase key \"c\"")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "uppercase") {
		t.Errorf("error = %q, want it to contain 'uppercase'", err.Error())
	}
}

// --- Action scope tests ---

func TestLoad_ActionScopeBoard_ValidNoCardVars(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Open board
    type: url
    scope: board
    url: "https://github.com/{repo_owner}/{repo_name}/issues"
`
	result := mustLoadConfig(t, yamlContent, "")
	action := result.Actions["B"]
	if action.Scope != "board" {
		t.Errorf("Actions[B].Scope = %q, want %q", action.Scope, "board")
	}
}

func TestLoad_ActionScopeCard_ExplicitIsValid(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Open card
    type: url
    scope: card
    url: "https://example.com/{number}"
`
	result := mustLoadConfig(t, yamlContent, "")
	action := result.Actions["B"]
	if action.Scope != "card" {
		t.Errorf("Actions[B].Scope = %q, want %q", action.Scope, "card")
	}
}

func TestLoad_ActionScopeEmpty_DefaultsToCard(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Open card
    type: url
    url: "https://example.com/{number}"
`
	result := mustLoadConfig(t, yamlContent, "")
	action := result.Actions["B"]
	if action.Scope != "card" {
		t.Errorf("Actions[B].Scope = %q, want %q (empty scope should default to card)", action.Scope, "card")
	}
}

func TestLoad_ActionScopeInvalid_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Bad scope
    type: url
    scope: global
    url: "https://example.com"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for invalid scope")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "scope") {
		t.Errorf("error = %q, want it to contain 'scope'", err.Error())
	}
}

func TestLoad_ActionScopeBoard_WithNumber_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Board with number
    type: url
    scope: board
    url: "https://example.com/{number}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for board-scope action using {number}")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "scope") || !strings.Contains(errLower, "number") {
		t.Errorf("error = %q, want it to contain 'scope' and 'number'", err.Error())
	}
}

func TestLoad_ActionScopeBoard_WithTitle_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Board with title
    type: url
    scope: board
    url: "https://example.com/{title}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for board-scope action using {title}")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "scope") || !strings.Contains(errLower, "title") {
		t.Errorf("error = %q, want it to contain 'scope' and 'title'", err.Error())
	}
}

func TestLoad_ActionScopeBoard_WithTags_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Board with tags
    type: url
    scope: board
    url: "https://example.com/?tags={tags}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for board-scope action using {tags}")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "scope") || !strings.Contains(errLower, "tags") {
		t.Errorf("error = %q, want it to contain 'scope' and 'tags'", err.Error())
	}
}

func TestLoad_ActionScopeBoard_WithSession_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Board with session
    type: url
    scope: board
    url: "https://example.com/{session}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for board-scope action using {session}")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "scope") || !strings.Contains(errLower, "session") {
		t.Errorf("error = %q, want it to contain 'scope' and 'session'", err.Error())
	}
}

func TestLoad_ActionScopeBoard_ShellWithCardVar_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  S:
    name: Board shell with session
    type: shell
    scope: board
    command: "deploy {session}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for board-scope shell action using {session}")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "scope") || !strings.Contains(errLower, "session") {
		t.Errorf("error = %q, want it to contain 'scope' and 'session'", err.Error())
	}
}

func TestLoad_ColumnActionScopeBoard_Valid(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: Backlog
    actions:
      B:
        name: View backlog
        type: url
        scope: board
        url: "https://github.com/{repo_owner}/{repo_name}/issues"
  - name: Done
`
	result := mustLoadConfig(t, yamlContent, "")
	colAction := result.Columns[0].Actions["B"]
	if colAction.Scope != "board" {
		t.Errorf("Columns[0].Actions[B].Scope = %q, want %q", colAction.Scope, "board")
	}
}
