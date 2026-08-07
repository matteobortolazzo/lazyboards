package config

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"gopkg.in/yaml.v3"
)

// Action defines a user-configured action bound to a key.
type Action struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	URL     string `yaml:"url"`
	Command string `yaml:"command"`
	Scope   string `yaml:"scope"`
	// Order is derived metadata reflecting the action's position in its
	// source YAML file (see assignActionOrder). It is never read from or
	// written to the YAML file itself, so it can't be hand-set by a user
	// and doesn't get scrambled by Save()'s random map-key re-marshal
	// order (pre-existing behavior, unaffected by this field).
	Order int `yaml:"-"`
}

// ColumnConfig defines a column with optional per-column actions.
type ColumnConfig struct {
	Name    string            `yaml:"name"`
	Actions map[string]Action `yaml:"actions"`
	Cleanup *string           `yaml:"cleanup,omitempty"`
}

// CleanupValue returns the column's own cleanup command, or "" if unset.
// By the time Load() returns, every column has an explicit (possibly empty)
// value; this accessor only guards direct construction (e.g. in tests) that
// bypasses Load()'s inheritance resolution.
func (cc ColumnConfig) CleanupValue() string {
	if cc.Cleanup == nil {
		return ""
	}
	return *cc.Cleanup
}

// DefaultSessionMaxLength must stay in sync with cenci's windowNameMaxLen
// (cenci/watch internal/run/slug.go).
const DefaultSessionMaxLength = 40
const DefaultRefreshInterval = 5
const DefaultWorkingLabel = "Working"

// Config holds the application configuration.
type Config struct {
	Provider         string            `yaml:"provider"`
	Repo             string            `yaml:"repo"`
	Project          string            `yaml:"project"`
	Actions          map[string]Action `yaml:"actions"`
	Columns          []ColumnConfig    `yaml:"columns"`
	SessionMaxLength int               `yaml:"session_max_length"`
	RefreshInterval  int               `yaml:"refresh_interval"`
	WorkingLabel     *string           `yaml:"working_label,omitempty"`
	Mouse            *bool             `yaml:"mouse,omitempty"`
	Cenci            *bool             `yaml:"cenci,omitempty"`
	Cleanup          *string           `yaml:"cleanup,omitempty"`
	UpdateCheck      *bool             `yaml:"update_check,omitempty"`
	SortOrder        *string           `yaml:"sort_order,omitempty"`
	Keymaps          *Keymaps          `yaml:"keymaps,omitempty"`
	// Deprecations holds human-readable notices surfaced to the user when
	// Load() translates a legacy config construct (e.g. a top-level
	// `actions:` or `columns[].actions:` block) onto the `keymaps:`
	// namespace (see legacy_actions.go, #510). Never read from or written
	// to the YAML file -- purely derived, like Action.Order.
	Deprecations []string `yaml:"-"`
	// LocalHash is the content hash of the local config file Load() read
	// (in "sha256:<hex>" form, see HashLocalConfig), or "" if no local file
	// was ever read. Never read from or written to the YAML file -- purely
	// derived, like Action.Order and Deprecations.
	LocalHash string `yaml:"-"`
	// Notices holds human-readable messages surfaced to the user when
	// Load() strips an untrusted local file's shell-executing constructs
	// (inline keymaps: shell bindings, legacy actions:/columns[].actions:
	// shell entries, or cleanup:/columns[].cleanup commands -- see
	// stripLocalShellSinks, trust_strip.go). Populated with at most one
	// entry per Load() call, naming every stripped sink kind together.
	// Never read from or written to the YAML file -- purely derived, like
	// Action.Order and Deprecations.
	Notices []string `yaml:"-"`
}

// Card sort directions accepted by the sort_order config field.
const (
	SortOrderOldest = "oldest"
	SortOrderNewest = "newest"
)

// DefaultSortOrder is the built-in card sort direction used when sort_order
// is not configured: oldest-created-first (#503).
const DefaultSortOrder = SortOrderOldest

// SortNewestFirstValue reports whether cards should sort newest-created-first.
// Defaults to false (oldest-first) when sort_order is not set. Load()
// validates the value, so anything reaching here is one of the two constants.
func (c Config) SortNewestFirstValue() bool {
	if c.SortOrder == nil {
		return DefaultSortOrder == SortOrderNewest
	}
	return *c.SortOrder == SortOrderNewest
}

// WorkingLabelValue returns the configured working label, or DefaultWorkingLabel if not set.
func (c Config) WorkingLabelValue() string {
	if c.WorkingLabel == nil {
		return DefaultWorkingLabel
	}
	return *c.WorkingLabel
}

// MouseValue returns true if mouse support is enabled.
// Defaults to true (mouse enabled) when the field is not set.
func (c Config) MouseValue() bool {
	if c.Mouse == nil {
		return true
	}
	return *c.Mouse
}

