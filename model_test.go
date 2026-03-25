package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Async Loading: Initial State ---

func TestNewBoard_StartsInLoadingMode(t *testing.T) {
	b := newTestBoard(t)
	if b.mode != loadingMode {
		t.Errorf("mode = %d, want loadingMode", b.mode)
	}
}

func TestNewBoard_InitReturnsCmds(t *testing.T) {
	b := newTestBoard(t)
	cmd := b.Init()
	if cmd == nil {
		t.Error("Init() should return non-nil cmd (batch of spinner tick + fetch)")
	}
}

// --- Async Loading: Fetch Success ---

func TestLoading_FetchSuccess_TransitionsToNormalMode(t *testing.T) {
	b := newTestBoard(t)
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{
			Title: "Col1",
			Cards: []provider.Card{{Number: 1, Title: "Card1", Labels: []provider.Label{{Name: "bug"}}}},
		}},
	}}
	m, _ := b.Update(msg)
	updated := m.(Board)
	if updated.mode != normalMode {
		t.Errorf("mode = %d, want normalMode after successful fetch", updated.mode)
	}
	if len(updated.Columns) == 0 {
		t.Error("Columns should be populated after successful fetch")
	}
}

// --- Async Loading: Fetch Error ---

func TestLoading_FetchError_TransitionsToErrorMode(t *testing.T) {
	b := newTestBoard(t)
	msg := boardFetchErrorMsg{err: errors.New("connection failed")}
	m, _ := b.Update(msg)
	updated := m.(Board)
	if updated.mode != errorMode {
		t.Errorf("mode = %d, want errorMode after fetch error", updated.mode)
	}
}

// --- Error Mode: Retry ---

func TestErrorMode_R_RetriesAndTransitionsToLoadingMode(t *testing.T) {
	b := newTestBoard(t)
	// Put board in errorMode.
	m, _ := b.Update(boardFetchErrorMsg{err: errors.New("fail")})
	b = m.(Board)
	// Press r to retry.
	m, cmd := b.Update(keyMsg("r"))
	b = m.(Board)
	if b.mode != loadingMode {
		t.Errorf("mode = %d, want loadingMode after retry", b.mode)
	}
	if cmd == nil {
		t.Error("retry should return non-nil cmd (spinner tick + fetch)")
	}
}

// --- Error Mode: Quit ---

func TestErrorMode_Q_Quits(t *testing.T) {
	b := newTestBoard(t)
	m, _ := b.Update(boardFetchErrorMsg{err: errors.New("fail")})
	b = m.(Board)
	_, cmd := b.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("'q' in errorMode should return quit cmd")
	}
}

// --- Loading Mode: Key Isolation ---

func TestLoadingMode_IgnoresNavigationKeys(t *testing.T) {
	b := newTestBoard(t)
	origTab := b.ActiveTab
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("k"))
	if b.ActiveTab != origTab {
		t.Error("navigation keys should be ignored in loadingMode")
	}
	if b.mode != loadingMode {
		t.Error("mode should still be loadingMode after navigation keys")
	}
}

// --- Loading Mode: View ---

func TestLoading_ViewShowsLoadingText(t *testing.T) {
	b := newTestBoard(t)
	b.Width = 120
	b.Height = 40
	view := b.View()
	if !strings.Contains(view, "Loading board") {
		t.Error("View() in loadingMode should contain 'Loading board'")
	}
}

// --- Error Mode: View ---

func TestError_ViewShowsErrorAndRetryHint(t *testing.T) {
	b := newTestBoard(t)
	b.Width = 120
	b.Height = 40
	m, _ := b.Update(boardFetchErrorMsg{err: errors.New("connection failed")})
	b = m.(Board)
	view := b.View()
	if !strings.Contains(view, "connection failed") {
		t.Error("View() in errorMode should contain the error message")
	}
	if !strings.Contains(view, "r") {
		t.Error("View() in errorMode should contain retry hint")
	}
}

// --- Loading Mode: Spinner ---

func TestLoading_SpinnerTickPropagated(t *testing.T) {
	b := newTestBoard(t)
	// Send a spinner.TickMsg to the board in loadingMode.
	tickMsg := spinner.TickMsg{}
	m, _ := b.Update(tickMsg)
	updated := m.(Board)
	if updated.mode != loadingMode {
		t.Error("mode should still be loadingMode after spinner tick")
	}
}

// --- Loaded Board: Initial State ---

func TestNewBoard_HasExpectedColumnCount(t *testing.T) {
	b := newLoadedTestBoard(t)
	if got := len(b.Columns); got != expectedColumnCount {
		t.Errorf("loaded board has %d columns, want %d", got, expectedColumnCount)
	}
}

