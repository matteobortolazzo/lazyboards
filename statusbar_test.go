package main

import (
	"strings"
	"testing"
	"time"
)

// --- StatusBar: Basic Rendering ---

func TestStatusBar_ViewShowsHints(t *testing.T) {
	hints := []Hint{
		{Key: "n", Desc: "New"},
		{Key: "r", Desc: "Refresh"},
		{Key: "q", Desc: "Quit"},
	}
	sb := NewStatusBar(hints)
	view := sb.View()

	// Each hint should be rendered as "key: desc".
	for _, h := range hints {
		expected := h.Key + ": " + h.Desc
		if !strings.Contains(view, expected) {
			t.Errorf("View() = %q, want it to contain %q", view, expected)
		}
	}

	// Hints should be separated by " | ".
	if !strings.Contains(view, " | ") {
		t.Errorf("View() = %q, want hints separated by %q", view, " | ")
	}
}

func TestStatusBar_EmptyHints_RendersEmpty(t *testing.T) {
	sb := NewStatusBar(nil)
	view := sb.View()
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
	view := sb.View()

	if !strings.Contains(view, "Board refreshed") {
		t.Errorf("View() = %q, want it to contain %q", view, "Board refreshed")
	}
	if strings.Contains(view, "n: New") {
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
	view := sb.View()

	if strings.Contains(view, "Temporary message") {
		t.Errorf("View() = %q, should NOT contain message after ClearMessage()", view)
	}
	if !strings.Contains(view, "n: New") {
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
	view := sb.View()

	// New hints should be visible.
	if !strings.Contains(view, "esc: Cancel") {
		t.Errorf("View() = %q, want it to contain %q after SetActionHints()", view, "esc: Cancel")
	}
	if !strings.Contains(view, "enter: Submit") {
		t.Errorf("View() = %q, want it to contain %q after SetActionHints()", view, "enter: Submit")
	}

	// Old hints should NOT be visible.
	if strings.Contains(view, "n: New") {
		t.Errorf("View() = %q, should NOT contain old hints after SetActionHints()", view)
	}
}
