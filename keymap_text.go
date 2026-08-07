package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

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
	return parenthesize(promptKeySuffix(entries, ids...))
}

// parenthesize wraps s in " (%s)", or returns "" (no parenthetical at all,
// not even empty parens) when s is "". Factored out of promptParenthetical
// so the delete prompts (#583) can reuse the same "omit when empty" wrapping.
func parenthesize(s string) string {
	if s == "" {
		return ""
	}
	return " (" + s + ")"
}

// capitalizeKeyLabel upper-cases the FIRST RUNE of a rendered key label and
// leaves the rest untouched: "esc" -> "Esc", "enter" -> "Enter",
// "shift+tab" -> "Shift+tab", "ctrl+d" -> "Ctrl+d", "/" -> "/", "1" -> "1",
// "◀" -> "◀". It is the delete modal's prose convention only -- the status
// bar, the help modal, and the Help Usage/filter-warning sentences all render
// raw lowercase key text, so this must never be applied there.
func capitalizeKeyLabel(label string) string {
	r, size := utf8.DecodeRuneInString(label)
	if size == 0 {
		return label
	}
	return string(unicode.ToUpper(r)) + label[size:]
}

// joinClauses ", "-joins the non-empty clauses, or returns "" when none are
// non-empty.
func joinClauses(clauses ...string) string {
	var nonEmpty []string
	for _, c := range clauses {
		if c != "" {
			nonEmpty = append(nonEmpty, c)
		}
	}
	return strings.Join(nonEmpty, ", ")
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

// deleteKeyClause renders "<Key> to <verb>" for id from the ModeDelete table,
// or "" when id has no single-key binding left. Capitalized per the delete
// modal's prose convention (capitalizeKeyLabel).
func deleteKeyClause(entries []keymap.Entry, id keymap.CommandID, verb string) string {
	key := textHintKey(entries, id)
	if key == "" {
		return ""
	}
	return capitalizeKeyLabel(key) + " to " + verb
}

// deleteCommentPromptSuffix renders the optional-comment-step prompt's
// trailing parenthetical (" (Enter to continue, Esc to cancel)" under the
// default table), dropping either clause whose command is unbound and
// omitting the parenthetical entirely when neither is bound.
func (b Board) deleteCommentPromptSuffix() string {
	entries := b.keys.Entries(keymap.ModeDelete, "")
	return parenthesize(joinClauses(
		deleteKeyClause(entries, keymap.CommandDeleteSubmit, "continue"),
		deleteKeyClause(entries, keymap.CommandDeleteCancel, "cancel"),
	))
}

// deleteConfirmPromptSuffix renders the retype-to-confirm-step prompt's
// trailing parenthetical (" (Esc to cancel)" under the default table).
// Cancel-only per Q2 -- no "to confirm" clause is added; today's wording is
// preserved byte-for-byte.
func (b Board) deleteConfirmPromptSuffix() string {
	entries := b.keys.Entries(keymap.ModeDelete, "")
	return parenthesize(deleteKeyClause(entries, keymap.CommandDeleteCancel, "cancel"))
}

// deleteMismatchMessage renders the retype-mismatch feedback ("Doesn't match
// #N — try again or Esc to cancel" under the default table), dropping the
// cancel clause entirely once delete.cancel is unbound.
func (b Board) deleteMismatchMessage(number int) string {
	entries := b.keys.Entries(keymap.ModeDelete, "")
	msg := fmt.Sprintf("Doesn't match #%d — try again", number)
	if clause := deleteKeyClause(entries, keymap.CommandDeleteCancel, "cancel"); clause != "" {
		msg += " or " + clause
	}
	return msg
}

// runDeleteCommand runs the delete command id resolves to, branching on
// b.delete.step internally since delete.submit is one id shared by both
// steps. Case bodies are transcribed verbatim from the pre-#539
// handleDeleteModeKey (mode_handlers.go), guard for guard.
func (b Board) runDeleteCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandDeleteCancel:
		b.mode = normalMode
		b.restoreFocusHints()
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
				b.delete.mismatchMsg = b.deleteMismatchMessage(card.Number)
				return b, nil
			}
			b.mode = normalMode
			b.restoreFocusHints()
			comment := strings.TrimSpace(b.delete.commentInput.Value())
			if comment != "" {
				return b, addCommentForDeleteCmd(b.provider, card, comment)
			}
			return b, deleteCardCmd(b.provider, card, false)
		}
	}
	return b, nil
}

