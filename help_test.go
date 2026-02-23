package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
)

// --- Help Mode: Open/Close ---

func TestHelpMode_QuestionMark_OpensHelp(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))

	if b.mode != helpMode {
		t.Errorf("after '?': mode = %d, want %d (helpMode)", b.mode, helpMode)
	}
}

func TestHelpMode_QuestionMark_OpensFromDetailFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Enter detail focus first.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	b = sendKey(t, b, keyMsg("?"))

	if b.mode != helpMode {
		t.Errorf("after '?' from detail focus: mode = %d, want %d (helpMode)", b.mode, helpMode)
	}
	if !b.helpFromDetailFocused {
		t.Error("helpFromDetailFocused should be true when opened from detail focus")
	}
}

func TestHelpMode_Escape_ClosesToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	if b.mode != helpMode {
		t.Fatal("precondition: mode should be helpMode")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Esc in helpMode: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestHelpMode_QuestionMark_TogglesClose(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	if b.mode != helpMode {
		t.Fatal("precondition: mode should be helpMode")
	}

	// Press ? again to close.
	b = sendKey(t, b, keyMsg("?"))

	if b.mode != normalMode {
		t.Errorf("after second '?': mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

func TestHelpMode_Escape_RestoresDetailFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Open from detail focus.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("?"))
	if b.mode != helpMode {
		t.Fatal("precondition: mode should be helpMode")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.mode != normalMode {
		t.Errorf("after Esc: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
	if !b.detailFocused {
		t.Error("after Esc: detailFocused should be restored to true")
	}
}

func TestHelpMode_QuestionMark_RestoresDetailFocused(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Open from detail focus.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("?"))

	// Close with ?.
	b = sendKey(t, b, keyMsg("?"))

	if !b.detailFocused {
		t.Error("after '?' close: detailFocused should be restored to true")
	}
}

// --- Help Mode: Ignored in Other Modes ---

func TestHelpMode_IgnoredInOtherModes(t *testing.T) {
	tests := []struct {
		name string
		mode boardMode
	}{
		{"createMode", createMode},
		{"creatingMode", creatingMode},
		{"configMode", configMode},
		{"loadingMode", loadingMode},
		{"errorMode", errorMode},
		{"prPickerMode", prPickerMode},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newLoadedTestBoard(t)
			b.Width = 120
			b.Height = 40
			b.mode = tt.mode

			b = sendKey(t, b, keyMsg("?"))

			if b.mode == helpMode {
				t.Errorf("pressing '?' in %s should not open helpMode", tt.name)
			}
		})
	}
}

// --- Help Mode: Scroll ---

func TestHelpMode_JKey_ScrollsDown(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	b = sendKey(t, b, keyMsg("j"))

	if b.helpScrollOffset < 1 {
		t.Errorf("helpScrollOffset = %d after 'j', want >= 1", b.helpScrollOffset)
	}
}

func TestHelpMode_KKey_ScrollsUp(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	offsetAfterDown := b.helpScrollOffset

	b = sendKey(t, b, keyMsg("k"))

	if b.helpScrollOffset >= offsetAfterDown {
		t.Errorf("helpScrollOffset = %d after 'k', want less than %d", b.helpScrollOffset, offsetAfterDown)
	}
}

func TestHelpMode_KKey_ClampsAtZero(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	b = sendKey(t, b, keyMsg("k"))

	if b.helpScrollOffset != 0 {
		t.Errorf("helpScrollOffset = %d after 'k' at offset 0, want 0", b.helpScrollOffset)
	}
}

func TestHelpMode_JKey_ClampsAtMaxOffset(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 20 // Small height so help content exceeds visible area.

	b = sendKey(t, b, keyMsg("?"))

	// Scroll down many times — should clamp at max offset.
	for i := 0; i < 200; i++ {
		b = sendKey(t, b, keyMsg("j"))
	}

	maxOffset := b.helpMaxScrollOffset()
	if b.helpScrollOffset != maxOffset {
		t.Errorf("helpScrollOffset = %d after excessive scrolling, want %d (maxOffset)", b.helpScrollOffset, maxOffset)
	}
	// Pressing k once from max offset should immediately respond.
	b = sendKey(t, b, keyMsg("k"))
	if b.helpScrollOffset != maxOffset-1 {
		t.Errorf("helpScrollOffset = %d after single 'k' from max, want %d", b.helpScrollOffset, maxOffset-1)
	}
}

// --- Help Mode: Quit ---

func TestHelpMode_QKey_Quits(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))

	_, cmd := b.Update(keyMsg("q"))
	if cmd == nil {
		t.Error("pressing 'q' in helpMode should return a non-nil cmd (tea.Quit)")
	}
}

