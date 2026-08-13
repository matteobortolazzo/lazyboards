package main

import (
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// TestCreateMode_AssigneePicker_SanitizesControlSequencesInLogin covers the
// same GitHub-sourced-untrusted-content gap for the create-mode assignee
// picker -- viewCreateModal renders the selected collaborator option as
// "< " + login + " >" without sanitization. A malicious collaborator login
// must not leak raw ESC/BEL/C1 control bytes into the rendered picker, while
// still keeping the visible username text.
func TestCreateMode_AssigneePicker_SanitizesControlSequencesInLogin(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b.collaborators = []Assignee{{Login: "\x1b[31mRED\x1b[0m"}}
	b.authenticatedUser = "someone-else"

	b = sendKey(t, b, keyMsg("n"))            // enter create mode
	b = sendKey(t, b, arrowMsg(tea.KeyTab))   // title -> label
	b = sendKey(t, b, arrowMsg(tea.KeyTab))   // label -> assignee
	b = sendKey(t, b, arrowMsg(tea.KeyRight)) // (none) -> authenticated user (me)
	b = sendKey(t, b, arrowMsg(tea.KeyRight)) // authenticated user (me) -> the malicious collaborator option

	view := b.View()

	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("View() = %q, want no ESC (0x1b) byte", view)
	}
	if !strings.Contains(view, "RED") {
		t.Errorf("View() = %q, want visible login text %q retained", view, "RED")
	}
}

// TestCreateMode_AssigneeOptions_PinsNoneAndMeSortsRestAlphabetically covers
// #477: the create-mode assignee dropdown must keep "(none)" first and
// "<user> (me)" second (both pins), with the remaining collaborators sorted
// case-insensitively alphabetically below them.
func TestCreateMode_AssigneeOptions_PinsNoneAndMeSortsRestAlphabetically(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b.collaborators = []Assignee{
		{Login: "zack"},
		{Login: "Amy"},
		{Login: "bob"},
	}
	b.authenticatedUser = "fake-user"

	b = sendKey(t, b, keyMsg("n"))
	if b.mode != createMode {
		t.Fatalf("expected createMode after 'n', got %d", b.mode)
	}

	options := b.create.assigneeOptions
	if len(options) < 2 {
		t.Fatalf("assigneeOptions = %v, want at least 2 entries ((none) and me pins)", options)
	}
	if options[0] != noneAssignee {
		t.Errorf("assigneeOptions[0] = %q, want %q", options[0], noneAssignee)
	}
	wantMe := b.authenticatedUser + " (me)"
	if options[1] != wantMe {
		t.Errorf("assigneeOptions[1] = %q, want %q", options[1], wantMe)
	}

	tail := append([]string(nil), options[2:]...)
	if len(tail) == 0 {
		t.Fatal("precondition: expected non-pinned collaborators in assigneeOptions tail")
	}

	want := make([]string, len(tail))
	copy(want, tail)
	sort.SliceStable(want, func(i, j int) bool {
		return strings.ToLower(want[i]) < strings.ToLower(want[j])
	})

	for i := range tail {
		if tail[i] != want[i] {
			t.Errorf("assigneeOptions tail = %v, want case-insensitive alphabetical order %v", tail, want)
			break
		}
	}
}

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
	if !b.create.titleInput.Focused() {
		t.Error("titleInput should be focused when entering createMode")
	}
	if b.create.labelInput.Focused() {
		t.Error("labelInput should NOT be focused when entering createMode")
	}

	// Tab should switch focus to labelInput.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.create.titleInput.Focused() {
		t.Error("titleInput should NOT be focused after Tab")
	}
	if !b.create.labelInput.Focused() {
		t.Error("labelInput should be focused after Tab")
	}

	// Another Tab should switch focus back to titleInput.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if !b.create.titleInput.Focused() {
		t.Error("titleInput should be focused after second Tab")
	}
	if b.create.labelInput.Focused() {
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

	if b.create.titleInput.Value() != "Fix bug" {
		t.Errorf("titleInput.Value() = %q, want %q", b.create.titleInput.Value(), "Fix bug")
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

	if b.create.labelInput.Value() != "bug" {
		t.Errorf("labelInput.Value() = %q, want %q", b.create.labelInput.Value(), "bug")
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

	if b.create.titleInput.Value() != "" {
		t.Errorf("titleInput.Value() after reopen = %q, want empty string (fields should reset)", b.create.titleInput.Value())
	}
	if b.create.labelInput.Value() != "" {
		t.Errorf("labelInput.Value() after reopen = %q, want empty string (fields should reset)", b.create.labelInput.Value())
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
	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "Labeled task", Labels: []provider.Label{{Name: "bug"}}}})
	b = m.(Board)

	newCard := b.Columns[0].Cards[len(b.Columns[0].Cards)-1]
	if len(newCard.Labels) == 0 || newCard.Labels[0].Name != "bug" {
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
	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "Some task", Labels: []provider.Label{{Name: "feature"}}}})
	b = m.(Board)

	if b.create.titleInput.Value() != "" {
		t.Errorf("titleInput.Value() = %q after submit, want empty string (fields should reset)", b.create.titleInput.Value())
	}
	if b.create.labelInput.Value() != "" {
		t.Errorf("labelInput.Value() = %q after submit, want empty string (fields should reset)", b.create.labelInput.Value())
	}
}

