package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- #567: thread the trust decision into config.Load and strip the two
// keystroke-triggered local-origin shell sinks (inline keymaps: actions and
// keymaps: shell bindings and cleanup:) at the points in the pipeline where
// provenance still exists. ---
//
// This is the RED-phase file for #567: it targets the new three-argument
// config.Load(globalPath, localPath string, trust Trust) signature, which
// does not exist yet (Load is still two-argument as of #566). The package
// will not compile until Phase 4 lands that signature change -- that
// compile failure IS the expected red state here, and it doubles as AC16's
// proof that no call site can silently omit the trust argument: every
// caller, including every helper below, must pass one explicitly.
//
// Every assertion in this file goes through Load -> ResolveKeymap ->
// km.Lookup (or through cfg's own exported accessors), never against
// internal map contents, mirroring shipped_config_test.go and
// legacy_restore_test.go's conventions.

// writeTempConfigs writes globalYAML/localYAML (if non-empty) to temp files
// and returns their paths, pointing at a nonexistent path for whichever side
// is empty -- mirroring loadConfigFromStrings' (helpers_test.go)
// missing-file convention, but stopping short of calling Load itself so
// callers can compute a trust hash from the written local file first.
func writeTempConfigs(t *testing.T, globalYAML, localYAML string) (globalPath, localPath string) {
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

// mustHashLocal hashes the local config file at path via the real
// HashLocalConfig, so a "trusted" test case never hand-copies a hash
// literal that could drift from what Load itself computes.
func mustHashLocal(t *testing.T, path string) string {
	t.Helper()
	hash, err := HashLocalConfig(path)
	if err != nil {
		t.Fatalf("HashLocalConfig(%q) error: %v", path, err)
	}
	return hash
}

// assertIdenticalLookup fails the test if a and b disagree on the resolved
// outcome/binding for (mode, column, seq).
func assertIdenticalLookup(t *testing.T, a, b *keymap.Keymap, mode keymap.Mode, column string, seq keymap.Sequence) {
	t.Helper()
	ra := a.Lookup(mode, column, seq)
	rb := b.Lookup(mode, column, seq)
	if ra.Outcome != rb.Outcome || ra.Binding != rb.Binding {
		t.Fatalf("Lookup(%v, %q, %v) diverged: %+v vs %+v", mode, column, seq, ra, rb)
	}
}

// --- AC1: no local .lazyboards.yml present -> behavior unchanged, and
// LocalHash stays empty since no local file was ever read or hashed. ---

func TestLoad_NoLocalConfig_BehavesExactlyAsToday(t *testing.T) {
	globalYAML := `
keymaps:
  normal:
    z: { name: Global Shell, type: shell, command: "echo global" }
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, "")

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if cfg.LocalHash != "" {
		t.Fatalf("cfg.LocalHash = %q, want empty (no local file was read)", cfg.LocalHash)
	}

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("z")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Type != "shell" {
		t.Fatalf("Lookup(normal, \"\", z) = %+v, want the global shell action untouched (no local file exists to strip anything from)", result)
	}
}

// --- AC2: local config declares no shell binding (only type: url + a
// command-id binding, the shape of the first-launch file lazyboards writes
// itself) -> untrusted and trusted loads must resolve byte-identically. ---

func TestLoad_LocalDeclaresNoShellBinding_UntrustedMatchesTrusted(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
keymaps:
  normal:
    u: { name: Open Issue, type: url, url: "https://example.com/{number}" }
    y: app.help
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	untrusted, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() (untrusted) returned unexpected error: %v", err)
	}
	trusted, err := Load(globalPath, localPath, Trust{Trusted: []TrustEntry{{Hash: mustHashLocal(t, localPath)}}})
	if err != nil {
		t.Fatalf("Load() (trusted) returned unexpected error: %v", err)
	}

	kmUntrusted, err := ResolveKeymap(&untrusted)
	if err != nil {
		t.Fatalf("ResolveKeymap() (untrusted) returned unexpected error: %v", err)
	}
	kmTrusted, err := ResolveKeymap(&trusted)
	if err != nil {
		t.Fatalf("ResolveKeymap() (trusted) returned unexpected error: %v", err)
	}

	for _, key := range []string{"u", "y"} {
		assertIdenticalLookup(t, kmUntrusted, kmTrusted, keymap.ModeNormal, "", keymap.Sequence{keymap.Key(key)})
	}
}

// --- AC5: untrusted local keymaps.normal/keymaps.columns.<name> shell
// bindings are absent from the resolved keymap, falling back to the
// matching global binding when global declares the key, or to the built-in
// default otherwise. ---

func TestLoad_Untrusted_LocalNormalShellFallsBackToGlobalBinding(t *testing.T) {
	globalYAML := `
keymaps:
  normal:
    b: app.help
`
	localYAML := `
provider: github
repo: owner/repo
keymaps:
  normal:
    b: { name: Evil, type: shell, command: "rm -rf /" }
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingCommand || result.Binding.Command != keymap.CommandHelp {
		t.Fatalf("Lookup(normal, \"\", b) = %+v, want fallback to global app.help (untrusted local shell override must be stripped)", result)
	}
}

func TestLoad_Untrusted_LocalNormalShellFallsBackToBuiltinDefaultWhenGlobalSilent(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
keymaps:
  normal:
    q: { name: Evil, type: shell, command: "rm -rf /" }
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("q")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingCommand || result.Binding.Command != keymap.CommandQuit {
		t.Fatalf("Lookup(normal, \"\", q) = %+v, want the built-in default app.quit (untrusted local shell override on a built-in key must be stripped, with no global entry to fall back to)", result)
	}
}

func TestLoad_Untrusted_LocalColumnShellFallsBackToGlobalColumnBinding(t *testing.T) {
	globalYAML := `
keymaps:
  columns:
    Refined:
      b: app.help
`
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
keymaps:
  columns:
    Refined:
      b: { name: Evil, type: shell, command: "rm -rf /" }
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	result := km.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("b")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingCommand || result.Binding.Command != keymap.CommandHelp {
		t.Fatalf("Lookup(normal, Refined, b) = %+v, want fallback to global column app.help (untrusted local column shell override must be stripped)", result)
	}
}

func TestLoad_Untrusted_LocalColumnShellFallsBackToBuiltinDefaultWhenGlobalSilent(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
keymaps:
  columns:
    Refined:
      q: { name: Evil, type: shell, command: "rm -rf /" }
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	result := km.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("q")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingCommand || result.Binding.Command != keymap.CommandQuit {
		t.Fatalf("Lookup(normal, Refined, q) = %+v, want the built-in default app.quit (untrusted local column shell override on a built-in key must be stripped, with no global column entry to fall back to)", result)
	}
}

// Q2: when stripping empties a local mode/column table entirely, the whole
// table must be deleted (not left as an explicit-but-empty table), so
// mergeKeymaps' existing nil-vs-empty distinction treats the mode/column as
// "not locally declared" and inherits every other global key too -- not
// just the stripped key falling back, but a key the local document never
// even mentioned surviving. Leaving an explicit-but-empty table would (per
// mergeKeymaps' current documented behavior)
// mean "no bindings at all", silently dropping that untouched global key.

func TestLoad_Untrusted_EmptiedLocalModeTableStillInheritsOtherGlobalKeys(t *testing.T) {
	globalYAML := `
keymaps:
  normal:
    b: app.help
    z: card.delete
`
	localYAML := `
provider: github
repo: owner/repo
keymaps:
  normal:
    b: { name: Evil, type: shell, command: "rm -rf /" }
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	resultB := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	if resultB.Outcome != keymap.OutcomeMatch || resultB.Binding.Kind != keymap.BindingCommand || resultB.Binding.Command != keymap.CommandHelp {
		t.Fatalf("Lookup(normal, \"\", b) = %+v, want fallback to global app.help", resultB)
	}
	resultZ := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("z")})
	if resultZ.Outcome != keymap.OutcomeMatch || resultZ.Binding.Kind != keymap.BindingCommand || resultZ.Binding.Command != keymap.CommandCardDelete {
		t.Fatalf("Lookup(normal, \"\", z) = %+v, want global card.delete to survive: stripping b (the local table's only entry) must delete the whole local keymaps.normal table so mergeKeymaps inherits the FULL global table, not leave it present-but-empty (which would block z's inheritance too)", resultZ)
	}
}

func TestLoad_Untrusted_EmptiedLocalColumnTableStillInheritsOtherGlobalColumnKeys(t *testing.T) {
	globalYAML := `
keymaps:
  columns:
    Refined:
      b: app.help
      z: card.delete
`
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
keymaps:
  columns:
    Refined:
      b: { name: Evil, type: shell, command: "rm -rf /" }
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	resultB := km.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("b")})
	if resultB.Outcome != keymap.OutcomeMatch || resultB.Binding.Kind != keymap.BindingCommand || resultB.Binding.Command != keymap.CommandHelp {
		t.Fatalf("Lookup(normal, Refined, b) = %+v, want fallback to global column app.help", resultB)
	}
	resultZ := km.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("z")})
	if resultZ.Outcome != keymap.OutcomeMatch || resultZ.Binding.Kind != keymap.BindingCommand || resultZ.Binding.Command != keymap.CommandCardDelete {
		t.Fatalf("Lookup(normal, Refined, z) = %+v, want global column card.delete to survive: stripping b must delete the whole local keymaps.columns.Refined table, not leave it present-but-empty", resultZ)
	}
}

// --- AC5 (trusted matrix): a config trusted by hash keeps every local
// shell binding, at both mode and column scope -- the untrusted load of the
// exact same bytes must have stripped them. ---

func TestLoad_Trusted_LocalShellBindingsResolveUnstripped(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
keymaps:
  normal:
    b: { name: Custom Shell, type: shell, command: "echo hi" }
  columns:
    Refined:
      c: { name: Column Shell, type: shell, command: "echo col" }
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)
	trust := Trust{Trusted: []TrustEntry{{Hash: mustHashLocal(t, localPath)}}}

	trusted, err := Load(globalPath, localPath, trust)
	if err != nil {
		t.Fatalf("Load() (trusted) returned unexpected error: %v", err)
	}
	kmTrusted, err := ResolveKeymap(&trusted)
	if err != nil {
		t.Fatalf("ResolveKeymap() (trusted) returned unexpected error: %v", err)
	}

	result := kmTrusted.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	if result.Outcome != keymap.OutcomeMatch || result.Binding.Kind != keymap.BindingAction || result.Binding.Action.Type != "shell" || result.Binding.Action.Command != "echo hi" {
		t.Fatalf("Lookup(normal, \"\", b) = %+v, want the trusted local shell binding to resolve unstripped", result)
	}
	resultCol := kmTrusted.Lookup(keymap.ModeNormal, "Refined", keymap.Sequence{keymap.Key("c")})
	if resultCol.Outcome != keymap.OutcomeMatch || resultCol.Binding.Kind != keymap.BindingAction || resultCol.Binding.Action.Command != "echo col" {
		t.Fatalf("Lookup(normal, Refined, c) = %+v, want the trusted local column shell binding to resolve unstripped", resultCol)
	}

	untrusted, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() (untrusted) returned unexpected error: %v", err)
	}
	kmUntrusted, err := ResolveKeymap(&untrusted)
	if err != nil {
		t.Fatalf("ResolveKeymap() (untrusted) returned unexpected error: %v", err)
	}
	resultUntrusted := kmUntrusted.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	if resultUntrusted.Outcome == keymap.OutcomeMatch && resultUntrusted.Binding.Kind == keymap.BindingAction {
		t.Fatalf("Lookup(normal, \"\", b) (untrusted load of the SAME bytes) = %+v, want the shell binding stripped -- trust must gate on the hash, not always allow", resultUntrusted)
	}
}

// --- AC8: untrusted local type: url bindings, bindings to catalogued
// built-in command ids, and every non-executing field (repo:, provider:,
// columns:, sort_order:, cleanup:) still apply -- the board stays fully
// usable. cleanup: is explicitly in scope for #569 (3/4), not this PR, so
// it must survive here untouched. ---

func TestLoad_Untrusted_NonShellSinksAndNonExecutingFieldsSurvive(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
sort_order: newest
cleanup: "echo cleanup survives"
columns:
  - name: Alpha
keymaps:
  normal:
    u: { name: Open Issue, type: url, url: "https://example.com/{number}" }
    y: app.help
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if cfg.Provider != "github" {
		t.Fatalf("cfg.Provider = %q, want %q", cfg.Provider, "github")
	}
	if cfg.Repo != "owner/repo" {
		t.Fatalf("cfg.Repo = %q, want %q", cfg.Repo, "owner/repo")
	}
	if !cfg.SortNewestFirstValue() {
		t.Fatalf("cfg.SortNewestFirstValue() = false, want true (sort_order: newest must still apply)")
	}
	// cleanup: IS in scope for #569 (3/4): it's the zero-keystroke shell sink
	// this ticket closes, so an untrusted local cleanup: with no global
	// fallback must now resolve empty, not survive (inverted from #567/#568's
	// "out of scope" fixture).
	if got := cfg.CleanupValue(); got != "" {
		t.Fatalf("cfg.CleanupValue() = %q, want empty: #569 strips untrusted local cleanup: with no global fallback", got)
	}
	if len(cfg.Columns) != 1 || cfg.Columns[0].Name != "Alpha" {
		t.Fatalf("cfg.Columns = %+v, want a single %q column", cfg.Columns, "Alpha")
	}

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	resultURL := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("u")})
	if resultURL.Outcome != keymap.OutcomeMatch || resultURL.Binding.Kind != keymap.BindingAction || resultURL.Binding.Action.Type != "url" {
		t.Fatalf("Lookup(normal, \"\", u) = %+v, want the untrusted local type: url binding to still apply", resultURL)
	}
	resultCmd := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("y")})
	if resultCmd.Outcome != keymap.OutcomeMatch || resultCmd.Binding.Kind != keymap.BindingCommand || resultCmd.Binding.Command != keymap.CommandHelp {
		t.Fatalf("Lookup(normal, \"\", y) = %+v, want the untrusted local built-in command-id binding to still apply", resultCmd)
	}
}

// --- AC9: global-config shell actions are never stripped, whatever the
// local config's trust state. ---

func TestLoad_GlobalShellBindingsNeverStrippedRegardlessOfTrust(t *testing.T) {
	globalYAML := `
keymaps:
  normal:
    b: { name: Global Inline Shell, type: shell, command: "echo global-inline" }
`
	localYAML := `
provider: github
repo: owner/repo
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cases := []struct {
		name  string
		trust Trust
	}{
		{"untrusted", Trust{}},
		{"trusted", Trust{Trusted: []TrustEntry{{Hash: mustHashLocal(t, localPath)}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := Load(globalPath, localPath, tc.trust)
			if err != nil {
				t.Fatalf("Load() returned unexpected error: %v", err)
			}
			km, err := ResolveKeymap(&cfg)
			if err != nil {
				t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
			}

			resultInline := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
			if resultInline.Outcome != keymap.OutcomeMatch || resultInline.Binding.Kind != keymap.BindingAction || resultInline.Binding.Action.Type != "shell" {
				t.Fatalf("Lookup(normal, \"\", b) = %+v, want the global inline shell action to resolve regardless of local trust state", resultInline)
			}
		})
	}
}

// --- AC16: Load's signature makes the trust decision an explicit argument.
// The zero-value Trust{} (never a hand-populated store) trusts nothing, so
// a shell-bearing local config loaded with it must be stripped. ---

func TestLoad_ZeroValueTrust_TrustsNothingAndStripsLocalShellSink(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
keymaps:
  normal:
    b: { name: Evil, type: shell, command: "rm -rf /" }
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	var zeroTrust Trust // the zero value -- must trust nothing
	cfg, err := Load(globalPath, localPath, zeroTrust)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}
	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}

	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("b")})
	if result.Outcome == keymap.OutcomeMatch && result.Binding.Kind == keymap.BindingAction {
		t.Fatalf("Lookup(normal, \"\", b) = %+v, want the shell sink stripped: Load's zero-value Trust argument must trust nothing (AC16)", result)
	}
}

