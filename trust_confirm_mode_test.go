package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- #640/#644: trustConfirmMode end-to-end ---
//
// trustConfirmEntry (main.go) is the single production decision function
// gating this mode: cfg.LocalHash != "" && !trust.Trusts(cfg.LocalHash) --
// StaleTrust's own semantics -- never len(cfg.Notices) > 0. These tests
// drive that function directly (the same "enter the real way" convention
// universal_quit_test.go's trust_confirm case uses), never a bare
// b.mode assignment that bypasses it.

// trustReloadYAML declares one column with a local cleanup command and one
// inline shell keymap binding on "z" -- the two constructs
// stripLocalShellSinks strips when untrusted and the accept flow must bring
// back live, without a restart.
const trustReloadYAML = `
provider: github
repo: owner/repo
columns:
  - name: Todo
    cleanup: 'echo cleanup {number}'
keymaps:
  normal:
    z: { name: Evil, type: shell, command: "echo evil" }
`

// trustConfirmFixture bundles the on-disk local config + trust store a test
// needs to drive trustConfirmEntry/acceptTrustCmd exactly the way main.go
// does: cfg and trust are the same values main.go loads once and reuses,
// and trustPath points at a real file acceptTrustCmd can independently
// re-read/rewrite (it never trusts an in-memory value, mirroring
// production).
type trustConfirmFixture struct {
	localPath string
	trustPath string
	cfg       config.Config
	trust     config.Trust
}

// newTrustConfirmFixture writes localYAML to a temp local config file,
// persists staleTrust to a temp trust store, and loads cfg through the real
// config.Load pipeline against that store.
func newTrustConfirmFixture(t *testing.T, localYAML string, staleTrust config.Trust) trustConfirmFixture {
	t.Helper()
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}
	trustPath := filepath.Join(dir, "trust.yml")
	if err := config.SaveTrust(trustPath, staleTrust); err != nil {
		t.Fatalf("config.SaveTrust() returned unexpected error: %v", err)
	}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")
	cfg, err := config.Load(globalPath, localPath, staleTrust)
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}
	return trustConfirmFixture{localPath: localPath, trustPath: trustPath, cfg: cfg, trust: staleTrust}
}

// newTrustConfirmBoard builds a Board from fx exactly the way main.go does
// (NewBoard -> config.ResolveKeymap -> withKeymap -> trustPath), then
// applies trustConfirmEntry(fx.cfg, fx.trust, identity) the same way
// main.go's call site does, failing the test if it doesn't report a stale
// match (every caller of this helper is exercising the positive path;
// negative cases call trustConfirmEntry directly instead).
func newTrustConfirmBoard(t *testing.T, fx trustConfirmFixture, identity string) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, fx.cfg.Columns, nil, "owner-before", "repo-before", "github", 0, 0, "Working", false, false, nil, nil, true)
	km, err := config.ResolveKeymap(&fx.cfg)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}
	b = b.withKeymap(km)
	b.trustPath = fx.trustPath
	b.config.localPath = fx.localPath
	b.Width, b.Height = 120, 40

	state, ok := trustConfirmEntry(fx.cfg, fx.trust, identity)
	if !ok {
		t.Fatalf("precondition: trustConfirmEntry(cfg, trust, %q) ok = false, want true", identity)
	}
	b.mode = trustConfirmMode
	b.trustConfirm = state
	return b
}

// --- Entry / non-entry precedence ---

func TestTrustConfirmEntry_FirstEverUntrustedLoad_DoesNotPrompt(t *testing.T) {
	fx := newTrustConfirmFixture(t, "provider: github\nrepo: owner/repo\n", config.Trust{})

	if _, ok := trustConfirmEntry(fx.cfg, fx.trust, "some-identity"); ok {
		t.Fatal("trustConfirmEntry() ok = true, want false for a genuine first-ever untrusted load (no prior entry for this identity)")
	}
}

