package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveTrustIdentity_GitRepo_ReturnsCommonDirAbs(t *testing.T) {
	repoRoot := t.TempDir()
	gitDir := filepath.Join(repoRoot, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create gitdir: %v", err)
	}
	localPath := filepath.Join(repoRoot, ".lazyboards.yml")

	got := resolveTrustIdentity(gitDir, localPath)

	wantAbs, err := filepath.Abs(gitDir)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if got != wantAbs {
		t.Errorf("resolveTrustIdentity() = %q, want %q", got, wantAbs)
	}
}

func TestResolveTrustIdentity_LinkedWorktree_ConvergesWithMainRepo(t *testing.T) {
	// A linked worktree of the same physical repo must resolve to the same
	// identity as the main repo itself — the whole point of keying trust
	// identity on the git common dir instead of the local config's path.
	mainRoot := t.TempDir()
	commonGitDir := filepath.Join(mainRoot, ".git")
	if err := os.MkdirAll(commonGitDir, 0755); err != nil {
		t.Fatalf("failed to create common gitdir: %v", err)
	}
	worktreeGitDir := filepath.Join(commonGitDir, "worktrees", "wt1")
	if err := os.MkdirAll(worktreeGitDir, 0755); err != nil {
		t.Fatalf("failed to create worktree gitdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktreeGitDir, "commondir"), []byte("../..\n"), 0644); err != nil {
		t.Fatalf("failed to write commondir file: %v", err)
	}

	linkedRoot := t.TempDir()
	linkedGitFile := filepath.Join(linkedRoot, ".git")
	if err := os.WriteFile(linkedGitFile, []byte("gitdir: "+worktreeGitDir+"\n"), 0644); err != nil {
		t.Fatalf("failed to write gitdir pointer file: %v", err)
	}

	mainIdentity := resolveTrustIdentity(commonGitDir, filepath.Join(mainRoot, ".lazyboards.yml"))
	linkedIdentity := resolveTrustIdentity(linkedGitFile, filepath.Join(linkedRoot, ".lazyboards.yml"))

	if mainIdentity == "" || linkedIdentity == "" {
		t.Fatalf("resolveTrustIdentity() returned empty: main=%q linked=%q", mainIdentity, linkedIdentity)
	}
	if mainIdentity != linkedIdentity {
		t.Errorf("identities diverged: main = %q, linked worktree = %q", mainIdentity, linkedIdentity)
	}
}

func TestResolveTrustIdentity_NoGit_FallsBackToAbsLocalPath(t *testing.T) {
	dir := t.TempDir()
	localPath := filepath.Join(dir, ".lazyboards.yml")
	missingGitPath := filepath.Join(dir, "nonexistent-git")

	got := resolveTrustIdentity(missingGitPath, localPath)

	wantAbs, err := filepath.Abs(localPath)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if got != wantAbs {
		t.Errorf("resolveTrustIdentity() = %q, want %q", got, wantAbs)
	}
}

func TestResolveTrustIdentity_MalformedGitdir_FallsBackToAbsLocalPath(t *testing.T) {
	dir := t.TempDir()
	gitFile := filepath.Join(dir, ".git")
	if err := os.WriteFile(gitFile, []byte("not-a-gitdir-pointer"), 0644); err != nil {
		t.Fatalf("failed to write malformed gitdir pointer file: %v", err)
	}
	localPath := filepath.Join(dir, ".lazyboards.yml")

	got := resolveTrustIdentity(gitFile, localPath)

	wantAbs, err := filepath.Abs(localPath)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	if got != wantAbs {
		t.Errorf("resolveTrustIdentity() = %q, want %q", got, wantAbs)
	}
}

func TestResolveTrustIdentity_RelativeLocalPath_ResolvesFromCwd(t *testing.T) {
	// localPath is frequently a bare relative filename (config.DefaultLocalPath,
	// ".lazyboards.yml") — the fallback must still resolve it to an absolute
	// path rather than returning it unchanged.
	dir := t.TempDir()
	missingGitPath := filepath.Join(dir, "nonexistent-git")

	got := resolveTrustIdentity(missingGitPath, "relative-local-config.yml")

	if !filepath.IsAbs(got) {
		t.Errorf("resolveTrustIdentity() = %q, want an absolute path", got)
	}
}
