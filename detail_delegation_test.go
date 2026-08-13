package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// Detail-panel command delegation and focus-hint-restore consolidation
// (#588)
//
// runDetailCommand (update.go) currently handles only the ~13 command ids
// bound in detailDefaults (internal/keymap/defaults_board.go) and silently
// falls off the end of its switch for everything else. #588 widens it with a
// `default:` branch that delegates to runNormalCommand, so every command id
// runNormalCommand accepts becomes dispatchable from the detail panel
// without blurring it -- and consolidates the six hand-duplicated
// `if b.detailFocused { rebuildDetailHints() } else { SetActionHints(b.normalHints) }`
// copies into one restoreFocusHints() helper, swapped into every
// unconditional SetActionHints(b.normalHints) exit site a delegated command
// can now reach.

// --- detailDelegationKeymapYAML: shared keymaps.detail override wiring every
// normal-mode command this ticket makes reachable from the detail panel onto
// a key detailDefaults doesn't already claim (see internal/keymap/
// defaults_board.go's detailDefaults table for the keys already spoken for:
// q e r o "g r" p ? h left esc j down k up tab shift+tab 1-9). f2 is bound to
// nav.cursor_down specifically for the reading-pane test (#588 test 4) since
// the detail panel's default j/k are already claimed by detail.scroll_down/
// up, not nav.cursor_down/up.

const detailDelegationKeymapYAML = `keymaps:
  detail:
    f: board.filter
    a: card.assign
    P: view.pr_list
    m: view.milestone_list
    A: view.agent_list
    G: view.git_panel
    D: view.dispatch
    d: card.delete
    /: board.search
    f2: nav.cursor_down
`

// newDetailDelegationTestBoard writes localYAML to a temp .lazyboards.yml and
// wires the resulting Board exactly like main.go's real startup sequence
// (config.Load -> NewBoard -> config.ResolveKeymap -> withKeymap), mirroring
// helpers_test.go's newKeymapConfigLoadedTestBoard -- except defaultActions is
// config.DefaultGitActions() (not nil), so view.git_panel's "is this a git
// repo" availability gate (enterGitPanel, model.go) doesn't silently no-op
// entry in these tests, and collaborators are seeded post-load so
// card.assign's "any collaborators to assign" gate (runNormalCommand,
// mode_handlers.go) doesn't silently no-op entry either.
func newDetailDelegationTestBoard(t *testing.T, localYAML string) Board {
	t.Helper()
	dir := t.TempDir()
	localPath := filepath.Join(dir, "local.yml")
	if err := os.WriteFile(localPath, []byte(localYAML), 0644); err != nil {
		t.Fatalf("failed to write local config: %v", err)
	}
	globalPath := filepath.Join(dir, "nonexistent-global.yml")

	cfg, err := config.Load(globalPath, localPath, trustingLocal(t, localPath))
	if err != nil {
		t.Fatalf("config.Load() returned unexpected error: %v", err)
	}

	p := provider.NewFakeProvider()
	b := NewBoard(p, config.DefaultGitActions(), cfg.Columns, nil, "matteobortolazzo", "lazyboards", "github", 0, 0, "Working", false, false, nil, nil, true)
	b = loadFromFakeProvider(t, b, p)
	b.collaborators = []Assignee{{Login: "alice"}}

	km, err := config.ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}
	return b.withKeymap(km)
}

// --- Test 1: pin the nav.column_* blur-before-switchColumn ordering ---
//
// runDetailCommand's column-jump cases (CommandNavColumnNext/Prev and the
// nav.column_N block via navColumnIndex) set detailFocused = false BEFORE
// calling switchColumn, so switchColumn -> onCursorMoved -> (the future)
// restoreFocusHints() observes detailFocused == false and picks the
// card-list hint bar, not the detail one. These two assertions must PASS
// against today's unmodified code (they exercise ordering that already
// exists, independent of #588's delegation/restoreFocusHints() work) so a
// future accidental reorder is caught immediately by this pin.