// --- create/config (#540 PR 1/2) ---
//
// create and config are both ConsumesPrintableRunes() text-input surfaces
// with a per-instance focus field that gates two of their five commands:
// create.assignee_prev/assignee_next only cycle the assignee at
// b.create.focus == 2, and config.provider_prev/provider_next only cycle
// the provider at b.config.focus == 0. createCommandActive/
// configCommandActive are that fifth eligibility condition, layered onto
// handleCreateModeKey/handleConfigModeKey's textBinding-resolved command
// the same way handleDeleteModeKey's recognized-command predicate is
// layered onto delete.submit/delete.cancel: an eligible command dispatches
// through runCreateCommand/runConfigCommand, anything else (including a
// resolved-but-currently-ineligible cycle command) falls through to the
// mode's own fallback in mode_handlers.go.

// glyphHintKey is textHintKey's arrow-glyph-substituting sibling: for each
// id, it picks a single bound key (preferring an arrow-alias key -- left/
// right/up/down -- when preferArrow is true and one is bound, otherwise
// commandHintKeys' default shortest-then-alphabetical pick), renders it
// through glyphOrKey (keymap_dispatch.go), and joins the results with "/".
// Used by createModalHints' Cycle hint so the default "left"/"right" table
// renders "◀/▶" (byte-identical to today's literal) while a remap onto
// non-arrow keys (e.g. "h"/"l") falls back to the raw key text instead of a
// stale glyph.
//
// preferArrow's arrow-alias lookup scans entries directly rather than
// commandHintKeys' returned keys: commandHintKeys already suppresses an
// arrow-alias key whenever a non-alias key is also bound to the same
// command (its own alias-suppression logic, designed for normal-mode hints
// where the alias is genuinely secondary), so by the time its result
// reaches this function the arrow key may already be gone -- exactly
// search's default up+ctrl+p/down+ctrl+n table. Scanning the unfiltered
// (single-key) entries directly is the only way to see a bound arrow key
// that commandHintKeys has already suppressed away.
func glyphHintKey(entries []keymap.Entry, preferArrow bool, ids ...keymap.CommandID) string {
	single := singleKeyEntries(entries)
	var parts []string
	for _, id := range ids {
		var key string
		if preferArrow {
			for _, e := range single {
				if e.Binding.Kind == keymap.BindingCommand && e.Binding.Command == id && arrowAliasKeys[e.Sequence] {
					key = e.Sequence
					break
				}
			}
		}
		if key == "" {
			keys := commandHintKeys(single, id)
			if len(keys) == 0 {
				continue
			}
			key = keys[0]
		}
		parts = append(parts, glyphOrKey(key))
	}
	return strings.Join(parts, "/")
}

// createHintSpecs curates the create modal's non-focus-gated status-bar
// hints (Cancel, Next); Submit is applied separately (createSubmitHintSpec)
// so createModalHints can insert the focus == 2-gated Cycle hint between
// Next and Submit, matching today's Cancel/Next/Cycle/Submit order
// (view.go's pre-#540 inline literal).
var createHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandCreateCancel}},
	{desc: "Next", commands: []keymap.CommandID{keymap.CommandCreateNextField}},
}

// createSubmitHintSpec is createHintSpecs' Submit sibling, applied last.
var createSubmitHintSpec = hintSpec{desc: "Submit", commands: []keymap.CommandID{keymap.CommandCreateSubmit}}

