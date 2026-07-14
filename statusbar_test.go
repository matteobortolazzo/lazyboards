package main

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
)

// --- StatusBar: Basic Rendering ---

func TestStatusBar_ViewShowsHints(t *testing.T) {
	hints := []Hint{
		{Key: "n", Desc: "New"},
		{Key: "r", Desc: "Refresh"},
		{Key: "q", Desc: "Quit"},
	}
	sb := NewStatusBar(hints)
	view := sb.View(200, 0, 0)

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
	view := sb.View(200, 0, 0)
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
	sb.SetTimedMessage("Board refreshed", StatusSuccess, 3*time.Second)
	view := sb.View(200, 0, 0)

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
	sb.SetTimedMessage("Temporary message", StatusInfo, 3*time.Second)
	sb.ClearMessage()
	view := sb.View(200, 0, 0)

	if strings.Contains(view, "Temporary message") {
		t.Errorf("View() = %q, should NOT contain message after ClearMessage()", view)
	}
	if !strings.Contains(view, "New") {
		t.Errorf("View() = %q, want hints restored after ClearMessage()", view)
	}
}

func TestStatusBar_SetTimedMessage_ReturnsCmd(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	cmd := sb.SetTimedMessage("Done!", StatusSuccess, 3*time.Second)
	if cmd == nil {
		t.Error("SetTimedMessage() should return a non-nil tea.Cmd")
	}
}

// --- StatusBar: Timed Message Levels ---

func TestStatusBar_TimedMessage_ErrorLevel_RendersRed(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	sb.SetTimedMessage("Refresh failed", StatusError, 3*time.Second)
	view := sb.View(200, 0, 0)

	// The message text must appear in the output.
	if !strings.Contains(view, "Refresh failed") {
		t.Errorf("View() = %q, want it to contain %q", view, "Refresh failed")
	}
	// Error-level messages should be styled (contain ANSI escape sequences),
	// so the rendered output must differ from the raw message text.
	if view == "Refresh failed" {
		t.Errorf("View() = %q, want error-level message to be styled (not raw text)", view)
	}
}

func TestStatusBar_TimedMessage_WarningLevel_RendersYellow(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	sb.SetTimedMessage("No linked PRs", StatusWarning, 3*time.Second)
	view := sb.View(200, 0, 0)

	// The message text must appear in the output.
	if !strings.Contains(view, "No linked PRs") {
		t.Errorf("View() = %q, want it to contain %q", view, "No linked PRs")
	}
	// Warning-level messages should be styled (contain ANSI escape sequences),
	// so the rendered output must differ from the raw message text.
	if view == "No linked PRs" {
		t.Errorf("View() = %q, want warning-level message to be styled (not raw text)", view)
	}
}

func TestStatusBar_TimedMessage_SuccessLevel_RendersGreen(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	sb.SetTimedMessage("Board refreshed", StatusSuccess, 3*time.Second)
	view := sb.View(200, 0, 0)

	// The message text must appear in the output.
	if !strings.Contains(view, "Board refreshed") {
		t.Errorf("View() = %q, want it to contain %q", view, "Board refreshed")
	}
	// Success-level messages should be styled (contain ANSI escape sequences),
	// so the rendered output must differ from the raw message text.
	if view == "Board refreshed" {
		t.Errorf("View() = %q, want success-level message to be styled (not raw text)", view)
	}
}

