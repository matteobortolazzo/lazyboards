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
					{Number: 1, Title: "Setup CI", Labels: []string{"infra"}, Body: "Configure GitHub Actions for CI pipeline."},
					{Number: 2, Title: "Data model", Labels: []string{"design"}, Body: "Design the core data model for boards and cards."},
					{Number: 3, Title: "Add README", Labels: []string{"docs"}},
				},
			},
			{
				Title: "Refined",
				Cards: []Card{
					{Number: 4, Title: "User auth", Labels: []string{"feature"}},
					{Number: 5, Title: "API routes", Labels: []string{"backend"}},
					{Number: 6, Title: "Error types", Labels: []string{"backend"}},
					{Number: 7, Title: "DB migrate", Labels: []string{"infra"}},
				},
			},
			{
				Title: "Implementing",
				Cards: []Card{
					{Number: 8, Title: "Board view", Labels: []string{"feature"}},
					{Number: 9, Title: "Key binds", Labels: []string{"feature"}},
					{Number: 10, Title: "Col nav", Labels: []string{"feature"}},
					{Number: 11, Title: "Lipgloss", Labels: []string{"ui"}},
					{Number: 12, Title: "Config", Labels: []string{"feature"}},
				},
			},
			{
				Title: "PR Ready",
				Cards: []Card{
					{Number: 13, Title: "Fix clamp", Labels: []string{"bug"}},
					{Number: 14, Title: "Refactor", Labels: []string{"chore"}},
					{Number: 15, Title: "Help bar", Labels: []string{"ui"}},
				},
			},
			{
				Title: "Done",
				Cards: []Card{
					{Number: 16, Title: "Go module", Labels: []string{"infra"}},
					{Number: 17, Title: "Scaffold", Labels: []string{"feature"}},
					{Number: 18, Title: "Fake data", Labels: []string{"feature"}},
					{Number: 19, Title: "Tests", Labels: []string{"test"}},
				},
			},
		},
		nextNumber: 20,
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

	var labels []string
	if label != "" {
		labels = []string{label}
	}
	card := Card{
		Number: f.nextNumber,
		Title:  title,
		Labels: labels,
	}
	f.columns[0].Cards = append(f.columns[0].Cards, card)
	f.nextNumber++

	return card, nil
}
