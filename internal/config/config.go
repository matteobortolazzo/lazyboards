package config

import (
	"errors"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// DefaultColumns are the hardcoded Kanban column names.
var DefaultColumns = []string{"New", "Refined", "Implementing", "PR Ready"}

// Config holds the application configuration.
type Config struct {
	Provider string `yaml:"provider"`
	Repo     string `yaml:"repo"`
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

// SaveGlobal writes only the provider field to the given path.
// It creates parent directories if they don't exist.
func (c Config) SaveGlobal(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(struct {
		Provider string `yaml:"provider"`
	}{Provider: c.Provider})
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// SaveLocal writes only the repo field to the given path.
// It does not create parent directories (local config is always in CWD).
func (c Config) SaveLocal(path string) error {
	data, err := yaml.Marshal(struct {
		Repo string `yaml:"repo"`
	}{Repo: c.Repo})
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

// Exists returns true if either the global or local config file exists.
func Exists(globalPath, localPath string) bool {
	if _, err := os.Stat(globalPath); !os.IsNotExist(err) {
		return true
	}
	if _, err := os.Stat(localPath); !os.IsNotExist(err) {
		return true
	}
	return false
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

	return cfg, nil
}
