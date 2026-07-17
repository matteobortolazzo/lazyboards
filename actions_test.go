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
		"X": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Get the selected card's number to verify expansion.
	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedURL := fmt.Sprintf("https://example.com/%d", selectedCard.Number)

	// Press the action key in normalMode.
	b = sendKey(t, b, keyMsg("X"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

func TestAction_ShellTriggersRunShell(t *testing.T) {
	actions := map[string]config.Action{
		"S": {Name: "Shell", Type: "shell", Command: "echo {title}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Get the selected card's title (slugified) to verify expansion.
	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedCmd := "echo " + action.ShellEscape(action.Slugify(selectedCard.Title))

	// Press the action key in normalMode -- shell runs async via tea.Cmd.
	m, cmd := b.Update(keyMsg("S"))
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
		"X": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Enter createMode, then press the action key.
	b = sendKey(t, b, keyMsg("n"))
	b = sendKey(t, b, keyMsg("X"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls in createMode, got %d", len(fe.OpenURLCalls))
	}
}

func TestAction_IgnoredInLoadingMode(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, nil, nil, fe, "", "", "", 0, 0, 0, "Working", false, false, nil, nil)

	// Board starts in loadingMode. Press the action key.
	b = sendKey(t, b, keyMsg("X"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls in loadingMode, got %d", len(fe.OpenURLCalls))
	}
}

func TestAction_IgnoredWhenNoCards(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newBoardWithEmptyColumn(t, actions)

	// Press the action key with no cards in the column.
	b = sendKey(t, b, keyMsg("X"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls when no cards, got %d", len(fe.OpenURLCalls))
	}
}

func TestAction_ShellSuccess_ShowsDone(t *testing.T) {
	actions := map[string]config.Action{
		"S": {Name: "Shell", Type: "shell", Command: "echo {title}"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Trigger the shell action.
	b = sendKey(t, b, keyMsg("S"))

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
		"S": {Name: "Shell", Type: "shell", Command: "failing-cmd"},
	}
	b, _ := newActionTestBoard(t, actions)

	// Trigger the shell action.
	b = sendKey(t, b, keyMsg("S"))

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
		"X": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, _ := newActionTestBoard(t, actions)

	view := b.View()
	if !strings.Contains(view, "Open") {
		t.Errorf("View() should contain action hint desc %q in the status bar", "Open")
	}
}

func TestAction_URLError_ShowsErrorInStatusBar(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)
	fe.OpenURLErr = errors.New("failed to open browser")

	// Press the action key.
	m, cmd := b.Update(keyMsg("X"))
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
		"X": {Name: "Open", Type: "url", URL: "https://gh.com/{repo_owner}/{repo_name}/issues/{number}"},
	}
	cardNumber := 42
	cardTitle := "Add custom actions"
	cardLabels := []provider.Label{{Name: "bug"}, {Name: "enhancement"}}
	b, fe := newActionTestBoardWithColumns(t, actions, []provider.Column{
		{Title: "New", Cards: []provider.Card{
			{Number: cardNumber, Title: cardTitle, Labels: cardLabels},
		}},
	})

	// Press the action key.
	b = sendKey(t, b, keyMsg("X"))
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
		"X": {Name: "Global Open", Type: "url", URL: "https://global.com/{number}"},
	}
	columnConfigs := []config.ColumnConfig{
		{
			Name: "New",
			Actions: map[string]config.Action{
				"X": {Name: "Column Open", Type: "url", URL: "https://column.com/{number}"},
			},
		},
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, fe := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// Board starts on column 0 ("New") which has the column-level override.
	sendKey(t, b, keyMsg("X"))

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
		"X": {Name: "Global Open", Type: "url", URL: "https://global.com/{number}"},
	}
	columnConfigs := []config.ColumnConfig{
		{Name: "New"}, // No column-level actions for "X".
		{Name: "Refined"},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, fe := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// Board starts on column 0 ("New") which has no column-level actions.
	sendKey(t, b, keyMsg("X"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called via global fallback, but no calls recorded")
	}
	if !strings.Contains(fe.OpenURLCalls[0], "global.com") {
		t.Errorf("expected global action URL containing %q, got %q", "global.com", fe.OpenURLCalls[0])
	}
}

func TestAction_ColumnActionOnlyFiresInMatchingColumn(t *testing.T) {
	// No global action for key "X".
	globalActions := map[string]config.Action{}
	columnConfigs := []config.ColumnConfig{
		{Name: "New"}, // No actions for column 0.
		{
			Name: "Refined",
			Actions: map[string]config.Action{
				"X": {Name: "Deploy", Type: "url", URL: "https://deploy.com/{number}"},
			},
		},
		{Name: "Implementing"},
		{Name: "Implemented"},
	}
	b, fe := newColumnActionTestBoard(t, globalActions, columnConfigs)

	// Start on column 0 ("New"). Press "X" — should have no effect.
	sendKey(t, b, keyMsg("X"))
	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls on column 0, got %d", len(fe.OpenURLCalls))
	}

	// Tab to column 1 ("Refined"). Press "X" — should trigger the deploy action.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	sendKey(t, b, keyMsg("X"))

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
				"S": {Name: "Run", Type: "shell", Command: "echo {title}"},
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
	m, cmd := b.Update(keyMsg("S"))
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

func TestAction_URLEscapesTemplateVars(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Open", Type: "url", URL: "https://example.com/search?tags={tags}"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil)

	// Load a board with a card that has labels containing URL-special characters.
	cardLabels := []provider.Label{{Name: "bug&fix"}, {Name: "feature?v2"}}
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "New", Cards: []provider.Card{
				{Number: 1, Title: "Test card", Labels: cardLabels},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Press the action key.
	b = sendKey(t, b, keyMsg("X"))

	// The tags value is "bug&fix,feature?v2" (joined with comma by BuildTemplateVars).
	// After URL escaping, &, ?, and , should be percent-encoded.
	expectedURL := "https://example.com/search?tags=bug%26fix%2Cfeature%3Fv2"

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

// --- Comment template variable expansion ---

func TestAction_CommentVariableExpansion(t *testing.T) {
	// End-to-end: Alt+key enters comment mode, user types text, submits,
	// and the {comment} variable is expanded in the shell command via
	// BuildTemplateVars (not manual post-injection).
	actions := map[string]config.Action{
		"X": {Name: "Annotate", Type: "shell", Command: "gh issue comment {number} --body {comment}"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Get the selected card's number for verification.
	col := b.Columns[b.ActiveTab]
	selectedCard := col.Cards[col.Cursor]

	// Press Alt+X to enter comment mode.
	b = sendKey(t, b, altKeyMsg("X"))
	if b.mode != commentMode {
		t.Fatalf("expected commentMode after Alt+x, got mode = %d", b.mode)
	}

	// Type a comment.
	commentText := "fix applied"
	for _, ch := range commentText {
		b = sendKey(t, b, keyMsg(string(ch)))
	}

	// Press Enter to submit the comment and execute the action.
	m, cmd := b.Update(arrowMsg(tea.KeyEnter))
	b = m.(Board)
	execCmds(cmd)

	// Verify the board returned to normalMode.
	if b.mode != normalMode {
		t.Errorf("after submit: mode = %d, want %d (normalMode)", b.mode, normalMode)
	}

	// Verify RunShell was called with the comment expanded and shell-escaped.
	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called after comment submission, but no calls recorded")
	}
	expandedCmd := fe.RunShellCalls[0]
	expectedNumber := action.ShellEscape(fmt.Sprintf("%d", selectedCard.Number))
	expectedComment := action.ShellEscape(commentText)
	if !strings.Contains(expandedCmd, expectedNumber) {
		t.Errorf("RunShell command = %q, want it to contain shell-escaped number %q", expandedCmd, expectedNumber)
	}
	if !strings.Contains(expandedCmd, expectedComment) {
		t.Errorf("RunShell command = %q, want it to contain shell-escaped comment %q", expandedCmd, expectedComment)
	}
}

// --- Ticket Open (o key) ---

func TestTicketOpen_NormalMode_OpensCardURL(t *testing.T) {
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil)

	// Load a board with a card that has a URL.
	cardURL := "https://github.com/matteobortolazzo/lazyboards/issues/42"
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "New", Cards: []provider.Card{
				{Number: 42, Title: "Add ticket open hotkey", Labels: []provider.Label{{Name: "feature"}}, URL: cardURL},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Press "o" in normal mode to open the card's ticket URL.
	b = sendKey(t, b, keyMsg("o"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	if fe.OpenURLCalls[0] != cardURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], cardURL)
	}
}

func TestTicketOpen_NormalMode_EmptyURL_ShowsMessage(t *testing.T) {
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil)

	// Load a board with a card that has no URL.
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "New", Cards: []provider.Card{
				{Number: 1, Title: "No URL card", Labels: []provider.Label{{Name: "bug"}}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Press "o" with empty URL.
	b = sendKey(t, b, keyMsg("o"))

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls when card URL is empty, got %d", len(fe.OpenURLCalls))
	}

	view := b.View()
	if !strings.Contains(view, "URL not available") {
		t.Errorf("View() should contain %q when card URL is empty, got:\n%s", "URL not available", view)
	}
}

func TestTicketOpen_NormalMode_NoCards_DoesNothing(t *testing.T) {
	b, fe := newBoardWithEmptyColumn(t, nil)

	// Press "o" with no cards in the column.
	b = sendKey(t, b, keyMsg("o"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls when no cards, got %d", len(fe.OpenURLCalls))
	}
}

func TestTicketOpen_DetailFocused_OpensCardURL(t *testing.T) {
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil)

	// Load a board with a card that has a URL.
	cardURL := "https://github.com/matteobortolazzo/lazyboards/issues/7"
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "New", Cards: []provider.Card{
				{Number: 7, Title: "Detail card", Labels: []provider.Label{{Name: "feature"}}, URL: cardURL},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Enter detail focus, then press "o" to open ticket.
	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("o"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called from detail focus, but no calls recorded")
	}
	if fe.OpenURLCalls[0] != cardURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], cardURL)
	}
}

func TestTicketOpen_ShowsOpenedMessage(t *testing.T) {
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil)

	// Load a board with a card that has a URL.
	cardNumber := 99
	cardURL := "https://github.com/matteobortolazzo/lazyboards/issues/99"
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "New", Cards: []provider.Card{
				{Number: cardNumber, Title: "Message test", Labels: []provider.Label{{Name: "feature"}}, URL: cardURL},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	// Press "o" to open the ticket.
	b = sendKey(t, b, keyMsg("o"))

	view := b.View()
	expectedMsg := fmt.Sprintf("Opened #%d", cardNumber)
	if !strings.Contains(view, expectedMsg) {
		t.Errorf("View() should contain %q after opening ticket, got:\n%s", expectedMsg, view)
	}
}

// --- Ticket Open hint visibility tests ---

func TestTicketOpen_OpenHintHiddenOnEmptyColumn(t *testing.T) {
	b, _ := newBoardWithEmptyColumn(t, nil)

	// The "Open" hint for the "o" key should NOT appear when there are no cards.
	statusBarView := b.statusBar.View(200, 0, 0)
	if strings.Contains(statusBarView, "Open") {
		t.Errorf("status bar should NOT contain %q hint on empty column, got:\n%s", "Open", statusBarView)
	}
}

func TestTicketOpen_OpenHintNotInNormalBar(t *testing.T) {
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, nil, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil)
	b = loadFromFakeProvider(t, b, p)

	// Even with cards loaded, the "o" (Open) hint should NOT appear in the
	// always-visible normal-mode hint bar; the keybinding stays functional
	// and remains listed in the '?' Help popup.
	foundOpenHint := false
	for _, hint := range b.normalHints {
		if hint.Key == "o" {
			foundOpenHint = true
			break
		}
	}
	if foundOpenHint {
		t.Error("normalHints should NOT include 'o' hint even when cards exist; the keybinding stays functional but is no longer shown in the always-visible hint bar")
	}
}

// --- Board-scope action dispatch tests ---

func TestAction_BoardScope_URLFiresWithEmptyColumn(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://github.com/{repo_owner}/{repo_name}/issues"},
	}
	b, fe := newBoardWithEmptyColumn(t, actions)

	// Press the board-scope action key.
	b = sendKey(t, b, keyMsg("B"))
	_ = b

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called for board-scope action on empty column, but no calls recorded")
	}
	expectedURL := "https://github.com/matteobortolazzo/lazyboards/issues"
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

func TestAction_BoardScope_URLFiresWithCards(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://github.com/{repo_owner}/{repo_name}/issues"},
	}
	b, fe := newActionTestBoard(t, actions)

	// Press the board-scope action key (column has cards from FakeProvider).
	sendKey(t, b, keyMsg("B"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called for board-scope action with cards, but no calls recorded")
	}
	expectedURL := "https://github.com/matteobortolazzo/lazyboards/issues"
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

func TestAction_BoardScope_UsesOnlyBoardVars(t *testing.T) {
	// Use {number} in URL to verify board-scope does NOT expand card-specific vars.
	// {number} should remain unexpanded (left as-is) because board vars don't include it.
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://example.com/{repo_owner}/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	sendKey(t, b, keyMsg("B"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	// repo_owner should be expanded; {number} should remain literal.
	expectedURL := "https://example.com/matteobortolazzo/{number}"
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

func TestAction_BoardScope_ShellFiresWithEmptyColumn(t *testing.T) {
	actions := map[string]config.Action{
		"S": {Name: "Deploy", Type: "shell", Scope: "board", Command: "deploy --repo {repo_owner}/{repo_name}"},
	}
	b, fe := newBoardWithEmptyColumn(t, actions)

	// Press the board-scope shell action key.
	m2, cmd := b.Update(keyMsg("S"))
	b = m2.(Board)
	_ = b

	// Execute the returned cmd(s) to trigger RunShell.
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called for board-scope shell action on empty column, but no calls recorded")
	}
	// Shell vars are shell-escaped.
	expectedCmd := "deploy --repo " + action.ShellEscape("matteobortolazzo") + "/" + action.ShellEscape("lazyboards")
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
}

func TestAction_CardScope_StillIgnoredWhenNoCards(t *testing.T) {
	// Explicit scope: card should preserve existing behavior: silently ignored when no cards.
	actions := map[string]config.Action{
		"X": {Name: "Open card", Type: "url", Scope: "card", URL: "https://example.com/{number}"},
	}
	b, fe := newBoardWithEmptyColumn(t, actions)

	// Press the card-scope action key with no cards.
	b = sendKey(t, b, keyMsg("X"))
	_ = b

	if len(fe.OpenURLCalls) != 0 {
		t.Errorf("expected no OpenURL calls for card-scope action on empty column, got %d", len(fe.OpenURLCalls))
	}
}

// --- Hint visibility tests ---

func TestAction_BoardScopeHint_VisibleOnEmptyColumn(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://github.com/{repo_owner}/{repo_name}/issues"},
	}
	b, _ := newBoardWithEmptyColumn(t, actions)

	view := b.View()
	if !strings.Contains(view, "Open board") {
		t.Errorf("View() should contain board-scope hint %q on empty column, got:\n%s", "Open board", view)
	}
}

func TestAction_CardScopeHint_HiddenOnEmptyColumn(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Card action", Type: "url", Scope: "card", URL: "https://example.com/{number}"},
	}
	b, _ := newBoardWithEmptyColumn(t, actions)

	view := b.View()
	if strings.Contains(view, "Card action") {
		t.Errorf("View() should NOT contain card-scope hint %q on empty column, got:\n%s", "Card action", view)
	}
}

func TestAction_CardScopeHint_VisibleWithCards(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Card action", Type: "url", Scope: "card", URL: "https://example.com/{number}"},
	}
	b, _ := newActionTestBoard(t, actions)

	view := b.View()
	if !strings.Contains(view, "Card action") {
		t.Errorf("View() should contain card-scope hint %q when cards exist, got:\n%s", "Card action", view)
	}
}

// --- Custom actions dispatch from the detail-focused panel ---

func TestAction_DetailFocused_CardScopeURLTriggersOpenURL(t *testing.T) {
	actions := map[string]config.Action{
		"X": {Name: "Open", Type: "url", URL: "https://example.com/{number}"},
	}
	b, fe := newActionTestBoard(t, actions)

	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedURL := fmt.Sprintf("https://example.com/%d", selectedCard.Number)

	// Focus the detail panel, then press the action key.
	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}
	b = sendKey(t, b, keyMsg("X"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called from detail-focused panel, but no calls recorded")
	}
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

func TestAction_DetailFocused_ShellTriggersRunShell(t *testing.T) {
	actions := map[string]config.Action{
		"S": {Name: "Shell", Type: "shell", Command: "echo {title}"},
	}
	b, fe := newActionTestBoard(t, actions)

	selectedCard := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	expectedCmd := "echo " + action.ShellEscape(action.Slugify(selectedCard.Title))

	b = sendKey(t, b, keyMsg("l"))
	m, cmd := b.Update(keyMsg("S"))
	b = m.(Board)

	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called from detail-focused panel, but no calls recorded")
	}
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
	if !b.detailFocused {
		t.Error("firing a card-scope action from detail focus should not drop detailFocused")
	}
}

func TestAction_DetailFocused_BoardScopeFires(t *testing.T) {
	actions := map[string]config.Action{
		"B": {Name: "Open board", Type: "url", Scope: "board", URL: "https://github.com/{repo_owner}/{repo_name}/issues"},
	}
	b, fe := newActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("B"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called for board-scope action from detail-focused panel, but no calls recorded")
	}
	expectedURL := "https://github.com/matteobortolazzo/lazyboards/issues"
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
	if !b.detailFocused {
		t.Error("firing a board-scope action from detail focus should not drop detailFocused")
	}
}

func TestAction_DetailFocused_UnboundKeyIsNoop(t *testing.T) {
	b, fe := newActionTestBoard(t, nil)

	b = sendKey(t, b, keyMsg("l"))
	b = sendKey(t, b, keyMsg("Z"))

	if !b.detailFocused {
		t.Error("an unbound custom-action key should not affect detailFocused")
	}
	if len(fe.OpenURLCalls) != 0 || len(fe.RunShellCalls) != 0 {
		t.Error("an unbound custom-action key should not trigger any action")
	}
}

func TestAction_DetailFocused_BuiltinKeysStillWin(t *testing.T) {
	// A custom action bound to "e" is impossible (lowercase is reserved for
	// built-ins), but this test locks in that the built-in "e" (Edit) inside
	// detail focus always dispatches openEditorCmd, never a custom action --
	// there is no custom-action code path for lowercase keys to begin with.
	b := newLoadedTestBoard(t)

	b = sendKey(t, b, keyMsg("l"))
	m, cmd := b.Update(keyMsg("e"))
	b = m.(Board)

	if cmd == nil {
		t.Error("'e' in detail focus should still trigger openEditorCmd")
	}
	if !b.detailFocused {
		t.Error("'e' in detail focus should not drop detailFocused")
	}
}

// --- scope: pr action dispatch tests (#340) ---
//
// Full 0/1/2+ linked-PR precedence, mirroring handlePROpenKey (the existing
// anchor for the built-in "p" key), per CLAUDE.md's "consume the FULL
// precedence" rule.

func TestAction_PRScope_ZeroPRs_NoDispatchAndNoHint(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && ng serve"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	// Cursor starts on card 1 (0 linked PRs).
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 0 {
		t.Fatalf("test setup: expected card at cursor to have 0 LinkedPRs, got %d", len(card.LinkedPRs))
	}

	// No-dispatch facet.
	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls for pr-scope action on a card with 0 linked PRs, got %d", len(fe.RunShellCalls))
	}

	// No-hint facet.
	view := b.statusBar.View(200, 0, 0)
	if strings.Contains(view, "Serve branch") {
		t.Errorf("statusBar.View() should NOT show pr-scope hint on a card with 0 linked PRs, got:\n%s", view)
	}
}

func TestAction_PRActionKeyWithComment_ZeroPRs_ShowsStatusMessage(t *testing.T) {
	// Defensive-branch test: handlePRActionKeyWithComment's 0-linked-PR case
	// is unreachable through the documented dispatch flow today —
	// resolveAction's prScopeGated check already refuses to dispatch a
	// scope: pr action against a 0-PR card (see
	// TestAction_PRScope_ZeroPRs_NoDispatchAndNoHint above). This test calls
	// the handler directly to exercise that defensive branch and confirm it
	// gives the same user-facing feedback as the equivalent built-in "p"
	// open-PR path (handlePROpenKey) instead of failing silently.
	act := config.Action{Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch}"}
	b, fe := newPRActionTestBoard(t, nil)

	// Cursor starts on card 1 (0 linked PRs).
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 0 {
		t.Fatalf("test setup: expected card at cursor to have 0 LinkedPRs, got %d", len(card.LinkedPRs))
	}

	m, cmd := b.handlePRActionKeyWithComment(act, card, "")
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls for a 0-linked-PR card, got %d", len(fe.RunShellCalls))
	}
	if !strings.Contains(b.statusBar.View(200, 0, 0), "No linked PRs") {
		t.Errorf("statusBar.View(, 0, 0) = %q, want it to contain %q", b.statusBar.View(200, 0, 0), "No linked PRs")
	}
}

