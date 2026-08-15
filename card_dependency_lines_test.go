package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
	"github.com/muesli/termenv"
)

// card_dependency_lines_test.go covers #631's two dependency status lines
// rendered ahead of the sub-issue lines inside cardStatusLines: the
// width-fitted blocked-by line (PR 1/2) and the blocking count line
// (PR 2/2).

// --- openBlockers ---

// TestOpenBlockers verifies the open-state filter is case-insensitive
// against "OPEN" and preserves GitHub's returned order.
func TestOpenBlockers(t *testing.T) {
	card := Card{Blockers: []Blocker{
		{Number: 1, State: "OPEN"},
		{Number: 2, State: "CLOSED"},
		{Number: 3, State: "open"},
		{Number: 4, State: "Open"},
		{Number: 5, State: "MERGED"},
	}}
	got := openBlockers(card)
	wantNumbers := []int{1, 3, 4}
	if len(got) != len(wantNumbers) {
		t.Fatalf("openBlockers() = %d blockers, want %d; got %+v", len(got), len(wantNumbers), got)
	}
	for i, n := range wantNumbers {
		if got[i].Number != n {
			t.Errorf("openBlockers()[%d].Number = %d, want %d (order must be preserved)", i, got[i].Number, n)
		}
	}
}

func TestOpenBlockers_EmptyWhenNoBlockers(t *testing.T) {
	got := openBlockers(Card{})
	if len(got) != 0 {
		t.Errorf("openBlockers() = %v, want empty for a card with no Blockers", got)
	}
}

// --- blockerLabel ---

