package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// expectedColumnCount is the number of Kanban columns in the board.
const expectedColumnCount = 4

// expectedColumnTitles are the Kanban column names from the spec.
var expectedColumnTitles = []string{"New", "Refined", "Implementing", "Implemented"}

// newTestBoard creates a Board in loadingMode using NewBoard.
func newTestBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	return NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)
}

// newLoadedTestBoard creates a Board and sends a boardFetchedMsg to transition
// it to normalMode with populated columns (simulating a successful fetch).
func newLoadedTestBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)
	// Simulate the provider returning board data.
	board, err := p.FetchBoard(context.TODO())
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

// simulateRefresh simulates a background refresh completing by fetching
// default board data from a FakeProvider and sending a boardFetchedMsg.
func simulateRefresh(t *testing.T, b Board) Board {
	t.Helper()
	board, err := provider.NewFakeProvider().FetchBoard(context.TODO())
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

// execCmds recursively executes a tea.Cmd, handling tea.BatchMsg.
// Uses a timeout to avoid blocking on tea.Tick commands.
func execCmds(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(100 * time.Millisecond):
		return // Skip blocking commands (e.g., tea.Tick)
	}
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

// loadFromFakeProvider fetches board data from the FakeProvider,
// sends it through Update, and sets standard test dimensions (120x40).
func loadFromFakeProvider(t *testing.T, b Board, p *provider.FakeProvider) Board {
	t.Helper()
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

// newCreatingTestBoard creates a Board in creatingMode for testing async creation.
func newCreatingTestBoard(t *testing.T) Board {
	t.Helper()
	b := newLoadedTestBoard(t)
	b.mode = creatingMode
	return b
}

// newBoardWithCards creates a Board with a single column containing cardCount
// cards, plus a second column with one card (for tab-switch tests).
// Width is set to 120 and Height to the given height parameter.
func newBoardWithCards(t *testing.T, cardCount, height int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	// Build provider cards.
	providerCards := make([]provider.Card, cardCount)
	for i := range providerCards {
		providerCards[i] = provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf("Card %d", i+1),
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

// newActionTestBoard creates a loaded Board with the given actions and a FakeExecutor.
// It returns the board and the FakeExecutor for assertion.
func newActionTestBoard(t *testing.T, actions map[string]config.Action) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false)
	return loadFromFakeProvider(t, b, p), fe
}

// newBoardWithBody creates a Board with one column containing two cards.
// The first card has body1 as its body text; the second card has body2.
func newBoardWithBody(t *testing.T, body1, body2 string) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "Card One", Labels: []provider.Label{{Name: "bug"}}, Body: body1},
				{Number: 2, Title: "Card Two", Labels: []provider.Label{{Name: "feature"}}, Body: body2},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// newBoardWithLongBody creates a board where the first card has a body with
// lineCount paragraphs (e.g., 50), which exceeds the visible panel area at Height=40,
// enabling scroll testing. Uses \n\n paragraph separators so glamour renders
// each as a distinct paragraph (single \n are soft breaks that glamour may collapse).
func newBoardWithLongBody(t *testing.T, lineCount int) Board {
	t.Helper()
	var lines []string
	for i := 1; i <= lineCount; i++ {
		lines = append(lines, fmt.Sprintf("scroll line %d", i))
	}
	longBody := strings.Join(lines, "\n\n")
	return newBoardWithBody(t, longBody, "Other body")
}

// newBoardWithCustomCard creates a board with a single card using the given title, labels, and body.
func newBoardWithCustomCard(t *testing.T, title string, labels []provider.Label, body string) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: title, Labels: labels, Body: body},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 80
	board.Height = 20
	return board
}

// newBoardWithGeneratedCards creates a Board with a single column containing
// count cards. Each card's title is generated from titleFmt (which must contain
// a %d placeholder for the card number). Width and Height are set to the given values.
func newBoardWithGeneratedCards(t *testing.T, count int, titleFmt string, width, height int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	cards := make([]provider.Card, count)
	for i := range cards {
		cards[i] = provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf(titleFmt, i+1),
			Labels: []provider.Label{{Name: "test"}},
		}
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = width
	board.Height = height
	return board
}

