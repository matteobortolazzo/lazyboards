package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// testColumn pairs a column name with the per-column custom actions a
// keymaps.columns.<name> table would declare. Column actions live only in
// the keymaps: namespace now, so this is the test-side fixture shape that
// used to be config.ColumnConfig's (deleted) Actions field.
type testColumn struct {
	Name    string
	Actions map[string]config.Action
}

// columnConfigs projects the name half of cols onto the []config.ColumnConfig
// NewBoard still takes (names, and — for cleanup fixtures — cleanup).
func columnConfigsOf(cols []testColumn) []config.ColumnConfig {
	if cols == nil {
		return nil
	}
	out := make([]config.ColumnConfig, len(cols))
	for i, c := range cols {
		out[i] = config.ColumnConfig{Name: c.Name}
	}
	return out
}

// keymapsFromActions resolves a *keymap.Keymap from in-memory custom-action
// literals, so the action-oriented builders below keep their convenient
// map[string]config.Action fixtures without every test having to hand-write
// a keymaps: YAML document.
//
// It is the test-only equivalent of what a real `keymaps:` config produces:
// each global action is declared in the normal and detail mode tables (and,
// for a single-uppercase-letter scope: pr action, in the pr_list table too,
// mirroring what keymaps.pr_list bindings do), each column's actions land in
// the matching keymaps.columns.<name> overlay, and the whole thing goes
// through the one production resolution path, config.ResolveKeymap.
//
// Keys are canonical keymap.Sequence strings: a multi-key sequence is
// space-separated ("Z f"), exactly as a user writes it under keymaps:.
func keymapsFromActions(t *testing.T, actions map[string]config.Action, columns []testColumn) *keymap.Keymap {
	t.Helper()

	cfg := &config.Config{Columns: columnConfigsOf(columns)}

	if len(actions) > 0 {
		if cfg.Keymaps == nil {
			cfg.Keymaps = &config.Keymaps{}
		}
		cfg.Keymaps.Modes = make(map[keymap.Mode]config.KeymapTable)
		modes := []keymap.Mode{keymap.ModeNormal, keymap.ModeDetail}
		for key, act := range actions {
			for _, mode := range modes {
				modeTable, ok := cfg.Keymaps.Modes[mode]
				if !ok {
					modeTable = make(config.KeymapTable)
					cfg.Keymaps.Modes[mode] = modeTable
				}
				modeTable[key] = config.KeymapBinding{Kind: keymap.BindingAction, Action: act, Order: act.Order}
			}
			// A scope: pr action bound to a single uppercase letter is
			// also reachable inside the PR list modal.
			if act.Scope == "pr" && len([]rune(key)) == 1 && key[0] >= 'A' && key[0] <= 'Z' {
				prTable, ok := cfg.Keymaps.Modes[keymap.ModePRList]
				if !ok {
					prTable = make(config.KeymapTable)
					cfg.Keymaps.Modes[keymap.ModePRList] = prTable
				}
				prTable[key] = config.KeymapBinding{Kind: keymap.BindingAction, Action: act, Order: act.Order}
			}
		}
	}

	for _, col := range columns {
		if len(col.Actions) == 0 {
			continue
		}
		if cfg.Keymaps == nil {
			cfg.Keymaps = &config.Keymaps{}
		}
		if cfg.Keymaps.Columns == nil {
			cfg.Keymaps.Columns = make(map[string]config.KeymapTable)
		}
		colTable, ok := cfg.Keymaps.Columns[col.Name]
		if !ok {
			colTable = make(config.KeymapTable)
			cfg.Keymaps.Columns[col.Name] = colTable
		}
		for key, act := range col.Actions {
			colTable[key] = config.KeymapBinding{Kind: keymap.BindingAction, Action: act, Order: act.Order}
		}
	}

	km, err := config.ResolveKeymap(cfg)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}
	return km
}

