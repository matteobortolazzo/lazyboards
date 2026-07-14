package provider

import (
	"context"
	"strconv"
	"strings"
	"testing"
)

// expectedColumnCount is the number of Kanban columns in a fresh board.
const expectedColumnCount = 3

// expectedColumnTitles are the Kanban column names in order.
var expectedColumnTitles = []string{"New", "Refined", "Implementing"}

func TestFetchBoard_ReturnsExpectedColumns(t *testing.T) {
	fp := NewFakeProvider()
	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	if len(board.Columns) != expectedColumnCount {
		t.Fatalf("got %d columns, want %d", len(board.Columns), expectedColumnCount)
	}

	for i, col := range board.Columns {
		if col.Title != expectedColumnTitles[i] {
			t.Errorf("column %d title = %q, want %q", i, col.Title, expectedColumnTitles[i])
		}
	}
}

func TestFetchBoard_EachColumnHasAtLeastOneCard(t *testing.T) {
	fp := NewFakeProvider()
	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	for i, col := range board.Columns {
		if len(col.Cards) == 0 {
			t.Errorf("column %d (%q) has no cards", i, col.Title)
		}
	}
}

func TestFetchBoard_AllCardsHaveRequiredFields(t *testing.T) {
	fp := NewFakeProvider()
	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	for _, col := range board.Columns {
		for _, card := range col.Cards {
			if card.Number == 0 {
				t.Errorf("card in column %q has zero Number", col.Title)
			}
			if card.Title == "" {
				t.Errorf("card #%d in column %q has empty Title", card.Number, col.Title)
			}
			if len(card.Labels) == 0 {
				t.Errorf("card #%d in column %q has empty Labels", card.Number, col.Title)
			}
		}
	}
}

func TestCreateCard_ReturnsAutoIncrementedNumber(t *testing.T) {
	fp := NewFakeProvider()

	// Fetch the board to find the highest existing card number.
	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	maxNumber := 0
	for _, col := range board.Columns {
		for _, card := range col.Cards {
			if card.Number > maxNumber {
				maxNumber = card.Number
			}
		}
	}

	card, err := fp.CreateCard(context.Background(), "New task", "feature")
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	if card.Number <= maxNumber {
		t.Errorf("created card number %d is not higher than max existing %d", card.Number, maxNumber)
	}
}

func TestCreateCard_AppearsInSubsequentFetchBoard(t *testing.T) {
	fp := NewFakeProvider()
	title := "Integration card"
	label := "test"

	created, err := fp.CreateCard(context.Background(), title, label)
	if err != nil {
		t.Fatalf("CreateCard returned error: %v", err)
	}

	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}

	// The created card should appear in the first column.
	firstCol := board.Columns[0]
	found := false
	for _, card := range firstCol.Cards {
		if card.Number == created.Number && card.Title == title && len(card.Labels) > 0 && card.Labels[0].Name == label {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("created card #%d not found in first column %q", created.Number, firstCol.Title)
	}
}

func TestCreateCard_EmptyTitleReturnsError(t *testing.T) {
	fp := NewFakeProvider()

	_, err := fp.CreateCard(context.Background(), "", "bug")
	if err == nil {
		t.Fatal("expected error for empty title, got nil")
	}

	// Also test whitespace-only title.
	_, err = fp.CreateCard(context.Background(), "   ", "bug")
	if err == nil {
		t.Fatal("expected error for whitespace-only title, got nil")
	}
}

func TestCreateCard_EmptyLabelIsAllowed(t *testing.T) {
	fp := NewFakeProvider()

	card, err := fp.CreateCard(context.Background(), "No label card", "")
	if err != nil {
		t.Fatalf("CreateCard with empty label returned error: %v", err)
	}

	if card.Title != "No label card" {
		t.Errorf("card title = %q, want %q", card.Title, "No label card")
	}
	if len(card.Labels) != 0 {
		t.Errorf("card labels = %v, want empty slice", card.Labels)
	}
}

