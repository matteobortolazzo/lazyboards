package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// This file is the registry dispatch seam for the app's text-prompt confirm
// modes (#539): close_confirm (this PR, PR 1/2) and label_confirm (this PR,
// PR 1/2), plus delete (PR 2/2). textBinding/singleKeyEntries/textHintKey/
// promptKeySuffix below are the shared, mode-generic building blocks these
// modes dispatch through and render their "(y/n)"-style prompts from,
// mirroring keymap_panels.go's panelEntries/panelBinding/panelHintKey shape
// for command-panel modes.

// textBinding resolves msg against mode's table with single-key exact-match
// semantics, mirroring panelBinding (keymap_panels.go). It additionally
// guards mode's printable-rune passthrough: when mode.ConsumesPrintableRunes()
// is true and msg is a literal, non-Alt rune, textBinding reports not found
// so the mode's textinput can own the keystroke instead of the registry
// intercepting it as a single-key command. This is defense-in-depth --
// internal/config/keymap_semantic_validate.go already rejects bare
// printable-rune bindings in these modes at config-load time -- but keeps
// textBinding safe to reuse even before that validation has run.
func (b Board) textBinding(mode keymap.Mode, msg tea.KeyMsg) (keymap.Binding, bool) {
	if mode.ConsumesPrintableRunes() && msg.Type == tea.KeyRunes && !msg.Alt {
		return keymap.Binding{}, false
	}
	r := b.keys.Lookup(mode, "", keymap.Sequence{keymap.Key(msg.String())})
	if r.Outcome != keymap.OutcomeMatch {
		return keymap.Binding{}, false
	}
	return r.Binding, true
}

// singleKeyEntries drops every multi-key sequence from entries: textBinding
// can never dispatch a multi-key sequence (it's single-key exact-match
// only), so no hint derived for these modes may advertise one either --
// the hint<->dispatch invariant shared with panelHintKey's multi-key
// filtering.
func singleKeyEntries(entries []keymap.Entry) []keymap.Entry {
	filtered := make([]keymap.Entry, 0, len(entries))
	for _, e := range entries {
		if strings.Contains(e.Sequence, " ") {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// textHintKey picks the single best display key bound to each id in
// entries and joins them with "/", skipping any id with nothing bound
// (returns "" when none of ids resolve to a key). It reuses
// commandHintKeys (keymap_dispatch.go) -- shortest-then-alphabetical,
// arrow-alias-suppressed -- rather than panelHintKey's named-key-preferred
// tie-break: today's "(y/n)" text picks the single-rune "n" over the
// also-bound "esc", and that default-parity text must not regress.
func textHintKey(entries []keymap.Entry, ids ...keymap.CommandID) string {
	single := singleKeyEntries(entries)
	var parts []string
	for _, id := range ids {
		keys := commandHintKeys(single, id)
		if len(keys) == 0 {
			continue
		}
		parts = append(parts, keys[0])
	}
	return strings.Join(parts, "/")
}

// promptKeySuffix builds the registry-derived "y/n"-style key text for a
// (confirm-command-id, cancel-command-id) pair, dropping either side that
// has no bound key left and returning "" when neither side is bound -- per
// docs/view-state-consistency.md's "never advertise a key that silently
// no-ops" rule. This is the bare, unwrapped form; promptParenthetical below
// is what view.go's helpBar prompt lines actually call.
func promptKeySuffix(entries []keymap.Entry, ids ...keymap.CommandID) string {
	return textHintKey(entries, ids...)
}

// promptParenthetical wraps promptKeySuffix's result in " (%s)" for
// view.go's helpBar prompt lines, returning "" (no parenthetical at all,
// not even empty parens) when promptKeySuffix reports neither side bound.
func promptParenthetical(entries []keymap.Entry, ids ...keymap.CommandID) string {
	suffix := promptKeySuffix(entries, ids...)
	if suffix == "" {
		return ""
	}
	return " (" + suffix + ")"
}

// --- delete mode (#539 PR 2/2) ---
//
// delete stays one keymap.Mode: enter and esc both resolve to the same two
// command ids (delete.submit, delete.cancel) regardless of which step
// (comment/confirm) the handler is currently in -- only the hint wording
// ("Continue" vs "Confirm") differs per step, not the id. deleteCommentHintSpecs
// and deleteConfirmHintSpecs below are the two step-specific hintSpec lists,
// transcribed from the deleted deleteCommentHints/deleteConfirmHints package
// vars (model.go, pre-#539).

// deleteCommentHintSpecs curates the delete flow's optional-comment-step
// status-bar hints.
var deleteCommentHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandDeleteCancel}},
	{desc: "Continue", commands: []keymap.CommandID{keymap.CommandDeleteSubmit}},
}

// deleteConfirmHintSpecs curates the delete flow's retype-to-confirm-step
// status-bar hints.
var deleteConfirmHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandDeleteCancel}},
	{desc: "Confirm", commands: []keymap.CommandID{keymap.CommandDeleteSubmit}},
}

// deleteCommentHints derives the delete flow's optional-comment-step
// status-bar hints from the active keymap, so a remap/unbind is reflected
// automatically. entries are filtered to single-key sequences (singleKeyEntries)
// since textBinding can never dispatch a multi-key sequence -- the same
// hint<->dispatch invariant textHintKey enforces above. builtinHints
// (keymap_dispatch.go) already omits a hint whose command has no key bound
// left, rather than rendering a blank Key (docs/view-state-consistency.md's
// "never advertise a key that silently no-ops" rule).
func (b Board) deleteCommentHints() []Hint {
	entries := singleKeyEntries(b.keys.Entries(keymap.ModeDelete, ""))
	return builtinHints(entries, deleteCommentHintSpecs)
}

// deleteConfirmHints is deleteCommentHints' retype-to-confirm-step sibling.
func (b Board) deleteConfirmHints() []Hint {
	entries := singleKeyEntries(b.keys.Entries(keymap.ModeDelete, ""))
	return builtinHints(entries, deleteConfirmHintSpecs)
}

// runDeleteCommand runs the delete command id resolves to, branching on
// b.delete.step internally since delete.submit is one id shared by both
// steps. Case bodies are transcribed verbatim from the pre-#539
// handleDeleteModeKey (mode_handlers.go), guard for guard.
func (b Board) runDeleteCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandDeleteCancel:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		cmd := b.statusBar.SetTimedMessage("Delete cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	case keymap.CommandDeleteSubmit:
		switch b.delete.step {
		case deleteStepComment:
			ci := textinput.New()
			ci.Placeholder = strconv.Itoa(b.delete.card.Number)
			ci.CharLimit = 20
			b.delete.step = deleteStepConfirm
			b.delete.confirmInput = ci
			b.delete.mismatchMsg = ""
			b.statusBar.SetActionHints(b.deleteConfirmHints())
			return b, b.delete.confirmInput.Focus()
		case deleteStepConfirm:
			card := b.delete.card
			if b.delete.confirmInput.Value() != strconv.Itoa(card.Number) {
				b.delete.mismatchMsg = fmt.Sprintf("Doesn't match #%d — try again or Esc to cancel", card.Number)
				return b, nil
			}
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			comment := strings.TrimSpace(b.delete.commentInput.Value())
			if comment != "" {
				return b, addCommentForDeleteCmd(b.provider, card, comment)
			}
			return b, deleteCardCmd(b.provider, card, false)
		}
	}
	return b, nil
}
