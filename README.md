# lazyboards

A terminal Kanban board inspired by [lazygit](https://github.com/jesseduffield/lazygit).

Built with [BubbleTea](https://github.com/charmbracelet/bubbletea) and [lipgloss](https://github.com/charmbracelet/lipgloss).

## Features

- Vim-style navigation across columns and cards
- Split-pane layout: card list + detail view
- Card creation via modal form with validation
- Responsive terminal resizing

## Configuration

Copy the example config and adjust values:

```
cp .lazyboards.yml.example .lazyboards.yml
```

Set your GitHub token:

```
export GITHUB_TOKEN=your_token_here
```

See `.lazyboards.yml.example` for all available options.

## Install

```
go install github.com/matteobortolazzo/lazyboards@latest
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
| `q` | Quit |
| `Ctrl+C` | Force quit |

### Create Mode

| Key | Action |
|-----|--------|
| `Tab` | Switch between Title and Label fields |
| `Enter` | Submit card |
| `Esc` | Cancel |

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
