package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func TestWrapTitle_ShortTitleSingleLine(t *testing.T) {
	title := "Short"
	maxWidth := 20
	got := wrapTitle(title, maxWidth, 0)
	if len(got) != 1 {
		t.Errorf("wrapTitle(%q, %d, 0) returned %d lines, want 1", title, maxWidth, len(got))
	}
	if got[0] != title {
		t.Errorf("wrapTitle(%q, %d, 0)[0] = %q, want %q", title, maxWidth, got[0], title)
	}
}

func TestWrapTitle_ExactWidthSingleLine(t *testing.T) {
	title := "Exactly ten"
	maxWidth := len(title)
	got := wrapTitle(title, maxWidth, 0)
	if len(got) != 1 {
		t.Errorf("wrapTitle(%q, %d, 0) returned %d lines, want 1", title, maxWidth, len(got))
	}
	if got[0] != title {
		t.Errorf("wrapTitle(%q, %d, 0)[0] = %q, want %q", title, maxWidth, got[0], title)
	}
}

func TestWrapTitle_WrapsAtWordBoundary(t *testing.T) {
	title := "This is a very long title that should wrap"
	maxWidth := 20
	indentWidth := 2
	got := wrapTitle(title, maxWidth, indentWidth)

	if len(got) < 2 {
		t.Fatalf("wrapTitle(%q, %d, %d) returned %d lines, want >= 2", title, maxWidth, indentWidth, len(got))
	}

	// First line must fit within maxWidth.
	if len([]rune(got[0])) > maxWidth {
		t.Errorf("first line %q has %d runes, want <= %d", got[0], len([]rune(got[0])), maxWidth)
	}

	// Continuation lines must be indented by indentWidth spaces.
	indent := strings.Repeat(" ", indentWidth)
	for i := 1; i < len(got); i++ {
		if !strings.HasPrefix(got[i], indent) {
			t.Errorf("continuation line %d = %q, want prefix %q", i, got[i], indent)
		}
		if len([]rune(got[i])) > maxWidth {
			t.Errorf("continuation line %d = %q has %d runes, want <= %d", i, got[i], len([]rune(got[i])), maxWidth)
		}
	}

	// All original words should be present across all lines.
	joined := strings.Join(got, " ")
	for _, word := range strings.Fields(title) {
		if !strings.Contains(joined, word) {
			t.Errorf("word %q missing from wrapped output: %v", word, got)
		}
	}
}

func TestWrapTitle_LongWordCharacterBreak(t *testing.T) {
	title := "abcdefghij"
	maxWidth := 5
	got := wrapTitle(title, maxWidth, 0)

	if len(got) < 2 {
		t.Fatalf("wrapTitle(%q, %d, 0) returned %d lines, want >= 2", title, maxWidth, len(got))
	}

	// Each line must not exceed maxWidth.
	for i, line := range got {
		if len([]rune(line)) > maxWidth {
			t.Errorf("line %d = %q has %d runes, want <= %d", i, line, len([]rune(line)), maxWidth)
		}
	}

	// All characters should be preserved.
	joined := strings.Join(got, "")
	joinedTrimmed := strings.ReplaceAll(joined, " ", "")
	if joinedTrimmed != title {
		t.Errorf("character-broken lines joined = %q, want %q", joinedTrimmed, title)
	}
}

func TestWrapTitle_EmptyTitle(t *testing.T) {
	got := wrapTitle("", 20, 0)
	if len(got) < 1 {
		t.Fatal("wrapTitle(\"\", 20, 0) returned empty slice, want at least one element")
	}
}

