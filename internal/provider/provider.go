package provider

import (
	"context"
	"time"
)

// LinkedPR represents a pull request linked to a card.
//
// IsDraft/Mergeable/MergeStateStatus/State carry GitHub's raw isDraft/
// mergeable/mergeStateStatus/state GraphQL fields through verbatim
// (Mergeable/MergeStateStatus/State as their enum string values, e.g.
// "MERGEABLE", "CONFLICTING", "BLOCKED", "OPEN"/"CLOSED"/"MERGED"). Deriving
// a human-facing status/glyph/style from these fields is presentation logic
// and lives in view.go (prStatus, prStatusSymbol, prStatusStyle), not in
// this package.
type LinkedPR struct {
	Number           int
	Title            string
	URL              string
	Branch           string
	IsDraft          bool
	Mergeable        string
	MergeStateStatus string
	State            string
}

// Label represents a card label with an optional hex color from the provider.
type Label struct {
	Name  string
	Color string
}

// Assignee represents a user assigned to a card.
type Assignee struct {
	Login string
}

// Card represents a single Kanban card (e.g., a GitHub issue).
//
// ParentNumber/SubIssueCount/SubIssueCompleted carry GitHub's native
// sub-issue relationship (#460, #475): ParentNumber is the issue number of
// this card's parent (0 if it has none), SubIssueCount is the number of
// sub-issues this card has (0 if it has none), and SubIssueCompleted is how
// many of those sub-issues are closed (0 if it has none). All are
// read-only, additive fields -- lazyboards never creates/links/unlinks/
// reparents sub-issues, it only displays the relationship GitHub already
// has. The Azure DevOps provider has no equivalent concept, so its cards
// leave all three fields at the zero-value "none" sentinel.
type Card struct {
	Number            int
	Title             string
	Labels            []Label
	Body              string
	URL               string
	LinkedPRs         []LinkedPR
	Assignees         []Assignee
	Milestone         string
	CreatedAt         time.Time
	ParentNumber      int
	SubIssueCount     int
	SubIssueCompleted int
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
	// ListOpenPRs returns every open pull request in the repository,
	// regardless of whether any card links to it. Rows reuse the LinkedPR
	// shape (number/title/URL/branch); "linked" in the type name refers to
	// its original card-scoped use, not a constraint on this list.
	ListOpenPRs(ctx context.Context) ([]LinkedPR, error)
	CreateCard(ctx context.Context, title string, label string) (Card, error)
	UpdateCard(ctx context.Context, number int, title string, body string, labels []string) (Card, error)
	CreateLabel(ctx context.Context, name string) error
	ListLabels(ctx context.Context) ([]string, error)
	FetchCollaborators(ctx context.Context) ([]Assignee, error)
	SetAssignees(ctx context.Context, number int, logins []string) (Card, error)
	GetAuthenticatedUser(ctx context.Context) (string, error)
	CloseCard(ctx context.Context, number int) (Card, error)
	AddComment(ctx context.Context, number int, body string) error
	DeleteCard(ctx context.Context, number int) error
}
