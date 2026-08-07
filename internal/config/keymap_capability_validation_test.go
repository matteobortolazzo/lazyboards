package config

import (
	"strings"
	"testing"
)

// --- #577: validateModeCapabilities -- reject a keymaps.<mode>.<key>
// command or inline-action binding the mode can never actually dispatch ---
//
// One representative valid + one representative foreign case per rule
// family named in the ticket (normal, detail, git_panel, pr_list, one
// modal (filter), one text mode (comment), keymaps.columns.<name>), plus
// the inline-action accept/reject matrix and the pr_list scope-omitted risk
// case. This is deliberately NOT the full 19-mode x command/action matrix --
// that's PR 2 scope (per the ticket).
//
// Every error assertion below checks substrings only (config path, key,
// offending rhs, at least one representative valid id) -- never whole-
// string equality -- per .claude/rules/testing.md.

// --- Command-binding capability: normal ---

func TestLoad_KeymapCapability_Normal_ValidCommand_LoadsCleanly(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    Z: board.refresh
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a normalDefaults-bound id in keymaps.normal: %v", err)
	}
}

func TestLoad_KeymapCapability_Normal_ForeignCommand_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    Z: filter.select
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a filter-only command id bound in keymaps.normal")
	}
	assertCapabilityError(t, err, "keymaps.normal", `"Z"`, "filter.select", "board.refresh")
}

// --- Command-binding capability: detail (inherits normal, #588) ---

// TestLoad_KeymapCapability_Detail_NormalOnlyCommand_LoadsCleanly is the
// ticket's own named risk case: card.delete is bound only in normalDefaults
// (not detailDefaults), so it must still load cleanly under keymaps.detail
// once ModeDetail's allowed set is widened to detailDefaults UNION
// normalDefaults.
func TestLoad_KeymapCapability_Detail_NormalOnlyCommand_LoadsCleanly(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  detail:
    d: card.delete
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for keymaps.detail.d: card.delete: %v", err)
	}
}

// TestLoad_KeymapCapability_Detail_IDInNeitherTable_ReturnsError is the
// ticket's own named risk case: create.submit is bound in neither
// detailDefaults nor normalDefaults, so it must still be rejected under
// keymaps.detail even after the #588 inheritance widening.
func TestLoad_KeymapCapability_Detail_IDInNeitherTable_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  detail:
    z: create.submit
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for create.submit (in neither detailDefaults nor normalDefaults) bound in keymaps.detail")
	}
	assertCapabilityError(t, err, "keymaps.detail", `"z"`, "create.submit", "card.delete")
}

// --- Command-binding capability: git_panel ---

func TestLoad_KeymapCapability_GitPanel_ValidCommand_LoadsCleanly(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  git_panel:
    z: git_panel.run
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a gitPanelDefaults-bound id in keymaps.git_panel: %v", err)
	}
}

func TestLoad_KeymapCapability_GitPanel_ForeignCommand_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  git_panel:
    z: assign.toggle
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an assign-only command id bound in keymaps.git_panel")
	}
	assertCapabilityError(t, err, "keymaps.git_panel", `"z"`, "assign.toggle", "git_panel.run")
}

// --- Command-binding capability: pr_list ---

func TestLoad_KeymapCapability_PRList_ValidCommand_LoadsCleanly(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  pr_list:
    z: pr_list.open
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a prListDefaults-bound id in keymaps.pr_list: %v", err)
	}
}

func TestLoad_KeymapCapability_PRList_ForeignCommand_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  pr_list:
    z: assign.toggle
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an assign-only command id bound in keymaps.pr_list")
	}
	assertCapabilityError(t, err, "keymaps.pr_list", `"z"`, "assign.toggle", "pr_list.open")
}

// --- Command-binding capability: one modal (filter) ---

func TestLoad_KeymapCapability_Filter_ValidCommand_LoadsCleanly(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  filter:
    z: filter.select
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a filterDefaults-bound id in keymaps.filter: %v", err)
	}
}

func TestLoad_KeymapCapability_Filter_ForeignCommand_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  filter:
    z: board.refresh
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a normal-only command id bound in keymaps.filter")
	}
	assertCapabilityError(t, err, "keymaps.filter", `"z"`, "board.refresh", "filter.select")
}

// TestLoad_KeymapCapability_Filter_ForeignCommand_ValidIDListIsSorted is the
// deterministic-ordering literal spot check: the error's valid-id list must
// be alphabetically sorted, not Go's randomized map-iteration order.
// app.quit (the universal command) sorts before every filter.* id, so its
// substring index must come first.
func TestLoad_KeymapCapability_Filter_ForeignCommand_ValidIDListIsSorted(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  filter:
    z: board.refresh
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a normal-only command id bound in keymaps.filter")
	}
	msg := err.Error()
	quitIdx := strings.Index(msg, "app.quit")
	selectIdx := strings.Index(msg, "filter.select")
	if quitIdx == -1 || selectIdx == -1 {
		t.Fatalf("error = %q, want it to list both app.quit and filter.select among the valid ids", msg)
	}
	if quitIdx >= selectIdx {
		t.Errorf("error = %q: \"app.quit\" (index %d) does not precede \"filter.select\" (index %d), want the valid-id list sorted alphabetically", msg, quitIdx, selectIdx)
	}
}

// --- Command-binding capability: one text mode (comment) ---
//
// Uses a named (non-printable-rune) key so validatePrintableRuneBindings
// (already active, #526) never becomes the reason these cases pass/fail --
// see docs/keymap-conventions.md's remap-testing rule for text-input modes.

