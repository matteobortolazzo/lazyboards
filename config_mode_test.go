package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func TestConfigMode_C_EntersConfigMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	if b.mode != configMode {
		t.Errorf("after 'c': mode = %d, want %d (configMode)", b.mode, configMode)
	}
}

func TestConfigMode_Escape_ReturnsToNormalMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))
	if b.mode != normalMode {
		t.Errorf("after 'c' then Escape: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}
}

// --- Config Mode: View Rendering ---

func TestConfigMode_ViewShowsConfigurationHeader(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("c"))
	view := b.View()
	if !strings.Contains(view, "Configuration") {
		t.Error("View() in configMode should contain 'Configuration'")
	}
}

func TestConfigMode_ViewShowsProviderField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("c"))
	view := b.View()
	if !strings.Contains(view, "Provider") {
		t.Error("View() in configMode should contain 'Provider' label")
	}
	if !strings.Contains(view, "github") {
		t.Error("View() in configMode should show 'github' as a provider option")
	}
}

func TestConfigMode_ViewShowsRepoField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("c"))
	view := b.View()
	if !strings.Contains(view, "Repo") {
		t.Error("View() in configMode should contain 'Repo' label")
	}
}

// --- Config Mode: Provider Cycling ---

func TestConfigMode_LeftRight_CyclesProvider(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Record initial provider index.
	initialIndex := b.config.providerIndex

	// Press Right to cycle to next provider.
	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.config.providerIndex == initialIndex {
		t.Error("Right arrow in configMode should change providerIndex")
	}
}

func TestConfigMode_ProviderWrapsAround(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Cycle through all providers and one more to wrap around.
	totalProviders := len(b.config.providerOptions)
	for i := 0; i < totalProviders; i++ {
		b = sendKey(t, b, arrowMsg(tea.KeyRight))
	}

	// Should wrap back to the first provider.
	if b.config.providerIndex != 0 {
		t.Errorf("providerIndex = %d after wrapping around %d providers, want 0", b.config.providerIndex, totalProviders)
	}
}

// --- Config Mode: Tab Navigation ---

func TestConfigMode_TabSwitchesFocus(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Initially focus should be on provider field (configFocus == 0).
	if b.config.focus != 0 {
		t.Errorf("configFocus = %d on entering configMode, want 0 (provider field)", b.config.focus)
	}

	// Tab should switch focus to repo field.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.config.focus != 1 {
		t.Errorf("configFocus = %d after Tab, want 1 (repo field)", b.config.focus)
	}

	// Another Tab should switch back to provider field.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.config.focus != 0 {
		t.Errorf("configFocus = %d after second Tab, want 0 (provider field)", b.config.focus)
	}
}

// --- Config Mode: Typing ---

func TestConfigMode_TypingUpdatesRepoField(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Tab to repo field.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))

	// Type characters.
	for _, ch := range "owner/repo" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	if b.config.repoInput.Value() != "owner/repo" {
		t.Errorf("repoInput.Value() = %q, want %q", b.config.repoInput.Value(), "owner/repo")
	}
}

// --- Config Mode: Save (Enter) ---

func TestConfigMode_Enter_TriggersConfigSave(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Tab to repo field and type a value.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	for _, ch := range "owner/repo" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to save.
	_, cmd := b.Update(arrowMsg(tea.KeyEnter))

	// Enter should trigger a save command (async).
	if cmd == nil {
		t.Error("Enter in configMode should return a non-nil cmd (config save)")
	}
}

func TestConfigMode_ConfigSaved_TransitionsToLoadingMode(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Send configSavedMsg to simulate successful save.
	m, cmd := b.Update(configSavedMsg{})
	b = m.(Board)

	// Should transition to loadingMode (auto-refresh after save).
	if b.mode != loadingMode {
		t.Errorf("mode = %d after configSavedMsg, want %d (loadingMode)", b.mode, loadingMode)
	}

	// Should return a cmd for fetching the board.
	if cmd == nil {
		t.Error("configSavedMsg should return a non-nil cmd (fetch board)")
	}
}

