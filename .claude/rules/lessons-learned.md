# Lessons Learned

This file captures mistakes made during implementation to prevent recurrence.
Claude reads this file automatically. Its rules are authoritative and override assumptions.

---

<!-- Entries will be added below this line by the lessons-collector agent -->

### Go cache dirs are read-only in Claude Code sandbox
- **Mistake:** `go test` and `go get` fail with "read-only file system" when writing to default `~/go/` and `~/.cache/go-build/`.
- **Fix:** Set `GOPATH=/tmp/claude-1000/gopath GOCACHE=/tmp/claude-1000/gocache GOMODCACHE=/tmp/claude-1000/gomodcache` before any Go commands. Note: The actual writable temp directory is user-specific (e.g., `/tmp/claude-1000/` for UID 1000). Use `$TMPDIR` to detect the correct path, or inspect `/tmp/` to find the writable directory.
- **Rule:** Always use a user-specific temp directory under `/tmp/` (not `/tmp/claude/`). If `TMPDIR` env var is set, use that as the base path.

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

### Testing async BubbleTea commands requires execCmds helper
- **Date**: 2026-02-12
- **Ticket**: #46
- **What happened**: Test `TestAction_ShellTriggersRunShell` failed because the `sendKey` helper discarded the `tea.Cmd` returned from `Update()`. Shell actions return an async `tea.Cmd` (goroutine that executes and sends `actionResultMsg`), so the test never saw the result.
- **Root cause**: BubbleTea async commands (returned from `Update()`) must be executed to get their messages. Test helpers that discard `tea.Cmd` cannot observe async behavior.
- **Fix**: Capture `tea.Cmd` from `Update()` and use a recursive `execCmds()` helper to execute batch commands and collect all resulting messages. Pattern: `cmd := m.Update(msg); msgs := execCmds(cmd); for _, msg := range msgs { m.Update(msg) }`.
- **Rule**: When testing BubbleTea features that use async commands (goroutines, timers, subscriptions), always capture and execute the `tea.Cmd` to observe the full behavior. Create an `execCmds()` helper for tests.

### Shell command injection via template variables
- **Date**: 2026-02-12
- **Ticket**: #46
- **What happened**: Card labels from GitHub API (untrusted user input) were interpolated into shell commands via `{tags}` template variable without escaping. A malicious label like `"; rm -rf /; "` would execute arbitrary commands.
- **Root cause**: Template expansion directly substitutes user-controlled strings into shell commands without validation or escaping. Shell metacharacters (`;`, `|`, `&`, `$()`, etc.) in labels enable command injection.
- **Fix**: Added `ShellEscape()` (POSIX single-quote wrapping: replace `'` with `'\''`, wrap in `'...'`) and `BuildShellSafeVars()` to escape all template variables before shell command expansion.
- **Rule**: Always escape user-controlled input before interpolating into shell commands. Use POSIX single-quote wrapping for shell safety. Never trust data from external APIs (GitHub labels, issue titles, etc.) in shell contexts.

### Glamour adds leading blank line — use TrimSpace not TrimRight
- **Date**: 2026-02-13
- **Ticket**: #61
- **What happened**: Detail panel scroll tests failed because glamour's markdown rendering added a leading blank line to its output. When calculating rendered line count (to determine max scroll offset), the leading blank line inflated the count incorrectly, breaking scroll tests that needed the rendered content to exceed the visible area.
- **Root cause**: `strings.TrimRight(out, "\n ")` only removes trailing whitespace, leaving the leading blank line intact. Glamour always outputs a leading newline character as part of its rendering format.
- **Fix**: Changed to `strings.TrimSpace(out)` in `view.go` line 181, which removes both leading and trailing whitespace. This gives the accurate rendered line count needed for proper scroll offset calculation.
- **Rule**: When working with glamour-rendered markdown in scrollable panels, always use `strings.TrimSpace()` on the rendered output to normalize it for line counting. Never assume markdown renderers produce zero leading whitespace.

