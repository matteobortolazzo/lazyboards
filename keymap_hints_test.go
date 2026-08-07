package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- Hint derivation from the active registry (#489) ---
//
// The normal-mode and detail-focused status-bar hint bars must derive from
// the same active keymap dispatch itself resolves against, so a remap or
// unbind is reflected automatically -- the failure mode this ticket's final
// AC exists to prevent (a hint bar that lies about active bindings). For
// the default (unmodified) table, the bar must stay byte-identical to
// today's hardcoded content (per the plan's "Hint bars stay pixel-identical
// today, but become registry-derived" assumption).

func TestKeymapHints_NormalMode_DefaultTableMatchesTodaysHints(t *testing.T) {
	b := newLoadedTestBoard(t)

	want := []Hint{
		{Key: "?", Desc: "Help"},
		{Key: "n", Desc: "New"},
		{Key: "e", Desc: "Edit"},
	}
	got := b.normalHints
	if len(got) != len(want) {
		t.Fatalf("normalHints = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("normalHints[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestKeymapHints_DetailMode_DefaultTableMatchesTodaysHints(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	want := []Hint{
		{Key: "?", Desc: "Help"},
		{Key: "e", Desc: "Edit"},
		{Key: "j/k", Desc: "Scroll"},
		{Key: "h", Desc: "Back"},
		{Key: "esc", Desc: "Back"},
	}
	got := b.statusBar.hints
	if len(got) != len(want) {
		t.Fatalf("detail-focused hints = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("detail-focused hints[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// --- Remap reflection ---

func TestKeymapHints_NormalMode_RemappedCommandShowsNewKey(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"n": keymap.UnboundBinding(),
			"N": keymap.CommandBinding(keymap.CommandCardNew),
		},
	}, nil)

	hints := b.normalHints
	if hintIndex(hints, "n") != -1 {
		t.Errorf("normalHints still shows the old key %q for card.new after remap, got: %+v", "n", hints)
	}
	idx := hintIndex(hints, "N")
	if idx == -1 || hints[idx].Desc != "New" {
		t.Errorf("normalHints missing the remapped key %q for card.new, got: %+v", "N", hints)
	}
}

func TestKeymapHints_NormalMode_UnboundCommandHintDisappears(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"n": keymap.UnboundBinding()},
	}, nil)

	hints := b.normalHints
	if hintIndex(hints, "n") != -1 {
		t.Errorf("normalHints still shows %q after card.new was unbound, got: %+v", "n", hints)
	}
	for _, h := range hints {
		if h.Desc == "New" {
			t.Errorf("normalHints still contains a %q-described hint after card.new was unbound with no replacement key: %+v", "New", hints)
		}
	}
}

func TestKeymapHints_DetailMode_RemappedBlurKeyReflectsNewBinding(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDetail: {
			"h": keymap.UnboundBinding(),
			"b": keymap.CommandBinding(keymap.CommandDetailBlur),
		},
	}, nil)

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	hints := b.statusBar.hints
	if hintIndex(hints, "h") != -1 {
		t.Errorf("detail hints still show the unbound key %q for detail.blur, got: %+v", "h", hints)
	}
	idx := hintIndex(hints, "b")
	if idx == -1 || hints[idx].Desc != "Back" {
		t.Errorf("detail hints missing the remapped key %q for detail.blur, got: %+v", "b", hints)
	}
	if hintIndex(hints, "esc") == -1 {
		t.Errorf("detail hints should still show the other detail.blur key %q, got: %+v", "esc", hints)
	}
}

// --- Column-scoped built-in rebind reflection (Should-Fix review finding) ---

// TestKeymapHints_NormalMode_ColumnScopedBuiltinRebindShowsNewKeyOnlyInThatColumn
// covers the Should-Fix review finding: commandHintKeys must derive from the
// active column's overlaid entries (b.keys.Entries(mode, column)), exactly
// like inlineActionHints already does for inline actions, so a per-column
// rebind of a curated built-in (card.edit) is reflected in the hint bar
// while that column is active and does NOT leak into a column with no
// override.
func TestKeymapHints_NormalMode_ColumnScopedBuiltinRebindShowsNewKeyOnlyInThatColumn(t *testing.T) {
	b := newLoadedTestBoard(t)
	if got := b.activeColumnTitle(); got != "New" {
		t.Fatalf("precondition: active column = %q, want %q", got, "New")
	}
	b = boardWithOverrideKeymap(t, b, nil, map[string]keymap.Table{
		"new": {
			"e": keymap.UnboundBinding(),
			"x": keymap.CommandBinding(keymap.CommandCardEdit),
		},
	})

	hints := b.normalHints
	if hintIndex(hints, "e") != -1 {
		t.Errorf(`normalHints still shows the stale global key "e" for card.edit while the "New" column overrides it, got: %+v`, hints)
	}
	idx := hintIndex(hints, "x")
	if idx == -1 || hints[idx].Desc != "Edit" {
		t.Errorf(`normalHints missing the column-rebound key "x" for card.edit while the "New" column is active, got: %+v`, hints)
	}

	// Switching to a column with no override must show the unmodified global
	// key again, not leak the "New" column's overlay.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if got := b.activeColumnTitle(); got == "New" {
		t.Fatalf("precondition: active column should have changed away from %q", "New")
	}

	hints = b.normalHints
	if hintIndex(hints, "x") != -1 {
		t.Errorf(`normalHints shows the "New"-column-only key "x" for card.edit outside that column, got: %+v`, hints)
	}
	idx = hintIndex(hints, "e")
	if idx == -1 || hints[idx].Desc != "Edit" {
		t.Errorf(`normalHints missing the global key "e" for card.edit outside the overridden column, got: %+v`, hints)
	}
}

