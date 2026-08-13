package config

import (
	"fmt"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
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

// =====================================================================
// PR 2: the exhaustive 19-mode matrices.
//
// Above is PR 1's representative one-case-per-family coverage. Below: a
// literal, hand-verified valid/foreign command-id pair per keymap.Modes()
// entry (never derived from keymap.CommandDispatchable/DispatchableCommands
// -- the "self-referential matrix" risk the plan names), an exhaustive
// detail-matrix test sourced from keymap.Defaults() (the raw table, not the
// derived index), an inline-action reject matrix over every non-dispatching
// mode, and legacy-translation parity coverage.
// =====================================================================

// --- Full 19-mode command capability matrix ---

// capabilityCommandCase is one hand-verified (validID, foreignID) pair per
// mode: validID is bound in the mode's own defaults_*.go table; foreignID is
// bound elsewhere and reaches this mode through neither of the two
// documented widenings (IsUniversalCommand's union, ModeDetail's
// normalDefaults inheritance).
type capabilityCommandCase struct {
	mode      keymap.Mode
	key       string
	validID   keymap.CommandID
	foreignID keymap.CommandID
}

// capabilityCommandMatrix covers every keymap.Modes() entry exactly once
// (verified below by ...CoversEveryMode). key is "f1" (named, never a bare
// rune) for the five text-input modes -- a bare rune there would also trip
// validatePrintableRuneBindings (runs after this validator), failing the
// "valid" half for an unrelated reason (docs/keymap-conventions.md's remap-
// testing rule). Every other mode leaves "z" unbound in its defaults table.
var capabilityCommandMatrix = []capabilityCommandCase{
	// board (defaults_board.go)
	{keymap.ModeNormal, "z", keymap.CommandBoardRefresh, keymap.CommandFilterSelect},
	{keymap.ModeDetail, "z", keymap.CommandCardEdit, keymap.CommandCreateSubmit},

	// panel (defaults_panel.go)
	{keymap.ModeGitPanel, "z", keymap.CommandGitPanelRun, keymap.CommandAssignToggle},
	{keymap.ModeDispatch, "z", keymap.CommandDispatchOnce, keymap.CommandAgentListNext},

	// modal (defaults_modal.go)
	{keymap.ModePRList, "z", keymap.CommandPRListOpen, keymap.CommandAssignToggle},
	{keymap.ModeFilter, "z", keymap.CommandFilterSelect, keymap.CommandBoardRefresh},
	{keymap.ModeAssign, "z", keymap.CommandAssignToggle, keymap.CommandFilterSelect},
	{keymap.ModePRPicker, "z", keymap.CommandPRPickerSelect, keymap.CommandAssignToggle},
	{keymap.ModeMilestoneList, "z", keymap.CommandMilestoneListOpen, keymap.CommandPRPickerSelect},
	{keymap.ModeAgentList, "z", keymap.CommandAgentListGoToWindow, keymap.CommandMilestoneListOpen},

	// system (defaults_system.go)
	{keymap.ModeHelp, "z", keymap.CommandHelpScrollDown, keymap.CommandDispatchOnce},
	{keymap.ModeError, "z", keymap.CommandErrorRetry, keymap.CommandHelpScrollDown},

	// text -- non-printable-rune-consuming half (close_confirm, label_confirm
	// have no textinput widget, only y/n/esc, so "z" is safe there too)
	{keymap.ModeCloseConfirm, "z", keymap.CommandCloseConfirmConfirm, keymap.CommandErrorRetry},
	{keymap.ModeLabelConfirm, "z", keymap.CommandLabelConfirmCreate, keymap.CommandCloseConfirmConfirm},

	// text -- printable-rune-consuming half (create, config, search, comment,
	// delete): named key "f1", never a bare rune
	{keymap.ModeDelete, "f1", keymap.CommandDeleteSubmit, keymap.CommandLabelConfirmCreate},
	{keymap.ModeCreate, "f1", keymap.CommandCreateSubmit, keymap.CommandDeleteSubmit},
	{keymap.ModeConfig, "f1", keymap.CommandConfigSave, keymap.CommandCreateSubmit},
	{keymap.ModeSearch, "f1", keymap.CommandSearchApply, keymap.CommandConfigSave},
	{keymap.ModeComment, "f1", keymap.CommandCommentSubmit, keymap.CommandSearchApply},
}

// capabilityCommandYAML builds a minimal keymaps.<mode>.<key>: <id> config.
func capabilityCommandYAML(mode keymap.Mode, key string, id keymap.CommandID) string {
	return fmt.Sprintf("provider: github\nkeymaps:\n  %s:\n    %s: %s\n", mode, key, id)
}

// TestLoad_KeymapCapability_FullModeMatrix_CoversEveryMode pins
// capabilityCommandMatrix's exhaustiveness against keymap.Modes() itself, so
// it can't silently go stale as new modes are added -- the same drift-guard
// shape as keymap_help_test.go's helpModeSections completeness test.
func TestLoad_KeymapCapability_FullModeMatrix_CoversEveryMode(t *testing.T) {
	seen := make(map[keymap.Mode]bool, len(capabilityCommandMatrix))
	for _, tc := range capabilityCommandMatrix {
		if seen[tc.mode] {
			t.Errorf("capabilityCommandMatrix has more than one entry for mode %q, want exactly one", tc.mode)
		}
		seen[tc.mode] = true
	}
	for _, mode := range keymap.Modes() {
		if !seen[mode] {
			t.Errorf("capabilityCommandMatrix is missing an entry for mode %q, want one per keymap.Modes()", mode)
		}
	}
}

// TestLoad_KeymapCapability_FullModeMatrix runs the valid/foreign pair for
// every mode in capabilityCommandMatrix through Load.
func TestLoad_KeymapCapability_FullModeMatrix(t *testing.T) {
	for _, tc := range capabilityCommandMatrix {
		t.Run(string(tc.mode)+"/valid", func(t *testing.T) {
			yamlContent := capabilityCommandYAML(tc.mode, tc.key, tc.validID)
			if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
				t.Fatalf("Load() returned unexpected error for keymaps.%s.%s: %s: %v", tc.mode, tc.key, tc.validID, err)
			}
		})
		t.Run(string(tc.mode)+"/foreign", func(t *testing.T) {
			yamlContent := capabilityCommandYAML(tc.mode, tc.key, tc.foreignID)
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for keymaps.%s.%s: %s (foreign to mode %q)", tc.mode, tc.key, tc.foreignID, tc.mode)
			}
			assertCapabilityError(t, err,
				fmt.Sprintf("keymaps.%s", tc.mode),
				fmt.Sprintf("%q", tc.key),
				string(tc.foreignID),
				string(tc.validID),
			)
		})
	}
}

