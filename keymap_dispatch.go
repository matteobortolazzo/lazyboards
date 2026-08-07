package main

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// This file is the registry dispatch seam for package main (#489): it routes
// normal-mode and detail-panel key presses through keymap.Keymap.Lookup
// instead of the legacy switch msg.String()/handleCustomActionKey path,
// generalizes the pending-sequence/which-key flow so built-ins and inline
// actions share it, and derives the normal/detail status-bar hint bars from
// the same active table dispatch itself resolves against.

// withKeymap returns a copy of b with its active keymap replaced by keys and
// every hint bar immediately rebuilt from it -- the single choke point that
// keeps b.normalHints/b.statusBar.hints in sync with whatever table dispatch
// will actually resolve against next, so a remap can never leave a stale
// hint bar advertising an old binding. Used by NewBoard's initial legacy-
// derived keymap and by main.go's post-config.Load() override with the
// fully resolved config.ResolveKeymap result.
func (b Board) withKeymap(keys *keymap.Keymap) Board {
	b.keys = keys
	b.rebuildNormalHints()
	b.restoreFocusHints()
	return b
}

// navColumnIndex reports the 0-based column index a nav.column_N command
// jumps to, and whether id actually is one of the nine nav.column_N
// commands -- shared by runNormalCommand and runDetailCommand so the two
// number-key jump implementations (which differ only in the detailFocused
// side effect) can't drift on the id-to-index mapping.
func navColumnIndex(id keymap.CommandID) (int, bool) {
	switch id {
	case keymap.CommandNavColumn1:
		return 0, true
	case keymap.CommandNavColumn2:
		return 1, true
	case keymap.CommandNavColumn3:
		return 2, true
	case keymap.CommandNavColumn4:
		return 3, true
	case keymap.CommandNavColumn5:
		return 4, true
	case keymap.CommandNavColumn6:
		return 5, true
	case keymap.CommandNavColumn7:
		return 6, true
	case keymap.CommandNavColumn8:
		return 7, true
	case keymap.CommandNavColumn9:
		return 8, true
	}
	return 0, false
}

// activeColumnTitle returns the active column's Title, or "" when there is
// no active column yet (no columns loaded, or ActiveTab out of range) --
// Resolve's column matching is case-insensitive, so callers can pass this
// straight through to Lookup/Entries without lowercasing it themselves.
func (b Board) activeColumnTitle() string {
	if len(b.Columns) == 0 || b.ActiveTab < 0 || b.ActiveTab >= len(b.Columns) {
		return ""
	}
	return b.Columns[b.ActiveTab].Title
}

// dispatchKey resolves a single fresh key press (no pending sequence yet)
// against (mode, the active column overlay) and dispatches the result:
//   - OutcomeMatch dispatches the resolved binding immediately.
//   - OutcomePending, with at least one scope-eligible candidate, enters the
//     pending-sequence which-key state.
//   - Anything else (OutcomeNoMatch, or every OutcomePending candidate
//     scope-gated away) is a silent no-op -- mirroring today's behavior for
//     both a genuinely unbound key and a prefix whose only continuations are
//     refused (AC6; TestKeySequence_PRScopeGatedPrefixDoesNotEnterPending
//     TestKeySequence_CardScopePrefixIgnoredWhenNoCards).
//
// The Alt-strip-and-retry fallback (A3) is applied via lookupWithAltFallback
// before this precedence is evaluated, so an explicit alt+key binding always
// wins and a stripped fallback can only ever produce an inline-action
// result, never a built-in command.
func (b Board) dispatchKey(mode keymap.Mode, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	column := b.activeColumnTitle()
	seq := keymap.Sequence{keymap.Key(msg.String())}
	result, resolvedSeq := b.lookupWithAltFallback(mode, column, seq)
	alt := msg.Alt

	switch result.Outcome {
	case keymap.OutcomeMatch:
		return b.dispatchBinding(mode, result.Binding, alt)
	case keymap.OutcomePending:
		if cands := b.eligibleCandidates(result.Candidates); len(cands) > 0 {
			b.pendingSeq = resolvedSeq.String()
			b.pendingSeqAlt = alt
			b.statusBar.SetActionHints(seqHints(cands))
		}
	}
	return b, nil
}