func TestView_HelpBarShowsNewHint(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()

	if !strings.Contains(view, "New") {
		t.Errorf("View() status bar does not contain hint desc %q", "New")
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

	msg := cardCreatedMsg{card: provider.Card{Number: 99, Title: "New task", Labels: []provider.Label{{Name: "feature"}}}}
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
	if len(newCard.Labels) == 0 || newCard.Labels[0].Name != "feature" {
		t.Errorf("new card Labels = %v, want [\"feature\"]", newCard.Labels)
	}

	// Fields should be reset.
	if updated.create.titleInput.Value() != "" {
		t.Errorf("titleInput.Value() = %q after success, want empty string", updated.create.titleInput.Value())
	}
	if updated.create.labelInput.Value() != "" {
		t.Errorf("labelInput.Value() = %q after success, want empty string", updated.create.labelInput.Value())
	}
	if updated.validationErr != "" {
		t.Errorf("validationErr = %q after success, want empty string", updated.validationErr)
	}
}

func TestCreatingMode_Error_ShowsErrorAndPreservesInput(t *testing.T) {
	b := newCreatingTestBoard(t)
	b.create.titleInput.SetValue("My title")
	b.create.labelInput.SetValue("my-label")

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
	if updated.create.titleInput.Value() != "My title" {
		t.Errorf("titleInput.Value() = %q after error, want %q (input should be preserved)", updated.create.titleInput.Value(), "My title")
	}
	if updated.create.labelInput.Value() != "my-label" {
		t.Errorf("labelInput.Value() = %q after error, want %q (input should be preserved)", updated.create.labelInput.Value(), "my-label")
	}

	// Title input should be focused for easy editing.
	if !updated.create.titleInput.Focused() {
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

func TestCreateMode_StatusBarShowsEscapeHint(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Enter createMode.
	b = sendKey(t, b, keyMsg("n"))
	view := b.View()

	if !strings.Contains(view, "Cancel") {
		t.Errorf("View() in createMode should contain hint desc %q, got:\n%s", "Cancel", view)
	}
}

// --- Textarea Title Field ---

func TestCreateMode_LongTitlePreservedBeyondVisibleWidth(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))

	// Build a title that is longer than the visible textarea width
	// to verify the textarea preserves the full text and wraps
	// visually instead of scrolling.
	longTitle := strings.Repeat("a", 80)
	for _, ch := range longTitle {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	got := b.create.titleInput.Value()
	if got != longTitle {
		t.Errorf("titleInput.Value() length = %d, want %d (full text should be preserved, not truncated)",
			len(got), len(longTitle))
	}
}

// --- Dynamic Textarea ---

func TestCreateMode_ModalWidthScalesToTerminal(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n"))

	// The modal width should be 60% of terminal width.
	expectedModalWidth := b.Width * 60 / 100

	// Verify through the textarea width: the textarea should fill the modal
	// inner width (modal width minus padding). renderModal uses Padding(1,2)
	// which adds 2 chars on each side = 4 chars horizontal padding,
	// plus 2 chars for border = 6 total horizontal overhead.
	modalPaddingAndBorder := 6
	expectedInputWidth := expectedModalWidth - modalPaddingAndBorder

	titleWidth := b.create.titleInput.Width()
	if titleWidth != expectedInputWidth {
		t.Errorf("titleInput.Width() = %d, want %d (should fill 60%% of terminal width %d minus modal padding)",
			titleWidth, expectedInputWidth, b.Width)
	}
}

func TestCreateMode_TextareaGrowsWithContent(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n"))

	initialHeight := b.create.titleInput.Height()

	// Type enough text to cause wrapping. The textarea width at 120-wide terminal
	// (60% = 72, minus padding/border = 66) means we need text longer than 66 chars
	// to wrap. Type ~200 chars to ensure multiple wraps.
	longText := strings.Repeat("word ", 40) // 200 chars
	for _, ch := range longText {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	grownHeight := b.create.titleInput.Height()
	if grownHeight <= initialHeight {
		t.Errorf("titleInput.Height() after long text = %d, want > %d (textarea should grow with content)",
			grownHeight, initialHeight)
	}
}

func TestCreateMode_TextareaMaxHeight(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n"))

	initialHeight := b.create.titleInput.Height()

	// Max textarea height should be 50% of terminal height.
	maxHeight := b.Height * 50 / 100

	// Type a very large amount of text to exceed the max height.
	// At ~66 chars per line and max 20 lines, we need > 1320 chars.
	veryLongText := strings.Repeat("x", 2000)
	for _, ch := range veryLongText {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	textareaHeight := b.create.titleInput.Height()

	// The textarea should have grown beyond its initial height (proving it grew).
	if textareaHeight <= initialHeight {
		t.Errorf("titleInput.Height() = %d, want > %d (textarea should grow with content before being capped)",
			textareaHeight, initialHeight)
	}

	// But it should not exceed the max height (50% of terminal).
	if textareaHeight > maxHeight {
		t.Errorf("titleInput.Height() = %d, want <= %d (50%% of terminal height %d)",
			textareaHeight, maxHeight, b.Height)
	}
}

func TestCreateMode_NoCharLimit(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))

	// Type more than 200 characters — well beyond the current CharLimit of 100.
	longTitle := strings.Repeat("z", 250)
	for _, ch := range longTitle {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	got := b.create.titleInput.Value()
	if len(got) != len(longTitle) {
		t.Errorf("titleInput.Value() length = %d, want %d (no character limit should be enforced)",
			len(got), len(longTitle))
	}
}

func TestCreateMode_ResizeDuringCreateMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n"))

	// Record initial textarea width.
	initialWidth := b.create.titleInput.Width()

	// Simulate a terminal resize to a wider terminal.
	newWidth := 200
	b = sendKey(t, b, tea.WindowSizeMsg{Width: newWidth, Height: 50})

	// After resize, the textarea should adapt to the new terminal width.
	// New expected modal width = 200 * 60 / 100 = 120.
	// Modal padding+border = 6, so new input width = 114.
	resizedWidth := b.create.titleInput.Width()
	if resizedWidth == initialWidth {
		t.Errorf("titleInput.Width() after resize = %d, want different from initial %d (should adapt to new terminal width)",
			resizedWidth, initialWidth)
	}

	expectedModalWidth := newWidth * 60 / 100
	modalPaddingAndBorder := 6
	expectedInputWidth := expectedModalWidth - modalPaddingAndBorder

	if resizedWidth != expectedInputWidth {
		t.Errorf("titleInput.Width() after resize = %d, want %d (60%% of new width %d minus padding)",
			resizedWidth, expectedInputWidth, newWidth)
	}
}

func TestCreateMode_InputsFillModalWidth(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("n"))

	// Both inputs should have the same width, matching the modal inner width.
	// Modal width = 120 * 60 / 100 = 72.
	// renderModal uses Padding(1,2) + border: 2*2 padding + 2 border = 6 total horizontal.
	expectedModalWidth := b.Width * 60 / 100
	modalPaddingAndBorder := 6
	expectedInputWidth := expectedModalWidth - modalPaddingAndBorder

	titleWidth := b.create.titleInput.Width()
	labelWidth := b.create.labelInput.Width

	if titleWidth != expectedInputWidth {
		t.Errorf("titleInput.Width() = %d, want %d (should fill modal inner width)",
			titleWidth, expectedInputWidth)
	}
	if labelWidth != expectedInputWidth {
		t.Errorf("labelInput.Width = %d, want %d (should fill modal inner width)",
			labelWidth, expectedInputWidth)
	}
	if titleWidth != labelWidth {
		t.Errorf("titleInput.Width() = %d, labelInput.Width = %d, want equal (both should fill modal inner width)",
			titleWidth, labelWidth)
	}
}

func TestCreateMode_EnterDoesNotInsertNewline(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))

	// Type some text into the title field.
	for _, ch := range "My title" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter — this should submit the form, not insert a newline.
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	// The title value should never contain a newline character,
	// confirming Enter was intercepted before reaching the textarea.
	if strings.Contains(b.create.titleInput.Value(), "\n") {
		t.Error("titleInput.Value() contains a newline; Enter should submit the form, not insert a newline")
	}
}

