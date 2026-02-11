package provider

import (
	"context"
	"errors"
	"strings"
)

// Compile-time check: *FakeProvider implements BoardProvider.
var _ BoardProvider = (*FakeProvider)(nil)

// FakeProvider is an in-memory BoardProvider for development and testing.
type FakeProvider struct {
	columns    []Column
	nextNumber int
}

// NewFakeProvider returns a FakeProvider pre-populated with hardcoded Kanban data.
func NewFakeProvider() *FakeProvider {
	return &FakeProvider{
		columns: []Column{
			{
				Title: "New",
				Cards: []Card{
					{Number: 1, Title: "Setup CI", Label: "infra"},
					{Number: 2, Title: "Data model", Label: "design"},
					{Number: 3, Title: "Add README", Label: "docs"},
				},
			},
			{
				Title: "Refined",
				Cards: []Card{
					{Number: 4, Title: "User auth", Label: "feature"},
					{Number: 5, Title: "API routes", Label: "backend"},
					{Number: 6, Title: "Error types", Label: "backend"},
					{Number: 7, Title: "DB migrate", Label: "infra"},
				},
			},
			{
				Title: "Implementing",
				Cards: []Card{
					{Number: 8, Title: "Board view", Label: "feature"},
					{Number: 9, Title: "Key binds", Label: "feature"},
					{Number: 10, Title: "Col nav", Label: "feature"},
					{Number: 11, Title: "Lipgloss", Label: "ui"},
					{Number: 12, Title: "Config", Label: "feature"},
				},
			},
			{
				Title: "PR Ready",
				Cards: []Card{
					{Number: 13, Title: "Fix clamp", Label: "bug"},
					{Number: 14, Title: "Refactor", Label: "chore"},
					{Number: 15, Title: "Help bar", Label: "ui"},
				},
			},
		},
		nextNumber: 16,
	}
}

// FetchBoard returns a copy of the current board state.
func (f *FakeProvider) FetchBoard(_ context.Context) (Board, error) {
	cols := make([]Column, len(f.columns))
	for i, col := range f.columns {
		cards := make([]Card, len(col.Cards))
		copy(cards, col.Cards)
		cols[i] = Column{Title: col.Title, Cards: cards}
	}
	return Board{Columns: cols}, nil
}

// CreateCard adds a new card to the first column with an auto-incremented number.
// Title must be non-empty after trimming whitespace. Empty label is allowed.
func (f *FakeProvider) CreateCard(_ context.Context, title string, label string) (Card, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Card{}, errors.New("title is required")
	}

	card := Card{
		Number: f.nextNumber,
		Title:  title,
		Label:  label,
	}
	f.columns[0].Cards = append(f.columns[0].Cards, card)
	f.nextNumber++

	return card, nil
}