func TestAction_PRScopeHint_VisibleWithLinkedPR(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && ng serve"},
	}
	b, _ := newPRActionTestBoard(t, actions)

	// Move to card 2 (1 linked PR).
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) == 0 {
		t.Fatalf("test setup: expected card at cursor to have a linked PR")
	}

	view := b.statusBar.View(200, 0, 0)
	if !strings.Contains(view, "Serve branch") {
		t.Errorf("statusBar.View() should show pr-scope hint on a card with a linked PR, got:\n%s", view)
	}
}

func TestAction_PRScope_SinglePR_RunsImmediatelyAgainstPRData(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	// Move to card 2 (exactly 1 linked PR).
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 1 {
		t.Fatalf("test setup: expected exactly 1 linked PR, got %d", len(card.LinkedPRs))
	}
	pr := card.LinkedPRs[0]

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called for a pr-scope action on a card with exactly 1 linked PR")
	}
	expectedCmd := "cd " + action.ShellEscape(pr.Branch)
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
}

func TestAction_PRScope_SinglePR_URLType_OpensExpandedURL(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "View PR diff", Type: "url", Scope: "pr", URL: "https://example.com/diff/{pr_number}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	pr := card.LinkedPRs[0]

	b = sendKey(t, b, keyMsg("W"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called for a url-type pr-scope action on a 1-PR card")
	}
	expectedURL := fmt.Sprintf("https://example.com/diff/%d", pr.Number)
	if fe.OpenURLCalls[0] != expectedURL {
		t.Errorf("OpenURL called with %q, want %q", fe.OpenURLCalls[0], expectedURL)
	}
}

func TestAction_PRScope_SinglePR_ShellEscapesMaliciousBranch(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch} && ng serve"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil)

	maliciousBranch := "feature/x; rm -rf / #"
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "Malicious PR", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "feat", URL: "https://github.com/owner/repo/pull/10", Branch: maliciousBranch},
				}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	m2, cmd := b.Update(keyMsg("W"))
	b = m2.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called, but no calls recorded")
	}
	expectedCmd := "cd " + action.ShellEscape(maliciousBranch) + " && ng serve"
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q (pr_branch must be shell-escaped)", fe.RunShellCalls[0], expectedCmd)
	}
}