// newBoardWithInlineCards creates a Board with a single column containing the
// given cards. Width and Height are set to the given values.
func newBoardWithInlineCards(t *testing.T, cards []provider.Card, width, height int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = width
	board.Height = height
	return board
}

// newActionTestBoardWithColumns creates a loaded Board with the given actions
// and custom columns. It returns the board and the FakeExecutor for assertion.
func newActionTestBoardWithColumns(t *testing.T, actions map[string]config.Action, columns []provider.Column) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false)

	m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: columns}})
	loaded, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	loaded.Width = 120
	loaded.Height = 40
	return loaded, fe
}

// newColumnActionTestBoard creates a loaded Board with global actions AND
// per-column configs. It returns the board and FakeExecutor for assertion.
func newColumnActionTestBoard(t *testing.T, actions map[string]config.Action, columnConfigs []config.ColumnConfig) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, columnConfigs, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false)
	return loadFromFakeProvider(t, b, p), fe
}

// newBoardWithPRsAndExecutor creates a Board with one column containing three cards:
// - Card 1: no LinkedPRs
// - Card 2: 1 LinkedPR
// - Card 3: 2 LinkedPRs
// It also returns a FakeExecutor for asserting OpenURL/RunShell calls.
// newBoardWithPRs delegates to this function when the executor is not needed.
func newBoardWithPRsAndExecutor(t *testing.T) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, fe, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "No PRs", Labels: []provider.Label{{Name: "bug"}}},
				{Number: 2, Title: "One PR", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "feat: one PR", URL: "https://github.com/owner/repo/pull/10"},
				}},
				{Number: 3, Title: "Two PRs", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 20, Title: "feat: first PR", URL: "https://github.com/owner/repo/pull/20"},
					{Number: 21, Title: "feat: second PR", URL: "https://github.com/owner/repo/pull/21"},
				}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board, fe
}

// newBoardWithWorkingLabel creates a Board with one column containing four cards
// covering all combinations of "Working" label and linked PRs:
// - Card 1: No "Working" label, no PR (baseline — no indicators)
// - Card 2: Has "Working" label, no PR (Working indicator only)
// - Card 3: Has PR, no "Working" label (PR indicator only)
// - Card 4: Has both PR and "Working" label (both indicators)
func newBoardWithWorkingLabel(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "No indicators", Labels: []provider.Label{{Name: "bug"}}},
				{Number: 2, Title: "Working only", Labels: []provider.Label{{Name: "Working"}}},
				{Number: 3, Title: "PR only", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "feat: some PR", URL: "https://github.com/owner/repo/pull/10"},
				}},
				{Number: 4, Title: "Both indicators", Labels: []provider.Label{{Name: "Working"}, {Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 20, Title: "feat: another PR", URL: "https://github.com/owner/repo/pull/20"},
				}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// newBoardWithPRs creates a Board with one column containing three cards:
// - Card 1: no LinkedPRs
// - Card 2: 1 LinkedPR
// - Card 3: 2 LinkedPRs
func newBoardWithPRs(t *testing.T) Board {
	t.Helper()
	b, _ := newBoardWithPRsAndExecutor(t)
	return b
}

// newBoardWithCustomWorkingLabel creates a Board with one column containing
// cards with specific labels, and the board's workingLabel set to the given value.
// This tests the configurable working label feature (#113).
// - Card 1: label matches workingLabel (should show spinner)
// - Card 2: label "Working" (only shows spinner if workingLabel == "Working")
// - Card 3: label "bug" (baseline, never shows spinner)
func newBoardWithCustomWorkingLabel(t *testing.T, workingLabel string, cards []provider.Card) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, workingLabel, false, false)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// newBoardWithAssignees creates a Board with one column containing one card.
// The card has the given assignee logins. If no logins are provided, the card
// has no assignees.
func newBoardWithAssignees(t *testing.T, assigneeLogins ...string) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)

	assignees := make([]provider.Assignee, len(assigneeLogins))
	for i, login := range assigneeLogins {
		assignees[i] = provider.Assignee{Login: login}
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{
					Number:    1,
					Title:     "Test card",
					Labels:    []provider.Label{{Name: "bug"}},
					Body:      "Card body text",
					Assignees: assignees,
				},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}
