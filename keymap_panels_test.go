package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Route the git panel through the registry (#511, PR 1/2) ---
//
// handleGitPanelKey cuts over from dispatching against b.defaultActions (a
// plain map[string]config.Action) to keymap.Keymap.Lookup against
// keymap.ModeGitPanel, mirroring the normal-mode/detail-panel cutover
// (#489, keymap_dispatch.go): keymap_panels.go will provide panelEntries/
// panelBinding/panelHintKey (shared, mode-generic building blocks also used
// by the dispatch-modal cutover in PR 2/2), plus git-panel-specific
// gitPanelHints/gitPanelItemsFromKeymap/runGitPanelCommand/
// dispatchGitMenuAction. None of these exist yet -- this file is expected to
// fail to compile until keymap_panels.go lands.
//
// Default-table end-to-end coverage (mode transitions, cursor wrapping,
// direct-key dispatch, view rendering) already lives in git_panel_test.go
// and stays green throughout (behavior-preserving refactor). The tests here
// cover what the registry newly makes possible for the git panel: a
// keymaps.git_panel override winning over a built-in, explicit unbind (both
// the no-op dispatch AND the menu row disappearing), the hint<->dispatch
// invariant, and untrusted action-name sanitization in the menu view.

// --- panelEntries / panelBinding: mode-generic lookup primitives ---

func TestKeymapPanels_PanelEntries_ReturnsGitPanelModeTable(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	entries := b.panelEntries(keymap.ModeGitPanel)

	found := false
	for _, e := range entries {
		if e.Sequence == "P" {
			found = true
			if e.Binding.Kind != keymap.BindingAction || e.Binding.Action.Command != "git push" {
				t.Errorf("panelEntries(ModeGitPanel) entry for \"P\" = %+v, want the default git push action", e)
			}
		}
	}
	if !found {
		t.Fatal("panelEntries(ModeGitPanel) missing an entry for \"P\"")
	}
}

// TestKeymapPanels_PanelBinding_ExactSingleKeyMatchOnly's multi-key-prefix
// case below is defensive-runtime coverage: #578's validateSequenceCapability
// now rejects a multi-key keymaps.git_panel binding at config-load time; the
// boardWithOverrideKeymap fixture used here bypasses config.Load, so this
// stays as the hand-built-keymap negative --
// internal/config/keymap_sequence_validation_test.go is the user-facing
// contract.
func TestKeymapPanels_PanelBinding_ExactSingleKeyMatchOnly(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	binding, ok := b.panelBinding(keymap.ModeGitPanel, keyMsg("P"))
	if !ok {
		t.Fatal(`panelBinding(ModeGitPanel, "P") = not found, want a resolved binding for the default Push action`)
	}
	if binding.Kind != keymap.BindingAction || binding.Action.Command != "git push" {
		t.Errorf(`panelBinding(ModeGitPanel, "P") = %+v, want the default git push action`, binding)
	}

	// An OutcomePending prefix must not resolve -- git panel dispatch is
	// single-key exact match only (unlike normal-mode's multi-key
	// sequences).
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeGitPanel: {"z a": keymap.CommandBinding(keymap.CommandGitPanelClose)},
	}, nil)
	if _, ok := b.panelBinding(keymap.ModeGitPanel, keyMsg("z")); ok {
		t.Error("panelBinding must not resolve an OutcomePending prefix key, only exact single-key matches")
	}

	// OutcomeNoMatch must not resolve either.
	if _, ok := b.panelBinding(keymap.ModeGitPanel, keyMsg("x")); ok {
		t.Error(`panelBinding(ModeGitPanel, "x") resolved, want not found for a genuinely unbound key`)
	}
}

// --- panelHintKey: named-first, "/"-joined, multi-key-filtered hint label ---

func TestKeymapPanels_PanelHintKey_NamedFirstPerCommandJoinedAcrossIDs(t *testing.T) {
	entries := []keymap.Entry{
		{Sequence: "esc", Binding: keymap.CommandBinding(keymap.CommandGitPanelClose)},
		{Sequence: "q", Binding: keymap.CommandBinding(keymap.CommandGitPanelClose)},
		{Sequence: "j", Binding: keymap.CommandBinding(keymap.CommandGitPanelNext)},
		{Sequence: "k", Binding: keymap.CommandBinding(keymap.CommandGitPanelPrev)},
		{Sequence: "g d", Binding: keymap.CommandBinding(keymap.CommandGitPanelPrev)},
	}

	// Two keys bound to the same command: the named multi-char key ("esc")
	// is preferred over the single-rune alternative ("q").
	if got := panelHintKey(entries, keymap.CommandGitPanelClose); got != "esc" {
		t.Errorf("panelHintKey(entries, CommandGitPanelClose) = %q, want %q (named key preferred over single-rune)", got, "esc")
	}

	// Multiple ids: each contributes its own picked key, "/"-joined -- the
	// multi-key sequence "g d" bound to Prev must be filtered out in favor
	// of the single-key "k".
	if got := panelHintKey(entries, keymap.CommandGitPanelNext, keymap.CommandGitPanelPrev); got != "j/k" {
		t.Errorf("panelHintKey(entries, Next, Prev) = %q, want %q", got, "j/k")
	}

	// Nothing bound for the id -> empty string.
	if got := panelHintKey(entries, keymap.CommandGitPanelRun); got != "" {
		t.Errorf("panelHintKey(entries, Run) = %q, want empty string when nothing is bound", got)
	}
}

// --- Default parity: registry-derived items/hints/dispatch match today ---

func TestKeymapPanels_GitPanel_DefaultParity_ItemsMatchLegacyBuiltinOrder(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	got := b.gitPanelItemsFromKeymap()
	want := config.DefaultGitActions()

	if len(got) != len(gitPanelBuiltinOrder) {
		t.Fatalf("gitPanelItemsFromKeymap() returned %d items, want %d (len(gitPanelBuiltinOrder))", len(got), len(gitPanelBuiltinOrder))
	}
	for i, key := range gitPanelBuiltinOrder {
		if got[i].key != key {
			t.Fatalf("items[%d].key = %q, want %q (gitPanelBuiltinOrder order)", i, got[i].key, key)
		}
		if got[i].name != want[key].Name {
			t.Errorf("items[%d].name = %q, want %q", i, got[i].name, want[key].Name)
		}
	}
}

func TestKeymapPanels_GitPanel_DefaultParity_HintsMatchTodaysGitPanelModeHints(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	got := b.gitPanelHints()

	if len(got) != len(gitPanelModeHints) {
		t.Fatalf("gitPanelHints() = %+v, want length %d matching gitPanelModeHints %+v", got, len(gitPanelModeHints), gitPanelModeHints)
	}
	for i := range gitPanelModeHints {
		if got[i] != gitPanelModeHints[i] {
			t.Errorf("gitPanelHints()[%d] = %+v, want %+v", i, got[i], gitPanelModeHints[i])
		}
	}
}

func TestKeymapPanels_GitPanel_DefaultParity_NavRunCloseKeysResolveToExpectedCommands(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)

	cases := []struct {
		msg  tea.KeyMsg
		want keymap.CommandID
	}{
		{keyMsg("j"), keymap.CommandGitPanelNext},
		{arrowMsg(tea.KeyDown), keymap.CommandGitPanelNext},
		{keyMsg("k"), keymap.CommandGitPanelPrev},
		{arrowMsg(tea.KeyUp), keymap.CommandGitPanelPrev},
		{arrowMsg(tea.KeyEnter), keymap.CommandGitPanelRun},
		{arrowMsg(tea.KeyEsc), keymap.CommandGitPanelClose},
	}
	for _, tc := range cases {
		binding, ok := b.panelBinding(keymap.ModeGitPanel, tc.msg)
		if !ok {
			t.Errorf("panelBinding(ModeGitPanel, %q) = not found, want command %v", tc.msg.String(), tc.want)
			continue
		}
		if binding.Kind != keymap.BindingCommand || binding.Command != tc.want {
			t.Errorf("panelBinding(ModeGitPanel, %q) = %+v, want command %v", tc.msg.String(), binding, tc.want)
		}
	}
}

func TestKeymapPanels_GitPanel_Navigation_NextPrevViaRunGitPanelCommand(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)
	b = sendKey(t, b, keyMsg("G"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
	}
	initial := b.gitPanel.cursor

	m, _ := b.runGitPanelCommand(keymap.CommandGitPanelNext)
	b = m.(Board)
	wantNext := moveCursor(initial, len(b.gitPanel.items), true)
	if b.gitPanel.cursor != wantNext {
		t.Errorf("cursor after CommandGitPanelNext = %d, want %d", b.gitPanel.cursor, wantNext)
	}

	afterNext := b.gitPanel.cursor
	m, _ = b.runGitPanelCommand(keymap.CommandGitPanelPrev)
	b = m.(Board)
	wantPrev := moveCursor(afterNext, len(b.gitPanel.items), false)
	if b.gitPanel.cursor != wantPrev {
		t.Errorf("cursor after CommandGitPanelPrev = %d, want %d", b.gitPanel.cursor, wantPrev)
	}
}

func TestKeymapPanels_GitPanel_CloseViaRunGitPanelCommand(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)
	b = sendKey(t, b, keyMsg("G"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
	}

	m, _ := b.runGitPanelCommand(keymap.CommandGitPanelClose)
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %v after CommandGitPanelClose, want normalMode", b.mode)
	}
}

func TestKeymapPanels_GitPanel_RunViaRunGitPanelCommand_DispatchesBuiltinAction(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)
	b = sendKey(t, b, keyMsg("G"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
	}
	idx := gitPanelItemIndex(b, "f")
	if idx == -1 {
		t.Fatal("expected a Fetch (key f) entry in the git panel items")
	}
	b.gitPanel.cursor = idx

	m, cmd := b.runGitPanelCommand(keymap.CommandGitPanelRun)
	b = m.(Board)
	if cmd == nil {
		t.Fatal("CommandGitPanelRun should return a non-nil cmd")
	}
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != "git fetch" {
		t.Errorf("RunShellCalls = %v, want first call to be %q", fe.RunShellCalls, "git fetch")
	}
	if b.mode != normalMode {
		t.Errorf("mode = %v after CommandGitPanelRun, want normalMode (menu closes on run)", b.mode)
	}
}

// TestKeymapPanels_GitPanel_DefaultParity_AllBuiltinActionsMatchLegacyDispatch
// asserts, for every one of the six built-in git actions, that pressing its
// default key dispatches -- through the registry path (panelBinding ->
// dispatchGitMenuAction) -- the exact config.Action config.DefaultGitActions
// defines for that key (same Name/Type/Command), and runs the resulting
// shell command. This mirrors
// TestKeymapPanels_GitPanel_DefaultParity_ItemsMatchLegacyBuiltinOrder in
// comparing against config.DefaultGitActions() directly rather than a second
// production dispatch path (the old b.defaultActions-based legacy dispatch
// helper was deleted once handleGitPanelKey's cutover to the registry left
// it with no production call sites -- see #511 PR 1/2).
func TestKeymapPanels_GitPanel_DefaultParity_AllBuiltinActionsMatchLegacyDispatch(t *testing.T) {
	want := config.DefaultGitActions()

	for _, key := range gitPanelBuiltinOrder {
		t.Run(key, func(t *testing.T) {
			wantAction, ok := want[key]
			if !ok {
				t.Fatalf("config.DefaultGitActions() has no entry for key %q (gitPanelBuiltinOrder)", key)
			}

			b, fe := newGitPanelTestBoard(t, nil, nil)
			b = sendKey(t, b, keyMsg("G"))
			binding, ok := b.panelBinding(keymap.ModeGitPanel, keyMsg(key))
			if !ok || binding.Kind != keymap.BindingAction {
				t.Fatalf("panelBinding(ModeGitPanel, %q) = (%+v, %v), want a resolved BindingAction", key, binding, ok)
			}

			gotAction := configActionFromKeymap(binding.Action)
			if gotAction.Name != wantAction.Name || gotAction.Type != wantAction.Type || gotAction.Command != wantAction.Command {
				t.Errorf("panelBinding(ModeGitPanel, %q) action = %+v, want %+v (config.DefaultGitActions())", key, gotAction, wantAction)
			}

			m, cmd := b.dispatchGitMenuAction(binding.Action)
			b = m.(Board)
			if cmd == nil {
				t.Fatal("dispatchGitMenuAction should return a non-nil cmd")
			}
			execCmds(cmd)

			if b.mode != normalMode {
				t.Errorf("mode after dispatching %q = %v, want normalMode (menu closes on dispatch)", key, b.mode)
			}
			if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != wantAction.Command {
				t.Errorf("RunShellCalls for %q = %v, want first call to be %q", key, fe.RunShellCalls, wantAction.Command)
			}
		})
	}
}

// --- keymaps.git_panel override wins over a built-in ---