func TestWrapTitle_VeryNarrowWidth(t *testing.T) {
	// maxWidth of 1 should not panic and should produce output.
	got := wrapTitle("Hello", 1, 0)
	if len(got) < 1 {
		t.Fatal("wrapTitle(\"Hello\", 1, 0) returned empty slice, want at least one element")
	}
	for i, line := range got {
		if len([]rune(line)) > 1 {
			t.Errorf("line %d = %q has %d runes, want <= 1", i, line, len([]rune(line)))
		}
	}

	// maxWidth of 2 should also not panic.
	got2 := wrapTitle("Hi there", 2, 0)
	if len(got2) < 1 {
		t.Fatal("wrapTitle(\"Hi there\", 2, 0) returned empty slice, want at least one element")
	}
	for i, line := range got2 {
		if len([]rune(line)) > 2 {
			t.Errorf("line %d = %q has %d runes, want <= 2", i, line, len([]rune(line)))
		}
	}
}

func TestWrapTitle_MultipleWraps(t *testing.T) {
	title := "one two three four five six seven eight nine ten eleven twelve"
	maxWidth := 15
	indentWidth := 2
	got := wrapTitle(title, maxWidth, indentWidth)

	if len(got) < 3 {
		t.Fatalf("wrapTitle(%q, %d, %d) returned %d lines, want >= 3", title, maxWidth, indentWidth, len(got))
	}

	// First line fits within maxWidth.
	if len([]rune(got[0])) > maxWidth {
		t.Errorf("first line %q has %d runes, want <= %d", got[0], len([]rune(got[0])), maxWidth)
	}

	// All continuation lines are indented and fit within maxWidth.
	indent := strings.Repeat(" ", indentWidth)
	for i := 1; i < len(got); i++ {
		if !strings.HasPrefix(got[i], indent) {
			t.Errorf("continuation line %d = %q, want prefix %q", i, got[i], indent)
		}
		if len([]rune(got[i])) > maxWidth {
			t.Errorf("continuation line %d = %q has %d runes, want <= %d", i, got[i], len([]rune(got[i])), maxWidth)
		}
	}

	// All original words should be present.
	joined := strings.Join(got, " ")
	for _, word := range strings.Fields(title) {
		if !strings.Contains(joined, word) {
			t.Errorf("word %q missing from wrapped output: %v", word, got)
		}
	}
}

// --- truncateCell unit tests ---
//
// truncateCell (view.go, beside padCell) is the #595 replacement for the
// deleted rune-based truncateOutput: it measures with lipgloss.Width and
// truncates with ansi.Truncate(s, n, "\u2026") so the ellipsis itself counts
// inside the requested cell budget. These cases lock in the cell-based
// contract directly -- they must not hardcode the old rune-counting shape
// (e.g. a fixed "+3" for a literal "..." suffix), since a wide/CJK/emoji
// rune is 2 cells and a rune-based budget would let truncated output run to
// roughly double the intended terminal width.

func TestTruncateCell_NonPositiveWidth_ReturnsEmpty(t *testing.T) {
	if got := truncateCell("abc", 0); got != "" {
		t.Errorf("truncateCell(%q, 0) = %q, want empty string", "abc", got)
	}
	if got := truncateCell("abc", -5); got != "" {
		t.Errorf("truncateCell(%q, -5) = %q, want empty string", "abc", got)
	}
}

func TestTruncateCell_FitsWithinBudget_ReturnsInputUnchanged(t *testing.T) {
	input := "short error"
	got := truncateCell(input, 200)
	if got != input {
		t.Errorf("truncateCell(%q, 200) = %q, want %q (unchanged)", input, got, input)
	}
}

func TestTruncateCell_ExactCellFit_ReturnsInputUnchanged(t *testing.T) {
	input := strings.Repeat("x", 200)
	got := truncateCell(input, lipgloss.Width(input))
	if got != input {
		t.Errorf("truncateCell(len=%d, %d) should return input unchanged, got %q", len(input), lipgloss.Width(input), got)
	}
}

func TestTruncateCell_EmptyString_ReturnsEmpty(t *testing.T) {
	got := truncateCell("", 200)
	if got != "" {
		t.Errorf("truncateCell(%q, 200) = %q, want empty string", "", got)
	}
}

