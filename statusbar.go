package main

import (
	"strconv"
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

// agentPrefix builds the styled agent-status count prefix shown at the head of
// the status bar (e.g. "▶2 ‼1"): running via agentRunningStyle, need_input via
// agentNeedInputStyle. Zero-valued counts are omitted; both zero yields "".
func agentPrefix(running, needInput int) string {
	var tokens []string
	if running > 0 {
		tokens = append(tokens, agentRunningStyle.Render("▶"+strconv.Itoa(running)))
	}
	if needInput > 0 {
		tokens = append(tokens, agentNeedInputStyle.Render("‼"+strconv.Itoa(needInput)))
	}
	return strings.Join(tokens, " ")
}

// View renders the status bar, truncating hints that exceed the given width.
// The agent-status counts (running, needInput) render as an always-visible
// prefix ahead of both hints and timed messages; when both are zero the prefix
// and its separator are omitted. Timed messages are still shown untruncated.
func (s StatusBar) View(width, running, needInput int) string {
	prefix := agentPrefix(running, needInput)
	if prefix != "" {
		prefix += hintDescStyle.Render(" | ")
	}

	if s.message != "" {
		if st := s.level.style(); st != nil {
			return prefix + st.Render(s.message)
		}
		return prefix + s.message
	}
	if len(s.hints) == 0 {
		return prefix
	}

	// The prefix consumes width that is no longer available for hints.
	width -= lipgloss.Width(prefix)

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
				return prefix + strings.Join(parts, separator) + ellipsis
			}
			return prefix + ellipsis
		}

		parts = append(parts, part)
		currentWidth += addedWidth
	}

	return prefix + strings.Join(parts, separator)
}