func TestKeymapPanels_GitPanel_OverrideWinsOverBuiltinAction(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeGitPanel: {
			"P": keymap.ActionBinding(keymap.Action{Name: "Custom Push", Type: "shell", Command: "git push --force-with-lease", Scope: "board"}),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("G"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
	}

	binding, ok := b.panelBinding(keymap.ModeGitPanel, keyMsg("P"))
	if !ok || binding.Action.Command != "git push --force-with-lease" {
		t.Fatalf(`panelBinding(ModeGitPanel, "P") = (%+v, %v), want the overridden action`, binding, ok)
	}

	items := b.gitPanelItemsFromKeymap()
	idx := -1
	for i, it := range items {
		if it.key == "P" {
			idx = i
		}
	}
	if idx == -1 {
		t.Fatal(`gitPanelItemsFromKeymap() missing the overridden "P" entry`)
	}
	if items[idx].name != "Custom Push" {
		t.Errorf("items[%d].name = %q, want the overridden action's Name %q", idx, items[idx].name, "Custom Push")
	}

	m, cmd := b.dispatchGitMenuAction(binding.Action)
	b = m.(Board)
	if cmd == nil {
		t.Fatal("dispatchGitMenuAction should return a non-nil cmd")
	}
	execCmds(cmd)
	if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != "git push --force-with-lease" {
		t.Errorf("RunShellCalls = %v, want the overridden command to run instead of the default %q", fe.RunShellCalls, "git push")
	}
}

// --- A multi-key-bound action stays in the menu, but with a blank key label ---

// TestKeymapPanels_GitPanel_MultiKeySequenceItemKeepsRowBlanksKeyLabel
// asserts gitPanelItemsFromKeymap's treatment of a keymaps.git_panel entry
// bound to a multi-key sequence (e.g. "g d"): the row must stay in the item
// list -- it's still reachable via j/k navigation + Enter, since
// runGitPanelCommand's CommandGitPanelRun dispatches whatever the cursor is
// on, not a re-lookup by key -- but its displayed key label must be blank,
// since panelBinding can never resolve that sequence directly (single-key
// exact match only). This mirrors bestPanelHintKey's multi-key filtering for
// the hint bar.
func TestKeymapPanels_GitPanel_MultiKeySequenceItemKeepsRowBlanksKeyLabel(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeGitPanel: {
			"g d": keymap.ActionBinding(keymap.Action{Name: "Custom Diff", Type: "shell", Command: "git diff", Scope: "board"}),
		},
	}, nil)
	b = sendKey(t, b, keyMsg("G"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
	}

	items := b.gitPanelItemsFromKeymap()
	idx := -1
	for i, it := range items {
		if it.name == "Custom Diff" {
			idx = i
		}
	}
	if idx == -1 {
		t.Fatal(`gitPanelItemsFromKeymap() missing the "g d"-bound "Custom Diff" entry -- multi-key-bound actions must stay in the list`)
	}
	if items[idx].key != "" {
		t.Errorf(`items[%d].key = %q, want "" (a multi-key sequence must not be advertised as a directly-pressable key)`, idx, items[idx].key)
	}

	// Still reachable via j/k navigation + Enter.
	b.gitPanel.items = items
	b.gitPanel.cursor = idx
	m, cmd := b.runGitPanelCommand(keymap.CommandGitPanelRun)
	b = m.(Board)
	if cmd == nil {
		t.Fatal("CommandGitPanelRun should return a non-nil cmd")
	}
	execCmds(cmd)
	if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != "git diff" {
		t.Errorf("RunShellCalls = %v, want the multi-key-bound command to still run via cursor+Enter", fe.RunShellCalls)
	}
}

// --- Explicit unbind: no-op AND removed from the menu ---

func TestKeymapPanels_GitPanel_UnbindIsNoOpAndRemovesMenuRow(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeGitPanel: {"P": keymap.UnboundBinding()},
	}, nil)
	b = sendKey(t, b, keyMsg("G"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
	}

	if _, ok := b.panelBinding(keymap.ModeGitPanel, keyMsg("P")); ok {
		t.Error(`panelBinding(ModeGitPanel, "P") resolved after an explicit unbind, want not found`)
	}

	for _, it := range b.gitPanelItemsFromKeymap() {
		if it.key == "P" {
			t.Fatalf(`gitPanelItemsFromKeymap() still includes the unbound key "P": %+v`, it)
		}
	}

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("unbind must not have dispatched anything, got RunShellCalls = %v", fe.RunShellCalls)
	}
}

// --- Hint <-> dispatch invariant (the ticket's named risk) ---

// TestKeymapPanels_GitPanel_HintKeysAlwaysDispatch asserts, for every hint
// shown in the git panel's derived hint bar, that splitting Hint.Key on "/"
// and looking each resulting key up against ModeGitPanel always resolves --
// a hint bar must never advertise a key that silently no-ops. Run against
// both the default table and a remapped/unbound table, so a stale-hint
// regression introduced by either path is caught.
func TestKeymapPanels_GitPanel_HintKeysAlwaysDispatch(t *testing.T) {
	tests := []struct {
		name  string
		modes map[keymap.Mode]keymap.Table
	}{
		{name: "default table", modes: nil},
		{
			name: "remapped and unbound table",
			modes: map[keymap.Mode]keymap.Table{
				keymap.ModeGitPanel: {
					"esc": keymap.UnboundBinding(),
					"q":   keymap.CommandBinding(keymap.CommandGitPanelClose),
					"P":   keymap.UnboundBinding(),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, _ := newGitPanelTestBoard(t, nil, nil)
			if tc.modes != nil {
				b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			}
			b = sendKey(t, b, keyMsg("G"))
			if b.mode != gitPanelMode {
				t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
			}

			hints := b.gitPanelHints()
			if len(hints) == 0 {
				t.Fatal("gitPanelHints() returned no hints")
			}
			for _, h := range hints {
				for _, key := range strings.Split(h.Key, "/") {
					result := b.keys.Lookup(keymap.ModeGitPanel, "", keymap.Sequence{keymap.Key(key)})
					if result.Outcome != keymap.OutcomeMatch {
						t.Errorf("hint %+v: Lookup(ModeGitPanel, %q) outcome = %v, want OutcomeMatch (every rendered hint key must actually dispatch)", h, key, result.Outcome)
					}
				}
			}
		})
	}
}

// --- Untrusted action-name sanitization in the menu view ---

// TestKeymapPanels_GitPanel_ActionNameWithControlBytesSanitizedInView mirrors
// viewAssignModal's sanitizeSingleLine(item.login) pattern: a
// keymaps.git_panel action whose Name carries raw control/ANSI bytes must
// render sanitized in viewGitPanelModal, not leak them into the terminal.
func TestKeymapPanels_GitPanel_ActionNameWithControlBytesSanitizedInView(t *testing.T) {
	rawName := "Evil\x1b[31mPush\x07"
	b, _ := newGitPanelTestBoard(t, nil, nil)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeGitPanel: {
			"P": keymap.ActionBinding(keymap.Action{Name: rawName, Type: "shell", Command: "git push", Scope: "board"}),
		},
	}, nil)

	b.gitPanel = gitPanelState{items: b.gitPanelItemsFromKeymap(), cursor: 0}
	b.mode = gitPanelMode

	view := b.View()

	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("View() = %q, want no ESC (0x1b) byte", view)
	}
	if strings.ContainsRune(view, '\x07') {
		t.Errorf("View() = %q, want no BEL (0x07) byte", view)
	}
	want := sanitizeSingleLine(rawName)
	if !strings.Contains(view, want) {
		t.Errorf("View() = %q, want sanitized action name %q present", view, want)
	}
}

