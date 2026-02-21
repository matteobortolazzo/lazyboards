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
	view := sb.View()

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
	view := sb.View()

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
