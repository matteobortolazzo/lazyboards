package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// Universal app.quit dispatch across all modes (#589)
//
// app.quit ("q" by default in normal/detail/help/error) is a system-level
// command, not a mode feature: Lookup's unconditional "ctrl+c" short-circuit
// (internal/keymap/lookup.go) already treats it that way at the key-
// resolution layer. This file's dispatch matrix proves the same holds at the
// dispatch layer, in every one of the 19 resolvable modes
// (keymap.Modes()) -- a user-bound keymaps.<mode>.<key>: app.quit must both
// load (internal/config, keymap_universal_quit_validation_test.go) and
// dispatch tea.Quit end-to-end through b.Update, regardless of whether that
// mode binds app.quit by default.
//
// Every case below enters its mode the real way (a production message or
// keypress through b.Update), never a raw b.mode assignment -- see
// keymap_panels_test.go's errorMode precedent and AGENTS.md's "enter modes
// the real way" rule.

// newUniversalQuitConfig loads localYAML through the real config.Load
// pipeline, failing the test on error.
func newUniversalQuitConfig(t *testing.T, localYAML string) config.Config {
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
	return cfg
}

// newUniversalQuitBoard wires a Board through the real production startup
// sequence (config.Load -> NewBoard -> board-data load -> config.ResolveKeymap
// -> withKeymap), mirroring newDetailDelegationTestBoard
// (detail_delegation_test.go:61). columns, when non-nil, replaces the
// FakeProvider's own fixture board -- needed by modes whose entry key gates
// on specific card shape (e.g. pr_picker's 2-LinkedPR card). defaultActions
// wires git_panel's "is this a git repo" availability gate
// (config.DefaultGitActions()). Returns the backing FakeProvider too.
func newUniversalQuitBoard(t *testing.T, localYAML string, defaultActions map[string]config.Action, columns []provider.Column) (Board, *provider.FakeProvider) {
	t.Helper()
	cfg := newUniversalQuitConfig(t, localYAML)

	p := provider.NewFakeProvider()
	b := NewBoard(p, defaultActions, cfg.Columns, nil, "matteobortolazzo", "lazyboards", "github", 0, 0, "Working", false, false, nil, nil, true)

	if columns != nil {
		m, _ := b.Update(boardFetchedMsg{board: provider.Board{Columns: columns}})
		loaded, ok := m.(Board)
		if !ok {
			t.Fatalf("Update returned %T, want Board", m)
		}
		loaded.Width = 120
		loaded.Height = 40
		b = loaded
	} else {
		b = loadFromFakeProvider(t, b, p)
	}

	km, err := config.ResolveKeymap(&cfg)
	if err != nil {
		t.Fatalf("config.ResolveKeymap() returned unexpected error: %v", err)
	}
	return b.withKeymap(km), p
}

// universalQuitKeymapYAML returns a keymaps.<mode>.ctrl+q: app.quit override
// -- the shared per-mode fixture every case in the dispatch matrix binds
// app.quit onto. ctrl+q is unbound by every default table
// (internal/keymap/defaults_*.go) and is a named (non-rune) key, so it is
// safe to bind even in the five ConsumesPrintableRunes() text modes
// (create/config/search/comment/delete) without tripping
// validatePrintableRuneBindings.
func universalQuitKeymapYAML(mode keymap.Mode) string {
	return "keymaps:\n  " + string(mode) + ":\n    ctrl+q: app.quit\n"
}

// quitDispatchKey is the tea.KeyMsg every case in the dispatch matrix sends
// after entering its mode, matching universalQuitKeymapYAML's binding.
func quitDispatchKey() tea.KeyMsg {
	return arrowMsg(tea.KeyCtrlQ)
}

// assertQuitCmd fails the test unless cmd is non-nil and, when invoked,
// produces tea.QuitMsg -- the precise "is this tea.Quit" check, not just a
// non-nil-Cmd approximation. tea.Quit is a plain synchronous func (no
// goroutine/timer involved), so calling cmd() directly is safe here (see
// docs/bubbletea-async-patterns.md for when that would NOT be safe).
func assertQuitCmd(t *testing.T, mode keymap.Mode, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		t.Fatalf("mode %q: ctrl+q returned a nil Cmd, want tea.Quit", mode)
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("mode %q: ctrl+q's Cmd did not produce tea.QuitMsg, want tea.Quit", mode)
	}
}