func TestLoad_KeymapCapability_TextMode_ValidCommand_LoadsCleanly(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  comment:
    f1: comment.submit
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a commentDefaults-bound id in keymaps.comment: %v", err)
	}
}

func TestLoad_KeymapCapability_TextMode_ForeignCommand_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  comment:
    f2: card.delete
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a normal-only command id bound in keymaps.comment")
	}
	assertCapabilityError(t, err, "keymaps.comment", `"f2"`, "card.delete", "comment.submit")
}

// --- Command-binding capability: keymaps.columns.<name> ---

func TestLoad_KeymapCapability_Columns_NormalOnlyCommand_LoadsCleanly(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  columns:
    Doing:
      z: nav.cursor_down
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for nav.cursor_down (normalDefaults-only) bound in keymaps.columns.Doing: %v", err)
	}
}

func TestLoad_KeymapCapability_Columns_DetailOnlyCommand_ReturnsError(t *testing.T) {
	for _, id := range []string{"detail.blur", "detail.scroll_down", "detail.scroll_up"} {
		t.Run(id, func(t *testing.T) {
			yamlContent := `provider: github
keymaps:
  columns:
    Doing:
      z: ` + id + `
`
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for detail-only id %q bound in keymaps.columns.Doing", id)
			}
			assertCapabilityError(t, err, `keymaps.columns."Doing"`, `"z"`, id, "nav.cursor_down")
		})
	}
}

// --- Inline-action capability ---

func TestLoad_KeymapCapability_InlineAction_Normal_Accepted(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    Z:
      name: Board action
      type: shell
      scope: board
      command: "echo hi"
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a board-scope inline action in keymaps.normal: %v", err)
	}
}

func TestLoad_KeymapCapability_InlineAction_Detail_Accepted(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  detail:
    z:
      name: Board action
      type: shell
      scope: board
      command: "echo hi"
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a board-scope inline action in keymaps.detail: %v", err)
	}
}

func TestLoad_KeymapCapability_InlineAction_GitPanel_Accepted(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  git_panel:
    z:
      name: Board action
      type: shell
      scope: board
      command: "echo hi"
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a board-scope inline action in keymaps.git_panel (accepts any scope): %v", err)
	}
}

func TestLoad_KeymapCapability_InlineAction_Columns_Accepted(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  columns:
    Doing:
      z:
        name: Board action
        type: shell
        scope: board
        command: "echo hi"
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a board-scope inline action in keymaps.columns.Doing: %v", err)
	}
}

func TestLoad_KeymapCapability_InlineAction_PRList_ScopePR_Accepted(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  pr_list:
    z:
      name: PR action
      type: url
      scope: pr
      url: "{pr_url}"
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a pr-scope inline action in keymaps.pr_list: %v", err)
	}
}

// TestLoad_KeymapCapability_InlineAction_PRList_ScopeOmitted_Rejected is the
// ticket's own named risk case: inferScope (config.go) never yields "pr"
// for a scope-omitted action -- a template with no ticket-specific
// placeholder infers "board", so a scope-omitted pr_list action can never
// pass pr_list's scope=="pr" gate and must be rejected, not silently
// accepted because "scope wasn't explicitly wrong".
func TestLoad_KeymapCapability_InlineAction_PRList_ScopeOmitted_Rejected(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  pr_list:
    z:
      name: No scope action
      type: shell
      command: "echo hi"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a scope-omitted inline action in keymaps.pr_list (inferScope never yields \"pr\")")
	}
	assertCapabilityError(t, err, "keymaps.pr_list", `"z"`, `"pr"`)
}

func TestLoad_KeymapCapability_InlineAction_PRList_ScopeBoard_Rejected(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  pr_list:
    z:
      name: Board action
      type: shell
      scope: board
      command: "echo hi"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a board-scope inline action in keymaps.pr_list (only scope \"pr\" dispatches there)")
	}
	assertCapabilityError(t, err, "keymaps.pr_list", `"z"`, `"pr"`)
}

func TestLoad_KeymapCapability_InlineAction_Filter_Rejected(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  filter:
    z:
      name: Board action
      type: shell
      scope: board
      command: "echo hi"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an inline action in keymaps.filter (filter never dispatches inline actions)")
	}
	assertCapabilityError(t, err, "keymaps.filter", `"z"`)
}

func TestLoad_KeymapCapability_InlineAction_TextMode_Rejected(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  comment:
    f3:
      name: Board action
      type: shell
      scope: board
      command: "echo hi"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an inline action in keymaps.comment (comment never dispatches inline actions)")
	}
	assertCapabilityError(t, err, "keymaps.comment", `"f3"`)
}

// --- Regression pin: universal app.quit is not walled off by this check ---

// TestLoad_KeymapCapability_UniversalQuit_UnboundMode_LoadsCleanly mirrors
// keymap_universal_quit_validation_test.go's own regression pin, at this
// validator's own boundary: keymaps.filter never binds app.quit by default,
// so validateModeCapabilities must still accept it via
// keymap.IsUniversalCommand's union, not reject it as "foreign to filter".
func TestLoad_KeymapCapability_UniversalQuit_UnboundMode_LoadsCleanly(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  filter:
    Q: app.quit
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for keymaps.filter.Q: app.quit: %v", err)
	}
}

// assertCapabilityError asserts err is non-nil (callers already checked)
// and its message contains every substring in want -- never a whole-string
// equality check, per .claude/rules/testing.md.
func assertCapabilityError(t *testing.T, err error, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("error = %q, want it to contain %q", err.Error(), w)
		}
	}
}