// CenciValue returns true if cenci integration is enabled.
// Defaults to true (enabled) when the field is not set.
func (c Config) CenciValue() bool {
	if c.Cenci == nil {
		return true
	}
	return *c.Cenci
}

// UpdateCheckValue returns true if the startup version-update check is enabled.
// Defaults to true (enabled) when the field is not set.
func (c Config) UpdateCheckValue() bool {
	if c.UpdateCheck == nil {
		return true
	}
	return *c.UpdateCheck
}

// CleanupValue returns the configured top-level default cleanup command, or ""
// if not set. Used as the fallback for any column that doesn't set its own.
func (c Config) CleanupValue() string {
	if c.Cleanup == nil {
		return ""
	}
	return *c.Cleanup
}

// DefaultScope returns "card" when s is empty, otherwise s unchanged. This is
// a defensive passthrough for actions whose scope has already been resolved
// by validateActions (which calls inferScope to pick "card" or "board" based
// on the action's template) — it is not itself the source of truth for the
// default-scope policy.
func DefaultScope(s string) string {
	if s == "" {
		return "card"
	}
	return s
}

// DefaultColumns is the default set of column names when none are configured.
var DefaultColumns = []ColumnConfig{
	{Name: "New"},
	{Name: "Refined"},
	{Name: "Implementing"},
}

// DefaultGitActions returns the built-in lazygit-style git actions. These are
// board-scope shell actions available inside a git repo with a remote. Their
// keys are scoped to the git menu (opened with `g` in normal mode) and never
// dispatch from normal mode, so the normal-mode uppercase A-Z namespace stays
// fully reserved for user-defined custom actions.
func DefaultGitActions() map[string]Action {
	return map[string]Action{
		"P": {Name: "Push", Type: "shell", Command: "git push", Scope: "board"},
		"p": {Name: "Pull (rebase)", Type: "shell", Command: "git pull --rebase", Scope: "board"},
		"f": {Name: "Fetch", Type: "shell", Command: "git fetch", Scope: "board"},
		"m": {Name: "Mergetool", Type: "shell", Command: "git mergetool", Scope: "board"},
		"s": {Name: "Stash push", Type: "shell", Command: "git stash push", Scope: "board"},
		"S": {Name: "Stash pop", Type: "shell", Command: "git stash pop", Scope: "board"},
	}
}

// ColumnNames extracts the column name strings from the ColumnConfig slice.
func (c Config) ColumnNames() []string {
	names := make([]string, len(c.Columns))
	for i, col := range c.Columns {
		names[i] = col.Name
	}
	return names
}

const DefaultLocalPath = ".lazyboards.yml"

// DefaultGlobalPath returns the default global config file path.
func DefaultGlobalPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lazyboards", "config.yml"), nil
}

// DefaultCrashLogPath returns the default file for panic reports, alongside
// the global config at ~/.config/lazyboards/crash.log. The parent directory
// is created on demand at crash time, so it need not exist yet.
func DefaultCrashLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lazyboards", "crash.log"), nil
}

