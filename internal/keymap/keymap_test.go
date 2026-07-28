package keymap

import (
	"strings"
	"testing"
)

// resolveOrFatal is a test helper: Resolve and fail immediately on error, so
// individual test bodies don't repeat the same boilerplate.
func resolveOrFatal(t *testing.T, defaults, user Tables) *Keymap {
	t.Helper()
	km, err := Resolve(defaults, user)
	if err != nil {
		t.Fatalf("Resolve returned unexpected error: %v", err)
	}
	return km
}

// findEntry looks up an Entry by its canonical sequence string, for
// assertions that don't want to depend on Entries' overall ordering.
func findEntry(entries []Entry, seq string) (Entry, bool) {
	for _, e := range entries {
		if e.Sequence == seq {
			return e, true
		}
	}
	return Entry{}, false
}

// TestResolve_NormalizesTableKeys pins that Resolve re-canonicalizes every
// raw table key via ParseSequence/Sequence.String -- a key written with
// stray whitespace in a default/user table still resolves to its canonical,
// space-joined form.
func TestResolve_NormalizesTableKeys(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"g   d": CommandBinding("go.down")},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	entries := km.Entries(ModeNormal, "")
	if _, ok := findEntry(entries, "g   d"); ok {
		t.Errorf("Entries contains un-normalized key %q, want it canonicalized", "g   d")
	}
	entry, ok := findEntry(entries, "g d")
	if !ok {
		t.Fatalf("Entries missing canonicalized key %q; got %+v", "g d", entries)
	}
	if entry.Binding.Kind != BindingCommand || entry.Binding.Command != "go.down" {
		t.Errorf("Entries[%q].Binding = %+v, want CommandBinding(\"go.down\")", "g d", entry.Binding)
	}
}

// TestResolve_RejectsInvalidKey asserts Resolve errors, naming the offending
// key, when a default table key doesn't parse.
func TestResolve_RejectsInvalidKey(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"Ctrl+A": CommandBinding("go.up")},
	}}
	_, err := Resolve(defaults, Tables{})
	if err == nil {
		t.Fatal("Resolve returned nil error, want an error naming the invalid key")
	}
	if !strings.Contains(err.Error(), "Ctrl+A") {
		t.Errorf("Resolve error = %q, want it to name %q", err.Error(), "Ctrl+A")
	}
}

// TestResolve_RejectsInvalidUserKey mirrors TestResolve_RejectsInvalidKey
// for the user table, so a bad key isn't silently swallowed just because it
// came from the overlay side of the merge.
func TestResolve_RejectsInvalidUserKey(t *testing.T) {
	user := Tables{Modes: map[Mode]Table{
		ModeNormal: {"notakey": CommandBinding("go.up")},
	}}
	_, err := Resolve(Tables{}, user)
	if err == nil {
		t.Fatal("Resolve returned nil error, want an error naming the invalid key")
	}
	if !strings.Contains(err.Error(), "notakey") {
		t.Errorf("Resolve error = %q, want it to name %q", err.Error(), "notakey")
	}
}

// TestResolve_RejectsCollidingKeys pins that two distinct raw keys which
// normalize to the same canonical sequence within one table are rejected --
// Resolve can't silently pick a winner between them.
func TestResolve_RejectsCollidingKeys(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {
			"g d":  CommandBinding("go.down"),
			"g  d": CommandBinding("go.down.dup"),
		},
	}}
	_, err := Resolve(defaults, Tables{})
	if err == nil {
		t.Fatal("Resolve returned nil error, want an error naming the colliding keys")
	}
	if !strings.Contains(err.Error(), "g d") {
		t.Errorf("Resolve error = %q, want it to name the canonical sequence %q", err.Error(), "g d")
	}
}

// TestResolve_UserEntryReplacesOnlyThatKey pins the merge rule: a user entry
// overwrites the default for that one (mode, key) pair, and every other
// default key -- even for the same mode -- survives untouched.
func TestResolve_UserEntryReplacesOnlyThatKey(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {
			"n":   CommandBinding("default.n"),
			"g d": CommandBinding("default.gd"),
		},
	}}
	user := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("user.n")},
	}}
	km := resolveOrFatal(t, defaults, user)
	entries := km.Entries(ModeNormal, "")

	n, ok := findEntry(entries, "n")
	if !ok || n.Binding.Command != "user.n" {
		t.Errorf("Entries[%q] = %+v, want CommandBinding(\"user.n\")", "n", n.Binding)
	}
	gd, ok := findEntry(entries, "g d")
	if !ok || gd.Binding.Command != "default.gd" {
		t.Errorf("Entries[%q] = %+v, want the untouched default CommandBinding(\"default.gd\")", "g d", gd.Binding)
	}
}

