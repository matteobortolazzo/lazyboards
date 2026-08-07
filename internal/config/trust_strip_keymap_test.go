package config

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- #584: stripShellFromKeymapTable compares each local-origin `keymaps:`
// shell binding against the trusted global snapshot (mirroring
// stripShellFromActions/stripLocalCleanup), so a local binding that is
// value-equivalent to its global counterpart is inherited rather than
// stripped and counted. This file is the keymap half of that coverage (the
// legacy actions:/columns[].actions: half lives in trust_strip_legacy_test.go).
//
// Per the ticket's binding Q&A:
//   - Q1: the equivalence comparison excludes KeymapBinding.Order (and the
//     nested Action.Order) -- a literal keymaps: block stamps Order 1,2,...
//     from document position, but a local keymaps: *anchor leaves every
//     aliased entry at Order: 0 (stampKeymapsOrder bails out as soon as it
//     sees the value node for the "keymaps" key is an AliasNode, not a
//     MappingNode -- see config.go's stampKeymapsOrder). Only Kind, Command,
//     and the executing Action fields (Name/Type/URL/Command/Scope) are
//     compared.
//   - Q3: an explicitly-empty local table (keymaps: {normal: {}}) that
//     strips nothing must not trigger the delete-whole-table-so-global-
//     inherits behavior -- that behavior is reserved for a table that
//     genuinely strips something down to empty (already covered,
//     unmodified, by TestLoad_Untrusted_EmptiedLocalModeTableStillInheritsOtherGlobalKeys
//     and TestLoad_Untrusted_EmptiedLocalColumnTableStillInheritsOtherGlobalColumnKeys
//     in trust_strip_test.go -- not re-added here).
//
// Every assertion goes through Load -> ResolveKeymap -> km.Lookup (or
// Config.Notices), never against internal map contents, mirroring
// trust_strip_test.go's established convention: each case asserts both
// Config.Notices and the resolved Lookup outcome, since a Notices-only
// assertion could miss a case where a wrongly-stripped local entry is
// silently re-inherited from global with the same effective value -- exactly
// the contradiction ("stripped" reported, but the same command still runs)
// this ticket exists to close.
//
// Plan Assumption: this comparison runs before validateActions/
// validateKeymapActions infer a default scope:. A local binding omitting
// scope: while global specifies one therefore compares as differing and is
// stripped, then falls back to the identical global entry -- safe, and
// unchanged from prior behavior. Every fixture below sets scope: explicitly
// on both sides to keep that inference timing out of scope for this matrix.

// keymapCasePlacement describes where a keymaps table entry is embedded --
// either directly under keymaps.normal (mode-scoped) or under
// keymaps.columns.Refined (column-scoped) -- and which (mode, column) pair
// Lookup must be queried against to observe it. Every case below runs once
// per placement via t.Run(placement.name, ...), per the ticket's "each case
// x mode table AND keymaps.columns.<name>" requirement.
type keymapCasePlacement struct {
	name   string
	column string
	// embed wraps one or more single-line "key: <binding>" YAML fragments
	// (e.g. `b: { name: Build, type: shell, command: make }` or `y: app.help`
	// or `y: ~`) into a full keymaps: block at this placement's scope. Local
	// callers still need to prepend their own provider/repo/columns:
	// preamble; embed only returns the keymaps: block itself.
	embed func(entries ...string) string
}

func modeKeymapPlacement() keymapCasePlacement {
	return keymapCasePlacement{
		name:   "mode",
		column: "",
		embed: func(entries ...string) string {
			var b strings.Builder
			b.WriteString("keymaps:\n  normal:\n")
			for _, e := range entries {
				b.WriteString("    " + e + "\n")
			}
			return b.String()
		},
	}
}

func columnKeymapPlacement() keymapCasePlacement {
	return keymapCasePlacement{
		name:   "column",
		column: "Refined",
		embed: func(entries ...string) string {
			var b strings.Builder
			b.WriteString("columns:\n  - name: Refined\nkeymaps:\n  columns:\n    Refined:\n")
			for _, e := range entries {
				b.WriteString("      " + e + "\n")
			}
			return b.String()
		},
	}
}