// configHintSpecs curates the config modal's status-bar hints (Cancel,
// Next, Save) -- config's provider cycle has no hint-bar entry today
// (view.go's pre-#540 inline literal doesn't advertise left/right either),
// so configModalHints doesn't need a glyphHintKey-derived Cycle entry.
var configHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandConfigCancel}},
	{desc: "Next", commands: []keymap.CommandID{keymap.CommandConfigNextField}},
	{desc: "Save", commands: []keymap.CommandID{keymap.CommandConfigSave}},
}

// createModalHints derives the create modal's status-bar hints from the
// active keymap, byte-identical to today's inline literal (view.go) under
// the default table: Cancel, Next, Cycle (only at b.create.focus == 2), then
// Submit.
func (b Board) createModalHints() []Hint {
	entries := singleKeyEntries(b.keys.Entries(keymap.ModeCreate, ""))
	hints := builtinHints(entries, createHintSpecs)
	if b.create.focus == 2 {
		if key := glyphHintKey(entries, true, keymap.CommandCreateAssigneePrev, keymap.CommandCreateAssigneeNext); key != "" {
			hints = append(hints, Hint{Key: key, Desc: "Cycle"})
		}
	}
	hints = append(hints, builtinHints(entries, []hintSpec{createSubmitHintSpec})...)
	return hints
}

// configModalHints derives the config modal's status-bar hints from the
// active keymap, byte-identical to today's inline literal (view.go) under
// the default table.
func (b Board) configModalHints() []Hint {
	entries := singleKeyEntries(b.keys.Entries(keymap.ModeConfig, ""))
	return builtinHints(entries, configHintSpecs)
}

// createCommandActive reports whether a textBinding-resolved create.*
// command id is currently eligible to run: assignee_prev/assignee_next
// require b.create.focus == 2 (the assignee picker) AND at least one
// assignee option (mirroring today's `len(b.create.assigneeOptions) == 0`
// no-op guard, mode_handlers.go); every other create.* command id is not
// focus-gated.
func (b Board) createCommandActive(id keymap.CommandID) bool {
	switch id {
	case keymap.CommandCreateAssigneePrev, keymap.CommandCreateAssigneeNext:
		return b.create.focus == 2 && len(b.create.assigneeOptions) > 0
	default:
		return true
	}
}

// configCommandActive is createCommandActive's config sibling:
// provider_prev/provider_next require b.config.focus == 0 (the provider
// picker); every other config.* command id is not focus-gated.
func (b Board) configCommandActive(id keymap.CommandID) bool {
	switch id {
	case keymap.CommandConfigProviderPrev, keymap.CommandConfigProviderNext:
		return b.config.focus == 0
	default:
		return true
	}
}

