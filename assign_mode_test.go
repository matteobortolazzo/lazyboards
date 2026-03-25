package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newBoardWithCollaborators creates a Board loaded with collaborators and
// an authenticated user from the FakeProvider. The board has the standard
// FakeProvider columns/cards with collaborators cached for assign mode.
func newBoardWithCollaborators(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false)
	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	collabs, err := p.FetchCollaborators(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchCollaborators failed: %v", err)
	}
	authUser, err := p.GetAuthenticatedUser(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.GetAuthenticatedUser failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{
		board:             board,
		collaborators:     collabs,
		authenticatedUser: authUser,
	})
	loaded := m.(Board)
	loaded.Width = 120
	loaded.Height = 40
	return loaded
}

// --- Mode transition tests ---

func TestAssignMode_PressA_OpensPickerModal(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))

	if b.mode != assignMode {
		t.Errorf("after pressing 'a': mode = %d, want assignMode (%d)", b.mode, assignMode)
	}
}

func TestAssignMode_PressA_SetsAssignModeHints(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))

	view := b.View()
	if !strings.Contains(view, "Cancel") {
		t.Error("assign mode view should contain 'Cancel' hint")
	}
	if !strings.Contains(view, "Navigate") {
		t.Error("assign mode view should contain 'Navigate' hint")
	}
	if !strings.Contains(view, "Toggle") {
		t.Error("assign mode view should contain 'Toggle' hint")
	}
}

