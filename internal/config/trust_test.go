package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
)

// --- HashLocalConfig (AC3) ---

func TestHashLocalConfig_ByteIdenticalFilesAtDifferentPaths_HashIdentically(t *testing.T) {
	dir := t.TempDir()
	pathA := filepath.Join(dir, "a", "config.yml")
	pathB := filepath.Join(dir, "b", "config.yml")
	if err := os.MkdirAll(filepath.Dir(pathA), 0700); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(pathB), 0700); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	content := []byte("columns:\n  - name: Todo\n")
	if err := os.WriteFile(pathA, content, 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := os.WriteFile(pathB, content, 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	hashA, err := HashLocalConfig(pathA)
	if err != nil {
		t.Fatalf("HashLocalConfig(pathA) returned error: %v", err)
	}
	hashB, err := HashLocalConfig(pathB)
	if err != nil {
		t.Fatalf("HashLocalConfig(pathB) returned error: %v", err)
	}

	if hashA != hashB {
		t.Errorf("hashA = %q, hashB = %q, want equal for byte-identical content at different paths", hashA, hashB)
	}
}

func TestHashLocalConfig_WhitespaceOnlyDifference_HashesDifferently(t *testing.T) {
	dir := t.TempDir()
	pathTrailingSpace := filepath.Join(dir, "trailing-space.yml")
	pathTrailingNewline := filepath.Join(dir, "trailing-newline.yml")

	if err := os.WriteFile(pathTrailingSpace, []byte("columns:\n  - name: Todo "), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if err := os.WriteFile(pathTrailingNewline, []byte("columns:\n  - name: Todo\n"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	hashSpace, err := HashLocalConfig(pathTrailingSpace)
	if err != nil {
		t.Fatalf("HashLocalConfig(pathTrailingSpace) returned error: %v", err)
	}
	hashNewline, err := HashLocalConfig(pathTrailingNewline)
	if err != nil {
		t.Fatalf("HashLocalConfig(pathTrailingNewline) returned error: %v", err)
	}

	if hashSpace == hashNewline {
		t.Errorf("hashes matched (%q) for whitespace-differing content, want different hashes", hashSpace)
	}
}

func TestHashLocalConfig_MissingFile_ReturnsErrNotExist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.yml")

	_, err := HashLocalConfig(path)

	if err == nil {
		t.Fatal("HashLocalConfig() on a missing file returned no error, want an error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("errors.Is(err, os.ErrNotExist) = false, want true; err = %v", err)
	}
}

func TestHashLocalConfig_PathIsDirectory_ReturnsNonNotExistError(t *testing.T) {
	dir := t.TempDir()

	_, err := HashLocalConfig(dir)

	if err == nil {
		t.Fatal("HashLocalConfig() on a directory path returned no error, want an error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("errors.Is(err, os.ErrNotExist) = true, want false (a directory exists, it's just unreadable as a file); err = %v", err)
	}
}

func TestHashLocalConfig_UnreadablePermissions_ReturnsNonNotExistError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0000 semantics differ on windows")
	}

	path := filepath.Join(t.TempDir(), "unreadable.yml")
	if err := os.WriteFile(path, []byte("columns: []\n"), 0000); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(path, 0600)
	})

	_, err := HashLocalConfig(path)

	if err == nil {
		t.Fatal("HashLocalConfig() on an unreadable file returned no error, want an error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Errorf("errors.Is(err, os.ErrNotExist) = true, want false (the file exists, it's just unreadable); err = %v", err)
	}
}

// Published FIPS-180 SHA-256 test vector for "abc" — not derived from this
// implementation, so it catches a wrong hash algorithm or a malformed
// "sha256:" prefix.
func TestHashLocalConfig_KnownVector_MatchesPublishedSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "abc.yml")
	if err := os.WriteFile(path, []byte("abc"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	hash, err := HashLocalConfig(path)

	if err != nil {
		t.Fatalf("HashLocalConfig() returned error: %v", err)
	}
	want := "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if hash != want {
		t.Errorf("HashLocalConfig() = %q, want %q (published FIPS-180 SHA-256 test vector for %q)", hash, want, "abc")
	}
}

// --- Trust store (AC4) ---

func TestLoadTrust_MissingFile_ReturnsZeroTrustWithoutError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.yml")

	trust, err := LoadTrust(path)

	if err != nil {
		t.Fatalf("LoadTrust() on a missing file returned error %v, want nil (a first launch has no trust store)", err)
	}
	if len(trust.Trusted) != 0 {
		t.Errorf("Trusted has %d entries, want 0 for a missing store", len(trust.Trusted))
	}
}

func TestLoadTrust_EmptyFile_ReturnsZeroTrustWithoutError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.yml")
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatalf("failed to write trust file: %v", err)
	}

	trust, err := LoadTrust(path)

	if err != nil {
		t.Fatalf("LoadTrust() on a 0-byte file returned error %v, want nil", err)
	}
	if len(trust.Trusted) != 0 {
		t.Errorf("Trusted has %d entries, want 0 for an empty store", len(trust.Trusted))
	}
}

