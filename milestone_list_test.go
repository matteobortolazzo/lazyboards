package main

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// Milestones modal (#487): opened with 'i' from Normal Mode, lists every open
// repository milestone (independent of any board card). Production symbols
// referenced below do not exist yet -- this file is the RED phase, and the
// package is expected to fail to compile until Phase 4 implements them.
//
// Type/field assumptions this file locks in (see the plan's Files to Modify
// section, which names these symbols and field shapes explicitly):
//   - milestoneListMode boardMode (new mode constant, next to prListMode)
//   - Milestone struct{ Title, URL string; DueOn *time.Time; OpenIssueCount,
//     ClosedIssueCount int; ProgressPercentage float64 } -- mirrors
//     provider.Milestone field-for-field, the same convention mapLinkedPRs
//     uses for LinkedPR.
//   - milestoneListState{ entries []Milestone; cursor int; loading bool;
//     err string; generation uint64 } -- entries is []Milestone directly,
//     no wrapper type (resolved decision in the plan's Q&A).
//   - milestonesFetchedMsg{ milestones []provider.Milestone; err error;
//     generation uint64 }
//   - Board.milestoneList milestoneListState field.
//   - (*Board).enterMilestoneList(), fetchMilestonesCmd(provider.BoardProvider,
//     uint64) tea.Cmd, (Board).handleMilestonesFetched(milestonesFetchedMsg),
//     (Board).handleMilestoneListModeKey(tea.KeyMsg),
//     (Board).handleMilestoneListWheel(tea.MouseMsg),
//     (Board).viewMilestoneListModal() string, padCell(string, int) string.

// dueDate returns a pointer to a fixed absolute UTC due date, mirroring the
// fake provider's fakeMilestoneDue convention (deterministic, never
// time.Now()-relative).
func dueDate(year int, month time.Month, day int) *time.Time {
	d := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	return &d
}

// openMilestoneList presses 'i' from Normal Mode, entering milestoneListMode
// with the fetch in flight.
func openMilestoneList(t *testing.T, b Board) Board {
	t.Helper()
	return sendKey(t, b, keyMsg("i"))
}

// openMilestoneListWithResult presses 'i' and immediately feeds a successful
// milestonesFetchedMsg carrying the given milestones, landing the modal in
// its loaded state.
func openMilestoneListWithResult(t *testing.T, b Board, milestones []provider.Milestone) Board {
	t.Helper()
	b = openMilestoneList(t, b)
	return sendKey(t, b, milestonesFetchedMsg{generation: b.milestoneList.generation, milestones: milestones})
}

// newMouseEnabledMilestoneBoard builds a loaded board with mouse support
// enabled (mirroring mouse_test.go's newMouseEnabledBoard), needed only by
// the wheel tests since keyboard-driven tests don't require it.
func newMouseEnabledMilestoneBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", true, false, nil, nil, true)
	return loadFromFakeProvider(t, b, p)
}

// --- State + fetch ---

func TestNormalMode_I_OpensMilestoneListModal_EmptyBoard_NoPanic(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)
	m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: nil}})
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Update(i) on a zero-column board panicked: %v", r)
		}
	}()

	m, cmd := b.Update(keyMsg("i"))
	b = m.(Board)

	if b.mode != milestoneListMode {
		t.Fatalf("mode = %d, want milestoneListMode (%d)", b.mode, milestoneListMode)
	}
	if cmd == nil {
		t.Fatal("Update(i) returned nil cmd, want the milestones fetch command")
	}
	if !b.milestoneList.loading {
		t.Error("milestoneList.loading = false, want true (fetch in flight)")
	}

	// The view must not panic either, even though it renders "" for a
	// zero-column board today (the len(Columns)==0 early return in View()
	// precedes every mode-specific branch).
	_ = b.View()
}

func TestNormalMode_I_OpensMilestoneListModal_LoadedBoard(t *testing.T) {
	b := newLoadedTestBoard(t)

	m, cmd := b.Update(keyMsg("i"))
	b = m.(Board)

	if b.mode != milestoneListMode {
		t.Fatalf("mode = %d, want milestoneListMode (%d)", b.mode, milestoneListMode)
	}
	if cmd == nil {
		t.Fatal("Update(i) returned nil cmd, want the milestones fetch command")
	}
	if !b.milestoneList.loading {
		t.Error("milestoneList.loading = false, want true (fetch in flight)")
	}
	if b.milestoneList.entries != nil {
		t.Errorf("milestoneList.entries = %+v, want nil (no board-derived fallback)", b.milestoneList.entries)
	}
	if b.milestoneList.cursor != 0 {
		t.Errorf("milestoneList.cursor = %d, want 0", b.milestoneList.cursor)
	}
	if b.milestoneList.err != "" {
		t.Errorf("milestoneList.err = %q, want empty", b.milestoneList.err)
	}

	msg, ok := cmd().(milestonesFetchedMsg)
	if !ok {
		t.Fatalf("cmd() returned %T, want milestonesFetchedMsg", msg)
	}
	if msg.generation != b.milestoneList.generation {
		t.Errorf("cmd() generation = %d, want %d", msg.generation, b.milestoneList.generation)
	}
	if msg.err != nil {
		t.Errorf("cmd() err = %v, want nil (FakeProvider.ListMilestones succeeds)", msg.err)
	}
	if len(msg.milestones) == 0 {
		t.Error("cmd() milestones = empty, want the FakeProvider's fixture rows")
	}
}