func TestNewBoard_ColumnsHaveCorrectTitles(t *testing.T) {
	b := newLoadedTestBoard(t)
	if len(b.Columns) != len(expectedColumnTitles) {
		t.Fatalf("column count %d != expected title count %d", len(b.Columns), len(expectedColumnTitles))
	}
	for i, want := range expectedColumnTitles {
		if got := b.Columns[i].Title; got != want {
			t.Errorf("column %d title = %q, want %q", i, got, want)
		}
	}
}

func TestNewBoard_ActiveTabStartsAtZero(t *testing.T) {
	b := newLoadedTestBoard(t)
	if b.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d, want 0", b.ActiveTab)
	}
}

func TestNewBoard_EachColumnHasCards(t *testing.T) {
	b := newLoadedTestBoard(t)
	for i, col := range b.Columns {
		if len(col.Cards) == 0 {
			t.Errorf("column %d (%q) has no cards, want at least one", i, col.Title)
		}
	}
}

func TestNewBoard_CardsHaveRequiredFields(t *testing.T) {
	b := newLoadedTestBoard(t)
	for ci, col := range b.Columns {
		for cardIdx, card := range col.Cards {
			if card.Number == 0 {
				t.Errorf("column %d card %d: Number is 0, want a positive issue number", ci, cardIdx)
			}
			if card.Title == "" {
				t.Errorf("column %d card %d: Title is empty", ci, cardIdx)
			}
			if len(card.Labels) == 0 {
				t.Errorf("column %d card %d: Labels is empty, want at least one label", ci, cardIdx)
			}
		}
	}
}

func TestNewBoard_ColumnCursorsStartAtZero(t *testing.T) {
	b := newLoadedTestBoard(t)
	for i, col := range b.Columns {
		if col.Cursor != 0 {
			t.Errorf("column %d cursor = %d, want 0", i, col.Cursor)
		}
	}
}

// --- Error Mode: Status Bar ---

func TestErrorMode_StatusBarShowsRetryAndQuit(t *testing.T) {
	b := newTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Transition to errorMode.
	m, _ := b.Update(boardFetchErrorMsg{err: fmt.Errorf("connection failed")})
	b = m.(Board)

	view := b.View()

	// Should show retry and quit hint keys and descriptions.
	if !strings.Contains(view, "Retry") {
		t.Errorf("View() in errorMode should contain desc %q", "Retry")
	}
	if !strings.Contains(view, "Quit") {
		t.Errorf("View() in errorMode should contain desc %q", "Quit")
	}

	// Should NOT show normalMode hint descriptions.
	if strings.Contains(view, "New") {
		t.Errorf("View() in errorMode should NOT contain %q", "New")
	}
}

// --- LinkedPR Mapping ---

func TestBoardFetched_MapsLinkedPRs(t *testing.T) {
	b := newBoardWithPRs(t)

	if len(b.Columns) == 0 {
		t.Fatal("board has 0 columns after fetch")
	}
	col := b.Columns[0]
	if len(col.Cards) < 3 {
		t.Fatalf("column has %d cards, want at least 3", len(col.Cards))
	}

	// Card 0: no LinkedPRs.
	if got := len(col.Cards[0].LinkedPRs); got != 0 {
		t.Errorf("card 0 LinkedPRs count = %d, want 0", got)
	}

	// Card 1: 1 LinkedPR.
	if got := len(col.Cards[1].LinkedPRs); got != 1 {
		t.Fatalf("card 1 LinkedPRs count = %d, want 1", got)
	}
	pr1 := col.Cards[1].LinkedPRs[0]
	if pr1.Number != 10 {
		t.Errorf("card 1 LinkedPRs[0].Number = %d, want 10", pr1.Number)
	}
	if pr1.Title != "feat: one PR" {
		t.Errorf("card 1 LinkedPRs[0].Title = %q, want %q", pr1.Title, "feat: one PR")
	}
	if pr1.URL != "https://github.com/owner/repo/pull/10" {
		t.Errorf("card 1 LinkedPRs[0].URL = %q, want %q", pr1.URL, "https://github.com/owner/repo/pull/10")
	}

	// Card 2: 2 LinkedPRs.
	if got := len(col.Cards[2].LinkedPRs); got != 2 {
		t.Fatalf("card 2 LinkedPRs count = %d, want 2", got)
	}
	pr2a := col.Cards[2].LinkedPRs[0]
	if pr2a.Number != 20 {
		t.Errorf("card 2 LinkedPRs[0].Number = %d, want 20", pr2a.Number)
	}
	if pr2a.Title != "feat: first PR" {
		t.Errorf("card 2 LinkedPRs[0].Title = %q, want %q", pr2a.Title, "feat: first PR")
	}
	if pr2a.URL != "https://github.com/owner/repo/pull/20" {
		t.Errorf("card 2 LinkedPRs[0].URL = %q, want %q", pr2a.URL, "https://github.com/owner/repo/pull/20")
	}
	pr2b := col.Cards[2].LinkedPRs[1]
	if pr2b.Number != 21 {
		t.Errorf("card 2 LinkedPRs[1].Number = %d, want 21", pr2b.Number)
	}
	if pr2b.Title != "feat: second PR" {
		t.Errorf("card 2 LinkedPRs[1].Title = %q, want %q", pr2b.Title, "feat: second PR")
	}
	if pr2b.URL != "https://github.com/owner/repo/pull/21" {
		t.Errorf("card 2 LinkedPRs[1].URL = %q, want %q", pr2b.URL, "https://github.com/owner/repo/pull/21")
	}
}

