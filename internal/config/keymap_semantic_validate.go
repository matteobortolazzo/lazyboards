package config

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// validateKeymapActions validates every inline action definition in
// keymaps.<mode> and keymaps.columns.<name> with the same rules
// validateActions applies to the legacy actions:/columns[].actions: blocks
// (see validateActionValue), inferring and writing back the default scope
// in place when it was omitted (closes #526: previously only the top-level
// actions: block got this inference).
func validateKeymapActions(keymaps *Keymaps) error {
	if keymaps == nil {
		return nil
	}
	for mode, table := range keymaps.Modes {
		if err := validateKeymapActionTable(table); err != nil {
			return fmt.Errorf("keymaps.%s: %w", mode, err)
		}
	}
	for column, table := range keymaps.Columns {
		if err := validateKeymapActionTable(table); err != nil {
			return fmt.Errorf("keymaps.columns.%q: %w", column, err)
		}
	}
	return nil
}

// validateKeymapActionTable validates every BindingAction entry of table in
// place, writing back any inferred scope (KeymapBinding.Action sits inside
// a map value, not addressable, so this is an explicit read-modify-write --
// see validateActions' analogous actions[key] = action write-back).
func validateKeymapActionTable(table KeymapTable) error {
	for key, binding := range table {
		if binding.Kind != keymap.BindingAction {
			continue
		}
		if err := validateActionValue(key, &binding.Action); err != nil {
			return err
		}
		table[key] = binding
	}
	return nil
}

// validateCommandIDs checks that every BindingCommand entry in keymaps.<mode>
// and keymaps.columns.<name> names a catalogued command, erroring, naming
// the mode, key, and unrecognized id, on the first one that doesn't.
func validateCommandIDs(keymaps *Keymaps) error {
	if keymaps == nil {
		return nil
	}
	for mode, table := range keymaps.Modes {
		if key, id, ok := findUnknownCommand(table); !ok {
			return fmt.Errorf("keymaps.%s: key %q: unknown command %q", mode, key, id)
		}
	}
	for column, table := range keymaps.Columns {
		if key, id, ok := findUnknownCommand(table); !ok {
			return fmt.Errorf("keymaps.columns.%q: key %q: unknown command %q", column, key, id)
		}
	}
	return nil
}

// findUnknownCommand returns the first BindingCommand entry in table whose
// command id isn't catalogued. ok is true (with key/id zero) when every
// command entry resolves.
func findUnknownCommand(table KeymapTable) (key string, id keymap.CommandID, ok bool) {
	for k, binding := range table {
		if binding.Kind != keymap.BindingCommand {
			continue
		}
		if _, found := keymap.FindCommand(binding.Command); !found {
			return k, binding.Command, false
		}
	}
	return "", "", true
}

// validatePrintableRuneBindings rejects a bare printable-rune key bound in
// any mode that Mode.ConsumesPrintableRunes() reports true for -- the
// mode's text-input handler swallows every printable rune as literal input
// before any lookup could ever see it, so such a binding could never
// dispatch. Per Q4, only the bare rune form is rejected; an alt+<rune> form
// is exempt, since Alt-held keypresses aren't consumed as text.
func validatePrintableRuneBindings(keymaps *Keymaps) error {
	if keymaps == nil {
		return nil
	}
	for mode, table := range keymaps.Modes {
		if !mode.ConsumesPrintableRunes() {
			continue
		}
		for key := range table {
			if field, found := findBarePrintableRune(key); found {
				return fmt.Errorf("keymaps.%s: key %q binds %q, a bare printable rune: mode %q consumes every printable rune as text input, so this key could never dispatch (bind alt+%s or a named key instead)", mode, key, field, mode, field)
			}
		}
	}
	return nil
}

// findBarePrintableRune returns the first field of key (its space-separated
// sequence) that is a single rune not prefixed by "alt+".
func findBarePrintableRune(key string) (string, bool) {
	for _, field := range strings.Fields(key) {
		if strings.HasPrefix(field, "alt+") {
			continue
		}
		if utf8.RuneCountInString(field) == 1 {
			return field, true
		}
	}
	return "", false
}

// altFreeBaseSequence parses seq (via keymap.ParseSequence, the same
// canonicalization normalizeTable, internal/keymap/keymap.go, already uses)
// and strips "alt+" from every token, not just the first, e.g. "alt+G" ->
// "G", "Z alt+f" -> "Z f", "alt+Z alt+f" -> "Z f", returning the alt-free
// sequence in its canonical Sequence.String() form. hasAlt reports whether
// any token carried an "alt+" prefix; parse errors from seq propagate.
func altFreeBaseSequence(seq string) (base string, hasAlt bool, err error) {
	parsed, err := keymap.ParseSequence(seq)
	if err != nil {
		return "", false, err
	}

	stripped := make(keymap.Sequence, len(parsed))
	for i, tok := range parsed {
		s := string(tok)
		trimmed := strings.TrimPrefix(s, "alt+")
		if trimmed != s {
			hasAlt = true
		}
		stripped[i] = keymap.Key(trimmed)
	}
	return stripped.String(), hasAlt, nil
}