// TestBlockerLabel covers the six-outcome same-repo/cross-repo matrix from
// the plan: exact-case same repo, case-insensitive same repo (EqualFold),
// cross-repo, and the three "either field empty defaults to same-repo"
// fallbacks (empty blocker RepoNameWithOwner, empty board repoOwner, empty
// board repoName).
func TestBlockerLabel(t *testing.T) {
	tests := []struct {
		name    string
		board   Board
		blocker Blocker
		want    string
	}{
		{
			name:    "same repo exact case renders bare number",
			board:   Board{repoOwner: "matteobortolazzo", repoName: "lazyboards"},
			blocker: Blocker{Number: 42, RepoNameWithOwner: "matteobortolazzo/lazyboards"},
			want:    "#42",
		},
		{
			name:    "same repo case-insensitive match renders bare number",
			board:   Board{repoOwner: "matteobortolazzo", repoName: "lazyboards"},
			blocker: Blocker{Number: 42, RepoNameWithOwner: "MatteoBortolazzo/LazyBoards"},
			want:    "#42",
		},
		{
			name:    "cross-repo renders owner/repo#N",
			board:   Board{repoOwner: "matteobortolazzo", repoName: "lazyboards"},
			blocker: Blocker{Number: 42, RepoNameWithOwner: "other/repo"},
			want:    "other/repo#42",
		},
		{
			name:    "empty blocker RepoNameWithOwner falls back to bare number",
			board:   Board{repoOwner: "matteobortolazzo", repoName: "lazyboards"},
			blocker: Blocker{Number: 42, RepoNameWithOwner: ""},
			want:    "#42",
		},
		{
			name:    "empty board repoOwner falls back to bare number",
			board:   Board{repoOwner: "", repoName: "lazyboards"},
			blocker: Blocker{Number: 42, RepoNameWithOwner: "other/repo"},
			want:    "#42",
		},
		{
			name:    "empty board repoName falls back to bare number",
			board:   Board{repoOwner: "matteobortolazzo", repoName: ""},
			blocker: Blocker{Number: 42, RepoNameWithOwner: "other/repo"},
			want:    "#42",
		},
		{
			name:    "both board repo fields empty falls back to bare number",
			board:   Board{},
			blocker: Blocker{Number: 42, RepoNameWithOwner: "other/repo"},
			want:    "#42",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.board.blockerLabel(tt.blocker)
			if got != tt.want {
				t.Errorf("blockerLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestBlockerLabel_SanitizesHostileRepoNameWithOwner is the explicit
// security-risk coverage (.claude/rules/security.md): a hostile
// RepoNameWithOwner carrying an embedded newline and an ANSI escape must be
// routed through sanitizeSingleLine before composing the cross-repo label,
// never concatenated raw.
func TestBlockerLabel_SanitizesHostileRepoNameWithOwner(t *testing.T) {
	board := Board{repoOwner: "matteobortolazzo", repoName: "lazyboards"}
	hostile := "evil\nrepo\x1b[31m"
	blocker := Blocker{Number: 9, RepoNameWithOwner: hostile}

	got := board.blockerLabel(blocker)

	want := sanitizeSingleLine(hostile) + "#9"
	if got != want {
		t.Errorf("blockerLabel() = %q, want %q (sanitized via sanitizeSingleLine)", got, want)
	}
	if strings.ContainsAny(got, "\n\r\x1b") {
		t.Errorf("blockerLabel() = %q, want no raw control bytes (newline/ESC) in a hostile cross-repo label", got)
	}
}

// --- composeBlockedLine ---

// TestComposeBlockedLine covers the plain-string composition contract: the
// glyph, the named blockers' labels in order, and the "+N" remainder only
// when non-zero. named=nil is the degenerate k=0 form.
func TestComposeBlockedLine(t *testing.T) {
	board := Board{} // no repo configured; every fixture blocker below carries no RepoNameWithOwner, so blockerLabel always renders a bare number
	tests := []struct {
		name      string
		named     []Blocker
		remainder int
		want      string
	}{
		{
			name:      "degenerate zero named with remainder",
			named:     nil,
			remainder: 5,
			want:      fmt.Sprintf("%s +5", blockedByGlyph),
		},
		{
			name:      "single named, no remainder",
			named:     []Blocker{{Number: 1}},
			remainder: 0,
			want:      fmt.Sprintf("%s #1", blockedByGlyph),
		},
		{
			name:      "three named with remainder",
			named:     []Blocker{{Number: 1}, {Number: 2}, {Number: 3}},
			remainder: 7,
			want:      fmt.Sprintf("%s #1 #2 #3 +7", blockedByGlyph),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := board.composeBlockedLine(tt.named, tt.remainder)
			if got != tt.want {
				t.Errorf("composeBlockedLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestComposeBlockedLine_CrossRepoNamedBlockerUsesOwnerRepoForm proves
// composeBlockedLine routes each named blocker through blockerLabel (not a
// bare "#N" concatenation), so a cross-repo blocker in the named set still
// renders as owner/repo#N.
func TestComposeBlockedLine_CrossRepoNamedBlockerUsesOwnerRepoForm(t *testing.T) {
	board := Board{repoOwner: "matteobortolazzo", repoName: "lazyboards"}
	named := []Blocker{{Number: 5, RepoNameWithOwner: "other/repo"}}

	got := board.composeBlockedLine(named, 0)

	want := fmt.Sprintf("%s other/repo#5", blockedByGlyph)
	if got != want {
		t.Errorf("composeBlockedLine() = %q, want %q", got, want)
	}
}

// --- blockedByLine: gate + descending-fit-loop matrix ---

// TestBlockedByLine_FitMatrix is the table-driven fit matrix named in the
// plan's Test Strategy: 0/1/3/>3 open blockers, budget<=0 (both via a
// non-positive contentWidth-indentWidth and via contentWidth==0 itself),
// the exact-fit boundary and the one-cell-narrower degrade, the degenerate
// k=0 "glyph +N" fallback, BlockedByCount > len(Blockers), BlockedByCount <
// len(openBlockers) (the "summary count is the display authority" cap), and
// all-open-Blockers-closed-with-nonzero-count.
func TestBlockedByLine_FitMatrix(t *testing.T) {
	board := Board{} // no repo configured; every fixture blocker below carries no RepoNameWithOwner, so blockerLabel always renders a bare number
	indentWidth := 3
	fiveOpenBlockers := []Blocker{
		{Number: 1, State: "OPEN"},
		{Number: 2, State: "OPEN"},
		{Number: 3, State: "OPEN"},
		{Number: 4, State: "OPEN"},
		{Number: 5, State: "OPEN"},
	}

	t.Run("BlockedByCount zero yields no line", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: 0}
		got := board.blockedByLine(card, indentWidth, 100)
		if got != "" {
			t.Errorf("blockedByLine() = %q, want \"\" (BlockedByCount == 0)", got)
		}
	})

	t.Run("one open blocker at a wide width", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: 1, Blockers: []Blocker{{Number: 1, State: "OPEN"}}}
		content := fmt.Sprintf("%s #1", blockedByGlyph)
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(content)
		got := board.blockedByLine(card, indentWidth, 100)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q", got, want)
		}
	})

	t.Run("three open blockers at a wide width, no remainder", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: 3, Blockers: fiveOpenBlockers[:3]}
		content := fmt.Sprintf("%s #1 #2 #3", blockedByGlyph)
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(content)
		got := board.blockedByLine(card, indentWidth, 100)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q", got, want)
		}
	})

	t.Run("more than three open blockers caps at three plus remainder", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: 5, Blockers: fiveOpenBlockers}
		content := fmt.Sprintf("%s #1 #2 #3 +2", blockedByGlyph)
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(content)
		got := board.blockedByLine(card, indentWidth, 100)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q", got, want)
		}
	})

	t.Run("BlockedByCount below len(openBlockers) caps named to the count, not the list", func(t *testing.T) {
		// 5 open blockers are listed but the summary count says only 2 are
		// open -- the count is the display authority (AC1), so at most 2
		// are ever named, never 3.
		card := Card{Number: 1, BlockedByCount: 2, Blockers: fiveOpenBlockers}
		content := fmt.Sprintf("%s #1 #2", blockedByGlyph)
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(content)
		got := board.blockedByLine(card, indentWidth, 100)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q", got, want)
		}
	})

	t.Run("budget exactly zero (contentWidth == indentWidth) truncates the degenerate form to empty", func(t *testing.T) {
		// budget == 0 hits the early "budget <= 0" branch, not the
		// loop-exhausted fallback the "degenerate form ... truncated to
		// budget" case below exercises -- both must route the same
		// unbounded k=0 form through truncateCell, and truncateCell(_, 0)
		// is defined to return "" (never the untruncated form, which could
		// carry an unbounded digit count from a hostile/malformed count).
		card := Card{Number: 1, BlockedByCount: 5, Blockers: fiveOpenBlockers}
		content := fmt.Sprintf("%s +5", blockedByGlyph)
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(truncateCell(content, 0))
		got := board.blockedByLine(card, indentWidth, indentWidth)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q (budget == 0 must truncate the degenerate form via truncateCell, never render it unbounded)", got, want)
		}
	})

	t.Run("non-positive contentWidth (pre-WindowSizeMsg) truncates the degenerate form to empty", func(t *testing.T) {
		// contentWidth == 0 drives budget negative (0 - indentWidth), the
		// same early "budget <= 0" branch as the zero-budget case above;
		// truncateCell treats any non-positive width identically, so this
		// too must resolve to "", never the untruncated named form.
		card := Card{Number: 1, BlockedByCount: 5, Blockers: fiveOpenBlockers}
		content := fmt.Sprintf("%s +5", blockedByGlyph)
		budget := 0 - indentWidth
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(truncateCell(content, budget))
		got := board.blockedByLine(card, indentWidth, 0)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q (contentWidth == 0 must truncate the degenerate form via truncateCell, never render it unbounded)", got, want)
		}
	})

	t.Run("exact-fit boundary selects the full named set", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: 5, Blockers: fiveOpenBlockers}
		fullContent := fmt.Sprintf("%s #1 #2 #3 +2", blockedByGlyph)
		fullWidth := lipgloss.Width(fullContent)
		contentWidth := indentWidth + fullWidth // budget == fullWidth exactly
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(fullContent)
		got := board.blockedByLine(card, indentWidth, contentWidth)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q (line exactly fills the budget)", got, want)
		}
	})

	t.Run("one cell narrower than the exact fit degrades to two named", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: 5, Blockers: fiveOpenBlockers}
		fullContent := fmt.Sprintf("%s #1 #2 #3 +2", blockedByGlyph)
		fullWidth := lipgloss.Width(fullContent)
		contentWidth := indentWidth + fullWidth - 1
		degradedContent := fmt.Sprintf("%s #1 #2 +3", blockedByGlyph)
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(degradedContent)
		got := board.blockedByLine(card, indentWidth, contentWidth)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q (one cell under the exact fit must drop to k=2, remainder 3)", got, want)
		}
	})

	t.Run("degenerate form when nothing fits, truncated to budget", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: 5, Blockers: fiveOpenBlockers}
		fullContent := fmt.Sprintf("%s +5", blockedByGlyph)
		budget := 1
		wantContent := truncateCell(fullContent, budget)
		if lipgloss.Width(wantContent) > budget {
			t.Fatalf("test setup: truncateCell(%q, %d) = %q still exceeds budget %d", fullContent, budget, wantContent, budget)
		}
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(wantContent)
		// budget == 1: too narrow to fit even a single named blocker, and
		// even the unbounded k=0 form ("<glyph> +5") itself overflows this
		// budget, so it must be truncated via truncateCell rather than
		// dropped or left overflowing (the k=0 form is never dropped
		// entirely, but it must never exceed budget either).
		got := board.blockedByLine(card, indentWidth, indentWidth+budget)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q (the k=0 fallback must be truncated to fit budget, never left overflowing)", got, want)
		}
	})

	t.Run("negative BlockedByCount yields no line and does not panic", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: -3, Blockers: fiveOpenBlockers}
		got := board.blockedByLine(card, indentWidth, 100)
		if got != "" {
			t.Errorf("blockedByLine() = %q, want \"\" for a negative BlockedByCount (malformed/hostile summary count must not slice open[:named] with a negative index)", got)
		}
	})

	t.Run("BlockedByCount exceeds len(Blockers)", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: 10, Blockers: []Blocker{
			{Number: 1, State: "OPEN"},
			{Number: 2, State: "OPEN"},
			{Number: 3, State: "CLOSED"},
		}}
		content := fmt.Sprintf("%s #1 #2 +8", blockedByGlyph)
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(content)
		got := board.blockedByLine(card, indentWidth, 100)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q", got, want)
		}
	})

	t.Run("all sampled blockers closed with a nonzero BlockedByCount degenerates to glyph plus N", func(t *testing.T) {
		card := Card{Number: 1, BlockedByCount: 4, Blockers: []Blocker{
			{Number: 1, State: "CLOSED"},
			{Number: 2, State: "CLOSED"},
		}}
		content := fmt.Sprintf("%s +4", blockedByGlyph)
		want := strings.Repeat(" ", indentWidth) + prBlockedStyle.Render(content)
		got := board.blockedByLine(card, indentWidth, 100)
		if got != want {
			t.Errorf("blockedByLine() = %q, want %q (no open blockers sampled, so named=0 regardless of width)", got, want)
		}
	})
}