// TestResolve_UnbindIsRecorded pins that a user entry of BindingUnbound
// replaces the default binding and is preserved as BindingUnbound in the
// resolved table -- not silently dropped back to the default, and not
// collapsed into the zero-value BindingInvalid.
func TestResolve_UnbindIsRecorded(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("default.n")},
	}}
	user := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": UnboundBinding()},
	}}
	km := resolveOrFatal(t, defaults, user)

	entry, ok := findEntry(km.Entries(ModeNormal, ""), "n")
	if !ok {
		t.Fatalf("Entries missing key %q after unbind", "n")
	}
	if entry.Binding.Kind != BindingUnbound {
		t.Errorf("Entries[%q].Binding.Kind = %v, want BindingUnbound", "n", entry.Binding.Kind)
	}
}

// TestKeymap_Entries_Deterministic asserts repeated calls return the entries
// in the same order, so a caller (e.g. a future help/which-key view) can
// render a stable list without sorting it itself.
func TestKeymap_Entries_Deterministic(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {
			"n":   CommandBinding("cmd.n"),
			"g d": CommandBinding("cmd.gd"),
			"g r": CommandBinding("cmd.gr"),
			"z":   CommandBinding("cmd.z"),
		},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	first := km.Entries(ModeNormal, "")
	for i := 0; i < 5; i++ {
		got := km.Entries(ModeNormal, "")
		if len(got) != len(first) {
			t.Fatalf("Entries() call %d returned %d entries, want %d", i, len(got), len(first))
		}
		for j := range first {
			if got[j] != first[j] {
				t.Fatalf("Entries() call %d differs at index %d: got %+v, want %+v", i, j, got[j], first[j])
			}
		}
	}
}

// TestKeymap_Entries_ReturnsCopies asserts mutating a slice returned by
// Entries does not perturb the Keymap's internal state or a subsequent call
// -- required for Keymap to be safely shared across Board's value copies.
func TestKeymap_Entries_ReturnsCopies(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("cmd.n")},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	first := km.Entries(ModeNormal, "")
	if len(first) == 0 {
		t.Fatal("Entries returned no entries, want at least one")
	}
	first[0].Binding.Command = "mutated"
	first[0].Sequence = "mutated-sequence"

	second := km.Entries(ModeNormal, "")
	entry, ok := findEntry(second, "n")
	if !ok {
		t.Fatalf("Entries after mutation of a previous slice lost key %q; got %+v", "n", second)
	}
	if entry.Binding.Command != "cmd.n" {
		t.Errorf("Entries()[%q].Binding.Command = %q after mutating an earlier returned slice, want unaffected %q", "n", entry.Binding.Command, "cmd.n")
	}
}

// TestKeymap_Entries_EmptyColumnUsesModeTableOnly asserts that passing an
// empty column name returns the mode's base table, unaffected by any
// column overlay.
func TestKeymap_Entries_EmptyColumnUsesModeTableOnly(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("default.n")},
	}}
	user := Tables{Columns: map[string]Table{
		"backlog": {"n": CommandBinding("column.n")},
	}}
	km := resolveOrFatal(t, defaults, user)

	entry, ok := findEntry(km.Entries(ModeNormal, ""), "n")
	if !ok || entry.Binding.Command != "default.n" {
		t.Errorf("Entries(ModeNormal, \"\")[%q] = %+v, want the un-overlaid default", "n", entry.Binding)
	}
}

// TestResolve_ColumnKeysMatchedCaseInsensitively pins that Tables.Columns
// keys are lowercased on entry and matched case-insensitively -- mirroring
// resolveAction's strings.EqualFold column-title matching.
func TestResolve_ColumnKeysMatchedCaseInsensitively(t *testing.T) {
	user := Tables{Columns: map[string]Table{
		"Backlog": {"n": CommandBinding("column.n")},
	}}
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("default.n")},
	}}
	km := resolveOrFatal(t, defaults, user)

	for _, column := range []string{"Backlog", "BACKLOG", "backlog", "BackLog"} {
		entry, ok := findEntry(km.Entries(ModeNormal, column), "n")
		if !ok || entry.Binding.Command != "column.n" {
			t.Errorf("Entries(ModeNormal, %q)[%q] = %+v, want the column overlay CommandBinding(\"column.n\")", column, "n", entry.Binding)
		}
	}
}

// TestResolve_ColumnOverlayLeavesOtherColumnsUntouched asserts a column
// overlay only affects lookups/entries for its own column name -- a
// different column falls back to the un-overlaid mode table.
func TestResolve_ColumnOverlayLeavesOtherColumnsUntouched(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("default.n")},
	}}
	user := Tables{Columns: map[string]Table{
		"backlog": {"n": CommandBinding("column.n")},
	}}
	km := resolveOrFatal(t, defaults, user)

	entry, ok := findEntry(km.Entries(ModeNormal, "in-progress"), "n")
	if !ok || entry.Binding.Command != "default.n" {
		t.Errorf("Entries(ModeNormal, %q)[%q] = %+v, want the un-overlaid default", "in-progress", "n", entry.Binding)
	}
}

