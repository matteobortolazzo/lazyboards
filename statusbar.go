package main

import (
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
)

// Hint represents a single key-description pair shown in the status bar.
type Hint struct {
	// Key is exempt from sanitizeSingleLine: every Key value originates
	// from a ParseKey-validated table entry (normalizeTable ->
	// ParseSequence -> ParseKey), and a hostile key is rejected at startup
	// (config.ResolveKeymap -> main.go's exit-1 path) before it can ever
	// reach a hint bar.
	Key string
	// Desc can carry untrusted repo-local data (e.g. a config.Action.Name)
	// and requires sanitizeSingleLine at its producers.
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
	// filterStatus/filterStatusCompact hold the pre-formatted active-filter
	// status segment (#654), full and compact forms respectively. Both empty
	// hides the segment.
	filterStatus        string
	filterStatusCompact string
	// stickyMessage is a separate, non-timed notice (used by the
	// update-available check, #444). It survives ClearMessage() -- only
	// ClearStickyMessage() removes it -- and only appears in View() when no
	// timed message is currently active.
	stickyMessage string
	stickyLevel   StatusLevel
}

// NewStatusBar creates a StatusBar with the given default hints.
func NewStatusBar(hints []Hint) StatusBar {
	return StatusBar{hints: hints}
}

// SetTimedMessage sets a temporary message that overrides hints. msg is
// sanitized via sanitizeSingleLine (#499) before being stored, so untrusted
// text that ends up here (e.g. subprocess stderr, a tmux window label) can
// never break the status bar onto multiple physical lines or smuggle
// bidi/zero-width control runes. This deliberately does NOT extend to
// SetGitStatus/SetDispatchStatus: both receive pre-formatted segments built
// by formatGitSegment/formatDispatchSegment that legitimately carry
// ANSI/SGR color styling, which a blanket sanitize here would strip. The
// dispatch segment is fully app-controlled (fixed literals + state-driven
// styling only) and needs no sanitization. The git segment's Branch field is
// untrusted (from `git branch --show-current`) but is already sanitized with
// sanitizeSingleLine at its source, inside formatGitSegment, before being
// composed into the styled segment -- that's why SetGitStatus itself doesn't
// need to sanitize again. A whitespace-only msg sanitizes to "", which View()
// already treats as "no timed message set" (falls through to the sticky
// message or hints).
// It returns a tea.Cmd that will send a clearStatusMsg after the duration.
func (s *StatusBar) SetTimedMessage(msg string, level StatusLevel, duration time.Duration) tea.Cmd {
	s.message = sanitizeSingleLine(msg)
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

// SetStickyMessage sets a persistent status-bar notice that stays visible
// (styled by level) until explicitly cleared with ClearStickyMessage(). Unlike
// SetTimedMessage, it does not auto-clear and is not affected by
// ClearMessage(). A timed message, while active, still takes precedence over
// the sticky message.
//
// Like SetTimedMessage, msg is sanitized via sanitizeSingleLine (#499) before
// being stored -- see that method's doc comment for why SetGitStatus/
// SetDispatchStatus are deliberately excluded from this sink (they carry
// pre-rendered ANSI/lipgloss styling that a blanket sanitize would strip, and
// the git segment's only untrusted field, Branch, is already sanitized at
// its source in formatGitSegment). A whitespace-only msg sanitizes to "", so
// HasStickyMessage() returns false and View() falls through to hints, the
// same as if SetStickyMessage had never been called.
func (s *StatusBar) SetStickyMessage(msg string, level StatusLevel) {
	s.stickyMessage = sanitizeSingleLine(msg)
	s.stickyLevel = level
}

// ClearStickyMessage removes the sticky message.
func (s *StatusBar) ClearStickyMessage() {
	s.stickyMessage = ""
	s.stickyLevel = StatusInfo
}

// HasStickyMessage reports whether a sticky message is currently set.
func (s StatusBar) HasStickyMessage() bool {
	return s.stickyMessage != ""
}

// SetActionHints replaces the current hints.
func (s *StatusBar) SetActionHints(hints []Hint) {
	s.hints = hints
}

// SetGitStatus sets the pre-formatted git status segment shown right-aligned
// in the status bar. Pass "" to hide the segment (e.g. on a read failure).
// Unlike SetTimedMessage/SetStickyMessage, this does not sanitize segment
// itself: it's pre-formatted by formatGitSegment, which already sanitizes
// its only untrusted input (status.Branch) at the point of concatenation,
// before the segment's legitimate ANSI/SGR styling is applied -- sanitizing
// again here would strip that styling.
func (s *StatusBar) SetGitStatus(segment string) {
	s.gitStatus = segment
}

// SetDispatchStatus sets the pre-formatted dispatch-loop status segment shown
// right-aligned in the status bar, to the left of the git segment. Pass ""
// to hide the segment (e.g. the loop is disabled or the watcher is down).
func (s *StatusBar) SetDispatchStatus(segment string) {
	s.dispatchStatus = segment
}

// SetFilterStatus sets the pre-formatted active-filter status segment (#654),
// shown left of the dispatch/git tail. full is used when there's room for the
// named-selection form; compact is the "⚑ N" fallback shown under width
// contention. Pass ("", "") to hide the segment entirely (no filter active).
// Like SetGitStatus/SetDispatchStatus, this deliberately does NOT sanitize:
// both strings are pre-formatted by formatFilterSegment, which already
// sanitizes its only untrusted input (the named selection's value) at the
// point of concatenation, before the segment's legitimate ANSI/SGR styling is
// applied -- sanitizing again here would strip that styling.
func (s *StatusBar) SetFilterStatus(full, compact string) {
	s.filterStatus = full
	s.filterStatusCompact = compact
}

// formatFilterSegment formats a filterSet into the active-filter status-bar
// segment's full and compact forms (#654). The full form names the entry
// sorted first by fs.ordered() (itemType, then case-insensitive value),
// appending " +N" when more than one selection is active (N = total-1); the
// compact form is always "⚑ <total>". An empty set returns ("", ""), hiding
// the segment. The named entry's value is sanitized with sanitizeSingleLine
// and clamped with truncateCell at filterSegmentNameMaxLen BEFORE styling --
// mirroring formatGitSegment's sanitize-at-the-point-of-concatenation
// pattern. If the sanitized/trimmed name is empty (a whitespace-only hostile
// value), the full form falls back to being identical to the compact form
// rather than rendering a blank-named "⚑  +N".
func formatFilterSegment(fs filterSet) (full, compact string) {
	if len(fs) == 0 {
		return "", ""
	}
	ordered := fs.ordered()
	name := truncateCell(sanitizeSingleLine(ordered[0].value), filterSegmentNameMaxLen)
	total := len(fs)

	compactText := filterGlyph + " " + strconv.Itoa(total)
	fullText := compactText
	if name != "" {
		fullText = filterGlyph + " " + name
		if total > 1 {
			fullText += " +" + strconv.Itoa(total-1)
		}
	}

	return filterSegmentStyle.Render(fullText), filterSegmentStyle.Render(compactText)
}

// joinNonEmpty joins the non-empty parts with a single space, skipping empty
// ones entirely (so a missing part never introduces a stray leading/
// doubled/trailing space).
func joinNonEmpty(parts ...string) string {
	var nonEmpty []string
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, " ")
}

