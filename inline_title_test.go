package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// inline_title_test.go centralizes all #597 coverage: bounding the untrusted
// card/label/PR title inlined into five one-line prompt sites (close-confirm
// help bar, label-confirm help bar, both delete-mode prompts, and the
// PR-picker title) so a long/malicious title can no longer shift the
// in-layout help bar, grow the delete modal vertically, or -- via escaping
// applied AFTER truncation -- blow past the intended cell budget.
//
// It references the not-yet-defined production helpers/constants
// (inlineTitleMinCells, inlineTitleBudget, escapeInline, fitQuotedTitle,
// closeConfirmPromptFmt, labelConfirmPromptFmt, deleteConfirmPromptFmt,
// deleteCommentPromptFmt, prPickerTitleFmt) that Phase 4 implements in
// view.go. Until then this file intentionally fails to compile.

// --- Unit tests: inlineTitleBudget ---

func TestInlineTitleBudget_NormalCase_SubtractsChromeFromAvailable(t *testing.T) {
	got := inlineTitleBudget(118, 20)
	if want := 98; got != want {
		t.Errorf("inlineTitleBudget(118, 20) = %d, want %d", got, want)
	}
}

func TestInlineTitleBudget_FloorClamped_WhenSubtractionGoesNegative(t *testing.T) {
	got := inlineTitleBudget(30, 50)
	if got != inlineTitleMinCells {
		t.Errorf("inlineTitleBudget(30, 50) = %d, want floor %d (available - chrome is negative)", got, inlineTitleMinCells)
	}
}

func TestInlineTitleBudget_FloorClamped_WhenPositiveButBelowFloor(t *testing.T) {
	got := inlineTitleBudget(15, 10)
	if got != inlineTitleMinCells {
		t.Errorf("inlineTitleBudget(15, 10) = %d, want floor %d (5 is below the floor)", got, inlineTitleMinCells)
	}
}

// TestInlineTitleBudget_FloorAppliedAtNarrowDeleteConfirmWidth is the
// explicit ticket example: at b.Width = 60, the delete-confirm-step modal's
// content width (32) minus the deleteConfirmPromptFmt chrome (~35) goes
// negative, so inlineTitleBudget must clamp to the floor. This test asserts
// only the arithmetic -- NOT that the rendered line fits within the modal at
// this width (it legitimately still wraps at very narrow widths, an
// accepted trade-off per the plan).
func TestInlineTitleBudget_FloorAppliedAtNarrowDeleteConfirmWidth(t *testing.T) {
	b := Board{Width: 60}
	available := modalContentWidth(b.createModalWidth())
	card := Card{Number: 1}
	chromeWidth := lipgloss.Width(fmt.Sprintf(deleteConfirmPromptFmt, card.Number, card.Number, ""))

	got := inlineTitleBudget(available, chromeWidth)
	if got != inlineTitleMinCells {
		t.Errorf("inlineTitleBudget(%d, %d) = %d, want floor %d (available - chrome goes negative at b.Width=60)", available, chromeWidth, got, inlineTitleMinCells)
	}
}

// --- Unit tests: escapeInline ---