func TestLoadTrust_MalformedYAML_ReturnsErrorAndZeroTrust(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.yml")
	if err := os.WriteFile(path, []byte("trusted: [unclosed\n"), 0600); err != nil {
		t.Fatalf("failed to write trust file: %v", err)
	}

	trust, err := LoadTrust(path)

	if err == nil {
		t.Fatal("LoadTrust() returned no error for malformed YAML, want an error")
	}
	if len(trust.Trusted) != 0 {
		t.Errorf("Trusted has %d entries, want 0 (never a partially-parsed store on error)", len(trust.Trusted))
	}
}

func TestLoadTrust_WrongShapeDocument_ReturnsErrorAndZeroTrust(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.yml")
	if err := os.WriteFile(path, []byte("trusted: \"notalist\"\n"), 0600); err != nil {
		t.Fatalf("failed to write trust file: %v", err)
	}

	trust, err := LoadTrust(path)

	if err == nil {
		t.Fatal("LoadTrust() returned no error for a wrong-shape document (trusted must be a list), want an error")
	}
	if len(trust.Trusted) != 0 {
		t.Errorf("Trusted has %d entries, want 0 (never a partially-parsed store on error)", len(trust.Trusted))
	}
}

func TestSaveTrust_CreatesParentDirWithModePermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "lazyboards")
	path := filepath.Join(dir, "trust.yml")

	if err := SaveTrust(path, Trust{Trusted: []TrustEntry{{Hash: "sha256:abc", Note: "first"}}}); err != nil {
		t.Fatalf("SaveTrust() returned error: %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("trust dir not created at %s: %v", dir, err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("trust dir mode = %o, want 0700", perm)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("trust file not written at %s: %v", path, err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("trust file mode = %o, want 0600", perm)
	}
}

// SaveState only relies on create-time mode bits; SaveTrust must explicitly
// chmod even when the dir/file pre-existed with looser permissions, since a
// trust store's contents are more security-sensitive (it gates command
// execution) than runtime UI state.
func TestSaveTrust_PreExistingLoosePermissions_TightensToRequiredModes(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses file permission checks")
	}
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits don't apply on windows")
	}

	dir := filepath.Join(t.TempDir(), "lazyboards")
	if err := os.MkdirAll(dir, 0777); err != nil {
		t.Fatalf("failed to pre-create dir: %v", err)
	}
	path := filepath.Join(dir, "trust.yml")
	if err := os.WriteFile(path, []byte("trusted: []\n"), 0666); err != nil {
		t.Fatalf("failed to pre-create file: %v", err)
	}

	if err := SaveTrust(path, Trust{Trusted: []TrustEntry{{Hash: "sha256:abc", Note: "first"}}}); err != nil {
		t.Fatalf("SaveTrust() returned error: %v", err)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("failed to stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0700 {
		t.Errorf("trust dir mode = %o, want 0700 (must be tightened even though it pre-existed as 0777)", perm)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0600 {
		t.Errorf("trust file mode = %o, want 0600 (must be tightened even though it pre-existed as 0666)", perm)
	}
}

func TestSaveTrust_LoadTrust_RoundTripPreservesMultipleEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.yml")
	want := Trust{Trusted: []TrustEntry{
		{Hash: "sha256:aaa", Note: "repo one"},
		{Hash: "sha256:bbb", Note: "repo two"},
	}}

	if err := SaveTrust(path, want); err != nil {
		t.Fatalf("SaveTrust() returned error: %v", err)
	}
	got, err := LoadTrust(path)
	if err != nil {
		t.Fatalf("LoadTrust() returned error: %v", err)
	}

	if len(got.Trusted) != len(want.Trusted) {
		t.Fatalf("Trusted has %d entries, want %d", len(got.Trusted), len(want.Trusted))
	}
	for i, entry := range want.Trusted {
		if got.Trusted[i].Hash != entry.Hash {
			t.Errorf("Trusted[%d].Hash = %q, want %q", i, got.Trusted[i].Hash, entry.Hash)
		}
		if got.Trusted[i].Note != entry.Note {
			t.Errorf("Trusted[%d].Note = %q, want %q", i, got.Trusted[i].Note, entry.Note)
		}
	}
}

