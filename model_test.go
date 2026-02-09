package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// expectedColumnCount is the number of Kanban columns in the board.
const expectedColumnCount = 5

// expectedColumnTitles are the Kanban column names from the spec.
var expectedColumnTitles = []string{"New", "Refined", "Implementing", "PR Ready", "Done"}

// keyMsg builds a tea.KeyMsg for a single rune key (e.g., "h", "l", "j", "k", "q").
func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// arrowMsg builds a tea.KeyMsg for a special key type.
func arrowMsg(kt tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: kt}
}

// sendKey is a helper that sends a key message through Update and returns the updated Board.
func sendKey(t *testing.T, b Board, msg tea.Msg) Board {
	t.Helper()
	m, _ := b.Update(msg)
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return updated
}

// requireColumns fails the test immediately if the board has no columns,
// preventing panics from index-out-of-range on the stub implementation.
func requireColumns(t *testing.T, b Board) {
	t.Helper()
	if len(b.Columns) == 0 {
		t.Fatal("NewBoard() returned 0 columns; cannot test item navigation")
	}
}

// --- Initial State ---

func TestNewBoard_HasExpectedColumnCount(t *testing.T) {
	b := NewBoard()
	if got := len(b.Columns); got != expectedColumnCount {
		t.Errorf("NewBoard() has %d columns, want %d", got, expectedColumnCount)
	}
}

func TestNewBoard_ColumnsHaveCorrectTitles(t *testing.T) {
	b := NewBoard()
	if len(b.Columns) != len(expectedColumnTitles) {
		t.Fatalf("column count %d != expected title count %d", len(b.Columns), len(expectedColumnTitles))
	}
	for i, want := range expectedColumnTitles {
		if got := b.Columns[i].Title; got != want {
			t.Errorf("column %d title = %q, want %q", i, got, want)
		}
	}
}

