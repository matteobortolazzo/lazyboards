package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// This file is the registry dispatch seam for the app's command-panel modes
// (#511): the git menu (this PR, PR 1/2), and the dispatch/help modals (PR
// 2/2). panelEntries/panelBinding/panelHintKey below are the shared,
// mode-generic building blocks every command-panel mode dispatches through;
// everything under "Git panel" is git-panel-specific.

// panelEntries returns every resolved (sequence, binding) entry for mode's
// global table -- command-panel modes are never column-scoped, so this is
// always b.keys.Entries(mode, "").
func (b Board) panelEntries(mode keymap.Mode) []keymap.Entry {
	return b.keys.Entries(mode, "")
}

// panelBinding resolves msg against mode's table with single-key exact-match
// semantics only: an OutcomePending prefix or OutcomeNoMatch both report not
// found. Command-panel modes dispatch one key at a time (lazygit-style
// direct keys, plus j/k/enter/esc) -- unlike normal-mode/detail-panel's
// multi-key custom-action sequences (keymap_dispatch.go), they never
// accumulate a pending sequence.
func (b Board) panelBinding(mode keymap.Mode, msg tea.KeyMsg) (keymap.Binding, bool) {
	result := b.keys.Lookup(mode, "", keymap.Sequence{keymap.Key(msg.String())})
	if result.Outcome != keymap.OutcomeMatch {
		return keymap.Binding{}, false
	}
	return result.Binding, true
}

// panelHintKey picks one display key per id in entries and joins them with
// "/", skipping any id with nothing bound (returns "" when none of ids
// resolve to a key). Per id, candidates are narrowed in three steps: multi-
// key sequences are filtered out entirely (a command-panel hint bar only
// ever shows single keys); an arrow-key alias (up/down/left/right) is
// suppressed in favor of a non-alias key when at least one is also bound
// (mirroring keymap_dispatch.go's arrowAliasKeys use in commandHintKeys);
// and finally, among what remains, a named (multi-rune) key is preferred
// over a single-rune key (e.g. "esc" over "q"), matching the git panel's own
// default table's mnemonic-key convention.
func panelHintKey(entries []keymap.Entry, ids ...keymap.CommandID) string {
	var parts []string
	for _, id := range ids {
		if key := bestPanelHintKey(entries, id); key != "" {
			parts = append(parts, key)
		}
	}
	return strings.Join(parts, "/")
}

// bestPanelHintKey applies panelHintKey's per-id candidate narrowing (see
// its doc comment) and returns the single best display key for id, or "" if
// nothing in entries resolves to it.
func bestPanelHintKey(entries []keymap.Entry, id keymap.CommandID) string {
	var candidates []string
	for _, e := range entries {
		if e.Binding.Kind != keymap.BindingCommand || e.Binding.Command != id {
			continue
		}
		if strings.Contains(e.Sequence, " ") {
			continue
		}
		candidates = append(candidates, e.Sequence)
	}
	if len(candidates) == 0 {
		return ""
	}

	nonAlias := make([]string, 0, len(candidates))
	for _, k := range candidates {
		if !arrowAliasKeys[k] {
			nonAlias = append(nonAlias, k)
		}
	}
	if len(nonAlias) > 0 {
		candidates = nonAlias
	}

	best := candidates[0]
	for _, k := range candidates[1:] {
		if isNamedPanelKey(k) && !isNamedPanelKey(best) {
			best = k
		}
	}
	return best
}

// isNamedPanelKey reports whether s is a named (multi-rune) key label like
// "esc" rather than a single-rune key like "q".
func isNamedPanelKey(s string) bool {
	return len([]rune(s)) > 1
}

// --- Git panel (#511 PR 1/2) ---

// gitPanelHints derives the git panel's status-bar hint bar from the active
// registry: byte-identical to the pre-#511 gitPanelModeHints for the default
// table, and automatically reflecting a keymaps.git_panel remap/unbind.
func (b Board) gitPanelHints() []Hint {
	entries := b.panelEntries(keymap.ModeGitPanel)
	return []Hint{
		{Key: panelHintKey(entries, keymap.CommandGitPanelClose), Desc: "Cancel"},
		{Key: panelHintKey(entries, keymap.CommandGitPanelNext, keymap.CommandGitPanelPrev), Desc: "Navigate"},
		{Key: panelHintKey(entries, keymap.CommandGitPanelRun), Desc: "Run"},
	}
}

// gitPanelItemsFromKeymap builds the git menu's item list from the
// git_panel keymap table: every BindingAction entry (the six built-ins,
// plus any user-declared keymaps.git_panel inline action, minus any
// explicitly unbound key), ordered by (Action.Order, Sequence) via
// sortByActionOrder -- the built-ins carry Order 1..6, matching
// gitPanelBuiltinOrder's legacy fixed display/dispatch order. A row bound to
// a multi-key sequence (e.g. "g d") stays in the list -- it's still
// reachable via j/k navigation + Enter -- but its displayed key label is
// blanked, since panelBinding is single-key exact match only (see its doc
// comment) and can never dispatch that sequence directly; this mirrors
// bestPanelHintKey's multi-key filtering for the hint bar so the menu never
// advertises a key press that silently no-ops.
func (b Board) gitPanelItemsFromKeymap() []gitPanelItem {
	entries := actionOnlyEntries(b.panelEntries(keymap.ModeGitPanel))
	sortByActionOrder(entries)

	items := make([]gitPanelItem, 0, len(entries))
	for _, e := range entries {
		act := configActionFromKeymap(e.Binding.Action)
		key := e.Sequence
		if strings.Contains(key, " ") {
			key = ""
		}
		items = append(items, gitPanelItem{key: key, name: act.Name, action: act})
	}
	return items
}

// runGitPanelCommand runs the built-in git panel command id resolves to.
// Case bodies are transcribed verbatim from the pre-#511 handleGitPanelKey
// (mode_handlers.go), guard for guard.
func (b Board) runGitPanelCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandQuit:
		return b, tea.Quit
	case keymap.CommandGitPanelClose:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case keymap.CommandGitPanelRun:
		if len(b.gitPanel.items) == 0 || b.gitPanel.cursor >= len(b.gitPanel.items) {
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			return b, nil
		}
		return b.closeGitMenuAndDispatch(b.gitPanel.items[b.gitPanel.cursor].action)
	case keymap.CommandGitPanelNext:
		b.gitPanel.cursor = moveCursor(b.gitPanel.cursor, len(b.gitPanel.items), true)
		return b, nil
	case keymap.CommandGitPanelPrev:
		b.gitPanel.cursor = moveCursor(b.gitPanel.cursor, len(b.gitPanel.items), false)
		return b, nil
	}
	return b, nil
}
