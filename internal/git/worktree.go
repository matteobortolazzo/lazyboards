package git

import "strings"

// WorktreeForBranch returns the path of the worktree whose local branch matches
// branch, as reported by git worktree list --porcelain. It returns an empty
// string when no matching worktree is registered.
func WorktreeForBranch(porcelain, branch string) string {
	branchLine := "branch refs/heads/" + branch
	var path string
	for line := range strings.SplitSeq(porcelain, "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path = strings.TrimPrefix(line, "worktree ")
		case line == branchLine:
			return path
		}
	}
	return ""
}
