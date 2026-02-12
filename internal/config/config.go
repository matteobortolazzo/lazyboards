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

// Config holds the application configuration.
type Config struct {
	Provider string            `yaml:"provider"`
	Repo     string            `yaml:"repo"`
	Project  string            `yaml:"project"`
	Actions  map[string]Action `yaml:"actions"`
}

// builtinKeys is the set of single-character keys reserved for built-in navigation.
var builtinKeys = map[string]bool{
	"h": true, "l": true, "j": true, "k": true,
	"q": true, "r": true, "n": true,
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

	// Save global actions before local override.
	globalActions := cfg.Actions

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

	if err := validateActions(cfg.Actions); err != nil {
		return Config{}, err
	}

	return cfg, nil
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