// TestResolve_UserModeUnbindWinsOverDefaultColumn pins the merge-precedence
// contract: default-mode < default-column < user-mode < user-column. A
// default column entry must never override a user's explicit mode-level
// entry (including an explicit unbind) -- the column-overlaid table for
// that (mode, column) must reflect the user mode-level unbind, not the
// default column's binding.
func TestResolve_UserModeUnbindWinsOverDefaultColumn(t *testing.T) {
	defaults := Tables{
		Modes: map[Mode]Table{
			ModeNormal: {"K": CommandBinding("default.mode.K")},
		},
		Columns: map[string]Table{
			"backlog": {"K": CommandBinding("default.column.K")},
		},
	}
	user := Tables{
		Modes: map[Mode]Table{
			ModeNormal: {"K": UnboundBinding()},
		},
	}
	km := resolveOrFatal(t, defaults, user)

	entry, ok := findEntry(km.Entries(ModeNormal, "backlog"), "K")
	if !ok || entry.Binding.Kind != BindingUnbound {
		t.Errorf("Entries(ModeNormal, \"backlog\")[%q] = (found=%v) %+v, want BindingUnbound (user mode-level unbind must beat the default column binding)", "K", ok, entry.Binding)
	}
}

// TestResolve_FullPrecedenceOrder pins the complete four-layer merge order
// on a single key: default-mode < default-column < user-mode < user-column.
// Each layer sets a distinct binding for the same key; only the user-column
// binding should survive in the column-overlaid table.
func TestResolve_FullPrecedenceOrder(t *testing.T) {
	defaults := Tables{
		Modes: map[Mode]Table{
			ModeNormal: {"K": CommandBinding("default.mode.K")},
		},
		Columns: map[string]Table{
			"backlog": {"K": CommandBinding("default.column.K")},
		},
	}
	user := Tables{
		Modes: map[Mode]Table{
			ModeNormal: {"K": CommandBinding("user.mode.K")},
		},
		Columns: map[string]Table{
			"backlog": {"K": CommandBinding("user.column.K")},
		},
	}
	km := resolveOrFatal(t, defaults, user)

	entry, ok := findEntry(km.Entries(ModeNormal, "backlog"), "K")
	if !ok || entry.Binding.Command != "user.column.K" {
		t.Errorf("Entries(ModeNormal, \"backlog\")[%q] = %+v, want the user-column binding CommandBinding(\"user.column.K\") to win over all other layers", "K", entry.Binding)
	}
}

// TestResolve_RejectsCollidingColumnNamesAfterLowercasing pins that two
// distinct raw column names in the same layer that lowercase to the same
// value are rejected, mirroring normalizeTable's raw-key collision check --
// Resolve can't silently drop one column's table via last-write-wins map
// iteration order.
func TestResolve_RejectsCollidingColumnNamesAfterLowercasing(t *testing.T) {
	defaults := Tables{Columns: map[string]Table{
		"Backlog": {"n": CommandBinding("upper.n")},
		"backlog": {"n": CommandBinding("lower.n")},
	}}
	_, err := Resolve(defaults, Tables{})
	if err == nil {
		t.Fatal("Resolve returned nil error, want an error naming the colliding column names")
	}
	if !strings.Contains(err.Error(), "Backlog") || !strings.Contains(err.Error(), "backlog") {
		t.Errorf("Resolve error = %q, want it to name both colliding raw column names %q and %q", err.Error(), "Backlog", "backlog")
	}
}

// TestResolve_ColumnOverlayAppliesToNormalAndDetailOnly pins that the
// per-column overlay is only precomputed/applied for ModeNormal and
// ModeDetail -- passing a column name for any other mode must not leak the
// column's bindings into that mode's entries.
func TestResolve_ColumnOverlayAppliesToNormalAndDetailOnly(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("normal.default.n")},
		ModeDetail: {"n": CommandBinding("detail.default.n")},
		ModeSearch: {"n": CommandBinding("search.default.n")},
	}}
	user := Tables{Columns: map[string]Table{
		"backlog": {"n": CommandBinding("column.n")},
	}}
	km := resolveOrFatal(t, defaults, user)

	normalEntry, ok := findEntry(km.Entries(ModeNormal, "backlog"), "n")
	if !ok || normalEntry.Binding.Command != "column.n" {
		t.Errorf("Entries(ModeNormal, \"backlog\")[%q] = %+v, want the column overlay", "n", normalEntry.Binding)
	}
	detailEntry, ok := findEntry(km.Entries(ModeDetail, "backlog"), "n")
	if !ok || detailEntry.Binding.Command != "column.n" {
		t.Errorf("Entries(ModeDetail, \"backlog\")[%q] = %+v, want the column overlay", "n", detailEntry.Binding)
	}
	searchEntry, ok := findEntry(km.Entries(ModeSearch, "backlog"), "n")
	if !ok || searchEntry.Binding.Command != "search.default.n" {
		t.Errorf("Entries(ModeSearch, \"backlog\")[%q] = %+v, want the un-overlaid ModeSearch default (columns only overlay normal/detail)", "n", searchEntry.Binding)
	}
}
