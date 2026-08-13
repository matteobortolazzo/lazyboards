package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- #502: the README's legacy-keymaps-restore snippet must actually load ---
//
// README.md documents a copy-pasteable `keymaps:` snippet (fenced between
// HTML comment markers `<!-- legacy-keymaps-restore:start -->` and
// `<!-- legacy-keymaps-restore:end -->`, under the "Keybinding migration"
// subsection) that restores every pre-#502 default key. This test extracts
// that exact snippet from the real README, loads it through config.Load,
// and asserts it resolves every legacy key to its legacy command -- so an
// accidental edit to either the README prose or the snippet's YAML can't
// silently drift from what actually loads.

const legacyRestoreMarker = "legacy-keymaps-restore"

// extractLegacyRestoreSnippet reads README.md (relative to this package)
// and returns the YAML content between the two legacy-keymaps-restore HTML
// comment fences, stripping the surrounding ```yaml/``` code-fence markers.
// It is a thin wrapper around the shared extractFencedBlock
// (docs_capability_drift_test.go, #586), which generalizes this exact
// fence-and-code-fence extraction shape for other doc/marker pairs.
func extractLegacyRestoreSnippet(t *testing.T) string {
	t.Helper()
	return extractFencedBlock(t, "README.md", legacyRestoreMarker)
}

// TestLegacyRestoreSnippet_LoadsCleanlyAndRestoresOldKeys extracts the
// README's legacy-keymaps-restore snippet, loads it through the real
// config.Load, resolves it through config.ResolveKeymap, and asserts every
// pre-#502 key resolves to its pre-#502 command while every #502 default
// the snippet is documented to unbind is gone.
func TestLegacyRestoreSnippet_LoadsCleanlyAndRestoresOldKeys(t *testing.T) {
	yamlBody := extractLegacyRestoreSnippet(t)

	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	// The snippet is a bare `keymaps:` block; config.Load needs a provider
	// to load cleanly, so prepend one the same way every other test in this
	// package does (mustLoadConfig's fixtures).
	fullYAML := "provider: github\nrepo: owner/repo\n" + yamlBody
	if err := os.WriteFile(localPath, []byte(fullYAML), 0644); err != nil {
		t.Fatalf("failed to write extracted snippet to a temp config file: %v", err)
	}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")

	// Self-trusting: the snippet contains only command bindings (no shell
	// actions), so trust state is behaviorally irrelevant here, but this is
	// explicit for clarity per #567.
	trust := Trust{Trusted: []TrustEntry{{Hash: mustHashLocal(t, localPath)}}}
	cfg, err := Load(globalPath, localPath, trust)
	if err != nil {
		t.Fatalf("config.Load() of the README's legacy-keymaps-restore snippet returned unexpected error: %v", err)
	}

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	legacyNormalCases := []struct {
		key  string
		want keymap.CommandID
	}{
		{"t", keymap.CommandCardDelete},
		{"d", keymap.CommandViewDispatch},
		{"m", keymap.CommandNavReference},
		{"i", keymap.CommandViewMilestoneList},
		{"s", keymap.CommandNavAgent},
		{"u", keymap.CommandBoardSortOrder},
		{"v", keymap.CommandViewPRList},
		{"w", keymap.CommandViewAgentList},
		{"g", keymap.CommandViewGitPanel},
	}
	for _, tc := range legacyNormalCases {
		t.Run("normal/"+tc.key, func(t *testing.T) {
			seq, err := keymap.ParseSequence(tc.key)
			if err != nil {
				t.Fatalf("ParseSequence(%q) error: %v", tc.key, err)
			}
			result := km.Lookup(keymap.ModeNormal, "", seq)
			if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingCommand || result.Binding.Command != tc.want {
				t.Errorf("Lookup(ModeNormal, \"\", %q) = %+v, want OutcomeMatch CommandBinding(%q)", tc.key, result, tc.want)
			}
		})
	}

	t.Run("detail/m", func(t *testing.T) {
		result := km.Lookup(keymap.ModeDetail, "", keymap.Sequence{keymap.Key("m")})
		if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingCommand || result.Binding.Command != keymap.CommandNavReference {
			t.Errorf("Lookup(ModeDetail, \"\", \"m\") = %+v, want OutcomeMatch CommandBinding(%q)", result, keymap.CommandNavReference)
		}
	})

	goneKeys := []string{"D", "P", "A", "G"}
	for _, key := range goneKeys {
		t.Run("normal/gone/"+key, func(t *testing.T) {
			result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key(key)})
			if result.Outcome != keymap.OutcomeNoMatch {
				t.Errorf("Lookup(ModeNormal, \"\", %q) outcome = %v, want OutcomeNoMatch (the snippet must unbind the #502 default)", key, result.Outcome)
			}
		})
	}

	goneSequences := []string{"g a", "g r"}
	for _, seqStr := range goneSequences {
		t.Run("normal/gone/"+seqStr, func(t *testing.T) {
			seq, err := keymap.ParseSequence(seqStr)
			if err != nil {
				t.Fatalf("ParseSequence(%q) error: %v", seqStr, err)
			}
			result := km.Lookup(keymap.ModeNormal, "", seq)
			if result.Outcome != keymap.OutcomeNoMatch {
				t.Errorf("Lookup(ModeNormal, \"\", %q) outcome = %v, want OutcomeNoMatch (the snippet must unbind the #502 default)", seqStr, result.Outcome)
			}
		})
	}
}