func TestNewBoard_ActiveTabStartsAtZero(t *testing.T) {
	b := NewBoard()
	if b.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestNewBoard_EachColumnHasCards(t *testing.T) {
	b := NewBoard()
	for i, col := range b.Columns {
		if len(col.Cards) == 0 {
			t.Errorf("column %d (%q) has no cards, want at least one", i, col.Title)
		}
	}
}

func TestNewBoard_CardsHaveRequiredFields(t *testing.T) {
	b := NewBoard()
	for ci, col := range b.Columns {
		for cardIdx, card := range col.Cards {
			if card.Number == 0 {
				t.Errorf("column %d card %d: Number is 0, want a positive issue number", ci, cardIdx)
			}
			if card.Title == "" {
				t.Errorf("column %d card %d: Title is empty", ci, cardIdx)
			}
			if card.Label == "" {
				t.Errorf("column %d card %d: Label is empty", ci, cardIdx)
			}
		}
	}
}

func TestNewBoard_ColumnCursorsStartAtZero(t *testing.T) {
	b := NewBoard()
	for i, col := range b.Columns {
		if col.Cursor != 0 {
			t.Errorf("column %d cursor = %d, want 0", i, col.Cursor)
		}
	}
}

// --- Tab Navigation ---

func TestTabNavigation_L_MovesRight(t *testing.T) {
	b := NewBoard()
	b = sendKey(t, b, keyMsg("l"))
	if b.ActiveTab != 1 {
		t.Errorf("after 'l': ActiveTab = %d, want 1", b.ActiveTab)
	}
}

func TestTabNavigation_H_MovesLeft(t *testing.T) {
	b := NewBoard()
	// Move right first so we can move left
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("h"))
	if b.ActiveTab != 0 {
		t.Errorf("after 'l' then 'h': ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestTabNavigation_RightArrow_MovesRight(t *testing.T) {
	b := NewBoard()
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.ActiveTab != 1 {
		t.Errorf("after Right arrow: ActiveTab = %d, want 1", b.ActiveTab)
	}
}

func TestTabNavigation_LeftArrow_MovesLeft(t *testing.T) {
	b := NewBoard()
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.ActiveTab != 0 {
		t.Errorf("after Right then Left arrow: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestTabNavigation_H_ClampsAtStart(t *testing.T) {
	b := NewBoard()
	// Already at column 0, pressing h should stay at 0
	b = sendKey(t, b, keyMsg("h"))
	if b.ActiveTab != 0 {
		t.Errorf("'h' at column 0: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestTabNavigation_L_ClampsAtEnd(t *testing.T) {
	b := NewBoard()
	if len(b.Columns) < 2 {
		t.Fatal("NewBoard() must have at least 2 columns for this test")
	}
	lastColumn := len(b.Columns) - 1
	// Move to the last column and then one more
	for i := 0; i < len(b.Columns); i++ {
		b = sendKey(t, b, keyMsg("l"))
	}
	if b.ActiveTab != lastColumn {
		t.Errorf("pressing 'l' past end: ActiveTab = %d, want %d", b.ActiveTab, lastColumn)
	}
}

func TestTabNavigation_FullTraversal(t *testing.T) {
	b := NewBoard()
	if len(b.Columns) < 2 {
		t.Fatal("NewBoard() must have at least 2 columns for this test")
	}
	lastColumn := len(b.Columns) - 1

	// Move all the way right
	for i := 0; i < lastColumn; i++ {
		b = sendKey(t, b, keyMsg("l"))
	}
	if b.ActiveTab != lastColumn {
		t.Errorf("after traversing right: ActiveTab = %d, want %d", b.ActiveTab, lastColumn)
	}

	// Move all the way back left
	for i := 0; i < lastColumn; i++ {
		b = sendKey(t, b, keyMsg("h"))
	}
	if b.ActiveTab != 0 {
		t.Errorf("after traversing back left: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

// --- Item Navigation ---

func TestItemNavigation_J_MovesCursorDown(t *testing.T) {
	b := NewBoard()
	requireColumns(t, b)
	b = sendKey(t, b, keyMsg("j"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 1 {
		t.Errorf("after 'j': cursor = %d, want 1", cursor)
	}
}

func TestItemNavigation_K_MovesCursorUp(t *testing.T) {
	b := NewBoard()
	requireColumns(t, b)
	// Move down first so we can move up
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("k"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("after 'j' then 'k': cursor = %d, want 0", cursor)
	}
}

func TestItemNavigation_DownArrow_MovesCursorDown(t *testing.T) {
	b := NewBoard()
	requireColumns(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 1 {
		t.Errorf("after Down arrow: cursor = %d, want 1", cursor)
	}
}

func TestItemNavigation_UpArrow_MovesCursorUp(t *testing.T) {
	b := NewBoard()
	requireColumns(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("after Down then Up arrow: cursor = %d, want 0", cursor)
	}
}

func TestItemNavigation_K_ClampsAtStart(t *testing.T) {
	b := NewBoard()
	requireColumns(t, b)
	// Already at cursor 0, pressing k should stay at 0
	b = sendKey(t, b, keyMsg("k"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("'k' at cursor 0: cursor = %d, want 0", cursor)
	}
}

func TestItemNavigation_J_ClampsAtEnd(t *testing.T) {
	b := NewBoard()
	requireColumns(t, b)
	cardCount := len(b.Columns[b.ActiveTab].Cards)
	if cardCount == 0 {
		t.Fatal("active column has no cards; cannot test cursor clamping")
	}
	// Press j more times than there are cards
	for i := 0; i < cardCount+1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	cursor := b.Columns[b.ActiveTab].Cursor
	lastIndex := cardCount - 1
	if cursor != lastIndex {
		t.Errorf("pressing 'j' past end: cursor = %d, want %d", cursor, lastIndex)
	}
}

func TestItemNavigation_CursorIsPerColumn(t *testing.T) {
	b := NewBoard()
	requireColumns(t, b)
	if len(b.Columns) < 2 {
		t.Fatal("NewBoard() must have at least 2 columns for this test")
	}
	// Move cursor down in column 0
	b = sendKey(t, b, keyMsg("j"))
	// Switch to column 1
	b = sendKey(t, b, keyMsg("l"))
	// Column 1 cursor should still be at 0
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("column 1 cursor after switching = %d, want 0 (cursor should be per-column)", cursor)
	}
}

// --- Quit ---

func TestQuit_Q_ReturnsQuitCmd(t *testing.T) {
	b := NewBoard()
	_, cmd := b.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("'q' key should return a non-nil Cmd (tea.Quit)")
	}
}

func TestQuit_CtrlC_ReturnsQuitCmd(t *testing.T) {
	b := NewBoard()
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C should return a non-nil Cmd (tea.Quit)")
	}
}

// --- Window Resize ---

func TestWindowResize_UpdatesDimensions(t *testing.T) {
	b := NewBoard()
	wantWidth := 120
	wantHeight := 40
	m, _ := b.Update(tea.WindowSizeMsg{Width: wantWidth, Height: wantHeight})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if updated.Width != wantWidth {
		t.Errorf("Width = %d, want %d", updated.Width, wantWidth)
	}
	if updated.Height != wantHeight {
		t.Errorf("Height = %d, want %d", updated.Height, wantHeight)
	}
}

// --- View Rendering ---

func TestView_TabBarShowsAllTabNames(t *testing.T) {
	b := NewBoard()
	b.Width = 120
	b.Height = 40
	view := b.View()
	for _, title := range expectedColumnTitles {
		if !strings.Contains(view, title) {
			t.Errorf("View() does not contain tab name %q", title)
		}
	}
}

func TestView_ContainsActiveTabCardData(t *testing.T) {
	b := NewBoard()
	b.Width = 120
	b.Height = 40
	view := b.View()
	// Only the active tab's cards should appear in the view
	activeCol := b.Columns[b.ActiveTab]
	for _, card := range activeCol.Cards {
		if !strings.Contains(view, card.Title) {
			t.Errorf("View() does not contain active tab card title %q", card.Title)
		}
	}
}

func TestView_DetailPanelShowsSelectedCard(t *testing.T) {
	b := NewBoard()
	b.Width = 120
	b.Height = 40
	view := b.View()

	// Detail panel should show the selected card's label
	selectedCard := b.Columns[b.ActiveTab].Cards[0]
	if !strings.Contains(view, selectedCard.Label) {
		t.Errorf("View() detail panel does not contain selected card label %q", selectedCard.Label)
	}

	// After navigating down, detail should update to the new card
	b = sendKey(t, b, keyMsg("j"))
	view = b.View()
	nextCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if !strings.Contains(view, nextCard.Label) {
		t.Errorf("View() detail panel does not contain card label %q after navigating", nextCard.Label)
	}
}

func TestView_OnlyActiveTabCardsVisible(t *testing.T) {
	b := NewBoard()
	b.Width = 120
	b.Height = 40

	// Switch to tab 1 (Refined)
	b = sendKey(t, b, keyMsg("l"))
	view := b.View()

	// Cards from tab 0 (New) should NOT be visible
	for _, card := range b.Columns[0].Cards {
		if strings.Contains(view, card.Title) {
			// Only fail if this card title doesn't also appear in the active tab
			found := false
			for _, activeCard := range b.Columns[b.ActiveTab].Cards {
				if activeCard.Title == card.Title {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("View() contains card title %q from inactive tab", card.Title)
			}
		}
	}
}

func TestView_ContainsHelpBar(t *testing.T) {
	b := NewBoard()
	b.Width = 120
	b.Height = 40
	view := b.View()
	helpKeywords := []string{"h/l", "j/k", "q"}
	for _, kw := range helpKeywords {
		if !strings.Contains(view, kw) {
			t.Errorf("View() does not contain help text %q", kw)
		}
	}
}

func TestView_IsNotEmpty(t *testing.T) {
	b := NewBoard()
	b.Width = 120
	b.Height = 40
	view := b.View()
	if strings.TrimSpace(view) == "" {
		t.Error("View() returned empty string, want rendered board content")
	}
}

// --- Create Mode ---

func TestNewBoard_StartsInNormalMode(t *testing.T) {
	b := NewBoard()
	if b.mode != normalMode {
		t.Errorf("NewBoard().mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestCreateMode_N_EntersCreateMode(t *testing.T) {
	b := NewBoard()
	b = sendKey(t, b, keyMsg("n"))
	if b.mode != createMode {
		t.Errorf("after 'n': mode = %d, want %d (createMode)", b.mode, createMode)
	}
}

func TestCreateMode_Escape_ReturnsToNormalMode(t *testing.T) {
	b := NewBoard()
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	if b.mode != normalMode {
		t.Errorf("after 'n' then Escape: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestCreateMode_BlocksNavigation(t *testing.T) {
	b := NewBoard()
	requireColumns(t, b)
	b = sendKey(t, b, keyMsg("n"))

	origTab := b.ActiveTab
	origCursor := b.Columns[b.ActiveTab].Cursor

	// h, l should not change ActiveTab
	b = sendKey(t, b, keyMsg("h"))
	if b.ActiveTab != origTab {
		t.Errorf("'h' in createMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}
	b = sendKey(t, b, keyMsg("l"))
	if b.ActiveTab != origTab {
		t.Errorf("'l' in createMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}

	// j, k should not change cursor
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("'j' in createMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
	b = sendKey(t, b, keyMsg("k"))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("'k' in createMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
}

func TestCreateMode_BlocksArrowKeys(t *testing.T) {
	b := NewBoard()
	requireColumns(t, b)
	b = sendKey(t, b, keyMsg("n"))

	origTab := b.ActiveTab
	origCursor := b.Columns[b.ActiveTab].Cursor

	// Arrow keys should not change ActiveTab or cursor
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.ActiveTab != origTab {
		t.Errorf("Left arrow in createMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.ActiveTab != origTab {
		t.Errorf("Right arrow in createMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("Down arrow in createMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("Up arrow in createMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
}

func TestCreateMode_BlocksQuit(t *testing.T) {
	b := NewBoard()
	b = sendKey(t, b, keyMsg("n"))
	_, cmd := b.Update(keyMsg("q"))
	if cmd != nil {
		t.Error("'q' in createMode should NOT return a Cmd, but got non-nil")
	}
}

func TestCreateMode_CtrlC_StillQuits(t *testing.T) {
	b := NewBoard()
	b = sendKey(t, b, keyMsg("n"))
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in createMode should return a non-nil Cmd (tea.Quit)")
	}
}

func TestCreateMode_N_DoesNotToggle(t *testing.T) {
	b := NewBoard()
	b = sendKey(t, b, keyMsg("n"))
	// Pressing n again should NOT toggle back to normalMode
	b = sendKey(t, b, keyMsg("n"))
	if b.mode != createMode {
		t.Errorf("pressing 'n' twice: mode = %d, want %d (createMode, should not toggle)", b.mode, createMode)
	}
}