func TestTrustConfirmEntry_HashAlreadyTrusted_DoesNotPrompt(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	yaml := "provider: github\nrepo: owner/repo\n"
	if err := os.WriteFile(localPath, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}
	hash, err := config.HashLocalConfig(localPath)
	if err != nil {
		t.Fatalf("config.HashLocalConfig() returned unexpected error: %v", err)
	}
	identity := "trust-confirm-identity"
	// The stored entry trusts the CURRENT hash under this identity -- a
	// stale Path match must never fire once the hash itself is trusted,
	// regardless of what else is in the store.
	trust := config.Trust{Trusted: []config.TrustEntry{{Hash: hash, Path: identity}}}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")
	cfg, err := config.Load(globalPath, localPath, trust)
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}

	if _, ok := trustConfirmEntry(cfg, trust, identity); ok {
		t.Fatal("trustConfirmEntry() ok = true, want false when the local config's hash is already trusted")
	}
}

func TestTrustConfirmEntry_StaleMatch_PromptsEvenWhenNothingWasStripped(t *testing.T) {
	// No shell bindings, no cleanup: cfg.Notices must be empty even though
	// this is exactly the case the reprompt exists for -- proving the
	// trigger is gated on cfg.LocalHash/trust.StaleTrust, never on
	// len(cfg.Notices) > 0 (the trap named in AGENTS.md/the plan).
	identity := "trust-confirm-identity"
	fx := newTrustConfirmFixture(t, "provider: github\nrepo: owner/repo\n",
		config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity, Note: "legacy note"}}})

	if len(fx.cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty (nothing in this config needs stripping)", fx.cfg.Notices)
	}

	state, ok := trustConfirmEntry(fx.cfg, fx.trust, identity)
	if !ok {
		t.Fatal("trustConfirmEntry() ok = false, want true for an untrusted hash with a stale same-identity entry, even with empty cfg.Notices")
	}
	if state.hash != fx.cfg.LocalHash {
		t.Errorf("state.hash = %q, want %q", state.hash, fx.cfg.LocalHash)
	}
	if state.identity != identity {
		t.Errorf("state.identity = %q, want %q", state.identity, identity)
	}
	if state.note != "legacy note" {
		t.Errorf("state.note = %q, want %q (carried forward from the stale entry)", state.note, "legacy note")
	}
}

// --- Init() / race pin ---

func TestTrustConfirmMode_Init_ReturnsNil(t *testing.T) {
	b := newTestBoard(t)
	b.mode = trustConfirmMode

	if cmd := b.Init(); cmd != nil {
		t.Fatal("Init() returned a non-nil Cmd while mode == trustConfirmMode, want nil (must not race the prompt with a concurrent board fetch)")
	}
}

func TestTrustConfirmMode_BoardFetchedMsgDuringPrompt_DoesNotChangeMode(t *testing.T) {
	b := newTestBoard(t)
	b.mode = trustConfirmMode
	b.trustConfirm = trustConfirmState{hash: "sha256:new", identity: "id", note: "n"}

	board, err := provider.NewFakeProvider().FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}

	m, cmd := b.Update(boardFetchedMsg{board: board})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.mode != trustConfirmMode {
		t.Fatalf("mode after a boardFetchedMsg arriving mid-prompt = %d, want unchanged trustConfirmMode (this should be structurally unreachable in production, since Init() never issues the fetch for this mode -- this pins the guard regardless)", updated.mode)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil (the guard short-circuits before handleBoardFetched runs)", cmd)
	}
	if updated.trustConfirm != b.trustConfirm {
		t.Errorf("trustConfirm state changed by the stray boardFetchedMsg: got %+v, want unchanged %+v", updated.trustConfirm, b.trustConfirm)
	}
}

// --- Skip ---