func TestCreateCard_MultipleCallsProduceUniqueIncrementingNumbers(t *testing.T) {
	fp := NewFakeProvider()

	card1, err := fp.CreateCard(context.Background(), "First", "a")
	if err != nil {
		t.Fatalf("CreateCard #1 returned error: %v", err)
	}

	card2, err := fp.CreateCard(context.Background(), "Second", "b")
	if err != nil {
		t.Fatalf("CreateCard #2 returned error: %v", err)
	}

	card3, err := fp.CreateCard(context.Background(), "Third", "c")
	if err != nil {
		t.Fatalf("CreateCard #3 returned error: %v", err)
	}

	if card1.Number == card2.Number || card2.Number == card3.Number || card1.Number == card3.Number {
		t.Errorf("card numbers are not unique: %d, %d, %d", card1.Number, card2.Number, card3.Number)
	}

	if card2.Number <= card1.Number {
		t.Errorf("card2 number %d is not greater than card1 number %d", card2.Number, card1.Number)
	}
	if card3.Number <= card2.Number {
		t.Errorf("card3 number %d is not greater than card2 number %d", card3.Number, card2.Number)
	}
}

func TestFakeProvider_UpdateCard_Success(t *testing.T) {
	fp := NewFakeProvider()

	// Pick an existing card from the board to update.
	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	existingCard := board.Columns[0].Cards[0]

	updatedTitle := "Updated " + existingCard.Title
	updatedBody := "New body content"
	updatedLabels := []string{"updated-label"}

	card, err := fp.UpdateCard(context.Background(), existingCard.Number, updatedTitle, updatedBody, updatedLabels)
	if err != nil {
		t.Fatalf("UpdateCard returned error: %v", err)
	}

	// Verify the returned card has the updated fields.
	if card.Number != existingCard.Number {
		t.Errorf("card.Number = %d, want %d", card.Number, existingCard.Number)
	}
	if card.Title != updatedTitle {
		t.Errorf("card.Title = %q, want %q", card.Title, updatedTitle)
	}
	if card.Body != updatedBody {
		t.Errorf("card.Body = %q, want %q", card.Body, updatedBody)
	}
	if len(card.Labels) != len(updatedLabels) {
		t.Fatalf("card.Labels has %d entries, want %d", len(card.Labels), len(updatedLabels))
	}
	if card.Labels[0].Name != updatedLabels[0] {
		t.Errorf("card.Labels[0].Name = %q, want %q", card.Labels[0].Name, updatedLabels[0])
	}

	// Verify the update is persisted — fetch the board again and find the card.
	board, err = fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard after update returned error: %v", err)
	}

	found := false
	for _, col := range board.Columns {
		for _, c := range col.Cards {
			if c.Number == existingCard.Number {
				found = true
				if c.Title != updatedTitle {
					t.Errorf("persisted card.Title = %q, want %q", c.Title, updatedTitle)
				}
				if c.Body != updatedBody {
					t.Errorf("persisted card.Body = %q, want %q", c.Body, updatedBody)
				}
			}
		}
	}
	if !found {
		t.Errorf("updated card #%d not found in board after update", existingCard.Number)
	}
}

func TestFakeProvider_UpdateCard_NotFound(t *testing.T) {
	fp := NewFakeProvider()
	nonExistentNumber := 9999

	_, err := fp.UpdateCard(context.Background(), nonExistentNumber, "title", "body", []string{})
	if err == nil {
		t.Fatal("expected error for non-existent card number, got nil")
	}
}

func TestFakeProvider_UpdateCard_EmptyTitle(t *testing.T) {
	fp := NewFakeProvider()

	// Use an existing card number from the fake board.
	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	existingNumber := board.Columns[0].Cards[0].Number

	// Empty string title should return an error.
	_, err = fp.UpdateCard(context.Background(), existingNumber, "", "body", []string{})
	if err == nil {
		t.Fatal("expected error for empty title, got nil")
	}

	// Whitespace-only title should also return an error.
	_, err = fp.UpdateCard(context.Background(), existingNumber, "   ", "body", []string{})
	if err == nil {
		t.Fatal("expected error for whitespace-only title, got nil")
	}
}