func TestAction_PRScope_SinglePR_URLEscapesMaliciousBranch(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Open branch info", Type: "url", Scope: "pr", URL: "https://example.com/?branch={pr_branch}"},
	}
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, actions, nil, nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil)

	maliciousBranch := "feature/x&evil=1"
	msg := boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{
			{Title: "Column A", Cards: []provider.Card{
				{Number: 1, Title: "Malicious PR", Labels: []provider.Label{{Name: "feature"}}, LinkedPRs: []provider.LinkedPR{
					{Number: 10, Title: "feat", URL: "https://github.com/owner/repo/pull/10", Branch: maliciousBranch},
				}},
			}},
		},
	}}
	m, _ := b.Update(msg)
	b = m.(Board)
	b.Width = 120
	b.Height = 40

	b = sendKey(t, b, keyMsg("W"))

	if len(fe.OpenURLCalls) == 0 {
		t.Fatal("expected OpenURL to be called, but no calls recorded")
	}
	got := fe.OpenURLCalls[0]
	// & and = in the injected branch must be percent-encoded, not left as raw
	// query-parameter separators (mirrors the {tags} injection lesson).
	if strings.Contains(got, "&evil=1") {
		t.Errorf("OpenURL URL = %q, pr_branch's & / = must be percent-encoded, not injected as a raw query param", got)
	}
	if !strings.Contains(got, action.URLEscape(maliciousBranch)) {
		t.Errorf("OpenURL URL = %q, want it to contain the URL-escaped branch %q", got, action.URLEscape(maliciousBranch))
	}
}

