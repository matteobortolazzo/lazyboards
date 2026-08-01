package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// This file is the registry dispatch seam for the list-style modals (#490):
// filter, assign, pr_picker (PR 7a), plus pr_list, milestone_list, and
// agent_list (PR 7b). It mirrors keymap_dispatch.go's (#489) normal/detail
// seam -- lookupModalBinding is the single-key equivalent of dispatchKey's
// Lookup call (column is always "": none of these modals resolve against the
// per-column overlay) -- but these modals have no multi-key pending-sequence/
// which-key machinery (Q4), so their handlers just switch on the resolved
// Outcome/Command directly instead of going through
// dispatchBinding/handlePendingSeqKey.

// lookupModalBinding resolves a single fresh key press against mode with no
// column overlay (column: "") -- every one of these six modal surfaces sees
// only the global table, never a per-column rebind.
func (b Board) lookupModalBinding(mode keymap.Mode, msg tea.KeyMsg) keymap.Result {
	seq := keymap.Sequence{keymap.Key(msg.String())}
	return b.keys.Lookup(mode, "", seq)
}

// --- Filter mode hints ---

// filterHintSpecs curates the filter-picker modal's status-bar hints,
// transcribed from the deleted filterModeHints var.
var filterHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandFilterClose}},
	{desc: "Navigate", commands: []keymap.CommandID{keymap.CommandFilterNext, keymap.CommandFilterPrev}, grouped: true},
	{desc: "Select", commands: []keymap.CommandID{keymap.CommandFilterSelect}},
}

// filterHints derives the filter-picker modal's status-bar hints from the
// active keymap, so a remap/unbind is reflected automatically.
func (b Board) filterHints() []Hint {
	entries := b.keys.Entries(keymap.ModeFilter, "")
	return builtinHints(entries, filterHintSpecs)
}

// --- Assign mode hints ---

// assignHintSpecs curates the assignee-picker modal's status-bar hints,
// transcribed from the deleted assignModeHints var.
var assignHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandAssignClose}},
	{desc: "Navigate", commands: []keymap.CommandID{keymap.CommandAssignNext, keymap.CommandAssignPrev}, grouped: true},
	{desc: "Toggle", commands: []keymap.CommandID{keymap.CommandAssignToggle}},
}

// assignHints derives the assignee-picker modal's status-bar hints from the
// active keymap.
func (b Board) assignHints() []Hint {
	entries := b.keys.Entries(keymap.ModeAssign, "")
	return builtinHints(entries, assignHintSpecs)
}

// --- PR picker mode hints ---

// prPickerHintSpecs curates the PR picker modal's non-Cycle status-bar
// hints (Select, Cancel), transcribed from the deleted prPickerHints var.
// Cycle is built separately by prPickerCycleHint because it needs the
// arrow-glyph display-label substitution (Q3), which builtinHints/hintSpec
// has no notion of.
var prPickerHintSpecs = []hintSpec{
	{desc: "Select", commands: []keymap.CommandID{keymap.CommandPRPickerSelect}},
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandPRPickerClose}},
}

// prPickerArrowGlyphs maps the default PR picker's raw key names to the
// arrow glyphs the pre-registry prPickerHints var rendered (Q3): keyed on
// the literal key name, not on the command id, so remapping Cycle onto keys
// outside this map (e.g. "h"/"l") falls back to the raw key text instead of
// a stale/misleading glyph.
var prPickerArrowGlyphs = map[string]string{
	"left":  "◀",
	"right": "▶",
	"up":    "↑",
	"down":  "↓",
}

// prPickerGlyphOrKey returns key's arrow glyph if it has one, the raw key
// text otherwise.
func prPickerGlyphOrKey(key string) string {
	if glyph, ok := prPickerArrowGlyphs[key]; ok {
		return glyph
	}
	return key
}

// prPickerCycleHint builds the PR picker's "Cycle" hint from entries' pr_picker.prev/
// pr_picker.next bindings, substituting arrow glyphs per Q3. It reports
// false when either command has no bound key left (nothing to advertise).
func prPickerCycleHint(entries []keymap.Entry) (Hint, bool) {
	prevKeys := commandHintKeys(entries, keymap.CommandPRPickerPrev)
	nextKeys := commandHintKeys(entries, keymap.CommandPRPickerNext)
	if len(prevKeys) == 0 || len(nextKeys) == 0 {
		return Hint{}, false
	}
	key := prPickerGlyphOrKey(prevKeys[0]) + "/" + prPickerGlyphOrKey(nextKeys[0])
	return Hint{Key: key, Desc: "Cycle"}, true
}