// --- CRITICAL (security review): YAML merge keys (`<<: *anchor`) let a
// local document smuggle a shell action into the decoded cfg.Actions/
// cfg.Columns without the smuggled key ever appearing as a literal mapping
// key in the raw yaml.Node tree stampOrderAndRejectLegacy walks. cfg.Keymaps and
// cfg.Columns have no custom UnmarshalYAML (unlike *Keymaps), so yaml.v3's
// own generic decoder -- which DOES resolve merge keys -- populates them
// directly off Config's top-level fields; the raw-node walk's hand-rolled
// walk over docNode.Content never sees the merged-in key, so
// decls.ActionKeys/decls.HasColumns silently disagree with what actually
// landed in cfg.Actions/cfg.Columns. Gating the strip decision on that
// stale provenance snapshot let a merge-key-smuggled shell action survive
// all the way to ResolveKeymap -> execution. ---

// TestLoad_Untrusted_MergeKeyInsideKeymapsTableIsNotABypass confirms that
// keymaps.go's hand-rolled parsing does not silently expand a merge key into
// a live keymap binding: *Keymaps has a custom UnmarshalYAML
// (Keymaps.UnmarshalYAML, keymaps.go) whose parseKeymapTable walks
// node.Content by hand instead of letting yaml.v3's generic map decoder run
// -- the generic decoder is the only place merge-key *table* expansion
// (splicing the aliased mapping's own keys in as siblings) happens, so
// parseKeymapTable never does that. It still resolves "<<" as one literal
// key whose value node is the alias, and yaml.v3's Node.Decode does resolve
// an AliasNode before dispatching to Unmarshaler -- so here the alias
// resolves to `{b: {name: Evil, type: shell, command: "rm -rf /"}}`, which
// KeymapBinding.UnmarshalYAML's mapping branch decodes into an Action. Action
// has no "b" field, so every field decodes to its zero value (Type == ""),
// producing a syntactically-valid but semantically-empty BindingAction under
// key "<<" -- never a live shell binding. That zero-value Action is never
// expanded into a live binding at all: it fails validateKeymapActions's
// validateActionValue check (Name is checked before Type, and both are ""
// here, so the load fails on the "name is required" branch) before the
// resolved keymap ever exists, so the load fails loudly instead of silently
// letting a shell binding through.
func TestLoad_Untrusted_MergeKeyInsideKeymapsTableIsNotABypass(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
.x: &x
  b: {name: Evil, type: shell, command: "rm -rf /"}
keymaps:
  normal:
    <<: *x
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	_, err := Load(globalPath, localPath, Trust{})
	if err == nil {
		t.Fatalf("Load() succeeded for a merge key inside keymaps.normal; want an error")
	}
	const wantSubstr = "name is required"
	if !strings.Contains(err.Error(), wantSubstr) {
		t.Fatalf("Load() error = %q, want it to contain %q -- the merge key's aliased value decodes into a zero-value Action (never a live shell binding), so it must fail validateActionValue's validation, not some earlier structural parse error", err.Error(), wantSubstr)
	}
}

