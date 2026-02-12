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
	return NewBoard(p, nil, nil, "", "", "")
}

// newLoadedTestBoard creates a Board and sends a boardFetchedMsg to transition
// it to normalMode with populated columns (simulating a successful fetch).
func newLoadedTestBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "", "", "")
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
	b = sendKey(t, b, keyMsg("h"))
	b = sendKey(t, b, keyMsg("l"))
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
	b = sendKey(t, b, keyMsg("l"))
	if b.ActiveTab != 1 {
		t.Errorf("after 'l': ActiveTab = %d, want 1", b.ActiveTab)
	}
}

func TestTabNavigation_H_MovesLeft(t *testing.T) {
	b := newLoadedTestBoard(t)
	// Move right first so we can move left
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("h"))
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
	b = sendKey(t, b, keyMsg("h"))
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
		b = sendKey(t, b, keyMsg("l"))
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
	b = sendKey(t, b, keyMsg("l"))
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
	b := NewBoard(p, nil, nil, "", "", "")

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

// --- truncateTitle Unit Tests ---

func TestTruncateTitle_ShortTitleUnchanged(t *testing.T) {
	title := "Short"
	maxWidth := 20
	got := truncateTitle(title, maxWidth)
	if got != title {
		t.Errorf("truncateTitle(%q, %d) = %q, want %q (unchanged)", title, maxWidth, got, title)
	}
}

func TestTruncateTitle_ExactWidthUnchanged(t *testing.T) {
	title := "Exactly ten"
	maxWidth := len(title)
	got := truncateTitle(title, maxWidth)
	if got != title {
		t.Errorf("truncateTitle(%q, %d) = %q, want %q (unchanged at exact width)", title, maxWidth, got, title)
	}
}

func TestTruncateTitle_ExceedingWidthTruncated(t *testing.T) {
	title := "This is a very long title that should be truncated"
	maxWidth := 20
	got := truncateTitle(title, maxWidth)

	// Should end with "..."
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncateTitle(%q, %d) = %q, want suffix %q", title, maxWidth, got, "...")
	}

	// Total length should be exactly maxWidth runes.
	if len([]rune(got)) != maxWidth {
		t.Errorf("truncateTitle(%q, %d) has %d runes, want %d", title, maxWidth, len([]rune(got)), maxWidth)
	}

	// Prefix before "..." should be the first maxWidth-3 runes of the original.
	expectedPrefix := string([]rune(title)[:maxWidth-3])
	if !strings.HasPrefix(got, expectedPrefix) {
		t.Errorf("truncateTitle(%q, %d) prefix = %q, want %q", title, maxWidth, got[:len(expectedPrefix)], expectedPrefix)
	}
}

func TestTruncateTitle_MaxWidthThreeOrLess(t *testing.T) {
	title := "Hello"

	// maxWidth = 3: should return "..."
	got := truncateTitle(title, 3)
	if len([]rune(got)) > 3 {
		t.Errorf("truncateTitle(%q, 3) = %q, want at most 3 runes", title, got)
	}

	// maxWidth = 1: should not panic and return something short.
	got = truncateTitle(title, 1)
	if len([]rune(got)) > 1 {
		t.Errorf("truncateTitle(%q, 1) = %q, want at most 1 rune", title, got)
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
	b = sendKey(t, b, keyMsg("l"))

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

// --- Title Truncation in View ---

func TestView_TruncatesLongTitle(t *testing.T) {
	longTitle := "This is a very long title that should definitely be truncated in the card list panel"
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, "", "", "")

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

	// The full long title should NOT appear in the view.
	if strings.Contains(view, longTitle) {
		t.Error("View should not contain the full long title; it should be truncated")
	}

	// The truncation marker should appear.
	if !strings.Contains(view, "...") {
		t.Error("View should contain '...' for a truncated title")
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
	b := NewBoard(p, actions, fe, "matteobortolazzo", "lazyboards", "github")
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
	b := NewBoard(p, actions, fe, "", "", "")

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
	b := NewBoard(p, actions, fe, "", "", "")

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
	b := NewBoard(p, actions, fe, "matteobortolazzo", "lazyboards", "github")

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
