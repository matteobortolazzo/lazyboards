package main

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// filterSearchCards carries labels and assignees whose values exercise
// narrowing: "bug"/"Build"/"debug" share "bu", "docs" does not.
func filterSearchCards() []provider.Card {
	return []provider.Card{
		{Number: 1, Title: "A", Labels: []provider.Label{{Name: "bug"}, {Name: "Build"}, {Name: "docs"}}, Assignees: []provider.Assignee{{Login: "bud"}}},
		{Number: 2, Title: "B", Labels: []provider.Label{{Name: "debug"}}, Assignees: []provider.Assignee{{Login: "alice"}}},
	}
}

func openFilterSearchBoard(t *testing.T) Board {
	t.Helper()
	b := newBoardWithInlineCards(t, filterSearchCards(), 120, 40)
	return sendKey(t, b, keyMsg("f"))
}

func filterRowValues(b Board) []string {
	var values []string
	for _, item := range b.visibleFilterItems() {
		values = append(values, item.value)
	}
	return values
}

func TestFilterSearch_SlashOpensQueryInputKeepingThePickerOpen(t *testing.T) {
	b := openFilterSearchBoard(t)

	m, cmd := b.Update(keyMsg("/"))
	b = m.(Board)

	if b.mode != filterSearchMode || !b.filterQuery.Focused() {
		t.Errorf("after '/': mode = %d, focused = %v; want filterSearchMode with the query focused", b.mode, b.filterQuery.Focused())
	}
	if cmd == nil {
		t.Error("after '/': cmd = nil, want the textinput's cursor-blink Cmd")
	}
	if view := b.View(); !strings.Contains(view, "Filter") || !strings.Contains(view, "/ ") {
		t.Errorf("View() in filterSearchMode should still show the picker with the query line; got:\n%s", view)
	}
}

func TestFilterSearch_TypingNarrowsCaseInsensitively(t *testing.T) {
	b := typeFilterQuery(t, openFilterSearchBoard(t), "BU")

	if got, want := strings.Join(filterRowValues(b), "|"), "bug|Build|debug"; got != want {
		t.Errorf("narrowed rows = %s, want %s", got, want)
	}
	view := b.viewFilterModal()
	if strings.Contains(view, "docs") {
		t.Errorf("view still shows the non-matching row 'docs':\n%s", view)
	}
}

