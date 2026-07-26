package main

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// --- renderProgressBar fill math (#486) ---
//
// renderProgressBar(percentage, width, muted) is a small pure function used
// by the upcoming Milestones view to render a `▓▓▓░░` style progress bar.
// fill = round(percentage/100*width), clamped so percentage outside [0,100]
// and width <= 0 cannot produce an out-of-range or negative fill count.

// TestRenderProgressBar_FillMath_DefaultProfile asserts the filled/unfilled
// cell split at a colorless color profile (the default in non-TTY test
// environments), using lipgloss.Width -- not len/rune count, per
// docs/terminal-rendering.md -- to measure rendered cell width since the
// glyphs (▓ ░) are already single-width but the rule is the mandated
// measurement approach for this codebase's terminal layout math.
func TestRenderProgressBar_FillMath_DefaultProfile(t *testing.T) {
	tests := []struct {
		name       string
		percentage float64
		width      int
		wantFilled int
	}{
		{"0 percent", 0, 10, 0},
		{"50 percent", 50, 10, 5},
		{"100 percent", 100, 10, 10},
		{"33.3 percent rounds down", 33.3, 10, 3},
		{"35 percent rounds half up", 35, 10, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bar := renderProgressBar(tt.percentage, tt.width, false)

			if got := lipgloss.Width(bar); got != tt.width {
				t.Errorf("renderProgressBar(%v, %d, false): lipgloss.Width = %d, want %d (total cell count must equal width)",
					tt.percentage, tt.width, got, tt.width)
			}

			wantFilledGlyphs := 0
			wantUnfilledGlyphs := 0
			for _, r := range bar {
				switch r {
				case '▓':
					wantFilledGlyphs++
				case '░':
					wantUnfilledGlyphs++
				}
			}
			if wantFilledGlyphs != tt.wantFilled {
				t.Errorf("renderProgressBar(%v, %d, false): filled glyph count = %d, want %d",
					tt.percentage, tt.width, wantFilledGlyphs, tt.wantFilled)
			}
			wantUnfilled := tt.width - tt.wantFilled
			if wantUnfilledGlyphs != wantUnfilled {
				t.Errorf("renderProgressBar(%v, %d, false): unfilled glyph count = %d, want %d",
					tt.percentage, tt.width, wantUnfilledGlyphs, wantUnfilled)
			}
		})
	}
}

// TestRenderProgressBar_ClampsBelowZeroPercentage asserts a negative
// percentage clamps to fully unfilled instead of producing a negative fill
// count.
func TestRenderProgressBar_ClampsBelowZeroPercentage(t *testing.T) {
	bar := renderProgressBar(-50, 10, false)

	if got := lipgloss.Width(bar); got != 10 {
		t.Fatalf("renderProgressBar(-50, 10, false): lipgloss.Width = %d, want 10", got)
	}
	filled := 0
	for _, r := range bar {
		if r == '▓' {
			filled++
		}
	}
	if filled != 0 {
		t.Errorf("renderProgressBar(-50, 10, false): filled glyph count = %d, want 0 (fully unfilled)", filled)
	}
}

// TestRenderProgressBar_ClampsAbove100Percentage asserts a percentage above
// 100 clamps to fully filled instead of overflowing past width.
func TestRenderProgressBar_ClampsAbove100Percentage(t *testing.T) {
	bar := renderProgressBar(150, 10, false)

	if got := lipgloss.Width(bar); got != 10 {
		t.Fatalf("renderProgressBar(150, 10, false): lipgloss.Width = %d, want 10", got)
	}
	filled := 0
	for _, r := range bar {
		if r == '▓' {
			filled++
		}
	}
	if filled != 10 {
		t.Errorf("renderProgressBar(150, 10, false): filled glyph count = %d, want 10 (fully filled)", filled)
	}
}

// TestRenderProgressBar_ZeroWidth_ReturnsEmptyNoPanic asserts width == 0
// returns an empty string without panicking.
func TestRenderProgressBar_ZeroWidth_ReturnsEmptyNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("renderProgressBar(50, 0, false) panicked: %v", r)
		}
	}()
	if got := renderProgressBar(50, 0, false); got != "" {
		t.Errorf("renderProgressBar(50, 0, false) = %q, want empty string", got)
	}
}