// --- cardStatusLines integration: gate + line order ---

// TestCardStatusLines_BlockedByLine_PrependedAheadOfSubIssue verifies the
// blocked-by line renders first among all four status-line kinds, per the
// ticket's stated order: blocked-by, blocking, sub-issue(s), agent(s),
// PR(s) (blocking itself is out of scope for this PR).
func TestCardStatusLines_BlockedByLine_PrependedAheadOfSubIssue(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{
			Number:         7,
			Title:          "Blocked parent card",
			BlockedByCount: 1,
			Blockers:       []provider.Blocker{{Number: 3, State: "OPEN"}},
			SubIssueCount:  2,
			LinkedPRs: []provider.LinkedPR{
				{Number: 11, Title: "feat: PR", URL: "https://github.com/o/r/pull/11", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
			},
		},
	}, 120, 40)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{{WindowName: "7", Status: "running", Agent: "claude"}},
	}
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth, 100)
	if len(lines) != 4 {
		t.Fatalf("cardStatusLines() = %d lines, want 4 (blocked + sub-issue + agent + PR); got %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], blockedByGlyph) {
		t.Errorf("lines[0] = %q, want the blocked-by line first", lines[0])
	}
	if !strings.Contains(lines[1], subIssueParentGlyph) {
		t.Errorf("lines[1] = %q, want the sub-issue line second", lines[1])
	}
	wantAgentBadge := agentBadgeStyle("running").Render(agentBadgeText("running", "claude"))
	if !strings.Contains(lines[2], wantAgentBadge) {
		t.Errorf("lines[2] = %q, want the agent line third", lines[2])
	}
	wantPR := prStatusPrefix("mergeable") + "#11"
	if !strings.Contains(lines[3], wantPR) {
		t.Errorf("lines[3] = %q, want the PR line fourth", lines[3])
	}
}