func TestEnterMilestoneList_GenerationIncrementsAndResetsState(t *testing.T) {
	b := newLoadedTestBoard(t)

	b = openMilestoneList(t, b)
	gen1 := b.milestoneList.generation
	if gen1 == 0 {
		t.Fatal("generation after first enterMilestoneList = 0, want a nonzero generation")
	}
	if !b.milestoneList.loading || b.milestoneList.entries != nil || b.milestoneList.cursor != 0 || b.milestoneList.err != "" {
		t.Fatalf("milestoneList after first open = %+v, want loading=true entries=nil cursor=0 err=\"\"", b.milestoneList)
	}

	// Mutate cursor/err to prove reopening resets them, mirroring
	// enterPRList's fresh-state contract.
	b.milestoneList.cursor = 3
	b.milestoneList.err = "stale"

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	b = openMilestoneList(t, b)
	gen2 := b.milestoneList.generation

	if gen2 != gen1+1 {
		t.Errorf("generation after reopen = %d, want %d (gen1+1)", gen2, gen1+1)
	}
	if !b.milestoneList.loading || b.milestoneList.entries != nil || b.milestoneList.cursor != 0 || b.milestoneList.err != "" {
		t.Errorf("milestoneList after reopen = %+v, want loading=true entries=nil cursor=0 err=\"\"", b.milestoneList)
	}
}

func TestHandleMilestonesFetched_LoadingToLoaded(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneList(t, b)

	fixture := []provider.Milestone{
		{Title: "v1.0", URL: "https://github.com/owner/repo/milestone/2", DueOn: dueDate(2024, time.March, 15), OpenIssueCount: 3, ClosedIssueCount: 1, ProgressPercentage: 25.0},
	}
	b = sendKey(t, b, milestonesFetchedMsg{generation: b.milestoneList.generation, milestones: fixture})

	if b.milestoneList.loading {
		t.Error("milestoneList.loading = true after success, want false")
	}
	if b.milestoneList.err != "" {
		t.Errorf("milestoneList.err = %q after success, want empty", b.milestoneList.err)
	}
	if len(b.milestoneList.entries) != 1 {
		t.Fatalf("milestoneList.entries = %d, want 1", len(b.milestoneList.entries))
	}
	if got := b.milestoneList.entries[0].Title; got != "v1.0" {
		t.Errorf("entries[0].Title = %q, want %q", got, "v1.0")
	}
	if got := b.milestoneList.entries[0].ProgressPercentage; got != 25.0 {
		t.Errorf("entries[0].ProgressPercentage = %v, want 25.0", got)
	}
}

func TestHandleMilestonesFetched_LoadingToError_EntriesStayEmpty(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneList(t, b)

	fetchErr := errors.New("boom: raw provider failure detail")
	b = sendKey(t, b, milestonesFetchedMsg{generation: b.milestoneList.generation, err: fetchErr})

	if b.milestoneList.loading {
		t.Error("milestoneList.loading = true after error, want false")
	}
	want := provider.SanitizeError(fetchErr)
	if b.milestoneList.err != want {
		t.Errorf("milestoneList.err = %q, want %q (provider.SanitizeError)", b.milestoneList.err, want)
	}
	if len(b.milestoneList.entries) != 0 {
		t.Errorf("milestoneList.entries = %d, want 0 (no fallback, unlike prListState)", len(b.milestoneList.entries))
	}
}

func TestHandleMilestonesFetched_StaleGeneration_Dropped(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneList(t, b)
	staleGen := b.milestoneList.generation

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	b = openMilestoneList(t, b)

	b = sendKey(t, b, milestonesFetchedMsg{
		generation: staleGen,
		milestones: []provider.Milestone{{Title: "stale result"}},
	})

	if !b.milestoneList.loading {
		t.Error("milestoneList.loading = false after stale result, want current request to remain loading")
	}
	if len(b.milestoneList.entries) != 0 {
		t.Errorf("milestoneList.entries = %d after stale result, want 0 (dropped)", len(b.milestoneList.entries))
	}
	if b.mode != milestoneListMode {
		t.Errorf("mode = %d, want milestoneListMode (%d) (modal stays open)", b.mode, milestoneListMode)
	}
}

func TestHandleMilestonesFetched_WrongMode_Dropped(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneList(t, b)
	gen := b.milestoneList.generation
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	if b.mode != normalMode {
		t.Fatalf("precondition: mode = %d, want normalMode after esc", b.mode)
	}

	b = sendKey(t, b, milestonesFetchedMsg{generation: gen, milestones: []provider.Milestone{{Title: "late arrival"}}})

	if b.mode != normalMode {
		t.Errorf("mode = %d, want normalMode (%d) (result must not reopen the modal)", b.mode, normalMode)
	}
	if len(b.milestoneList.entries) != 0 {
		t.Errorf("milestoneList.entries = %d, want 0 (stale-mode result dropped)", len(b.milestoneList.entries))
	}
}

// --- Sort ---
//
// Sorting happens inside handleMilestonesFetched (no standalone exported sort
// function), so it's exercised through the full Update() flow. Table-driven
// per the plan's Test Strategy: catches nil-due-last and the case-insensitive
// tie-break, neither of which the fake provider's fixture alone exercises
// (only one nil-due row, no case-colliding titles).

func TestHandleMilestonesFetched_SortOrder(t *testing.T) {
	tests := []struct {
		name       string
		in         []provider.Milestone
		wantTitles []string
	}{
		{
			name: "due date ascending",
			in: []provider.Milestone{
				{Title: "B", DueOn: dueDate(2024, time.June, 1)},
				{Title: "A", DueOn: dueDate(2024, time.January, 1)},
				{Title: "C", DueOn: dueDate(2024, time.March, 1)},
			},
			wantTitles: []string{"A", "C", "B"},
		},
		{
			name: "nil due date sorts last",
			in: []provider.Milestone{
				{Title: "NoDate", DueOn: nil},
				{Title: "Dated", DueOn: dueDate(2024, time.January, 1)},
			},
			wantTitles: []string{"Dated", "NoDate"},
		},
		{
			name: "multiple nil-due entries sort last, tie-broken by title",
			in: []provider.Milestone{
				{Title: "Zeta", DueOn: nil},
				{Title: "Alpha", DueOn: dueDate(2024, time.May, 1)},
				{Title: "beta", DueOn: nil},
			},
			wantTitles: []string{"Alpha", "beta", "Zeta"},
		},
		{
			name: "case-insensitive tie-break on identical due date",
			in: []provider.Milestone{
				{Title: "Banana", DueOn: dueDate(2024, time.January, 1)},
				{Title: "apple", DueOn: dueDate(2024, time.January, 1)},
			},
			wantTitles: []string{"apple", "Banana"},
		},
		{
			name: "case-insensitive tie-break when both nil due date",
			in: []provider.Milestone{
				{Title: "Zebra", DueOn: nil},
				{Title: "apple", DueOn: nil},
			},
			wantTitles: []string{"apple", "Zebra"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLoadedTestBoard(t)
			b = openMilestoneListWithResult(t, b, tt.in)

			if len(b.milestoneList.entries) != len(tt.wantTitles) {
				t.Fatalf("entries = %d, want %d", len(b.milestoneList.entries), len(tt.wantTitles))
			}
			for i, want := range tt.wantTitles {
				if got := b.milestoneList.entries[i].Title; got != want {
					t.Errorf("entries[%d].Title = %q, want %q (full order: %v)", i, got, want, tt.wantTitles)
				}
			}
		})
	}
}

