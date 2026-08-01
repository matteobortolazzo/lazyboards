package main

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
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
	b := NewBoard(nil, nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, true, nil, nil, true)
	if b.mode != configMode {
		t.Errorf("mode = %d, want %d (configMode) for firstLaunch board", b.mode, configMode)
	}
}

func TestFirstLaunch_PrePopulatesProvider(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, true, nil, nil, true)
	if b.config.providerOptions[b.config.providerIndex] != "github" {
		t.Errorf("providerOptions[providerIndex] = %q, want %q", b.config.providerOptions[b.config.providerIndex], "github")
	}
}

func TestFirstLaunch_PrePopulatesProviderAzure(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, nil, "owner", "repo", "azure-devops", 0, 0, "Working", false, true, nil, nil, true)
	if b.config.providerOptions[b.config.providerIndex] != "azure-devops" {
		t.Errorf("providerOptions[providerIndex] = %q, want %q", b.config.providerOptions[b.config.providerIndex], "azure-devops")
	}
}

func TestFirstLaunch_PrePopulatesRepo(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, nil, "myowner", "myrepo", "github", 0, 0, "Working", false, true, nil, nil, true)
	if b.config.repoInput.Value() != "myowner/myrepo" {
		t.Errorf("repoInput.Value() = %q, want %q", b.config.repoInput.Value(), "myowner/myrepo")
	}
}

func TestFirstLaunch_EmptyRepoNotPrePopulated(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, nil, "", "", "github", 0, 0, "Working", false, true, nil, nil, true)
	if b.config.repoInput.Value() != "" {
		t.Errorf("repoInput.Value() = %q, want empty when no repo detected", b.config.repoInput.Value())
	}
}

func TestFirstLaunch_Init_ReturnsNil(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, true, nil, nil, true)
	cmd := b.Init()
	if cmd != nil {
		t.Error("Init() should return nil for firstLaunch board (no fetch)")
	}
}

func TestFirstLaunch_Escape_Quits(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, true, nil, nil, true)
	_, cmd := b.Update(arrowMsg(tea.KeyEsc))
	if cmd == nil {
		t.Error("Escape in firstLaunch configMode should return a quit cmd")
	}
}

func TestFirstLaunch_Escape_ConfigSavedIsFalse(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, true, nil, nil, true)
	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	updated := m.(Board)
	if updated.config.configSaved {
		t.Error("ConfigSaved should be false after Escape in firstLaunch")
	}
}

func TestFirstLaunch_ConfigSaved_SetsConfigSavedAndQuits(t *testing.T) {
	b := NewBoard(nil, nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, true, nil, nil, true)
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
	b := NewBoard(nil, nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, true, nil, nil, true)
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
	b := NewBoard(p, nil, nil, nil, nil, "owner", "repo", "github", 0, 0, "Working", false, false, nil, nil, true)
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
	b := NewBoard(p, nil, nil, nil, nil, "myowner", "myrepo", "github", 0, 0, "Working", false, false, nil, nil, true)
	board, _ := p.FetchBoard(context.TODO())
	m, _ := b.Update(boardFetchedMsg{board: board})
	b = m.(Board)

	// Press "c" to enter configMode.
	b = sendKey(t, b, keyMsg("c"))
	if b.config.repoInput.Value() != "myowner/myrepo" {
		t.Errorf("repoInput.Value() = %q after 'c', want %q", b.config.repoInput.Value(), "myowner/myrepo")
	}
}

// --- Route config mode through the registry (#540 PR 1/2) ---
//
// handleConfigModeKey cuts over from a hardcoded `switch msg.Type` to
// keymap.Keymap.Lookup against ModeConfig (textBinding, keymap_text.go).
// Unlike create mode, config's left/right are dedicated top-level cases
// today (not inside a `default:` fallback that forwards to the focused
// input) -- see TestConfigMode_LeftRight_AtRepoFocus_DoesNotReachInput below
// for the deliberate behavior-parity decision this preserves.

