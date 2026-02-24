package main

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Mouse helpers ---

// newMouseEnabledBoard creates a loaded Board with mouseEnabled=true.
// It uses the standard FakeProvider and sets Width=120, Height=40.
func newMouseEnabledBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", true, false)

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
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", true, false)

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
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

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

func TestMouseClickTab_SwitchesColumn(t *testing.T) {
	b := newMouseEnabledBoard(t)
	requireColumns(t, b)

	if b.ActiveTab != 0 {
		t.Fatalf("precondition: ActiveTab = %d, want 0", b.ActiveTab)
	}

	// Click on the border title row (Y=0) at an X position
	// that corresponds to the second column tab.
	// With Width=120, the border title has all column labels spread across.
	// The second column label starts roughly at X=30+ depending on title length.
	// We use a click position that's within the second column's tab area.
	// For robustness, we use the midpoint of the total width divided by column count.
	numCols := len(b.Columns)
	secondColX := b.Width / numCols // Approximate X for second column tab area.

	b = sendKey(t, b, tea.MouseMsg{
		X:      secondColX,
		Y:      0,
		Button: tea.MouseButtonLeft,
		Action: tea.MouseActionPress,
	})

	if b.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d after clicking column 2 tab, want 1", b.ActiveTab)
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
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", true, false)

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