func keymapCasePlacements() []keymapCasePlacement {
	return []keymapCasePlacement{modeKeymapPlacement(), columnKeymapPlacement()}
}

const buildShellEntry = `b: { name: Build, type: shell, scope: board, command: make }`

// --- Case: identical global/local shell binding -> not stripped, no
// notice, and the shell action itself (not some fallback) resolves. ---

func TestLoad_Untrusted_KeymapIdenticalToGlobal_NotStrippedNoNotice(t *testing.T) {
	for _, p := range keymapCasePlacements() {
		t.Run(p.name, func(t *testing.T) {
			globalYAML := p.embed(buildShellEntry)
			localYAML := "provider: github\nrepo: owner/repo\n" + p.embed(buildShellEntry)

			globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
			cfg, err := Load(globalPath, localPath, Trust{})
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			km, err := ResolveKeymap(&cfg)
			if err != nil {
				t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
			}

			result := km.Lookup(keymap.ModeNormal, p.column, keymap.Sequence{keymap.Key("b")})
			if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Command != "make" {
				t.Fatalf("Lookup(normal, %q, b) = %+v, want the global-equivalent local shell binding to remain effective", p.column, result)
			}
			if len(cfg.Notices) != 0 {
				t.Fatalf("cfg.Notices = %v, want empty: a local keymap shell binding byte-identical to global's must not be counted as stripped", cfg.Notices)
			}
		})
	}
}

// --- Case: identical fields at a different document position (via a YAML
// anchor/alias, so KeymapBinding.Order differs) -> not stripped, no notice.
// This pins Q1's Order-exclusion decision. Both mode and column tables are
// exercised in the same document, since stampKeymapsOrder's AliasNode bail-
// out (config.go) applies to the WHOLE keymaps: value node once, covering
// both keymaps.normal and keymaps.columns in one shot. ---

func TestLoad_Untrusted_KeymapAliasedIdenticalToGlobal_OrderExcludedFromComparison(t *testing.T) {
	globalYAML := `
keymaps:
  normal:
    b: { name: Build, type: shell, scope: board, command: make }
  columns:
    Refined:
      c: { name: Col Build, type: shell, scope: board, command: "make col" }
`
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
.anchor: &anc
  normal:
    b: { name: Build, type: shell, scope: board, command: make }
  columns:
    Refined:
      c: { name: Col Build, type: shell, scope: board, command: "make col" }
keymaps: *anc
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	resultMode := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	if resultMode.Outcome != keymap.OutcomeMatch || resultMode.Binding.Kind != keymap.BindingAction || resultMode.Binding.Action.Command != "make" {
		t.Fatalf("Lookup(normal, \"\", b) = %+v, want the aliased-but-content-identical mode binding to remain effective", resultMode)
	}
	resultCol := km.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("c")})
	if resultCol.Outcome != keymap.OutcomeMatch || resultCol.Binding.Kind != keymap.BindingAction || resultCol.Binding.Action.Command != "make col" {
		t.Fatalf("Lookup(normal, Refined, c) = %+v, want the aliased-but-content-identical column binding to remain effective", resultCol)
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: aliasing the whole keymaps: block leaves every binding's Order at 0 (stampKeymapsOrder never runs), which must not make a content-identical binding look different from its global counterpart", cfg.Notices)
	}
}

// --- Case: the aliased companion to the above -- when the aliased local
// content genuinely DIFFERS from global, it must still be stripped and
// counted. Aliases must not bypass stripping. ---