func TestTrust_ZeroValue_TrustsNothing(t *testing.T) {
	var trust Trust

	if trust.Trusts("sha256:anything") {
		t.Error("Trust{}.Trusts() = true, want false; a zero-value trust store must trust nothing")
	}
}

func TestTrust_Trusts_MatchesOnHashRegardlessOfNote(t *testing.T) {
	trust := Trust{Trusted: []TrustEntry{
		{Hash: "sha256:matchme", Note: "any note at all"},
	}}

	if !trust.Trusts("sha256:matchme") {
		t.Error("Trusts() = false, want true for a hash present in the store regardless of its note")
	}
}

func TestTrust_Trusts_UnknownHash_ReturnsFalse(t *testing.T) {
	trust := Trust{Trusted: []TrustEntry{
		{Hash: "sha256:known", Note: "trusted repo"},
	}}

	if trust.Trusts("sha256:unknown") {
		t.Error("Trusts() = true for a hash not present in the store, want false")
	}
}

func TestTrust_Trusts_EmptyHash_ReturnsFalseEvenWithEmptyHashEntry(t *testing.T) {
	trust := Trust{Trusted: []TrustEntry{
		{Hash: "", Note: "malformed entry"},
	}}

	if trust.Trusts("") {
		t.Error("Trusts(\"\") = true, want false; an empty hash must never match, even against an empty-hash entry")
	}
}

// --- TrustEntry.Path / UpsertTrustEntry / PriorEntryForPath / StaleTrust (#642) ---

// TestTrust_Trusts_PathMatchDoesNotGrantTrust is the single most important
// test in this file: a stored entry whose Path matches the identity being
// checked, but whose Hash does not match the content hash, must NOT be
// trusted. Path is a re-approval-UX identity only; it must never leak into
// the trust decision itself.
func TestTrust_Trusts_PathMatchDoesNotGrantTrust(t *testing.T) {
	trust := Trust{Trusted: []TrustEntry{
		{Hash: "sha256:old-content", Note: "repo", Path: "/repo/.git"},
	}}

	// Same Path (same repo identity), but the queried hash is for
	// DIFFERENT content than what was actually reviewed and trusted.
	if trust.Trusts("sha256:new-untrusted-content") {
		t.Fatal("Trusts() = true for a hash not in the store, even though an entry shares its Path -- Path must never grant trust")
	}
}

// TestTrust_PriorEntryForPath covers every match/no-match combination the
// empty-path guard must handle on both sides of the comparison.
func TestTrust_PriorEntryForPath(t *testing.T) {
	trust := Trust{Trusted: []TrustEntry{
		{Hash: "sha256:a", Note: "legacy entry, no identity recorded", Path: ""},
		{Hash: "sha256:b", Note: "n", Path: "/repo/.git"},
	}}

	tests := []struct {
		name      string
		queryPath string
		wantOK    bool
		wantHash  string
	}{
		{"empty query path never matches, even a stored empty-Path entry", "", false, ""},
		{"stored empty Path never matches a non-empty query either", "/other/.git", false, ""},
		{"matching non-empty path hits", "/repo/.git", true, "sha256:b"},
		{"unrelated non-empty path misses", "/unrelated/.git", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := trust.PriorEntryForPath(tt.queryPath)
			if ok != tt.wantOK {
				t.Fatalf("PriorEntryForPath(%q) ok = %v, want %v", tt.queryPath, ok, tt.wantOK)
			}
			if ok && got.Hash != tt.wantHash {
				t.Errorf("PriorEntryForPath(%q).Hash = %q, want %q", tt.queryPath, got.Hash, tt.wantHash)
			}
		})
	}
}

// TestTrustEntry_LegacyYAML_NoPathFieldRoundTripsUnchanged asserts a store
// written before #642 (no "path:" key at all) loads with Path == "" and,
// once saved back out, still contains no "path:" line (thanks to
// yaml:"path,omitempty") -- a legacy store's on-disk shape doesn't drift
// just because it was loaded and re-saved through a #642-aware binary.
func TestTrustEntry_LegacyYAML_NoPathFieldRoundTripsUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.yml")
	legacy := "trusted:\n  - hash: \"sha256:legacy\"\n    note: \"owner/repo\"\n"
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatalf("failed to write legacy trust store: %v", err)
	}

	trust, err := LoadTrust(path)
	if err != nil {
		t.Fatalf("LoadTrust() returned unexpected error: %v", err)
	}
	if len(trust.Trusted) != 1 {
		t.Fatalf("Trusted count = %d, want 1", len(trust.Trusted))
	}
	if trust.Trusted[0].Path != "" {
		t.Errorf("Trusted[0].Path = %q, want \"\" for a legacy entry with no path: key", trust.Trusted[0].Path)
	}
	if !trust.Trusts("sha256:legacy") {
		t.Error("Trusts() = false for the legacy entry's own hash, want true")
	}

	if err := SaveTrust(path, trust); err != nil {
		t.Fatalf("SaveTrust() returned unexpected error: %v", err)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read saved trust store: %v", err)
	}
	if strings.Contains(string(saved), "path:") {
		t.Errorf("saved trust store contains a path: key for a legacy entry, want it omitted (yaml:\"path,omitempty\"); saved = %q", saved)
	}
}

