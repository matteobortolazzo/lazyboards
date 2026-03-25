package provider

import (
	"context"
	"errors"
	"fmt"
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
					{Number: 1, Title: "Setup CI", Labels: []Label{{Name: "infra"}}, Body: "Configure GitHub Actions for CI pipeline.", Assignees: []Assignee{{Login: "alice"}}},
					{Number: 2, Title: "Data model", Labels: []Label{{Name: "design"}}, Body: "Design the core data model for boards and cards.", Assignees: []Assignee{{Login: "alice"}, {Login: "bob"}}, LinkedPRs: []LinkedPR{
						{Number: 20, Title: "feat: add data model", URL: "https://github.com/owner/repo/pull/20"},
					}},
					{Number: 3, Title: "Add README", Labels: []Label{{Name: "docs"}}, LinkedPRs: []LinkedPR{
						{Number: 30, Title: "docs: add README", URL: "https://github.com/owner/repo/pull/30"},
						{Number: 31, Title: "docs: improve README", URL: "https://github.com/owner/repo/pull/31"},
					}},
				},
			},
			{
				Title: "Refined",
				Cards: []Card{
					{Number: 4, Title: "User auth", Labels: []Label{{Name: "feature"}}},
					{Number: 5, Title: "API routes", Labels: []Label{{Name: "backend"}}},
					{Number: 6, Title: "Error types", Labels: []Label{{Name: "backend"}}},
					{Number: 7, Title: "DB migrate", Labels: []Label{{Name: "infra"}}},
				},
			},
			{
				Title: "Implementing",
				Cards: []Card{
					{Number: 8, Title: "Board view", Labels: []Label{{Name: "feature"}}},
					{Number: 9, Title: "Key binds", Labels: []Label{{Name: "feature"}}},
					{Number: 10, Title: "Col nav", Labels: []Label{{Name: "feature"}}},
					{Number: 11, Title: "Lipgloss", Labels: []Label{{Name: "ui"}}},
					{Number: 12, Title: "Config", Labels: []Label{{Name: "feature"}}},
				},
			},
			{
				Title: "Implemented",
				Cards: []Card{
					{Number: 13, Title: "Fix clamp", Labels: []Label{{Name: "bug"}}},
					{Number: 14, Title: "Refactor", Labels: []Label{{Name: "chore"}}},
					{Number: 15, Title: "Go module", Labels: []Label{{Name: "infra"}}},
					{Number: 16, Title: "Scaffold", Labels: []Label{{Name: "feature"}}},
				},
			},
		},
		nextNumber: 17,
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

	var labels []Label
	if label != "" {
		labels = []Label{{Name: label}}
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

// UpdateCard updates an existing card's title, body, and labels in memory.
// Title must be non-empty after trimming whitespace.
func (f *FakeProvider) UpdateCard(_ context.Context, number int, title string, body string, labels []string) (Card, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return Card{}, errors.New("title is required")
	}

	for ci := range f.columns {
		for i := range f.columns[ci].Cards {
			if f.columns[ci].Cards[i].Number == number {
				f.columns[ci].Cards[i].Title = title
				f.columns[ci].Cards[i].Body = body
				cardLabels := make([]Label, len(labels))
				for j, name := range labels {
					cardLabels[j] = Label{Name: name}
				}
				f.columns[ci].Cards[i].Labels = cardLabels
				return f.columns[ci].Cards[i], nil
			}
		}
	}
	return Card{}, fmt.Errorf("card #%d not found", number)
}

// CreateLabel is a no-op for the fake provider.
func (f *FakeProvider) CreateLabel(_ context.Context, _ string) error {
	return nil
}

// FetchCollaborators returns a hardcoded list of collaborators for the fake provider.
func (f *FakeProvider) FetchCollaborators(_ context.Context) ([]Assignee, error) {
	return []Assignee{{Login: "alice"}, {Login: "bob"}, {Login: "charlie"}}, nil
}

// SetAssignees updates the assignees of a card in the fake provider.
func (f *FakeProvider) SetAssignees(_ context.Context, number int, logins []string) (Card, error) {
	for ci := range f.columns {
		for i := range f.columns[ci].Cards {
			if f.columns[ci].Cards[i].Number == number {
				assignees := make([]Assignee, len(logins))
				for j, login := range logins {
					assignees[j] = Assignee{Login: login}
				}
				f.columns[ci].Cards[i].Assignees = assignees
				return f.columns[ci].Cards[i], nil
			}
		}
	}
	return Card{}, fmt.Errorf("card #%d not found", number)
}

// GetAuthenticatedUser returns a hardcoded username for the fake provider.
func (f *FakeProvider) GetAuthenticatedUser(_ context.Context) (string, error) {
	return "fake-user", nil
}