// handlePendingSeqKey consumes the next key of a pending sequence (built-in
// or inline-action, generalized from the legacy custom-action-only flow).
// It runs before every other normal-mode/detail-focused key handler (see
// handleNormalModeKey), so a key that is itself bound to something else
// (e.g. "j") still only extends/completes the pending sequence instead of
// also triggering its own binding.
//
// Precedence on the extended sequence: an exact Match dispatches; an
// OutcomePending with at least one scope-eligible candidate extends the
// pending state; anything else cancels -- silently if the key was esc (a
// user could in principle bind e.g. "P esc"; Lookup already tried that exact
// match first, so reaching this fallback means it's genuinely unbound) or
// with a "No action bound to ..." warning otherwise.
func (b Board) handlePendingSeqKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	mode := keymap.ModeNormal
	if b.detailFocused {
		mode = keymap.ModeDetail
	}
	column := b.activeColumnTitle()
	typedSeq := append(canonicalSequence(b.pendingSeq), keymap.Key(msg.String()))
	result, resolvedSeq := b.lookupWithAltFallback(mode, column, typedSeq)
	alt := b.pendingSeqAlt || msg.Alt

	switch result.Outcome {
	case keymap.OutcomeMatch:
		b.clearPendingSeq()
		b.restoreFocusHints()
		return b.dispatchBinding(mode, result.Binding, alt)
	case keymap.OutcomePending:
		if cands := b.eligibleCandidates(result.Candidates); len(cands) > 0 {
			b.pendingSeq = resolvedSeq.String()
			b.pendingSeqAlt = alt
			b.statusBar.SetActionHints(seqHints(cands))
			return b, nil
		}
	}

	b.clearPendingSeq()
	b.restoreFocusHints()
	if msg.Type == tea.KeyEsc {
		return b, nil
	}
	cmd := b.statusBar.SetTimedMessage("No action bound to "+typedSeq.String(), StatusWarning, statusMessageDuration)
	return b, cmd
}

// clearPendingSeq resets the pending key-sequence state.
func (b *Board) clearPendingSeq() {
	b.pendingSeq = ""
	b.pendingSeqAlt = false
}

// canonicalSequence splits a canonical, space-joined Sequence.String() form
// back into a keymap.Sequence. It performs no validation: the input always
// originates from a prior Lookup result or msg.String(), both already
// canonical, so this is a straight, allocation-cheap field split (mirroring
// keymap.ParseSequence's own strings.Fields use, minus the per-key
// re-validation that package already did).
func canonicalSequence(s string) keymap.Sequence {
	fields := strings.Fields(s)
	seq := make(keymap.Sequence, len(fields))
	for i, f := range fields {
		seq[i] = keymap.Key(f)
	}
	return seq
}

// dispatchBinding turns a resolved keymap.Binding into (Model, Cmd): a
// BindingCommand runs through the mode-appropriate command runner
// (runNormalCommand/runDetailCommand); a BindingAction is refused silently
// when it is scope: pr and the selected card has no linked PR -- the
// registry's own pr-scope gate (see eligibleCandidates below for the pending-
// sequence equivalent), so a pr-scope action can never reach
// dispatchActionWithAlt's downstream 0-linked-PR warning branch
// through ordinary dispatch (that branch stays defensive-only, exercised
// only by direct handler tests) -- then dispatches through
// dispatchActionWithAlt exactly like every other action dispatch path.
func (b Board) dispatchBinding(mode keymap.Mode, binding keymap.Binding, alt bool) (tea.Model, tea.Cmd) {
	switch binding.Kind {
	case keymap.BindingCommand:
		if mode == keymap.ModeDetail {
			return b.runDetailCommand(binding.Command)
		}
		return b.runNormalCommand(binding.Command)
	case keymap.BindingAction:
		act := configActionFromKeymap(binding.Action)
		if act.Scope == "pr" && len(b.selectedCard().LinkedPRs) == 0 {
			return b, nil
		}
		return b.dispatchActionWithAlt(act, alt)
	}
	return b, nil
}

