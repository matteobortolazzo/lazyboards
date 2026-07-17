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

// --- Key sequences (prefix keybindings) ---

func TestLoad_ActionKeySequence_Valid(t *testing.T) {
	// Multi-character keys are key sequences (neovim-style prefix bindings):
	// the first key must stay in the reserved uppercase A-Z namespace, the
	// continuation keys may be any letter or digit.
	sequenceKeys := []string{"Pf", "Pb", "OPEN", "Da1"}

	for _, key := range sequenceKeys {
		t.Run("key_"+key, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  ` + key + `:
    name: Sequence action
    type: url
    url: "https://example.com"
`

			result := mustLoadConfig(t, yamlContent, "")
			if _, ok := result.Actions[key]; !ok {
				t.Fatalf("Actions missing sequence key %q", key)
			}
		})
	}
}

func TestLoad_ActionKeySequence_LowercaseFirstKey_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  pf:
    name: Bad sequence
    type: url
    url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for sequence key with lowercase first key")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "uppercase") {
		t.Errorf("error = %q, want it to contain 'uppercase'", err.Error())
	}
}

func TestLoad_ActionKeySequence_InvalidContinuationKey_ReturnsError(t *testing.T) {
	invalidKeys := []string{"P!", "P f", "P-x"}

	for _, key := range invalidKeys {
		t.Run("key_"+key, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  "` + key + `":
    name: Bad sequence
    type: url
    url: "https://example.com"
`

			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for sequence key %q", key)
			}
			errLower := strings.ToLower(err.Error())
			if !strings.Contains(errLower, "letter") && !strings.Contains(errLower, "digit") {
				t.Errorf("error = %q, want it to mention allowed continuation keys (letters/digits)", err.Error())
			}
		})
	}
}

