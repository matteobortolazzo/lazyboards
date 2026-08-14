package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- #569: coverage for the three runCleanupCmds dispatch sites (background
// refresh's detectDepartures, handleCardClosed, handleCardDeleted). Each
// resolves its column cleanup command from a real config.Load pipeline that
// strips an untrusted local cleanup:/columns[].cleanup, restoring a
// global-declared cleanup when one exists (see stripLocalCleanup,
// internal/config/trust_strip.go). A stripped local override must never
// surface as a fe.RunShellCalls entry; a surviving global default must.

// writeTrustCleanupConfigs writes globalYAML/localYAML (if non-empty) to temp
// files and returns their paths, mirroring internal/config/trust_strip_test.go's
// writeTempConfigs. Duplicated here (rather than exported cross-package)
// because this file exercises config.Load purely through its public
// package-external surface (config.Load, config.Trust), the same boundary
// production code (main.go) crosses.
func writeTrustCleanupConfigs(t *testing.T, globalYAML, localYAML string) (globalPath, localPath string) {
	t.Helper()
	dir := t.TempDir()
	globalPath = filepath.Join(dir, "global.yml")
	localPath = filepath.Join(dir, "local.yml")

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

	return globalPath, localPath
}

// newTrustCleanupTestBoard mirrors cleanup_test.go's newCleanupTestBoardWith,
// but sources columnConfigs (and actions) from a real
// config.Load(globalPath, localPath, trust) over temp files instead of a
// hand-built config.ColumnConfig slice -- so untrusted-local cleanup
// stripping (#569) is exercised through the real Load pipeline this ticket
// changes, not a hand-simulated shortcut. Column names (New/Refined/
// Implementing/Implemented) mirror newCleanupTestBoardWith's fixture so the
// resolved config's columns line up by name (case-insensitively, see
// columnCleanup) with provider.NewFakeProvider's fetched board columns
// (New/Refined/Implementing).
func newTrustCleanupTestBoard(t *testing.T, globalYAML, localYAML string, trust config.Trust) (Board, *action.FakeExecutor, *provider.FakeProvider) {
	t.Helper()
	globalPath, localPath := writeTrustCleanupConfigs(t, globalYAML, localYAML)
	cfg, err := config.Load(globalPath, localPath, trust)
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}

	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, cfg.Columns, fe, "matteobortolazzo", "lazyboards", "github", 32, 0, "Working", false, false, nil, nil, true)
	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)
	b.Width = 120
	b.Height = 40
	return b, fe, p
}

// untrustedLocalOnlyCleanupYAML declares a non-empty columns[].cleanup on
// "New" with no matching global column at all, so a correct strip leaves the
// column with no cleanup command whatsoever (nothing to fall back to).
const untrustedLocalOnlyCleanupYAML = `
provider: github
repo: owner/repo
columns:
  - name: New
    cleanup: "echo local-evil"
  - name: Refined
  - name: Implementing
  - name: Implemented
`

// globalCleanupYAML declares a per-column global cleanup for "New" that must
// survive and resolve after the local per-column override above is stripped.
// This fixture only ever exercises the pre-existing globalColumns snapshot
// (mergeColumnCleanup's fallback) -- it never touches the value-copied
// globalCleanup snapshot (see globalTopLevelCleanupYAML below for that).
const globalCleanupYAML = `
columns:
  - name: New
    cleanup: "echo global-safe"
`

// globalTopLevelCleanupYAML declares a global top-level default cleanup with
// no columns: block at all, so it reaches every column purely through
// applyDefaultCleanup. Pairing this with untrustedLocalTopLevelCleanupYAML
// below (a local top-level override of the same field) is the case that
// actually depends on globalCleanup being a value-copied snapshot rather
// than a pointer alias (see the comment on globalCleanup in config.go): a
// pointer-alias regression would let the local override silently overwrite
// the snapshot in place before the strip ever compares against it.
const globalTopLevelCleanupYAML = `
cleanup: "echo global-safe"
`

// untrustedLocalTopLevelCleanupYAML declares a local top-level cleanup
// override with no columns: block, so a correct strip restores the global
// top-level default (globalTopLevelCleanupYAML) to every column via
// applyDefaultCleanup.
const untrustedLocalTopLevelCleanupYAML = `
provider: github
repo: owner/repo
cleanup: "echo local-evil"
`

// --- Background refresh (update.go's detectDepartures, dispatched behind
// the mass-departure circuit breaker) ---

func TestTrustCleanup_BackgroundRefresh_UntrustedLocalOnly_NoCommandRuns(t *testing.T) {
	b, fe, _ := newTrustCleanupTestBoard(t, "", untrustedLocalOnlyCleanupYAML, config.Trust{})

	// The move-debounce (#363) defers a column-change departure to the
	// second consecutive fetch; one moved card of the fixture's 12 tracked
	// cards stays well under the circuit breaker's thresholds.
	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected no RunShell calls: untrusted local cleanup must be stripped with no global fallback, got: %v", fe.RunShellCalls)
	}
}