// --- Label Color ---

func TestLabelColor_SemanticLabelsGetFixedColors(t *testing.T) {
	// Semantic labels should return their assigned color.
	bugColor := labelColor(Label{Name: "bug"})
	featureColor := labelColor(Label{Name: "feature"})

	if bugColor == featureColor {
		t.Error("bug and feature should have different colors")
	}

	// bug and critical share the same color (rose).
	criticalColor := labelColor(Label{Name: "critical"})
	if bugColor != criticalColor {
		t.Errorf("bug (%s) and critical (%s) should share the same color", bugColor, criticalColor)
	}
}

func TestLabelColor_CaseInsensitive(t *testing.T) {
	lower := labelColor(Label{Name: "bug"})
	upper := labelColor(Label{Name: "BUG"})
	mixed := labelColor(Label{Name: "Bug"})

	if lower != upper || lower != mixed {
		t.Errorf("labelColor should be case-insensitive: bug=%s, BUG=%s, Bug=%s", lower, upper, mixed)
	}
}

func TestLabelColor_Deterministic(t *testing.T) {
	// Same label always produces the same color.
	c1 := labelColor(Label{Name: "custom-label"})
	c2 := labelColor(Label{Name: "custom-label"})
	if c1 != c2 {
		t.Errorf("labelColor should be deterministic: got %s then %s for same input", c1, c2)
	}
}