// --- Detail matrix: exhaustive normalDefaults acceptance (#588) ---

// TestLoad_KeymapCapability_Detail_AcceptsEveryNormalDefaultsID generates
// cases from keymap.Defaults()'s raw ModeNormal Table -- shipped source
// data, never the derived DispatchableCommands/CommandDispatchable index --
// so this is a genuine check of #588's runDetailCommand fall-through, not a
// tautology against the index under test. Confirms AC 7 for the full 35-id
// set, not just the card.delete case already pinned above.
func TestLoad_KeymapCapability_Detail_AcceptsEveryNormalDefaultsID(t *testing.T) {
	normalTable := keymap.Defaults().Modes[keymap.ModeNormal]
	seen := make(map[keymap.CommandID]bool)
	for _, binding := range normalTable {
		if binding.Kind != keymap.BindingCommand {
			continue
		}
		id := binding.Command
		if seen[id] {
			continue
		}
		seen[id] = true

		t.Run(string(id), func(t *testing.T) {
			yamlContent := capabilityCommandYAML(keymap.ModeDetail, "z", id)
			if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
				t.Fatalf("Load() returned unexpected error for keymaps.detail.z: %s (a normalDefaults id, #588 fall-through): %v", id, err)
			}
		})
	}
	if len(seen) == 0 {
		t.Fatal("precondition failed: keymap.Defaults().Modes[ModeNormal] produced zero distinct BindingCommand ids")
	}
}

