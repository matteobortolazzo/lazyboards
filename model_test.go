package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// expectedColumnCount is the number of Kanban columns in the board.
const expectedColumnCount = 5

// expectedColumnTitles are the Kanban column names from the spec.
var expectedColumnTitles = []string{"New", "Refined", "Implementing", "PR Ready", "Done"}

// newTestBoard creates a Board in loadingMode using NewBoard.
func newTestBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	return NewBoard(p, nil, nil, "", "", "", false)
}

// newLoadedTestBoard creates a Board and sends a boardFetchedMsg to transition
// it to normalMode with populated columns (simulating a successful fetch).
func newLoadedTestBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "", "", "", false)
	// Simulate the provider returning board data.
	board, err := p.FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return updated
}

// errorProvider is a test-only provider that always returns errors.
type errorProvider struct{}

func (e errorProvider) FetchBoard(_ context.Context) (provider.Board, error) {
	return provider.Board{}, errors.New("connection failed")
}

func (e errorProvider) CreateCard(_ context.Context, _ string, _ string) (provider.Card, error) {
	return provider.Card{}, errors.New("not implemented")
}

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

// execCmds recursively executes a tea.Cmd, handling tea.BatchMsg.
func execCmds(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	msg := cmd()
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		for _, subCmd := range batchMsg {
			execCmds(subCmd)
		}
	}
}

// requireColumns fails the test immediately if the board has no columns,
// preventing panics from index-out-of-range on the stub implementation.
func requireColumns(t *testing.T, b Board) {
	t.Helper()
	if len(b.Columns) == 0 {
		t.Fatal("board has 0 columns; cannot test item navigation")
	}
}

// --- Async Loading: Initial State ---

func TestNewBoard_StartsInLoadingMode(t *testing.T) {
	b := newTestBoard(t)
	if b.mode != loadingMode {
		t.Errorf("mode = %d, want loadingMode", b.mode)
	}
}

func TestNewBoard_InitReturnsCmds(t *testing.T) {
	b := newTestBoard(t)
	cmd := b.Init()
	if cmd == nil {
		t.Error("Init() should return non-nil cmd (batch of spinner tick + fetch)")
	}
}

// --- Async Loading: Fetch Success ---

func TestLoading_FetchSuccess_TransitionsToNormalMode(t *testing.T) {
	b := newTestBoard(t)
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{
			Title: "Col1",
			Cards: []provider.Card{{Number: 1, Title: "Card1", Labels: []string{"bug"}}},
		}},
	}}
	m, _ := b.Update(msg)
	updated := m.(Board)
	if updated.mode != normalMode {
		t.Errorf("mode = %d, want normalMode after successful fetch", updated.mode)
	}
	if len(updated.Columns) == 0 {
		t.Error("Columns should be populated after successful fetch")
	}
}

// --- Async Loading: Fetch Error ---

func TestLoading_FetchError_TransitionsToErrorMode(t *testing.T) {
	b := newTestBoard(t)
	msg := boardFetchErrorMsg{err: errors.New("connection failed")}
	m, _ := b.Update(msg)
	updated := m.(Board)
	if updated.mode != errorMode {
		t.Errorf("mode = %d, want errorMode after fetch error", updated.mode)
	}
}

// --- Error Mode: Retry ---

func TestErrorMode_R_RetriesAndTransitionsToLoadingMode(t *testing.T) {
	b := newTestBoard(t)
	// Put board in errorMode.
	m, _ := b.Update(boardFetchErrorMsg{err: errors.New("fail")})
	b = m.(Board)
	// Press r to retry.
	m, cmd := b.Update(keyMsg("r"))
	b = m.(Board)
	if b.mode != loadingMode {
		t.Errorf("mode = %d, want loadingMode after retry", b.mode)
	}
	if cmd == nil {
		t.Error("retry should return non-nil cmd (spinner tick + fetch)")
	}
}

// --- Error Mode: Quit ---

func TestErrorMode_Q_Quits(t *testing.T) {
	b := newTestBoard(t)
	m, _ := b.Update(boardFetchErrorMsg{err: errors.New("fail")})
	b = m.(Board)
	_, cmd := b.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("'q' in errorMode should return quit cmd")
	}
}

// --- Loading Mode: Key Isolation ---

func TestLoadingMode_IgnoresNavigationKeys(t *testing.T) {
	b := newTestBoard(t)
	origTab := b.ActiveTab
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("k"))
	if b.ActiveTab != origTab {
		t.Error("navigation keys should be ignored in loadingMode")
	}
	if b.mode != loadingMode {
		t.Error("mode should still be loadingMode after navigation keys")
	}
}

// --- Loading Mode: View ---

func TestLoading_ViewShowsLoadingText(t *testing.T) {
	b := newTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()
	if !strings.Contains(view, "Loading board") {
		t.Error("View() in loadingMode should contain 'Loading board'")
	}
}

// --- Error Mode: View ---

func TestError_ViewShowsErrorAndRetryHint(t *testing.T) {
	b := newTestBoard(t)
	b.Width = 120
	b.Height = 40
	m, _ := b.Update(boardFetchErrorMsg{err: errors.New("connection failed")})
	b = m.(Board)
	view := b.View()
	if !strings.Contains(view, "connection failed") {
		t.Error("View() in errorMode should contain the error message")
	}
	if !strings.Contains(view, "r") {
		t.Error("View() in errorMode should contain retry hint")
	}
}

// --- Loading Mode: Spinner ---

func TestLoading_SpinnerTickPropagated(t *testing.T) {
	b := newTestBoard(t)
	// Send a spinner.TickMsg to the board in loadingMode.
	tickMsg := spinner.TickMsg{}
	m, _ := b.Update(tickMsg)
	updated := m.(Board)
	if updated.mode != loadingMode {
		t.Error("mode should still be loadingMode after spinner tick")
	}
}

// --- Loaded Board: Initial State ---

func TestNewBoard_HasExpectedColumnCount(t *testing.T) {
	b := newLoadedTestBoard(t)
	if got := len(b.Columns); got != expectedColumnCount {
		t.Errorf("loaded board has %d columns, want %d", got, expectedColumnCount)
	}
}

func TestNewBoard_ColumnsHaveCorrectTitles(t *testing.T) {
	b := newLoadedTestBoard(t)
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
	b := newLoadedTestBoard(t)
	if b.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestNewBoard_EachColumnHasCards(t *testing.T) {
	b := newLoadedTestBoard(t)
	for i, col := range b.Columns {
		if len(col.Cards) == 0 {
			t.Errorf("column %d (%q) has no cards, want at least one", i, col.Title)
		}
	}
}

func TestNewBoard_CardsHaveRequiredFields(t *testing.T) {
	b := newLoadedTestBoard(t)
	for ci, col := range b.Columns {
		for cardIdx, card := range col.Cards {
			if card.Number == 0 {
				t.Errorf("column %d card %d: Number is 0, want a positive issue number", ci, cardIdx)
			}
			if card.Title == "" {
				t.Errorf("column %d card %d: Title is empty", ci, cardIdx)
			}
			if len(card.Labels) == 0 {
				t.Errorf("column %d card %d: Labels is empty, want at least one label", ci, cardIdx)
			}
		}
	}
}

func TestNewBoard_ColumnCursorsStartAtZero(t *testing.T) {
	b := newLoadedTestBoard(t)
	for i, col := range b.Columns {
		if col.Cursor != 0 {
			t.Errorf("column %d cursor = %d, want 0", i, col.Cursor)
		}
	}
}

// --- Tab Navigation ---

func TestTabNavigation_L_MovesRight(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 1 {
		t.Errorf("after 'l': ActiveTab = %d, want 1", b.ActiveTab)
	}
}

func TestTabNavigation_H_MovesLeft(t *testing.T) {
	b := newLoadedTestBoard(t)
	// Move right first so we can move left
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	if b.ActiveTab != 0 {
		t.Errorf("after 'l' then 'h': ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestTabNavigation_RightArrow_MovesRight(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.ActiveTab != 1 {
		t.Errorf("after Right arrow: ActiveTab = %d, want 1", b.ActiveTab)
	}
}

func TestTabNavigation_LeftArrow_MovesLeft(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.ActiveTab != 0 {
		t.Errorf("after Right then Left arrow: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestTabNavigation_H_ClampsAtStart(t *testing.T) {
	b := newLoadedTestBoard(t)
	// Already at column 0, pressing h should stay at 0
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	if b.ActiveTab != 0 {
		t.Errorf("'h' at column 0: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestTabNavigation_L_ClampsAtEnd(t *testing.T) {
	b := newLoadedTestBoard(t)
	if len(b.Columns) < 2 {
		t.Fatal("board must have at least 2 columns for this test")
	}
	lastColumn := len(b.Columns) - 1
	// Move to the last column and then one more
	for i := 0; i < len(b.Columns); i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyTab))
	}
	if b.ActiveTab != lastColumn {
		t.Errorf("pressing 'l' past end: ActiveTab = %d, want %d", b.ActiveTab, lastColumn)
	}
}

func TestTabNavigation_FullTraversal(t *testing.T) {
	b := newLoadedTestBoard(t)
	if len(b.Columns) < 2 {
		t.Fatal("board must have at least 2 columns for this test")
	}
	lastColumn := len(b.Columns) - 1

	// Move all the way right
	for i := 0; i < lastColumn; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyTab))
	}
	if b.ActiveTab != lastColumn {
		t.Errorf("after traversing right: ActiveTab = %d, want %d", b.ActiveTab, lastColumn)
	}

	// Move all the way back left
	for i := 0; i < lastColumn; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	}
	if b.ActiveTab != 0 {
		t.Errorf("after traversing back left: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

// --- Item Navigation ---

func TestItemNavigation_J_MovesCursorDown(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	b = sendKey(t, b, keyMsg("j"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 1 {
		t.Errorf("after 'j': cursor = %d, want 1", cursor)
	}
}

func TestItemNavigation_K_MovesCursorUp(t *testing.T) {
	b := newLoadedTestBoard(t)
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
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 1 {
		t.Errorf("after Down arrow: cursor = %d, want 1", cursor)
	}
}

func TestItemNavigation_UpArrow_MovesCursorUp(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	b = sendKey(t, b, arrowMsg(tea.KeyUp))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("after Down then Up arrow: cursor = %d, want 0", cursor)
	}
}

func TestItemNavigation_K_ClampsAtStart(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	// Already at cursor 0, pressing k should stay at 0
	b = sendKey(t, b, keyMsg("k"))
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("'k' at cursor 0: cursor = %d, want 0", cursor)
	}
}

func TestItemNavigation_J_ClampsAtEnd(t *testing.T) {
	b := newLoadedTestBoard(t)
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
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	if len(b.Columns) < 2 {
		t.Fatal("board must have at least 2 columns for this test")
	}
	// Move cursor down in column 0
	b = sendKey(t, b, keyMsg("j"))
	// Switch to column 1
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	// Column 1 cursor should still be at 0
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor != 0 {
		t.Errorf("column 1 cursor after switching = %d, want 0 (cursor should be per-column)", cursor)
	}
}

// --- Quit ---

func TestQuit_Q_ReturnsQuitCmd(t *testing.T) {
	b := newLoadedTestBoard(t)
	_, cmd := b.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("'q' key should return a non-nil Cmd (tea.Quit)")
	}
}

func TestQuit_CtrlC_ReturnsQuitCmd(t *testing.T) {
	b := newLoadedTestBoard(t)
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C should return a non-nil Cmd (tea.Quit)")
	}
}

