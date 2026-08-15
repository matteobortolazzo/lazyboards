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
//
// BlockedByCount/TotalBlockedByCount/BlockingCount/TotalBlockingCount/
// Blockers carry GitHub's native issue-dependency relationship (#628, #630)
// verbatim from issueDependenciesSummary and the bounded blockedBy(first:
// 10) connection: BlockedByCount/BlockingCount are GitHub's open-only
// counts, while TotalBlockedByCount/TotalBlockingCount include closed
// issues too. Blockers is a bounded (first 10) list of the issues blocking
// this one, carried through with no state filtering or dedup -- a consumer
// wanting only open blockers must filter Blockers by State itself. All five
// are read-only, additive fields defaulting to the zero-value "none"
// sentinel for providers without the concept.
type Card struct {
	Number              int
	Title               string
	Labels              []Label
	Body                string
	URL                 string
	LinkedPRs           []LinkedPR
	Assignees           []Assignee
	Milestone           string
	CreatedAt           time.Time
	ParentNumber        int
	SubIssueCount       int
	SubIssueCompleted   int
	BlockedByCount      int
	TotalBlockedByCount int
	BlockingCount       int
	TotalBlockingCount  int
	Blockers            []Blocker
}

// Blocker represents a GitHub issue that blocks a card, as returned by the
// issue's blockedBy connection.
//
// Number/State/URL/RepoNameWithOwner carry GitHub's raw number/state/url/
// repository.nameWithOwner GraphQL fields through verbatim (State as its
// enum string value, e.g. "OPEN"/"CLOSED"). Deriving a human-facing status
// glyph/style, or comparing RepoNameWithOwner against the board's own
// configured repo to detect a cross-repo blocker, is presentation logic and
// belongs in view.go, not in this package.
type Blocker struct {
	Number            int
	State             string
	URL               string
	RepoNameWithOwner string
}

// Milestone represents a repository-wide GitHub milestone, independent of
// any board card.
//
// ProgressPercentage carries GitHub's progressPercentage GraphQL field
// through verbatim (a 0-100 scale, e.g. 40.0, never a 0-1 fraction) --
// per this package's raw-values-verbatim convention (see the LinkedPR doc
// comment above), it is never derived from OpenIssueCount/ClosedIssueCount
// here; any presentation (bar rendering, color) lives in view.go.
// OpenIssueCount/ClosedIssueCount/ProgressPercentage count all items GitHub
// assigns to the milestone, including pull requests, matching GitHub's own
// web UI -- deliberately not reconciled against this app's issue-only board
// view. DueOn is nil when the milestone has no due date set.
type Milestone struct {
	Title              string
	URL                string
	DueOn              *time.Time
	OpenIssueCount     int
	ClosedIssueCount   int
	ProgressPercentage float64
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
	// ListMilestones returns every open milestone in the repository,
	// independent of any board card. Rows carry GitHub's counts/progress
	// verbatim (see the Milestone doc comment above); no cross-page dedup
	// (Milestone carries no numeric key to dedup on).
	ListMilestones(ctx context.Context) ([]Milestone, error)
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