func TestTrustConfirmMode_Skip_TransitionsToLoadingAndClearsState(t *testing.T) {
	for _, key := range []string{"s", "esc"} {
		t.Run(key, func(t *testing.T) {
			identity := "trust-confirm-skip-" + key
			fx := newTrustConfirmFixture(t, "provider: github\nrepo: owner/repo\n",
				config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity}}})
			b := newTrustConfirmBoard(t, fx, identity)

			before, err := os.ReadFile(fx.trustPath)
			if err != nil {
				t.Fatalf("failed to read trust store before keypress: %v", err)
			}

			var keyMsgToSend tea.KeyMsg
			if key == "esc" {
				keyMsgToSend = arrowMsg(tea.KeyEsc)
			} else {
				keyMsgToSend = keyMsg(key)
			}
			m, cmd := b.Update(keyMsgToSend)
			updated := m.(Board)

			if updated.mode != loadingMode {
				t.Fatalf("mode after %q = %d, want loadingMode", key, updated.mode)
			}
			if updated.trustConfirm != (trustConfirmState{}) {
				t.Fatalf("trustConfirm = %+v after skip, want the zero value", updated.trustConfirm)
			}

			msgs := runCmd(cmd)
			foundFetch := false
			for _, mm := range msgs {
				if _, ok := mm.(boardFetchedMsg); ok {
					foundFetch = true
				}
			}
			if !foundFetch {
				t.Fatalf("skip's returned Cmd did not produce a boardFetchedMsg, want startupCmds()'s fetch to have run; got %#v", msgs)
			}

			after, err := os.ReadFile(fx.trustPath)
			if err != nil {
				t.Fatalf("failed to read trust store after keypress: %v", err)
			}
			if !bytes.Equal(before, after) {
				t.Error("trust store bytes changed after skip, want byte-identical (skip never writes)")
			}
		})
	}
}

func TestTrustConfirmMode_Skip_StartupWarningStillShown(t *testing.T) {
	identity := "trust-confirm-skip-warning"
	// A shell binding present -> this local config genuinely gets stripped,
	// so cfg.Notices is non-empty -- the exact scenario startupWarning
	// exists to surface, independent of the trust-confirm prompt itself.
	fx := newTrustConfirmFixture(t, trustReloadYAML,
		config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity}}})
	if len(fx.cfg.Notices) == 0 {
		t.Fatal("precondition: cfg.Notices is empty, want a strip notice from the untrusted shell binding")
	}

	b := newTrustConfirmBoard(t, fx, identity)
	// Mirrors main.go's seeding order: startupWarning is set before
	// trustConfirmEntry ever runs.
	b.startupWarning = strings.Join(fx.cfg.Notices, "; ")

	m, cmd := b.Update(keyMsg("s"))
	b = m.(Board)
	if b.mode != loadingMode {
		t.Fatalf("mode after skip = %d, want loadingMode", b.mode)
	}

	msgs := runCmd(cmd)
	var fetched boardFetchedMsg
	found := false
	for _, mm := range msgs {
		if bf, ok := mm.(boardFetchedMsg); ok {
			fetched = bf
			found = true
		}
	}
	if !found {
		t.Fatalf("skip's Cmd did not produce boardFetchedMsg; got %#v", msgs)
	}

	m, _ = b.Update(fetched)
	b = m.(Board)

	if b.statusBar.level != StatusWarning {
		t.Fatalf("statusBar.level = %v, want StatusWarning", b.statusBar.level)
	}
	if !strings.Contains(b.statusBar.message, "untrusted") {
		t.Fatalf("statusBar.message = %q, want it to contain the strip notice", b.statusBar.message)
	}
	if b.startupWarning != "" {
		t.Fatalf("startupWarning = %q, want cleared after being applied", b.startupWarning)
	}
}

// --- Accept ---