// Load reads configuration from globalPath and localPath YAML files.
// Local config merges on top of global. Returns defaults if no files exist.
// trust gates whether the local file's keystroke-triggered shell-executing
// constructs (inline keymaps: shell bindings, legacy actions:/
// columns[].actions: shell entries, and cleanup:/columns[].cleanup commands)
// are honored: when the local file's content hash isn't in trust, they are
// silently stripped before the merge (see stripLocalShellSinks,
// trust_strip.go) -- global-declared shell constructs are never affected,
// whatever trust says (AC9). Stripping is a value comparison against the
// pre-local global snapshots (globalKeymaps/globalActions/globalColumns/
// globalCleanup, taken below before the local unmarshal runs), ignoring each
// entry's derived Order field (see sameShellAction/sameShellBinding,
// trust_strip.go): a local shell construct equal to its global counterpart
// in every execution-relevant field is inherited, not stripped, whatever
// document position or YAML alias it was declared through. Each stripped
// sink is not silently dropped: it is recorded in the returned Config's
// Notices field (see buildStripNotice), naming every stripped kind together
// in a single entry per Load() call.
func Load(globalPath, localPath string, trust Trust) (Config, error) {
	var cfg Config

	// Read global config file.
	globalData, err := os.ReadFile(globalPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	var globalDecls localDecls
	if err == nil {
		if err := yaml.Unmarshal(globalData, &cfg); err != nil {
			return Config{}, err
		}
		d, err := assignActionOrder(globalData, &cfg)
		if err != nil {
			return Config{}, err
		}
		globalDecls = d
	}

	// Identity fields (provider, repo, project) only come from local config,
	// not from global config. Clear them after global load.
	cfg.Provider = ""
	cfg.Repo = ""
	cfg.Project = ""

	// Save global actions and columns before local override. Columns is a
	// genuine frozen snapshot here: yaml.v3 fully replaces a slice field on a
	// second Unmarshal (never merges), so globalColumns keeps referring to
	// the original global-only slice untouched by the local load below.
	// Actions is NOT a frozen snapshot the same way: yaml.v3 reuses an
	// existing non-nil map field and merges new/overridden keys into it in
	// place, so cfg.Actions itself gains the local document's keys once the
	// local unmarshal runs below -- that's why the key-existence-based Order
	// offset further down can't rely on map identity/length here the way the
	// column-level merge (mergeColumnActions) can; it instead tracks which
	// keys the local document itself declared (decls.ActionKeys, from
	// assignActionOrder's return value) to know which entries are genuinely
	// global-only. globalActions itself DOES need to be a real copy
	// (maps.Clone, not a plain alias): an untrusted local shell action gets
	// deleted from cfg.Actions entirely (see stripShellFromActions,
	// trust_strip.go) so the merge loop below can fall back to the matching
	// global entry -- an alias would have nothing left to fall back to, since
	// deleting from cfg.Actions would delete from the "global" snapshot too.
	globalActions := maps.Clone(cfg.Actions)
	globalColumns := cfg.Columns

	// globalCleanup MUST be a value copy, never a pointer alias
	// (globalCleanup := cfg.Cleanup would NOT do this): empirically, yaml.v3's
	// second Unmarshal into the same struct reuses the existing *string
	// pointer for a Cleanup field and overwrites its pointee in place when
	// the local document also declares a cleanup: key. An alias snapshot
	// would therefore already hold the local value by the time
	// stripLocalCleanup (trust_strip.go) compares against it, silently
	// letting an untrusted local override "restore" itself as if it were
	// global -- the same pointer-reuse trap globalKeymaps/cfg.Keymaps = nil
	// works around below, and the map analog of maps.Clone(cfg.Actions)
	// above.
	var globalCleanup *string
	if cfg.Cleanup != nil {
		v := *cfg.Cleanup
		globalCleanup = &v
	}

	// Snapshot the global Keymaps by value and reset cfg.Keymaps to nil
	// before the local unmarshal below. Keymaps has a custom UnmarshalYAML
	// on a pointer field: if cfg.Keymaps were left non-nil here, a second
	// yaml.Unmarshal into &cfg would reuse that same *Keymaps pointer
	// (rather than allocating a fresh one) whenever the local document also
	// declares a keymaps: block, aliasing global's parsed value. Resetting
	// to nil forces a fresh allocation, and mergeKeymaps below explicitly
	// recombines the two snapshots the same way mergeColumnActions does for
	// per-column actions.
	globalKeymaps := cfg.Keymaps
	cfg.Keymaps = nil

	// Read local config file, unmarshal into the same struct.
	localData, err := os.ReadFile(localPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	var decls localDecls
	if err == nil {
		if err := yaml.Unmarshal(localData, &cfg); err != nil {
			return Config{}, err
		}
		d, err := assignActionOrder(localData, &cfg)
		if err != nil {
			return Config{}, err
		}
		decls = d

		// LocalHash/trust gate whether this local document's own
		// keystroke-triggered shell-executing constructs are honored.
		// cfg.Keymaps is purely local at this point (global was already
		// snapshotted into globalKeymaps above and reset to nil), and
		// cfg.Columns/cfg.Actions still hold only what the local unmarshal
		// just produced (the global-preserving merge steps below haven't run
		// yet) -- exactly the provenance window stripLocalShellSinks needs to
		// strip only local-declared shell entries and never a global one
		// (AC9). stripLocalShellSinks compares against the globalKeymaps/
		// globalActions/globalColumns snapshots by value (ignoring each
		// entry's derived Order field -- see sameShellAction/sameShellBinding,
		// trust_strip.go) rather than consulting decls -- see
		// stripShellFromActions (trust_strip.go) for why the raw-node walk
		// decls carries isn't a safe strip-eligibility gate.
		cfg.LocalHash = hashConfigBytes(localData)
		if !trust.Trusts(cfg.LocalHash) {
			counts := stripLocalShellSinks(&cfg, decls, globalKeymaps, globalActions, globalColumns, globalCleanup)
			if notice := buildStripNotice(counts); notice != "" {
				cfg.Notices = append(cfg.Notices, notice)
			}
		}
	}

	// Merge actions: preserve global-only entries as defaults, local entries take priority.
	if len(globalActions) > 0 {
		if cfg.Actions == nil {
			cfg.Actions = make(map[string]Action)
		}
		for k, v := range globalActions {
			if _, exists := cfg.Actions[k]; !exists {
				cfg.Actions[k] = v
			}
		}
	}

	// Push every key the local document didn't declare itself (i.e.
	// inherited unchanged from global, or fallen back to global after an
	// untrusted local shell entry was stripped -- stripShellFromActions
	// removes a stripped key from decls.ActionKeys for exactly this reason)
	// after all locally-declared keys, preserving each group's relative
	// order.
	if localCount := len(decls.ActionKeys); localCount > 0 {
		for k, v := range cfg.Actions {
			if !decls.ActionKeys[k] {
				v.Order += localCount
				cfg.Actions[k] = v
			}
		}
	}

	// Columns: local replaces global entirely. If local had no columns, keep global.
	if cfg.Columns == nil {
		cfg.Columns = globalColumns
	}

	// Merge per-column actions: for each local column, merge with matching global column's actions.
	mergeColumnActions(cfg.Columns, globalColumns)

	// Merge per-column cleanup: for each local column, inherit the matching
	// global column's cleanup if the local column didn't set its own.
	mergeColumnCleanup(cfg.Columns, globalColumns)

	// Merge keymaps: local per-mode/per-column tables win per key, with
	// global-only keys preserved (see mergeKeymaps).
	cfg.Keymaps = mergeKeymaps(cfg.Keymaps, globalKeymaps)

	if err := validateSortOrder(cfg.SortOrder); err != nil {
		return Config{}, err
	}

	if err := validateColumns(&cfg); err != nil {
		return Config{}, err
	}

	if err := validateActions(cfg.Actions); err != nil {
		return Config{}, err
	}

	translateLegacyActions(&cfg, globalDecls.LegacyBlock || decls.LegacyBlock)

	// validateKeymapActions must run before validateScopeConflicts: it
	// infers and writes back the default scope for natively-declared
	// keymaps: inline actions, the same way validateActions already did for
	// cfg.Actions above -- validateScopeConflicts must see every action's
	// concrete scope, not an unresolved "".
	if err := validateKeymapActions(cfg.Keymaps); err != nil {
		return Config{}, err
	}

	if err := validateCommandIDs(cfg.Keymaps); err != nil {
		return Config{}, err
	}

	if err := validateModeCapabilities(cfg.Keymaps); err != nil {
		return Config{}, err
	}

	if err := validatePrintableRuneBindings(cfg.Keymaps); err != nil {
		return Config{}, err
	}

	if err := validateScopeConflicts(&cfg); err != nil {
		return Config{}, err
	}

	if err := validateKeymap(&cfg); err != nil {
		return Config{}, err
	}

	// Any column still without an explicit cleanup (including defaulted
	// columns) falls back to the resolved top-level default.
	applyDefaultCleanup(cfg.Columns, cfg.CleanupValue())

	if cfg.SessionMaxLength <= 0 {
		cfg.SessionMaxLength = DefaultSessionMaxLength
	}

	if cfg.RefreshInterval == 0 {
		cfg.RefreshInterval = DefaultRefreshInterval
	} else if cfg.RefreshInterval < 0 {
		cfg.RefreshInterval = 0 // 0 means disabled internally
	}

	return cfg, nil
}

// localDecls records which top-level action keys a document's own raw YAML
// declared, distinguishing genuinely-declared-by-this-document entries from
// ones merely inherited from another document (global vs local) once the
// two are merged together. assignActionOrder populates it as a side effect
// of its own document-position walk; Load()'s Order-offset logic is its
// only consumer (a purely cosmetic rendering concern, never a security
// control) -- stripLocalShellSinks/stripShellFromActions (trust_strip.go)
// do NOT consume it: a raw-node "was this key mentioned in the document"
// walk can't see a YAML merge-key-smuggled entry, so it isn't a safe
// strip-eligibility gate (see stripShellFromActions for the full
// explanation). This struct used to also carry a HasColumns bool for the
// analogous columns: gate; that field was removed for the same reason once
// stripShellFromActions stopped consuming it.
type localDecls struct {
	// ActionKeys is the set of top-level action keys this document's own
	// actions: mapping declares (nil if the document has none, or declares
	// no actions: block at all).
	ActionKeys map[string]bool
	// LegacyBlock reports whether this document's own raw YAML declares a
	// legacy actions: or columns[].actions: block at all -- a real mapping
	// node (including an explicit, empty `actions: {}`), never a `!!null`
	// scalar (`actions:`/`actions: ~`). This is a cosmetic notice signal
	// only (see legacyDeprecationNotice, legacy_actions.go): it feeds
	// translateLegacyActions' deprecation-notice presence check and must
	// never be consulted as a strip/security gate the way ActionKeys
	// explicitly is not (see stripShellFromActions, trust_strip.go).
	LegacyBlock bool
}

// assignActionOrder parses data a second time as a yaml.Node tree and stamps
// each entry in cfg.Actions (and each column's own actions, and every
// keymaps mode/column table) with its 1-based position in the raw document,
// so callers can render entries in the order the user wrote them instead of
// Go's randomized map order. cfg.Actions, cfg.Columns, and cfg.Keymaps must
// already hold the values unmarshaled from this same data, since map values
// holding structs aren't addressable and require a read-modify-write.
// Columns are matched by index, not name: within one raw document, columns
// is already in document order courtesy of normal yaml.v3 unmarshaling, so
// columnsNode.Content[i] lines up with cfg.Columns[i] positionally.
// Name-based matching across documents (global vs local) is a separate,
// later concern handled by mergeColumnActions/columnsByNameLower (and, for
// keymaps, mergeKeymaps).
//
// It returns a localDecls describing which top-level action keys this
// document's own raw YAML declares, which Load()'s Order-offset logic uses
// to tell genuinely local declarations apart from values merely inherited
// unchanged from another document (see the comment on globalActions in
// Load()) -- a purely cosmetic rendering concern; it is deliberately never
// consumed by stripLocalShellSinks/stripShellFromActions (trust_strip.go)
// as a strip-eligibility gate (see that function's comment for why).
func assignActionOrder(data []byte, cfg *Config) (localDecls, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return localDecls{}, err
	}
	if len(root.Content) == 0 {
		return localDecls{}, nil
	}
	docNode := root.Content[0]
	if docNode.Kind != yaml.MappingNode {
		return localDecls{}, nil
	}

	var actionsNode, columnsNode, keymapsNode *yaml.Node
	for i := 0; i+1 < len(docNode.Content); i += 2 {
		key := docNode.Content[i]
		value := docNode.Content[i+1]
		switch key.Value {
		case "actions":
			actionsNode = value
		case "columns":
			columnsNode = value
		case "keymaps":
			keymapsNode = value
		}
	}

	var decls localDecls

	if actionsNode != nil {
		decls.ActionKeys = stampActionOrder(actionsNode, cfg.Actions)
	}

	// Legacy-block presence (decls.LegacyBlock) is a separate concern from
	// the Order-stamping this loop otherwise performs: it's about raw-syntax
	// presence (did this document write a real actions: mapping node at all,
	// including an explicit empty one), not about stamping Action.Order onto
	// already-decoded map entries. Both concerns walk the same columns tree,
	// so they share this one pass rather than each re-walking it separately.
	decls.LegacyBlock = isLegacyActionsBlock(actionsNode)
	if columnsNode != nil && columnsNode.Kind == yaml.SequenceNode {
		for i, colNode := range columnsNode.Content {
			if i >= len(cfg.Columns) || colNode.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(colNode.Content); j += 2 {
				key := colNode.Content[j]
				value := colNode.Content[j+1]
				if key.Value == "actions" {
					stampActionOrder(value, cfg.Columns[i].Actions)
					if isLegacyActionsBlock(value) {
						decls.LegacyBlock = true
					}
				}
			}
		}
	}

	if keymapsNode != nil && cfg.Keymaps != nil {
		stampKeymapsOrder(keymapsNode, cfg.Keymaps)
	}

	return decls, nil
}

// isLegacyActionsBlock reports whether node is a real, declared actions:
// mapping node -- true for an explicit empty block (`actions: {}`) as well
// as a populated one, but false for a `!!null` scalar node (`actions:` or
// `actions: ~`, which is never a declaration) or a nil node (key absent
// entirely).
func isLegacyActionsBlock(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.MappingNode
}

// nodeKeysInOrder returns node's top-level mapping keys in document order,
// or nil if node is not a mapping. Shared by stampActionOrder and the
// keymaps order stamper below so both walk mapping nodes the same way.
func nodeKeysInOrder(node *yaml.Node) []string {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	keys := make([]string, 0, len(node.Content)/2)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keys = append(keys, node.Content[i].Value)
	}
	return keys
}

