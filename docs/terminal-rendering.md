# Terminal Rendering & Layout

Rules for computing terminal-cell widths and handling glamour-rendered markdown in scrollable/clickable UI.

## Rules

- `lipgloss.Width()` sets *content* width; border characters are additional. When dividing total terminal width across columns (`b.Width / len(b.Columns)`), subtract border overhead per column or the total rendered width exceeds the terminal.
- Never use `len(s)` or `len([]rune(s))` for terminal layout math (hit-detection, column sizing, truncation). Use `lipgloss.Width(s)` instead — it calls `go-runewidth` internally and correctly counts CJK characters and emoji as 2 cells wide, where rune-counting undercounts them by half. Example: `handleTabClick`'s tab hit-zone math (`update.go`) was offset for any non-ASCII label until switched to `lipgloss.Width`.
- Glamour's markdown rendering adds a leading blank line to its output. When computing rendered line count for scroll-offset math, use `strings.TrimSpace(out)`, not `strings.TrimRight(out, "\n ")` — TrimRight only strips trailing whitespace and leaves the leading blank line, inflating the count.
- Glamour collapses consecutive single-newline soft breaks within a paragraph; only `\n\n` produces a hard paragraph break with a guaranteed line. When a test needs N distinct rendered lines from glamour output, join fixture lines with `"\n\n"`, not `"\n"` — single-newline fixtures render fewer lines than expected and can make scroll tests fail to exceed the visible area.