// --- Route the dispatch modal and help modal through the registry (#511, PR 2/2) ---
//
// This is the RED step: it targets an API surface that does not exist yet --
// b.runDispatchCommand, b.handleDispatchModalKey, b.dispatchModalHints,
// b.helpHints and b.runHelpCommand -- so this whole test file (and thus the
// package) is expected to fail to compile until keymap_panels.go grows these
// building blocks (a separate, later delegation). handleDispatchModeKey and
// handleHelpModeKey (mode_handlers.go) stay untouched in this delegation, so
// every test below drives the new building blocks directly rather than
// through b.Update()/sendKey -- mirroring how PR 1/2's git panel tests call
// b.runGitPanelCommand/b.dispatchGitMenuAction directly. Per
// .claude/rules/testing.md's "Parity Tests During Refactoring" rule, default-
// parity assertions compare against literal fixture data transcribed from
// today's production code (dispatchModalHints*/helpModalHints* below,
// mirroring gitPanelModeHints in git_keymap_defaults_test.go), never against
// the legacy handleDispatchModeKey/handleHelpModeKey functions themselves.
//
// assertHintsEqual is the shared comparison helper for the literal-fixture
// hint assertions below.
func assertHintsEqual(t *testing.T, got, want []Hint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("hints = %+v, want %+v (length mismatch)", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("hints[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// --- Dispatch modal: panelBinding default-table resolution ---

func TestKeymapPanels_Dispatch_PanelBinding_DefaultKeysResolveToExpectedCommands(t *testing.T) {
	b := newDispatchTestBoard(t)

	cases := []struct {
		msg  tea.KeyMsg
		want keymap.CommandID
	}{
		{arrowMsg(tea.KeyEsc), keymap.CommandDispatchClose},
		{arrowMsg(tea.KeyEnter), keymap.CommandDispatchToggleEnroll},
		{keyMsg("o"), keymap.CommandDispatchOnce},
		{keyMsg("l"), keymap.CommandDispatchToggleLoop},
		{keyMsg("y"), keymap.CommandDispatchConfirmLoop},
		{keyMsg("n"), keymap.CommandDispatchCancelLoop},
	}
	for _, tc := range cases {
		binding, ok := b.panelBinding(keymap.ModeDispatch, tc.msg)
		if !ok {
			t.Errorf("panelBinding(ModeDispatch, %q) = not found, want command %v", tc.msg.String(), tc.want)
			continue
		}
		if binding.Kind != keymap.BindingCommand || binding.Command != tc.want {
			t.Errorf("panelBinding(ModeDispatch, %q) = %+v, want command %v", tc.msg.String(), binding, tc.want)
		}
	}
}

// --- Dispatch modal: runDispatchCommand normal-state branch ---

func TestKeymapPanels_Dispatch_RunDispatchCommand_Close(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchClose)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("runDispatchCommand returned %T, want Board", m)
	}
	if b2.mode != normalMode {
		t.Errorf("mode after CommandDispatchClose = %v, want normalMode", b2.mode)
	}
	if cmd != nil {
		t.Error("CommandDispatchClose should not fire a Cmd")
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_ToggleEnroll_NoopWhenRepoEmpty(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchToggleEnroll)
	b2 := m.(Board)
	if b2.dispatch.loading {
		t.Error("expected dispatch.loading to remain false when dispatch.repo is empty")
	}
	if cmd != nil {
		t.Error("expected a nil Cmd when dispatch.repo is empty")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("expected no RunShellOutput calls, got %v", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_ToggleEnroll_FiresWhenIdleAndRepoSet(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: false}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchToggleEnroll)
	b2 := m.(Board)
	if !b2.dispatch.loading {
		t.Error("expected dispatch.loading=true after firing the enroll toggle")
	}
	if cmd == nil {
		t.Fatal("expected a non-nil Cmd")
	}
	execCmds(cmd)
	if len(fe.RunShellOutputCalls) == 0 || !strings.Contains(fe.RunShellOutputCalls[0], "dispatch enroll --dir") {
		t.Errorf("RunShellOutputCalls = %v, want an enroll call", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_ToggleEnroll_NoopWhileBusy(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state dispatchState
	}{
		{"loading", dispatchState{repo: "owner/repo", loading: true}},
		{"running", dispatchState{repo: "owner/repo", running: true}},
		{"error", dispatchState{repo: "owner/repo", err: "boom"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fe := &action.FakeExecutor{}
			b := newDispatchTestBoardWithExecutor(t, fe)
			b.mode = dispatchMode
			b.dispatch = tc.state

			_, cmd := b.runDispatchCommand(keymap.CommandDispatchToggleEnroll)
			if cmd != nil {
				t.Errorf("ToggleEnroll while %s should be a no-op", tc.name)
			}
			if len(fe.RunShellOutputCalls) != 0 {
				t.Errorf("ToggleEnroll while %s must not run any command, got %v", tc.name, fe.RunShellOutputCalls)
			}
		})
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_Once_NoopWhenNotEnrolled(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: false}

	_, cmd := b.runDispatchCommand(keymap.CommandDispatchOnce)
	if cmd != nil {
		t.Error("expected Once to be a no-op when not enrolled")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("expected no RunShellOutput calls, got %v", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_Once_FiresWhenEnrolledAndIdle(t *testing.T) {
	fe := &action.FakeExecutor{RunShellOutputStdout: "owner/repo#1 dispatch session-a"}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchOnce)
	b2 := m.(Board)
	if !b2.dispatch.running {
		t.Error("expected dispatch.running=true")
	}
	if cmd == nil {
		t.Fatal("expected a non-nil Cmd")
	}
	execCmds(cmd)
	if len(fe.RunShellOutputCalls) == 0 || strings.Contains(fe.RunShellOutputCalls[0], "--dir") {
		t.Errorf("dispatch-once must be fleet-wide (no --dir filter), got %v", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_ToggleLoop_EntersConfirmWhenLoopKnown(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: false}}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchToggleLoop)
	b2 := m.(Board)
	if !b2.dispatch.confirmingLoop {
		t.Error("expected dispatch.confirmingLoop=true")
	}
	if cmd != nil {
		t.Error("ToggleLoop should only open the confirm prompt, not fire a Cmd")
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_ToggleLoop_NoopWhenLoopUnknown(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: nil}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchToggleLoop)
	b2 := m.(Board)
	if b2.dispatch.confirmingLoop {
		t.Error("ToggleLoop with unknown loop state must NOT open the confirm prompt")
	}
	if cmd != nil {
		t.Error("ToggleLoop with unknown loop state should be a no-op")
	}
}

// TestKeymapPanels_Dispatch_ToggleLoop_NoopWhenRepoEmpty is the fleet-wide
// mutation's repo=="" guard (docs/cenciwatch-integration.md's blast-radius
// rule, mirroring CommandDispatchToggleEnroll's existing repo=="" guard):
// viewDispatchModal never renders the loop-toggle affordance or the
// blast-radius confirm text in a repo-less state, so 'l' then 'y' must not
// silently start/commit the fleet-wide loop toggle with no visible warning
// on screen. Exercised through handleDispatchModalKey (not runDispatchCommand
// directly) so the full key-press flow -- including the confirmingLoop
// commit step -- is covered end to end.
func TestKeymapPanels_Dispatch_ToggleLoop_NoopWhenRepoEmpty(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "", loop: &cenciwatch.DispatchState{Enabled: false}}

	m, cmd := b.handleDispatchModalKey(keyMsg("l"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("handleDispatchModalKey returned %T, want Board", m)
	}
	if b2.dispatch.confirmingLoop {
		t.Error("'l' with repo==\"\" must not open the confirm prompt")
	}
	if cmd != nil {
		t.Error("'l' with repo==\"\" should be a no-op")
	}

	m, cmd = b2.handleDispatchModalKey(keyMsg("y"))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("handleDispatchModalKey returned %T, want Board", m)
	}
	if b3.dispatch.confirmingLoop {
		t.Error("'y' after a no-op 'l' with repo==\"\" must not leave confirmingLoop set")
	}
	if cmd != nil {
		t.Error("'y' after a no-op 'l' with repo==\"\" should be a no-op")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("'l' then 'y' with repo==\"\" must not run any command, got %v", fe.RunShellOutputCalls)
	}
}

// TestKeymapPanels_Dispatch_RunDispatchCommand_ConfirmLoop_NoopWhenRepoEmpty
// is runDispatchCommand's defense-in-depth guard directly: even if
// confirmingLoop were somehow set without a repo, CommandDispatchConfirmLoop
// must still refuse to fire the fleet-wide toggle.
func TestKeymapPanels_Dispatch_RunDispatchCommand_ConfirmLoop_NoopWhenRepoEmpty(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "", loop: &cenciwatch.DispatchState{Enabled: false}, confirmingLoop: true}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchConfirmLoop)
	b2 := m.(Board)
	if b2.dispatch.confirmingLoop {
		t.Error("ConfirmLoop with repo==\"\" should still clear confirmingLoop (it consumes the confirm step)")
	}
	if cmd != nil {
		t.Error("ConfirmLoop with repo==\"\" must not fire the loop-toggle Cmd")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("ConfirmLoop with repo==\"\" must not run any command, got %v", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_ToggleLoop_NoopWhileBusy(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state dispatchState
	}{
		{"loading", dispatchState{repo: "owner/repo", loading: true, loop: &cenciwatch.DispatchState{}}},
		{"running", dispatchState{repo: "owner/repo", running: true, loop: &cenciwatch.DispatchState{}}},
		{"error", dispatchState{repo: "owner/repo", err: "boom", loop: &cenciwatch.DispatchState{}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newDispatchTestBoard(t)
			b.mode = dispatchMode
			b.dispatch = tc.state

			m, cmd := b.runDispatchCommand(keymap.CommandDispatchToggleLoop)
			b2 := m.(Board)
			if b2.dispatch.confirmingLoop {
				t.Errorf("ToggleLoop while %s must not open the confirm prompt", tc.name)
			}
			if cmd != nil {
				t.Errorf("ToggleLoop while %s should be a no-op", tc.name)
			}
		})
	}
}

// --- Dispatch modal: runDispatchCommand confirmingLoop branch ---

func TestKeymapPanels_Dispatch_RunDispatchCommand_ConfirmLoop_NoopOutsideConfirm(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: false}

	_, cmd := b.runDispatchCommand(keymap.CommandDispatchConfirmLoop)
	if cmd != nil {
		t.Error("CommandDispatchConfirmLoop outside confirmingLoop must be a no-op")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("expected no RunShellOutput calls, got %v", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_ConfirmLoop_TogglesOffWhenEnabled(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: true}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchConfirmLoop)
	b2 := m.(Board)
	if b2.dispatch.confirmingLoop {
		t.Error("confirming should clear dispatch.confirmingLoop")
	}
	if !b2.dispatch.loading {
		t.Error("expected dispatch.loading=true after confirming a loop toggle")
	}
	if cmd == nil {
		t.Fatal("expected a non-nil Cmd")
	}
	execCmds(cmd)
	if len(fe.RunShellOutputCalls) == 0 || !strings.Contains(fe.RunShellOutputCalls[0], "dispatch loop off") {
		t.Errorf("loop enabled -> confirm should turn it OFF, got %v", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_ConfirmLoop_TogglesOnWhenDisabled(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: false}, confirmingLoop: true}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchConfirmLoop)
	b2 := m.(Board)
	if !b2.dispatch.loading {
		t.Error("expected dispatch.loading=true after confirming a loop toggle")
	}
	if cmd == nil {
		t.Fatal("expected a non-nil Cmd")
	}
	execCmds(cmd)
	if len(fe.RunShellOutputCalls) == 0 || !strings.Contains(fe.RunShellOutputCalls[0], "dispatch loop on") {
		t.Errorf("loop disabled -> confirm should turn it ON, got %v", fe.RunShellOutputCalls)
	}
}

// TestKeymapPanels_Dispatch_RunDispatchCommand_ConfirmLoop_NoopWhileBusy is the
// explicit risk-coverage test for docs/view-state-consistency.md's commit-step
// re-check rule: a busy/error state can arrive between opening the confirm
// (gated on !busy) and confirming, so ConfirmLoop must repeat that same guard
// -- not just the loop==nil defense -- before firing toggleLoopCmd.
func TestKeymapPanels_Dispatch_RunDispatchCommand_ConfirmLoop_NoopWhileBusy(t *testing.T) {
	for _, tc := range []struct {
		name  string
		state dispatchState
	}{
		{"loading", dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: true, loading: true}},
		{"running", dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: true, running: true}},
		{"error", dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: true, err: "boom"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fe := &action.FakeExecutor{}
			b := newDispatchTestBoardWithExecutor(t, fe)
			b.mode = dispatchMode
			b.dispatch = tc.state

			m, cmd := b.runDispatchCommand(keymap.CommandDispatchConfirmLoop)
			b2 := m.(Board)
			if b2.dispatch.confirmingLoop {
				t.Errorf("confirming while %s should still dismiss the confirm", tc.name)
			}
			if cmd != nil {
				t.Errorf("confirming while %s must not fire the toggle Cmd", tc.name)
			}
			if len(fe.RunShellOutputCalls) != 0 {
				t.Errorf("confirming while %s must not run any command, got %v", tc.name, fe.RunShellOutputCalls)
			}
		})
	}
}

// TestKeymapPanels_Dispatch_RunDispatchCommand_ConfirmLoop_NoopWhenLoopNil is
// the explicit risk-coverage test for the loop==nil defense staying intact:
// the live state can go unknown between opening the confirm and confirming.
func TestKeymapPanels_Dispatch_RunDispatchCommand_ConfirmLoop_NoopWhenLoopNil(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: nil, confirmingLoop: true}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchConfirmLoop)
	b2 := m.(Board)
	if b2.dispatch.confirmingLoop {
		t.Error("confirming should still dismiss the confirm even when loop is nil")
	}
	if cmd != nil {
		t.Error("confirming with a nil loop must not fire a Cmd")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("expected no RunShellOutput calls, got %v", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_CancelLoop_ClearsConfirmNoCmd(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: true}

	m, cmd := b.runDispatchCommand(keymap.CommandDispatchCancelLoop)
	b2 := m.(Board)
	if b2.dispatch.confirmingLoop {
		t.Error("CancelLoop should clear dispatch.confirmingLoop")
	}
	if b2.mode != dispatchMode {
		t.Errorf("CancelLoop should keep the dispatch modal open, got mode %v", b2.mode)
	}
	if cmd != nil {
		t.Error("cancelling should not fire a Cmd")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("cancelling must not run any command, got %v", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_RunDispatchCommand_CancelLoop_NoopOutsideConfirm(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, confirmingLoop: false}

	_, cmd := b.runDispatchCommand(keymap.CommandDispatchCancelLoop)
	if cmd != nil {
		t.Error("CommandDispatchCancelLoop outside confirmingLoop must be a no-op")
	}
}

// --- Dispatch modal: the esc-during-confirm regression risk (docs/view-state-consistency.md) ---
//
// handleDispatchModalKey is the yet-to-be-built key router structurally
// parallel to handleGitPanelKey (PR 1/2): it special-cases Esc while
// confirmingLoop (cancel the confirm ONLY, never fall through to the table's
// esc->CommandDispatchClose binding) before consulting panelBinding, then
// dispatches a resolved BindingCommand through runDispatchCommand.

func TestKeymapPanels_Dispatch_EscDuringConfirm_CancelsConfirmOnly_KeepsModalOpen(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: true}

	m, cmd := b.handleDispatchModalKey(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("handleDispatchModalKey returned %T, want Board", m)
	}
	if b2.dispatch.confirmingLoop {
		t.Error("Esc during confirmingLoop should clear confirmingLoop")
	}
	if b2.mode != dispatchMode {
		t.Errorf("Esc during confirm must keep the dispatch modal open (cancel the confirm ONLY), got mode %v", b2.mode)
	}
	if cmd != nil {
		t.Error("cancelling the confirm with Esc should not fire a Cmd")
	}
	if len(fe.RunShellOutputCalls) != 0 {
		t.Errorf("Esc during confirm must not run any command, got %v", fe.RunShellOutputCalls)
	}
}

func TestKeymapPanels_Dispatch_EscOutsideConfirm_ClosesModal(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true}

	m, cmd := b.handleDispatchModalKey(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("handleDispatchModalKey returned %T, want Board", m)
	}
	if b2.mode != normalMode {
		t.Errorf("Esc outside confirm should close the modal, got mode %v", b2.mode)
	}
	if cmd != nil {
		t.Error("closing should not fire a Cmd")
	}
}

// --- Dispatch modal: remapped y/n still drive the confirm flow ---

func TestKeymapPanels_Dispatch_RemappedConfirmCancelKeys_StillDriveConfirmFlow(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDispatch: {
			"y": keymap.UnboundBinding(),
			"n": keymap.UnboundBinding(),
			"c": keymap.CommandBinding(keymap.CommandDispatchConfirmLoop),
			"x": keymap.CommandBinding(keymap.CommandDispatchCancelLoop),
		},
	}, nil)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: false}, confirmingLoop: true}

	// The old default 'y' must now no-op (explicitly unbound).
	m, cmd := b.handleDispatchModalKey(keyMsg("y"))
	b2 := m.(Board)
	if !b2.dispatch.confirmingLoop {
		t.Error("unbound 'y' must not affect confirmingLoop")
	}
	if cmd != nil {
		t.Error("unbound 'y' must not fire a Cmd")
	}

	// The remapped 'c' key should confirm the toggle.
	m, cmd = b2.handleDispatchModalKey(keyMsg("c"))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("handleDispatchModalKey returned %T, want Board", m)
	}
	if b3.dispatch.confirmingLoop {
		t.Error("remapped 'c' should clear confirmingLoop")
	}
	if cmd == nil {
		t.Fatal("remapped 'c' should fire the loop-toggle Cmd")
	}
	execCmds(cmd)
	if len(fe.RunShellOutputCalls) == 0 || !strings.Contains(fe.RunShellOutputCalls[0], "dispatch loop on") {
		t.Errorf("RunShellOutputCalls = %v, want a loop-on call", fe.RunShellOutputCalls)
	}
}

// --- Dispatch modal: a keymaps.dispatch override wins over a normal-state built-in ---