// stampActionOrder walks a YAML mapping node of action keys, assigns each
// matching entry in actions its 1-based document position, and returns the
// set of keys found in the node.
func stampActionOrder(node *yaml.Node, actions map[string]Action) map[string]bool {
	keys := nodeKeysInOrder(node)
	if keys == nil {
		return nil
	}
	declared := make(map[string]bool, len(keys))
	for i, key := range keys {
		declared[key] = true
		if a, ok := actions[key]; ok {
			a.Order = i + 1
			actions[key] = a
		}
	}
	return declared
}

// stampKeymapOrder walks a YAML mapping node of keymap keys and assigns
// each matching entry in table its 1-based document position.
func stampKeymapOrder(node *yaml.Node, table KeymapTable) {
	for i, key := range nodeKeysInOrder(node) {
		if b, ok := table[key]; ok {
			b.Order = i + 1
			table[key] = b
		}
	}
}

// stampKeymapsOrder walks the raw keymaps: mapping node and stamps document
// position into every matching entry of keymaps.Modes and keymaps.Columns
// (keyed the same way Keymaps.UnmarshalYAML parsed them from this same
// node).
func stampKeymapsOrder(node *yaml.Node, keymaps *Keymaps) {
	if node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		modeName := node.Content[i].Value
		modeValue := node.Content[i+1]

		if modeName == string(keymap.ModeColumns) {
			if modeValue.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(modeValue.Content); j += 2 {
				colName := modeValue.Content[j].Value
				if table, ok := keymaps.Columns[colName]; ok {
					stampKeymapOrder(modeValue.Content[j+1], table)
				}
			}
			continue
		}

		mode, err := keymap.ParseMode(modeName)
		if err != nil {
			continue // already surfaced as a load error by Keymaps.UnmarshalYAML
		}
		if table, ok := keymaps.Modes[mode]; ok {
			stampKeymapOrder(modeValue, table)
		}
	}
}