func TestFakeProvider_CreateLabel(t *testing.T) {
	fp := NewFakeProvider()

	err := fp.CreateLabel(context.Background(), "new-label")
	if err != nil {
		t.Fatalf("CreateLabel returned error: %v, want nil (no-op)", err)
	}
}

// --- ListLabels Tests ---

func TestFakeProvider_ListLabels_IncludesOnCardAndExtraLabels(t *testing.T) {
	fp := NewFakeProvider()

	labels, err := fp.ListLabels(context.Background())
	if err != nil {
		t.Fatalf("ListLabels returned error: %v", err)
	}

	labelSet := make(map[string]bool, len(labels))
	for _, l := range labels {
		labelSet[l] = true
	}

	// Every label attached to a card must appear in the repo label set.
	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	onCard := make(map[string]bool)
	for _, col := range board.Columns {
		for _, card := range col.Cards {
			for _, l := range card.Labels {
				onCard[l.Name] = true
				if !labelSet[l.Name] {
					t.Errorf("ListLabels missing on-card label %q", l.Name)
				}
			}
		}
	}

	// The repo label set must include at least one label not attached to any
	// card, so the "existing repo label not shown on the board" scenario is
	// testable.
	extraFound := false
	for _, l := range labels {
		if !onCard[l] {
			extraFound = true
			break
		}
	}
	if !extraFound {
		t.Errorf("ListLabels should include an extra label not on any card, got %v", labels)
	}
}

// --- FetchCollaborators Tests ---

func TestFakeProvider_FetchCollaborators(t *testing.T) {
	fp := NewFakeProvider()

	collaborators, err := fp.FetchCollaborators(context.Background())
	if err != nil {
		t.Fatalf("FetchCollaborators returned error: %v", err)
	}

	// Fake provider should return a non-empty hardcoded collaborator list.
	if len(collaborators) == 0 {
		t.Error("FetchCollaborators returned empty list, want hardcoded collaborators")
	}

	// Each collaborator should have a non-empty login.
	for i, c := range collaborators {
		if c.Login == "" {
			t.Errorf("collaborators[%d].Login is empty, want non-empty login", i)
		}
	}
}

// --- SetAssignees Tests ---

func TestFakeProvider_SetAssignees_Success(t *testing.T) {
	fp := NewFakeProvider()

	// Get an existing card number from the board.
	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	existingCard := board.Columns[0].Cards[0]

	newAssignees := []string{"charlie", "dave"}
	card, err := fp.SetAssignees(context.Background(), existingCard.Number, newAssignees)
	if err != nil {
		t.Fatalf("SetAssignees returned error: %v", err)
	}

	// Verify the returned card has the updated assignees.
	if len(card.Assignees) != len(newAssignees) {
		t.Fatalf("card.Assignees has %d entries, want %d", len(card.Assignees), len(newAssignees))
	}
	for i, login := range newAssignees {
		if card.Assignees[i].Login != login {
			t.Errorf("card.Assignees[%d].Login = %q, want %q", i, card.Assignees[i].Login, login)
		}
	}

	// Verify the update is persisted — fetch the board again and check.
	board, err = fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard after SetAssignees returned error: %v", err)
	}
	found := false
	for _, col := range board.Columns {
		for _, c := range col.Cards {
			if c.Number == existingCard.Number {
				found = true
				if len(c.Assignees) != len(newAssignees) {
					t.Errorf("persisted card.Assignees has %d entries, want %d", len(c.Assignees), len(newAssignees))
				}
			}
		}
	}
	if !found {
		t.Errorf("card #%d not found after SetAssignees", existingCard.Number)
	}
}

func TestFakeProvider_SetAssignees_NotFound(t *testing.T) {
	fp := NewFakeProvider()
	nonExistentNumber := 9999

	_, err := fp.SetAssignees(context.Background(), nonExistentNumber, []string{"alice"})
	if err == nil {
		t.Fatal("expected error for non-existent card number, got nil")
	}
}

// --- CloseCard Tests ---

