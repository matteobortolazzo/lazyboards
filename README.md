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
- Custom actions: open URLs or run shell commands bound to Shift+key, with column cleanup on departure
- Mouse support: scroll, click tabs, click cards
- Auto-detection of provider and repo from git remote
- In-app configuration UI (first-launch flow or press `c`)
- Board refresh (manual and periodic background refresh)
- Agent dispatch panel: enroll repos and trigger fleet-wide dispatch (`d`)
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

Linked pull requests are auto-detected from the GitHub issue timeline (cross-references). Press `p` to open a linked PR, or pick from multiple.

The board auto-refreshes in the background (default: every 5 minutes). Press `r` for an immediate refresh.

### Dispatch Panel

Press `d` to open the agent dispatch panel for the repo you're currently in. It shows whether the repo is enrolled with the agentwatch daemon and lets you toggle enrollment with `Enter`.

Once a repo is enrolled, `o` triggers a dispatch run — but this is **fleet-wide**: it dispatches across *all* enrolled repos, not just the one currently open. The panel shows a summary of the last run (dispatched/skipped counts) after it completes.

The panel also shows a read-only "Loop" line reporting the daemon-owned background dispatch loop's state (off, on with its interval, daemon not running, no runs yet, or the last run's dispatched/skipped counts and any error) — lazyboards never starts or stops this loop itself. To start or stop the loop, configure a custom shell action that calls `agentwatch dispatch loop on`/`off` directly, for example in `~/.config/lazyboards/config.yml` (global) or `.lazyboards.yml` (per-project):

```yaml
actions:
  S: { name: Start dispatch loop, type: shell, command: "agentwatch dispatch loop on", scope: board }
  X: { name: Stop dispatch loop, type: shell, command: "agentwatch dispatch loop off", scope: board }
```