// cmdIsQuit reports whether cmd is exactly universalDispatch's tea.Quit,
// identified by function-pointer identity rather than invocation. Unlike
// assertQuitCmd's positive check (which safely calls cmd() because tea.Quit
// is a plain synchronous func), a *negative* "this must not be tea.Quit"
// check cannot call an arbitrary cmd: several of the risk-guard tests below
// send a bare printable rune into a focused bubbles/textinput, whose Update
// batches in a cursor-blink tea.Tick whenever the cursor position changes
// (see bubbles/textinput.Model.Update) -- invoking that synchronously would
// hang the test for the blink interval (docs/bubbletea-async-patterns.md).
// universalDispatch returns the tea.Quit function value directly (never a
// wrapping closure), so comparing the underlying code pointer is an exact,
// side-effect-free identity check.
func cmdIsQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	return reflect.ValueOf(cmd).Pointer() == reflect.ValueOf(tea.Cmd(tea.Quit)).Pointer()
}

// TestUniversalQuit_DispatchesInEveryMode is the 19-mode dispatch matrix
// (AC1; AC8's pending-sequence case is covered separately below by
// TestUniversalQuit_PendingSequence_GQ_DispatchesViaHandlePendingSeqKey,
// since none of these 19 cases exercises a multi-key binding). Each case
// enters its mode the real way, then sends ctrl+q (bound to app.quit by
// universalQuitKeymapYAML) and asserts the resulting Cmd is tea.Quit.
func TestUniversalQuit_DispatchesInEveryMode(t *testing.T) {
	cases := []struct {
		name  string
		mode  keymap.Mode
		enter func(t *testing.T) Board
	}{
		{"normal", keymap.ModeNormal, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeNormal), nil, nil)
			if b.mode != normalMode {
				t.Fatalf("precondition: mode = %d, want normalMode", b.mode)
			}
			return b
		}},
		{"detail", keymap.ModeDetail, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeDetail), nil, nil)
			b = sendKey(t, b, keyMsg("l"))
			if !b.detailFocused {
				t.Fatalf("precondition: detailFocused = false after 'l', want true")
			}
			return b
		}},
		{"create", keymap.ModeCreate, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeCreate), nil, nil)
			b = sendKey(t, b, keyMsg("n"))
			if b.mode != createMode {
				t.Fatalf("precondition: mode = %d, want createMode", b.mode)
			}
			return b
		}},
		{"error", keymap.ModeError, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeError), nil, nil)
			b = sendKey(t, b, boardFetchErrorMsg{err: errors.New("boom")})
			if b.mode != errorMode {
				t.Fatalf("precondition: mode = %d, want errorMode", b.mode)
			}
			return b
		}},
		{"config", keymap.ModeConfig, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeConfig), nil, nil)
			b = sendKey(t, b, keyMsg("c"))
			if b.mode != configMode {
				t.Fatalf("precondition: mode = %d, want configMode", b.mode)
			}
			return b
		}},
		{"pr_picker", keymap.ModePRPicker, func(t *testing.T) Board {
			columns := []provider.Column{{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "Two PRs", LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "PR One", URL: "https://github.com/owner/repo/pull/10"},
					{Number: 20, Title: "PR Two", URL: "https://github.com/owner/repo/pull/20"},
				}},
			}}}
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModePRPicker), nil, columns)
			b = sendKey(t, b, keyMsg("p"))
			if b.mode != prPickerMode {
				t.Fatalf("precondition: mode = %d, want prPickerMode", b.mode)
			}
			return b
		}},
		{"search", keymap.ModeSearch, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeSearch), nil, nil)
			b = sendKey(t, b, keyMsg("/"))
			if b.mode != searchMode {
				t.Fatalf("precondition: mode = %d, want searchMode", b.mode)
			}
			return b
		}},
		{"help", keymap.ModeHelp, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeHelp), nil, nil)
			b = sendKey(t, b, keyMsg("?"))
			if b.mode != helpMode {
				t.Fatalf("precondition: mode = %d, want helpMode", b.mode)
			}
			return b
		}},
		{"label_confirm", keymap.ModeLabelConfirm, func(t *testing.T) Board {
			columns := []provider.Column{{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "Card One", Labels: []provider.Label{{Name: "bug"}}},
			}}}
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeLabelConfirm), nil, columns)
			card := b.Columns[0].Cards[0]
			originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
			editedContent := composeFrontmatter(card.Title, []string{"bug", "newlabel"}, card.Body)
			b = sendKey(t, b, editorFinishedMsg{editedContent: editedContent, originalContent: originalContent, card: card})
			if b.mode != labelConfirmMode {
				t.Fatalf("precondition: mode = %d, want labelConfirmMode", b.mode)
			}
			return b
		}},
		{"close_confirm", keymap.ModeCloseConfirm, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeCloseConfirm), nil, nil)
			b = sendKey(t, b, keyMsg("x"))
			if b.mode != closeConfirmMode {
				t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
			}
			return b
		}},
		{"comment", keymap.ModeComment, func(t *testing.T) Board {
			// The alt-overload of a {comment}-bearing normal-mode action is
			// what opens comment mode, so both tables live under one
			// keymaps: block (a second top-level keymaps: key would be a
			// duplicate-mapping-key YAML error).
			yamlContent := universalQuitKeymapYAML(keymap.ModeComment) + `  normal:
    X:
      name: Annotate
      type: shell
      scope: card
      command: "echo {comment} {number}"
`
			b, _ := newUniversalQuitBoard(t, yamlContent, nil, nil)
			b = sendKey(t, b, altKeyMsg("X"))
			if b.mode != commentMode {
				t.Fatalf("precondition: mode = %d, want commentMode", b.mode)
			}
			return b
		}},
		{"delete", keymap.ModeDelete, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeDelete), nil, nil)
			b = sendKey(t, b, keyMsg("d"))
			if b.mode != deleteMode {
				t.Fatalf("precondition: mode = %d, want deleteMode", b.mode)
			}
			return b
		}},
		{"filter", keymap.ModeFilter, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeFilter), nil, nil)
			b = sendKey(t, b, keyMsg("f"))
			if b.mode != filterMode {
				t.Fatalf("precondition: mode = %d, want filterMode", b.mode)
			}
			return b
		}},
		{"assign", keymap.ModeAssign, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeAssign), nil, nil)
			// Populate collaborators post-load, exactly like
			// newDetailDelegationTestBoard (detail_delegation_test.go) does --
			// production populates this via a separate metadata fetch
			// (boardFetchedMsg.collaborators), not the initial board fetch.
			b.collaborators = []Assignee{{Login: "alice"}}
			b = sendKey(t, b, keyMsg("a"))
			if b.mode != assignMode {
				t.Fatalf("precondition: mode = %d, want assignMode", b.mode)
			}
			return b
		}},
		{"git_panel", keymap.ModeGitPanel, func(t *testing.T) Board {
			columns := []provider.Column{{Title: "Empty", Cards: nil}}
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeGitPanel), config.DefaultGitActions(), columns)
			b = sendKey(t, b, keyMsg("G"))
			if b.mode != gitPanelMode {
				t.Fatalf("precondition: mode = %d, want gitPanelMode", b.mode)
			}
			return b
		}},
		{"dispatch", keymap.ModeDispatch, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeDispatch), nil, nil)
			b = sendKey(t, b, keyMsg("D"))
			if b.mode != dispatchMode {
				t.Fatalf("precondition: mode = %d, want dispatchMode", b.mode)
			}
			return b
		}},
		{"pr_list", keymap.ModePRList, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModePRList), nil, nil)
			b = sendKey(t, b, keyMsg("P"))
			if b.mode != prListMode {
				t.Fatalf("precondition: mode = %d, want prListMode", b.mode)
			}
			return b
		}},
		{"milestone_list", keymap.ModeMilestoneList, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeMilestoneList), nil, nil)
			b = sendKey(t, b, keyMsg("m"))
			if b.mode != milestoneListMode {
				t.Fatalf("precondition: mode = %d, want milestoneListMode", b.mode)
			}
			return b
		}},
		{"agent_list", keymap.ModeAgentList, func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeAgentList), nil, nil)
			b = sendKey(t, b, keyMsg("A"))
			if b.mode != agentListMode {
				t.Fatalf("precondition: mode = %d, want agentListMode", b.mode)
			}
			return b
		}},
	}

	if len(cases) != len(keymap.Modes()) {
		t.Fatalf("dispatch matrix has %d cases, want %d (one per keymap.Modes())", len(cases), len(keymap.Modes()))
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.enter(t)
			m, cmd := b.Update(quitDispatchKey())
			if _, ok := m.(Board); !ok {
				t.Fatalf("Update returned %T, want Board", m)
			}
			assertQuitCmd(t, tc.mode, cmd)
		})
	}
}

