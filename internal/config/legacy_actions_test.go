package config

import (
	"reflect"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- legacySequence: rune-splitting a legacy action key into canonical form (#510) ---

// TestLegacySequence_RuneSplitsIntoCanonicalForm pins the translation rule
// straight from handlePendingSeqKey's `b.pendingSeq + string(msg.Runes)`
// concatenation (action_dispatch.go:63): a legacy key is a bare
// rune-by-rune concatenation with no separator, so translating it to the
// canonical, space-separated keymap.Sequence form is a straight rune split.
func TestLegacySequence_RuneSplitsIntoCanonicalForm(t *testing.T) {
	cases := []struct {
		legacyKey string
		want      string
	}{
		{"P", "P"},
		{"Pf", "P f"},
		{"OPEN", "O P E N"},
	}
	for _, tc := range cases {
		if got := legacySequence(tc.legacyKey); got != tc.want {
			t.Errorf("legacySequence(%q) = %q, want %q", tc.legacyKey, got, tc.want)
		}
	}
}

// --- translateLegacyActions: actions: -> keymaps.Modes[normal]/[detail] (#510) ---

// TestTranslateLegacyActions_TopLevelActionProducesNormalAndDetailEntries
// covers the Implementation Order's step 1: a config with only a top-level
// `actions:` block must produce matching entries in both
// cfg.Keymaps.Modes[normal] and cfg.Keymaps.Modes[detail], since a custom
// action dispatches from both normal mode and detail-focused mode
// (action_dispatch.go's handleCustomActionKey is shared by both).
func TestTranslateLegacyActions_TopLevelActionProducesNormalAndDetailEntries(t *testing.T) {
	localYAML := `actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	if cfg.Keymaps == nil {
		t.Fatal("Keymaps is nil, want non-nil after a legacy actions: block is translated")
	}

	for _, mode := range []keymap.Mode{keymap.ModeNormal, keymap.ModeDetail} {
		table, ok := cfg.Keymaps.Modes[mode]
		if !ok {
			t.Fatalf("Keymaps.Modes missing mode %q after legacy translation", mode)
		}
		binding, ok := table["P"]
		if !ok {
			t.Fatalf("Keymaps.Modes[%q] missing key \"P\" after legacy translation", mode)
		}
		if binding.Kind != keymap.BindingAction {
			t.Fatalf("Keymaps.Modes[%q][P].Kind = %v, want BindingAction", mode, binding.Kind)
		}
		if binding.Action.Name != "Push" || binding.Action.Type != "shell" || binding.Action.Command != "git push" {
			t.Errorf("Keymaps.Modes[%q][P].Action = %+v, want the legacy action's fields to survive translation", mode, binding.Action)
		}
	}
}

// TestTranslateLegacyActions_MultiKeySequenceUsesCanonicalForm covers a
// legacy multi-key action ("Zf") landing under its canonical, space-joined
// sequence key ("Z f") rather than the bare legacy key. Uses "Z" (unused by
// any default binding, #502) rather than "P" (now an exact-match built-in
// after #502's remap): this test loads through mustLoadConfig, which
// resolves against the real built-in defaults, so a bare "Pf"-derived
// "P f" would trip the prefix-conflict validator against default "P"
// instead of exercising the translation this test targets.
func TestTranslateLegacyActions_MultiKeySequenceUsesCanonicalForm(t *testing.T) {
	localYAML := `actions:
  Zf:
    name: Push force
    type: shell
    command: "git push --force"
    scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	if cfg.Keymaps == nil {
		t.Fatal("Keymaps is nil, want non-nil after a legacy actions: block is translated")
	}
	table := cfg.Keymaps.Modes[keymap.ModeNormal]
	if _, bare := table["Zf"]; bare {
		t.Error("Keymaps.Modes[normal] has bare legacy key \"Zf\", want it translated to canonical form \"Z f\"")
	}
	binding, ok := table["Z f"]
	if !ok {
		t.Fatal("Keymaps.Modes[normal] missing canonical key \"Z f\" after legacy translation")
	}
	if binding.Action.Name != "Push force" {
		t.Errorf("Keymaps.Modes[normal][\"Z f\"].Action.Name = %q, want %q", binding.Action.Name, "Push force")
	}
}

// --- translateLegacyActions: columns[].actions -> keymaps.Columns[<name>] (#510) ---

// TestTranslateLegacyActions_ColumnActionProducesColumnsEntry covers the
// Implementation Order's step 1 column half: a column's own `actions:`
// block must land in cfg.Keymaps.Columns[<name>], not in any mode table
// (mirroring the keymap engine's own column-overlay-only precedent: column
// bindings only overlay ModeNormal/ModeDetail via Resolve, never a
// standalone mode of their own).
func TestTranslateLegacyActions_ColumnActionProducesColumnsEntry(t *testing.T) {
	localYAML := `columns:
  - name: Implementing
    actions:
      Q:
        name: Quick
        type: shell
        command: "echo quick"
        scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	if cfg.Keymaps == nil {
		t.Fatal("Keymaps is nil, want non-nil after a legacy columns[].actions block is translated")
	}
	table, ok := cfg.Keymaps.Columns["Implementing"]
	if !ok {
		t.Fatal("Keymaps.Columns missing \"Implementing\" entry after legacy translation")
	}
	binding, ok := table["Q"]
	if !ok {
		t.Fatal("Keymaps.Columns[\"Implementing\"] missing key \"Q\" after legacy translation")
	}
	if binding.Kind != keymap.BindingAction || binding.Action.Name != "Quick" {
		t.Errorf("Keymaps.Columns[\"Implementing\"][Q] = %+v, want BindingAction(Quick)", binding)
	}

	// The column translation must not also land in a mode table.
	if normalTable, ok := cfg.Keymaps.Modes[keymap.ModeNormal]; ok {
		if _, exists := normalTable["Q"]; exists {
			t.Error("Keymaps.Modes[normal] has key \"Q\", want per-column actions confined to Keymaps.Columns")
		}
	}
}

// TestTranslateLegacyActions_RunsAfterScopeInference pins the
// validate-before-translate ordering invariant from finding #4 of the #510
// PR review: translateLegacyActions must run after validateActions, which
// mutates an omitted action.Scope in place via inferScope. Here "P"'s
// command has no card/pr-specific placeholders, so inferScope resolves it
// to "board" -- but only if validateActions ran first. translateLegacyActions
// copies the Action struct by value into the keymaps table at the moment
// it's called, so if a future refactor reordered Load() to translate before
// validating, the copied binding would freeze on the pre-inference empty
// scope, silently diverging from cfg.Actions["P"].Scope (which validateActions
// still mutates correctly in place, regardless of ordering). Asserting the
// translated binding's scope pins today's working order.
func TestTranslateLegacyActions_RunsAfterScopeInference(t *testing.T) {
	localYAML := `actions:
  P:
    name: Push
    type: shell
    command: "git push"
`
	cfg := mustLoadConfig(t, "", localYAML)

	binding, ok := cfg.Keymaps.Modes[keymap.ModeNormal]["P"]
	if !ok {
		t.Fatal("Keymaps.Modes[normal] missing key \"P\" after legacy translation")
	}
	if binding.Action.Scope != "board" {
		t.Errorf("Keymaps.Modes[normal][P].Action.Scope = %q, want %q (validateActions' scope inference must run before translateLegacyActions copies the action)", binding.Action.Scope, "board")
	}
}

// --- Equivalence: legacy-only config resolves identically to a hand-written keymaps: config (#510) ---

// TestTranslateLegacyActions_ResolvesIdenticallyToHandWrittenKeymaps is the
// Implementation Order's step 2 equivalence check: keymap.Resolve(...)
// .Entries(mode, col) must be reflect.DeepEqual between a legacy-only
// config and the hand-written keymaps: equivalent. Since A4 reversed #492's
// deferral, Tables() now carries KeymapBinding.Order through to
// keymap.Action.Order (pinned in keymaps_convert_test.go) -- this
// comparison still holds because both configs place their one action at
// document position 1 in every table it lands in (mirrored by
// TestInsertLegacyActions_StampsKeymapBindingOrder below, which pins the
// legacy side of that stamp directly), so the resulting Order values match
// even though they now flow through the comparison. Entries always returns
// a freshly sorted slice.
//
// Both configs set an explicit scope on every action: unlike cfg.Actions
// (which validateActions mutates in place to infer a scope when omitted),
// an inline keymaps: action's scope is never inferred, so an implicit
// comparison would spuriously fail on the inferred-vs-empty scope field
// rather than on anything translateLegacyActions itself gets wrong.
func TestTranslateLegacyActions_ResolvesIdenticallyToHandWrittenKeymaps(t *testing.T) {
	legacyYAML := `actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
columns:
  - name: Implementing
    actions:
      Q:
        name: Quick
        type: shell
        command: "echo quick"
        scope: board
`
	handWrittenYAML := `keymaps:
  normal:
    P:
      name: Push
      type: shell
      command: "git push"
      scope: board
  detail:
    P:
      name: Push
      type: shell
      command: "git push"
      scope: board
  columns:
    Implementing:
      Q:
        name: Quick
        type: shell
        command: "echo quick"
        scope: board
`
	legacyCfg := mustLoadConfig(t, "", legacyYAML)
	handWrittenCfg := mustLoadConfig(t, "", handWrittenYAML)

	legacyKM, err := keymap.Resolve(keymap.Tables{}, legacyCfg.Keymaps.Tables())
	if err != nil {
		t.Fatalf("keymap.Resolve(legacy) returned unexpected error: %v", err)
	}
	handWrittenKM, err := keymap.Resolve(keymap.Tables{}, handWrittenCfg.Keymaps.Tables())
	if err != nil {
		t.Fatalf("keymap.Resolve(hand-written) returned unexpected error: %v", err)
	}

	for _, tc := range []struct {
		mode   keymap.Mode
		column string
	}{
		{keymap.ModeNormal, ""},
		{keymap.ModeDetail, ""},
		{keymap.ModeNormal, "Implementing"},
		{keymap.ModeDetail, "Implementing"},
	} {
		legacyEntries := legacyKM.Entries(tc.mode, tc.column)
		handWrittenEntries := handWrittenKM.Entries(tc.mode, tc.column)
		if !reflect.DeepEqual(legacyEntries, handWrittenEntries) {
			t.Errorf("Entries(%q, %q): legacy = %+v, hand-written = %+v, want identical resolution",
				tc.mode, tc.column, legacyEntries, handWrittenEntries)
		}
	}
}

// --- Precedence: keymaps: wins over a colliding legacy-derived key (#510) ---

// TestTranslateLegacyActions_KeymapsDeclaredKeyWinsOverLegacy covers the
// Implementation Order's step 3: legacy entries never overwrite an
// existing keymaps:-declared key for the same canonical sequence. The
// keymaps: block here binds "P" to an unrelated built-in command, which
// must survive untouched instead of being replaced by the legacy action.
func TestTranslateLegacyActions_KeymapsDeclaredKeyWinsOverLegacy(t *testing.T) {
	localYAML := `actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
keymaps:
  normal:
    P: card.new
`
	cfg := mustLoadConfig(t, "", localYAML)

	binding := cfg.Keymaps.Modes[keymap.ModeNormal]["P"]
	if binding.Kind != keymap.BindingCommand || binding.Command != "card.new" {
		t.Errorf("Keymaps.Modes[normal][P] = %+v, want the keymaps:-declared CommandBinding(%q) to win over the colliding legacy action",
			binding, "card.new")
	}
}

// TestTranslateLegacyActions_KeymapsDeclaredColumnKeyWinsOverLegacy is the
// per-column analog of the mode-level collision guard above: a
// keymaps.columns.<name> declared key must win over a colliding
// columns[].actions legacy entry for the same column and key.
func TestTranslateLegacyActions_KeymapsDeclaredColumnKeyWinsOverLegacy(t *testing.T) {
	localYAML := `columns:
  - name: Implementing
    actions:
      Q:
        name: Quick
        type: shell
        command: "echo quick"
        scope: board
keymaps:
  columns:
    Implementing:
      Q: card.new
`
	cfg := mustLoadConfig(t, "", localYAML)

	binding := cfg.Keymaps.Columns["Implementing"]["Q"]
	if binding.Kind != keymap.BindingCommand || binding.Command != "card.new" {
		t.Errorf("Keymaps.Columns[\"Implementing\"][Q] = %+v, want the keymaps:-declared CommandBinding(%q) to win over the colliding legacy action",
			binding, "card.new")
	}
}

// TestTranslateLegacyActions_ColumnLookupIsCaseInsensitive covers finding
// #2 from the #510 PR review: insertLegacyActions' column lookup must
// match case-insensitively, mirroring mergeKeymaps' globalColumnsByLower
// convention (keymaps.go) and every other column-matching site in this
// package. A legacy columns[].actions block with mixed-case name
// ("Implementing") plus an existing keymaps.columns entry under the
// lowercase name ("implementing") must resolve to a single column table,
// not two separate ones -- otherwise keymap.Resolve later rejects the
// config as a duplicate-column error.
func TestTranslateLegacyActions_ColumnLookupIsCaseInsensitive(t *testing.T) {
	localYAML := `columns:
  - name: Implementing
    actions:
      Q:
        name: Quick
        type: shell
        command: "echo quick"
        scope: board
keymaps:
  columns:
    implementing:
      R: card.new
`
	cfg := mustLoadConfig(t, "", localYAML)

	if len(cfg.Keymaps.Columns) != 1 {
		t.Fatalf("Keymaps.Columns count = %d, want exactly 1 (mixed-case legacy column name and existing lowercase keymaps.columns entry must merge into one table)", len(cfg.Keymaps.Columns))
	}

	table, ok := cfg.Keymaps.Columns["implementing"]
	if !ok {
		t.Fatal("Keymaps.Columns missing \"implementing\" entry after case-insensitive merge")
	}
	if _, exists := table["Q"]; !exists {
		t.Error("Keymaps.Columns[\"implementing\"] missing key \"Q\" from the legacy translation")
	}
	if _, exists := table["R"]; !exists {
		t.Error("Keymaps.Columns[\"implementing\"] missing key \"R\" from the existing keymaps: entry")
	}

	// keymap.Resolve must not reject this config as a duplicate column.
	if _, err := keymap.Resolve(keymap.Tables{}, cfg.Keymaps.Tables()); err != nil {
		t.Errorf("keymap.Resolve returned unexpected error after case-insensitive column merge: %v", err)
	}
}

// --- Non-destructive: legacy fields stay populated after translation (#510) ---

// TestTranslateLegacyActions_LegacyActionsFieldStaysPopulated covers the
// Implementation Order's step 3 Q3 decision: translation is additive, not
// destructive -- cfg.Actions must remain populated unchanged so any
// existing code path that still reads it keeps working untouched.
func TestTranslateLegacyActions_LegacyActionsFieldStaysPopulated(t *testing.T) {
	localYAML := `actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	if len(cfg.Actions) != 1 {
		t.Fatalf("Actions count = %d, want 1 (legacy translation must not clear cfg.Actions)", len(cfg.Actions))
	}
	action, ok := cfg.Actions["P"]
	if !ok {
		t.Fatal("Actions missing key \"P\" after legacy translation")
	}
	if action.Name != "Push" || action.Type != "shell" || action.Command != "git push" {
		t.Errorf("Actions[P] = %+v, want the original legacy action fields untouched", action)
	}
}

// TestTranslateLegacyActions_LegacyColumnActionsFieldStaysPopulated is the
// per-column analog: cfg.Columns[i].Actions must remain populated
// unchanged after translation.
func TestTranslateLegacyActions_LegacyColumnActionsFieldStaysPopulated(t *testing.T) {
	localYAML := `columns:
  - name: Implementing
    actions:
      Q:
        name: Quick
        type: shell
        command: "echo quick"
        scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	if len(cfg.Columns) != 1 {
		t.Fatalf("Columns count = %d, want 1", len(cfg.Columns))
	}
	col := cfg.Columns[0]
	if len(col.Actions) != 1 {
		t.Fatalf("Columns[0].Actions count = %d, want 1 (legacy translation must not clear column actions)", len(col.Actions))
	}
	action, ok := col.Actions["Q"]
	if !ok {
		t.Fatal("Columns[0].Actions missing key \"Q\" after legacy translation")
	}
	if action.Name != "Quick" {
		t.Errorf("Columns[0].Actions[Q].Name = %q, want %q", action.Name, "Quick")
	}
}

// --- Deprecation notices (#510) ---

// TestLoad_Deprecations_TopLevelActionsProducesExactlyOneNotice covers the
// Implementation Order's step 4: a config with a legacy actions: block
// must produce exactly one deprecation notice.
func TestLoad_Deprecations_TopLevelActionsProducesExactlyOneNotice(t *testing.T) {
	localYAML := `actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	if len(cfg.Deprecations) != 1 {
		t.Fatalf("Deprecations count = %d, want 1 for a config with a legacy actions: block", len(cfg.Deprecations))
	}
}

// TestLoad_Deprecations_ColumnActionsProducesExactlyOneNotice is the
// per-column analog: a config whose only legacy construct is a
// columns[].actions block must also produce exactly one notice.
func TestLoad_Deprecations_ColumnActionsProducesExactlyOneNotice(t *testing.T) {
	localYAML := `columns:
  - name: Implementing
    actions:
      Q:
        name: Quick
        type: shell
        command: "echo quick"
        scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	if len(cfg.Deprecations) != 1 {
		t.Fatalf("Deprecations count = %d, want 1 for a config with a legacy columns[].actions block", len(cfg.Deprecations))
	}
}

// TestLoad_Deprecations_BothLegacyBlocksProduceExactlyOneNotice covers the
// "exactly one line" requirement when both legacy constructs are present
// at once -- not one notice per construct.
func TestLoad_Deprecations_BothLegacyBlocksProduceExactlyOneNotice(t *testing.T) {
	localYAML := `actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
columns:
  - name: Implementing
    actions:
      Q:
        name: Quick
        type: shell
        command: "echo quick"
        scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	if len(cfg.Deprecations) != 1 {
		t.Fatalf("Deprecations count = %d, want exactly 1 when both legacy actions: and columns[].actions: are present", len(cfg.Deprecations))
	}
}

// TestLoad_Deprecations_KeymapsOnlyConfigProducesNoNotices is the
// back-compat/false-positive guard: a config using only the keymaps:
// namespace (no legacy actions: or columns[].actions: block at all) must
// not produce any deprecation notice.
func TestLoad_Deprecations_KeymapsOnlyConfigProducesNoNotices(t *testing.T) {
	localYAML := `keymaps:
  normal:
    n: card.new
`
	cfg := mustLoadConfig(t, "", localYAML)

	if len(cfg.Deprecations) != 0 {
		t.Errorf("Deprecations = %v, want empty for a keymaps:-only config with no legacy blocks", cfg.Deprecations)
	}
}

// TestEdge_FullyShadowedLegacyBlockStillEmitsDeprecation covers finding #1
// from the #510 PR review: the deprecation notice must trigger on the
// legacy actions:/columns[].actions: block's mere presence, not on whether
// translateLegacyActions actually inserted anything. Here every derived
// legacy key ("P" and, once translated, the column's "Q") collides with an
// existing keymaps:-declared entry in every table it would land in, so
// insertLegacyActions inserts nothing at all -- yet the legacy block is
// still present in the config and must still produce exactly one notice.
func TestEdge_FullyShadowedLegacyBlockStillEmitsDeprecation(t *testing.T) {
	localYAML := `actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
columns:
  - name: Implementing
    actions:
      Q:
        name: Quick
        type: shell
        command: "echo quick"
        scope: board
keymaps:
  normal:
    P: card.new
  detail:
    P: card.new
  columns:
    Implementing:
      Q: card.new
`
	cfg := mustLoadConfig(t, "", localYAML)

	if len(cfg.Deprecations) != 1 {
		t.Fatalf("Deprecations count = %d, want exactly 1 even when every legacy-derived key is shadowed by an existing keymaps: entry", len(cfg.Deprecations))
	}

	// Confirm nothing was actually inserted (i.e. keymaps: really did win on
	// every collision), so this test genuinely exercises the "shadowed"
	// case rather than accidentally testing an ordinary insertion.
	normalBinding := cfg.Keymaps.Modes[keymap.ModeNormal]["P"]
	if normalBinding.Kind != keymap.BindingCommand || normalBinding.Command != "card.new" {
		t.Fatalf("Keymaps.Modes[normal][P] = %+v, want the keymaps:-declared command to remain unshadowed", normalBinding)
	}
	colBinding := cfg.Keymaps.Columns["Implementing"]["Q"]
	if colBinding.Kind != keymap.BindingCommand || colBinding.Command != "card.new" {
		t.Fatalf("Keymaps.Columns[\"Implementing\"][Q] = %+v, want the keymaps:-declared command to remain unshadowed", colBinding)
	}
}

// TestLoad_Deprecations_NoActionsAtAllProducesNoNotices is the
// no-configuration baseline: a config with neither actions: nor keymaps:
// at all must not produce any deprecation notice.
func TestLoad_Deprecations_NoActionsAtAllProducesNoNotices(t *testing.T) {
	localYAML := `provider: github
repo: owner/repo
`
	cfg := mustLoadConfig(t, "", localYAML)

	if len(cfg.Deprecations) != 0 {
		t.Errorf("Deprecations = %v, want empty for a config with no actions: or keymaps: blocks at all", cfg.Deprecations)
	}
}

// TestLoad_Deprecations_SyntaxPresenceMatrix covers #585: the legacy
// deprecation notice must trigger on legacy actions:/columns[].actions:
// SYNTAX PRESENCE at the raw-YAML level, independently of whether any
// derived binding survives translation or trust stripping -- not on
// post-strip/post-merge map length, which is what today's
// len(cfg.Actions) > 0 / len(col.Actions) > 0 checks actually measure.
func TestLoad_Deprecations_SyntaxPresenceMatrix(t *testing.T) {
	tests := []struct {
		name             string
		globalYAML       string
		localYAML        string
		wantDeprecations int
	}{
		{
			name: "local actions: {} emits the notice (AC1)",
			localYAML: `actions: {}
`,
			wantDeprecations: 1,
		},
		{
			name: "local columns[].actions: {} emits the notice (AC3)",
			localYAML: `columns:
  - name: New
    actions: {}
`,
			wantDeprecations: 1,
		},
		{
			name: "local actions: (null) emits no notice (AC2)",
			localYAML: `actions:
`,
			wantDeprecations: 0,
		},
		{
			name: "local actions: ~ (null) emits no notice (AC2)",
			localYAML: `actions: ~
`,
			wantDeprecations: 0,
		},
		{
			name: "local columns[].actions: (null) emits no notice (AC2)",
			localYAML: `columns:
  - name: New
    actions:
`,
			wantDeprecations: 0,
		},
		{
			name: "global actions: {} with no local config emits the notice (pins the previously-discarded global assignActionOrder walk result)",
			globalYAML: `actions: {}
`,
			wantDeprecations: 1,
		},
		{
			name: "global non-empty actions: + local keymaps: shadowing every derived key in normal/detail still emits the notice (AC5)",
			globalYAML: `actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
`,
			localYAML: `keymaps:
  normal:
    P: card.new
  detail:
    P: card.new
`,
			wantDeprecations: 1,
		},
		{
			name: "global actions: {} + local columns[].actions: {} emits exactly one notice, not two (AC7)",
			globalYAML: `actions: {}
`,
			localYAML: `columns:
  - name: New
    actions: {}
`,
			wantDeprecations: 1,
		},
		{
			name: "local merge-key-smuggled actions: still emits the notice via the decoded-map half of the presence union",
			localYAML: `.x: &b
  actions:
    X: {name: Smuggled, type: url, url: "https://example.com"}
<<: *b
`,
			wantDeprecations: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadConfigFromStrings(t, tc.globalYAML, tc.localYAML)
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			if len(cfg.Deprecations) != tc.wantDeprecations {
				t.Errorf("Deprecations = %v (len %d), want len %d", cfg.Deprecations, len(cfg.Deprecations), tc.wantDeprecations)
			}
		})
	}
}

// TestLoad_Deprecations_UntrustedShellOnlyLegacyBlockEmitsStripAndDeprecation
// covers AC4: an untrusted top-level actions: block containing only
// type: shell entries gets emptied by trust stripping before translation,
// but the legacy actions: SYNTAX was still present in the raw local
// document -- the user must still learn to migrate it, not just that a
// shell command was stripped. Both notices must be present, and distinct:
// the strip notice explains why the action is gone, the deprecation notice
// explains that the syntax itself needs migrating.
func TestLoad_Deprecations_UntrustedShellOnlyLegacyBlockEmitsStripAndDeprecation(t *testing.T) {
	localYAML := `provider: github
repo: owner/repo
actions:
  L:
    name: Local Legacy Shell
    type: shell
    command: "rm -rf /"
`
	cfg, err := loadConfigFromStringsWithTrust(t, "", localYAML, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Deprecations) != 1 {
		t.Fatalf("Deprecations = %v, want exactly 1: an untrusted shell-only legacy actions: block is still deprecated syntax even after trust stripping empties it (AC4)", cfg.Deprecations)
	}
	if len(cfg.Notices) != 1 {
		t.Fatalf("Notices = %v, want exactly 1 (the trust-strip notice)", cfg.Notices)
	}
	if strings.Contains(cfg.Deprecations[0], cfg.Notices[0]) || strings.Contains(cfg.Notices[0], cfg.Deprecations[0]) {
		t.Errorf("Deprecations[0] = %q and Notices[0] = %q, want textually distinct messages", cfg.Deprecations[0], cfg.Notices[0])
	}
	if _, exists := cfg.Actions["L"]; exists {
		t.Error("cfg.Actions[\"L\"] survived untrusted stripping: the stripped shell action must not be resurrected to fake presence")
	}
}

// TestLoad_Deprecations_UntrustedShellOnlyColumnActionsEmitsStripAndDeprecation
// is the per-column analog: stripShellFromActions resets a column's
// Actions field to nil (not an empty map) once stripping empties it
// entirely, so this exercises a distinct code path from the top-level case
// above -- the column's post-strip len(Actions) == 0 with Actions == nil,
// exactly like a column that never declared actions: at all.
func TestLoad_Deprecations_UntrustedShellOnlyColumnActionsEmitsStripAndDeprecation(t *testing.T) {
	localYAML := `provider: github
repo: owner/repo
columns:
  - name: Refined
    actions:
      L:
        name: Local Column Legacy Shell
        type: shell
        command: "rm -rf /"
`
	cfg, err := loadConfigFromStringsWithTrust(t, "", localYAML, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Deprecations) != 1 {
		t.Fatalf("Deprecations = %v, want exactly 1: an untrusted shell-only columns[].actions: block is still deprecated syntax even after trust stripping resets it to nil (AC4)", cfg.Deprecations)
	}
	if len(cfg.Notices) != 1 {
		t.Fatalf("Notices = %v, want exactly 1 (the trust-strip notice)", cfg.Notices)
	}
	if strings.Contains(cfg.Deprecations[0], cfg.Notices[0]) || strings.Contains(cfg.Notices[0], cfg.Deprecations[0]) {
		t.Errorf("Deprecations[0] = %q and Notices[0] = %q, want textually distinct messages", cfg.Deprecations[0], cfg.Notices[0])
	}

	var refined *ColumnConfig
	for i := range cfg.Columns {
		if cfg.Columns[i].Name == "Refined" {
			refined = &cfg.Columns[i]
		}
	}
	if refined == nil {
		t.Fatalf("cfg.Columns has no %q entry: %+v", "Refined", cfg.Columns)
	}
	if _, exists := refined.Actions["L"]; exists {
		t.Error("Refined column's Actions[\"L\"] survived untrusted stripping: the stripped shell action must not be resurrected to fake presence")
	}
}

// --- A4: insertLegacyActions must stamp the top-level KeymapBinding.Order too ---

// TestInsertLegacyActions_StampsKeymapBindingOrder pins the config-layer
// half of A4: insertLegacyActions must stamp the top-level
// KeymapBinding.Order (not just the nested Action.Order, which
// stampActionOrder already sets on cfg.Actions before translation runs),
// so Tables()'s now-reversed Order propagation (keymaps_convert_test.go)
// has something non-zero to carry through for legacy-derived bindings.
func TestInsertLegacyActions_StampsKeymapBindingOrder(t *testing.T) {
	localYAML := `actions:
  Z:
    name: Zebra
    type: shell
    command: "echo z"
  A:
    name: Apple
    type: shell
    command: "echo a"
`
	cfg := mustLoadConfig(t, "", localYAML)

	table := cfg.Keymaps.Modes[keymap.ModeNormal]
	zOrder := table["Z"].Order
	aOrder := table["A"].Order
	if zOrder == 0 || aOrder == 0 {
		t.Fatalf("KeymapBinding.Order for legacy-derived entries = Z:%d A:%d, want both non-zero (insertLegacyActions must stamp the top-level KeymapBinding.Order)", zOrder, aOrder)
	}
	if zOrder >= aOrder {
		t.Errorf("KeymapBinding.Order: Z=%d, A=%d, want Z < A (legacy document position order)", zOrder, aOrder)
	}
}

// TestInsertLegacyActions_ColumnBindingOrderAlsoStamped is the per-column
// analog: a legacy columns[].actions entry's derived KeymapBinding.Order
// must also be non-zero.
func TestInsertLegacyActions_ColumnBindingOrderAlsoStamped(t *testing.T) {
	localYAML := `columns:
  - name: Implementing
    actions:
      Z:
        name: Zebra
        type: shell
        command: "echo z"
      A:
        name: Apple
        type: shell
        command: "echo a"
`
	cfg := mustLoadConfig(t, "", localYAML)

	table := cfg.Keymaps.Columns["Implementing"]
	zOrder := table["Z"].Order
	aOrder := table["A"].Order
	if zOrder == 0 || aOrder == 0 {
		t.Fatalf("Keymaps.Columns[\"Implementing\"] Order for Z:%d A:%d, want both non-zero", zOrder, aOrder)
	}
	if zOrder >= aOrder {
		t.Errorf("Keymaps.Columns[\"Implementing\"] Order: Z=%d, A=%d, want Z < A (legacy document position order)", zOrder, aOrder)
	}
}

// --- translateLegacyActions: scope: pr actions -> keymaps.pr_list (#490 PR 7b, Q1) ---
//
// Today the global PR list modal reaches a legacy scope: pr action via a raw
// A-Z filter over b.actions[key] (mode_handlers.go's handlePRListActionKey).
// Once the PR list dispatches through the pr_list keymap (this ticket),
// that raw filter is deleted, so translateLegacyActions must also insert
// global scope: pr legacy actions into cfg.Keymaps.Modes[pr_list] --
// alongside the existing normal/detail insertion -- or a legacy config's
// scope: pr actions would silently stop firing in the PR list. This is a
// RED-phase addition: keymap.ModePRList exists (#508), but
// translateLegacyActions does not insert into it yet.

// TestTranslateLegacyActions_ScopePRActionReachableInPRListMode covers the
// happy path of Q1: a legacy actions: entry with scope: pr must be
// translated into cfg.Keymaps.Modes[pr_list], with its action fields intact,
// exactly like the existing normal/detail insertion.
func TestTranslateLegacyActions_ScopePRActionReachableInPRListMode(t *testing.T) {
	localYAML := `actions:
  P:
    name: Open PR
    type: shell
    command: "gh pr view {pr_number}"
    scope: pr
`
	cfg := mustLoadConfig(t, "", localYAML)

	if cfg.Keymaps == nil {
		t.Fatal("Keymaps is nil, want non-nil after a legacy scope: pr action is translated")
	}
	table, ok := cfg.Keymaps.Modes[keymap.ModePRList]
	if !ok {
		t.Fatal("Keymaps.Modes missing pr_list mode after translating a legacy scope: pr action")
	}
	binding, ok := table["P"]
	if !ok {
		t.Fatal("Keymaps.Modes[pr_list] missing key \"P\" after legacy translation of a scope: pr action")
	}
	if binding.Kind != keymap.BindingAction {
		t.Fatalf("Keymaps.Modes[pr_list][P].Kind = %v, want BindingAction", binding.Kind)
	}
	if binding.Action.Name != "Open PR" || binding.Action.Scope != "pr" {
		t.Errorf("Keymaps.Modes[pr_list][P].Action = %+v, want Name=%q Scope=%q", binding.Action, "Open PR", "pr")
	}
}

// TestTranslateLegacyActions_NonPRScopeActionNotInPRListMode covers the
// negative half of Q1: a legacy action whose scope is NOT "pr" (board, card,
// or omitted -- which infers to card/board, never pr) must not be inserted
// into cfg.Keymaps.Modes[pr_list], even though it is still inserted into
// normal/detail as before. Table-driven over the scopes the PR list must
// never see.
func TestTranslateLegacyActions_NonPRScopeActionNotInPRListMode(t *testing.T) {
	tests := []struct {
		name string
		yaml string
	}{
		{
			name: "scope: board",
			yaml: `actions:
  P:
    name: Push
    type: shell
    command: "git push"
    scope: board
`,
		},
		{
			name: "scope: card",
			yaml: `actions:
  P:
    name: Card Action
    type: shell
    command: "echo {number}"
    scope: card
`,
		},
		{
			name: "no scope (infers to board)",
			yaml: `actions:
  P:
    name: No Scope
    type: shell
    command: "echo hi"
`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := mustLoadConfig(t, "", tc.yaml)

			// The action must still land in normal/detail as before.
			if _, ok := cfg.Keymaps.Modes[keymap.ModeNormal]["P"]; !ok {
				t.Fatal("test setup: expected legacy action to still be translated into Keymaps.Modes[normal]")
			}

			table, ok := cfg.Keymaps.Modes[keymap.ModePRList]
			if ok {
				if _, exists := table["P"]; exists {
					t.Errorf("Keymaps.Modes[pr_list] has key \"P\" for a non-pr-scoped action (%s), want it absent -- only global scope: pr actions may land in pr_list", tc.name)
				}
			}
		})
	}
}

