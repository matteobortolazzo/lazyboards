package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Registry dispatch for the list-style modals (#490, PR 7a) ---
//
// handleFilterModeKey, handleAssignModeKey, and handlePRPickerModeKey cut
// over from a hardcoded switch msg.String()/msg.Type to
// keymap.Keymap.Lookup against their own catalogued modes (ModeFilter,
// ModeAssign, ModePRPicker), exactly like #489 did for normal/detail. The
// six hardcoded *Hints vars for these three modes are replaced by
// registry-derived accessors (b.filterHints(), b.assignHints(),
// b.prPickerHints()) defined in the not-yet-created keymap_modals.go.
//
// pr_list, milestone_list, and agent_list convert in the follow-up PR 7b;
// this file covers filter/assign/pr_picker only. Reuses
// boardWithOverrideKeymap from keymap_dispatch_test.go.

// --- Filter mode ---

func TestKeymapModals_Filter_UserOverrideWinsOverBuiltinKey(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeFilter: {"j": keymap.CommandBinding(keymap.CommandFilterClose)},
	}, nil)

	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("test setup: expected filterMode after 'f', got %d", b.mode)
	}

	b = sendKey(t, b, keyMsg("j"))

	if b.mode != normalMode {
		t.Errorf("mode = %v after 'j' remapped to filter.close, want normalMode -- the override must win over the built-in filter.next", b.mode)
	}
}

func TestKeymapModals_Filter_ExplicitUnbindIsNoOp(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeFilter: {"j": keymap.UnboundBinding()},
	}, nil)

	b = sendKey(t, b, keyMsg("f"))
	initialCursor := b.filterCursor

	b = sendKey(t, b, keyMsg("j"))

	if b.filterCursor != initialCursor {
		t.Errorf("filterCursor = %d after pressing an explicitly unbound 'j', want unchanged (%d)", b.filterCursor, initialCursor)
	}
	if b.mode != filterMode {
		t.Errorf("mode = %d after pressing an explicitly unbound key, want filterMode (%d)", b.mode, filterMode)
	}
}

// TestKeymapModals_Filter_HintsReflectRemappedKey covers the hint-bar
// accessor named in the plan's Files to Create section: b.filterHints()
// must derive from the active keymap, so after "j"/"down" are freed up and
// "n" takes over filter.next, the Navigate hint shows "n" and no longer
// advertises "j".
func TestKeymapModals_Filter_HintsReflectRemappedKey(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeFilter: {
			"n":    keymap.CommandBinding(keymap.CommandFilterNext),
			"j":    keymap.UnboundBinding(),
			"down": keymap.UnboundBinding(),
		},
	}, nil)

	b = sendKey(t, b, keyMsg("f"))

	hints := b.filterHints()
	foundNewKey := false
	for _, h := range hints {
		if h.Desc != "Navigate" {
			continue
		}
		if strings.Contains(h.Key, "n") {
			foundNewKey = true
		}
		if strings.Contains(h.Key, "j") {
			t.Errorf("Navigate hint Key = %q still advertises the remapped-away 'j', want it gone", h.Key)
		}
	}
	if !foundNewKey {
		t.Errorf("filterHints() = %+v, want a Navigate hint advertising the remapped key 'n'", hints)
	}
}

// TestKeymapModals_Filter_OutcomePendingPrefixIsNoOp is the Q4 explicit-risk
// test: filter mode has no multi-key pending-sequence machinery, so a key
// that is only a *prefix* of a bound sequence (OutcomePending) must be a
// silent no-op -- not a crash, not a partial dispatch.
func TestKeymapModals_Filter_OutcomePendingPrefixIsNoOp(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeFilter: {"z a": keymap.CommandBinding(keymap.CommandFilterSelect)},
	}, nil)

	b = sendKey(t, b, keyMsg("f"))
	initialCursor := b.filterCursor

	b = sendKey(t, b, keyMsg("z"))

	if b.mode != filterMode {
		t.Errorf("mode = %d after pressing a pending-sequence prefix key, want filterMode (%d) -- OutcomePending must be a no-op in modals", b.mode, filterMode)
	}
	if b.filterCursor != initialCursor {
		t.Errorf("filterCursor = %d after pressing a pending prefix key, want unchanged (%d)", b.filterCursor, initialCursor)
	}
	if b.activeFilterValue != "" {
		t.Errorf("activeFilterValue = %q after pressing a pending prefix key, want empty (must not partially dispatch filter.select)", b.activeFilterValue)
	}
}