func TestKeymapPanels_Dispatch_OverrideWinsOverBuiltinNormalStateAction(t *testing.T) {
	fe := &action.FakeExecutor{}
	b := newDispatchTestBoardWithExecutor(t, fe)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDispatch: {
			"enter": keymap.UnboundBinding(),
			"x":     keymap.CommandBinding(keymap.CommandDispatchToggleEnroll),
		},
	}, nil)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: false}

	// The old default 'enter' must now no-op.
	m, cmd := b.handleDispatchModalKey(arrowMsg(tea.KeyEnter))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("handleDispatchModalKey returned %T, want Board", m)
	}
	if b2.dispatch.loading {
		t.Error("unbound 'enter' must not fire the enroll toggle")
	}
	if cmd != nil {
		t.Error("unbound 'enter' must not fire a Cmd")
	}

	// The remapped 'x' key fires the enroll toggle instead.
	m, cmd = b2.handleDispatchModalKey(keyMsg("x"))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("handleDispatchModalKey returned %T, want Board", m)
	}
	if !b3.dispatch.loading {
		t.Error("remapped 'x' should fire the enroll toggle")
	}
	if cmd == nil {
		t.Fatal("remapped 'x' should fire a Cmd")
	}
}

// --- Dispatch modal: dispatchModalHints per-state matrix ---
//
// The literal fixtures below are transcribed verbatim from viewDispatchModal's
// pre-#511 inline hint construction (view.go) -- the default-parity oracle,
// mirroring gitPanelModeHints (git_keymap_defaults_test.go), never the legacy
// handleDispatchModeKey/viewDispatchModal functions themselves.
var dispatchModalHintsBusy = []Hint{{Key: "esc", Desc: "Close"}}
var dispatchModalHintsConfirming = []Hint{{Key: "y", Desc: "Confirm"}, {Key: "n/esc", Desc: "Cancel"}}
var dispatchModalHintsNotEnrolled = []Hint{{Key: "esc", Desc: "Close"}, {Key: "enter", Desc: "Enroll"}}
var dispatchModalHintsEnrolled = []Hint{{Key: "esc", Desc: "Close"}, {Key: "enter", Desc: "Unenroll"}, {Key: "o", Desc: "Dispatch once"}}

func TestKeymapPanels_Dispatch_DispatchModalHints_Loading(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.dispatch = dispatchState{loading: true}

	assertHintsEqual(t, b.dispatchModalHints(), dispatchModalHintsBusy)
}

func TestKeymapPanels_Dispatch_DispatchModalHints_Running(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.dispatch = dispatchState{running: true, repo: "owner/repo"}

	assertHintsEqual(t, b.dispatchModalHints(), dispatchModalHintsBusy)
}

func TestKeymapPanels_Dispatch_DispatchModalHints_Error(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.dispatch = dispatchState{err: "boom", repo: "owner/repo"}

	assertHintsEqual(t, b.dispatchModalHints(), dispatchModalHintsBusy)
}

func TestKeymapPanels_Dispatch_DispatchModalHints_NoRepo(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.dispatch = dispatchState{}

	assertHintsEqual(t, b.dispatchModalHints(), dispatchModalHintsBusy)
}

func TestKeymapPanels_Dispatch_DispatchModalHints_Confirming(t *testing.T) {
	b := newDispatchTestBoard(t)
	loop := &cenciwatch.DispatchState{Enabled: false}
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: loop, confirmingLoop: true}

	assertHintsEqual(t, b.dispatchModalHints(), dispatchModalHintsConfirming)
}

func TestKeymapPanels_Dispatch_DispatchModalHints_NotEnrolled(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: false}

	assertHintsEqual(t, b.dispatchModalHints(), dispatchModalHintsNotEnrolled)
}

func TestKeymapPanels_Dispatch_DispatchModalHints_Enrolled(t *testing.T) {
	b := newDispatchTestBoard(t)
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true}

	assertHintsEqual(t, b.dispatchModalHints(), dispatchModalHintsEnrolled)
}

// TestKeymapPanels_Dispatch_DispatchModalHints_UnboundOnceKeyOmitsHint mirrors
// viewDispatchModal's existing "l:" loop-line omission (loop == nil hides the
// affordance rather than rendering a dead key): when CommandDispatchOnce has
// no bound key left, dispatchModalHints must omit the "Dispatch once" hint
// entirely rather than render it with a blank Key -- a hint bar must never
// advertise a key that silently no-ops.
func TestKeymapPanels_Dispatch_DispatchModalHints_UnboundOnceKeyOmitsHint(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDispatch: {"o": keymap.UnboundBinding()},
	}, nil)
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true}

	got := b.dispatchModalHints()
	for _, h := range got {
		if h.Desc == "Dispatch once" {
			t.Errorf("dispatchModalHints() = %+v, want the 'Dispatch once' hint omitted when its key is unbound", got)
		}
	}
}

// --- Dispatch modal: hint<->dispatch invariant (the ticket's named risk) ---

func TestKeymapPanels_Dispatch_HintKeysAlwaysDispatch(t *testing.T) {
	tests := []struct {
		name  string
		modes map[keymap.Mode]keymap.Table
	}{
		{name: "default table", modes: nil},
		{
			name: "remapped and unbound table",
			modes: map[keymap.Mode]keymap.Table{
				keymap.ModeDispatch: {
					"enter": keymap.UnboundBinding(),
					"u":     keymap.CommandBinding(keymap.CommandDispatchToggleEnroll),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newDispatchTestBoard(t)
			if tc.modes != nil {
				b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			}
			b.mode = dispatchMode

			for _, state := range []dispatchState{
				{loading: true},
				{repo: "owner/repo", enrolled: false},
				{repo: "owner/repo", enrolled: true},
				{repo: "owner/repo", enrolled: true, confirmingLoop: true, loop: &cenciwatch.DispatchState{Enabled: true}},
			} {
				b.dispatch = state
				hints := b.dispatchModalHints()
				if len(hints) == 0 {
					t.Fatalf("state=%+v: dispatchModalHints() returned no hints", state)
				}
				for _, h := range hints {
					for _, key := range strings.Split(h.Key, "/") {
						if key == "" {
							t.Errorf("state=%+v hint %+v: Hint.Key contains an empty key segment -- a hint must never advertise a blank-key affordance", state, h)
							continue
						}
						result := b.keys.Lookup(keymap.ModeDispatch, "", keymap.Sequence{keymap.Key(key)})
						if result.Outcome != keymap.OutcomeMatch {
							t.Errorf("state=%+v hint %+v: Lookup(ModeDispatch, %q) outcome = %v, want OutcomeMatch", state, h, key, result.Outcome)
						}
					}
				}
			}
		})
	}
}

// TestKeymapPanels_Dispatch_ConfirmingLoop_CloseHintKey_CancelsConfirmOnly is the
// real behavioral counterpart to the Lookup-resolution check above: the
// "Cancel" hint dispatchModalHints renders while confirmingLoop composites
// BOTH CommandDispatchCancelLoop and CommandDispatchClose into one key
// (panelHintKey(entries, CommandDispatchCancelLoop, CommandDispatchClose)).
// Under the default table that key is literal Escape (already covered by
// TestKeymapPanels_Dispatch_EscDuringConfirm_CancelsConfirmOnly_KeepsModalOpen),
// but under a table where a user's keymaps.dispatch config rebinds
// dispatch.close off Escape onto another key, that OTHER key must still
// cancel the confirm only -- not close the whole modal -- or the hint would
// advertise an undispatchable key (the ticket's named regression risk).
func TestKeymapPanels_Dispatch_ConfirmingLoop_CloseHintKey_CancelsConfirmOnly(t *testing.T) {
	tests := []struct {
		name    string
		modes   map[keymap.Mode]keymap.Table
		msg     tea.KeyMsg
		hintKey string
	}{
		{name: "default table (literal esc)", modes: nil, msg: arrowMsg(tea.KeyEsc), hintKey: "esc"},
		{
			name: "remapped table (close rebound off esc onto 'x')",
			modes: map[keymap.Mode]keymap.Table{
				keymap.ModeDispatch: {
					"esc": keymap.UnboundBinding(),
					"x":   keymap.CommandBinding(keymap.CommandDispatchClose),
				},
			},
			msg:     keyMsg("x"),
			hintKey: "x",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fe := &action.FakeExecutor{}
			b := newDispatchTestBoardWithExecutor(t, fe)
			if tc.modes != nil {
				b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			}
			b.mode = dispatchMode
			b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}, confirmingLoop: true}

			// The hint bar must actually advertise this key as the way to cancel.
			hints := b.dispatchModalHints()
			found := false
			for _, h := range hints {
				if h.Desc != "Cancel" {
					continue
				}
				for _, k := range strings.Split(h.Key, "/") {
					if k == tc.hintKey {
						found = true
					}
				}
			}
			if !found {
				t.Fatalf("dispatchModalHints() = %+v, want the Cancel hint to include key %q", hints, tc.hintKey)
			}

			m, cmd := b.handleDispatchModalKey(tc.msg)
			b2, ok := m.(Board)
			if !ok {
				t.Fatalf("handleDispatchModalKey returned %T, want Board", m)
			}
			if b2.dispatch.confirmingLoop {
				t.Errorf("key %q during confirmingLoop should clear confirmingLoop", tc.hintKey)
			}
			if b2.mode != dispatchMode {
				t.Errorf("key %q during confirm must keep the dispatch modal open (cancel the confirm ONLY), got mode %v", tc.hintKey, b2.mode)
			}
			if cmd != nil {
				t.Errorf("key %q cancelling the confirm should not fire a Cmd", tc.hintKey)
			}
			if len(fe.RunShellOutputCalls) != 0 {
				t.Errorf("key %q during confirm must not run any command, got %v", tc.hintKey, fe.RunShellOutputCalls)
			}
		})
	}
}

// --- Dispatch modal: the "l:" loop-toggle line derives its key from the registry ---

func TestKeymapPanels_Dispatch_LoopLine_OmittedWhenToggleLoopUnbound(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDispatch: {"l": keymap.UnboundBinding()},
	}, nil)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}}

	view := b.View()

	if strings.Contains(view, "Turn loop") {
		t.Errorf("dispatch view with CommandDispatchToggleLoop unbound must NOT show the loop-toggle line, even though loop != nil, got:\n%s", view)
	}
}

func TestKeymapPanels_Dispatch_LoopLine_KeyReflectsRemap(t *testing.T) {
	b := newDispatchTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDispatch: {"l": keymap.UnboundBinding(), "t": keymap.CommandBinding(keymap.CommandDispatchToggleLoop)},
	}, nil)
	b.mode = dispatchMode
	b.dispatch = dispatchState{repo: "owner/repo", enrolled: true, loop: &cenciwatch.DispatchState{Enabled: true}}

	view := b.View()

	if !strings.Contains(view, "t: Turn loop off") {
		t.Errorf("dispatch view should render the remapped key 't' in the loop-toggle line, got:\n%s", view)
	}
	if strings.Contains(view, "l: Turn loop") {
		t.Errorf("dispatch view should not still show the old default key 'l' in the loop-toggle line, got:\n%s", view)
	}
}

// --- Help modal: panelBinding default-table resolution ---

func TestKeymapPanels_Help_PanelBinding_DefaultKeysResolveToExpectedCommands(t *testing.T) {
	b := newLoadedTestBoard(t)

	cases := []struct {
		msg  tea.KeyMsg
		want keymap.CommandID
	}{
		{arrowMsg(tea.KeyEsc), keymap.CommandHelpClose},
		{keyMsg("?"), keymap.CommandHelpClose},
		{keyMsg("j"), keymap.CommandHelpScrollDown},
		{arrowMsg(tea.KeyDown), keymap.CommandHelpScrollDown},
		{keyMsg("k"), keymap.CommandHelpScrollUp},
		{arrowMsg(tea.KeyUp), keymap.CommandHelpScrollUp},
		{keyMsg("q"), keymap.CommandQuit},
	}
	for _, tc := range cases {
		binding, ok := b.panelBinding(keymap.ModeHelp, tc.msg)
		if !ok {
			t.Errorf("panelBinding(ModeHelp, %q) = not found, want command %v", tc.msg.String(), tc.want)
			continue
		}
		if binding.Kind != keymap.BindingCommand || binding.Command != tc.want {
			t.Errorf("panelBinding(ModeHelp, %q) = %+v, want command %v", tc.msg.String(), binding, tc.want)
		}
	}
}

// --- Help modal: runHelpCommand (mirrors runGitPanelCommand's verbatim-transcription pattern) ---

// TestKeymapPanels_Help_RunHelpCommand_Quit exercises the default-table 'q'
// key end-to-end through b.Update, entered the real way ('?' opens help
// mode -- never a raw b.mode assignment): CommandQuit is dispatched by the
// shared universalDispatch seam (keymap_dispatch.go) ahead of
// runHelpCommand, not by a `case keymap.CommandQuit:` inside runHelpCommand
// itself (removed by #589).
func TestKeymapPanels_Help_RunHelpCommand_Quit(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("?"))
	if b.mode != helpMode {
		t.Fatalf("precondition: mode = %d, want helpMode", b.mode)
	}

	m, cmd := b.Update(keyMsg("q"))
	if _, ok := m.(Board); !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if cmd == nil {
		t.Fatal("'q' in helpMode should return a non-nil Cmd (tea.Quit)")
	}
	if _, isQuit := cmd().(tea.QuitMsg); !isQuit {
		t.Fatal("'q' in helpMode should dispatch tea.Quit")
	}
}

