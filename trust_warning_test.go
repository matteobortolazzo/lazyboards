package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- #568: in-app status-bar startup warning for a Config.Notices strip
// notice (Board.startupWarning, mirroring the established
// cleanupBreakerWarning transient hand-off precedent in model.go), and the
// AC18 regression guard that trust survives an in-app config Save() rewrite
// via Board.trustPath. ---

// shellKeymapYAML declares a single inline keymaps: normal shell binding on
// "z", the shape stripLocalShellSinks strips when untrusted (see
// internal/config/trust_strip.go's stripShellFromKeymapTable) and restores
// unstripped when trusted.
const shellKeymapYAML = `
provider: github
repo: owner/repo
keymaps:
  normal:
    z: { name: Evil, type: shell, command: "echo evil" }
`

// loadShellKeymapConfig writes shellKeymapYAML to a temp local config file
// and loads it via the real config.Load(globalPath, localPath, trust)
// pipeline, with no global config present (mirroring
// trust_cleanup_test.go's writeTrustCleanupConfigs' missing-file convention).
func loadShellKeymapConfig(t *testing.T, trust config.Trust) config.Config {
	t.Helper()
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	if err := os.WriteFile(localPath, []byte(shellKeymapYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")

	cfg, err := config.Load(globalPath, localPath, trust)
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}
	return cfg
}

// --- AC13: the strip notice surfaces as a timed StatusWarning status-bar
// message, applied and cleared through the same transient hand-off shape as
// cleanupBreakerWarning, in BOTH handleBoardFetched branches (#568's full
// precedence rule, mirroring cleanupBreakerWarning's own two consumption
// sites at update.go ~547-552 and ~592-597). ---

func TestStartupWarning_UntrustedConfig_AppliedAndClearedOnInitialLoad(t *testing.T) {
	cfg := loadShellKeymapConfig(t, config.Trust{})
	if len(cfg.Notices) != 1 {
		t.Fatalf("cfg.Notices = %v, want exactly one strip notice for an untrusted local shell keymap binding", cfg.Notices)
	}

	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, cfg.Columns, nil, "owner", "repo", "github", 0, 0, "Working", false, false, nil, nil, true)
	// b.loaded is false here (fresh board, never fetched), so this Update
	// call exercises handleBoardFetched's b.refreshing == false branch.
	b.startupWarning = cfg.Notices[0]

	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)

	if b.statusBar.level != StatusWarning {
		t.Fatalf("statusBar.level = %v, want StatusWarning", b.statusBar.level)
	}
	if !strings.Contains(b.statusBar.message, "untrusted") {
		t.Fatalf("statusBar.message = %q, want it to contain the strip notice", b.statusBar.message)
	}
	if b.startupWarning != "" {
		t.Fatalf("startupWarning = %q, want cleared after being applied to the status bar", b.startupWarning)
	}
}

func TestStartupWarning_UntrustedConfig_AppliedAndClearedOnRefreshingBranch(t *testing.T) {
	cfg := loadShellKeymapConfig(t, config.Trust{})
	if len(cfg.Notices) != 1 {
		t.Fatalf("cfg.Notices = %v, want exactly one strip notice for an untrusted local shell keymap binding", cfg.Notices)
	}

	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, cfg.Columns, nil, "owner", "repo", "github", 0, 0, "Working", false, false, nil, nil, true)

	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	// First load, unrelated to the warning: gets the board into loaded/normal
	// state so the second Update below exercises the b.refreshing == true
	// branch of handleBoardFetched instead of the initial-load branch.
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)

	b.refreshing = true
	b.startupWarning = cfg.Notices[0]
	m, cmd = b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)

	if b.statusBar.level != StatusWarning {
		t.Fatalf("statusBar.level = %v, want StatusWarning", b.statusBar.level)
	}
	if !strings.Contains(b.statusBar.message, "untrusted") {
		t.Fatalf("statusBar.message = %q, want it to contain the strip notice", b.statusBar.message)
	}
	if b.startupWarning != "" {
		t.Fatalf("startupWarning = %q, want cleared after being applied to the status bar", b.startupWarning)
	}
}