func TestAction_PRScope_MultiplePRs_EntersPickerWithPendingAction(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	// Move to card 3 (2 linked PRs).
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) < 2 {
		t.Fatalf("test setup: expected card at cursor to have 2+ LinkedPRs, got %d", len(card.LinkedPRs))
	}

	b = sendKey(t, b, keyMsg("W"))

	if b.mode != prPickerMode {
		t.Errorf("mode = %d, want prPickerMode (%d)", b.mode, prPickerMode)
	}
	if b.pendingPRAction == nil {
		t.Fatal("expected pendingPRAction to be set when entering the picker from a pr-scope action")
	}
	if b.pendingPRAction.action.Name != "Serve branch" {
		t.Errorf("pendingPRAction.action.Name = %q, want %q", b.pendingPRAction.action.Name, "Serve branch")
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls yet (action pending PR selection), got %d", len(fe.RunShellCalls))
	}
}

func TestAction_PRScope_TemplateIncludesAllPRAndCardVars(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Full vars", Type: "shell", Scope: "pr", Command: "echo {number} {pr_number} {pr_branch} {pr_title}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	pr := card.LinkedPRs[0]

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called")
	}
	got := fe.RunShellCalls[0]
	expectedNumber := action.ShellEscape(fmt.Sprintf("%d", card.Number))
	expectedPRNumber := action.ShellEscape(fmt.Sprintf("%d", pr.Number))
	expectedBranch := action.ShellEscape(pr.Branch)
	expectedTitle := action.ShellEscape(action.Slugify(pr.Title))
	for _, want := range []string{expectedNumber, expectedPRNumber, expectedBranch, expectedTitle} {
		if !strings.Contains(got, want) {
			t.Errorf("RunShell command = %q, want it to contain %q", got, want)
		}
	}
}