func TestKeymapPanels_Help_RunHelpCommand_Close(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.mode = helpMode

	m, cmd := b.runHelpCommand(keymap.CommandHelpClose)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("runHelpCommand returned %T, want Board", m)
	}
	if b2.mode != normalMode {
		t.Errorf("mode after CommandHelpClose = %v, want normalMode", b2.mode)
	}
	if cmd != nil {
		t.Error("CommandHelpClose should not fire a Cmd")
	}
}

func TestKeymapPanels_Help_RunHelpCommand_ScrollDown_ClampsAtMaxOffset(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 20 // small height so help content exceeds the visible area
	b.mode = helpMode
	maxOffset := b.helpMaxScrollOffset()

	for i := 0; i < maxOffset+50; i++ {
		m, _ := b.runHelpCommand(keymap.CommandHelpScrollDown)
		b = m.(Board)
	}
	if b.helpScrollOffset != maxOffset {
		t.Errorf("helpScrollOffset = %d after excessive ScrollDown, want %d (clamped)", b.helpScrollOffset, maxOffset)
	}

	m, _ := b.runHelpCommand(keymap.CommandHelpScrollUp)
	b2 := m.(Board)
	if b2.helpScrollOffset != maxOffset-1 {
		t.Errorf("helpScrollOffset = %d after a single ScrollUp from max, want %d", b2.helpScrollOffset, maxOffset-1)
	}
}

func TestKeymapPanels_Help_RunHelpCommand_ScrollUp_ClampsAtZero(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.mode = helpMode

	m, _ := b.runHelpCommand(keymap.CommandHelpScrollUp)
	b2 := m.(Board)
	if b2.helpScrollOffset != 0 {
		t.Errorf("helpScrollOffset = %d after ScrollUp at offset 0, want 0", b2.helpScrollOffset)
	}
}

// --- Help modal: remap works, old default key becomes a no-op ---

func TestKeymapPanels_Help_RemappedCloseKeyWorks_OldDefaultKeyBecomesNoop(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeHelp: {
			"esc": keymap.UnboundBinding(),
			"?":   keymap.UnboundBinding(),
			"x":   keymap.CommandBinding(keymap.CommandHelpClose),
		},
	}, nil)
	b.mode = helpMode

	if _, ok := b.panelBinding(keymap.ModeHelp, arrowMsg(tea.KeyEsc)); ok {
		t.Error("panelBinding(ModeHelp, esc) resolved after an explicit unbind, want not found")
	}

	binding, ok := b.panelBinding(keymap.ModeHelp, keyMsg("x"))
	if !ok || binding.Kind != keymap.BindingCommand || binding.Command != keymap.CommandHelpClose {
		t.Fatalf("panelBinding(ModeHelp, x) = (%+v, %v), want CommandHelpClose", binding, ok)
	}

	m, cmd := b.runHelpCommand(binding.Command)
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("runHelpCommand returned %T, want Board", m)
	}
	if b2.mode != normalMode {
		t.Errorf("mode after remapped close = %v, want normalMode", b2.mode)
	}
	if cmd != nil {
		t.Error("close should not fire a Cmd")
	}
}

// --- Help modal: helpHints() default parity ---
//
// The literal fixture below is transcribed from helpModeHints (model.go),
// the pre-#511 hint bar -- except the Close hint drops "?" from "esc/?":
// panelHintKey (keymap_panels.go, shared with the git panel and reused
// as-is here) picks exactly one best display key per command id, and both
// "esc" and "?" resolve to the SAME CommandHelpClose id, so only the named
// key ("esc") is picked. Both keys still work (see the panelBinding test
// above); only the status-bar LABEL text changes for the default table.
var helpModalHintsDefault = []Hint{
	{Key: "esc", Desc: "Close"},
	{Key: "j/k", Desc: "Scroll"},
}

func TestKeymapPanels_Help_DefaultParity_Hints(t *testing.T) {
	b := newLoadedTestBoard(t)

	assertHintsEqual(t, b.helpHints(), helpModalHintsDefault)
}

// --- Help modal: hint<->dispatch invariant ---

func TestKeymapPanels_Help_HintKeysAlwaysDispatch(t *testing.T) {
	tests := []struct {
		name  string
		modes map[keymap.Mode]keymap.Table
	}{
		{name: "default table", modes: nil},
		{
			name: "remapped and unbound table",
			modes: map[keymap.Mode]keymap.Table{
				keymap.ModeHelp: {
					"esc":  keymap.UnboundBinding(),
					"?":    keymap.CommandBinding(keymap.CommandHelpClose),
					"down": keymap.UnboundBinding(),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newLoadedTestBoard(t)
			if tc.modes != nil {
				b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			}

			hints := b.helpHints()
			if len(hints) == 0 {
				t.Fatal("helpHints() returned no hints")
			}
			for _, h := range hints {
				for _, key := range strings.Split(h.Key, "/") {
					if key == "" {
						t.Errorf("hint %+v: Hint.Key contains an empty key segment -- a hint must never advertise a blank-key affordance", h)
						continue
					}
					result := b.keys.Lookup(keymap.ModeHelp, "", keymap.Sequence{keymap.Key(key)})
					if result.Outcome != keymap.OutcomeMatch {
						t.Errorf("hint %+v: Lookup(ModeHelp, %q) outcome = %v, want OutcomeMatch (every rendered hint key must actually dispatch)", h, key, result.Outcome)
					}
				}
			}
		})
	}
}

// --- Route errorMode through the registry (#557) ---
//
// This is the RED step: it targets an API surface that does not exist yet --
// b.errorHints, b.runErrorCommand -- so this file (and thus the package) is
// expected to fail to compile until keymap_panels.go grows these building
// blocks and mode_handlers.go grows handleErrorModeKey (a separate, later
// delegation). Every test below enters errorMode the real way (a
// boardFetchErrorMsg through b.Update, never a raw b.mode assignment) and
// dispatches through the real b.Update()/sendKey path -- unlike PR 2/2's
// dispatch/help-modal tests above, handleErrorModeKey is small enough (and
// errorMode's boardFetchErrorMsg entry point simple enough) to exercise
// end-to-end rather than by calling runErrorCommand directly.
//
// Per the plan's caveat: boardWithOverrideKeymap must be applied BEFORE
// sending boardFetchErrorMsg -- withKeymap (keymap_dispatch.go) rebuilds and
// re-sets normal hints, and applying it after entering errorMode would
// clobber whatever error hints boardFetchErrorMsg's handler set. errorMode
// is not in keymap's textInputModes, so remapping onto a printable letter
// (e.g. "x") is safe.

// enterErrorMode drives b through the real boardFetchErrorMsg transition
// into errorMode, mirroring model_test.go's TestLoading_FetchError_*
// pattern -- never a raw b.mode assignment.
func enterErrorMode(t *testing.T, b Board) Board {
	t.Helper()
	m, _ := b.Update(boardFetchErrorMsg{err: errors.New("fail")})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update(boardFetchErrorMsg) returned %T, want Board", m)
	}
	return updated
}

// --- Error mode: panelBinding default-table resolution ---

func TestKeymapPanels_Error_PanelBinding_DefaultKeysResolveToExpectedCommands(t *testing.T) {
	b := enterErrorMode(t, newTestBoard(t))

	cases := []struct {
		msg  tea.KeyMsg
		want keymap.CommandID
	}{
		{keyMsg("q"), keymap.CommandQuit},
		{keyMsg("r"), keymap.CommandErrorRetry},
	}
	for _, tc := range cases {
		binding, ok := b.panelBinding(keymap.ModeError, tc.msg)
		if !ok {
			t.Errorf("panelBinding(ModeError, %q) = not found, want command %v", tc.msg.String(), tc.want)
			continue
		}
		if binding.Kind != keymap.BindingCommand || binding.Command != tc.want {
			t.Errorf("panelBinding(ModeError, %q) = %+v, want command %v", tc.msg.String(), binding, tc.want)
		}
	}
}

// --- Error mode: errorHints() default parity ---
//
// The literal fixture below is transcribed from update.go's pre-#557 inline
// []Hint{} literal (SetActionHints call in the boardFetchErrorMsg handler).
var errorModalHintsDefault = []Hint{
	{Key: "r", Desc: "Retry"},
	{Key: "q", Desc: "Quit"},
}

func TestKeymapPanels_Error_DefaultParity_Hints(t *testing.T) {
	b := enterErrorMode(t, newTestBoard(t))

	assertHintsEqual(t, b.errorHints(), errorModalHintsDefault)
}

// --- Error mode: end-to-end default-key dispatch through b.Update ---