func TestTrustConfirmMode_Accept_HappyPath(t *testing.T) {
	identity := "trust-confirm-accept-happy"
	staleTrust := config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity, Note: "legacy note"}}}
	fx := newTrustConfirmFixture(t, trustReloadYAML, staleTrust)
	if len(fx.cfg.Notices) == 0 {
		t.Fatal("precondition: cfg.Notices is empty, want the shell binding/cleanup to have been stripped pre-accept")
	}
	b := newTrustConfirmBoard(t, fx, identity)

	// Precondition: the shell binding is genuinely stripped right now.
	if r := b.keys.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("z")}); r.Outcome == keymap.OutcomeMatch && r.Binding.Kind == keymap.BindingAction {
		t.Fatal("precondition: 'z' already resolves to an action binding before accept, want it stripped")
	}
	if len(b.columnConfigs) == 0 || b.columnConfigs[0].CleanupValue() != "" {
		t.Fatalf("precondition: columnConfigs[0].CleanupValue() = %q, want empty (stripped) before accept", b.columnConfigs[0].CleanupValue())
	}

	m, cmd := b.Update(keyMsg("t"))
	b = m.(Board)
	if b.mode != trustConfirmMode {
		t.Fatalf("mode immediately after 't' = %d, want trustConfirmMode (stays until the async result arrives)", b.mode)
	}

	msgs := runCmd(cmd)
	var accepted trustAcceptedMsg
	found := false
	for _, mm := range msgs {
		if a, ok := mm.(trustAcceptedMsg); ok {
			accepted = a
			found = true
		}
	}
	if !found {
		t.Fatalf("acceptTrustCmd did not produce trustAcceptedMsg; got %#v", msgs)
	}

	m, cmd = b.Update(accepted)
	b = m.(Board)

	// (a) exactly one trust entry for this identity, with the new hash and
	// the carried-forward note.
	newHash, err := config.HashLocalConfig(fx.localPath)
	if err != nil {
		t.Fatalf("config.HashLocalConfig() returned unexpected error: %v", err)
	}
	store, err := config.LoadTrust(fx.trustPath)
	if err != nil {
		t.Fatalf("config.LoadTrust() returned unexpected error: %v", err)
	}
	var matches []config.TrustEntry
	for _, e := range store.Trusted {
		if e.Path == identity {
			matches = append(matches, e)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("trust store has %d entries for identity %q, want exactly 1", len(matches), identity)
	}
	if matches[0].Hash != newHash {
		t.Errorf("trust entry Hash = %q, want the new content hash %q", matches[0].Hash, newHash)
	}
	if matches[0].Note != "legacy note" {
		t.Errorf("trust entry Note = %q, want the carried-forward %q", matches[0].Note, "legacy note")
	}

	// (b) the previously-stripped shell binding is now live.
	r := b.keys.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("z")})
	if r.Outcome != keymap.OutcomeMatch || r.Binding.Kind != keymap.BindingAction {
		t.Fatalf("Lookup(normal, z) after accept = %+v, want a resolved BindingAction", r)
	}

	// (c) the previously-stripped column cleanup is now live.
	if len(b.columnConfigs) == 0 || b.columnConfigs[0].CleanupValue() != "echo cleanup {number}" {
		t.Fatalf("columnConfigs[0].CleanupValue() after accept = %q, want %q", b.columnConfigs[0].CleanupValue(), "echo cleanup {number}")
	}

	// (d) no accidental repo retarget.
	if b.repoOwner != "owner-before" || b.repoName != "repo-before" {
		t.Fatalf("repoOwner/repoName after accept = %q/%q, want unchanged %q/%q", b.repoOwner, b.repoName, "owner-before", "repo-before")
	}

	// (e) proceeds to fetch.
	if b.mode != loadingMode {
		t.Fatalf("mode after trustAcceptedMsg = %d, want loadingMode", b.mode)
	}
	msgs2 := runCmd(cmd)
	foundFetch := false
	for _, mm := range msgs2 {
		if _, ok := mm.(boardFetchedMsg); ok {
			foundFetch = true
		}
	}
	if !foundFetch {
		t.Fatalf("handleTrustAccepted's Cmd did not produce boardFetchedMsg; got %#v", msgs2)
	}
}