// expectedColumnCount is the number of Kanban columns in the board.
const expectedColumnCount = 3

// expectedColumnTitles are the Kanban column names from the spec.
var expectedColumnTitles = []string{"New", "Refined", "Implementing"}

// newTestBoard creates a Board in loadingMode using NewBoard.
func newTestBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	return NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)
}

// newLoadedTestBoard creates a Board and sends a boardFetchedMsg to transition
// it to normalMode with populated columns (simulating a successful fetch).
func newLoadedTestBoard(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)
	// Simulate the provider returning board data.
	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return updated
}

// keyMsg builds a tea.KeyMsg for a single rune key (e.g., "h", "l", "j", "k", "q").
func keyMsg(key string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
}

// arrowMsg builds a tea.KeyMsg for a special key type.
func arrowMsg(kt tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: kt}
}

// sendKey is a helper that sends a key message through Update and returns the updated Board.
func sendKey(t *testing.T, b Board, msg tea.Msg) Board {
	t.Helper()
	m, _ := b.Update(msg)
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return updated
}

// sendKeys sends a sequence of single-rune keys through Update one at a
// time (via keyMsg/sendKey), returning the Board after the final keypress.
// Exists so #502's two-key "g"-prefixed go-navigation sequences ("g r", "g
// a") don't need a repeated two-line sendKey(sendKey(...)) at every call
// site.
func sendKeys(t *testing.T, b Board, keys ...string) Board {
	t.Helper()
	for _, key := range keys {
		b = sendKey(t, b, keyMsg(key))
	}
	return b
}

// simulateRefresh simulates a background refresh completing by fetching
// default board data from a FakeProvider and sending a boardFetchedMsg.
func simulateRefresh(t *testing.T, b Board) Board {
	t.Helper()
	board, err := provider.NewFakeProvider().FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	return updated
}

// execCmds recursively executes a tea.Cmd, handling tea.BatchMsg.
// Uses a timeout to avoid blocking on tea.Tick commands.
func execCmds(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-ch:
	case <-time.After(100 * time.Millisecond):
		return // Skip blocking commands (e.g., tea.Tick)
	}
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		for _, subCmd := range batchMsg {
			execCmds(subCmd)
		}
	}
}

// ansiEscapeCount returns the number of ANSI escape-sequence introducers
// (ESC, 0x1b) in a rendered line. Used by the #478 muted-row modal tests to
// detect whether a row's own content was styled, independent of the
// modal's surrounding border/Place() padding (which contributes a fixed
// number of escape sequences to every row regardless of that row's
// content) -- comparing against a known-bare reference line (e.g. the
// modal's title, which is never passed through selectedRowStyle) isolates
// exactly the row-content styling under test without hardcoding any
// specific style's raw ANSI bytes.
func ansiEscapeCount(line string) int {
	return strings.Count(line, "\x1b")
}

// assertMutedRowStyle asserts, for a list-like modal, that both its
// selected and non-selected row lines carry more ANSI styling than a
// border-only baseline line (e.g. the modal's title, which is never passed
// through selectedRowStyle) -- the selected row via selectedCardStyle
// (bold-white), the non-selected row via mutedRowStyle (gray), rather than
// rendering as bare unstyled text (#478). kind names the modal in failure
// messages (e.g. "agent", "assign", "filter", "git panel").
func assertMutedRowStyle(t *testing.T, kind, titleLine, selectedLine, nonSelectedLine string) {
	t.Helper()
	baseline := ansiEscapeCount(titleLine)
	if got := ansiEscapeCount(selectedLine); got <= baseline {
		t.Errorf("selected %s row escape count = %d, want > baseline %d (bold-white styling missing)", kind, got, baseline)
	}
	if got := ansiEscapeCount(nonSelectedLine); got <= baseline {
		t.Errorf("non-selected %s row escape count = %d, want > baseline %d (row content must mute to gray, not render as bare unstyled text)", kind, got, baseline)
	}
}