// --- #589 explicit risk-guard coverage (.claude/rules/testing.md) ---
//
// git_panel's "user-bound key" requirement (Test Strategy: runGitPanelCommand
// accepts CommandQuit today while gitPanelDefaults binds no app.quit key by
// default, so a default-table-only test could never observe the dispatch)
// is already satisfied by the 19-mode matrix above: its "git_panel" case
// binds ctrl+q via universalQuitKeymapYAML and dispatches through the real
// config.DefaultGitActions()-gated panel, so it is not duplicated here.

// --- R1: printable-rune theft ---
//
// In each of the five ConsumesPrintableRunes() modes, binding app.quit onto
// a named (non-rune) key like ctrl+q must never let a bare printable rune
// steal the keystroke from that mode's textinput: textBinding's guard
// (keymap_text.go) refuses to resolve ANY printable rune in these modes
// before universalDispatch is ever consulted, so "q" must land as literal
// text and the returned Cmd must not be tea.Quit.
func TestUniversalQuit_R1_BareRuneStillReachesTextInputNotQuit(t *testing.T) {
	cases := []struct {
		name  string
		enter func(t *testing.T) Board
		value func(b Board) string
	}{
		{"create", func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeCreate), nil, nil)
			b = sendKey(t, b, keyMsg("n"))
			if b.mode != createMode {
				t.Fatalf("precondition: mode = %d, want createMode", b.mode)
			}
			return b
		}, func(b Board) string { return b.create.titleInput.Value() }},
		{"config", func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeConfig), nil, nil)
			b = sendKey(t, b, keyMsg("c"))
			b = sendKey(t, b, arrowMsg(tea.KeyTab)) // focus 0 is the provider picker; focus 1 (repoInput) is the text field.
			if b.mode != configMode || b.config.focus != 1 {
				t.Fatalf("precondition: mode=%d focus=%d, want configMode/focus=1 (repo field)", b.mode, b.config.focus)
			}
			return b
		}, func(b Board) string { return b.config.repoInput.Value() }},
		{"search", func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeSearch), nil, nil)
			b = sendKey(t, b, keyMsg("/"))
			if b.mode != searchMode {
				t.Fatalf("precondition: mode = %d, want searchMode", b.mode)
			}
			return b
		}, func(b Board) string { return b.searchInput.Value() }},
		{"comment", func(t *testing.T) Board {
			// The alt-overload of a {comment}-bearing normal-mode action is
			// what opens comment mode, so both tables live under one
			// keymaps: block (a second top-level keymaps: key would be a
			// duplicate-mapping-key YAML error).
			yamlContent := universalQuitKeymapYAML(keymap.ModeComment) + `  normal:
    X:
      name: Annotate
      type: shell
      scope: card
      command: "echo {comment} {number}"
`
			b, _ := newUniversalQuitBoard(t, yamlContent, nil, nil)
			b = sendKey(t, b, altKeyMsg("X"))
			if b.mode != commentMode {
				t.Fatalf("precondition: mode = %d, want commentMode", b.mode)
			}
			return b
		}, func(b Board) string { return b.comment.input.Value() }},
		{"delete", func(t *testing.T) Board {
			b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeDelete), nil, nil)
			b = sendKey(t, b, keyMsg("d"))
			if b.mode != deleteMode || b.delete.step != deleteStepComment {
				t.Fatalf("precondition: mode=%d step=%d, want deleteMode/deleteStepComment", b.mode, b.delete.step)
			}
			return b
		}, func(b Board) string { return b.delete.commentInput.Value() }},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.enter(t)
			before := tc.value(b) // config's repoInput pre-populates from repoOwner/repoName, so assert an appended "q", not an exact "q".
			m, cmd := b.Update(keyMsg("q"))
			b2, ok := m.(Board)
			if !ok {
				t.Fatalf("Update returned %T, want Board", m)
			}
			if got, want := tc.value(b2), before+"q"; got != want {
				t.Errorf("textinput value = %q after bare 'q' (app.quit bound to ctrl+q), want %q -- the rune must land as literal text, not be intercepted", got, want)
			}
			if cmdIsQuit(cmd) {
				t.Error("bare 'q' must not dispatch tea.Quit when app.quit is bound to ctrl+q -- printable-rune theft (R1)")
			}
		})
	}
}