// configActionFromKeymap converts a keymap.Action into the config.Action the
// existing action-dispatch machinery (dispatchActionWithAlt and everything
// downstream of it) already consumes. The two types are kept field-for-
// field/yaml-tag identical by hand (see keymap.Action's doc comment and
// TestConfigAction_KeymapAction_FieldsStayInSync), so this is a straight
// field copy.
func configActionFromKeymap(a keymap.Action) config.Action {
	return config.Action{
		Name:    a.Name,
		Type:    a.Type,
		URL:     a.URL,
		Command: a.Command,
		Scope:   a.Scope,
		Order:   a.Order,
	}
}

// eligibleCandidates filters cands down to those that could actually
// dispatch right now: a BindingCommand candidate is always eligible: a
// BindingAction candidate is scope-gated exactly like dispatchBinding above
// -- board-scope always eligible, pr-scope needs the selected
// card to have a linked PR, and every other (card/default) scope needs at
// least one visible card. Preserves the input order (Lookup's Candidates are
// already sorted by canonical sequence string).
func (b Board) eligibleCandidates(cands []keymap.Candidate) []keymap.Candidate {
	hasCards := len(b.visibleCards()) > 0
	out := make([]keymap.Candidate, 0, len(cands))
	for _, c := range cands {
		if c.Binding.Kind == keymap.BindingAction {
			switch config.DefaultScope(c.Binding.Action.Scope) {
			case "board":
			case "pr":
				if len(b.selectedCard().LinkedPRs) == 0 {
					continue
				}
			default:
				if !hasCards {
					continue
				}
			}
		}
		out = append(out, c)
	}
	return out
}

// seqHints builds the which-key style hint bar for a pending sequence: one
// hint per eligible candidate (full canonical key sequence + description),
// then esc to cancel.
func seqHints(cands []keymap.Candidate) []Hint {
	hints := make([]Hint, 0, len(cands)+1)
	for _, c := range cands {
		hints = append(hints, Hint{Key: c.Sequence, Desc: describeBinding(c.Binding)})
	}
	return append(hints, Hint{Key: "esc", Desc: "cancel"})
}

// describeBinding returns the human-readable label for a binding's
// which-key/hint-bar entry: an inline action's own Name, or a built-in
// command's catalogued Desc. An inline action's Name is untrusted (it comes
// straight from user/repo config) and is sanitized with sanitizeSingleLine
// before it ever reaches the status bar, mirroring every other untrusted
// text rendered there (SetTimedMessage/SetStickyMessage).
func describeBinding(binding keymap.Binding) string {
	switch binding.Kind {
	case keymap.BindingAction:
		return sanitizeSingleLine(binding.Action.Name)
	case keymap.BindingCommand:
		if cmd, ok := keymap.FindCommand(binding.Command); ok {
			return cmd.Desc
		}
	}
	return ""
}

// --- Alt-strip-and-retry fallback (A3) ---

