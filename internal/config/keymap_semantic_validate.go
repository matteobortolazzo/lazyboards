package config

import (
	"fmt"
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
			return fmt.Errorf("keymaps.columns.%s: %w", column, err)
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
			return fmt.Errorf("keymaps.columns.%s: key %q: unknown command %q", column, key, id)
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