// --- R1b: alt+<rune> exemption ---
//
// alt+q bound to app.quit in create mode must dispatch tea.Quit and must NOT
// reach the textinput: textBinding's guard only refuses a printable rune
// WITHOUT the Alt modifier (msg.Type == tea.KeyRunes && !msg.Alt), so an
// explicit alt+ binding is exempt by design.
func TestUniversalQuit_R1b_AltRuneDispatchesQuit_BypassesTextInput(t *testing.T) {
	b, _ := newUniversalQuitBoard(t, "keymaps:\n  create:\n    alt+q: app.quit\n", nil, nil)
	b = sendKey(t, b, keyMsg("n"))
	if b.mode != createMode {
		t.Fatalf("precondition: mode = %d, want createMode", b.mode)
	}

	m, cmd := b.Update(altKeyMsg("q"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	assertQuitCmd(t, keymap.ModeCreate, cmd)
	if got := b2.create.titleInput.Value(); got != "" {
		t.Errorf("titleInput.Value() = %q after alt+q, want empty -- alt+<rune> bound to app.quit must dispatch, not reach the textinput", got)
	}
}

// --- R2: literal-Escape-mid-confirm precedence ---
//
// handleDispatchModalKey's confirmingLoop branch checks msg.Type ==
// tea.KeyEscape BEFORE consulting the table (and thus before
// universalDispatch), so a literal Escape must always cancel the confirm
// step, even when a user rebinds "esc" onto app.quit.
func TestUniversalQuit_R2_LiteralEscapeDuringConfirmingLoop_StillCancelsConfirmOnly(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDispatch: {"esc": keymap.CommandBinding(keymap.CommandQuit)},
	}, nil)
	b = sendKey(t, b, keyMsg("D"))
	if b.mode != dispatchMode {
		t.Fatalf("precondition: mode = %d, want dispatchMode", b.mode)
	}
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: true}

	m, cmd := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.dispatch.confirmingLoop {
		t.Error("literal Escape mid-confirm should resolve to CommandDispatchCancelLoop and clear confirmingLoop, even with esc rebound to app.quit")
	}
	if b2.mode != dispatchMode {
		t.Errorf("mode = %v after literal Escape mid-confirm, want dispatchMode to stay open (cancel the confirm ONLY)", b2.mode)
	}
	if cmdIsQuit(cmd) {
		t.Error("literal Escape mid-confirm must never dispatch app.quit, even when esc is rebound to it (R2)")
	}
}

