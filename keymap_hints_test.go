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
