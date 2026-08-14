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
	// CleanupFields is the number of cleanup:/columns[].cleanup fields
	// removed (top-level counts as 1, plus 1 per column stripped).
	CleanupFields int
}

// stripLocalShellSinks removes every local-origin shell-executing construct
// from cfg before it can reach the merge/resolve pipeline: inline keymaps:
// shell bindings (mode and column tables) and cleanup:/columns[].cleanup
// commands the local document itself declared. Load() calls this only when
// the local file's content hash isn't in the caller's trust store -- a
// stripped sink never produces an error, it is silently absent from the
// resolved config; Load() surfaces the returned stripCounts as a single
// Config.Notices entry (see buildStripNotice) when any count is non-zero.
//
// globalKeymaps/globalColumns/globalCleanup are the snapshots Load() took of
// the global document BEFORE the local unmarshal ran (see their comments in
// config.go): they are the trusted source of truth this function compares
// cfg.Keymaps/cfg.Columns/cfg.Cleanup against to decide strip-eligibility by
// value, not by re-walking the local document's raw YAML nodes. The raw-node
// approach was a security bug: a YAML merge key can populate cfg.Columns/
// cfg.Keymaps without ever appearing as a literal mapping key such a walk
// could find, so a smuggled shell construct would never be stripped.
// Global-declared shell constructs are never touched, whatever the local
// document's trust state (AC9): they always compare equal (Order aside --
// see sameShellAction/sameShellBinding) to their own global snapshot entry.
func stripLocalShellSinks(cfg *Config, globalKeymaps *Keymaps, globalColumns []ColumnConfig, globalCleanup *string) stripCounts {
	return stripCounts{
		KeymapBindings: stripShellFromKeymapTable(cfg.Keymaps, globalKeymaps),
		CleanupFields:  stripLocalCleanup(cfg, globalCleanup, globalColumns),
	}
}

// sameShellAction reports whether a and b represent the same executing
// shell action, ignoring each side's own derived Order (see the doc comment
// on Action.Order in config.go): Order reflects document position, is never
// execution-relevant, and a YAML alias/merge-key-populated keymaps: block
// leaves every aliased entry's Order at 0 regardless of the aliased
// content's true position (stampKeymapsOrder bails out as soon as the value
// node isn't a MappingNode) -- comparing it would spuriously treat a
// global-equivalent aliased or merely-repositioned local entry as different,
// which is exactly the false-positive strip #568 fixes.
// Zeroing Order and then comparing by value (rather than listing the
// remaining fields explicitly) means any future field added to Action is
// compared by default -- fail-safe, since an unlisted field would otherwise
// be silently ignored by the comparison.
func sameShellAction(a, b Action) bool {
	a.Order = 0
	b.Order = 0
	return a == b
}

// sameShellBinding reports whether a and b are the same executing keymap
// binding: same Kind, same Command, and a sameShellAction-equivalent Action,
// ignoring each side's own derived KeymapBinding.Order the same way
// sameShellAction ignores the nested Action.Order (stampKeymapsOrder has the
// identical AliasNode bail-out for a `keymaps: *anchor` document). Neutralizing
// both Order fields and the already-checked Action before falling back to
// plain struct equality -- rather than listing KeymapBinding's remaining
// fields by hand -- keeps the comparison fail-safe as KeymapBinding grows.
func sameShellBinding(a, b KeymapBinding) bool {
	if !sameShellAction(a.Action, b.Action) {
		return false
	}
	a.Order, b.Order = 0, 0
	a.Action, b.Action = Action{}, Action{}
	return a == b
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
	if counts.CleanupFields > 0 {
		kinds = append(kinds, fmt.Sprintf("%d cleanup field(s)", counts.CleanupFields))
	}
	if len(kinds) == 0 {
		return ""
	}
	return fmt.Sprintf("untrusted .lazyboards.yml: stripped %s -- run `lazyboards trust` to allow this file's shell commands", strings.Join(kinds, ", "))
}

// stripShellFromKeymapTable removes every local shell-action binding from
// km's mode and column tables whose value doesn't match the corresponding
// entry in globalKeymaps (the pre-local snapshot Load() took -- see the
// comment on globalKeymaps in config.go): a local binding sameShellBinding-
// equivalent to its global counterpart is genuinely global-equivalent and is
// left alone, uncounted, by the same value-comparison gate. Mode tables are matched by exact keymap.Mode key; column tables are
// matched case-insensitively, mirroring mergeKeymaps' globalColumnsByLower.
//
// If stripping actually removed something AND that emptied a mode or column
// table entirely, the whole entry is deleted from the map rather than left
// as an explicit-but-empty table: mergeKeymaps distinguishes "mode/column
// never mentioned" (inherit the whole matching global table) from
// "mode/column declared, even as an empty table" (inherit nothing), and a
// stripped-down local table must fall into the former so every other global
// key at that mode/column -- not just the stripped one -- still resolves.
// The "actually stripped something" guard matters: an untrusted local table
// that was already explicit-and-empty (keymaps: {normal: {}}), with nothing
// to strip, must keep its explicit-empty "no bindings" meaning instead of
// being reinterpreted as "never declared" and wrongly inheriting the whole
// global table.
func stripShellFromKeymapTable(km, globalKeymaps *Keymaps) int {
	if km == nil {
		return 0
	}
	count := 0

	var globalModes map[keymap.Mode]KeymapTable
	var globalColumnsByLower map[string]KeymapTable
	if globalKeymaps != nil {
		globalModes = globalKeymaps.Modes
		globalColumnsByLower = make(map[string]KeymapTable, len(globalKeymaps.Columns))
		for name, table := range globalKeymaps.Columns {
			globalColumnsByLower[strings.ToLower(name)] = table
		}
	}

	for mode, table := range km.Modes {
		stripped := stripShellBindings(table, globalModes[mode])
		count += stripped
		if stripped > 0 && len(table) == 0 {
			delete(km.Modes, mode)
		}
	}
	for name, table := range km.Columns {
		stripped := stripShellBindings(table, globalColumnsByLower[strings.ToLower(name)])
		count += stripped
		if stripped > 0 && len(table) == 0 {
			delete(km.Columns, name)
		}
	}
	return count
}

// stripShellBindings deletes every local-origin shell-action binding from
// table in place, returning the number of bindings removed. Only Kind ==
// keymap.BindingAction && Action.Type == "shell" is ever a candidate -- a
// type: url binding, a binding to a catalogued built-in command id, or an
// explicit unbind is never removed, regardless of trust state. A shell
// binding that is sameShellBinding-equivalent to globalTable's entry at the
// same key (globalTable may be nil, e.g. when the mode/column has no
// global-declared counterpart at all) is genuinely global-equivalent and is
// left in place, uncounted.
func stripShellBindings(table, globalTable KeymapTable) int {
	count := 0
	for key, binding := range table {
		if binding.Kind != keymap.BindingAction || binding.Action.Type != "shell" {
			continue
		}
		if g, ok := globalTable[key]; ok && sameShellBinding(g, binding) {
			continue // equivalent (Order aside) to the matching global entry: genuinely global, not stripped
		}
		delete(table, key)
		count++
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
// *string pointee). Per-column matching is by lowercased name: when
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
