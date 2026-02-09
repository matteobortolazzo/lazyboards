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

### Always run git worktree add from main repo directory
- **Date**: 2026-02-09
- **What happened**: Running `git worktree add .worktrees/5-create-modal-ui` from inside another worktree (`.worktrees/5-create-mode-state/`) created the new worktree nested inside the first one instead of under the main repo's `.worktrees/` directory.
- **Root cause**: Git interprets relative paths from the current working directory, so running the command from a nested worktree made `.worktrees/` resolve relative to that nested location.
- **Fix**: Always `cd /home/mborto/Repos/lazyboards && git worktree add .worktrees/<name> ...` before creating stacked or new worktrees. Use absolute paths for the main repository.
- **Rule**: Git worktree operations must be executed from the main repository root, not from within an existing worktree.

### BubbleTea textinput Cmd must always be propagated
- **Date**: 2026-02-09
- **What happened**: When forwarding key messages to `textinput.Model.Update()` in createMode, the returned `tea.Cmd` was discarded with `_`. This broke the cursor blink animation after the first keystroke.
- **Root cause**: BubbleTea's Cmd return value encodes async work (timers, animations, subscriptions). Discarding it breaks all animations. The textinput component uses a Cmd to schedule cursor blinks.
- **Fix**: Always capture and propagate the Cmd: `var cmd tea.Cmd; model, cmd = model.Update(msg); return model, cmd`. Also update tests to check behavior (e.g., mode stays in createMode) instead of checking `cmd == nil`.
- **Rule**: Never discard `tea.Cmd` values from sub-model `Update()` calls. Always propagate them up through the component hierarchy.

### Heredoc in git commit fails in sandbox — use commit -F instead
- **Date**: 2026-02-09
- **What happened**: `git commit -m "$(cat <<'EOF' ... EOF)"` failed with "can't create temp file for here document: read-only file system", even with `TMPDIR=/tmp/claude`.
- **Root cause**: The shell (zsh) needs to write heredoc content to a temp file before expansion. The sandbox blocks writes to the default temp directory, and setting `TMPDIR` in the same command doesn't affect the shell's heredoc processing.
- **Fix**: Write the commit message to a file first with `printf ... > /tmp/claude/commit-msg.txt`, then use `git commit -F /tmp/claude/commit-msg.txt`.
- **Rule**: Never use heredoc syntax for git commit messages in sandbox. Always use `git commit -F <file>` with a pre-written message file.