// --- Inline-action matrix: every mode without a dispatcher rejects ---

// nonInlineActionModes is every keymap.Modes() entry except the four modes
// that dispatch inline actions (normal, detail, git_panel, pr_list --
// already covered above). Listed by hand, not derived from
// Mode.DispatchesInlineActions() (the predicate under test); its complement
// against keymap.Modes() is pinned below by ...PartitionsAllModes -- the
// same anti-self-reference reasoning as capabilityCommandMatrix.
var nonInlineActionModes = []keymap.Mode{
	keymap.ModeFilter,
	keymap.ModeAssign,
	keymap.ModePRPicker,
	keymap.ModeMilestoneList,
	keymap.ModeAgentList,
	keymap.ModeDispatch,
	keymap.ModeHelp,
	keymap.ModeError,
	keymap.ModeCreate,
	keymap.ModeConfig,
	keymap.ModeSearch,
	keymap.ModeComment,
	keymap.ModeDelete,
	keymap.ModeCloseConfirm,
	keymap.ModeLabelConfirm,
}

// capabilityActionYAML builds a minimal keymaps.<mode>.<key>: <inline
// action> config with a generic board-scope shell action -- scope is
// immaterial here (pr_list's scope=="pr" gate is covered separately above)
// since DispatchesInlineActions() gates on the mode alone.
func capabilityActionYAML(mode keymap.Mode, key string) string {
	return fmt.Sprintf(`provider: github
keymaps:
  %s:
    %s:
      name: Board action
      type: shell
      scope: board
      command: "echo hi"
`, mode, key)
}

// TestLoad_KeymapCapability_InlineAction_FullModeMatrix_PartitionsAllModes
// pins that nonInlineActionModes plus the four dispatching modes exactly
// partition keymap.Modes() -- none double-counted, none missing -- so the
// reject matrix below can't silently go stale.
func TestLoad_KeymapCapability_InlineAction_FullModeMatrix_PartitionsAllModes(t *testing.T) {
	dispatching := map[keymap.Mode]bool{
		keymap.ModeNormal:   true,
		keymap.ModeDetail:   true,
		keymap.ModeGitPanel: true,
		keymap.ModePRList:   true,
	}
	seen := make(map[keymap.Mode]bool, len(nonInlineActionModes))
	for _, mode := range nonInlineActionModes {
		if dispatching[mode] {
			t.Errorf("nonInlineActionModes includes %q, which is also listed as a dispatching mode", mode)
		}
		if seen[mode] {
			t.Errorf("nonInlineActionModes lists %q more than once", mode)
		}
		seen[mode] = true
	}
	for _, mode := range keymap.Modes() {
		if !seen[mode] && !dispatching[mode] {
			t.Errorf("mode %q is in neither nonInlineActionModes nor the dispatching set, want it in exactly one", mode)
		}
	}
}

// TestLoad_KeymapCapability_InlineAction_FullModeMatrix asserts an inline
// action is rejected in every mode of nonInlineActionModes, naming the
// config path and key (AC 3).
func TestLoad_KeymapCapability_InlineAction_FullModeMatrix(t *testing.T) {
	for _, mode := range nonInlineActionModes {
		t.Run(string(mode), func(t *testing.T) {
			key := "z"
			if mode.ConsumesPrintableRunes() {
				key = "f1"
			}
			yamlContent := capabilityActionYAML(mode, key)
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for an inline action in keymaps.%s (mode never dispatches inline actions)", mode)
			}
			assertCapabilityError(t, err, fmt.Sprintf("keymaps.%s", mode), fmt.Sprintf("%q", key), "never dispatches inline actions")
		})
	}
}