func TestTrustCleanup_BackgroundRefresh_UntrustedLocalWithGlobal_GlobalCommandRuns(t *testing.T) {
	b, fe, _ := newTrustCleanupTestBoard(t, globalCleanupYAML, untrustedLocalOnlyCleanupYAML, config.Trust{})

	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected the global cleanup command to run despite the untrusted local override, got none")
	}
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "local-evil") {
			t.Fatalf("RunShell call %q contains the untrusted local override; it must have been stripped", call)
		}
	}
	found := false
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "global-safe") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a RunShell call containing the global cleanup command, got: %v", fe.RunShellCalls)
	}
}

// TestTrustCleanup_BackgroundRefresh_UntrustedLocalTopLevelWithGlobalTopLevel_GlobalCommandRuns
// pairs globalTopLevelCleanupYAML/untrustedLocalTopLevelCleanupYAML (both
// top-level cleanup:, no columns: block) rather than the per-column
// globalCleanupYAML/untrustedLocalOnlyCleanupYAML pair used above -- this is
// the case that actually depends on globalCleanup being a value-copied
// snapshot, not a pointer alias (see the comment on globalTopLevelCleanupYAML).
func TestTrustCleanup_BackgroundRefresh_UntrustedLocalTopLevelWithGlobalTopLevel_GlobalCommandRuns(t *testing.T) {
	b, fe, _ := newTrustCleanupTestBoard(t, globalTopLevelCleanupYAML, untrustedLocalTopLevelCleanupYAML, config.Trust{})

	b = refreshCleanupBoard(t, b, fakeRefreshBoard(1))
	refreshCleanupBoard(t, b, fakeRefreshBoard(1))

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected the global top-level cleanup command to run despite the untrusted local top-level override, got none")
	}
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "local-evil") {
			t.Fatalf("RunShell call %q contains the untrusted local override; it must have been stripped", call)
		}
	}
	found := false
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "global-safe") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a RunShell call containing the global top-level cleanup command, got: %v", fe.RunShellCalls)
	}
}

// --- handleCardClosed (update.go, no breaker/debounce) ---

func TestTrustCleanup_CardClosed_UntrustedLocalOnly_NoCommandRuns(t *testing.T) {
	b, fe, _ := newTrustCleanupTestBoard(t, "", untrustedLocalOnlyCleanupYAML, config.Trust{})

	m, cmd := b.Update(cardClosedMsg{card: Card{Number: 1, Title: "Setup CI", Labels: []Label{{Name: "infra"}}}})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected no RunShell calls: untrusted local cleanup must be stripped with no global fallback, got: %v", fe.RunShellCalls)
	}
	for _, c := range b.Columns[0].Cards {
		if c.Number == 1 {
			t.Fatal("expected card removed from Columns after cardClosedMsg")
		}
	}
}

func TestTrustCleanup_CardClosed_UntrustedLocalWithGlobal_GlobalCommandRuns(t *testing.T) {
	b, fe, _ := newTrustCleanupTestBoard(t, globalCleanupYAML, untrustedLocalOnlyCleanupYAML, config.Trust{})

	m, cmd := b.Update(cardClosedMsg{card: Card{Number: 1, Title: "Setup CI", Labels: []Label{{Name: "infra"}}}})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected the global cleanup command to run despite the untrusted local override, got none")
	}
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "local-evil") {
			t.Fatalf("RunShell call %q contains the untrusted local override; it must have been stripped", call)
		}
	}
	found := false
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "global-safe") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a RunShell call containing the global cleanup command, got: %v", fe.RunShellCalls)
	}
	for _, c := range b.Columns[0].Cards {
		if c.Number == 1 {
			t.Fatal("expected card removed from Columns after cardClosedMsg")
		}
	}
}

// --- handleCardDeleted (update.go, no breaker/debounce) ---

func TestTrustCleanup_CardDeleted_UntrustedLocalOnly_NoCommandRuns(t *testing.T) {
	b, fe, _ := newTrustCleanupTestBoard(t, "", untrustedLocalOnlyCleanupYAML, config.Trust{})

	m, cmd := b.Update(cardDeletedMsg{card: Card{Number: 1, Title: "Setup CI", Labels: []Label{{Name: "infra"}}}})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("expected no RunShell calls: untrusted local cleanup must be stripped with no global fallback, got: %v", fe.RunShellCalls)
	}
	for _, c := range b.Columns[0].Cards {
		if c.Number == 1 {
			t.Fatal("expected card removed from Columns after cardDeletedMsg")
		}
	}
}

func TestTrustCleanup_CardDeleted_UntrustedLocalWithGlobal_GlobalCommandRuns(t *testing.T) {
	b, fe, _ := newTrustCleanupTestBoard(t, globalCleanupYAML, untrustedLocalOnlyCleanupYAML, config.Trust{})

	m, cmd := b.Update(cardDeletedMsg{card: Card{Number: 1, Title: "Setup CI", Labels: []Label{{Name: "infra"}}}})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected the global cleanup command to run despite the untrusted local override, got none")
	}
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "local-evil") {
			t.Fatalf("RunShell call %q contains the untrusted local override; it must have been stripped", call)
		}
	}
	found := false
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "global-safe") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a RunShell call containing the global cleanup command, got: %v", fe.RunShellCalls)
	}
	for _, c := range b.Columns[0].Cards {
		if c.Number == 1 {
			t.Fatal("expected card removed from Columns after cardDeletedMsg")
		}
	}
}