func TestTruncateCell_TruncatesASCII_NeverExceedsBudgetAndMarksTruncation(t *testing.T) {
	budget := 200
	input := strings.Repeat("a", budget+100)
	got := truncateCell(input, budget)

	if w := lipgloss.Width(got); w > budget {
		t.Fatalf("lipgloss.Width(truncateCell(len=%d, %d)) = %d, want <= %d", len(input), budget, w, budget)
	}
	if !strings.Contains(got, "\u2026") {
		t.Errorf("truncateCell(len=%d, %d) = %q, want it to contain the ellipsis marking truncation", len(input), budget, got)
	}
	if got == input {
		t.Errorf("truncateCell(len=%d, %d) returned the input unchanged, want it truncated", len(input), budget)
	}
}

func TestTruncateCell_MultibyteNarrowRunes_BudgetCountsCellsNotBytes(t *testing.T) {
	// Each rune below ("\u00e9" repeated) is multi-byte in UTF-8 but still
	// only 1 terminal cell wide, so this locks in that byte-length is
	// irrelevant -- lipgloss.Width measures cells, not UTF-8 byte count.
	budget := 10
	input := strings.Repeat("\u00e9", budget+5)
	got := truncateCell(input, budget)

	if w := lipgloss.Width(got); w > budget {
		t.Fatalf("lipgloss.Width(truncateCell(...)) = %d, want <= %d", w, budget)
	}
	originalPrefix := string([]rune(input)[:budget-1])
	if !strings.HasPrefix(got, originalPrefix) {
		t.Errorf("truncateCell(...) = %q, want it to preserve the original content up to the truncation point", got)
	}
}

func TestTruncateCell_TruncatesCJK_NeverExceedsBudgetInCells(t *testing.T) {
	// Each CJK rune is 2 cells wide. A rune-based budget of 10 would keep up
	// to 10 of these (20 cells) -- double the intended terminal-cell budget.
	budget := 10
	input := strings.Repeat("\u4f60", 50) // "\u4f60"
	got := truncateCell(input, budget)

	if w := lipgloss.Width(got); w > budget {
		t.Fatalf("lipgloss.Width(truncateCell(CJK input, %d)) = %d, want <= %d", budget, w, budget)
	}
}

func TestTruncateCell_TruncatesEmoji_NeverExceedsBudgetInCells(t *testing.T) {
	// Each emoji rune below is 2 cells wide, same overshoot risk as CJK.
	budget := 9
	input := strings.Repeat("\U0001F600", 50) // "\ud83d\ude00"
	got := truncateCell(input, budget)

	if w := lipgloss.Width(got); w > budget {
		t.Fatalf("lipgloss.Width(truncateCell(emoji input, %d)) = %d, want <= %d", budget, w, budget)
	}
}

// --- Integration test: runShellCmd truncation ---

func TestRunShellCmd_TruncatesLongStderr(t *testing.T) {
	// Build a long stderr string that exceeds 200 characters.
	longStderr := strings.Repeat("E", 300)
	fe := &action.FakeExecutor{
		RunShellStderr: longStderr,
		RunShellErr:    errors.New("exit status 1"),
	}

	// Call runShellCmd directly and execute the returned tea.Cmd.
	cmd := runShellCmd(fe, "some-command")
	msg := cmd()

	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("runShellCmd returned %T, want actionResultMsg", msg)
	}

	if result.success {
		t.Error("expected success=false for a failed shell command")
	}

	// The message should be bounded. The "Error: " prefix plus truncated stderr
	// plus "..." should not exceed a reasonable length. The raw stderr is 300 chars,
	// so the message must be shorter than "Error: " + 300 chars.
	maxExpectedLen := len("Error: ") + maxErrorOutputLen + len("...")
	if len([]rune(result.message)) > maxExpectedLen {
		t.Errorf("actionResultMsg.message has %d runes, want <= %d (stderr should be truncated)", len([]rune(result.message)), maxExpectedLen)
	}

	// The message should still contain the "Error: " prefix.
	if !strings.HasPrefix(result.message, "Error: ") {
		t.Errorf("actionResultMsg.message should start with %q, got %q", "Error: ", result.message[:20])
	}
}

