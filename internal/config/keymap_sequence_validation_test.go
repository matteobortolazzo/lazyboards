package config

import (
	"fmt"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- #578: validateSequenceCapability -- reject a keymaps.<mode>.<key>
// binding whose parsed keymap.ParseSequence result has more than one
// element, for every mode outside {normal, detail, columns.<name>} --
// regardless of rhs kind (command, inline action, or explicit unbind).
//
// Every error assertion below checks substrings only (config path, raw
// key, canonical sequence, mode name) -- never whole-string equality --
// per .claude/rules/testing.md. assertCapabilityError
// (keymap_capability_validation_test.go) is reused rather than
// re-declaring an identical substring-checking helper in this file.

// twoKeySequence is the two-key raw binding used throughout this file:
// two named keys ("f1", "f2"), never a bare printable rune, so it stays
// valid input for the five ConsumesPrintableRunes() modes too (per
// docs/keymap-conventions.md's remap-testing rule) without needing a
// per-mode key variant.
const twoKeySequence = "f1 f2"

// sequenceCapabilityCase is one (mode, validID) pair reused from
// capabilityCommandMatrix (keymap_capability_validation_test.go): validID
// is a command id the mode can dispatch at a single key, so binding it to
// twoKeySequence isolates the sequence-length rule from #577's
// mode-capability rule (the "wrong validator fires" risk the plan names).
type sequenceCapabilityCase struct {
	mode    keymap.Mode
	validID keymap.CommandID
}

// sequenceCapabilityMatrix is derived from capabilityCommandMatrix's own
// hand-verified validID column rather than re-hand-picking ids, so this
// file's matrix cannot silently drift out of sync with the ids that
// #577's capability check already accepts for each mode.
var sequenceCapabilityMatrix = func() []sequenceCapabilityCase {
	out := make([]sequenceCapabilityCase, 0, len(capabilityCommandMatrix))
	for _, tc := range capabilityCommandMatrix {
		out = append(out, sequenceCapabilityCase{mode: tc.mode, validID: tc.validID})
	}
	return out
}()

// sequenceCapableModes is the ticket's Decision, hardcoded independently
// of any keymap.Mode predicate under test: multi-key sequences remain
// supported only in normal, detail, and keymaps.columns.<name> (the
// column case is covered separately below since ModeColumns is excluded
// from keymap.Modes()).
var sequenceCapableModes = map[keymap.Mode]bool{
	keymap.ModeNormal: true,
	keymap.ModeDetail: true,
}

// sequenceCapabilityYAML builds a minimal keymaps.<mode>."f1 f2": <id>
// config.
func sequenceCapabilityYAML(mode keymap.Mode, id keymap.CommandID) string {
	return fmt.Sprintf("provider: github\nkeymaps:\n  %s:\n    %q: %s\n", mode, twoKeySequence, id)
}

// TestLoad_KeymapSequenceCapability_FullModeMatrix_CoversEveryMode pins
// sequenceCapabilityMatrix's exhaustiveness against keymap.Modes() itself,
// mirroring TestLoad_KeymapCapability_FullModeMatrix_CoversEveryMode.
func TestLoad_KeymapSequenceCapability_FullModeMatrix_CoversEveryMode(t *testing.T) {
	seen := make(map[keymap.Mode]bool, len(sequenceCapabilityMatrix))
	for _, tc := range sequenceCapabilityMatrix {
		if seen[tc.mode] {
			t.Errorf("sequenceCapabilityMatrix has more than one entry for mode %q, want exactly one", tc.mode)
		}
		seen[tc.mode] = true
	}
	for _, mode := range keymap.Modes() {
		if !seen[mode] {
			t.Errorf("sequenceCapabilityMatrix is missing an entry for mode %q, want one per keymap.Modes()", mode)
		}
	}
}

// TestLoad_KeymapSequenceCapability_FullModeMatrix asserts a two-key
// command binding loads cleanly for normal/detail and errors, naming the
// config path, raw key, canonical sequence, and mode, for every other
// mode.
func TestLoad_KeymapSequenceCapability_FullModeMatrix(t *testing.T) {
	for _, tc := range sequenceCapabilityMatrix {
		t.Run(string(tc.mode), func(t *testing.T) {
			yamlContent := sequenceCapabilityYAML(tc.mode, tc.validID)
			_, err := loadConfigFromStrings(t, yamlContent, "")

			if sequenceCapableModes[tc.mode] {
				if err != nil {
					t.Fatalf("Load() returned unexpected error for a two-key sequence in keymaps.%s (sequence-capable mode): %v", tc.mode, err)
				}
				return
			}

			if err == nil {
				t.Fatalf("Load() returned nil error, want error for a two-key sequence in keymaps.%s (single-key-only mode)", tc.mode)
			}
			assertCapabilityError(t, err,
				fmt.Sprintf("keymaps.%s", tc.mode),
				fmt.Sprintf("%q", twoKeySequence),
				twoKeySequence,
				string(tc.mode),
			)
		})
	}
}

// --- rhs variants in one rejected mode ---
//
// The command-id and unbind variants use keymaps.filter, per the plan.
// The inline-action variant instead uses keymaps.git_panel: filter never
// dispatches inline actions at all (Mode.DispatchesInlineActions() is
// false), so #577's validateModeCapabilities -- which runs before this
// ticket's check, per the plan's Assumptions -- would reject any inline
// action bound in keymaps.filter regardless of key length, masking
// whether the sequence-length rule fired. git_panel does dispatch inline
// actions at a single key (any scope), so binding one to twoKeySequence
// isolates the sequence rule the same way the command-id case's
// filter.select (a filter-dispatchable id) isolates it above.

func TestLoad_KeymapSequenceCapability_Filter_CommandRHS_Rejected(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  filter:
    "f1 f2": filter.select
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a two-key command binding in keymaps.filter")
	}
	assertCapabilityError(t, err, "keymaps.filter", `"f1 f2"`, "f1 f2", "filter")
}

