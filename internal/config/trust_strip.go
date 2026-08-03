package config

import (
	"fmt"
	"strings"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// stripCounts tallies how many entries of each local-origin shell-sink kind
// stripLocalShellSinks actually removed from cfg, so Load() can decide
// whether to append a strip notice (see buildStripNotice) and, if so, name
// each stripped kind with its count. A zero-value stripCounts means nothing
// was stripped.
type stripCounts struct {
	// KeymapBindings is the number of inline keymaps: shell bindings removed
	// (mode and column tables combined).
	KeymapBindings int
	// LegacyActions is the number of legacy actions:/columns[].actions: shell
	// entries removed (top-level and per-column combined).
	LegacyActions int
	// CleanupFields is the number of cleanup:/columns[].cleanup fields
	// removed (top-level counts as 1, plus 1 per column stripped).
	CleanupFields int
}

// stripLocalShellSinks removes every local-origin shell-executing construct
// from cfg before it can reach the merge/translate/resolve pipeline: inline
// keymaps: shell bindings (mode and column tables), legacy actions:/
// columns[].actions: shell entries, and cleanup:/columns[].cleanup commands
// the local document itself declared. Load() calls this only when the local
// file's content hash isn't in the caller's trust store -- a stripped sink
// never produces an error, it is silently absent from the resolved config;
// Load() surfaces the returned stripCounts as a single Config.Notices entry
// (see buildStripNotice) when any count is non-zero.
//
// globalActions/globalColumns/globalCleanup are the snapshots Load() took of
// the global document BEFORE the local unmarshal ran (see the comments on
// globalActions/globalColumns/globalCleanup in config.go): they are the
// trusted source of truth this function compares cfg.Actions/cfg.Columns/
// cfg.Cleanup against to decide strip-eligibility by value, not by
// re-walking the local document's raw YAML nodes (see stripShellFromActions
// for why the raw-node approach was a security bug -- a YAML merge key can
// populate cfg.Actions/cfg.Columns without ever appearing as a literal
// mapping key the raw-node walk can find). Global-declared shell constructs
// are never touched, whatever the local document's trust state (AC9): they
// always compare byte-identical to their own global snapshot entry.
//
// decls is never consulted to decide what gets stripped, but it is not
// passed through untouched: stripShellFromActions mutates
// decls.ActionKeys as a side effect, deleting each key it strips so a
// key that falls back to global after being stripped is treated as if the
// local document never declared it at all. That mutation is consumed only
// by Load()'s later Order-offset bookkeeping (a cosmetic rendering concern,
// see config.go).
func stripLocalShellSinks(cfg *Config, decls localDecls, globalActions map[string]Action, globalColumns []ColumnConfig, globalCleanup *string) stripCounts {
	return stripCounts{
		KeymapBindings: stripShellFromKeymapTable(cfg.Keymaps),
		LegacyActions:  stripShellFromActions(cfg, decls, globalActions, globalColumns),
		CleanupFields:  stripLocalCleanup(cfg, globalCleanup, globalColumns),
	}
}

// buildStripNotice returns a single human-readable notice describing which
// local-origin shell sinks were stripped as untrusted, or "" if counts is
// entirely zero (Load() only appends a Config.Notices entry when this
// returns non-empty). It always names the literal ".lazyboards.yml" -- never
// the local path argument passed to Load, which can be a long absolute or
// temp path (Q2) -- and the exact `lazyboards trust` invocation a user runs
// to stop the stripping once they've reviewed the file.
func buildStripNotice(counts stripCounts) string {
	var kinds []string
	if counts.KeymapBindings > 0 {
		kinds = append(kinds, fmt.Sprintf("%d keymap shell binding(s)", counts.KeymapBindings))
	}
	if counts.LegacyActions > 0 {
		kinds = append(kinds, fmt.Sprintf("%d legacy shell action(s)", counts.LegacyActions))
	}
	if counts.CleanupFields > 0 {
		kinds = append(kinds, fmt.Sprintf("%d cleanup field(s)", counts.CleanupFields))
	}
	if len(kinds) == 0 {
		return ""
	}
	return fmt.Sprintf("untrusted .lazyboards.yml: stripped %s -- run `lazyboards trust` to allow this file's shell commands", strings.Join(kinds, ", "))
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
func stripShellFromKeymapTable(km *Keymaps) int {
	if km == nil {
		return 0
	}
	count := 0
	for mode, table := range km.Modes {
		count += stripShellBindings(table)
		if len(table) == 0 {
			delete(km.Modes, mode)
		}
	}
	for name, table := range km.Columns {
		count += stripShellBindings(table)
		if len(table) == 0 {
			delete(km.Columns, name)
		}
	}
	return count
}

// stripShellBindings deletes every shell-action binding from table in
// place, returning the number of bindings removed. Only Kind ==
// keymap.BindingAction && Action.Type == "shell" is ever removed -- a
// type: url binding or a binding to a catalogued built-in command id is
// never a candidate, regardless of trust state.
func stripShellBindings(table KeymapTable) int {
	count := 0
	for key, binding := range table {
		if binding.Kind == keymap.BindingAction && binding.Action.Type == "shell" {
			delete(table, key)
			count++
		}
	}
	return count
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
func stripShellFromActions(cfg *Config, decls localDecls, globalActions map[string]Action, globalColumns []ColumnConfig) int {
	count := 0
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
		count++
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
			count++
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
	return count
}

// stripLocalCleanup strips an untrusted local cleanup:/columns[].cleanup
// command, restoring the matching global value (nil, if none exists) so the
// existing merge steps (mergeColumnCleanup, applyDefaultCleanup) resolve it
// exactly as if the local document had never declared its own override. An
// explicit local cleanup: "" is left alone -- it is a disable directive, not
// an executing construct, and can never reach sh -c (Q1): stripping it would
// perversely restore the global command, making an untrusted load run
// strictly MORE commands than a trusted load of the same bytes.
//
// globalCleanup/globalColumns are the value-copied snapshots Load() took of
// the global document BEFORE the local unmarshal ran (see the comment on
// globalCleanup in config.go for why a value copy, not a pointer alias, is
// required -- yaml.v3's second Unmarshal reuses and overwrites the existing
// *string pointee). Per-column matching mirrors stripShellFromActions: when
// the local document declares no columns: block at all, cfg.Columns IS
// globalColumns (same backing slice), so every column's local Cleanup
// pointer is already byte-identical to its own "global" snapshot entry and
// the value comparison below is a no-op by construction -- no special case
// is needed for that aliasing trap.
func stripLocalCleanup(cfg *Config, globalCleanup *string, globalColumns []ColumnConfig) int {
	count := 0

	if cfg.Cleanup != nil && *cfg.Cleanup != "" {
		if globalCleanup == nil || *cfg.Cleanup != *globalCleanup {
			cfg.Cleanup = globalCleanup
			count++
		}
	}

	globalColumnsByName := columnsByNameLower(globalColumns)
	for i := range cfg.Columns {
		local := cfg.Columns[i].Cleanup
		if local == nil || *local == "" {
			continue
		}
		var global *string
		if gc, ok := globalColumnsByName[strings.ToLower(cfg.Columns[i].Name)]; ok {
			global = gc.Cleanup
		}
		if global == nil || *local != *global {
			cfg.Columns[i].Cleanup = nil
			count++
		}
	}

	return count
}