// --- Canonical multi-key labels (A2) ---

func TestKeymapHints_NormalMode_MultiKeyCustomActionUsesCanonicalLabel(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"z a": keymap.ActionBinding(keymap.Action{Name: "Za action", Type: "url", URL: "https://example.com/{number}"}),
		},
	}, nil)

	hints := b.normalHints
	idx := hintIndex(hints, "z a")
	if idx == -1 {
		t.Fatalf("normalHints missing the canonical multi-key label %q, got: %+v", "z a", hints)
	}
	if hints[idx].Desc != "Za action" {
		t.Errorf("hints[%d].Desc = %q, want %q", idx, hints[idx].Desc, "Za action")
	}
}

// --- Untrusted hint label sanitization (security review, #489) ---

// TestKeymapHints_ActionNameWithControlBytesSanitizedInStatusBar mirrors
// statusbar_test.go's SetTimedMessage/SetStickyMessage sanitization-sink
// tests: an inline action's Name is untrusted (user/repo config) and must be
// sanitized with sanitizeSingleLine before it reaches the hint bar, exactly
// like every other untrusted text rendered in the status bar.
func TestKeymapHints_ActionNameWithControlBytesSanitizedInStatusBar(t *testing.T) {
	rawName := "Evil\x1b[31m\nAction\x07"
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"z": keymap.ActionBinding(keymap.Action{Name: rawName, Type: "url", URL: "https://example.com/{number}"}),
		},
	}, nil)

	hints := b.normalHints
	idx := hintIndex(hints, "z")
	if idx == -1 {
		t.Fatalf("normalHints missing the %q hint, got: %+v", "z", hints)
	}
	want := sanitizeSingleLine(rawName)
	if hints[idx].Desc != want {
		t.Errorf("hints[%d].Desc = %q, want sanitized %q", idx, hints[idx].Desc, want)
	}
	if strings.ContainsAny(hints[idx].Desc, "\n\x1b\x07") {
		t.Errorf("hints[%d].Desc = %q, still contains raw control/ANSI bytes", idx, hints[idx].Desc)
	}

	view := b.statusBar.View(200)
	if strings.Count(view, "\n") != 0 {
		t.Errorf("View() = %q, want a single physical line (0 embedded newlines), got %d", view, strings.Count(view, "\n"))
	}
}

// --- Effective column overlay (#582) ---
//
// inlineActionHints must build its column-override map from the active
// column's UNFILTERED effective table (every BindingKind, via
// b.keys.Entries(mode, column)) -- not just its BindingAction entries, which
// is what actionOnlyEntries left it doing. A column that overrides a global
// action with an explicit unbind (`~`) or with a built-in command is still a
// real override -- dispatchBinding's own column-overlay lookup (the value
// b.keys.Lookup actually resolves against) honors it -- so the hint bar must
// stop advertising the stale global action the moment the column's own
// table no longer routes that key to it, exactly mirroring what dispatch
// itself does.

