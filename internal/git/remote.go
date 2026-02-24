package git

import (
	"bufio"
	"net/url"
	"os"
	"strings"
)

// RemoteInfo holds parsed information about a git remote.
type RemoteInfo struct {
	Provider string // "github", "azure-devops", or ""
	Repo     string // "owner/repo" for GitHub, "org/project/repo" for Azure DevOps
}

// DetectRemote reads a .git/config file, finds the [remote "origin"] section,
// extracts the url, and parses it into a RemoteInfo.
func DetectRemote(gitConfigPath string) RemoteInfo {
	originURL := extractOriginURL(gitConfigPath)
	if originURL == "" {
		return RemoteInfo{}
	}
	return parseRemoteURL(originURL)
}

// extractOriginURL reads the git config file and returns the URL from
// the [remote "origin"] section, or "" if not found.
func extractOriginURL(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	inOrigin := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if line == `[remote "origin"]` {
			inOrigin = true
			continue
		}

		// A new section header ends the origin section.
		if inOrigin && strings.HasPrefix(line, "[") {
			break
		}

		if inOrigin {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == "url" {
				return strings.TrimSpace(parts[1])
			}
		}
	}

	return ""
}

// parseRemoteURL determines the provider and repo from a remote URL.
func parseRemoteURL(rawURL string) RemoteInfo {
	// SSH URLs: git@<host>:<path>
	if strings.HasPrefix(rawURL, "git@") {
		return parseSSHURL(rawURL)
	}

	// HTTPS URLs
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return RemoteInfo{}
	}

	switch {
	case u.Host == "github.com":
		return parseGitHubHTTPS(u)
	case u.Host == "dev.azure.com":
		return parseAzureDevOpsHTTPS(u)
	case strings.HasSuffix(u.Host, ".visualstudio.com"):
		return parseAzureDevOpsLegacyHTTPS(u)
	default:
		return RemoteInfo{}
	}
}

// parseGitHubHTTPS parses https://github.com/owner/repo[.git]
func parseGitHubHTTPS(u *url.URL) RemoteInfo {
	path := strings.TrimPrefix(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")

	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RemoteInfo{}
	}

	return RemoteInfo{
		Provider: "github",
		Repo:     parts[0] + "/" + parts[1],
	}
}

// parseAzureDevOpsHTTPS parses https://dev.azure.com/org/project/_git/repo
func parseAzureDevOpsHTTPS(u *url.URL) RemoteInfo {
	path := strings.TrimPrefix(u.Path, "/")
	parts := strings.Split(path, "/")
	// Expected: org/project/_git/repo
	if len(parts) != 4 || parts[2] != "_git" {
		return RemoteInfo{}
	}

	return RemoteInfo{
		Provider: "azure-devops",
		Repo:     parts[0] + "/" + parts[1] + "/" + parts[3],
	}
}

// parseAzureDevOpsLegacyHTTPS parses https://myorg.visualstudio.com/project/_git/repo
func parseAzureDevOpsLegacyHTTPS(u *url.URL) RemoteInfo {
	// org is the subdomain before .visualstudio.com
	org := strings.TrimSuffix(u.Host, ".visualstudio.com")

	path := strings.TrimPrefix(u.Path, "/")
	parts := strings.Split(path, "/")
	// Expected: project/_git/repo
	if len(parts) != 3 || parts[1] != "_git" {
		return RemoteInfo{}
	}

	return RemoteInfo{
		Provider: "azure-devops",
		Repo:     org + "/" + parts[0] + "/" + parts[2],
	}
}

// parseSSHURL parses SSH-style URLs: git@<host>:<path>
func parseSSHURL(rawURL string) RemoteInfo {
	// Strip "git@" prefix
	rest := strings.TrimPrefix(rawURL, "git@")

	colonIdx := strings.Index(rest, ":")
	if colonIdx < 0 {
		return RemoteInfo{}
	}

	host := rest[:colonIdx]
	path := rest[colonIdx+1:]

	switch host {
	case "github.com":
		return parseGitHubSSH(path)
	case "ssh.dev.azure.com":
		return parseAzureDevOpsSSH(path)
	default:
		return RemoteInfo{}
	}
}

// parseGitHubSSH parses the path from git@github.com:owner/repo[.git]
func parseGitHubSSH(path string) RemoteInfo {
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return RemoteInfo{}
	}

	return RemoteInfo{
		Provider: "github",
		Repo:     parts[0] + "/" + parts[1],
	}
}

// parseAzureDevOpsSSH parses the path from git@ssh.dev.azure.com:v3/org/project/repo
func parseAzureDevOpsSSH(path string) RemoteInfo {
	parts := strings.Split(path, "/")
	// Expected: v3/org/project/repo
	if len(parts) != 4 || parts[0] != "v3" {
		return RemoteInfo{}
	}

	return RemoteInfo{
		Provider: "azure-devops",
		Repo:     parts[1] + "/" + parts[2] + "/" + parts[3],
	}
}
