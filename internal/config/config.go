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

// DefaultSessionMaxLength must stay in sync with agentwatch's windowNameMaxLen
// (agent-stack/agentwatch internal/run/slug.go).
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
	AgentWatch         *bool             `yaml:"agentwatch,omitempty"`
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

// AgentWatchValue returns true if agentwatch integration is enabled.
// Defaults to true (enabled) when the field is not set.
func (c Config) AgentWatchValue() bool {
	if c.AgentWatch == nil {
		return true
	}
	return *c.AgentWatch
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
	}

	// Identity fields (provider, repo, project) only come from local config,
	// not from global config. Clear them after global load.
	cfg.Provider = ""
	cfg.Repo = ""
	cfg.Project = ""

	// Save global actions and columns before local override.
	globalActions := cfg.Actions
	globalColumns := cfg.Columns

	// Read local config file, unmarshal into the same struct.
	localData, err := os.ReadFile(localPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	if err == nil {
		if err := yaml.Unmarshal(localData, &cfg); err != nil {
			return Config{}, err
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

// mergeColumnActions merges per-column actions from globalColumns into columns.
// For each column, if a matching global column exists (case-insensitive name),
// global-only action keys are preserved. Local action keys take priority.
// If a local column has nil actions, it inherits all matching global column actions.
func mergeColumnActions(columns []ColumnConfig, globalColumns []ColumnConfig) {
	globalByName := make(map[string]ColumnConfig, len(globalColumns))
	for _, gc := range globalColumns {
		globalByName[strings.ToLower(gc.Name)] = gc
	}

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
		for k, v := range gc.Actions {
			if _, exists := columns[i].Actions[k]; !exists {
				columns[i].Actions[k] = v
			}
		}
	}
}

// mergeColumnCleanup fills in a column's Cleanup from the matching global
// column when the local column didn't specify one. An explicit local value
// (including an explicit empty string, which disables cleanup) always wins.
func mergeColumnCleanup(columns []ColumnConfig, globalColumns []ColumnConfig) {
	globalByName := make(map[string]ColumnConfig, len(globalColumns))
	for _, gc := range globalColumns {
		globalByName[strings.ToLower(gc.Name)] = gc
	}

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
var prSpecificVarPattern = regexp.MustCompile(`\{(pr_branch|pr_number|pr_url|pr_title)\}`)

// validateActions checks that all action definitions are well-formed.
func validateActions(actions map[string]Action) error {
	for key, action := range actions {
		// Key must be a single uppercase letter A-Z.
		runes := []rune(key)
		if len(runes) != 1 || runes[0] < 'A' || runes[0] > 'Z' {
			return fmt.Errorf("action key %q must be an uppercase letter (A-Z)", key)
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
				return fmt.Errorf("action %q: scope \"board\" cannot use pr-specific variables ({pr_branch}, {pr_number}, {pr_url}, {pr_title})", key)
			}
		}
		// Card-scope actions must not reference pr-specific variables.
		if action.Scope == "card" {
			if prSpecificVarPattern.MatchString(template) {
				return fmt.Errorf("action %q: scope \"card\" cannot use pr-specific variables ({pr_branch}, {pr_number}, {pr_url}, {pr_title})", key)
			}
		}
	}
	return nil
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
			scope := action.Scope
			if scope == "" {
				scope = "card"
			}
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