// TestLoad_Untrusted_MergeKeyProducingLiveShellBindingInKeymapsIsStripped is
// the sharper companion to the case above: here the anchored mapping *is*
// itself a well-formed action definition (name/type/command as its own
// top-level keys, not nested under an extra "b" key), so the alias resolves
// directly into a live `KeymapBinding{Kind: BindingAction, Action:
// Action{Type: "shell", ...}}` under the literal key "<<" -- a genuine
// merge-key bypass attempt that produces a real shell binding, not a
// validation error. stripShellFromKeymapTable's stripShellBindings strips by
// Kind/Action.Type alone, never by key name, so it must still catch and
// remove this binding regardless of its unusual "<<" key.
func TestLoad_Untrusted_MergeKeyProducingLiveShellBindingInKeymapsIsStripped(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
.x: &x
  name: Evil
  type: shell
  command: "rm -rf /"
keymaps:
  normal:
    <<: *x
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	km, err := ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("ResolveKeymap() returned unexpected error: %v", err)
	}
	result := km.Lookup(keymap.ModeNormal, "", keymap.Sequence{keymap.Key("<<")})
	if result.Outcome == keymap.OutcomeMatch && result.Binding.Kind == keymap.BindingAction {
		t.Fatalf("Lookup(normal, \"\", \"<<\") = %+v, want no live shell binding to survive untrusted stripping", result)
	}
}