// assertOneRow collects every physical line of view that contains at least
// one of needles, fails unless exactly one such line exists (a single-line
// render must not spill an embedded newline onto a second physical row --
// #500), and returns that one line. It also fails if the matched line
// doesn't carry every needle, since a genuinely single-row render must keep
// all fragments co-located. Mirrors the row-spill pattern established by
// TestMilestoneList_View_FlattensEmbeddedNewlineInTitle (#497).
func assertOneRow(t *testing.T, view string, needles ...string) string {
	t.Helper()
	var matching []string
	for _, line := range strings.Split(view, "\n") {
		for _, needle := range needles {
			if strings.Contains(line, needle) {
				matching = append(matching, line)
				break
			}
		}
	}
	if len(matching) != 1 {
		t.Fatalf("needles %v matched %d physical lines, want exactly 1 (single-row render); view:\n%s", needles, len(matching), view)
	}
	row := matching[0]
	for _, needle := range needles {
		if !strings.Contains(row, needle) {
			t.Fatalf("row = %q, want it to contain needle %q (all needles must co-occur on the single matched row)", row, needle)
		}
	}
	return row
}

// modalRowContent extracts a rendered modal line's own content -- stripping
// renderModal's outer lipgloss.Place centering padding (which pads every
// split line out to the full terminal width, per docs/terminal-rendering.md)
// and the box's own border + Padding(1, 2) decoration -- by slicing between
// the line's two "│" border markers and trimming the padding spaces. This is
// the row's true budget for the row-level clamp assertions in
// pr_list_test.go/agent_list_test.go (#596): a raw split line's
// lipgloss.Width is always the full terminal width (b.Width) regardless of
// the row's own content, so it cannot be compared against the modal's
// modalWidth-4 content budget directly.
func modalRowContent(t *testing.T, line string) string {
	t.Helper()
	first := strings.Index(line, "│")
	last := strings.LastIndex(line, "│")
	if first == -1 || last == -1 || first == last {
		t.Fatalf("line %q missing modal border markers (│); want a rendered modal row", line)
	}
	return strings.TrimSpace(line[first+len("│") : last])
}

// requireColumns fails the test immediately if the board has no columns,
// preventing panics from index-out-of-range on the stub implementation.
func requireColumns(t *testing.T, b Board) {
	t.Helper()
	if len(b.Columns) == 0 {
		t.Fatal("board has 0 columns; cannot test item navigation")
	}
}

// loadFromFakeProvider fetches board data from the FakeProvider,
// sends it through Update, and sets standard test dimensions (120x40).
func loadFromFakeProvider(t *testing.T, b Board, p *provider.FakeProvider) Board {
	t.Helper()
	board, err := p.FetchBoard(context.TODO())
	if err != nil {
		t.Fatalf("FakeProvider.FetchBoard failed: %v", err)
	}
	m, _ := b.Update(boardFetchedMsg{board: board})
	loaded, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	loaded.Width = 120
	loaded.Height = 40
	return loaded
}

// newCreatingTestBoard creates a Board in creatingMode for testing async creation.
func newCreatingTestBoard(t *testing.T) Board {
	t.Helper()
	b := newLoadedTestBoard(t)
	b.mode = creatingMode
	return b
}

// newBoardWithCards creates a Board with a single column containing cardCount
// cards, plus a second column with one card (for tab-switch tests).
// Width is set to 120 and Height to the given height parameter.
func newBoardWithCards(t *testing.T, cardCount, height int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	// Build provider cards.
	providerCards := make([]provider.Card, cardCount)
	for i := range providerCards {
		providerCards[i] = provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf("Card %d", i+1),
			Labels: []provider.Label{{Name: "test"}},
		}
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: providerCards},
			{Title: "Column B", Cards: []provider.Card{
				{Number: 100, Title: "Other card", Labels: []provider.Label{{Name: "test"}}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = height
	return board
}

// newActionTestBoard creates a loaded Board with the given actions and a FakeExecutor.
// It returns the board and the FakeExecutor for assertion.
func newActionTestBoard(t *testing.T, actions map[string]config.Action) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, "Working", false, false, nil, nil, true)
	b = b.withKeymap(keymapsFromActions(t, actions, nil))
	return loadFromFakeProvider(t, b, p), fe
}