// --- Textarea Viewport Scroll Bug (#223) ---

func TestCreateMode_TextareaViewportResetOnWrap(t *testing.T) {
	// When text wraps in the textarea, the viewport must keep the cursor
	// visible. With a small terminal height, max textarea height is small
	// (e.g., 1 line at Height=2). When wrapping occurs and the height is
	// capped, the viewport must scroll to show the cursor line.
	b := newLoadedTestBoard(t)
	b.Width = 80
	b.Height = 4 // Max textarea height = 4*50/100 = 2 lines
	b = sendKey(t, b, keyMsg("n"))

	contentWidth := b.create.titleInput.Width()
	if contentWidth < 1 {
		t.Fatalf("titleInput.Width() = %d, want >= 1", contentWidth)
	}

	maxHeight := b.Height * 50 / 100 // = 2

	// Type enough text to wrap to exactly 3 visual lines (exceeds maxHeight of 2).
	// Line 1: A's, Line 2: A's, Line 3: "BBBBB"
	firstTwoLines := strings.Repeat("A", contentWidth*2)
	thirdLine := "BBBBB"
	fullText := firstTwoLines + thirdLine

	for _, ch := range fullText {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Verify the textarea is capped at max height (2 lines, not 3).
	if b.create.titleInput.Height() > maxHeight {
		t.Errorf("titleInput.Height() = %d, want <= %d (max height cap)",
			b.create.titleInput.Height(), maxHeight)
	}

	// The cursor is at the end (after "BBBBB"). The viewport MUST scroll
	// to show the cursor line. With the bug, the viewport stays at line 1,
	// hiding the cursor line with "BBBBB".
	textareaView := b.create.titleInput.View()
	if !strings.Contains(textareaView, "BBBBB") {
		t.Errorf("textarea View() does not contain cursor-line text 'BBBBB'; "+
			"viewport did not scroll to follow cursor after text wrap.\n"+
			"View():\n%s", textareaView)
	}
}

func TestCreateMode_TextareaViewportResetOnMultipleWraps(t *testing.T) {
	// Type text that wraps to 5+ visual lines with max height of 3.
	// The viewport must show the most recently typed text (cursor line).
	b := newLoadedTestBoard(t)
	b.Width = 80
	b.Height = 6 // Max textarea height = 6*50/100 = 3 lines
	b = sendKey(t, b, keyMsg("n"))

	contentWidth := b.create.titleInput.Width()
	if contentWidth < 1 {
		t.Fatalf("titleInput.Width() = %d, want >= 1", contentWidth)
	}

	maxHeight := b.Height * 50 / 100 // = 3

	// Build text that spans 5 visual lines, exceeding maxHeight of 3.
	// Lines 1-4: X's, Line 5: identifiable cursor-line text.
	fourLines := strings.Repeat("X", contentWidth*4)
	cursorLine := "ENDTEXT"
	fullText := fourLines + cursorLine

	for _, ch := range fullText {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Verify height is capped at maxHeight.
	if b.create.titleInput.Height() > maxHeight {
		t.Errorf("titleInput.Height() = %d, want <= %d (max height cap)",
			b.create.titleInput.Height(), maxHeight)
	}

	// The cursor is at the end (after "ENDTEXT"). The viewport must
	// scroll to show the cursor line — all 3 visible lines should be
	// from the bottom of the content, including the cursor line.
	textareaView := b.create.titleInput.View()
	if !strings.Contains(textareaView, "ENDTEXT") {
		t.Errorf("textarea View() does not contain cursor-line text 'ENDTEXT'; "+
			"viewport did not scroll to follow cursor on multiple wraps.\n"+
			"View():\n%s", textareaView)
	}
}

func TestCreateMode_TextareaMaxHeightScrollKeepsCursorVisible(t *testing.T) {
	// When text exceeds the max height (50% of terminal), the textarea
	// should cap at max height and the most recently typed characters
	// (near the cursor) must be visible in View().
	b := newLoadedTestBoard(t)
	b.Width = 80
	b.Height = 20 // Max height = 20*50/100 = 10 lines
	b = sendKey(t, b, keyMsg("n"))

	contentWidth := b.create.titleInput.Width()
	if contentWidth < 1 {
		t.Fatalf("titleInput.Width() = %d, want >= 1", contentWidth)
	}

	maxHeight := b.Height * 50 / 100

	// Build text that requires more visual lines than maxHeight.
	// We need (maxHeight + 5) lines worth of text to exceed the cap.
	totalChars := contentWidth * (maxHeight + 5)
	longText := strings.Repeat("X", totalChars)
	// Append a unique suffix that the cursor is next to.
	suffix := "ZZEND"
	fullText := longText + suffix

	for _, ch := range fullText {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Verify the textarea is capped at max height.
	if b.create.titleInput.Height() != maxHeight {
		t.Errorf("titleInput.Height() = %d, want %d (max height = 50%% of terminal height %d)",
			b.create.titleInput.Height(), maxHeight, b.Height)
	}

	// The most recently typed text (near cursor) must be visible.
	textareaView := b.create.titleInput.View()
	if !strings.Contains(textareaView, "ZZEND") {
		t.Errorf("textarea View() does not contain recently typed text 'ZZEND'; "+
			"cursor line is not visible when textarea exceeds max height.\n"+
			"View():\n%s", textareaView)
	}
}

// --- Focus New Card On Create (#418) ---

// TestCardCreated_SwitchesActiveTabToTargetColumn covers AC1: when the user
// was viewing a column other than the target column (Columns[0]) at creation
// time, ActiveTab switches to it so the new card is visible.
func TestCardCreated_SwitchesActiveTabToTargetColumn(t *testing.T) {
	b := newLoadedTestBoard(t)

	// Move to a column other than the target (Columns[0]).
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab == 0 {
		t.Fatal("precondition: ActiveTab should not be 0 after Tab")
	}

	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "New task"}})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d after card created, want 0 (the column the card was created into)", updated.ActiveTab)
	}
}