func TestLoad_Untrusted_KeymapAliasedDiffering_StillStrippedAndCounted(t *testing.T) {
	globalYAML := `
keymaps:
  normal:
    b: { name: Build, type: shell, scope: board, command: make }
  columns:
    Refined:
      c: { name: Col Build, type: shell, scope: board, command: "make col" }
`
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
.anchor: &anc
  normal:
    b: { name: Build, type: shell, scope: board, command: "rm -rf /" }
  columns:
    Refined:
      c: { name: Col Build, type: shell, scope: board, command: "rm -rf /col" }
keymaps: *anc
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	resultMode := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	if resultMode.Outcome != keymap.OutcomeMatch || resultMode.Binding.Kind != keymap.BindingAction || resultMode.Binding.Action.Command != "make" {
		t.Fatalf("Lookup(normal, \"\", b) = %+v, want fallback to the global command (aliased local override differs and must still be stripped)", resultMode)
	}
	resultCol := km.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("c")})
	if resultCol.Outcome != keymap.OutcomeMatch || resultCol.Binding.Kind != keymap.BindingAction || resultCol.Binding.Action.Command != "make col" {
		t.Fatalf("Lookup(normal, Refined, c) = %+v, want fallback to the global column command (aliased local override differs and must still be stripped)", resultCol)
	}
	if len(cfg.Notices) != 1 {
		t.Fatalf("cfg.Notices = %v, want exactly one aggregated notice for the two stripped aliased bindings", cfg.Notices)
	}
	if !strings.Contains(cfg.Notices[0], "2 keymap shell binding(s)") {
		t.Fatalf("cfg.Notices[0] = %q, want it to count both stripped bindings (\"2 keymap shell binding(s)\")", cfg.Notices[0])
	}
}

// --- Case: a differing local override (same key, different command) is
// stripped, counted exactly once, and falls back to the global binding. ---

func TestLoad_Untrusted_KeymapDifferingOverride_StrippedFallsBackToGlobalNoticedOnce(t *testing.T) {
	for _, p := range keymapCasePlacements() {
		t.Run(p.name, func(t *testing.T) {
			localEntry := `b: { name: Build, type: shell, scope: board, command: "rm -rf /" }`
			globalYAML := p.embed(buildShellEntry)
			localYAML := "provider: github\nrepo: owner/repo\n" + p.embed(localEntry)

			globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
			cfg, err := Load(globalPath, localPath, Trust{})
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			km, err := ResolveKeymap(&cfg)
			if err != nil {
				t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
			}

			result := km.Lookup(keymap.ModeNormal, p.column, keymap.Sequence{keymap.Key("b")})
			if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Command != "make" {
				t.Fatalf("Lookup(normal, %q, b) = %+v, want fallback to the global command \"make\" (differing local override must be stripped)", p.column, result)
			}
			if len(cfg.Notices) != 1 {
				t.Fatalf("cfg.Notices = %v, want exactly one notice for the single stripped keymap binding", cfg.Notices)
			}
			if !strings.Contains(cfg.Notices[0], "1 keymap shell binding(s)") {
				t.Fatalf("cfg.Notices[0] = %q, want it to read %q", cfg.Notices[0], "1 keymap shell binding(s)")
			}
		})
	}
}

// --- Case: a local-only shell binding (no global counterpart at all) is
// stripped, counted exactly once, and falls back to the built-in default. ---

func TestLoad_Untrusted_KeymapLocalOnly_StrippedFallsBackToBuiltinDefaultNoticedOnce(t *testing.T) {
	for _, p := range keymapCasePlacements() {
		t.Run(p.name, func(t *testing.T) {
			localEntry := `q: { name: Evil, type: shell, command: "rm -rf /" }`
			localYAML := "provider: github\nrepo: owner/repo\n" + p.embed(localEntry)

			globalPath, localPath := writeTempConfigs(t, "", localYAML)
			cfg, err := Load(globalPath, localPath, Trust{})
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			km, err := ResolveKeymap(&cfg)
			if err != nil {
				t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
			}

			result := km.Lookup(keymap.ModeNormal, p.column, keymap.Sequence{keymap.Key("q")})
			if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingCommand || result.Binding.Command != keymap.CommandQuit {
				t.Fatalf("Lookup(normal, %q, q) = %+v, want the built-in default app.quit (local-only shell override must be stripped, with no global entry to fall back to)", p.column, result)
			}
			if len(cfg.Notices) != 1 {
				t.Fatalf("cfg.Notices = %v, want exactly one notice for the single stripped keymap binding", cfg.Notices)
			}
			if !strings.Contains(cfg.Notices[0], "1 keymap shell binding(s)") {
				t.Fatalf("cfg.Notices[0] = %q, want it to read %q", cfg.Notices[0], "1 keymap shell binding(s)")
			}
		})
	}
}

