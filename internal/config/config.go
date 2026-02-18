package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Action defines a user-configured action bound to a key.
type Action struct {
	Name    string `yaml:"name"`
	Type    string `yaml:"type"`
	URL     string `yaml:"url"`
	Command string `yaml:"command"`
}

// ColumnConfig defines a column with optional per-column actions.
type ColumnConfig struct {
	Name    string            `yaml:"name"`
	Actions map[string]Action `yaml:"actions"`
}

const DefaultSessionMaxLength = 32

// Config holds the application configuration.
type Config struct {
	Provider         string            `yaml:"provider"`
	Repo             string            `yaml:"repo"`
	Project          string            `yaml:"project"`
	Actions          map[string]Action `yaml:"actions"`
	Columns          []ColumnConfig    `yaml:"columns"`
	SessionMaxLength int               `yaml:"session_max_length"`
}

// DefaultColumns is the default set of column names when none are configured.
var DefaultColumns = []ColumnConfig{
	{Name: "New"},
	{Name: "Refined"},
	{Name: "Implementing"},
	{Name: "Implemented"},
}

// ColumnNames extracts the column name strings from the ColumnConfig slice.
func (c Config) ColumnNames() []string {
	names := make([]string, len(c.Columns))
	for i, col := range c.Columns {
		names[i] = col.Name
	}
	return names
}

// builtinKeys is the set of single-character keys reserved for built-in navigation.
var builtinKeys = map[string]bool{
	"j": true, "k": true,
	"q": true, "r": true, "n": true, "c": true, "p": true,
	"1": true, "2": true, "3": true, "4": true,
	"5": true, "6": true, "7": true, "8": true, "9": true,
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

	if err := validateColumns(&cfg); err != nil {
		return Config{}, err
	}

	if err := validateActions(cfg.Actions); err != nil {
		return Config{}, err
	}

	if cfg.SessionMaxLength <= 0 {
		cfg.SessionMaxLength = DefaultSessionMaxLength
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
		yaml.Unmarshal(data, &cfg) // ignore error, start fresh if invalid
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

// validateActions checks that all action definitions are well-formed.
func validateActions(actions map[string]Action) error {
	for key, action := range actions {
		// Key must be a single character.
		if len([]rune(key)) != 1 {
			return fmt.Errorf("action key %q must be a single character", key)
		}
		// Key must not conflict with built-in keys.
		if builtinKeys[key] {
			return fmt.Errorf("action key %q conflicts with built-in key", key)
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
	}
	return nil
}