// TestCardCreated_CursorFocusesNewCard covers AC2: the new card becomes the
// selected/highlighted card, whether or not the user was already viewing
// the target column.
func TestCardCreated_CursorFocusesNewCard(t *testing.T) {
	b := newLoadedTestBoard(t)
	createdNumber := 99

	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: createdNumber, Title: "New task"}})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	selected := updated.selectedCard()
	if selected.Number != createdNumber {
		t.Errorf("selectedCard().Number = %d, want %d (the newly created card)", selected.Number, createdNumber)
	}

	col := updated.Columns[updated.ActiveTab]
	if col.Cursor != len(col.Cards)-1 {
		t.Errorf("Cursor = %d, want %d (index of the newly appended card)", col.Cursor, len(col.Cards)-1)
	}
}

// TestCardCreated_CursorFocusesNewCard_WhenAlreadyViewingTargetColumn covers
// the "user wasn't already viewing it" branch of AC1 explicitly: when the
// user is already on Columns[0], ActiveTab must remain 0 (no-op switch) and
// the cursor must still move to the new card.
func TestCardCreated_CursorFocusesNewCard_WhenAlreadyViewingTargetColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	if b.ActiveTab != 0 {
		t.Fatalf("precondition: ActiveTab = %d, want 0", b.ActiveTab)
	}
	createdNumber := 99

	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: createdNumber, Title: "New task"}})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d, want 0 (already viewing the target column)", updated.ActiveTab)
	}
	if updated.selectedCard().Number != createdNumber {
		t.Errorf("selectedCard().Number = %d, want %d", updated.selectedCard().Number, createdNumber)
	}
}

// TestCardCreated_ClearsActiveSearchQuery covers AC3: an active search query
// is cleared on creation so the new card can't be hidden behind a stale query.
func TestCardCreated_ClearsActiveSearchQuery(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.searchQuery = "setup" // matches an existing card, not the new one
	createdNumber := 99

	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: createdNumber, Title: "Brand new task"}})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.searchQuery != "" {
		t.Errorf("searchQuery = %q after card created, want empty string (search must be cleared)", updated.searchQuery)
	}
	if updated.selectedCard().Number != createdNumber {
		t.Errorf("selectedCard().Number = %d, want %d (new card visible/selected once search is cleared)", updated.selectedCard().Number, createdNumber)
	}
}

// TestCardCreated_ClearsActiveLabelFilter covers AC3 for the label-filter case:
// an active global filter that would exclude the new card is cleared on
// creation, guaranteeing the new card is visible and selectable.
func TestCardCreated_ClearsActiveLabelFilter(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "infra" // an existing label the new (unlabeled) card won't have
	createdNumber := 99

	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: createdNumber, Title: "Unlabeled task"}})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.activeFilterType != filterTypeNone {
		t.Errorf("activeFilterType = %v after card created, want filterTypeNone (filter must be cleared)", updated.activeFilterType)
	}
	if updated.activeFilterValue != "" {
		t.Errorf("activeFilterValue = %q after card created, want empty string", updated.activeFilterValue)
	}
	if updated.selectedCard().Number != createdNumber {
		t.Errorf("selectedCard().Number = %d, want %d (new card visible/selected once filter is cleared)", updated.selectedCard().Number, createdNumber)
	}
}