// tailGroups builds the ordered priority-tier table of candidate tail
// segments for View()'s width-contention loop (#654), highest-priority tier
// first: [filter(full)+dispatch+git], [filter(full)+git,
// filter(compact)+git], [filter(compact) alone]. Each inner group holds the
// full-vs-compact filter-form sub-choices available at that segment-presence
// tier. A tier that would introduce a segment (dispatch or git) that isn't
// actually set is omitted entirely -- rather than reducing to a duplicate of
// the next tier's forms -- so the following tier's forms become the new
// highest priority instead (this is also what makes the no-filter-active
// case collapse structurally to the pre-#654 dispatch+git/git/dispatch tier
// list, the AC2 parity guard).
//
// View() tries FULL hints across every form in a group, in order, before
// ever trying truncated/ellipsis hints on ANY form in that same group --
// preserving the filter degradation ladder's preference for full hints over
// a merely-tighter filter rendering of the SAME segment set. Only once an
// entire group fails even with truncated hints does View() move on to the
// next (lower-priority) tier. That per-group-then-advance shape is what the
// #654 review fix targets: a higher-priority tier that fits only with
// truncated hints must still be chosen over a lower-priority tier that
// merely tolerates full hints -- grouping keeps that invariant from
// colliding with the full-vs-compact filter form choice, which needs the
// opposite preference (full hints beat a tighter form within the same
// tier).
func (s StatusBar) tailGroups() [][]string {
	hasDispatch := s.dispatchStatus != ""
	hasGit := s.gitStatus != ""

	var groups [][]string
	addGroup := func(forms ...string) {
		var g []string
		for _, f := range forms {
			if f == "" {
				continue
			}
			if len(g) > 0 && g[len(g)-1] == f {
				continue
			}
			g = append(g, f)
		}
		if len(g) > 0 {
			groups = append(groups, g)
		}
	}

	if hasDispatch {
		addGroup(joinNonEmpty(s.filterStatus, s.dispatchStatus, s.gitStatus))
	}
	addGroup(joinNonEmpty(s.filterStatus, s.gitStatus), joinNonEmpty(s.filterStatusCompact, s.gitStatus))
	if hasGit {
		addGroup(joinNonEmpty(s.filterStatusCompact))
	}
	return groups
}

