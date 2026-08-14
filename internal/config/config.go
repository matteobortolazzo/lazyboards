package config

import (
	"errors"
	"fmt"
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
	// source YAML file (see stampOrderAndRejectLegacy). It is never read
	// from or written to the YAML file itself, so it can't be hand-set by a
	// user and doesn't get scrambled by Save()'s random map-key re-marshal
	// order (pre-existing behavior, unaffected by this field).
	Order int `yaml:"-"`
}

// ColumnConfig defines a column and its optional cleanup command.
type ColumnConfig struct {
	Name    string  `yaml:"name"`
	Cleanup *string `yaml:"cleanup,omitempty"`
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
	Provider         string         `yaml:"provider"`
	Repo             string         `yaml:"repo"`
	Project          string         `yaml:"project"`
	Columns          []ColumnConfig `yaml:"columns"`
	SessionMaxLength int            `yaml:"session_max_length"`
	RefreshInterval  int            `yaml:"refresh_interval"`
	WorkingLabel     *string        `yaml:"working_label,omitempty"`
	Mouse            *bool          `yaml:"mouse,omitempty"`
	Cenci            *bool          `yaml:"cenci,omitempty"`
	Cleanup          *string        `yaml:"cleanup,omitempty"`
	UpdateCheck      *bool          `yaml:"update_check,omitempty"`
	SortOrder        *string        `yaml:"sort_order,omitempty"`
	Keymaps          *Keymaps       `yaml:"keymaps,omitempty"`
	// LocalHash is the content hash of the local config file Load() read
	// (in "sha256:<hex>" form, see HashLocalConfig), or "" if no local file
	// was ever read. Never read from or written to the YAML file -- purely
	// derived, like Action.Order.
	LocalHash string `yaml:"-"`
	// Notices holds human-readable messages surfaced to the user when
	// Load() strips an untrusted local file's shell-executing constructs
	// (inline keymaps: shell bindings, or cleanup:/columns[].cleanup
	// commands -- see stripLocalShellSinks, trust_strip.go). Populated with
	// at most one entry per Load() call, naming every stripped sink kind
	// together. Never read from or written to the YAML file -- purely
	// derived, like Action.Order.
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
// by validateKeymapActions (which calls inferScope to pick "card" or "board"
// based on the action's template) — it is not itself the source of truth for the
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
// keys are scoped to the git menu (opened with `G` in normal mode, #502) and
// never dispatch from normal mode -- the menu is its own mode
// (keymap.ModeGitPanel), independent of normal mode's own table, so a
// normal-mode binding on the same key (built-in or user-defined) coexists
// without conflict.
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
// constructs (inline keymaps: shell bindings and cleanup:/columns[].cleanup
// commands)
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
	if err == nil {
		if err := yaml.Unmarshal(globalData, &cfg); err != nil {
			return Config{}, err
		}
		if err := stampOrderAndRejectLegacy(globalData, &cfg, globalPath); err != nil {
			return Config{}, err
		}
	}

	// Identity fields (provider, repo, project) only come from local config,
	// not from global config. Clear them after global load.
	cfg.Provider = ""
	cfg.Repo = ""
	cfg.Project = ""

	// Save the global columns before the local override. This is a genuine
	// frozen snapshot: yaml.v3 fully replaces a slice field on a second
	// Unmarshal (never merges), so globalColumns keeps referring to the
	// original global-only slice untouched by the local load below.
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
	// recombines the two snapshots explicitly.
	globalKeymaps := cfg.Keymaps
	cfg.Keymaps = nil

	// Read local config file, unmarshal into the same struct.
	localData, err := os.ReadFile(localPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	if err == nil {
		if err := yaml.Unmarshal(localData, &cfg); err != nil {
			return Config{}, err
		}
		if err := stampOrderAndRejectLegacy(localData, &cfg, localPath); err != nil {
			return Config{}, err
		}

		// LocalHash/trust gate whether this local document's own
		// keystroke-triggered shell-executing constructs are honored.
		// cfg.Keymaps is purely local at this point (global was already
		// snapshotted into globalKeymaps above and reset to nil), and
		// cfg.Columns still holds only what the local unmarshal just
		// produced (the global-preserving merge steps below haven't run yet)
		// -- exactly the provenance window stripLocalShellSinks needs to
		// strip only local-declared shell constructs and never a global one
		// (AC9). It compares against the globalKeymaps/globalColumns/
		// globalCleanup snapshots by value (ignoring each entry's derived
		// Order field -- see sameShellAction/sameShellBinding,
		// trust_strip.go) rather than by re-walking the local document's raw
		// YAML nodes, which a YAML merge key can bypass entirely.
		cfg.LocalHash = hashConfigBytes(localData)
		if !trust.Trusts(cfg.LocalHash) {
			counts := stripLocalShellSinks(&cfg, globalKeymaps, globalColumns, globalCleanup)
			if notice := buildStripNotice(counts); notice != "" {
				cfg.Notices = append(cfg.Notices, notice)
			}
		}
	}

	// Columns: local replaces global entirely. If local had no columns, keep global.
	if cfg.Columns == nil {
		cfg.Columns = globalColumns
	}

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

	// validateKeymapActions must run before validateScopeConflicts: it
	// infers and writes back the default scope for every keymaps: inline
	// action -- validateScopeConflicts must see every action's concrete
	// scope, not an unresolved "".
	if err := validateKeymapActions(cfg.Keymaps); err != nil {
		return Config{}, err
	}

	if err := validateCommandIDs(cfg.Keymaps); err != nil {
		return Config{}, err
	}

	if err := validateModeCapabilities(cfg.Keymaps); err != nil {
		return Config{}, err
	}

	if err := validateSequenceCapability(cfg.Keymaps); err != nil {
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

// legacyActionsRemovedHint is appended to every legacy-block rejection: the
// blocks are gone for good, so the error's job is to point at the one
// supported syntax rather than at a compatibility flag.
const legacyActionsRemovedHint = "is no longer supported; migrate to the keymaps: namespace (see README)"

// stampOrderAndRejectLegacy parses data a second time as a yaml.Node tree
// and stamps every keymaps mode/column table entry with its 1-based position
// in the raw document, so callers can render entries in the order the user
// wrote them instead of Go's randomized map order. cfg.Keymaps must already
// hold the values unmarshaled from this same data, since map values holding
// structs aren't addressable and require a read-modify-write.
//
// The same walk rejects the pre-0.73 legacy action blocks: a top-level
// `actions:` key, or an `actions:` key on any columns[] entry, fails the
// load with an error naming path (the document's own file) and the offending
// key. Rejection is presence-based on the raw key, whatever its value: an
// empty mapping (`actions: {}`) and an explicit null (`actions:` /
// `actions: ~`) are stale blocks a user should delete, not a request for
// "no actions".
//
// Known gap: a legacy block reachable only through a YAML merge key or alias
// (`base: &b {actions: ...}` plus `<<: *b`) never appears as a literal
// `actions:` key in the node tree, so this walk cannot see it. With the
// decoded field gone there is no second signal either, so such a block is
// silently ignored rather than rejected -- strictly better than the old
// behavior, where it silently worked (see docs/yaml-parsing.md).
func stampOrderAndRejectLegacy(data []byte, cfg *Config, path string) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if len(root.Content) == 0 {
		return nil
	}
	docNode := root.Content[0]
	if docNode.Kind != yaml.MappingNode {
		return nil
	}

	var columnsNode, keymapsNode *yaml.Node
	for i := 0; i+1 < len(docNode.Content); i += 2 {
		key := docNode.Content[i]
		value := docNode.Content[i+1]
		switch key.Value {
		case "actions":
			return fmt.Errorf("%s: actions: %s", path, legacyActionsRemovedHint)
		case "columns":
			columnsNode = value
		case "keymaps":
			keymapsNode = value
		}
	}

	if columnsNode != nil && columnsNode.Kind == yaml.SequenceNode {
		for _, colNode := range columnsNode.Content {
			if colNode.Kind != yaml.MappingNode {
				continue
			}
			var name string
			for j := 0; j+1 < len(colNode.Content); j += 2 {
				if colNode.Content[j].Value == "name" {
					name = colNode.Content[j+1].Value
				}
			}
			for j := 0; j+1 < len(colNode.Content); j += 2 {
				if colNode.Content[j].Value == "actions" {
					return fmt.Errorf("%s: columns[%q].actions: %s", path, name, legacyActionsRemovedHint)
				}
			}
		}
	}

	if keymapsNode != nil && cfg.Keymaps != nil {
		stampKeymapsOrder(keymapsNode, cfg.Keymaps)
	}

	return nil
}

// nodeKeysInOrder returns node's top-level mapping keys in document order,
// or nil if node is not a mapping.
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
// If the file already exists, it preserves existing fields (like keymaps).
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
		if err := stampOrderAndRejectLegacy(preData, &cfg, path); err != nil {
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

// validateActionValue validates that one action definition is well-formed
// (name, type, url/command per type, and scope, including the board/card
// ticket-variable restrictions), inferring and writing back the default
// scope in place when it was omitted. Called by validateKeymapActions for
// every inline keymaps: action definition -- key only identifies the
// offending entry in any returned error, it plays no part in the validation
// itself.
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

// validateScopeConflicts checks that no key sequence is assigned a "card"
// scope by one inline action and a "pr" scope by another, across
// keymaps.normal, keymaps.detail, and every keymaps.columns.<name> table.
// Per #510's Q1 decision, only card<->pr conflicts are rejected; a key shared
// between "board" and either "card" or "pr" across tables is unchanged,
// existing behavior.
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