func TestFilterSearch_EveryKeystrokeResetsTheCursor(t *testing.T) {
	b := sendKey(t, openFilterSearchBoard(t), keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.filterCursor == 0 {
		t.Fatal("precondition: cursor should have moved off the first row")
	}

	b = typeFilterQuery(t, b, "b")
	if b.filterCursor != 0 {
		t.Errorf("cursor = %d after typing, want 0", b.filterCursor)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	b = sendKey(t, b, arrowMsg(tea.KeyBackspace))
	if b.filterCursor != 0 {
		t.Errorf("cursor = %d after backspace, want 0", b.filterCursor)
	}
}

func TestFilterSearch_NavigateMovesWithinNarrowedRowsAndWraps(t *testing.T) {
	b := typeFilterQuery(t, openFilterSearchBoard(t), "bu")
	rows := len(b.visibleFilterItems())

	for _, msg := range []tea.KeyMsg{arrowMsg(tea.KeyDown), arrowMsg(tea.KeyCtrlN)} {
		b = sendKey(t, b, msg)
	}
	if b.filterCursor != 2 {
		t.Errorf("cursor after down, ctrl+n = %d, want 2", b.filterCursor)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if b.filterCursor != 0 {
		t.Errorf("cursor after down past %d rows = %d, want 0 (wrap)", rows, b.filterCursor)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyCtrlP))
	if b.filterCursor != rows-1 {
		t.Errorf("cursor after ctrl+p from the first row = %d, want %d (wrap)", b.filterCursor, rows-1)
	}
	if b.mode != filterSearchMode {
		t.Errorf("mode = %d after navigating, want filterSearchMode", b.mode)
	}
}

func TestFilterSearch_EnterKeepsQueryAndToggleHitsTheNarrowedRow(t *testing.T) {
	b := typeFilterQuery(t, openFilterSearchBoard(t), "bu")
	b = sendKey(t, b, arrowMsg(tea.KeyDown)) // second narrowed row: "Build"
	want := currentFilterRow(t, b).value

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.mode != filterMode || b.filterQuery.Value() != "bu" || b.filterQuery.Focused() {
		t.Fatalf("after enter: mode = %d, query = %q, focused = %v; want filterMode keeping the blurred query", b.mode, b.filterQuery.Value(), b.filterQuery.Focused())
	}
	if !strings.Contains(b.viewFilterModal(), "bu") {
		t.Errorf("the applied query should stay visible in the picker")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if !hasFilter(&b, filterByLabel, want) || filterCount(&b) != 1 {
		t.Errorf("filters = %+v, want only the narrowed row %q toggled", b.filters, want)
	}
}

func TestFilterSearch_EscClearsQueryAndReturnsToPicker(t *testing.T) {
	b := typeFilterQuery(t, openFilterSearchBoard(t), "bu")
	all := len(b.filterGroups[b.filterTab].items)

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != filterMode || b.filterQuery.Value() != "" || len(b.visibleFilterItems()) != all {
		t.Errorf("after esc: mode = %d, query = %q, rows = %d; want filterMode, empty query, all %d rows", b.mode, b.filterQuery.Value(), len(b.visibleFilterItems()), all)
	}
	view := b.viewFilterModal()
	for _, line := range strings.Split(view, "\n") {
		content := strings.TrimSpace(strings.Trim(strings.TrimSpace(line), "│"))
		if strings.HasPrefix(content, b.filterQuery.Prompt) {
			t.Errorf("the query line should disappear once the query is cleared; found %q in:\n%s", line, view)
		}
	}
	if got, want := lipgloss.Height(view), lipgloss.Height(openFilterSearchBoard(t).viewFilterModal()); got != want {
		t.Errorf("modal height after esc = %d, want %d (the picker's height before any query)", got, want)
	}
}

func TestFilterSearch_CursorKeysThatKeepTheTextKeepTheCursor(t *testing.T) {
	b := typeFilterQuery(t, openFilterSearchBoard(t), "bu")
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if b.filterCursor != 1 {
		t.Fatalf("precondition: cursor = %d after down, want 1", b.filterCursor)
	}

	for _, kt := range []tea.KeyType{tea.KeyLeft, tea.KeyRight, tea.KeyHome, tea.KeyEnd} {
		b = sendKey(t, b, arrowMsg(kt))
		if b.filterCursor != 1 || b.filterQuery.Value() != "bu" {
			t.Errorf("after %v: cursor = %d, query = %q; want cursor 1 and the query unchanged", kt, b.filterCursor, b.filterQuery.Value())
		}
	}

	b = sendKey(t, b, keyMsg("g"))
	if b.filterCursor != 0 {
		t.Errorf("cursor = %d after typing a rune, want 0", b.filterCursor)
	}
}

// longLabelsBoard builds a board whose labels share a prefix wider than the
// filter modal, so a query typed as that prefix still matches every row.
func longLabelsBoard(t *testing.T, prefix string, count, height int) Board {
	t.Helper()
	cards := make([]provider.Card, count)
	for i := range cards {
		cards[i] = provider.Card{Number: i + 1, Title: "Card", Labels: []provider.Label{{Name: fmt.Sprintf("%s-%02d", prefix, i)}}}
	}
	return newBoardWithInlineCards(t, cards, 120, height)
}

func TestFilterSearch_LongQueryKeepsTheModalWithinTerminalHeight(t *testing.T) {
	const height = 20
	// Wider than the modal's content area, yet within the input's CharLimit.
	prefix := strings.Repeat("x", modalContentWidth(filterModalWidth)+20)
	b := longLabelsBoard(t, prefix, 40, height)
	b = sendKey(t, b, keyMsg("f"))
	b = typeFilterQuery(t, b, prefix)
	if b.filterQuery.Value() != prefix || len(b.visibleFilterItems()) == 0 {
		t.Fatal("precondition: the whole long query should be typed and still match rows")
	}

	assertHeight := func(when string) {
		t.Helper()
		if h := lipgloss.Height(b.viewFilterModal()); h > height {
			t.Errorf("modal height %s = %d, want <= %d:\n%s", when, h, height, b.viewFilterModal())
		}
	}
	assertHeight("while typing, cursor on the first row")
	// Mid-list both scroll indicators render, using every row line.
	for i := 0; i < len(b.visibleFilterItems())/2; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyDown))
	}
	assertHeight("while typing, cursor mid-list")

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.mode != filterMode || b.filterQuery.Value() != prefix {
		t.Fatalf("after enter: mode = %d, query kept = %v; want filterMode keeping the query", b.mode, b.filterQuery.Value() == prefix)
	}
	assertHeight("after enter, cursor mid-list")
}