func TestTrustConfirmMode_Accept_SaveTrustFailure_StaysInModeWithError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on windows")
	}

	identity := "trust-confirm-accept-save-failure"
	staleTrust := config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity}}}

	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	if err := os.WriteFile(localPath, []byte(trustReloadYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")
	cfg, err := config.Load(globalPath, localPath, staleTrust)
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}

	// trustPath's parent directory does not exist yet, and ITS parent is
	// read-only -- SaveTrust's own os.MkdirAll(dir, 0700) fails trying to
	// create it, before any write is attempted. LoadTrust(trustPath) itself
	// succeeds (missing file == "nothing trusted yet", not an error), so
	// this specifically exercises the SaveTrust failure branch.
	readonlyParent := filepath.Join(dir, "readonly-parent")
	if err := os.MkdirAll(readonlyParent, 0755); err != nil {
		t.Fatalf("failed to create readonly parent: %v", err)
	}
	trustPath := filepath.Join(readonlyParent, "trust-dir", "trust.yml")
	if err := os.Chmod(readonlyParent, 0500); err != nil {
		t.Fatalf("failed to chmod readonly parent: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(readonlyParent, 0700) })

	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, cfg.Columns, nil, "owner-before", "repo-before", "github", 0, 0, "Working", false, false, nil, nil, true)
	km, err := config.ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}
	b = b.withKeymap(km)
	b.trustPath = trustPath
	b.config.localPath = localPath

	state, ok := trustConfirmEntry(cfg, staleTrust, identity)
	if !ok {
		t.Fatal("precondition: trustConfirmEntry did not report a stale match")
	}
	b.mode = trustConfirmMode
	b.trustConfirm = state

	m, cmd := b.Update(keyMsg("t"))
	b = m.(Board)

	msgs := runCmd(cmd)
	var acceptErr trustAcceptErrorMsg
	found := false
	for _, mm := range msgs {
		if e, ok := mm.(trustAcceptErrorMsg); ok {
			acceptErr = e
			found = true
		}
	}
	if !found {
		t.Fatalf("acceptTrustCmd did not produce trustAcceptErrorMsg for an unwritable trust store; got %#v", msgs)
	}

	m, _ = b.Update(acceptErr)
	b = m.(Board)

	if b.mode != trustConfirmMode {
		t.Fatalf("mode after a SaveTrust failure = %d, want unchanged trustConfirmMode (board must not proceed half-reloaded)", b.mode)
	}
	if _, err := os.Stat(trustPath); err == nil {
		t.Error("trust store file exists after a failed SaveTrust, want it never created")
	}
	if b.statusBar.level != StatusError {
		t.Fatalf("statusBar.level = %v, want StatusError", b.statusBar.level)
	}
}

func TestTrustConfirmMode_Accept_MalformedTrustStore_NeverRewritten(t *testing.T) {
	identity := "trust-confirm-accept-malformed"
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	if err := os.WriteFile(localPath, []byte(trustReloadYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")
	staleTrust := config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity}}}
	cfg, err := config.Load(globalPath, localPath, staleTrust)
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}

	trustPath := filepath.Join(dir, "trust.yml")
	garbage := []byte("trusted: [unclosed\n")
	if err := os.WriteFile(trustPath, garbage, 0600); err != nil {
		t.Fatalf("failed to write malformed trust store: %v", err)
	}

	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, cfg.Columns, nil, "owner-before", "repo-before", "github", 0, 0, "Working", false, false, nil, nil, true)
	km, err := config.ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}
	b = b.withKeymap(km)
	b.trustPath = trustPath
	b.config.localPath = localPath

	state, ok := trustConfirmEntry(cfg, staleTrust, identity)
	if !ok {
		t.Fatal("precondition: trustConfirmEntry did not report a stale match")
	}
	b.mode = trustConfirmMode
	b.trustConfirm = state

	m, cmd := b.Update(keyMsg("t"))
	b = m.(Board)

	msgs := runCmd(cmd)
	found := false
	for _, mm := range msgs {
		if _, ok := mm.(trustAcceptErrorMsg); ok {
			found = true
		}
	}
	if !found {
		t.Fatalf("acceptTrustCmd did not produce trustAcceptErrorMsg for a malformed trust store; got %#v", msgs)
	}

	after, err := os.ReadFile(trustPath)
	if err != nil {
		t.Fatalf("failed to read trust store after accept attempt: %v", err)
	}
	if !bytes.Equal(garbage, after) {
		t.Errorf("trust store bytes changed after a failed LoadTrust, want byte-identical; got %q, want %q", after, garbage)
	}
}