// TestCardStatusLines_BlockedByLine_AbsentWhenBlockedByCountZero is the
// direct AC1 gate assertion at the cardStatusLines level: a card whose
// blockers have all closed (BlockedByCount == 0) renders no blocked-by
// line, even though TotalBlockedByCount and the raw Blockers list are both
// non-empty.
func TestCardStatusLines_BlockedByLine_AbsentWhenBlockedByCountZero(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{
			Number:              8,
			Title:               "All blockers closed",
			BlockedByCount:      0,
			TotalBlockedByCount: 2,
			Blockers: []provider.Blocker{
				{Number: 1, State: "CLOSED"},
				{Number: 2, State: "CLOSED"},
			},
			SubIssueCount: 1,
		},
	}, 120, 40)
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth, 100)
	for _, line := range lines {
		if strings.Contains(line, blockedByGlyph) {
			t.Fatalf("cardStatusLines() = %v, want no blocked-by line when BlockedByCount == 0 (even though TotalBlockedByCount > 0 and Blockers is non-empty)", lines)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("cardStatusLines() = %d lines, want 1 (only the sub-issue line); got %v", len(lines), lines)
	}
}

// --- Height invariance across widths ---

// TestCardLineCount_BlockedCard_HeightInvariantAcrossWidths is the direct
// AC7 assertion: the same blocked card yields an identical cardLineCount at
// a wide and a narrow contentWidth, with differing blockedByLine content --
// fitting changes line content only, never line count.
func TestCardLineCount_BlockedCard_HeightInvariantAcrossWidths(t *testing.T) {
	board := newBoardWithInlineCards(t, []provider.Card{
		{
			Number:         1,
			Title:          "X",
			BlockedByCount: 5,
			Blockers: []provider.Blocker{
				{Number: 10, State: "OPEN"},
				{Number: 11, State: "OPEN"},
				{Number: 12, State: "OPEN"},
				{Number: 13, State: "OPEN"},
				{Number: 14, State: "OPEN"},
			},
		},
	}, 120, 40)
	card := board.Columns[0].Cards[0]
	columnNames := []string{board.Columns[0].Title}
	indentWidth := cardTitlePrefixWidth(card)
	wideWidth, narrowWidth := 100, 10

	wideCount := board.cardLineCount(card, wideWidth, columnNames)
	narrowCount := board.cardLineCount(card, narrowWidth, columnNames)
	if wideCount != narrowCount {
		t.Fatalf("cardLineCount() = %d at width %d, %d at width %d; want identical line counts (fitting changes content, never count)", wideCount, wideWidth, narrowCount, narrowWidth)
	}
	if want := 2; wideCount != want { // 1 title line ("X" never wraps at either width) + 1 blocked-by line
		t.Fatalf("cardLineCount() = %d, want %d (1 title line + 1 blocked-by line)", wideCount, want)
	}

	wideLine := board.blockedByLine(card, indentWidth, wideWidth)
	narrowLine := board.blockedByLine(card, indentWidth, narrowWidth)
	if wideLine == narrowLine {
		t.Fatalf("blockedByLine content is identical at both widths (%q); want it to differ, proving the fitter actually degraded at the narrow width", wideLine)
	}
}

// TestViewCardList_BlockedCard_NarrowWidth_RowCountMatchesCardLineCount is
// the explicit risk-mitigation test named in the plan's Risks section: an
// unfitted or mis-fitted blocked line wider than contentWidth would render
// as two physical rows while cardLineCount still counts one, corrupting
// clampScrollOffset/handleCardClick (docs/list-cursor-invariants.md).
// Modeled on TestCardLineCount_MatchesViewCardList_TitleWithEmbeddedNewline
// (view_test.go:756-778).
func TestViewCardList_BlockedCard_NarrowWidth_RowCountMatchesCardLineCount(t *testing.T) {
	board := newBoardWithInlineCards(t, []provider.Card{
		{
			Number:         1,
			Title:          "Short",
			BlockedByCount: 5,
			Blockers: []provider.Blocker{
				{Number: 10, State: "OPEN"},
				{Number: 11, State: "OPEN"},
				{Number: 12, State: "OPEN"},
				{Number: 13, State: "OPEN"},
				{Number: 14, State: "OPEN"},
			},
		},
	}, 120, 40)
	card := board.Columns[0].Cards[0]
	columnNames := []string{board.Columns[0].Title}
	contentWidth := 12

	view := board.viewCardList(board.Columns[0], 20, contentWidth, leftPanelStyle)
	assertOneRow(t, view, blockedByGlyph)

	titleRows := 0
	blockedRows := 0
	for _, line := range strings.Split(view, "\n") {
		switch {
		case strings.Contains(line, "Short"):
			titleRows++
		case strings.Contains(line, blockedByGlyph):
			blockedRows++
		}
	}
	actualRows := titleRows + blockedRows

	gotLines := board.cardLineCount(card, contentWidth, columnNames)
	if gotLines != actualRows {
		t.Errorf("cardLineCount() = %d, want %d to match the card's actual physical rows (title=%d + blocked=%d) at a narrow width -- a mis-fitted blocked line spilling onto 2 rows would desync this", gotLines, actualRows, titleRows, blockedRows)
	}
}