// --- Window Resize ---

func TestWindowResize_UpdatesDimensions(t *testing.T) {
	b := newLoadedTestBoard(t)
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
	b := newLoadedTestBoard(t)
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
	b := newLoadedTestBoard(t)
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
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()

	// Detail panel should show the selected card's labels (comma-separated).
	selectedCard := b.Columns[b.ActiveTab].Cards[0]
	labelsStr := strings.Join(selectedCard.Labels, ", ")
	if !strings.Contains(view, labelsStr) {
		t.Errorf("View() detail panel does not contain selected card labels %q", labelsStr)
	}

	// After navigating down, detail should update to the new card.
	b = sendKey(t, b, keyMsg("j"))
	view = b.View()
	nextCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	nextLabelsStr := strings.Join(nextCard.Labels, ", ")
	if !strings.Contains(view, nextLabelsStr) {
		t.Errorf("View() detail panel does not contain card labels %q after navigating", nextLabelsStr)
	}
}

func TestView_OnlyActiveTabCardsVisible(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Switch to tab 1 (Refined)
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
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
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()

	// Status bar should show contextual hints for normalMode.
	expectedHints := []string{"n: New", "r: Refresh", "q: Quit"}
	for _, hint := range expectedHints {
		if !strings.Contains(view, hint) {
			t.Errorf("View() does not contain status bar hint %q", hint)
		}
	}

	// Old-style combined key hints should NOT appear.
	oldHints := []string{"h/l", "j/k"}
	for _, old := range oldHints {
		if strings.Contains(view, old) {
			t.Errorf("View() still contains old help text %q, want new status bar format", old)
		}
	}
}

func TestView_IsNotEmpty(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()
	if strings.TrimSpace(view) == "" {
		t.Error("View() returned empty string, want rendered board content")
	}
}

// --- Create Mode ---

func TestCreateMode_N_EntersCreateMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	if b.mode != createMode {
		t.Errorf("after 'n': mode = %d, want %d (createMode)", b.mode, createMode)
	}
}

func TestCreateMode_Escape_ReturnsToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	if b.mode != normalMode {
		t.Errorf("after 'n' then Escape: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestCreateMode_BlocksNavigation(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	b = sendKey(t, b, keyMsg("n"))

	origTab := b.ActiveTab
	origCursor := b.Columns[b.ActiveTab].Cursor

	// h, l should not change ActiveTab
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	if b.ActiveTab != origTab {
		t.Errorf("'h' in createMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
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
	b := newLoadedTestBoard(t)
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
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	m, _ := b.Update(keyMsg("q"))
	updated := m.(Board)
	// q should NOT quit — board should still be in createMode
	if updated.mode != createMode {
		t.Errorf("'q' in createMode changed mode to %d, want %d (createMode)", updated.mode, createMode)
	}
}

func TestCreateMode_CtrlC_StillQuits(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in createMode should return a non-nil Cmd (tea.Quit)")
	}
}

func TestCreateMode_N_DoesNotToggle(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	// Pressing n again should NOT toggle back to normalMode
	b = sendKey(t, b, keyMsg("n"))
	if b.mode != createMode {
		t.Errorf("pressing 'n' twice: mode = %d, want %d (createMode, should not toggle)", b.mode, createMode)
	}
}

// --- Create Mode UI ---

func TestCreateMode_ViewShowsModal(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n"))
	view := b.View()
	if !strings.Contains(view, "New Card") {
		t.Error("View() in createMode should contain 'New Card' header text")
	}
}

func TestCreateMode_ViewShowsTitleField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n"))
	view := b.View()
	if !strings.Contains(view, "Title") {
		t.Error("View() in createMode should contain 'Title' label or placeholder")
	}
}

func TestCreateMode_ViewShowsLabelField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n"))
	view := b.View()
	if !strings.Contains(view, "Label") {
		t.Error("View() in createMode should contain 'Label' label or placeholder")
	}
}

func TestCreateMode_TabSwitchesFocus(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))

	// Title should be focused initially.
	if !b.titleInput.Focused() {
		t.Error("titleInput should be focused when entering createMode")
	}
	if b.labelInput.Focused() {
		t.Error("labelInput should NOT be focused when entering createMode")
	}

	// Tab should switch focus to labelInput.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.titleInput.Focused() {
		t.Error("titleInput should NOT be focused after Tab")
	}
	if !b.labelInput.Focused() {
		t.Error("labelInput should be focused after Tab")
	}

	// Another Tab should switch focus back to titleInput.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if !b.titleInput.Focused() {
		t.Error("titleInput should be focused after second Tab")
	}
	if b.labelInput.Focused() {
		t.Error("labelInput should NOT be focused after second Tab")
	}
}

func TestCreateMode_TypingUpdatesTitleField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))

	// Type characters while title is focused.
	for _, ch := range "Fix bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	if b.titleInput.Value() != "Fix bug" {
		t.Errorf("titleInput.Value() = %q, want %q", b.titleInput.Value(), "Fix bug")
	}
}

func TestCreateMode_TypingUpdatesLabelField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))

	// Tab to label field.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	// Type characters while label is focused.
	for _, ch := range "bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	if b.labelInput.Value() != "bug" {
		t.Errorf("labelInput.Value() = %q, want %q", b.labelInput.Value(), "bug")
	}
}

func TestCreateMode_FieldsResetOnReopen(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))

	// Type something in the title field.
	for _, ch := range "hello" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Escape back to normalMode.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	// Re-enter createMode.
	b = sendKey(t, b, keyMsg("n"))

	if b.titleInput.Value() != "" {
		t.Errorf("titleInput.Value() after reopen = %q, want empty string (fields should reset)", b.titleInput.Value())
	}
	if b.labelInput.Value() != "" {
		t.Errorf("labelInput.Value() after reopen = %q, want empty string (fields should reset)", b.labelInput.Value())
	}
}

// --- Form Submission ---

func TestSubmit_CreatesCardInNewColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	originalCardCount := len(b.Columns[0].Cards)

	// Enter createMode and type a title.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "My task" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to submit (transitions to creatingMode).
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Simulate async success.
	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "My task", Labels: nil}})
	b = m.(Board)

	// A new card should exist in the "New" column (index 0).
	if len(b.Columns[0].Cards) != originalCardCount+1 {
		t.Fatalf("Columns[0].Cards count = %d, want %d (one card added)", len(b.Columns[0].Cards), originalCardCount+1)
	}

	// The new card should be the last card in the column.
	newCard := b.Columns[0].Cards[len(b.Columns[0].Cards)-1]
	if newCard.Title != "My task" {
		t.Errorf("new card Title = %q, want %q", newCard.Title, "My task")
	}
}