func TestFilterSearch_RefreshClampsTheCursorWithinNarrowedRows(t *testing.T) {
	b := typeFilterQuery(t, openFilterSearchBoard(t), "bu")
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	b = sendKey(t, b, arrowMsg(tea.KeyDown))
	if b.filterCursor != 2 {
		t.Fatalf("precondition: cursor = %d, want 2", b.filterCursor)
	}

	b = simulateRefreshWithCards(t, b, []provider.Column{{Title: "Column A", Cards: []provider.Card{
		{Number: 1, Title: "A", Labels: []provider.Label{{Name: "bug"}, {Name: "docs"}}},
	}}})

	rows := len(b.visibleFilterItems())
	if rows == 0 || b.filterCursor >= rows {
		t.Errorf("after refresh: cursor = %d over %d narrowed rows, want it clamped within them", b.filterCursor, rows)
	}
	if b.mode != filterSearchMode || b.filterQuery.Value() != "bu" {
		t.Errorf("after refresh: mode = %d, query = %q; want filterSearchMode keeping the query", b.mode, b.filterQuery.Value())
	}
}

func TestFilterSearch_RepoSwitchClearsTheQuery(t *testing.T) {
	b, _, _ := newRepoSwitchBoard(t, singleCardBoard("New", 900, "New repo card"))
	// A config save can't happen with the picker open, so the query is set
	// directly to pin resetRepoScopedState's own reset.
	b.filterQuery.SetValue("bu")

	m, cmd := b.Update(configSavedMsg{provider: "github", repo: "new-owner/new-repo"})
	b = m.(Board)

	if cmd == nil {
		t.Fatal("a repo change should return a fetch cmd")
	}
	if b.filterQuery.Value() != "" {
		t.Errorf("filterQuery = %q after a repo switch, want empty", b.filterQuery.Value())
	}
}

func TestFilterSearch_MouseWheelMovesWithinNarrowedRows(t *testing.T) {
	b := openFilterSearchBoard(t)
	b.mouseEnabled = true
	b = typeFilterQuery(t, b, "bu")
	rows := len(b.visibleFilterItems())

	for i := 0; i < rows+1; i++ {
		m, cmd := b.Update(wheelDownMsg())
		b = m.(Board)
		if cmd != nil {
			t.Errorf("wheel down cmd = %v, want nil", cmd)
		}
	}
	if b.filterCursor != rows-1 {
		t.Errorf("cursor after wheeling past the end = %d, want %d (clamped to the narrowed rows)", b.filterCursor, rows-1)
	}

	m, cmd := b.Update(wheelUpMsg())
	b = m.(Board)
	if cmd != nil {
		t.Errorf("wheel up cmd = %v, want nil", cmd)
	}
	if b.filterCursor != rows-2 {
		t.Errorf("cursor after wheel up = %d, want %d", b.filterCursor, rows-2)
	}
	if b.mode != filterSearchMode || b.filterQuery.Value() != "bu" {
		t.Errorf("after wheeling: mode = %d, query = %q; want filterSearchMode keeping the query", b.mode, b.filterQuery.Value())
	}
}

