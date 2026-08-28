package main

import (
	"path/filepath"

	"github.com/matteobortolazzo/lazyboards/internal/git"
)

// resolveTrustIdentity resolves the stable per-repo identity that the trust
// re-prompt keys on (see docs/trust-model.md): the git common directory
// (absolute) when resolvable, so every worktree of a repo converges on the
// same identity, else the absolute local config path, else "" when even
// that can't be resolved. Follows the existing "fail to empty, never error"
// convention of gitdetect.DetectRemote/git.ResolveConfigPath. It lives in
// package main because it composes internal/git and internal/config
// concerns, mirroring how main.go/cli_trust.go already do this composition
// rather than making either internal package import the other.
func resolveTrustIdentity(gitPath, localPath string) string {
	if dir, ok := git.CommonDirAbs(gitPath); ok {
		return dir
	}
	abs, err := filepath.Abs(localPath)
	if err != nil {
		return ""
	}
	return abs
}