// runCreateCommand runs the create command id resolves to. Case bodies are
// transcribed verbatim from the pre-#540 handleCreateModeKey
// (mode_handlers.go), guard for guard, including the
// b.validationErr = "" clear on the assignee-cycle commands (today's
// `default:` branch clears it before the assignee switch).
func (b Board) runCreateCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandCreateCancel:
		b.mode = normalMode
		return b, nil
	case keymap.CommandCreateSubmit:
		title := strings.TrimSpace(strings.ReplaceAll(b.create.titleInput.Value(), "\n", " "))
		if title == "" {
			b.validationErr = "Title is required"
			return b, nil
		}
		label := strings.TrimSpace(b.create.labelInput.Value())
		for _, col := range b.Columns {
			if strings.EqualFold(col.Title, label) {
				b.validationErr = "Cannot use reserved column label"
				return b, nil
			}
		}
		// Store pending assignee if a real collaborator is selected (not "(none)").
		if len(b.create.assigneeOptions) > 1 && b.create.assigneeOptions[b.create.assigneeIndex] != noneAssignee {
			login := b.create.assigneeOptions[b.create.assigneeIndex]
			login = strings.TrimSuffix(login, " (me)")
			b.create.pendingAssignee = login
		} else {
			b.create.pendingAssignee = ""
		}
		b.mode = creatingMode
		b.create.titleInput.Blur()
		b.create.labelInput.Blur()
		return b, tea.Batch(b.spinner.Tick, createCardCmd(b.provider, title, label))
	case keymap.CommandCreateNextField:
		var cmd tea.Cmd
		hasAssignee := len(b.create.assigneeOptions) > 1
		switch b.create.focus {
		case 0: // title -> label
			b.create.focus = 1
			b.create.titleInput.Blur()
			cmd = b.create.labelInput.Focus()
		case 1: // label -> assignee (if available) or title
			b.create.labelInput.Blur()
			if hasAssignee {
				b.create.focus = 2
			} else {
				b.create.focus = 0
				cmd = b.create.titleInput.Focus()
			}
		case 2: // assignee -> title
			b.create.focus = 0
			cmd = b.create.titleInput.Focus()
		}
		return b, cmd
	case keymap.CommandCreateAssigneePrev:
		b.validationErr = ""
		b.create.assigneeIndex--
		if b.create.assigneeIndex < 0 {
			b.create.assigneeIndex = len(b.create.assigneeOptions) - 1
		}
		return b, nil
	case keymap.CommandCreateAssigneeNext:
		b.validationErr = ""
		b.create.assigneeIndex++
		if b.create.assigneeIndex >= len(b.create.assigneeOptions) {
			b.create.assigneeIndex = 0
		}
		return b, nil
	}
	return b, nil
}

// runConfigCommand runs the config command id resolves to. Case bodies are
// transcribed verbatim from the pre-#540 handleConfigModeKey
// (mode_handlers.go), guard for guard.
func (b Board) runConfigCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandConfigCancel:
		if b.config.firstLaunch {
			return b, tea.Quit
		}
		b.mode = normalMode
		return b, nil
	case keymap.CommandConfigSave:
		provider := b.config.providerOptions[b.config.providerIndex]
		repo := strings.TrimSpace(b.config.repoInput.Value())
		if repo == "" {
			b.validationErr = "Repository is required"
			return b, nil
		}
		b.validationErr = ""
		return b, saveConfigCmd(b.config.localPath, provider, repo, b.trustPath)
	case keymap.CommandConfigNextField:
		if b.config.focus == 0 {
			b.config.focus = 1
			cmd := b.config.repoInput.Focus()
			return b, cmd
		}
		b.config.focus = 0
		b.config.repoInput.Blur()
		return b, nil
	case keymap.CommandConfigProviderNext:
		b.config.providerIndex = (b.config.providerIndex + 1) % len(b.config.providerOptions)
		return b, nil
	case keymap.CommandConfigProviderPrev:
		b.config.providerIndex = (b.config.providerIndex - 1 + len(b.config.providerOptions)) % len(b.config.providerOptions)
		return b, nil
	}
	return b, nil
}

// --- search/comment (#540 PR 2/2) ---
//
// search and comment are both ConsumesPrintableRunes() text-input surfaces,
// same as create/config, but neither has a focus-gated command: every one
// of their command ids is always eligible, so unlike createCommandActive/
// configCommandActive there is no eligibility predicate here -- a resolved
// search.*/comment.* command id always dispatches through
// runSearchCommand/runCommentCommand.

// searchHintSpecs curates the search mode's non-glyph status-bar hints
// (Apply, Clear); the Navigate hint is applied separately (searchHints)
// since it needs glyphHintKey's arrow-preferred substitution, not
// builtinHints' plain commandHintKeys pick.
var searchHintSpecs = []hintSpec{
	{desc: "Apply", commands: []keymap.CommandID{keymap.CommandSearchApply}},
	{desc: "Clear", commands: []keymap.CommandID{keymap.CommandSearchCancel}},
}