### Glamour collapse consecutive newlines in markdown — use paragraph separators in tests
- **Date**: 2026-02-13
- **Ticket**: #61
- **What happened**: Initial scroll tests in detail panel used single-line markdown bodies separated by `\n`, expecting each line to render as a distinct line. However, glamour sometimes collapses consecutive single lines (without paragraph breaks) into fewer rendered lines, causing tests to fail when the rendered body didn't exceed the visible panel area and thus didn't allow scrolling.
- **Root cause**: Markdown rendering treats single newlines as soft breaks within a paragraph; only double newlines (`\n\n`) create hard paragraph breaks. Glamour's rendering engine may collapse soft breaks depending on context.
- **Fix**: Test data for glamour rendering should use `\n\n` (paragraph separators) instead of `\n` (line breaks) when the test needs to ensure a specific number of rendered lines. For example: `strings.Join(lines, "\n\n")` instead of `strings.Join(lines, "\n")`.
- **Rule**: When writing tests for glamour-rendered content, use double newlines (`\n\n`) between lines to ensure consistent rendering and predictable line counts. Single newlines are unreliable because markdown treats them as soft breaks that may be collapsed.

### url.PathEscape does NOT encode &, =, or , — use QueryEscape for query-string security
- **Date**: 2026-02-18
- **Ticket**: #128
- **What happened**: Three reviewers recommended switching from `url.QueryEscape` to `url.PathEscape` for URL template variable encoding, and the change was implemented and tested. However, `url.PathEscape` does not encode `&`, `=`, `,` (they are RFC 3986 sub-delimiters valid in path segments), leaving the primary attack vector (query parameter injection via `&`) completely open. A label like `"bug&extra=malicious"` in `?tags={tags}` would produce `?tags=bug&extra=malicious`, injecting a new query parameter.
- **Root cause**: `url.PathEscape` and `url.QueryEscape` have different character sets they encode. PathEscape is designed for path segments where `&` is a valid sub-delimiter. QueryEscape is designed for query values where `&` is a parameter separator and must be encoded.
- **Fix**: Reverted to `url.QueryEscape`, which correctly encodes `&` as `%26`, `=` as `%3D`, etc. The `+` for spaces trade-off is acceptable because: (1) most template variables don't contain spaces (number, slugified title, slugified session), and (2) for tags in query strings, `+` as space is correct per application/x-www-form-urlencoded specification.
- **Rule**: When encoding values for URL query parameters, always use `url.QueryEscape` (encodes `&`, `=`, `+` as `%26`, `%3D`, `%2B`). Only use `url.PathEscape` for values in URL path segments. Never change encoding functions based on reviewer recommendation without verifying which characters each function encodes against the RFC 3986 character set.

### Column matching must use name equality, not positional index
- **Date**: 2026-02-19
- **Ticket**: #174
- **What happened**: In `columnCleanup` (column departure cleanup), columns were matched by positional index (`b.columnConfigs[colIdx]`) instead of by name comparison. This worked by accident when config column order matched provider column order, but would silently misfire if a provider returned columns in a different order than the user's config.
- **Root cause**: While most other places in the codebase (`resolveAction`, `rebuildNormalHints`) use name-based matching with `strings.EqualFold`, the `columnCleanup` function took a shortcut using array index. The code reviewer found this pattern inconsistency during review, not via test failure.
- **Fix**: Changed `columnCleanup` to look up the column name first (`colName := b.Columns[colIdx].Title`), then iterate `b.columnConfigs` to find the matching config entry using `strings.EqualFold(config.Name, colName)`.
- **Rule**: Column matching must always use name equality (`strings.EqualFold`) rather than positional index. Never assume config column order matches provider column order. When a type of operation (resolveAction, rebuildHints, cleanup, etc.) appears in multiple functions, use the same matching strategy consistently across all of them.

### BubbleTea Cmd returned from helper functions must reach all code paths
- **Date**: 2026-02-19
- **Ticket**: #174
- **What happened**: In `handleBoardFetched`, the cleanup command (`cleanupCmd`) was computed early in the function but only used in the `if b.refreshing` branch. The non-refreshing path (after config save) returned `cmd` without batching in `cleanupCmd`, silently discarding the cleanup work.
- **Root cause**: The command was computed before the conditional split, and only one branch remembered to apply it. Without explicit batch operations on all code paths, computed commands are easily lost. The test suite passed because the test only covered the refreshing path or did not execute the async cleanup command.
- **Fix**: Added explicit batch logic to the non-refreshing return: `if cleanupCmd != nil { cmd = tea.Batch(cmd, cleanupCmd) }` before returning. This ensures cleanup is scheduled regardless of the refresh state.
- **Rule**: When a helper function (like a command builder) returns a `tea.Cmd`, track where it is applied in all conditional branches. Use `tea.Batch()` explicitly on every code path that returns. Consider computing such commands right before use (not at the start of the function) to reduce the span where they can be forgotten.