// lookupWithAltFallback looks seq up against (mode, column). If that exact
// lookup is OutcomeNoMatch and seq's last key carries an explicit "alt+"
// prefix, it strips that prefix from the last key only and retries --
// adopting the retried result only when altFallbackEligible approves it (an
// exact inline-action match, or a pending prefix whose candidates are ALL
// inline actions). An explicit alt+key binding always wins outright (it
// never reaches the NoMatch branch); the stripped fallback can therefore
// never fire a built-in command, only an inline action (A3's required
// negative case). The returned Sequence is the one the winning Result
// actually resolved against -- the stripped form when the fallback fired,
// so callers store the alt-free sequence as pendingSeq (continuation keys
// must extend from the base key, not from "alt+z").
func (b Board) lookupWithAltFallback(mode keymap.Mode, column string, seq keymap.Sequence) (keymap.Result, keymap.Sequence) {
	result := b.keys.Lookup(mode, column, seq)
	if result.Outcome != keymap.OutcomeNoMatch || len(seq) == 0 {
		return result, seq
	}
	last := seq[len(seq)-1]
	stripped, ok := strippedAltKey(last)
	if !ok {
		return result, seq
	}
	strippedSeq := make(keymap.Sequence, len(seq))
	copy(strippedSeq, seq)
	strippedSeq[len(strippedSeq)-1] = stripped
	retried := b.keys.Lookup(mode, column, strippedSeq)
	if !altFallbackEligible(retried) {
		return result, seq
	}
	return retried, strippedSeq
}

// strippedAltKey reports whether k carries an explicit "alt+" prefix, and if
// so returns the key with that prefix removed.
func strippedAltKey(k keymap.Key) (keymap.Key, bool) {
	s := string(k)
	if !strings.HasPrefix(s, "alt+") {
		return k, false
	}
	return keymap.Key(strings.TrimPrefix(s, "alt+")), true
}

