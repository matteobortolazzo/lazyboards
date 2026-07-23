package main

import (
	"errors"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// Tests for the "m" reference-navigation trigger (#464): reads the selected
// card's #N references (parsed by #463's parseCardRefs), enters a pending
// which-key state, and on label selection either jumps to the on-board card
// or opens the constructed same-repo issue URL. Mirrors the structure of
// key_sequence_test.go (the pendingSeq analog this feature deliberately does
// NOT reuse -- reference labels are lowercase, pendingSeq is uppercase-only).

// --- Trigger: entering the pending state ---

func TestReferenceNav_TriggersPendingStateWithHints(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2 for details", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Target", Body: ""},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("m"))

	if len(b.pendingRefs) != 1 {
		t.Fatalf("pendingRefs = %+v, want 1 entry", b.pendingRefs)
	}
	if b.pendingRefs[0].Number != 2 || b.pendingRefs[0].Label != 'a' {
		t.Errorf("pendingRefs[0] = %+v, want {Number: 2, Label: 'a'}", b.pendingRefs[0])
	}

	wantHints := []Hint{
		{Key: "a", Desc: "#2"},
		{Key: "esc", Desc: "cancel"},
	}
	if !reflect.DeepEqual(b.statusBar.hints, wantHints) {
		t.Errorf("hints = %v, want %v", b.statusBar.hints, wantHints)
	}
}

