// Package keymap resolves user-configured and built-in key bindings into a
// small, allocation-free lookup engine: Resolve merges a defaults Tables
// layer with a user Tables layer (re-canonicalizing every raw key and
// rejecting collisions), and the resulting Keymap answers Lookup queries
// against per-mode tables with an optional per-column overlay.
package keymap

import (
	"fmt"
	"sort"
	"strings"
)

// Table maps a canonical key-sequence string (Sequence.String()) to its
// resolved Binding.
type Table map[string]Binding

// Tables groups one full layer of bindings: per-mode tables plus per-column
// overlay tables. Columns overlay ModeNormal/ModeDetail only -- see
// Resolve.
type Tables struct {
	Modes   map[Mode]Table
	Columns map[string]Table
}

// Keymap is the immutable result of merging a defaults Tables layer with a
// user Tables layer. Every field is unexported and every table it holds is
// precomputed by Resolve, so a *Keymap never mutates after construction and
// is safe to share across Board's value copies.
type Keymap struct {
	// modes holds the merged (defaults + user) table for every mode named
	// in either layer, keyed by Mode.
	modes map[Mode]Table

	// columnOverlays holds, for every column named in either layer
	// (lowercased), the precomputed effective table for ModeNormal and
	// ModeDetail -- the column's own entries win over that mode's merged
	// default on conflict. No other mode is present in the inner map.
	columnOverlays map[string]map[Mode]Table
}

// Entry is one resolved (canonical sequence, binding) pair, as returned by
// Entries.
type Entry struct {
	Sequence string
	Binding  Binding
}

// Resolve merges a defaults Tables layer with a user Tables layer into an
// immutable *Keymap.
//
// For every table in both layers (every mode's table, every column's
// table), each raw key is re-canonicalized via ParseSequence and
// Sequence.String(); Resolve errors, naming the offending key and table,
// if a key fails to parse or if two distinct raw keys in the same table
// normalize to the same canonical sequence.
//
// Per mode, the defaults table is copied and then overwritten per
// canonical key with the user table's entry for that key -- including
// BindingUnbound -- leaving every default key the user table doesn't
// mention untouched.
//
// Tables.Columns keys are lowercased on entry (case-insensitive column
// matching, mirroring resolveAction's strings.EqualFold); two distinct raw
// column names in the same layer that lowercase to the same value are
// rejected, mirroring normalizeTable's raw-key collision check.
//
// For every resulting column, Resolve precomputes the effective ModeNormal
// and ModeDetail table by layering default-mode < default-column <
// user-mode < user-column -- user config always wins over any default
// regardless of mode/column scope, and column always wins over mode within
// the same layer -- so Lookup never allocates. The plain per-mode table
// used when no column matches stays default-mode overlaid by user-mode
// only.
func Resolve(defaults, user Tables) (*Keymap, error) {
	mergedModes := make(map[Mode]Table)
	defModeTables := make(map[Mode]Table)
	userModeTables := make(map[Mode]Table)
	for _, mode := range modeNameUnion(defaults.Modes, user.Modes) {
		defTable, err := normalizeTable(defaults.Modes[mode], fmt.Sprintf("mode %q defaults", mode))
		if err != nil {
			return nil, err
		}
		userTable, err := normalizeTable(user.Modes[mode], fmt.Sprintf("mode %q user config", mode))
		if err != nil {
			return nil, err
		}
		defModeTables[mode] = defTable
		userModeTables[mode] = userTable
		mergedModes[mode] = overlayTable(defTable, userTable)
	}

	defaultColumns, err := lowercaseColumnNames(defaults.Columns, "defaults")
	if err != nil {
		return nil, err
	}
	userColumns, err := lowercaseColumnNames(user.Columns, "user config")
	if err != nil {
		return nil, err
	}

	defColumnTables := make(map[string]Table)
	userColumnTables := make(map[string]Table)
	for _, name := range columnNameUnion(defaultColumns, userColumns) {
		defTable, err := normalizeTable(defaultColumns[name], fmt.Sprintf("column %q defaults", name))
		if err != nil {
			return nil, err
		}
		userTable, err := normalizeTable(userColumns[name], fmt.Sprintf("column %q user config", name))
		if err != nil {
			return nil, err
		}
		defColumnTables[name] = defTable
		userColumnTables[name] = userTable
	}

	columnOverlays := make(map[string]map[Mode]Table, len(defColumnTables))
	for _, name := range columnNameUnion(defaultColumns, userColumns) {
		overlay := make(map[Mode]Table, 2)
		for _, mode := range []Mode{ModeNormal, ModeDetail} {
			effective := overlayTable(defModeTables[mode], defColumnTables[name])
			effective = overlayTable(effective, userModeTables[mode])
			effective = overlayTable(effective, userColumnTables[name])
			overlay[mode] = effective
		}
		columnOverlays[name] = overlay
	}

	return &Keymap{modes: mergedModes, columnOverlays: columnOverlays}, nil
}