// --- Interaction: handleMilestoneListModeKey ---

func TestMilestoneList_Esc_ReturnsToNormalAndRestoresHints(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneList(t, b)

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode after esc = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if hintIndex(b.statusBar.hints, "i") == -1 && hintIndex(b.normalHints, "i") != -1 {
		// normalHints only ever carries a subset of built-ins by design
		// elsewhere in the app (e.g. "o" is deliberately absent from the
		// hint bar); this guard only fires if normalHints does carry "i"
		// but the status bar doesn't -- i.e. an actual restoration bug.
		t.Errorf("statusBar hints = %+v, want normalHints restored: %+v", b.statusBar.hints, b.normalHints)
	}
	if len(b.statusBar.hints) != len(b.normalHints) {
		t.Errorf("statusBar hints = %+v (len %d), want normalHints restored: %+v (len %d)",
			b.statusBar.hints, len(b.statusBar.hints), b.normalHints, len(b.normalHints))
	}
}

func TestMilestoneList_JK_WrapsCursor(t *testing.T) {
	b := newLoadedTestBoard(t)
	fixture := []provider.Milestone{
		{Title: "v0.9", DueOn: dueDate(2024, time.February, 1)},
		{Title: "v1.0", DueOn: dueDate(2024, time.March, 15)},
		{Title: "v1.1", DueOn: dueDate(2024, time.June, 30)},
	}
	b = openMilestoneListWithResult(t, b, fixture)
	lastIndex := len(b.milestoneList.entries) - 1

	// k at the top wraps to the last entry.
	b = sendKey(t, b, keyMsg("k"))
	if b.milestoneList.cursor != lastIndex {
		t.Errorf("cursor after k at top = %d, want %d (wrap to last)", b.milestoneList.cursor, lastIndex)
	}

	// j from the last entry wraps back to the first.
	b = sendKey(t, b, keyMsg("j"))
	if b.milestoneList.cursor != 0 {
		t.Errorf("cursor after j at bottom = %d, want 0 (wrap to first)", b.milestoneList.cursor)
	}

	b = sendKey(t, b, keyMsg("j"))
	if b.milestoneList.cursor != 1 {
		t.Errorf("cursor after j = %d, want 1", b.milestoneList.cursor)
	}
}

func TestMilestoneList_Enter_AppliesFilterAndClosesToNormal(t *testing.T) {
	b := newLoadedTestBoard(t)
	// "Refined" is column index 1: cards #4 and #5 both carry Milestone
	// "v1.0" in the fake provider's fixture (fake.go).
	b.ActiveTab = 1

	fixture := []provider.Milestone{
		{Title: "v1.0", URL: "https://github.com/owner/repo/milestone/2", DueOn: dueDate(2024, time.March, 15), OpenIssueCount: 3, ClosedIssueCount: 1, ProgressPercentage: 25.0},
	}
	b = openMilestoneListWithResult(t, b, fixture)

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if b.activeFilterType != filterByMilestone {
		t.Errorf("activeFilterType = %d, want filterByMilestone (%d)", b.activeFilterType, filterByMilestone)
	}
	if b.activeFilterValue != "v1.0" {
		t.Errorf("activeFilterValue = %q, want %q", b.activeFilterValue, "v1.0")
	}
	if got := b.filteredCardsForColumn(1); got != 2 {
		t.Errorf("filteredCardsForColumn(1) = %d, want 2 (cards #4 and #5)", got)
	}
	if !strings.Contains(b.statusBar.message, "v1.0") {
		t.Errorf("status message = %q, want it to name the milestone %q", b.statusBar.message, "v1.0")
	}
}

// TestMilestoneList_Enter_StatusMessagePreservesTailAfterCollapsingNewlines
// covers handleMilestoneListModeKey's Enter path (mode_handlers.go), which
// composes the "Filtered by milestone: <title>" status message with
// truncateOutput(flattenToSingleLine(sanitizeControlSequences(m.Title)),
// milestoneStatusTitleMaxLen) today. flattenToSingleLine replaces each "\n"
// with a single space (a 1:1 substitution, not a collapse), so a title with
// many consecutive newlines inflates in length before truncation ever runs
// and can lose real trailing content within the fixed
// milestoneStatusTitleMaxLen budget. sanitizeSingleLine's
// strings.Fields-based collapse (many whitespace runes -> exactly one
// space) does not have this inflation problem, so migrating this call site
// must retain the tail character that the old order-of-operations loses.
func TestMilestoneList_Enter_StatusMessagePreservesTailAfterCollapsingNewlines(t *testing.T) {
	b := newLoadedTestBoard(t)
	// "ZZZ" (not "b") avoids colliding with the surrounding "Filtered by
	// milestone: " boilerplate text, which itself contains a "b" (in "by").
	title := "a" + strings.Repeat("\n", 70) + "ZZZ"
	fixture := []provider.Milestone{{Title: title}}
	b = openMilestoneListWithResult(t, b, fixture)

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if !strings.Contains(b.statusBar.message, "ZZZ") {
		t.Errorf("status message = %q, want it to retain the title's trailing %q after collapsing the embedded newlines (old truncate-after-flatten order loses it at milestoneStatusTitleMaxLen)", b.statusBar.message, "ZZZ")
	}
}

