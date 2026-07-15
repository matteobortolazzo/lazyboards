# Agent Sandbox Environment

Operational conventions for running Go builds/tests and git commands inside the Claude Code sandbox itself — not application behavior.

## Rules

- `go test`/`go get` fail with "read-only file system" against the default `~/go/` and `~/.cache/go-build/`. Always set `GOPATH`, `GOCACHE`, `GOMODCACHE` to a user-specific writable dir under `/tmp/` (e.g. `/tmp/claude-1000/...` for UID 1000) before any Go command. Use `$TMPDIR` to detect the correct path rather than hardcoding `/tmp/claude/`.
- Working directory persists across separate Bash tool calls in the current harness (verified 2026-07-15: a `cd` in one call is still in effect in a later, unrelated call). An older lesson here claimed the opposite and instructed chaining `cd <path> && <command>` on every call — that claim no longer holds and should not be followed as a hard rule. Still, don't rely on a `cd` from many calls or a different task ago; if there's any doubt which directory a build/test/compile command will run in, chain `cd <path> && <command>` explicitly rather than assuming. Git commands resolve their worktree context from `.git` files regardless of cwd either way.
- `git worktree add` must be run from the main repository root with an absolute path (`cd /path/to/repo && git worktree add .worktrees/<name> ...`), never from inside an existing worktree — git resolves `.worktrees/` relative to cwd, so running it from a nested worktree nests the new worktree inside the first one.
- Never use heredoc syntax for `git commit -m "$(cat <<'EOF' ... EOF)"` — the shell needs to write the heredoc to a temp file first, which fails read-only in the sandbox even with `TMPDIR` set for the same command. Write the message to a file first (`printf ... > $TMPDIR/commit-msg.txt`), then `git commit -F $TMPDIR/commit-msg.txt`.