// prPickerHints derives the PR picker modal's status-bar hints from the
// active keymap: Cycle (with glyph substitution) first, then the curated
// Select/Cancel specs.
func (b Board) prPickerHints() []Hint {
	entries := b.keys.Entries(keymap.ModePRPicker, "")
	var hints []Hint
	if cycle, ok := prPickerCycleHint(entries); ok {
		hints = append(hints, cycle)
	}
	hints = append(hints, builtinHints(entries, prPickerHintSpecs)...)
	return hints
}

// --- PR list mode hints (#490 PR 7b) ---

// prListHintSpecs curates the global PR list modal's base status-bar hints,
// transcribed from the deleted prListModeHints var. Inline scope: pr action
// hints are appended separately by prListHints, since they come from
// arbitrary user-bound keys rather than a fixed command id.
var prListHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandPRListClose}},
	{desc: "Navigate", commands: []keymap.CommandID{keymap.CommandPRListNext, keymap.CommandPRListPrev}, grouped: true},
	{desc: "Open", commands: []keymap.CommandID{keymap.CommandPRListOpen}},
}

// prListHints derives the PR list modal's full status-bar hints: the curated
// base navigation hints plus one named hint per scope: pr inline action bound
// under keymaps.pr_list, ordered by (Action.Order, Sequence) via
// sortByActionOrder -- mirroring how normal mode surfaces custom-action
// hints. Only scope: pr actions are hinted, matching the Q2 gate
// handlePRListModeKey enforces on dispatch: hinting any other scope would
// advertise a key that silently no-ops inside this modal. A multi-key-bound
// inline action is excluded too: the PR list has no pending-sequence/
// which-key machinery (Q4 -- a multi-key sequence resolves to
// OutcomePending and is an explicit no-op here), so hinting one would
// advertise a key combo that silently does nothing if pressed.
func (b Board) prListHints() []Hint {
	entries := b.keys.Entries(keymap.ModePRList, "")
	hints := builtinHints(entries, prListHintSpecs)

	prScoped := make([]keymap.Entry, 0, len(entries))
	for _, e := range actionOnlyEntries(entries) {
		if strings.Contains(e.Sequence, " ") {
			continue
		}
		if config.DefaultScope(e.Binding.Action.Scope) == "pr" {
			prScoped = append(prScoped, e)
		}
	}
	sortByActionOrder(prScoped)
	for _, e := range prScoped {
		hints = append(hints, Hint{Key: e.Sequence, Desc: sanitizeSingleLine(e.Binding.Action.Name)})
	}
	return hints
}

// --- Milestones list mode hints (#490 PR 7b) ---

// milestoneListHintSpecs curates the Milestones modal's status-bar hints,
// transcribed from the deleted milestoneListModeHints var.
var milestoneListHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandMilestoneListClose}},
	{desc: "Navigate", commands: []keymap.CommandID{keymap.CommandMilestoneListNext, keymap.CommandMilestoneListPrev}, grouped: true},
	{desc: "Filter board", commands: []keymap.CommandID{keymap.CommandMilestoneListFilter}},
	{desc: "Open in browser", commands: []keymap.CommandID{keymap.CommandMilestoneListOpen}},
}

// milestoneListHints derives the Milestones modal's status-bar hints from the
// active keymap.
func (b Board) milestoneListHints() []Hint {
	entries := b.keys.Entries(keymap.ModeMilestoneList, "")
	return builtinHints(entries, milestoneListHintSpecs)
}

// --- Agents list mode hints (#490 PR 7b) ---

// agentListHintSpecs curates the agents list modal's status-bar hints for the
// loaded state (there are rows to act on), transcribed from the deleted
// agentListModeHints var.
var agentListHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandAgentListClose}},
	{desc: "Navigate", commands: []keymap.CommandID{keymap.CommandAgentListNext, keymap.CommandAgentListPrev}, grouped: true},
	{desc: "Go to window", commands: []keymap.CommandID{keymap.CommandAgentListGoToWindow}},
}

// agentListEmptyHintSpecs curates the agents list modal's status-bar hints
// for its empty/unavailable states, transcribed from the deleted
// agentListEmptyHints var: enter and j/k are no-ops there, so hinting them
// would advertise keys that silently do nothing.
var agentListEmptyHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandAgentListClose}},
}

// agentListHints derives the agents list modal's loaded-state status-bar
// hints from the active keymap.
func (b Board) agentListHints() []Hint {
	entries := b.keys.Entries(keymap.ModeAgentList, "")
	return builtinHints(entries, agentListHintSpecs)
}

// agentListEmptyHints derives the agents list modal's empty/unavailable-state
// status-bar hints from the active keymap.
func (b Board) agentListEmptyHints() []Hint {
	entries := b.keys.Entries(keymap.ModeAgentList, "")
	return builtinHints(entries, agentListEmptyHintSpecs)
}