func TestConfigMode_ConfigSaveError_ShowsValidationError(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Send configSaveErrorMsg to simulate save failure.
	m, _ := b.Update(configSaveErrorMsg{err: errors.New("permission denied")})
	b = m.(Board)

	// Should stay in configMode.
	if b.mode != configMode {
		t.Errorf("mode = %d after configSaveErrorMsg, want %d (configMode)", b.mode, configMode)
	}

	// Should show the error.
	if !strings.Contains(b.validationErr, "permission denied") {
		t.Errorf("validationErr = %q, want it to contain %q", b.validationErr, "permission denied")
	}
}

// --- Config Mode: Blocking ---

func TestConfigMode_BlocksNavigation(t *testing.T) {
	b := newLoadedTestBoard(t)
	requireColumns(t, b)
	b = sendKey(t, b, keyMsg("c"))

	origTab := b.ActiveTab
	origCursor := b.Columns[b.ActiveTab].Cursor

	// h, l should NOT navigate the board tabs.
	b = sendKey(t, b, arrowMsg(tea.KeyShiftTab))
	if b.ActiveTab != origTab {
		t.Errorf("'h' in configMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	if b.ActiveTab != origTab {
		t.Errorf("'l' in configMode changed ActiveTab to %d, want %d", b.ActiveTab, origTab)
	}

	// j, k should NOT move the card cursor.
	b = sendKey(t, b, keyMsg("j"))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("'j' in configMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
	b = sendKey(t, b, keyMsg("k"))
	if b.Columns[b.ActiveTab].Cursor != origCursor {
		t.Errorf("'k' in configMode changed cursor to %d, want %d", b.Columns[b.ActiveTab].Cursor, origCursor)
	}
}

func TestConfigMode_BlocksQuit(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	m, _ := b.Update(keyMsg("q"))
	updated := m.(Board)
	// q should NOT quit while in configMode.
	if updated.mode != configMode {
		t.Errorf("'q' in configMode changed mode to %d, want %d (configMode)", updated.mode, configMode)
	}
}

func TestConfigMode_CtrlC_StillQuits(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in configMode should return a non-nil Cmd (tea.Quit)")
	}
}

// --- Config Mode: Status Bar ---

func TestConfigMode_StatusBarShowsHints(t *testing.T) {
	b := newLoadedTestBoard(t)
	b.Width = 120
	b.Height = 40
	b = sendKey(t, b, keyMsg("c"))
	view := b.View()

	expectedHints := []string{"esc", "tab", "enter"}
	for _, hint := range expectedHints {
		if !strings.Contains(strings.ToLower(view), hint) {
			t.Errorf("View() in configMode should contain hint %q", hint)
		}
	}
}

func TestConfigMode_EnterWithEmptyRepo_ShowsValidationError(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	// Press Enter without typing a repo value.
	m, _ := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)

	// Should stay in configMode with a validation error.
	if b.mode != configMode {
		t.Errorf("mode = %d, want %d (configMode) when repo is empty", b.mode, configMode)
	}
	if !strings.Contains(b.validationErr, "Repository is required") {
		t.Errorf("validationErr = %q, want it to contain %q", b.validationErr, "Repository is required")
	}
}

// --- First Launch ---

func TestFirstLaunch_StartsInConfigMode(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "github", 0, 0, 0, "Working", false, true)
	if b.mode != configMode {
		t.Errorf("mode = %d, want %d (configMode) for firstLaunch board", b.mode, configMode)
	}
}

func TestFirstLaunch_PrePopulatesProvider(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "github", 0, 0, 0, "Working", false, true)
	if b.config.providerOptions[b.config.providerIndex] != "github" {
		t.Errorf("providerOptions[providerIndex] = %q, want %q", b.config.providerOptions[b.config.providerIndex], "github")
	}
}