func TestAction_PRScope_PRWorktreeExpandsRegisteredWorktree(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Run worktree", Type: "shell", Scope: "pr", Command: "cd {pr_worktree} && ng serve"},
	}
	b, fe := newPRActionTestBoard(t, actions)
	fe.RunShellOutputStdout = "worktree /repo/.worktrees/one-pr\nHEAD 1234567\nbranch refs/heads/feature/one-pr\n"

	b = sendKey(t, b, keyMsg("j"))
	_, cmd := b.Update(keyMsg("W"))
	execCmds(cmd)

	if len(fe.RunShellOutputCalls) != 1 || fe.RunShellOutputCalls[0] != "git worktree list --porcelain" {
		t.Fatalf("RunShellOutputCalls = %v, want git worktree list --porcelain", fe.RunShellOutputCalls)
	}
	if len(fe.RunShellCalls) != 1 {
		t.Fatalf("RunShellCalls = %v, want one action command", fe.RunShellCalls)
	}
	want := "cd " + action.ShellEscape("/repo/.worktrees/one-pr") + " && ng serve"
	if got := fe.RunShellCalls[0]; got != want {
		t.Errorf("RunShell command = %q, want %q", got, want)
	}
}

func TestAction_PRScope_PRWorktreeMissingDoesNotRunAction(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Run worktree", Type: "shell", Scope: "pr", Command: "cd {pr_worktree} && ng serve"},
	}
	b, fe := newPRActionTestBoard(t, actions)
	fe.RunShellOutputStdout = "worktree /repo\nHEAD 1234567\nbranch refs/heads/main\n"

	b = sendKey(t, b, keyMsg("j"))
	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Errorf("RunShellCalls = %v, want no action command when the worktree is missing", fe.RunShellCalls)
	}
	if !strings.Contains(b.statusBar.message, "no Git worktree found") {
		t.Errorf("status message = %q, want missing-worktree error", b.statusBar.message)
	}
}