// newBoardWithEmptyColumn creates a loaded Board with a single column titled
// "Empty" and no cards, wired with the given actions and a FakeExecutor.
// It returns the board and the FakeExecutor for assertion.
func newBoardWithEmptyColumn(t *testing.T, actions map[string]config.Action) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, "Working", false, false, nil, nil, true)
	b = b.withKeymap(keymapsFromActions(t, actions, nil))

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Empty", Cards: nil},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board, fe
}

// trustingLocal returns a config.Trust that self-trusts the local config
// file at path, hashed via the real config.HashLocalConfig so a test never
// hand-copies a hash literal that could drift from what config.Load itself
// computes. Shared by every test board/config builder below that writes a
// local config file to disk and loads it through the real config.Load
// pipeline (#567): these builders exist to exercise trusted local-file
// behavior (action/keymap scope resolution, custom actions, etc.), not
// untrusted-stripping, which has its own dedicated coverage in
// internal/config/trust_strip_test.go and shipped_config_test.go.
func trustingLocal(t *testing.T, path string) config.Trust {
	t.Helper()
	hash, err := config.HashLocalConfig(path)
	if err != nil {
		t.Fatalf("config.HashLocalConfig(%q) returned unexpected error: %v", path, err)
	}
	return config.Trust{Trusted: []config.TrustEntry{{Hash: hash}}}
}

// mustLoadTestConfig writes yamlContent to a temp local config file and loads
// it via config.Load, failing the test on error. This exercises action scope
// resolution (defaulting/inference) through the real parsing/validation path
// production code uses, rather than constructing config.Action literals
// directly with an already-resolved Scope.
func mustLoadTestConfig(t *testing.T, yamlContent string) config.Config {
	t.Helper()
	dir := t.TempDir()
	localPath := filepath.Join(dir, ".lazyboards.yml")
	if err := os.WriteFile(localPath, []byte(yamlContent), 0600); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")
	cfg, err := config.Load(globalPath, localPath, trustingLocal(t, localPath))
	if err != nil {
		t.Fatalf("config.Load() failed: %v", err)
	}
	return cfg
}

// newBoardWithBody creates a Board with one column containing two cards.
// The first card has body1 as its body text; the second card has body2.
func newBoardWithBody(t *testing.T, body1, body2 string) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "Card One", Labels: []provider.Label{{Name: "bug"}}, Body: body1},
				{Number: 2, Title: "Card Two", Labels: []provider.Label{{Name: "feature"}}, Body: body2},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// newBoardWithLongBody creates a board where the first card has a body with
// lineCount paragraphs (e.g., 50), which exceeds the visible panel area at Height=40,
// enabling scroll testing. Uses \n\n paragraph separators so glamour renders
// each as a distinct paragraph (single \n are soft breaks that glamour may collapse).
func newBoardWithLongBody(t *testing.T, lineCount int) Board {
	t.Helper()
	var lines []string
	for i := 1; i <= lineCount; i++ {
		lines = append(lines, fmt.Sprintf("scroll line %d", i))
	}
	longBody := strings.Join(lines, "\n\n")
	return newBoardWithBody(t, longBody, "Other body")
}

// newBoardWithCustomCard creates a board with a single card using the given title, labels, and body.
func newBoardWithCustomCard(t *testing.T, title string, labels []provider.Label, body string) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: title, Labels: labels, Body: body},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 80
	board.Height = 20
	return board
}