// LocalExists returns true if the file at path exists.
func LocalExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Save writes provider and repo to the config file at path.
// If the file already exists, it preserves existing fields (like actions).
//
// trustPath, if non-empty, carries trust forward across the rewrite (#568):
// when the pre-write file content was already trusted, the post-write
// content is trusted too, so editing a config exclusively through the app's
// own Save() path never silently drops back to untrusted. An untrusted (or
// missing/malformed) pre-write file never gains trust as a side effect of
// saving. Any error in that carry-forward step is swallowed -- the config
// write itself has already succeeded and must not be failed by it.
func Save(path, provider, repo, trustPath string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yml" && ext != ".yaml" {
		return fmt.Errorf("config path %q must have .yml or .yaml extension", path)
	}

	// Read existing config if file exists. A missing file is fine (start
	// fresh); a structurally invalid existing file is not -- surfacing that
	// as an error avoids silently rewriting a truncated config (e.g. one
	// whose keymaps: block failed to parse).
	var cfg Config
	preData, err := os.ReadFile(path)
	if err == nil {
		if err := yaml.Unmarshal(preData, &cfg); err != nil {
			return fmt.Errorf("config %q: %w", path, err)
		}
		if _, err := assignActionOrder(preData, &cfg); err != nil {
			return fmt.Errorf("config %q: %w", path, err)
		}
	}
	hadPreData := err == nil

	// Update provider and repo.
	cfg.Provider = provider
	cfg.Repo = repo

	// Marshal and write.
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		return err
	}

	if hadPreData && trustPath != "" {
		preHash := hashConfigBytes(preData)
		postHash := hashConfigBytes(out)
		_ = carryTrustForward(trustPath, preHash, postHash)
	}

	return nil
}

