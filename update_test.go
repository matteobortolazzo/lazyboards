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
