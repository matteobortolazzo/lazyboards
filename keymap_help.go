package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// This file is the registry dispatch seam for the help modal (#550): it
// replaces the hardcoded helpSections/buildHelpContent table (formerly
// view.go) with a generator that derives every row from the active
// *keymap.Keymap (b.keys.Entries), so the help modal can never advertise a
// stale binding after a remap/unbind. Only section titles/order/openedBy
// stay curated (helpModeSections below) -- every row label is the
// catalogued command's own Desc (keymap.FindCommand), per AC2's "no
// help-only wording table" rule. It sits beside keymap_dispatch.go,
// keymap_modals.go, keymap_panels.go and keymap_text.go, and reuses their
// glyphOrKey/actionOnlyEntries/sortByActionOrder/describeBinding helpers
// rather than reimplementing key-label logic.

// helpModeSectionSpec curates one help section's display metadata: its
// title, the Mode its rows are derived from, and (optionally) the built-in
// command that opens it from Normal Mode. openedBy's row is resolved
// against the *normal-mode* table (Q3) so its key/desc reflect any remap and
// disappears entirely if that normal-mode command is ever unbound --
// exactly the same truthfulness guarantee every other generated row gets.
type helpModeSectionSpec struct {
	title    string
	mode     keymap.Mode
	openedBy keymap.CommandID
}

// helpModeSections is the curated, ordered list of help sections -- one
// entry per keymap.Modes() (19), guarded by the drift test in
// keymap_help_test.go. Only title/mode/openedBy are curated here; every row
// beneath a section is derived from b.keys.Entries(mode, column) at render
// time. When a new mode is added to keymap.Modes(), add its entry here (and
// nowhere else -- no separate wording table to keep in sync).
var helpModeSections = []helpModeSectionSpec{
	{title: "Normal Mode", mode: keymap.ModeNormal},
	{title: "Detail Panel", mode: keymap.ModeDetail},
	{title: "Create Card", mode: keymap.ModeCreate, openedBy: keymap.CommandCardNew},
	{title: "Configuration", mode: keymap.ModeConfig, openedBy: keymap.CommandConfig},
	{title: "PR Picker", mode: keymap.ModePRPicker},
	{title: "Pull Requests", mode: keymap.ModePRList, openedBy: keymap.CommandViewPRList},
	{title: "Milestones", mode: keymap.ModeMilestoneList, openedBy: keymap.CommandViewMilestoneList},
	{title: "Agents (cenci)", mode: keymap.ModeAgentList, openedBy: keymap.CommandViewAgentList},
	{title: "Comment", mode: keymap.ModeComment},
	{title: "Delete", mode: keymap.ModeDelete, openedBy: keymap.CommandCardDelete},
	{title: "Close Confirm", mode: keymap.ModeCloseConfirm, openedBy: keymap.CommandCardClose},
	{title: "Filter", mode: keymap.ModeFilter, openedBy: keymap.CommandBoardFilter},
	{title: "Search", mode: keymap.ModeSearch, openedBy: keymap.CommandBoardSearch},
	{title: "Assign", mode: keymap.ModeAssign, openedBy: keymap.CommandCardAssign},
	{title: "Git Menu", mode: keymap.ModeGitPanel, openedBy: keymap.CommandViewGitPanel},
	{title: "Dispatch (cenci)", mode: keymap.ModeDispatch, openedBy: keymap.CommandViewDispatch},
	{title: "Label Confirm", mode: keymap.ModeLabelConfirm},
	{title: "Help", mode: keymap.ModeHelp},
	{title: "Error", mode: keymap.ModeError},
}

// normalDetailStaticRows are the rows appended to Normal Mode/Detail Panel
// that describe an implicit overload rather than any single catalogued
// command: the Alt+key comment-action overload dispatchActionWithAlt
// implements for every {comment}-templated action. It is not backed by a
// keymap.CommandID, so it stays a curated static row.
//
// #484 dropped the two "A-Z"/"A-Z.." custom-action rows that used to sit
// above it: they advertised the deleted uppercase-is-yours convention,
// which the redesigned defaults contradict (P/A/D/G are built-ins now) and
// which no longer bounds custom actions at all -- any key of either case
// binds either kind. A user's actual bindings need no namespace legend:
// helpActionRows already renders every configured inline action as its own
// row, under its real key.
var normalDetailStaticRows = [][2]string{
	{"alt+shift+key", "Comment action"},
}

// staticHelpRows returns mode's curated static rows (rows with no
// keymap.CommandID backing them), or nil for every mode without any.
func staticHelpRows(mode keymap.Mode) [][2]string {
	if mode == keymap.ModeNormal || mode == keymap.ModeDetail {
		return normalDetailStaticRows
	}
	return nil
}