// TestKeymapModals_Filter_UnrecognizedCommandIsNoOp is the Q4 explicit-risk
// test's second half: a key bound to a command id from a wholly different
// domain (here, normal mode's board.refresh) must resolve to OutcomeMatch
// but still be a documented no-op in handleFilterModeKey's switch, not
// silently fall through to running board.refresh.
func TestKeymapModals_Filter_UnrecognizedCommandIsNoOp(t *testing.T) {
	b := newBoardWithLabelsAndAssignees(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeFilter: {"z": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)

	b = sendKey(t, b, keyMsg("f"))
	initialCursor := b.filterCursor

	b = sendKey(t, b, keyMsg("z"))

	if b.mode != filterMode {
		t.Errorf("mode = %d after pressing a key bound to an out-of-domain command, want filterMode (%d)", b.mode, filterMode)
	}
	if b.filterCursor != initialCursor {
		t.Errorf("filterCursor = %d after pressing a key bound to board.refresh, want unchanged (%d)", b.filterCursor, initialCursor)
	}
	if b.refreshing {
		t.Error("refreshing = true after pressing a key bound to board.refresh inside filterMode, want false -- an unrecognized command id must not dispatch")
	}
}

// TestKeymapModals_Filter_InlineActionBindingDoesNotDispatch is the security-
// review regression test (#490 PR 7a fix-now finding): internal/config's
// keymap-action validation does not mode-scope inline actions, so a user
// config can legally bind a type: shell inline action inside keymaps.filter
// even though handleFilterModeKey has no inline-action dispatch path (Q4).
// Before the fix this relied on the untested implicit invariant that an
// action binding's zero-value Command lands in the switch's default no-op
// case; this asserts the explicit Binding.Kind guard instead, using
// b.Update directly (not sendKey, which discards the returned tea.Cmd) plus
// execCmds so a wrongly-dispatched shell action would actually run and be
// observed by the fake executor.
func TestKeymapModals_Filter_InlineActionBindingDoesNotDispatch(t *testing.T) {
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "", "", "", 0, 0, "Working", false, false, nil, nil, true)
	m, _ := b.Update(boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "Card One", Labels: []provider.Label{{Name: "bug"}}},
			}},
		},
	}})
	b = m.(Board)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeFilter: {"z": keymap.ActionBinding(keymap.Action{Name: "Pwn", Type: "shell", Command: "echo pwned", Scope: "board"})},
	}, nil)

	b = sendKey(t, b, keyMsg("f"))
	if b.mode != filterMode {
		t.Fatalf("test setup: expected filterMode after 'f', got %d", b.mode)
	}

	m, cmd := b.Update(keyMsg("z"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShell called %d times after pressing a key bound to an inline shell action inside filter mode, want 0 -- filter mode has no inline-action dispatch path and must not fire it: %v", len(fe.RunShellCalls), fe.RunShellCalls)
	}
	if b.mode != filterMode {
		t.Errorf("mode = %d after pressing a key bound to an inline action inside filterMode, want unchanged filterMode (%d)", b.mode, filterMode)
	}
}

// --- Assign mode ---

func TestKeymapModals_Assign_UserOverrideWinsOverBuiltinKey(t *testing.T) {
	b := newBoardWithCollaborators(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeAssign: {"j": keymap.CommandBinding(keymap.CommandAssignClose)},
	}, nil)

	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Fatalf("test setup: expected assignMode after 'a', got %d", b.mode)
	}

	b = sendKey(t, b, keyMsg("j"))

	if b.mode != normalMode {
		t.Errorf("mode = %v after 'j' remapped to assign.close, want normalMode -- the override must win over the built-in assign.next", b.mode)
	}
}

func TestKeymapModals_Assign_ExplicitUnbindIsNoOp(t *testing.T) {
	b := newBoardWithCollaborators(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeAssign: {"j": keymap.UnboundBinding()},
	}, nil)

	b = sendKey(t, b, keyMsg("a"))
	initialCursor := b.assign.cursor

	b = sendKey(t, b, keyMsg("j"))

	if b.assign.cursor != initialCursor {
		t.Errorf("assign.cursor = %d after pressing an explicitly unbound 'j', want unchanged (%d)", b.assign.cursor, initialCursor)
	}
	if b.mode != assignMode {
		t.Errorf("mode = %d after pressing an explicitly unbound key, want assignMode (%d)", b.mode, assignMode)
	}
}

