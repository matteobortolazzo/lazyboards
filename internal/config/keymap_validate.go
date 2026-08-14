package config

import (
	"fmt"
	"strings"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// ctrlCKey is the literal raw-key token BubbleTea emits for Ctrl+C.
const ctrlCKey = "ctrl+c"

// ResolveKeymap merges the built-in default Tables with cfg.Keymaps into an
// immutable *keymap.Keymap. It is the single resolution path every
// validator and (eventually, #489) runtime consumer shares -- there is no
// second way to combine defaults with user config.
func ResolveKeymap(cfg *Config) (*keymap.Keymap, error) {
	return keymap.Resolve(keymap.Defaults(), cfg.Keymaps.Tables())
}

// DefaultKeymap resolves the built-in default Tables with no user config
// layered on top -- the keymap a Board starts from before main.go applies
// the loaded config's own ResolveKeymap result. Resolving the defaults
// against themselves cannot fail, so any error is reported to the caller
// only for symmetry with ResolveKeymap.
func DefaultKeymap() (*keymap.Keymap, error) {
	return ResolveKeymap(&Config{})
}

// validateKeymap runs the unified checks over cfg.Keymaps: first rejecting
// any raw key sequence that mentions ctrl+c anywhere (direct binding,
// sequence continuation, or explicit ~ unbind) -- Lookup
// (internal/keymap/lookup.go) unconditionally short-circuits any sequence
// whose last key is ctrl+c to CommandQuit regardless of table contents, and
// since a pending sequence is built up and re-looked-up one keypress at a
// time, a ctrl+c anywhere in a bound sequence would hijack it the moment
// it's pressed, producing an entry that can never actually dispatch.
//
// Then, over the fully resolved namespace (built-in defaults merged with
// cfg.Keymaps, built-ins and custom actions together, via ResolveKeymap),
// it rejects, per mode and per column:
//   - any bound key that is a strict, whitespace-boundary prefix of another
//     bound key -- a prefix key could never dispatch, since a pending
//     sequence always waits for a continuation key;
//   - any bound key with an explicit "alt+<key>" token anywhere in its
//     sequence that shadows the implicit Alt-overload of a {comment}-bearing
//     action bound to its alt-free base sequence (dispatchActionWithAlt,
//     action_dispatch.go): holding Alt on that base key already means
//     "enter comment mode first", so an explicit separate alt+ binding on
//     the same key can never be reached.
//
// An explicit `~` unbind on any of the keys involved removes it from
// consideration, clearing the conflict.
func validateKeymap(cfg *Config) error {
	if err := validateNoCtrlC(cfg.Keymaps); err != nil {
		return err
	}

	resolved, err := ResolveKeymap(cfg)
	if err != nil {
		return err
	}

	for _, mode := range keymap.Modes() {
		if err := validateModeEntries(resolved, mode, ""); err != nil {
			return err
		}
	}

	if cfg.Keymaps != nil {
		for column := range cfg.Keymaps.Columns {
			for _, mode := range []keymap.Mode{keymap.ModeNormal, keymap.ModeDetail} {
				if err := validateModeEntries(resolved, mode, column); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// validateModeEntries runs the prefix-conflict and alt/{comment}-shadow
// checks over (mode, column)'s resolved, bound (non-unbound) entries.
func validateModeEntries(resolved *keymap.Keymap, mode keymap.Mode, column string) error {
	entries := resolved.Entries(mode, column)
	bound := make(map[string]keymap.Binding, len(entries))
	sequences := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.Binding.Kind == keymap.BindingUnbound {
			continue
		}
		bound[e.Sequence] = e.Binding
		sequences = append(sequences, e.Sequence)
	}

	if err := validateModePrefixes(sequences, mode, column); err != nil {
		return err
	}
	return validateModeAltCommentShadow(sequences, bound, mode, column)
}

// validateNoCtrlC scans every raw key in keymaps (every mode table, every
// column table) for a ctrl+c token anywhere in its space-separated
// sequence, regardless of the binding's kind (command, inline action, or
// explicit unbind) -- an unbind of ctrl+c is rejected too, since it can
// never do anything (Lookup's short-circuit doesn't consult the table).
func validateNoCtrlC(keymaps *Keymaps) error {
	if keymaps == nil {
		return nil
	}
	for mode, table := range keymaps.Modes {
		if key, found := findCtrlC(table); found {
			return fmt.Errorf("keymaps.%s: key %q cannot bind ctrl+c: it always quits, regardless of table contents", mode, key)
		}
	}
	for column, table := range keymaps.Columns {
		if key, found := findCtrlC(table); found {
			return fmt.Errorf("keymaps.columns.%q: key %q cannot bind ctrl+c: it always quits, regardless of table contents", column, key)
		}
	}
	return nil
}

// findCtrlC returns the first raw key in table that contains a ctrl+c
// token anywhere in its space-separated fields.
func findCtrlC(table KeymapTable) (string, bool) {
	for key := range table {
		for _, field := range strings.Fields(key) {
			if field == ctrlCKey {
				return key, true
			}
		}
	}
	return "", false
}

// validateModePrefixes checks sequences (a mode/column's resolved, bound
// keys) for a whitespace-boundary prefix conflict: key a conflicts with key
// b when b == a + " " + <more>, mirroring Lookup's own pending-match
// boundary check (lookup.go) so a validated config never contains a key
// that Lookup would report OutcomeNoMatch for a partial match that another
// entry's prefix claims to resolve.
func validateModePrefixes(sequences []string, mode keymap.Mode, column string) error {
	for i, a := range sequences {
		for _, b := range sequences[i+1:] {
			if strings.HasPrefix(b, a+" ") {
				if column != "" {
					return fmt.Errorf("keymap: column %q mode %q: key %q is a prefix of key %q: a prefix key can never dispatch", column, mode, a, b)
				}
				return fmt.Errorf("keymap: mode %q: key %q is a prefix of key %q: a prefix key can never dispatch", mode, a, b)
			}
		}
	}
	return nil
}

// validateModeAltCommentShadow checks sequences (a mode/column's resolved,
// bound keys, in deterministic sorted order for multi-conflict reporting)
// for a key with an "alt+" token anywhere shadowing the implicit Alt-overload
// of a {comment}-bearing action bound to its alt-free base sequence
// (altFreeBaseSequence). Only the base key's own binding matters -- the
// shadowing alt+ key can resolve to anything, it just has to exist.
func validateModeAltCommentShadow(sequences []string, bound map[string]keymap.Binding, mode keymap.Mode, column string) error {
	for _, seq := range sequences {
		base, hasAlt, err := altFreeBaseSequence(seq)
		if err != nil {
			if column != "" {
				return fmt.Errorf("keymap: column %q mode %q: %w", column, mode, err)
			}
			return fmt.Errorf("keymap: mode %q: %w", mode, err)
		}
		if !hasAlt {
			continue
		}
		baseBinding, exists := bound[base]
		if !exists || baseBinding.Kind != keymap.BindingAction {
			continue
		}
		if !strings.Contains(baseBinding.Action.URL+baseBinding.Action.Command, "{comment}") {
			continue
		}
		if column != "" {
			return fmt.Errorf("keymap: column %q mode %q: key %q shadows the implicit alt+ comment overload of {comment}-action key %q", column, mode, seq, base)
		}
		return fmt.Errorf("keymap: mode %q: key %q shadows the implicit alt+ comment overload of {comment}-action key %q", mode, seq, base)
	}
	return nil
}
