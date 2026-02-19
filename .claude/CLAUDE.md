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
| `commands.go` | Async `tea.Cmd` builders (`fetchBoardCmd`, `createCardCmd`, `runShellCmd`, `saveConfigCmd`) + `wrapTitle` |
| `statusbar.go` | `StatusBar` component (hints, timed messages) |
| `main.go` | Entry point, config loading, provider setup |

Tests are split by domain to mirror production code:

| Test File | Coverage |
|-----------|----------|
| `helpers_test.go` | Shared test infrastructure (board builders, key helpers, `execCmds`) |
| `model_test.go` | Board init, structure, loading/error modes |
| `update_test.go` | Navigation, quit, resize, scroll, refresh, number keys, status bar |
| `view_test.go` | View rendering, scroll indicators, border titles, card counts |
| `commands_test.go` | `wrapTitle` tests |
| `create_mode_test.go` | Create mode state, UI, input, form submission |
| `config_mode_test.go` | Config mode, first launch flow |
| `detail_panel_test.go` | Detail panel focus, scrolling, glamour rendering |
| `actions_test.go` | Action triggers (URL, shell), column actions |
| `statusbar_test.go` | StatusBar component tests |

Internal packages: `internal/action`, `internal/config`, `internal/git`, `internal/provider`.

## Rule Files
See `.claude/rules/` for conventions:
- `lessons-learned.md` — real mistakes from this codebase (authoritative, overrides assumptions)
- `testing.md` — TDD and test quality rules
- `security.md` — security guidelines
- `git-workflow.md` — branching, commits, worktrees, PRs