// TestKeymapModals_Assign_HintsReflectRemappedKey covers the b.assignHints()
// accessor: after "enter" is unbound and "t" takes over assign.toggle, the
// Toggle hint shows "t" and stops advertising "enter".
func TestKeymapModals_Assign_HintsReflectRemappedKey(t *testing.T) {
	b := newBoardWithCollaborators(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeAssign: {
			"t":     keymap.CommandBinding(keymap.CommandAssignToggle),
			"enter": keymap.UnboundBinding(),
		},
	}, nil)

	b = sendKey(t, b, keyMsg("a"))

	hints := b.assignHints()
	foundNewKey := false
	for _, h := range hints {
		if h.Desc != "Toggle" {
			continue
		}
		if strings.Contains(h.Key, "t") {
			foundNewKey = true
		}
		if strings.Contains(h.Key, "enter") {
			t.Errorf("Toggle hint Key = %q still advertises the remapped-away 'enter', want it gone", h.Key)
		}
	}
	if !foundNewKey {
		t.Errorf("assignHints() = %+v, want a Toggle hint advertising the remapped key 't'", hints)
	}
}

func TestKeymapModals_Assign_OutcomePendingPrefixIsNoOp(t *testing.T) {
	b := newBoardWithCollaborators(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeAssign: {"z a": keymap.CommandBinding(keymap.CommandAssignToggle)},
	}, nil)

	b = sendKey(t, b, keyMsg("a"))
	initialCursor := b.assign.cursor

	b = sendKey(t, b, keyMsg("z"))

	if b.mode != assignMode {
		t.Errorf("mode = %d after pressing a pending-sequence prefix key, want assignMode (%d) -- OutcomePending must be a no-op in modals", b.mode, assignMode)
	}
	if b.assign.cursor != initialCursor {
		t.Errorf("assign.cursor = %d after pressing a pending prefix key, want unchanged (%d)", b.assign.cursor, initialCursor)
	}
	if strings.Contains(b.statusBar.message, "Updating assignees") {
		t.Error("statusBar.message reports an assignee update after only a pending prefix key, want no dispatch")
	}
}

func TestKeymapModals_Assign_UnrecognizedCommandIsNoOp(t *testing.T) {
	b := newBoardWithCollaborators(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeAssign: {"z": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)

	b = sendKey(t, b, keyMsg("a"))
	initialCursor := b.assign.cursor

	b = sendKey(t, b, keyMsg("z"))

	if b.mode != assignMode {
		t.Errorf("mode = %d after pressing a key bound to an out-of-domain command, want assignMode (%d)", b.mode, assignMode)
	}
	if b.assign.cursor != initialCursor {
		t.Errorf("assign.cursor = %d after pressing a key bound to board.refresh, want unchanged (%d)", b.assign.cursor, initialCursor)
	}
	if b.refreshing {
		t.Error("refreshing = true after pressing a key bound to board.refresh inside assignMode, want false -- an unrecognized command id must not dispatch")
	}
}

// --- PR picker mode ---

// newBoardAtTwoPRCard navigates to card 3 ("Two PRs": #20, #21) of the
// shared prFixtureColumns and opens the PR picker via the built-in 'p' key.
func newBoardAtTwoPRCard(t *testing.T, b Board) Board {
	t.Helper()
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) < 2 {
		t.Fatalf("test setup: expected card at cursor to have 2+ LinkedPRs, got %d", len(card.LinkedPRs))
	}
	b = sendKey(t, b, keyMsg("p"))
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected prPickerMode, got %d", b.mode)
	}
	return b
}

func TestKeymapModals_PRPicker_UserOverrideWinsOverBuiltinKey(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModePRPicker: {"enter": keymap.CommandBinding(keymap.CommandPRPickerClose)},
	}, nil)
	b = newBoardAtTwoPRCard(t, b)

	b = sendKey(t, b, arrowMsg(tea.KeyEnter))

	if b.mode != normalMode {
		t.Errorf("mode = %v after 'enter' remapped to pr_picker.close, want normalMode", b.mode)
	}
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("OpenURL called %d times after 'enter' remapped to pr_picker.close, want 0 -- the old pr_picker.select binding on 'enter' must stop firing", len(fe.OpenURLCalls))
	}
}

