package config

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- Printable-rune rejection in text-input modes (Q4) ---

func TestLoad_KeymapPrintableRune_TextInputMode_ReturnsError(t *testing.T) {
	for _, mode := range []string{"create", "config", "search", "comment", "delete"} {
		t.Run(mode, func(t *testing.T) {
			yamlContent := `provider: github
keymaps:
  ` + mode + `:
    j: board.refresh
`
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for a bare printable-rune key in mode %q", mode)
			}
			if !strings.Contains(err.Error(), "printable rune") {
				t.Errorf("error = %q, want it to explain that this mode consumes every printable rune", err.Error())
			}
		})
	}
}

func TestLoad_KeymapPrintableRune_AltAndNamedKeysInTextInputMode_LoadCleanly(t *testing.T) {
	// Per Q4, only the bare printable-rune form is rejected: alt+<rune> and
	// named (non-rune) keys are exempt in text-input modes.
	yamlContent := `provider: github
keymaps:
  search:
    alt+j: board.refresh
    esc: app.quit
    enter: board.search
    ctrl+s: board.filter
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for alt+/named keys in a text-input mode: %v", err)
	}
}

func TestLoad_KeymapPrintableRune_NonTextInputMode_LoadsCleanly(t *testing.T) {
	// normal is not a text-input mode, so a bare printable-rune key is fine.
	yamlContent := `provider: github
keymaps:
  normal:
    z: board.refresh
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a printable-rune key in normal mode: %v", err)
	}
}

// --- alt+ / {comment} Alt-overload shadowing ---

func TestLoad_KeymapAltCommentShadow_BareKey_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Comment action
      type: shell
      command: "echo {comment}"
    alt+G: board.refresh
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for alt+G shadowing G's implicit {comment} Alt-overload")
	}
	if !strings.Contains(err.Error(), `"G"`) || !strings.Contains(err.Error(), `"alt+G"`) {
		t.Errorf("error = %q, want it to reference both keys \"G\" and \"alt+G\"", err.Error())
	}
}

func TestLoad_KeymapAltCommentShadow_SequenceVariant_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    "G f":
      name: Comment action
      type: shell
      command: "echo {comment}"
    "alt+G f": board.refresh
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for \"alt+G f\" shadowing \"G f\"'s implicit {comment} Alt-overload")
	}
	if !strings.Contains(err.Error(), `"G f"`) || !strings.Contains(err.Error(), `"alt+G f"`) {
		t.Errorf("error = %q, want it to reference both keys \"G f\" and \"alt+G f\"", err.Error())
	}
}

func TestLoad_KeymapAltCommentShadow_BaseWithoutComment_LoadsCleanly(t *testing.T) {
	// An alt+ binding whose base key has no {comment} action loads clean.
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Plain action
      type: shell
      command: "echo plain"
    alt+G: board.refresh
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for alt+G with a non-{comment} base action: %v", err)
	}
}

// --- Inline keymaps: action validation (mirrors top-level actions:) ---

func TestLoad_KeymapInlineAction_MissingName_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      type: shell
      command: "echo hi"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an inline keymaps: action missing name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("error = %q, want it to mention that name is required", err.Error())
	}
}

func TestLoad_KeymapInlineAction_BadType_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Bad type
      type: carrier_pigeon
      command: "echo hi"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an inline keymaps: action with an invalid type")
	}
	if !strings.Contains(err.Error(), "type must be") {
		t.Errorf("error = %q, want it to mention the allowed types", err.Error())
	}
}

func TestLoad_KeymapInlineAction_BoardScopeWithNumberVar_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Board with card var
      type: shell
      scope: board
      command: "echo {number}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a board-scope inline action using {number}")
	}
	if !strings.Contains(err.Error(), "card-specific variables") {
		t.Errorf("error = %q, want it to mention card-specific variables", err.Error())
	}
}

func TestLoad_KeymapInlineAction_CardScopeWithPRURLVar_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Card with pr var
      type: url
      scope: card
      url: "{pr_url}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a card-scope inline action using {pr_url}")
	}
	if !strings.Contains(err.Error(), "pr-specific variables") {
		t.Errorf("error = %q, want it to mention pr-specific variables", err.Error())
	}
}

func TestLoad_KeymapInlineAction_OmittedScope_InferredAndWrittenBack(t *testing.T) {
	// Closes #526: an inline keymaps: action with no scope infers one
	// (board, since the template has no ticket-specific placeholder) and
	// writes it back, the same way the top-level actions: block already
	// does.
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: No scope
      type: shell
      command: "echo hi"
`
	result := mustLoadConfig(t, yamlContent, "")

	binding := result.Keymaps.Modes[keymap.ModeNormal]["G"]
	if binding.Action.Scope != "board" {
		t.Errorf("Keymaps.Modes[normal][G].Action.Scope = %q, want inferred %q", binding.Action.Scope, "board")
	}
}

// --- Unknown command id ---

func TestLoad_KeymapUnknownCommandID_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G: nonexistent.command
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an unknown command id")
	}
	if !strings.Contains(err.Error(), "normal") || !strings.Contains(err.Error(), `"G"`) || !strings.Contains(err.Error(), "nonexistent.command") {
		t.Errorf("error = %q, want it to name the mode, key, and unknown id", err.Error())
	}
}

func TestLoad_KeymapUnknownCommandID_Column_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  columns:
    Doing:
      G: nonexistent.command
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an unknown command id in a column table")
	}
	if !strings.Contains(err.Error(), "Doing") || !strings.Contains(err.Error(), `"G"`) || !strings.Contains(err.Error(), "nonexistent.command") {
		t.Errorf("error = %q, want it to name the column, key, and unknown id", err.Error())
	}
}

// --- Scope conflicts spanning keymaps.<mode> and keymaps.columns.<name> ---

func TestLoad_KeymapScopeConflict_ModeVsColumn_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Card scope
      type: shell
      scope: card
      command: "echo {number}"
  columns:
    Doing:
      G:
        name: PR scope
        type: shell
        scope: pr
        command: "cd {pr_branch}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for key \"G\" being card-scope in keymaps.normal and pr-scope in keymaps.columns.Doing")
	}
	if !strings.Contains(err.Error(), `"G"`) {
		t.Errorf("error = %q, want it to reference the conflicting key \"G\"", err.Error())
	}
}