func TestFakeProvider_CloseCard_Success(t *testing.T) {
	fp := NewFakeProvider()

	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	existingCard := board.Columns[0].Cards[0]

	closed, err := fp.CloseCard(context.Background(), existingCard.Number)
	if err != nil {
		t.Fatalf("CloseCard returned error: %v", err)
	}

	if closed.Number != existingCard.Number {
		t.Errorf("closed.Number = %d, want %d", closed.Number, existingCard.Number)
	}
	if closed.Title != existingCard.Title {
		t.Errorf("closed.Title = %q, want %q", closed.Title, existingCard.Title)
	}
}

func TestFakeProvider_CloseCard_NotFound(t *testing.T) {
	fp := NewFakeProvider()
	nonExistentNumber := 9999

	_, err := fp.CloseCard(context.Background(), nonExistentNumber)
	if err == nil {
		t.Fatal("expected error for non-existent card number, got nil")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(nonExistentNumber)) {
		t.Errorf("error = %q, want it to mention card number %d", err.Error(), nonExistentNumber)
	}
}

// --- AddComment Tests ---

func TestFakeProvider_AddComment_Success(t *testing.T) {
	fp := NewFakeProvider()

	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	existingCard := board.Columns[0].Cards[0]
	commentBody := "Closing this out, thanks!"

	err = fp.AddComment(context.Background(), existingCard.Number, commentBody)
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}

	recorded := fp.Comments[existingCard.Number]
	if len(recorded) == 0 {
		t.Fatalf("no comment recorded for card #%d", existingCard.Number)
	}
	if recorded[len(recorded)-1] != commentBody {
		t.Errorf("recorded comment = %q, want %q", recorded[len(recorded)-1], commentBody)
	}
}

func TestFakeProvider_AddComment_NotFound(t *testing.T) {
	fp := NewFakeProvider()
	nonExistentNumber := 9999

	err := fp.AddComment(context.Background(), nonExistentNumber, "test comment")
	if err == nil {
		t.Fatal("expected error for non-existent card number, got nil")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(nonExistentNumber)) {
		t.Errorf("error = %q, want it to mention card number %d", err.Error(), nonExistentNumber)
	}
}

// --- DeleteCard Tests ---

// TestFakeProvider_DeleteCard_Success asserts DeleteCard actually removes the
// card from the fixture (unlike CloseCard, which only finds/returns without
// mutating) -- a subsequent FetchBoard must no longer contain it.
func TestFakeProvider_DeleteCard_Success(t *testing.T) {
	fp := NewFakeProvider()

	board, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard returned error: %v", err)
	}
	existingCard := board.Columns[0].Cards[0]

	if err := fp.DeleteCard(context.Background(), existingCard.Number); err != nil {
		t.Fatalf("DeleteCard returned error: %v", err)
	}

	boardAfter, err := fp.FetchBoard(context.Background())
	if err != nil {
		t.Fatalf("FetchBoard after DeleteCard returned error: %v", err)
	}
	for _, col := range boardAfter.Columns {
		for _, c := range col.Cards {
			if c.Number == existingCard.Number {
				t.Fatalf("card #%d still present after DeleteCard", existingCard.Number)
			}
		}
	}
}

func TestFakeProvider_DeleteCard_NotFound(t *testing.T) {
	fp := NewFakeProvider()
	nonExistentNumber := 9999

	err := fp.DeleteCard(context.Background(), nonExistentNumber)
	if err == nil {
		t.Fatal("expected error for non-existent card number, got nil")
	}
	if !strings.Contains(err.Error(), strconv.Itoa(nonExistentNumber)) {
		t.Errorf("error = %q, want it to mention card number %d", err.Error(), nonExistentNumber)
	}
}

// --- GetAuthenticatedUser Tests ---

func TestFakeProvider_GetAuthenticatedUser(t *testing.T) {
	fp := NewFakeProvider()

	login, err := fp.GetAuthenticatedUser(context.Background())
	if err != nil {
		t.Fatalf("GetAuthenticatedUser returned error: %v", err)
	}

	// Fake provider should return a hardcoded non-empty login.
	if login == "" {
		t.Error("GetAuthenticatedUser returned empty login, want hardcoded user")
	}
}
