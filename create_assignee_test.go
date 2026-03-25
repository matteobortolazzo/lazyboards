package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// newCreateTestBoardWithCollaborators creates a loaded board with collaborators
// and authenticated user set, sized for create-mode testing.
func newCreateTestBoardWithCollaborators(t *testing.T) Board {
	t.Helper()
	b := newLoadedTestBoard(t)
	b.collaborators = []Assignee{
		{Login: "alice"},
		{Login: "bob"},
		{Login: "fake-user"},
	}
	b.authenticatedUser = "fake-user"
	b.Width = 120
	b.Height = 40
	return b
}

// enterCreateAndFocusAssignee enters create mode and tabs to the assignee field.
func enterCreateAndFocusAssignee(t *testing.T, b Board) Board {
	t.Helper()
	b = sendKey(t, b, keyMsg("n"))          // enter create mode
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // title -> label
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // label -> assignee
	return b
}

func TestCreateAssignee_FieldShownWithCollaborators(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = sendKey(t, b, keyMsg("n"))

	view := b.View()
	if !strings.Contains(view, "Assignee") {
		t.Error("create modal should show 'Assignee' field when collaborators exist")
	}
}

func TestCreateAssignee_FieldHiddenWithoutCollaborators(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n"))

	view := b.View()
	if strings.Contains(view, "Assignee") {
		t.Error("create modal should NOT show 'Assignee' field when no collaborators exist")
	}
}

func TestCreateAssignee_DefaultIsNone(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = sendKey(t, b, keyMsg("n"))

	view := b.View()
	if !strings.Contains(view, "(none)") {
		t.Error("assignee field should default to '(none)'")
	}
}

func TestCreateAssignee_MePinnedFirst(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b)

	// Press right once to move from (none) to the next option
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	view := b.View()

	// The authenticated user should appear with "(me)" suffix as the first option after "(none)"
	if !strings.Contains(view, "fake-user (me)") {
		t.Error("first option after (none) should be the authenticated user with '(me)' suffix")
	}
}

func TestCreateAssignee_TabCyclesThreeFields(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = sendKey(t, b, keyMsg("n")) // enter create mode, focus=title

	// Verify title is focused
	if b.create.focus != 0 {
		t.Errorf("initial focus = %d, want 0 (title)", b.create.focus)
	}

	// Tab: title -> label
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.create.focus != 1 {
		t.Errorf("after first Tab: focus = %d, want 1 (label)", b.create.focus)
	}

	// Tab: label -> assignee
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.create.focus != 2 {
		t.Errorf("after second Tab: focus = %d, want 2 (assignee)", b.create.focus)
	}

	// Tab: assignee -> title
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.create.focus != 0 {
		t.Errorf("after third Tab: focus = %d, want 0 (title)", b.create.focus)
	}
}

func TestCreateAssignee_TabCyclesTwoFieldsWithoutCollaborators(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n")) // enter create mode, focus=title

	// Tab: title -> label
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.create.focus != 1 {
		t.Errorf("after first Tab: focus = %d, want 1 (label)", b.create.focus)
	}

	// Tab: label -> title (skip assignee since no collaborators)
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.create.focus != 0 {
		t.Errorf("after second Tab: focus = %d, want 0 (title), should skip assignee", b.create.focus)
	}
}

func TestCreateAssignee_LeftRightCyclesAssignee(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b)

	// Default is index 0 = (none)
	if b.create.assigneeIndex != 0 {
		t.Fatalf("initial assigneeIndex = %d, want 0", b.create.assigneeIndex)
	}

	// Right: move to index 1
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.create.assigneeIndex != 1 {
		t.Errorf("after right: assigneeIndex = %d, want 1", b.create.assigneeIndex)
	}

	// Left: back to index 0
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.create.assigneeIndex != 0 {
		t.Errorf("after left: assigneeIndex = %d, want 0", b.create.assigneeIndex)
	}
}

func TestCreateAssignee_LeftRightWrapsAround(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b)

	optionCount := len(b.create.assigneeOptions)
	if optionCount < 2 {
		t.Fatalf("need at least 2 assignee options, got %d", optionCount)
	}

	// Left from index 0 wraps to last
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.create.assigneeIndex != optionCount-1 {
		t.Errorf("left from 0: assigneeIndex = %d, want %d (last)", b.create.assigneeIndex, optionCount-1)
	}

	// Right from last wraps to 0
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.create.assigneeIndex != 0 {
		t.Errorf("right from last: assigneeIndex = %d, want 0", b.create.assigneeIndex)
	}
}

func TestCreateAssignee_SubmitWithAssignee(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b)

	// Select assignee (right from (none))
	b = sendKey(t, b, arrowMsg(tea.KeyRight))

	// Tab back to title and type a title
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b = sendKey(t, b, keyMsg("Test card"))

	// Submit
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	if b.mode != creatingMode {
		t.Errorf("mode = %d, want %d (creatingMode)", b.mode, creatingMode)
	}
	if b.create.pendingAssignee == "" {
		t.Error("pendingAssignee should be set after submitting with assignee selected")
	}
	if cmd == nil {
		t.Error("cmd should be non-nil (spinner tick + createCardCmd)")
	}
}

func TestCreateAssignee_SubmitWithoutAssignee(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = sendKey(t, b, keyMsg("n"))

	// Type a title (focus is on title by default)
	b = sendKey(t, b, keyMsg("Test card"))

	// Submit with (none) selected (default)
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	if b.create.pendingAssignee != "" {
		t.Errorf("pendingAssignee = %q, want empty when (none) selected", b.create.pendingAssignee)
	}
	if cmd == nil {
		t.Error("cmd should be non-nil (spinner tick + createCardCmd)")
	}
}

func TestCreateAssignee_ChainSetAssigneesAfterCreate(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b)

	// Select "fake-user (me)"
	b = sendKey(t, b, arrowMsg(tea.KeyRight))

	// Store the pending assignee for assertion
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // back to title
	b = sendKey(t, b, keyMsg("Test card"))

	// Submit to enter creatingMode
	m, submitCmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if submitCmd == nil {
		t.Error("submit cmd should be non-nil (spinner + createCardCmd)")
	}

	// Simulate card creation complete
	createdCard := provider.Card{Number: 99, Title: "Test card"}
	m, cmd := b.Update(cardCreatedMsg{card: createdCard})
	b = m.(Board)

	// Should return to normal mode
	if b.mode != normalMode {
		t.Errorf("mode = %d, want %d (normalMode)", b.mode, normalMode)
	}

	// Should have a non-nil cmd for setAssigneesCmd
	if cmd == nil {
		t.Error("cmd should be non-nil when pending assignee exists (setAssigneesCmd)")
	}
}

func TestCreateAssignee_NoChainWithoutPendingAssignee(t *testing.T) {
	b := newCreatingTestBoard(t)
	b.create.pendingAssignee = ""

	createdCard := provider.Card{Number: 99, Title: "Test card"}
	m, cmd := b.Update(cardCreatedMsg{card: createdCard})
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if cmd != nil {
		t.Error("cmd should be nil when no pending assignee")
	}
}

func TestCreateAssignee_AssigneeResetOnReenter(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)

	// Enter create mode, select an assignee
	b = enterCreateAndFocusAssignee(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyRight)) // select non-none

	// Escape
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	// Re-enter create mode
	b = sendKey(t, b, keyMsg("n"))

	// Assignee index should be reset to 0 (none)
	if b.create.assigneeIndex != 0 {
		t.Errorf("assigneeIndex = %d, want 0 after re-entering create mode", b.create.assigneeIndex)
	}
}