// --- View rendering ---

func TestTrustConfirmMode_View_SanitizesHostileNote(t *testing.T) {
	hostileNote := "legacy\n\x1b[31mHACKED\x1b[0m \u202eRTL\x07"
	b := newTestBoard(t)
	b.Width, b.Height = 120, 40
	b.mode = trustConfirmMode
	b.trustConfirm = trustConfirmState{hash: "sha256:new", identity: "id", note: hostileNote}

	view := b.View()
	promptLine := findLineContaining(t, view, "previously trusted as")

	if strings.ContainsRune(promptLine, '\x1b') {
		t.Errorf("trust-confirm prompt line = %q, want no ESC (0x1b) byte", promptLine)
	}
	if strings.ContainsRune(promptLine, '\x07') {
		t.Errorf("trust-confirm prompt line = %q, want no BEL (0x07) byte", promptLine)
	}
	if strings.ContainsRune(promptLine, '‮') {
		t.Errorf("trust-confirm prompt line = %q, want no raw bidi-override rune", promptLine)
	}
	if !strings.Contains(promptLine, "legacy HACKED RTL") {
		t.Errorf("trust-confirm prompt line = %q, want it to contain the flattened, sanitized note %q", promptLine, "legacy HACKED RTL")
	}
}

func TestTrustConfirmMode_View_NoNote_OmitsNotePhrase(t *testing.T) {
	b := newTestBoard(t)
	b.Width, b.Height = 120, 40
	b.mode = trustConfirmMode
	b.trustConfirm = trustConfirmState{hash: "sha256:new", identity: "id", note: ""}

	view := b.View()
	if strings.Contains(view, "previously trusted as") {
		t.Errorf("view contains a note phrase with no note set: %q", view)
	}
	if !strings.Contains(view, ".lazyboards.yml changed since you trusted it") {
		t.Errorf("view = %q, want the base prompt text present", view)
	}
}

// --- promptParenthetical / hint<->dispatch invariant ---

func TestTrustConfirmMode_PromptParenthetical_DefaultAndRemap(t *testing.T) {
	identity := "trust-confirm-prompt-default"
	fx := newTrustConfirmFixture(t, "provider: github\nrepo: owner/repo\n",
		config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity}}})
	b := newTrustConfirmBoard(t, fx, identity)

	if !strings.Contains(b.View(), "(t/s)") {
		t.Errorf("default trustConfirmMode prompt = %q, want it to contain %q", b.View(), "(t/s)")
	}

	identity2 := "trust-confirm-prompt-remap"
	remapYAML := "provider: github\nrepo: owner/repo\nkeymaps:\n  trust_confirm:\n    t: ~\n    y: trust_confirm.trust\n"
	fx2 := newTrustConfirmFixture(t, remapYAML,
		config.Trust{Trusted: []config.TrustEntry{{Hash: "sha256:stale", Path: identity2}}})
	b2 := newTrustConfirmBoard(t, fx2, identity2)

	if !strings.Contains(b2.View(), "(y/s)") {
		t.Errorf("remapped trustConfirmMode prompt = %q, want it to contain %q", b2.View(), "(y/s)")
	}

	// The old key 't' is now unbound and must no-op; the new key 'y' must
	// dispatch trust_confirm.trust (proving the rendered hint and the
	// actual dispatch table agree).
	m, _ := b2.Update(keyMsg("t"))
	afterOldKey := m.(Board)
	if afterOldKey.mode != trustConfirmMode {
		t.Fatalf("mode after unbound 't' = %d, want unchanged trustConfirmMode", afterOldKey.mode)
	}

	m, cmd := b2.Update(keyMsg("y"))
	afterNewKey := m.(Board)
	if afterNewKey.mode != trustConfirmMode {
		t.Fatalf("mode immediately after 'y' = %d, want trustConfirmMode (accept stays until the async result)", afterNewKey.mode)
	}
	if cmd == nil {
		t.Fatal("'y' (remapped to trust_confirm.trust) returned a nil Cmd, want acceptTrustCmd's Cmd")
	}
}
