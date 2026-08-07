package config

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- Printable-rune rejection in text-input modes (Q4) ---

// printableRuneModeCommand maps each ConsumesPrintableRunes() mode to one of
// its own default-table command ids -- #577's validateModeCapabilities would
// otherwise reject a foreign id (e.g. board.refresh, normal-only) before
// validatePrintableRuneBindings ever got a chance to fire, breaking this
// test for an unrelated reason. Picking a mode-appropriate id here keeps the
// printable-rune rejection the operative failure regardless of which
// validator runs first.
var printableRuneModeCommand = map[string]string{
	"create":  "create.submit",
	"config":  "config.save",
	"search":  "search.apply",
	"comment": "comment.submit",
	"delete":  "delete.submit",
}

func TestLoad_KeymapPrintableRune_TextInputMode_ReturnsError(t *testing.T) {
	for _, mode := range []string{"create", "config", "search", "comment", "delete"} {
		t.Run(mode, func(t *testing.T) {
			yamlContent := `provider: github
keymaps:
  ` + mode + `:
    j: ` + printableRuneModeCommand[mode] + `
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
	// named (non-rune) keys are exempt in text-input modes. Uses
	// search-appropriate command ids (app.quit is universal, the rest are
	// searchDefaults' own ids) rather than normal-only ids like
	// board.refresh/board.search/board.filter -- #577's validateModeCapabilities
	// would otherwise reject those as foreign to search before this test's
	// alt+/named-key exemption is ever reached.
	yamlContent := `provider: github
keymaps:
  search:
    alt+j: search.next_result
    esc: app.quit
    enter: search.apply
    ctrl+s: search.next_column
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
//
// Uses "Z"/"g"/"f" (unused by any default binding, #502) rather than "G" (an
// exact-match built-in after #502's remap), exercising the alt/{comment}
// -shadow check in isolation from the prefix-conflict check.

// keymapYAML wraps body (already indented under "keymaps:") in a full config
// document with a "provider: github" header.
func keymapYAML(body string) string {
	return "provider: github\nkeymaps:\n" + body
}

// altShadowYAML wraps body (already indented under "  normal:") in a full
// config document with a single keymaps.normal table.
func altShadowYAML(body string) string {
	return keymapYAML("  normal:\n" + body)
}

// keymapLoadCase/runKeymapLoadCases are the shared table-case shape/runner
// every test below dispatches through: want lists substrings the error must
// contain, checked only when wantErr.
type keymapLoadCase struct {
	name    string
	yaml    string
	wantErr bool
	want    []string
}

func runKeymapLoadCases(t *testing.T, cases []keymapLoadCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loadConfigFromStrings(t, tc.yaml, "")
			if tc.wantErr {
				if err == nil {
					t.Fatal("Load() returned nil error, want error")
				}
				for _, want := range tc.want {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error = %q, want it to contain %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
		})
	}
}

const commentActionAtZ = `    Z:
      name: Comment action
      type: shell
      command: "run --comment {comment}"
`

const commentActionAtZF = `    "Z f":
      name: Comment action
      type: shell
      command: "run --comment {comment}"
`

const commentActionAtZGF = `    "Z g f":
      name: Comment action
      type: shell
      command: "run --comment {comment}"
`

func TestLoad_KeymapAltCommentShadow(t *testing.T) {
	runKeymapLoadCases(t, []keymapLoadCase{
		{
			name:    "bare key (AC1)",
			yaml:    altShadowYAML(commentActionAtZ + "    alt+Z: board.refresh\n"),
			wantErr: true,
			want:    []string{`"Z"`, `"alt+Z"`},
		},
		{
			name:    "first token (AC2)",
			yaml:    altShadowYAML(commentActionAtZF + `    "alt+Z f": board.refresh` + "\n"),
			wantErr: true,
			want:    []string{`"Z f"`, `"alt+Z f"`},
		},
		{
			name:    "final token (AC3)",
			yaml:    altShadowYAML(commentActionAtZF + `    "Z alt+f": board.refresh` + "\n"),
			wantErr: true,
			want:    []string{`"Z f"`, `"Z alt+f"`},
		},
		{
			name:    "middle token, 3-key (AC4)",
			yaml:    altShadowYAML(commentActionAtZGF + `    "Z alt+g f": board.refresh` + "\n"),
			wantErr: true,
			want:    []string{`"Z g f"`, `"Z alt+g f"`},
		},
		{
			name:    "final token, 3-key (AC4)",
			yaml:    altShadowYAML(commentActionAtZGF + `    "Z g alt+f": board.refresh` + "\n"),
			wantErr: true,
			want:    []string{`"Z g f"`, `"Z g alt+f"`},
		},
		{
			name:    "multiple Alt (AC5)",
			yaml:    altShadowYAML(commentActionAtZF + `    "alt+Z alt+f": board.refresh` + "\n"),
			wantErr: true,
			want:    []string{`"Z f"`, `"alt+Z alt+f"`},
		},
		{
			name: "explicit rhs is an inline url action",
			yaml: altShadowYAML(commentActionAtZF + `    "Z alt+f":
      name: URL action
      type: url
      url: "https://example.com"
`),
			wantErr: true,
			want:    []string{`"Z f"`, `"Z alt+f"`},
		},
		{
			name: "{comment} in a url template (AC8)",
			yaml: altShadowYAML(`    "Z f":
      name: URL comment action
      type: url
      url: "https://example.com/comment?c={comment}"
    "Z alt+f": board.refresh
`),
			wantErr: true,
			want:    []string{`"Z f"`, `"Z alt+f"`},
		},
		{
			name:    "base absent (AC7)",
			yaml:    altShadowYAML(`    "Z alt+f": board.refresh` + "\n"),
			wantErr: false,
		},
		{
			name: "base explicitly unbound (AC7)",
			yaml: altShadowYAML(`    "Z f": ~
    "Z alt+f": board.refresh
`),
			wantErr: false,
		},
		{
			name: "base is a command (AC7)",
			yaml: altShadowYAML(`    "Z f": board.refresh
    "Z alt+f": board.filter
`),
			wantErr: false,
		},
		{
			name: "base action without {comment} (AC7)",
			yaml: altShadowYAML(`    "Z f":
      name: Plain action
      type: shell
      command: "echo plain"
    "Z alt+f": board.refresh
`),
			wantErr: false,
		},
		{
			name: "no Alt anywhere (negative control)",
			yaml: altShadowYAML(commentActionAtZF + `    "Z g":
      name: Other action
      type: shell
      command: "echo other"
`),
			wantErr: false,
		},
		{
			name: "named key preserved (over-broad alt+ stripping risk)",
			yaml: altShadowYAML(`    enter:
      name: Comment action
      type: shell
      command: "run --comment {comment}"
    alt+enter: board.refresh
`),
			wantErr: true,
			want:    []string{`"enter"`},
		},
		{
			name: "named key negative control",
			yaml: altShadowYAML(`    enter:
      name: Comment action
      type: shell
      command: "run --comment {comment}"
`),
			wantErr: false,
		},
	})
}

func TestLoad_KeymapAltCommentShadow_ColumnOverlay(t *testing.T) {
	runKeymapLoadCases(t, []keymapLoadCase{
		{
			name: "base in normal, alt+ variant in column overlay",
			yaml: keymapYAML(`  normal:
` + commentActionAtZF + `  columns:
    Doing:
      "Z alt+f": board.refresh
`),
			wantErr: true,
			want:    []string{`"Doing"`, `"Z f"`, `"Z alt+f"`},
		},
		{
			name: "column override of base does not clear the mode-level conflict",
			yaml: keymapYAML(`  normal:
` + commentActionAtZF + `    "Z alt+f": board.refresh
  columns:
    Doing:
      "Z f":
        name: Override action
        type: shell
        command: "echo override"
`),
			wantErr: true,
			want:    []string{`"Z f"`, `"Z alt+f"`},
		},
		{
			name: "column unbind of base clears the conflict for that column",
			yaml: keymapYAML(`  normal:
` + commentActionAtZF + `  columns:
    Doing:
      "Z f": ~
      "Z alt+f": board.refresh
`),
			wantErr: false,
		},
	})
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
//
// Canonicalization must collapse whitespace variants of "Z f" (unused by any
// default binding, #502) onto the same physical key before scopes are
// compared -- the canonical form keymap.ParseSequence(...).String() and
// runtime dispatch already agree on.

const cardActionAtZF = `    "Z f":
      name: Card action
      type: shell
      scope: card
      command: "echo {number}"
`

const boardActionAtZF = `    "Z f":
      name: Board action
      type: shell
      scope: board
      command: "echo hi"
`

const prActionYAML = `      name: PR action
      type: url
      scope: pr
      url: "{pr_url}"
`

// prActionYAMLColumn is prActionYAML re-indented for keymaps.columns.<name>,
// whose keys sit one level deeper than a plain mode table's.
const prActionYAMLColumn = `        name: PR action
        type: url
        scope: pr
        url: "{pr_url}"
`

func TestLoad_KeymapScopeConflict(t *testing.T) {
	runKeymapLoadCases(t, []keymapLoadCase{
		{
			name: "normal card vs detail pr, double-space whitespace variant",
			yaml: keymapYAML(`  normal:
` + cardActionAtZF + `  detail:
    "Z  f":
` + prActionYAML),
			wantErr: true,
			want:    []string{`"Z f"`, `"card"`, `"pr"`},
		},
		{
			name: "normal card vs column pr, leading/trailing whitespace variant",
			yaml: keymapYAML(`  normal:
` + cardActionAtZF + `  columns:
    Doing:
      " Z f ":
` + prActionYAMLColumn),
			wantErr: true,
			want:    []string{`"Z f"`, `"card"`, `"pr"`},
		},
		{
			name: "detail card vs column pr, double-space whitespace variant",
			yaml: keymapYAML(`  detail:
` + cardActionAtZF + `  columns:
    Doing:
      "Z  f":
` + prActionYAMLColumn),
			wantErr: true,
			want:    []string{`"Z f"`, `"card"`, `"pr"`},
		},
		{
			name: "board vs card, same canonical seq, different tables: allowed",
			yaml: keymapYAML(`  normal:
` + boardActionAtZF + `  detail:
` + cardActionAtZF),
			wantErr: false,
		},
		{
			name: "board vs pr, same canonical seq, different tables: allowed",
			yaml: keymapYAML(`  normal:
` + boardActionAtZF + `  columns:
    Doing:
      "Z f":
` + prActionYAMLColumn),
			wantErr: false,
		},
		{
			name: "card vs card across normal + detail, different spellings: allowed",
			yaml: keymapYAML(`  normal:
` + cardActionAtZF + `  detail:
    "Z f":
      name: Card action B
      type: shell
      scope: card
      command: "echo {number}"
`),
			wantErr: false,
		},
		{
			name: "card action + command binding on the same canonical seq: commands excluded",
			yaml: keymapYAML(`  normal:
` + cardActionAtZF + `  detail:
    "Z  f": board.refresh
`),
			wantErr: false,
		},
		{
			name: "card action + explicit unbind on the same canonical seq: unbinds excluded",
			yaml: keymapYAML(`  normal:
` + cardActionAtZF + `  detail:
    "Z  f": ~
`),
			wantErr: false,
		},
		{
			name: "unparseable action key propagates a load error with mode/key context",
			yaml: keymapYAML(`  normal:
    "nope-key":
      name: Bad key action
      type: shell
      scope: card
      command: "echo {number}"
`),
			wantErr: true,
			want:    []string{"keymaps.normal", `"nope-key"`},
		},
	})
}

// --- validateCapabilityTable: unrecognized binding kind (defense-in-depth) ---

func TestValidateCapabilityTable_UnrecognizedBindingKind_ReturnsError(t *testing.T) {
	// KeymapBinding.UnmarshalYAML (keymaps.go) never itself produces
	// keymap.BindingInvalid -- every parsed binding is BindingCommand,
	// BindingAction, or BindingUnbound -- so this constructs one directly,
	// bypassing YAML parsing entirely, to pin validateCapabilityTable's
	// default case as fail-closed rather than silently treating an
	// unrecognized Kind as valid.
	table := KeymapTable{
		"z": KeymapBinding{Kind: keymap.BindingInvalid},
	}
	err := validateCapabilityTable(table, keymap.ModeNormal, "keymaps.normal")
	if err == nil {
		t.Fatal("validateCapabilityTable() returned nil error, want error for an unrecognized BindingKind")
	}
	for _, want := range []string{"keymaps.normal", `"z"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to contain %q", err.Error(), want)
		}
	}
}