// TestConfigMode_LeftRight_AtRepoFocus_DoesNotReachInput is the named
// regression guard for the #540 plan's top config-specific risk (Q&A #2):
// AC #1 (strict parity with today's behavior) wins over AC #4's generic
// "otherwise reach the focused textinput/textarea as cursor movement"
// wording. Today's handleConfigModeKey handles tea.KeyLeft/tea.KeyRight as
// dedicated top-level cases that return (b, nil) whenever focus != 0
// (mode_handlers.go), unlike create mode's `default:` branch which DOES
// forward them to the focused input at focus 0/1. This asymmetry is
// intentional and must NOT be "corrected" to match create -- left/right
// must stay swallowed at config's repo-input focus, matching the
// pre-registry handler exactly.
func TestConfigMode_LeftRight_AtRepoFocus_DoesNotReachInput(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // provider -> repo, focus == 1
	if b.config.focus != 1 {
		t.Fatalf("precondition: config.focus = %d, want 1 (repo)", b.config.focus)
	}
	for _, ch := range "owner/repo" {
		b = sendKey(t, b, keyMsg(string(ch)))
	}
	before := b.config.repoInput.Value()
	if before != "owner/repo" {
		t.Fatalf("precondition: repoInput.Value() = %q, want %q", before, "owner/repo")
	}

	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if got := b.config.repoInput.Value(); got != before {
		t.Errorf("repoInput.Value() = %q after left at repo focus, want unchanged %q (left/right must stay swallowed, never reach repoInput)", got, before)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if got := b.config.repoInput.Value(); got != before {
		t.Errorf("repoInput.Value() = %q after right at repo focus, want unchanged %q (left/right must stay swallowed, never reach repoInput)", got, before)
	}
}

// --- left/right cycle the provider only at focus 0 ---

func TestConfigMode_LeftRight_NoOpAtRepoFocus_ProviderIndexUnchanged(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // provider -> repo, focus == 1
	before := b.config.providerIndex

	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.config.providerIndex != before {
		t.Errorf("providerIndex = %d after right at repo focus, want unchanged %d (provider cycle is gated on focus == 0)", b.config.providerIndex, before)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.config.providerIndex != before {
		t.Errorf("providerIndex = %d after left at repo focus, want unchanged %d (provider cycle is gated on focus == 0)", b.config.providerIndex, before)
	}
}

// --- keymaps.config remap + explicit unbind ---

func TestConfigMode_RemapCancelKey_DispatchAndHintStaySync(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeConfig: {
			"esc": keymap.UnboundBinding(),
			"f1":  keymap.CommandBinding(keymap.CommandConfigCancel),
		},
	}, nil)

	before := b.mode
	m, cmd := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != before {
		t.Errorf("mode after unbound esc = %v, want unchanged (%v)", b2.mode, before)
	}
	if cmd != nil {
		t.Error("unbound esc should not fire a cmd")
	}

	m, cmd = b2.Update(arrowMsg(tea.KeyF1))
	b3, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b3.mode != normalMode {
		t.Errorf("mode after remapped 'f1' = %v, want normalMode", b3.mode)
	}
	if cmd != nil {
		t.Error("cancel via remapped 'f1' should not fire a cmd, matching today's esc-cancel behavior")
	}
}

// TestConfigMode_UnbindProviderCycleKeys_LeftRightNoOpAtFocus0 encodes AC #2
// for config's provider-cycle commands: unlike create's assignee cycle
// (which sits inside a `default:` fallback that also handles text-input
// forwarding), config's left/right are dedicated top-level cases with no
// ambiguity -- an explicit unbind must make them a genuine no-op at
// focus 0, exactly like the built-in's old key ceasing to work anywhere
// else in the registry.
func TestConfigMode_UnbindProviderCycleKeys_LeftRightNoOpAtFocus0(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeConfig: {
			"left":  keymap.UnboundBinding(),
			"right": keymap.UnboundBinding(),
		},
	}, nil)
	if b.config.focus != 0 {
		t.Fatalf("precondition: config.focus = %d, want 0", b.config.focus)
	}
	before := b.config.providerIndex

	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.config.providerIndex != before {
		t.Errorf("providerIndex = %d after unbound right, want unchanged %d", b.config.providerIndex, before)
	}
	b = sendKey(t, b, arrowMsg(tea.KeyLeft))
	if b.config.providerIndex != before {
		t.Errorf("providerIndex = %d after unbound left, want unchanged %d", b.config.providerIndex, before)
	}
}