// --- scope: pr actions dispatched from the detail-focused panel (#349 x #340) ---

func TestAction_DetailFocused_PRScope_SinglePRFiresImmediately(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	// Move to card 2 (exactly 1 linked PR).
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) != 1 {
		t.Fatalf("test setup: expected exactly 1 linked PR, got %d", len(card.LinkedPRs))
	}
	pr := card.LinkedPRs[0]

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	m, cmd := b.Update(keyMsg("W"))
	b = m.(Board)
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called for a pr-scope action fired from the detail-focused panel")
	}
	expectedCmd := "cd " + action.ShellEscape(pr.Branch)
	if fe.RunShellCalls[0] != expectedCmd {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], expectedCmd)
	}
	if !b.detailFocused {
		t.Error("firing a pr-scope action from detail focus should not drop detailFocused")
	}
}

// --- Custom-action hint bar follows config file order (#435/#437) ---

func TestAction_HintBar_OrderMatchesConfigOrder(t *testing.T) {
	localYAML := `provider: github
repo: matteobortolazzo/lazyboards
actions:
  Z:
    name: Zebra action
    type: shell
    scope: board
    command: "echo z"
  A:
    name: Apple action
    type: shell
    scope: board
    command: "echo a"
  M:
    name: Mango action
    type: shell
    scope: board
    command: "echo m"
`
	b, _ := newConfigLoadedActionTestBoard(t, localYAML)

	hints := b.normalHints
	z := hintIndex(hints, "Z")
	a := hintIndex(hints, "A")
	m := hintIndex(hints, "M")
	if z == -1 || a == -1 || m == -1 {
		t.Fatalf("expected hints for Z, A, M; got: %+v", hints)
	}
	if !(z < a && a < m) {
		t.Errorf("hint order should match the config file order Z, A, M; got indices Z=%d A=%d M=%d in %+v", z, a, m, hints)
	}
}

