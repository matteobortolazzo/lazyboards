package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"gopkg.in/yaml.v3"
)

// KeymapTable maps a raw key (or key sequence) string to its parsed
// KeymapBinding.
type KeymapTable map[string]KeymapBinding

// KeymapBinding is the parsed right-hand side of one keymaps.<mode>.<key>
// (or keymaps.columns.<name>.<key>) entry: either a built-in command id, an
// inline action definition, or an explicit unbind (`~`/null).
type KeymapBinding struct {
	Kind    keymap.BindingKind
	Command keymap.CommandID
	Action  Action
	// Order is derived metadata reflecting the binding's position in its
	// source YAML file (see stampKeymapsOrder). Never read from or written
	// to the YAML file directly.
	Order int
}

// Keymaps holds the parsed keymaps: block: a table per resolvable mode plus
// the keymaps.columns.<name> per-column overlay table.
type Keymaps struct {
	Modes   map[keymap.Mode]KeymapTable
	Columns map[string]KeymapTable
}

// UnmarshalYAML parses one keymaps.<mode>.<key> (or columns.<name>.<key>)
// right-hand side: a `!!null` scalar (covers `~`, `null`, and an empty
// value) is an explicit unbind, any other scalar is a built-in command id,
// a mapping is an inline action definition, and anything else (sequence,
// alias, ...) is a structural error.
func (b *KeymapBinding) UnmarshalYAML(node *yaml.Node) error {
	// yaml.v3 never actually dispatches to this method for a `!!null` node
	// (Decode short-circuits null scalars to the zero value before
	// consulting Unmarshaler, verified experimentally) -- parseKeymapTable
	// below checks node.Tag itself before calling Decode. This branch stays
	// as defense-in-depth for any other decode path that does reach here
	// with a null node.
	if node.Tag == "!!null" {
		*b = KeymapBinding{Kind: keymap.BindingUnbound}
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		var command string
		if err := node.Decode(&command); err != nil {
			return fmt.Errorf("must be a command string: %w", err)
		}
		*b = KeymapBinding{Kind: keymap.BindingCommand, Command: keymap.CommandID(command)}
		return nil
	case yaml.MappingNode:
		var action Action
		if err := node.Decode(&action); err != nil {
			return fmt.Errorf("must be a valid inline action mapping: %w", err)
		}
		*b = KeymapBinding{Kind: keymap.BindingAction, Action: action}
		return nil
	default:
		return fmt.Errorf("must be a command string, an inline action mapping, or ~/null to unbind")
	}
}

// MarshalYAML implements a deterministic marshal order for a single binding:
// an explicit unbind marshals back to null, a command binding to its bare
// string, an inline action to its mapping.
func (b KeymapBinding) MarshalYAML() (interface{}, error) {
	switch b.Kind {
	case keymap.BindingUnbound:
		return nil, nil
	case keymap.BindingCommand:
		return string(b.Command), nil
	case keymap.BindingAction:
		return b.Action, nil
	default:
		return nil, nil
	}
}

// parseKeymapTable parses a YAML mapping node of key -> binding entries.
func parseKeymapTable(node *yaml.Node) (KeymapTable, error) {
	if node.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("must be a mapping")
	}
	table := make(KeymapTable, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		valueNode := node.Content[i+1]
		// parseKeymapTable walks node.Content manually instead of decoding
		// into a Go map, which bypasses yaml.v3's own duplicate-key
		// detection -- check for a repeated key explicitly so the same key
		// appearing twice in one table doesn't silently last-write-win.
		if _, exists := table[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		// yaml.v3's Decode short-circuits a `!!null` node (`~`, `null`, or
		// an empty value) to the target's zero value without ever calling
		// Unmarshaler -- checked here, ahead of Decode, so an explicit
		// unbind is parsed correctly instead of silently becoming a
		// zero-value BindingInvalid entry.
		if valueNode.Tag == "!!null" {
			table[key] = KeymapBinding{Kind: keymap.BindingUnbound}
			continue
		}
		var binding KeymapBinding
		if err := valueNode.Decode(&binding); err != nil {
			return nil, fmt.Errorf("%q: %w", key, err)
		}
		table[key] = binding
	}
	return table, nil
}