func TestHelpMode_CtrlC_Quits(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))

	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("pressing ctrl+c in helpMode should return a non-nil cmd (tea.Quit)")
	}
}

// --- Help Mode: Blocks Navigation ---

func TestHelpMode_BlocksNavigation(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	initialTab := b.ActiveTab
	initialCursor := b.Columns[b.ActiveTab].Cursor

	b = sendKey(t, b, keyMsg("?"))

	// Try various navigation keys.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, keyMsg("r"))

	if b.mode != helpMode {
		t.Errorf("navigation keys should not change mode in helpMode, got %d", b.mode)
	}
	if b.ActiveTab != initialTab {
		t.Errorf("ActiveTab changed from %d to %d in helpMode", initialTab, b.ActiveTab)
	}
	if b.Columns[b.ActiveTab].Cursor != initialCursor {
		t.Errorf("Cursor changed from %d to %d in helpMode", initialCursor, b.Columns[b.ActiveTab].Cursor)
	}
}

// --- Help Mode: Scroll Reset on Reopen ---

func TestHelpMode_ScrollResetOnReopen(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	// Open help and scroll down.
	b = sendKey(t, b, keyMsg("?"))
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	if b.helpScrollOffset == 0 {
		t.Fatal("precondition: helpScrollOffset should be > 0 after scrolling")
	}

	// Close help.
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	// Reopen help — scroll offset should be reset to 0.
	b = sendKey(t, b, keyMsg("?"))
	if b.helpScrollOffset != 0 {
		t.Errorf("helpScrollOffset = %d after reopening help, want 0", b.helpScrollOffset)
	}
}

// --- Help Mode: View ---

func TestHelpMode_ViewShowsKeybindings(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	expectedTexts := []string{
		"Quit",
		"New card",
		"Help",
		"Normal Mode",
		"Detail Panel",
		"Create Card",
		"Configuration",
	}
	for _, text := range expectedTexts {
		if !strings.Contains(view, text) {
			t.Errorf("View() in helpMode should contain %q", text)
		}
	}
}

func TestHelpMode_ViewShowsCustomActions(t *testing.T) {
	actions := map[string]config.Action{
		"x": {Name: "Deploy App", Type: "url", URL: "https://example.com/{number}"},
	}
	b, _ := newActionTestBoard(t, actions)
	b.Height = 80

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	if !strings.Contains(view, "Custom Actions") {
		t.Error("View() in helpMode should contain 'Custom Actions' section header")
	}
	if !strings.Contains(view, "Deploy App") {
		t.Error("View() in helpMode should contain custom action name 'Deploy App'")
	}
}

func TestHelpMode_ViewShowsColumnActions(t *testing.T) {
	globalActions := map[string]config.Action{}
	columnConfigs := []config.ColumnConfig{
		{
			Name: "New",
			Actions: map[string]config.Action{
				"d": {Name: "Deploy Column", Type: "url", URL: "https://deploy.com/{number}"},
			},
		},
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, _ := newColumnActionTestBoard(t, globalActions, columnConfigs)
	b.Height = 80

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	if !strings.Contains(view, "Deploy Column") {
		t.Error("View() in helpMode should contain column action name 'Deploy Column'")
	}
}

func TestHelpMode_ViewShowsUsageSection(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 80

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	if !strings.Contains(view, "Usage") {
		t.Error("View() in helpMode should contain 'Usage' section")
	}
	if !strings.Contains(view, ".lazyboards.yml") {
		t.Error("View() in helpMode should mention config file name")
	}
}

func TestHelpMode_StatusBarShowsHints(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("?"))
	view := b.View()

	if !strings.Contains(view, "Close") {
		t.Errorf("View() in helpMode should contain hint desc %q", "Close")
	}
	if !strings.Contains(view, "Scroll") {
		t.Errorf("View() in helpMode should contain hint desc %q", "Scroll")
	}
}