func TestAction_HintBar_ColumnOverride_KeepsGlobalPosition(t *testing.T) {
	localYAML := `provider: github
repo: matteobortolazzo/lazyboards
actions:
  X:
    name: Global X
    type: shell
    scope: board
    command: "echo x"
  Y:
    name: Global Y
    type: shell
    scope: board
    command: "echo y"
  Z:
    name: Global Z
    type: shell
    scope: board
    command: "echo z"
columns:
  - name: New
    actions:
      Y:
        name: Overridden Y
        type: shell
        scope: board
        command: "echo overridden-y"
`
	b, _ := newConfigLoadedActionTestBoard(t, localYAML)

	hints := b.normalHints
	x := hintIndex(hints, "X")
	y := hintIndex(hints, "Y")
	z := hintIndex(hints, "Z")
	if x == -1 || y == -1 || z == -1 {
		t.Fatalf("expected hints for X, Y, Z; got: %+v", hints)
	}
	if !(x < y && y < z) {
		t.Errorf("Y's overridden hint should keep its global position (between X and Z); got indices X=%d Y=%d Z=%d in %+v", x, y, z, hints)
	}
	if hints[y].Desc != "Overridden Y" {
		t.Errorf("Y hint Desc = %q, want %q (column override should win the value)", hints[y].Desc, "Overridden Y")
	}
}