func TestMilestoneList_Enter_SafeWithZeroColumns(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)
	m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: nil}})
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	b = openMilestoneListWithResult(t, b, []provider.Milestone{{Title: "v1.0"}})

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Enter on a zero-column board panicked: %v", r)
		}
	}()

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter = %d, want normalMode (%d)", b.mode, normalMode)
	}
}

func TestMilestoneList_Enter_EmptyList_ClosesWithNoFilterAndNoMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneListWithResult(t, b, nil) // successful fetch, zero rows

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter on empty list = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if b.activeFilterType != filterTypeNone {
		t.Errorf("activeFilterType = %d, want filterTypeNone (%d) (no filter applied)", b.activeFilterType, filterTypeNone)
	}
	if b.statusBar.message != "" {
		t.Errorf("statusBar.message = %q, want empty (no status message on empty-list enter)", b.statusBar.message)
	}
}

func TestMilestoneList_Enter_SanitizesControlSequencesInStatusMessage(t *testing.T) {
	b := newLoadedTestBoard(t)
	fixture := []provider.Milestone{{Title: "\x1b[31mRED\x1b[0m\x07", URL: "https://github.com/owner/repo/milestone/9"}}
	b = openMilestoneListWithResult(t, b, fixture)

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if strings.ContainsRune(b.statusBar.message, '\x1b') {
		t.Errorf("status message = %q, want no ESC (0x1b) byte", b.statusBar.message)
	}
	if strings.ContainsRune(b.statusBar.message, '\x07') {
		t.Errorf("status message = %q, want no BEL (0x07) byte", b.statusBar.message)
	}
	if !strings.Contains(b.statusBar.message, "RED") {
		t.Errorf("status message = %q, should still contain visible title text %q", b.statusBar.message, "RED")
	}
}

func TestMilestoneList_Enter_CursorOutOfRange_NoOp(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneListWithResult(t, b, []provider.Milestone{{Title: "v1.0"}})
	b.milestoneList.cursor = 5 // out of range for a 1-entry list

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != normalMode {
		t.Errorf("mode after enter with out-of-range cursor = %d, want normalMode (%d)", b.mode, normalMode)
	}
	if b.activeFilterType != filterTypeNone {
		t.Errorf("activeFilterType = %d, want filterTypeNone (%d) (no filter applied)", b.activeFilterType, filterTypeNone)
	}
}

func TestMilestoneList_Enter_NoMatchingCard_LeavesZeroVisibleCards(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	// "Refined" (index 1) has cards #4-#7; none carries "Ghost Milestone".
	b.ActiveTab = 1
	b = openMilestoneListWithResult(t, b, []provider.Milestone{{Title: "Ghost Milestone", URL: "https://github.com/owner/repo/milestone/99"}})

	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	if got := b.filteredCardsForColumn(1); got != 0 {
		t.Errorf("filteredCardsForColumn(1) = %d, want 0 (no card carries this milestone)", got)
	}
	view := b.View()
	if !strings.Contains(view, "No matching cards") {
		t.Errorf("View() after filtering to a non-matching milestone should contain %q; got:\n%s", "No matching cards", view)
	}
}

func TestMilestoneList_O_OpensSelectedMilestoneURL(t *testing.T) {
	fe := &action.FakeExecutor{}
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)
	b = loadFromFakeProvider(t, b, p)

	fixture := []provider.Milestone{{Title: "v1.0", URL: "https://github.com/owner/repo/milestone/2"}}
	b = openMilestoneListWithResult(t, b, fixture)

	m, cmd := b.Update(keyMsg("o"))
	b = m.(Board)
	execCmds(cmd)

	if b.mode != milestoneListMode {
		t.Errorf("mode after o = %d, want milestoneListMode (%d) (modal stays open)", b.mode, milestoneListMode)
	}
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("OpenURL called %d times, want 1", len(fe.OpenURLCalls))
	}
	if fe.OpenURLCalls[0] != "https://github.com/owner/repo/milestone/2" {
		t.Errorf("OpenURL = %q, want the milestone URL", fe.OpenURLCalls[0])
	}
	if !strings.Contains(b.statusBar.message, "v1.0") {
		t.Errorf("status message = %q, want it to name the milestone %q", b.statusBar.message, "v1.0")
	}
}

func TestMilestoneList_O_SanitizesControlSequencesInStatusMessage(t *testing.T) {
	fe := &action.FakeExecutor{}
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)
	b = loadFromFakeProvider(t, b, p)

	fixture := []provider.Milestone{{Title: "\x1b[31mRED\x1b[0m\x07", URL: "https://github.com/owner/repo/milestone/2"}}
	b = openMilestoneListWithResult(t, b, fixture)

	m, cmd := b.Update(keyMsg("o"))
	b = m.(Board)
	execCmds(cmd)

	if strings.ContainsRune(b.statusBar.message, '\x1b') {
		t.Errorf("status message = %q, want no ESC (0x1b) byte", b.statusBar.message)
	}
	if strings.ContainsRune(b.statusBar.message, '\x07') {
		t.Errorf("status message = %q, want no BEL (0x07) byte", b.statusBar.message)
	}
	if !strings.Contains(b.statusBar.message, "RED") {
		t.Errorf("status message = %q, should still contain visible title text %q", b.statusBar.message, "RED")
	}
}