See the [Dispatch keybindings](#dispatch) for the full key reference.

### Example: agentwatch + agent-stack

This walks through wiring lazyboards to a real [agentwatch](https://github.com/matteobortolazzo/agent-stack/tree/main/agentwatch) daemon from [agent-stack](https://github.com/matteobortolazzo/agent-stack), so cards move through `New` → `Refined` → `Planned` → `In Review` with agents doing the work.

1. **Install and run the daemon.** Use the [agent-stack installer](https://github.com/matteobortolazzo/agent-stack#readme) (`curl -fsSL https://raw.githubusercontent.com/matteobortolazzo/agent-stack/main/install.sh | bash`), then start the daemon once:

   ```
   agentwatch daemon &
   ```

   The daemon owns the broadcast socket that lazyboards' agent-status badges and dispatch panel both read from.

2. **Enroll the repo.** From inside the repo, either run `agentwatch dispatch enroll` yourself, or open lazyboards and press `d` then `Enter` — enrollment is idempotent either way, and only affects the currently open repo.

3. **Wire per-column actions to `agentwatch run`** in `~/.config/lazyboards/config.yml` (global) or `.lazyboards.yml` (per-project):

   ```yaml
   agentwatch: true
   session_max_length: 40 # matches agentwatch's window-name cap
   cleanup: "tmux kill-window -t ={window} 2>/dev/null || true"

   actions:
     G: { name: Jump to agent, type: shell, command: 'tmux switch-client -t "={window}"' }

   columns:
     - name: New
       actions:
         R: { name: Refine, type: shell, command: "agentwatch run refine {number} --model sonnet -- {comment}" }
     - name: Refined
       actions:
         D: { name: Design, type: shell, command: "agentwatch run design {number} --model sonnet -- {comment}" }
         I: { name: Implement, type: shell, command: "agentwatch run implement {number} --model sonnet -- {comment}" }
     - name: Planned
       actions:
         I: { name: Implement, type: shell, command: "agentwatch run implement {number} --model sonnet -- {comment}" }
     - name: In Review
   ```

   Pressing `R` on a `New` card runs `agentwatch run refine 42 -- <comment>` in a detached tmux window named `42-refine`. The live ▶/✓ badge matches that window by its `42-` prefix, the `G` custom action above jumps straight to it (via `{window}`, the live agentwatch window name), and the top-level `cleanup` command reaps the window once the card leaves the column — see [Column Cleanup](#column-cleanup).

4. **Let agentwatch pick up approved plans automatically.** Once a ticket reaches `Planned` with an approved `.plans/<id>-*.md` file, `agentwatch dispatch` will run it for you — fleet-wide, across every enrolled repo. Trigger a single pass from the panel with `o`, or start the recurring loop with the custom `agentwatch dispatch loop on` action described above. Tune concurrency, quiet hours, and per-agent budgets in agentwatch's own `dispatch` config block (`$XDG_CONFIG_HOME/agentwatch/config.json`) — see the [agentwatch README](https://github.com/matteobortolazzo/agent-stack/tree/main/agentwatch#configuration-1) for the full reference.

## Configuration

Lazyboards auto-detects the provider and repository from your git remote. To override, create a `.lazyboards.yml` in your project root:

```yaml
provider: github
repo: owner/repo
```

### Global Config

Place shared settings in `~/.config/lazyboards/config.yml` for options that apply across all your projects. Local config (`.lazyboards.yml`) merges on top, with local values taking priority.

**Note:** `provider`, `repo`, and `project` are project-specific and cannot be set in global config — they come from `.lazyboards.yml` or git remote auto-detection.

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
| `agentwatch` | bool | `true` | Enable live agent status badges + status-bar counts (requires the agentwatch daemon; silently off when absent) |
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

Bind uppercase keys (A-Z) to URL or shell actions in your config. The uppercase namespace is fully yours — no built-in ever claims an uppercase key in normal mode (the built-in git shortcuts live inside the [Git Menu](#git-menu)):

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

Press the key to execute the action on the selected card.

### Template Variables

| Variable | Scope | Description |
|----------|-------|-------------|
| `{number}` | card | Issue number |
| `{title}` | card | Slugified title (lowercase, hyphens) |
| `{tags}` | card | Comma-separated labels |
| `{session}` | card | `{number}-{title}`, capped at `session_max_length` |
| `{window}` | card | Live agentwatch window name for the card (joined by ticket-number prefix), falling back to `{session}` when no agent window is live |
| `{comment}` | both | User-entered comment (see [Comment Mode](#comment-mode)) |
| `{repo_owner}` | both | Repository owner |
| `{repo_name}` | both | Repository name |
| `{provider}` | both | Provider name (e.g., `github`) |

Shell commands automatically escape template variables with POSIX single quotes to prevent injection.

### Action Scope

Actions default to `scope: "card"` (operate on the selected card). Set `scope: "board"` for actions that don't need a selected card — board-scope actions cannot use card-specific variables (`{number}`, `{title}`, `{tags}`, `{session}`, `{window}`).

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

### Git Status Segment

Inside a git repository with a remote, the status bar shows a compact, right-aligned, plain-ASCII git segment: current branch, staged/unstaged file counts, and commits ahead/behind upstream — e.g. `main +2~1 ↑3↓0`. The `↑N↓N` portion is omitted when the branch has no upstream configured. The segment is hidden entirely (no error shown) outside a git repo, when there's no remote, or on narrow terminals where there isn't room — hints always keep priority over the segment.

The segment refreshes on board start, after every board refresh, after any successful action, and on a background poll every ~12s to catch changes made outside the app.

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

### Column Cleanup

Run a command automatically when a card leaves a column (detected on board refresh):

```yaml
columns:
  - name: New
    cleanup: 'tmux kill-window -t {window} 2>/dev/null || true'
  - name: Refined
```

The `cleanup` command uses the same template variables as actions. It runs when a card moves to another column or disappears. Prefer `{window}` over `{session}` for reaping tmux windows — agentwatch names dispatched windows `{number}-{skill}` (e.g. `230-refine`), not the reconstructed `{session}` name, so `{window}` is the target that actually matches the live window (falling back to `{session}` when no agent window is live).

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

Agent-status matching (the live ▶/✓/… badges) does **not** rely on this name. Cards join agentwatch windows by **ticket-number prefix**: a card matches a window whose name is exactly the card number or starts with `<number>-` (agentwatch names dispatched windows `<number>-<skill>`, e.g. `230-refine`). The `-` boundary keeps card #23 from matching `230-…`, and the scheme is backward-compatible with agentwatch's older `<number>-<title-slug>` names.

Use `{window}` (not `{session}`) when an action or `cleanup` command needs to target that live agentwatch window by name — for example `tmux kill-window -t {window}` to reap it. `{session}` still generates the reconstructed name above and is the right choice for actions that create a window before agentwatch has dispatched one.

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

### Create Mode

| Key | Action |
|-----|--------|
| `Esc` | Cancel |
| `Tab` | Next field |
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

### Comment Mode

| Key | Action |
|-----|--------|
| `Esc` | Cancel |
| `Enter` | Submit |

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

| Key | Action |
|-----|--------|
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

| Key | Action |
|-----|--------|
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