func TestSubmit_AutoNumbersCard(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Find the max card number across all columns.
	maxNumber := 0
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			if card.Number > maxNumber {
				maxNumber = card.Number
			}
		}
	}

	// Enter createMode, type a title, and submit.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "Auto numbered" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Simulate async success with expected auto-numbered card.
	expectedNumber := maxNumber + 1
	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: expectedNumber, Title: "Auto numbered", Labels: nil}})
	b = m.(Board)

	newCard := b.Columns[0].Cards[len(b.Columns[0].Cards)-1]
	if newCard.Number != expectedNumber {
		t.Errorf("new card Number = %d, want %d (max existing + 1)", newCard.Number, expectedNumber)
	}
}

func TestSubmit_WithLabel(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode, type title, Tab to label, type label, submit.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "Labeled task" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	for _, ch := range "bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Simulate async success.
	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "Labeled task", Labels: []string{"bug"}}})
	b = m.(Board)

	newCard := b.Columns[0].Cards[len(b.Columns[0].Cards)-1]
	if len(newCard.Labels) == 0 || newCard.Labels[0] != "bug" {
		t.Errorf("new card Labels = %v, want [\"bug\"]", newCard.Labels)
	}
}

func TestSubmit_EmptyLabelAllowed(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode, type title only (no label), submit.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "No label task" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Simulate async success with empty labels.
	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "No label task", Labels: nil}})
	b = m.(Board)

	newCard := b.Columns[0].Cards[len(b.Columns[0].Cards)-1]
	if len(newCard.Labels) != 0 {
		t.Errorf("new card Labels = %v, want empty (empty label is OK)", newCard.Labels)
	}
}

func TestSubmit_EmptyTitleShowsError(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode and press Enter without typing a title.
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Should stay in createMode.
	if b.mode != createMode {
		t.Errorf("mode = %d, want %d (createMode) when title is empty", b.mode, createMode)
	}

	// Should have a validation error containing "Title is required".
	if !strings.Contains(b.validationErr, "Title is required") {
		t.Errorf("validationErr = %q, want it to contain %q", b.validationErr, "Title is required")
	}
}

func TestSubmit_ErrorClearsOnTyping(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Trigger validation error.
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Confirm error is set.
	if b.validationErr == "" {
		t.Fatal("expected validationErr to be set after empty submit, got empty string")
	}

	// Type a character — error should clear.
	b = sendKey(t, b, keyMsg("a"))
	if b.validationErr != "" {
		t.Errorf("validationErr = %q after typing, want empty string (error should clear)", b.validationErr)
	}
}

func TestSubmit_ReturnsToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode, type title, submit.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "Done task" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Simulate async success.
	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "Done task", Labels: nil}})
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d after successful submit, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestSubmit_ResetsFieldsAfterCreation(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode, type title and label, submit.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "Some task" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	for _, ch := range "feature" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Simulate async success.
	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "Some task", Labels: []string{"feature"}}})
	b = m.(Board)

	if b.titleInput.Value() != "" {
		t.Errorf("titleInput.Value() = %q after submit, want empty string (fields should reset)", b.titleInput.Value())
	}
	if b.labelInput.Value() != "" {
		t.Errorf("labelInput.Value() = %q after submit, want empty string (fields should reset)", b.labelInput.Value())
	}
}

func TestView_HelpBarShowsNewHint(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()

	if !strings.Contains(view, "n: New") {
		t.Errorf("View() status bar does not contain %q", "n: New")
	}
}

// --- Reserved Label Validation ---