// TestConfigMode_RemapProviderCycleKeys_OldKeysNoLongerCycleNewKeysDo is
// UnbindProviderCycleKeys' remap sibling.
func TestConfigMode_RemapProviderCycleKeys_OldKeysNoLongerCycleNewKeysDo(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeConfig: {
			"left":  keymap.UnboundBinding(),
			"right": keymap.UnboundBinding(),
			"f1":    keymap.CommandBinding(keymap.CommandConfigProviderPrev),
			"f2":    keymap.CommandBinding(keymap.CommandConfigProviderNext),
		},
	}, nil)
	before := b.config.providerIndex

	b = sendKey(t, b, arrowMsg(tea.KeyRight))
	if b.config.providerIndex != before {
		t.Errorf("providerIndex = %d after old (now unbound) right, want unchanged %d", b.config.providerIndex, before)
	}

	b = sendKey(t, b, arrowMsg(tea.KeyF2))
	if b.config.providerIndex == before {
		t.Error("providerIndex unchanged after remapped 'f2', want it to cycle forward")
	}
}

// TestConfigMode_EscBoundToAction_FallsThroughToTextinputNotDispatched and
// TestConfigMode_ForeignCommandIDBoundToKey_FallsThroughToTextinputNotDispatched
// cover two of handleConfigModeKey's four documented fall-through outcomes
// (mirroring handleDeleteModeKey's TestDeleteMode_CommentStep_EscBoundToAction_
// FallsThroughToTextinputNotDispatched /
// TestDeleteMode_CommentStep_ForeignCommandIDBoundToKey_
// FallsThroughToTextinputNotDispatched, delete_mode_test.go): a resolved
// non-command BindingAction, and a command id valid elsewhere in the catalog
// but foreign to ModeConfig's own five ids. Exercised at focus == 1 (repo
// input focused) so a wrongly-dispatched action/foreign command would be
// distinguishable from the textinput-forwarding fallback.
func TestConfigMode_EscBoundToAction_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // provider -> repo, focus == 1
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeConfig: {
			"esc": keymap.ActionBinding(keymap.Action{Name: "Noop", Type: "url", URL: "https://example.com/{number}"}),
		},
	}, nil)
	before := b.config.repoInput.Value()

	m, _ := b.Update(arrowMsg(tea.KeyEsc))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != configMode {
		t.Errorf("mode after esc resolved to a non-command action = %v, want unchanged configMode (must not dispatch the action)", b2.mode)
	}
	if got := b2.config.repoInput.Value(); got != before {
		t.Errorf("repoInput.Value() = %q after esc resolved to a non-command action, want unchanged %q (falls through to the textinput)", got, before)
	}
}

func TestConfigMode_ForeignCommandIDBoundToKey_FallsThroughToTextinputNotDispatched(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = sendKey(t, b, arrowMsg(tea.KeyTab)) // provider -> repo, focus == 1
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeConfig: {
			"f1": keymap.CommandBinding(keymap.CommandCloseConfirmConfirm),
		},
	}, nil)
	before := b.config.repoInput.Value()

	m, _ := b.Update(arrowMsg(tea.KeyF1))
	b2, ok := m.(Board)
	if !ok {
		t.Fatalf("Update returned %T, want Board", m)
	}
	if b2.mode != configMode {
		t.Errorf("mode after a foreign command id (close_confirm.confirm) bound into ModeConfig = %v, want unchanged configMode", b2.mode)
	}
	if got := b2.config.repoInput.Value(); got != before {
		t.Errorf("repoInput.Value() = %q after a foreign command id bound into ModeConfig, want unchanged %q (falls through to the textinput, not dispatched)", got, before)
	}
}

// --- configModalHints(): byte-identity, remap ---