// TestCardCreated_ScrollOffsetMakesNewCardVisible covers AC4: the scroll
// offset is updated (via the existing onCursorMoved/clampScrollOffset path)
// so the newly focused card, which sits below the fold in a long list, is
// actually visible on screen.
func TestCardCreated_ScrollOffsetMakesNewCardVisible(t *testing.T) {
	cardCount := 30
	height := 15 // panelHeight = 15 - 6 = 9, far fewer lines than 30 cards
	b := newBoardWithGeneratedCards(t, cardCount, "Card %d", 120, height)

	if b.Columns[0].ScrollOffset != 0 {
		t.Fatalf("precondition: ScrollOffset = %d, want 0 before card creation", b.Columns[0].ScrollOffset)
	}

	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: cardCount + 1, Title: "Bottom-of-list task"}})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	col := updated.Columns[0]
	if col.ScrollOffset <= 0 {
		t.Errorf("ScrollOffset = %d after creating a card appended past the visible area, want > 0", col.ScrollOffset)
	}
	if col.Cursor < col.ScrollOffset || col.Cursor >= len(col.Cards) {
		t.Errorf("Cursor = %d, ScrollOffset = %d, len(Cards) = %d: cursor must stay within bounds and at/after the scroll offset", col.Cursor, col.ScrollOffset, len(col.Cards))
	}
}

// TestCardCreated_DoesNotAutoFocusDetailPanel covers AC5 (not-focused case):
// detailFocused must remain false -- creation is cursor/list-highlight focus
// only, it never auto-opens the detail panel.
func TestCardCreated_DoesNotAutoFocusDetailPanel(t *testing.T) {
	b := newLoadedTestBoard(t)
	if b.detailFocused {
		t.Fatal("precondition: detailFocused should be false")
	}

	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "New task"}})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if updated.detailFocused {
		t.Error("detailFocused = true after card created, want false (detail panel must not auto-open)")
	}
}

// TestCardCreated_PreservesDetailFocusedWhenTrue covers AC5 (left-as-is case):
// if detailFocused was already true before creation, it must stay true --
// creation must not toggle it in either direction. Today's key handling has
// no path that reaches create mode with detailFocused already true, so this
// guards a currently-unreachable combination defensively rather than a live
// user flow.
func TestCardCreated_PreservesDetailFocusedWhenTrue(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.detailFocused = true

	m, _ := b.Update(cardCreatedMsg{card: provider.Card{Number: 99, Title: "New task"}})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if !updated.detailFocused {
		t.Error("detailFocused = false after card created, want true (must be left as-is, not toggled by creation)")
	}
}

// TestCardCreated_AssigneeChainUnaffectedByFocusChange covers AC6: the
// pending-assignee async chain (setAssigneesCmd/handleAssigneesUpdated) must
// still fire correctly, and the cursor focus set at creation must not be
// disturbed once the assignee update completes.
func TestCardCreated_AssigneeChainUnaffectedByFocusChange(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b)
	b = sendKey(t, b, arrowMsg(tea.KeyRight)) // select "alice"
	b = sendKey(t, b, arrowMsg(tea.KeyTab))   // back to title
	b = sendKey(t, b, keyMsg("Assigned task"))

	m, submitCmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	if submitCmd == nil {
		t.Fatal("precondition: submit cmd should be non-nil (spinner + createCardCmd)")
	}

	createdNumber := 99
	createdCard := provider.Card{Number: createdNumber, Title: "Assigned task"}
	m, cmd := b.Update(cardCreatedMsg{card: createdCard})
	b, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}

	if cmd == nil {
		t.Fatal("cmd should be non-nil when pending assignee exists (setAssigneesCmd chain must still fire)")
	}
	if b.selectedCard().Number != createdNumber {
		t.Fatalf("selectedCard().Number = %d, want %d (focus must be applied even when the assignee chain fires)", b.selectedCard().Number, createdNumber)
	}

	// Simulate the assignee chain completing.
	assigneeUpdatedCard := createdCard
	assigneeUpdatedCard.Assignees = []provider.Assignee{{Login: "alice"}}
	m2, _ := b.Update(assigneesUpdatedMsg{card: assigneeUpdatedCard})
	final, ok := m2.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m2)
	}

	if final.selectedCard().Number != createdNumber {
		t.Errorf("selectedCard().Number = %d after assignee update completed, want %d (focus must remain on the new card)", final.selectedCard().Number, createdNumber)
	}
	if len(final.selectedCard().Assignees) == 0 || final.selectedCard().Assignees[0].Login != "alice" {
		t.Errorf("selectedCard().Assignees = %v, want [alice] (assignee update must still apply to the new card)", final.selectedCard().Assignees)
	}
}

// --- Route create mode through the registry (#540 PR 1/2) ---
//
// handleCreateModeKey cuts over from a hardcoded `switch msg.Type` to
// keymap.Keymap.Lookup against ModeCreate (textBinding, keymap_text.go),
// mirroring handleDeleteModeKey's recognized-command-else-fall-through-to-
// textinput shape (#539 PR 2/2, the direct template for this ticket): a
// resolved-and-eligible create.* command dispatches through
// runCreateCommand; anything else (no match, OutcomePending, a non-command
// binding, a foreign command id, or an eligibility-gate miss) falls through
// to the focused field exactly like today's `default:` branch. The
// eligibility gate is create.assignee_prev/assignee_next's fifth condition:
// createCommandActive(id) is false whenever b.create.focus != 2 or
// len(b.create.assigneeOptions) == 0, in which case left/right must reach
// the focused textinput/textarea as cursor movement (focus 0/1) or be a
// genuine no-op (focus 2, no options) instead of cycling.