// TestTranslateLegacyActions_KeymapsDeclaredPRListKeyWinsOverLegacy is the
// pr_list analog of TestTranslateLegacyActions_KeymapsDeclaredKeyWinsOverLegacy:
// when a native (non-legacy) keymaps.pr_list.<key> entry already exists for
// the same key, the legacy scope: pr translation must NOT overwrite it -- the
// native entry always wins.
func TestTranslateLegacyActions_KeymapsDeclaredPRListKeyWinsOverLegacy(t *testing.T) {
	localYAML := `actions:
  P:
    name: Open PR
    type: shell
    command: "gh pr view {pr_number}"
    scope: pr
keymaps:
  pr_list:
    P: pr_list.open
`
	cfg := mustLoadConfig(t, "", localYAML)

	binding := cfg.Keymaps.Modes[keymap.ModePRList]["P"]
	if binding.Kind != keymap.BindingCommand || binding.Command != keymap.CommandPRListOpen {
		t.Errorf("Keymaps.Modes[pr_list][P] = %+v, want the keymaps:-declared CommandBinding(%q) to win over the colliding legacy scope: pr action",
			binding, keymap.CommandPRListOpen)
	}
}

// TestTranslateLegacyActions_LowercaseKeyedScopePRActionNotInPRListMode
// covers the code-review fix for Q1: the OLD PR-list dispatch (deleted raw
// scan, previously handlePRListActionKey in mode_handlers.go) only ever
// recognized single uppercase letters A-Z with no Alt modifier
// (`!msg.Alt && msg.Runes[0] >= 'A' && msg.Runes[0] <= 'Z'`). A legacy
// scope: pr action bound to a lowercase key was previously a hard no-op
// inside the PR list and must stay a no-op after this ticket -- only
// single-uppercase-letter-keyed legacy scope: pr actions may be translated
// into keymaps.pr_list.
func TestTranslateLegacyActions_LowercaseKeyedScopePRActionNotInPRListMode(t *testing.T) {
	localYAML := `actions:
  p:
    name: Open PR
    type: shell
    command: "gh pr view {pr_number}"
    scope: pr
`
	cfg := mustLoadConfig(t, "", localYAML)

	// The action must still land in normal/detail as before -- this
	// restriction is scoped to the pr_list insertion path only.
	if _, ok := cfg.Keymaps.Modes[keymap.ModeNormal]["p"]; !ok {
		t.Fatal("test setup: expected legacy action to still be translated into Keymaps.Modes[normal]")
	}

	table, ok := cfg.Keymaps.Modes[keymap.ModePRList]
	if ok {
		if _, exists := table["p"]; exists {
			t.Error("Keymaps.Modes[pr_list] has key \"p\" for a lowercase-keyed scope: pr action, want it absent -- the old PR-list dispatch only ever recognized single uppercase A-Z keys")
		}
	}
}
