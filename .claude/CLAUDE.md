# Project: lazyboards

Go (BubbleTea) TUI application. Single project.
GitHub Issues for tracking. GitHub for code and PRs.

## Critical Rules
- ALWAYS read relevant `.claude/rules/` files before working on any layer.
- Test-first: write tests that assert behavior, not implementation details.
- No PR should exceed ~300 lines. Split into stacked PRs if larger.
- Use git worktrees for all feature work. Never modify code in main worktree.
- When adding a new mode or keybinding, update `helpSections` in `view.go` and the README Keybindings section.
- When changing how a documented UI element renders (status glyphs, badges, colors, indicators), update its description in README.md — changes to visual meaning must stay synchronized with documentation.
- Keybinding case convention (amended by #489 — normal mode and the detail panel now dispatch through the `internal/keymap` registry; the full rewrite of this rule is still deferred to #492): the uppercase-`A-Z`-is-reserved-for-custom-actions gate no longer exists in code — `keymap.Keymap.Lookup` (`keymap_dispatch.go`) resolves any key, upper or lower, against the merged defaults+user table, so a user config can now bind a lowercase key to a custom action or rebind a built-in's key outright (their old key stops working unless separately re-bound). The **convention** still holds for what ships out of the box: lowercase single keys remain the built-in normal-mode commands (`n e c o r p a g d f l j k q`, etc.), and inside sub-panels/modals uppercase still denotes scoped sub-commands (e.g. Git Menu `P` = Push, `S` = Stash pop, both still on their own legacy `switch msg.String()` path, untouched by #489). Don't assign a new top-level built-in feature to an uppercase key by default; pick a free lowercase key instead — but don't assume uppercase is unreachable by user config either.
- When a new feature consumes a state struct that already has an established rendering or handling precedence elsewhere in the codebase (in an earlier feature or related component), the new consumer must implement the FULL precedence, not just the happy path and obvious errors. Write tests that verify every state the existing precedence distinguishes — a missing intermediate state (e.g., "daemon not running" vs. "healthy running") will silently render incorrectly and undermine the feature's purpose.
- cenci integration placement rule: two things are built-in code paths — (1) stateful reads of live cenci/cenci-watch state that the app continuously displays (agent status badges, the dispatch modal's Loop line, the status-bar dispatch segment), and (2) the mutating cenci triggers scoped to the dispatch modal (enroll/unenroll, dispatch-once, and loop on/off), each fired by a modal key against cenci's own fleet/repo state. Fleet-wide, persistent mutations (loop on/off) must go through a two-step confirm in both directions and name their blast radius ("all enrolled repos"), mirroring the close-confirm flow (#433). Everything else that imperatively shells out to cenci with app-specific templating stays a user-configured custom action — don't build those in as code paths.
- When a refactor removes all production call sites of a function, delete it from production code — don't keep it alive because tests still call it. Migrate test coverage to the surviving building blocks instead.
- When a feature needs to reference a fixed, app-wide entity (e.g., lazyboards' own GitHub repo for update checks), don't reuse Board fields like `repoOwner`/`repoName` which are semantically the user's configured tracked repo. Create separate, explicitly-named constants to prevent confusion and misuse.
- When a spec defines precedence across multiple orthogonal axes (e.g., layer: user vs. default, scope: column vs. mode), state the explicit combined order across all combinations — not just each axis independently. A spec stating "user merges over default" and separately "column wins on conflict" doesn't specify whether `default-column` overrides `user-mode`. Implementation will encode one axis as the outer loop; if unspecified, this silent choice risks fail-open bugs (e.g., default bindings overriding user unbinds).

## File Structure

The main BubbleTea model is split by responsibility:

| File | Responsibility |
|------|---------------|
| `model.go` | Board struct, types, constants, styles, `NewBoard()`, `Init()`, `enterConfigMode()`, `dispatchState`, `dispatchMode` |
| `update.go` | `Update()` dispatcher + async message handlers (`handleBoardFetched`, `handleCardClosed`, `handleCardUpdated`, `handleAssigneesUpdated`, etc.) + shared helpers (`findCard`, `onCursorMoved`, `moveCursor`) |
| `mode_handlers.go` | Per-mode key handlers (`handleCreateModeKey`, `handleConfigModeKey`, `handleNormalModeKey`, `runNormalCommand`, `handleSearchModeKey`, `handleDispatchModeKey`, etc.) |
| `keymap_dispatch.go` | Registry dispatch seam for normal mode and the detail panel (#489): `dispatchKey`, generalized `handlePendingSeqKey`, the Alt-strip-and-retry fallback (`lookupWithAltFallback`, `altFallbackEligible`), `withKeymap`, and registry-derived hint building (`registryHints`) |
| `action_dispatch.go` | Custom-action leaf dispatch (`dispatchActionWithAlt`, `dispatchResolvedAction`, `handleActionKey`, `handleBoardActionKey`, PR-scoped action handlers) -- reached from `keymap_dispatch.go`'s `dispatchBinding` for a `BindingAction` result, and still directly by `handlePRListActionKey` (#490) and `handleGitPanelKey` (#511) |
| `mouse_handlers.go` | Mouse event handling (`handleMouseMsg`, `handleMouseScroll`, `handleMouseClick`, `handleTabClick`, `handleCardClick`) |
| `view.go` | `View()` dispatcher + rendering helpers (card list, detail, modals) + display helpers (`cardDisplayText`, `cardLineCount`, `clampScrollOffset`), `viewDispatchModal` |
| `commands.go` | Async `tea.Cmd` builders (`fetchBoardCmd`, `createCardCmd`, `runShellCmd`, `runCleanupCmds`, `saveConfigCmd`, `queryDispatchStatusCmd`, `toggleEnrollCmd`, `dispatchOnceCmd`) + `wrapTitle` |
| `statusbar.go` | `StatusBar` component (hints, timed messages) |
| `references.go` | `#N` reference parsing (`parseCardRefs`, `annotateBodyRefs`) and the `m` reference-navigation trigger (`handleReferenceNavKey`, `handlePendingRefKey`, `resolveReference`, `refIssueURL`) |
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
| `agent_list_test.go` | Agents list modal (all cenci windows, tmux window navigation, state precedence) |
| `cleanup_test.go` | Column cleanup on card departure |
| `statusbar_test.go` | StatusBar component tests |
| `dispatch_mode_test.go` | Dispatch mode (agent dispatch modal) scaffolding |
| `dispatch_loop_test.go` | Dispatch modal Loop line rendering (CLI wire samples, live-vs-CLI view integration) |
| `dispatch_loop_source_test.go` | `dispatchLoopSource` live-vs-CLI precedence matrix |
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
| `milestone_list_test.go` | Milestones list modal (fetch/sort/interaction/view state precedence) |
| `mouse_test.go` | Mouse wheel scroll/cursor movement |
| `pr_count_test.go` | PR count aggregation and status bar indicator |
| `pr_list_test.go` | Global PR list modal navigation and selection |
| `pr_picker_test.go` | Single/multi-PR picker (open in browser, status messages) |
| `pr_status_test.go` | Pure PR-status derivation/priority functions (`prStatus`, `prStatusSymbol`, `prStatusStyle`, `worstPRStatus`) |
| `progress_bar_test.go` | Pure `renderProgressBar` fill math, clamping, and muted/color selection |
| `key_sequence_test.go` | Custom-action key sequences (prefix keys, pending state, which-key hints, cancellation) |
| `search_mode_test.go` | Search mode (enter/exit, query clearing) |
| `version_test.go` | App version injection/fallback, `--version` flag handling |
| `git_keymap_defaults_test.go` | Drift guard: `internal/keymap`'s git panel default `ActionBinding` entries vs. `config.DefaultGitActions()`/`gitPanelBuiltinOrder` |
| `keymap_dispatch_test.go` | Registry dispatch (user override wins, explicit unbind, lowercase custom-action dispatch, built-ins in pending sequences, per-column overlay scoping, Alt-strip-and-retry fallback incl. the required built-in-must-not-fire negative case, pending-sequence which-key hint label sanitization) |
| `keymap_hints_test.go` | Registry-derived hint bar (default table parity, remap/unbind reflection, canonical multi-key labels, column-scoped built-in rebind reflection, card-scope hint gate uses the raw active-column card count not the filter/search-aware visible list, untrusted action-name hint label sanitization) |

Internal packages: `internal/action`, `internal/auth`, `internal/cenciwatch`, `internal/config`, `internal/debuglog`, `internal/git`, `internal/keymap`, `internal/provider`.

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