func TestKeymapModals_PRPicker_ExplicitUnbindIsNoOp(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModePRPicker: {"left": keymap.UnboundBinding()},
	}, nil)
	b = newBoardAtTwoPRCard(t, b)
	initialIndex := b.prPickerIndex

	b = sendKey(t, b, arrowMsg(tea.KeyLeft))

	if b.prPickerIndex != initialIndex {
		t.Errorf("prPickerIndex = %d after pressing an explicitly unbound Left, want unchanged (%d)", b.prPickerIndex, initialIndex)
	}
	if b.mode != prPickerMode {
		t.Errorf("mode = %d after pressing an explicitly unbound key, want prPickerMode (%d)", b.mode, prPickerMode)
	}
}

// TestKeymapModals_PRPicker_DefaultHints_PreserveArrowGlyphs is Q3's default-
// config pixel-identity case: with no override, b.prPickerHints() must still
// render the arrow glyphs "◀/▶" (◀/▶) for Cycle, even
// though the underlying keymap binds the raw key names "left"/"right".
func TestKeymapModals_PRPicker_DefaultHints_PreserveArrowGlyphs(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = newBoardAtTwoPRCard(t, b)

	hints := b.prPickerHints()

	found := false
	for _, h := range hints {
		if h.Desc == "Cycle" && strings.Contains(h.Key, "◀") && strings.Contains(h.Key, "▶") {
			found = true
		}
		if h.Desc == "Cycle" && (strings.Contains(h.Key, "left") || strings.Contains(h.Key, "right")) {
			t.Errorf("Cycle hint Key = %q, want the arrow glyphs, not the raw key names, for default-config users", h.Key)
		}
	}
	if !found {
		t.Errorf("prPickerHints() = %+v, want a Cycle hint rendering the \\u25c0/\\u25b6 arrow glyphs (pixel-identical to today)", hints)
	}
}

// TestKeymapModals_PRPicker_HintsReflectRemappedKey_UsesRawKeyNotGlyph
// covers both the general hint-reflection AC and the other half of Q3: the
// display-label map is keyed on the literal key name ("left"/"right"/
// "up"/"down"), not on the command id, so remapping Cycle onto "h"/"l"
// (neither of which appears in the display-label map) must show the raw key
// text, not a glyph -- and must no longer advertise "left"/"right".
func TestKeymapModals_PRPicker_HintsReflectRemappedKey_UsesRawKeyNotGlyph(t *testing.T) {
	b, _ := newBoardWithPRsAndExecutor(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModePRPicker: {
			"h":     keymap.CommandBinding(keymap.CommandPRPickerPrev),
			"l":     keymap.CommandBinding(keymap.CommandPRPickerNext),
			"left":  keymap.UnboundBinding(),
			"right": keymap.UnboundBinding(),
		},
	}, nil)
	b = newBoardAtTwoPRCard(t, b)

	hints := b.prPickerHints()

	found := false
	for _, h := range hints {
		if h.Desc != "Cycle" {
			continue
		}
		if strings.Contains(h.Key, "h") && strings.Contains(h.Key, "l") {
			found = true
		}
		if strings.ContainsAny(h.Key, "◀▶") {
			t.Errorf("Cycle hint Key = %q, want no arrow glyph once Cycle is remapped off left/right onto h/l", h.Key)
		}
	}
	if !found {
		t.Errorf("prPickerHints() = %+v, want a Cycle hint advertising the remapped raw keys 'h'/'l'", hints)
	}
}

func TestKeymapModals_PRPicker_OutcomePendingPrefixIsNoOp(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModePRPicker: {"z a": keymap.CommandBinding(keymap.CommandPRPickerSelect)},
	}, nil)
	b = newBoardAtTwoPRCard(t, b)
	initialIndex := b.prPickerIndex

	b = sendKey(t, b, keyMsg("z"))

	if b.mode != prPickerMode {
		t.Errorf("mode = %d after pressing a pending-sequence prefix key, want prPickerMode (%d) -- OutcomePending must be a no-op in modals", b.mode, prPickerMode)
	}
	if b.prPickerIndex != initialIndex {
		t.Errorf("prPickerIndex = %d after pressing a pending prefix key, want unchanged (%d)", b.prPickerIndex, initialIndex)
	}
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("OpenURL called %d times after only a pending prefix key, want 0", len(fe.OpenURLCalls))
	}
}