func TestFirstLaunch_PrePopulatesProviderAzure(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "azure-devops", 0, 0, 0, "Working", false, true)
	if b.config.providerOptions[b.config.providerIndex] != "azure-devops" {
		t.Errorf("providerOptions[providerIndex] = %q, want %q", b.config.providerOptions[b.config.providerIndex], "azure-devops")
	}
}

func TestFirstLaunch_PrePopulatesRepo(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "myowner", "myrepo", "github", 0, 0, 0, "Working", false, true)
	if b.config.repoInput.Value() != "myowner/myrepo" {
		t.Errorf("repoInput.Value() = %q, want %q", b.config.repoInput.Value(), "myowner/myrepo")
	}
}

func TestFirstLaunch_EmptyRepoNotPrePopulated(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "", "", "github", 0, 0, 0, "Working", false, true)
	if b.config.repoInput.Value() != "" {
		t.Errorf("repoInput.Value() = %q, want empty when no repo detected", b.config.repoInput.Value())
	}
}

func TestFirstLaunch_Init_ReturnsNil(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "github", 0, 0, 0, "Working", false, true)
	cmd := b.Init()
	if cmd != nil {
		t.Error("Init() should return nil for firstLaunch board (no fetch)")
	}
}

func TestFirstLaunch_Escape_Quits(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "github", 0, 0, 0, "Working", false, true)
	_, cmd := b.Update(arrowMsg(tea.KeyEsc))
	if cmd == nil {
		t.Error("Escape in firstLaunch configMode should return a quit cmd")
	}
}

func TestFirstLaunch_Escape_ConfigSavedIsFalse(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "github", 0, 0, 0, "Working", false, true)
	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	updated := m.(Board)
	if updated.config.configSaved {
		t.Error("ConfigSaved should be false after Escape in firstLaunch")
	}
}

func TestFirstLaunch_ConfigSaved_SetsConfigSavedAndQuits(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "github", 0, 0, 0, "Working", false, true)
	m, cmd := b.Update(configSavedMsg{})
	updated := m.(Board)
	if !updated.config.configSaved {
		t.Error("ConfigSaved should be true after configSavedMsg in firstLaunch")
	}
	if cmd == nil {
		t.Error("configSavedMsg in firstLaunch should return a quit cmd")
	}
}

func TestFirstLaunch_ViewShowsConfigModal(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, "owner", "repo", "github", 0, 0, 0, "Working", false, true)
	b.Width = 120
	b.Height = 40
	view := b.View()
	if !strings.Contains(view, "Configuration") {
		t.Error("View() in firstLaunch should show Configuration modal")
	}
}

// --- Config Mode: Pre-populate from runtime (normal "c" key) ---

func TestConfigMode_PrePopulatesProviderFromRuntime(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "owner", "repo", "github", 0, 0, 0, "Working", false, false)
	board, _ := p.FetchBoard(context.TODO())
	m, _ := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// Press "c" to enter configMode.
	b = sendKey(t, b, keyMsg("c"))
	if b.config.providerOptions[b.config.providerIndex] != "github" {
		t.Errorf("providerOptions[providerIndex] = %q after 'c', want %q", b.config.providerOptions[b.config.providerIndex], "github")
	}
}

func TestConfigMode_PrePopulatesRepoFromRuntime(t *testing.T) {
	p := provider.NewFakeProvider()
	b := NewBoard(p, nil, nil, nil, "myowner", "myrepo", "github", 0, 0, 0, "Working", false, false)
	board, _ := p.FetchBoard(context.TODO())
	m, _ := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// Press "c" to enter configMode.
	b = sendKey(t, b, keyMsg("c"))
	if b.config.repoInput.Value() != "myowner/myrepo" {
		t.Errorf("repoInput.Value() = %q after 'c', want %q", b.config.repoInput.Value(), "myowner/myrepo")
	}
}
