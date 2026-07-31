package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
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
	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
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
	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
	}

	m, _ := b.runGitPanelCommand(keymap.CommandGitPanelClose)
	b = m.(Board)

	if b.mode != normalMode {
		t.Errorf("mode = %v after CommandGitPanelClose, want normalMode", b.mode)
	}
}

func TestKeymapPanels_GitPanel_RunViaRunGitPanelCommand_DispatchesBuiltinAction(t *testing.T) {
	b, fe := newGitPanelTestBoard(t, nil, nil)
	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
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
			b = sendKey(t, b, keyMsg("g"))
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
	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
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
	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
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
	b = sendKey(t, b, keyMsg("g"))
	if b.mode != gitPanelMode {
		t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
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
			b = sendKey(t, b, keyMsg("g"))
			if b.mode != gitPanelMode {
				t.Fatalf("expected gitPanelMode after 'g', got %d", b.mode)
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