// commentHintSpecs curates the comment modal's status-bar hints (Cancel,
// Submit).
var commentHintSpecs = []hintSpec{
	{desc: "Cancel", commands: []keymap.CommandID{keymap.CommandCommentCancel}},
	{desc: "Submit", commands: []keymap.CommandID{keymap.CommandCommentSubmit}},
}

// searchHints derives search mode's status-bar hints from the active
// keymap, byte-identical to today's searchModeHints package var (pre-#540
// model.go) under the default table: Apply, Clear, then Navigate (arrow-
// preferred glyph-substituted, mirroring createModalHints' Cycle hint) --
// search's ctrl+n/ctrl+p are aliases for up/down (the inverse of normal
// mode's j/k-primary convention), so the arrow-preferred tie-break is what
// keeps today's "↑/↓" literal byte-identical under the default table.
func (b Board) searchHints() []Hint {
	entries := singleKeyEntries(b.keys.Entries(keymap.ModeSearch, ""))
	hints := builtinHints(entries, searchHintSpecs)
	if key := glyphHintKey(entries, true, keymap.CommandSearchPrevResult, keymap.CommandSearchNextResult); key != "" {
		hints = append(hints, Hint{Key: key, Desc: "Navigate"})
	}
	return hints
}

// commentHints derives the comment modal's status-bar hints from the active
// keymap, byte-identical to today's commentModeHints package var (pre-#540
// model.go) under the default table.
func (b Board) commentHints() []Hint {
	entries := singleKeyEntries(b.keys.Entries(keymap.ModeComment, ""))
	return builtinHints(entries, commentHintSpecs)
}

// runSearchCommand runs the search command id resolves to. Case bodies are
// transcribed verbatim from the pre-#540 handleSearchModeKey
// (mode_handlers.go), guard for guard.
func (b Board) runSearchCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandSearchCancel:
		b.clearSearch()
		b.mode = normalMode
		b.rebuildNormalHints()
		b.restoreFocusHints()
		return b, nil
	case keymap.CommandSearchApply:
		b.searchInput.Blur()
		b.mode = normalMode
		b.clampScrollOffset()
		b.rebuildNormalHints()
		b.restoreFocusHints()
		return b, nil
	case keymap.CommandSearchNextColumn:
		b.clearSearch()
		b.mode = normalMode
		b.switchColumn((b.ActiveTab + 1) % len(b.Columns))
		return b, nil
	case keymap.CommandSearchPrevColumn:
		b.clearSearch()
		b.mode = normalMode
		b.switchColumn((b.ActiveTab - 1 + len(b.Columns)) % len(b.Columns))
		return b, nil
	case keymap.CommandSearchNextResult:
		col := &b.Columns[b.ActiveTab]
		col.Cursor = moveCursor(col.Cursor, len(b.visibleCards()), true)
		b.detailScrollOffset = 0
		b.clampScrollOffset()
		return b, nil
	case keymap.CommandSearchPrevResult:
		col := &b.Columns[b.ActiveTab]
		col.Cursor = moveCursor(col.Cursor, len(b.visibleCards()), false)
		b.detailScrollOffset = 0
		b.clampScrollOffset()
		return b, nil
	}
	return b, nil
}

// runCommentCommand runs the comment command id resolves to. Case bodies
// are transcribed verbatim from the pre-#540 handleCommentModeKey
// (mode_handlers.go), guard for guard.
func (b Board) runCommentCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandCommentCancel:
		b.mode = normalMode
		b.restoreModeHints()
		return b, nil
	case keymap.CommandCommentSubmit:
		b.mode = normalMode
		b.restoreModeHints()
		comment := b.comment.input.Value()
		act := b.comment.pendingAction
		if b.comment.boardScope {
			return b.handleBoardActionKeyWithComment(act, comment)
		}
		if b.comment.prScope {
			return b.handlePRActionKeyWithComment(act, b.comment.pendingCard, comment)
		}
		return b.handleActionKeyWithComment(act, b.comment.pendingCard, comment)
	}
	return b, nil
}