// newBoardWithGeneratedCards creates a Board with a single column containing
// count cards. Each card's title is generated from titleFmt (which must contain
// a %d placeholder for the card number). Width and Height are set to the given values.
func newBoardWithGeneratedCards(t *testing.T, count int, titleFmt string, width, height int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	cards := make([]provider.Card, count)
	for i := range cards {
		cards[i] = provider.Card{
			Number: i + 1,
			Title:  fmt.Sprintf(titleFmt, i+1),
			Labels: []provider.Label{{Name: "test"}},
		}
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = width
	board.Height = height
	return board
}

// newBoardWithInlineCards creates a Board with a single column containing the
// given cards. Width and Height are set to the given values.
func newBoardWithInlineCards(t *testing.T, cards []provider.Card, width, height int) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = width
	board.Height = height
	return board
}

// cardTitlePrefixWidth returns the rune-length of a card's "#N " title
// prefix -- the same indentWidth cardDisplayText/wrapTitle compute for the
// card, used by cardStatusLines tests to align status lines under the title.
func cardTitlePrefixWidth(card Card) int {
	return len([]rune(fmt.Sprintf("#%d ", card.Number)))
}

// newBoardWithInlineCardsAndExecutor is newBoardWithInlineCards with a
// FakeExecutor wired in so tests can assert OpenURL/shell side effects.
func newBoardWithInlineCardsAndExecutor(t *testing.T, cards []provider.Card, fe *action.FakeExecutor) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = 120
	board.Height = 40
	return board
}

// newActionTestBoardWithColumns creates a loaded Board with the given actions
// and custom columns. It returns the board and the FakeExecutor for assertion.
func newActionTestBoardWithColumns(t *testing.T, actions map[string]config.Action, columns []provider.Column) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, "Working", false, false, nil, nil, true)
	b = b.withKeymap(keymapsFromActions(t, actions, nil))

	m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: columns}})
	loaded, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	loaded.Width = 120
	loaded.Height = 40
	return loaded, fe
}

// newColumnActionTestBoard creates a loaded Board with global actions AND
// per-column configs. It returns the board and FakeExecutor for assertion.
func newColumnActionTestBoard(t *testing.T, actions map[string]config.Action, columns []testColumn) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, columnConfigsOf(columns), fe, "matteobortolazzo", "lazyboards", "github", 0, 0, "Working", false, false, nil, nil, true)
	b = b.withKeymap(keymapsFromActions(t, actions, columns))
	return loadFromFakeProvider(t, b, p), fe
}

// newConfigLoadedActionTestBoard writes localYAML to a temp .lazyboards.yml,
// loads it through the real config.Load() (so each binding's Order is
// populated from document position, unlike hand-built
// map[string]config.Action fixtures used elsewhere, which leave Order at its
// zero value), and builds a loaded Board from it. Returns the board and the
// FakeExecutor for assertion.
func newConfigLoadedActionTestBoard(t *testing.T, localYAML string) (Board, *action.FakeExecutor) {
	t.Helper()
	fe := &action.FakeExecutor{}
	return newConfigLoadedBoardWithExecutor(t, localYAML, fe), fe
}

// newKeymapConfigLoadedTestBoard writes localYAML to a temp .lazyboards.yml
// and wires the resulting Board exactly like main.go's real startup sequence
// does: config.Load() -> NewBoard(cfg.Columns) -> config.ResolveKeymap(&cfg)
// -> withKeymap().
func newKeymapConfigLoadedTestBoard(t *testing.T, localYAML string) Board {
	t.Helper()
	return newConfigLoadedBoardWithExecutor(t, localYAML, nil)
}