func TestAssignMode_PressA_NoCollaborators_Noop(t *testing.T) {
	// Use a board without collaborators loaded.
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("a"))

	if b.mode != normalMode {
		t.Errorf("after pressing 'a' with no collaborators: mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

func TestAssignMode_PressA_NoCards_Noop(t *testing.T) {
	b := newBoardWithCollaborators(t)

	// Set up a board state where the active column has no cards.
	b.Columns[b.ActiveTab].Cards = nil

	b = sendKey(t, b, keyMsg("a"))

	if b.mode != normalMode {
		t.Errorf("after pressing 'a' with no cards: mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

// --- Escape ---

func TestAssignMode_Escape_ReturnToNormal(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Escape in assignMode: mode = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

func TestAssignMode_Escape_RestoresNormalHints(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	// After returning to normal mode, the view should show normal-mode hints.
	view := b.View()
	if !strings.Contains(view, "Edit") {
		t.Error("after Escape from assignMode, normal hint 'Edit' should be visible")
	}
}

// --- Navigation ---

func TestAssignMode_JK_Navigation(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	initialCursor := b.assign.cursor

	// Move down.
	b = sendKey(t, b, keyMsg("j"))
	if b.assign.cursor <= initialCursor {
		t.Errorf("after 'j': assign.cursor = %d, want > %d", b.assign.cursor, initialCursor)
	}

	afterDown := b.assign.cursor

	// Move back up.
	b = sendKey(t, b, keyMsg("k"))
	if b.assign.cursor >= afterDown {
		t.Errorf("after 'k': assign.cursor = %d, want < %d", b.assign.cursor, afterDown)
	}
}

func TestAssignMode_CursorClampsAtBounds(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	// Press k many times to go above 0.
	for i := 0; i < 20; i++ {
		b = sendKey(t, b, keyMsg("k"))
	}
	if b.assign.cursor < 0 {
		t.Errorf("cursor went below 0: %d", b.assign.cursor)
	}

	// Press j many times to exceed items length.
	for i := 0; i < 20; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	if b.assign.cursor >= len(b.assign.items) {
		t.Errorf("cursor went past items: cursor=%d, len=%d", b.assign.cursor, len(b.assign.items))
	}
}

// --- "Me" entry pinned at top ---

func TestAssignMode_MeEntryPinnedAtTop(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	if len(b.assign.items) == 0 {
		t.Fatal("assign.items is empty")
	}

	firstItem := b.assign.items[0]
	if !firstItem.isMe {
		t.Error("first item in assign picker should be the 'me' entry (authenticated user)")
	}
	if firstItem.login != "fake-user" {
		t.Errorf("first item login = %q, want %q (authenticated user)", firstItem.login, "fake-user")
	}
}

// --- Assigned users marked ---

func TestAssignMode_AssignedUsersMarked(t *testing.T) {
	b := newBoardWithCollaborators(t)

	// The first card in FakeProvider's "New" column has assignees: ["alice"].
	// Verify that "alice" is marked as assigned in the picker.
	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	foundAlice := false
	for _, item := range b.assign.items {
		if item.login == "alice" {
			foundAlice = true
			if !item.isAssigned {
				t.Error("alice should be marked as assigned (isAssigned=true)")
			}
		}
	}
	if !foundAlice {
		t.Error("expected 'alice' in assign items (she is a collaborator)")
	}

	// "charlie" is a collaborator but not assigned to this card.
	for _, item := range b.assign.items {
		if item.login == "charlie" {
			if item.isAssigned {
				t.Error("charlie should NOT be marked as assigned")
			}
		}
	}
}

func TestAssignMode_View_ShowsAsteriskForAssigned(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	view := b.View()
	// The view should mark assigned users with a '*' prefix or similar indicator.
	// The first card has "alice" assigned, so the view should show "*" near "alice".
	if !strings.Contains(view, "*") {
		t.Error("View() in assignMode should contain '*' to indicate assigned users")
	}
}

// --- Enter to assign/unassign ---

func TestAssignMode_Enter_AssignUser(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	// Navigate to an unassigned user.
	// Find "charlie" who is a collaborator but not assigned to card #1.
	for i, item := range b.assign.items {
		if item.login == "charlie" && !item.isAssigned {
			b.assign.cursor = i
			break
		}
	}

	// Press Enter to toggle (assign).
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	// Should return a non-nil cmd (setAssigneesCmd).
	if cmd == nil {
		t.Fatal("pressing Enter on unassigned user should return non-nil cmd")
	}

	// Execute the command and feed result back.
	msgs := collectAssignMsgs(cmd)
	for _, msg := range msgs {
		m, cmd = b.Update(msg)
		b = m.(Board)
	}
	if cmd == nil {
		t.Error("assigneesUpdatedMsg should return a non-nil cmd (status bar timer)")
	}

	// After the update, the card's assignees should include "charlie".
	card := b.selectedCard()
	foundCharlie := false
	for _, a := range card.Assignees {
		if a.Login == "charlie" {
			foundCharlie = true
		}
	}
	if !foundCharlie {
		t.Error("after assigning charlie, card.Assignees should include charlie")
	}
}

func TestAssignMode_Enter_UnassignUser(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	// Navigate to "alice" who is assigned to card #1.
	for i, item := range b.assign.items {
		if item.login == "alice" && item.isAssigned {
			b.assign.cursor = i
			break
		}
	}

	// Press Enter to toggle (unassign).
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	if cmd == nil {
		t.Fatal("pressing Enter on assigned user should return non-nil cmd")
	}

	// Execute the command and feed result back.
	msgs := collectAssignMsgs(cmd)
	for _, msg := range msgs {
		m, cmd = b.Update(msg)
		b = m.(Board)
	}
	if cmd == nil {
		t.Error("assigneesUpdatedMsg should return a non-nil cmd (status bar timer)")
	}

	// After the update, the card's assignees should NOT include "alice".
	card := b.selectedCard()
	for _, a := range card.Assignees {
		if a.Login == "alice" {
			t.Error("after unassigning alice, card.Assignees should not include alice")
		}
	}
}

// --- Error handling ---

func TestAssignMode_ErrorMsg_ShowsStatusBar(t *testing.T) {
	b := newBoardWithCollaborators(t)

	m, cmd := b.Update(assigneesUpdateErrorMsg{err: errors.New("assign failed")})
	b = m.(Board)

	// Should show error in status bar.
	if b.statusBar.message == "" {
		t.Error("assigneesUpdateErrorMsg should set a status bar error message")
	}
	if !strings.Contains(b.statusBar.message, "assign failed") {
		t.Errorf("status bar message = %q, want to contain %q", b.statusBar.message, "assign failed")
	}
	// cmd may be non-nil for timed message clearing.
	_ = cmd
}

// --- View rendering ---

func TestAssignMode_View_RendersModal(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	view := b.View()
	if view == "" {
		t.Fatal("View() returned empty string in assignMode")
	}

	// View should contain collaborator logins.
	if !strings.Contains(view, "alice") {
		t.Error("View() in assignMode should contain 'alice'")
	}
	if !strings.Contains(view, "bob") {
		t.Error("View() in assignMode should contain 'bob'")
	}
	if !strings.Contains(view, "charlie") {
		t.Error("View() in assignMode should contain 'charlie'")
	}
}

func TestAssignMode_View_ContainsHints(t *testing.T) {
	b := newBoardWithCollaborators(t)

	b = sendKey(t, b, keyMsg("a"))

	view := b.View()
	for _, hint := range assignModeHints {
		if !strings.Contains(view, hint.Desc) {
			t.Errorf("View() in assignMode should contain hint %q", hint.Desc)
		}
	}
}

// --- Global filter interaction ---

func TestAssignMode_WithGlobalFilter_UsesSelectedCard(t *testing.T) {
	b := newBoardWithCollaborators(t)

	// Apply a filter that still shows some cards.
	// Card #1 "Setup CI" has label "infra", card #2 "Data model" has label "design".
	// Filter by label "design" so only card #2 is shown.
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "design"

	// Reset cursor to 0 (which now points to card #2 "Data model" in filtered view).
	b.Columns[b.ActiveTab].Cursor = 0

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("expected assignMode after 'a', got %d", b.mode)
	}

	// Card #2 has assignees ["alice", "bob"]. Both should be marked assigned.
	aliceAssigned := false
	bobAssigned := false
	for _, item := range b.assign.items {
		if item.login == "alice" && item.isAssigned {
			aliceAssigned = true
		}
		if item.login == "bob" && item.isAssigned {
			bobAssigned = true
		}
	}
	if !aliceAssigned {
		t.Error("with filter active, alice should be marked assigned on card #2")
	}
	if !bobAssigned {
		t.Error("with filter active, bob should be marked assigned on card #2")
	}
}

// --- Normal hints ---

func TestAssignMode_HintShown_WhenCollaboratorsAvailable(t *testing.T) {
	b := newBoardWithCollaborators(t)

	foundAssignHint := false
	for _, hint := range b.normalHints {
		if hint.Key == "a" {
			foundAssignHint = true
			break
		}
	}
	if !foundAssignHint {
		t.Error("normalHints should include 'a' hint when collaborators are cached and cards exist")
	}
}

// --- Helpers ---

// collectAssignMsgs executes a tea.Cmd and collects resulting messages.
// Uses goroutine+timeout per lessons-learned to avoid blocking on tea.Tick.
func collectAssignMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	var msgs []tea.Msg
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if batchMsg, ok := msg.(tea.BatchMsg); ok {
			for _, subCmd := range batchMsg {
				msgs = append(msgs, collectAssignMsgs(subCmd)...)
			}
		} else {
			msgs = append(msgs, msg)
		}
	case <-time.After(100 * time.Millisecond):
		// Skip blocking commands (e.g., tea.Tick)
	}
	return msgs
}
