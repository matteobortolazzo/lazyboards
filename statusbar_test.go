package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// --- StatusBar: Basic Rendering ---

func TestStatusBar_ViewShowsHints(t *testing.T) {
	hints := []Hint{
		{Key: "n", Desc: "New"},
		{Key: "r", Desc: "Refresh"},
		{Key: "q", Desc: "Quit"},
	}
	sb := NewStatusBar(hints)
	view := sb.View(200)

	// Each hint's key and description should appear in the rendered output.
	for _, h := range hints {
		if !strings.Contains(view, h.Key) {
			t.Errorf("View() = %q, want it to contain key %q", view, h.Key)
		}
		if !strings.Contains(view, h.Desc) {
			t.Errorf("View() = %q, want it to contain desc %q", view, h.Desc)
		}
	}

	// Hints should be separated by " | " (possibly styled).
	if !strings.Contains(view, "|") {
		t.Errorf("View() = %q, want hints separated by pipe character", view)
	}
}

func TestStatusBar_EmptyHints_RendersEmpty(t *testing.T) {
	sb := NewStatusBar(nil)
	view := sb.View(200)
	if strings.TrimSpace(view) != "" {
		t.Errorf("View() with no hints = %q, want empty or whitespace-only string", view)
	}
}

// --- StatusBar: Timed Messages ---

func TestStatusBar_TimedMessage_OverridesHints(t *testing.T) {
	hints := []Hint{
		{Key: "n", Desc: "New"},
		{Key: "r", Desc: "Refresh"},
		{Key: "q", Desc: "Quit"},
	}
	sb := NewStatusBar(hints)
	sb.SetTimedMessage("Board refreshed", 3*time.Second)
	view := sb.View(200)

	if !strings.Contains(view, "Board refreshed") {
		t.Errorf("View() = %q, want it to contain %q", view, "Board refreshed")
	}
	// Hint descriptions should NOT appear when a timed message is active.
	if strings.Contains(view, "New") {
		t.Errorf("View() = %q, should NOT contain hints when a timed message is active", view)
	}
}

func TestStatusBar_ClearMessage_RestoresHints(t *testing.T) {
	hints := []Hint{
		{Key: "n", Desc: "New"},
		{Key: "r", Desc: "Refresh"},
		{Key: "q", Desc: "Quit"},
	}
	sb := NewStatusBar(hints)
	sb.SetTimedMessage("Temporary message", 3*time.Second)
	sb.ClearMessage()
	view := sb.View(200)

	if strings.Contains(view, "Temporary message") {
		t.Errorf("View() = %q, should NOT contain message after ClearMessage()", view)
	}
	if !strings.Contains(view, "New") {
		t.Errorf("View() = %q, want hints restored after ClearMessage()", view)
	}
}

func TestStatusBar_SetTimedMessage_ReturnsCmd(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	cmd := sb.SetTimedMessage("Done!", 3*time.Second)
	if cmd == nil {
		t.Error("SetTimedMessage() should return a non-nil tea.Cmd")
	}
}

// --- StatusBar: SetActionHints ---

func TestStatusBar_SetActionHints_OverridesDefaults(t *testing.T) {
	defaultHints := []Hint{
		{Key: "n", Desc: "New"},
		{Key: "q", Desc: "Quit"},
	}
	sb := NewStatusBar(defaultHints)

	newHints := []Hint{
		{Key: "esc", Desc: "Cancel"},
		{Key: "enter", Desc: "Submit"},
	}
	sb.SetActionHints(newHints)
	view := sb.View(200)

	// New hint keys and descriptions should be visible.
	if !strings.Contains(view, "esc") {
		t.Errorf("View() = %q, want it to contain key %q after SetActionHints()", view, "esc")
	}
	if !strings.Contains(view, "Cancel") {
		t.Errorf("View() = %q, want it to contain desc %q after SetActionHints()", view, "Cancel")
	}
	if !strings.Contains(view, "enter") {
		t.Errorf("View() = %q, want it to contain key %q after SetActionHints()", view, "enter")
	}
	if !strings.Contains(view, "Submit") {
		t.Errorf("View() = %q, want it to contain desc %q after SetActionHints()", view, "Submit")
	}

	// Old hint descriptions should NOT be visible.
	if strings.Contains(view, "New") {
		t.Errorf("View() = %q, should NOT contain old hint desc after SetActionHints()", view)
	}
}

// --- StatusBar: Hint Truncation ---