// TestRenderProgressBar_NegativeWidth_ReturnsEmptyNoPanic asserts a negative
// width returns an empty string without panicking.
func TestRenderProgressBar_NegativeWidth_ReturnsEmptyNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("renderProgressBar(50, -5, false) panicked: %v", r)
		}
	}()
	if got := renderProgressBar(50, -5, false); got != "" {
		t.Errorf("renderProgressBar(50, -5, false) = %q, want empty string", got)
	}
}

// --- renderProgressBar color selection (#486) ---
//
// Color is chosen inside renderProgressBar from the muted argument: success
// green (114) at 100% when not muted, existing muted gray (245) otherwise.
// This must be applied at construction time, not via an outer wrap -- see
// docs/terminal-rendering.md's note on pre-colored glyphs. Expected colors
// are restated here from the spec (not imported from a not-yet-written
// production style constant), mirroring wantSubIssueStyle in view_test.go.

// TestRenderProgressBar_Color_Complete_NotMuted_IsGreen asserts a 100%,
// non-muted bar renders in the success green (114) foreground.
func TestRenderProgressBar_Color_Complete_NotMuted_IsGreen(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	got := renderProgressBar(100, 10, false)

	plainBar := "▓▓▓▓▓▓▓▓▓▓"
	want := lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render(plainBar)
	if got != want {
		t.Errorf("renderProgressBar(100, 10, false) = %q, want %q (success green 114)", got, want)
	}
}

// TestRenderProgressBar_Color_Complete_Muted_IsGray asserts a 100%, muted
// bar renders in the existing muted gray (245), not green.
func TestRenderProgressBar_Color_Complete_Muted_IsGray(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	got := renderProgressBar(100, 10, true)

	plainBar := "▓▓▓▓▓▓▓▓▓▓"
	want := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(plainBar)
	if got != want {
		t.Errorf("renderProgressBar(100, 10, true) = %q, want %q (muted gray 245)", got, want)
	}
}

// TestRenderProgressBar_Color_Partial_NotMuted_IsGray asserts a <100%,
// non-muted bar (50%) still renders in the muted gray (245) -- only a
// complete, non-muted bar gets the green treatment.
func TestRenderProgressBar_Color_Partial_NotMuted_IsGray(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	got := renderProgressBar(50, 10, false)

	plainBar := "▓▓▓▓▓░░░░░"
	want := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(plainBar)
	if got != want {
		t.Errorf("renderProgressBar(50, 10, false) = %q, want %q (muted gray 245)", got, want)
	}
}

// TestRenderProgressBar_Color_CompleteMuted_DiffersFromCompleteNotMuted
// asserts the two 100% renders (muted vs. not) are behaviorally distinct --
// this catches an implementation that ignores the muted argument entirely.
func TestRenderProgressBar_Color_CompleteMuted_DiffersFromCompleteNotMuted(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	notMuted := renderProgressBar(100, 10, false)
	muted := renderProgressBar(100, 10, true)

	if notMuted == muted {
		t.Error("renderProgressBar(100, 10, false) and renderProgressBar(100, 10, true) render identically, want distinct (green vs. gray)")
	}
}

// TestRenderProgressBar_Color_DistinctFromPRPurple asserts neither the
// complete-green nor the muted-gray render matches the PR-purple (183) hue
// used elsewhere in the app, so a progress bar can never be misread as a PR
// indicator.
func TestRenderProgressBar_Color_DistinctFromPRPurple(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	complete := renderProgressBar(100, 10, false)
	partial := renderProgressBar(50, 10, false)

	plainBar := "▓▓▓▓▓▓▓▓▓▓"
	purple := lipgloss.NewStyle().Foreground(lipgloss.Color("183")).Render(plainBar)
	if complete == purple {
		t.Error("renderProgressBar(100, 10, false) matches PR-purple (183) styling, want a distinct color")
	}
	if partial == purple {
		t.Error("renderProgressBar(50, 10, false) matches PR-purple (183) styling, want a distinct color")
	}
}