func TestLoad_ActionKeyPrefixConflict_Global_ReturnsError(t *testing.T) {
	// "P" can never dispatch if "Pf" also exists (pressing P must wait for a
	// continuation), so a key that is a strict prefix of another is rejected.
	yamlContent := `provider: github
actions:
  P:
    name: Single action
    type: url
    url: "https://example.com"
  Pf:
    name: Sequence action
    type: url
    url: "https://example.com/frontend"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for key P being a prefix of key Pf")
	}
	if !strings.Contains(err.Error(), "P") || !strings.Contains(err.Error(), "Pf") {
		t.Errorf("error = %q, want it to reference both conflicting keys P and Pf", err.Error())
	}
}

func TestLoad_ActionKeyPrefixConflict_GlobalVsColumn_ReturnsError(t *testing.T) {
	// A column's single-key action would shadow (or be shadowed by) a global
	// sequence sharing the prefix while that column is active, so the
	// conflict is rejected across the merged global+column key set too.
	yamlContent := `provider: github
actions:
  Pf:
    name: Global sequence
    type: url
    url: "https://example.com/frontend"
columns:
  - name: Implementing
    actions:
      P:
        name: Column single action
        type: url
        url: "https://example.com"
`

	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for column key P conflicting with global key Pf")
	}
	if !strings.Contains(err.Error(), "P") || !strings.Contains(err.Error(), "Pf") {
		t.Errorf("error = %q, want it to reference both conflicting keys P and Pf", err.Error())
	}
}

func TestLoad_ActionKeySequences_NoConflict_NoError(t *testing.T) {
	// Sibling sequences sharing a prefix ("Pf"/"Pb"), an unrelated single
	// key, and a column overriding the SAME sequence key are all fine.
	yamlContent := `provider: github
actions:
  Pf:
    name: PR frontend
    type: url
    url: "https://example.com/frontend"
  Pb:
    name: PR backend
    type: url
    url: "https://example.com/backend"
  X:
    name: Other
    type: shell
    command: "echo x"
columns:
  - name: Implementing
    actions:
      Pf:
        name: PR frontend override
        type: url
        url: "https://example.com/frontend-implementing"
`

	result := mustLoadConfig(t, yamlContent, "")
	if len(result.Actions) != 3 {
		t.Fatalf("Actions count = %d, want 3", len(result.Actions))
	}
	if result.Columns[0].Actions["Pf"].Name != "PR frontend override" {
		t.Errorf("Columns[0].Actions[Pf].Name = %q, want %q", result.Columns[0].Actions["Pf"].Name, "PR frontend override")
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

// --- DefaultGitActions ---

func TestDefaultGitActions_LazygitStyleKeys(t *testing.T) {
	actions := DefaultGitActions()

	cases := []struct {
		key     string
		command string
	}{
		{"P", "git push"},
		{"p", "git pull --rebase"},
		{"f", "git fetch"},
		{"m", "git mergetool"},
		{"s", "git stash push"},
		{"S", "git stash pop"},
	}

	for _, c := range cases {
		act, ok := actions[c.key]
		if !ok {
			t.Fatalf("DefaultGitActions() missing key %q", c.key)
		}
		if act.Command != c.command {
			t.Errorf("DefaultGitActions()[%q].Command = %q, want %q", c.key, act.Command, c.command)
		}
		if act.Type != "shell" {
			t.Errorf("DefaultGitActions()[%q].Type = %q, want %q", c.key, act.Type, "shell")
		}
		if act.Scope != "board" {
			t.Errorf("DefaultGitActions()[%q].Scope = %q, want %q", c.key, act.Scope, "board")
		}
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

// --- scope: pr tests (#340) ---

func TestLoad_ActionScopePR_ExplicitIsValid(t *testing.T) {
	yamlContent := `provider: github
actions:
  W:
    name: Serve on branch
    type: shell
    scope: pr
    command: "cd {pr_branch} && ng serve"
`
	result := mustLoadConfig(t, yamlContent, "")
	action := result.Actions["W"]
	if action.Scope != "pr" {
		t.Errorf("Actions[W].Scope = %q, want %q", action.Scope, "pr")
	}
}

func TestLoad_ActionScopePR_WithCardAndPRVars_Accepted(t *testing.T) {
	// scope: pr actions must have access to all PR vars PLUS all
	// existing card-scope vars in the same template.
	yamlContent := `provider: github
actions:
  W:
    name: Annotate PR
    type: shell
    scope: pr
    command: "echo {number} {title} {tags} {session} {comment} {pr_number} {pr_branch} {pr_url} {pr_title} {pr_worktree}"
`
	result := mustLoadConfig(t, yamlContent, "")
	action := result.Actions["W"]
	if action.Scope != "pr" {
		t.Errorf("Actions[W].Scope = %q, want %q", action.Scope, "pr")
	}
}

func TestLoad_ColumnActionScopePR_Valid(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: Implementing
    actions:
      W:
        name: Serve PR branch
        type: shell
        scope: pr
        command: "cd {pr_branch} && ng serve"
`
	result := mustLoadConfig(t, yamlContent, "")
	colAction := result.Columns[0].Actions["W"]
	if colAction.Scope != "pr" {
		t.Errorf("Columns[0].Actions[W].Scope = %q, want %q", colAction.Scope, "pr")
	}
}

// TestLoad_ActionScopeCard_WithPRVar_ReturnsError is a NEW rejection: card
// scope currently allows every var, but {pr_*} vars must now be rejected
// there too (mirrors the existing board-scope rejection of card vars).
func TestLoad_ActionScopeCard_WithPRVar_ReturnsError(t *testing.T) {
	prVars := []string{"pr_branch", "pr_number", "pr_url", "pr_title", "pr_worktree"}
	for _, v := range prVars {
		t.Run(v, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  W:
    name: Card action
    type: shell
    scope: card
    command: "echo {` + v + `}"
`
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for card-scope action using {%s}", v)
			}
			errLower := strings.ToLower(err.Error())
			if !strings.Contains(errLower, "scope") || !strings.Contains(errLower, v) {
				t.Errorf("error = %q, want it to contain 'scope' and %q", err.Error(), v)
			}
		})
	}
}

func TestLoad_ActionScopeBoard_WithPRVar_ReturnsError(t *testing.T) {
	prVars := []string{"pr_branch", "pr_number", "pr_url", "pr_title", "pr_worktree"}
	for _, v := range prVars {
		t.Run(v, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  W:
    name: Board action
    type: shell
    scope: board
    command: "echo {` + v + `}"
`
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for board-scope action using {%s}", v)
			}
			errLower := strings.ToLower(err.Error())
			if !strings.Contains(errLower, "scope") || !strings.Contains(errLower, v) {
				t.Errorf("error = %q, want it to contain 'scope' and %q", err.Error(), v)
			}
		})
	}
}

// --- validateScopeConflicts: cross-map card<->pr letter conflicts (#340, Q1) ---