// UnmarshalYAML parses the whole keymaps: block. It always resets the
// receiver to fresh maps (replace semantics) rather than merging into
// whatever the receiver already holds: a second yaml.Unmarshal into the
// same *Config reuses the existing non-nil *Keymaps pointer and calls
// UnmarshalYAML again on the already-populated value, so a merge-in-place
// implementation would silently accumulate stale entries across Load()'s
// global-then-local unmarshal pair.
func (k *Keymaps) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("keymaps: must be a mapping")
	}

	modes := make(map[keymap.Mode]KeymapTable)
	columns := make(map[string]KeymapTable)
	seenModes := make(map[keymap.Mode]bool, len(node.Content)/2)
	columnNameByLower := make(map[string]string)

	for i := 0; i+1 < len(node.Content); i += 2 {
		modeName := node.Content[i].Value
		valueNode := node.Content[i+1]

		mode, err := keymap.ParseMode(modeName)
		if err != nil {
			return fmt.Errorf("keymaps.%q: %w", modeName, err)
		}
		if seenModes[mode] {
			return fmt.Errorf("keymaps: duplicate mode %q", modeName)
		}
		seenModes[mode] = true

		if mode == keymap.ModeColumns {
			if valueNode.Kind != yaml.MappingNode {
				return fmt.Errorf("keymaps.columns: must be a mapping")
			}
			for j := 0; j+1 < len(valueNode.Content); j += 2 {
				colName := valueNode.Content[j].Value
				lower := strings.ToLower(colName)
				if prev, exists := columnNameByLower[lower]; exists {
					return fmt.Errorf("keymaps.columns: %q and %q both normalize to %q", prev, colName, lower)
				}
				columnNameByLower[lower] = colName

				table, err := parseKeymapTable(valueNode.Content[j+1])
				if err != nil {
					return fmt.Errorf("keymaps.columns.%q: %w", colName, err)
				}
				columns[colName] = table
			}
			continue
		}

		table, err := parseKeymapTable(valueNode)
		if err != nil {
			return fmt.Errorf("keymaps.%q: %w", modeName, err)
		}
		modes[mode] = table
	}

	k.Modes = modes
	k.Columns = columns
	return nil
}

// MarshalYAML (value receiver, so it fires through the *Keymaps field on
// Config) rebuilds the keymaps: block as an explicit yaml.Node tree so key
// order is deterministic instead of Go's randomized map order: mode blocks
// in keymap.Modes() order with columns last; within a table, keys ordered
// by Order (ties broken by name), so declared-key order survives a
// Load()/Save() round trip once Order has been stamped from the raw
// document.
func (k Keymaps) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{Kind: yaml.MappingNode}

	for _, mode := range keymap.Modes() {
		table, ok := k.Modes[mode]
		if !ok {
			continue
		}
		tableNode, err := marshalKeymapTableNode(table)
		if err != nil {
			return nil, err
		}
		node.Content = append(node.Content, yamlStringNode(string(mode)), tableNode)
	}

	if len(k.Columns) > 0 {
		columnNames := make([]string, 0, len(k.Columns))
		for name := range k.Columns {
			columnNames = append(columnNames, name)
		}
		sort.Strings(columnNames)

		columnsNode := &yaml.Node{Kind: yaml.MappingNode}
		for _, name := range columnNames {
			tableNode, err := marshalKeymapTableNode(k.Columns[name])
			if err != nil {
				return nil, err
			}
			columnsNode.Content = append(columnsNode.Content, yamlStringNode(name), tableNode)
		}
		node.Content = append(node.Content, yamlStringNode(string(keymap.ModeColumns)), columnsNode)
	}

	return node, nil
}

// marshalKeymapTableNode builds the mapping node for one KeymapTable, with
// keys ordered by Order (falling back to name order when Order is 0 --
// Config literals built directly in tests never pass through the node
// walk, so the marshaler must stay deterministic without it).
func marshalKeymapTableNode(table KeymapTable) (*yaml.Node, error) {
	keys := make([]string, 0, len(table))
	for key := range table {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		oi, oj := table[keys[i]].Order, table[keys[j]].Order
		if oi != oj {
			return oi < oj
		}
		return keys[i] < keys[j]
	})

	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, key := range keys {
		valueNode := &yaml.Node{}
		if err := valueNode.Encode(table[key]); err != nil {
			return nil, err
		}
		node.Content = append(node.Content, yamlStringNode(key), valueNode)
	}
	return node, nil
}

func yamlStringNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

// cloneKeymapTable returns a shallow copy of table, so a column/mode
// inherited verbatim from global config doesn't alias the original map.
func cloneKeymapTable(table KeymapTable) KeymapTable {
	out := make(KeymapTable, len(table))
	for k, v := range table {
		out[k] = v
	}
	return out
}

