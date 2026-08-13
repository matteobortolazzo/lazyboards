package config

import (
	"strings"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// legacyDeprecationNotice is the single, one-line startup notice emitted
// whenever a legacy actions: or columns[].actions: block is present in the
// loaded config, regardless of whether any of its keys were actually
// inserted into keymaps: (an existing keymaps:-declared entry may shadow
// every one of them).
const legacyDeprecationNotice = "actions:/columns[].actions are deprecated; migrate to the keymaps: namespace (see README)"

// legacySequence translates a raw legacy action key -- a bare,
// rune-concatenated string exactly as it appears as a map key in a
// top-level `actions:` or per-column `columns[].actions:` block (e.g.
// "Pf", "OPEN") -- into the canonical, space-separated keymap.Sequence
// string form that keymap.ParseSequence/Sequence.String() (and therefore
// KeymapTable's map keys, once translated) expect (e.g. "P f", "O P E N").
//
// This mirrors handlePendingSeqKey's `b.pendingSeq + string(msg.Runes)`
// rune-by-rune concatenation (action_dispatch.go:63): a legacy multi-key
// action was always built up one keystroke at a time with no separator,
// so translating it back to canonical form is a straight rune split.
// isUppercaseLetterKey reports whether key is exactly one rune in the range
// A-Z, mirroring the old PR-list dispatch's key filter (deleted raw scan,
// previously handlePRListActionKey in mode_handlers.go:
// `!msg.Alt && msg.Runes[0] >= 'A' && msg.Runes[0] <= 'Z'`). Used to gate the
// pr_list legacy-action insertion path so a legacy scope: pr action bound to
// any other kind of key stays the no-op it always was inside the PR list.
func isUppercaseLetterKey(key string) bool {
	r := []rune(key)
	return len(r) == 1 && r[0] >= 'A' && r[0] <= 'Z'
}

func legacySequence(key string) string {
	runes := []rune(key)
	seq := make(keymap.Sequence, len(runes))
	for i, r := range runes {
		seq[i] = keymap.Key(string(r))
	}
	return seq.String()
}

// translateLegacyActions maps cfg.Actions (into cfg.Keymaps.Modes[normal]
// and cfg.Keymaps.Modes[detail]) and each cfg.Columns[i].Actions (into
// cfg.Keymaps.Columns[<name>]) onto the keymaps: namespace.
//
// The translation is additive, never destructive:
//   - cfg.Actions and cfg.Columns[i].Actions are left fully populated and
//     unchanged (callers that still read the legacy fields keep working);
//   - an existing keymaps:-declared entry for the same canonical sequence
//     is never overwritten by a legacy-derived entry (keymaps: always
//     wins on collision).
//
// Exactly one entry is appended to cfg.Deprecations whenever a legacy
// actions: or columns[].actions: block was present in the loaded config --
// even if every derived key was shadowed by an existing keymaps: entry and
// nothing was actually inserted. The notice is about the deprecated block's
// mere presence, not whether translation happened to insert anything; none
// is appended for a keymaps:-only (or actions-free) config.
//
// Presence is a union of two independent sources, since neither alone
// suffices (#585):
//   - legacyDeclared, the caller-supplied raw-node-walk result
//     (assignActionOrder's localDecls.LegacyBlock, unioned across the
//     global and local documents in Load()) sees a legacy block whose
//     decoded map ends up empty -- an explicit `actions: {}`/
//     `columns[].actions: {}`, or an untrusted local shell-only block that
//     stripLocalShellSinks has already emptied by the time this function
//     runs -- cases the decoded map can no longer see;
//   - len(cfg.Actions) > 0 (and, in the column loop below, a column's own
//     len(col.Actions) > 0) sees a legacy block the raw-node walk can't:
//     one smuggled in via a YAML merge key/alias (`base: &b {actions:...}`
//     plus `<<: *b`), which never appears as a literal `actions:` key
//     during a raw-node walk of the document.
//
// Precondition: this must run after validateActions/validateColumns (see
// Load() in config.go), since legacySequence assumes every legacy key was
// already validated to be non-empty with every continuation rune a letter
// or digit (#510 dropped the old first-rune-must-be-uppercase requirement).
// Running it earlier would silently translate unvalidated (or
// not-yet-scope-inferred) keys. It must also run before validateKeymap, so
// the unified prefix/ctrl+c checks see legacy-derived entries alongside
// native keymaps: ones in the same resolved namespace.
func translateLegacyActions(cfg *Config, legacyDeclared bool) {
	legacyPresent := legacyDeclared || len(cfg.Actions) > 0

	if len(cfg.Actions) > 0 {
		if cfg.Keymaps == nil {
			cfg.Keymaps = &Keymaps{}
		}
		if cfg.Keymaps.Modes == nil {
			cfg.Keymaps.Modes = make(map[keymap.Mode]KeymapTable)
		}
		for _, mode := range []keymap.Mode{keymap.ModeNormal, keymap.ModeDetail} {
			insertLegacyActions(modeTable(cfg, mode), cfg.Actions)
		}

		// #490 PR 7b, Q1: a global scope: pr legacy action must also reach the
		// PR list, which (once this ticket lands) dispatches inline actions
		// through keymaps.pr_list instead of the raw A-Z scan over
		// b.actions[key] handlePRListActionKey used to do. Only scope: pr
		// actions are eligible -- board/card-scoped (or scope-omitted, which
		// infers to board/card, never pr) legacy actions never had a route
		// into the PR list before and must not gain one now. The OLD PR-list
		// dispatch (deleted raw scan) only ever recognized single uppercase
		// letters A-Z with no Alt modifier, so a legacy scope: pr action
		// bound to any other kind of key (lowercase, multi-key, digit, ...)
		// was previously a hard no-op inside the PR list and must stay one --
		// isUppercaseLetterKey restricts this insertion path accordingly.
		prScopeActions := make(map[string]Action)
		for key, action := range cfg.Actions {
			if action.Scope == "pr" && isUppercaseLetterKey(key) {
				prScopeActions[key] = action
			}
		}
		if len(prScopeActions) > 0 {
			insertLegacyActions(modeTable(cfg, keymap.ModePRList), prScopeActions)
		}
	}

	for _, col := range cfg.Columns {
		if len(col.Actions) == 0 {
			continue
		}
		legacyPresent = true
		if cfg.Keymaps == nil {
			cfg.Keymaps = &Keymaps{}
		}
		if cfg.Keymaps.Columns == nil {
			cfg.Keymaps.Columns = make(map[string]KeymapTable)
		}
		lowerName := strings.ToLower(col.Name)
		_, table := findKeymapColumnByLower(cfg.Keymaps.Columns, lowerName)
		if table == nil {
			table = make(KeymapTable)
			cfg.Keymaps.Columns[col.Name] = table
		}
		insertLegacyActions(table, col.Actions)
	}

	if legacyPresent {
		cfg.Deprecations = append(cfg.Deprecations, legacyDeprecationNotice)
	}
}

// modeTable returns cfg.Keymaps.Modes[mode], creating an empty table and
// storing it back first if none exists yet. Callers must ensure
// cfg.Keymaps and cfg.Keymaps.Modes are already non-nil (translateLegacyActions
// does this once, ahead of every modeTable call).
func modeTable(cfg *Config, mode keymap.Mode) KeymapTable {
	table, ok := cfg.Keymaps.Modes[mode]
	if !ok {
		table = make(KeymapTable)
		cfg.Keymaps.Modes[mode] = table
	}
	return table
}

// findKeymapColumnByLower looks up columns by case-insensitive name match
// against lowerName (already lowercased by the caller), mirroring
// mergeKeymaps' globalColumnsByLower approach (keymaps.go) so a legacy
// columns[].actions block reuses an existing keymaps.columns entry of
// differing case instead of creating a second, separate column table. It
// returns the entry's original-case key name and table, or ("", nil) if no
// existing entry matches.
func findKeymapColumnByLower(columns map[string]KeymapTable, lowerName string) (string, KeymapTable) {
	for name, table := range columns {
		if strings.ToLower(name) == lowerName {
			return name, table
		}
	}
	return "", nil
}

// insertLegacyActions translates every entry of legacyActions into table
// via legacySequence, skipping (never overwriting) any canonical key table
// already has -- keymaps:-declared entries always win on a collision.
// Each inserted entry's Order is offset by table's existing key count,
// mirroring mergeKeymapTable's convention (keymaps.go:276). Per A4, the
// top-level KeymapBinding.Order is stamped too (not just the nested
// Action.Order) -- action.Order already reflects the legacy block's
// document position (stamped by stampActionOrder ahead of translation), so
// copying the post-offset value straight into KeymapBinding.Order gives
// Tables() something non-zero to carry through into keymap.Action.Order for
// legacy-derived bindings, same as native keymaps: entries.
func insertLegacyActions(table KeymapTable, legacyActions map[string]Action) {
	offset := len(table)
	for key, action := range legacyActions {
		seq := legacySequence(key)
		if _, exists := table[seq]; exists {
			continue
		}
		action.Order += offset
		table[seq] = KeymapBinding{Kind: keymap.BindingAction, Action: action, Order: action.Order}
	}
}
