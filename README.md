# lazyboards

A terminal Kanban board inspired by [lazygit](https://github.com/jesseduffield/lazygit).

Built with [BubbleTea](https://github.com/charmbracelet/bubbletea) and [lipgloss](https://github.com/charmbracelet/lipgloss).

## Features

- Vim-style navigation across columns and cards
- Split-pane layout: card list + detail panel with markdown rendering
- Scrollable card lists with overflow indicators
- Card creation via modal form with label and assignee fields
- Assign and unassign collaborators to cards
- Search cards by title and filter by label or assignee
- PR linking with picker modal
- Custom actions: open URLs or run shell commands bound to any key, with column cleanup on departure
- Auto-detection of provider and repo from git remote
- In-app configuration UI (first-launch flow or press `c`)
- Board refresh (manual and periodic background refresh)
- Help popup with full keybinding reference (`?`)
- Error screen with retry support
- Responsive terminal resizing

## Install

```
go install github.com/matteobortolazzo/lazyboards@latest
```

## Configuration

Lazyboards auto-detects the provider and repository from your git remote. To override, create a `.lazyboards.yml` in your project root:

```yaml
provider: github
repo: owner/repo
```

### Authentication

If you have the [GitHub CLI](https://cli.github.com/) installed, lazyboards will use your existing authentication automatically:

```
gh auth login
```

Alternatively, set a token manually:

```
export GITHUB_TOKEN=your_token_here
```

On first launch without a local config, an interactive configuration popup guides you through setup.

### Global Config

Place shared settings in `~/.config/lazyboards/config.yml` for options that apply across all your projects: actions, columns, refresh interval, session max length, working label, and action refresh delay. Local config (`.lazyboards.yml`) merges on top, with local values taking priority.

**Note:** `provider`, `repo`, and `project` are project-specific and cannot be set in global config — they come from `.lazyboards.yml` or git remote auto-detection.

### Custom Actions

Bind single-character keys to URL or shell actions in your config:

```yaml
actions:
  o:
    name: Open
    type: url
    url: "https://github.com/{repo_owner}/{repo_name}/issues/{number}"
  b:
    name: Branch
    type: shell
    command: "git checkout -b {number}-{title}"
```

**Template variables:** `{number}`, `{title}` (slugified), `{tags}`, `{session}`, `{repo_owner}`, `{repo_name}`, `{provider}`, `{comment}`

Shell commands automatically escape template variables to prevent injection. Variables are wrapped in POSIX single quotes at runtime (e.g., `{comment}` becomes `'my comment'`). When nesting shell commands (e.g., tmux), this interacts with outer quoting — see the examples below for recommended patterns.

Keys reserved for built-in navigation (`h`, `l`, `j`, `k`, `q`, `r`, `n`, `c`) cannot be used for actions.

#### Tmux Integration

Open a new tmux window for each card without leaving the board:

```yaml
actions:
  t:
    name: Tmux window
    type: shell
    command: "tmux new-window -d -n {session}"
```

The `-d` flag keeps focus on the current window. The `{session}` variable generates a tmux-friendly name from the card number and title (e.g., `42-fix-login-bug`), capped at `session_max_length` (default: 32).

#### Column Cleanup

Run a command automatically when a card leaves a column (detected on board refresh). Useful for closing tmux windows or stopping processes spawned by column actions:

```yaml
columns:
  - name: New
    actions:
      R:
        name: Refine ticket
        type: shell
        command: 'tmux new-window -d -n {session} "claude --comment {comment}"'
    cleanup: 'tmux kill-window -t {session} 2>/dev/null || true'
  - name: Refined
```

The `cleanup` command uses the same template variables as actions. It runs when a card moves to another column or disappears.

#### Comment Mode

Actions that include `{comment}` in their URL or shell template open a text input modal when triggered with `Alt+key` instead of executing immediately. This lets you type a comment before the action runs:

```yaml
actions:
  a:
    name: Annotate
    type: shell
    command: 'gh issue comment {number} --body {comment}'
```

Press `a` to run the command with an empty comment. Press `Alt+a` to open the comment modal, type your text, and press `Enter` to submit.

### Action Refresh Delay

After a shell action completes successfully, the board automatically refreshes after a short delay. Configure the delay in seconds with `action_refresh_delay`:

```yaml
action_refresh_delay: 10
```

The default is 5 seconds. Setting to 0 disables auto-refresh after shell actions entirely:

```yaml
action_refresh_delay: 0
```

## Keybindings

Press `?` at any time to open the in-app help popup with all keybindings.

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
| `f` | Filter (toggle) |
| `l` / `→` | Detail panel |
| `h` / `←` | Previous column |
| `j` / `↓` | Next card |
| `k` / `↑` | Previous card |
| `Tab` / `Shift+Tab` | Switch columns |
| `1-9` | Jump to column |
| `Alt+Shift+key` | Comment action |

### Detail Panel

| Key | Action |
|-----|--------|
| `e` | Edit card |
| `j` / `k` | Scroll body |
| `h` / `←` / `Esc` | Back to card list |
| `Tab` / `Shift+Tab` | Switch columns |
| `o` | Open ticket |
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
| `f` | Filter (toggle) |
| `j` / `k` | Navigate |
| `Enter` | Select |
| `Esc` | Cancel |

### Search

| Key | Action |
|-----|--------|
| `Esc` | Clear search |

### Assign

| Key | Action |
|-----|--------|
| `j` / `k` | Navigate |
| `Enter` | Toggle assignee |
| `Esc` | Cancel |

### Error Mode

| Key | Action |
|-----|--------|
| `r` | Retry loading |
| `q` | Quit |

## Build from Source

```
git clone https://github.com/matteobortolazzo/lazyboards.git
cd lazyboards
go build
```

## Run Tests

```
go test ./...
```

## License

[MIT](LICENSE)
