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
