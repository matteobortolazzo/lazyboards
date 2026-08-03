package config

import (
	"os"
	"path/filepath"
	"testing"
)

// loadConfigFromStrings loads globalYAML/localYAML with a self-trusting
// Trust computed from localYAML's own bytes (or the zero-value Trust when
// localYAML is empty -- there's no local file for a hash to mean anything
// against), so every pre-existing caller of this helper keeps exercising
// the trusted-local path it always implicitly exercised before Load() grew
// its trust argument. Tests that need to exercise untrusted-local stripping
// call loadConfigFromStringsWithTrust directly with an explicit Trust.
func loadConfigFromStrings(t *testing.T, globalYAML, localYAML string) (Config, error) {
	t.Helper()
	var trust Trust
	if localYAML != "" {
		trust = Trust{Trusted: []TrustEntry{{Hash: hashConfigBytes([]byte(localYAML))}}}
	}
	return loadConfigFromStringsWithTrust(t, globalYAML, localYAML, trust)
}

// loadConfigFromStringsWithTrust writes globalYAML/localYAML (if non-empty)
// to temp files and loads them via Load, passing trust through unchanged.
func loadConfigFromStringsWithTrust(t *testing.T, globalYAML, localYAML string, trust Trust) (Config, error) {
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

	return Load(globalPath, localPath, trust)
}

func mustLoadConfig(t *testing.T, globalYAML, localYAML string) Config {
	t.Helper()
	cfg, err := loadConfigFromStrings(t, globalYAML, localYAML)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	return cfg
}