// TestViewCardList_BlockedCard_NarrowerWidth_K0FormTruncatedRowCountMatchesCardLineCount
// is the narrower-width sibling of the test above: it drives contentWidth
// down far enough that even the degenerate k=0 fallback ("<glyph> +N")
// itself overflows budget, the exact scenario the review round's fix
// addressed (blockedByLine now truncates that form via truncateCell). The
// test above only reaches the k=1 branch, so it cannot exercise this path
// on its own.
func TestViewCardList_BlockedCard_NarrowerWidth_K0FormTruncatedRowCountMatchesCardLineCount(t *testing.T) {
	board := newBoardWithInlineCards(t, []provider.Card{
		{
			// A 2-rune title keeps "#1 Hi" (5 runes) on a single
			// unwrapped title row at the narrow contentWidth below --
			// unlike the sibling test's "Short", which would itself wrap
			// at this width and defeat the row-counting substring match.
			Number:         1,
			Title:          "Hi",
			BlockedByCount: 5,
			Blockers: []provider.Blocker{
				{Number: 10, State: "OPEN"},
				{Number: 11, State: "OPEN"},
				{Number: 12, State: "OPEN"},
				{Number: 13, State: "OPEN"},
				{Number: 14, State: "OPEN"},
			},
		},
	}, 120, 40)
	card := board.Columns[0].Cards[0]
	columnNames := []string{board.Columns[0].Title}
	indentWidth := 3 // "#1 " prefix, per cardDisplayText
	contentWidth := indentWidth + 2

	// Self-check: even the unbounded degenerate form must overflow this
	// budget, proving the test actually drives the fitter into the k=0
	// overflow case rather than just the k=1 branch the sibling test above
	// reaches.
	degenerateContent := board.composeBlockedLine(nil, card.BlockedByCount)
	if lipgloss.Width(degenerateContent) <= contentWidth-indentWidth {
		t.Fatalf("test setup: degenerate form %q (width %d) fits budget %d without truncation -- narrow contentWidth further", degenerateContent, lipgloss.Width(degenerateContent), contentWidth-indentWidth)
	}
	// Self-check: the title itself must not wrap at this contentWidth, or
	// the row-counting substring match below becomes unreliable.
	if got := len(wrapTitle("#1 Hi", contentWidth, indentWidth)); got != 1 {
		t.Fatalf("test setup: title wraps into %d rows at contentWidth %d -- pick a shorter title or wider contentWidth", got, contentWidth)
	}

	view := board.viewCardList(board.Columns[0], 20, contentWidth, leftPanelStyle)
	assertOneRow(t, view, blockedByGlyph)

	// Count physical content rows directly from the bordered panel's
	// interior lines, rather than pattern-matching known substrings like
	// the sibling test above: an untruncated k=0 form that lipgloss's
	// word-wrap splits into a continuation row (e.g. "+5" wrapping onto
	// its own line, with no blockedByGlyph on it) would otherwise go
	// uncounted by a substring-based tally and mask exactly the row-count
	// desync this test exists to catch.
	viewLines := strings.Split(view, "\n")
	actualRows := 0
	for _, line := range viewLines {
		if strings.HasPrefix(line, "╭") || strings.HasPrefix(line, "╰") {
			continue // top/bottom border
		}
		inner := strings.TrimSuffix(strings.TrimPrefix(line, "│"), "│")
		if strings.TrimSpace(inner) != "" {
			actualRows++
		}
	}

	gotLines := board.cardLineCount(card, contentWidth, columnNames)
	if gotLines != actualRows {
		t.Errorf("cardLineCount() = %d, want %d to match the card's actual physical content rows when even the k=0 fallback overflows budget -- an untruncated k=0 form spilling onto 2 rows would desync this", gotLines, actualRows)
	}
}

// TestViewCardList_BlockedCard_ZeroBudget_TruncatedRowCountMatchesCardLineCount
// is the second remaining-Must-Fix regression guard from the review round
// (the first is TestViewCardList_BlockedCard_NarrowerWidth_K0FormTruncatedRowCountMatchesCardLineCount
// above): it drives contentWidth down to exactly indentWidth, so budget ==
// contentWidth-indentWidth == 0 and blockedByLine takes the *early*
// "budget <= 0" return -- a different code path than the sibling test
// above, which only ever reaches the loop-exhausted fallback (budget > 0,
// but even k=0 overflows it). Both branches produce the same unbounded
// "<glyph> +N" degenerate form and must be truncated identically via
// truncateCell; before the second review-round fix, only the
// loop-exhausted branch was truncated, leaving this early-return branch
// free to render the untruncated form and spill onto a second physical
// row.
//
// Unlike the sibling test, truncateCell(_, budget) at budget == 0 always
// resolves to "" (never a partial glyph), so the rendered blocked-by row
// is indent-only whitespace -- indistinguishable from a blank
// height-padding row under the sibling test's TrimSpace-filtered tally.
// This test instead sizes panelHeight exactly to cardLineCount() (no
// slack for padding rows) and tallies every interior row unconditionally,
// so a wrap-induced extra physical row (the bug this guards against)
// still desyncs the count against the tight panelHeight even though the
// row's content is blank.
func TestViewCardList_BlockedCard_ZeroBudget_TruncatedRowCountMatchesCardLineCount(t *testing.T) {
	board := newBoardWithInlineCards(t, []provider.Card{
		{
			// An empty title keeps "#1 " (exactly indentWidth runes) on a
			// single unwrapped title row at contentWidth == indentWidth
			// below -- any non-empty title would itself force wrapTitle to
			// wrap at this width (a separate, out-of-scope narrow-width
			// behavior of wrapTitle's own capacity clamp), which would
			// contaminate the row tally this test needs to isolate to the
			// blocked-by line alone.
			Number:         1,
			Title:          "",
			BlockedByCount: 5,
			Blockers: []provider.Blocker{
				{Number: 10, State: "OPEN"},
				{Number: 11, State: "OPEN"},
				{Number: 12, State: "OPEN"},
				{Number: 13, State: "OPEN"},
				{Number: 14, State: "OPEN"},
			},
		},
	}, 120, 40)
	card := board.Columns[0].Cards[0]
	columnNames := []string{board.Columns[0].Title}
	indentWidth := cardTitlePrefixWidth(card) // "#1 " prefix
	contentWidth := indentWidth               // budget == contentWidth - indentWidth == 0

	// Self-check: the branch under test is the early "budget <= 0" return,
	// not the loop-exhausted fallback the sibling test above reaches.
	if budget := contentWidth - indentWidth; budget > 0 {
		t.Fatalf("test setup: budget = %d, want <= 0 to hit the early-return branch under test", budget)
	}
	// Self-check: the fix must truncate the degenerate form to empty at
	// this budget, proving the row under test really is the blank-content
	// case the TrimSpace-filtered tally in the sibling test cannot see.
	if got := board.blockedByLine(card, indentWidth, contentWidth); strings.TrimSpace(got) != "" {
		t.Fatalf("test setup: blockedByLine() = %q, want indent-only whitespace (truncateCell(_, 0) == \"\") at this budget", got)
	}

	panelHeight := board.cardLineCount(card, contentWidth, columnNames)

	view := board.viewCardList(board.Columns[0], panelHeight, contentWidth, leftPanelStyle)

	actualRows := 0
	for _, line := range strings.Split(view, "\n") {
		if strings.HasPrefix(line, "╭") || strings.HasPrefix(line, "╰") {
			continue // top/bottom border
		}
		if !strings.HasPrefix(line, "│") {
			continue // outside the bordered panel entirely
		}
		actualRows++
	}

	if actualRows != panelHeight {
		t.Errorf("rendered %d interior rows, want %d (== cardLineCount()) -- an untruncated budget<=0 degenerate form spilling onto an extra physical row via lipgloss word-wrap would desync this even though its content is blank", actualRows, panelHeight)
	}
}