// --- LOW (security review): stripping the shell entries out of a column's
// actions map must only reset the map to nil (\"inherit global\") when the
// strip pass actually deleted something. An untrusted local column that
// already declared an explicit, empty actions: {} (no shell entries to
// strip at all) must stay explicit-empty after the strip pass touches it --
// resetting it to nil purely because its post-strip length happens to be
// zero would silently flip "explicit empty, no actions" into "nil, inherit
// global", diverging from the trusted load of the identical bytes. ---

// --- #569 (3/4): close the last local-origin shell sink -- cleanup:/
// columns[].cleanup -- and make the whole strip mechanism visible via
// Config.Notices. ---

// --- AC7: untrusted local top-level cleanup: and columns[].cleanup are
// stripped when there is no matching global fallback. ---

func TestLoad_Untrusted_TopLevelCleanupStripped(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
cleanup: "rm -rf /"
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if got := cfg.CleanupValue(); got != "" {
		t.Fatalf("cfg.CleanupValue() = %q, want empty: untrusted local top-level cleanup: must be stripped with no global fallback", got)
	}
}

func TestLoad_Untrusted_ColumnCleanupStripped(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
    cleanup: "rm -rf /"
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Columns) != 1 {
		t.Fatalf("cfg.Columns = %+v, want a single Refined column", cfg.Columns)
	}
	if got := cfg.Columns[0].CleanupValue(); got != "" {
		t.Fatalf("cfg.Columns[0].CleanupValue() = %q, want empty: untrusted local columns[].cleanup must be stripped with no global fallback", got)
	}
}