// validateModeCapabilities checks that every keymaps.<mode>.<key> and
// keymaps.columns.<name>.<key> binding is something the mode's dispatch
// seam can actually reach:
//   - a BindingCommand entry's command id must be in
//     keymap.DispatchableCommands(mode) (checked via
//     keymap.CommandDispatchable), erroring with the config path, the key,
//     the offending id, and the full sorted list of valid ids (no cap, no
//     elision) when it isn't;
//   - a BindingAction (inline action) entry requires
//     mode.DispatchesInlineActions(), and, for keymaps.pr_list
//     specifically, additionally requires the binding's already-inferred
//     effective scope (validateKeymapActions runs before this validator and
//     writes it back) to be exactly "pr" -- enforced here, in
//     internal/config, since keymap.Mode has no notion of an Action's
//     scope;
//   - a BindingUnbound entry is always skipped, nothing to check.
//
// keymaps.columns.<name> tables are checked against keymap.ModeColumns's
// own entry in the capability index, not against ModeNormal/ModeDetail
// directly.
//
// Walks keymaps.Modes then keymaps.Columns in sorted key order, and each
// table's keys in sorted order too, so the reported failure is
// deterministic (not Go's randomized map-iteration order) -- returns on the
// first failing binding, matching every sibling validator's fail-fast
// style.
func validateModeCapabilities(keymaps *Keymaps) error {
	if keymaps == nil {
		return nil
	}
	for _, mode := range sortedKeys(keymaps.Modes) {
		if err := validateCapabilityTable(keymaps.Modes[mode], mode, fmt.Sprintf("keymaps.%s", mode)); err != nil {
			return err
		}
	}
	for _, column := range sortedKeys(keymaps.Columns) {
		if err := validateCapabilityTable(keymaps.Columns[column], keymap.ModeColumns, fmt.Sprintf("keymaps.columns.%q", column)); err != nil {
			return err
		}
	}
	return nil
}

// validateCapabilityTable runs validateModeCapabilities' checks over one
// table (a single mode's or column's), in sorted key order.
func validateCapabilityTable(table KeymapTable, mode keymap.Mode, path string) error {
	for _, key := range sortedKeys(table) {
		binding := table[key]
		switch binding.Kind {
		case keymap.BindingCommand:
			if !keymap.CommandDispatchable(mode, binding.Command) {
				return capabilityCommandError(path, key, binding.Command, mode)
			}
		case keymap.BindingAction:
			if err := validateCapabilityAction(mode, path, key, binding.Action); err != nil {
				return err
			}
		case keymap.BindingUnbound:
			continue
		default:
			return fmt.Errorf("%s: key %q: unrecognized binding kind %v", path, key, binding.Kind)
		}
	}
	return nil
}

// validateCapabilityAction runs the inline-action half of
// validateModeCapabilities' checks for one BindingAction entry.
func validateCapabilityAction(mode keymap.Mode, path, key string, action Action) error {
	if !mode.DispatchesInlineActions() {
		return fmt.Errorf("%s: key %q: mode %q never dispatches inline actions", path, key, mode)
	}
	if mode == keymap.ModePRList {
		if scope := DefaultScope(action.Scope); scope != "pr" {
			return fmt.Errorf("%s: key %q: pr_list only dispatches inline actions with scope %q, got %q", path, key, "pr", scope)
		}
	}
	return nil
}

// capabilityCommandError builds validateCapabilityTable's BindingCommand
// rejection: the config path, the offending key, the offending command id,
// and the full sorted list of ids mode can actually dispatch.
func capabilityCommandError(path, key string, id keymap.CommandID, mode keymap.Mode) error {
	valid := keymap.DispatchableCommands(mode)
	names := make([]string, len(valid))
	for i, v := range valid {
		names[i] = string(v)
	}
	return fmt.Errorf("%s: key %q: command %q cannot dispatch in mode %q, want one of: %s", path, key, id, mode, strings.Join(names, ", "))
}

// sortedKeys returns m's keys in ascending order, so
// validateModeCapabilities' mode/column/table walks are deterministic
// instead of Go's randomized map-iteration order. It is shared by all three
// walks (keymaps.Modes, keymaps.Columns, and a single KeymapTable) since
// each is just a map keyed by an ordered type (keymap.Mode or string).
func sortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