// --- Color exemption on unfocused cards ---

// TestCardStatusLines_BlockedByLine_ColorsOnUnfocusedCard is the AC5
// coverage: the blocked-by line renders in hue 215 on every card, focused
// or not. Mirrors TestCardStatusLines_AgentAndPRGlyphsKeepStatusColor
// (view_test.go) at the board (viewCardList) integration level, per the
// Test Strategy's "assert the prBlockedStyle-rendered content survives the
// selectedRowStyle wrap on a non-cursor card" note -- an outer wrap cannot
// recolor an already-rendered inner style (docs/terminal-rendering.md), so
// this only passes if the color is baked in at construction inside
// blockedByLine/cardStatusLines, never applied by the caller.
func TestCardStatusLines_BlockedByLine_ColorsOnUnfocusedCard(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	focusedCard := provider.Card{Number: 1, Title: "Focused card"}
	unfocusedCard := provider.Card{
		Number: 2, Title: "Blocked card", BlockedByCount: 1,
		Blockers: []provider.Blocker{{Number: 9, State: "OPEN"}},
	}
	b := newBoardWithInlineCards(t, []provider.Card{focusedCard, unfocusedCard}, 120, 40)
	b.Columns[0].Cursor = 0 // card 1 is focused; card 2 (the blocked one) is not.

	out := b.viewCardList(b.Columns[0], 40, 60, leftPanelStyle)

	appCard := Card{Number: 2, BlockedByCount: 1, Blockers: []Blocker{{Number: 9, State: "OPEN"}}}
	wantContent := b.composeBlockedLine(openBlockers(appCard), 0)
	wantBlockedLine := prBlockedStyle.Render(wantContent)
	if wantBlockedLine == mutedRowStyle.Render(wantContent) {
		t.Fatal("test setup: hue-215 blocked-by styling and its muted rendering must differ (color profile not forced?)")
	}
	if !strings.Contains(out, wantBlockedLine) {
		t.Errorf("unfocused card's blocked-by line missing hue-215 styled rendering %q; got:\n%s", wantBlockedLine, out)
	}
}

// --- blockingLine (PR 2/2) ---

// TestBlockingLine_Gate covers the open-count display gate: the line
// renders only when the summary's open blocking count is positive. A card
// whose blocked dependents have all closed (BlockingCount == 0 while
// TotalBlockingCount is non-zero) renders nothing, and a malformed
// negative count is treated as "none" rather than composing a "󰳘 -1" row.
func TestBlockingLine_Gate(t *testing.T) {
	tests := []struct {
		name          string
		card          Card
		wantRendered  bool
		wantCountText string
	}{
		{
			name:          "positive open count renders the line",
			card:          Card{BlockingCount: 3, TotalBlockingCount: 3},
			wantRendered:  true,
			wantCountText: "3",
		},
		{
			name:         "zero open count renders nothing even with closed dependents",
			card:         Card{BlockingCount: 0, TotalBlockingCount: 4},
			wantRendered: false,
		},
		{
			name:         "negative open count renders nothing",
			card:         Card{BlockingCount: -1, TotalBlockingCount: 2},
			wantRendered: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Board{}.blockingLine(tt.card, 3, 40)
			if !tt.wantRendered {
				if got != "" {
					t.Fatalf("blockingLine() = %q, want an empty string (no line)", got)
				}
				return
			}
			if !strings.Contains(got, blockingGlyph) {
				t.Errorf("blockingLine() = %q, want it to carry the blocking glyph %q", got, blockingGlyph)
			}
			if !strings.Contains(got, tt.wantCountText) {
				t.Errorf("blockingLine() = %q, want it to name the open blocking count %q", got, tt.wantCountText)
			}
		})
	}
}