// helpSectionRows derives every row spec's section renders: the openedBy
// row first (if any -- omitted if its normal-mode command is unbound), then
// built-in command rows, then inline-action rows, then any static rows for
// the mode (mirroring registryHints' built-ins-then-actions composition).
func (b Board) helpSectionRows(spec helpModeSectionSpec) [][2]string {
	var rows [][2]string
	if spec.openedBy != "" {
		normalEntries := b.keys.Entries(keymap.ModeNormal, b.activeColumnTitle())
		if keys := helpKeysForCommand(normalEntries, spec.openedBy); len(keys) > 0 {
			if cmd, ok := keymap.FindCommand(spec.openedBy); ok {
				rows = append(rows, [2]string{strings.Join(keys, "/"), cmd.Desc})
			}
		}
	}

	column := ""
	if spec.mode == keymap.ModeNormal || spec.mode == keymap.ModeDetail {
		column = b.activeColumnTitle()
	}
	entries := b.keys.Entries(spec.mode, column)
	rows = append(rows, helpBuiltinRows(entries)...)
	rows = append(rows, helpActionRows(entries)...)
	rows = append(rows, staticHelpRows(spec.mode)...)
	return rows
}

// helpKeysForCommand returns every key in entries bound to id, glyph-
// substituted (glyphOrKey) and sorted by (rune-length ascending, then
// string ascending) -- the help-only sibling of commandHintKeys (Q1) that
// deliberately skips its arrow-alias suppression: help must list every
// bound key, not just the hint bar's single preferred one.
func helpKeysForCommand(entries []keymap.Entry, id keymap.CommandID) []string {
	var keys []string
	for _, e := range entries {
		if e.Binding.Kind == keymap.BindingCommand && e.Binding.Command == id {
			keys = append(keys, glyphOrKey(e.Sequence))
		}
	}
	sort.SliceStable(keys, func(i, j int) bool {
		li, lj := utf8.RuneCountInString(keys[i]), utf8.RuneCountInString(keys[j])
		if li != lj {
			return li < lj
		}
		return keys[i] < keys[j]
	})
	return keys
}

// commandIDNumericSuffix splits id into (prefix, number, ok) when it ends in
// ASCII digits (e.g. "nav.column_1" -> ("nav.column_", 1, true)) -- a
// structural signal over the CommandID itself (never the desc text, per
// AC2) that helpBuiltinRows uses to detect a "jump to column N" style
// family without a hardcoded id list.
func commandIDNumericSuffix(id keymap.CommandID) (string, int, bool) {
	s := string(id)
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) || i == 0 {
		return "", 0, false
	}
	n, err := strconv.Atoi(s[i:])
	if err != nil {
		return "", 0, false
	}
	return s[:i], n, true
}

// isAscendingDigitRun reports whether every key in keys is exactly one
// ASCII digit and consecutive keys' digit values increase by exactly 1
// (e.g. ["1","2","3"]) -- the signal helpBuiltinRows uses to decide whether
// a numeric-id-suffix run collapses to a "first-last" range label or falls
// back to a plain "/"-joined key list (after a remap breaks the sequence).
func isAscendingDigitRun(keys []string) bool {
	prev := -1
	for _, k := range keys {
		if len(k) != 1 || k[0] < '0' || k[0] > '9' {
			return false
		}
		d := int(k[0] - '0')
		if prev != -1 && d != prev+1 {
			return false
		}
		prev = d
	}
	return true
}