func TestMilestoneList_O_EmptyURL_WarnsAndModalStaysOpen(t *testing.T) {
	fe := &action.FakeExecutor{}
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)
	b = loadFromFakeProvider(t, b, p)

	fixture := []provider.Milestone{{Title: "v1.0", URL: ""}}
	b = openMilestoneListWithResult(t, b, fixture)

	m, cmd := b.Update(keyMsg("o"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("OpenURL called %d times with an empty URL, want 0", len(fe.OpenURLCalls))
	}
	if b.statusBar.message != "URL not available" {
		t.Errorf("status message = %q, want %q", b.statusBar.message, "URL not available")
	}
	if b.statusBar.level != StatusWarning {
		t.Errorf("status level = %d, want StatusWarning (%d)", b.statusBar.level, StatusWarning)
	}
	if b.mode != milestoneListMode {
		t.Errorf("mode after o with empty URL = %d, want milestoneListMode (%d) (modal stays open)", b.mode, milestoneListMode)
	}
}

func TestMilestoneList_O_ExecutorError_ShowsErrorAndModalStaysOpen(t *testing.T) {
	openErr := errors.New("xdg-open: command not found")
	fe := &action.FakeExecutor{OpenURLErr: openErr}
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)
	b = loadFromFakeProvider(t, b, p)

	fixture := []provider.Milestone{{Title: "v1.0", URL: "https://github.com/owner/repo/milestone/2"}}
	b = openMilestoneListWithResult(t, b, fixture)

	m, cmd := b.Update(keyMsg("o"))
	b = m.(Board)
	execCmds(cmd)

	if !strings.Contains(b.statusBar.message, openErr.Error()) {
		t.Errorf("status message = %q, want it to contain the executor error %q", b.statusBar.message, openErr.Error())
	}
	if b.statusBar.level != StatusError {
		t.Errorf("status level = %d, want StatusError (%d)", b.statusBar.level, StatusError)
	}
	if b.mode != milestoneListMode {
		t.Errorf("mode after o with executor error = %d, want milestoneListMode (%d) (modal stays open)", b.mode, milestoneListMode)
	}
}

func TestMilestoneList_O_EmptyList_NoOp(t *testing.T) {
	fe := &action.FakeExecutor{}
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, 0, "Working", false, false, nil, nil, true)
	b = loadFromFakeProvider(t, b, p)
	b = openMilestoneListWithResult(t, b, nil)

	m, cmd := b.Update(keyMsg("o"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("OpenURL called %d times on an empty list, want 0", len(fe.OpenURLCalls))
	}
	if b.mode != milestoneListMode {
		t.Errorf("mode after o on empty list = %d, want milestoneListMode (%d) (modal stays open)", b.mode, milestoneListMode)
	}
}

// --- Interaction: handleMilestoneListWheel ---

func TestMilestoneList_Wheel_ClampsAtBothEnds(t *testing.T) {
	b := newMouseEnabledMilestoneBoard(t)
	fixture := []provider.Milestone{
		{Title: "v0.9"},
		{Title: "v1.0"},
		{Title: "v1.1"},
	}
	b = openMilestoneListWithResult(t, b, fixture)

	// Wheel up at the top must stay at 0 (clamp, not wrap to the last row).
	b = sendKey(t, b, tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	if b.milestoneList.cursor != 0 {
		t.Errorf("cursor after wheel up at top = %d, want 0 (clamped)", b.milestoneList.cursor)
	}

	// Wheel down moves forward one row at a time.
	b = sendKey(t, b, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	b = sendKey(t, b, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if b.milestoneList.cursor != 2 {
		t.Fatalf("cursor after two wheel-downs = %d, want 2", b.milestoneList.cursor)
	}

	// Wheel down at the bottom must stay at the last index (clamp, not wrap).
	b = sendKey(t, b, tea.MouseMsg{Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress})
	if b.milestoneList.cursor != 2 {
		t.Errorf("cursor after wheel down at bottom = %d, want 2 (clamped)", b.milestoneList.cursor)
	}
}

// --- View: viewMilestoneListModal ---

func TestMilestoneList_View_LoadingState(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneList(t, b)

	view := b.viewMilestoneListModal()
	if !strings.Contains(view, "Loading milestones...") {
		t.Errorf("loading view = %q, want it to contain %q", view, "Loading milestones...")
	}
}

func TestMilestoneList_View_ErrorState_FixedMessageOnly(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneList(t, b)
	rawErr := errors.New("connection reset by peer at 10.0.0.5:443")
	b = sendKey(t, b, milestonesFetchedMsg{generation: b.milestoneList.generation, err: rawErr})

	view := b.viewMilestoneListModal()
	if !strings.Contains(view, "Couldn't load milestones") {
		t.Errorf("error view = %q, want it to contain %q", view, "Couldn't load milestones")
	}
	if strings.Contains(view, "10.0.0.5") {
		t.Errorf("error view = %q, must not leak the raw/sanitized provider error text", view)
	}
}

func TestMilestoneList_View_EmptyLoadedState(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneListWithResult(t, b, nil)

	view := b.viewMilestoneListModal()
	if !strings.Contains(view, "No open milestones") {
		t.Errorf("empty view = %q, want it to contain %q", view, "No open milestones")
	}
}

func TestMilestoneList_View_RowShowsTitleBarPercentCountsAndDueDate(t *testing.T) {
	b := newLoadedTestBoard(t)
	fixture := []provider.Milestone{
		{Title: "v1.0", URL: "https://github.com/owner/repo/milestone/2", DueOn: dueDate(2024, time.March, 15), OpenIssueCount: 3, ClosedIssueCount: 1, ProgressPercentage: 25.0},
	}
	b = openMilestoneListWithResult(t, b, fixture)

	view := b.viewMilestoneListModal()
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "v1.0") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("view missing row for v1.0; got:\n%s", view)
	}

	wantPct := " 25%"
	wantCounts := "1/4" // closed/(closed+open)
	wantDue := "2024-03-15"
	for _, want := range []string{wantPct, wantCounts, wantDue} {
		if !strings.Contains(row, want) {
			t.Errorf("row = %q, missing %q", row, want)
		}
	}

	// Column order: title, bar, percentage, counts, due date.
	titleIdx := strings.Index(row, "v1.0")
	pctIdx := strings.Index(row, wantPct)
	countsIdx := strings.Index(row, wantCounts)
	dueIdx := strings.Index(row, wantDue)
	if titleIdx >= pctIdx || pctIdx >= countsIdx || countsIdx >= dueIdx {
		t.Errorf("column order wrong: title=%d pct=%d counts=%d due=%d, want strictly increasing; row = %q",
			titleIdx, pctIdx, countsIdx, dueIdx, row)
	}

	// 25% of a 12-cell bar: round(3) = 3 filled, 9 unfilled.
	filled := strings.Count(row, "▓")
	unfilled := strings.Count(row, "░")
	if filled != 3 {
		t.Errorf("filled bar cells = %d, want 3", filled)
	}
	if unfilled != 9 {
		t.Errorf("unfilled bar cells = %d, want 9", unfilled)
	}
}

func TestMilestoneList_View_PercentageRoundsHalfAwayFromZeroAt37Point5(t *testing.T) {
	b := newLoadedTestBoard(t)
	// 37.5 is the fake provider's "v1.1" fixture value (fake.go) -- it lands
	// exactly on the round-half boundary. int(math.Round(37.5)) (round-half-
	// away-from-zero) yields 38%; a future regression to fmt's %.0f (round-
	// half-to-even) would silently disagree and produce "38" too by
	// coincidence on the percentage alone, so the bar's filled-cell count
	// (37.5% of 12 cells = 4.5, which also rounds away-from-zero to 5) is
	// the assertion that actually disambiguates the two rounding modes.
	fixture := []provider.Milestone{
		{Title: "v1.1", URL: "https://github.com/owner/repo/milestone/3", DueOn: dueDate(2024, time.June, 30), OpenIssueCount: 2, ClosedIssueCount: 2, ProgressPercentage: 37.5},
	}
	b = openMilestoneListWithResult(t, b, fixture)

	view := b.viewMilestoneListModal()
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "v1.1") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("view missing row for v1.1; got:\n%s", view)
	}

	wantPct := " 38%"
	if !strings.Contains(row, wantPct) {
		t.Errorf("row = %q, want it to contain %q (round-half-away-from-zero of 37.5)", row, wantPct)
	}

	filled := strings.Count(row, "▓")
	unfilled := strings.Count(row, "░")
	if filled != 5 {
		t.Errorf("filled bar cells = %d, want 5 (round-half-away-from-zero of 37.5%% of 12 cells)", filled)
	}
	if unfilled != 7 {
		t.Errorf("unfilled bar cells = %d, want 7", unfilled)
	}
}