func TestCreateMode_ReservedLabel_ShowsError(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode and type a title.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "Test title" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Tab to label field and type the first column title (a reserved label).
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	reservedLabel := b.Columns[0].Title // "New"
	for _, ch := range reservedLabel {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to submit.
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Should stay in createMode with a validation error.
	if b.mode != createMode {
		t.Errorf("mode = %d, want %d (createMode) when reserved label used", b.mode, createMode)
	}
	if !strings.Contains(b.validationErr, "Cannot use reserved column label") {
		t.Errorf("validationErr = %q, want it to contain %q", b.validationErr, "Cannot use reserved column label")
	}
}

func TestCreateMode_ReservedLabel_CaseInsensitive(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode and type a title.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "Test title" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Tab to label field and type the first column title in lowercase.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	reservedLabel := strings.ToLower(b.Columns[0].Title) // "new"
	for _, ch := range reservedLabel {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to submit.
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Should stay in createMode with a validation error (case-insensitive check).
	if b.mode != createMode {
		t.Errorf("mode = %d, want %d (createMode) when reserved label used (lowercase)", b.mode, createMode)
	}
	if !strings.Contains(b.validationErr, "Cannot use reserved column label") {
		t.Errorf("validationErr = %q, want it to contain %q", b.validationErr, "Cannot use reserved column label")
	}
}

func TestCreateMode_ReservedLabel_NonReservedAllowed(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode and type a title.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "Test title" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Tab to label field and type a non-reserved label.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	for _, ch := range "bug" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to submit.
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Should NOT be stuck in createMode with a reserved label error.
	if b.mode == createMode && strings.Contains(b.validationErr, "Cannot use reserved column label") {
		t.Errorf("non-reserved label 'bug' should not trigger reserved label error, but got validationErr = %q", b.validationErr)
	}
}

// --- Async Submission ---

// newCreatingTestBoard creates a Board in creatingMode for testing async creation.
func newCreatingTestBoard(t *testing.T) Board {
	t.Helper()
	b := newLoadedTestBoard(t)
	b.mode = creatingMode
	return b
}

func TestCreateMode_Submit_TransitionsToCreatingMode(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Enter createMode and type a title.
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "Test" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to submit.
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// Should transition to creatingMode (async submission in progress).
	if b.mode != creatingMode {
		t.Errorf("mode = %d, want %d (creatingMode) after submitting form", b.mode, creatingMode)
	}
}

func TestCreatingMode_IgnoresKeys(t *testing.T) {
	b := newCreatingTestBoard(t)

	origTab := b.ActiveTab
	origCursor := b.Columns[b.ActiveTab].Cursor

	// All navigation and action keys should be ignored.
	for _, key := range []string{"h", "l", "j", "k", "q", "n"} {
		b = sendKey(t, b, keyMsg(key))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != creatingMode {
		t.Errorf("mode = %d after keys in creatingMode, want %d (creatingMode)", b.mode, creatingMode)
	}
	if b.ActiveTab != origTab {
		t.Errorf("ActiveTab = %d after keys in creatingMode, want %d (unchanged)", b.ActiveTab, origTab)
	}
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("cursor = %d after keys in creatingMode, want %d (unchanged)", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
}

func TestCreatingMode_SpinnerTickPropagated(t *testing.T) {
	b := newCreatingTestBoard(t)

	tickMsg := spinner.TickMsg{}
	m, cmd := b.Update(tickMsg)
	updated := m.(Board)

	if updated.mode != creatingMode {
		t.Errorf("mode = %d after spinner tick in creatingMode, want %d (creatingMode)", updated.mode, creatingMode)
	}
	if cmd == nil {
		t.Error("spinner tick in creatingMode should return a non-nil cmd")
	}
}

func TestCreatingMode_Success_AppendsCardAndClosesModal(t *testing.T) {
	b := newCreatingTestBoard(t)
	originalCardCount := len(b.Columns[0].Cards)

	msg := cardCreatedMsg{card: provider.Card{Number: 99, Title: "New task", Labels: []string{"feature"}}}
	m, _ := b.Update(msg)
	updated := m.(Board)

	// Should return to normalMode.
	if updated.mode != normalMode {
		t.Errorf("mode = %d after cardCreatedMsg, want %d (normalMode)", updated.mode, normalMode)
	}

	// New card should be appended to the first column.
	if len(updated.Columns[0].Cards) != originalCardCount+1 {
		t.Fatalf("Columns[0].Cards count = %d, want %d (one card added)", len(updated.Columns[0].Cards), originalCardCount+1)
	}
	newCard := updated.Columns[0].Cards[len(updated.Columns[0].Cards)-1]
	if newCard.Number != 99 {
		t.Errorf("new card Number = %d, want 99", newCard.Number)
	}
	if newCard.Title != "New task" {
		t.Errorf("new card Title = %q, want %q", newCard.Title, "New task")
	}
	if len(newCard.Labels) == 0 || newCard.Labels[0] != "feature" {
		t.Errorf("new card Labels = %v, want [\"feature\"]", newCard.Labels)
	}

	// Fields should be reset.
	if updated.titleInput.Value() != "" {
		t.Errorf("titleInput.Value() = %q after success, want empty string", updated.titleInput.Value())
	}
	if updated.labelInput.Value() != "" {
		t.Errorf("labelInput.Value() = %q after success, want empty string", updated.labelInput.Value())
	}
	if updated.validationErr != "" {
		t.Errorf("validationErr = %q after success, want empty string", updated.validationErr)
	}
}

func TestCreatingMode_Error_ShowsErrorAndPreservesInput(t *testing.T) {
	b := newCreatingTestBoard(t)
	b.titleInput.SetValue("My title")
	b.labelInput.SetValue("my-label")

	msg := cardCreateErrorMsg{err: errors.New("API error")}
	m, _ := b.Update(msg)
	updated := m.(Board)

	// Should go back to createMode so user can edit and retry.
	if updated.mode != createMode {
		t.Errorf("mode = %d after cardCreateErrorMsg, want %d (createMode)", updated.mode, createMode)
	}

	// Validation error should contain the API error message.
	if !strings.Contains(updated.validationErr, "API error") {
		t.Errorf("validationErr = %q, want it to contain %q", updated.validationErr, "API error")
	}

	// Input fields should be preserved so user can retry.
	if updated.titleInput.Value() != "My title" {
		t.Errorf("titleInput.Value() = %q after error, want %q (input should be preserved)", updated.titleInput.Value(), "My title")
	}
	if updated.labelInput.Value() != "my-label" {
		t.Errorf("labelInput.Value() = %q after error, want %q (input should be preserved)", updated.labelInput.Value(), "my-label")
	}

	// Title input should be focused for easy editing.
	if !updated.titleInput.Focused() {
		t.Error("titleInput should be focused after error so user can edit and retry")
	}
}

func TestCreatingMode_View_ShowsSpinner(t *testing.T) {
	b := newCreatingTestBoard(t)
	b.Width = 120
	b.Height = 40

	view := b.View()
	if !strings.Contains(view, "Creating card") {
		t.Error("View() in creatingMode should contain 'Creating card'")
	}
}

// --- Scroll Helper ---

// newBoardWithCards creates a Board with a single column containing cardCount
// cards, plus a second column with one card (for tab-switch tests).
// Width is set to 120 and Height to the given height parameter.
func newBoardWithCards(t *testing.T, cardCount, height int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "", "", "", false)

	// Build provider cards.
	providerCards := make([]provider.Card, cardCount)
	for i := range providerCards {
		providerCards[i] = provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf("Card %d", i+1),
			Labels: []string{"test"},
		}
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: providerCards},
			{Title: "Column B", Cards: []provider.Card{
				{Number: 100, Title: "Other card", Labels: []string{"test"}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = height
	return board
}

// --- wrapTitle Unit Tests ---

func TestWrapTitle_ShortTitleSingleLine(t *testing.T) {
	title := "Short"
	maxWidth := 20
	got := wrapTitle(title, maxWidth, 0)
	if len(got) != 1 {
		t.Errorf("wrapTitle(%q, %d, 0) returned %d lines, want 1", title, maxWidth, len(got))
	}
	if got[0] != title {
		t.Errorf("wrapTitle(%q, %d, 0)[0] = %q, want %q", title, maxWidth, got[0], title)
	}
}

func TestWrapTitle_ExactWidthSingleLine(t *testing.T) {
	title := "Exactly ten"
	maxWidth := len(title)
	got := wrapTitle(title, maxWidth, 0)
	if len(got) != 1 {
		t.Errorf("wrapTitle(%q, %d, 0) returned %d lines, want 1", title, maxWidth, len(got))
	}
	if got[0] != title {
		t.Errorf("wrapTitle(%q, %d, 0)[0] = %q, want %q", title, maxWidth, got[0], title)
	}
}

func TestWrapTitle_WrapsAtWordBoundary(t *testing.T) {
	title := "This is a very long title that should wrap"
	maxWidth := 20
	indentWidth := 2
	got := wrapTitle(title, maxWidth, indentWidth)

	if len(got) < 2 {
		t.Fatalf("wrapTitle(%q, %d, %d) returned %d lines, want >= 2", title, maxWidth, indentWidth, len(got))
	}

	// First line must fit within maxWidth.
	if len([]rune(got[0])) > maxWidth {
		t.Errorf("first line %q has %d runes, want <= %d", got[0], len([]rune(got[0])), maxWidth)
	}

	// Continuation lines must be indented by indentWidth spaces.
	indent := strings.Repeat(" ", indentWidth)
	for i := 1; i < len(got); i++ {
		if !strings.HasPrefix(got[i], indent) {
			t.Errorf("continuation line %d = %q, want prefix %q", i, got[i], indent)
		}
		if len([]rune(got[i])) > maxWidth {
			t.Errorf("continuation line %d = %q has %d runes, want <= %d", i, got[i], len([]rune(got[i])), maxWidth)
		}
	}

	// All original words should be present across all lines.
	joined := strings.Join(got, " ")
	for _, word := range strings.Fields(title) {
		if !strings.Contains(joined, word) {
			t.Errorf("word %q missing from wrapped output: %v", word, got)
		}
	}
}

func TestWrapTitle_LongWordCharacterBreak(t *testing.T) {
	title := "abcdefghij"
	maxWidth := 5
	got := wrapTitle(title, maxWidth, 0)

	if len(got) < 2 {
		t.Fatalf("wrapTitle(%q, %d, 0) returned %d lines, want >= 2", title, maxWidth, len(got))
	}

	// Each line must not exceed maxWidth.
	for i, line := range got {
		if len([]rune(line)) > maxWidth {
			t.Errorf("line %d = %q has %d runes, want <= %d", i, line, len([]rune(line)), maxWidth)
		}
	}

	// All characters should be preserved.
	joined := strings.Join(got, "")
	joinedTrimmed := strings.ReplaceAll(joined, " ", "")
	if joinedTrimmed != title {
		t.Errorf("character-broken lines joined = %q, want %q", joinedTrimmed, title)
	}
}

func TestWrapTitle_EmptyTitle(t *testing.T) {
	got := wrapTitle("", 20, 0)
	if len(got) < 1 {
		t.Fatal("wrapTitle(\"\", 20, 0) returned empty slice, want at least one element")
	}
}

func TestWrapTitle_VeryNarrowWidth(t *testing.T) {
	// maxWidth of 1 should not panic and should produce output.
	got := wrapTitle("Hello", 1, 0)
	if len(got) < 1 {
		t.Fatal("wrapTitle(\"Hello\", 1, 0) returned empty slice, want at least one element")
	}
	for i, line := range got {
		if len([]rune(line)) > 1 {
			t.Errorf("line %d = %q has %d runes, want <= 1", i, line, len([]rune(line)))
		}
	}

	// maxWidth of 2 should also not panic.
	got2 := wrapTitle("Hi there", 2, 0)
	if len(got2) < 1 {
		t.Fatal("wrapTitle(\"Hi there\", 2, 0) returned empty slice, want at least one element")
	}
	for i, line := range got2 {
		if len([]rune(line)) > 2 {
			t.Errorf("line %d = %q has %d runes, want <= 2", i, line, len([]rune(line)))
		}
	}
}

func TestWrapTitle_MultipleWraps(t *testing.T) {
	title := "one two three four five six seven eight nine ten eleven twelve"
	maxWidth := 15
	indentWidth := 2
	got := wrapTitle(title, maxWidth, indentWidth)

	if len(got) < 3 {
		t.Fatalf("wrapTitle(%q, %d, %d) returned %d lines, want >= 3", title, maxWidth, indentWidth, len(got))
	}

	// First line fits within maxWidth.
	if len([]rune(got[0])) > maxWidth {
		t.Errorf("first line %q has %d runes, want <= %d", got[0], len([]rune(got[0])), maxWidth)
	}

	// All continuation lines are indented and fit within maxWidth.
	indent := strings.Repeat(" ", indentWidth)
	for i := 1; i < len(got); i++ {
		if !strings.HasPrefix(got[i], indent) {
			t.Errorf("continuation line %d = %q, want prefix %q", i, got[i], indent)
		}
		if len([]rune(got[i])) > maxWidth {
			t.Errorf("continuation line %d = %q has %d runes, want <= %d", i, got[i], len([]rune(got[i])), maxWidth)
		}
	}

	// All original words should be present.
	joined := strings.Join(got, " ")
	for _, word := range strings.Fields(title) {
		if !strings.Contains(joined, word) {
			t.Errorf("word %q missing from wrapped output: %v", word, got)
		}
	}
}

// --- Scroll Behavior Tests ---

func TestScroll_AllCardsFit_NoScrollNeeded(t *testing.T) {
	cardCount := 5
	// Height large enough: panelHeight = Height - 6, each card ~1 line.
	// Use a tall terminal so all cards fit.
	height := cardCount + 6 + 10 // plenty of room
	b := newBoardWithCards(t, cardCount, height)

	// Navigate to the last card.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	col := b.Columns[b.ActiveTab]
	if col.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d, want 0 when all cards fit in the viewport", col.ScrollOffset)
	}
}

func TestScroll_CursorDownScrollsViewport(t *testing.T) {
	cardCount := 30
	height := 15 // panelHeight = 15 - 6 = 9, far fewer than 30 cards
	b := newBoardWithCards(t, cardCount, height)

	// Navigate cursor well past the visible area.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	col := b.Columns[b.ActiveTab]
	if col.ScrollOffset <= 0 {
		t.Errorf("ScrollOffset = %d after scrolling past visible area, want > 0", col.ScrollOffset)
	}

	// Cursor should be at the last card.
	if col.Cursor != cardCount-1 {
		t.Errorf("Cursor = %d, want %d (last card)", col.Cursor, cardCount-1)
	}
}

func TestScroll_CursorUpScrollsViewport(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll all the way down.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	// Record the offset after scrolling down.
	offsetAfterDown := b.Columns[b.ActiveTab].ScrollOffset
	if offsetAfterDown <= 0 {
		t.Fatalf("expected ScrollOffset > 0 after scrolling down, got %d", offsetAfterDown)
	}

	// Now scroll all the way back up.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("k"))
	}

	col := b.Columns[b.ActiveTab]
	if col.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d after scrolling back to top, want 0", col.ScrollOffset)
	}
	if col.Cursor != 0 {
		t.Errorf("Cursor = %d after scrolling back to top, want 0", col.Cursor)
	}
}

func TestScroll_OffsetResetsOnTabSwitch(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll down in column A.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	// Switch to column B.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	// Column B should have ScrollOffset appropriate for its cursor position.
	// Since Column B cursor starts at 0 and has only 1 card, offset should be 0.
	col := b.Columns[b.ActiveTab]
	if col.ScrollOffset != 0 {
		t.Errorf("ScrollOffset = %d after tab switch, want 0 (Column B has only 1 card)", col.ScrollOffset)
	}
}

// --- Scroll Indicator Rendering Tests ---

func TestView_NoScrollIndicators_WhenAllCardsFit(t *testing.T) {
	cardCount := 3
	height := cardCount + 6 + 10 // plenty of room
	b := newBoardWithCards(t, cardCount, height)

	view := b.View()
	if strings.Contains(view, "\u25b2") {
		t.Error("View should not contain up arrow indicator when all cards fit")
	}
	if strings.Contains(view, "\u25bc") {
		t.Error("View should not contain down arrow indicator when all cards fit")
	}
}

func TestView_DownIndicator_WhenCardsBelow(t *testing.T) {
	cardCount := 30
	height := 15 // panelHeight = 9, far fewer than 30 cards
	b := newBoardWithCards(t, cardCount, height)

	// ScrollOffset starts at 0, so there are cards below.
	view := b.View()
	if !strings.Contains(view, "\u25bc") {
		t.Error("View should contain down arrow indicator when more cards are below the viewport")
	}
	if strings.Contains(view, "\u25b2") {
		t.Error("View should not contain up arrow indicator when at the top")
	}
}

func TestView_UpIndicator_WhenCardsAbove(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll all the way down.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	view := b.View()
	if !strings.Contains(view, "\u25b2") {
		t.Error("View should contain up arrow indicator when cards are above the viewport")
	}
}

func TestView_BothIndicators_WhenMiddle(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Navigate to somewhere in the middle.
	panelHeight := height - 6
	for i := 0; i < panelHeight+2; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	view := b.View()
	if !strings.Contains(view, "\u25b2") {
		t.Error("View should contain up arrow indicator when scrolled to middle")
	}
	if !strings.Contains(view, "\u25bc") {
		t.Error("View should contain down arrow indicator when scrolled to middle")
	}
}

// --- Title Wrapping in View ---

func TestView_WrapsLongTitle(t *testing.T) {
	longTitle := "This is a very long title that should definitely be wrapped in the card list panel"
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "", "", "", false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: longTitle, Labels: []string{"test"}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 80
	board.Height = 30

	view := board.View()

	// The full long title text should appear in the view (wrapped, not truncated).
	// Check that all words from the title are present somewhere in the view.
	for _, word := range strings.Fields(longTitle) {
		if !strings.Contains(view, word) {
			t.Errorf("View should contain word %q from the long title, but it does not", word)
		}
	}

	// There should be no truncation marker.
	if strings.Contains(view, "...") {
		t.Error("View should NOT contain '...' — titles should be wrapped, not truncated")
	}
}

func TestView_WrappedTitles_SelectedCardAllLinesStyled(t *testing.T) {
	longTitle := "This is a card title that is long enough to require wrapping across lines"
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "", "", "", false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: longTitle, Labels: []string{"test"}},
				{Number: 2, Title: "Short", Labels: []string{"test"}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 80
	board.Height = 30

	view := board.View()

	// All words from the wrapped title of the selected card should appear in the view.
	for _, word := range strings.Fields(longTitle) {
		if !strings.Contains(view, word) {
			t.Errorf("View should contain word %q from the selected card's wrapped title", word)
		}
	}
}

func TestScroll_WrappedTitles_CursorCardFullyVisible(t *testing.T) {
	// Create cards with long titles that will wrap, filling more visual lines.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "", "", "", false)

	var cards []provider.Card
	for i := 0; i < 15; i++ {
		cards = append(cards, provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf("Card %d with a long title that should wrap to multiple lines in the panel", i+1),
			Labels: []string{"test"},
		})
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 80
	board.Height = 20

	// Navigate down to a card near the bottom.
	for i := 0; i < 10; i++ {
		board = sendKey(t, board, keyMsg("j"))
	}

	col := board.Columns[board.ActiveTab]
	// Cursor should be at the card we navigated to.
	if col.Cursor != 10 {
		t.Errorf("Cursor = %d, want 10", col.Cursor)
	}

	// ScrollOffset should have adjusted (non-zero, since wrapped titles take more space).
	if col.ScrollOffset <= 0 {
		t.Errorf("ScrollOffset = %d, want > 0 when navigating to card 10 with wrapped titles", col.ScrollOffset)
	}
}

func TestView_WrappedTitles_PartialCardHidden(t *testing.T) {
	// Create a board where cards have titles long enough to wrap.
	// With limited height, the last card that would only partially fit should be hidden.
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "", "", "", false)

	var cards []provider.Card
	for i := 0; i < 10; i++ {
		cards = append(cards, provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf("Card %d with a very long title that wraps to take more vertical space", i+1),
			Labels: []string{"test"},
		})
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 60  // narrow width to force wrapping
	board.Height = 15 // short height to force some cards off-screen

	view := board.View()

	// With wrapping enabled and limited height, not all 10 cards should be fully visible.
	// Count how many card numbers appear in the view.
	visibleCount := 0
	for i := 0; i < 10; i++ {
		marker := fmt.Sprintf("#%d ", i+1)
		if strings.Contains(view, marker) {
			visibleCount++
		}
	}

	if visibleCount >= 10 {
		t.Errorf("expected fewer than 10 cards visible with wrapped titles in limited height, got %d", visibleCount)
	}
}

// --- Resize Behavior ---

func TestScroll_ResizeClampsOffset(t *testing.T) {
	cardCount := 30
	height := 15
	b := newBoardWithCards(t, cardCount, height)

	// Scroll down.
	for i := 0; i < cardCount-1; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	offsetBefore := b.Columns[b.ActiveTab].ScrollOffset
	if offsetBefore <= 0 {
		t.Fatalf("expected ScrollOffset > 0 before resize, got %d", offsetBefore)
	}

	// Resize to a much larger height (all cards fit now).
	largeHeight := cardCount + 6 + 10
	m, _ := b.Update(tea.WindowSizeMsg{Width: 120, Height: largeHeight})
	b = m.(Board)

	col := b.Columns[b.ActiveTab]
	// With all cards fitting, the scroll offset should be clamped so we don't
	// have unnecessary blank space at the top.
	newPanelHeight := largeHeight - 6
	maxOffset := cardCount - newPanelHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if col.ScrollOffset > maxOffset {
		t.Errorf("ScrollOffset = %d after resize to large height, want <= %d (clamped)", col.ScrollOffset, maxOffset)
	}
}

// --- Status Bar: Refresh ---

func TestNormalMode_R_RefreshesBoard(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Press 'r' in normalMode to trigger a refresh.
	m, cmd := b.Update(keyMsg("r"))
	updated := m.(Board)

	// Should transition to loadingMode.
	if updated.mode != loadingMode {
		t.Errorf("mode = %d after 'r' in normalMode, want %d (loadingMode)", updated.mode, loadingMode)
	}

	// Should return a non-nil cmd (spinner tick + fetch).
	if cmd == nil {
		t.Error("'r' in normalMode should return a non-nil cmd for refresh")
	}
}

func TestBoardFetched_AfterRefresh_ShowsMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Press 'r' to trigger refresh (transitions to loadingMode).
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Simulate the board being fetched again (this is a refresh, not first load).
	board, err := provider.NewFakeProvider().FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ = b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, "Board refreshed") {
		t.Errorf("View() after refresh should contain %q, got:\n%s", "Board refreshed", view)
	}
}

