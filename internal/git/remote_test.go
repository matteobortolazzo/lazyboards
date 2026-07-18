package git

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRemote(t *testing.T) {
	tests := []struct {
		name             string
		configContent    string
		skipFile         bool // when true, pass a nonexistent path instead of writing a file
		expectedProvider string
		expectedRepo     string
	}{
		{
			name:             "GitHub_HTTPS",
			configContent:    "[remote \"origin\"]\n\turl = https://github.com/owner/repo.git",
			expectedProvider: "github",
			expectedRepo:     "owner/repo",
		},
		{
			name:             "GitHub_HTTPS_NoGitSuffix",
			configContent:    "[remote \"origin\"]\n\turl = https://github.com/owner/repo",
			expectedProvider: "github",
			expectedRepo:     "owner/repo",
		},
		{
			name:             "GitHub_SSH",
			configContent:    "[remote \"origin\"]\n\turl = git@github.com:owner/repo.git",
			expectedProvider: "github",
			expectedRepo:     "owner/repo",
		},
		{
			name:             "GitHub_SSH_NoGitSuffix",
			configContent:    "[remote \"origin\"]\n\turl = git@github.com:owner/repo",
			expectedProvider: "github",
			expectedRepo:     "owner/repo",
		},
		{
			name:             "AzureDevOps_HTTPS",
			configContent:    "[remote \"origin\"]\n\turl = https://dev.azure.com/org/project/_git/repo",
			expectedProvider: "azure-devops",
			expectedRepo:     "org/project/repo",
		},
		{
			name:             "AzureDevOps_OldHTTPS",
			configContent:    "[remote \"origin\"]\n\turl = https://myorg.visualstudio.com/project/_git/repo",
			expectedProvider: "azure-devops",
			expectedRepo:     "myorg/project/repo",
		},
		{
			name:             "AzureDevOps_SSH",
			configContent:    "[remote \"origin\"]\n\turl = git@ssh.dev.azure.com:v3/org/project/repo",
			expectedProvider: "azure-devops",
			expectedRepo:     "org/project/repo",
		},
		{
			name:             "UnrecognizedHost",
			configContent:    "[remote \"origin\"]\n\turl = https://gitlab.com/owner/repo.git",
			expectedProvider: "",
			expectedRepo:     "",
		},
		{
			name:             "MissingFile",
			skipFile:         true,
			expectedProvider: "",
			expectedRepo:     "",
		},
		{
			name:             "MissingOrigin",
			configContent:    "[remote \"upstream\"]\n\turl = https://github.com/owner/repo.git",
			expectedProvider: "",
			expectedRepo:     "",
		},
		{
			name:             "EmptyFile",
			configContent:    "",
			expectedProvider: "",
			expectedRepo:     "",
		},
		{
			name:             "MalformedURL",
			configContent:    "[remote \"origin\"]\n\turl = not-a-url",
			expectedProvider: "",
			expectedRepo:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var configPath string

			if tt.skipFile {
				// Use a path that does not exist.
				configPath = filepath.Join(t.TempDir(), "nonexistent", "config")
			} else {
				dir := t.TempDir()
				configPath = filepath.Join(dir, "config")
				if err := os.WriteFile(configPath, []byte(tt.configContent), 0644); err != nil {
					t.Fatalf("failed to write test config file: %v", err)
				}
			}

			result := DetectRemote(configPath)

			if result.Provider != tt.expectedProvider {
				t.Errorf("Provider = %q, want %q", result.Provider, tt.expectedProvider)
			}
			if result.Repo != tt.expectedRepo {
				t.Errorf("Repo = %q, want %q", result.Repo, tt.expectedRepo)
			}
		})
	}
}

func TestResolveConfigPath(t *testing.T) {
	t.Run("PlainDirectory", func(t *testing.T) {
		// A normal (non-worktree) repo: ".git" is a directory containing
		// "config" directly.
		gitDir := t.TempDir()

		got := ResolveConfigPath(gitDir)

		want := filepath.Join(gitDir, "config")
		if got != want {
			t.Errorf("ResolveConfigPath() = %q, want %q", got, want)
		}
	})

	t.Run("WorktreeGitdirFileWithCommondir", func(t *testing.T) {
		// A linked worktree (`git worktree add`): the repo root's ".git" is
		// a file pointing at a per-worktree gitdir under the common
		// ".git/worktrees/<name>" directory, and that gitdir's "commondir"
		// file records the relative path back to the shared config.
		repoRoot := t.TempDir()
		commonGitDir := filepath.Join(repoRoot, ".git")
		worktreeGitDir := filepath.Join(commonGitDir, "worktrees", "wt1")
		if err := os.MkdirAll(worktreeGitDir, 0755); err != nil {
			t.Fatalf("failed to create worktree gitdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(worktreeGitDir, "commondir"), []byte("../..\n"), 0644); err != nil {
			t.Fatalf("failed to write commondir file: %v", err)
		}
		gitFile := filepath.Join(repoRoot, ".git-worktree-entry")
		if err := os.WriteFile(gitFile, []byte("gitdir: "+worktreeGitDir+"\n"), 0644); err != nil {
			t.Fatalf("failed to write gitdir pointer file: %v", err)
		}

		got := ResolveConfigPath(gitFile)

		want := filepath.Join(commonGitDir, "config")
		if got != want {
			t.Errorf("ResolveConfigPath() = %q, want %q", got, want)
		}
	})

	t.Run("GitdirFileWithoutCommondir", func(t *testing.T) {
		// A gitdir pointer with no "commondir" file: treat the pointed-at
		// directory itself as the config's home.
		repoRoot := t.TempDir()
		targetGitDir := filepath.Join(repoRoot, "actual-gitdir")
		if err := os.MkdirAll(targetGitDir, 0755); err != nil {
			t.Fatalf("failed to create target gitdir: %v", err)
		}
		gitFile := filepath.Join(repoRoot, ".git-worktree-entry")
		if err := os.WriteFile(gitFile, []byte("gitdir: "+targetGitDir), 0644); err != nil {
			t.Fatalf("failed to write gitdir pointer file: %v", err)
		}

		got := ResolveConfigPath(gitFile)

		want := filepath.Join(targetGitDir, "config")
		if got != want {
			t.Errorf("ResolveConfigPath() = %q, want %q", got, want)
		}
	})

	t.Run("MalformedGitdirFile", func(t *testing.T) {
		repoRoot := t.TempDir()
		gitFile := filepath.Join(repoRoot, ".git-worktree-entry")
		if err := os.WriteFile(gitFile, []byte("not-a-gitdir-pointer"), 0644); err != nil {
			t.Fatalf("failed to write malformed gitdir pointer file: %v", err)
		}

		got := ResolveConfigPath(gitFile)

		if got != "" {
			t.Errorf("ResolveConfigPath() = %q, want empty string", got)
		}
	})

	t.Run("MissingPath", func(t *testing.T) {
		got := ResolveConfigPath(filepath.Join(t.TempDir(), "nonexistent"))

		if got != "" {
			t.Errorf("ResolveConfigPath() = %q, want empty string", got)
		}
	})
}