func TestMilestoneList_View_ZeroTotalIssues_RendersZeroZeroAndEmptyBar(t *testing.T) {
	b := newLoadedTestBoard(t)
	fixture := []provider.Milestone{
		{Title: "Icebox", URL: "https://github.com/owner/repo/milestone/4", DueOn: dueDate(2024, time.December, 31), OpenIssueCount: 0, ClosedIssueCount: 0, ProgressPercentage: 0.0},
	}
	b = openMilestoneListWithResult(t, b, fixture)

	view := b.viewMilestoneListModal()
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Icebox") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("view missing row for Icebox; got:\n%s", view)
	}
	if !strings.Contains(row, "0/0") {
		t.Errorf("row = %q, want it to contain %q", row, "0/0")
	}
	if !strings.Contains(row, "  0%") {
		t.Errorf("row = %q, want it to contain %q (right-aligned 0%%)", row, "  0%")
	}
	if strings.Count(row, "▓") != 0 {
		t.Errorf("row = %q, want a fully-unfilled bar (0 filled cells)", row)
	}
}

func TestMilestoneList_View_NoDueDate_RendersPlaceholder(t *testing.T) {
	b := newLoadedTestBoard(t)
	fixture := []provider.Milestone{
		{Title: "Backlog", URL: "https://github.com/owner/repo/milestone/5", DueOn: nil, OpenIssueCount: 5, ClosedIssueCount: 0, ProgressPercentage: 0.0},
	}
	b = openMilestoneListWithResult(t, b, fixture)

	view := b.viewMilestoneListModal()
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Backlog") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("view missing row for Backlog; got:\n%s", view)
	}
	if !strings.Contains(row, "no due date") {
		t.Errorf("row = %q, want it to contain %q", row, "no due date")
	}
}

func TestMilestoneList_View_DueDateRendersInUTCNotLocalZone(t *testing.T) {
	b := newLoadedTestBoard(t)
	// 2024-03-15T23:00:00-05:00 is 2024-03-16T04:00:00Z: formatting in the
	// source's local zone instead of UTC would wrongly show "2024-03-15".
	west := time.FixedZone("UTC-5", -5*3600)
	due := time.Date(2024, time.March, 15, 23, 0, 0, 0, west)
	fixture := []provider.Milestone{{Title: "Zoned", DueOn: &due}}
	b = openMilestoneListWithResult(t, b, fixture)

	view := b.viewMilestoneListModal()
	var row string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Zoned") {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("view missing row for Zoned; got:\n%s", view)
	}
	if !strings.Contains(row, "2024-03-16") {
		t.Errorf("row = %q, want the UTC-normalized due date %q", row, "2024-03-16")
	}
	if strings.Contains(row, "2024-03-15") {
		t.Errorf("row = %q, want no trace of the local-zone date %q", row, "2024-03-15")
	}
}

func TestMilestoneList_View_SanitizesControlSequencesInTitle(t *testing.T) {
	b := newLoadedTestBoard(t)
	fixture := []provider.Milestone{{Title: "\x1b[31mRED\x1b[0m", URL: "https://github.com/owner/repo/milestone/9"}}
	b = openMilestoneListWithResult(t, b, fixture)

	view := b.viewMilestoneListModal()
	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("viewMilestoneListModal() = %q, want no ESC (0x1b) byte", view)
	}
	if strings.ContainsRune(view, '\x07') {
		t.Errorf("viewMilestoneListModal() = %q, want no BEL (0x07) byte", view)
	}
	if !strings.Contains(view, "RED") {
		t.Errorf("viewMilestoneListModal() should still contain visible title text %q", "RED")
	}
}