func TestFilterSearch_QueryPersistsAcrossTabsAndShowsNoMatches(t *testing.T) {
	b := typeFilterQuery(t, openFilterSearchBoard(t), "bu")
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	b = focusFilterTab(t, b, filterByAssignee)
	if got := strings.Join(filterRowValues(b), "|"); got != "bud" {
		t.Errorf("Assignees rows under query 'bu' = %s, want bud", got)
	}

	b = sendKey(t, b, keyMsg("/"))
	b = sendKey(t, b, keyMsg("g"))
	if len(b.visibleFilterItems()) != 0 || !strings.Contains(b.viewFilterModal(), "No matches") {
		t.Errorf("query 'bug' on Assignees: rows = %v, want none and a 'No matches' line", filterRowValues(b))
	}
	if !strings.Contains(b.viewFilterModal(), "Labels") {
		t.Error("tab row should stay visible while a tab has no matches")
	}
}

func TestFilterSearch_ClosingThePickerClearsTheQuery(t *testing.T) {
	b := typeFilterQuery(t, openFilterSearchBoard(t), "bu")
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	if b.mode != normalMode || b.filterQuery.Value() != "" {
		t.Fatalf("after closing: mode = %d, query = %q; want normalMode and an empty query", b.mode, b.filterQuery.Value())
	}

	b = sendKey(t, b, keyMsg("f"))
	if len(b.visibleFilterItems()) != len(b.filterGroups[b.filterTab].items) {
		t.Errorf("reopened picker shows %d of %d rows, want all (no stale query)", len(b.visibleFilterItems()), len(b.filterGroups[b.filterTab].items))
	}
}

func TestFilterSearch_PrintableRunesAreTextNotCommands(t *testing.T) {
	b := openFilterSearchBoard(t)
	setActiveFilter(&b, filterByLabel, "bug")
	tab := b.filterTab

	b = typeFilterQuery(t, b, "jkchl")

	if b.filterQuery.Value() != "jkchl" {
		t.Errorf("query = %q, want the literal runes", b.filterQuery.Value())
	}
	if !hasFilter(&b, filterByLabel, "bug") || b.filterTab != tab || b.mode != filterSearchMode {
		t.Errorf("filter-mode commands fired from typed runes: filters = %+v, tab = %d, mode = %d", b.filters, b.filterTab, b.mode)
	}
}

func TestFilterSearch_RemapAndUnbindOntoNamedKeys(t *testing.T) {
	b := newBoardWithInlineCards(t, filterSearchCards(), 120, 40)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeFilterSearch: {
			"esc":   keymap.UnboundBinding(),
			"enter": keymap.UnboundBinding(),
			"f1":    keymap.CommandBinding(keymap.CommandFilterSearchCancel),
			"f2":    keymap.CommandBinding(keymap.CommandFilterSearchApply),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("f"))
	b = typeFilterQuery(t, b, "bu")

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.mode != filterSearchMode || b.filterQuery.Value() != "bu" {
		t.Fatalf("unbound esc/enter changed state: mode = %d, query = %q", b.mode, b.filterQuery.Value())
	}

	b = sendKey(t, b, arrowMsg(tea.KeyF2))
	if b.mode != filterMode || b.filterQuery.Value() != "bu" {
		t.Errorf("after remapped apply (f2): mode = %d, query = %q; want filterMode keeping the query", b.mode, b.filterQuery.Value())
	}
	b = sendKey(t, b, keyMsg("/"))
	b = sendKey(t, b, arrowMsg(tea.KeyF1))
	if b.mode != filterMode || b.filterQuery.Value() != "" {
		t.Errorf("after remapped cancel (f1): mode = %d, query = %q; want filterMode, empty query", b.mode, b.filterQuery.Value())
	}

	var keys []string
	for _, h := range b.filterSearchHints() {
		keys = append(keys, h.Key+":"+h.Desc)
	}
	if got := strings.Join(keys, " "); !strings.Contains(got, "f2:Apply") || !strings.Contains(got, "f1:Clear") || strings.Contains(got, "esc") || strings.Contains(got, "enter") {
		t.Errorf("filterSearchHints() = %s, want the remapped f1/f2 keys and no unbound esc/enter", got)
	}
}

