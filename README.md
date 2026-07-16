# lazyboards

A terminal Kanban board inspired by [lazygit](https://github.com/jesseduffield/lazygit).

Built with [BubbleTea](https://github.com/charmbracelet/bubbletea) and [lipgloss](https://github.com/charmbracelet/lipgloss).

![lazyboards demo](docs/demo.gif)

## Features

- Vim-style navigation across columns and cards
- Split-pane layout: card list + detail panel with markdown rendering
- Edit cards in your editor with YAML frontmatter (title, labels, body)
- Card creation via modal form with label and assignee fields
- Assign and unassign collaborators to cards
- Search cards by title and filter by label or assignee
- PR linking with picker modal
- Custom actions: open URLs or run shell commands bound to Shift+key or multi-key sequences (neovim-style prefix keys), with column cleanup on departure
- Mouse support: scroll, click tabs, click cards
- Auto-detection of provider and repo from git remote
- In-app configuration UI (first-launch flow or press `c`)
- Board refresh (manual and periodic background refresh)
- Agent dispatch panel: enroll repos and trigger fleet-wide dispatch (`d`)
- Agents modal: every cenci-watch window in this instance's own tmux session — matched to a card or not — labeled by `session:index`, with `Enter` jumping to its tmux window (`w`), or jump straight to a card's own agent window with `s`
- Help popup with full keybinding reference (`?`)
- Error screen with retry support
- Responsive terminal resizing

## Contents

- [Install](#install)
- [Quick Start](#quick-start)
- [How It Works](#how-it-works)
- [Configuration](#configuration)
- [Editing Cards](#editing-cards)
- [Custom Actions](#custom-actions)
- [Keybindings](#keybindings)
- [Mouse Support](#mouse-support)
- [Build from Source](#build-from-source)
- [Releases](#releases)
- [License](#license)

## Install

```
go install github.com/matteobortolazzo/lazyboards@latest
```

Check the installed version:

```
lazyboards --version
```

## Quick Start

1. `cd` into a git repository with a GitHub remote
2. Run `lazyboards`
3. The provider and repo are auto-detected from your git remote

### Authentication

If you have the [GitHub CLI](https://cli.github.com/) installed, lazyboards uses your existing authentication automatically:

```
gh auth login
```

Alternatively, set a token manually:

```
export GITHUB_TOKEN=your_token_here
```

### First Launch

On first launch without a local config, an interactive configuration popup guides you through setup. You can also open it at any time with `c`.

## How It Works

Cards are GitHub issues. Each column maps to a label — an issue with the label "Implementing" appears in the Implementing column. When a card has multiple matching labels, it appears in the rightmost matching column. Cards without a matching label default to the first column.

Linked pull requests come from GitHub's closing-PR relationship: supported closing keywords such as `Fixes #123`, `Closes #123`, and `Resolves #123`, plus links added manually through GitHub's Development sidebar. Mere issue mentions are ignored, as are closed PRs. Press `p` to open a linked PR, or pick from multiple.

The board auto-refreshes in the background (default: every 5 minutes). Press `r` for an immediate refresh.

### Dispatch Panel

Press `d` to open the agent dispatch panel for the repo you're currently in. It shows whether the repo is enrolled with the cenci-watch daemon and lets you toggle enrollment with `Enter`.

Once a repo is enrolled, `o` triggers a dispatch run — but this is **fleet-wide**: it dispatches across *all* enrolled repos, not just the one currently open. The panel shows a summary of the last run (dispatched/skipped counts) after it completes.

The panel also shows a read-only "Loop" line reporting the daemon-owned background dispatch loop's state (off, on with its interval, daemon not running, no runs yet, or the last run's dispatched/skipped counts and any error) — lazyboards never starts or stops this loop itself. To start or stop the loop, configure a custom shell action that calls `cenci dispatch loop on`/`off` directly, for example in `~/.config/lazyboards/config.yml` (global) or `.lazyboards.yml` (per-project):

```yaml
actions:
  S: { name: Start dispatch loop, type: shell, command: "cenci dispatch loop on", scope: board }
  X: { name: Stop dispatch loop, type: shell, command: "cenci dispatch loop off", scope: board }
```

This split is deliberate and holds across the app: anything that continuously *displays* live cenci state (agent badges, this Loop line, the status-bar dispatch segment) is built in, while anything that *changes* cenci state (loop on/off, enroll) is yours to bind as a [custom action](#custom-actions). When the cenci-watch daemon connection is up (`cenci: true`), the Loop line updates live from the daemon's pushed state; on disconnect it falls back to the result of the last `cenci dispatch status` query made when the panel opened.

See the [Dispatch keybindings](#dispatch) for the full key reference.

### Example: cenci + cenci-watch

This walks through wiring lazyboards to a real [cenci-watch](https://github.com/matteobortolazzo/cenci/tree/main/watch) daemon from [cenci](https://github.com/matteobortolazzo/cenci), so cards move through `New` → `Refined` → `Planned` → `In Review` with agents doing the work.

1. **Install and run the daemon.** Use the [cenci installer](https://github.com/matteobortolazzo/cenci#readme) (`curl -fsSL https://raw.githubusercontent.com/matteobortolazzo/cenci/main/install.sh | bash`), then start the daemon once:

   ```
   cenci daemon &
   ```

   The daemon owns the broadcast socket that lazyboards' agent-status badges and dispatch panel both read from.

2. **Enroll the repo.** From inside the repo, either run `cenci dispatch enroll` yourself, or open lazyboards and press `d` then `Enter` — enrollment is idempotent either way, and only affects the currently open repo.

3. **Wire per-column actions to `cenci run`** in `~/.config/lazyboards/config.yml` (global) or `.lazyboards.yml` (per-project):

   ```yaml
   cenci: true
   session_max_length: 40 # matches cenci's window-name cap
   cleanup: "tmux kill-window -t ={window} 2>/dev/null || true"

   columns:
     - name: New
       actions:
         R: { name: Refine, type: shell, command: "cenci run refine {number} --model sonnet -- {comment}" }
     - name: Refined
       actions:
         D: { name: Design, type: shell, command: "cenci run design {number} --model sonnet -- {comment}" }
         I: { name: Implement, type: shell, command: "cenci run implement {number} --model sonnet -- {comment}" }
     - name: Planned
       actions:
         I: { name: Implement, type: shell, command: "cenci run implement {number} --model sonnet -- {comment}" }
     - name: In Review
       actions:
         W: { name: Open worktree, type: shell, scope: pr, command: 'tmux new-window -d -n pr-{pr_number} "cd {pr_worktree}"' }
   ```

   Pressing `R` on a `New` card runs `cenci run refine 42 -- <comment>` in a detached tmux window named `42-refine`. The live ▶/✓ badge matches that window by its `42-` prefix, and the top-level `cleanup` command reaps the window once the card leaves the column — see [Column Cleanup](#column-cleanup). When the agent's PR lands the card in `In Review`, `W` opens its worktree in a fresh tmux window so you can review and run it locally — append the project's run command (`ng serve`, `dotnet run`, …) in a per-project `.lazyboards.yml` (see [Action Scope](#action-scope)).

   Jumping to a card's agent window is built in — no custom action needed. Press `s` on a card to jump straight to its agent's tmux window (a picker opens if several windows match), or press `w` to open the full Agents modal listing every cenci-watch window.

4. **Let cenci pick up approved plans automatically.** Once a ticket reaches `Planned` with an approved `.plans/<id>-*.md` file, `cenci dispatch` will run it for you — fleet-wide, across every enrolled repo. Trigger a single pass from the panel with `o`, or start the recurring loop with the custom `cenci dispatch loop on` action described above. Tune concurrency, quiet hours, and per-agent budgets in cenci's own `dispatch` config block (`$XDG_CONFIG_HOME/cenci/config.json`) — see the [cenci README](https://github.com/matteobortolazzo/cenci/tree/main/watch#configuration-1) for the full reference.

## Configuration

Lazyboards auto-detects the provider and repository from your git remote. To override, create a `.lazyboards.yml` in your project root:

```yaml
provider: github
repo: owner/repo
```

### Global Config

Place shared settings in `~/.config/lazyboards/config.yml` for options that apply across all your projects. Local config (`.lazyboards.yml`) merges on top, with local values taking priority.

**Note:** `provider`, `repo`, and `project` are project-specific and cannot be set in global config — they come from `.lazyboards.yml` or git remote auto-detection.

**Note on `columns`:** scalar fields and the `actions` map merge across the two files, but the `columns` list does not — defining `columns` locally replaces the global list entirely (column order is the board layout, so it always comes from one file). To override a single column's actions or cleanup, re-list every column name locally; bare `- name:` entries still inherit that column's global actions and cleanup, so nothing else needs restating (see [Column-Specific Actions](#column-specific-actions)).

### Config Reference

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `provider` | string | *(auto-detected)* | `github` (local config only) |
| `repo` | string | *(auto-detected)* | `owner/repo` (local config only) |
| `refresh_interval` | int | `5` | Minutes between auto-refresh (`0` to disable) |
| `action_refresh_delay` | int | `5` | Seconds before refresh after a shell action (`0` to disable) |
| `session_max_length` | int | `40` | Max characters for the `{session}` template variable |
| `working_label` | string | `"Working"` | Label that shows a working indicator on cards |
| `mouse` | bool | `true` | Enable mouse support |
| `cenci` | bool | `true` | Enable live agent status badges + status-bar counts (requires the cenci-watch daemon; silently off when absent) |
| `cleanup` | string | — | Default cleanup command applied to every column that doesn't set its own (see [Column Cleanup](#column-cleanup)) |
| `columns` | list | `[New, Refined, Implementing]` | Column definitions (name, actions, cleanup) |
| `actions` | map | — | Global custom actions (see [Custom Actions](#custom-actions)) |

## Editing Cards

Press `e` to edit the selected card in your editor (`$VISUAL`, `$EDITOR`, or `vi`). The card opens as a temporary file with YAML frontmatter:

```yaml
---
title: Fix login timeout
labels: bug, urgent
---
The login page times out after 30 seconds when...
```

Save and close to apply changes. Leave the title blank to cancel. If you add labels that don't exist yet, lazyboards prompts you to create them.

## Custom Actions

Bind uppercase keys (A-Z) to URL or shell actions in your config. The uppercase namespace is fully yours — no built-in ever claims an uppercase key in normal mode (the built-in git shortcuts live inside the [Git Menu](#git-menu)). Custom actions are also the designated home for commands that change cenci state, like `cenci dispatch loop on`/`off` (see the [Dispatch Panel](#dispatch-panel)):

```yaml
actions:
  O:
    name: Open issue
    type: url
    url: "https://github.com/{repo_owner}/{repo_name}/issues/{number}"
  B:
    name: Branch
    type: shell
    command: "git checkout -b {number}-{title}"
```

Press the key to execute the action on the selected card. Custom actions and `Alt+Shift+key` comment actions work identically whether the card list or the [detail panel](#detail-panel) is focused.

### Key Sequences (Prefix Keys)

When single keys run out — a monorepo where you want to run several projects from a PR, say — bind multi-key sequences, neovim-style. A key is a sequence when it's longer than one character: the first key must still be uppercase `A-Z` (the namespace reserved for you), continuation keys can be any letter or digit:

```yaml
actions:
  Rf:
    name: "Run frontend"
    type: shell
    scope: pr
    command: 'tmux new-window -d -n fe-{pr_number} "cd {pr_worktree}/frontend && npm run dev"'
  Rb:
    name: "Run backend"
    type: shell
    scope: pr
    command: 'tmux new-window -d -n be-{pr_number} "cd {pr_worktree}/backend && dotnet run"'
  Rw:
    name: "Run worker"
    type: shell
    scope: pr
    command: 'tmux new-window -d -n wk-{pr_number} "cd {pr_worktree}/worker && go run ."'
```

Press `R` and the status bar switches to a which-key style list of everything the prefix can complete to (`Rf: Run frontend | Rb: Run backend | ...`); press the next key to run it. While a sequence is pending it owns the keyboard — built-in keys like `j`/`k` act as continuation keys, not navigation. `Esc` cancels, as does any key that doesn't match a bound sequence. Holding `Alt` on any key of the sequence gives the same [comment-first flow](#comment-mode) as `Alt+Shift+key` on a single-key action.

Sequences can be any length (`R`, `Rf`, `RFa1`, ...) and follow all the usual action rules: scopes, template variables, per-column overrides, and gating (a prefix whose only completions are `pr`-scope won't even open on a card with no linked PRs). One constraint is validated at startup: a key can't be a strict prefix of another key that can be active at the same time — a standalone `P` action plus a `Pf` sequence is a config error, because `P` could then never fire.

### Template Variables

| Variable | Scope | Description |
|----------|-------|-------------|
| `{number}` | card, pr | Issue number |
| `{title}` | card, pr | Slugified title (lowercase, hyphens) |
| `{tags}` | card, pr | Comma-separated labels |
| `{session}` | card, pr | `{number}-{title}`, capped at `session_max_length` |
| `{window}` | card, pr | Live cenci window name for the card (joined by ticket-number prefix), falling back to `{session}` when no agent window is live |
| `{comment}` | all | User-entered comment (see [Comment Mode](#comment-mode)) |
| `{repo_owner}` | all | Repository owner |
| `{repo_name}` | all | Repository name |
| `{provider}` | all | Provider name (e.g., `github`) |
| `{pr_branch}` | pr | Linked PR's branch name |
| `{pr_number}` | pr | Linked PR's number |
| `{pr_url}` | pr | Linked PR's URL |
| `{pr_title}` | pr | Slugified linked PR title (lowercase, hyphens) |
| `{pr_worktree}` | pr | Absolute path of the registered Git worktree for the linked PR's branch |

Shell commands automatically escape template variables with POSIX single quotes to prevent injection.

`{pr_branch}`, `{pr_number}`, `{pr_url}`, `{pr_title}`, and `{pr_worktree}` are only available in `pr`-scope actions — using them in a `card`- or `board`-scope action is a config validation error. `{pr_worktree}` is resolved from `git worktree list` when the action runs; if the PR branch has no registered local worktree, the action reports an error instead of running from an unintended directory.

Actions that include `{comment}` support the same [Comment Mode](#comment-mode) first-then-run flow regardless of scope: press the key, type a comment, submit — then the action's normal scope resolution runs (immediate for `card`/`board`, and for `pr` immediate with 1 linked PR or via the PR picker with 2+).

### Action Scope

Actions default to `scope: "card"` (operate on the selected card). Set `scope: "board"` for actions that don't need a selected card — board-scope actions cannot use card-specific variables (`{number}`, `{title}`, `{tags}`, `{session}`, `{window}`) or PR-specific variables (`{pr_branch}`, `{pr_number}`, `{pr_url}`, `{pr_title}`, `{pr_worktree}`).

Set `scope: "pr"` for actions that operate on a card's linked pull request — a stricter cousin of `card` scope that additionally requires the selected card to have at least one linked PR. With 0 linked PRs the action is unavailable (no-op, absent from hints). With exactly 1 linked PR it runs immediately against that PR's data. With 2+ linked PRs it opens the same PR-picker modal used by the built-in `p` key; selecting a PR runs the action against that PR's data. `pr`-scope actions can use both card-specific variables (`{number}`, `{title}`, `{tags}`, `{session}`, `{window}`) and the [PR-specific template variables](#template-variables) (`{pr_branch}`, `{pr_number}`, `{pr_url}`, `{pr_title}`, `{pr_worktree}`).

A typical PR action opens the card's worktree and runs the project, so reviewing a PR is one keypress on the card:

```yaml
columns:
  - name: In Review
    actions:
      W:
        name: Run worktree
        type: shell
        scope: pr
        command: 'tmux new-window -d -n pr-{pr_number} "cd {pr_worktree} && ng serve"'
```

Swap `ng serve` for whatever the project runs — `dotnet run`, `npm run dev`, `go run .`, `make dev`. `{pr_worktree}` finds the PR branch's registered Git worktree, so the action does not depend on a worktree directory naming convention. Since the run command is project-specific, define it in that project's `.lazyboards.yml`; a global `~/.config/lazyboards/config.yml` can keep a command-agnostic variant (open the worktree only, no run step) that works everywhere.

Long-running or foreground shell commands will block that action's key slot until the command exits. Prefer a self-detaching command such as `tmux new-window -d '<command>'` for anything long-running (like the `ng serve` example above) — see [Tmux Integration](#tmux-integration).

### Git Menu

Inside a git repository with a remote, press `g` to open the **Git Menu** — six built-in board-scope git shortcuts with lazygit-style keys, no config required:

| Key | Action | Command |
|-----|--------|---------|
| `P` | Push | `git push` |
| `p` | Pull (rebase) | `git pull --rebase` |
| `f` | Fetch | `git fetch` |
| `m` | Mergetool | `git mergetool` |
| `s` | Stash push | `git stash push` |
| `S` | Stash pop | `git stash pop` |

Inside the menu, press an action's key to run it immediately (like lazygit), or navigate with `j`/`k` and press `Enter`; `Esc` cancels. The keys are scoped to the menu: they do nothing in normal mode, so the normal-mode uppercase namespace stays fully reserved for your [custom actions](#custom-actions) (a custom `P` and the menu's Push coexist without conflict). The menu is also listed in the `?` help popup and only opens inside a git repo.

### Status Bar Prefix (agent + PR counts)

At the left of the status bar, an always-visible prefix summarizes the whole repository: agent-status counts (`▶N` running, `!N` awaiting input) followed by the repo-wide open-PR total (` N`, using the same PR glyph shown on cards). Each token is omitted when its count is zero, and the prefix disappears entirely when all are zero. Because the prefix is reserved before anything else, it stays visible through timed status messages and is never truncated to make room for hints or the right-aligned git/dispatch segments.

The agent counts cover every window the cenci-watch daemon tracks — across all tmux sessions, whether or not a window's name joins to a card on the board. (The `w` agents modal, by contrast, is scoped to this instance's own tmux session, so its row count may be smaller than the status-bar total.) The PR total counts every open PR in the repository — the same set the `v` [open-PR list](#pull-requests) shows — not just PRs linked to cards. Until the first repo-wide listing succeeds (or if it isn't available), the PR token falls back to the card-linked sum; afterwards a failed refresh keeps the last known total rather than dropping the token.

### Git Status Segment

Inside a git repository with a remote, the status bar shows a compact, right-aligned, plain-ASCII git segment: current branch, staged/unstaged file counts, and commits ahead/behind upstream — e.g. `main +2~1 ↑3↓0`. The `↑N↓N` portion is omitted when the branch has no upstream configured. The segment is hidden entirely (no error shown) outside a git repo, when there's no remote, or on narrow terminals where there isn't room — hints always keep priority over the segment.

The segment refreshes on board start, after every board refresh, after any successful action, and on a background poll every ~12s to catch changes made outside the app.

### Dispatch Status Segment

When the cenci-watch daemon reports the background dispatch loop enabled, the status bar shows a `⟳ dispatch` segment, right-aligned to the left of the git segment (see [Git Status Segment](#git-status-segment) for priority rules — the dispatch segment is dropped first on narrow terminals). It's sourced live from the same watcher subscription that drives agent-status badges, so it appears and disappears immediately as the loop is toggled or the daemon becomes unreachable — no restart needed. If the last dispatch pass failed, the segment renders in red instead of its normal color. A single transient watcher reconnect blip is tolerated and does not clear the segment; it only clears after a second consecutive watcher error with no successful reconnect in between.

Set `LAZYBOARDS_DEBUG_LOG=<path>` to append watcher connection errors (including tolerated blips) to a file at `<path>`, one timestamped line per error — useful for diagnosing daemon connectivity issues. Unset (the default), this is a complete no-op: no file is created and there's no overhead.

### Column-Specific Actions

Define actions under a column to override global actions for that column:

```yaml
columns:
  - name: New
    actions:
      R:
        name: Refine ticket
        type: shell
        command: 'tmux new-window -d -n {session} "claude --comment {comment}"'
  - name: Refined
```

Within one column, local and global actions merge by key: local keys win, global-only keys are kept, and a bare `- name:` entry (no `actions`) inherits the matching global column's actions in full (columns match by name, case-insensitively). An explicit empty `actions: {}` disables all actions for that column. But remember the list itself doesn't merge — a local `columns:` replaces the global list, so re-list every column you want to keep (see [Global Config](#global-config)).

### Column Cleanup

Run a command automatically when a card leaves a column (detected on board refresh):

```yaml
columns:
  - name: New
    cleanup: 'tmux kill-window -t {window} 2>/dev/null || true'
  - name: Refined
```

The `cleanup` command uses the same template variables as actions. It runs when a card moves to another column or disappears.

If you're running cenci, prefer `cenci close {number}` over a raw `tmux kill-window`:

```yaml
cleanup: 'cenci close {number}'
```

`cenci close` asks the daemon for the window's exact `session:index` target instead of guessing a name, so it reaps the right window regardless of which tmux session it's running in. It also refuses to kill a window whose agent is still `running` or waiting for input (unless passed `--force`), exits non-zero without touching tmux if the daemon is unreachable, and exits `0` when no window matches (safe to run even if the agent already finished). No `|| true` needed.

`tmux kill-window -t {window}` still works, but has a sharp edge: a bare window name is resolved by tmux **only within lazyboards' own tmux session**. If the agent's window lives in a different session, the kill silently no-ops; if you run one lazyboards instance per session, each instance only ever reaps windows in its own session. Prefer `{window}` over `{session}` for this target — cenci names dispatched windows `{number}-{skill}` (e.g. `230-refine`), not the reconstructed `{session}` name — but be aware of the cross-session limitation either way.

Set a top-level `cleanup` to apply the same command to every column that doesn't define its own:

```yaml
cleanup: 'tmux kill-window -t {window} 2>/dev/null || true'
columns:
  - name: New
  - name: Refined
    cleanup: ''                          # explicitly disables cleanup for this column
  - name: Implementing
    cleanup: 'docker stop {window}'      # overrides the top-level default
```

A column's own `cleanup` (including an explicit empty string) always wins over the top-level default. Global and local config follow the usual precedence: a local top-level `cleanup` overrides global, and omitting it locally inherits the global value.

### Comment Mode

Actions that include `{comment}` in their template can be triggered with `Alt+Shift+key` to open a text input first:

```yaml
actions:
  A:
    name: Annotate
    type: shell
    command: 'gh issue comment {number} --body {comment}'
```

Press `A` to run with an empty comment. Press `Alt+Shift+A` to type a comment first, then `Enter` to submit.

### Tmux Integration

Open a new tmux window for each card:

```yaml
actions:
  T:
    name: Tmux window
    type: shell
    command: "tmux new-window -d -n {session}"
```

The `{session}` variable generates a tmux-friendly name (e.g., `42-fix-login-bug`), capped at `session_max_length` (default: 40). Punctuation and non-ASCII characters in the title are dropped (not hyphenated).

Agent-status matching (the live ▶/✓/… badges) does **not** rely on this name. Cards join cenci windows by **ticket-number prefix**: a card matches a window whose name is exactly the card number or starts with `<number>-` (cenci names dispatched windows `<number>-<skill>`, e.g. `230-refine`). The `-` boundary keeps card #23 from matching `230-…`, and the scheme is backward-compatible with cenci's older `<number>-<title-slug>` names.

Use `{window}` (not `{session}`) when an action or `cleanup` command needs to target that live cenci window by name — for example `tmux kill-window -t {window}` to reap it. `{session}` still generates the reconstructed name above and is the right choice for actions that create a window before cenci has dispatched one.

### Action Refresh Delay

After a shell action completes, the board automatically refreshes after a delay. Configure in seconds:

```yaml
action_refresh_delay: 10  # default: 5, set to 0 to disable
```

## Keybindings

Press `?` at any time to open the in-app help popup.

### Normal Mode

| Key | Action |
|-----|--------|
| `?` | Help |
| `q` | Quit |
| `Ctrl+C` | Force quit |
| `n` | New card |
| `e` | Edit card |
| `c` | Configuration |
| `o` | Open ticket |
| `r` | Refresh board |
| `p` | Open PR |
| `x` | Close card (with confirmation) |
| `t` | Delete card permanently (with two-step confirmation) |
| `v` | Open PRs (all open PRs in the repo) |
| `w` | Agents (cenci-watch windows in this instance's tmux session, labeled `session:index`; `Enter` jumps to the tmux window) |
| `s` | Go to agent (jumps straight to the selected card's agent window in this session when there's exactly one; opens a picker when there are several) |
| `/` | Search |
| `a` | Assign collaborator |
| `g` | Git menu |
| `d` | Dispatch |
| `f` | Filter (toggle) |
| `l` / `→` | Detail panel |
| `j` / `↓` | Next card |
| `k` / `↑` | Previous card |
| `Tab` / `Shift+Tab` | Switch columns |
| `1-9` | Jump to column |
| `A-Z` | Custom action (uppercase is fully user-owned) |
| `A-Z` `…` | Custom action [key sequence](#key-sequences-prefix-keys) (`Esc` cancels a pending sequence) |
| `Alt+Shift+key` | Comment action |

### Detail Panel

| Key | Action |
|-----|--------|
| `e` | Edit card |
| `j` / `k` | Scroll body |
| `h` / `←` / `Esc` | Back to card list |
| `Tab` / `Shift+Tab` | Switch columns |
| `o` | Open ticket |
| `p` | Open PR |
| `r` | Refresh |
| `q` | Quit |
| `?` | Help |
| `A-Z` | Custom action |
| `A-Z` `…` | Custom action [key sequence](#key-sequences-prefix-keys) |
| `Alt+Shift+key` | Comment action |

### Create Mode

| Key | Action |
|-----|--------|
| `Esc` | Cancel |
| `Tab` | Next field |
| `←` / `→` | Cycle assignee |
| `Enter` | Submit |

### Config Mode

| Key | Action |
|-----|--------|
| `Esc` | Cancel (quit on first launch) |
| `Tab` | Next field |
| `←` / `→` | Cycle provider |
| `Enter` | Save |

### PR Picker

| Key | Action |
|-----|--------|
| `←` / `→` | Cycle PR |
| `Enter` | Select |
| `Esc` | Cancel |

### Pull Requests

Opened with `v` from normal mode. Lists every **open PR in the repository**,
not just those linked to a board card. While the repo-wide fetch is in
flight, the card-linked PRs (aggregated across all columns and cards,
regardless of any active search/filter) render immediately as a fallback; if
the fetch fails, that fallback is kept with an explicit note. PRs linked to
a card show the owning column and card next to the title; unlinked PRs are
listed plainly.

Uppercase keys run your global `scope: pr` [custom actions](#custom-actions)
against the selected PR, with the same template variables as a normal-mode
dispatch. On a PR with no linked card, the card-derived variables
(`{number}`, `{title}`, `{tags}`, `{session}`, `{window}`) expand to empty
strings. Per-column action overrides and the `Alt` comment variant are not
available inside the modal.

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate |
| `Enter` | Open selected PR |
| `A-Z` | Custom action (`scope: pr`) on selected PR |
| `Esc` | Cancel |

### Agents

`w` always opens the modal, listing every cenci-watch window in this
instance's own tmux session (labeled `session:index`) regardless of whether
it matches a card. `s` is a smart jump scoped to the selected card: zero
matching windows shows a status message, exactly one switches the tmux
client directly (no modal), and several open this same modal scoped to just
that card's windows.

| Key | Action |
|-----|--------|
| `w` | Open (every cenci-watch window in this instance's tmux session) |
| `s` | Open, scoped to the selected card (from normal mode; only when it has several agent windows) |
| `j` / `k` | Navigate |
| `Enter` | Go to tmux window |
| `Esc` | Cancel |

### Comment Mode

| Key | Action |
|-----|--------|
| `Esc` | Cancel |
| `Enter` | Submit |

### Delete

Opened with `t` from normal mode. Permanently deletes the selected card via
the provider (not a column move) after a two-step confirmation. Cards with
any linked PR cannot be deleted — the status bar shows an error and the card
list stays unchanged. Step 1 accepts an optional comment (blank is fine);
step 2 requires retyping the card's number exactly before the delete fires.
A mismatched retype shows an inline error and stays on the confirm step;
`Esc` at either step cancels the whole flow, discarding any comment typed.

| Key | Action |
|-----|--------|
| `Enter` (comment step) | Continue to confirm step |
| `Enter` (confirm step) | Confirm delete (must match the card number) |
| `Esc` | Cancel |

### Close Confirm

Opened with `x` from normal mode. A lighter one-step confirmation than
Delete — it moves the card to the closed state via the provider rather than
deleting it outright.

| Key | Action |
|-----|--------|
| `y` | Confirm close |
| `n` / `Esc` | Cancel |

### Label Confirm

Entered automatically after saving an [edited card](#editing-cards) that adds
labels lazyboards doesn't already know about. Confirms one unknown label at a
time; once every unknown label is resolved, the edit is applied.

| Key | Action |
|-----|--------|
| `y` | Create this label, continue to the next unknown label (or apply the edit if none remain) |
| `n` | Cancel the whole edit |
| `Esc` | Cancel the whole edit |

### Filter

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate |
| `Enter` | Select |
| `Esc` | Cancel |

### Search

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate results |
| `Ctrl+N` / `Ctrl+P` | Navigate results |
| `Tab` / `Shift+Tab` | Exit search and switch columns |
| `Enter` | Apply search |
| `Esc` | Clear search |

All letters and digits type into the query (queries match titles, labels, and card numbers).

### Assign

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate |
| `Enter` | Toggle assignee |
| `Esc` | Cancel |

### Git Menu

Opened with `g` from normal mode.

| Key | Action |
|-----|--------|
| `g` | Open (from normal mode) |
| `P` | Push |
| `p` | Pull (rebase) |
| `f` | Fetch |
| `m` | Mergetool |
| `s` | Stash push |
| `S` | Stash pop |
| `j` / `k` | Navigate |
| `Enter` | Run selected |
| `Esc` | Cancel |

### Dispatch

Opened with `d` from normal mode. See [Dispatch Panel](#dispatch-panel) for
what enrollment and a dispatch run actually do.

| Key | Action |
|-----|--------|
| `d` | Open (from normal mode) |
| `Enter` | Enroll/Unenroll current repo |
| `o` | Dispatch once (all enrolled repos) |
| `Esc` | Close |

### Error Mode

| Key | Action |
|-----|--------|
| `r` | Retry loading |
| `q` | Quit |

## Mouse Support

Mouse support is enabled by default. Disable it in your config:

```yaml
mouse: false
```

- **Scroll wheel** on card list: navigate up/down
- **Scroll wheel** on detail panel: scroll body
- **Click** column tabs: switch columns
- **Click** a card: select it

## Build from Source

Requires Go 1.25 or later.

```
git clone https://github.com/matteobortolazzo/lazyboards.git
cd lazyboards
go build
```

Run tests:

```
go test ./...
```

## Releases

Releases are cut automatically. Every push to `main` runs the **Version Bump**
workflow, which computes the next [semantic version](https://semver.org) from
the latest `v*` tag and the triggering commit's [conventional-commit](https://www.conventionalcommits.org)
type:

| Commit | Bump |
|--------|------|
| `feat!:` / `<type>(scope)!:` / `BREAKING CHANGE` | major |
| `feat:` / `feat(scope):` | minor |
| anything else (`fix`, `docs`, `chore`, …) | patch |

It then tags the commit and dispatches the **Release** workflow, which builds
cross-platform archives with GoReleaser (injecting the exact version via
`-ldflags -X main.version=…`) and publishes a GitHub Release. The running
binary reports its version with `lazyboards --version`.

## License

[MIT](LICENSE)