// TestKeymapHints_EffectiveColumnOverlay_UnbindHidesGlobalActionHintInThatColumnOnly
// covers the ticket's primary bug: a column `~`-unbinding a global action
// must hide that action's hint while the column is active, and the hint
// must reappear unchanged on a sibling column with no override -- proving
// the suppression is scoped to the overriding column, not global.
func TestKeymapHints_EffectiveColumnOverlay_UnbindHidesGlobalActionHintInThatColumnOnly(t *testing.T) {
	b := newLoadedTestBoard(t)
	if got := b.activeColumnTitle(); got != "New" {
		t.Fatalf("precondition: active column = %q, want %q", got, "New")
	}
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"Z": keymap.ActionBinding(keymap.Action{Name: "Global Z", Type: "url", URL: "https://example.com/{number}", Scope: "board"}),
		},
	}, map[string]keymap.Table{
		"new": {"Z": keymap.UnboundBinding()},
	})

	hints := b.normalHints
	if hintIndex(hints, "Z") != -1 {
		t.Errorf(`normalHints still shows "Z" while the active column ("New") unbinds it, got: %+v`, hints)
	}

	// Switching to a sibling column with no override must restore the
	// global action's hint.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if got := b.activeColumnTitle(); got == "New" {
		t.Fatalf("precondition: active column should have changed away from %q", "New")
	}
	hints = b.normalHints
	idx := hintIndex(hints, "Z")
	if idx == -1 || hints[idx].Desc != "Global Z" {
		t.Errorf(`normalHints missing "Z" -> "Global Z" outside the unbinding column, got: %+v`, hints)
	}
}

// TestKeymapHints_EffectiveColumnOverlay_CommandOverrideHidesGlobalActionHint
// covers the ticket's second non-action-marker case: a column overriding a
// global action's key with a built-in command must hide the stale action
// hint entirely (the key isn't curated into any normalHintSpecs entry, so
// no replacement hint is expected either).
func TestKeymapHints_EffectiveColumnOverlay_CommandOverrideHidesGlobalActionHint(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"Z": keymap.ActionBinding(keymap.Action{Name: "Global Z", Type: "url", URL: "https://example.com/{number}", Scope: "board"}),
		},
	}, map[string]keymap.Table{
		"new": {"Z": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	})

	hints := b.normalHints
	if hintIndex(hints, "Z") != -1 {
		t.Errorf(`normalHints still shows "Z" while the active column overrides it with a built-in command, got: %+v`, hints)
	}
	for _, h := range hints {
		if h.Desc == "Global Z" {
			t.Errorf(`normalHints still carries the stale global Desc %q after the column's "Z" now dispatches a built-in command, got: %+v`, "Global Z", hints)
		}
	}
}

// TestKeymapHints_EffectiveColumnOverlay_DifferentActionKeepsGlobalPosition
// extends TestAction_HintBar_ColumnOverride_KeepsGlobalPosition
// (actions_test.go) through the effective-binding-selection path: a column
// overriding a global action with a DIFFERENT action must keep the hint at
// its global position, with the column action's own Desc/scope winning.
func TestKeymapHints_EffectiveColumnOverlay_DifferentActionKeepsGlobalPosition(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"X": keymap.ActionBinding(keymap.Action{Name: "Global X", Type: "url", URL: "https://example.com/{number}", Scope: "board"}),
			"Y": keymap.ActionBinding(keymap.Action{Name: "Global Y", Type: "url", URL: "https://example.com/{number}", Scope: "board"}),
			"Z": keymap.ActionBinding(keymap.Action{Name: "Global Z", Type: "url", URL: "https://example.com/{number}", Scope: "board"}),
		},
	}, map[string]keymap.Table{
		"new": {"Y": keymap.ActionBinding(keymap.Action{Name: "Overridden Y", Type: "shell", Command: "echo overridden-y", Scope: "board"})},
	})

	hints := b.normalHints
	x := hintIndex(hints, "X")
	y := hintIndex(hints, "Y")
	z := hintIndex(hints, "Z")
	if x == -1 || y == -1 || z == -1 {
		t.Fatalf("expected hints for X, Y, Z; got: %+v", hints)
	}
	if x >= y || y >= z {
		t.Errorf("Y's overridden hint should keep its global position (between X and Z); got indices X=%d Y=%d Z=%d in %+v", x, y, z, hints)
	}
	if hints[y].Desc != "Overridden Y" {
		t.Errorf("Y hint Desc = %q, want %q (column override should win the value)", hints[y].Desc, "Overridden Y")
	}
}