// commonPrefixLabel returns the longest common prefix of descs, trimmed --
// the programmatic label computation Q2 requires so no help-only wording
// (e.g. a hardcoded "Jump to column") is ever introduced.
func commonPrefixLabel(descs []string) string {
	if len(descs) == 0 {
		return ""
	}
	prefix := descs[0]
	for _, d := range descs[1:] {
		for !strings.HasPrefix(d, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	return strings.TrimSpace(prefix)
}

// tryNumericRun attempts to collapse a maximal run of present[start:] whose
// CommandIDs share a numeric-suffix family (commandIDNumericSuffix) with
// consecutively incrementing numbers and exactly one bound key each. It
// returns ok=false (no collapse) for a family of fewer than two members, so
// a lone numeric-suffixed command still falls through to the normal
// identical-desc/single-row handling in helpBuiltinRows.
func tryNumericRun(present []keymap.Command, start int, entries []keymap.Entry) (next int, desc string, keyLabel string, ok bool) {
	prefix, n, isNum := commandIDNumericSuffix(present[start].ID)
	if !isNum {
		return 0, "", "", false
	}

	end := start
	expected := n
	var keys []string
	var descs []string
	for end < len(present) {
		p, num, isN := commandIDNumericSuffix(present[end].ID)
		if !isN || p != prefix || num != expected {
			break
		}
		ck := helpKeysForCommand(entries, present[end].ID)
		if len(ck) != 1 {
			break
		}
		keys = append(keys, ck[0])
		descs = append(descs, present[end].Desc)
		end++
		expected++
	}

	if end-start < 2 {
		return 0, "", "", false
	}

	desc = commonPrefixLabel(descs)
	if desc == "" {
		// The family's catalogue Desc strings share no common prefix (a
		// structurally-detected numeric-suffix family with inconsistent
		// wording). Collapsing here would silently render a blank label
		// (e.g. "1-9    "), so decline the collapse entirely and let
		// helpBuiltinRows' per-command fallback (its own identical-desc
		// merge / one-row-per-command loop) render each member instead.
		return 0, "", "", false
	}
	if isAscendingDigitRun(keys) {
		keyLabel = keys[0] + "-" + keys[len(keys)-1]
	} else {
		keyLabel = strings.Join(keys, "/")
	}
	return end, desc, keyLabel, true
}

// helpBuiltinRows walks keymap.Commands() in catalogue declaration order,
// keeping only the ids actually bound (BindingCommand) in entries, and
// produces one row per command or merged group:
//   - a numeric-id-suffix family (tryNumericRun) collapses to a "first-last"
//     range when its bound keys form an ascending single-digit run, or
//     falls back to a "/"-joined row otherwise (a remap broke the run);
//   - otherwise, adjacent commands sharing an identical catalogue Desc merge
//     into one row, concatenating each command's own helpKeysForCommand
//     result in catalogue order (reproducing "j/k Navigate cards",
//     "tab/shift+tab Switch columns", "h/◀/esc Back to card list" with no
//     curated grouping table);
//   - every other bound command gets its own row, labeled by its catalogue
//     Desc verbatim (AC2).
//
// BindingUnbound and BindingAction entries, and any id with zero bound keys,
// are skipped -- inline actions render separately via helpActionRows.
func helpBuiltinRows(entries []keymap.Entry) [][2]string {
	bound := make(map[keymap.CommandID]bool)
	for _, e := range entries {
		if e.Binding.Kind == keymap.BindingCommand {
			bound[e.Binding.Command] = true
		}
	}

	var present []keymap.Command
	for _, c := range keymap.Commands() {
		if bound[c.ID] {
			present = append(present, c)
		}
	}

	return builtinRowsFromCommands(present, entries)
}

// builtinRowsFromCommands is helpBuiltinRows' present-commands-to-rows walk,
// split out so tests can exercise it against a synthetic present slice (a
// numeric-suffix command family that doesn't exist in the real catalogue) --
// see TestTryNumericRun_NoCommonPrefixDesc_FallsBackToPerCommandRows.
func builtinRowsFromCommands(present []keymap.Command, entries []keymap.Entry) [][2]string {
	var rows [][2]string
	for i := 0; i < len(present); {
		if next, desc, keyLabel, ok := tryNumericRun(present, i, entries); ok {
			rows = append(rows, [2]string{keyLabel, desc})
			i = next
			continue
		}

		j := i + 1
		for j < len(present) && present[j].Desc == present[i].Desc {
			j++
		}
		var keys []string
		for k := i; k < j; k++ {
			keys = append(keys, helpKeysForCommand(entries, present[k].ID)...)
		}
		if len(keys) > 0 {
			rows = append(rows, [2]string{strings.Join(keys, "/"), present[i].Desc})
		}
		i = j
	}
	return rows
}

// helpActionRows renders entries' inline-action bindings (git panel P/p/f/m/
// s/S, a keymaps:-configured action, ...), ordered by sortByActionOrder --
// mirroring registryHints' built-ins-then-actions composition. Name/Type are
// untrusted config-sourced strings, sanitized with sanitizeSingleLine at
// this render site per .claude/rules/security.md (view.go's prior raw
// rendering was the gap).
func helpActionRows(entries []keymap.Entry) [][2]string {
	actions := actionOnlyEntries(entries)
	sortByActionOrder(actions)
	rows := make([][2]string, 0, len(actions))
	for _, e := range actions {
		name := sanitizeSingleLine(e.Binding.Action.Name)
		typ := sanitizeSingleLine(e.Binding.Action.Type)
		rows = append(rows, [2]string{e.Sequence, fmt.Sprintf("%s (%s)", name, typ)})
	}
	return rows
}

// helpColumnActionRows derives the Custom Actions section body: for every
// configured column (b.columnConfigs -- the resolved *keymap.Keymap exposes
// no column list and internal/keymap is off-limits per the epic's
// decisions), it diffs that column's Normal-mode inline-action entries
// against the global (no-column) Normal-mode table and keeps only the ones
// that differ -- a column entry byte-identical to the global binding for the
// same key is excluded (Q4), while a column override of a global key, or a
// column-only key, is included. ModeNormal alone suffices: Resolve builds
// the ModeNormal/ModeDetail column overlays from the same column table.
// Per Q4 the section is column-only and lists every configured column with
// at least one differing binding, including the currently active column
// (whose own overrides also render inline in Normal/Detail via
// helpActionRows). Returns "" when no column has any qualifying row, so
// buildHelpContent omits the header entirely for a global-actions-only (or
// action-free) config.
func (b Board) helpColumnActionRows() string {
	globalEntries := actionOnlyEntries(b.keys.Entries(keymap.ModeNormal, ""))
	globalBySeq := make(map[string]keymap.Binding, len(globalEntries))
	for _, e := range globalEntries {
		globalBySeq[e.Sequence] = e.Binding
	}

	var sb strings.Builder
	any := false
	for _, cc := range b.columnConfigs {
		colEntries := actionOnlyEntries(b.keys.Entries(keymap.ModeNormal, cc.Name))
		sortByActionOrder(colEntries)

		var rows [][2]string
		for _, e := range colEntries {
			if global, ok := globalBySeq[e.Sequence]; ok && global == e.Binding {
				continue
			}
			name := sanitizeSingleLine(e.Binding.Action.Name)
			typ := sanitizeSingleLine(e.Binding.Action.Type)
			rows = append(rows, [2]string{e.Sequence, fmt.Sprintf("%s (%s)", name, typ)})
		}
		if len(rows) == 0 {
			continue
		}

		any = true
		fmt.Fprintf(&sb, "  %s:\n", sanitizeSingleLine(cc.Name))
		for _, kv := range rows {
			fmt.Fprintf(&sb, "    %-10s %s\n", kv[0], kv[1])
		}
	}

	if !any {
		return ""
	}
	return "\nCustom Actions\n" + sb.String()
}

// helpUsageDetailFocusLine renders "  Press l or → to view card details.\n"
// from nav.detail_focus resolved against the effective normal-mode table for
// the active column, or "" when nothing is bound. Keys stay lowercase and
// glyph-substituted via helpKeysForCommand, matching every other help-modal
// row -- with one historical exception: this sentence's shipped default
// wording (AC8, byte-identical output) predates arrowGlyphs and rendered a
// right-bound key as U+2192 "→", not arrowGlyphs' "▶" (the glyph every other
// help row renders for the same nav.detail_focus command, e.g. the Normal
// Mode section's "l/▶" row). Substituting "▶" back to "→" here is what keeps
// this one sentence byte-identical to today; no other help surface gets it.
func (b Board) helpUsageDetailFocusLine() string {
	entries := b.keys.Entries(keymap.ModeNormal, b.activeColumnTitle())
	keys := helpKeysForCommand(entries, keymap.CommandNavDetailFocus)
	if len(keys) == 0 {
		return ""
	}
	rendered := make([]string, len(keys))
	for i, k := range keys {
		if k == arrowGlyphs["right"] {
			k = "→"
		}
		rendered[i] = k
	}
	return "  Press " + strings.Join(rendered, " or ") + " to view card details.\n"
}

// buildHelpContent renders the full help modal body: one section per
// helpModeSections entry (omitted when it has no rows at all -- e.g. every
// binding in the mode was unbound), then the registry-derived Custom
// Actions section (helpColumnActionRows -- omitted entirely for a
// global-actions-only config, since those rows already render inline in
// Normal/Detail via helpActionRows, per Q4), then the static Status Bar and
// Usage appendices.
func (b Board) buildHelpContent() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Help — lazyboards %s\n\n", appVersion())

	first := true
	for _, spec := range helpModeSections {
		rows := b.helpSectionRows(spec)
		if len(rows) == 0 {
			continue
		}
		if !first {
			sb.WriteString("\n")
		}
		first = false
		sb.WriteString(spec.title + "\n")
		for _, kv := range rows {
			fmt.Fprintf(&sb, "  %-12s %s\n", kv[0], kv[1])
		}
	}

	sb.WriteString(b.helpColumnActionRows())

	// Status Bar (always-visible prefix indicators, not keys). Each is
	// omitted when its count is zero at render time (view.go), but the
	// legend itself is always shown.
	sb.WriteString("\nStatus Bar\n")
	fmt.Fprintf(&sb, "  %-12s %s\n", "▶N", "Agents running")
	fmt.Fprintf(&sb, "  %-12s %s\n", "!N", "Agents awaiting input")
	fmt.Fprintf(&sb, "  %-12s %s\n", linkedPRGlyph+"N", "Open PRs in the repository")

	// Usage.
	sb.WriteString("\nUsage\n")
	sb.WriteString("  Columns represent board states (e.g., New, Implementing).\n")
	sb.WriteString(b.helpUsageDetailFocusLine())
	sb.WriteString("  Custom actions are configured in .lazyboards.yml.\n")

	return sb.String()
}