func TestReferenceNav_MultipleReferencesEachGetAHint(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2 and #5", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Target"},
			{Number: 5, Title: "Other target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("m"))

	wantHints := []Hint{
		{Key: "a", Desc: "#2"},
		{Key: "b", Desc: "#5"},
		{Key: "esc", Desc: "cancel"},
	}
	if !reflect.DeepEqual(b.statusBar.hints, wantHints) {
		t.Errorf("hints = %v, want %v", b.statusBar.hints, wantHints)
	}
}

func TestReferenceNav_TriggersFromDetailFocusedMode(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2 for details", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("l")) // focus detail panel
	if !b.detailFocused {
		t.Fatal("expected detailFocused after l")
	}
	b = sendKey(t, b, keyMsg("m"))

	if len(b.pendingRefs) != 1 || b.pendingRefs[0].Number != 2 {
		t.Fatalf("pendingRefs = %+v, want 1 entry for #2", b.pendingRefs)
	}
}

func TestReferenceNav_AlwaysActsOnCurrentlySelectedCardBody(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Second source", Body: "See #3", URL: "https://github.com/owner/repo/issues/2"},
			{Number: 3, Title: "Target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("j")) // cursor now on card 2
	b = sendKey(t, b, keyMsg("m"))

	if len(b.pendingRefs) != 1 || b.pendingRefs[0].Number != 3 {
		t.Fatalf("pendingRefs = %+v, want 1 entry for #3 (card 2's own reference, not card 1's)", b.pendingRefs)
	}
}

func TestReferenceNav_NoReferencesIsNoOpWithMessage(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "Nothing to see here", URL: "https://github.com/owner/repo/issues/1"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("m"))

	if len(b.pendingRefs) != 0 {
		t.Errorf("pendingRefs = %+v, want empty (no references in body)", b.pendingRefs)
	}
	if b.statusBar.message != "No references" {
		t.Errorf("status message = %q, want %q", b.statusBar.message, "No references")
	}
	if b.mode != normalMode {
		t.Errorf("mode = %v, want normalMode (no-op must not change mode)", b.mode)
	}
}

// --- Cancellation: esc ---

func TestReferenceNav_EscCancelsPendingAndRestoresHints(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)
	hintsBefore := b.statusBar.hints

	b = sendKey(t, b, keyMsg("m"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if len(b.pendingRefs) != 0 {
		t.Errorf("pendingRefs = %+v after esc, want empty", b.pendingRefs)
	}
	if !reflect.DeepEqual(b.statusBar.hints, hintsBefore) {
		t.Errorf("hints = %v after esc, want restored to %v", b.statusBar.hints, hintsBefore)
	}
}

func TestReferenceNav_EscFromDetailFocusOnlyCancelsPending(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("m"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if len(b.pendingRefs) != 0 {
		t.Errorf("pendingRefs = %+v after esc, want empty", b.pendingRefs)
	}
	if !b.detailFocused {
		t.Error("esc during a pending reference nav must cancel the pending state, not leave the detail panel")
	}
}

// --- Cancellation: unmatched continuation ---

func TestReferenceNav_UnmatchedContinuationCancelsWithWarningAndDoesNotFallThrough(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)
	cursorBefore := b.Columns[b.ActiveTab].Cursor

	b = sendKey(t, b, keyMsg("m"))
	// Only label "a" is bound; "j" is unmatched and is also the built-in
	// down-cursor key -- it must NOT fall through and move the cursor.
	b = sendKey(t, b, keyMsg("j"))

	if len(b.pendingRefs) != 0 {
		t.Errorf("pendingRefs = %+v after unmatched continuation, want empty", b.pendingRefs)
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != cursorBefore {
		t.Errorf("cursor = %d after unmatched continuation, want %d (must not fall through to built-in navigation)", got, cursorBefore)
	}
	if b.statusBar.message == "" {
		t.Error("expected a status-bar message after unmatched continuation, got none")
	}
}

// --- Jumping: on-board and visible ---

func TestReferenceNav_SelectLabelJumpsToOnBoardVisibleCard(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2 for details", URL: "https://github.com/owner/repo/issues/1"},
		}},
		{Title: "Column B", Cards: []provider.Card{
			{Number: 3, Title: "Filler"},
			{Number: 2, Title: "Target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("m"))
	b = sendKey(t, b, keyMsg("a"))

	if b.ActiveTab != 1 {
		t.Fatalf("ActiveTab = %d, want 1 (Column B, where #2 lives)", b.ActiveTab)
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != 1 {
		t.Errorf("cursor = %d, want 1 (index of #2 within Column B)", got)
	}
	if len(b.pendingRefs) != 0 {
		t.Errorf("pendingRefs = %+v after jump, want empty", b.pendingRefs)
	}
}

func TestReferenceNav_SelectLabelJumpsFromDetailFocused(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2 for details", URL: "https://github.com/owner/repo/issues/1"},
		}},
		{Title: "Column B", Cards: []provider.Card{
			{Number: 2, Title: "Target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("m"))
	b = sendKey(t, b, keyMsg("a"))

	if b.ActiveTab != 1 {
		t.Fatalf("ActiveTab = %d, want 1 (Column B, where #2 lives)", b.ActiveTab)
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != 0 {
		t.Errorf("cursor = %d, want 0 (index of #2 within Column B)", got)
	}
}

// TestReferenceNav_JumpFromDetailFocusedExitsDetailFocus asserts the jump
// resets detailFocused, mirroring the existing precedent for every other
// column-switching action taken from detail focus (number-key navigation and
// tab/shift+tab in handleDetailFocusedKey, update.go ~1184-1237): switching
// to a different card/column always drops back to the card-list focus.
func TestReferenceNav_JumpFromDetailFocusedExitsDetailFocus(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2 for details", URL: "https://github.com/owner/repo/issues/1"},
		}},
		{Title: "Column B", Cards: []provider.Card{
			{Number: 2, Title: "Target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("m"))
	b = sendKey(t, b, keyMsg("a"))

	if b.detailFocused {
		t.Error("expected detailFocused = false after a reference jump, mirroring column-switch precedent")
	}
}

// --- Jumping: on-board but hidden by filter/search ---

func TestReferenceNav_SelectLabelHiddenByFilterClearsFilterThenJumps(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #3", URL: "https://github.com/owner/repo/issues/1", Labels: []provider.Label{{Name: "bug"}}},
			{Number: 2, Title: "Filler", Labels: []provider.Label{{Name: "bug"}}},
			{Number: 3, Title: "Target", Labels: []provider.Label{{Name: "feature"}}},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)
	b.activeFilterType = filterByLabel
	b.activeFilterValue = "bug"

	// Precondition: the filter hides #3 (target) but not #1 (source, which
	// must stay selectable since "m" acts on it).
	visible := b.visibleCards()
	if len(visible) != 2 {
		t.Fatalf("precondition: visibleCards() = %d, want 2 (filter hides #3)", len(visible))
	}

	b = sendKey(t, b, keyMsg("m"))
	b = sendKey(t, b, keyMsg("a"))

	if b.activeFilterType != filterTypeNone {
		t.Errorf("activeFilterType = %v after jump to a filter-hidden card, want filterTypeNone (filter cleared)", b.activeFilterType)
	}
	if b.statusBar.message != "Filter cleared" {
		t.Errorf("status message = %q, want %q", b.statusBar.message, "Filter cleared")
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != 2 {
		t.Errorf("cursor = %d, want 2 (index of #3 in the unfiltered column)", got)
	}
}

func TestReferenceNav_SelectLabelHiddenBySearchClearsSearchThenJumps(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #3", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Filler"},
			{Number: 3, Title: "Target"},
		}},
	}
	b, _ := newActionTestBoardWithColumns(t, nil, columns)
	b.searchQuery = "Source"

	// Precondition: only the source card matches the search.
	visible := b.visibleCards()
	if len(visible) != 1 {
		t.Fatalf("precondition: visibleCards() = %d, want 1 (search hides #2 and #3)", len(visible))
	}

	b = sendKey(t, b, keyMsg("m"))
	b = sendKey(t, b, keyMsg("a"))

	if b.searchQuery != "" {
		t.Errorf("searchQuery = %q after jump to a search-hidden card, want empty (search cleared)", b.searchQuery)
	}
	if got := b.Columns[b.ActiveTab].Cursor; got != 2 {
		t.Errorf("cursor = %d, want 2 (index of #3 in the unfiltered column)", got)
	}
}

// --- Jumping: not on board (opens constructed URL) ---

func TestReferenceNav_SelectLabelNotOnBoardOpensConstructedURL(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #999", URL: "https://github.com/owner/repo/issues/1"},
		}},
	}
	b, fe := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("m"))
	b = sendKey(t, b, keyMsg("a"))

	wantURL := "https://github.com/owner/repo/issues/999"
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL calls = %d, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != wantURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], wantURL)
	}
	if len(b.pendingRefs) != 0 {
		t.Errorf("pendingRefs = %+v after selection, want empty", b.pendingRefs)
	}
}