func TestDetailDelegation_ColumnNext_BlursBeforeSwitchColumn_OrderingPin(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // nav.column_next

	if b.detailFocused {
		t.Error("detailFocused should be false after nav.column_next from the detail panel (must blur before switchColumn runs)")
	}
}

func TestDetailDelegation_ColumnNumberJump_BlursBeforeSwitchColumn_OrderingPin(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	b = sendKey(t, b, keyMsg("2")) // nav.column_2

	if b.detailFocused {
		t.Error("detailFocused should be false after a nav.column_N jump from the detail panel (must blur before switchColumn runs)")
	}
}

// --- Test 2: delegation parity matrix ---
//
// detailDelegationSnapshot captures the board state a delegated command's
// dispatch should leave byte-identical between runDetailCommand and
// runNormalCommand, deliberately excluding detailFocused (the whole point of
// delegation is that it differs -- runNormalCommand's caller starts un-
// focused and stays that way, runDetailCommand's caller starts focused and
// stays that way, per AC2) and the status bar (mode-entry hints are set
// identically by both paths already; nav.cursor_down/up's hint bar
// deliberately differs by design -- AC4/Test 4 covers that combination on
// its own).
type detailDelegationSnapshot struct {
	mode              boardMode
	activeTab         int
	columnCursor      int
	createFocus       int
	createTitle       string
	createLabel       string
	createAssigneeIdx int
	createPending     string
	deleteStep        deleteStep
	deleteCardNumber  int
	assignCursor      int
	assignItemCount   int
	filterItemCount   int
	filterCursor      int
	activeFilterType  filterType
	activeFilterValue string
	prListCursor      int
	prListGeneration  uint64
	prListEntryCount  int
	milestoneCursor   int
	milestoneGen      uint64
	milestoneLoading  bool
	agentListCursor   int
	agentListCard     int
	dispatchLoading   bool
	gitPanelCursor    int
	gitPanelItemCount int
	searchQuery       string
	sortNewestFirst   bool
	validationErr     string
}

func snapshotDetailDelegationState(b Board) detailDelegationSnapshot {
	colCursor := -1
	if b.ActiveTab < len(b.Columns) {
		colCursor = b.Columns[b.ActiveTab].Cursor
	}
	return detailDelegationSnapshot{
		mode:              b.mode,
		activeTab:         b.ActiveTab,
		columnCursor:      colCursor,
		createFocus:       b.create.focus,
		createTitle:       b.create.titleInput.Value(),
		createLabel:       b.create.labelInput.Value(),
		createAssigneeIdx: b.create.assigneeIndex,
		createPending:     b.create.pendingAssignee,
		deleteStep:        b.delete.step,
		deleteCardNumber:  b.delete.card.Number,
		assignCursor:      b.assign.cursor,
		assignItemCount:   len(b.assign.items),
		filterItemCount:   len(b.filterItems),
		filterCursor:      b.filterCursor,
		activeFilterType:  b.activeFilterType,
		activeFilterValue: b.activeFilterValue,
		prListCursor:      b.prList.cursor,
		prListGeneration:  b.prList.generation,
		prListEntryCount:  len(b.prList.entries),
		milestoneCursor:   b.milestoneList.cursor,
		milestoneGen:      b.milestoneList.generation,
		milestoneLoading:  b.milestoneList.loading,
		agentListCursor:   b.agentList.cursor,
		agentListCard:     b.agentList.cardNumber,
		dispatchLoading:   b.dispatch.loading,
		gitPanelCursor:    b.gitPanel.cursor,
		gitPanelItemCount: len(b.gitPanel.items),
		searchQuery:       b.searchQuery,
		sortNewestFirst:   b.sortNewestFirst,
		validationErr:     b.validationErr,
	}
}