func TestClearStatusMsg_ClearsTimedMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Press 'r' to trigger refresh.
	m, _ := b.Update(keyMsg("r"))
	b = m.(Board)

	// Simulate board fetched (triggers "Board refreshed" message).
	board, err := provider.NewFakeProvider().FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ = b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// Verify "Board refreshed" is visible before clearing (precondition).
	viewBefore := b.View()
	if !strings.Contains(viewBefore, "Board refreshed") {
		t.Fatalf("precondition: View() should contain %q before clearStatusMsg", "Board refreshed")
	}

	// Send clearStatusMsg to clear the timed message.
	m, _ = b.Update(clearStatusMsg{})
	b = m.(Board)

	view := b.View()
	if strings.Contains(view, "Board refreshed") {
		t.Errorf("View() after clearStatusMsg should NOT contain %q", "Board refreshed")
	}

	// Normal hints should be restored.
	if !strings.Contains(view, "n: New") {
		t.Errorf("View() after clearStatusMsg should contain %q (hints restored)", "n: New")
	}
}

// --- Status Bar: Mode-Specific Hints ---

func TestErrorMode_StatusBarShowsRetryAndQuit(t *testing.T) {
	b := newTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Transition to errorMode.
	m, _ := b.Update(boardFetchErrorMsg{err: fmt.Errorf("connection failed")})
	b = m.(Board)

	view := b.View()

	// Should show retry and quit hints.
	if !strings.Contains(view, "r: Retry") {
		t.Errorf("View() in errorMode should contain %q", "r: Retry")
	}
	if !strings.Contains(view, "q: Quit") {
		t.Errorf("View() in errorMode should contain %q", "q: Quit")
	}

	// Should NOT show normalMode hints.
	if strings.Contains(view, "n: New") {
		t.Errorf("View() in errorMode should NOT contain %q", "n: New")
	}
}