// formatGitSegment formats a git Status into a compact, plain-ASCII segment,
// e.g. "main +2~1 ↑3↓0", colored: inserted lines in green, deleted lines in
// red (staged, unstaged, and untracked changes combined), ahead (push) and
// behind (pull) both in the same gentle orange since they're sync state
// rather than a warning. The ahead/behind portion is omitted entirely when
// HasUpstream is false.
//
// status.Branch comes from `git branch --show-current` (internal/git) and is
// NOT app-controlled: a ref name checked out from an untrusted fork/repo can
// contain arbitrary non-ASCII bytes, including UTF-8-encoded C1 controls or
// bidi overrides. It is sanitized with sanitizeSingleLine (#499) right here,
// at the point it's concatenated, so the composed segment can still safely
// carry the ANSI/SGR styling applied below (which SetGitStatus/View() must
// not strip).
func formatGitSegment(status gitdetect.Status) string {
	segment := sanitizeSingleLine(status.Branch) + " " +
		gitAddedStyle.Render("+"+strconv.Itoa(status.Insertions)) +
		gitDeletedStyle.Render("~"+strconv.Itoa(status.Deletions))
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
func formatDispatchSegment(state *cenciwatch.DispatchState) string {
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

// renderLevelMessage renders msg styled per level (or unstyled for
// StatusInfo), prefixed with prefix. Shared by View()'s timed-message and
// sticky-message branches, which render the exact same "prefix + leveled
// text" shape for their respective message fields.
func renderLevelMessage(prefix, msg string, level StatusLevel) string {
	if st := level.style(); st != nil {
		return prefix + st.Render(msg)
	}
	return prefix + msg
}

// agentPrefix builds the styled agent-status count prefix shown at the head of
// the status bar (e.g. "▶2 !1 ✓1"), rendering all six window statuses the
// cenci-watch daemon reports, in this order: running, need-input, done,
// failed, stopped, idle. Each status's symbol/style is reused from view.go's
// existing badge system (agentStatusSymbol/agentBadgeStyle) so the status bar
// never disagrees with the card badges or the agents modal; idle has no badge
// symbol (agentStatusSymbol("idle") == ""), so it reuses the agents modal's
// literal "·" placeholder, left unstyled to match. A status's count is
// omitted from the prefix entirely when that count is zero, independently per
// status; all six zero yields "".
func agentPrefix(running, needInput, done, failed, stopped, idle int) string {
	var tokens []string
	appendToken := func(status string, count int) {
		if count <= 0 {
			return
		}
		symbol := agentStatusSymbol(status)
		if symbol == "" {
			symbol = "·"
		}
		// agentBadgeStyle's default case (idle/unknown) is already a plain,
		// unstyled lipgloss.Style, so no separate unstyled branch is needed.
		tokens = append(tokens, agentBadgeStyle(status).Render(symbol+strconv.Itoa(count)))
	}
	appendToken(agentStatusRunning, running)
	appendToken(agentStatusNeedInput, needInput)
	appendToken("done", done)
	appendToken(agentStatusFailed, failed)
	appendToken("stopped", stopped)
	appendToken("idle", idle)
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
// The agent-status counts (running, needInput, done, failed, stopped, idle)
// and the repo-wide open-PR count render as an always-visible prefix ahead of
// both hints and timed messages; each token is omitted when its count is
// zero, and when all are zero the prefix and its separator are omitted
// entirely. Timed messages are still shown untruncated. counts is variadic
// (running, needInput, prCount, done, failed, stopped, idle) for caller
// convenience; missing values default to 0. The first three positions
// (running, needInput, prCount) are load-bearing for existing callers and
// must not be reordered; done/failed/stopped/idle were appended at the end
// (#420).
func (s StatusBar) View(width int, counts ...int) string {
	var running, needInput, prCount, done, failed, stopped, idle int
	if len(counts) > 0 {
		running = counts[0]
	}
	if len(counts) > 1 {
		needInput = counts[1]
	}
	if len(counts) > 2 {
		prCount = counts[2]
	}
	if len(counts) > 3 {
		done = counts[3]
	}
	if len(counts) > 4 {
		failed = counts[4]
	}
	if len(counts) > 5 {
		stopped = counts[5]
	}
	if len(counts) > 6 {
		idle = counts[6]
	}

	prefix := agentPrefix(running, needInput, done, failed, stopped, idle)
	// The repo-wide open-PR count trails the agent tokens in the same
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
		return renderLevelMessage(prefix, s.message, s.level)
	}

	if s.stickyMessage != "" {
		return renderLevelMessage(prefix, s.stickyMessage, s.stickyLevel)
	}

	// The prefix consumes width that is no longer available for hints.
	width -= lipgloss.Width(prefix)

	// Build the candidate tail segment tiers (filter + dispatch + git) in
	// priority order via tailGroups(). Within a group, fitTail is tried
	// against FULL (untruncated) hints for every member first, then against
	// truncated/ellipsis hints for every member, before moving on to the
	// next (lower-priority) group -- see tailGroups()'s doc comment for why
	// full-vs-truncated preference is scoped to within a group rather than
	// across the whole tier list (the #654 review fix).
	groups := s.tailGroups()
	fullHints := renderHints(s.hints, 1<<30)

	// fitTail tries tail against hintsFor(hintsWidth): the room left for
	// hints once tail and its 1-space separator are reserved. It returns the
	// composed line and true on the first fit, or ("", false) when hintsFor's
	// result doesn't fit the remaining width.
	fitTail := func(tail string, hintsFor func(hintsWidth int) string) (string, bool) {
		tailWidth := lipgloss.Width(tail)
		reserved := tailWidth + 1 // 1-space separator before the tail segment
		if reserved > width {
			return "", false
		}
		hintsWidth := width - reserved
		hintsView := hintsFor(hintsWidth)
		if lipgloss.Width(hintsView) > hintsWidth {
			return "", false
		}
		padding := width - lipgloss.Width(hintsView) - tailWidth
		return prefix + hintsView + strings.Repeat(" ", padding) + tail, true
	}

	for _, group := range groups {
		for _, tail := range group {
			if line, ok := fitTail(tail, func(int) string { return fullHints }); ok {
				return line
			}
		}
		for _, tail := range group {
			if line, ok := fitTail(tail, func(hintsWidth int) string { return renderHints(s.hints, hintsWidth) }); ok {
				return line
			}
		}
		// Not enough room for hints (even truncated) alongside any form in
		// this group; try the next (lower-priority) group.
	}

	return prefix + renderHints(s.hints, width)
}