// --- Legacy-translation parity ---
//
// translateLegacyActions (legacy_actions.go) runs before this validator and
// is documented as purely additive -- these tests pin that the mode-
// capability check doesn't reject what it produces.

// TestLoad_KeymapCapability_LegacyTopLevelActions_MirrorIntoNormalAndDetail_LoadsCleanly
// mirrors legacy_actions_test.go's translation-shape test at this
// validator's boundary: a top-level actions: entry mirrors into both
// keymaps.normal and keymaps.detail (#510), both inline-action dispatchers,
// so the mirrored entries must load cleanly.
func TestLoad_KeymapCapability_LegacyTopLevelActions_MirrorIntoNormalAndDetail_LoadsCleanly(t *testing.T) {
	localYAML := `actions:
  Z:
    name: Push
    type: shell
    command: "git push"
    scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	for _, mode := range []keymap.Mode{keymap.ModeNormal, keymap.ModeDetail} {
		table, ok := cfg.Keymaps.Modes[mode]
		if !ok {
			t.Fatalf("Keymaps.Modes missing mode %q after legacy translation", mode)
		}
		binding, ok := table["Z"]
		if !ok || binding.Kind != keymap.BindingAction {
			t.Fatalf("Keymaps.Modes[%q][\"Z\"] = %+v, ok=%v, want a BindingAction surviving validateModeCapabilities", mode, binding, ok)
		}
	}
}

// TestLoad_KeymapCapability_LegacyColumnActions_LoadsCleanly mirrors a
// columns[].actions: block (#510) into keymaps.columns.<name>, which
// dispatches inline actions -- must load cleanly.
func TestLoad_KeymapCapability_LegacyColumnActions_LoadsCleanly(t *testing.T) {
	localYAML := `columns:
  - name: Implementing
    actions:
      Q:
        name: Custom action
        type: shell
        command: "echo hi"
        scope: board
`
	cfg := mustLoadConfig(t, "", localYAML)

	table, ok := cfg.Keymaps.Columns["Implementing"]
	if !ok {
		t.Fatal(`Keymaps.Columns missing "Implementing" after legacy column-action translation`)
	}
	binding, ok := table["Q"]
	if !ok || binding.Kind != keymap.BindingAction {
		t.Fatalf(`Keymaps.Columns["Implementing"]["Q"] = %+v, ok=%v, want a BindingAction surviving validateModeCapabilities`, binding, ok)
	}
}

// TestLoad_KeymapCapability_LegacyScopePRAction_ReachesPRList_LoadsCleanly
// pins the uppercase-letter, scope: pr legacy-action path
// (translateLegacyActions' isUppercaseLetterKey gate): the mirrored
// keymaps.pr_list entry must satisfy the pr_list scope=="pr" rule, exactly
// the scope the legacy action declares.
func TestLoad_KeymapCapability_LegacyScopePRAction_ReachesPRList_LoadsCleanly(t *testing.T) {
	localYAML := `actions:
  P:
    name: Open PR checks
    type: url
    scope: pr
    url: "{pr_url}"
`
	cfg := mustLoadConfig(t, "", localYAML)

	table, ok := cfg.Keymaps.Modes[keymap.ModePRList]
	if !ok {
		t.Fatal("Keymaps.Modes missing pr_list after legacy scope:pr translation")
	}
	binding, ok := table["P"]
	if !ok || binding.Kind != keymap.BindingAction || binding.Action.Scope != "pr" {
		t.Fatalf(`Keymaps.Modes["pr_list"]["P"] = %+v, ok=%v, want a scope "pr" BindingAction surviving validateModeCapabilities`, binding, ok)
	}
}