// --- left/right reach the focused text input at focus 0/1 ---

func TestCreateMode_Left_AtTitleFocus_MovesCursorInTitleInput(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "abc" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	if got := b.create.titleInput.Value(); got != "abc" {
		t.Fatalf("precondition: titleInput.Value() = %q, want %q", got, "abc")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyLeft)) // cursor: a b|c
	b = sendKey(t, b, keyMsg("X"))

	if got := b.create.titleInput.Value(); got != "abXc" {
		t.Errorf("titleInput.Value() = %q after left+type, want %q (left must move the textarea cursor, not be swallowed)", got, "abXc")
	}
}

func TestCreateMode_Right_AtTitleFocus_MovesCursorInTitleInput(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "abc" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))  // cursor: |abc
	b = sendKey(t, b, arrowMsg(tea.KeyRight)) // cursor: a|bc
	b = sendKey(t, b, keyMsg("X"))

	if got := b.create.titleInput.Value(); got != "aXbc" {
		t.Errorf("titleInput.Value() = %q after left*3+right+type, want %q (right must move the textarea cursor, not be swallowed)", got, "aXbc")
	}
}

func TestCreateMode_Left_AtLabelFocus_MovesCursorInLabelInput(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // title -> label
	for _, ch := range "abc" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	if got := b.create.labelInput.Value(); got != "abc" {
		t.Fatalf("precondition: labelInput.Value() = %q, want %q", got, "abc")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	b = sendKey(t, b, keyMsg("X"))

	if got := b.create.labelInput.Value(); got != "abXc" {
		t.Errorf("labelInput.Value() = %q after left+type, want %q (left must move the labelInput cursor, not be swallowed)", got, "abXc")
	}
}

func TestCreateMode_Right_AtLabelFocus_MovesCursorInLabelInput(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	for _, ch := range "abc" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	b = sendKey(t, b, keyMsg("X"))

	if got := b.create.labelInput.Value(); got != "aXbc" {
		t.Errorf("labelInput.Value() = %q after left*3+right+type, want %q (right must move the labelInput cursor, not be swallowed)", got, "aXbc")
	}
}

// --- left/right cycle the assignee only at focus 2 ---

func TestCreateMode_LeftRight_AtAssigneeFocus_NoOpWhenNoOptions(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b.create.focus = 2
	b.create.assigneeOptions = nil
	b.create.assigneeIndex = 0

	m, cmd := b.Update(arrowMsg(tea.KeyLeft))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.create.assigneeIndex != 0 {
		t.Errorf("assigneeIndex = %d after left with zero assigneeOptions, want unchanged 0", b2.create.assigneeIndex)
	}
	if cmd != nil {
		t.Error("left at focus 2 with zero assigneeOptions should not fire a cmd")
	}

	m, cmd = b2.Update(arrowMsg(tea.KeyRight))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.create.assigneeIndex != 0 {
		t.Errorf("assigneeIndex = %d after right with zero assigneeOptions, want unchanged 0", b3.create.assigneeIndex)
	}
	if cmd != nil {
		t.Error("right at focus 2 with zero assigneeOptions should not fire a cmd")
	}
}

// --- validationErr: cleared by left/right at focus 2, not by esc/enter/tab ---

func TestCreateMode_Left_AtAssigneeFocus_ClearsValidationErr(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b) // focus == 2
	b.validationErr = "stale error"

	b = sendKey(t, b, arrowMsg(tea.KeyLeft))

	if b.validationErr != "" {
		t.Errorf("validationErr = %q after left at focus 2, want empty (left/right clear validationErr)", b.validationErr)
	}
}

func TestCreateMode_Right_AtAssigneeFocus_ClearsValidationErr(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b) // focus == 2
	b.validationErr = "stale error"

	b = sendKey(t, b, arrowMsg(tea.KeyRight))

	if b.validationErr != "" {
		t.Errorf("validationErr = %q after right at focus 2, want empty (left/right clear validationErr)", b.validationErr)
	}
}

func TestCreateMode_Tab_DoesNotClearValidationErr(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b.validationErr = "stale error"

	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	if b.validationErr != "stale error" {
		t.Errorf("validationErr = %q after tab, want unchanged %q (tab must not clear validationErr)", b.validationErr, "stale error")
	}
}

func TestCreateMode_Esc_DoesNotClearValidationErr(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b.validationErr = "stale error"

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.validationErr != "stale error" {
		t.Errorf("validationErr = %q after esc, want unchanged %q (esc must not clear validationErr)", b.validationErr, "stale error")
	}
}

func TestCreateMode_Enter_DoesNotClearValidationErrOnSuccessfulSubmit(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	for _, ch := range "Valid title" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	b.validationErr = "stale error"

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("expected a non-nil cmd (spinner + createCardCmd) from a valid submit")
	}
	if b2.validationErr != "stale error" {
		t.Errorf("validationErr = %q after successful submit, want unchanged %q (enter must not clear validationErr as a side effect)", b2.validationErr, "stale error")
	}
	execCmds(cmd)
}

// --- keymaps.create remap + explicit unbind ---

func TestCreateMode_RemapCancelKey_DispatchAndHintStaySync(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCreate: {
			"esc": keymap.UnboundBinding(),
			"f1":  keymap.CommandBinding(keymap.CommandCreateCancel),
		},
	}, nil)

	before := b.mode
	m, cmd := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after unbound (now-old) esc = %v, want unchanged (%v)", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unbound esc should not fire a cmd")
	}

	m, cmd = b2.Update(arrowMsg(tea.KeyF1))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.mode != normalMode {
		t.Errorf("mode after remapped 'f1' = %v, want normalMode", b3.mode)
	}
	if cmd != nil {
		t.Error("cancel via remapped 'f1' should not fire a cmd, matching today's esc-cancel behavior")
	}
}