// columnsByNameLower builds a lookup map of columns keyed by their
// lowercased name, so callers can match columns case-insensitively by name
// (never by positional index).
func columnsByNameLower(columns []ColumnConfig) map[string]ColumnConfig {
	byName := make(map[string]ColumnConfig, len(columns))
	for _, c := range columns {
		byName[strings.ToLower(c.Name)] = c
	}
	return byName
}

// mergeColumnActions merges per-column actions from globalColumns into columns.
// For each column, if a matching global column exists (case-insensitive name),
// global-only action keys are preserved. Local action keys take priority.
// If a local column has nil actions, it inherits all matching global column actions.
func mergeColumnActions(columns []ColumnConfig, globalColumns []ColumnConfig) {
	globalByName := columnsByNameLower(globalColumns)

	for i := range columns {
		gc, found := globalByName[strings.ToLower(columns[i].Name)]
		if !found || len(gc.Actions) == 0 {
			continue
		}
		if columns[i].Actions == nil {
			// Nil means actions were not specified; inherit all global actions.
			columns[i].Actions = make(map[string]Action, len(gc.Actions))
			for k, v := range gc.Actions {
				columns[i].Actions[k] = v
			}
			continue
		}
		if len(columns[i].Actions) == 0 {
			// Explicit empty map means "no actions"; skip merge.
			continue
		}
		// Non-empty local actions: fill in global-only keys (local wins on conflicts).
		localCount := len(columns[i].Actions)
		for k, v := range gc.Actions {
			if _, exists := columns[i].Actions[k]; !exists {
				// Push global-only fill-ins after all local entries, preserving
				// each group's relative order.
				v.Order += localCount
				columns[i].Actions[k] = v
			}
		}
	}
}

// mergeColumnCleanup fills in a column's Cleanup from the matching global
// column when the local column didn't specify one. An explicit local value
// (including an explicit empty string, which disables cleanup) always wins.
func mergeColumnCleanup(columns []ColumnConfig, globalColumns []ColumnConfig) {
	globalByName := columnsByNameLower(globalColumns)

	for i := range columns {
		if columns[i].Cleanup != nil {
			continue
		}
		if gc, found := globalByName[strings.ToLower(columns[i].Name)]; found {
			columns[i].Cleanup = gc.Cleanup
		}
	}
}

