# Project: lazyboards

Go (BubbleTea) TUI application. Single project.
GitHub Issues for tracking. GitHub for code and PRs.

## Critical Rules
- ALWAYS read relevant `.claude/rules/` files before working on any layer.
- Test-first: write tests that assert behavior, not implementation details.
- No PR should exceed ~300 lines. Split into stacked PRs if larger.
- Use git worktrees for all feature work. Never modify code in main worktree.

## File Structure

The main BubbleTea model is split by responsibility:

| File | Responsibility |
|------|---------------|
| `model.go` | Board struct, types, constants, styles, `NewBoard()`, `Init()`, `enterConfigMode()`, `clampScrollOffset()` |
| `update.go` | `Update()` dispatcher + key/message handler methods |
| `view.go` | `View()` dispatcher + rendering helpers (card list, detail, modals) |
| `commands.go` | Async `tea.Cmd` builders (`fetchBoardCmd`, `createCardCmd`, `runShellCmd`, `saveConfigCmd`) + `truncateTitle` |
| `statusbar.go` | `StatusBar` component (hints, timed messages) |
| `main.go` | Entry point, config loading, provider setup |

Internal packages: `internal/action`, `internal/config`, `internal/git`, `internal/provider`.

## Rule Files
See `.claude/rules/` for conventions:
- `lessons-learned.md` — real mistakes from this codebase (authoritative, overrides assumptions)
- `testing.md` — TDD and test quality rules
- `security.md` — security guidelines
- `git-workflow.md` — branching, commits, worktrees, PRs
