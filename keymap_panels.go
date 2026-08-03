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
// everything under "Git panel" is git-panel-specific. #557 extends the seam
// with the error mode ("Error mode" section at the end of this file).

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

// --- Dispatch modal (#511 PR 2/2) ---

// dispatchModalHints derives the dispatch modal's status-bar hint bar from
// the active ModeDispatch registry table and dispatch state, mirroring the
// pre-#511 inline []Hint construction in viewDispatchModal case-by-case. The
// hint matrix only inspects b.dispatch's own busy/confirm/enrolled fields,
// never viewDispatchModal's separately-computed dispatchLoopSource() result.
// Only the keys come from the registry; the Desc strings stay hardcoded
// (#511's Assumption -- catalogue descs are #492's job).
func (b Board) dispatchModalHints() []Hint {
	entries := b.panelEntries(keymap.ModeDispatch)

	switch {
	case b.dispatch.loading, b.dispatch.running, b.dispatch.err != "", b.dispatch.repo == "":
		return []Hint{{Key: panelHintKey(entries, keymap.CommandDispatchClose), Desc: "Close"}}
	case b.dispatch.confirmingLoop:
		return []Hint{
			{Key: panelHintKey(entries, keymap.CommandDispatchConfirmLoop), Desc: "Confirm"},
			{Key: panelHintKey(entries, keymap.CommandDispatchCancelLoop, keymap.CommandDispatchClose), Desc: "Cancel"},
		}
	default:
		enterDesc := "Enroll"
		if b.dispatch.enrolled {
			enterDesc = "Unenroll"
		}
		hints := []Hint{
			{Key: panelHintKey(entries, keymap.CommandDispatchClose), Desc: "Close"},
			{Key: panelHintKey(entries, keymap.CommandDispatchToggleEnroll), Desc: enterDesc},
		}
		if b.dispatch.enrolled {
			// A hint bar must never advertise a key that silently no-ops
			// (docs/view-state-consistency.md): omit the affordance entirely
			// once CommandDispatchOnce has no bound key left, rather than
			// render it with a blank Key.
			if key := panelHintKey(entries, keymap.CommandDispatchOnce); key != "" {
				hints = append(hints, Hint{Key: key, Desc: "Dispatch once"})
			}
		}
		return hints
	}
}

// runDispatchCommand runs the dispatch-modal command id resolves to. Case
// bodies are transcribed verbatim from the pre-#511 handleDispatchModeKey
// (mode_handlers.go), guard for guard, including the confirmingLoop
// commit-step re-check documented in docs/view-state-consistency.md: a
// busy/error state or a nil loop can arrive between opening the confirm
// (gated on !busy) and confirming, so CommandDispatchConfirmLoop repeats
// those same guards before firing toggleLoopCmd.
func (b Board) runDispatchCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandDispatchConfirmLoop:
		if !b.dispatch.confirmingLoop {
			return b, nil
		}
		b.dispatch.confirmingLoop = false
		if b.dispatch.loading || b.dispatch.err != "" || b.dispatch.running {
			return b, nil
		}
		// Defense in depth: repo == "" means viewDispatchModal never rendered
		// the loop-toggle affordance or the blast-radius confirm text in the
		// first place, so confirmingLoop should never be true here -- but
		// mirror CommandDispatchToggleEnroll's guard in case it somehow was.
		if b.dispatch.repo == "" {
			return b, nil
		}
		loop, _ := b.dispatchLoopSource()
		if loop == nil {
			return b, nil
		}
		b.dispatch.loading = true
		return b, toggleLoopCmd(b.executor, loop.Enabled)
	case keymap.CommandDispatchCancelLoop:
		if !b.dispatch.confirmingLoop {
			return b, nil
		}
		b.dispatch.confirmingLoop = false
		return b, nil
	case keymap.CommandDispatchClose:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case keymap.CommandDispatchToggleEnroll:
		if b.dispatch.loading || b.dispatch.err != "" || b.dispatch.running {
			return b, nil
		}
		if b.dispatch.repo == "" {
			return b, nil
		}
		b.dispatch.loading = true
		return b, toggleEnrollCmd(b.executor, b.dispatch.enrolled)
	case keymap.CommandDispatchOnce:
		if b.dispatch.loading || b.dispatch.err != "" || b.dispatch.running {
			return b, nil
		}
		if !b.dispatch.enrolled {
			return b, nil
		}
		b.dispatch.running = true
		return b, dispatchOnceCmd(b.executor)
	case keymap.CommandDispatchToggleLoop:
		if b.dispatch.loading || b.dispatch.err != "" || b.dispatch.running {
			return b, nil
		}
		// viewDispatchModal doesn't render the loop-toggle affordance or the
		// blast-radius confirm text in a repo-less state, so this must be a
		// no-op here too -- otherwise 'l' then 'y' could silently start/commit
		// the fleet-wide confirm flow with no visible warning on screen.
		if b.dispatch.repo == "" {
			return b, nil
		}
		// The loop is fleet-wide and not tied to this repo's enrollment, but a
		// toggle needs a known current state to pick a direction. An old cenci
		// binary (nil loop) can't report that, so there is nothing to toggle.
		if loop, _ := b.dispatchLoopSource(); loop == nil {
			return b, nil
		}
		b.dispatch.confirmingLoop = true
		return b, nil
	}
	return b, nil
}