func TestLabelColor_UnknownLabelsUseValidPaletteColor(t *testing.T) {
	color := labelColor(Label{Name: "some-unknown-label-xyz"})

	found := false
	for _, pc := range labelPalette {
		if lipgloss.Color(color) == pc {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("unknown label color %s should be from labelPalette", color)
	}
}

func TestLabelColor_UsesGitHubColor_WhenPresent(t *testing.T) {
	// When a label has a GitHub hex color, labelColor should use it
	// (prepended with "#") instead of the semantic map or FNV hash.
	color := labelColor(Label{Name: "bug", Color: "d73a4a"})
	expected := lipgloss.Color("#d73a4a")
	if color != expected {
		t.Errorf("labelColor with GitHub color = %s, want %s", color, expected)
	}
}

func TestLabelColor_FallsBackToSemanticMap_WhenNoColor(t *testing.T) {
	// When a label has no GitHub color but matches the semantic map,
	// it should use the semantic color.
	withColor := labelColor(Label{Name: "bug"})
	withoutColor := labelColor(Label{Name: "bug", Color: ""})
	if withColor != withoutColor {
		t.Errorf("labelColor should be the same with empty Color: got %s vs %s", withColor, withoutColor)
	}
	// Should still be the semantic color, not a palette color.
	if _, ok := semanticLabelColors[strings.ToLower("bug")]; !ok {
		t.Fatal("precondition: 'bug' should be in semanticLabelColors")
	}
}

func TestLabelColor_FallsBackToFNVHash_WhenNoColorAndNotSemantic(t *testing.T) {
	// When a label has no GitHub color and is not in the semantic map,
	// it should use the FNV hash palette.
	color := labelColor(Label{Name: "custom-xyz-label"})
	found := false
	for _, pc := range labelPalette {
		if lipgloss.Color(color) == pc {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("labelColor for non-semantic label without GitHub color = %s, want a color from labelPalette", color)
	}
}

func TestLabelColor_GitHubColorOverridesSemanticMap(t *testing.T) {
	// Even for a label that matches the semantic map (like "bug"),
	// a GitHub color should take priority.
	semanticColor := labelColor(Label{Name: "bug", Color: ""})
	githubColor := labelColor(Label{Name: "bug", Color: "ff0000"})
	if semanticColor == githubColor {
		t.Error("GitHub color should override semantic color, but they are the same")
	}
	if githubColor != lipgloss.Color("#ff0000") {
		t.Errorf("labelColor with GitHub color = %s, want %s", githubColor, lipgloss.Color("#ff0000"))
	}
}

// --- Collaborator Data in Board ---

func TestBoardFetched_StoresCollaborators(t *testing.T) {
	b := newTestBoard(t)

	collaborators := []provider.Assignee{
		{Login: "alice"},
		{Login: "bob"},
	}
	msg := boardFetchedMsg{
		board: provider.Board{
			Columns: []provider.Column{{
				Title: "Col1",
				Cards: []provider.Card{{Number: 1, Title: "Card1", Labels: []provider.Label{{Name: "bug"}}}},
			}},
		},
		collaborators: collaborators,
	}
	m, _ := b.Update(msg)
	updated := m.(Board)

	if len(updated.collaborators) != 2 {
		t.Fatalf("collaborators count = %d, want 2", len(updated.collaborators))
	}
	if updated.collaborators[0].Login != "alice" {
		t.Errorf("collaborators[0].Login = %q, want %q", updated.collaborators[0].Login, "alice")
	}
	if updated.collaborators[1].Login != "bob" {
		t.Errorf("collaborators[1].Login = %q, want %q", updated.collaborators[1].Login, "bob")
	}
}

func TestBoardFetched_StoresAuthenticatedUser(t *testing.T) {
	b := newTestBoard(t)

	msg := boardFetchedMsg{
		board: provider.Board{
			Columns: []provider.Column{{
				Title: "Col1",
				Cards: []provider.Card{{Number: 1, Title: "Card1", Labels: []provider.Label{{Name: "bug"}}}},
			}},
		},
		authenticatedUser: "testuser",
	}
	m, _ := b.Update(msg)
	updated := m.(Board)

	if updated.authenticatedUser != "testuser" {
		t.Errorf("authenticatedUser = %q, want %q", updated.authenticatedUser, "testuser")
	}
}

func TestBoardFetched_CollaboratorErrorNonFatal(t *testing.T) {
	b := newTestBoard(t)

	// When collaborator fetch fails, the board should still load normally.
	msg := boardFetchedMsg{
		board: provider.Board{
			Columns: []provider.Column{{
				Title: "Col1",
				Cards: []provider.Card{{Number: 1, Title: "Card1", Labels: []provider.Label{{Name: "bug"}}}},
			}},
		},
		collaboratorErr: fmt.Errorf("collaborator API failed"),
	}
	m, _ := b.Update(msg)
	updated := m.(Board)

	// Board should be in normalMode despite collaborator error.
	if updated.mode != normalMode {
		t.Errorf("mode = %d, want normalMode after collaborator error", updated.mode)
	}

	// Columns should be populated (board data was fine).
	if len(updated.Columns) == 0 {
		t.Error("Columns should be populated even when collaborator fetch fails")
	}

	// Collaborator list should be empty (fetch failed).
	if len(updated.collaborators) != 0 {
		t.Errorf("collaborators = %v, want empty after collaborator error", updated.collaborators)
	}
}

func TestBoardRefresh_RefreshesCollaborators(t *testing.T) {
	b := newTestBoard(t)

	// Initial load with collaborators.
	initialCollaborators := []provider.Assignee{{Login: "alice"}}
	msg := boardFetchedMsg{
		board: provider.Board{
			Columns: []provider.Column{{
				Title: "Col1",
				Cards: []provider.Card{{Number: 1, Title: "Card1", Labels: []provider.Label{{Name: "bug"}}}},
			}},
		},
		collaborators: initialCollaborators,
	}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	if len(b.collaborators) != 1 {
		t.Fatalf("precondition: collaborators count = %d, want 1", len(b.collaborators))
	}

	// Start background refresh.
	m, _ = b.Update(keyMsg("r"))
	b = m.(Board)

	// Simulate refresh completing with updated collaborators.
	updatedCollaborators := []provider.Assignee{{Login: "alice"}, {Login: "charlie"}}
	refreshMsg := boardFetchedMsg{
		board: provider.Board{
			Columns: []provider.Column{{
				Title: "Col1",
				Cards: []provider.Card{{Number: 1, Title: "Card1", Labels: []provider.Label{{Name: "bug"}}}},
			}},
		},
		collaborators: updatedCollaborators,
	}
	m, _ = b.Update(refreshMsg)
	b = m.(Board)

	// Collaborators should be updated to the new list.
	if len(b.collaborators) != 2 {
		t.Fatalf("collaborators count after refresh = %d, want 2", len(b.collaborators))
	}
	if b.collaborators[0].Login != "alice" {
		t.Errorf("collaborators[0].Login = %q, want %q", b.collaborators[0].Login, "alice")
	}
	if b.collaborators[1].Login != "charlie" {
		t.Errorf("collaborators[1].Login = %q, want %q", b.collaborators[1].Login, "charlie")
	}
}