func TestStatusBar_ViewTruncatesHintsAtWidth(t *testing.T) {
	// Create a StatusBar with many hints so that not all fit in a narrow width.
	hints := []Hint{
		{Key: "o", Desc: "Open"},
		{Key: "n", Desc: "New"},
		{Key: "r", Desc: "Refresh"},
		{Key: "c", Desc: "Config"},
		{Key: "q", Desc: "Quit"},
	}
	sb := NewStatusBar(hints)

	// Compute the width needed for the first 2 hints plus the styled ellipsis.
	hint0 := hintKeyStyle.Render(hints[0].Key) + hintDescStyle.Render(": "+hints[0].Desc)
	hint1 := hintKeyStyle.Render(hints[1].Key) + hintDescStyle.Render(": "+hints[1].Desc)
	separator := hintDescStyle.Render(" | ")
	ellipsis := hintDescStyle.Render(" ...")
	ellipsisWidth := lipgloss.Width(ellipsis)
	twoHintsWidth := lipgloss.Width(hint0) + lipgloss.Width(separator) + lipgloss.Width(hint1)

	// Use a width that fits 2 hints + ellipsis but not the 3rd hint.
	width := twoHintsWidth + ellipsisWidth + 1

	view := sb.View(width)

	// First 2 hints' keys and descriptions should appear.
	if !strings.Contains(view, hints[0].Key) {
		t.Errorf("View(%d) = %q, want it to contain first hint key %q", width, view, hints[0].Key)
	}
	if !strings.Contains(view, hints[0].Desc) {
		t.Errorf("View(%d) = %q, want it to contain first hint desc %q", width, view, hints[0].Desc)
	}
	if !strings.Contains(view, hints[1].Key) {
		t.Errorf("View(%d) = %q, want it to contain second hint key %q", width, view, hints[1].Key)
	}
	if !strings.Contains(view, hints[1].Desc) {
		t.Errorf("View(%d) = %q, want it to contain second hint desc %q", width, view, hints[1].Desc)
	}

	// The 3rd hint should NOT appear.
	if strings.Contains(view, hints[2].Desc) {
		t.Errorf("View(%d) = %q, should NOT contain third hint desc %q (truncated)", width, view, hints[2].Desc)
	}

	// Truncation indicator " ..." should appear.
	if !strings.Contains(view, "...") {
		t.Errorf("View(%d) = %q, want it to contain truncation indicator %q", width, view, "...")
	}
}

func TestStatusBar_ViewNoTruncationWhenAllFit(t *testing.T) {
	hints := []Hint{
		{Key: "o", Desc: "Open"},
		{Key: "n", Desc: "New"},
	}
	sb := NewStatusBar(hints)

	// Use a large width where all hints easily fit.
	view := sb.View(200)

	// All hint keys and descriptions should appear.
	for _, h := range hints {
		if !strings.Contains(view, h.Key) {
			t.Errorf("View(200) = %q, want it to contain key %q", view, h.Key)
		}
		if !strings.Contains(view, h.Desc) {
			t.Errorf("View(200) = %q, want it to contain desc %q", view, h.Desc)
		}
	}

	// No truncation indicator should appear.
	if strings.Contains(view, "...") {
		t.Errorf("View(200) = %q, should NOT contain truncation indicator when all hints fit", view)
	}
}

func TestStatusBar_ViewTimedMessageIgnoresWidth(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	sb.SetTimedMessage("Board refreshed", 3*time.Second)

	// Call with a tiny width -- the timed message should still appear in full.
	view := sb.View(1)

	if !strings.Contains(view, "Board refreshed") {
		t.Errorf("View(1) = %q, want it to contain timed message %q regardless of width", view, "Board refreshed")
	}
}

func TestStatusBar_ViewTruncatedOutputFitsWithinWidth(t *testing.T) {
	// The output of View() must never exceed the given width, even when
	// the ellipsis is appended after truncation. Use a wide first hint so
	// that hint0 + ellipsis would overflow if ellipsis space is not reserved.
	hints := []Hint{
		{Key: "enter", Desc: "Submit Changes"},
		{Key: "n", Desc: "New"},
	}
	sb := NewStatusBar(hints)

	// Compute the first hint's rendered width.
	hint0 := hintKeyStyle.Render(hints[0].Key) + hintDescStyle.Render(": "+hints[0].Desc)
	hint0Width := lipgloss.Width(hint0)
	ellipsis := hintDescStyle.Render(" ...")
	ellipsisWidth := lipgloss.Width(ellipsis)

	// Set width so that hint0 fits alone but hint0 + ellipsis does NOT fit.
	// This triggers the bug: hint0 is accepted, hint1 overflows, ellipsis is
	// appended to hint0, and the combined output exceeds width.
	width := hint0Width + ellipsisWidth - 1

	view := sb.View(width)
	viewWidth := lipgloss.Width(view)

	if viewWidth > width {
		t.Errorf("View(%d) rendered width = %d, exceeds allowed width; output = %q", width, viewWidth, view)
	}
}

func TestStatusBar_ViewEmptyHintsWithWidth(t *testing.T) {
	sb := NewStatusBar(nil)
	view := sb.View(50)

	if strings.TrimSpace(view) != "" {
		t.Errorf("View(50) with no hints = %q, want empty or whitespace-only string", view)
	}
}

func TestStatusBar_ViewFirstHintDoesNotFit(t *testing.T) {
	hints := []Hint{
		{Key: "o", Desc: "Open"},
		{Key: "n", Desc: "New"},
	}
	sb := NewStatusBar(hints)

	// Compute width dynamically: one cell short of fitting the first hint.
	hint0 := hintKeyStyle.Render(hints[0].Key) + hintDescStyle.Render(": "+hints[0].Desc)
	width := lipgloss.Width(hint0) - 1

	view := sb.View(width)

	// With such a narrow width, " ..." should appear as a truncation indicator.
	if !strings.Contains(view, "...") {
		t.Errorf("View(%d) = %q, want it to contain truncation indicator %q when no hints fit", width, view, "...")
	}
}