// newConfigLoadedBoardWithExecutor is the shared body of the two builders
// above: write localYAML, load it through the real config.Load(), build a
// loaded Board, and apply the resolved keymap the way main.go does.
func newConfigLoadedBoardWithExecutor(t *testing.T, localYAML string, executor action.Executor) Board {
	t.Helper()
	cfg := mustLoadTestConfig(t, localYAML)
	p := provider.NewFakeProvider()
	b := configLoadedBoard(t, cfg, p, executor)
	return loadFromFakeProvider(t, b, p)
}

// newConfigLoadedEmptyColumnBoard is newConfigLoadedActionTestBoard's
// empty-column sibling: same real config.Load()/ResolveKeymap wiring, but
// the fetched board holds a single card-less column, so board-scope actions
// can be exercised with nothing for a card-scope action to target.
func newConfigLoadedEmptyColumnBoard(t *testing.T, localYAML string) (Board, *action.FakeExecutor) {
	t.Helper()
	cfg := mustLoadTestConfig(t, localYAML)
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := configLoadedBoard(t, cfg, p, fe)

	m, _ := b.Update(boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Empty", Cards: nil},
		},
	}})
	board, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	board.Width = 120
	board.Height = 40
	return board, fe
}

// configLoadedBoard builds an unloaded Board from an already-loaded cfg the
// way main.go's startup does: NewBoard(cfg.Columns) then
// withKeymap(config.ResolveKeymap(&cfg)).
func configLoadedBoard(t *testing.T, cfg config.Config, p provider.BoardProvider, executor action.Executor) Board {
	t.Helper()
	b := NewBoard(p, nil, nil, cfg.Columns, executor, "matteobortolazzo", "lazyboards", "github", 0, 0, "Working", false, false, nil, nil, true)
	km, err := config.ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}
	return b.withKeymap(km)
}

// boundActionScope returns the resolved scope of the inline action bound to
// seq in b's active normal-mode table, failing the test if no such action
// binding exists. Tests use it to assert what config.Load()'s scope
// inference actually resolved to, now that the resolved keymap -- not a
// decoded actions: map -- is the only place a custom action's scope lives.
func boundActionScope(t *testing.T, b Board, seq string) string {
	t.Helper()
	for _, entry := range b.keys.Entries(keymap.ModeNormal, "") {
		if entry.Sequence != seq {
			continue
		}
		if entry.Binding.Kind != keymap.BindingAction {
			t.Fatalf("binding for %q is kind %v, want an inline action", seq, entry.Binding.Kind)
		}
		return entry.Binding.Action.Scope
	}
	t.Fatalf("no normal-mode binding found for %q", seq)
	return ""
}

// prFixtureColumns returns the shared three-card PR fixture used by
// newBoardWithPRsAndExecutor and newPRActionTestBoard:
// - Card 1: "No PRs", no LinkedPRs
// - Card 2: "One PR", 1 LinkedPR (#10, branch feature/one-pr)
// - Card 3: "Two PRs", 2 LinkedPRs (#20 feature/first-pr, #21 feature/second-pr)
func prFixtureColumns() []provider.Column {
	return []provider.Column{
		{Title: "Column A", Cards: []provider.Card{
			{Number: 1, Title: "No PRs", Labels: []provider.Label{{Name: "bug"}}},
			{Number: 2, Title: "One PR", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
				{Number: 10, Title: "feat: one PR", URL: "https://github.com/owner/repo/pull/10", Branch: "feature/one-pr"},
			}},
			{Number: 3, Title: "Two PRs", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
				{Number: 20, Title: "feat: first PR", URL: "https://github.com/owner/repo/pull/20", Branch: "feature/first-pr"},
				{Number: 21, Title: "feat: second PR", URL: "https://github.com/owner/repo/pull/21", Branch: "feature/second-pr"},
			}},
		}},
	}
}

// newBoardWithPRsAndExecutor creates a Board with the shared prFixtureColumns
// three-card PR fixture. It also returns a FakeExecutor for asserting
// OpenURL/RunShell calls. newBoardWithPRs delegates to this function when the
// executor is not needed.
func newBoardWithPRsAndExecutor(t *testing.T) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{Columns: prFixtureColumns()}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board, fe
}

