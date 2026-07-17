package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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
const DefaultActionRefreshDelay = 5
const DefaultWorkingLabel = "Working"

// Config holds the application configuration.
type Config struct {
	Provider           string            `yaml:"provider"`
	Repo               string            `yaml:"repo"`
	Project            string            `yaml:"project"`
	Actions            map[string]Action `yaml:"actions"`
	Columns            []ColumnConfig    `yaml:"columns"`
	SessionMaxLength   int               `yaml:"session_max_length"`
	RefreshInterval    int               `yaml:"refresh_interval"`
	ActionRefreshDelay *int              `yaml:"action_refresh_delay,omitempty"`
	WorkingLabel       *string           `yaml:"working_label,omitempty"`
	Mouse              *bool             `yaml:"mouse,omitempty"`
	Cenci              *bool             `yaml:"cenci,omitempty"`
	Cleanup            *string           `yaml:"cleanup,omitempty"`
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

// CleanupValue returns the configured top-level default cleanup command, or ""
// if not set. Used as the fallback for any column that doesn't set its own.
func (c Config) CleanupValue() string {
	if c.Cleanup == nil {
		return ""
	}
	return *c.Cleanup
}

// ActionRefreshDelayValue returns the configured action refresh delay in seconds,
// or DefaultActionRefreshDelay if not set. Negative values are clamped to 0.
func (c Config) ActionRefreshDelayValue() int {
	if c.ActionRefreshDelay == nil {
		return DefaultActionRefreshDelay
	}
	if *c.ActionRefreshDelay < 0 {
		return 0
	}
	return *c.ActionRefreshDelay
}

// DefaultScope returns "card" when s is empty, otherwise s unchanged. An
// action's scope defaults to "card" when not explicitly set.
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

// Load reads configuration from globalPath and localPath YAML files.
// Local config merges on top of global. Returns defaults if no files exist.
func Load(globalPath, localPath string) (Config, error) {
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
		if _, err := assignActionOrder(globalData, cfg.Actions, cfg.Columns); err != nil {
			return Config{}, err
		}
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
	// place, so globalActions ends up aliasing cfg.Actions once the local
	// unmarshal runs. That's why the key-existence-based Order offset below
	// can't rely on map identity/length here the way the column-level merge
	// (mergeColumnActions) can — it instead tracks which keys the local
	// document itself declared (localActionKeys, from assignActionOrder's
	// return value) to know which entries are genuinely global-only.
	globalActions := cfg.Actions
	globalColumns := cfg.Columns

	// Read local config file, unmarshal into the same struct.
	localData, err := os.ReadFile(localPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	var localActionKeys map[string]bool
	if err == nil {
		if err := yaml.Unmarshal(localData, &cfg); err != nil {
			return Config{}, err
		}
		keys, err := assignActionOrder(localData, cfg.Actions, cfg.Columns)
		if err != nil {
			return Config{}, err
		}
		localActionKeys = keys
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
	// inherited unchanged from global) after all locally-declared keys,
	// preserving each group's relative order.
	if localCount := len(localActionKeys); localCount > 0 {
		for k, v := range cfg.Actions {
			if !localActionKeys[k] {
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

	if err := validateColumns(&cfg); err != nil {
		return Config{}, err
	}

	if err := validateActions(cfg.Actions); err != nil {
		return Config{}, err
	}

	if err := validateScopeConflicts(&cfg); err != nil {
		return Config{}, err
	}

	if err := validatePrefixConflicts(&cfg); err != nil {
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

// assignActionOrder parses data a second time as a yaml.Node tree and stamps
// each entry in actions (and each column's own actions) with its 1-based
// position in the raw document, so callers can render actions in the order
// the user wrote them instead of Go's randomized map order. actions and
// columns must be the already-unmarshaled values produced from this same
// data, since map values holding structs aren't addressable and require a
// read-modify-write. Columns are matched by index, not name: within one raw
// document, columns is already in document order courtesy of normal yaml.v3
// unmarshaling, so columnsNode.Content[i] lines up with columns[i]
// positionally. Name-based matching across documents (global vs local) is a
// separate, later concern handled by mergeColumnActions/columnsByNameLower.
//
// It returns the set of top-level action keys this document's own actions:
// mapping declares (nil if the document has none), which Load() uses to tell
// genuinely local keys apart from keys merely inherited unchanged from
// another document (see the comment on globalActions in Load()).
func assignActionOrder(data []byte, actions map[string]Action, columns []ColumnConfig) (declaredKeys map[string]bool, err error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if len(root.Content) == 0 {
		return nil, nil
	}
	docNode := root.Content[0]
	if docNode.Kind != yaml.MappingNode {
		return nil, nil
	}

	var actionsNode, columnsNode *yaml.Node
	for i := 0; i+1 < len(docNode.Content); i += 2 {
		key := docNode.Content[i]
		value := docNode.Content[i+1]
		switch key.Value {
		case "actions":
			actionsNode = value
		case "columns":
			columnsNode = value
		}
	}

	if actionsNode != nil {
		declaredKeys = stampActionOrder(actionsNode, actions)
	}

	if columnsNode != nil && columnsNode.Kind == yaml.SequenceNode {
		for i, colNode := range columnsNode.Content {
			if i >= len(columns) || colNode.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(colNode.Content); j += 2 {
				key := colNode.Content[j]
				value := colNode.Content[j+1]
				if key.Value == "actions" {
					stampActionOrder(value, columns[i].Actions)
				}
			}
		}
	}

	return declaredKeys, nil
}

// stampActionOrder walks a YAML mapping node of action keys, assigns each
// matching entry in actions its 1-based document position, and returns the
// set of keys found in the node.
func stampActionOrder(node *yaml.Node, actions map[string]Action) map[string]bool {
	if node.Kind != yaml.MappingNode {
		return nil
	}
	keys := make(map[string]bool, len(node.Content)/2)
	order := 1
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		keys[key] = true
		if a, ok := actions[key]; ok {
			a.Order = order
			actions[key] = a
		}
		order++
	}
	return keys
}

// LocalExists returns true if the file at path exists.
func LocalExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Save writes provider and repo to the config file at path.
// If the file already exists, it preserves existing fields (like actions).
func Save(path, provider, repo string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".yml" && ext != ".yaml" {
		return fmt.Errorf("config path %q must have .yml or .yaml extension", path)
	}

	// Read existing config if file exists.
	var cfg Config
	data, err := os.ReadFile(path)
	if err == nil {
		_ = yaml.Unmarshal(data, &cfg) // ignore error, start fresh if invalid
	}

	// Update provider and repo.
	cfg.Provider = provider
	cfg.Repo = repo

	// Marshal and write.
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0600)
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
var cardSpecificVarPattern = regexp.MustCompile(`\{(number|title|tags|session)\}`)

// prSpecificVarPattern matches PR-specific template variables (scope: pr only).
var prSpecificVarPattern = regexp.MustCompile(`\{(pr_branch|pr_number|pr_url|pr_title|pr_worktree)\}`)

// validateActions checks that all action definitions are well-formed.
func validateActions(actions map[string]Action) error {
	for key, action := range actions {
		// Key is a sequence of one or more keys pressed one after another
		// (neovim-style prefix bindings). The first key must be an uppercase
		// letter A-Z (the reserved custom-action namespace); continuation
		// keys may be any letter or digit, since a pending sequence consumes
		// every key until it resolves.
		runes := []rune(key)
		if len(runes) == 0 || runes[0] < 'A' || runes[0] > 'Z' {
			return fmt.Errorf("action key %q must start with an uppercase letter (A-Z)", key)
		}
		for _, r := range runes[1:] {
			if !IsSequenceKey(r) {
				return fmt.Errorf("action key %q: sequence keys after the first must be letters or digits", key)
			}
		}
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
		// Default empty scope to "card".
		if action.Scope == "" {
			action.Scope = "card"
			actions[key] = action
		}
		// Validate scope value.
		if action.Scope != "card" && action.Scope != "board" && action.Scope != "pr" {
			return fmt.Errorf("action %q: scope must be \"card\", \"board\", or \"pr\", got %q", key, action.Scope)
		}
		template := action.URL + action.Command
		// Board-scope actions must not reference card-specific variables.
		if action.Scope == "board" {
			if cardSpecificVarPattern.MatchString(template) {
				return fmt.Errorf("action %q: scope \"board\" cannot use card-specific variables ({number}, {title}, {tags}, {session})", key)
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
	}
	return nil
}

// IsSequenceKey reports whether r is a valid continuation key of an action
// key sequence: any letter or digit.
func IsSequenceKey(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

// validatePrefixConflicts rejects any action key that is a strict prefix of
// another action key in a key set that can be active at the same time: the
// prefix key could then never dispatch (pressing it must wait for a
// continuation key). Resolution consults the active column's actions plus
// the global actions, so the check runs on the global map alone and on each
// column's keys merged with the global keys. An identical key appearing in
// both maps is an ordinary column override, not a conflict.
func validatePrefixConflicts(cfg *Config) error {
	globalKeys := make([]string, 0, len(cfg.Actions))
	for key := range cfg.Actions {
		globalKeys = append(globalKeys, key)
	}
	if a, b, ok := findPrefixPair(globalKeys); ok {
		return fmt.Errorf("action key %q is a prefix of action key %q: a prefix key can never dispatch", a, b)
	}

	for _, col := range cfg.Columns {
		merged := make(map[string]bool, len(globalKeys)+len(col.Actions))
		for _, key := range globalKeys {
			merged[key] = true
		}
		for key := range col.Actions {
			merged[key] = true
		}
		keys := make([]string, 0, len(merged))
		for key := range merged {
			keys = append(keys, key)
		}
		if a, b, ok := findPrefixPair(keys); ok {
			return fmt.Errorf("column %q: action key %q is a prefix of action key %q: a prefix key can never dispatch", col.Name, a, b)
		}
	}
	return nil
}

// findPrefixPair returns the first pair of distinct keys where one is a
// strict prefix of the other (shorter key first). Action maps are tiny, so
// the pairwise scan is fine.
func findPrefixPair(keys []string) (prefix, key string, found bool) {
	for _, a := range keys {
		for _, b := range keys {
			if a != b && strings.HasPrefix(b, a) {
				return a, b, true
			}
		}
	}
	return "", "", false
}

// validateScopeConflicts checks that no action key is assigned a "card" scope
// in one map and a "pr" scope in another (across the global actions map and
// every column's action override map). Per the ticket's Q1 decision, only
// card<->pr conflicts are rejected; a letter shared between "board" and
// either "card" or "pr" across maps is existing (unchanged) behavior.
func validateScopeConflicts(cfg *Config) error {
	scopesByKey := make(map[string]map[string]bool)

	addScopes := func(actions map[string]Action) {
		for key, action := range actions {
			scope := DefaultScope(action.Scope)
			if scopesByKey[key] == nil {
				scopesByKey[key] = make(map[string]bool)
			}
			scopesByKey[key][scope] = true
		}
	}

	addScopes(cfg.Actions)
	for _, col := range cfg.Columns {
		addScopes(col.Actions)
	}

	for key, scopes := range scopesByKey {
		if scopes["card"] && scopes["pr"] {
			return fmt.Errorf("action key %q: cannot be both \"card\" scope and \"pr\" scope across global/column action maps", key)
		}
	}
	return nil
}