// TestDetailDelegation_ParityWithNormalCommand covers AC7 (the plan's
// parity test): for every id in keymap.Defaults()'s ModeNormal table that
// isn't one of runDetailCommand's ~13 explicitly-cased ids, dispatching it
// through runDetailCommand (from a detail-focused board) must produce the
// same resulting mode/board state as dispatching it through
// runNormalCommand (from a non-focused board), and must leave the
// detail-focused board still focused. Card #1 (the board's default cursor
// position) deliberately has zero LinkedPRs in the FakeProvider fixture, so
// this exercises card.delete's PR-gating happy path (entering deleteMode),
// not its "not supported" guard.
func TestDetailDelegation_ParityWithNormalCommand(t *testing.T) {
	// Explicitly cased in runDetailCommand today (update.go) -- not part of
	// the delegation surface this ticket adds.
	explicitlyCased := map[keymap.CommandID]bool{
		keymap.CommandQuit:             true,
		keymap.CommandCardEdit:         true,
		keymap.CommandBoardRefresh:     true,
		keymap.CommandCardOpenTicket:   true,
		keymap.CommandNavReference:     true,
		keymap.CommandCardOpenPR:       true,
		keymap.CommandHelp:             true,
		keymap.CommandDetailBlur:       true,
		keymap.CommandDetailScrollDown: true,
		keymap.CommandDetailScrollUp:   true,
		keymap.CommandNavColumnNext:    true,
		keymap.CommandNavColumnPrev:    true,
		keymap.CommandNavColumn1:       true,
		keymap.CommandNavColumn2:       true,
		keymap.CommandNavColumn3:       true,
		keymap.CommandNavColumn4:       true,
		keymap.CommandNavColumn5:       true,
		keymap.CommandNavColumn6:       true,
		keymap.CommandNavColumn7:       true,
		keymap.CommandNavColumn8:       true,
		keymap.CommandNavColumn9:       true,
	}

	normalTable := keymap.Defaults().Modes[keymap.ModeNormal]
	if len(normalTable) == 0 {
		t.Fatal("test setup: keymap.Defaults().Modes[keymap.ModeNormal] should be non-empty")
	}

	seen := map[keymap.CommandID]bool{}
	tested := 0
	for _, binding := range normalTable {
		if binding.Kind != keymap.BindingCommand {
			continue
		}
		id := binding.Command
		if explicitlyCased[id] || seen[id] {
			continue
		}
		seen[id] = true
		tested++

		t.Run(string(id), func(t *testing.T) {
			// Two independently-constructed boards, not one shared board
			// copied by value: Board.Columns is a slice, so two value copies
			// of the same board share one backing array -- an in-place
			// mutation through one copy (e.g. nav.cursor_down's
			// `col := &b.Columns[...]; col.Cursor = ...`) would silently
			// leak into the other copy and produce a false-positive parity
			// match that isn't actually exercising delegation.
			normalBoard := newLoadedTestBoard(t)
			normalBoard.Width = 120
			normalBoard.Height = 40
			normalBoard.detailFocused = false
			normalBoard.collaborators = []Assignee{{Login: "alice"}}
			normalBoard.defaultActions = config.DefaultGitActions()
			gotNormal, _ := normalBoard.runNormalCommand(id)
			normalModel, ok := gotNormal.(Board)
			if !ok {
				t.Fatalf("runNormalCommand(%s) returned %T, want Board", id, gotNormal)
			}

			detailBoard := newLoadedTestBoard(t)
			detailBoard.Width = 120
			detailBoard.Height = 40
			detailBoard.detailFocused = true
			detailBoard.collaborators = []Assignee{{Login: "alice"}}
			detailBoard.defaultActions = config.DefaultGitActions()
			gotDetail, _ := detailBoard.runDetailCommand(id)
			detailModel, ok := gotDetail.(Board)
			if !ok {
				t.Fatalf("runDetailCommand(%s) returned %T, want Board", id, gotDetail)
			}

			if !detailModel.detailFocused {
				t.Errorf("runDetailCommand(%s) dropped detailFocused; a delegated command must not blur the detail panel", id)
			}

			wantSnap := snapshotDetailDelegationState(normalModel)
			gotSnap := snapshotDetailDelegationState(detailModel)
			if gotSnap != wantSnap {
				t.Errorf("runDetailCommand(%s) state = %+v, want parity with runNormalCommand state %+v", id, gotSnap, wantSnap)
			}
		})
	}

	// Guards this test itself from silently checking zero ids if the
	// explicitlyCased set above ever drifted to cover the whole table.
	if tested == 0 {
		t.Fatal("test setup: expected at least one normalDefaults command id outside runDetailCommand's explicit cases")
	}
}