// TestRunShellCmd_TruncatesLongCJKStderr_BoundsCellWidthNotRunes is the #595
// regression guard for the runShellCmd error-message producer: a rune-based
// budget lets each 2-cell-wide CJK rune count as only 1, so a naive
// migration could still keep up to maxErrorOutputLen *runes* of CJK text --
// double the intended maxErrorOutputLen *cells*. The truncated portion of
// the message (after the fixed "Error: " prefix) must stay within
// maxErrorOutputLen terminal cells.
func TestRunShellCmd_TruncatesLongCJKStderr_BoundsCellWidthNotRunes(t *testing.T) {
	longStderr := strings.Repeat("测", 300)
	fe := &action.FakeExecutor{
		RunShellStderr: longStderr,
		RunShellErr:    errors.New("exit status 1"),
	}

	cmd := runShellCmd(fe, "some-command")
	msg := cmd()

	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("runShellCmd returned %T, want actionResultMsg", msg)
	}

	if !strings.HasPrefix(result.message, "Error: ") {
		t.Fatalf("actionResultMsg.message = %q, want it to start with %q", result.message, "Error: ")
	}
	truncated := strings.TrimPrefix(result.message, "Error: ")
	if w := lipgloss.Width(truncated); w > maxErrorOutputLen {
		t.Errorf("truncated stderr cell width = %d, want <= %d (maxErrorOutputLen cells, not runes)", w, maxErrorOutputLen)
	}
}

// TestRunShellCmd_TruncatesLongMultilineStderr_BoundsCellWidth is the #595
// security-review regression guard: truncateCell's fast path compares
// lipgloss.Width(s) (the widest single line) against the budget, not total
// length, so multi-line stderr where every line is individually narrow but
// the whole blob is huge bypasses truncation entirely if truncateCell is
// called before flattening newlines -- and critically, lipgloss.Width of
// that *untruncated* multi-line result still reads as narrow too (it keeps
// measuring only the widest line), so a naive width-only assertion would
// not catch the regression. runShellCmd must sanitize (via
// sanitizeSingleLine) before truncating, matching every other call site in
// this diff, so embedded newlines are flattened away and the message is
// both single-line and bounded to maxErrorOutputLen cells, regardless of
// how many lines the raw stderr contained.
func TestRunShellCmd_TruncatesLongMultilineStderr_BoundsCellWidth(t *testing.T) {
	longStderr := strings.Repeat("short line\n", 500)
	fe := &action.FakeExecutor{
		RunShellStderr: longStderr,
		RunShellErr:    errors.New("exit status 1"),
	}

	cmd := runShellCmd(fe, "some-command")
	msg := cmd()

	result, ok := msg.(actionResultMsg)
	if !ok {
		t.Fatalf("runShellCmd returned %T, want actionResultMsg", msg)
	}

	if !strings.HasPrefix(result.message, "Error: ") {
		t.Fatalf("actionResultMsg.message = %q, want it to start with %q", result.message, "Error: ")
	}
	truncated := strings.TrimPrefix(result.message, "Error: ")

	if strings.Contains(truncated, "\n") {
		t.Errorf("actionResultMsg.message retains embedded newlines from multi-line stderr; it must be sanitized (flattened) before truncation, not just cell-width truncated")
	}
	if w := lipgloss.Width(truncated); w > maxErrorOutputLen {
		t.Errorf("truncated stderr cell width = %d, want <= %d (maxErrorOutputLen cells); multi-line stderr must be sanitized before truncation", w, maxErrorOutputLen)
	}
}

// --- runCleanupCmds observability (#361) ---