// --- AC9 (cleanup scope): global cleanup: is never stripped and must be the
// resolved value after a local strip -- the highest-risk case, proving
// globalCleanup is a value-copied snapshot rather than a pointer alias that
// yaml.v3's second Unmarshal silently overwrote. ---

func TestLoad_Untrusted_GlobalTopLevelCleanupSurvivesLocalStrip(t *testing.T) {
	globalYAML := `
cleanup: "echo global-safe"
`
	localYAML := `
provider: github
repo: owner/repo
cleanup: "rm -rf /"
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if got := cfg.CleanupValue(); got != "echo global-safe" {
		t.Fatalf("cfg.CleanupValue() = %q, want the global cleanup to resolve after the untrusted local override is stripped (proves globalCleanup is a value-copied snapshot, not a pointer alias yaml.v3 silently overwrote)", got)
	}
}

func TestLoad_Untrusted_GlobalColumnCleanupSurvives_LocalDeclaresColumns(t *testing.T) {
	globalYAML := `
columns:
  - name: Refined
    cleanup: "echo global-refined"
`
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
    cleanup: "rm -rf /"
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Columns) != 1 {
		t.Fatalf("cfg.Columns = %+v, want a single Refined column", cfg.Columns)
	}
	if got := cfg.Columns[0].CleanupValue(); got != "echo global-refined" {
		t.Fatalf("cfg.Columns[0].CleanupValue() = %q, want the global column cleanup to resolve after the untrusted local column override is stripped", got)
	}
}

// TestLoad_Untrusted_GlobalColumnCleanupSurvives_LocalDeclaresNoColumns is the
// aliasing trap: when the local document declares no columns: block at all,
// cfg.Columns still IS globalColumns (same backing array) at strip time. An
// unconditional nil-out of cfg.Columns[i].Cleanup would blank the aliased
// global column's cleanup instead of a genuinely local one (AC9 violation).
func TestLoad_Untrusted_GlobalColumnCleanupSurvives_LocalDeclaresNoColumns(t *testing.T) {
	globalYAML := `
columns:
  - name: Refined
    cleanup: "echo global-refined"
`
	localYAML := `
provider: github
repo: owner/repo
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Columns) != 1 || cfg.Columns[0].Name != "Refined" {
		t.Fatalf("cfg.Columns = %+v, want the global Refined column inherited (cfg.Columns aliases globalColumns here since local declares no columns: block at all)", cfg.Columns)
	}
	if got := cfg.Columns[0].CleanupValue(); got != "echo global-refined" {
		t.Fatalf("cfg.Columns[0].CleanupValue() = %q, want the global column cleanup untouched -- an unconditional nil-out here would blank the aliased global column's cleanup, not just a stripped local one", got)
	}
}