// TestKeymapHints_EffectiveColumnOverlay_ColumnOnlyActionAppendedAfterGlobalOrder
// guards the append path through the new UNFILTERED override map: a
// column-only action key (bound only in the column's own table, never
// globally) must still be appended after every global-order key, exactly
// like #437's legacy hint-bar helper did.
func TestKeymapHints_EffectiveColumnOverlay_ColumnOnlyActionAppendedAfterGlobalOrder(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"X": keymap.ActionBinding(keymap.Action{Name: "Global X", Type: "url", URL: "https://example.com/{number}", Scope: "board"}),
			"Y": keymap.ActionBinding(keymap.Action{Name: "Global Y", Type: "url", URL: "https://example.com/{number}", Scope: "board"}),
		},
	}, map[string]keymap.Table{
		"new": {"W": keymap.ActionBinding(keymap.Action{Name: "Column-only W", Type: "url", URL: "https://example.com/{number}", Scope: "board"})},
	})

	hints := b.normalHints
	x := hintIndex(hints, "X")
	y := hintIndex(hints, "Y")
	w := hintIndex(hints, "W")
	if x == -1 || y == -1 || w == -1 {
		t.Fatalf("expected hints for X, Y, W; got: %+v", hints)
	}
	if x >= w || y >= w {
		t.Errorf("column-only action W should be appended after every global-order key (X=%d Y=%d W=%d); got: %+v", x, y, w, hints)
	}
}

// TestKeymapHints_EffectiveColumnOverlay_ScopeGateAppliesAfterEffectiveBindingSelection
// is the scope-gating regression: a column-overriding scope:pr action must
// still be gated by the selected card's LinkedPRs -- the gate must apply to
// whichever binding effective-binding-selection picked (the column's own),
// not to some binding chosen before the override was resolved. Card #1 (the
// FakeProvider's default selection in the "New" column) deliberately has no
// LinkedPRs.
func TestKeymapHints_EffectiveColumnOverlay_ScopeGateAppliesAfterEffectiveBindingSelection(t *testing.T) {
	b := newLoadedTestBoard(t)
	if got := b.selectedCard(); len(got.LinkedPRs) != 0 {
		t.Fatalf("precondition: selected card #%d has LinkedPRs, want none", got.Number)
	}
	b = boardWithOverrideKeymap(t, b, nil, map[string]keymap.Table{
		"new": {"Z": keymap.ActionBinding(keymap.Action{Name: "PR-scoped Z", Type: "url", URL: "https://example.com/{number}", Scope: "pr"})},
	})

	hints := b.normalHints
	if idx := hintIndex(hints, "Z"); idx != -1 {
		t.Errorf(`normalHints shows "Z" -> %q for a column-overriding scope:pr action while the selected card has no linked PR, got: %+v`, hints[idx].Desc, hints)
	}
}

// TestKeymapHints_EffectiveColumnOverlay_DetailFocusedReflectsUnbind is the
// ModeDetail divergence risk: inlineActionHints is called for both
// ModeNormal and ModeDetail, so the same unbind-hides-hint behavior must
// hold once focused into the detail panel too.
func TestKeymapHints_EffectiveColumnOverlay_DetailFocusedReflectsUnbind(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeDetail: {
			"Z": keymap.ActionBinding(keymap.Action{Name: "Global Z", Type: "url", URL: "https://example.com/{number}", Scope: "board"}),
		},
	}, map[string]keymap.Table{
		"new": {"Z": keymap.UnboundBinding()},
	})

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true after 'l'")
	}

	hints := b.statusBar.hints
	if hintIndex(hints, "Z") != -1 {
		t.Errorf(`detail-focused hints still show "Z" while the active column ("New") unbinds it in ModeDetail, got: %+v`, hints)
	}
}