// TestUpsertTrustEntry covers both branches: entry.Path == "" preserves the
// pre-#642 CLI idempotent-append behavior exactly (append unless the hash
// is already trusted), while entry.Path != "" always replaces every
// existing same-Path entry -- whether there was zero, one (the core #642
// AC: re-trust after content changed replaces, not accumulates), or more
// than one (self-healing an already-duplicated store) -- and never touches
// entries under a different Path.
func TestUpsertTrustEntry(t *testing.T) {
	tests := []struct {
		name        string
		before      []TrustEntry
		entry       TrustEntry
		wantHashes  []string // Trusted[i].Hash in order, after the upsert
		wantTrusted []string // hashes that must Trusts() == true afterward
		wantStale   []string // hashes that must Trusts() == false afterward
	}{
		{
			name:        "empty Path appends a new hash",
			entry:       TrustEntry{Hash: "sha256:a", Note: "n"},
			wantHashes:  []string{"sha256:a"},
			wantTrusted: []string{"sha256:a"},
		},
		{
			name:        "empty Path re-upserting an already-trusted hash does not duplicate",
			before:      []TrustEntry{{Hash: "sha256:a", Note: "n"}},
			entry:       TrustEntry{Hash: "sha256:a", Note: "n2"},
			wantHashes:  []string{"sha256:a"},
			wantTrusted: []string{"sha256:a"},
		},
		{
			name:        "non-empty Path replaces the single existing same-identity entry",
			before:      []TrustEntry{{Hash: "sha256:old", Note: "n", Path: "/repo/.git"}},
			entry:       TrustEntry{Hash: "sha256:new", Note: "n2", Path: "/repo/.git"},
			wantHashes:  []string{"sha256:new"},
			wantTrusted: []string{"sha256:new"},
			wantStale:   []string{"sha256:old"},
		},
		{
			name: "non-empty Path self-heals an already-duplicated same-identity store",
			before: []TrustEntry{
				{Hash: "sha256:old1", Note: "n", Path: "/repo/.git"},
				{Hash: "sha256:old2", Note: "n", Path: "/repo/.git"},
			},
			entry:      TrustEntry{Hash: "sha256:new", Note: "n2", Path: "/repo/.git"},
			wantHashes: []string{"sha256:new"},
		},
		{
			name:        "non-empty Path leaves a different identity's entry untouched",
			before:      []TrustEntry{{Hash: "sha256:a", Note: "n", Path: "/repo-a/.git"}},
			entry:       TrustEntry{Hash: "sha256:b", Note: "n", Path: "/repo-b/.git"},
			wantHashes:  []string{"sha256:a", "sha256:b"},
			wantTrusted: []string{"sha256:a", "sha256:b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UpsertTrustEntry(Trust{Trusted: tt.before}, tt.entry)

			if len(got.Trusted) != len(tt.wantHashes) {
				t.Fatalf("Trusted count = %d, want %d", len(got.Trusted), len(tt.wantHashes))
			}
			for i, want := range tt.wantHashes {
				if got.Trusted[i].Hash != want {
					t.Errorf("Trusted[%d].Hash = %q, want %q", i, got.Trusted[i].Hash, want)
				}
			}
			for _, hash := range tt.wantTrusted {
				if !got.Trusts(hash) {
					t.Errorf("Trusts(%q) = false, want true", hash)
				}
			}
			for _, hash := range tt.wantStale {
				if got.Trusts(hash) {
					t.Errorf("Trusts(%q) = true, want false (replaced/stale)", hash)
				}
			}
		})
	}
}