// TestStartupWarning_TrustedConfig_NoWarningShown covers the negative case:
// a trusted local config produces no Config.Notices, so there is nothing to
// apply and nothing to clear.
func TestStartupWarning_TrustedConfig_NoWarningShown(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	if err := os.WriteFile(localPath, []byte(shellKeymapYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}
	hash, err := config.HashLocalConfig(localPath)
	if err != nil {
		t.Fatalf("config.HashLocalConfig() returned unexpected error: %v", err)
	}
	trust := config.Trust{Trusted: []config.TrustEntry{{Hash: hash}}}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")
	cfg, err := config.Load(globalPath, localPath, trust)
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty for a trusted local shell keymap binding", cfg.Notices)
	}

	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, cfg.Columns, nil, "owner", "repo", "github", 0, 0, "Working", false, false, nil, nil, true)
	// startupWarning intentionally left at its zero value: cfg.Notices was
	// empty, so there is nothing to seed it with.

	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)

	if b.statusBar.level == StatusWarning {
		t.Fatalf("statusBar.level = %v, want no warning shown for a trusted config with no Notices", b.statusBar.level)
	}
	if b.startupWarning != "" {
		t.Fatalf("startupWarning = %q, want empty (nothing to clear)", b.startupWarning)
	}
}

// --- AC18: board-level regression guard -- trust must survive an in-app
// config Save() rewrite (config.Save's carry-forward, Commit 1), so a
// trusted local shell keymap binding still resolves unstripped after the
// config modal's Save flow rewrites the file on disk. ---

func TestConfigSave_TrustSurvivesRewrite_LocalShellBindingResolvesUnstripped(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	trustPath := filepath.Join(dir, "trust.yml")
	globalPath := filepath.Join(dir, "nonexistent-global.yml")

	if err := os.WriteFile(localPath, []byte(shellKeymapYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}
	preHash, err := config.HashLocalConfig(localPath)
	if err != nil {
		t.Fatalf("config.HashLocalConfig() returned unexpected error: %v", err)
	}
	if err := config.SaveTrust(trustPath, config.Trust{Trusted: []config.TrustEntry{{Hash: preHash}}}); err != nil {
		t.Fatalf("config.SaveTrust() returned unexpected error: %v", err)
	}

	cfg, err := config.Load(globalPath, localPath, config.Trust{Trusted: []config.TrustEntry{{Hash: preHash}}})
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: the local shell keymap binding is trusted before Save() ever runs", cfg.Notices)
	}

	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, cfg.Columns, nil, "owner", "repo", "github", 0, 0, "Working", false, false, nil, nil, true)
	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)

	b.config.localPath = localPath
	b.trustPath = trustPath

	// Enter the config modal (default 'c' key) and press Enter to save,
	// mirroring config_mode_test.go's TestConfigMode_Enter_TriggersConfigSave.
	// Both fields already match localPath's provider/repo, so no retyping is
	// needed for a byte-different-but-semantically-unchanged rewrite.
	b = sendKey(t, b, keyMsg("c"))
	m, saveCmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if saveCmd == nil {
		t.Fatal("Enter in configMode should return a non-nil cmd (config save)")
	}
	execCmds(saveCmd)

	postTrust, err := config.LoadTrust(trustPath)
	if err != nil {
		t.Fatalf("config.LoadTrust() (post-save) returned unexpected error: %v", err)
	}
	reloaded, err := config.Load(globalPath, localPath, postTrust)
	if err != nil {
		t.Fatalf("config.Load() (post-save reload) returned unexpected error: %v", err)
	}
	if len(reloaded.Notices) != 0 {
		t.Fatalf("reloaded.Notices = %v, want empty -- trust must have survived the Save() rewrite", reloaded.Notices)
	}

	km, err := config.ResolveKeymap(&reloaded)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("z")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Type != "shell" {
		t.Fatalf("Lookup(normal, \"\", z) = %+v, want the local shell binding to resolve unstripped after trust-carry-forward through Save()", result)
	}
}
