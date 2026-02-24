package main

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newCleanupTestBoard creates a Board with cleanup configured on col 0 ("New"),
// a FakeExecutor, and a FakeProvider. Initial load populates prevCards.
func newCleanupTestBoard(t *testing.T, cleanup string) (Board, *action.FakeExecutor, *provider.FakeProvider) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	columnConfigs := []config.ColumnConfig{
		{Name: "New", Cleanup: cleanup},
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b := NewBoard(p, nil, columnConfigs, fe, "matteobortolazzo", "lazyboards", "github", 32, 0, "Working", false)
	board, err := p.FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)
	b.Width = 120
	b.Height = 40
	return b, fe, p
}

// fakeRefreshBoard builds a provider.Board based on FakeProvider data but with
// the given card numbers removed from col 0 ("New") and added to col 1 ("Refined").
// If a card number is negative, it is removed entirely (simulating closure).
func fakeRefreshBoard(movedCards ...int) provider.Board {
	movedSet := make(map[int]bool)
	removedSet := make(map[int]bool)
	for _, n := range movedCards {
		if n < 0 {
			removedSet[-n] = true
		} else {
			movedSet[n] = true
		}
	}

	// Start from FakeProvider's default data.
	base := provider.NewFakeProvider()
	original, _ := base.FetchBoard(nil)

	var cols []provider.Column
	for i, col := range original.Columns {
		var filtered []provider.Card
		for _, c := range col.Cards {
			if i == 0 && (movedSet[c.Number] || removedSet[c.Number]) {
				continue
			}
			filtered = append(filtered, c)
		}
		// Add moved cards to col 1 ("Refined").
		if i == 1 {
			for _, c := range original.Columns[0].Cards {
				if movedSet[c.Number] {
					filtered = append([]provider.Card{c}, filtered...)
				}
			}
		}
		cols = append(cols, provider.Column{Title: col.Title, Cards: filtered})
	}
	return provider.Board{Columns: cols}
}

func TestCleanup_FirstLoad_NoPrevCards(t *testing.T) {
	_, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls on first load, got %d: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}

func TestCleanup_CardMovesColumn(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session} 2>/dev/null || true")

	b.refreshing = true
	m, cmd := b.Update(boardFetchedMsg{board: fakeRefreshBoard(1)})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call for card leaving column, got none")
	}
	found := false
	for _, call := range fe.RunShellCalls {
		if strings.Contains(call, "tmux kill-window") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected cleanup command containing 'tmux kill-window', got: %v", fe.RunShellCalls)
	}
}

func TestCleanup_CardDisappears(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	// Card #1 disappears entirely (negative = removed, not moved).
	b.refreshing = true
	m, cmd := b.Update(boardFetchedMsg{board: fakeRefreshBoard(-1)})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call for disappeared card, got none")
	}
}

func TestCleanup_CardStaysSameColumn(t *testing.T) {
	b, fe, p := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	board, err := p.FetchBoard(nil)
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	b.refreshing = true
	m, cmd := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls when no cards moved, got %d: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}

func TestCleanup_NoCleanupConfigured(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "") // empty cleanup

	b.refreshing = true
	m, cmd := b.Update(boardFetchedMsg{board: fakeRefreshBoard(1)})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls when no cleanup configured, got %d: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}

func TestCleanup_MultipleCards(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "tmux kill-window -t {session}")

	// Cards #1 and #2 both leave col 0.
	b.refreshing = true
	m, cmd := b.Update(boardFetchedMsg{board: fakeRefreshBoard(1, 2)})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) < 2 {
		t.Errorf("expected at least 2 RunShell calls for 2 cards leaving column, got %d: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
}

func TestCleanup_TemplateVarsExpanded(t *testing.T) {
	b, fe, _ := newCleanupTestBoard(t, "cleanup {number} {session}")

	b.refreshing = true
	m, cmd := b.Update(boardFetchedMsg{board: fakeRefreshBoard(1)})
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected cleanup RunShell call, got none")
	}

	call := fe.RunShellCalls[0]
	expectedSession := action.BuildSessionName(1, "Setup CI", 32)
	if !strings.Contains(call, "'1'") {
		t.Errorf("cleanup command should contain shell-escaped card number '1', got: %s", call)
	}
	if !strings.Contains(call, action.ShellEscape(expectedSession)) {
		t.Errorf("cleanup command should contain shell-escaped session %q, got: %s", action.ShellEscape(expectedSession), call)
	}
}

func TestCleanup_CleanupResultMsg_ShowsStatusMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	m, _ := b.Update(cleanupResultMsg{count: 2})
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, "Cleaned up") {
		t.Errorf("View() after cleanupResultMsg should contain 'Cleaned up', got:\n%s", view)
	}
}

func TestCleanup_CleanupResultMsg_ZeroCount_NoMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	_, cmd := b.Update(cleanupResultMsg{count: 0})

	if cmd != nil {
		t.Error("cleanupResultMsg with count=0 should not return a cmd")
	}
}