func TestReferenceNav_NotOnBoardOpenURLErrorSurfacesAsStatusMessage(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #999", URL: "https://github.com/owner/repo/issues/1"},
		}},
	}
	b, fe := newActionTestBoardWithColumns(t, nil, columns)
	fe.OpenURLErr = errors.New("no browser configured")

	b = sendKey(t, b, keyMsg("m"))
	b = sendKey(t, b, keyMsg("a"))

	if b.statusBar.level != StatusError {
		t.Errorf("status level = %v, want StatusError", b.statusBar.level)
	}
	if b.statusBar.message == "" {
		t.Error("expected an error status message after a failed OpenURL, got none")
	}
}

func TestReferenceNav_NotOnBoardNoValidURLSurfacesURLNotAvailable(t *testing.T) {
	tests := []struct {
		name    string
		cardURL string
	}{
		{name: "empty card URL", cardURL: ""},
		{name: "no trailing digit run", cardURL: "https://github.com/owner/repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := []provider.Column{
				{Title: "Column A", Cards: []provider.Card{
					{Number: 1, Title: "Source", Body: "See #999", URL: tt.cardURL},
				}},
			}
			b, fe := newActionTestBoardWithColumns(t, nil, columns)

			b = sendKey(t, b, keyMsg("m"))
			b = sendKey(t, b, keyMsg("a"))

			if len(fe.OpenURLCalls) != 0 {
				t.Errorf("OpenURL calls = %d, want 0 (no valid URL to open)", len(fe.OpenURLCalls))
			}
			if b.statusBar.level != StatusWarning {
				t.Errorf("status level = %v, want StatusWarning", b.statusBar.level)
			}
			if b.statusBar.message != "URL not available" {
				t.Errorf("status message = %q, want %q", b.statusBar.message, "URL not available")
			}
		})
	}
}

// --- Lifecycle cancellation (mirrors pendingSeq's cancellation triggers) ---

func TestReferenceNav_BoardRefreshCancelsPending(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Target"},
		}},
	}
	b, fe := newActionTestBoardWithColumns(t, nil, columns)

	b = sendKey(t, b, keyMsg("m"))
	b = simulateRefresh(t, b)

	if len(b.pendingRefs) != 0 {
		t.Errorf("pendingRefs = %+v after board refresh, want empty", b.pendingRefs)
	}
	// "a" after the refresh must not complete the stale pending selection.
	b = sendKey(t, b, keyMsg("a"))
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls after refresh cancelled the pending state, got %d", len(fe.OpenURLCalls))
	}
}