func TestLoad_ScopeConflict_GlobalCardColumnPR_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  W:
    name: Global card action
    type: shell
    scope: card
    command: "echo {number}"
columns:
  - name: Implementing
    actions:
      W:
        name: Column PR action
        type: shell
        scope: pr
        command: "cd {pr_branch}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for key W being card-scope globally and pr-scope in a column")
	}
	if !strings.Contains(err.Error(), "W") {
		t.Errorf("error = %q, want it to reference the conflicting key %q", err.Error(), "W")
	}
}

func TestLoad_ScopeConflict_GlobalPRColumnCard_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  W:
    name: Global PR action
    type: shell
    scope: pr
    command: "cd {pr_branch}"
columns:
  - name: Implementing
    actions:
      W:
        name: Column card action
        type: shell
        scope: card
        command: "echo {number}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for key W being pr-scope globally and card-scope in a column")
	}
	if !strings.Contains(err.Error(), "W") {
		t.Errorf("error = %q, want it to reference the conflicting key %q", err.Error(), "W")
	}
}

func TestLoad_ScopeConflict_AcrossTwoColumns_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: New
    actions:
      W:
        name: New card action
        type: shell
        scope: card
        command: "echo {number}"
  - name: Implementing
    actions:
      W:
        name: Implementing PR action
        type: shell
        scope: pr
        command: "cd {pr_branch}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for key W being card-scope in one column and pr-scope in another")
	}
	if !strings.Contains(err.Error(), "W") {
		t.Errorf("error = %q, want it to reference the conflicting key %q", err.Error(), "W")
	}
}

func TestLoad_ScopeConflict_SameScopeDifferentMaps_NoError(t *testing.T) {
	// Same scope (card) reused across global + column maps is an ordinary
	// override, not a conflict.
	yamlContent := `provider: github
actions:
  W:
    name: Global card action
    type: shell
    scope: card
    command: "echo {number}"
columns:
  - name: Implementing
    actions:
      W:
        name: Column card override
        type: shell
        scope: card
        command: "echo different {number}"
`
	result := mustLoadConfig(t, yamlContent, "")
	if result.Columns[0].Actions["W"].Scope != "card" {
		t.Errorf("Columns[0].Actions[W].Scope = %q, want %q", result.Columns[0].Actions["W"].Scope, "card")
	}
}

// --- Auto-infer board scope when scope is omitted and the template has no
// ticket-specific placeholders (#435) ---

func TestLoad_ActionScopeOmitted_NoTicketVars_InfersBoard(t *testing.T) {
	cases := []struct {
		name string
		yaml string
	}{
		{
			name: "url_with_repo_vars_only",
			yaml: `provider: github
actions:
  B:
    name: Open board
    type: url
    url: "https://github.com/{repo_owner}/{repo_name}/issues"
`,
		},
		{
			name: "url_with_no_vars",
			yaml: `provider: github
actions:
  B:
    name: Open board
    type: url
    url: "https://example.com/dashboard"
`,
		},
		{
			name: "shell_with_no_vars",
			yaml: `provider: github
actions:
  B:
    name: Deploy
    type: shell
    command: "docker compose up -d"
`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := mustLoadConfig(t, c.yaml, "")
			action := result.Actions["B"]
			if action.Scope != "board" {
				t.Errorf("Actions[B].Scope = %q, want %q (omitted scope with no ticket-specific vars should infer board)", action.Scope, "board")
			}
		})
	}
}

func TestLoad_ColumnActionScopeOmitted_NoTicketVars_InfersBoard(t *testing.T) {
	yamlContent := `provider: github
columns:
  - name: Backlog
    actions:
      B:
        name: View backlog
        type: url
        url: "https://github.com/{repo_owner}/{repo_name}/issues"
`
	result := mustLoadConfig(t, yamlContent, "")
	colAction := result.Columns[0].Actions["B"]
	if colAction.Scope != "board" {
		t.Errorf("Columns[0].Actions[B].Scope = %q, want %q (omitted scope with no ticket-specific vars should infer board in per-column overrides too)", colAction.Scope, "board")
	}
}

