package provider

import "context"

// LinkedPR represents a pull request linked to a card.
type LinkedPR struct {
	Number int
	Title  string
	URL    string
}

// Card represents a single Kanban card (e.g., a GitHub issue).
type Card struct {
	Number    int
	Title     string
	Labels    []string
	Body      string
	LinkedPRs []LinkedPR
}

// Column represents a Kanban column containing cards.
type Column struct {
	Title string
	Cards []Card
}

// Board holds the columns that make up a Kanban board.
type Board struct {
	Columns []Column
}

// BoardProvider is the interface for fetching and mutating board data.
type BoardProvider interface {
	FetchBoard(ctx context.Context) (Board, error)
	CreateCard(ctx context.Context, title string, label string) (Card, error)
}