// mergeKeymapTable merges globalTable into localTable in place: local
// entries win per key (including an explicit local unbind, which must
// survive untouched); global-only keys are added afterward, each Order
// offset by localTable's own key count so the merged group's relative
// order matches Load()'s existing action-merge convention (see
// the per-column merge below).
func mergeKeymapTable(localTable, globalTable KeymapTable) {
	localCount := len(localTable)
	for key, binding := range globalTable {
		if _, exists := localTable[key]; exists {
			continue
		}
		binding.Order += localCount
		localTable[key] = binding
	}
}

// mergeKeymaps merges global into local's per-mode and per-column keymap
// tables and returns the result (mutating local in place when non-nil).
//
// Per mode: a mode local never mentioned at all inherits the whole global
// table; an explicit local table (including an explicit empty one) is
// merged key-by-key with mergeKeymapTable, so an explicit empty table means
// "no bindings" (the nil-vs-empty distinction).
//
// Per column: same nil-vs-empty distinction, matching the mode tables
// exactly, with case-insensitive column-name matching.
//
// Both local and global may be nil; the result is nil only when both are.
func mergeKeymaps(local, global *Keymaps) *Keymaps {
	if local == nil && global == nil {
		return nil
	}
	if local == nil {
		local = &Keymaps{}
	}
	if local.Modes == nil {
		local.Modes = make(map[keymap.Mode]KeymapTable)
	}
	if local.Columns == nil {
		local.Columns = make(map[string]KeymapTable)
	}
	if global == nil {
		return local
	}

	for mode, globalTable := range global.Modes {
		localTable, exists := local.Modes[mode]
		if !exists {
			local.Modes[mode] = cloneKeymapTable(globalTable)
			continue
		}
		if len(localTable) == 0 {
			continue
		}
		mergeKeymapTable(localTable, globalTable)
	}

	globalColumnsByLower := make(map[string]KeymapTable, len(global.Columns))
	for name, table := range global.Columns {
		globalColumnsByLower[strings.ToLower(name)] = table
	}

	// Snapshot local column names before the fill-in loop below adds
	// global-only columns to local.Columns.
	localColumnNamesByLower := make(map[string]bool, len(local.Columns))
	for name := range local.Columns {
		localColumnNamesByLower[strings.ToLower(name)] = true
	}

	for name, localTable := range local.Columns {
		globalTable, found := globalColumnsByLower[strings.ToLower(name)]
		if !found || len(globalTable) == 0 {
			continue
		}
		if len(localTable) == 0 {
			continue
		}
		mergeKeymapTable(localTable, globalTable)
	}

	for name, globalTable := range global.Columns {
		if localColumnNamesByLower[strings.ToLower(name)] {
			continue
		}
		local.Columns[name] = cloneKeymapTable(globalTable)
	}

	return local
}

// Tables converts Keymaps into the keymap.Tables shape keymap.Resolve
// consumes: a straight field copy of each binding's Kind/Command/Action.
// Per A4 (reversing #492's deferral), KeymapBinding.Order -- derived
// config-layer metadata reflecting document position, used by #489's
// runtime hint derivation to order inline-action hints -- now flows through
// into the resolved keymap.Action.Order, so the engine layer can order
// hints without a second, config-layer-only lookup.
func (k *Keymaps) Tables() keymap.Tables {
	if k == nil {
		return keymap.Tables{}
	}
	modes := make(map[keymap.Mode]keymap.Table, len(k.Modes))
	for mode, table := range k.Modes {
		modes[mode] = convertKeymapTable(table)
	}
	columns := make(map[string]keymap.Table, len(k.Columns))
	for name, table := range k.Columns {
		columns[name] = convertKeymapTable(table)
	}
	return keymap.Tables{Modes: modes, Columns: columns}
}

func convertKeymapTable(table KeymapTable) keymap.Table {
	out := make(keymap.Table, len(table))
	for key, binding := range table {
		out[key] = convertKeymapBinding(binding)
	}
	return out
}

func convertKeymapBinding(b KeymapBinding) keymap.Binding {
	switch b.Kind {
	case keymap.BindingCommand:
		return keymap.CommandBinding(b.Command)
	case keymap.BindingAction:
		return keymap.ActionBinding(keymap.Action{
			Name:    b.Action.Name,
			Type:    b.Action.Type,
			URL:     b.Action.URL,
			Command: b.Action.Command,
			Scope:   b.Action.Scope,
			Order:   b.Order,
		})
	case keymap.BindingUnbound:
		return keymap.UnboundBinding()
	default:
		return keymap.Binding{}
	}
}