// normalizeTable re-canonicalizes every raw key in raw via ParseSequence
// and Sequence.String(), returning a fresh Table keyed by the canonical
// form. label identifies the offending table (mode/column name and
// defaults/user layer) in any returned error.
func normalizeTable(raw Table, label string) (Table, error) {
	out := make(Table, len(raw))
	rawKeyBySeq := make(map[string]string, len(raw))
	for rawKey, binding := range raw {
		seq, err := ParseSequence(rawKey)
		if err != nil {
			return nil, fmt.Errorf("keymap: invalid key %q in %s: %w", rawKey, label, err)
		}
		canonical := seq.String()
		if prevRaw, exists := rawKeyBySeq[canonical]; exists {
			return nil, fmt.Errorf("keymap: %s: keys %q and %q both normalize to %q", label, prevRaw, rawKey, canonical)
		}
		rawKeyBySeq[canonical] = rawKey
		out[canonical] = binding
	}
	return out, nil
}

// overlayTable returns a fresh Table containing every entry of base,
// overwritten per key by every entry of overlay.
func overlayTable(base, overlay Table) Table {
	out := make(Table, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		out[k] = v
	}
	return out
}

// modeNameUnion returns the set of Mode keys present in either map, in a
// stable (sorted) order.
func modeNameUnion(a, b map[Mode]Table) []Mode {
	set := make(map[Mode]bool, len(a)+len(b))
	for m := range a {
		set[m] = true
	}
	for m := range b {
		set[m] = true
	}
	out := make([]Mode, 0, len(set))
	for m := range set {
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// lowercaseColumnNames returns a copy of m keyed by strings.ToLower(name),
// erroring, naming both raw names, if two distinct raw names in m
// lowercase to the same value -- mirroring normalizeTable's raw-key
// collision check so a silent last-write-wins never drops a column's
// table. label identifies the offending layer (defaults/user config) in
// any returned error.
func lowercaseColumnNames(m map[string]Table, label string) (map[string]Table, error) {
	out := make(map[string]Table, len(m))
	rawNameByLower := make(map[string]string, len(m))
	for name, table := range m {
		lower := strings.ToLower(name)
		if prevRaw, exists := rawNameByLower[lower]; exists {
			return nil, fmt.Errorf("keymap: column %s: names %q and %q both normalize to %q", label, prevRaw, name, lower)
		}
		rawNameByLower[lower] = name
		out[lower] = table
	}
	return out, nil
}

// columnNameUnion returns the set of column name keys present in either
// map, in a stable (sorted) order.
func columnNameUnion(a, b map[string]Table) []string {
	set := make(map[string]bool, len(a)+len(b))
	for name := range a {
		set[name] = true
	}
	for name := range b {
		set[name] = true
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// effectiveTable returns the table Lookup/Entries should consult for
// (mode, column): the column overlay when column is non-empty, mode is
// ModeNormal or ModeDetail, and a matching (case-insensitive) column
// overlay exists; otherwise the mode's merged table.
func (k *Keymap) effectiveTable(mode Mode, column string) Table {
	if column != "" && (mode == ModeNormal || mode == ModeDetail) {
		if overlay, ok := k.columnOverlays[strings.ToLower(column)]; ok {
			if table, ok := overlay[mode]; ok {
				return table
			}
		}
	}
	return k.modes[mode]
}

// Entries returns every resolved (sequence, binding) entry for (mode,
// column), sorted by canonical sequence string for deterministic
// iteration. Each call returns a fresh slice/copy -- mutating it never
// perturbs the Keymap's internal state or a subsequent call.
func (k *Keymap) Entries(mode Mode, column string) []Entry {
	table := k.effectiveTable(mode, column)
	entries := make([]Entry, 0, len(table))
	for seq, binding := range table {
		entries = append(entries, Entry{Sequence: seq, Binding: binding})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Sequence < entries[j].Sequence })
	return entries
}