// TestUniversalQuit_R2_NonEscapeKeyDuringConfirmingLoop_Quits covers R2's
// second half ("a different key bound to app.quit must quit while
// confirming") and doubles as the dispatch-loop-confirm destructive-mode
// case (Test Strategy): quitting must leave confirmingLoop uncommitted
// (still true, neither confirmed nor cancelled) and must never run the
// loop-toggle command.
func TestUniversalQuit_R2_NonEscapeKeyDuringConfirmingLoop_Quits(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDispatch: {"ctrl+q": keymap.CommandBinding(keymap.CommandQuit)},
	}, nil)
	b = sendKey(t, b, keyMsg("D"))
	if b.mode != dispatchMode {
		t.Fatalf("precondition: mode = %d, want dispatchMode", b.mode)
	}
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: true}

	m, cmd := b.Update(quitDispatchKey())
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	assertQuitCmd(t, keymap.ModeDispatch, cmd)
	if !b2.dispatch.confirmingLoop {
		t.Error("quitting mid-confirm must leave confirmingLoop uncommitted (still true), not silently confirm/cancel the pending loop toggle")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("quitting mid-confirm must not run the loop-toggle command, got %v", fe.RunShellOutputCalls)
	}
}

// --- R3: silent quit regression (post-removal of the five CommandQuit cases) ---
//
// normal/help/error's default-table 'q' end-to-end through b.Update are
// already covered elsewhere (update_test.go's TestKeyPress_*Quit* tests,
// help_test.go's TestHelpMode_QKey_Quits, keymap_panels_test.go's error-mode
// default-parity dispatch test) -- only detail-focused 'q' has no existing
// end-to-end coverage, so that is the one gap this adds.
func TestUniversalQuit_R3_DetailFocused_DefaultQKey_StillQuits(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	m, cmd := b.Update(keyMsg("q"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if !b2.detailFocused {
		t.Fatal("precondition check: detailFocused should still be true going into the quit dispatch")
	}
	assertQuitCmd(t, keymap.ModeDetail, cmd)
}

// --- Destructive modes: quitting must never commit the pending action ---

// TestUniversalQuit_Destructive_DeleteCommentStep_QuitLeavesActionUncommitted
// covers the delete flow's first step: quitting must not post the optional
// comment and must not advance to the confirm step.
func TestUniversalQuit_Destructive_DeleteCommentStep_QuitLeavesActionUncommitted(t *testing.T) {
	b, p := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeDelete), nil, nil)
	b = sendKey(t, b, keyMsg("d"))
	if b.mode != deleteMode || b.delete.step != deleteStepComment {
		t.Fatalf("precondition: mode=%d step=%d, want deleteMode/deleteStepComment", b.mode, b.delete.step)
	}
	cardNumber := b.delete.card.Number

	m, cmd := b.Update(quitDispatchKey())
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	assertQuitCmd(t, keymap.ModeDelete, cmd)
	if b2.mode != deleteMode || b2.delete.step != deleteStepComment {
		t.Errorf("mode=%d step=%d after quit, want unchanged deleteMode/deleteStepComment (the delete flow must not advance)", b2.mode, b2.delete.step)
	}
	if len(p.Comments[cardNumber]) != 0 {
		t.Errorf("expected no comment posted after quitting mid-delete, got %v", p.Comments[cardNumber])
	}
}