// TestLoad_Untrusted_NoGlobalCleanup_EveryColumnResolvesEmpty covers the case
// where neither a top-level nor any per-column global cleanup exists at all:
// every column (whether it declared its own untrusted override or not) must
// resolve to "".
func TestLoad_Untrusted_NoGlobalCleanup_EveryColumnResolvesEmpty(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
cleanup: "rm -rf /"
columns:
  - name: Alpha
  - name: Beta
    cleanup: "rm -rf /beta"
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if got := cfg.CleanupValue(); got != "" {
		t.Fatalf("cfg.CleanupValue() = %q, want empty: no global cleanup to fall back to", got)
	}
	for _, col := range cfg.Columns {
		if got := col.CleanupValue(); got != "" {
			t.Fatalf("cfg.Columns[%q].CleanupValue() = %q, want empty: with no global cleanup at any level, every column must resolve to \"\"", col.Name, got)
		}
	}
}

// --- AC10 (trusted load unchanged): a trusted local cleanup override
// resolves exactly as authored, at both top level and per-column, with no
// notice. ---

func TestLoad_Trusted_CleanupResolvesUnstripped(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
cleanup: "echo trusted-top"
columns:
  - name: Refined
    cleanup: "echo trusted-col"
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)
	trust := Trust{Trusted: []TrustEntry{{Hash: mustHashLocal(t, localPath)}}}

	cfg, err := Load(globalPath, localPath, trust)
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if got := cfg.CleanupValue(); got != "echo trusted-top" {
		t.Fatalf("cfg.CleanupValue() = %q, want the trusted local top-level cleanup to resolve unstripped", got)
	}
	if len(cfg.Columns) != 1 || cfg.Columns[0].CleanupValue() != "echo trusted-col" {
		t.Fatalf("cfg.Columns = %+v, want the trusted local column cleanup to resolve unstripped", cfg.Columns)
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty for a trusted load", cfg.Notices)
	}
}