func TestEscapeInline(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"double quote", `"`, `\"`},
		{"backslash", `\`, `\\`},
		{"non-printable private-use rune", string(rune(0xE000)), "\\ue000"},
		{"plain printable ASCII", "hello", "hello"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeInline(tt.input)
			if got != tt.want {
				t.Errorf("escapeInline(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- Unit tests: fitQuotedTitle (escape-before-truncate ordering) ---

// TestFitQuotedTitle_DoubleQuoteHeavyTitle_StaysWithinBudget is a regression
// test for the ordering bug that motivated this ticket: escaping a title
// AFTER truncating it can double its cell width (each `"` becomes `\"`),
// blowing past the intended budget. Escaping first, then truncating, must
// keep the result within budget regardless of how many quote characters the
// untrusted title contains.
func TestFitQuotedTitle_DoubleQuoteHeavyTitle_StaysWithinBudget(t *testing.T) {
	title := strings.Repeat(`"`, 200)
	available, chromeWidth := 100, 10
	budget := inlineTitleBudget(available, chromeWidth)

	got := fitQuotedTitle(title, available, chromeWidth)
	if w := lipgloss.Width(got); w > budget {
		t.Errorf("fitQuotedTitle(200 double-quotes, %d, %d) width = %d cells, want <= budget %d", available, chromeWidth, w, budget)
	}
}

// TestFitQuotedTitle_NonPrintableRuneHeavyTitle_StaysWithinBudget mirrors the
// double-quote case for a title made entirely of non-printable runes, each
// of which escapeInline expands to a 6-character \uXXXX sequence.
func TestFitQuotedTitle_NonPrintableRuneHeavyTitle_StaysWithinBudget(t *testing.T) {
	title := strings.Repeat(string(rune(0xE000)), 100)
	available, chromeWidth := 60, 10
	budget := inlineTitleBudget(available, chromeWidth)

	got := fitQuotedTitle(title, available, chromeWidth)
	if w := lipgloss.Width(got); w > budget {
		t.Errorf("fitQuotedTitle(100 non-printable runes, %d, %d) width = %d cells, want <= budget %d", available, chromeWidth, w, budget)
	}
}

// TestFitQuotedTitle_BackslashHeavyTitle_TruncationEndsInEllipsisNotRawCut
// covers explicit risk coverage per .claude/rules/testing.md: escapeInline
// turns each raw `\` into the two-character pair `\\`, so a cell-width cut
// (truncateCell/ansi.Truncate has no notion of escape-sequence boundaries)
// could in principle land between the two characters of a pair. What
// actually prevents a dangling `\` from ever sitting immediately before the
// prompt's closing quote is that truncateCell always appends the "…" marker
// in place of whatever character the raw cut produced -- so this test
// asserts that guarantee directly (a title long enough to force truncation
// must end in "…"), rather than merely re-deriving it from the fact that no
// backslash happens to be last (which follows automatically and would pass
// even if truncateCell had no ellipsis-substitution behavior at all).
func TestFitQuotedTitle_BackslashHeavyTitle_TruncationEndsInEllipsisNotRawCut(t *testing.T) {
	title := strings.Repeat(`\`, 300)
	available, chromeWidth := 50, 10
	budget := inlineTitleBudget(available, chromeWidth)

	got := fitQuotedTitle(title, available, chromeWidth)
	if w := lipgloss.Width(got); w > budget {
		t.Errorf("fitQuotedTitle(300 backslashes, %d, %d) width = %d cells, want <= budget %d", available, chromeWidth, w, budget)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("fitQuotedTitle(300 backslashes, ...) = %q, want it to end in the truncation ellipsis marker (proves the cut never lands mid escape-pair and leaves a raw `\\` abutting the prompt's closing quote)", got)
	}
}

// --- Integration: five inline-title prompt render sites ---

// findLineContaining lives in helpers_test.go -- both this file and
// keymap_text_test.go locate prompt lines by a title-independent marker.

// rawPhysicalLineCount counts every physical line of view. Used for the
// close-confirm/label-confirm help bar, which (unlike a modal) is not
// wrapped in a further lipgloss.Place call -- its own physical line count
// grows if the inlined title causes the help bar to word-wrap.
func rawPhysicalLineCount(view string) int {
	return len(strings.Split(view, "\n"))
}

// borderedLineCount counts physical lines containing the rounded-border
// vertical marker "│". Modal views (delete modal, PR picker) are always
// centered via lipgloss.Place to the full terminal height, so their raw
// physical line count is constant regardless of content -- the modal box's
// own height (and therefore whether its content wrapped) is only visible in
// how many bordered lines it renders.
func borderedLineCount(view string) int {
	count := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "│") {
			count++
		}
	}
	return count
}

// setupCloseConfirmSite builds a board in closeConfirmMode with the selected
// card's title set to title.
func setupCloseConfirmSite(t *testing.T, title string) Board {
	t.Helper()
	b := newLoadedTestBoard(t)
	b.Width, b.Height = 120, 40
	b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].Title = title

	m, _ := b.Update(keyMsg("x"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if updated.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", updated.mode)
	}
	return updated
}

// setupLabelConfirmSite builds a board in labelConfirmMode with the current
// unknown label set to label.
func setupLabelConfirmSite(t *testing.T, label string) Board {
	t.Helper()
	b := newLoadedTestBoard(t)
	b.Width, b.Height = 120, 40
	b.mode = labelConfirmMode
	b.labelConfirm = labelConfirmState{unknownLabels: []string{label}, currentIdx: 0}
	return b
}

// setupDeleteCommentStepSite builds a board in deleteMode at the comment
// step, with the selected card's title set to title.
func setupDeleteCommentStepSite(t *testing.T, title string) Board {
	t.Helper()
	b, _ := newDeleteTestBoard(t)
	b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor].Title = title

	m, _ := b.Update(keyMsg("d"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if updated.delete.step != deleteStepComment {
		t.Fatalf("precondition: delete.step = %d, want deleteStepComment", updated.delete.step)
	}
	return updated
}

// setupDeleteConfirmStepSite builds a board in deleteMode at the
// retype-to-confirm step, with the selected card's title set to title.
func setupDeleteConfirmStepSite(t *testing.T, title string) Board {
	t.Helper()
	b := setupDeleteCommentStepSite(t, title)

	m, _ := b.Update(arrowMsg(tea.KeyEnter))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if updated.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: delete.step = %d, want deleteStepConfirm", updated.delete.step)
	}
	return updated
}

// setupPRPickerSite builds a board in prPickerMode (card "Two PRs", 2 linked
// PRs) with the first linked PR's title set to title.
func setupPRPickerSite(t *testing.T, title string) Board {
	t.Helper()
	b := newBoardWithPRs(t)
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))
	if b.mode != prPickerMode {
		t.Fatalf("precondition: mode = %d, want prPickerMode", b.mode)
	}

	col := b.Columns[b.ActiveTab]
	col.Cards[col.Cursor].LinkedPRs[0].Title = title
	return b
}

// promptSite describes one of the five inline-title prompt render sites for
// the table-driven test below.
type promptSite struct {
	name string
	// setup builds a board rendering the prompt with the given (possibly
	// very long) title inlined.
	setup func(t *testing.T, title string) Board
	// marker is a stable substring, independent of the title, that locates
	// the prompt/row line within the rendered view.
	marker string
	// available returns the loose available-width bound named by the
	// ticket's acceptance criteria (b.Width-2 / createModalWidth()-4).
	available func(b Board) int
	// budget returns the expected total rendered-row width (chrome +
	// inlineTitleBudget(available, chrome)), with chrome measured from the
	// real production format constant (mirrors the per-site "Measure chrome
	// by rendering <fmt> with ... an empty title" wiring the plan describes
	// for Phase 4). This equals available in the common case and only goes
	// tighter than available when inlineTitleBudget's floor clamps the
	// title's own sub-budget -- asserting against it proves the fix derives
	// its truncation width from the constant/helper pair, not merely that
	// it happens to fit under the looser bound.
	budget func(b Board) int
	// lineCounter measures the render-site-appropriate "did this wrap onto
	// extra physical lines" signal (see rawPhysicalLineCount /
	// borderedLineCount doc comments).
	lineCounter func(view string) int
}

func inlineTitlePromptSites() []promptSite {
	return []promptSite{
		{
			name:   "close-confirm help bar",
			setup:  setupCloseConfirmSite,
			marker: "Close #",
			available: func(b Board) int {
				return b.Width - 2
			},
			budget: func(b Board) int {
				card := b.closeConfirm.card
				suffix := promptParenthetical(b.keys.Entries(keymap.ModeCloseConfirm, ""), keymap.CommandCloseConfirmConfirm, keymap.CommandCloseConfirmCancel)
				chrome := lipgloss.Width(fmt.Sprintf(closeConfirmPromptFmt, card.Number, "", suffix))
				return chrome + inlineTitleBudget(b.Width-2, chrome)
			},
			lineCounter: rawPhysicalLineCount,
		},
		{
			name:   "label-confirm help bar",
			setup:  setupLabelConfirmSite,
			marker: "doesn't exist",
			available: func(b Board) int {
				return b.Width - 2
			},
			budget: func(b Board) int {
				suffix := promptParenthetical(b.keys.Entries(keymap.ModeLabelConfirm, ""), keymap.CommandLabelConfirmCreate, keymap.CommandLabelConfirmCancel)
				chrome := lipgloss.Width(fmt.Sprintf(labelConfirmPromptFmt, "", suffix))
				return chrome + inlineTitleBudget(b.Width-2, chrome)
			},
			lineCounter: rawPhysicalLineCount,
		},
		{
			name:   "delete confirm-step prompt",
			setup:  setupDeleteConfirmStepSite,
			marker: "permanently delete #",
			available: func(b Board) int {
				return modalContentWidth(b.createModalWidth())
			},
			budget: func(b Board) int {
				card := b.delete.card
				chrome := lipgloss.Width(fmt.Sprintf(deleteConfirmPromptFmt, card.Number, card.Number, ""))
				return chrome + inlineTitleBudget(modalContentWidth(b.createModalWidth()), chrome)
			},
			lineCounter: borderedLineCount,
		},
		{
			name:   "delete comment-step prompt",
			setup:  setupDeleteCommentStepSite,
			marker: "optional comment",
			available: func(b Board) int {
				return modalContentWidth(b.createModalWidth())
			},
			budget: func(b Board) int {
				card := b.delete.card
				chrome := lipgloss.Width(fmt.Sprintf(deleteCommentPromptFmt, card.Number, ""))
				return chrome + inlineTitleBudget(modalContentWidth(b.createModalWidth()), chrome)
			},
			lineCounter: borderedLineCount,
		},
		{
			name:   "PR-picker title",
			setup:  setupPRPickerSite,
			marker: "◀", // ◀ prefixes the always-selected PR row
			available: func(b Board) int {
				return modalContentWidth(50)
			},
			budget: func(b Board) int {
				col := b.Columns[b.ActiveTab]
				pr := col.Cards[col.Cursor].LinkedPRs[b.prPickerIndex]
				chrome := lipgloss.Width(fmt.Sprintf(prPickerTitleFmt, pr.Number, ""))
				var prPrefix string
				if symbol := prStatusSymbol(prStatus(pr)); symbol != "" {
					prPrefix = symbol + " "
				}
				decoration := lipgloss.Width(prPrefix) + lipgloss.Width("◀ ") + lipgloss.Width(" ▶")
				return decoration + chrome + inlineTitleBudget(modalContentWidth(50)-decoration, chrome)
			},
			lineCounter: borderedLineCount,
		},
	}
}

// TestInlineTitlePromptSites_BoundLongTitleAndDoNotWrap covers all four
// ticket acceptance criteria at once: a 500-cell title at each of the five
// sites must (1) render its prompt/row line within that site's available
// cell budget (both the ticket's stated loose bound and the tighter
// per-title budget the fix actually derives from the format constant), and
// (2) not cause the render to grow any extra physical lines relative to a
// short-title render (proving no wrap / no vertical modal growth).
func TestInlineTitlePromptSites_BoundLongTitleAndDoNotWrap(t *testing.T) {
	const shortTitle = "Short Title"
	longTitle := strings.Repeat("A", 500)

	for _, site := range inlineTitlePromptSites() {
		t.Run(site.name, func(t *testing.T) {
			shortBoard := site.setup(t, shortTitle)
			longBoard := site.setup(t, longTitle)

			shortView := shortBoard.View()
			longView := longBoard.View()

			longLine := findLineContaining(t, longView, site.marker)
			content := modalRowContent(t, longLine)
			w := lipgloss.Width(content)
			if avail := site.available(longBoard); w > avail {
				t.Errorf("%s: prompt line content width = %d cells, want <= %d (ticket's stated available-width bound)", site.name, w, avail)
			}
			if budget := site.budget(longBoard); w > budget {
				t.Errorf("%s: prompt line content width = %d cells, want <= %d (inlineTitleBudget derived from the real format constant's chrome)", site.name, w, budget)
			}

			gotLines, wantLines := site.lineCounter(longView), site.lineCounter(shortView)
			if gotLines != wantLines {
				t.Errorf("%s: long-title render has %d relevant lines, want %d (same as short-title render) -- a 500-cell title must not wrap onto extra physical lines", site.name, gotLines, wantLines)
			}
		})
	}
}

// TestLabelConfirmBar_SanitizesControlBytesInLabel covers a gap beyond
// length: unlike the close-confirm bar, the label-confirm bar never routed
// through sanitizeSingleLine (bidi/zero-width stripping, whitespace
// collapsing) before this fix -- %q's own escaping incidentally rendered
// ESC/BEL as visible \x1b/\a literals rather than leaking raw control bytes,
// but this closes the gap in a principled way and now matches every other
// title render site. A malicious label must not leak raw ESC/BEL control
// bytes into the rendered prompt, while the visible text is retained.
func TestLabelConfirmBar_SanitizesControlBytesInLabel(t *testing.T) {
	b := setupLabelConfirmSite(t, "\x1b[31mRED\x1b[0m label\x07")

	view := b.View()
	promptLine := findLineContaining(t, view, "doesn't exist")

	if strings.ContainsRune(promptLine, '\x1b') {
		t.Errorf("label-confirm prompt line = %q, want no ESC (0x1b) byte", promptLine)
	}
	if strings.ContainsRune(promptLine, '\x07') {
		t.Errorf("label-confirm prompt line = %q, want no BEL (0x07) byte", promptLine)
	}
	if !strings.Contains(promptLine, "RED label") {
		t.Errorf("label-confirm prompt line = %q, want visible label text %q retained", promptLine, "RED label")
	}
}

// TestDeleteModal_HeightInvariant_100CellVs500CellTitle covers the
// integration-level "no vertical growth" acceptance criterion directly at
// b.Width = 120 with two different long-title lengths (not just short vs.
// long), for both delete-mode steps.
func TestDeleteModal_HeightInvariant_100CellVs500CellTitle(t *testing.T) {
	title100 := strings.Repeat("B", 100)
	title500 := strings.Repeat("B", 500)

	t.Run("confirm step", func(t *testing.T) {
		b100 := setupDeleteConfirmStepSite(t, title100)
		b500 := setupDeleteConfirmStepSite(t, title500)

		got, want := borderedLineCount(b500.View()), borderedLineCount(b100.View())
		if got != want {
			t.Errorf("delete confirm-step modal bordered line count = %d (500-cell title), want %d (same as 100-cell title) -- modal must not grow vertically", got, want)
		}
	})

	t.Run("comment step", func(t *testing.T) {
		b100 := setupDeleteCommentStepSite(t, title100)
		b500 := setupDeleteCommentStepSite(t, title500)

		got, want := borderedLineCount(b500.View()), borderedLineCount(b100.View())
		if got != want {
			t.Errorf("delete comment-step modal bordered line count = %d (500-cell title), want %d (same as 100-cell title) -- modal must not grow vertically", got, want)
		}
	})
}
