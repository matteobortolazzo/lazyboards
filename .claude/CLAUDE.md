# Project: lazyboards

Go (BubbleTea) TUI application. Single project.
GitHub Issues for tracking. GitHub for code and PRs.

## Critical Rules
- ALWAYS read relevant `.claude/rules/` files before working on any layer.
- Test-first: write tests that assert behavior, not implementation details.
- No PR should exceed ~300 lines. Split into stacked PRs if larger.
- Use git worktrees for all feature work. Never modify code in main worktree.
- When adding a new mode or keybinding, update `helpSections` in `view.go` and the README Keybindings section.
- Keybinding case convention: **lowercase** single keys are built-in normal-mode commands (`n e c o r p a g d f l j k q`, etc.). **Uppercase `A-Z`** is reserved — in normal mode it is claimed by the user's custom-action system (`resolveAction` in `model.go`, including multi-key sequences that start uppercase and may continue with any letter/digit), and inside sub-panels/modals it denotes scoped sub-commands (e.g. Git Menu `P` = Push, `S` = Stash pop). Never assign a new top-level built-in feature to an uppercase key; pick a free lowercase key instead.
- When a new feature consumes a state struct that already has an established rendering or handling precedence elsewhere in the codebase (in an earlier feature or related component), the new consumer must implement the FULL precedence, not just the happy path and obvious errors. Write tests that verify every state the existing precedence distinguishes — a missing intermediate state (e.g., "daemon not running" vs. "healthy running") will silently render incorrectly and undermine the feature's purpose.

## File Structure

The main BubbleTea model is split by responsibility:

| File | Responsibility |
|------|---------------|
| `model.go` | Board struct, types, constants, styles, `NewBoard()`, `Init()`, `enterConfigMode()`, `dispatchState`, `dispatchMode` |
| `update.go` | `Update()` dispatcher + async message handlers (`handleBoardFetched`, `handleCardClosed`, `handleCardUpdated`, `handleAssigneesUpdated`, etc.) + shared helpers (`findCard`, `onCursorMoved`, `moveCursor`) |
| `mode_handlers.go` | Per-mode key handlers (`handleCreateModeKey`, `handleConfigModeKey`, `handleNormalModeKey`, `handleSearchModeKey`, `handleDispatchModeKey`, etc.) |
| `action_dispatch.go` | Custom-action dispatch hierarchy (`handleCustomActionKey`, `dispatchResolvedAction`, `handleActionKey`, `handleBoardActionKey`, PR-scoped action handlers) |
| `mouse_handlers.go` | Mouse event handling (`handleMouseMsg`, `handleMouseScroll`, `handleMouseClick`, `handleTabClick`, `handleCardClick`) |
| `view.go` | `View()` dispatcher + rendering helpers (card list, detail, modals) + display helpers (`cardDisplayText`, `cardLineCount`, `clampScrollOffset`), `viewDispatchModal` |
| `commands.go` | Async `tea.Cmd` builders (`fetchBoardCmd`, `createCardCmd`, `runShellCmd`, `runCleanupCmds`, `saveConfigCmd`, `queryDispatchStatusCmd`, `toggleEnrollCmd`, `dispatchOnceCmd`) + `wrapTitle` |
| `statusbar.go` | `StatusBar` component (hints, timed messages) |
| `main.go` | Entry point, config loading, provider setup |

Tests are split by domain to mirror production code:

| Test File | Coverage |
|-----------|----------|
| `helpers_test.go` | Shared test infrastructure (board builders, key helpers, `execCmds`) |
| `model_test.go` | Board init, structure, loading/error modes |
| `update_test.go` | Quit, resize, config hint, number hint, status bar |
| `navigation_test.go` | Tab/item navigation, card list scroll, resize clamp, number keys |
| `refresh_test.go` | Manual refresh, background refresh |
| `metadata_cache_test.go` | Metadata refresh TTL gating logic |
| `view_test.go` | View rendering, scroll indicators, border titles, card counts |
| `commands_test.go` | `wrapTitle` tests |
| `create_mode_test.go` | Create mode state, UI, input, form submission |
| `config_mode_test.go` | Config mode, first launch flow |
| `detail_panel_test.go` | Detail panel focus, scrolling, glamour rendering |
| `actions_test.go` | Action triggers (URL, shell), column actions |
| `cleanup_test.go` | Column cleanup on card departure |
| `statusbar_test.go` | StatusBar component tests |
| `dispatch_mode_test.go` | Dispatch mode (agent dispatch modal) scaffolding |
| `delete_mode_test.go` | Delete mode (two-step confirm flow, PR gating, cleanup guards) |
| `assign_mode_test.go` | Assign mode (assignee picker modal, collaborator list) |
| `cenciwatch_test.go` | Agent status matching (window→card), badge rendering, agent counts, wire-format decoding |
| `close_mode_test.go` | Close mode (close-confirm flow, target card resolution) |
| `comment_mode_test.go` | Comment mode (alt-key trigger, immediate vs. deferred action execution) |
| `filter_mode_test.go` | Filter picker modal (collecting/deduplicating label & assignee filter items) |
| `filter_test.go` | Active filter application (label/assignee matching, case sensitivity) |
| `git_actions_test.go` | Git default actions vs. custom action resolution/hints |
| `git_panel_test.go` | Git menu panel (open/close, default-action gating) |
| `gitstatus_wiring_test.go` | Git status fetch command + background poll scheduling |
| `help_test.go` | Help modal (open/close from normal & detail-focused states) |
| `label_confirm_test.go` | Frontmatter compose/parse round-trip for labels |
| `map_slice_test.go` | `mapSlice` generic helper |
| `mouse_test.go` | Mouse wheel scroll/cursor movement |
| `pr_count_test.go` | PR count aggregation and status bar indicator |
| `pr_list_test.go` | Global PR list modal navigation and selection |
| `pr_picker_test.go` | Single/multi-PR picker (open in browser, status messages) |
| `key_sequence_test.go` | Custom-action key sequences (prefix keys, pending state, which-key hints, cancellation) |
| `search_mode_test.go` | Search mode (enter/exit, query clearing) |
| `version_test.go` | App version injection/fallback, `--version` flag handling |

Internal packages: `internal/action`, `internal/auth`, `internal/cenciwatch`, `internal/config`, `internal/debuglog`, `internal/git`, `internal/provider`.

## Sandbox Image
- `.cenci/Dockerfile` — committed, per-repo image tailored to this repo's stack; the whole team builds the same image
- Rebuild after changing the stack or the Dockerfile: `cenci sandbox build` (run from inside this repo)

## Rule Files
See `.claude/rules/` for conventions:
- `lessons-learned.md` — pointer to topic-specific lessons (see below); authoritative, overrides assumptions
- `testing.md` — TDD and test quality rules
- `security.md` — security guidelines
- `git-workflow.md` — branching, commits, worktrees, PRs

## Topic Docs
See `docs/` for lessons and conventions scoped to a specific subsystem — read the one relevant to the code you're touching before starting:
- `agent-environment.md` — Claude Code sandbox operation (Go build cache, worktrees, git commits, Bash cwd)
- `terminal-rendering.md` — lipgloss/glamour width calculations and markdown rendering
- `bubbletea-async-patterns.md` — `tea.Cmd` propagation and async testing patterns
- `shell-and-url-safety.md` — escaping untrusted template variables in shell commands and URLs
- `frontmatter-parsing.md` — edit-mode frontmatter delimiter format
- `list-cursor-invariants.md` — cursor/index consistency across filtered views and column resolution
- `view-state-consistency.md` — keeping event-handler guards and view renderers in sync
- `git-integration.md` — `internal/git` background-poll and subprocess-result conventions
- `cenciwatch-integration.md` — `internal/cenciwatch` reconnect/backoff and wire-format status matching
