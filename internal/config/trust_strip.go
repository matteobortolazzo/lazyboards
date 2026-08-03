package config

import (
	"strings"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// stripLocalShellSinks removes every keystroke-triggered, local-origin
// shell-executing construct from cfg before it can reach the
// merge/translate/resolve pipeline: inline keymaps: shell bindings (mode and
// column tables) and legacy actions:/columns[].actions: shell entries the
// local document itself declared. Load() calls this only when the local
// file's content hash isn't in the caller's trust store -- a stripped sink
// never produces an error, it is silently absent from the resolved keymap
// (a future ticket adds a startup notice).
//
// globalActions/globalColumns are the snapshots Load() took of the global
// document BEFORE the local unmarshal ran (see the comments on
// globalActions/globalColumns in config.go): they are the trusted source of
// truth this function compares cfg.Actions/cfg.Columns against to decide
// strip-eligibility by value, not by re-walking the local document's raw
// YAML nodes (see stripShellFromActions for why the raw-node approach was a
// security bug -- a YAML merge key can populate cfg.Actions/cfg.Columns
// without ever appearing as a literal mapping key the raw-node walk can
// find). Global-declared shell constructs are never touched, whatever the
// local document's trust state (AC9): they always compare byte-identical to
// their own global snapshot entry.
//
// decls is never consulted to decide what gets stripped, but it is not
// passed through untouched: stripShellFromActions mutates
// decls.ActionKeys as a side effect, deleting each key it strips so a
// key that falls back to global after being stripped is treated as if the
// local document never declared it at all. That mutation is consumed only
// by Load()'s later Order-offset bookkeeping (a cosmetic rendering concern,
// see config.go).
func stripLocalShellSinks(cfg *Config, decls localDecls, globalActions map[string]Action, globalColumns []ColumnConfig) {
	stripShellFromKeymapTable(cfg.Keymaps)
	stripShellFromActions(cfg, decls, globalActions, globalColumns)
}

// stripShellFromKeymapTable removes every binding whose Kind is
// keymap.BindingAction and whose Action.Type is "shell" from km's mode and
// column tables. If stripping empties a mode or column table entirely, the
// whole entry is deleted from the map rather than left as an
// explicit-but-empty table: mergeKeymaps distinguishes "mode/column never
// mentioned" (inherit the whole matching global table) from "mode/column
// declared, even as an empty table" (inherit nothing), and a stripped-down
// local table must fall into the former so every other global key at that
// mode/column -- not just the stripped one -- still resolves.
func stripShellFromKeymapTable(km *Keymaps) {
	if km == nil {
		return
	}
	for mode, table := range km.Modes {
		stripShellBindings(table)
		if len(table) == 0 {
			delete(km.Modes, mode)
		}
	}
	for name, table := range km.Columns {
		stripShellBindings(table)
		if len(table) == 0 {
			delete(km.Columns, name)
		}
	}
}

// stripShellBindings deletes every shell-action binding from table in
// place. Only Kind == keymap.BindingAction && Action.Type == "shell" is
// ever removed -- a type: url binding or a binding to a catalogued built-in
// command id is never a candidate, regardless of trust state.
func stripShellBindings(table KeymapTable) {
	for key, binding := range table {
		if binding.Kind == keymap.BindingAction && binding.Action.Type == "shell" {
			delete(table, key)
		}
	}
}

// stripShellFromActions removes local-origin shell actions from cfg.Actions
// and every cfg.Columns[i].Actions, gated by a value comparison against the
// trusted globalActions/globalColumns snapshots instead of the raw-node
// "was this key mentioned in the local document" walk the previous
// implementation used (assignActionOrder's decls.ActionKeys/decls.HasColumns
// -- decls.ActionKeys is still consumed here, but only to keep a stripped
// key's later Order-offset bookkeeping identical to Load()'s pre-existing,
// non-security cosmetic behavior; decls.HasColumns is gone entirely, and
// neither field is ever consulted to decide what gets stripped anymore).
//
// The raw-node walk was a security bug: cfg.Actions/cfg.Columns have no
// custom UnmarshalYAML, so yaml.v3's own generic decoder -- which resolves
// YAML merge keys (`<<: *anchor`) -- populates them directly, while
// assignActionOrder's hand-rolled walk over the literal yaml.Node tree never
// expands a merge key. A local document could therefore smuggle a shell
// action into cfg.Actions/cfg.Columns via a merge key without it ever
// appearing as a literal mapping key the raw-node walk could find, leaving
// decls.ActionKeys[key]/decls.HasColumns false for an entry that had
// genuine (if indirect) local provenance -- the old gate then never
// stripped it.
//
// Comparing against the global snapshot instead makes provenance a value
// check: an entry present in cfg.Actions[key]/cfg.Columns[i].Actions[key]
// that is byte-identical to the corresponding globalActions[key]/matching
// global column's Actions[key] entry could only have come from the global
// document (Action is a plain comparable struct, no slice/map fields), so
// it is never stripped, whatever the local document's trust state (AC9).
// Anything else -- genuinely local, or merge-key-smuggled -- gets stripped
// unconditionally. This fails safe (over-strip, never under-strip): the
// worst case is an identical-looking local override losing its Order
// stamp and falling back to the global entry, never a local override
// surviving that shouldn't.
func stripShellFromActions(cfg *Config, decls localDecls, globalActions map[string]Action, globalColumns []ColumnConfig) {
	for key, action := range cfg.Actions {
		if action.Type != "shell" {
			continue
		}
		if g, ok := globalActions[key]; ok && g == action {
			continue // byte-identical to the global entry: genuinely global, not stripped
		}
		delete(cfg.Actions, key)
		// Cosmetic only (see Load()'s Order-offset comment): a key that
		// falls back to global after being stripped renders the same as a
		// key the local document never declared at all.
		delete(decls.ActionKeys, key)
	}

	globalColumnsByName := columnsByNameLower(globalColumns)
	for i := range cfg.Columns {
		gc, hasGlobalCol := globalColumnsByName[strings.ToLower(cfg.Columns[i].Name)]
		strippedAny := false
		for key, action := range cfg.Columns[i].Actions {
			if action.Type != "shell" {
				continue
			}
			if hasGlobalCol {
				if g, ok := gc.Actions[key]; ok && g == action {
					continue // byte-identical to the matching global column's entry
				}
			}
			delete(cfg.Columns[i].Actions, key)
			strippedAny = true
		}
		// Only reset to nil ("no local declaration, inherit global") when
		// this pass actually deleted something. An untrusted local column
		// that already declared an explicit, empty actions: {} (nothing
		// shell to strip at all) must stay explicit-empty -- resetting it
		// to nil purely because its post-strip length happens to be zero
		// would silently re-enable inherited global column actions that
		// today's existing (non-security) merge semantics say should stay
		// suppressed, diverging an untrusted load from a trusted load of
		// the identical bytes (LOW finding).
		if strippedAny && len(cfg.Columns[i].Actions) == 0 {
			cfg.Columns[i].Actions = nil
		}
	}
}