func TestCreateMode_StatusBarShowsEscapeHint(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Enter createMode.
	b = sendKey(t, b, keyMsg("n"))
	view := b.View()

	if !strings.Contains(view, "esc: Cancel") {
		t.Errorf("View() in createMode should contain %q, got:\n%s", "esc: Cancel", view)
	}
}

// --- Action Execution ---

// newActionTestBoard creates a loaded Board with the given actions and a FakeExecutor.
// It returns the board and the FakeExecutor for assertion.
func newActionTestBoard(t *testing.T, actions map[string]config.Action) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, fe, "matteobortolazzo", "lazyboards", "github", false)
	// Load the board.
	board, err := p.FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	loaded := m.(Board)
	loaded.Width = 120
	loaded.Height = 40
	return loaded, fe
}

func TestAction_URLTriggersOpenURL(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Get the selected card's number to verify expansion.
	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedURL := fmt.Sprintf("https://example.com/%d", selectedCard.Number)

	// Press the action key in normalMode.
	b = sendKey(t, b, keyMsg("o"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

func TestAction_ShellTriggersRunShell(t *testing.T) {
	actions := map[string]config.Action{
		"s": {Name: "Shell", Type: "shell", Command: "echo {title}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Get the selected card's title (slugified) to verify expansion.
	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedCmd := "echo " + action.ShellEscape(action.Slugify(selectedCard.Title))

	// Press the action key in normalMode -- shell runs async via tea.Cmd.
	m, cmd := b.Update(keyMsg("s"))
	b = m.(Board)
	_ = b

	// Execute the returned cmd(s) to trigger RunShell.
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called, but no calls recorded")
	}
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
}

func TestAction_IgnoredInCreateMode(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter createMode, then press the action key.
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, keyMsg("o"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls in createMode, got %d", len(fe.OpenURLCalls))
	}
}

func TestAction_IgnoredInLoadingMode(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, fe, "", "", "", false)

	// Board starts in loadingMode. Press the action key.
	b = sendKey(t, b, keyMsg("o"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls in loadingMode, got %d", len(fe.OpenURLCalls))
	}
}

func TestAction_IgnoredWhenNoCards(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, fe, "", "", "", false)

	// Load a board with an empty column.
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Empty", Cards: nil},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Press the action key with no cards in the column.
	b = sendKey(t, b, keyMsg("o"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls when no cards, got %d", len(fe.OpenURLCalls))
	}
}

func TestAction_ShellSuccess_ShowsDone(t *testing.T) {
	actions := map[string]config.Action{
		"s": {Name: "Shell", Type: "shell", Command: "echo {title}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Trigger the shell action.
	b = sendKey(t, b, keyMsg("s"))

	// Simulate the async result with success.
	m, _ := b.Update(actionResultMsg{success: true, message: "Done"})
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, "Done") {
		t.Errorf("View() after successful shell action should contain %q", "Done")
	}
}

func TestAction_ShellError_ShowsError(t *testing.T) {
	actions := map[string]config.Action{
		"s": {Name: "Shell", Type: "shell", Command: "failing-cmd"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Trigger the shell action.
	b = sendKey(t, b, keyMsg("s"))

	// Simulate the async result with failure.
	m, _ := b.Update(actionResultMsg{success: false, message: "Error: exit 1"})
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, "Error:") {
		t.Errorf("View() after failed shell action should contain %q", "Error:")
	}
}

func TestAction_HintsShowInStatusBar(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, _ := newActionTestBoard(t, actions)

	view := b.View()
	if !strings.Contains(view, "o: Open") {
		t.Errorf("View() should contain action hint %q in the status bar", "o: Open")
	}
}

func TestAction_URLError_ShowsErrorInStatusBar(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)
	fe.OpenURLErr = errors.New("failed to open browser")

	// Press the action key.
	m, cmd := b.Update(keyMsg("o"))
	b = m.(Board)

	// Should return a cmd for the timed status message.
	if cmd == nil {
		t.Error("OpenURL error should return a non-nil cmd for status message")
	}

	view := b.View()
	if !strings.Contains(view, "Error:") {
		t.Errorf("View() after OpenURL error should contain %q, got:\n%s", "Error:", view)
	}
}

func TestAction_TemplateVarsExpanded(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://gh.com/{repo_owner}/{repo_name}/issues/{number}"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, fe, "matteobortolazzo", "lazyboards", "github", false)

	// Load a board with a specific card that has known labels.
	cardNumber := 42
	cardTitle := "Add custom actions"
	cardLabels := []string{"bug", "enhancement"}
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "New", Cards: []provider.Card{
				{Number: cardNumber, Title: cardTitle, Labels: cardLabels},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Press the action key.
	b = sendKey(t, b, keyMsg("o"))
	_ = b

	expectedURL := fmt.Sprintf("https://gh.com/matteobortolazzo/lazyboards/issues/%d", cardNumber)
	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

// --- Config Mode: Entry/Exit ---

func TestConfigMode_C_EntersConfigMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	if b.mode != configMode {
		t.Errorf("after 'c': mode = %d, want %d (configMode)", b.mode, configMode)
	}
}

func TestConfigMode_Escape_ReturnsToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	if b.mode != normalMode {
		t.Errorf("after 'c' then Escape: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

// --- Config Mode: View Rendering ---

func TestConfigMode_ViewShowsConfigurationHeader(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("c"))
	view := b.View()
	if !strings.Contains(view, "Configuration") {
		t.Error("View() in configMode should contain 'Configuration'")
	}
}

func TestConfigMode_ViewShowsProviderField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("c"))
	view := b.View()
	if !strings.Contains(view, "Provider") {
		t.Error("View() in configMode should contain 'Provider' label")
	}
	if !strings.Contains(view, "github") {
		t.Error("View() in configMode should show 'github' as a provider option")
	}
}

func TestConfigMode_ViewShowsRepoField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("c"))
	view := b.View()
	if !strings.Contains(view, "Repo") {
		t.Error("View() in configMode should contain 'Repo' label")
	}
}

// --- Config Mode: Provider Cycling ---

func TestConfigMode_LeftRight_CyclesProvider(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Record initial provider index.
	initialIndex := b.providerIndex

	// Press Right to cycle to next provider.
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.providerIndex == initialIndex {
		t.Error("Right arrow in configMode should change providerIndex")
	}
}

func TestConfigMode_ProviderWrapsAround(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Cycle through all providers and one more to wrap around.
	totalProviders := len(b.providerOptions)
	for i := 0; i < totalProviders; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyRight))
	}

	// Should wrap back to the first provider.
	if b.providerIndex != 0 {
		t.Errorf("providerIndex = %d after wrapping around %d providers, want 0", b.providerIndex, totalProviders)
	}
}

// --- Config Mode: Tab Navigation ---

func TestConfigMode_TabSwitchesFocus(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Initially focus should be on provider field (configFocus == 0).
	if b.configFocus != 0 {
		t.Errorf("configFocus = %d on entering configMode, want 0 (provider field)", b.configFocus)
	}

	// Tab should switch focus to repo field.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.configFocus != 1 {
		t.Errorf("configFocus = %d after Tab, want 1 (repo field)", b.configFocus)
	}

	// Another Tab should switch back to provider field.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.configFocus != 0 {
		t.Errorf("configFocus = %d after second Tab, want 0 (provider field)", b.configFocus)
	}
}

// --- Config Mode: Typing ---

func TestConfigMode_TypingUpdatesRepoField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Tab to repo field.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	// Type characters.
	for _, ch := range "owner/repo" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	if b.repoInput.Value() != "owner/repo" {
		t.Errorf("repoInput.Value() = %q, want %q", b.repoInput.Value(), "owner/repo")
	}
}

// --- Config Mode: Save (Enter) ---

func TestConfigMode_Enter_TriggersConfigSave(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Tab to repo field and type a value.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	for _, ch := range "owner/repo" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to save.
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	// Enter should trigger a save command (async).
	if cmd == nil {
		t.Error("Enter in configMode should return a non-nil cmd (config save)")
	}
}

func TestConfigMode_ConfigSaved_TransitionsToLoadingMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Send configSavedMsg to simulate successful save.
	m, cmd := b.Update(configSavedMsg{})
	b = m.(Board)

	// Should transition to loadingMode (auto-refresh after save).
	if b.mode != loadingMode {
		t.Errorf("mode = %d after configSavedMsg, want %d (loadingMode)", b.mode, loadingMode)
	}

	// Should return a cmd for fetching the board.
	if cmd == nil {
		t.Error("configSavedMsg should return a non-nil cmd (fetch board)")
	}
}