// handleDispatchModalKey routes a key press in dispatchMode through the
// ModeDispatch registry. A pending loop-toggle confirmation scopes every key
// to CommandDispatchConfirmLoop/CommandDispatchCancelLoop only -- mirroring
// the pre-#511 handler, every other key (including a table entry resolving
// to some other command) is a no-op while confirming. Esc cancels the
// CONFIRM only: it is checked by literal key type, ahead of and independent
// of any table lookup (including a user override unbinding/remapping esc),
// so it can never fall through to the table's esc -> CommandDispatchClose
// binding and close the whole modal -- the ticket's named regression risk
// (docs/view-state-consistency.md).
func (b Board) handleDispatchModalKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if b.dispatch.confirmingLoop {
		if msg.Type == tea.KeyEscape {
			return b.runDispatchCommand(keymap.CommandDispatchCancelLoop)
		}
		binding, ok := b.panelBinding(keymap.ModeDispatch, msg)
		if !ok || binding.Kind != keymap.BindingCommand {
			return b, nil
		}
		switch binding.Command {
		case keymap.CommandDispatchConfirmLoop, keymap.CommandDispatchCancelLoop:
			return b.runDispatchCommand(binding.Command)
		case keymap.CommandDispatchClose:
			// CommandDispatchClose is a cancel-only alias while confirming: a
			// hint bar must never advertise a key that silently no-ops
			// (docs/view-state-consistency.md), and dispatchModalHints'
			// confirmingLoop case advertises whatever key resolves to Close as
			// "Cancel" (composited alongside CancelLoop). If a user config
			// rebinds dispatch.close off the literal Escape key, that other
			// key must still cancel the confirm ONLY, not close the whole
			// modal -- literal Escape already gets this via the check above;
			// this generalizes it to the table-resolved Close command too.
			return b.runDispatchCommand(keymap.CommandDispatchCancelLoop)
		}
		return b, nil
	}

	binding, ok := b.panelBinding(keymap.ModeDispatch, msg)
	if !ok || binding.Kind != keymap.BindingCommand {
		return b, nil
	}
	return b.runDispatchCommand(binding.Command)
}

// --- Help modal (#511 PR 2/2) ---

// helpHints derives the help modal's status-bar hint bar from the active
// ModeHelp registry table, reusing panelHintKey's named-first/"/"-joined
// picking (shared with the git panel and dispatch modal).
func (b Board) helpHints() []Hint {
	entries := b.panelEntries(keymap.ModeHelp)
	return []Hint{
		{Key: panelHintKey(entries, keymap.CommandHelpClose), Desc: "Close"},
		{Key: panelHintKey(entries, keymap.CommandHelpScrollDown, keymap.CommandHelpScrollUp), Desc: "Scroll"},
	}
}

// runHelpCommand runs the help-modal command id resolves to. Case bodies are
// transcribed verbatim from the pre-#511 handleHelpModeKey (mode_handlers.go).
func (b Board) runHelpCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandQuit:
		return b, tea.Quit
	case keymap.CommandHelpClose:
		b.closeHelp()
		return b, nil
	case keymap.CommandHelpScrollDown:
		maxOffset := b.helpMaxScrollOffset()
		if b.helpScrollOffset < maxOffset {
			b.helpScrollOffset++
		}
		return b, nil
	case keymap.CommandHelpScrollUp:
		if b.helpScrollOffset > 0 {
			b.helpScrollOffset--
		}
		return b, nil
	}
	return b, nil
}

// --- Error mode (#557) ---

// errorHintSpecs curates the error mode's status-bar hints -- Retry, Quit --
// transcribed from the pre-#557 inline []Hint literal in update.go's
// boardFetchErrorMsg handler.
var errorHintSpecs = []hintSpec{
	{desc: "Retry", commands: []keymap.CommandID{keymap.CommandErrorRetry}},
	{desc: "Quit", commands: []keymap.CommandID{keymap.CommandQuit}},
}

// errorHints derives the error mode's status-bar hint bar from the active
// ModeError registry table, reusing builtinHints (shared with every other
// command-panel mode in this file).
func (b Board) errorHints() []Hint {
	return builtinHints(b.panelEntries(keymap.ModeError), errorHintSpecs)
}

// runErrorCommand runs the error-mode command id resolves to. Case bodies
// are transcribed verbatim from the pre-#557 errorMode switch msg.String()
// (update.go), guard for guard.
func (b Board) runErrorCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandQuit:
		return b, tea.Quit
	case keymap.CommandErrorRetry:
		b.mode = loadingMode
		b.loadErr = ""
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider, true))
	default:
		return b, nil
	}
}
