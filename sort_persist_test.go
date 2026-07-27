package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newSortPersistBoard returns a loaded board whose sort direction is persisted
// to a state file in a temp dir, plus that file's path.
func newSortPersistBoard(t *testing.T) (Board, string) {
	t.Helper()
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)
	path := filepath.Join(t.TempDir(), "state.yml")
	b.statePath = path
	return b, path
}

func TestNormalMode_U_PersistsNewSortOrder(t *testing.T) {
	b, path := newSortPersistBoard(t)

	m, cmd := b.Update(keyMsg("u"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("'u' toggle returned a nil cmd, want a cmd that persists the new sort order")
	}
	if !updated.sortNewestFirst {
		t.Fatalf("precondition: sortNewestFirst = false after toggle, want true")
	}
	execCmds(cmd)

	st, err := config.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}
	if st.SortOrder != config.SortOrderNewest {
		t.Errorf("persisted sort_order = %q, want %q", st.SortOrder, config.SortOrderNewest)
	}
}

func TestNormalMode_U_PersistsBothDirections(t *testing.T) {
	b, path := newSortPersistBoard(t)

	m, cmd := b.Update(keyMsg("u"))
	b, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	execCmds(cmd)

	m, cmd = b.Update(keyMsg("u"))
	b, ok = m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("second 'u' toggle returned a nil cmd, want a cmd that persists the restored sort order")
	}
	execCmds(cmd)

	st, err := config.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}
	if st.SortOrder != config.SortOrderOldest {
		t.Errorf("persisted sort_order = %q, want %q (toggling back must persist too)", st.SortOrder, config.SortOrderOldest)
	}
	if b.sortNewestFirst {
		t.Error("sortNewestFirst = true after toggling twice, want false")
	}
}

// A board with no resolvable state path (e.g. no home directory) must still
// toggle — it just can't remember the choice.
func TestNormalMode_U_WithoutStatePath_TogglesWithoutPersisting(t *testing.T) {
	cards := []provider.Card{
		{Number: 1, Title: "Oldest", CreatedAt: sortTestOlder},
		{Number: 2, Title: "Newest", CreatedAt: sortTestNewest},
	}
	b := newBoardWithInlineCards(t, cards, 120, 40)

	m, cmd := b.Update(keyMsg("u"))
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("'u' toggle returned a non-nil cmd with no state path configured, want nil (nothing to persist to)")
	}
	if !updated.sortNewestFirst {
		t.Error("sortNewestFirst = false after toggle, want true (the toggle must work even when it can't be saved)")
	}
	assertCardOrder(t, updated.Columns[0].Cards, []int{2, 1})
}

// A failed write must surface, not disappear: the sort still flips, but the
// user is told the choice won't survive a restart.
func TestSortOrderSaveError_ShowsStatusMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	m, cmd := b.Update(sortOrderSaveErrorMsg{err: errors.New("permission denied")})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Error("sortOrderSaveErrorMsg returned a nil cmd, want the status-bar message timeout cmd")
	}

	view := updated.View()
	if !strings.Contains(view, "sort order") {
		t.Errorf("View() after a failed sort-order save does not mention the failure, got:\n%s", view)
	}
}

func TestSortOrderSaveErrorCmd_ReportsUnwritablePath(t *testing.T) {
	// A path whose parent is a regular file can't be created as a directory.
	file := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	cmd := saveSortOrderCmd(filepath.Join(file, "state.yml"), true)
	if cmd == nil {
		t.Fatal("saveSortOrderCmd() returned nil, want a cmd")
	}
	msg := cmd()

	if _, ok := msg.(sortOrderSaveErrorMsg); !ok {
		t.Errorf("saveSortOrderCmd() msg = %T, want sortOrderSaveErrorMsg", msg)
	}
}

func TestSaveSortOrderCmd_SuccessReportsSavedMsg(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yml")

	msg := saveSortOrderCmd(path, false)()

	if _, ok := msg.(sortOrderSavedMsg); !ok {
		t.Fatalf("saveSortOrderCmd() msg = %T, want sortOrderSavedMsg", msg)
	}
	st, err := config.LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() returned error: %v", err)
	}
	if st.SortOrder != config.SortOrderOldest {
		t.Errorf("persisted sort_order = %q, want %q", st.SortOrder, config.SortOrderOldest)
	}
}

// A successful save is silent — no status-bar noise on every 'u' press.
func TestSortOrderSavedMsg_ShowsNoStatusMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	before := b.View()

	m, cmd := b.Update(sortOrderSavedMsg{})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd != nil {
		t.Error("sortOrderSavedMsg returned a non-nil cmd, want nil (a successful save is silent)")
	}
	if updated.View() != before {
		t.Error("sortOrderSavedMsg changed the view, want no visible effect")
	}
}
