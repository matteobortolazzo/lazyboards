package config

import (
	"os"
	"path/filepath"
	"testing"
)

func loadConfigFromStrings(t *testing.T, globalYAML, localYAML string) (Config, error) {
	t.Helper()
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "global.yml")
	localPath := filepath.Join(dir, "local.yml")

	if globalYAML != "" {
		if err := os.WriteFile(globalPath, []byte(globalYAML), 0644); err != nil {
			t.Fatalf("failed to write global config: %v", err)
		}
	} else {
		globalPath = filepath.Join(dir, "nonexistent-global.yml")
	}

	if localYAML != "" {
		if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
			t.Fatalf("failed to write local config: %v", err)
		}
	} else {
		localPath = filepath.Join(dir, "nonexistent-local.yml")
	}

	return Load(globalPath, localPath)
}

func mustLoadConfig(t *testing.T, globalYAML, localYAML string) Config {
	t.Helper()
	cfg, err := loadConfigFromStrings(t, globalYAML, localYAML)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	return cfg
}