func TestConfigModalHints_DefaultParityMatchesTodaysLiteral(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))

	hints := b.configModalHints()
	want := []Hint{
		{Key: "esc", Desc: "Cancel"},
		{Key: "tab", Desc: "Next"},
		{Key: "enter", Desc: "Save"},
	}
	if !reflect.DeepEqual(hints, want) {
		t.Errorf("configModalHints() = %+v, want %+v (today's inline literal, byte-identical)", hints, want)
	}
}

func TestConfigModalHints_RemapSaveKey_ReflectsNewKey(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeConfig: {
			"enter": keymap.UnboundBinding(),
			"f5":    keymap.CommandBinding(keymap.CommandConfigSave),
		},
	}, nil)

	hints := b.configModalHints()
	if got := hintDesc(t, hints, "f5"); got != "Save" {
		t.Errorf("configModalHints() after remap: Desc for %q = %q, want %q", "f5", got, "Save")
	}
	for _, h := range hints {
		if h.Key == "enter" {
			t.Errorf("configModalHints() still advertises the old 'enter' key after remap, got %+v", hints)
		}
	}
}

// --- hint<->dispatch invariant across both focus positions ---

// TestConfigModalHints_KeysAlwaysDispatch_BothFocusPositionsDefaultAndRemappedTables
// is the hint<->dispatch invariant test named in the #540 plan's Explicit
// Risk Coverage, adapted to config's two focus positions (provider/repo):
// for every key advertised by b.configModalHints(), it must resolve
// through b.textBinding -- the real dispatch path handleConfigModeKey uses
// -- against ModeConfig. Run against both the default table and a
// remapped/unbound table.
func TestConfigModalHints_KeysAlwaysDispatch_BothFocusPositionsDefaultAndRemappedTables(t *testing.T) {
	keyMsgForLabel := func(label string) tea.KeyMsg {
		switch label {
		case "enter":
			return arrowMsg(tea.KeyEnter)
		case "esc":
			return arrowMsg(tea.KeyEsc)
		case "tab":
			return arrowMsg(tea.KeyTab)
		case "f1":
			return arrowMsg(tea.KeyF1)
		case "f2":
			return arrowMsg(tea.KeyF2)
		default:
			return keyMsg(label)
		}
	}

	tests := []struct {
		name  string
		modes map[keymap.Mode]keymap.Table
	}{
		{name: "default table", modes: nil},
		{
			name: "remapped table",
			modes: map[keymap.Mode]keymap.Table{
				keymap.ModeConfig: {
					"esc": keymap.UnboundBinding(),
					"f1":  keymap.CommandBinding(keymap.CommandConfigCancel),
					"tab": keymap.UnboundBinding(),
					"f2":  keymap.CommandBinding(keymap.CommandConfigNextField),
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			b := newLoadedTestBoard(t)
			b = sendKey(t, b, keyMsg("c"))
			if tc.modes != nil {
				b = boardWithOverrideKeymap(t, b, tc.modes, nil)
			}

			for _, focus := range []int{0, 1} {
				b.config.focus = focus
				for _, h := range b.configModalHints() {
					if h.Key == "" {
						continue
					}
					for _, key := range strings.Split(h.Key, "/") {
						if _, ok := b.textBinding(keymap.ModeConfig, keyMsgForLabel(key)); !ok {
							t.Errorf("focus %d, hint %+v: textBinding(ModeConfig, %q) not found, want a match (every advertised key must actually dispatch through the real handler)", focus, h, key)
						}
					}
				}
			}
		})
	}
}

// --- ctrl+c always quits, regardless of user keymap config ---

func TestConfigMode_CtrlCQuits_EvenWithOverriddenKeymap(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = sendKey(t, b, keyMsg("c"))
	// Attempt to steal ctrl+c for something else -- update.go's global
	// ctrl+c-always-quits check runs before any mode dispatch, so this must
	// have no effect.
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeConfig: {"ctrl+c": keymap.CommandBinding(keymap.CommandConfigSave)},
	}, nil)

	_, cmd := b.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Error("Ctrl+C in configMode should return a non-nil Cmd (tea.Quit), even with an overridden keymap")
	}
}