func TestLoad_ActionScopeOmitted_WithTicketVar_DefaultsToCard(t *testing.T) {
	// Presence of any ticket-specific placeholder must keep the default at
	// "card" -- unchanged from today's behavior -- even though the inference
	// now looks at the template.
	vars := []string{"number", "title", "tags", "session", "window"}
	for _, v := range vars {
		t.Run(v, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  B:
    name: Ticket var action
    type: url
    url: "https://example.com/{` + v + `}"
`
			result := mustLoadConfig(t, yamlContent, "")
			action := result.Actions["B"]
			if action.Scope != "card" {
				t.Errorf("Actions[B].Scope = %q, want %q (template referencing {%s} should still default to card)", action.Scope, "card", v)
			}
		})
	}
}

func TestLoad_ActionScopeOmitted_WithPRVar_ReturnsError(t *testing.T) {
	// A {pr_*} placeholder also keeps the default at "card" per the ticket,
	// which in turn trips the existing "card scope cannot use pr-specific
	// variables" validation -- so omitting scope on a pr-var template must
	// still surface that error rather than silently succeeding.
	prVars := []string{"pr_branch", "pr_number", "pr_url", "pr_title", "pr_worktree"}
	for _, v := range prVars {
		t.Run(v, func(t *testing.T) {
			yamlContent := `provider: github
actions:
  W:
    name: PR var action
    type: shell
    command: "echo {` + v + `}"
`
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for omitted-scope action using {%s} (defaults to card, which rejects pr vars)", v)
			}
			errLower := strings.ToLower(err.Error())
			if !strings.Contains(errLower, "scope") || !strings.Contains(errLower, v) {
				t.Errorf("error = %q, want it to contain 'scope' and %q", err.Error(), v)
			}
		})
	}
}

func TestLoad_ExplicitScopeCard_NotOverriddenByInference(t *testing.T) {
	// An explicit "card" scope must never be overridden by the inference,
	// even when the template has no ticket-specific placeholders.
	yamlContent := `provider: github
actions:
  B:
    name: Explicit card
    type: shell
    scope: card
    command: "docker compose up -d"
`
	result := mustLoadConfig(t, yamlContent, "")
	action := result.Actions["B"]
	if action.Scope != "card" {
		t.Errorf("Actions[B].Scope = %q, want %q (explicit scope must not be overridden by inference)", action.Scope, "card")
	}
}

func TestLoad_ExplicitScopePR_NotOverriddenByInference(t *testing.T) {
	// An explicit "pr" scope must never be overridden by the inference,
	// even when the template has no ticket-specific placeholders.
	yamlContent := `provider: github
actions:
  W:
    name: Explicit pr
    type: shell
    scope: pr
    command: "docker compose up -d"
`
	result := mustLoadConfig(t, yamlContent, "")
	action := result.Actions["W"]
	if action.Scope != "pr" {
		t.Errorf("Actions[W].Scope = %q, want %q (explicit scope must not be overridden by inference)", action.Scope, "pr")
	}
}

// --- cardSpecificVarPattern must include {window} (#435 latent bug fix) ---

func TestCardSpecificVarPattern_MatchesWindow(t *testing.T) {
	if !cardSpecificVarPattern.MatchString("{window}") {
		t.Error("cardSpecificVarPattern should match {window}: it is card-derived (per-card cenci window name) and documented as card-specific in the README")
	}
}

func TestLoad_ActionScopeBoard_WithWindow_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
actions:
  B:
    name: Board with window
    type: url
    scope: board
    url: "https://example.com/{window}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for board-scope action using {window}")
	}
	errLower := strings.ToLower(err.Error())
	if !strings.Contains(errLower, "scope") || !strings.Contains(errLower, "window") {
		t.Errorf("error = %q, want it to contain 'scope' and 'window'", err.Error())
	}
}

func TestLoad_ScopeConflict_BoardAndPRSameLetterDifferentMaps_NoError(t *testing.T) {
	// Per the Q1 decision, only card<->pr is a rejected conflict; board
	// sharing a letter with pr across maps is existing (unchanged) behavior.
	yamlContent := `provider: github
actions:
  W:
    name: Global board action
    type: shell
    scope: board
    command: "echo board"
columns:
  - name: Implementing
    actions:
      W:
        name: Column PR action
        type: shell
        scope: pr
        command: "cd {pr_branch}"
`
	result := mustLoadConfig(t, yamlContent, "")
	if result.Actions["W"].Scope != "board" {
		t.Errorf("Actions[W].Scope = %q, want %q", result.Actions["W"].Scope, "board")
	}
	if result.Columns[0].Actions["W"].Scope != "pr" {
		t.Errorf("Columns[0].Actions[W].Scope = %q, want %q", result.Columns[0].Actions["W"].Scope, "pr")
	}
}