// TestCreateMode_UnbindAssigneeCycleKeys_LeftRightNoOpAtFocus2 encodes AC #2
// ("a user config binding a non-printable key already used by a built-in
// overrides it; the built-in's old key stops working") for create's
// assignee-cycle commands specifically: unbinding both "left" and "right"
// from create.assignee_prev/assignee_next must make them a genuine no-op at
// focus 2, not a silent hardcoded fallback to the same cycling behavior.
func TestCreateMode_UnbindAssigneeCycleKeys_LeftRightNoOpAtFocus2(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b) // focus == 2
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCreate: {
			"left":  keymap.UnboundBinding(),
			"right": keymap.UnboundBinding(),
		},
	}, nil)
	before := b.create.assigneeIndex

	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.create.assigneeIndex != before {
		t.Errorf("assigneeIndex = %d after unbound right, want unchanged %d", b.create.assigneeIndex, before)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.create.assigneeIndex != before {
		t.Errorf("assigneeIndex = %d after unbound left, want unchanged %d", b.create.assigneeIndex, before)
	}
}

// TestCreateMode_RemapAssigneeCycleKeys_OldKeysNoLongerCycleNewKeysDo is
// UnbindAssigneeCycleKeys' remap sibling: rebinding assignee_prev/next onto
// "f1"/"f2" (a non-printable pair -- textBinding's ConsumesPrintableRunes
// guard refuses bare printable runes for ModeCreate regardless of focus, so
// a printable remap target like "h"/"l" could never dispatch through the
// registry here) must make the OLD "left"/"right" keys stop cycling and the
// NEW "f1"/"f2" keys start.
func TestCreateMode_RemapAssigneeCycleKeys_OldKeysNoLongerCycleNewKeysDo(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b) // focus == 2
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCreate: {
			"left":  keymap.UnboundBinding(),
			"right": keymap.UnboundBinding(),
			"f1":    keymap.CommandBinding(keymap.CommandCreateAssigneePrev),
			"f2":    keymap.CommandBinding(keymap.CommandCreateAssigneeNext),
		},
	}, nil)
	before := b.create.assigneeIndex

	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.create.assigneeIndex != before {
		t.Errorf("assigneeIndex = %d after old (now unbound) right, want unchanged %d", b.create.assigneeIndex, before)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyF2))
	if b.create.assigneeIndex == before {
		t.Error("assigneeIndex unchanged after remapped 'f2', want it to cycle forward")
	}
}

// TestCreateMode_EscBoundToAction_FallsThroughToTextinputNotDispatched and
// TestCreateMode_ForeignCommandIDBoundToKey_FallsThroughToTextinputNotDispatched
// cover two of handleCreateModeKey's four documented fall-through outcomes
// (mirroring handleDeleteModeKey's TestDeleteMode_CommentStep_EscBoundToAction_
// FallsThroughToTextinputNotDispatched /
// TestDeleteMode_CommentStep_ForeignCommandIDBoundToKey_
// FallsThroughToTextinputNotDispatched, delete_mode_test.go): a resolved
// non-command BindingAction, and a command id valid elsewhere in the catalog
// but foreign to ModeCreate's own five ids. Neither is dispatched -- the
// user config's semantic validator only checks a bound command id is
// catalogued somewhere, not that it belongs to the mode it's bound under
// (internal/config/keymap_semantic_validate.go), so both are reachable via a
// legal (if unusual) keymaps.create config.
func TestCreateMode_EscBoundToAction_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCreate: {
			"esc": keymap.ActionBinding(keymap.Action{Name: "Noop", Type: "url", URL: "https://example.com/{number}"}),
		},
	}, nil)
	before := b.create.titleInput.Value()

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != createMode {
		t.Errorf("mode after esc resolved to a non-command action = %v, want unchanged createMode (must not dispatch the action)", b2.mode)
	}
	if got := b2.create.titleInput.Value(); got != before {
		t.Errorf("titleInput.Value() = %q after esc resolved to a non-command action, want unchanged %q (falls through to the textinput)", got, before)
	}
}

func TestCreateMode_ForeignCommandIDBoundToKey_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCreate: {
			"f1": keymap.CommandBinding(keymap.CommandCloseConfirmConfirm),
		},
	}, nil)
	before := b.create.titleInput.Value()

	m, _ := b.Update(arrowMsg(tea.KeyF1))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != createMode {
		t.Errorf("mode after a foreign command id (close_confirm.confirm) bound into ModeCreate = %v, want unchanged createMode", b2.mode)
	}
	if got := b2.create.titleInput.Value(); got != before {
		t.Errorf("titleInput.Value() = %q after a foreign command id bound into ModeCreate, want unchanged %q (falls through to the textinput, not dispatched)", got, before)
	}
}

// --- createModalHints(): byte-identity, focus-gated Cycle hint, remap ---

func TestCreateModalHints_DefaultParityMatchesTodaysLiteral(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n")) // focus == 0, no Cycle hint

	hints := b.createModalHints()
	want := []Hint{
		{Key: "esc", Desc: "Cancel"},
		{Key: "tab", Desc: "Next"},
		{Key: "enter", Desc: "Submit"},
	}
	if !reflect.DeepEqual(hints, want) {
		t.Errorf("createModalHints() at focus 0 = %+v, want %+v (today's inline literal, byte-identical, no Cycle hint since focus != 2)", hints, want)
	}
}

