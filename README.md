# lazyboards

A terminal Kanban board inspired by [lazygit](https://github.com/jesseduffield/lazygit).

Built with [BubbleTea](https://github.com/charmbracelet/bubbletea) and [lipgloss](https://github.com/charmbracelet/lipgloss).

## Features

- Vim-style navigation across columns and cards
- Split-pane layout: card list + detail view
- Scrollable card lists with overflow indicators
- Card creation via modal form with validation
- Custom actions: open URLs or run shell commands bound to any key, with column cleanup on departure
- Auto-detection of provider and repo from git remote
- In-app configuration UI (first-launch flow or press `c`)
- Board refresh without restarting
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

Place shared settings (like custom actions) in `~/.config/lazyboards/config.yml`. Local config merges on top, with local values taking priority.

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

**Template variables:** `{number}`, `{title}` (slugified), `{tags}`, `{session}`, `{repo_owner}`, `{repo_name}`, `{provider}`

Shell commands automatically escape template variables to prevent injection.

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
        command: 'tmux new-window -d -n {session} "claude"'
    cleanup: 'tmux kill-window -t {session} 2>/dev/null || true'
  - name: Refined
```

The `cleanup` command uses the same template variables as actions. It runs when a card moves to another column or disappears.

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

### Normal Mode

| Key | Action |
|-----|--------|
| `h` / `←` | Previous column |
| `l` / `→` | Next column |
| `k` / `↑` | Previous card |
| `j` / `↓` | Next card |
| `n` | Create new card |
| `c` | Open configuration |
| `r` | Refresh board |
| `q` | Quit |
| `Ctrl+C` | Force quit |

### Create Mode

| Key | Action |
|-----|--------|
| `Tab` | Switch between Title and Label fields |
| `Enter` | Submit card |
| `Esc` | Cancel |

### Config Mode

| Key | Action |
|-----|--------|
| `←` / `→` | Cycle provider |
| `Tab` | Switch between Provider and Repo fields |
| `Enter` | Save configuration |
| `Esc` | Cancel (quit on first launch) |

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