func TestStatusBar_TimedMessage_InfoLevel_RendersUnstyled(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	sb.SetTimedMessage("Running...", StatusInfo, 3*time.Second)
	view := sb.View(200, 0, 0)

	// Info-level messages should be returned as raw text without styling.
	if view != "Running..." {
		t.Errorf("View() = %q, want info-level message to be unstyled raw text %q", view, "Running...")
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
	view := sb.View(200, 0, 0)

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

	view := sb.View(width, 0, 0)

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
	view := sb.View(200, 0, 0)

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
	sb.SetTimedMessage("Board refreshed", StatusSuccess, 3*time.Second)

	// Call with a tiny width -- the timed message should still appear in full.
	view := sb.View(1, 0, 0)

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

	view := sb.View(width, 0, 0)
	viewWidth := lipgloss.Width(view)

	if viewWidth > width {
		t.Errorf("View(%d) rendered width = %d, exceeds allowed width; output = %q", width, viewWidth, view)
	}
}

func TestStatusBar_ViewEmptyHintsWithWidth(t *testing.T) {
	sb := NewStatusBar(nil)
	view := sb.View(50, 0, 0)

	if strings.TrimSpace(view) != "" {
		t.Errorf("View(50) with no hints = %q, want empty or whitespace-only string", view)
	}
}

// --- StatusBar: Agent-status count prefix (#259) ---

func TestStatusBar_ViewAgentPrefix_RunningCountShown(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	view := sb.View(200, 2, 0)

	if !strings.Contains(view, "▶2") {
		t.Errorf("View() = %q, want running token %q", view, "▶2")
	}
	if strings.Contains(view, "!") {
		t.Errorf("View() = %q, should NOT contain need_input symbol when needInput=0", view)
	}
}

func TestStatusBar_ViewAgentPrefix_NeedInputCountShown(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	view := sb.View(200, 0, 3)

	if !strings.Contains(view, "!3") {
		t.Errorf("View() = %q, want need_input token %q", view, "!3")
	}
	if strings.Contains(view, "▶") {
		t.Errorf("View() = %q, should NOT contain running symbol when running=0", view)
	}
}

func TestStatusBar_ViewAgentPrefix_BothCountsAndSeparator(t *testing.T) {
	// A single hint has no internal separator, so any "|" present must come
	// from the prefix's trailing separator.
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	view := sb.View(200, 2, 1)

	if !strings.Contains(view, "▶2") {
		t.Errorf("View() = %q, want running token %q", view, "▶2")
	}
	if !strings.Contains(view, "!1") {
		t.Errorf("View() = %q, want need_input token %q", view, "!1")
	}
	if !strings.Contains(view, "|") {
		t.Errorf("View() = %q, want a separator between the prefix and hints", view)
	}
	// The running token must precede the need_input token, which must precede
	// the hint key.
	if strings.Index(view, "▶2") > strings.Index(view, "!1") {
		t.Errorf("View() = %q, want running token before need_input token", view)
	}
	if strings.Index(view, "!1") > strings.Index(view, "Quit") {
		t.Errorf("View() = %q, want prefix before the hints", view)
	}
}

func TestStatusBar_ViewAgentPrefix_BothZeroOmitsPrefixAndSeparator(t *testing.T) {
	// A single hint has no internal separator, so with zero counts there must
	// be no "|" at all (the prefix and its separator are omitted).
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	view := sb.View(200, 0, 0)

	if strings.Contains(view, "▶") || strings.Contains(view, "!") {
		t.Errorf("View() = %q, want no agent symbols when both counts are zero", view)
	}
	if strings.Contains(view, "|") {
		t.Errorf("View() = %q, want no prefix separator when both counts are zero", view)
	}
}

func TestStatusBar_ViewAgentPrefix_ShownWithTimedMessage(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	sb.SetTimedMessage("Opened PR #10", StatusSuccess, 3*time.Second)
	view := sb.View(200, 2, 1)

	// The prefix stays visible even while a timed message is active.
	if !strings.Contains(view, "▶2") || !strings.Contains(view, "!1") {
		t.Errorf("View() = %q, want agent counts visible alongside a timed message", view)
	}
	if !strings.Contains(view, "Opened PR #10") {
		t.Errorf("View() = %q, want the timed message to remain visible", view)
	}
	// The prefix precedes the message.
	if strings.Index(view, "▶2") > strings.Index(view, "Opened PR #10") {
		t.Errorf("View() = %q, want the prefix before the timed message", view)
	}
}

func TestStatusBar_ViewAgentPrefix_ReducesHintWidth(t *testing.T) {
	hints := []Hint{
		{Key: "a", Desc: "Alpha"},
		{Key: "b", Desc: "Bravo"},
		{Key: "c", Desc: "Charlie"},
	}
	sb := NewStatusBar(hints)

	// Width that fits every hint exactly when there is no prefix.
	var rendered []string
	for _, h := range hints {
		rendered = append(rendered, hintKeyStyle.Render(h.Key)+hintDescStyle.Render(": "+h.Desc))
	}
	separator := hintDescStyle.Render(" | ")
	fullWidth := lipgloss.Width(strings.Join(rendered, separator))

	// Without a prefix all hints fit: no truncation.
	noPrefix := sb.View(fullWidth, 0, 0)
	if strings.Contains(noPrefix, "...") {
		t.Fatalf("View(%d, 0, 0) = %q, want all hints to fit (test setup)", fullWidth, noPrefix)
	}
	if !strings.Contains(noPrefix, "Charlie") {
		t.Fatalf("View(%d, 0, 0) = %q, want last hint present (test setup)", fullWidth, noPrefix)
	}

	// With a prefix and the SAME width, the prefix consumes hint space, so the
	// last hint must be truncated away.
	withPrefix := sb.View(fullWidth, 5, 0)
	if lipgloss.Width(withPrefix) > fullWidth {
		t.Errorf("View(%d, 5, 0) rendered width = %d, exceeds allowed width; output = %q",
			fullWidth, lipgloss.Width(withPrefix), withPrefix)
	}
	if strings.Contains(withPrefix, "Charlie") {
		t.Errorf("View(%d, 5, 0) = %q, want the last hint truncated once the prefix consumes width", fullWidth, withPrefix)
	}
	if !strings.Contains(withPrefix, "...") {
		t.Errorf("View(%d, 5, 0) = %q, want a truncation indicator", fullWidth, withPrefix)
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

	view := sb.View(width, 0, 0)

	// With such a narrow width, " ..." should appear as a truncation indicator.
	if !strings.Contains(view, "...") {
		t.Errorf("View(%d) = %q, want it to contain truncation indicator %q when no hints fit", width, view, "...")
	}
}

// --- StatusBar: Git Status Segment ---

func TestStatusBar_SetGitStatus_AppearsInView(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	sb.SetGitStatus("main +2~1 ↑3↓0")
	view := sb.View(200)

	if !strings.Contains(view, "main +2~1") {
		t.Errorf("View(200) = %q, want it to contain the git segment %q", view, "main +2~1")
	}
	if !strings.Contains(view, "Quit") {
		t.Errorf("View(200) = %q, want hints to still be present alongside the git segment", view)
	}
}

func TestStatusBar_ViewGitSegment_RightAligned(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	gitSegment := "main +0~0"
	sb.SetGitStatus(gitSegment)
	view := sb.View(200)

	hintIdx := strings.Index(view, "Quit")
	gitIdx := strings.Index(view, gitSegment)

	if hintIdx == -1 {
		t.Fatalf("View(200) = %q, want it to contain hint desc %q", view, "Quit")
	}
	if gitIdx == -1 {
		t.Fatalf("View(200) = %q, want it to contain git segment %q", view, gitSegment)
	}
	if gitIdx <= hintIdx {
		t.Errorf("View(200) git segment at index %d, hints at index %d; want git segment right-aligned (positioned after hints)", gitIdx, hintIdx)
	}
}

func TestStatusBar_ViewGitSegment_TruncatesHintsToMakeRoom(t *testing.T) {
	hints := []Hint{
		{Key: "o", Desc: "Open"},
		{Key: "n", Desc: "New"},
		{Key: "r", Desc: "Refresh"},
	}
	sb := NewStatusBar(hints)

	// Compute a constrained width where all 3 hints fit fully and where the
	// git segment can replace them with the minimum truncation indicator.
	hint0 := hintKeyStyle.Render(hints[0].Key) + hintDescStyle.Render(": "+hints[0].Desc)
	hint1 := hintKeyStyle.Render(hints[1].Key) + hintDescStyle.Render(": "+hints[1].Desc)
	hint2 := hintKeyStyle.Render(hints[2].Key) + hintDescStyle.Render(": "+hints[2].Desc)
	separator := hintDescStyle.Render(" | ")
	fullHintsWidth := lipgloss.Width(hint0) + lipgloss.Width(separator) + lipgloss.Width(hint1) + lipgloss.Width(separator) + lipgloss.Width(hint2)
	gitSegment := "feature-branch +5~3 ↑10↓2"
	minGitWidth := lipgloss.Width(gitSegment) + 1 + lipgloss.Width(hintDescStyle.Render(" ..."))
	width := max(fullHintsWidth, minGitWidth)

	// Sanity check: at this width, with no git status set, all hints fit.
	baseline := sb.View(width)
	if !strings.Contains(baseline, hints[2].Desc) {
		t.Fatalf("baseline View(%d) = %q, want all hints (including %q) to fit without a git segment", width, baseline, hints[2].Desc)
	}

	// Now set a git status that needs extra room; at the same width, hints
	// must be truncated to make room for the git segment.
	sb.SetGitStatus(gitSegment)
	view := sb.View(width)

	if !strings.Contains(view, gitSegment) {
		t.Errorf("View(%d) = %q, want it to still contain the git segment %q", width, view, gitSegment)
	}
	if strings.Contains(view, hints[2].Desc) {
		t.Errorf("View(%d) = %q, want the last hint %q to be truncated to make room for the git segment", width, view, hints[2].Desc)
	}
	if !strings.Contains(view, "...") {
		t.Errorf("View(%d) = %q, want a truncation indicator once a hint is dropped to make room", width, view)
	}
}

func TestStatusBar_ViewGitSegment_DropsWhenWidthInsufficient(t *testing.T) {
	hints := []Hint{
		{Key: "o", Desc: "Open"},
		{Key: "n", Desc: "New"},
	}
	sb := NewStatusBar(hints)

	hint0 := hintKeyStyle.Render(hints[0].Key) + hintDescStyle.Render(": "+hints[0].Desc)
	hint1 := hintKeyStyle.Render(hints[1].Key) + hintDescStyle.Render(": "+hints[1].Desc)
	separator := hintDescStyle.Render(" | ")
	fullHintsWidth := lipgloss.Width(hint0) + lipgloss.Width(separator) + lipgloss.Width(hint1)

	gitSegment := "feature-branch +5~3 ↑10↓2"
	sb.SetGitStatus(gitSegment)

	// Width has exactly enough room for the hints and nothing else -- no
	// room for even a single extra column for the git segment.
	view := sb.View(fullHintsWidth)

	if strings.Contains(view, gitSegment) {
		t.Errorf("View(%d) = %q, want the git segment dropped entirely when there is no room for it", fullHintsWidth, view)
	}
	if !strings.Contains(view, hints[0].Desc) || !strings.Contains(view, hints[1].Desc) {
		t.Errorf("View(%d) = %q, want hints to keep priority and render fully when the git segment is dropped", fullHintsWidth, view)
	}
	if strings.Contains(view, "...") {
		t.Errorf("View(%d) = %q, hints fit fully on their own; want no truncation indicator just because the git segment was dropped", fullHintsWidth, view)
	}
}

func TestStatusBar_ViewGitSegment_DropsWhenOnlySegmentFits(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	gitSegment := "feature-branch +5~3 ↑10↓2"
	sb.SetGitStatus(gitSegment)

	// Leave room for the git segment and its separator, but not for the
	// minimum hint rendering (the truncation indicator). Hints keep priority,
	// so the segment must be dropped instead of overflowing the status bar.
	width := lipgloss.Width(gitSegment) + 1
	view := sb.View(width)

	if strings.Contains(view, gitSegment) {
		t.Errorf("View(%d) = %q, want the git segment dropped when only the segment fits", width, view)
	}
	if got := lipgloss.Width(view); got > width {
		t.Errorf("lipgloss.Width(View(%d)) = %d, want at most %d; view = %q", width, got, width, view)
	}
}

func TestStatusBar_ViewGitSegment_TimedMessageOverridesGitSegment(t *testing.T) {
	sb := NewStatusBar([]Hint{{Key: "q", Desc: "Quit"}})
	sb.SetGitStatus("main +2~1 ↑3↓0")
	sb.SetTimedMessage("Board refreshed", StatusSuccess, 3*time.Second)
	view := sb.View(200)

	if !strings.Contains(view, "Board refreshed") {
		t.Errorf("View(200) = %q, want it to contain the timed message %q", view, "Board refreshed")
	}
	if strings.Contains(view, "main +2~1") {
		t.Errorf("View(200) = %q, should NOT contain the git segment while a timed message is active", view)
	}
}

// --- formatGitSegment ---

func TestFormatGitSegment_WithUpstream_IncludesBranchCountsAndAheadBehind(t *testing.T) {
	status := gitdetect.Status{
		Branch:      "main",
		Staged:      2,
		Unstaged:    1,
		Ahead:       3,
		Behind:      0,
		HasUpstream: true,
	}

	segment := formatGitSegment(status)

	if !strings.Contains(segment, "main") {
		t.Errorf("formatGitSegment(%+v) = %q, want it to contain branch name %q", status, segment, "main")
	}
	if !strings.Contains(segment, "2") || !strings.Contains(segment, "1") {
		t.Errorf("formatGitSegment(%+v) = %q, want it to contain staged count %d and unstaged count %d", status, segment, status.Staged, status.Unstaged)
	}
	if !strings.Contains(segment, "3") || !strings.Contains(segment, "0") {
		t.Errorf("formatGitSegment(%+v) = %q, want it to contain ahead count %d and behind count %d", status, segment, status.Ahead, status.Behind)
	}
	if !strings.ContainsRune(segment, '↑') || !strings.ContainsRune(segment, '↓') {
		t.Errorf("formatGitSegment(%+v) = %q, want ahead/behind arrows when HasUpstream is true", status, segment)
	}
}

func TestFormatGitSegment_NoUpstream_OmitsAheadBehind(t *testing.T) {
	status := gitdetect.Status{
		Branch:      "main",
		Staged:      0,
		Unstaged:    0,
		HasUpstream: false,
	}

	segment := formatGitSegment(status)

	if !strings.Contains(segment, "main") {
		t.Errorf("formatGitSegment(%+v) = %q, want it to contain branch name %q", status, segment, "main")
	}
	if strings.ContainsRune(segment, '↑') || strings.ContainsRune(segment, '↓') {
		t.Errorf("formatGitSegment(%+v) = %q, should NOT contain ahead/behind arrows when HasUpstream is false", status, segment)
	}
}

// TestFormatGitSegment_LazygitStyleColors verifies staged/unstaged/ahead/behind
// counts are each individually colored (lazygit-style: additions green,
// deletions red, unpushed commits orange, unpulled commits yellow) rather than
// rendered as one plain-text segment.
func TestFormatGitSegment_LazygitStyleColors(t *testing.T) {
	status := gitdetect.Status{
		Branch:      "main",
		Staged:      2,
		Unstaged:    1,
		Ahead:       3,
		Behind:      4,
		HasUpstream: true,
	}

	segment := formatGitSegment(status)

	if !strings.Contains(segment, gitAddedStyle.Render("+2")) {
		t.Errorf("formatGitSegment(%+v) = %q, want staged count colored via gitAddedStyle", status, segment)
	}
	if !strings.Contains(segment, gitDeletedStyle.Render("~1")) {
		t.Errorf("formatGitSegment(%+v) = %q, want unstaged count colored via gitDeletedStyle", status, segment)
	}
	if !strings.Contains(segment, gitAheadStyle.Render("↑3")) {
		t.Errorf("formatGitSegment(%+v) = %q, want ahead count colored via gitAheadStyle", status, segment)
	}
	if !strings.Contains(segment, gitBehindStyle.Render("↓4")) {
		t.Errorf("formatGitSegment(%+v) = %q, want behind count colored via gitBehindStyle", status, segment)
	}
}

func TestFormatGitSegment_NoNerdFontGlyphs(t *testing.T) {
	// Q1 answer: plain ASCII, no nerd-font icons. The only permitted non-ASCII
	// glyphs are the plain Unicode arrows used for ahead/behind.
	status := gitdetect.Status{
		Branch:      "main",
		Staged:      1,
		Unstaged:    1,
		Ahead:       1,
		Behind:      1,
		HasUpstream: true,
	}

	segment := formatGitSegment(status)

	for _, r := range segment {
		if r > 127 && r != '↑' && r != '↓' {
			t.Errorf("formatGitSegment(%+v) = %q, contains unexpected non-ASCII glyph %q; want plain ASCII plus only ↑/↓ arrows", status, segment, r)
		}
	}
}
