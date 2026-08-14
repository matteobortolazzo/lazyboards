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

func TestLoad_KeymapInlineAction_URLTypeMissingURL_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: No url
      type: url
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a type: url inline action with no url")
	}
	if !strings.Contains(err.Error(), "url is required") {
		t.Errorf("error = %q, want it to mention that url is required", err.Error())
	}
}

// TestLoad_KeymapInlineAction_TerminalOnURLType_ReturnsError pins #623's
// only new validation rule: terminal: true means "hand this command the
// terminal", which is meaningless for a type: url action (it opens a
// browser, it never runs a command). Accepting it silently would leave the
// user believing a flag applies when nothing reads it.
func TestLoad_KeymapInlineAction_TerminalOnURLType_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Open issue
      type: url
      terminal: true
      url: "https://example.com"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for terminal: true on a type: url action")
	}
	if !strings.Contains(err.Error(), "terminal") {
		t.Errorf("error = %q, want it to name the terminal option", err.Error())
	}
	if !strings.Contains(err.Error(), "G") {
		t.Errorf("error = %q, want it to identify the offending key", err.Error())
	}
}

// TestLoad_KeymapInlineAction_TerminalOnShellType_Resolves is the positive
// control: a terminal: true shell action loads and the flag survives all the
// way through ResolveKeymap into the resolved keymap.Action the dispatcher
// consumes (config.Action -> keymap.Action conversion, #623 AC1).
func TestLoad_KeymapInlineAction_TerminalOnShellType_Resolves(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Run tests
      type: shell
      scope: board
      terminal: true
      command: "go test ./..."
`
	cfg, err := loadConfigFromStrings(t, yamlContent, "")
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("G")})
	if result.Binding.Kind != keymap.BindingAction {
		t.Fatalf("Lookup(G) kind = %v, want BindingAction", result.Binding.Kind)
	}
	if !result.Binding.Action.Terminal {
		t.Error("resolved keymap.Action.Terminal = false, want true -- the flag must survive config resolution")
	}
}

func TestLoad_KeymapInlineAction_ShellTypeMissingCommand_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: No command
      type: shell
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a type: shell inline action with no command")
	}
	if !strings.Contains(err.Error(), "command is required") {
		t.Errorf("error = %q, want it to mention that command is required", err.Error())
	}
}

func TestLoad_KeymapInlineAction_InvalidScope_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Bad scope
      type: shell
      scope: galaxy
      command: "echo hi"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for an inline action with an unrecognized scope")
	}
	if !strings.Contains(err.Error(), "scope must be") {
		t.Errorf("error = %q, want it to mention the allowed scopes", err.Error())
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

func TestLoad_KeymapInlineAction_BoardScopeWithPRVar_ReturnsError(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Board with pr var
      type: shell
      scope: board
      command: "cd {pr_branch}"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a board-scope inline action using {pr_branch}")
	}
	if !strings.Contains(err.Error(), "pr-specific variables") {
		t.Errorf("error = %q, want it to mention pr-specific variables", err.Error())
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

// --- #624: window:/cwd:/focus: inline action fields ---

// resolveInlineAction loads yamlContent, resolves it, and returns the inline
// action bound to key in normal mode. It fails the test if the key does not
// resolve to an inline action, so every caller below asserts on real
// post-resolution state rather than raw config maps.
func resolveInlineAction(t *testing.T, yamlContent, key string) keymap.Action {
	t.Helper()
	cfg, err := loadConfigFromStrings(t, yamlContent, "")
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key(key)})
	if result.Binding.Kind != keymap.BindingAction {
		t.Fatalf("Lookup(%q) kind = %v, want BindingAction", key, result.Binding.Kind)
	}
	return result.Binding.Action
}

func TestLoad_KeymapInlineAction_WindowFieldsResolve(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Serve worktree
      type: shell
      scope: board
      window: "srv"
      cwd: "/srv/app"
      focus: true
      command: "go run ."
`
	act := resolveInlineAction(t, yamlContent, "G")
	if act.Window != "srv" || act.Cwd != "/srv/app" || !act.Focus {
		t.Errorf("resolved action = %+v, want window/cwd/focus carried through resolution", act)
	}
}

