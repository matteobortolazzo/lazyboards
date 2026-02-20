package provider

import (
	"context"
	"testing"
)

// expectedColumnCount is the number of Kanban columns in a fresh board.
const expectedColumnCount = 4

// expectedColumnTitles are the Kanban column names in order.
var expectedColumnTitles = []string{"New", "Refined", "Implementing", "Implemented"}

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
