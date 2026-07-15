# Lessons Learned

Lessons are routed by topic instead of collected in one growing file: the `lessons-collector` agent writes each new lesson into the `docs/<topic>.md` file for the subsystem it touched (creating one if none fits), or into this repo's rule files (`testing.md`, `git-workflow.md`, etc.) when the lesson is a cross-cutting process rule rather than an app-code gotcha. Read the topic doc for the area you're touching before starting work — don't wait to be told; if none exists yet for that area, that's a signal no lesson has been captured there, not that it's safe to skip.

Current topic docs, by subsystem:
- `docs/agent-environment.md` — Claude Code sandbox operation (Go build cache, worktree creation, git commit messages, Bash cwd persistence)
- `docs/terminal-rendering.md` — lipgloss/glamour width calculations and markdown rendering quirks
- `docs/bubbletea-async-patterns.md` — `tea.Cmd` propagation and testing async BubbleTea behavior
- `docs/shell-and-url-safety.md` — escaping untrusted template variables in shell commands and URLs
- `docs/frontmatter-parsing.md` — the edit-mode frontmatter delimiter format
- `docs/list-cursor-invariants.md` — cursor/index consistency across filtered card views and column resolution
- `docs/view-state-consistency.md` — keeping event-handler guards and view renderers in sync
- `docs/git-integration.md` — `internal/git` background-poll and subprocess-result conventions
- `docs/cenciwatch-integration.md` — `internal/cenciwatch` reconnect/backoff and wire-format status matching

For test-methodology lessons (discarded assertions, test-forced production shapes, etc.), see `.claude/rules/testing.md`.