func TestKeymapModals_PRPicker_UnrecognizedCommandIsNoOp(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModePRPicker: {"z": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)
	b = newBoardAtTwoPRCard(t, b)
	initialIndex := b.prPickerIndex

	b = sendKey(t, b, keyMsg("z"))

	if b.mode != prPickerMode {
		t.Errorf("mode = %d after pressing a key bound to an out-of-domain command, want prPickerMode (%d)", b.mode, prPickerMode)
	}
	if b.prPickerIndex != initialIndex {
		t.Errorf("prPickerIndex = %d after pressing a key bound to board.refresh, want unchanged (%d)", b.prPickerIndex, initialIndex)
	}
	if b.refreshing {
		t.Error("refreshing = true after pressing a key bound to board.refresh inside prPickerMode, want false -- an unrecognized command id must not dispatch")
	}
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("OpenURL called %d times after a key bound to an out-of-domain command, want 0", len(fe.OpenURLCalls))
	}
}

// TestKeymapModals_PRPicker_ClampHoldsAfterShrinkAndRemappedNavigationKey is
// the list-cursor-invariants risk case: the picker's pre-Lookup index clamp
// (mode_handlers.go's defensive "an async board refresh can shrink
// LinkedPRs while the picker is open" guard) must still run before dispatch
// even when the navigation key that follows the shrink has been remapped
// away from its default binding -- the prologue must not be keyed off the
// specific key pressed.
func TestKeymapModals_PRPicker_ClampHoldsAfterShrinkAndRemappedNavigationKey(t *testing.T) {
	b, fe := newBoardWithPRsAndExecutor(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModePRPicker: {
			"l":     keymap.CommandBinding(keymap.CommandPRPickerNext),
			"right": keymap.UnboundBinding(),
		},
	}, nil)
	b = newBoardAtTwoPRCard(t, b)

	// Move to the second PR (index 1, PR #21) using the remapped key.
	b = sendKey(t, b, keyMsg("l"))
	if b.prPickerIndex != 1 {
		t.Fatalf("test setup: expected prPickerIndex 1 after the remapped navigation key, got %d", b.prPickerIndex)
	}

	// Simulate an in-flight background refresh completing while the picker
	// is still open, shrinking the same card's LinkedPRs from 2 to 1 --
	// mirrors TestPRPicker_LinkedPRsShrinkWhileOpen_ClampsIndexAndActsOnCorrectPR.
	b.refreshing = true
	shrunkPR := provider.LinkedPR{Number: 20, Title: "feat: first PR", URL: "https://github.com/owner/repo/pull/20", Branch: "feature/first-pr"}
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "No PRs", Labels: []provider.Label{{Name: "bug"}}},
				{Number: 2, Title: "One PR", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "feat: one PR", URL: "https://github.com/owner/repo/pull/10", Branch: "feature/one-pr"},
				}},
				{Number: 3, Title: "Two PRs", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{shrunkPR}},
			}},
		},
	}}
	m, cmd := b.Update(msg)
	b = m.(Board)
	execCmds(cmd)
	if b.mode != prPickerMode {
		t.Fatalf("test setup: expected mode to remain prPickerMode after in-flight refresh, got %d", b.mode)
	}

	// prPickerIndex is still 1, past the new length of 1. Pressing the
	// remapped navigation key must not panic, and must clamp before acting.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("remapped navigation key panicked after LinkedPRs shrank while the picker was open: %v", r)
		}
	}()
	b = sendKey(t, b, keyMsg("l"))

	if b.prPickerIndex != 0 {
		t.Errorf("prPickerIndex = %d after the clamp + remapped navigation key, want 0 (the only valid index)", b.prPickerIndex)
	}
	if b.mode != prPickerMode {
		t.Errorf("mode = %d after the remapped navigation key on a shrunk-but-nonempty list, want prPickerMode (%d)", b.mode, prPickerMode)
	}
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("OpenURL called %d times from a navigation key, want 0", len(fe.OpenURLCalls))
	}
}