// newBoardWithWorkingLabel creates a Board with one column containing four cards
// covering all combinations of "Working" label and linked PRs:
// - Card 1: No "Working" label, no PR (baseline — no indicators)
// - Card 2: Has "Working" label, no PR (Working indicator only)
// - Card 3: Has PR, no "Working" label (PR indicator only)
// - Card 4: Has both PR and "Working" label (both indicators)
func newBoardWithWorkingLabel(t *testing.T) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "No indicators", Labels: []provider.Label{{Name: "bug"}}},
				{Number: 2, Title: "Working only", Labels: []provider.Label{{Name: "Working"}}},
				{Number: 3, Title: "PR only", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "feat: some PR", URL: "https://github.com/owner/repo/pull/10"},
				}},
				{Number: 4, Title: "Both indicators", Labels: []provider.Label{{Name: "Working"}, {Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 20, Title: "feat: another PR", URL: "https://github.com/owner/repo/pull/20"},
				}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// newBoardWithPRs creates a Board with one column containing three cards:
// - Card 1: no LinkedPRs
// - Card 2: 1 LinkedPR
// - Card 3: 2 LinkedPRs
func newBoardWithPRs(t *testing.T) Board {
	t.Helper()
	b, _ := newBoardWithPRsAndExecutor(t)
	return b
}

// newPRActionTestBoard creates a loaded Board wired with the given custom
// actions AND the same three-card PR fixture as newBoardWithPRsAndExecutor
// (card 1: 0 LinkedPRs, card 2: 1 LinkedPR, card 3: 2 LinkedPRs), so
// scope: pr dispatch tests can exercise the full 0/1/2+ precedence against
// user-configured actions. Returns the board and a FakeExecutor for
// asserting OpenURL/RunShell calls (#340).
func newPRActionTestBoard(t *testing.T, actions map[string]config.Action) (Board, *action.FakeExecutor) {
	t.Helper()
	return newActionTestBoardWithColumns(t, actions, prFixtureColumns())
}

// newBoardWithCustomWorkingLabel creates a Board with one column containing
// cards with specific labels, and the board's workingLabel set to the given value.
// This tests the configurable working label feature (#113).
// - Card 1: label matches workingLabel (should show spinner)
// - Card 2: label "Working" (only shows spinner if workingLabel == "Working")
// - Card 3: label "bug" (baseline, never shows spinner)
func newBoardWithCustomWorkingLabel(t *testing.T, workingLabel string, cards []provider.Card) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, workingLabel, false, false, nil, nil, true)

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: cards},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// newBoardWithAssignees creates a Board with one column containing one card.
// The card has the given assignee logins. If no logins are provided, the card
// has no assignees.
func newBoardWithAssignees(t *testing.T, assigneeLogins ...string) Board {
	t.Helper()
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, nil, "", "", "", 0, 0, "Working", false, false, nil, nil, true)

	assignees := make([]provider.Assignee, len(assigneeLogins))
	for i, login := range assigneeLogins {
		assignees[i] = provider.Assignee{Login: login}
	}

	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{
					Number:    1,
					Title:     "Test card",
					Labels:    []provider.Label{{Name: "bug"}},
					Body:      "Card body text",
					Assignees: assignees,
				},
			}},
		},
	}}
	m, _ := b.Update(msg)
	board := m.(Board)
	board.Width = 120
	board.Height = 40
	return board
}

// findLineContaining returns the first physical line of view containing
// marker, or fails the test. marker must be a stable substring independent
// of any untrusted title content (e.g. "Close #"), so it locates the right
// line regardless of how the title itself renders.
func findLineContaining(t *testing.T, view, marker string) string {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(line, marker) {
			return line
		}
	}
	t.Fatalf("view has no line containing %q; got:\n%s", marker, view)
	return ""
}