// applyDefaultCleanup fills in the resolved top-level cleanup default for any
// column that still has no explicit value after per-column merge.
func applyDefaultCleanup(columns []ColumnConfig, defaultCleanup string) {
	for i := range columns {
		if columns[i].Cleanup == nil {
			columns[i].Cleanup = &defaultCleanup
		}
	}
}

// validateSortOrder rejects any sort_order value other than the two accepted
// directions. A present-but-unrecognized value (including an empty one) is a
// typo rather than a request for the default, so it fails loudly instead of
// silently falling back.
func validateSortOrder(order *string) error {
	if order == nil {
		return nil
	}
	if *order != SortOrderOldest && *order != SortOrderNewest {
		return fmt.Errorf("sort_order must be %q or %q, got %q", SortOrderOldest, SortOrderNewest, *order)
	}
	return nil
}

// validateColumns checks that columns are valid and applies defaults if empty.
func validateColumns(cfg *Config) error {
	if len(cfg.Columns) == 0 {
		cfg.Columns = make([]ColumnConfig, len(DefaultColumns))
		copy(cfg.Columns, DefaultColumns)
		return nil
	}

	// Validate column names and check for case-insensitive duplicates.
	seen := make(map[string]bool, len(cfg.Columns))
	for i, col := range cfg.Columns {
		trimmed := strings.TrimSpace(col.Name)
		if trimmed == "" {
			return fmt.Errorf("column %d: name cannot be empty or whitespace-only", i+1)
		}
		cfg.Columns[i].Name = trimmed
		lower := strings.ToLower(trimmed)
		if seen[lower] {
			return fmt.Errorf("duplicate column %q (case-insensitive)", trimmed)
		}
		seen[lower] = true

		// Validate per-column actions with the same rules as global actions.
		if err := validateActions(col.Actions); err != nil {
			return fmt.Errorf("column %q: %w", trimmed, err)
		}
	}
	return nil
}

// cardSpecificVarPattern matches card-specific template variables.
var cardSpecificVarPattern = regexp.MustCompile(`\{(number|title|tags|session|window)\}`)

// prSpecificVarPattern matches PR-specific template variables (scope: pr only).
var prSpecificVarPattern = regexp.MustCompile(`\{(pr_branch|pr_number|pr_url|pr_title|pr_worktree)\}`)

// inferScope returns the default scope for an action whose scope was omitted
// from config: "board" when the template references no ticket-specific
// placeholder (card-specific or pr-specific), otherwise "card" (today's
// default). It never overrides an explicit scope -- callers only invoke it
// when scope is empty.
func inferScope(template string) string {
	if cardSpecificVarPattern.MatchString(template) || prSpecificVarPattern.MatchString(template) {
		return "card"
	}
	return "board"
}

// validateActions checks that all action definitions are well-formed.
func validateActions(actions map[string]Action) error {
	for key, action := range actions {
		// Key is a sequence of one or more keys pressed one after another
		// (neovim-style prefix bindings), rune-concatenated with no
		// separator (see legacySequence). #510 removed the old "first key
		// must be an uppercase letter A-Z" requirement -- the reserved
		// custom-action namespace is gone now that legacy actions:/
		// columns[].actions: translate onto the unified keymaps: namespace
		// and get validated there (validateKeymap) alongside built-ins, so
		// any first key is permitted. Continuation keys must still be a
		// letter or digit, since a pending sequence consumes every key
		// until it resolves (handlePendingSeqKey only ever appends
		// msg.Runes) and the legacy key -> canonical sequence translation
		// depends on it.
		runes := []rune(key)
		if len(runes) == 0 {
			return fmt.Errorf("action key %q must not be empty", key)
		}
		for _, r := range runes[1:] {
			if !IsSequenceKey(r) {
				return fmt.Errorf("action key %q: sequence keys after the first must be letters or digits", key)
			}
		}
		if err := validateActionValue(key, &action); err != nil {
			return err
		}
		actions[key] = action
	}
	return nil
}