// A window: action needs no command: opening a window in a directory is a
// complete action on its own.
func TestLoad_KeymapInlineAction_WindowWithoutCommand_Loads(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Open worktree
      type: shell
      scope: board
      window: "wt"
      cwd: "/srv/app"
`
	act := resolveInlineAction(t, yamlContent, "G")
	if act.Command != "" || act.Window != "wt" {
		t.Errorf("resolved action = %+v, want an empty command and window \"wt\"", act)
	}
}

func TestLoad_KeymapInlineAction_WindowFieldRejections(t *testing.T) {
	cases := []struct {
		name     string
		entry    string
		wantWord string
	}{
		{
			name:     "window on a url action",
			entry:    "{ name: Open, type: url, window: w, url: \"https://example.com\" }",
			wantWord: "window",
		},
		{
			name:     "cwd on a url action",
			entry:    "{ name: Open, type: url, cwd: /tmp, url: \"https://example.com\" }",
			wantWord: "cwd",
		},
		{
			name:     "focus on a url action",
			entry:    "{ name: Open, type: url, focus: true, url: \"https://example.com\" }",
			wantWord: "focus",
		},
		{
			name:     "window together with terminal",
			entry:    "{ name: Test, type: shell, scope: board, window: w, terminal: true, command: \"go test ./...\" }",
			wantWord: "terminal",
		},
		{
			name:     "focus without window",
			entry:    "{ name: Test, type: shell, scope: board, focus: true, command: \"go test ./...\" }",
			wantWord: "focus",
		},
		{
			name:     "shell action with neither command nor window",
			entry:    "{ name: Test, type: shell, scope: board }",
			wantWord: "command is required",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			yamlContent := "provider: github\nkeymaps:\n  normal:\n    G: " + tc.entry + "\n"
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want a rejection for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantWord) {
				t.Errorf("error = %q, want it to mention %q", err.Error(), tc.wantWord)
			}
			if !strings.Contains(err.Error(), "G") {
				t.Errorf("error = %q, want it to identify the offending key", err.Error())
			}
		})
	}
}

// TestLoad_KeymapInlineAction_WindowAndCwdCountAsTemplate pins that the two
// new template-bearing fields are part of the action's template everywhere
// the template is inspected: scope inference sees them, and the board-scope
// restriction can't be bypassed by moving a card variable out of command:
// and into cwd:/window:.
func TestLoad_KeymapInlineAction_WindowAndCwdCountAsTemplate(t *testing.T) {
	t.Run("card variable in window infers card scope", func(t *testing.T) {
		yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Open
      type: shell
      window: "card-{number}"
`
		if got := resolveInlineAction(t, yamlContent, "G").Scope; got != "card" {
			t.Errorf("inferred scope = %q, want \"card\": {number} in window: is a card-specific variable", got)
		}
	})

	t.Run("card variable in cwd infers card scope", func(t *testing.T) {
		yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Open
      type: shell
      window: "w"
      cwd: "/repos/{title}"
`
		if got := resolveInlineAction(t, yamlContent, "G").Scope; got != "card" {
			t.Errorf("inferred scope = %q, want \"card\": {title} in cwd: is a card-specific variable", got)
		}
	})

	t.Run("board scope rejects a card variable smuggled into cwd", func(t *testing.T) {
		yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Open
      type: shell
      scope: board
      window: "w"
      cwd: "/repos/{number}"
`
		_, err := loadConfigFromStrings(t, yamlContent, "")
		if err == nil {
			t.Fatal("Load() returned nil error, want board scope to reject {number} in cwd:")
		}
		if !strings.Contains(err.Error(), "card-specific") {
			t.Errorf("error = %q, want it to name the card-specific-variable restriction", err.Error())
		}
	})

	t.Run("card scope rejects a pr variable smuggled into cwd", func(t *testing.T) {
		yamlContent := `provider: github
keymaps:
  normal:
    G:
      name: Open
      type: shell
      scope: card
      window: "w"
      cwd: "{pr_worktree}"
`
		_, err := loadConfigFromStrings(t, yamlContent, "")
		if err == nil {
			t.Fatal("Load() returned nil error, want card scope to reject {pr_worktree} in cwd:")
		}
		if !strings.Contains(err.Error(), "pr-specific") {
			t.Errorf("error = %q, want it to name the pr-specific-variable restriction", err.Error())
		}
	})
}
