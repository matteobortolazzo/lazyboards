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