// validateActionValue validates that one action definition is well-formed
// (name, type, url/command per type, and scope, including the board/card
// ticket-variable restrictions), inferring and writing back the default
// scope in place when it was omitted. Shared by validateActions (top-level
// actions:/columns[].actions:, #340) and validateKeymapActions (inline
// keymaps: action definitions, #510) so both surfaces enforce identical
// rules -- key only identifies the offending entry in any returned error,
// it plays no part in the validation itself.
func validateActionValue(key string, action *Action) error {
	// Name is required.
	if strings.TrimSpace(action.Name) == "" {
		return fmt.Errorf("action %q: name is required", key)
	}
	// Type must be "url" or "shell".
	if action.Type != "url" && action.Type != "shell" {
		return fmt.Errorf("action %q: type must be \"url\" or \"shell\", got %q", key, action.Type)
	}
	// URL required for url type.
	if action.Type == "url" && strings.TrimSpace(action.URL) == "" {
		return fmt.Errorf("action %q: url is required when type is \"url\"", key)
	}
	// Command required for shell type.
	if action.Type == "shell" && strings.TrimSpace(action.Command) == "" {
		return fmt.Errorf("action %q: command is required when type is \"shell\"", key)
	}
	// Default empty scope: infer "board" when the template has no
	// ticket-specific placeholders, otherwise "card" (today's default).
	template := action.URL + action.Command
	if action.Scope == "" {
		action.Scope = inferScope(template)
	}
	// Validate scope value.
	if action.Scope != "card" && action.Scope != "board" && action.Scope != "pr" {
		return fmt.Errorf("action %q: scope must be \"card\", \"board\", or \"pr\", got %q", key, action.Scope)
	}
	// Board-scope actions must not reference card-specific variables.
	if action.Scope == "board" {
		if cardSpecificVarPattern.MatchString(template) {
			return fmt.Errorf("action %q: scope \"board\" cannot use card-specific variables ({number}, {title}, {tags}, {session}, {window})", key)
		}
		if prSpecificVarPattern.MatchString(template) {
			return fmt.Errorf("action %q: scope \"board\" cannot use pr-specific variables ({pr_branch}, {pr_number}, {pr_url}, {pr_title}, {pr_worktree})", key)
		}
	}
	// Card-scope actions must not reference pr-specific variables.
	if action.Scope == "card" {
		if prSpecificVarPattern.MatchString(template) {
			return fmt.Errorf("action %q: scope \"card\" cannot use pr-specific variables ({pr_branch}, {pr_number}, {pr_url}, {pr_title}, {pr_worktree})", key)
		}
	}
	return nil
}

// IsSequenceKey reports whether r is a valid continuation key of an action
// key sequence: any letter or digit.
func IsSequenceKey(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// validateScopeConflicts checks that no key sequence is assigned a "card"
// scope by one inline action and a "pr" scope by another, across
// keymaps.normal, keymaps.detail, and every keymaps.columns.<name> table.
// #510 rewrite: scans the unified keymaps: namespace instead of the legacy
// cfg.Actions/cfg.Columns[].Actions maps directly. By the time this runs,
// translateLegacyActions has already mirrored every legacy actions:/
// columns[].actions: entry into cfg.Keymaps (see Load()), so this one scan
// covers legacy-derived and natively-declared keymaps: actions together --
// the "one validation path" the ticket asks for. Per the ticket's Q1
// decision, only card<->pr conflicts are rejected; a key shared between
// "board" and either "card" or "pr" across tables is unchanged, existing
// behavior.
//
// The grouping key is the sequence's canonical form (keymap.ParseSequence,
// then Sequence.String() -- the same normalization normalizeTable,
// internal/keymap/keymap.go, already uses), not its raw YAML spelling, so
// whitespace variants of the same physical key ("Z f", "Z  f", " Z f ")
// collapse onto one bucket instead of hiding a real conflict. The scan still
// reads the raw per-table maps (cfg.Keymaps.Modes/Columns), not the resolved
// *keymap.Keymap, since resolution would collapse column overlays onto mode
// entries and erase the cross-table pairs this check looks for; only the map
// KEY is canonicalized, cfg.Keymaps itself is never mutated.
func validateScopeConflicts(cfg *Config) error {
	if cfg.Keymaps == nil {
		return nil
	}

	scopesBySequence := make(map[string]map[string]bool)

	addScopes := func(table KeymapTable, label string) error {
		for rawKey, binding := range table {
			if binding.Kind != keymap.BindingAction {
				continue
			}
			parsed, err := keymap.ParseSequence(rawKey)
			if err != nil {
				return fmt.Errorf("%s: key %q: %w", label, rawKey, err)
			}
			seq := parsed.String()
			scope := DefaultScope(binding.Action.Scope)
			if scopesBySequence[seq] == nil {
				scopesBySequence[seq] = make(map[string]bool)
			}
			scopesBySequence[seq][scope] = true
		}
		return nil
	}

	for mode, table := range cfg.Keymaps.Modes {
		if err := addScopes(table, fmt.Sprintf("keymaps.%s", mode)); err != nil {
			return err
		}
	}
	for column, table := range cfg.Keymaps.Columns {
		if err := addScopes(table, fmt.Sprintf("keymaps.columns.%s", column)); err != nil {
			return err
		}
	}

	for seq, scopes := range scopesBySequence {
		if scopes["card"] && scopes["pr"] {
			return fmt.Errorf("keymap key %q: cannot be both \"card\" scope and \"pr\" scope across mode/column action tables", seq)
		}
	}
	return nil
}