// TestMilestoneList_View_FlattensEmbeddedNewlineInTitle is the #497
// regression guard for the milestone row's title cell: an embedded newline
// in m.Title must render on a single physical row. #500 migrates the
// production call site (view.go's viewMilestoneListModal) off the deleted
// flattenToSingleLine helper onto sanitizeSingleLine -- this test's
// assertions are unchanged (sanitizeSingleLine collapses the embedded
// newline to a space, same observable outcome flattenToSingleLine produced
// here), only the underlying implementation it exercises moves.
func TestMilestoneList_View_FlattensEmbeddedNewlineInTitle(t *testing.T) {
	b := newLoadedTestBoard(t)
	fixture := []provider.Milestone{{Title: "line one\nline two", URL: "https://github.com/owner/repo/milestone/9"}}
	b = openMilestoneListWithResult(t, b, fixture)

	view := b.viewMilestoneListModal()
	lines := strings.Split(view, "\n")

	var rowLines []string
	for _, line := range lines {
		if strings.Contains(line, "line one") || strings.Contains(line, "line two") {
			rowLines = append(rowLines, line)
		}
	}
	if len(rowLines) != 1 {
		t.Fatalf("embedded \\n in title spilled onto %d physical lines, want 1 (flattened onto a single row); view:\n%s", len(rowLines), view)
	}
	if !strings.Contains(rowLines[0], "line one") || !strings.Contains(rowLines[0], "line two") {
		t.Errorf("row = %q, want both halves of the flattened title on the same physical line", rowLines[0])
	}
}

func TestMilestoneList_View_TitleFitsTerminalWidth(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	fixture := []provider.Milestone{{Title: "v1.0"}}
	b = openMilestoneListWithResult(t, b, fixture)

	for _, line := range strings.Split(b.viewMilestoneListModal(), "\n") {
		if w := lipgloss.Width(line); w > b.Width {
			t.Errorf("modal line wider than terminal (%d > %d): %q", w, b.Width, line)
		}
	}
}

// TestMilestoneList_View_ColumnAlignment_ShortLongAndCJKTitlesMatch asserts
// the bar/percentage/counts/due-date columns land at the same rendered
// column for a short title, a long (truncated) title, and a CJK (wide-rune)
// title -- and that each milestone stays on exactly one physical line (its
// counts and due date co-occur on the same line as its title), per
// docs/terminal-rendering.md's ban on len()-based layout math.
func TestMilestoneList_View_ColumnAlignment_ShortLongAndCJKTitlesMatch(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	fixture := []provider.Milestone{
		{Title: "A", DueOn: dueDate(2024, time.January, 1), OpenIssueCount: 1, ClosedIssueCount: 1, ProgressPercentage: 50},
		{Title: strings.Repeat("Long milestone title ", 4), DueOn: dueDate(2024, time.February, 2), OpenIssueCount: 2, ClosedIssueCount: 2, ProgressPercentage: 50},
		{Title: "里程碑测试标题字符宽两倍显示", DueOn: dueDate(2024, time.March, 3), OpenIssueCount: 3, ClosedIssueCount: 3, ProgressPercentage: 50},
	}
	b = openMilestoneListWithResult(t, b, fixture)

	view := b.viewMilestoneListModal()
	dueDates := []string{"2024-01-01", "2024-02-02", "2024-03-03"}
	var rows []string
	for _, want := range dueDates {
		var found string
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, want) {
				found = line
				break
			}
		}
		if found == "" {
			t.Fatalf("view missing a row containing due date %q; got:\n%s", want, view)
		}
		rows = append(rows, found)
	}

	// Every fixture row shares the same counts ("1/2" style would differ per
	// row here since counts differ; use the bar glyph as the shared anchor
	// instead -- all three rows are 50%, so the first '▓'/'░' run starts
	// right after the title column in every row).
	prefixWidths := make([]int, len(rows))
	for i, row := range rows {
		idx := strings.IndexAny(row, "▓░")
		if idx == -1 {
			t.Fatalf("row %q missing progress bar glyphs", row)
		}
		prefixWidths[i] = lipgloss.Width(row[:idx])
	}
	for i := 1; i < len(prefixWidths); i++ {
		if prefixWidths[i] != prefixWidths[0] {
			t.Errorf("bar column start width = %d for row %d, want %d (row 0); rows: %v",
				prefixWidths[i], i, prefixWidths[0], rows)
		}
	}

	for i, row := range rows {
		if w := lipgloss.Width(row); w > b.Width {
			t.Errorf("row %d wider than terminal (%d > %d): %q", i, w, b.Width, row)
		}
	}
}

func TestMilestoneList_View_ScrollIndicatorsAndWindowing(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Height = 12
	fixture := make([]provider.Milestone, 20)
	for i := range fixture {
		fixture[i] = provider.Milestone{Title: "Milestone", DueOn: dueDate(2024, time.January, i+1)}
	}
	b = openMilestoneListWithResult(t, b, fixture)
	b.milestoneList.cursor = len(fixture) - 1

	view := b.viewMilestoneListModal()
	if !strings.Contains(view, "2024-01-20") {
		t.Errorf("view does not contain the selected final row's due date:\n%s", view)
	}
	if strings.Contains(view, "2024-01-01") {
		t.Errorf("view still contains the first row instead of a cursor-relative window:\n%s", view)
	}
	if !strings.Contains(view, "▲") {
		t.Errorf("view does not indicate rows above the visible window:\n%s", view)
	}
	if lipgloss.Height(view) > b.Height {
		t.Errorf("modal height = %d, want at most terminal height %d", lipgloss.Height(view), b.Height)
	}
}

