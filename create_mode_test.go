package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

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