// --- Test 3: focus-preservation matrix ---
//
// From a detail-focused board, entering and exiting each of the nine modes
// this ticket newly makes reachable from the detail panel must leave
// detailFocused true and restore the detail-panel hint bar
// (registryHints(keymap.ModeDetail)), not the card-list one. Today this
// fails even at the "enter" step for every one of these nine modes: none of
// their command ids are among runDetailCommand's ~13 explicit cases, so
// dispatching them via the keymaps.detail override above falls off
// runDetailCommand's switch as a no-op (mode never leaves normalMode).
func TestDetailDelegation_FocusPreservationMatrix(t *testing.T) {
	cases := []struct {
		name     string
		entryKey string
		wantMode boardMode
	}{
		{"filter", "f", filterMode},
		{"assign", "a", assignMode},
		{"pr_list", "P", prListMode},
		{"milestone_list", "m", milestoneListMode},
		{"agent_list", "A", agentListMode},
		{"git_panel", "G", gitPanelMode},
		{"dispatch", "D", dispatchMode},
		{"delete", "d", deleteMode},
		{"search", "/", searchMode},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newDetailDelegationTestBoard(t, detailDelegationKeymapYAML)

			b = sendKey(t, b, keyMsg("l"))
			if !b.detailFocused {
				t.Fatal("precondition: detailFocused should be true after 'l'")
			}

			b = sendKey(t, b, keyMsg(tc.entryKey))
			if b.mode != tc.wantMode {
				t.Fatalf("mode after delegated entry key %q from detail focus = %v, want %v", tc.entryKey, b.mode, tc.wantMode)
			}
			if !b.detailFocused {
				t.Fatalf("detailFocused should remain true after delegating into %s mode", tc.name)
			}

			b = sendKey(t, b, arrowMsg(tea.KeyEsc))

			if b.mode != normalMode {
				t.Errorf("mode after Esc-exiting %s (entered via detail delegation) = %v, want normalMode", tc.name, b.mode)
			}
			if !b.detailFocused {
				t.Errorf("detailFocused should still be true after exiting %s mode entered from the detail panel", tc.name)
			}

			wantHints := b.registryHints(keymap.ModeDetail)
			if !reflect.DeepEqual(b.statusBar.hints, wantHints) {
				t.Errorf("hint bar after exiting %s (entered via detail delegation) = %+v, want detail hints %+v", tc.name, b.statusBar.hints, wantHints)
			}
		})
	}
}