// TestRunCleanupCmds_LogsSuccessfulCommand verifies a successfully executed
// cleanup command is logged via debuglog for post-incident forensics.
func TestRunCleanupCmds_LogsSuccessfulCommand(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	if err := debuglog.Init(path); err != nil {
		t.Fatalf("debuglog.Init(%q) error = %v, want nil", path, err)
	}

	fe := &action.FakeExecutor{}
	cmd := runCleanupCmds(fe, []string{"cenci close 42"})
	cmd()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file %q: %v", path, err)
	}
	if !strings.Contains(string(data), "cleanup: executed: cenci close 42") {
		t.Errorf("log file content = %q, want it to mention the executed command", string(data))
	}
}

// TestRunCleanupCmds_LogsFailedCommandWithError verifies a cleanup command
// that fails is logged with both the error and stderr so an incident can be
// diagnosed after the fact.
func TestRunCleanupCmds_LogsFailedCommandWithError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	if err := debuglog.Init(path); err != nil {
		t.Fatalf("debuglog.Init(%q) error = %v, want nil", path, err)
	}

	fe := &action.FakeExecutor{
		RunShellErr:    errors.New("exit status 1"),
		RunShellStderr: "no matching windows",
	}
	cmd := runCleanupCmds(fe, []string{"cenci close 99"})
	cmd()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file %q: %v", path, err)
	}
	content := string(data)
	if !strings.Contains(content, "cenci close 99") {
		t.Errorf("log file content = %q, want it to mention the failed command", content)
	}
	if !strings.Contains(content, "exit status 1") {
		t.Errorf("log file content = %q, want it to mention the error", content)
	}
	if !strings.Contains(content, "no matching windows") {
		t.Errorf("log file content = %q, want it to mention stderr", content)
	}
}

// TestRunCleanupCmds_StillReturnsCleanupResultMsgWithCorrectCount verifies
// the added logging does not change the existing cleanupResultMsg count
// contract, even when some commands fail.
func TestRunCleanupCmds_StillReturnsCleanupResultMsgWithCorrectCount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "debug.log")
	if err := debuglog.Init(path); err != nil {
		t.Fatalf("debuglog.Init(%q) error = %v, want nil", path, err)
	}

	fe := &action.FakeExecutor{
		RunShellErr: errors.New("boom"),
	}
	commands := []string{"cenci close 1", "cenci close 2", "cenci close 3"}
	cmd := runCleanupCmds(fe, commands)
	msg := cmd()

	result, ok := msg.(cleanupResultMsg)
	if !ok {
		t.Fatalf("runCleanupCmds() returned %T, want cleanupResultMsg", msg)
	}
	if result.count != len(commands) {
		t.Errorf("cleanupResultMsg.count = %d, want %d", result.count, len(commands))
	}
}

// --- closeCardCmd ---

func TestCloseCardCmd_Success(t *testing.T) {
	p := provider.NewFakeProvider()
	board, err := p.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	existingCard := board.Columns[0].Cards[0]

	cmd := closeCardCmd(p, existingCard.Number)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from closeCardCmd")
	}

	msg := cmd()
	result, ok := msg.(cardClosedMsg)
	if !ok {
		t.Fatalf("closeCardCmd() returned %T, want cardClosedMsg", msg)
	}
	if result.card.Number != existingCard.Number {
		t.Errorf("result.card.Number = %d, want %d", result.card.Number, existingCard.Number)
	}
}

func TestCloseCardCmd_Error(t *testing.T) {
	p := provider.NewFakeProvider()
	nonExistentNumber := 9999

	cmd := closeCardCmd(p, nonExistentNumber)
	if cmd == nil {
		t.Fatal("expected non-nil cmd from closeCardCmd")
	}

	msg := cmd()
	result, ok := msg.(cardCloseErrorMsg)
	if !ok {
		t.Fatalf("closeCardCmd() returned %T, want cardCloseErrorMsg", msg)
	}
	if result.err == nil {
		t.Error("expected non-nil err in cardCloseErrorMsg")
	}
}
