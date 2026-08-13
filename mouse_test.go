package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Mouse helpers ---

// newMouseEnabledBoard creates a loaded Board with mouseEnabled=true.
// It uses the standard FakeProvider and sets Width=120, Height=40.
func newMouseEnabledBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", true, false, nil, nil, true)

	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	loaded, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	loaded.Width = 120
	loaded.Height = 40
	return loaded
}

// newMouseEnabledBoardWithCards creates a loaded Board with mouseEnabled=true
// and a single column containing cardCount cards plus a second column with one card.
func newMouseEnabledBoardWithCards(t *testing.T, cardCount, height int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", true, false, nil, nil, true)

	providerCards := make([]provider.Card, cardCount)
	for i := range providerCards {
		providerCards[i] = provider.Card{
			Number: i + 1,
			Title:  "Card",
			Labels: []provider.Label{{Name: "test"}},
		}
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: providerCards},
			{Title: "Column B", Cards: []provider.Card{
				{Number: 100, Title: "Other card", Labels: []provider.Label{{Name: "test"}}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = height
	return board
}

// newMouseDisabledBoard creates a loaded Board with mouseEnabled=false.
func newMouseDisabledBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	loaded, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	loaded.Width = 120
	loaded.Height = 40
	return loaded
}

// leftPanelX returns an X coordinate inside the left panel for Width=120.
// Layout: innerWidth=118, leftTotal=118*2/5=47, so left panel spans X 1..47.
func leftPanelX() int { return 10 }

// rightPanelX returns an X coordinate inside the right panel for Width=120.
// Layout: right panel starts at X=48+.
func rightPanelX() int { return 60 }

// --- Mouse wheel scroll: left panel (card list) ---

func TestMouseWheelDown_MoveCursorDown(t *testing.T) {
	b := newMouseEnabledBoard(t)
	requireColumns(t, b)

	cursorBefore := b.Columns[b.ActiveTab].Cursor
	if cursorBefore != 0 {
		t.Fatalf("precondition: cursor = %d, want 0", cursorBefore)
	}

	// Wheel down on left panel should move cursor down (like pressing j).
	b = sendKey(t, b, tea.MouseMsg{
		X:      leftPanelX(),
		Y:      5,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	cursorAfter := b.Columns[b.ActiveTab].Cursor
	if cursorAfter != 1 {
		t.Errorf("cursor = %d after wheel down, want 1", cursorAfter)
	}
}

func TestMouseWheelUp_MoveCursorUp(t *testing.T) {
	b := newMouseEnabledBoard(t)
	requireColumns(t, b)

	// Move cursor to 1 first using keyboard.
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[b.ActiveTab].Cursor != 1 {
		t.Fatalf("precondition: cursor = %d, want 1", b.Columns[b.ActiveTab].Cursor)
	}

	// Wheel up on left panel should move cursor up (like pressing k).
	b = sendKey(t, b, tea.MouseMsg{
		X:      leftPanelX(),
		Y:      5,
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})

	cursorAfter := b.Columns[b.ActiveTab].Cursor
	if cursorAfter != 0 {
		t.Errorf("cursor = %d after wheel up, want 0", cursorAfter)
	}
}

func TestMouseWheelDown_AtBottom_StaysAtBottom(t *testing.T) {
	b := newMouseEnabledBoardWithCards(t, 3, 40)
	requireColumns(t, b)

	lastCardIdx := len(b.Columns[b.ActiveTab].Cards) - 1

	// Move cursor to the last card.
	for i := 0; i < lastCardIdx; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.Columns[b.ActiveTab].Cursor != lastCardIdx {
		t.Fatalf("precondition: cursor = %d, want %d", b.Columns[b.ActiveTab].Cursor, lastCardIdx)
	}

	// Wheel down should not go past the last card.
	b = sendKey(t, b, tea.MouseMsg{
		X:      leftPanelX(),
		Y:      5,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	cursorAfter := b.Columns[b.ActiveTab].Cursor
	if cursorAfter != lastCardIdx {
		t.Errorf("cursor = %d after wheel down at bottom, want %d (should stay at last card)", cursorAfter, lastCardIdx)
	}
}

// --- Mouse wheel scroll: right panel (detail body) ---

func TestMouseWheelDown_RightPanel_ScrollsDetail(t *testing.T) {
	b := newMouseEnabledBoard(t)
	requireColumns(t, b)

	// Focus detail panel first.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	offsetBefore := b.detailScrollOffset

	// Wheel down on right panel should increment detailScrollOffset.
	b = sendKey(t, b, tea.MouseMsg{
		X:      rightPanelX(),
		Y:      5,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	// For a board with enough content, scrollOffset should increase.
	// Even if content is short and can't scroll, it should not decrease.
	if b.detailScrollOffset < offsetBefore {
		t.Errorf("detailScrollOffset = %d after wheel down on right panel, want >= %d", b.detailScrollOffset, offsetBefore)
	}
}

func TestMouseWheelUp_RightPanel_ScrollsDetail(t *testing.T) {
	b := newMouseEnabledBoard(t)
	requireColumns(t, b)

	// Focus detail panel and scroll down first via keyboard.
	b = sendKey(t, b, keyMsg("l"))
	b.detailScrollOffset = 3 // Simulate scrolled state.

	// Wheel up on right panel should decrement detailScrollOffset.
	b = sendKey(t, b, tea.MouseMsg{
		X:      rightPanelX(),
		Y:      5,
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})

	if b.detailScrollOffset >= 3 {
		t.Errorf("detailScrollOffset = %d after wheel up on right panel, want < 3", b.detailScrollOffset)
	}
}

// --- Mouse click on column tabs ---
//
// Click coordinates below are never computed from a formula that duplicates
// mouse_handlers.go's or borderTitleZones's own geometry math (per
// .claude/rules/testing.md: expected values must not come from the
// implementation under test). Instead, renderedTabZones parses the actual
// ANSI-stripped buildBorderTitle output -- the same string View() renders --
// to find where each column's label really sits.

// tabHitZone is one column's clickable cell range on the rendered
// border-title row (Y=0), in inclusive [start,end] cells.
type tabHitZone struct {
	start int
	end   int
}

// renderedTabZones renders the border title exactly as View() does (same
// buildBorderTitle/borderTitleCounts call, same b.Columns/b.ActiveTab/
// b.Width) and locates each column's "[N]" label within the ANSI-stripped
// text by finding its numeric marker, then finds where the label ends by
// scanning for the next " ─" run -- the literal text buildBorderTitle
// always places immediately after every label, whether the next thing is
// the " ─ " separator before another label or the " ───..."
// fill run before the closing corner. Returns nil if a marker isn't found
// (e.g. the ladder dropped labels entirely at this width -- rung 4).
func renderedTabZones(b Board) []tabHitZone {
	stripped := ansi.Strip(buildBorderTitle(b.Columns, b.ActiveTab, b.Width, b.borderTitleCounts()))
	zones := make([]tabHitZone, 0, len(b.Columns))
	searchFrom := 0
	for i := range b.Columns {
		marker := fmt.Sprintf("[%d]", i+1)
		idx := strings.Index(stripped[searchFrom:], marker)
		if idx < 0 {
			return nil
		}
		idx += searchFrom
		start := lipgloss.Width(stripped[:idx])

		runIdx := strings.Index(stripped[idx:], " ─")
		if runIdx < 0 {
			return nil
		}
		runIdx += idx
		end := lipgloss.Width(stripped[:runIdx]) - 1

		zones = append(zones, tabHitZone{start: start, end: end})
		searchFrom = runIdx
	}
	return zones
}

// newMouseEnabledBoardWithColumns builds a mouse-enabled, loaded Board with
// the given columns (title + card count only -- card contents don't matter
// for tab-bar rendering) at the given width.
func newMouseEnabledBoardWithColumns(t *testing.T, columns []Column, width int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", true, false, nil, nil, true)

	providerColumns := make([]provider.Column, len(columns))
	for i, col := range columns {
		cards := make([]provider.Card, len(col.Cards))
		for j := range cards {
			cards[j] = provider.Card{Number: j + 1, Title: fmt.Sprintf("Card %d", j+1), Labels: []provider.Label{{Name: "test"}}}
		}
		providerColumns[i] = provider.Column{Title: col.Title, Cards: cards}
	}

	m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: providerColumns}})
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = width
	board.Height = 40
	return board
}

// rungOfLabel classifies which truncation rung borderTitleZones chose for
// column colIdx's label: "full" (complete sanitized title, no ellipsis),
// "truncated" (partial title + "…"), or "numbers" (no title text at all,
// just "[N] (count)").
func rungOfLabel(label, sanitizedTitle string) string {
	if strings.Contains(label, "…") {
		return "truncated"
	}
	if strings.Contains(label, sanitizedTitle) {
		return "full"
	}
	return "numbers"
}

// findRungWidth scans widths (descending from a generous upper bound down
// through 0 and into negative territory) for the first one whose
// borderTitleZones() rung for column colIdx matches want ("full",
// "truncated", "numbers", or "none" for the ladder dropping labels
// entirely). This is fixture setup (picking which widths exercise which
// rung), not the click-coordinate assertions themselves, which come from
// renderedTabZones instead.
func findRungWidth(t *testing.T, columns []Column, colIdx int, sanitizedTitle, want string) int {
	t.Helper()
	for w := 200; w >= -5; w-- {
		zones := borderTitleZones(columns, w)
		if len(zones) == 0 {
			if want == "none" {
				return w
			}
			continue
		}
		if colIdx >= len(zones) {
			continue
		}
		if rungOfLabel(zones[colIdx].label, sanitizedTitle) == want {
			return w
		}
	}
	t.Fatalf("no width in [-5,200] reaches rung %q for column %d", want, colIdx)
	return 0
}

func TestMouseClickTab_SwitchesColumn(t *testing.T) {
	b := newMouseEnabledBoard(t)
	requireColumns(t, b)

	if b.ActiveTab != 0 {
		t.Fatalf("precondition: ActiveTab = %d, want 0", b.ActiveTab)
	}
	if len(b.Columns) < 2 {
		t.Fatalf("precondition: len(b.Columns) = %d, want >= 2", len(b.Columns))
	}

	zones := renderedTabZones(b)
	if len(zones) != len(b.Columns) {
		t.Fatalf("precondition: renderedTabZones returned %d zones, want %d", len(zones), len(b.Columns))
	}

	b = sendKey(t, b, tea.MouseMsg{
		X:      zones[1].start,
		Y:      0,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if b.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d after clicking column 2 tab (X=%d), want 1", b.ActiveTab, zones[1].start)
	}
}

// TestMouseClickTab_SanitizedTitleHitZones is a regression test for #501 Q2:
// handleTabClick independently rebuilds the rung-1 tab label from the raw
// (unsanitized) col.Title, which desyncs the click hit zones from the
// sanitized label buildBorderTitle actually renders. Column 0's title
// contains an embedded newline that sanitizeSingleLine flattens to a space,
// shortening its rendered label -- so the stale (unsanitized) zone
// boundaries are shifted right of where the sanitized render places them.
func TestMouseClickTab_SanitizedTitleHitZones(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", true, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Alpha\nBeta", Cards: nil},
			{Title: "Second", Cards: nil},
		},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = 120
	board.Height = 40

	if board.ActiveTab != 0 {
		t.Fatalf("precondition: ActiveTab = %d, want 0", board.ActiveTab)
	}

	zones := renderedTabZones(board)
	if len(zones) != 2 {
		t.Fatalf("precondition: renderedTabZones returned %d zones, want 2", len(zones))
	}

	t.Run("click last cell of tab 2's sanitized zone switches to tab 2", func(t *testing.T) {
		// The *first* cell of tab 2's sanitized zone would overlap the
		// stale (too-far-left) zone computed from the raw title, making
		// that case vacuous; the last cell is unambiguously past the
		// stale zone's end today.
		x := zones[1].end
		got := sendKey(t, board, tea.MouseMsg{
			X:      x,
			Y:      0,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress,
		})
		if got.ActiveTab != 1 {
			t.Errorf("ActiveTab = %d after clicking last cell of tab 2's sanitized zone (X=%d), want 1", got.ActiveTab, x)
		}
	})

	t.Run("click inside tab 1's sanitized zone stays on tab 1", func(t *testing.T) {
		x := zones[0].end
		got := sendKey(t, board, tea.MouseMsg{
			X:      x,
			Y:      0,
			Button: tea.MouseButtonLeft,
			Action: tea.MouseActionPress,
		})
		if got.ActiveTab != 0 {
			t.Errorf("ActiveTab = %d after clicking inside tab 1's sanitized zone (X=%d), want 0", got.ActiveTab, x)
		}
	})
}

// cjkEmojiTabFixtureColumns returns a small column set with CJK+emoji
// titles, used to exercise the truncation ladder's three text-bearing rungs
// (full / cell-truncated / numbers-only) plus rung 4 (no labels at all).
func cjkEmojiTabFixtureColumns() []Column {
	return []Column{
		// Column 0 is deliberately as long as column 1 (the click target) so
		// that at the "truncated" rung, BOTH columns' labels shrink from
		// their untruncated width -- this shifts column 1's real start
		// position leftward relative to what a hit-tester assuming full
		// (untruncated) column-0 width would compute, producing a genuine
		// zone mismatch rather than merely a wider-than-real zone that
		// still happens to contain the real one.
		{Title: "起始阶段总览概述文字说明", Cards: make([]Card, 2)},
		{Title: "进行中\U0001f680开发阶段追踪细节说明", Cards: make([]Card, 3)},
		{Title: "完成审核阶段", Cards: make([]Card, 2)},
	}
}

// TestMouseClickTab_RungMatrix_CJKEmojiTitles exercises all three
// text-bearing ladder rungs against a CJK+emoji fixture: clicking the first
// or last cell of column 2's rendered label always selects column 2,
// regardless of which rung the ladder chose, and clicking one cell before
// the label's first cell never does.
func TestMouseClickTab_RungMatrix_CJKEmojiTitles(t *testing.T) {
	columns := cjkEmojiTabFixtureColumns()
	sanitizedTitle2 := sanitizeSingleLine(columns[1].Title)

	rungs := []string{"full", "truncated", "numbers"}
	for _, rung := range rungs {
		rung := rung
		t.Run(rung, func(t *testing.T) {
			width := findRungWidth(t, columns, 1, sanitizedTitle2, rung)
			board := newMouseEnabledBoardWithColumns(t, columns, width)
			zones := renderedTabZones(board)
			if len(zones) != len(columns) {
				t.Fatalf("precondition: renderedTabZones returned %d zones, want %d at width %d (rung %s)", len(zones), len(columns), width, rung)
			}
			col2 := zones[1]

			t.Run("first cell selects column 2", func(t *testing.T) {
				b := board
				b.ActiveTab = 0
				got := sendKey(t, b, tea.MouseMsg{X: col2.start, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				if got.ActiveTab != 1 {
					t.Errorf("ActiveTab = %d after clicking first cell (X=%d) of column 2's %s-rung label, want 1", got.ActiveTab, col2.start, rung)
				}
			})

			t.Run("last cell selects column 2", func(t *testing.T) {
				b := board
				b.ActiveTab = 0
				got := sendKey(t, b, tea.MouseMsg{X: col2.end, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				if got.ActiveTab != 1 {
					t.Errorf("ActiveTab = %d after clicking last cell (X=%d) of column 2's %s-rung label, want 1", got.ActiveTab, col2.end, rung)
				}
			})

			t.Run("one cell before first cell does not select column 2", func(t *testing.T) {
				b := board
				b.ActiveTab = 0
				got := sendKey(t, b, tea.MouseMsg{X: col2.start - 1, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
				if got.ActiveTab == 1 {
					t.Errorf("ActiveTab = 1 after clicking one cell before column 2's %s-rung label (X=%d), want != 1", rung, col2.start-1)
				}
			})
		})
	}
}

// TestMouseClickTab_Rung4_NoLabels_NoOp covers the ladder's final rung: at a
// width where even the numbers-only rung doesn't fit, borderTitleZones
// returns zero zones and the tab bar renders with no clickable labels at
// all. A click anywhere on Y=0 must be a no-op: unchanged ActiveTab, nil
// cmd.
func TestMouseClickTab_Rung4_NoLabels_NoOp(t *testing.T) {
	columns := cjkEmojiTabFixtureColumns()
	sanitizedTitle2 := sanitizeSingleLine(columns[1].Title)
	width := findRungWidth(t, columns, 1, sanitizedTitle2, "none")

	if got := len(borderTitleZones(columns, width)); got != 0 {
		t.Fatalf("precondition: len(borderTitleZones(columns, %d)) = %d, want 0 (rung 4: no labels fit)", width, got)
	}

	board := newMouseEnabledBoardWithColumns(t, columns, width)
	board.ActiveTab = 0

	m, cmd := board.Update(tea.MouseMsg{X: 5, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	got, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil (rung 4 click is a no-op)", cmd)
	}
	if got.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d after clicking Y=0 at rung 4 (no labels), want 0 (unchanged)", got.ActiveTab)
	}
}

// TestMouseClickTab_SearchActive_LastCellSelectsColumn is a regression case
// for the search-active count-suffix shape: when a search query is active,
// borderTitleCounts overrides only the active column's count to
// "(f/N) ●", which is longer than the plain "(N)" every other column
// (and today's handleTabClick) still assumes -- shifting every zone after
// the active column to the right of where the old handler thinks it is.
func TestMouseClickTab_SearchActive_LastCellSelectsColumn(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	b.mouseEnabled = true
	b.ActiveTab = 0
	b.searchQuery = "bug"

	if b.mode != normalMode {
		t.Fatalf("precondition: mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if b.activeFilterType != filterTypeNone {
		t.Fatalf("precondition: activeFilterType = %d, want filterTypeNone", b.activeFilterType)
	}
	if len(b.Columns) < 2 {
		t.Fatalf("precondition: len(b.Columns) = %d, want >= 2", len(b.Columns))
	}

	zones := renderedTabZones(b)
	if len(zones) != len(b.Columns) {
		t.Fatalf("precondition: renderedTabZones returned %d zones, want %d", len(zones), len(b.Columns))
	}
	col2 := zones[1]

	got := sendKey(t, b, tea.MouseMsg{X: col2.end, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if got.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d after clicking last cell (X=%d) of column 2's search-active rendered label, want 1", got.ActiveTab, col2.end)
	}
}

// TestMouseClickTab_GlobalFilterActive_LastCellSelectsColumn mirrors the
// search-active case for the other borderTitleCounts branch: a global
// filter with no search query gives every column its own "(f/N) ●"
// count via filteredCardsForColumn, which can differ in width from column
// to column.
func TestMouseClickTab_GlobalFilterActive_LastCellSelectsColumn(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	b.mouseEnabled = true
	b.ActiveTab = 0
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"
	b.searchQuery = ""

	if len(b.Columns) < 2 {
		t.Fatalf("precondition: len(b.Columns) = %d, want >= 2", len(b.Columns))
	}

	zones := renderedTabZones(b)
	if len(zones) != len(b.Columns) {
		t.Fatalf("precondition: renderedTabZones returned %d zones, want %d", len(zones), len(b.Columns))
	}
	col2 := zones[1]

	got := sendKey(t, b, tea.MouseMsg{X: col2.end, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if got.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d after clicking last cell (X=%d) of column 2's filter-active rendered label, want 1", got.ActiveTab, col2.end)
	}
}

// TestMouseClickTab_SearchAndFilterBothActive_LastCellSelectsColumn covers
// the combined state: search wins over the global filter (per
// borderTitleCounts), so the rendered shape matches the search-only case,
// but the underlying state exercises both fields together.
func TestMouseClickTab_SearchAndFilterBothActive_LastCellSelectsColumn(t *testing.T) {
	b := newBoardWithFilterableCards(t)
	b.mouseEnabled = true
	b.ActiveTab = 0
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"
	b.searchQuery = "Specific"

	if len(b.Columns) < 2 {
		t.Fatalf("precondition: len(b.Columns) = %d, want >= 2", len(b.Columns))
	}

	zones := renderedTabZones(b)
	if len(zones) != len(b.Columns) {
		t.Fatalf("precondition: renderedTabZones returned %d zones, want %d", len(zones), len(b.Columns))
	}
	col2 := zones[1]

	got := sendKey(t, b, tea.MouseMsg{X: col2.end, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if got.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d after clicking last cell (X=%d) of column 2's search+filter-active rendered label, want 1", got.ActiveTab, col2.end)
	}
}

// TestMouseClickTab_ZeroColumnsWithSearchQuery_NoPanicNoStateChange covers
// the empty-board edge case combined with an active search query: neither
// handleTabClick's numCols==0 guard nor borderTitleCounts's
// fc[b.ActiveTab]=... assignment (with ActiveTab==0 and zero columns) may
// panic.
func TestMouseClickTab_ZeroColumnsWithSearchQuery_NoPanicNoStateChange(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", true, false, nil, nil, true)

	m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: []provider.Column{}}})
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = 120
	board.Height = 40
	board.searchQuery = "anything"

	if len(board.Columns) != 0 {
		t.Fatalf("precondition: len(board.Columns) = %d, want 0", len(board.Columns))
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Update panicked with zero columns and an active search query: %v", r)
		}
	}()

	m2, cmd := board.Update(tea.MouseMsg{X: 5, Y: 0, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	got, ok := m2.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m2)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if got.ActiveTab != board.ActiveTab {
		t.Errorf("ActiveTab changed from %d to %d after click with zero columns", board.ActiveTab, got.ActiveTab)
	}
	if len(got.Columns) != 0 {
		t.Errorf("Columns len = %d after click with zero columns, want 0 (unchanged)", len(got.Columns))
	}
}

// --- Mouse click on cards ---

func TestMouseClickCard_MovesCursor(t *testing.T) {
	b := newMouseEnabledBoardWithCards(t, 5, 40)
	requireColumns(t, b)

	if b.Columns[b.ActiveTab].Cursor != 0 {
		t.Fatalf("precondition: cursor = %d, want 0", b.Columns[b.ActiveTab].Cursor)
	}

	// Click on the Y position of the third card (index 2).
	// Row layout: Y=0 outer border title, Y=1 panel top border, Y=2+ card content.
	// Each card with a short title at Width=120 occupies 1 line.
	// Card 0 is at Y=2, card 1 at Y=3, card 2 at Y=4.
	targetCardIdx := 2
	targetY := 2 + targetCardIdx

	b = sendKey(t, b, tea.MouseMsg{
		X:      leftPanelX(),
		Y:      targetY,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	cursorAfter := b.Columns[b.ActiveTab].Cursor
	if cursorAfter != targetCardIdx {
		t.Errorf("cursor = %d after clicking card at Y=%d, want %d", cursorAfter, targetY, targetCardIdx)
	}
}

func TestMouseClickCard_DoesNotOpenDetail(t *testing.T) {
	b := newMouseEnabledBoardWithCards(t, 3, 40)
	requireColumns(t, b)

	// Click on a card should move cursor but NOT open detail panel.
	b = sendKey(t, b, tea.MouseMsg{
		X:      leftPanelX(),
		Y:      3, // card at index 1
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if b.detailFocused {
		t.Error("clicking a card should not set detailFocused = true")
	}
}

// --- Guards ---

func TestMouseDisabled_IgnoresEvents(t *testing.T) {
	b := newMouseDisabledBoard(t)
	requireColumns(t, b)

	cursorBefore := b.Columns[b.ActiveTab].Cursor
	tabBefore := b.ActiveTab

	// Send a wheel-down event. With mouseEnabled=false, it should be a no-op.
	b = sendKey(t, b, tea.MouseMsg{
		X:      leftPanelX(),
		Y:      5,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	if b.Columns[b.ActiveTab].Cursor != cursorBefore {
		t.Errorf("cursor = %d after mouse event with mouse disabled, want %d (unchanged)", b.Columns[b.ActiveTab].Cursor, cursorBefore)
	}
	if b.ActiveTab != tabBefore {
		t.Errorf("ActiveTab = %d after mouse event with mouse disabled, want %d (unchanged)", b.ActiveTab, tabBefore)
	}
}

func TestMouseNonNormalMode_IgnoresEvents(t *testing.T) {
	b := newMouseEnabledBoard(t)
	requireColumns(t, b)

	// Enter create mode.
	b = sendKey(t, b, keyMsg("n"))
	if b.mode != createMode {
		t.Fatalf("precondition: mode = %d, want createMode (%d)", b.mode, createMode)
	}

	cursorBefore := b.Columns[b.ActiveTab].Cursor

	// Send a wheel-down event. In createMode, mouse events should be ignored.
	b = sendKey(t, b, tea.MouseMsg{
		X:      leftPanelX(),
		Y:      5,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	if b.Columns[b.ActiveTab].Cursor != cursorBefore {
		t.Errorf("cursor = %d after mouse event in createMode, want %d (unchanged)", b.Columns[b.ActiveTab].Cursor, cursorBefore)
	}
	// Mode should remain createMode.
	if b.mode != createMode {
		t.Errorf("mode = %d after mouse event in createMode, want %d (unchanged)", b.mode, createMode)
	}
}

func TestMouseWheelDown_EmptyColumn_NoOp(t *testing.T) {
	// Create a board with an empty first column and a non-empty second column.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", true, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Empty Column", Cards: []provider.Card{}},
			{Title: "Has Cards", Cards: []provider.Card{
				{Number: 1, Title: "A card", Labels: []provider.Label{{Name: "test"}}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40

	// Wheel down on an empty column should not panic or change state.
	board = sendKey(t, board, tea.MouseMsg{
		X:      leftPanelX(),
		Y:      5,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	if board.Columns[board.ActiveTab].Cursor != 0 {
		t.Errorf("cursor = %d after wheel down on empty column, want 0", board.Columns[board.ActiveTab].Cursor)
	}
}

// --- Mouse wheel scroll: modal surfaces (#476) ---
//
// Extends wheel support beyond normalMode to every modal that has a cursor
// list or a scrollable viewport. Wheel-driven cursor movement clamps at the
// first/last item -- it deliberately does NOT wrap, unlike each modal's
// j/k handler (which wraps via moveCursor).

// newMouseEnabledPRListBoard opens prListMode (3 linked-PR entries across
// the shared PR fixture: card 2 has 1, card 3 has 2) on a mouse-enabled board.
func newMouseEnabledPRListBoard(t *testing.T) Board {
	t.Helper()
	b, _ := newBoardWithPRsAndExecutor(t)
	b.mouseEnabled = true
	m, _ := b.Update(keyMsg("P"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update(P) returned %T, want Board", m)
	}
	return board
}

// newMouseEnabledAgentListBoard opens agentListMode (3 window entries from
// threeWindows) on a mouse-enabled board.
func newMouseEnabledAgentListBoard(t *testing.T) Board {
	t.Helper()
	fe := &action.FakeExecutor{}
	b := newAgentListBoard(t, fe, threeWindows())
	b.mouseEnabled = true
	m, _ := b.Update(keyMsg("A"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update(A) returned %T, want Board", m)
	}
	return board
}

// newMouseEnabledAssignBoard opens assignMode (authenticated user + 3
// collaborators) on a mouse-enabled board.
func newMouseEnabledAssignBoard(t *testing.T) Board {
	t.Helper()
	b := newBoardWithCollaborators(t)
	b.mouseEnabled = true
	m, _ := b.Update(keyMsg("a"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update(a) returned %T, want Board", m)
	}
	return board
}

// newMouseEnabledGitPanelBoard opens gitPanelMode (the 6 built-in git
// shortcuts) on a mouse-enabled board.
func newMouseEnabledGitPanelBoard(t *testing.T) Board {
	t.Helper()
	b, _ := newGitPanelTestBoard(t, nil, nil)
	b.mouseEnabled = true
	m, _ := b.Update(keyMsg("G"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update(G) returned %T, want Board", m)
	}
	return board
}

// newMouseEnabledPRPickerBoard navigates to the "Two PRs" fixture card and
// opens prPickerMode (2 LinkedPRs) on a mouse-enabled board.
func newMouseEnabledPRPickerBoard(t *testing.T) Board {
	t.Helper()
	b, _ := newBoardWithPRsAndExecutor(t)
	b.mouseEnabled = true
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	m, _ := b.Update(keyMsg("p"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update(p) returned %T, want Board", m)
	}
	return board
}

// newMouseEnabledFilterBoard opens filterMode on a mouse-enabled board using
// the labels+assignees fixture (two sections separated by a header row, so
// the header-skip clamp behavior is exercisable).
func newMouseEnabledFilterBoard(t *testing.T) Board {
	t.Helper()
	b := newBoardWithLabelsAndAssignees(t)
	b.mouseEnabled = true
	m, _ := b.Update(keyMsg("f"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update(f) returned %T, want Board", m)
	}
	return board
}

// newMouseEnabledHelpBoard opens helpMode on a mouse-enabled board sized to
// the given height (a small height makes help content overflow, exercising
// the max-offset clamp).
func newMouseEnabledHelpBoard(t *testing.T, height int) Board {
	t.Helper()
	b := newLoadedTestBoard(t)
	b.mouseEnabled = true
	b.Width = 120
	b.Height = height
	m, _ := b.Update(keyMsg("?"))
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update(?) returned %T, want Board", m)
	}
	return board
}

// wheelDownMsg / wheelUpMsg build wheel tea.MouseMsg events. The X/Y values
// are irrelevant to modal routing (only normalMode's left/right panel split
// cares about X) but are set to realistic in-modal coordinates.
func wheelDownMsg() tea.MouseMsg {
	return tea.MouseMsg{X: leftPanelX(), Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress}
}

func wheelUpMsg() tea.MouseMsg {
	return tea.MouseMsg{X: leftPanelX(), Y: 5, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress}
}

// --- PR list modal ---

func TestMouseWheelDown_PRListMode_MovesCursorDown(t *testing.T) {
	b := newMouseEnabledPRListBoard(t)
	if b.prList.cursor != 0 {
		t.Fatalf("precondition: prList.cursor = %d, want 0", b.prList.cursor)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil (wheel cursor move is synchronous)", cmd)
	}
	if board.prList.cursor != 1 {
		t.Errorf("prList.cursor = %d after wheel down, want 1", board.prList.cursor)
	}
}

func TestMouseWheelUp_PRListMode_AtTop_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledPRListBoard(t)
	if b.prList.cursor != 0 {
		t.Fatalf("precondition: prList.cursor = %d, want 0", b.prList.cursor)
	}

	m, cmd := b.Update(wheelUpMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.prList.cursor != 0 {
		t.Errorf("prList.cursor = %d after wheel up at top, want 0 (no wrap)", board.prList.cursor)
	}
}

func TestMouseWheelDown_PRListMode_AtBottom_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledPRListBoard(t)
	lastIndex := len(b.prList.entries) - 1
	for i := 0; i < lastIndex; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.prList.cursor != lastIndex {
		t.Fatalf("precondition: prList.cursor = %d, want %d", b.prList.cursor, lastIndex)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.prList.cursor != lastIndex {
		t.Errorf("prList.cursor = %d after wheel down at bottom, want %d (no wrap)", board.prList.cursor, lastIndex)
	}
}

// --- Agents list modal ---

func TestMouseWheelDown_AgentListMode_MovesCursorDown(t *testing.T) {
	b := newMouseEnabledAgentListBoard(t)
	if b.agentList.cursor != 0 {
		t.Fatalf("precondition: agentList.cursor = %d, want 0", b.agentList.cursor)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.agentList.cursor != 1 {
		t.Errorf("agentList.cursor = %d after wheel down, want 1", board.agentList.cursor)
	}
}

func TestMouseWheelUp_AgentListMode_AtTop_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledAgentListBoard(t)
	if b.agentList.cursor != 0 {
		t.Fatalf("precondition: agentList.cursor = %d, want 0", b.agentList.cursor)
	}

	m, cmd := b.Update(wheelUpMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.agentList.cursor != 0 {
		t.Errorf("agentList.cursor = %d after wheel up at top, want 0 (no wrap)", board.agentList.cursor)
	}
}

func TestMouseWheelDown_AgentListMode_AtBottom_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledAgentListBoard(t)
	lastIndex := len(b.agentListEntries()) - 1
	for i := 0; i < lastIndex; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.agentList.cursor != lastIndex {
		t.Fatalf("precondition: agentList.cursor = %d, want %d", b.agentList.cursor, lastIndex)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.agentList.cursor != lastIndex {
		t.Errorf("agentList.cursor = %d after wheel down at bottom, want %d (no wrap)", board.agentList.cursor, lastIndex)
	}
}

// --- Assign picker modal ---

func TestMouseWheelDown_AssignMode_MovesCursorDown(t *testing.T) {
	b := newMouseEnabledAssignBoard(t)
	if b.assign.cursor != 0 {
		t.Fatalf("precondition: assign.cursor = %d, want 0", b.assign.cursor)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.assign.cursor != 1 {
		t.Errorf("assign.cursor = %d after wheel down, want 1", board.assign.cursor)
	}
}

func TestMouseWheelUp_AssignMode_AtTop_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledAssignBoard(t)
	if b.assign.cursor != 0 {
		t.Fatalf("precondition: assign.cursor = %d, want 0", b.assign.cursor)
	}

	m, cmd := b.Update(wheelUpMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.assign.cursor != 0 {
		t.Errorf("assign.cursor = %d after wheel up at top, want 0 (no wrap)", board.assign.cursor)
	}
}

func TestMouseWheelDown_AssignMode_AtBottom_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledAssignBoard(t)
	lastIndex := len(b.assign.items) - 1
	for i := 0; i < lastIndex; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.assign.cursor != lastIndex {
		t.Fatalf("precondition: assign.cursor = %d, want %d", b.assign.cursor, lastIndex)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.assign.cursor != lastIndex {
		t.Errorf("assign.cursor = %d after wheel down at bottom, want %d (no wrap)", board.assign.cursor, lastIndex)
	}
}

// --- Git menu modal ---

func TestMouseWheelDown_GitPanelMode_MovesCursorDown(t *testing.T) {
	b := newMouseEnabledGitPanelBoard(t)
	if b.gitPanel.cursor != 0 {
		t.Fatalf("precondition: gitPanel.cursor = %d, want 0", b.gitPanel.cursor)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.gitPanel.cursor != 1 {
		t.Errorf("gitPanel.cursor = %d after wheel down, want 1", board.gitPanel.cursor)
	}
}

func TestMouseWheelUp_GitPanelMode_AtTop_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledGitPanelBoard(t)
	if b.gitPanel.cursor != 0 {
		t.Fatalf("precondition: gitPanel.cursor = %d, want 0", b.gitPanel.cursor)
	}

	m, cmd := b.Update(wheelUpMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.gitPanel.cursor != 0 {
		t.Errorf("gitPanel.cursor = %d after wheel up at top, want 0 (no wrap)", board.gitPanel.cursor)
	}
}

func TestMouseWheelDown_GitPanelMode_AtBottom_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledGitPanelBoard(t)
	lastIndex := len(b.gitPanel.items) - 1
	for i := 0; i < lastIndex; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.gitPanel.cursor != lastIndex {
		t.Fatalf("precondition: gitPanel.cursor = %d, want %d", b.gitPanel.cursor, lastIndex)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.gitPanel.cursor != lastIndex {
		t.Errorf("gitPanel.cursor = %d after wheel down at bottom, want %d (no wrap)", board.gitPanel.cursor, lastIndex)
	}
}

// --- PR picker modal ---
//
// Unlike the other list modals, prPickerMode's keyboard navigation is
// Left/Right (not j/k), driving prPickerIndex with wrapping arithmetic. The
// wheel path mirrors that same field but clamps instead of wrapping.

func TestMouseWheelDown_PRPickerMode_MovesIndexForward(t *testing.T) {
	b := newMouseEnabledPRPickerBoard(t)
	if b.prPickerIndex != 0 {
		t.Fatalf("precondition: prPickerIndex = %d, want 0", b.prPickerIndex)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.prPickerIndex != 1 {
		t.Errorf("prPickerIndex = %d after wheel down, want 1", board.prPickerIndex)
	}
}

func TestMouseWheelUp_PRPickerMode_AtFirst_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledPRPickerBoard(t)
	if b.prPickerIndex != 0 {
		t.Fatalf("precondition: prPickerIndex = %d, want 0", b.prPickerIndex)
	}

	m, cmd := b.Update(wheelUpMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.prPickerIndex != 0 {
		t.Errorf("prPickerIndex = %d after wheel up at first, want 0 (no wrap)", board.prPickerIndex)
	}
}

func TestMouseWheelDown_PRPickerMode_AtLast_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledPRPickerBoard(t)
	card := b.selectedCard()
	lastIndex := len(card.LinkedPRs) - 1
	for i := 0; i < lastIndex; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyRight))
	}
	if b.prPickerIndex != lastIndex {
		t.Fatalf("precondition: prPickerIndex = %d, want %d", b.prPickerIndex, lastIndex)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.prPickerIndex != lastIndex {
		t.Errorf("prPickerIndex = %d after wheel down at last, want %d (no wrap)", board.prPickerIndex, lastIndex)
	}
}

// TestMouseWheelDown_PRPickerMode_LinkedPRsShrinkWhileOpen_ClampsIndex mirrors
// pr_picker_test.go's TestPRPicker_LinkedPRsShrinkWhileOpen_ClampsIndexAndActsOnCorrectPR
// but exercises the wheel path instead of Enter (#476 regression: the wheel
// handler used to call clampCursor's one-step move directly on an
// already-out-of-range index, which cannot self-heal it, and
// viewPRPickerModal indexes LinkedPRs with no bounds check).
func TestMouseWheelDown_PRPickerMode_LinkedPRsShrinkWhileOpen_ClampsIndex(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b.mouseEnabled = true

	// Navigate to card 3 ("Two PRs": #20, #21) and enter the picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}

	// Move to the second (last) PR (index 1).
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.prPickerIndex != 1 {
		t.Fatalf("test setup: expected prPickerIndex 1, got %d", b.prPickerIndex)
	}

	// Simulate an in-flight background refresh completing while the picker
	// is still open, shrinking the same card's LinkedPRs from 2 to 1.
	// b.prPickerIndex (still 1) now points past the end of the new slice.
	b.refreshing = true
	shrunkPR := provider.LinkedPR{Number: 20, Title: "feat: first PR", URL: "https://github.com/owner/repo/pull/20", Branch: "feature/first-pr"}
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "No PRs", Labels: []provider.Label{{Name: "bug"}}},
				{Number: 2, Title: "One PR", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "feat: one PR", URL: "https://github.com/owner/repo/pull/10", Branch: "feature/one-pr"},
				}},
				{Number: 3, Title: "Two PRs", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{shrunkPR}},
			}},
		},
	}}

	m, cmd := b.Update(msg)
	b = m.(Board)
	execCmds(cmd)

	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected mode to remain prPickerMode after in-flight refresh, got %d", b.mode)
	}
	if got := len(b.selectedCard().LinkedPRs); got != 1 {
		t.Fatalf("test setup: expected shrunk card to have 1 LinkedPR, got %d", got)
	}

	// Wheel down without re-navigating: prPickerIndex is still 1, past the
	// new length of 1. This must not panic (viewPRPickerModal indexes
	// LinkedPRs by prPickerIndex with no bounds check) and must clamp the
	// index back into range.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Update/View panicked after LinkedPRs shrank while the picker was open: %v", r)
		}
	}()
	m, cmd = b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	execCmds(cmd)
	_ = board.View()

	if board.prPickerIndex != 0 {
		t.Errorf("prPickerIndex = %d after wheel down on shrunk list, want 0 (clamped into range)", board.prPickerIndex)
	}
	if board.mode != prPickerMode {
		t.Errorf("mode = %d after wheel down on shrunk-but-nonempty list, want prPickerMode (%d)", board.mode, prPickerMode)
	}
}

// TestMouseWheelDown_PRPickerMode_LinkedPRsShrinkToZeroWhileOpen_ClosesPicker
// mirrors handlePRPickerModeKey's prCount==0 auto-close branch, but via the
// wheel path (#476 should-fix: handlePRPickerWheel previously only called
// clampCursor, which never closes the picker).
func TestMouseWheelDown_PRPickerMode_LinkedPRsShrinkToZeroWhileOpen_ClosesPicker(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b.mouseEnabled = true

	// Navigate to card 3 ("Two PRs": #20, #21) and enter the picker.
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("p"))
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}

	// Simulate an in-flight refresh that removes the card's PRs entirely
	// while the picker is still open.
	b.refreshing = true
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "No PRs", Labels: []provider.Label{{Name: "bug"}}},
				{Number: 2, Title: "One PR", Labels: []provider.Label{{Name: "feature"}}},
				{Number: 3, Title: "Two PRs", Labels: []provider.Label{{Name: "feature"}}},
			}},
		},
	}}
	m, cmd := b.Update(msg)
	b = m.(Board)
	execCmds(cmd)

	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected mode to remain prPickerMode after in-flight refresh, got %d", b.mode)
	}
	if got := len(b.selectedCard().LinkedPRs); got != 0 {
		t.Fatalf("test setup: expected shrunk card to have 0 LinkedPRs, got %d", got)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Update/View panicked after LinkedPRs shrank to zero while the picker was open: %v", r)
		}
	}()
	m, cmd = b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	execCmds(cmd)
	_ = board.View()

	if board.mode != normalMode {
		t.Errorf("mode = %d after wheel down with 0 LinkedPRs, want normalMode (%d) (auto-close mirrors handlePRPickerModeKey)", board.mode, normalMode)
	}
	if board.pendingPRAction != nil {
		t.Errorf("pendingPRAction = %v after auto-close, want nil", board.pendingPRAction)
	}
}

// --- Filter picker modal ---
//
// The filter picker mixes selectable items with non-selectable header rows
// (collectFilterItems groups Labels/Assignees/Milestones under headers).
// The wheel path must skip headers the same way filterMove does, but clamp
// at the first/last selectable item instead of wrapping.

func TestMouseWheelDown_FilterMode_SkipsHeaderRow(t *testing.T) {
	b := newMouseEnabledFilterBoard(t)

	// Locate a selectable item immediately followed by a header row
	// dynamically (rather than hardcoding an index) -- collectFilterItems'
	// exact section ordering is a fixture implementation detail, not
	// something this test should assume.
	beforeHeader, afterHeader := -1, -1
	for i := 0; i < len(b.filterItems)-2; i++ {
		if !b.filterItems[i].isHeader && b.filterItems[i+1].isHeader && !b.filterItems[i+2].isHeader {
			beforeHeader = i
			afterHeader = i + 2
			break
		}
	}
	if beforeHeader == -1 {
		t.Fatal("test setup: fixture must contain at least two filter sections separated by a header")
	}
	b.filterCursor = beforeHeader

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.filterCursor != afterHeader {
		t.Errorf("filterCursor = %d after wheel down past a header, want %d (header skipped)", board.filterCursor, afterHeader)
	}
	if board.filterItems[board.filterCursor].isHeader {
		t.Errorf("filterCursor landed on a header row (index %d)", board.filterCursor)
	}
}

func TestMouseWheelUp_FilterMode_AtFirstSelectable_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledFilterBoard(t)
	firstSelectable := -1
	for i, item := range b.filterItems {
		if !item.isHeader {
			firstSelectable = i
			break
		}
	}
	if firstSelectable == -1 {
		t.Fatal("test setup: fixture must contain at least one selectable filter item")
	}
	b.filterCursor = firstSelectable

	m, cmd := b.Update(wheelUpMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.filterCursor != firstSelectable {
		t.Errorf("filterCursor = %d after wheel up at first selectable, want %d (no wrap)", board.filterCursor, firstSelectable)
	}
}

func TestMouseWheelDown_FilterMode_AtLastSelectable_ClampsNoWrap(t *testing.T) {
	b := newMouseEnabledFilterBoard(t)
	lastSelectable := -1
	for i, item := range b.filterItems {
		if !item.isHeader {
			lastSelectable = i
		}
	}
	if lastSelectable == -1 {
		t.Fatal("test setup: fixture must contain at least one selectable filter item")
	}
	b.filterCursor = lastSelectable

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.filterCursor != lastSelectable {
		t.Errorf("filterCursor = %d after wheel down at last selectable, want %d (no wrap)", board.filterCursor, lastSelectable)
	}
}

// --- Help modal (viewport scroll, not a cursor list) ---

func TestMouseWheelDown_HelpMode_ScrollsViewportDown(t *testing.T) {
	b := newMouseEnabledHelpBoard(t, 40)
	offsetBefore := b.helpScrollOffset

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.helpScrollOffset <= offsetBefore {
		t.Errorf("helpScrollOffset = %d after wheel down, want > %d", board.helpScrollOffset, offsetBefore)
	}
}

func TestMouseWheelUp_HelpMode_ScrollsViewportUp(t *testing.T) {
	b := newMouseEnabledHelpBoard(t, 40)
	b = sendKey(t, b, wheelDownMsg())
	b = sendKey(t, b, wheelDownMsg())
	offsetAfterDown := b.helpScrollOffset

	m, cmd := b.Update(wheelUpMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.helpScrollOffset >= offsetAfterDown {
		t.Errorf("helpScrollOffset = %d after wheel up, want < %d", board.helpScrollOffset, offsetAfterDown)
	}
}

func TestMouseWheelUp_HelpMode_ClampsAtZero(t *testing.T) {
	b := newMouseEnabledHelpBoard(t, 40)
	if b.helpScrollOffset != 0 {
		t.Fatalf("precondition: helpScrollOffset = %d, want 0", b.helpScrollOffset)
	}

	m, cmd := b.Update(wheelUpMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.helpScrollOffset != 0 {
		t.Errorf("helpScrollOffset = %d after wheel up at 0, want 0", board.helpScrollOffset)
	}
}

func TestMouseWheelDown_HelpMode_ClampsAtMaxOffset(t *testing.T) {
	b := newMouseEnabledHelpBoard(t, 20) // Small height so help content overflows.
	for i := 0; i < 200; i++ {
		b = sendKey(t, b, wheelDownMsg())
	}
	maxOffset := b.helpMaxScrollOffset()
	if b.helpScrollOffset != maxOffset {
		t.Fatalf("precondition: helpScrollOffset = %d after excessive wheel down, want %d (maxOffset)", b.helpScrollOffset, maxOffset)
	}

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.helpScrollOffset != maxOffset {
		t.Errorf("helpScrollOffset = %d after wheel down at max, want %d (clamped)", board.helpScrollOffset, maxOffset)
	}
}

// --- mouseEnabled=false guard, across every modal surface ---

func TestMouseWheelDown_MouseDisabled_ListModals_NoOp(t *testing.T) {
	tests := []struct {
		name   string
		build  func(t *testing.T) Board
		cursor func(b Board) int
	}{
		{"prListMode", newMouseEnabledPRListBoard, func(b Board) int { return b.prList.cursor }},
		{"agentListMode", newMouseEnabledAgentListBoard, func(b Board) int { return b.agentList.cursor }},
		{"assignMode", newMouseEnabledAssignBoard, func(b Board) int { return b.assign.cursor }},
		{"gitPanelMode", newMouseEnabledGitPanelBoard, func(b Board) int { return b.gitPanel.cursor }},
		{"prPickerMode", newMouseEnabledPRPickerBoard, func(b Board) int { return b.prPickerIndex }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.build(t)
			b.mouseEnabled = false
			cursorBefore := tc.cursor(b)

			m, cmd := b.Update(wheelDownMsg())
			board, ok := m.(Board)
			if !ok {
				t.Fatalf("Update returned %T, want Board", m)
			}
			if cmd != nil {
				t.Errorf("cmd = %v, want nil", cmd)
			}
			if tc.cursor(board) != cursorBefore {
				t.Errorf("cursor = %d after wheel down with mouseEnabled=false, want %d (unchanged)", tc.cursor(board), cursorBefore)
			}
		})
	}
}

func TestMouseWheelDown_FilterMode_MouseDisabled_NoOp(t *testing.T) {
	b := newMouseEnabledFilterBoard(t)
	b.mouseEnabled = false
	cursorBefore := b.filterCursor

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.filterCursor != cursorBefore {
		t.Errorf("filterCursor = %d after wheel down with mouseEnabled=false, want %d (unchanged)", board.filterCursor, cursorBefore)
	}
}

func TestMouseWheelDown_HelpMode_MouseDisabled_NoOp(t *testing.T) {
	b := newMouseEnabledHelpBoard(t, 40)
	b.mouseEnabled = false
	offsetBefore := b.helpScrollOffset

	m, cmd := b.Update(wheelDownMsg())
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Errorf("cmd = %v, want nil", cmd)
	}
	if board.helpScrollOffset != offsetBefore {
		t.Errorf("helpScrollOffset = %d after wheel down with mouseEnabled=false, want %d (unchanged)", board.helpScrollOffset, offsetBefore)
	}
}

// --- Modes without scrollable content stay wheel no-ops ---

func TestMouseWheelDown_NoScrollModes_RemainNoOp(t *testing.T) {
	modes := []boardMode{
		createMode, configMode, searchMode, deleteMode,
		closeConfirmMode, labelConfirmMode, commentMode, dispatchMode,
	}

	for _, md := range modes {
		t.Run(fmt.Sprintf("mode=%d", md), func(t *testing.T) {
			b := newMouseEnabledBoard(t)
			b.mode = md

			m, cmd := b.Update(wheelDownMsg())
			board, ok := m.(Board)
			if !ok {
				t.Fatalf("Update returned %T, want Board", m)
			}
			if cmd != nil {
				t.Errorf("cmd = %v, want nil", cmd)
			}
			if board.mode != md {
				t.Errorf("mode = %d after wheel down, want %d (unchanged)", board.mode, md)
			}
		})
	}
}
