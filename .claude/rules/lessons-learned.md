# Lessons Learned

This file captures mistakes made during implementation to prevent recurrence.
Claude reads this file automatically. Its rules are authoritative and override assumptions.

---

<!-- Entries will be added below this line by the lessons-collector agent -->

### Go cache dirs are read-only in Claude Code sandbox
- **Mistake:** `go test` and `go get` fail with "read-only file system" when writing to default `~/go/` and `~/.cache/go-build/`.
- **Fix:** Set `GOPATH=/tmp/claude/gopath GOCACHE=/tmp/claude/gocache GOMODCACHE=/tmp/claude/gomodcache` before any Go commands.

### lipgloss Width() sets content width, borders are additional
- **Mistake:** Column width calculated as `b.Width / len(b.Columns)` without accounting for border characters, causing total rendered width to exceed terminal width.
- **Fix:** Subtract border overhead: `colWidth := (b.Width / len(b.Columns)) - borderWidth`.