func TestMilestoneList_View_SelectedRowStylingDiffersFromNonSelected(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	b := newLoadedTestBoard(t)
	fixture := []provider.Milestone{
		{Title: "First", DueOn: dueDate(2024, time.January, 1)},
		{Title: "Second", DueOn: dueDate(2024, time.February, 1)},
	}
	b = openMilestoneListWithResult(t, b, fixture)
	if b.milestoneList.cursor != 0 {
		t.Fatalf("test setup: expected cursor 0, got %d", b.milestoneList.cursor)
	}

	view := b.viewMilestoneListModal()
	var titleLine, selectedRow, nonSelectedRow string
	lines := strings.Split(view, "\n")
	if len(lines) > 0 {
		titleLine = lines[0]
	}
	for _, line := range lines {
		if strings.Contains(line, "First") {
			selectedRow = line
		}
		if strings.Contains(line, "Second") {
			nonSelectedRow = line
		}
	}
	if selectedRow == "" || nonSelectedRow == "" {
		t.Fatalf("view missing expected rows; got:\n%s", view)
	}
	assertMutedRowStyle(t, "milestone", titleLine, selectedRow, nonSelectedRow)
}

func TestMilestoneList_View_BarMutedOnNonSelectedRow_GreenOnSelectedComplete(t *testing.T) {
	original := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(original) })

	b := newLoadedTestBoard(t)
	fixture := []provider.Milestone{
		{Title: "Selected complete", DueOn: dueDate(2024, time.January, 1), OpenIssueCount: 0, ClosedIssueCount: 8, ProgressPercentage: 100},
		{Title: "Non-selected complete", DueOn: dueDate(2024, time.February, 1), OpenIssueCount: 0, ClosedIssueCount: 4, ProgressPercentage: 100},
	}
	b = openMilestoneListWithResult(t, b, fixture)
	if b.milestoneList.cursor != 0 {
		t.Fatalf("test setup: expected cursor 0, got %d", b.milestoneList.cursor)
	}

	view := b.viewMilestoneListModal()
	var selectedRow, nonSelectedRow string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, "Selected complete") {
			selectedRow = line
		}
		if strings.Contains(line, "Non-selected complete") {
			nonSelectedRow = line
		}
	}
	if selectedRow == "" || nonSelectedRow == "" {
		t.Fatalf("view missing expected rows; got:\n%s", view)
	}

	greenBar := renderProgressBar(100, 12, false)
	grayBar := renderProgressBar(100, 12, true)
	if greenBar == grayBar {
		t.Fatal("test setup: renderProgressBar(100,12,false) and renderProgressBar(100,12,true) must differ under a forced color profile")
	}
	if !strings.Contains(selectedRow, greenBar) {
		t.Errorf("selected 100%% row missing the non-muted (green) bar; row = %q", selectedRow)
	}
	if !strings.Contains(nonSelectedRow, grayBar) {
		t.Errorf("non-selected 100%% row missing the muted (gray) bar; row = %q", nonSelectedRow)
	}
}

func TestMilestoneList_View_ShowsActionHints(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = openMilestoneListWithResult(t, b, []provider.Milestone{{Title: "v1.0"}})

	view := b.viewMilestoneListModal()
	for _, want := range []string{"Filter board", "Open in browser"} {
		if !strings.Contains(view, want) {
			t.Errorf("view = %q, want it to hint %q", view, want)
		}
	}
}

// --- padCell (pure) ---

func TestPadCell_PadsShorterStringToExactWidth(t *testing.T) {
	got := padCell("abc", 6)
	if w := lipgloss.Width(got); w != 6 {
		t.Fatalf("lipgloss.Width(padCell(%q, 6)) = %d, want 6", "abc", w)
	}
	if !strings.HasPrefix(got, "abc") {
		t.Errorf("padCell(%q, 6) = %q, want it to start with the original text", "abc", got)
	}
}

func TestPadCell_ExactWidthReturnsUnchanged(t *testing.T) {
	got := padCell("abcdef", 6)
	if got != "abcdef" {
		t.Errorf("padCell(%q, 6) = %q, want unchanged", "abcdef", got)
	}
}

func TestPadCell_ClampsLongerAsciiStringToExactWidth(t *testing.T) {
	got := padCell("abcdefgh", 6)
	if w := lipgloss.Width(got); w != 6 {
		t.Errorf("lipgloss.Width(padCell(%q, 6)) = %d, want 6 (hard-clamped)", "abcdefgh", w)
	}
}

func TestPadCell_PadsWideRuneStringToExactWidth(t *testing.T) {
	// "你好" is 2 runes, each 2 cells wide (4 cells total); padded to 6.
	got := padCell("你好", 6)
	if w := lipgloss.Width(got); w != 6 {
		t.Fatalf("lipgloss.Width(padCell(%q, 6)) = %d, want 6", "你好", w)
	}
}

func TestPadCell_ClampsWideRuneStringToExactWidth(t *testing.T) {
	// 6 CJK runes x 2 cells = 12 cells, clamped to 6: a naive truncation
	// could land mid-rune (odd remainder), so this asserts the *exact* cell
	// width regardless of how the implementation resolves that boundary.
	got := padCell("你好你好你好", 6)
	if w := lipgloss.Width(got); w != 6 {
		t.Errorf("lipgloss.Width(padCell(%q, 6)) = %d, want exactly 6 (no over/under-clamp on a wide-rune boundary)", "你好你好你好", w)
	}
}

func TestPadCell_ZeroWidth_ReturnsEmptyNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("padCell(%q, 0) panicked: %v", "abc", r)
		}
	}()
	if got := padCell("abc", 0); got != "" {
		t.Errorf("padCell(%q, 0) = %q, want empty string", "abc", got)
	}
}

func TestPadCell_NegativeWidth_ReturnsEmptyNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("padCell(%q, -3) panicked: %v", "abc", r)
		}
	}()
	if got := padCell("abc", -3); got != "" {
		t.Errorf("padCell(%q, -3) = %q, want empty string", "abc", got)
	}
}