// --- Case: case-insensitive column match -- global declares the column as
// "Refined", local declares the byte-identical binding under "refined". Not
// stripped, no notice: the comparator must resolve the matching global
// column case-insensitively, mirroring stripShellFromActions'
// columnsByNameLower/mergeKeymaps' globalColumnsByLower. ---

func TestLoad_Untrusted_KeymapColumnCaseInsensitiveMatch_IdenticalNotStrippedNoNotice(t *testing.T) {
	globalYAML := `
keymaps:
  columns:
    Refined:
      b: { name: Build, type: shell, scope: board, command: make }
`
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: refined
keymaps:
  columns:
    refined:
      b: { name: Build, type: shell, scope: board, command: make }
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	result := km.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("b")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Command != "make" {
		t.Fatalf("Lookup(normal, Refined, b) = %+v, want the case-insensitively-matched, content-identical local binding to remain effective", result)
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: local column %q is content-identical to global column %q once matched case-insensitively", cfg.Notices, "refined", "Refined")
	}
}

// --- Case: an identical local binding co-resident with an explicit unbind
// AND other global-only keys in the same mode/column table. Verifies the
// comparator only touches the shell entry it actually needs to compare
// (never wrongly deletes/replaces the whole table when nothing was
// stripped) and that non-shell constructs (an explicit ~ unbind here) stay
// untouched alongside it. ---

func TestLoad_Untrusted_KeymapIdenticalBinding_CoResidentUnbindAndGlobalOnlyKeysResolve(t *testing.T) {
	for _, p := range keymapCasePlacements() {
		t.Run(p.name, func(t *testing.T) {
			globalYAML := p.embed(buildShellEntry, "y: app.help", "z: card.delete")
			localYAML := "provider: github\nrepo: owner/repo\n" + p.embed(buildShellEntry, "y: ~")

			globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)
			cfg, err := Load(globalPath, localPath, Trust{})
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			km, err := ResolveKeymap(&cfg)
			if err != nil {
				t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
			}

			resultB := km.Lookup(keymap.ModeNormal, p.column, keymap.Sequence{keymap.Key("b")})
			if resultB.Outcome != keymap.OutcomeMatch || resultB.Binding.Kind != keymap.BindingAction || resultB.Binding.Action.Command != "make" {
				t.Fatalf("Lookup(normal, %q, b) = %+v, want the identical shell binding to remain effective", p.column, resultB)
			}
			resultY := km.Lookup(keymap.ModeNormal, p.column, keymap.Sequence{keymap.Key("y")})
			if resultY.Outcome == keymap.OutcomeMatch {
				t.Fatalf("Lookup(normal, %q, y) = %+v, want no binding: the explicit local unbind must survive untouched, not be overridden by global's app.help", p.column, resultY)
			}
			resultZ := km.Lookup(keymap.ModeNormal, p.column, keymap.Sequence{keymap.Key("z")})
			if resultZ.Outcome != keymap.OutcomeMatch || resultZ.Binding.Kind != keymap.BindingCommand || resultZ.Binding.Command != keymap.CommandCardDelete {
				t.Fatalf("Lookup(normal, %q, z) = %+v, want the global-only card.delete key to still resolve: the table must not be wrongly deleted/replaced when nothing needed stripping", p.column, resultZ)
			}
			if len(cfg.Notices) != 0 {
				t.Fatalf("cfg.Notices = %v, want empty: nothing in this table actually differs from global", cfg.Notices)
			}
		})
	}
}

