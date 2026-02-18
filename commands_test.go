package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/action"
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

// --- truncateOutput unit tests ---

func TestTruncateOutput_ShortString(t *testing.T) {
	input := "short error"
	maxLen := 200
	got := truncateOutput(input, maxLen)
	if got != input {
		t.Errorf("truncateOutput(%q, %d) = %q, want %q (unchanged)", input, maxLen, got, input)
	}
}

func TestTruncateOutput_ExactLimit(t *testing.T) {
	input := strings.Repeat("x", 200)
	maxLen := 200
	got := truncateOutput(input, maxLen)
	if got != input {
		t.Errorf("truncateOutput(len=%d, %d) should return input unchanged, got len=%d", len(input), maxLen, len(got))
	}
}

func TestTruncateOutput_LongString(t *testing.T) {
	maxLen := 200
	input := strings.Repeat("a", maxLen+100)
	got := truncateOutput(input, maxLen)

	// Result should be truncated to maxLen runes plus the "..." suffix.
	expectedLen := maxLen + len("...")
	if len([]rune(got)) != expectedLen {
		t.Errorf("truncateOutput(len=%d, %d) returned %d runes, want %d", len([]rune(input)), maxLen, len([]rune(got)), expectedLen)
	}

	// Result should end with "..." to indicate truncation.
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncateOutput() result should end with %q, got %q", "...", got[len(got)-10:])
	}
}

func TestTruncateOutput_EmptyString(t *testing.T) {
	got := truncateOutput("", 200)
	if got != "" {
		t.Errorf("truncateOutput(%q, 200) = %q, want empty string", "", got)
	}
}

func TestTruncateOutput_Unicode(t *testing.T) {
	// Each character here is multi-byte in UTF-8 but counts as 1 rune.
	maxLen := 10
	input := strings.Repeat("\u00e9", maxLen+5) // 15 runes of e-acute
	got := truncateOutput(input, maxLen)

	// Should truncate by rune count, not byte count.
	gotRunes := []rune(got)
	expectedRuneCount := maxLen + len([]rune("..."))
	if len(gotRunes) != expectedRuneCount {
		t.Errorf("truncateOutput(rune_len=%d, %d) returned %d runes, want %d", len([]rune(input)), maxLen, len(gotRunes), expectedRuneCount)
	}

	// The first maxLen runes should be the original content.
	originalPrefix := string([]rune(input)[:maxLen])
	if !strings.HasPrefix(got, originalPrefix) {
		t.Errorf("truncateOutput() should preserve first %d runes of original content", maxLen)
	}

	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncateOutput() result should end with %q", "...")
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