// altFallbackEligible is the pure decision predicate lookupWithAltFallback
// consults after re-looking-up the alt-stripped sequence: eligible only for
// an exact match to an inline action, or a pending prefix whose candidates
// are ALL inline actions. A match to a built-in command is never eligible --
// this is the engine-level half of A3's guarantee that alt+n can never fall
// back to firing card.new.
func altFallbackEligible(result keymap.Result) bool {
	switch result.Outcome {
	case keymap.OutcomeMatch:
		return result.Binding.Kind == keymap.BindingAction
	case keymap.OutcomePending:
		if len(result.Candidates) == 0 {
			return false
		}
		for _, c := range result.Candidates {
			if c.Binding.Kind != keymap.BindingAction {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// --- Hint derivation from the active registry ---

// hintSpec describes one curated, command-id-derived status-bar hint: a
// fixed display label plus the built-in command id(s) whose currently bound
// key(s) supply the Key field, so a remap/unbind is reflected automatically
// without the label itself changing. grouped==false (the default) emits one
// Hint per bound key across every listed command (reproducing "h Back" +
// "esc Back"); grouped==true joins each listed command's own single primary
// key with "/" into one Hint (reproducing "j/k Scroll").
type hintSpec struct {
	desc     string
	commands []keymap.CommandID
	grouped  bool
}

// normalHintSpecs curates the normal-mode built-in hints shown in the status
// bar -- app.help, card.new, card.edit -- matching the plan's "hints stay
// pixel-identical today" assumption (rendering the whole ~25-key table would
// be a behavior change deferred to #492/#502).
var normalHintSpecs = []hintSpec{
	{desc: "Help", commands: []keymap.CommandID{keymap.CommandHelp}},
	{desc: "New", commands: []keymap.CommandID{keymap.CommandCardNew}},
	{desc: "Edit", commands: []keymap.CommandID{keymap.CommandCardEdit}},
}

// detailHintSpecs curates the detail-focused built-in hints.
var detailHintSpecs = []hintSpec{
	{desc: "Help", commands: []keymap.CommandID{keymap.CommandHelp}},
	{desc: "Edit", commands: []keymap.CommandID{keymap.CommandCardEdit}},
	{desc: "Scroll", commands: []keymap.CommandID{keymap.CommandDetailScrollDown, keymap.CommandDetailScrollUp}, grouped: true},
	{desc: "Back", commands: []keymap.CommandID{keymap.CommandDetailBlur}},
}

// arrowAliasKeys are the special multi-key names the default tables bind as
// silent aliases alongside a plainer key (j/down, k/up, h/left) -- suppressed
// from a command-derived hint's key set unless a command's ONLY bound key is
// one of them (better an arrow-key hint than none at all).
var arrowAliasKeys = map[string]bool{"up": true, "down": true, "left": true, "right": true}

// arrowGlyphs maps raw key names to the arrow glyphs the pre-registry hint
// vars rendered (Q3, #540): keyed on the literal key name, not on a command
// id, so remapping a Cycle/Navigate hint onto keys outside this map (e.g.
// "h"/"l") falls back to the raw key text instead of a stale/misleading
// glyph. Originally prPickerArrowGlyphs (#490); renamed and moved here so
// #540's create/search hint builders can share it beside arrowAliasKeys.
var arrowGlyphs = map[string]string{
	"left":  "◀",
	"right": "▶",
	"up":    "↑",
	"down":  "↓",
}

// glyphOrKey returns key's arrow glyph if it has one, the raw key text
// otherwise. Originally prPickerGlyphOrKey (#490).
func glyphOrKey(key string) string {
	if glyph, ok := arrowGlyphs[key]; ok {
		return glyph
	}
	return key
}

// commandHintKeys returns the resolved, alias-suppressed keys bound to id in
// entries (registryHints passes the active column's overlaid entries --
// Entries(mode, column) -- so a column-scoped rebind of a curated built-in is
// reflected in the hint bar exactly like inlineActionHints already does for
// inline actions; entries falls back to the global table when no column is
// active), ordered by (rune-length ascending, then alphabetically): the shortest,
// plainest key first. This reproduces today's fixed "h Back" before
// "esc Back" and picks "j"/"k" over their "down"/"up" aliases as Scroll's
// primary keys, without hardcoding either literal -- and stays deterministic
// (not Go map order) after an arbitrary remap.
func commandHintKeys(entries []keymap.Entry, id keymap.CommandID) []string {
	var all []string
	for _, e := range entries {
		if e.Binding.Kind == keymap.BindingCommand && e.Binding.Command == id {
			all = append(all, e.Sequence)
		}
	}
	filtered := make([]string, 0, len(all))
	for _, k := range all {
		if !arrowAliasKeys[k] {
			filtered = append(filtered, k)
		}
	}
	if len(filtered) == 0 {
		filtered = all
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if len(filtered[i]) != len(filtered[j]) {
			return len(filtered[i]) < len(filtered[j])
		}
		return filtered[i] < filtered[j]
	})
	return filtered
}

// builtinHints renders specs against entries into their Hint entries, in
// spec declaration order.
func builtinHints(entries []keymap.Entry, specs []hintSpec) []Hint {
	var hints []Hint
	for _, spec := range specs {
		if spec.grouped {
			var keys []string
			for _, id := range spec.commands {
				if ck := commandHintKeys(entries, id); len(ck) > 0 {
					keys = append(keys, ck[0])
				}
			}
			if len(keys) == 0 {
				continue
			}
			hints = append(hints, Hint{Key: strings.Join(keys, "/"), Desc: spec.desc})
			continue
		}
		for _, id := range spec.commands {
			for _, k := range commandHintKeys(entries, id) {
				hints = append(hints, Hint{Key: k, Desc: spec.desc})
			}
		}
	}
	return hints
}

// actionOnlyEntries filters entries down to BindingAction entries.
func actionOnlyEntries(entries []keymap.Entry) []keymap.Entry {
	out := make([]keymap.Entry, 0, len(entries))
	for _, e := range entries {
		if e.Binding.Kind == keymap.BindingAction {
			out = append(out, e)
		}
	}
	return out
}

// sortByActionOrder sorts entries in place by (Binding.Action.Order
// ascending, Sequence ascending) -- the same (Order, key) tie-break
// convention #437's now-deleted legacy hint-ordering helper used, degrading
// to alphabetical order for any hand-built fixture that leaves Order at its
// zero value.
func sortByActionOrder(entries []keymap.Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		oi, oj := entries[i].Binding.Action.Order, entries[j].Binding.Action.Order
		if oi != oj {
			return oi < oj
		}
		return entries[i].Sequence < entries[j].Sequence
	})
}

// hasCardsInActiveColumn reports whether the active column's raw card list
// (b.Columns[b.ActiveTab].Cards) is non-empty -- the same raw, filter/search-
// UNaware predicate #437's now-deleted legacy hint-bar helper used for its
// card-scope gate. This intentionally differs from eligibleCandidates'
// dispatch-time b.visibleCards() check: the hint bar must keep advertising a
// card-scope action while an active filter/search empties the visible list
// but the column still has cards underneath, matching pre-#489 behavior.
func (b Board) hasCardsInActiveColumn() bool {
	if len(b.Columns) == 0 || b.ActiveTab < 0 || b.ActiveTab >= len(b.Columns) {
		return false
	}
	return len(b.Columns[b.ActiveTab].Cards) > 0
}

// inlineActionHints returns the scope-gated inline-action hints for mode:
// the active column's own entries (if any) overlaid on the global (no
// column) entries, ordered exactly like #437's now-deleted legacy hint-bar
// helper -- global config-file order first, then any sequence present only
// in the column's own table appended in the column's order. A sequence the
// active column overrides keeps its *global* position; only its displayed
// Desc/scope switches to the column's own binding (TestAction_HintBar_
// ColumnOverride_KeepsGlobalPosition).
func (b Board) inlineActionHints(mode keymap.Mode) []Hint {
	column := b.activeColumnTitle()

	globalEntries := actionOnlyEntries(b.keys.Entries(mode, ""))
	sortByActionOrder(globalEntries)
	globalBySeq := make(map[string]keymap.Binding, len(globalEntries))
	seqOrder := make([]string, 0, len(globalEntries))
	for _, e := range globalEntries {
		globalBySeq[e.Sequence] = e.Binding
		seqOrder = append(seqOrder, e.Sequence)
	}

	var colBySeq map[string]keymap.Binding
	if column != "" {
		colEntries := actionOnlyEntries(b.keys.Entries(mode, column))
		sortByActionOrder(colEntries)
		colBySeq = make(map[string]keymap.Binding, len(colEntries))
		for _, e := range colEntries {
			colBySeq[e.Sequence] = e.Binding
			if _, exists := globalBySeq[e.Sequence]; !exists {
				seqOrder = append(seqOrder, e.Sequence)
			}
		}
	}

	hasCards := b.hasCardsInActiveColumn()
	hasLinkedPR := hasCards && len(b.selectedCard().LinkedPRs) > 0

	hints := make([]Hint, 0, len(seqOrder))
	for _, seq := range seqOrder {
		binding, ok := colBySeq[seq]
		if !ok {
			binding, ok = globalBySeq[seq]
		}
		if !ok {
			continue
		}
		hint := Hint{Key: seq, Desc: sanitizeSingleLine(binding.Action.Name)}
		switch config.DefaultScope(binding.Action.Scope) {
		case "board":
			hints = append(hints, hint)
		case "pr":
			if hasLinkedPR {
				hints = append(hints, hint)
			}
		default:
			if hasCards {
				hints = append(hints, hint)
			}
		}
	}
	return hints
}

// registryHints derives the full status-bar hint bar for mode (ModeNormal or
// ModeDetail) from the active keymap: the curated built-in specs first, then
// the scope-gated inline-action hints -- so a remap or unbind is reflected
// automatically and the bar can never lie about active bindings.
func (b Board) registryHints(mode keymap.Mode) []Hint {
	specs := normalHintSpecs
	if mode == keymap.ModeDetail {
		specs = detailHintSpecs
	}
	entries := b.keys.Entries(mode, b.activeColumnTitle())
	hints := builtinHints(entries, specs)
	hints = append(hints, b.inlineActionHints(mode)...)
	return hints
}
