package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// This file is the registry dispatch seam for the list-style modals (#490,
// PR 7a): filter, assign, and pr_picker. It mirrors keymap_dispatch.go's
// (#489) normal/detail seam -- lookupModalBinding is the single-key
// equivalent of dispatchKey's Lookup call (column is always "": none of
// these modals resolve against the per-column overlay) -- but these modals
// have no multi-key pending-sequence/which-key machinery (Q4), so their
// handlers just switch on the resolved Outcome/Command directly instead of
// going through dispatchBinding/handlePendingSeqKey.
//
// pr_list, milestone_list, and agent_list convert in the follow-up PR 7b.

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