func TestConfigMode_ConfigSaveError_ShowsValidationError(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Send configSaveErrorMsg to simulate save failure.
	m, _ := b.Update(configSaveErrorMsg{err: errors.New("permission denied")})
	b = m.(Board)

	// Should stay in configMode.
	if b.mode != configMode {
		t.Errorf("mode = %d after configSaveErrorMsg, want %d (configMode)", b.mode, configMode)
	}

	// Should show the error.
	if !strings.Contains(b.validationErr, "permission denied") {
		t.Errorf("validationErr = %q, want it to contain %q", b.validationErr, "permission denied")
	}
}

// --- Config Mode: Blocking ---

func TestConfigMode_BlocksNavigation(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	b = sendKey(t, b, keyMsg("c"))

	origTab := b.ActiveTab
	origCursor := b.Columns[b.ActiveTab].Cursor

	// h, l should NOT navigate the board tabs.
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	if b.ActiveTab != origTab {
		t.Errorf("'h' in configMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != origTab {
		t.Errorf("'l' in configMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}

	// j, k should NOT move the card cursor.
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("'j' in configMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
	b = sendKey(t, b, keyMsg("k"))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("'k' in configMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
}

func TestConfigMode_BlocksQuit(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	m, _ := b.Update(keyMsg("q"))
	updated := m.(Board)
	// q should NOT quit while in configMode.
	if updated.mode != configMode {
		t.Errorf("'q' in configMode changed mode to %d, want %d (configMode)", updated.mode, configMode)
	}
}

func TestConfigMode_CtrlC_StillQuits(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in configMode should return a non-nil Cmd (tea.Quit)")
	}
}

// --- Config Mode: Status Bar ---

func TestConfigMode_StatusBarShowsHints(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("c"))
	view := b.View()

	expectedHints := []string{"esc", "tab", "enter"}
	for _, hint := range expectedHints {
		if !strings.Contains(strings.ToLower(view), hint) {
			t.Errorf("View() in configMode should contain hint %q", hint)
		}
	}
}

func TestConfigMode_EnterWithEmptyRepo_ShowsValidationError(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Press Enter without typing a repo value.
	m, _ := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	// Should stay in configMode with a validation error.
	if b.mode != configMode {
		t.Errorf("mode = %d, want %d (configMode) when repo is empty", b.mode, configMode)
	}
	if !strings.Contains(b.validationErr, "Repository is required") {
		t.Errorf("validationErr = %q, want it to contain %q", b.validationErr, "Repository is required")
	}
}

func TestNormalMode_StatusBarShowsConfigHint(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()
	if !strings.Contains(view, "c: Config") {
		t.Errorf("View() status bar does not contain %q", "c: Config")
	}
}

// --- First-Launch Flow ---

func TestFirstLaunch_StartsInConfigMode(t *testing.T) {
	b := NewBoard(nil, nil, nil, "owner", "repo", "github", true)
	if b.mode != configMode {
		t.Errorf("mode = %d, want %d (configMode) for firstLaunch board", b.mode, configMode)
	}
}

func TestFirstLaunch_PrePopulatesProvider(t *testing.T) {
	b := NewBoard(nil, nil, nil, "owner", "repo", "github", true)
	if b.providerOptions[b.providerIndex] != "github" {
		t.Errorf("providerOptions[providerIndex] = %q, want %q", b.providerOptions[b.providerIndex], "github")
	}
}

func TestFirstLaunch_PrePopulatesProviderAzure(t *testing.T) {
	b := NewBoard(nil, nil, nil, "owner", "repo", "azure-devops", true)
	if b.providerOptions[b.providerIndex] != "azure-devops" {
		t.Errorf("providerOptions[providerIndex] = %q, want %q", b.providerOptions[b.providerIndex], "azure-devops")
	}
}

func TestFirstLaunch_PrePopulatesRepo(t *testing.T) {
	b := NewBoard(nil, nil, nil, "myowner", "myrepo", "github", true)
	if b.repoInput.Value() != "myowner/myrepo" {
		t.Errorf("repoInput.Value() = %q, want %q", b.repoInput.Value(), "myowner/myrepo")
	}
}

func TestFirstLaunch_EmptyRepoNotPrePopulated(t *testing.T) {
	b := NewBoard(nil, nil, nil, "", "", "github", true)
	if b.repoInput.Value() != "" {
		t.Errorf("repoInput.Value() = %q, want empty when no repo detected", b.repoInput.Value())
	}
}

func TestFirstLaunch_Init_ReturnsNil(t *testing.T) {
	b := NewBoard(nil, nil, nil, "owner", "repo", "github", true)
	cmd := b.Init()
	if cmd != nil {
		t.Error("Init() should return nil for firstLaunch board (no fetch)")
	}
}

func TestFirstLaunch_Escape_Quits(t *testing.T) {
	b := NewBoard(nil, nil, nil, "owner", "repo", "github", true)
	_, cmd := b.Update(arrowMsg(tea.KeyEsc))
	if cmd == nil {
		t.Error("Escape in firstLaunch configMode should return a quit cmd")
	}
}

func TestFirstLaunch_Escape_ConfigSavedIsFalse(t *testing.T) {
	b := NewBoard(nil, nil, nil, "owner", "repo", "github", true)
	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	updated := m.(Board)
	if updated.ConfigSaved {
		t.Error("ConfigSaved should be false after Escape in firstLaunch")
	}
}

func TestFirstLaunch_ConfigSaved_SetsConfigSavedAndQuits(t *testing.T) {
	b := NewBoard(nil, nil, nil, "owner", "repo", "github", true)
	m, cmd := b.Update(configSavedMsg{})
	updated := m.(Board)
	if !updated.ConfigSaved {
		t.Error("ConfigSaved should be true after configSavedMsg in firstLaunch")
	}
	if cmd == nil {
		t.Error("configSavedMsg in firstLaunch should return a quit cmd")
	}
}

func TestFirstLaunch_ViewShowsConfigModal(t *testing.T) {
	b := NewBoard(nil, nil, nil, "owner", "repo", "github", true)
	b.Width = 120
	b.Height = 40
	view := b.View()
	if !strings.Contains(view, "Configuration") {
		t.Error("View() in firstLaunch should show Configuration modal")
	}
}

// --- Config Mode: Pre-populate from runtime (normal "c" key) ---

func TestConfigMode_PrePopulatesProviderFromRuntime(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "owner", "repo", "github", false)
	board, _ := p.FetchBoard(nil)
	m, _ := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// Press "c" to enter configMode.
	b = sendKey(t, b, keyMsg("c"))
	if b.providerOptions[b.providerIndex] != "github" {
		t.Errorf("providerOptions[providerIndex] = %q after 'c', want %q", b.providerOptions[b.providerIndex], "github")
	}
}

func TestConfigMode_PrePopulatesRepoFromRuntime(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "myowner", "myrepo", "github", false)
	board, _ := p.FetchBoard(nil)
	m, _ := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// Press "c" to enter configMode.
	b = sendKey(t, b, keyMsg("c"))
	if b.repoInput.Value() != "myowner/myrepo" {
		t.Errorf("repoInput.Value() = %q after 'c', want %q", b.repoInput.Value(), "myowner/myrepo")
	}
}

// --- Detail Panel: Card Body ---

// newBoardWithBody creates a Board with one column containing two cards.
// The first card has body1 as its body text; the second card has body2.
func newBoardWithBody(t *testing.T, body1, body2 string) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "", "", "", false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "Card One", Labels: []string{"bug"}, Body: body1},
				{Number: 2, Title: "Card Two", Labels: []string{"feature"}, Body: body2},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

func TestView_DetailPanelShowsCardBody(t *testing.T) {
	bodyText := "This is the card description with important details."
	b := newBoardWithBody(t, bodyText, "other body")

	view := b.View()

	// The detail panel should display the body text of the selected card.
	if !strings.Contains(view, bodyText) {
		t.Errorf("View() detail panel does not contain card body %q", bodyText)
	}
}

func TestView_DetailPanelEmptyBody_NoExtraSpace(t *testing.T) {
	b := newBoardWithBody(t, "", "")

	view := b.View()

	// With an empty body, the view should still render without errors.
	// The detail panel should show the card title and labels but no body content.
	selectedCard := b.Columns[b.ActiveTab].Cards[0]
	titleStr := fmt.Sprintf("#%d %s", selectedCard.Number, selectedCard.Title)
	if !strings.Contains(view, titleStr) {
		t.Errorf("View() detail panel does not contain card title %q", titleStr)
	}

	// Count occurrences of consecutive newlines in the detail area.
	// An empty body should not produce extra blank lines (e.g., "\n\n\n").
	if strings.Contains(view, "\n\n\n") {
		t.Error("View() detail panel has excessive blank lines when body is empty")
	}
}

func TestView_DetailPanelBodyUpdatesOnNavigation(t *testing.T) {
	firstBody := "Description of the first card."
	secondBody := "Description of the second card."
	b := newBoardWithBody(t, firstBody, secondBody)

	// Initially, the first card is selected.
	view := b.View()
	if !strings.Contains(view, firstBody) {
		t.Errorf("View() detail panel does not contain first card body %q", firstBody)
	}

	// Navigate down to the second card.
	b = sendKey(t, b, keyMsg("j"))
	view = b.View()

	// The second card's body should now appear.
	if !strings.Contains(view, secondBody) {
		t.Errorf("View() detail panel does not contain second card body %q after navigation", secondBody)
	}

	// The first card's body should no longer be visible (it's unique text).
	if strings.Contains(view, firstBody) {
		t.Errorf("View() detail panel still contains first card body %q after navigating away", firstBody)
	}
}