// --- Test 4: reading-pane cursor-nav ---
//
// A delegated nav.cursor_down must move the cursor, keep the detail panel
// focused, reset detailScrollOffset for the newly selected card, and
// restore the detail hint bar (not the card-list one) -- "reading-pane"
// semantics: moving the selection while reading re-renders the newly
// selected card without leaving the panel. Bound onto f2 (see
// detailDelegationKeymapYAML's doc comment) since j/k in the detail panel
// are already claimed by detail.scroll_down/up.
func TestDetailDelegation_CursorDown_ReadingPaneSemantics(t *testing.T) {
	b := newDetailDelegationTestBoard(t, detailDelegationKeymapYAML)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	startCard := b.selectedCard()
	b.detailScrollOffset = 5 // simulate having scrolled into the current card

	b = sendKey(t, b, arrowMsg(tea.KeyF2)) // nav.cursor_down, via the override

	if !b.detailFocused {
		t.Error("a delegated nav.cursor_down from the detail panel should leave the panel focused (reading-pane semantics)")
	}

	newCard := b.selectedCard()
	if newCard.Number == startCard.Number {
		t.Fatalf("cursor did not move: still on card #%d after a delegated nav.cursor_down", startCard.Number)
	}

	if b.detailScrollOffset != 0 {
		t.Errorf("detailScrollOffset after a delegated nav.cursor_down = %d, want 0 (scroll resets for the newly selected card)", b.detailScrollOffset)
	}

	wantHints := b.registryHints(keymap.ModeDetail)
	if !reflect.DeepEqual(b.statusBar.hints, wantHints) {
		t.Errorf("hint bar after a delegated nav.cursor_down from the detail panel = %+v, want detail hints %+v", b.statusBar.hints, wantHints)
	}
}

// --- Test 5: search-from-detail empty-result edge case ---
//
// viewCardDetail (view.go) already guards len(col.Cards) > 0 before
// indexing into it. Before #588, board.search could never be triggered from
// the detail panel (it isn't in detailDefaults), so a search emptying the
// visible list while detailFocused == true was an unreachable combination.
// Delegation makes it reachable for the first time; this pins that View()
// renders a blank, still-focused panel instead of panicking.
func TestDetailDelegation_SearchEmptiesVisibleList_NoPanicBlankFocusedPanel(t *testing.T) {
	b := newDetailDelegationTestBoard(t, detailDelegationKeymapYAML)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	b = sendKey(t, b, keyMsg("/")) // board.search, via the override
	if b.mode != searchMode {
		t.Fatalf("precondition: mode after delegated board.search from detail focus = %v, want searchMode", b.mode)
	}
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should remain true after delegating into search mode")
	}

	b = sendKeys(t, b, "z", "z", "z", "n", "o", "m", "a", "t", "c", "h")
	if len(b.filteredCards()) != 0 {
		t.Fatalf("precondition: search query %q should match zero cards, got %d matches", b.searchQuery, len(b.filteredCards()))
	}

	b.Width = 120
	b.Height = 40

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("View() panicked while detail-focused with a search query matching zero cards: %v", r)
			}
		}()
		view := b.View()
		if view == "" {
			t.Error("View() should still render a non-empty layout (blank focused detail panel), not an empty string")
		}
	}()
}

// --- Test 6: keymaps.detail.d: card.delete end-to-end ---
//
// internal/config/keymap_semantic_validate.go's validateCommandIDs only
// checks catalogue membership (keymap.FindCommand), not mode capability, and
// validatePrintableRuneBindings only rejects bare printable runes in modes
// where Mode.ConsumesPrintableRunes() is true -- ModeDetail is not one of
// them. So keymaps.detail.d: card.delete already loads today without any
// internal/keymap/capability.go dependency (that's #577's job, out of scope
// here); this is the real config.Load -> config.ResolveKeymap -> dispatch ->
// deleteMode -> cancel path the plan calls for.
func TestDetailDelegation_KeymapsDetailCardDelete_EndToEnd(t *testing.T) {
	b := newDetailDelegationTestBoard(t, `keymaps:
  detail:
    d: card.delete
`)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	b = sendKey(t, b, keyMsg("d"))
	if b.mode != deleteMode {
		t.Fatalf("mode after delegated 'd' (keymaps.detail.d: card.delete) from detail focus = %v, want deleteMode", b.mode)
	}
	if !b.detailFocused {
		t.Fatal("detailFocused should remain true after entering the delete flow via detail-panel delegation")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("mode after Esc-cancelling the delegated delete flow = %v, want normalMode", b.mode)
	}
	if !b.detailFocused {
		t.Error("detailFocused should remain true after cancelling a delete flow entered via detail-panel delegation")
	}
}