// TestUniversalQuit_Destructive_DeleteConfirmStep_QuitLeavesActionUncommitted
// covers the delete flow's second step: quitting must not delete the card.
func TestUniversalQuit_Destructive_DeleteConfirmStep_QuitLeavesActionUncommitted(t *testing.T) {
	b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeDelete), nil, nil)
	b = sendKey(t, b, keyMsg("d"))
	b = sendKey(t, b, arrowMsg(tea.KeyEnter))
	if b.mode != deleteMode || b.delete.step != deleteStepConfirm {
		t.Fatalf("precondition: mode=%d step=%d, want deleteMode/deleteStepConfirm", b.mode, b.delete.step)
	}
	cardNumber := b.delete.card.Number

	m, cmd := b.Update(quitDispatchKey())
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	assertQuitCmd(t, keymap.ModeDelete, cmd)
	if b2.mode != deleteMode || b2.delete.step != deleteStepConfirm {
		t.Errorf("mode=%d step=%d after quit, want unchanged deleteMode/deleteStepConfirm", b2.mode, b2.delete.step)
	}
	found := false
	for _, col := range b2.Columns {
		for _, c := range col.Cards {
			if c.Number == cardNumber {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected the card to remain on the board (DeleteCard never dispatched) after quitting at the confirm step")
	}
}

// TestUniversalQuit_Destructive_CloseConfirm_QuitLeavesActionUncommitted
// covers close_confirm: quitting must not close the card.
func TestUniversalQuit_Destructive_CloseConfirm_QuitLeavesActionUncommitted(t *testing.T) {
	b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeCloseConfirm), nil, nil)
	b = sendKey(t, b, keyMsg("x"))
	if b.mode != closeConfirmMode {
		t.Fatalf("precondition: mode = %d, want closeConfirmMode", b.mode)
	}
	cardNumber := b.closeConfirm.card.Number

	m, cmd := b.Update(quitDispatchKey())
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	assertQuitCmd(t, keymap.ModeCloseConfirm, cmd)
	if b2.mode != closeConfirmMode {
		t.Errorf("mode = %d after quit, want unchanged closeConfirmMode (close must not be committed)", b2.mode)
	}
	found := false
	for _, col := range b2.Columns {
		for _, c := range col.Cards {
			if c.Number == cardNumber {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected the card to remain present (CloseCard never dispatched) after quitting mid-confirm")
	}
}

// TestUniversalQuit_Destructive_LabelConfirm_QuitLeavesActionUncommitted
// covers label_confirm: quitting must not create the pending label or
// advance labelConfirm.currentIdx.
func TestUniversalQuit_Destructive_LabelConfirm_QuitLeavesActionUncommitted(t *testing.T) {
	columns := []provider.Column{{Title: "Column A", Cards: []provider.Card{
		{Number: 1, Title: "Card One", Labels: []provider.Label{{Name: "bug"}}},
	}}}
	b, _ := newUniversalQuitBoard(t, universalQuitKeymapYAML(keymap.ModeLabelConfirm), nil, columns)
	card := b.Columns[0].Cards[0]
	originalContent := composeFrontmatter(card.Title, []string{"bug"}, card.Body)
	editedContent := composeFrontmatter(card.Title, []string{"bug", "newlabel"}, card.Body)
	b = sendKey(t, b, editorFinishedMsg{editedContent: editedContent, originalContent: originalContent, card: card})
	if b.mode != labelConfirmMode {
		t.Fatalf("precondition: mode = %d, want labelConfirmMode", b.mode)
	}
	idxBefore := b.labelConfirm.currentIdx

	m, cmd := b.Update(quitDispatchKey())
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	assertQuitCmd(t, keymap.ModeLabelConfirm, cmd)
	if b2.mode != labelConfirmMode {
		t.Errorf("mode = %d after quit, want unchanged labelConfirmMode (label creation must not be committed)", b2.mode)
	}
	if b2.labelConfirm.currentIdx != idxBefore {
		t.Errorf("labelConfirm.currentIdx = %d after quit, want unchanged %d", b2.labelConfirm.currentIdx, idxBefore)
	}
}

// --- Pending sequence (AC 8) ---

// TestUniversalQuit_PendingSequence_GQ_DispatchesViaHandlePendingSeqKey binds
// app.quit onto the two-key "g q" sequence in normal mode: 'g' alone must
// enter the pending-sequence which-key state (no dispatch yet), and 'q'
// must then complete it via handlePendingSeqKey -> dispatchBinding's
// universalDispatch check.
func TestUniversalQuit_PendingSequence_GQ_DispatchesViaHandlePendingSeqKey(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"g q": keymap.CommandBinding(keymap.CommandQuit)},
	}, nil)

	m1, cmd1 := b.Update(keyMsg("g"))
	b1, ok := m1.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m1)
	}
	if b1.pendingSeq != "g" {
		t.Fatalf("precondition: pendingSeq = %q after 'g', want %q (pending \"g q\")", b1.pendingSeq, "g")
	}
	if cmd1 != nil {
		t.Error("entering a pending sequence must not itself fire a Cmd")
	}

	m2, cmd2 := b1.Update(keyMsg("q"))
	b2, ok := m2.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m2)
	}
	assertQuitCmd(t, keymap.ModeNormal, cmd2)
	if b2.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after dispatch, want empty", b2.pendingSeq)
	}
}