// TestBlockingLine_UsesOpenCountNeverTotal is the direct AC6 authority
// assertion: the line reports the summary's open blocking count, never the
// closed-inclusive total.
func TestBlockingLine_UsesOpenCountNeverTotal(t *testing.T) {
	card := Card{BlockingCount: 2, TotalBlockingCount: 7}

	got := Board{}.blockingLine(card, 0, 40)

	if want := fmt.Sprintf("%s %d", blockingGlyph, card.BlockingCount); !strings.Contains(got, want) {
		t.Errorf("blockingLine() = %q, want it to contain %q (the open count)", got, want)
	}
	if strings.Contains(got, fmt.Sprintf("%d", card.TotalBlockingCount)) {
		t.Errorf("blockingLine() = %q, want it never to render TotalBlockingCount (%d)", got, card.TotalBlockingCount)
	}
}

// TestBlockingLine_RendersInStructuralGrayNotWarningHue is the AC6 color
// assertion: the blocking direction is quiet structural context in the
// existing muted gray (subIssueStyle's hue 245), deliberately not the
// blocked-by line's hue-215 warning style.
func TestBlockingLine_RendersInStructuralGrayNotWarningHue(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	card := Card{BlockingCount: 3}
	content := fmt.Sprintf("%s %d", blockingGlyph, card.BlockingCount)
	if subIssueStyle.Render(content) == prBlockedStyle.Render(content) {
		t.Fatal("test setup: structural-gray and warning-hue renderings must differ (color profile not forced?)")
	}

	got := Board{}.blockingLine(card, 0, 40)

	if want := subIssueStyle.Render(content); got != want {
		t.Errorf("blockingLine() = %q, want %q (structural gray, the sub-issue lines' hue)", got, want)
	}
}

// TestBlockingLine_IndentedUnderTitle asserts the line carries the same
// continuation indent every other status line uses to align under the card
// title text.
func TestBlockingLine_IndentedUnderTitle(t *testing.T) {
	indentWidth := 4

	got := Board{}.blockingLine(Card{BlockingCount: 1}, indentWidth, 40)

	if want := strings.Repeat(" ", indentWidth); !strings.HasPrefix(got, want) {
		t.Errorf("blockingLine() = %q, want it to start with %d spaces of continuation indent", got, indentWidth)
	}
}

// TestBlockingLine_ClampedAtPathologicalWidth covers the bounds guard the
// blocked-by line's degenerate form already has (see the Sibling Code Path
// Auditing rule in .claude/rules/testing.md): although "󰳘 N" is bounded by
// construction, a column narrower than the glyph plus the count's digits
// would spill onto a second physical row and desync cardLineCount. The
// line is clamped to the payload budget instead, never wrapped -- and with
// a non-positive budget only the continuation indent survives, exactly as
// blockedByLine's degenerate form behaves at the same widths.
func TestBlockingLine_ClampedAtPathologicalWidth(t *testing.T) {
	tests := []struct {
		name         string
		indentWidth  int
		contentWidth int
	}{
		{name: "budget narrower than the composed line", indentWidth: 3, contentWidth: 5},
		{name: "budget of exactly one cell", indentWidth: 0, contentWidth: 1},
		{name: "indent alone consumes the width", indentWidth: 4, contentWidth: 4},
		{name: "pre-WindowSizeMsg zero width", indentWidth: 0, contentWidth: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			card := Card{BlockingCount: 123456}

			got := Board{}.blockingLine(card, tt.indentWidth, tt.contentWidth)

			if strings.Contains(got, "\n") {
				t.Fatalf("blockingLine() = %q, want a single physical row", got)
			}
			budget := tt.contentWidth - tt.indentWidth
			// With a positive budget the whole line must fit contentWidth;
			// with a non-positive one the payload is clamped away and only
			// the indent remains.
			maxWidth := tt.indentWidth
			if budget > 0 {
				maxWidth = tt.contentWidth
			}
			if w := lipgloss.Width(got); w > maxWidth {
				t.Errorf("blockingLine() width = %d, want <= %d so it cannot wrap onto a second row", w, maxWidth)
			}
			if budget > 0 && got == "" {
				t.Error("blockingLine() returned no line at a positive budget; want a clamped one -- dropping it would misreport the card as unblocking")
			}
			if budget <= 0 && strings.TrimSpace(got) != "" {
				t.Errorf("blockingLine() = %q at a non-positive budget, want the payload clamped away to at most the indent", got)
			}
		})
	}
}

// --- cardStatusLines integration: blocking line placement ---

// TestCardStatusLines_BlockingLine_BetweenBlockedAndSubIssue verifies the
// full status-line order the ticket specifies: blocked-by, blocking,
// sub-issue(s), agent(s), PR(s).
func TestCardStatusLines_BlockingLine_BetweenBlockedAndSubIssue(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{
			Number:         7,
			Title:          "Blocked and blocking card",
			BlockedByCount: 1,
			Blockers:       []provider.Blocker{{Number: 3, State: "OPEN"}},
			BlockingCount:  2,
			SubIssueCount:  2,
			LinkedPRs: []provider.LinkedPR{
				{Number: 11, Title: "feat: PR", URL: "https://github.com/o/r/pull/11", Mergeable: "MERGEABLE", MergeStateStatus: "CLEAN"},
			},
		},
	}, 120, 40)
	b.agentSnapshot = &cenciwatch.StateSnapshot{
		Windows: []cenciwatch.WindowState{{WindowName: "7", Status: "running", Agent: "claude"}},
	}
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth, 100)
	if len(lines) != 5 {
		t.Fatalf("cardStatusLines() = %d lines, want 5 (blocked + blocking + sub-issue + agent + PR); got %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], blockedByGlyph) {
		t.Errorf("lines[0] = %q, want the blocked-by line first", lines[0])
	}
	if !strings.Contains(lines[1], blockingGlyph) {
		t.Errorf("lines[1] = %q, want the blocking line second", lines[1])
	}
	if !strings.Contains(lines[2], subIssueParentGlyph) {
		t.Errorf("lines[2] = %q, want the sub-issue line third", lines[2])
	}
	wantAgentBadge := agentBadgeStyle("running").Render(agentBadgeText("running", "claude"))
	if !strings.Contains(lines[3], wantAgentBadge) {
		t.Errorf("lines[3] = %q, want the agent line fourth", lines[3])
	}
	wantPR := prStatusPrefix("mergeable") + "#11"
	if !strings.Contains(lines[4], wantPR) {
		t.Errorf("lines[4] = %q, want the PR line fifth", lines[4])
	}
}

