package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func TestAction_URLTriggersOpenURL(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Get the selected card's number to verify expansion.
	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedURL := fmt.Sprintf("https://example.com/%d", selectedCard.Number)

	// Press the action key in normalMode.
	b = sendKey(t, b, keyMsg("o"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

func TestAction_ShellTriggersRunShell(t *testing.T) {
	actions := map[string]config.Action{
		"s": {Name: "Shell", Type: "shell", Command: "echo {title}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Get the selected card's title (slugified) to verify expansion.
	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedCmd := "echo " + action.ShellEscape(action.Slugify(selectedCard.Title))

	// Press the action key in normalMode -- shell runs async via tea.Cmd.
	m, cmd := b.Update(keyMsg("s"))
	b = m.(Board)
	_ = b

	// Execute the returned cmd(s) to trigger RunShell.
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called, but no calls recorded")
	}
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
}

func TestAction_IgnoredInCreateMode(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter createMode, then press the action key.
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, keyMsg("o"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls in createMode, got %d", len(fe.OpenURLCalls))
	}
}

func TestAction_IgnoredInLoadingMode(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, nil, fe, "", "", "", 0, false)

	// Board starts in loadingMode. Press the action key.
	b = sendKey(t, b, keyMsg("o"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls in loadingMode, got %d", len(fe.OpenURLCalls))
	}
}

func TestAction_IgnoredWhenNoCards(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, nil, fe, "", "", "", 0, false)

	// Load a board with an empty column.
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Empty", Cards: nil},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Press the action key with no cards in the column.
	b = sendKey(t, b, keyMsg("o"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls when no cards, got %d", len(fe.OpenURLCalls))
	}
}

func TestAction_ShellSuccess_ShowsDone(t *testing.T) {
	actions := map[string]config.Action{
		"s": {Name: "Shell", Type: "shell", Command: "echo {title}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Trigger the shell action.
	b = sendKey(t, b, keyMsg("s"))

	// Simulate the async result with success.
	m, _ := b.Update(actionResultMsg{success: true, message: "Done"})
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, "Done") {
		t.Errorf("View() after successful shell action should contain %q", "Done")
	}
}

func TestAction_ShellError_ShowsError(t *testing.T) {
	actions := map[string]config.Action{
		"s": {Name: "Shell", Type: "shell", Command: "failing-cmd"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Trigger the shell action.
	b = sendKey(t, b, keyMsg("s"))

	// Simulate the async result with failure.
	m, _ := b.Update(actionResultMsg{success: false, message: "Error: exit 1"})
	b = m.(Board)

	view := b.View()
	if !strings.Contains(view, "Error:") {
		t.Errorf("View() after failed shell action should contain %q", "Error:")
	}
}

func TestAction_HintsShowInStatusBar(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, _ := newActionTestBoard(t, actions)

	view := b.View()
	if !strings.Contains(view, "Open") {
		t.Errorf("View() should contain action hint desc %q in the status bar", "Open")
	}
}

func TestAction_URLError_ShowsErrorInStatusBar(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)
	fe.OpenURLErr = errors.New("failed to open browser")

	// Press the action key.
	m, cmd := b.Update(keyMsg("o"))
	b = m.(Board)

	// Should return a cmd for the timed status message.
	if cmd == nil {
		t.Error("OpenURL error should return a non-nil cmd for status message")
	}

	view := b.View()
	if !strings.Contains(view, "Error:") {
		t.Errorf("View() after OpenURL error should contain %q, got:\n%s", "Error:", view)
	}
}

func TestAction_TemplateVarsExpanded(t *testing.T) {
	actions := map[string]config.Action{
		"o": {Name: "Open", Type: "url", URL: "https://gh.com/{repo_owner}/{repo_name}/issues/{number}"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, false)

	// Load a board with a specific card that has known labels.
	cardNumber := 42
	cardTitle := "Add custom actions"
	cardLabels := []string{"bug", "enhancement"}
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "New", Cards: []provider.Card{
				{Number: cardNumber, Title: cardTitle, Labels: cardLabels},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Press the action key.
	b = sendKey(t, b, keyMsg("o"))
	_ = b

	expectedURL := fmt.Sprintf("https://gh.com/matteobortolazzo/lazyboards/issues/%d", cardNumber)
	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

func TestAction_ColumnActionOverridesGlobal(t *testing.T) {
	globalActions := map[string]config.Action{
		"o": {Name: "Global Open", Type: "url", URL: "https://global.com/{number}"},
	}
	columnConfigs := []config.ColumnConfig{
		{
			Name: "New",
			Actions: map[string]config.Action{
				"o": {Name: "Column Open", Type: "url", URL: "https://column.com/{number}"},
			},
		},
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, fe := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// Board starts on column 0 ("New") which has the column-level override.
	b = sendKey(t, b, keyMsg("o"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	// The column action URL should have been used, not the global one.
	if !strings.Contains(fe.OpenURLCalls[0], "column.com") {
		t.Errorf("expected column action URL containing %q, got %q", "column.com", fe.OpenURLCalls[0])
	}
}

func TestAction_FallbackToGlobalWhenColumnHasNoAction(t *testing.T) {
	globalActions := map[string]config.Action{
		"o": {Name: "Global Open", Type: "url", URL: "https://global.com/{number}"},
	}
	columnConfigs := []config.ColumnConfig{
		{Name: "New"}, // No column-level actions for "o".
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, fe := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// Board starts on column 0 ("New") which has no column-level actions.
	b = sendKey(t, b, keyMsg("o"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called via global fallback, but no calls recorded")
	}
	if !strings.Contains(fe.OpenURLCalls[0], "global.com") {
		t.Errorf("expected global action URL containing %q, got %q", "global.com", fe.OpenURLCalls[0])
	}
}

func TestAction_ColumnActionOnlyFiresInMatchingColumn(t *testing.T) {
	// No global action for key "x".
	globalActions := map[string]config.Action{}
	columnConfigs := []config.ColumnConfig{
		{Name: "New"}, // No actions for column 0.
		{
			Name: "Refined",
			Actions: map[string]config.Action{
				"x": {Name: "Deploy", Type: "url", URL: "https://deploy.com/{number}"},
			},
		},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, fe := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// Start on column 0 ("New"). Press "x" — should have no effect.
	b = sendKey(t, b, keyMsg("x"))
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls on column 0, got %d", len(fe.OpenURLCalls))
	}

	// Tab to column 1 ("Refined"). Press "x" — should trigger the deploy action.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	b = sendKey(t, b, keyMsg("x"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called on column 1 ('Refined'), but no calls recorded")
	}
	if !strings.Contains(fe.OpenURLCalls[0], "deploy.com") {
		t.Errorf("expected deploy URL containing %q, got %q", "deploy.com", fe.OpenURLCalls[0])
	}
}

func TestAction_ColumnShellUsesShellEscape(t *testing.T) {
	globalActions := map[string]config.Action{}
	columnConfigs := []config.ColumnConfig{
		{
			Name: "New",
			Actions: map[string]config.Action{
				"s": {Name: "Run", Type: "shell", Command: "echo {title}"},
			},
		},
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, fe := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// Get the selected card's title to compute expected escaped value.
	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedCmd := "echo " + action.ShellEscape(action.Slugify(selectedCard.Title))

	// Press the column shell action key.
	m, cmd := b.Update(keyMsg("s"))
	b = m.(Board)
	_ = b

	// Execute the returned cmd(s) to trigger RunShell.
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called via column shell action, but no calls recorded")
	}
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
}