// --- Single-key-only caveat pin (Q4) ---

// TestUniversalQuit_SingleKeyOnlyCaveat_MultiKeyBindingInFilterModeIsNoOp
// pins documented behavior, not a bug: outside normal/detail, every seam
// (here lookupModalBinding, keymap_modals.go) is single-key exact-match
// only -- there is no pending-sequence machinery to extend 'g' into "g q"
// in filterMode (see handleFilterModeKey's OutcomePending case, "Q4" in the
// comment), so a multi-key app.quit binding there can never dispatch.
func TestUniversalQuit_SingleKeyOnlyCaveat_MultiKeyBindingInFilterModeIsNoOp(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeFilter: {"g q": keymap.CommandBinding(keymap.CommandQuit)},
	}, nil)
	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("precondition: mode = %d, want filterMode", b.mode)
	}

	m1, cmd1 := b.Update(keyMsg("g"))
	b1, ok := m1.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m1)
	}
	if cmdIsQuit(cmd1) {
		t.Fatal("'g' alone must never dispatch quit")
	}
	if b1.mode != filterMode {
		t.Fatalf("mode = %d after 'g' in filterMode, want unchanged filterMode", b1.mode)
	}

	m2, cmd2 := b1.Update(keyMsg("q"))
	b2, ok := m2.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m2)
	}
	if cmdIsQuit(cmd2) {
		t.Error("a multi-key app.quit binding (\"g q\") in a single-key seam mode must be a documented no-op, never dispatch (Q4)")
	}
	if b2.mode != filterMode {
		t.Errorf("mode = %d after 'g q' typed sequentially in filterMode, want unchanged filterMode", b2.mode)
	}
}
