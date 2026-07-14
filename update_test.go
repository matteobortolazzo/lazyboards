package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// --- Quit ---

func TestQuit_Q_ReturnsQuitCmd(t *testing.T) {
	b := newLoadedTestBoard(t)
	_, cmd := b.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("'q' key should return a non-nil Cmd (tea.Quit)")
	}
}

func TestQuit_CtrlC_ReturnsQuitCmd(t *testing.T) {
	b := newLoadedTestBoard(t)
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C should return a non-nil Cmd (tea.Quit)")
	}
}

// --- Window Resize ---

func TestWindowResize_UpdatesDimensions(t *testing.T) {
	b := newLoadedTestBoard(t)
	wantWidth := 120
	wantHeight := 40
	m, _ := b.Update(tea.WindowSizeMsg{Width: wantWidth, Height: wantHeight})
	updated, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if updated.Width != wantWidth {
		t.Errorf("Width = %d, want %d", updated.Width, wantWidth)
	}
	if updated.Height != wantHeight {
		t.Errorf("Height = %d, want %d", updated.Height, wantHeight)
	}
}

// --- Status Bar ---

func TestStatusBar_HintsUpdateOnColumnSwitch(t *testing.T) {
	globalActions := map[string]config.Action{
		"X": {Name: "Global Open", Type: "url", URL: "https://global.com/{number}"},
	}
	columnConfigs := []config.ColumnConfig{
		{Name: "New"}, // No column-level actions.
		{
			Name: "Refined",
			Actions: map[string]config.Action{
				"X": {Name: "Deploy", Type: "url", URL: "https://deploy.com/{number}"},
			},
		},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, _ := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// On column 0: should show global action hint.
	view0 := b.View()
	if !strings.Contains(view0, "Global Open") {
		t.Errorf("on column 0, View() should contain %q, got:\n%s", "Global Open", view0)
	}

	// Tab to column 1: should show column-level action hint overriding global.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	view1 := b.View()
	if !strings.Contains(view1, "Deploy") {
		t.Errorf("on column 1, View() should contain %q, got:\n%s", "Deploy", view1)
	}
	if strings.Contains(view1, "Global Open") {
		t.Errorf("on column 1, View() should NOT contain %q", "Global Open")
	}

	// Shift+tab back to column 0: should show global hint again.
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	view0again := b.View()
	if !strings.Contains(view0again, "Global Open") {
		t.Errorf("back on column 0, View() should contain %q, got:\n%s", "Global Open", view0again)
	}
}

// hintIndex returns the position of the first hint with the given key, or -1.
func hintIndex(hints []Hint, key string) int {
	for i, h := range hints {
		if h.Key == key {
			return i
		}
	}
	return -1
}

func TestHelpHint_PresentAndLeftmostOnNewBoard(t *testing.T) {
	b := newTestBoard(t)

	help := hintIndex(b.normalHints, "?")
	if help == -1 {
		t.Fatalf("normalHints should contain a %q hint, got: %+v", "?", b.normalHints)
	}
	if b.normalHints[help].Desc != "Help" {
		t.Errorf("? hint Desc = %q, want %q", b.normalHints[help].Desc, "Help")
	}
	if e := hintIndex(b.normalHints, "e"); e != -1 && help > e {
		t.Errorf("? hint (index %d) should appear before e (index %d)", help, e)
	}
	if n := hintIndex(b.normalHints, "n"); n != -1 && help > n {
		t.Errorf("? hint (index %d) should appear before n (index %d)", help, n)
	}
}

func TestHelpHint_SurvivesRebuildAfterFetch(t *testing.T) {
	b := newLoadedTestBoard(t)

	help := hintIndex(b.normalHints, "?")
	if help == -1 {
		t.Fatalf("after board fetch, normalHints should contain a %q hint, got: %+v", "?", b.normalHints)
	}
	// It must be leftmost so it survives left-to-right truncation on narrow bars.
	if help != 0 {
		t.Errorf("? hint should be leftmost (index 0), got index %d: %+v", help, b.normalHints)
	}
	for _, key := range []string{"e", "n"} {
		if i := hintIndex(b.normalHints, key); i != -1 && help > i {
			t.Errorf("? hint (index %d) should appear before %q (index %d)", help, key, i)
		}
	}
}

func TestHelpHint_PresentWhenCardSelected(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)

	// Move the cursor to select a card, then rebuild hints.
	b = sendKey(t, b, keyMsg("j"))

	if hintIndex(b.normalHints, "?") == -1 {
		t.Errorf("with a card selected, normalHints should contain a %q hint, got: %+v", "?", b.normalHints)
	}
}

func TestNormalHints_TrimmedAndReorderedAfterFetch(t *testing.T) {
	b := newLoadedTestBoard(t)

	// n (New) must come before e (Edit) in the static hint ordering.
	n := hintIndex(b.normalHints, "n")
	e := hintIndex(b.normalHints, "e")
	if n == -1 {
		t.Fatalf("normalHints should contain an %q hint, got: %+v", "n", b.normalHints)
	}
	if e == -1 {
		t.Fatalf("normalHints should contain an %q hint, got: %+v", "e", b.normalHints)
	}
	if n > e {
		t.Errorf("n hint (index %d) should appear before e hint (index %d): %+v", n, e, b.normalHints)
	}

	// The conditional o/p/a/f hints must never appear in the always-visible
	// hint bar; the keybindings stay functional and remain listed in the
	// '?' Help popup, but hint-bar visibility is removed.
	for _, key := range []string{"o", "p", "a", "f"} {
		if i := hintIndex(b.normalHints, key); i != -1 {
			t.Errorf("normalHints should NOT include %q hint, found at index %d: %+v", key, i, b.normalHints)
		}
	}
}

func TestStatusBar_ColumnOnlyActionAppearsOnlyInColumn(t *testing.T) {
	// No global action for "X".
	globalActions := map[string]config.Action{}
	columnConfigs := []config.ColumnConfig{
		{
			Name: "New",
			Actions: map[string]config.Action{
				"X": {Name: "Special", Type: "url", URL: "https://special.com/{number}"},
			},
		},
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, _ := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// On column 0: should show the column-only action hint.
	view0 := b.View()
	if !strings.Contains(view0, "Special") {
		t.Errorf("on column 0, View() should contain %q, got:\n%s", "Special", view0)
	}

	// Tab to column 1: should NOT show the column-only action hint.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	view1 := b.View()
	if strings.Contains(view1, "Special") {
		t.Errorf("on column 1, View() should NOT contain %q", "Special")
	}
}
