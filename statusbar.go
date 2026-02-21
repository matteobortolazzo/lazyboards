package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Hint represents a single key-description pair shown in the status bar.
type Hint struct {
	Key  string
	Desc string
}

// clearStatusMsg is sent when a timed message should be cleared.
type clearStatusMsg struct{}

// StatusBar displays contextual key hints and timed messages.
type StatusBar struct {
	hints   []Hint
	message string
}

// NewStatusBar creates a StatusBar with the given default hints.
func NewStatusBar(hints []Hint) StatusBar {
	return StatusBar{hints: hints}
}

// SetTimedMessage sets a temporary message that overrides hints.
// It returns a tea.Cmd that will send a clearStatusMsg after the duration.
func (s *StatusBar) SetTimedMessage(msg string, duration time.Duration) tea.Cmd {
	s.message = msg
	return tea.Tick(duration, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

// ClearMessage removes the timed message and restores hints.
func (s *StatusBar) ClearMessage() {
	s.message = ""
}

// SetActionHints replaces the current hints.
func (s *StatusBar) SetActionHints(hints []Hint) {
	s.hints = hints
}

// View renders the status bar.
func (s StatusBar) View() string {
	if s.message != "" {
		return s.message
	}
	if len(s.hints) == 0 {
		return ""
	}
	parts := make([]string, len(s.hints))
	for i, h := range s.hints {
		parts[i] = hintKeyStyle.Render(h.Key) + hintDescStyle.Render(": "+h.Desc)
	}
	return strings.Join(parts, hintDescStyle.Render(" | "))
}