func TestAction_HintBar_ZeroOrderActionsFallBackToAlphabetical(t *testing.T) {
	// Hand-built map fixtures (not through config.Load()) leave Order at its
	// zero value for every entry; the hint bar must degrade to alphabetical
	// order in that case, keeping every existing single/no-order test
	// meaningful.
	actions := map[string]config.Action{
		"Z": {Name: "Zebra", Type: "shell", Scope: "board", Command: "echo z"},
		"A": {Name: "Apple", Type: "shell", Scope: "board", Command: "echo a"},
		"M": {Name: "Mango", Type: "shell", Scope: "board", Command: "echo m"},
	}
	b, _ := newActionTestBoard(t, actions)

	hints := b.normalHints
	a := hintIndex(hints, "A")
	m := hintIndex(hints, "M")
	z := hintIndex(hints, "Z")
	if a == -1 || m == -1 || z == -1 {
		t.Fatalf("expected hints for A, M, Z; got: %+v", hints)
	}
	if !(a < m && m < z) {
		t.Errorf("zero-Order actions should render alphabetically (A, M, Z); got indices A=%d M=%d Z=%d in %+v", a, m, z, hints)
	}
}

func TestAction_HintBar_ColumnOnlyKeysAppendAfterGlobalOrder(t *testing.T) {
	localYAML := `provider: github
repo: matteobortolazzo/lazyboards
actions:
  B:
    name: Global B
    type: shell
    scope: board
    command: "echo b"
  A:
    name: Global A
    type: shell
    scope: board
    command: "echo a"
columns:
  - name: New
    actions:
      B:
        name: Global B
        type: shell
        scope: board
        command: "echo b"
      A:
        name: Global A
        type: shell
        scope: board
        command: "echo a"
      D:
        name: Column-only D
        type: shell
        scope: board
        command: "echo d"
`
	b, _ := newConfigLoadedActionTestBoard(t, localYAML)

	hints := b.normalHints
	bIdx := hintIndex(hints, "B")
	aIdx := hintIndex(hints, "A")
	d := hintIndex(hints, "D")
	if bIdx == -1 || aIdx == -1 || d == -1 {
		t.Fatalf("expected hints for B, A, D; got: %+v", hints)
	}
	if !(bIdx < d && aIdx < d) {
		t.Errorf("column-only key D should append after the global order (B, A); got indices B=%d A=%d D=%d in %+v", bIdx, aIdx, d, hints)
	}
}

func TestAction_DetailFocused_PRScope_MultiplePRsOpensPicker(t *testing.T) {
	actions := map[string]config.Action{
		"W": {Name: "Serve branch", Type: "shell", Scope: "pr", Command: "cd {pr_branch}"},
	}
	b, fe := newPRActionTestBoard(t, actions)

	// Move to card 3 (2 linked PRs).
	b = sendKey(t, b, keyMsg("j"))
	b = sendKey(t, b, keyMsg("j"))
	card := b.Columns[b.ActiveTab].Cards[b.Columns[b.ActiveTab].Cursor]
	if len(card.LinkedPRs) < 2 {
		t.Fatalf("test setup: expected 2+ linked PRs, got %d", len(card.LinkedPRs))
	}

	b = sendKey(t, b, keyMsg("l"))
	if !b.detailFocused {
		t.Fatal("precondition: detailFocused should be true")
	}

	b = sendKey(t, b, keyMsg("W"))

	if b.mode != prPickerMode {
		t.Errorf("mode = %d, want prPickerMode (%d)", b.mode, prPickerMode)
	}
	if b.pendingPRAction == nil {
		t.Fatal("expected pendingPRAction to be set when entering the picker from detail focus")
	}
	// Matches the convention TestAction_DetailFocused_BoardScopeFires established:
	// dispatching a custom action from detail focus does not drop detailFocused.
	if !b.detailFocused {
		t.Error("opening the PR picker from a pr-scope action fired from detail focus should not drop detailFocused")
	}
	if len(fe.RunShellCalls) != 0 {
		t.Errorf("expected no RunShell calls yet (action pending PR selection), got %d", len(fe.RunShellCalls))
	}
}