func TestLoad_KeymapSequenceCapability_Filter_UnbindRHS_Rejected(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  filter:
    "f1 f2": ~
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a two-key unbind (~) in keymaps.filter -- AC 1 rejects unbinds too, unlike #577's capability check")
	}
	assertCapabilityError(t, err, "keymaps.filter", `"f1 f2"`, "f1 f2", "filter")
}

func TestLoad_KeymapSequenceCapability_GitPanel_InlineActionRHS_Rejected(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  git_panel:
    "f1 f2":
      name: Board action
      type: shell
      scope: board
      command: "echo hi"
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a two-key inline action in keymaps.git_panel")
	}
	assertCapabilityError(t, err, "keymaps.git_panel", `"f1 f2"`, "f1 f2", "git_panel")
}

// --- named-single-key controls: proves counting happens after
// keymap.ParseSequence, not by strings.Contains(key, " ") or key length ---

// TestLoad_KeymapSequenceCapability_NamedSingleKeyControls_LoadCleanly
// binds each of these named, multi-character-but-single-element keys in
// keymaps.filter (a rejected mode) and asserts they all load cleanly.
// ctrl+c is deliberately excluded -- it is separately rejected by
// validateNoCtrlC regardless of mode, and testing it here would conflate
// the two rules.
func TestLoad_KeymapSequenceCapability_NamedSingleKeyControls_LoadCleanly(t *testing.T) {
	keys := []string{"shift+tab", "alt+j", "ctrl+a", "alt+enter", "up", "esc"}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			yamlContent := fmt.Sprintf("provider: github\nkeymaps:\n  filter:\n    %q: filter.select\n", key)
			if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
				t.Fatalf("Load() returned unexpected error for named single key %q in keymaps.filter: %v", key, err)
			}
		})
	}
}

// --- keymaps.columns.<name> stays sequence-capable ---

func TestLoad_KeymapSequenceCapability_Columns_TwoKeyInlineAction_LoadsCleanly(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  columns:
    Doing:
      "f1 f2":
        name: Board action
        type: shell
        scope: board
        command: "echo hi"
`
	if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
		t.Fatalf("Load() returned unexpected error for a two-key inline action in keymaps.columns.Doing: %v", err)
	}
}

// --- whitespace normalization ---

// TestLoad_KeymapSequenceCapability_WhitespaceNormalization_ReportsCanonicalForm
// pins the plan's named risk: the validator must canonicalize via
// keymap.ParseSequence/Sequence.String() (whitespace-collapsing), not a
// naive strings.Contains(key, " ") scan, and the error must report the
// canonical "z x" form alongside the raw, whitespace-padded "z   x" key.
func TestLoad_KeymapSequenceCapability_WhitespaceNormalization_ReportsCanonicalForm(t *testing.T) {
	yamlContent := `provider: github
keymaps:
  filter:
    "z   x": filter.select
`
	_, err := loadConfigFromStrings(t, yamlContent, "")
	if err == nil {
		t.Fatal("Load() returned nil error, want error for a whitespace-padded multi-key sequence in keymaps.filter")
	}
	assertCapabilityError(t, err, "keymaps.filter", `"z   x"`, `"z x"`, "filter")
}

// --- positive control ---
//
// The repo root's shipped_config_test.go (package main) already loads the
// repo's own .lazyboards.yml through the real Load pipeline
// (TestShippedConfig_MigratedToKeymaps); that test is not duplicated here
// (per the ticket's Tests item 7) -- it stays the confirmation that this
// new check doesn't regress the shipped config.