func TestCreateModalHints_CycleHintPresentOnlyAtFocus2(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b) // focus == 2

	hints := b.createModalHints()
	want := []Hint{
		{Key: "esc", Desc: "Cancel"},
		{Key: "tab", Desc: "Next"},
		{Key: "◀/▶", Desc: "Cycle"},
		{Key: "enter", Desc: "Submit"},
	}
	if !reflect.DeepEqual(hints, want) {
		t.Errorf("createModalHints() at focus 2 = %+v, want %+v", hints, want)
	}
}

func TestCreateModalHints_RemapCancelKey_ReflectsNewKey(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCreate: {
			"esc": keymap.UnboundBinding(),
			"q":   keymap.CommandBinding(keymap.CommandCreateCancel),
		},
	}, nil)

	hints := b.createModalHints()
	if got := hintDesc(t, hints, "q"); got != "Cancel" {
		t.Errorf("createModalHints() after remap: Desc for %q = %q, want %q", "q", got, "Cancel")
	}
	for _, h := range hints {
		if h.Key == "esc" {
			t.Errorf("createModalHints() still advertises the old 'esc' key after remap, got %+v", hints)
		}
	}
}

// TestCreateModalHints_RemapAssigneeCycleKeys_ReflectsNewKeysNoGlyph covers
// arrowGlyphs' documented fallback (keymap_dispatch.go): remapping the
// Cycle hint onto keys outside the glyph map (h/l) must render the raw key
// text, not a stale/misleading arrow glyph.
func TestCreateModalHints_RemapAssigneeCycleKeys_ReflectsNewKeysNoGlyph(t *testing.T) {
	b := newCreateTestBoardWithCollaborators(t)
	b = enterCreateAndFocusAssignee(t, b) // focus == 2
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCreate: {
			"left":  keymap.UnboundBinding(),
			"right": keymap.UnboundBinding(),
			"h":     keymap.CommandBinding(keymap.CommandCreateAssigneePrev),
			"l":     keymap.CommandBinding(keymap.CommandCreateAssigneeNext),
		},
	}, nil)

	hints := b.createModalHints()
	if got := hintDesc(t, hints, "h/l"); got != "Cycle" {
		t.Errorf("createModalHints() after remap onto h/l: Desc for %q = %q, want %q", "h/l", got, "Cycle")
	}
	for _, h := range hints {
		if h.Key == "◀/▶" {
			t.Errorf("createModalHints() still advertises the arrow glyph hint after remapping off left/right, got %+v", hints)
		}
	}
}

// --- hint<->dispatch invariant across all three focus positions ---

// TestCreateModalHints_KeysAlwaysDispatch_AllFocusPositionsDefaultAndRemappedTables
// is the hint<->dispatch invariant test named in the #540 plan's Explicit
// Risk Coverage, adapted to create's three focus positions (title/label/
// assignee) and its []Hint-slice hint bar: for every key advertised by
// b.createModalHints() at every focus position, it must resolve through
// b.textBinding -- the real dispatch path handleCreateModeKey uses -- against
// ModeCreate. Run against both the default table and a remapped/unbound
// table, mirroring TestDeleteMode_HintKeysAlwaysDispatch_BothStepsDefaultAndRemappedTables
// (delete_mode_test.go).
func TestCreateModalHints_KeysAlwaysDispatch_AllFocusPositionsDefaultAndRemappedTables(t *testing.T) {
	arrowGlyphToKey := map[string]string{"◀": "left", "▶": "right"}
	keyMsgForLabel := func(label string) tea.KeyMsg {
		if raw, ok := arrowGlyphToKey[label]; ok {
			label = raw
		}
		switch label {
		case "enter":
			return arrowMsg(tea.KeyEnter)
		case "esc":
			return arrowMsg(tea.KeyEsc)
		case "tab":
			return arrowMsg(tea.KeyTab)
		case "left":
			return arrowMsg(tea.KeyLeft)
		case "right":
			return arrowMsg(tea.KeyRight)
		case "f1":
			return arrowMsg(tea.KeyF1)
		case "f2":
			return arrowMsg(tea.KeyF2)
		default:
			return keyMsg(label)
		}
	}

	tests := []struct {
		name  string
		modes map[keymap.Mode]keymap.Table
	}{
		{name: "default table", modes: nil},
		{
			name: "remapped table",
			modes: map[keymap.Mode]keymap.Table{
				keymap.ModeCreate: {
					"esc": keymap.UnboundBinding(),
					"f1":  keymap.CommandBinding(keymap.CommandCreateCancel),
					"tab": keymap.UnboundBinding(),
					"f2":  keymap.CommandBinding(keymap.CommandCreateNextField),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newCreateTestBoardWithCollaborators(t)
			b = sendKey(t, b, keyMsg("n"))
			if tc.modes != nil {
				b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			}

			for _, focus := range []int{0, 1, 2} {
				b.create.focus = focus
				for _, h := range b.createModalHints() {
					if h.Key == "" {
						continue
					}
					for _, key := range strings.Split(h.Key, "/") {
						if _, ok := b.textBinding(keymap.ModeCreate, keyMsgForLabel(key)); !ok {
							t.Errorf("focus %d, hint %+v: textBinding(ModeCreate, %q) not found, want a match (every advertised key must actually dispatch through the real handler)", focus, h, key)
						}
					}
				}
			}
		})
	}
}

// --- ctrl+c always quits, regardless of user keymap config ---

func TestCreateMode_CtrlCQuits_EvenWithOverriddenKeymap(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("n"))
	// Attempt to steal ctrl+c for something else -- update.go's global
	// ctrl+c-always-quits check runs before any mode dispatch, so this must
	// have no effect.
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeCreate: {"ctrl+c": keymap.CommandBinding(keymap.CommandCreateSubmit)},
	}, nil)

	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in createMode should return a non-nil Cmd (tea.Quit), even with an overridden keymap")
	}
}