// --- Spurious-notice guard: a local cleanup byte-identical to global's is
// not a strip, so it must neither change the resolved value nor raise a
// notice. ---

func TestLoad_Untrusted_LocalCleanupByteIdenticalToGlobal_NoStripNoNotice(t *testing.T) {
	globalYAML := `
cleanup: "echo same"
`
	localYAML := `
provider: github
repo: owner/repo
cleanup: "echo same"
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if got := cfg.CleanupValue(); got != "echo same" {
		t.Fatalf("cfg.CleanupValue() = %q, want %q", got, "echo same")
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: a local cleanup byte-identical to global's is not a strip, so it must not raise a notice either", cfg.Notices)
	}
}

// --- Q1: an explicit cleanup: "" (a disable directive, never executing) is
// left alone and must never be stripped/restored to the global value, at
// both top level and per-column. ---

func TestLoad_Untrusted_EmptyTopLevelCleanupLeftAlone(t *testing.T) {
	globalYAML := `
cleanup: "echo global-safe"
`
	localYAML := `
provider: github
repo: owner/repo
cleanup: ""
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if got := cfg.CleanupValue(); got != "" {
		t.Fatalf("cfg.CleanupValue() = %q, want empty: an explicit local cleanup: \"\" is a disable directive, never stripped -- stripping it would restore the global command, making an untrusted load run STRICTLY MORE commands than a trusted load of the same bytes", got)
	}
	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty: cleanup: \"\" is left alone, not a strip", cfg.Notices)
	}
}

