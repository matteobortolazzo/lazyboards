package main

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/agentwatch"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
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
	hints          []Hint
	message        string
	level          StatusLevel
	gitStatus      string
	dispatchStatus string
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

// SetGitStatus sets the pre-formatted git status segment shown right-aligned
// in the status bar. Pass "" to hide the segment (e.g. on a read failure).
func (s *StatusBar) SetGitStatus(segment string) {
	s.gitStatus = segment
}

// SetDispatchStatus sets the pre-formatted dispatch-loop status segment shown
// right-aligned in the status bar, to the left of the git segment. Pass ""
// to hide the segment (e.g. the loop is disabled or the watcher is down).
func (s *StatusBar) SetDispatchStatus(segment string) {
	s.dispatchStatus = segment
}

// formatGitSegment formats a git Status into a compact, plain-ASCII segment,
// e.g. "main +2~1 ↑3↓0", colored: staged (added) in green, unstaged (deleted)
// in red, ahead (push) and behind (pull) both in the same gentle orange since
// they're sync state rather than a warning. The ahead/behind portion is
// omitted entirely when HasUpstream is false.
func formatGitSegment(status gitdetect.Status) string {
	segment := status.Branch + " " +
		gitAddedStyle.Render("+"+strconv.Itoa(status.Staged)) +
		gitDeletedStyle.Render("~"+strconv.Itoa(status.Unstaged))
	if status.HasUpstream {
		segment += " " +
			gitAheadStyle.Render("↑"+strconv.Itoa(status.Ahead)) +
			gitBehindStyle.Render("↓"+strconv.Itoa(status.Behind))
	}
	return segment
}

// formatDispatchSegment formats a dispatch loop DispatchState into a compact
// segment, e.g. "⟳ dispatch". Visibility mirrors the daemon's own status
// frontend: hidden (returns "") when state is nil (watcher hasn't delivered
// dispatch data, e.g. a pre-#219 daemon) or when the loop is disabled.
// Precedence for the visible states: a failed last dispatch pass (LastError
// set) always renders via statusErrorStyle, the highest-priority visible
// state. Otherwise, when the loop is enabled but the daemon managing it
// isn't actually running (Enabled && !DaemonRunning — the same "daemon not
// running" problem state the dispatch modal's renderLoopLine distinguishes),
// the segment also renders via statusErrorStyle so it isn't mistaken for a
// healthy running loop. Only Enabled && DaemonRunning with no error renders
// via the normal "on" dispatchSegmentStyle.
func formatDispatchSegment(state *agentwatch.DispatchState) string {
	if state == nil || !state.Enabled {
		return ""
	}
	const segment = "⟳ dispatch"
	if state.LastError != "" {
		return statusErrorStyle.Render(segment)
	}
	if !state.DaemonRunning {
		return statusErrorStyle.Render(segment)
	}
	return dispatchSegmentStyle.Render(segment)
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
// the status bar (e.g. "▶2 !1"): running via agentRunningStyle, need_input via
// agentNeedInputStyle. Zero-valued counts are omitted; both zero yields "".
func agentPrefix(running, needInput int) string {
	var tokens []string
	if running > 0 {
		tokens = append(tokens, agentRunningStyle.Render("▶"+strconv.Itoa(running)))
	}
	if needInput > 0 {
		tokens = append(tokens, agentNeedInputStyle.Render("!"+strconv.Itoa(needInput)))
	}
	return strings.Join(tokens, " ")
}

// renderHints renders hints within the given width, truncating with a
// trailing ellipsis when they don't all fit. Returns "" when there are no
// hints.
func renderHints(hints []Hint, width int) string {
	if len(hints) == 0 {
		return ""
	}

	separator := hintDescStyle.Render(" | ")
	separatorWidth := lipgloss.Width(separator)
	ellipsis := hintDescStyle.Render(" ...")
	ellipsisWidth := lipgloss.Width(ellipsis)

	var parts []string
	currentWidth := 0

	for i, h := range hints {
		part := hintKeyStyle.Render(h.Key) + hintDescStyle.Render(": "+h.Desc)
		partWidth := lipgloss.Width(part)

		addedWidth := partWidth
		if i > 0 {
			addedWidth += separatorWidth
		}

		// For non-last hints, reserve space for the ellipsis that would be
		// appended if a later hint doesn't fit.
		spaceNeeded := currentWidth + addedWidth
		if i < len(hints)-1 {
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

// View renders the status bar, truncating hints that exceed the given width.
// The agent-status counts (running, needInput) and the board-wide linked-PR
// count render as an always-visible prefix ahead of both hints and timed
// messages; each token is omitted when its count is zero, and when all are zero
// the prefix and its separator are omitted entirely. Timed messages are still
// shown untruncated. counts is variadic (running, needInput, prCount) for
// caller convenience; missing values default to 0.
//
// When a git status segment is set (via SetGitStatus) and/or a dispatch
// status segment is set (via SetDispatchStatus), they are right-aligned
// after the hints, taking priority for space over hints: hints truncate to
// make room for them. When both are set and there isn't enough width for
// both, the dispatch segment is dropped first (git wins the contention);
// when even the git segment alone doesn't fit, it is dropped too (not
// truncated). Timed messages always override both.
func (s StatusBar) View(width int, counts ...int) string {
	var running, needInput, prCount int
	if len(counts) > 0 {
		running = counts[0]
	}
	if len(counts) > 1 {
		needInput = counts[1]
	}
	if len(counts) > 2 {
		prCount = counts[2]
	}

	prefix := agentPrefix(running, needInput)
	// The board-wide linked-PR count trails the agent tokens in the same
	// always-visible prefix: omitted when zero, and (because the prefix is
	// reserved out of the width before hints/tail segments) never truncated.
	if prCount > 0 {
		prToken := prIndicatorStyle.Render(linkedPRGlyph + strconv.Itoa(prCount))
		if prefix != "" {
			prefix += " " + prToken
		} else {
			prefix = prToken
		}
	}
	if prefix != "" {
		prefix += hintDescStyle.Render(" | ")
	}

	if s.message != "" {
		if st := s.level.style(); st != nil {
			return prefix + st.Render(s.message)
		}
		return prefix + s.message
	}

	// The prefix consumes width that is no longer available for hints.
	width -= lipgloss.Width(prefix)

	// Build the candidate tail segments (dispatch + git) in priority order:
	// try both together first, then fall back to git alone (dispatch is
	// dropped first on width contention), then dispatch alone if there is no
	// git segment to compete with. Whichever candidate first fits (together
	// with at least the truncated hints) wins.
	var candidates []string
	switch {
	case s.dispatchStatus != "" && s.gitStatus != "":
		candidates = []string{s.dispatchStatus + " " + s.gitStatus, s.gitStatus}
	case s.gitStatus != "":
		candidates = []string{s.gitStatus}
	case s.dispatchStatus != "":
		candidates = []string{s.dispatchStatus}
	}

	for _, tail := range candidates {
		tailWidth := lipgloss.Width(tail)
		reserved := tailWidth + 1 // 1-space separator before the tail segment
		if reserved > width {
			continue
		}
		hintsWidth := width - reserved
		hintsView := renderHints(s.hints, hintsWidth)
		if lipgloss.Width(hintsView) <= hintsWidth {
			padding := width - lipgloss.Width(hintsView) - tailWidth
			return prefix + hintsView + strings.Repeat(" ", padding) + tail
		}
		// Not enough room for hints alongside this candidate; try the next
		// (lower-priority) candidate.
	}

	return prefix + renderHints(s.hints, width)
}
