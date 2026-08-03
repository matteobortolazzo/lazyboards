package config

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	"gopkg.in/yaml.v3"
)

// TrustEntry records a single trusted local-config hash, along with a
// human-readable note (e.g. the repo it belongs to) for the user's own
// reference.
type TrustEntry struct {
	Hash string `yaml:"hash"`
	Note string `yaml:"note"`
}

// Trust is the on-disk trust store: the set of local-config hashes the user
// has explicitly approved for execution. Its zero value trusts nothing --
// that is the epic's core security invariant, so a missing or unreadable
// store must never fail open.
type Trust struct {
	Trusted []TrustEntry `yaml:"trusted"`
}

// Trusts reports whether hash matches a trusted entry. An empty hash never
// matches, even against a stored entry that also has an empty Hash --
// otherwise a malformed entry could accidentally trust every unhashed input.
func (t Trust) Trusts(hash string) bool {
	if hash == "" {
		return false
	}
	for _, entry := range t.Trusted {
		if entry.Hash == hash {
			return true
		}
	}
	return false
}

// hashConfigBytes hashes data already read into memory, so a caller that has
// already read the file (e.g. Load, which reads localData before this store
// is consulted) doesn't need to re-read it just to hash it.
func hashConfigBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// HashLocalConfig reads the local config file at path and returns its content
// hash in "sha256:<hex>" form.
func HashLocalConfig(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("local config %s: %w", path, err)
	}
	return hashConfigBytes(data), nil
}

// DefaultTrustPath returns the default trust-store file path, alongside the
// global config and runtime state at ~/.config/lazyboards/trust.yml. The
// parent directory is created on demand by SaveTrust, so it need not exist
// yet.
func DefaultTrustPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "lazyboards", "trust.yml"), nil
}

// LoadTrust reads the trust store from path. A missing or empty file is not
// an error -- it just means nothing has been trusted yet -- but unlike
// LoadState, a malformed or wrong-shape document is never silently
// downgraded to an empty-but-no-error result: this store gates command
// execution, so a parse failure must be reported, and must never return a
// partially-populated Trust alongside that error.
func LoadTrust(path string) (Trust, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Trust{}, nil
	}
	if err != nil {
		return Trust{}, fmt.Errorf("trust file %s: %w", path, err)
	}

	var t Trust
	if err := yaml.Unmarshal(data, &t); err != nil {
		return Trust{}, fmt.Errorf("trust file %s: %w", path, err)
	}
	return t, nil
}

// carryTrustForward carries trust forward across a Save()-initiated rewrite:
// if the pre-write content (preHash) was already trusted, the post-write
// content (postHash) becomes trusted too, so editing a config exclusively
// through the app's own Save() path never silently drops back to untrusted.
// If preHash was not trusted -- including because the store is empty,
// missing, or malformed -- this never grants new trust; it only ever carries
// forward trust that already existed. An empty trustPath means trust-carry
// is disabled entirely (no I/O). Any error loading or saving the store is
// swallowed: a broken trust store must never fail the config write itself,
// it only means the carry-forward step is skipped this time.
func carryTrustForward(trustPath, preHash, postHash string) error {
	if trustPath == "" {
		return nil
	}

	store, err := LoadTrust(trustPath)
	if err != nil {
		// Fail closed: a malformed/unreadable store is never rewritten. Log
		// it (mirroring main.go's own startup LoadTrust fallback) so a
		// broken trust store is debuggable instead of silently degrading
		// every Save() carry-forward with no trace.
		debuglog.Log(fmt.Sprintf("trust: carry-forward skipped, could not load trust store %s: %v", trustPath, err))
		return nil
	}
	if !store.Trusts(preHash) {
		return nil
	}

	updated := make([]TrustEntry, 0, len(store.Trusted))
	seenPost := false
	for _, entry := range store.Trusted {
		if entry.Hash == preHash {
			entry.Hash = postHash
		}
		if entry.Hash == postHash {
			if seenPost {
				continue // dedupe against a pre-existing postHash entry
			}
			seenPost = true
		}
		updated = append(updated, entry)
	}

	return SaveTrust(trustPath, Trust{Trusted: updated})
}

// SaveTrust writes t to path, creating the parent directory if needed. It
// explicitly tightens the directory to 0700 even when it pre-existed with
// looser permissions -- a trust store is more security-sensitive than
// runtime UI state, since it gates command execution, so SaveState's
// create-time-only mode bits aren't sufficient here. The directory is
// tightened before the file is written, and the file itself is written to a
// 0600 temp file and atomically renamed into place, so the store's content
// is never briefly reachable at a looser mode than its final one.
func SaveTrust(path string, t Trust) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("trust file %s: mkdir %s: %w", path, dir, err)
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return fmt.Errorf("trust file %s: chmod dir %s: %w", path, dir, err)
	}
	out, err := yaml.Marshal(t)
	if err != nil {
		return fmt.Errorf("trust file %s: marshal: %w", path, err)
	}
	tmp, err := os.CreateTemp(dir, ".trust-*.tmp")
	if err != nil {
		return fmt.Errorf("trust file %s: create temp: %w", path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }() // no-op once the rename below succeeds
	if _, err := tmp.Write(out); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("trust file %s: write temp: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("trust file %s: close temp: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("trust file %s: rename: %w", path, err)
	}
	return nil
}