// --- Detail Panel Focus: Focus Switching ---

func TestDetailFocus_LKey_FocusesDetailPanel(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Press 'l' to focus the detail panel.
	b = sendKey(t, b, keyMsg("l"))

	if !b.detailFocused {
		t.Error("after 'l': detailFocused should be true")
	}
}

func TestDetailFocus_HKey_ReturnsFocusToCardList(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus with 'l', then exit with 'h'.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("h"))

	if b.detailFocused {
		t.Error("after 'l' then 'h': detailFocused should be false")
	}
}

func TestDetailFocus_Escape_ReturnsFocusToCardList(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus with 'l', then exit with Escape.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.detailFocused {
		t.Error("after 'l' then Escape: detailFocused should be false")
	}
}

func TestDetailFocus_Tab_ReturnsFocusAndSwitchesColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	initialTab := b.ActiveTab

	// Enter detail focus with 'l', then press Tab.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	if b.detailFocused {
		t.Error("after Tab in detail focus: detailFocused should be false")
	}
	if b.ActiveTab != initialTab+1 {
		t.Errorf("after Tab in detail focus: ActiveTab = %d, want %d", b.ActiveTab, initialTab+1)
	}
}

func TestDetailFocus_ShiftTab_ReturnsFocusAndSwitchesColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Move to column 1 first so Shift+Tab can decrement.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != 1 {
		t.Fatalf("precondition: ActiveTab = %d, want 1", b.ActiveTab)
	}

	// Enter detail focus with 'l', then press Shift+Tab.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))

	if b.detailFocused {
		t.Error("after Shift+Tab in detail focus: detailFocused should be false")
	}
	if b.ActiveTab != 0 {
		t.Errorf("after Shift+Tab in detail focus: ActiveTab = %d, want 0", b.ActiveTab)
	}
}

// --- Detail Panel Focus: Scroll ---

func TestDetailFocus_JKey_ScrollsDown(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Record the card cursor before entering detail focus.
	cursorBefore := b.Columns[b.ActiveTab].Cursor

	// Enter detail focus, then press 'j' to scroll down.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("j"))

	// detailScrollOffset should increment.
	if b.detailScrollOffset < 1 {
		t.Errorf("detailScrollOffset = %d after 'j' in detail focus, want >= 1", b.detailScrollOffset)
	}

	// Card cursor should NOT change when in detail focus.
	cursorAfter := b.Columns[b.ActiveTab].Cursor
	if cursorAfter != cursorBefore {
		t.Errorf("card cursor changed from %d to %d during detail scroll, want unchanged", cursorBefore, cursorAfter)
	}
}

func TestDetailFocus_KKey_ScrollsUp(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus, scroll down twice, then scroll up once.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	offsetAfterDown := b.detailScrollOffset

	b = sendKey(t, b, keyMsg("k"))

	if b.detailScrollOffset >= offsetAfterDown {
		t.Errorf("detailScrollOffset = %d after 'k', want less than %d", b.detailScrollOffset, offsetAfterDown)
	}
}

func TestDetailFocus_KKey_ClampsAtZero(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus and press 'k' without scrolling down first.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("k"))

	if b.detailScrollOffset < 0 {
		t.Errorf("detailScrollOffset = %d after 'k' at top, want >= 0 (should not go negative)", b.detailScrollOffset)
	}
}

func TestDetailFocus_ScrollOffsetResetsOnCardChange(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus, scroll down.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))

	if b.detailScrollOffset == 0 {
		t.Fatal("precondition: detailScrollOffset should be > 0 after scrolling")
	}

	// Exit detail focus with 'h', then navigate to a different card with 'j'.
	b = sendKey(t, b, keyMsg("h"))
	b = sendKey(t, b, keyMsg("j"))

	if b.detailScrollOffset != 0 {
		t.Errorf("detailScrollOffset = %d after changing card, want 0 (should reset)", b.detailScrollOffset)
	}
}

func TestDetailFocus_ScrollOffsetResetsOnRefresh(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus, scroll down.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))

	if b.detailScrollOffset == 0 {
		t.Fatal("precondition: detailScrollOffset should be > 0 after scrolling")
	}

	// Exit detail focus and refresh.
	b = sendKey(t, b, keyMsg("h"))
	b = sendKey(t, b, keyMsg("r"))

	// Simulate the board being fetched again.
	p := provider.NewFakeProvider()
	board, err := p.FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	if b.detailScrollOffset != 0 {
		t.Errorf("detailScrollOffset = %d after board refresh, want 0 (should reset)", b.detailScrollOffset)
	}
}

// --- Detail Panel Focus: View ---

func TestView_DetailFocused_BorderHighlighted(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))

	// When detail panel is focused, the view should render.
	// We verify that the model state is set correctly.
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	view := b.View()
	// The view should render without panic when detailFocused is true.
	if strings.TrimSpace(view) == "" {
		t.Error("View() should not be empty when detail panel is focused")
	}
}

func TestView_DetailUnfocused_BorderDim(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Without entering detail focus, the default state.
	if b.detailFocused {
		t.Fatal("precondition: detailFocused should be false by default")
	}

	view := b.View()
	if strings.TrimSpace(view) == "" {
		t.Error("View() should not be empty in default (unfocused) state")
	}
}

func TestView_DetailFocused_StatusBarShowsDetailHints(t *testing.T) {
	b := newBoardWithBody(t, "Some body", "Other body")

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))

	view := b.View()

	// Status bar should show detail-specific hints.
	if !strings.Contains(view, "j/k: Scroll") {
		t.Errorf("View() in detail focus should contain %q in status bar", "j/k: Scroll")
	}
	if !strings.Contains(view, "h: Back") {
		t.Errorf("View() in detail focus should contain %q in status bar", "h: Back")
	}

	// Normal-mode hints should NOT appear.
	if strings.Contains(view, "n: New") {
		t.Errorf("View() in detail focus should NOT contain normal hint %q", "n: New")
	}
}

func TestView_GlamourRendersMarkdown(t *testing.T) {
	markdownBody := "This has **bold** text and a list:\n- item one\n- item two"
	b := newBoardWithBody(t, markdownBody, "")

	// Enter detail focus to trigger glamour rendering.
	b = sendKey(t, b, keyMsg("l"))

	view := b.View()

	// The raw markdown syntax should NOT appear.
	if strings.Contains(view, "**bold**") {
		t.Error("View() should not contain raw markdown '**bold**' - glamour should render it")
	}

	// The word "bold" should still be present (rendered without markdown syntax).
	if !strings.Contains(view, "bold") {
		t.Error("View() should contain the word 'bold' (rendered from markdown)")
	}
}

// --- Fix: Scroll offset upper bound ---

func TestDetailFocus_JKey_ClampsAtMaxLines(t *testing.T) {
	// Use a short body so we can verify scrolling stops at the end.
	shortBody := "line one\nline two"
	b := newBoardWithBody(t, shortBody, "")

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))

	// Press 'j' many times (more than the number of lines).
	for i := 0; i < 100; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	// The offset should be capped; it should not grow unboundedly.
	// With a 2-line body, the offset should not exceed the line count.
	bodyLineCount := strings.Count(shortBody, "\n") + 1
	if b.detailScrollOffset > bodyLineCount {
		t.Errorf("detailScrollOffset = %d after excessive scrolling, want <= %d (body line count)", b.detailScrollOffset, bodyLineCount)
	}
}

// --- Fix: Tab/Shift+Tab at column boundaries ---

func TestDetailFocus_Tab_AtLastColumn_StaysFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Navigate to the last column.
	lastCol := len(b.Columns) - 1
	for b.ActiveTab < lastCol {
		b = sendKey(t, b, arrowMsg(tea.KeyTab))
	}
	if b.ActiveTab != lastCol {
		t.Fatalf("precondition: ActiveTab = %d, want %d (last column)", b.ActiveTab, lastCol)
	}

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	// Press Tab at the last column boundary.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	// Should stay on last column and remain in detail focus.
	if b.ActiveTab != lastCol {
		t.Errorf("after Tab at last column: ActiveTab = %d, want %d (should not change)", b.ActiveTab, lastCol)
	}
	if !b.detailFocused {
		t.Error("after Tab at last column: detailFocused should remain true (no column to switch to)")
	}
}

func TestDetailFocus_ShiftTab_AtFirstColumn_StaysFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	if b.ActiveTab != 0 {
		t.Fatalf("precondition: ActiveTab = %d, want 0 (first column)", b.ActiveTab)
	}

	// Enter detail focus.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	// Press Shift+Tab at the first column boundary.
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))

	// Should stay on first column and remain in detail focus.
	if b.ActiveTab != 0 {
		t.Errorf("after Shift+Tab at first column: ActiveTab = %d, want 0 (should not change)", b.ActiveTab)
	}
	if !b.detailFocused {
		t.Error("after Shift+Tab at first column: detailFocused should remain true (no column to switch to)")
	}
}
