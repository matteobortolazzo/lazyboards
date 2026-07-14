package git

import "testing"

func TestWorktreeForBranch(t *testing.T) {
	porcelain := `worktree /repo
HEAD 1234567
branch refs/heads/main

worktree /repo/.worktrees/42-fix%20spaces
HEAD 7654321
branch refs/heads/feature/42-fix-spaces
`

	if got, want := WorktreeForBranch(porcelain, "feature/42-fix-spaces"), "/repo/.worktrees/42-fix%20spaces"; got != want {
		t.Errorf("WorktreeForBranch() = %q, want %q", got, want)
	}
}

func TestWorktreeForBranch_NoMatch(t *testing.T) {
	porcelain := "worktree /repo\nHEAD 1234567\nbranch refs/heads/main\n"
	if got := WorktreeForBranch(porcelain, "feature/missing"); got != "" {
		t.Errorf("WorktreeForBranch() = %q, want empty string", got)
	}
}