func TestLoad_Untrusted_EmptyColumnCleanupLeftAlone(t *testing.T) {
	globalYAML := `
columns:
  - name: Refined
    cleanup: "echo global-refined"
`
	localYAML := `
provider: github
repo: owner/repo
columns:
  - name: Refined
    cleanup: ""
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Columns) != 1 {
		t.Fatalf("cfg.Columns = %+v, want a single Refined column", cfg.Columns)
	}
	if got := cfg.Columns[0].CleanupValue(); got != "" {
		t.Fatalf("cfg.Columns[0].CleanupValue() = %q, want empty: an explicit local columns[].cleanup: \"\" disables cleanup and must never be stripped/restored to the global value", got)
	}
}

// --- AC11: Config carries a notice entry exactly when at least one local
// sink was stripped (keymap binding or cleanup), always naming the literal
// ".lazyboards.yml" (never the local path argument passed to Load, per Q2)
// and the exact `lazyboards trust` invocation; absent when nothing was
// stripped. ---

func TestLoad_Untrusted_CleanupStripEmitsSingleNoticeNamingLocalConfigFile(t *testing.T) {
	globalYAML := `
cleanup: "echo global-safe"
`
	localYAML := `
provider: github
repo: owner/repo
cleanup: "rm -rf /"
`
	globalPath, localPath := writeTempConfigs(t, globalYAML, localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Notices) != 1 {
		t.Fatalf("cfg.Notices = %v, want exactly one notice for a single stripped cleanup: sink", cfg.Notices)
	}
	notice := cfg.Notices[0]
	if !strings.Contains(notice, ".lazyboards.yml") {
		t.Fatalf("cfg.Notices[0] = %q, want it to name the literal \".lazyboards.yml\", never the (possibly long/temp) localPath argument passed to Load", notice)
	}
	if !strings.Contains(notice, "lazyboards trust") {
		t.Fatalf("cfg.Notices[0] = %q, want it to name the exact `lazyboards trust` invocation", notice)
	}
	if !strings.Contains(notice, "cleanup") {
		t.Fatalf("cfg.Notices[0] = %q, want it to name the stripped sink kind (\"cleanup\")", notice)
	}
}

func TestLoad_NothingStripped_NoNotice(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Notices) != 0 {
		t.Fatalf("cfg.Notices = %v, want empty when nothing was stripped", cfg.Notices)
	}
}

func TestLoad_Untrusted_KeymapOnlyStripEmitsSingleNotice(t *testing.T) {
	localYAML := `
provider: github
repo: owner/repo
keymaps:
  normal:
    q: { name: Evil, type: shell, command: "rm -rf /" }
`
	globalPath, localPath := writeTempConfigs(t, "", localYAML)

	cfg, err := Load(globalPath, localPath, Trust{})
	if err != nil {
		t.Fatalf("Load() returned unexpected error: %v", err)
	}

	if len(cfg.Notices) != 1 {
		t.Fatalf("cfg.Notices = %v, want exactly one notice for a keymap-only strip (AC11: keymap binding or cleanup)", cfg.Notices)
	}
	if !strings.Contains(cfg.Notices[0], ".lazyboards.yml") {
		t.Fatalf("cfg.Notices[0] = %q, want it to name the literal \".lazyboards.yml\"", cfg.Notices[0])
	}
}