// --- Case (Q3 strippedAny guard): an untrusted local mode/column table
// declared explicitly empty (keymaps: {normal: {}}) has nothing to strip,
// so it must retain the SAME "explicit empty means no bindings, don't
// inherit global" semantics a trusted load of the identical bytes would
// resolve to -- not be treated as "never declared" (which would wrongly
// inherit the whole global table). ---

func TestLoad_Untrusted_KeymapExplicitEmptyModeTable_MatchesTrustedNoInheritance(t *testing.T) {
	globalYAML := `
keymaps:
  normal:
    b: app.help
    z: card.delete
`
	localYAML := `
provider: github
repo: owner/repo
keymaps:
  normal: {}
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	untrusted, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() (untrusted) returned unexpected error: %v", err)
	}
	trusted, err := Load(globalPath, localPath, Trust{Trusted: []TrustEntry{{Hash: mustHashLocal(t, localPath)}}})
	if err != nil {
		t.Fatalf("Load() (trusted) returned unexpected error: %v", err)
	}

	kmUntrusted, err := ResolveKeymap(&untrusted)
	if err != nil {
		t.Fatalf("ResolveKeymap() (untrusted) returned unexpected error: %v", err)
	}
	kmTrusted, err := ResolveKeymap(&trusted)
	if err != nil {
		t.Fatalf("ResolveKeymap() (trusted) returned unexpected error: %v", err)
	}

	resultTrustedB := kmTrusted.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	if resultTrustedB.Outcome == keymap.OutcomeMatch && resultTrustedB.Binding.Kind == keymap.BindingCommand && resultTrustedB.Binding.Command == keymap.CommandHelp {
		t.Fatalf("trusted Lookup(normal, \"\", b) = %+v, want NO binding: an explicit local keymaps.normal: {} declares no bindings and must not inherit global's app.help", resultTrustedB)
	}

	assertIdenticalLookup(t, kmUntrusted, kmTrusted, keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	assertIdenticalLookup(t, kmUntrusted, kmTrusted, keymap.ModeNormal, "", keymap.Sequence{keymap.Key("z")})

	if len(untrusted.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: an explicitly-empty local keymap table stripped nothing, so it must not raise a strip notice either", untrusted.Notices)
	}
}

func TestLoad_Untrusted_KeymapExplicitEmptyColumnTable_MatchesTrustedNoInheritance(t *testing.T) {
	globalYAML := `
keymaps:
  columns:
    Refined:
      b: app.help
      z: card.delete
`
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
keymaps:
  columns:
    Refined: {}
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	untrusted, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() (untrusted) returned unexpected error: %v", err)
	}
	trusted, err := Load(globalPath, localPath, Trust{Trusted: []TrustEntry{{Hash: mustHashLocal(t, localPath)}}})
	if err != nil {
		t.Fatalf("Load() (trusted) returned unexpected error: %v", err)
	}

	kmUntrusted, err := ResolveKeymap(&untrusted)
	if err != nil {
		t.Fatalf("ResolveKeymap() (untrusted) returned unexpected error: %v", err)
	}
	kmTrusted, err := ResolveKeymap(&trusted)
	if err != nil {
		t.Fatalf("ResolveKeymap() (trusted) returned unexpected error: %v", err)
	}

	resultTrustedB := kmTrusted.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("b")})
	if resultTrustedB.Outcome == keymap.OutcomeMatch && resultTrustedB.Binding.Kind == keymap.BindingCommand && resultTrustedB.Binding.Command == keymap.CommandHelp {
		t.Fatalf("trusted Lookup(normal, Refined, b) = %+v, want NO binding: an explicit local keymaps.columns.Refined: {} declares no bindings and must not inherit global's app.help", resultTrustedB)
	}

	assertIdenticalLookup(t, kmUntrusted, kmTrusted, keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("b")})
	assertIdenticalLookup(t, kmUntrusted, kmTrusted, keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("z")})

	if len(untrusted.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: an explicitly-empty local column keymap table stripped nothing, so it must not raise a strip notice either", untrusted.Notices)
	}
}