// TestStaleTrust covers StaleTrust's full precedence: already-trusted
// always wins (false) regardless of Path, a matching prior entry under an
// untrusted hash is the only true case, no prior entry for the identity is
// false (a genuine first-ever load must never look stale), and an empty
// identity -- on either side -- never matches.
func TestStaleTrust(t *testing.T) {
	tests := []struct {
		name      string
		trusted   []TrustEntry
		hash      string
		path      string
		wantOK    bool
		wantEntry string // wantEntry.Hash, when wantOK
	}{
		{
			name:    "false when hash already trusted",
			trusted: []TrustEntry{{Hash: "sha256:current", Note: "n", Path: "/repo/.git"}},
			hash:    "sha256:current", path: "/repo/.git",
			wantOK: false,
		},
		{
			name:    "true when hash untrusted and identity matched",
			trusted: []TrustEntry{{Hash: "sha256:old", Note: "n", Path: "/repo/.git"}},
			hash:    "sha256:new", path: "/repo/.git",
			wantOK: true, wantEntry: "sha256:old",
		},
		{
			name:    "false when no prior entry for this identity",
			trusted: []TrustEntry{{Hash: "sha256:other", Note: "n", Path: "/other-repo/.git"}},
			hash:    "sha256:new", path: "/repo/.git",
			wantOK: false,
		},
		{
			name:    "false when the query identity is empty",
			trusted: []TrustEntry{{Hash: "sha256:old", Note: "legacy", Path: ""}},
			hash:    "sha256:new", path: "",
			wantOK: false,
		},
		{
			name:    "false when a legacy Path-less entry is checked against a non-empty identity",
			trusted: []TrustEntry{{Hash: "sha256:old", Note: "legacy", Path: ""}},
			hash:    "sha256:new", path: "/repo/.git",
			wantOK: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trust := Trust{Trusted: tt.trusted}
			entry, ok := trust.StaleTrust(tt.hash, tt.path)
			if ok != tt.wantOK {
				t.Fatalf("StaleTrust(%q, %q) ok = %v, want %v", tt.hash, tt.path, ok, tt.wantOK)
			}
			if ok && entry.Hash != tt.wantEntry {
				t.Errorf("StaleTrust(%q, %q).Hash = %q, want %q", tt.hash, tt.path, entry.Hash, tt.wantEntry)
			}
		})
	}
}

// carryTrustForward fails closed on a malformed trust store (never rewrites
// it), but that error must not vanish without a trace: main.go's own
// startup LoadTrust call logs via debuglog before falling back, and
// carryTrustForward must do the same so a broken trust store is debuggable
// instead of silently degrading Save()'s carry-forward every time.
func TestCarryTrustForward_MalformedTrustStore_LogsSwallowedError(t *testing.T) {
	dir := t.TempDir()
	trustPath := filepath.Join(dir, "trust.yml")
	if err := os.WriteFile(trustPath, []byte("trusted: \"this is not a list\"\n"), 0600); err != nil {
		t.Fatalf("failed to write malformed trust store: %v", err)
	}

	logPath := filepath.Join(dir, "debug.log")
	if err := debuglog.Init(logPath); err != nil {
		t.Fatalf("debuglog.Init() returned error: %v", err)
	}
	t.Cleanup(func() { _ = debuglog.Init("") })

	if err := carryTrustForward(trustPath, "sha256:pre", "sha256:post"); err != nil {
		t.Fatalf("carryTrustForward() returned error: %v, want nil (fail closed, never fails the caller)", err)
	}

	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read debug log: %v", err)
	}
	logged := string(logBytes)
	if !strings.Contains(logged, trustPath) {
		t.Errorf("debug log = %q, want it to mention the trust store path %q", logged, trustPath)
	}
	if !strings.Contains(strings.ToLower(logged), "carry-forward") {
		t.Errorf("debug log = %q, want it to mention the carry-forward skip", logged)
	}
}

func TestDefaultTrustPath_SitsBesideGlobalConfigAndStatePath(t *testing.T) {
	trustPath, err := DefaultTrustPath()
	if err != nil {
		t.Fatalf("DefaultTrustPath() returned error: %v", err)
	}
	globalPath, err := DefaultGlobalPath()
	if err != nil {
		t.Fatalf("DefaultGlobalPath() returned error: %v", err)
	}
	statePath, err := DefaultStatePath()
	if err != nil {
		t.Fatalf("DefaultStatePath() returned error: %v", err)
	}

	if filepath.Dir(trustPath) != filepath.Dir(globalPath) {
		t.Errorf("trust path dir = %q, want it alongside the global config dir %q", filepath.Dir(trustPath), filepath.Dir(globalPath))
	}
	if trustPath == globalPath {
		t.Error("DefaultTrustPath() must not equal DefaultGlobalPath() — the trust store is a separate file")
	}
	if trustPath == statePath {
		t.Error("DefaultTrustPath() must not equal DefaultStatePath() — the trust store is a separate file")
	}
}