// TestCardStatusLines_BlockingLine_AbsentWhenBlockingCountZero is the AC6
// gate at the cardStatusLines level: a card whose blocked dependents have
// all closed renders no blocking line even though TotalBlockingCount is
// non-zero, and incurs no vertical cost.
func TestCardStatusLines_BlockingLine_AbsentWhenBlockingCountZero(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{
			Number:             8,
			Title:              "All dependents closed",
			BlockingCount:      0,
			TotalBlockingCount: 3,
			SubIssueCount:      1,
		},
	}, 120, 40)
	card := b.Columns[0].Cards[0]
	indentWidth := cardTitlePrefixWidth(card)

	lines := b.cardStatusLines(card, indentWidth, 100)
	for _, line := range lines {
		if strings.Contains(line, blockingGlyph) {
			t.Fatalf("cardStatusLines() = %v, want no blocking line when BlockingCount == 0 (even though TotalBlockingCount > 0)", lines)
		}
	}
	if len(lines) != 1 {
		t.Fatalf("cardStatusLines() = %d lines, want 1 (only the sub-issue line); got %v", len(lines), lines)
	}
}

// --- Height ripple ---

// TestCardLineCount_BlockingCard_HeightGrowsByExactlyOne is the AC8
// assertion for the blocking direction: producing the line inside
// cardStatusLines means every height consumer picks it up automatically,
// and it costs exactly one row -- at any width, since the line's content is
// bounded rather than width-fitted.
func TestCardLineCount_BlockingCard_HeightGrowsByExactlyOne(t *testing.T) {
	board := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "X"},
		{Number: 2, Title: "X", BlockingCount: 4, TotalBlockingCount: 4},
	}, 120, 40)
	plainCard := board.Columns[0].Cards[0]
	blockingCard := board.Columns[0].Cards[1]
	columnNames := []string{board.Columns[0].Title}

	for _, width := range []int{100, 10} {
		plain := board.cardLineCount(plainCard, width, columnNames)
		blocking := board.cardLineCount(blockingCard, width, columnNames)
		if got := blocking - plain; got != 1 {
			t.Errorf("cardLineCount() at width %d grew by %d rows for a blocking card, want exactly 1 (plain=%d, blocking=%d)", width, got, plain, blocking)
		}
	}
}

// TestViewCardList_BlockingCard_NarrowWidth_RowCountMatchesCardLineCount
// pins the height invariant against the real renderer: an unclamped
// blocking line wider than contentWidth wraps onto a second physical row
// (lipgloss's panel Width wraps overflowing content) while cardLineCount
// still counts one, corrupting clampScrollOffset and handleCardClick
// (docs/list-cursor-invariants.md). The 6-digit count is what makes the
// composed line overflow this deliberately narrow column.
//
// The rows are counted by occupancy rather than by matching the glyph:
// a wrapped continuation row carries only the count's spilled digits and
// no glyph, so a glyph-matching count cannot see the very wrap this test
// exists to catch (verified by mutation -- removing the clamp from
// blockingLine must fail this test).
func TestViewCardList_BlockingCard_NarrowWidth_RowCountMatchesCardLineCount(t *testing.T) {
	board := newBoardWithInlineCards(t, []provider.Card{
		// A 2-rune title keeps "#1 Hi" on a single unwrapped title row at
		// the narrow contentWidth below, so the only row count that can
		// vary is the status line's.
		{Number: 1, Title: "Hi", BlockingCount: 987654, TotalBlockingCount: 987654},
	}, 120, 40)
	card := board.Columns[0].Cards[0]
	columnNames := []string{board.Columns[0].Title}
	contentWidth := 6

	view := board.viewCardList(board.Columns[0], 20, contentWidth, leftPanelStyle)
	assertOneRow(t, view, blockingGlyph)

	actualRows := occupiedPanelRows(view)
	gotLines := board.cardLineCount(card, contentWidth, columnNames)
	if gotLines != actualRows {
		t.Errorf("cardLineCount() = %d, want %d to match the card's actual occupied physical rows at a narrow width -- a clamped-away blocking line must not spill onto a second row", gotLines, actualRows)
	}
}

// occupiedPanelRows counts the rows of a rendered card-list panel that
// carry any content, excluding the panel's border rows and its blank
// filler rows. Unlike matching a specific glyph or title substring, this
// also counts a wrapped continuation row, which carries neither.
func occupiedPanelRows(view string) int {
	count := 0
	for _, row := range strings.Split(view, "\n") {
		inner := strings.TrimSpace(strings.Trim(ansi.Strip(row), "│╭╮╰╯─"))
		if inner != "" {
			count++
		}
	}
	return count
}