// Every key the query input's hint bar advertises dispatches through
// textBinding (the real dispatch path) to a command the handler runs.
func TestFilterSearch_HintKeysAlwaysDispatch_DefaultAndRemappedTables(t *testing.T) {
	handled := map[keymap.CommandID]bool{
		keymap.CommandFilterSearchApply: true, keymap.CommandFilterSearchCancel: true,
		keymap.CommandFilterSearchNext: true, keymap.CommandFilterSearchPrev: true,
	}
	tables := []map[keymap.Mode]keymap.Table{
		nil,
		{keymap.ModeFilterSearch: {
			"up":   keymap.UnboundBinding(),
			"down": keymap.UnboundBinding(),
			"f3":   keymap.CommandBinding(keymap.CommandFilterSearchNext),
		}},
	}
	keyFor := map[string]tea.KeyMsg{
		"enter": arrowMsg(tea.KeyEnter), "esc": arrowMsg(tea.KeyEsc), "↑": arrowMsg(tea.KeyUp),
		"↓": arrowMsg(tea.KeyDown), "ctrl+p": arrowMsg(tea.KeyCtrlP), "ctrl+n": arrowMsg(tea.KeyCtrlN),
		"f3": arrowMsg(tea.KeyF3),
	}
	for i, modes := range tables {
		b := newBoardWithInlineCards(t, filterSearchCards(), 120, 40)
		if modes != nil {
			b = boardWithOverrideKeymap(t, b, modes, nil)
		}
		hints := b.filterSearchHints()
		if len(hints) == 0 {
			t.Fatalf("table %d: filterSearchHints() returned no hints", i)
		}
		for _, h := range hints {
			for _, key := range strings.Split(h.Key, "/") {
				msg, ok := keyFor[key]
				if !ok {
					t.Errorf("table %d: hint %+v advertises unexpected key %q", i, h, key)
					continue
				}
				binding, ok := b.textBinding(keymap.ModeFilterSearch, msg)
				if !ok || !handled[binding.Command] {
					t.Errorf("table %d: hint %+v key %q resolves to %+v, want a filter_search command", i, h, key, binding)
				}
			}
		}
	}
}

func TestFilterSearch_CtrlCQuits(t *testing.T) {
	b := typeFilterQuery(t, openFilterSearchBoard(t), "bu")

	m, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.(Board).mode != filterSearchMode {
		t.Errorf("mode changed on ctrl+c, want it left as is")
	}
	if cmd == nil {
		t.Fatal("ctrl+c returned a nil Cmd, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Errorf("ctrl+c Cmd produced %T, want tea.QuitMsg", cmd())
	}
}

func TestFilterSearch_HostileQueryAndValuesRenderSanitized(t *testing.T) {
	b := newBoardWithInlineCards(t, []provider.Card{
		{Number: 1, Title: "A", Labels: []provider.Label{{Name: "evil\x1b[31mRED\nline"}}},
	}, 120, 40)
	b = sendKey(t, b, keyMsg("f"))
	b = sendKey(t, b, keyMsg("/"))
	b = sendKey(t, b, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ev\x07\x1bil"), Paste: true})

	if strings.ContainsRune(b.filterQuery.Value(), '\x1b') {
		t.Errorf("query = %q, want control bytes stripped", b.filterQuery.Value())
	}
	// Tests render under the no-color profile, so any ESC byte in the view
	// would come from the untrusted query or row value.
	view := b.viewFilterModal()
	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("view leaks a control sequence:\n%q", view)
	}
	assertOneRow(t, view, "RED", "line")
}