func TestReferenceNav_AsyncCardRemovalCancelsPending(t *testing.T) {
	tests := []struct {
		name string
		msg  func(Card) tea.Msg
	}{
		{name: "deleted", msg: func(card Card) tea.Msg { return cardDeletedMsg{card: card} }},
		{name: "closed", msg: func(card Card) tea.Msg { return cardClosedMsg{card: card} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := []provider.Column{
				{Title: "Column A", Cards: []provider.Card{
					{Number: 1, Title: "Source", Body: "See #2", URL: "https://github.com/owner/repo/issues/1"},
					{Number: 2, Title: "Target"},
				}},
			}
			b, fe := newActionTestBoardWithColumns(t, nil, columns)
			removedCard := b.selectedCard()

			b = sendKey(t, b, keyMsg("m"))
			b = sendKey(t, b, tt.msg(removedCard))

			if len(b.pendingRefs) != 0 {
				t.Errorf("pendingRefs = %+v after card was %s, want empty", b.pendingRefs, tt.name)
			}

			b = sendKey(t, b, keyMsg("a"))
			if len(fe.OpenURLCalls) != 0 {
				t.Errorf("expected no OpenURL calls after async card removal, got %d", len(fe.OpenURLCalls))
			}
		})
	}
}

// newMouseEnabledRefNavBoard creates a loaded, mouse-enabled Board with the
// given columns, for exercising the mid-pending mouse-driven cursor move
// that cancels pendingRefs (mirrors onCursorMoved's existing pendingSeq
// cancellation, exercised by mouse events since keyboard events are consumed
// by the pending state itself).
func newMouseEnabledRefNavBoard(t *testing.T, columns []provider.Column) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", true, false, nil, nil, true)

	m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: columns}})
	loaded, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	loaded.Width = 120
	loaded.Height = 40
	return loaded
}

func TestReferenceNav_MouseCursorMoveCancelsPending(t *testing.T) {
	columns := []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "Source", Body: "See #2", URL: "https://github.com/owner/repo/issues/1"},
			{Number: 2, Title: "Target"},
		}},
	}
	b := newMouseEnabledRefNavBoard(t, columns)

	b = sendKey(t, b, keyMsg("m"))
	if len(b.pendingRefs) != 1 {
		t.Fatalf("precondition: pendingRefs = %+v, want 1 entry", b.pendingRefs)
	}

	b = sendKey(t, b, tea.MouseMsg{
		X:      10,
		Y:      5,
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})

	if len(b.pendingRefs) != 0 {
		t.Errorf("pendingRefs = %+v after a mouse-driven cursor move, want empty", b.pendingRefs)
	}
}

// --- refIssueURL: pure helper ---

func TestRefIssueURL_ReplacesTrailingNumberWithTargetNumber(t *testing.T) {
	cases := []struct {
		name    string
		cardURL string
		number  int
		wantURL string
	}{
		{
			name:    "issues URL",
			cardURL: "https://github.com/owner/repo/issues/463",
			number:  458,
			wantURL: "https://github.com/owner/repo/issues/458",
		},
		{
			name:    "pull URL",
			cardURL: "https://github.com/owner/repo/pull/12",
			number:  7,
			wantURL: "https://github.com/owner/repo/pull/7",
		},
		{
			name:    "provider-agnostic host",
			cardURL: "https://gitlab.example.com/group/project/issues/100",
			number:  1,
			wantURL: "https://gitlab.example.com/group/project/issues/1",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := refIssueURL(tc.cardURL, tc.number)
			if !ok {
				t.Fatalf("refIssueURL(%q, %d) ok = false, want true", tc.cardURL, tc.number)
			}
			if got != tc.wantURL {
				t.Errorf("refIssueURL(%q, %d) = %q, want %q", tc.cardURL, tc.number, got, tc.wantURL)
			}
		})
	}
}

func TestRefIssueURL_NoValidURLReturnsNotOK(t *testing.T) {
	cases := []struct {
		name    string
		cardURL string
	}{
		{name: "empty card URL", cardURL: ""},
		{name: "no trailing digit run", cardURL: "https://github.com/owner/repo"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := refIssueURL(tc.cardURL, 458)
			if ok {
				t.Errorf("refIssueURL(%q, 458) ok = true, want false", tc.cardURL)
			}
			if got != "" {
				t.Errorf("refIssueURL(%q, 458) url = %q, want empty", tc.cardURL, got)
			}
		})
	}
}
