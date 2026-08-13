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
- Search cards by title and filter by label, assignee, or milestone
- PR linking with picker modal
- Milestones modal: every open milestone in the repository with progress bar, counts, and due date, `Enter` to filter the board by milestone (`m`)
- Custom actions: open URLs or run shell commands bound to any key (not just Shift+key — see [Keymaps](#keymaps)) or multi-key sequences (neovim-style prefix keys), with column cleanup on departure
- Mouse support: scroll, click tabs, click cards
- Auto-detection of provider and repo from git remote
- In-app configuration UI (first-launch flow or press `c`)
- Board refresh (manual and periodic background refresh)
- Agent dispatch panel: enroll repos and trigger fleet-wide dispatch (`D`)
- Agents modal: every cenci-watch window in this instance's own tmux session — matched to a card or not — labeled by `session:index`, with `Enter` jumping to its tmux window (`A`), or jump straight to a card's own agent window with `g a`
- Help popup with full keybinding reference (`?`)
- Error screen with retry support
- Responsive terminal resizing

## Contents

- [Install](#install)
- [Quick Start](#quick-start)
- [How It Works](#how-it-works)
- [Configuration](#configuration)
  - [Keymaps](#keymaps)
  - [Trust Model](#trust-model)
- [Editing Cards](#editing-cards)
- [Custom Actions](#custom-actions)
- [Keybindings](#keybindings)
  - [Keybinding migration](#keybinding-migration)
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

Saving from the popup writes the local config and reloads the board against the repository you entered — switching repositories takes effect immediately, no restart needed. The repository must be given as `owner/repo`; anything else is rejected before it reaches the config file.

## How It Works

Cards are GitHub issues. Each column maps to a label — an issue with the label "Implementing" appears in the Implementing column. When a card has multiple matching labels, it appears in the rightmost matching column. Cards without a matching label default to the first column.

Linked pull requests come from two sources, unioned: GitHub's closing-PR relationship (supported closing keywords such as `Fixes #123`, `Closes #123`, and `Resolves #123`, plus links added manually through GitHub's Development sidebar) and any other open PR that mentions the issue number anywhere (e.g. `Related to #123`, or this project's own `Stack: 2/3 — depends on #123`). Closed or merged PRs are excluded from mentions so a stale reference doesn't leave a dead link behind. Press `p` to open a linked PR, or pick from multiple.

Each linked PR renders on its own line beneath the card title, prefixed with the purple  PR marker followed by a glyph colored by that PR's status: gray `●` draft, green `✓` mergeable, red `✗` conflicting, yellow `●` checks still running, yellow `!` blocked (behind base or needs review). A card with several linked PRs shows one line per PR — each with its own status, not a single collapsed worst-of-all glyph — so stacked PRs stay individually visible. While GitHub is still computing a PR's mergeability, the glyph keeps its neutral color rather than guessing. Live agent windows render the same way: one line per non-idle window beneath the title. The same purple-prefixed, colored glyph prefixes each row in the PR list (`P`) modal; the PR picker (`p`) modal shows the same status glyph without the purple marker, blank there instead of neutral when status isn't known yet. These status glyphs keep their full color on every card and every PR list row, focused or not, so a glance across the board shows which tickets have an agent or a linked PR attached. Muting applies to row *text*: a non-selected card title, and every non-selected row across the PR list, filter, assignee, git menu, and agents list modals, renders gray; the currently selected/focused row stays bold white.

If a card has GitHub's native sub-issue relationships, they render as muted gray status lines above the agent/PR lines: a `󰙅` line shows a parent card's completed/total sub-issue count (e.g. `󰙅 2/3`), and a `󱞫` line shows a child card's parent issue number (e.g. `󱞫 #12`). A card that's both a parent and a child shows both lines, parent first; a card with neither relationship shows no extra line. The full status-line order beneath a card's title is: sub-issue line(s), then agent line(s), then PR line(s). Sub-issue lines are gray by design on every card. Agent badges, PR status glyphs, and the per-label color dots next to a card title keep their assigned colors on non-focused cards too — only the title text mutes — preserving cross-card color-coded scanning.

The board auto-refreshes in the background (default: every 5 minutes). Press `r` for an immediate refresh.

### Dispatch Panel

Press `D` to open the agent dispatch panel for the repo you're currently in. It shows whether the repo is enrolled with the cenci-watch daemon and lets you toggle enrollment with `Enter`.

Once a repo is enrolled, `o` triggers a dispatch run — but this is **fleet-wide**: it dispatches across *all* enrolled repos, not just the one currently open. The panel shows a summary of the last run (dispatched/skipped counts) after it completes.

The panel also shows a "Loop" line reporting the daemon-owned background dispatch loop's state (off, on with its interval, daemon not running, no runs yet, or the last run's dispatched/skipped counts and any error). Press `l` to toggle the loop on or off. Because the loop is **fleet-wide and persistent** — it keeps dispatching across *every* enrolled repo on a timer until turned off — the toggle asks for confirmation in both directions (`y` to confirm, `n`/`Esc` to cancel) and names the blast radius before you commit. The toggle is offered only when cenci can report the loop's current state; against a cenci binary too old to report it, the panel shows the Loop line without the `l` affordance.

This split is deliberate and holds across the app: continuously *displaying* live cenci state (agent badges, this Loop line, the status-bar dispatch segment) is built in, and so are the dispatch panel's own mutating controls (enroll/unenroll, dispatch-once, and the loop toggle) — each acts on cenci's own repo/fleet state through a single modal key. Anything else that shells out to cenci with app-specific templating is yours to bind as a [custom action](#custom-actions). When the cenci-watch daemon connection is up (`cenci: true`), the Loop line updates live from the daemon's pushed state; on disconnect it falls back to the result of the last `cenci dispatch status` query made when the panel opened.

See the [Dispatch keybindings](#dispatch-cenci) for the full key reference.

### Example: cenci + cenci-watch

This walks through wiring lazyboards to a real [cenci-watch](https://github.com/matteobortolazzo/cenci/tree/main/watch) daemon from [cenci](https://github.com/matteobortolazzo/cenci), so cards move through `New` → `Refined` → `Planned` → `In Review` with agents doing the work.

1. **Install and run the daemon.** Use the [cenci installer](https://github.com/matteobortolazzo/cenci#readme) (`curl -fsSL https://raw.githubusercontent.com/matteobortolazzo/cenci/main/install.sh | bash`), then start the daemon once:

   ```
   cenci daemon &
   ```

   The daemon owns the broadcast socket that lazyboards' agent-status badges and dispatch panel both read from.

2. **Enroll the repo.** From inside the repo, either run `cenci dispatch enroll` yourself, or open lazyboards and press `D` then `Enter` — enrollment is idempotent either way, and only affects the currently open repo.

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

   Note: the `Refined` column's `D:` (Design) action shadows the built-in `D` (`view.dispatch`) while that column is active — column overlays always win over the global default for their own column, by design (see [Keymaps](#keymaps)). `W:` (Open worktree) is a plain uppercase custom action with no built-in collision of its own.

   Jumping to a card's agent window is built in — no custom action needed. Press `g a` on a card to jump straight to its agent's tmux window (a picker opens if several windows match), or press `A` to open the full Agents modal listing every cenci-watch window.

4. **Let cenci pick up approved plans automatically.** Once a ticket reaches `Planned` with an approved `.plans/<id>-*.md` file, `cenci dispatch` will run it for you — fleet-wide, across every enrolled repo. Trigger a single pass from the panel with `o`, or turn the recurring loop on/off with `l` (see the [Dispatch Panel](#dispatch-panel)). Tune concurrency, quiet hours, and per-agent budgets in cenci's own `dispatch` config block (`$XDG_CONFIG_HOME/cenci/config.json`) — see the [cenci README](https://github.com/matteobortolazzo/cenci/tree/main/watch#configuration-1) for the full reference.

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
| `session_max_length` | int | `40` | Max characters for the `{session}` template variable |
| `working_label` | string | `"Working"` | Label that shows a working indicator on cards |
| `mouse` | bool | `true` | Enable mouse support |
| `cenci` | bool | `true` | Enable live agent status badges + status-bar counts (requires the cenci-watch daemon; silently off when absent) |
| `update_check` | bool | `true` | Check for newer lazyboards releases on startup and show a sticky notice when one is available |
| `sort_order` | string | `oldest` | Card sort direction by creation date: `oldest` or `newest` created first (board-wide; sets the starting direction, and `s` toggles it) |
| `cleanup` | string | — | Default cleanup command applied to every column that doesn't set its own (see [Column Cleanup](#column-cleanup)) |
| `columns` | list | `[New, Refined, Implementing]` | Column definitions (name, actions, cleanup) |
| `actions` | map | — | Global custom actions (see [Custom Actions](#custom-actions)) |
| `keymaps` | map | — | Per-mode key bindings: command ids, inline actions, or `~` to unbind (see [Keymaps](#keymaps)) |

**Note on remembered state:** pressing `s` to flip the sort order writes your choice to `~/.config/lazyboards/state.yml`, so it survives a restart. That file is written by lazyboards alone — your config files are never rewritten — and a remembered direction takes precedence over `sort_order`. Delete it to go back to the configured default.

### Keymaps

Every key press in lazyboards resolves through a single `keymaps:` namespace: one table per mode, plus per-column overlays. Each entry is `keymaps.<mode>.<key>`, where the right-hand side is one of:

<!-- keymap-schema-example:start -->
```yaml
keymaps:
  normal:
    q: app.quit          # a built-in command id (see the per-mode tables in Keybindings)
    O:                   # an inline action mapping, same shape as top-level actions:
      name: Open issue
      type: url
      url: "https://github.com/{repo_owner}/{repo_name}/issues/{number}"
    n: ~                 # explicit unbind: `~` (or `null`, or an empty value)
```
<!-- keymap-schema-example:end -->

A key is any BubbleTea key notation exactly as shown in the Keybindings tables (`q`, `ctrl+a`, `alt+enter`, `shift+tab`, ...). A key **sequence** is space-separated (`"g d"`), the canonical form of a neovim-style prefix binding — this replaces the legacy `actions:` sequence notation (`Rf`), which concatenated keys with no separator; see [Key Sequences (Prefix Keys)](#key-sequences-prefix-keys) below. A bound key that is a strict prefix of another bound key in the same table (e.g. `R` and `R f` both bound) is a load-time config error, since the shorter key could never dispatch — Lookup always waits for a continuation key once a prefix is pending.

<!-- keymap-bindable-modes:start -->
Bindable modes: `normal`, `detail`, `create`, `error`, `config`, `pr_picker`, `search`, `help`, `label_confirm`, `close_confirm`, `comment`, `delete`, `filter`, `assign`, `git_panel`, `dispatch`, `pr_list`, `milestone_list`, `agent_list`. See [Keybindings](#keybindings) for each mode's shipped command-id table.
<!-- keymap-bindable-modes:end -->

**Mode capabilities:** not every mode's dispatch seam can do everything a binding might ask of it. Multi-key sequences dispatch only in `normal`, `detail`, and per-column `keymaps.columns.<name>` overlays — every other mode resolves a single key by exact match only. Inline actions dispatch only in `normal`, `detail`, `git_panel`, `pr_list` (restricted there to `scope: pr` actions, never inferred — see [Pull Requests](#pull-requests)), and `keymaps.columns.<name>` overlays — every other mode can only bind a built-in command id. A bare printable-rune key (a single character, no modifier) is rejected in `create`, `config`, `search`, `comment`, and `delete` — those modes' text inputs swallow every printable keystroke before any lookup runs; a named key (`enter`, `esc`, `ctrl+n`, ...) or an `alt+<rune>` form is exempt and binds normally. A binding one of these seams can never reach is a load-time config error, not a silent no-op — see [`docs/keymaps.md#mode-capability-matrix`](docs/keymaps.md#mode-capability-matrix) for the full per-mode matrix.

**Merge and precedence:** local config (`.lazyboards.yml`) merges over global config (`~/.config/lazyboards/config.yml`) per key, mirroring the `actions:`/`columns[].actions:` merge rules above — a mode or column the local file never mentions at all inherits the global table wholesale; an explicitly empty local table (`keymaps.normal: {}`) means "inherit nothing from global," not "unbind the built-in defaults" — unbinding a specific built-in still requires an explicit `~` entry for that key. Once merged, the resulting user config always wins over the built-in defaults, key by key — a user binding overrides a default with the same key, and every default key the user config doesn't mention is left untouched.

**Column overlays:** `keymaps.columns.<name>` binds keys scoped to one column, matched case-insensitively. Column overlays apply to `normal` and `detail` only — no other mode can be overlaid per column. The full precedence for a bound key inside a column is `default-mode < default-column < user-mode < user-column`: a column's own user binding wins over everything, then a mode-wide user binding, then a column's own default, then the mode-wide default.

**`ctrl+c` is special:** binding *or* unbinding `ctrl+c` anywhere in a key or sequence, in any mode or column table, is a load-time config error. `ctrl+c` always force-quits regardless of table contents — it is checked before any table lookup runs, so nothing can ever intercept it.

**`app.quit` may be bound in any mode:** unlike other built-in commands, `app.quit` isn't limited to the four modes that ship it by default (`normal`, `detail`, `help`, `error` — see their tables in [Keybindings](#keybindings)). It's a universal command, valid and dispatchable in every bindable mode listed above, without changing any of those four defaults. Outside `normal`/`detail`, dispatch is single-key only — a multi-key sequence bound to `app.quit` in any other mode's table is a documented no-op, since those modes' key-resolution seams have no pending-sequence machinery.

**Security note:** `.lazyboards.yml` is repo-local and is typically checked into the repository, so it is attacker-controlled the moment you clone someone else's repo. Any key — including built-in lowercase keys like `j`, `k`, or `q` — can be rebound to an inline action. A `type: url` action, or a rebind onto a catalogued built-in command id, applies immediately regardless of trust. A `type: shell` action is inert until you explicitly run `lazyboards trust` for that exact file content — an untrusted repo's `.lazyboards.yml` loads and applies its non-executing settings normally, but every shell-executing construct it declares (inline `keymaps:` shell bindings, legacy `actions:`/`columns[].actions:` shell entries, `cleanup:`/`columns[].cleanup`) is silently stripped before it can run. See [Trust Model](#trust-model) for the full mechanism, and note that this does not cover a rebind of a destructive built-in (e.g. `card.delete`) onto an innocuous key — every destructive built-in still sits behind its own confirm step.

### Trust Model

`lazyboards trust` marks the current directory's `.lazyboards.yml` (by exact
content hash) as trusted, so its `type: shell` constructs are honored; `lazyboards
untrust` revokes that. Both are argument-free, idempotent, and never touch
your global `~/.config/lazyboards/config.yml`. The trust store lives at
`~/.config/lazyboards/trust.yml`. Editing and saving a trusted config through
the in-app config modal (`c`) carries its trust forward automatically — it
never grants trust on its own, only preserves it across the rewrite. See
[`docs/trust-model.md`](docs/trust-model.md) for the full mechanism (what
counts as a sink, hash identity, store format, and the residual risk this
model deliberately doesn't cover).

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

> **Deprecated:** the top-level `actions:` block is deprecated in favor of the [`keymaps:`](#keymaps) namespace (`keymaps.normal`/`keymaps.detail`).

Bind keys to URL or shell actions in your config. Any key, uppercase or lowercase, can bind a built-in command or an inline action — a user binding always wins over a default. Normal mode's shipped defaults aren't lowercase-only: `D`/`P`/`A`/`G` ship as the wider-scope uppercase siblings of their lowercase counterparts (`D`ispatch vs. `d`elete, `P`R list vs. `p`R open, `A`gents list vs. `g a` go-to-agent, `G`it menu — see [Normal Mode](#normal-mode)), and rebinding any of them still lets a user binding win over the default. The built-in git shortcuts inside the [Git Menu](#git-menu) itself are scoped to their own modal, independent of whatever normal mode binds on the same key. The dispatch panel's own cenci controls (enroll, dispatch-once, loop on/off) are built in — see the [Dispatch Panel](#dispatch-panel) — so you only need custom actions for cenci commands the panel doesn't cover:

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

Press the key to execute the action on the selected card. Custom actions and their `Alt`-held [comment-mode](#comment-mode) overload work identically whether the card list or the [detail panel](#detail-panel) is focused.

### Key Sequences (Prefix Keys)

When single keys run out — a monorepo where you want to run several projects from a PR, say — bind multi-key sequences, neovim-style. A key is a sequence when it's longer than one character; every key of the sequence, including the first, can be any letter or digit (uppercase or lowercase) — not just uppercase, since any key can bind a built-in command or an inline action, and built-in commands can participate in a sequence too, not just custom actions:

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

Press `R` and the status bar switches to a which-key style list of everything the prefix can complete to, rendered in canonical, space-separated form (`R f: Run frontend | R b: Run backend | R w: Run worker | esc: cancel`); press the next key to run it. While a sequence is pending it owns the keyboard — built-in keys like `j`/`k` act as continuation keys, not navigation. `Esc` cancels, as does any key that doesn't match a bound sequence. Holding `Alt` on any key of the sequence gives the same [comment-first flow](#comment-mode) as `Alt+key` on a single-key action — though on a non-final key it only resolves when every binding under that prefix is an inline action.

Sequences can be any length (`R`, `Rf`, `RFa1`, ...) and follow all the usual action rules: scopes, template variables, per-column overrides, and gating (a prefix whose only completions are `pr`-scope won't even open on a card with no linked PRs). One constraint is validated at startup: a key can't be a strict prefix of another key that can be active at the same time — a standalone `P` action plus a `Pf` sequence is a config error, because `P` could then never fire.

The legacy `actions:` notation above concatenates a sequence's keys with no separator (`Rf`); the canonical [`keymaps:`](#keymaps) form space-separates them instead (`"R f"`) — this is the form `legacySequence` translates a legacy key into internally, and the form the which-key hint bar and `keymaps.<mode>` tables always use.

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

Actions default to `scope: "card"` when the template references a ticket-specific placeholder (a card-specific variable — `{number}`, `{title}`, `{tags}`, `{session}`, `{window}` — or a PR-specific variable, `{pr_*}`). If `scope` is omitted and the template references none of those, it now defaults to `scope: "board"` instead — most `url`/`command` templates without a card variable are board-wide by nature (a dashboard link, a deploy command), and setting `scope: "card"` isn't the useful behavior for a template that never uses the selected card. Set `scope: "board"` explicitly for a board-scope action anyway if you prefer to be explicit, or when the default inference doesn't apply (e.g. you want `card` scope but the template happens not to reference a card variable). Board-scope actions — inferred or explicit — cannot use card-specific variables (`{number}`, `{title}`, `{tags}`, `{session}`, `{window}`) or PR-specific variables (`{pr_branch}`, `{pr_number}`, `{pr_url}`, `{pr_title}`, `{pr_worktree}`).

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

Inside a git repository with a remote, press `G` to open the **Git Menu** — six built-in board-scope git shortcuts with lazygit-style keys, no config required:

| Key | Action | Command |
|-----|--------|---------|
| `P` | Push | `git push` |
| `p` | Pull (rebase) | `git pull --rebase` |
| `f` | Fetch | `git fetch` |
| `m` | Mergetool | `git mergetool` |
| `s` | Stash push | `git stash push` |
| `S` | Stash pop | `git stash pop` |

Inside the menu, press an action's key to run it immediately (like lazygit), or navigate with `j`/`k` and press `enter`; `esc` cancels. The keys are scoped to the menu: they do nothing in normal mode, so a custom `P` [action](#custom-actions) bound in normal mode and the menu's Push coexist without conflict — any key, uppercase or not, can bind a normal-mode custom action independently of what the Git Menu binds inside its own modal. The menu is also listed in the `?` help popup and only opens inside a git repo.

Custom inline actions bound via `keymaps.git_panel` honor `board`/`card`/`pr` scope exactly like normal-mode actions: `card`-scope templates resolve against the selected card, and `pr`-scope templates follow the same 0/1/2+ linked-PR precedence and PR picker as elsewhere — 0 linked PRs refuses silently (no status message), 1 runs immediately, 2+ opens the PR picker.

### Status Bar Prefix (agent + PR counts)

At the left of the status bar, an always-visible prefix summarizes the whole repository: agent-status counts (`▶N` running, `!N` awaiting input) followed by the repo-wide open-PR total (` N`, using the same PR glyph shown on cards). Each token is omitted when its count is zero, and the prefix disappears entirely when all are zero. Because the prefix is reserved before anything else, it stays visible through timed status messages and is never truncated to make room for hints or the right-aligned git/dispatch segments.

The agent counts cover every window the cenci-watch daemon tracks — across all tmux sessions, whether or not a window's name joins to a card on the board. (The `A` agents modal, by contrast, is scoped to this instance's own tmux session, so its row count may be smaller than the status-bar total.) The PR total counts every open PR in the repository — the same set the `P` [open-PR list](#pull-requests) shows — not just PRs linked to cards. Until the first repo-wide listing succeeds (or if it isn't available), the PR token falls back to the card-linked sum; afterwards a failed refresh keeps the last known total rather than dropping the token.

### Git Status Segment

Inside a git repository with a remote, the status bar shows a compact, right-aligned, plain-ASCII git segment: current branch, staged/unstaged file counts, and commits ahead/behind upstream — e.g. `main +2~1 ↑3↓0`. The `↑N↓N` portion is omitted when the branch has no upstream configured. The segment is hidden entirely (no error shown) outside a git repo, when there's no remote, or on narrow terminals where there isn't room — hints always keep priority over the segment.

The segment refreshes on board start, after every board refresh, after any successful action, and on a background poll every ~12s to catch changes made outside the app.

### Dispatch Status Segment

When the cenci-watch daemon reports the background dispatch loop enabled, the status bar shows a `⟳ dispatch` segment, right-aligned to the left of the git segment (see [Git Status Segment](#git-status-segment) for priority rules — the dispatch segment is dropped first on narrow terminals). It's sourced live from the same watcher subscription that drives agent-status badges, so it appears and disappears immediately as the loop is toggled or the daemon becomes unreachable — no restart needed. If the last dispatch pass failed, the segment renders in red instead of its normal color. A single transient watcher reconnect blip is tolerated and does not clear the segment; it only clears after a second consecutive watcher error with no successful reconnect in between.

Set `LAZYBOARDS_DEBUG_LOG=<path>` to append watcher connection errors (including tolerated blips) to a file at `<path>`, one timestamped line per error — useful for diagnosing daemon connectivity issues. Unset (the default), this is a complete no-op: no file is created and there's no overhead.

### Crash Reports

If lazyboards panics, the stack trace is normally printed to stderr as the terminal is restored — where the altscreen switch tends to wipe it before you can read it. To make crashes diagnosable, lazyboards also appends each panic (timestamp, call site, panic value, and full stack trace) to `~/.config/lazyboards/crash.log`, alongside your config. This is always on and needs no configuration; the file and its parent directory are created on demand at crash time, so nothing is written during normal operation. After a crash, attach the latest entry from that file when reporting the issue.

### Column-Specific Actions

> **Deprecated:** `columns[].actions` is deprecated in favor of `keymaps.columns.<name>` in the [`keymaps:`](#keymaps) namespace.

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

Any key — one that binds a built-in command by default, or an inline action — can be overridden per column via `keymaps.columns.<name>`; a column-scoped binding wins over both the global default and the global user binding for that key.

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

Actions that include `{comment}` in their template can be triggered with **Alt held** to open a text input first:

```yaml
keymaps:
  normal:
    b:
      name: Annotate
      type: shell
      command: 'gh issue comment {number} --body {comment}'
```

Press `b` to run with an empty comment. Press `Alt+b` to type a comment first, then `Enter` to submit.

The modifier is just **Alt**. You only ever add Shift when the bound key is itself uppercase — `Alt+Shift+A` is simply how you type `alt+A` for a key bound as `A`. Pre-[`keymaps:`](#keymaps) documentation described this as "Alt+Shift+key" because custom-action keys were then restricted to `A-Z`; now that any key can bind an inline action, a lowercase binding takes plain `Alt+<key>`.

For a [key sequence](#key-sequences-prefix-keys), the Alt flag is sticky: hold it on any key and the comment-first flow triggers once the sequence completes. The **final** key is the reliable place to hold it, because an earlier key only resolves when *every* binding under that prefix is an inline action. With a mixed prefix — say `"u p"` bound to the built-in `card.open_pr` alongside `"u c"` bound to an action — `Alt+u` does nothing, while `u` then `Alt+c` works.

That refusal is the rule, not an edge case: the Alt overload resolves to an inline action or to nothing, and can never fire a built-in command. Binding `alt+<key>` explicitly on a key whose alt-free form is already a `{comment}` action is a load-time config error naming both, since the explicit binding would shadow the implicit overload.

#### macOS: enabling the Alt modifier

On macOS the Option key is a **compose key** by default: `⌥B` types `∫`, `⌥C` types `ç`. When Option composes, the terminal never sends the `ESC` prefix that lazyboards reads as Alt, so `Alt+b` arrives as a plain `∫` and the comment input never opens.

This cannot be fixed in lazyboards. Composition happens in macOS before the bytes reach the terminal, and a composed `ç` is byte-identical to typing `ç` directly — by the time lazyboards sees the key, the modifier is gone. Nor can it be worked around in `keymaps:`: the comment overload is triggered by the Alt modifier on the keystroke, not by a bindable key, so there is no alt-free mapping that opens the comment input. You have to tell your terminal to send Option as Meta:

| Terminal | Setting |
|----------|---------|
| Terminal.app | Settings → Profiles → Keyboard → **Use Option as Meta key** |
| iTerm2 | Settings → Profiles → Keys → General → Left/Right Option key → **Esc+** |
| Ghostty | `macos-option-as-alt = true` in `~/.config/ghostty/config` |
| kitty | `macos_option_as_alt yes` in `~/.config/kitty/kitty.conf` |
| WezTerm | `send_composed_key_when_left_alt_is_pressed = false` |
| Alacritty | `option_as_alt = "Both"` under `[window]` |
| VS Code | `"terminal.integrated.macOptionIsMeta": true` |

Verify the terminal before suspecting lazyboards — run `cat -v` and press `⌥B`:

```
^[b     # correct: Option is sending Meta
∫       # Option is still composing; the setting above is not applied
```

Note that this applies to the whole terminal profile, not just lazyboards: with Option as Meta you can no longer type accented characters (`é`, `ñ`, `ç`) in that profile. If you need both, enable the setting on a dedicated profile and run lazyboards there.

Over SSH and inside tmux the same rule holds — only the setting of the terminal you are physically typing into matters.

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

## Keybindings

Press `?` at any time to open the in-app help popup.

### Normal Mode

Every row below is a shipped default; any key (including the ones already
listed here) can also be rebound, unbound, or given a [custom
action](#custom-actions) or [key sequence](#key-sequences-prefix-keys) via
`keymaps.normal` — a user binding always wins over a default. The shipped
defaults follow neovim-style mnemonics: lowercase keys are the primary,
single-purpose commands; `g` is a go-prefix for "go to X" navigation
sequences (`g a`, `g r`); and uppercase keys are the wider-scope siblings of
their lowercase counterpart (`D`ispatch vs. `d`elete, `P`R list vs. `p`R
open, `A`gents list vs. `g a` go-to-agent, `G`it menu). See [Keybinding
migration](#keybinding-migration) if you're upgrading from a pre-#502
config and want the old keys back.

| Key | Command | Action |
|-----|---------|--------|
| `?` | `app.help` | Help |
| `q` | `app.quit` | Quit |
| `ctrl+c` | — | Force quit (always active; cannot be rebound or unbound) |
| `n` | `card.new` | New card |
| `e` | `card.edit` | Edit card |
| `c` | `app.config` | Configuration |
| `o` | `card.open_ticket` | Open ticket |
| `g r` | `nav.reference` | Go to referenced issue |
| `r` | `board.refresh` | Refresh board |
| `p` | `card.open_pr` | Open PR |
| `x` | `card.close` | Close card (with confirmation) |
| `d` | `card.delete` | Delete card permanently (with two-step confirmation) |
| `P` | `view.pr_list` | Open PRs (all open PRs in the repo) |
| `m` | `view.milestone_list` | Milestones (all open milestones in the repo) |
| `A` | `view.agent_list` | (cenci) Agents (cenci-watch windows in this instance's tmux session, labeled `session:index`; `enter` jumps to the tmux window) |
| `g a` | `nav.agent` | (cenci) Go to agent (jumps straight to the selected card's agent window in this session when there's exactly one; opens a picker when there are several) |
| `/` | `board.search` | Search |
| `a` | `card.assign` | Assign collaborator |
| `G` | `view.git_panel` | Git menu |
| `D` | `view.dispatch` | (cenci) Dispatch |
| `s` | `board.sort_order` | Toggle sort order (oldest/newest created first; board-wide, applies to all columns; remembered across restarts) |
| `f` | `board.filter` | Filter (toggle) |
| `l` / `right` (→) | `nav.detail_focus` | Detail panel |
| `j` / `down` (↓) | `nav.cursor_down` | Next card |
| `k` / `up` (↑) | `nav.cursor_up` | Previous card |
| `tab` | `nav.column_next` | Next column |
| `shift+tab` | `nav.column_prev` | Previous column |
| `1`-`9` | `nav.column_1` … `nav.column_9` | Jump to column |
| `alt+key` | — | Comment action (see [Comment Mode](#comment-mode); macOS: [enable the Alt modifier](#macos-enabling-the-alt-modifier)) |

### Detail Panel

Labels and assignees display alphabetically (case-insensitive). Any key can
also be rebound, unbound, or given a [custom action](#custom-actions) or [key
sequence](#key-sequences-prefix-keys) via `keymaps.detail`, same as [Normal
Mode](#normal-mode).

| Key | Command | Action |
|-----|---------|--------|
| `e` | `card.edit` | Edit card |
| `h` / `left` (←) / `esc` | `detail.blur` | Back to card list |
| `j` / `down` (↓) | `detail.scroll_down` | Scroll body down |
| `k` / `up` (↑) | `detail.scroll_up` | Scroll body up |
| `tab` | `nav.column_next` | Next column |
| `shift+tab` | `nav.column_prev` | Previous column |
| `1`-`9` | `nav.column_1` … `nav.column_9` | Jump to column |
| `o` | `card.open_ticket` | Open ticket |
| `g r` | `nav.reference` | Go to referenced issue |
| `p` | `card.open_pr` | Open PR |
| `r` | `board.refresh` | Refresh |
| `q` | `app.quit` | Quit |
| `?` | `app.help` | Help |
| `alt+key` | — | Comment action (see [Comment Mode](#comment-mode); macOS: [enable the Alt modifier](#macos-enabling-the-alt-modifier)) |

### Create Mode

The assignee field cycles through `(none)`, then you (`<user> (me)`), then the
remaining collaborators sorted alphabetically (case-insensitive).

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `create.cancel` | Cancel |
| `tab` | `create.next_field` | Next field |
| `left` (←) | `create.assignee_prev` | Previous assignee |
| `right` (→) | `create.assignee_next` | Next assignee |
| `enter` | `create.submit` | Submit |

### Config Mode

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `config.cancel` | Cancel (quit on first launch) |
| `tab` | `config.next_field` | Next field |
| `left` (←) | `config.provider_prev` | Previous provider |
| `right` (→) | `config.provider_next` | Next provider |
| `enter` | `config.save` | Save |

### PR Picker

| Key | Command | Action |
|-----|---------|--------|
| `left` (←) | `pr_picker.prev` | Previous PR |
| `right` (→) | `pr_picker.next` | Next PR |
| `enter` | `pr_picker.select` | Select |
| `esc` | `pr_picker.close` | Cancel |

### Pull Requests

Opened with `P` from normal mode. Lists every **open PR in the repository**,
not just those linked to a board card. While the repo-wide fetch is in
flight, the card-linked PRs (aggregated across all columns and cards,
regardless of any active search/filter) render immediately as a fallback; if
the fetch fails, that fallback is kept with an explicit note. PRs linked to
a card show the owning column and card next to the title; unlinked PRs are
listed plainly.

Keys bound via `keymaps.pr_list` run your global `scope: pr` [custom
actions](#custom-actions) against the selected PR, with the same template
variables as a normal-mode dispatch (legacy `actions:` entries only
translate into `pr_list` bindings for uppercase single-letter keys, mirroring
the pre-registry behavior). On a PR with no linked card, the card-derived
variables (`{number}`, `{title}`, `{tags}`, `{session}`, `{window}`) expand
to empty strings. Per-column action overrides and the `Alt` comment variant
are not available inside the modal. Every `keymaps.pr_list` inline action's
effective scope must resolve to exactly `pr` — scope is never inferred to
`pr`, so a scope-omitted (or `card`/`board`-scope) action bound there is a
load-time config error, not a silent no-op.

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `pr_list.close` | Cancel |
| `enter` | `pr_list.open` | Open selected PR |
| `j` / `down` (↓) | `pr_list.next` | Navigate |
| `k` / `up` (↑) | `pr_list.prev` | Navigate |

### Milestones

Opened with `m` from normal mode. Lists every **open milestone in the
repository** on one line each: title, a block progress bar, percentage,
`closed/total` issue counts, and its due date (or `no due date` when unset).
Three states: `Loading milestones...` while the fetch is in flight, the list
on success, and `Couldn't load milestones` (no rows) on error.

`enter` sets the selected milestone as the active board filter and closes the
modal, exactly like the filter picker; `f` clears it. `o` opens the
milestone's GitHub URL in your browser without closing the modal.

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `milestone_list.close` | Cancel |
| `enter` | `milestone_list.filter` | Filter board |
| `j` / `down` (↓) | `milestone_list.next` | Navigate |
| `k` / `up` (↑) | `milestone_list.prev` | Navigate |
| `o` | `milestone_list.open` | Open in browser |

### Agents

`A` always opens the modal, listing every cenci-watch window in this
instance's own tmux session (labeled `session:index`) regardless of whether
it matches a card. `g a` is a smart jump scoped to the selected card: zero
matching windows shows a status message, exactly one switches the tmux
client directly (no modal), and several open this same modal scoped to just
that card's windows.

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `agent_list.close` | Cancel |
| `enter` | `agent_list.go_to_window` | Go to tmux window |
| `j` / `down` (↓) | `agent_list.next` | Navigate |
| `k` / `up` (↑) | `agent_list.prev` | Navigate |

### Comment Mode

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `comment.cancel` | Cancel |
| `enter` | `comment.submit` | Submit |

### Delete

Opened with `d` from normal mode. Permanently deletes the selected card via
the provider (not a column move) after a two-step confirmation. Cards with
any linked PR cannot be deleted — the status bar shows an error and the card
list stays unchanged. Step 1 accepts an optional comment (blank is fine);
step 2 requires retyping the card's number exactly before the delete fires.
`enter` resolves to the same `delete.submit` command at both steps — step 1
continues to the confirm step, step 2 fires the delete once the retyped
number matches. A mismatched retype shows an inline error and stays on the
confirm step; `esc` at either step cancels the whole flow, discarding any
comment typed.

| Key | Command | Action |
|-----|---------|--------|
| `enter` | `delete.submit` | Continue to confirm step / confirm delete (must match the card number) |
| `esc` | `delete.cancel` | Cancel |

### Close Confirm

Opened with `x` from normal mode. A lighter one-step confirmation than
Delete — it moves the card to the closed state via the provider rather than
deleting it outright.

| Key | Command | Action |
|-----|---------|--------|
| `y` | `close_confirm.confirm` | Confirm close |
| `n` / `esc` | `close_confirm.cancel` | Cancel |

### Label Confirm

Entered automatically after saving an [edited card](#editing-cards) that adds
labels lazyboards doesn't already know about. Confirms one unknown label at a
time; once every unknown label is resolved, the edit is applied.

| Key | Command | Action |
|-----|---------|--------|
| `y` | `label_confirm.create` | Create this label, continue to the next unknown label (or apply the edit if none remain) |
| `n` / `esc` | `label_confirm.cancel` | Cancel the whole edit |

### Filter

The picker lists Labels, Assignees, and Milestones sections (only sections with
at least one value are shown), built from the cards currently on the board.
Entries within each section are sorted alphabetically (case-insensitive).

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `filter.close` | Cancel |
| `enter` | `filter.select` | Select |
| `j` / `down` (↓) | `filter.next` | Navigate |
| `k` / `up` (↑) | `filter.prev` | Navigate |

### Search

| Key | Command | Action |
|-----|---------|--------|
| `enter` | `search.apply` | Apply search |
| `esc` | `search.cancel` | Clear search |
| `down` (↓) / `ctrl+n` | `search.next_result` | Navigate results |
| `up` (↑) / `ctrl+p` | `search.prev_result` | Navigate results |
| `tab` | `search.next_column` | Exit search and switch to next column |
| `shift+tab` | `search.prev_column` | Exit search and switch to previous column |

All letters and digits type into the query (queries match titles, labels, and card numbers).

### Assign

You stay pinned first in the list; the remaining collaborators are sorted
alphabetically (case-insensitive) below.

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `assign.close` | Cancel |
| `enter` | `assign.toggle` | Toggle assignee |
| `j` / `down` (↓) | `assign.next` | Navigate |
| `k` / `up` (↑) | `assign.prev` | Navigate |

### Git Menu

Opened with `G` from normal mode.

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `git_panel.close` | Cancel |
| `enter` | `git_panel.run` | Run selected |
| `j` / `down` (↓) | `git_panel.next` | Navigate |
| `k` / `up` (↑) | `git_panel.prev` | Navigate |
| `P` | — (inline action) | Push |
| `p` | — (inline action) | Pull (rebase) |
| `f` | — (inline action) | Fetch |
| `m` | — (inline action) | Mergetool |
| `s` | — (inline action) | Stash push |
| `S` | — (inline action) | Stash pop |

### Dispatch (cenci)

Opened with `D` from normal mode. See [Dispatch Panel](#dispatch-panel) for
what enrollment and a dispatch run actually do.

| Key | Command | Action |
|-----|---------|--------|
| `esc` | `dispatch.close` | Close |
| `enter` | `dispatch.toggle_enroll` | Enroll/Unenroll current repo |
| `o` | `dispatch.once` | Dispatch once (all enrolled repos) |
| `l` | `dispatch.toggle_loop` | Toggle dispatch loop on/off (all enrolled repos) |
| `y` | `dispatch.confirm_loop` | Confirm the loop toggle (only while a loop-toggle confirmation is pending) |
| `n` | `dispatch.cancel_loop` | Cancel the loop toggle (only while a loop-toggle confirmation is pending) |

### Help

Opened with `?` from normal mode or the detail panel.

| Key | Command | Action |
|-----|---------|--------|
| `esc` / `?` | `help.close` | Close |
| `j` / `down` (↓) | `help.scroll_down` | Scroll down |
| `k` / `up` (↑) | `help.scroll_up` | Scroll up |
| `q` | `app.quit` | Quit |

### Error Mode

| Key | Command | Action |
|-----|---------|--------|
| `r` | `error.retry` | Retry loading |
| `q` | `app.quit` | Quit |

### Keybinding migration

#502 remapped nine Normal Mode defaults to neovim-style mnemonics (see the
[Normal Mode](#normal-mode) table above for the full current list):

| Command | Old key | New key |
|---|---|---|
| `card.delete` | `t` | `d` |
| `view.dispatch` | `d` | `D` |
| `nav.reference` | `m` | `g r` |
| `view.milestone_list` | `i` | `m` |
| `nav.agent` | `s` | `g a` |
| `board.sort_order` | `u` | `s` |
| `view.pr_list` | `v` | `P` |
| `view.agent_list` | `w` | `A` |
| `view.git_panel` | `g` | `G` |

Every other default key (`q ? c n e o p x a r / f l j k tab shift+tab 1`-`9`,
and every Detail Panel/modal key) kept its pre-#502 binding — only the nine
commands above moved, so any config that doesn't already bind one of the old
keys above needs no changes.

If you bind a sequence under `D`/`P`/`A`/`G`, or bind `g` itself, unbind the
colliding default first — e.g. `P: ~` — otherwise `config.Load` rejects the
config with a prefix-conflict error (a key that's also a bound default can
never dispatch once a longer sequence shares its prefix).

This also applies to the legacy `actions:` block: an entry like `Pf:` is
translated internally to the same canonical `"P f"` sequence, so it collides
with the new `P` default exactly like a `keymaps:` sequence would, and the
resulting error names the resolved sequence (e.g. `"P f"`), not your
original `Pf:` key. Unbind the collision the same way (`P: ~` under
`keymaps.normal`), or migrate the entry to `keymaps:` directly.

To restore every pre-#502 key exactly as it worked before, add this to your
`keymaps:` block (global `~/.config/lazyboards/config.yml` or per-project
`.lazyboards.yml`):

<!-- legacy-keymaps-restore:start -->
```yaml
keymaps:
  normal:
    t: card.delete
    d: view.dispatch
    m: nav.reference
    i: view.milestone_list
    s: nav.agent
    u: board.sort_order
    v: view.pr_list
    w: view.agent_list
    g: view.git_panel
    "g a": ~
    "g r": ~
    D: ~
    P: ~
    A: ~
    G: ~
  detail:
    m: nav.reference
    "g r": ~
```
<!-- legacy-keymaps-restore:end -->

## Mouse Support

Mouse support is enabled by default. Disable it in your config:

```yaml
mouse: false
```

- **Scroll wheel** on card list: navigate up/down
- **Scroll wheel** on detail panel: scroll body
- **Scroll wheel** in modals (PR list, milestones list, agents list, filter picker, assign picker, git menu, PR picker, help): move the row cursor up/down, clamped at the first/last item (does not wrap); the filter picker skips header rows, and the help modal scrolls its viewport instead
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
