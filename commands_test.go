package main

import (
	"strings"
	"testing"
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