func TestKeymapPanels_Error_EndToEnd_RetryTransitionsToLoadingMode(t *testing.T) {
	b := enterErrorMode(t, newTestBoard(t))

	m, cmd := b.Update(keyMsg("r"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != loadingMode {
		t.Errorf("mode = %v after 'r' in errorMode, want loadingMode", b2.mode)
	}
	if cmd == nil {
		t.Error("'r' in errorMode should return a non-nil Cmd (spinner tick + fetch)")
	}
	if b2.loadErr != "" {
		t.Errorf("loadErr = %q after retry, want empty", b2.loadErr)
	}
}

func TestKeymapPanels_Error_EndToEnd_QuitReturnsQuitCmd(t *testing.T) {
	b := enterErrorMode(t, newTestBoard(t))

	_, cmd := b.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("'q' in errorMode should return a non-nil Cmd (tea.Quit)")
	}
}

// --- Error mode: remap/unbind ---

func TestKeymapPanels_Error_RemapRetryKey_OldDefaultKeyBecomesNoop(t *testing.T) {
	b := boardWithOverrideKeymap(t, newTestBoard(t), map[keymap.Mode]keymap.Table{
		keymap.ModeError: {
			"r": keymap.UnboundBinding(),
			"x": keymap.CommandBinding(keymap.CommandErrorRetry),
		},
	}, nil)
	b = enterErrorMode(t, b)

	m, _ := b.Update(keyMsg("r"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != errorMode {
		t.Errorf("mode = %v after pressing the explicitly unbound 'r', want unchanged errorMode", b2.mode)
	}

	m, cmd := b2.Update(keyMsg("x"))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.mode != loadingMode {
		t.Errorf("mode = %v after the remapped retry key 'x', want loadingMode", b3.mode)
	}
	if cmd == nil {
		t.Error("the remapped retry key should return a non-nil Cmd")
	}

	hints := b2.errorHints()
	foundNewKey := false
	for _, h := range hints {
		if strings.Contains(h.Key, "x") {
			foundNewKey = true
		}
		if strings.Contains(h.Key, "r") {
			t.Errorf("errorHints() Key = %q still advertises the unbound 'r', want it gone", h.Key)
		}
	}
	if !foundNewKey {
		t.Errorf("errorHints() = %+v, want a hint advertising the remapped key 'x'", hints)
	}
}

// --- Error mode: hint<->dispatch invariant ---

func TestKeymapPanels_Error_HintKeysAlwaysDispatch(t *testing.T) {
	tests := []struct {
		name  string
		modes map[keymap.Mode]keymap.Table
	}{
		{name: "default table", modes: nil},
		{
			name: "remapped and unbound table",
			modes: map[keymap.Mode]keymap.Table{
				keymap.ModeError: {
					"r": keymap.UnboundBinding(),
					"x": keymap.CommandBinding(keymap.CommandErrorRetry),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newTestBoard(t)
			if tc.modes != nil {
				b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			}
			b = enterErrorMode(t, b)

			hints := b.errorHints()
			if len(hints) == 0 {
				t.Fatal("errorHints() returned no hints")
			}
			for _, h := range hints {
				for _, key := range strings.Split(h.Key, "/") {
					if key == "" {
						t.Errorf("hint %+v: Hint.Key contains an empty key segment -- a hint must never advertise a blank-key affordance", h)
						continue
					}
					result := b.keys.Lookup(keymap.ModeError, "", keymap.Sequence{keymap.Key(key)})
					if result.Outcome != keymap.OutcomeMatch {
						t.Errorf("hint %+v: Lookup(ModeError, %q) outcome = %v, want OutcomeMatch (every rendered hint key must actually dispatch)", h, key, result.Outcome)
					}
				}
			}
		})
	}
}

// --- Error mode: negative cases (Q4 explicit-risk coverage) ---
//
// Table-driven to control line count, mirroring keymap_modals_test.go's
// filter-mode negative cases (:107-167): a pending-sequence prefix key, a
// command id valid elsewhere in the catalogue but foreign to errorMode, and
// an inline BindingAction (which errorMode has no dispatch path for) must
// all be silent no-ops.

func TestKeymapPanels_Error_PendingPrefixKeyIsNoOp(t *testing.T) {
	b := boardWithOverrideKeymap(t, newTestBoard(t), map[keymap.Mode]keymap.Table{
		keymap.ModeError: {"g x": keymap.CommandBinding(keymap.CommandErrorRetry)},
	}, nil)
	b = enterErrorMode(t, b)
	loadErrBefore := b.loadErr

	m, _ := b.Update(keyMsg("g"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != errorMode {
		t.Errorf("mode = %v after pressing a pending-sequence prefix key, want errorMode (OutcomePending must be a no-op)", b2.mode)
	}
	if b2.loadErr != loadErrBefore {
		t.Errorf("loadErr = %q after pressing a pending prefix key, want unchanged %q", b2.loadErr, loadErrBefore)
	}
}

func TestKeymapPanels_Error_ForeignCommandIDIsNoOp(t *testing.T) {
	b := boardWithOverrideKeymap(t, newTestBoard(t), map[keymap.Mode]keymap.Table{
		keymap.ModeError: {"z": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)
	b = enterErrorMode(t, b)

	m, _ := b.Update(keyMsg("z"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != errorMode {
		t.Errorf("mode = %v after pressing a key bound to an out-of-domain command, want errorMode", b2.mode)
	}
	if b2.refreshing {
		t.Error("refreshing = true after pressing a key bound to board.refresh inside errorMode, want false -- an unrecognized command id must not dispatch")
	}
}

func TestKeymapPanels_Error_InlineActionBindingDoesNotDispatch(t *testing.T) {
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, "Working", false, false, nil, nil, true)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeError: {"z": keymap.ActionBinding(keymap.Action{Name: "Pwn", Type: "shell", Command: "echo pwned", Scope: "board"})},
	}, nil)
	b = enterErrorMode(t, b)

	m, cmd := b.Update(keyMsg("z"))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShell called %d times after pressing a key bound to an inline shell action inside errorMode, want 0 -- errorMode has no inline-action dispatch path and must not fire it: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
	if b2.mode != errorMode {
		t.Errorf("mode = %v after pressing a key bound to an inline action inside errorMode, want unchanged errorMode", b2.mode)
	}
}

// --- Error mode: ctrl+c always quits, even under a fully overridden table ---

func TestKeymapPanels_Error_CtrlCQuits_EvenWithFullyOverriddenTable(t *testing.T) {
	b := boardWithOverrideKeymap(t, newTestBoard(t), map[keymap.Mode]keymap.Table{
		keymap.ModeError: {
			"r":      keymap.UnboundBinding(),
			"q":      keymap.UnboundBinding(),
			"ctrl+c": keymap.CommandBinding(keymap.CommandErrorRetry),
		},
	}, nil)
	b = enterErrorMode(t, b)

	// Attempt to steal ctrl+c for something else above -- update.go's global
	// ctrl+c-always-quits check runs before any mode dispatch, so this must
	// have no effect.
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("ctrl+c in errorMode should return a non-nil Cmd (tea.Quit), even with a fully overridden ModeError table")
	}
}

// --- Truthful keymap hints (#582) ---
//
// The git/dispatch/help panel hint builders above (gitPanelHints,
// dispatchModalHints, helpHints) unconditionally append a Hint{Desc: ...}
// even when panelHintKey returns "" -- every key for that command's family
// was unbound. The status bar then renders a dead affordance (e.g. ": Close",
// ": Run", ": Enroll") that silently no-ops if pressed. errorHints (still
// wired through builtinHints, which already skips a command with no bound
// key entirely) is explicitly OUT of scope for a production fix here --
// the tests below pin its already-correct behavior as a regression guard,
// not a RED case.

// TestKeymapPanels_UnbindWholeHintFamily_HintDisappearsCleanly is table-
// driven across every curated panel hint (git panel's Cancel/Navigate/Run,
// dispatch's Close/Enroll/Dispatch once/Confirm/Cancel, help's Close/
// Scroll): once every key underlying a hint's command family is unbound,
// the hint must disappear -- no Hint.Key == "", and no remaining hint still
// carries the removed family's Desc.
func TestKeymapPanels_UnbindWholeHintFamily_HintDisappearsCleanly(t *testing.T) {
	tests := []struct {
		name        string
		modes       map[keymap.Mode]keymap.Table
		dispatch    *dispatchState
		hints       func(Board) []Hint
		removedDesc string
	}{
		{
			name:        "git panel: esc unbound removes Cancel",
			modes:       map[keymap.Mode]keymap.Table{keymap.ModeGitPanel: {"esc": keymap.UnboundBinding()}},
			hints:       func(b Board) []Hint { return b.gitPanelHints() },
			removedDesc: "Cancel",
		},
		{
			name: "git panel: j/down/k/up unbound removes Navigate",
			modes: map[keymap.Mode]keymap.Table{keymap.ModeGitPanel: {
				"j": keymap.UnboundBinding(), "down": keymap.UnboundBinding(),
				"k": keymap.UnboundBinding(), "up": keymap.UnboundBinding(),
			}},
			hints:       func(b Board) []Hint { return b.gitPanelHints() },
			removedDesc: "Navigate",
		},
		{
			name:        "git panel: enter unbound removes Run",
			modes:       map[keymap.Mode]keymap.Table{keymap.ModeGitPanel: {"enter": keymap.UnboundBinding()}},
			hints:       func(b Board) []Hint { return b.gitPanelHints() },
			removedDesc: "Run",
		},
		{
			name:        "dispatch: esc unbound removes Close (not-enrolled state)",
			modes:       map[keymap.Mode]keymap.Table{keymap.ModeDispatch: {"esc": keymap.UnboundBinding()}},
			dispatch:    &dispatchState{repo: "owner/repo", enrolled: false},
			hints:       func(b Board) []Hint { return b.dispatchModalHints() },
			removedDesc: "Close",
		},
		{
			name:        "dispatch: enter unbound removes Enroll",
			modes:       map[keymap.Mode]keymap.Table{keymap.ModeDispatch: {"enter": keymap.UnboundBinding()}},
			dispatch:    &dispatchState{repo: "owner/repo", enrolled: false},
			hints:       func(b Board) []Hint { return b.dispatchModalHints() },
			removedDesc: "Enroll",
		},
		{
			name:        "dispatch: o unbound removes Dispatch once",
			modes:       map[keymap.Mode]keymap.Table{keymap.ModeDispatch: {"o": keymap.UnboundBinding()}},
			dispatch:    &dispatchState{repo: "owner/repo", enrolled: true},
			hints:       func(b Board) []Hint { return b.dispatchModalHints() },
			removedDesc: "Dispatch once",
		},
		{
			name:        "dispatch: y unbound removes Confirm",
			modes:       map[keymap.Mode]keymap.Table{keymap.ModeDispatch: {"y": keymap.UnboundBinding()}},
			dispatch:    &dispatchState{repo: "owner/repo", enrolled: true, confirmingLoop: true, loop: &cenciwatch.DispatchState{Enabled: false}},
			hints:       func(b Board) []Hint { return b.dispatchModalHints() },
			removedDesc: "Confirm",
		},
		{
			name: "dispatch: n+esc unbound removes Cancel",
			modes: map[keymap.Mode]keymap.Table{keymap.ModeDispatch: {
				"n": keymap.UnboundBinding(), "esc": keymap.UnboundBinding(),
			}},
			dispatch:    &dispatchState{repo: "owner/repo", enrolled: true, confirmingLoop: true, loop: &cenciwatch.DispatchState{Enabled: false}},
			hints:       func(b Board) []Hint { return b.dispatchModalHints() },
			removedDesc: "Cancel",
		},
		{
			name: "help: esc+? unbound removes Close",
			modes: map[keymap.Mode]keymap.Table{keymap.ModeHelp: {
				"esc": keymap.UnboundBinding(), "?": keymap.UnboundBinding(),
			}},
			hints:       func(b Board) []Hint { return b.helpHints() },
			removedDesc: "Close",
		},
		{
			name: "help: j/down/k/up unbound removes Scroll",
			modes: map[keymap.Mode]keymap.Table{keymap.ModeHelp: {
				"j": keymap.UnboundBinding(), "down": keymap.UnboundBinding(),
				"k": keymap.UnboundBinding(), "up": keymap.UnboundBinding(),
			}},
			hints:       func(b Board) []Hint { return b.helpHints() },
			removedDesc: "Scroll",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newLoadedTestBoard(t)
			b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			if tc.dispatch != nil {
				b.dispatch = *tc.dispatch
			}

			hints := tc.hints(b)
			for _, h := range hints {
				if h.Key == "" {
					t.Errorf("hints = %+v, want no Hint with an empty Key after unbinding the whole %q family", hints, tc.removedDesc)
				}
				if h.Desc == tc.removedDesc {
					t.Errorf("hints = %+v, still contains a %q-described hint after every underlying key was unbound", hints, tc.removedDesc)
				}
			}
		})
	}
}

// --- errorHints regression: OUT of scope for a production fix, must never
// render a blank-key hint even with an unbound Retry/Quit ------------------

// TestKeymapPanels_Error_ErrorHintsRegression_NeverBlankKeyEvenWhenUnbound
// pins errorHints' already-correct behavior (it stays on builtinHints,
// which already skips a command entirely once it has no bound key -- see
// builtinHints, keymap_dispatch.go). This is a regression guard, not a RED
// case: it is expected to PASS unmodified, proving #582's production fix
// must leave errorHints alone.
func TestKeymapPanels_Error_ErrorHintsRegression_NeverBlankKeyEvenWhenUnbound(t *testing.T) {
	tests := []struct {
		name  string
		modes map[keymap.Mode]keymap.Table
	}{
		{
			name:  "Retry (r) unbound alone",
			modes: map[keymap.Mode]keymap.Table{keymap.ModeError: {"r": keymap.UnboundBinding()}},
		},
		{
			name:  "Quit (q) unbound alone",
			modes: map[keymap.Mode]keymap.Table{keymap.ModeError: {"q": keymap.UnboundBinding()}},
		},
		{
			name: "Retry (r) and Quit (q) both unbound",
			modes: map[keymap.Mode]keymap.Table{keymap.ModeError: {
				"r": keymap.UnboundBinding(), "q": keymap.UnboundBinding(),
			}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := boardWithOverrideKeymap(t, newTestBoard(t), tc.modes, nil)
			b = enterErrorMode(t, b)

			hints := b.errorHints()
			for _, h := range hints {
				if h.Key == "" {
					t.Errorf("errorHints() = %+v, want no blank-key hint (errorHints stays on builtinHints, which already skips an unbound command entirely)", hints)
				}
			}
		})
	}
}

// --- Remap-shows-new-key (AC7): the git-panel case ---
//
// Help and Dispatch are already covered by
// TestKeymapPanels_Help_RemappedCloseKeyWorks_OldDefaultKeyBecomesNoop and
// TestKeymapPanels_Dispatch_RemappedConfirmCancelKeys_StillDriveConfirmFlow;
// this is the missing git-panel-hint-bar counterpart.

func TestKeymapPanels_GitPanel_RemapShowsNewKeyInHints(t *testing.T) {
	b, _ := newGitPanelTestBoard(t, nil, nil)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeGitPanel: {
			"esc": keymap.UnboundBinding(),
			"x":   keymap.CommandBinding(keymap.CommandGitPanelClose),
		},
	}, nil)

	hints := b.gitPanelHints()
	found := false
	for _, h := range hints {
		if h.Desc != "Cancel" {
			continue
		}
		if strings.Contains(h.Key, "esc") {
			t.Errorf("gitPanelHints() Cancel hint Key = %q, still advertises the unbound key %q", h.Key, "esc")
		}
		for _, k := range strings.Split(h.Key, "/") {
			if k == "x" {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("gitPanelHints() = %+v, want the Cancel hint to advertise the remapped key %q", hints, "x")
	}
}

// --- Grouped-hint disappearance (AC8 boundary) ---

// TestKeymapPanels_Help_ScrollHint_SurvivesWithOneKeyDisappearsWithNone pins
// the AC8 boundary: a composite hint (Scroll = ScrollDown + ScrollUp)
// survives, showing only the remaining member's key, once just ONE member
// command loses its key -- and disappears entirely only once EVERY member
// command has no key left.
func TestKeymapPanels_Help_ScrollHint_SurvivesWithOneKeyDisappearsWithNone(t *testing.T) {
	// Unbind only ScrollDown's keys: Scroll must survive, showing only
	// ScrollUp's remaining key ("k").
	bOneUnbound := newLoadedTestBoard(t)
	bOneUnbound = boardWithOverrideKeymap(t, bOneUnbound, map[keymap.Mode]keymap.Table{
		keymap.ModeHelp: {"j": keymap.UnboundBinding(), "down": keymap.UnboundBinding()},
	}, nil)

	hints := bOneUnbound.helpHints()
	idx := -1
	for i, h := range hints {
		if h.Desc == "Scroll" {
			idx = i
		}
	}
	if idx == -1 {
		t.Fatalf("helpHints() = %+v, want the Scroll hint to survive when ScrollUp still has a key", hints)
	}
	if hints[idx].Key != "k" {
		t.Errorf("helpHints() Scroll Key = %q, want %q (only ScrollUp's remaining key)", hints[idx].Key, "k")
	}

	// Unbind BOTH ScrollDown's and ScrollUp's keys: Scroll must disappear
	// entirely.
	bBothUnbound := newLoadedTestBoard(t)
	bBothUnbound = boardWithOverrideKeymap(t, bBothUnbound, map[keymap.Mode]keymap.Table{
		keymap.ModeHelp: {
			"j": keymap.UnboundBinding(), "down": keymap.UnboundBinding(),
			"k": keymap.UnboundBinding(), "up": keymap.UnboundBinding(),
		},
	}, nil)
	for _, h := range bBothUnbound.helpHints() {
		if h.Desc == "Scroll" {
			t.Errorf("helpHints() = %+v, want no Scroll hint once every underlying key is unbound", bBothUnbound.helpHints())
		}
	}
}

// --- Fully-empty hint bar (accepted end state per the ticket's Q&A) ---

// TestKeymapPanels_Help_FullyUnboundKeymap_HintsSliceIsEmpty asserts that
// once every key underlying every one of helpHints' curated hints (Close,
// Scroll) is unbound, helpHints returns an empty slice -- this is an
// intentional, accepted end state (not a bug to guard against), pinned here
// so a future change doesn't "fix" it into rendering some fallback hint.
func TestKeymapPanels_Help_FullyUnboundKeymap_HintsSliceIsEmpty(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeHelp: {
			"esc": keymap.UnboundBinding(), "?": keymap.UnboundBinding(),
			"j": keymap.UnboundBinding(), "down": keymap.UnboundBinding(),
			"k": keymap.UnboundBinding(), "up": keymap.UnboundBinding(),
		},
	}, nil)

	hints := b.helpHints()
	if len(hints) != 0 {
		t.Errorf("helpHints() = %+v, want an empty slice once every underlying key of every curated hint is unbound", hints)
	}
}

// --- #579: Git Menu inline actions preserve board/card/PR scope ---
//
// closeGitMenuAndDispatch (update.go) is called by both Git Menu entry paths
// -- dispatchGitMenuAction (direct-key) and runGitPanelCommand's
// CommandGitPanelRun case (cursor+Enter) -- and routes through
// dispatchResolvedAction and the shared prScopeUnavailable predicate, so a
// Git Menu action honors its declared Scope (board/card/pr) exactly like
// normal-mode custom-action dispatch: a card-scope action's
// {number}/{title}/{tags} variables resolve against the selected card, and a
// pr-scope action goes through the same 0/1/2+ linked-PR precedence.
//
// gitMenuDispatchPath abstracts over the two Git Menu entry paths so every
// scenario below is exercised through both without copy-pasting the
// scenario itself, per the plan's "structural, not copy-pasted" parity
// requirement.
type gitMenuDispatchPath struct {
	name string
	// dispatch fires act's key against an already-open git menu, returning
	// the resulting board and cmd.
	dispatch func(t *testing.T, b Board, key string) (Board, tea.Cmd)
}

var gitMenuDispatchPaths = []gitMenuDispatchPath{
	{
		name: "direct-key",
		dispatch: func(t *testing.T, b Board, key string) (Board, tea.Cmd) {
			t.Helper()
			m, cmd := b.Update(keyMsg(key))
			board, ok := m.(Board)
			if !ok {
				t.Fatalf("Update returned %T, want Board", m)
			}
			return board, cmd
		},
	},
	{
		name: "cursor+Run",
		dispatch: func(t *testing.T, b Board, key string) (Board, tea.Cmd) {
			t.Helper()
			idx := gitPanelItemIndex(b, key)
			if idx == -1 {
				t.Fatalf("no git panel item bound to key %q", key)
			}
			b.gitPanel.cursor = idx
			m, cmd := b.Update(arrowMsg(tea.KeyEnter))
			board, ok := m.(Board)
			if !ok {
				t.Fatalf("Update returned %T, want Board", m)
			}
			return board, cmd
		},
	},
}

// newGitMenuScopeTestBoard builds a git-panel test board seeded with
// prFixtureColumns() (the shared 0/1/2-linked-PR card fixture: card #1 "No
// PRs", card #2 "One PR", card #3 "Two PRs") and the given ModeGitPanel
// table layered on top of the built-in defaults (via boardWithOverrideKeymap
// -- keymap.Resolve merges, so the six built-ins stay available alongside
// table's entries). The column cursor is moved down cardIndex times before
// returning (cardIndex 0 leaves the cursor on card #1).
func newGitMenuScopeTestBoard(t *testing.T, table map[keymap.Mode]keymap.Table, cardIndex int) (Board, *action.FakeExecutor) {
	t.Helper()
	b, fe := newGitPanelTestBoardWithColumns(t, nil, nil, prFixtureColumns())
	if table != nil {
		b = boardWithOverrideKeymap(t, b, table, nil)
	}
	for i := 0; i < cardIndex; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}
	return b, fe
}

// --- Board scope: behaviorally identical to today, across both entry paths ---

func TestKeymapPanels_GitMenuScope_Board_ParityAcrossEntryPaths(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		table   map[keymap.Mode]keymap.Table // nil selects the built-in default table
		wantCmd string
	}{
		{
			name:    "built-in Fetch",
			key:     "f",
			wantCmd: "git fetch",
		},
		{
			name: "custom board-scope override",
			key:  "c",
			table: map[keymap.Mode]keymap.Table{
				keymap.ModeGitPanel: {
					"c": keymap.ActionBinding(keymap.Action{Name: "Custom Board", Type: "shell", Scope: "board", Command: "echo {repo_owner}/{repo_name}"}),
				},
			},
			wantCmd: "echo " + action.ShellEscape("matteobortolazzo") + "/" + action.ShellEscape("lazyboards"),
		},
	}

	for _, tc := range cases {
		for _, path := range gitMenuDispatchPaths {
			t.Run(tc.name+"/"+path.name, func(t *testing.T) {
				b, fe := newGitPanelTestBoard(t, nil, nil)
				if tc.table != nil {
					b = boardWithOverrideKeymap(t, b, tc.table, nil)
				}
				b = sendKey(t, b, keyMsg("G"))
				if b.mode != gitPanelMode {
					t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
				}

				b, cmd := path.dispatch(t, b, tc.key)
				execCmds(cmd)

				if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != tc.wantCmd {
					t.Errorf("RunShellCalls = %v, want first call %q", fe.RunShellCalls, tc.wantCmd)
				}
				if b.mode != normalMode {
					t.Errorf("mode = %v after board-scope dispatch, want normalMode", b.mode)
				}
				if !reflect.DeepEqual(b.statusBar.hints, b.normalHints) {
					t.Errorf("hints = %+v, want b.normalHints %+v (board-scope dispatch must restore normal hints, unchanged from today)", b.statusBar.hints, b.normalHints)
				}
			})
		}
	}
}

// --- Card scope: the action must receive the selected card's own vars ---

// TestKeymapPanels_GitMenuScope_Card_ReceivesSelectedCardVars_ParityAcrossEntryPaths
// asserts a scope: card Git Menu action expands {number}/{title}/{tags}
// against the selected card (#2, "One PR", labels ["feature"]) -- values that
// differ from the board/repo ("matteobortolazzo"/"lazyboards"), so a
// board-only expansion would leave {number}/{title}/{tags} as literal,
// unexpanded text.
func TestKeymapPanels_GitMenuScope_Card_ReceivesSelectedCardVars_ParityAcrossEntryPaths(t *testing.T) {
	const cmdTemplate = "echo {number} {title} {tags}"
	table := map[keymap.Mode]keymap.Table{
		keymap.ModeGitPanel: {
			"c": keymap.ActionBinding(keymap.Action{Name: "Custom Card", Type: "shell", Scope: "card", Command: cmdTemplate}),
		},
	}

	for _, path := range gitMenuDispatchPaths {
		t.Run(path.name, func(t *testing.T) {
			b, fe := newGitMenuScopeTestBoard(t, table, 1) // card #2, "One PR"
			card := b.selectedCard()
			if card.Number != 2 {
				t.Fatalf("test setup: selected card = #%d, want #2 (prFixtureColumns())", card.Number)
			}
			labelNames := make([]string, len(card.Labels))
			for i, l := range card.Labels {
				labelNames[i] = l.Name
			}
			vars := action.BuildShellSafeVars(action.BuildTemplateVars(card.Number, card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, "", ""))
			wantCmd := action.ExpandTemplate(cmdTemplate, vars)

			b = sendKey(t, b, keyMsg("G"))
			if b.mode != gitPanelMode {
				t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
			}

			b, cmd := path.dispatch(t, b, "c")
			execCmds(cmd)

			if len(fe.RunShellCalls) == 0 {
				t.Fatal("expected RunShell to be called for the card-scope action")
			}
			if fe.RunShellCalls[0] != wantCmd {
				t.Errorf("RunShellCalls[0] = %q, want %q (card #2's own template expansion, not board-only expansion)", fe.RunShellCalls[0], wantCmd)
			}
			if b.mode != normalMode {
				t.Errorf("mode = %v after card-scope dispatch, want normalMode", b.mode)
			}
		})
	}
}

// TestKeymapPanels_GitMenuScope_Card_RefusedSilentlyWhenNoCardVisible_ParityAcrossEntryPaths
// covers AC3: a scope: card Git Menu action must be refused without side
// effects when the active column has no cards (mirroring
// dispatchResolvedAction's len(b.visibleCards()) == 0 guard for normal-mode
// card-scope actions). Row visibility is out of scope (Q2) -- the row stays
// in the menu regardless -- so both entry paths can still reach the key.
func TestKeymapPanels_GitMenuScope_Card_RefusedSilentlyWhenNoCardVisible_ParityAcrossEntryPaths(t *testing.T) {
	table := map[keymap.Mode]keymap.Table{
		keymap.ModeGitPanel: {
			"c": keymap.ActionBinding(keymap.Action{Name: "Custom Card", Type: "shell", Scope: "card", Command: "echo {number}"}),
		},
	}

	for _, path := range gitMenuDispatchPaths {
		t.Run(path.name, func(t *testing.T) {
			b, fe := newGitPanelTestBoard(t, nil, nil) // single empty column, no cards
			b = boardWithOverrideKeymap(t, b, table, nil)
			b = sendKey(t, b, keyMsg("G"))
			if b.mode != gitPanelMode {
				t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
			}

			b, cmd := path.dispatch(t, b, "c")
			execCmds(cmd)

			if len(fe.RunShellCalls) != 0 {
				t.Errorf("RunShellCalls = %v, want no dispatch when no card is visible", fe.RunShellCalls)
			}
			if len(fe.OpenURLCalls) != 0 {
				t.Errorf("OpenURLCalls = %v, want no dispatch when no card is visible", fe.OpenURLCalls)
			}
			if b.mode != normalMode {
				t.Errorf("mode = %v after refused card-scope dispatch, want normalMode", b.mode)
			}
		})
	}
}

// --- PR scope: 0/1/2+ linked-PR precedence, mirroring normal-mode dispatch ---

// TestKeymapPanels_GitMenuScope_PR_ZeroLinkedPRs_RefusedSilently_ParityAcrossEntryPaths
// covers Q&A #1: a scope: pr Git Menu action on a card with 0 linked PRs must
// be refused SILENTLY -- no status message, no executor call, no PR picker --
// exactly like dispatchBinding's pr-scope gate in keymap_dispatch.go refuses
// a normal-mode scope: pr action before handlePRActionKeyWithComment's
// defensive-only case 0 branch is ever reached.
func TestKeymapPanels_GitMenuScope_PR_ZeroLinkedPRs_RefusedSilently_ParityAcrossEntryPaths(t *testing.T) {
	table := map[keymap.Mode]keymap.Table{
		keymap.ModeGitPanel: {
			"r": keymap.ActionBinding(keymap.Action{Name: "Custom PR", Type: "shell", Scope: "pr", Command: "echo {pr_number}"}),
		},
	}

	for _, path := range gitMenuDispatchPaths {
		t.Run(path.name, func(t *testing.T) {
			b, fe := newGitMenuScopeTestBoard(t, table, 0) // card #1, "No PRs"
			card := b.selectedCard()
			if card.Number != 1 || len(card.LinkedPRs) != 0 {
				t.Fatalf("test setup: selected card = %+v, want card #1 with 0 linked PRs", card)
			}

			b = sendKey(t, b, keyMsg("G"))
			if b.mode != gitPanelMode {
				t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
			}

			b, cmd := path.dispatch(t, b, "r")
			execCmds(cmd)

			if len(fe.RunShellCalls) != 0 {
				t.Errorf("RunShellCalls = %v, want silent refusal with 0 linked PRs (no dispatch at all)", fe.RunShellCalls)
			}
			if b.statusBar.message != "" {
				t.Errorf("status message = %q, want no message -- refusal must be silent, matching normal-mode's pr-scope gate", b.statusBar.message)
			}
			if b.mode == prPickerMode {
				t.Errorf("mode = prPickerMode, want NOT entering the PR picker with 0 linked PRs")
			}
			if b.pendingPRAction != nil {
				t.Errorf("pendingPRAction = %+v, want nil with 0 linked PRs", b.pendingPRAction)
			}
			if b.mode != normalMode {
				t.Errorf("mode = %v, want normalMode (the git menu still closes even when the action itself is refused)", b.mode)
			}
		})
	}
}

// TestKeymapPanels_GitMenuScope_PR_OneLinkedPR_RunsImmediately_ParityAcrossEntryPaths
// covers a scope: pr Git Menu action against a card with exactly 1 linked PR
// (card #2, PR #10): it must run immediately with the full PR variable set
// ({pr_number}, {pr_url}, ...), for both a url-type and a shell-type action.
func TestKeymapPanels_GitMenuScope_PR_OneLinkedPR_RunsImmediately_ParityAcrossEntryPaths(t *testing.T) {
	cases := []struct {
		name string
		key  string
		act  keymap.Action
	}{
		{
			name: "url action",
			key:  "u",
			act:  keymap.Action{Name: "Open PR", Type: "url", Scope: "pr", URL: "https://example.com/pr/{pr_number}?src={pr_url}"},
		},
		{
			name: "shell action",
			key:  "r",
			act:  keymap.Action{Name: "Shell PR", Type: "shell", Scope: "pr", Command: "echo {pr_number} {pr_branch}"},
		},
	}

	for _, tc := range cases {
		for _, path := range gitMenuDispatchPaths {
			t.Run(tc.name+"/"+path.name, func(t *testing.T) {
				table := map[keymap.Mode]keymap.Table{keymap.ModeGitPanel: {tc.key: keymap.ActionBinding(tc.act)}}
				b, fe := newGitMenuScopeTestBoard(t, table, 1) // card #2, 1 linked PR
				card := b.selectedCard()
				if len(card.LinkedPRs) != 1 {
					t.Fatalf("test setup: selected card has %d linked PRs, want 1", len(card.LinkedPRs))
				}
				pr := card.LinkedPRs[0]

				b = sendKey(t, b, keyMsg("G"))
				if b.mode != gitPanelMode {
					t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
				}

				b, cmd := path.dispatch(t, b, tc.key)
				execCmds(cmd)

				labelNames := make([]string, len(card.Labels))
				for i, l := range card.Labels {
					labelNames[i] = l.Name
				}
				baseVars := action.BuildTemplateVars(card.Number, card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, "", "")
				prVars := action.BuildPRTemplateVars(baseVars, pr.Number, pr.Title, pr.URL, pr.Branch, "")

				switch tc.act.Type {
				case "url":
					want := action.ExpandTemplate(tc.act.URL, action.BuildURLSafeVars(prVars))
					if len(fe.OpenURLCalls) == 0 || fe.OpenURLCalls[0] != want {
						t.Errorf("OpenURLCalls = %v, want first call %q", fe.OpenURLCalls, want)
					}
				case "shell":
					want := action.ExpandTemplate(tc.act.Command, action.BuildShellSafeVars(prVars))
					if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != want {
						t.Errorf("RunShellCalls = %v, want first call %q", fe.RunShellCalls, want)
					}
				}
				if b.mode != normalMode {
					t.Errorf("mode = %v after a 1-linked-PR git menu action, want normalMode (runs immediately, no picker)", b.mode)
				}
			})
		}
	}
}

// TestKeymapPanels_GitMenuScope_PR_MultipleLinkedPRs_OpensPickerThenRunsSelected_ParityAcrossEntryPaths
// covers a scope: pr Git Menu action against a card with 2+ linked PRs (card
// #3): it must stash the action as pendingPRAction and open prPickerMode
// (mirroring handlePRActionKeyWithComment's default case), advertising
// b.prPickerHints(); selecting a PR (Enter, the default prPickerIndex 0)
// must run the pending action against LinkedPRs[0] and clear pendingPRAction.
func TestKeymapPanels_GitMenuScope_PR_MultipleLinkedPRs_OpensPickerThenRunsSelected_ParityAcrossEntryPaths(t *testing.T) {
	const cmdTemplate = "echo {pr_number} {pr_branch}"
	act := keymap.Action{Name: "Custom PR", Type: "shell", Scope: "pr", Command: cmdTemplate}
	table := map[keymap.Mode]keymap.Table{keymap.ModeGitPanel: {"r": keymap.ActionBinding(act)}}

	for _, path := range gitMenuDispatchPaths {
		t.Run(path.name, func(t *testing.T) {
			b, fe := newGitMenuScopeTestBoard(t, table, 2) // card #3, "Two PRs"
			card := b.selectedCard()
			if len(card.LinkedPRs) < 2 {
				t.Fatalf("test setup: selected card has %d linked PRs, want >= 2", len(card.LinkedPRs))
			}

			b = sendKey(t, b, keyMsg("G"))
			if b.mode != gitPanelMode {
				t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
			}

			b, cmd := path.dispatch(t, b, "r")
			execCmds(cmd)

			if b.mode != prPickerMode {
				t.Fatalf("mode = %v after a %d-linked-PR git menu action, want prPickerMode", b.mode, len(card.LinkedPRs))
			}
			if b.pendingPRAction == nil {
				t.Fatal("pendingPRAction = nil, want the git menu action stashed pending PR selection")
			}
			if b.pendingPRAction.action.Name != act.Name {
				t.Errorf("pendingPRAction.action.Name = %q, want %q", b.pendingPRAction.action.Name, act.Name)
			}
			wantHints := b.prPickerHints()
			if !reflect.DeepEqual(b.statusBar.hints, wantHints) {
				t.Errorf("hints = %+v, want prPickerHints() %+v", b.statusBar.hints, wantHints)
			}
			if len(fe.RunShellCalls) != 0 {
				t.Errorf("RunShellCalls = %v, want no dispatch yet -- the picker is awaiting PR selection", fe.RunShellCalls)
			}

			pr := card.LinkedPRs[0]
			m2, cmd2 := b.Update(arrowMsg(tea.KeyEnter))
			b2, ok := m2.(Board)
			if !ok {
				t.Fatalf("Update returned %T, want Board", m2)
			}
			execCmds(cmd2)

			if b2.pendingPRAction != nil {
				t.Errorf("pendingPRAction after picker selection = %+v, want nil", b2.pendingPRAction)
			}
			if b2.mode != normalMode {
				t.Errorf("mode after picker selection = %v, want normalMode", b2.mode)
			}

			labelNames := make([]string, len(card.Labels))
			for i, l := range card.Labels {
				labelNames[i] = l.Name
			}
			baseVars := action.BuildTemplateVars(card.Number, card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, "", "")
			prVars := action.BuildPRTemplateVars(baseVars, pr.Number, pr.Title, pr.URL, pr.Branch, "")
			want := action.ExpandTemplate(cmdTemplate, action.BuildShellSafeVars(prVars))

			if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != want {
				t.Errorf("RunShellCalls = %v, want first call %q (expanded against LinkedPRs[0], the picker's default selection)", fe.RunShellCalls, want)
			}
			// A single dispatch call guards against the picker's Enter key
			// somehow firing the pending action twice (once eagerly on open,
			// once on selection) -- the sanctioned no-duplicate-dispatch
			// exception (.claude/rules/testing.md).
			if len(fe.RunShellCalls) != 1 {
				t.Errorf("len(fe.RunShellCalls) = %d, want exactly 1 (no duplicate dispatch)", len(fe.RunShellCalls))
			}
		})
	}
}

// --- {pr_worktree}: on-demand resolution, both success and missing-worktree ---

// TestKeymapPanels_GitMenuScope_PR_WorktreeExpandsRegisteredWorktree_ParityAcrossEntryPaths
// mirrors TestAction_PRScope_PRWorktreeExpandsRegisteredWorktree (normal-mode
// scope: pr {pr_worktree}) through the Git Menu's two entry paths.
func TestKeymapPanels_GitMenuScope_PR_WorktreeExpandsRegisteredWorktree_ParityAcrossEntryPaths(t *testing.T) {
	act := keymap.Action{Name: "Run worktree", Type: "shell", Scope: "pr", Command: "cd {pr_worktree} && ng serve"}
	table := map[keymap.Mode]keymap.Table{keymap.ModeGitPanel: {"w": keymap.ActionBinding(act)}}

	for _, path := range gitMenuDispatchPaths {
		t.Run(path.name, func(t *testing.T) {
			b, fe := newGitMenuScopeTestBoard(t, table, 1) // card #2, 1 linked PR (branch feature/one-pr)
			fe.RunShellOutputStdout = "worktree /repo/.worktrees/one-pr\nHEAD 1234567\nbranch refs/heads/feature/one-pr\n"

			b = sendKey(t, b, keyMsg("G"))
			if b.mode != gitPanelMode {
				t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
			}

			b, cmd := path.dispatch(t, b, "w")
			execCmds(cmd)

			if len(fe.RunShellOutputCalls) != 1 || fe.RunShellOutputCalls[0] != "git worktree list --porcelain" {
				t.Fatalf("RunShellOutputCalls = %v, want a single %q call", fe.RunShellOutputCalls, "git worktree list --porcelain")
			}
			want := "cd " + action.ShellEscape("/repo/.worktrees/one-pr") + " && ng serve"
			if len(fe.RunShellCalls) == 0 || fe.RunShellCalls[0] != want {
				t.Errorf("RunShellCalls = %v, want %q", fe.RunShellCalls, want)
			}
			if b.mode != normalMode {
				t.Errorf("mode = %v after {pr_worktree} dispatch, want normalMode", b.mode)
			}
		})
	}
}

// TestKeymapPanels_GitMenuScope_PR_WorktreeMissing_SurfacesError_ParityAcrossEntryPaths
// mirrors TestAction_PRScope_PRWorktreeMissingDoesNotRunAction: when no
// registered worktree matches the PR's branch, the action must not run and
// the missing-worktree error must surface in the status bar.
func TestKeymapPanels_GitMenuScope_PR_WorktreeMissing_SurfacesError_ParityAcrossEntryPaths(t *testing.T) {
	act := keymap.Action{Name: "Run worktree", Type: "shell", Scope: "pr", Command: "cd {pr_worktree} && ng serve"}
	table := map[keymap.Mode]keymap.Table{keymap.ModeGitPanel: {"w": keymap.ActionBinding(act)}}

	for _, path := range gitMenuDispatchPaths {
		t.Run(path.name, func(t *testing.T) {
			b, fe := newGitMenuScopeTestBoard(t, table, 1) // card #2, 1 linked PR (branch feature/one-pr)
			fe.RunShellOutputStdout = "worktree /repo\nHEAD 1234567\nbranch refs/heads/main\n"

			b = sendKey(t, b, keyMsg("G"))
			if b.mode != gitPanelMode {
				t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
			}

			b, cmd := path.dispatch(t, b, "w")
			execCmds(cmd)

			if len(fe.RunShellCalls) != 0 {
				t.Errorf("RunShellCalls = %v, want no action command when the worktree is missing", fe.RunShellCalls)
			}
			if !strings.Contains(b.statusBar.message, "no Git worktree found") {
				t.Errorf("status message = %q, want a missing-worktree error", b.statusBar.message)
			}
		})
	}
}

// --- Preserved behavior: an empty/out-of-range git panel Run is still a no-op ---

// TestKeymapPanels_GitMenuScope_EmptyItemsOrCursorOutOfRange_RunIsNoOp is a
// regression-preservation check on runGitPanelCommand's existing
// CommandGitPanelRun guard (unrelated to scope routing, but named in the
// plan's test matrix so the #579 refactor can't accidentally drop it): with
// no items, or a cursor past the end of the items list, CommandGitPanelRun
// must close the menu without dispatching anything.
func TestKeymapPanels_GitMenuScope_EmptyItemsOrCursorOutOfRange_RunIsNoOp(t *testing.T) {
	tests := []struct {
		name string
		prep func(b *Board)
	}{
		{name: "empty items", prep: func(b *Board) { b.gitPanel.items = nil; b.gitPanel.cursor = 0 }},
		{name: "cursor out of range", prep: func(b *Board) { b.gitPanel.cursor = len(b.gitPanel.items) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b, fe := newGitPanelTestBoard(t, nil, nil)
			b = sendKey(t, b, keyMsg("G"))
			if b.mode != gitPanelMode {
				t.Fatalf("expected gitPanelMode after 'G', got %d", b.mode)
			}
			tc.prep(&b)

			m, cmd := b.runGitPanelCommand(keymap.CommandGitPanelRun)
			b2, ok := m.(Board)
			if !ok {
				t.Fatalf("runGitPanelCommand returned %T, want Board", m)
			}
			if cmd != nil {
				t.Errorf("cmd = %v, want nil", cmd)
			}
			if b2.mode != normalMode {
				t.Errorf("mode = %v, want normalMode", b2.mode)
			}
			if len(fe.RunShellCalls) != 0 || len(fe.OpenURLCalls) != 0 {
				t.Errorf("expected no executor calls, got RunShellCalls=%v OpenURLCalls=%v", fe.RunShellCalls, fe.OpenURLCalls)
			}
		})
	}
}
