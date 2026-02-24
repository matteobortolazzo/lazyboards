package main

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Hint represents a single key-description pair shown in the status bar.
type Hint struct {
	Key  string
	Desc string
}

// StatusLevel indicates the severity/category of a timed status message.
type StatusLevel int

const (
	StatusInfo    StatusLevel = iota // default/uncolored
	StatusSuccess                    // green
	StatusWarning                    // yellow
	StatusError                      // red
)

// clearStatusMsg is sent when a timed message should be cleared.
type clearStatusMsg struct{}

// StatusBar displays contextual key hints and timed messages.
type StatusBar struct {
	hints   []Hint
	message string
	level   StatusLevel
}

// NewStatusBar creates a StatusBar with the given default hints.
func NewStatusBar(hints []Hint) StatusBar {
	return StatusBar{hints: hints}
}

// SetTimedMessage sets a temporary message that overrides hints.
// It returns a tea.Cmd that will send a clearStatusMsg after the duration.
func (s *StatusBar) SetTimedMessage(msg string, level StatusLevel, duration time.Duration) tea.Cmd {
	s.message = msg
	s.level = level
	return tea.Tick(duration, func(time.Time) tea.Msg {
		return clearStatusMsg{}
	})
}

// ClearMessage removes the timed message and restores hints.
func (s *StatusBar) ClearMessage() {
	s.message = ""
	s.level = StatusInfo
}

// SetActionHints replaces the current hints.
func (s *StatusBar) SetActionHints(hints []Hint) {
	s.hints = hints
}

// style returns the lipgloss style for this level, or nil for unstyled (StatusInfo).
func (l StatusLevel) style() *lipgloss.Style {
	switch l {
	case StatusError:
		return &statusErrorStyle
	case StatusWarning:
		return &statusWarningStyle
	case StatusSuccess:
		return &statusSuccessStyle
	default:
		return nil
	}
}

// View renders the status bar, truncating hints that exceed the given width.
// Timed messages are returned as-is without truncation.
func (s StatusBar) View(width int) string {
	if s.message != "" {
		if st := s.level.style(); st != nil {
			return st.Render(s.message)
		}
		return s.message
	}
	if len(s.hints) == 0 {
		return ""
	}

	separator := hintDescStyle.Render(" | ")
	separatorWidth := lipgloss.Width(separator)
	ellipsis := hintDescStyle.Render(" ...")
	ellipsisWidth := lipgloss.Width(ellipsis)

	var parts []string
	currentWidth := 0

	for i, h := range s.hints {
		part := hintKeyStyle.Render(h.Key) + hintDescStyle.Render(": "+h.Desc)
		partWidth := lipgloss.Width(part)

		addedWidth := partWidth
		if i > 0 {
			addedWidth += separatorWidth
		}

		// For non-last hints, reserve space for the ellipsis that would be
		// appended if a later hint doesn't fit.
		spaceNeeded := currentWidth + addedWidth
		if i < len(s.hints)-1 {
			spaceNeeded += ellipsisWidth
		}

		if spaceNeeded > width {
			if len(parts) > 0 {
				return strings.Join(parts, separator) + ellipsis
			}
			return ellipsis
		}

		parts = append(parts, part)
		currentWidth += addedWidth
	}

	return strings.Join(parts, separator)
}
