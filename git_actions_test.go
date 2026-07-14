package main

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// gitDefaultsBoard builds a board seeded with the built-in git default actions
// (in defaultActions, not cfg.Actions) plus any provided user actions.
func gitDefaultsBoard(t *testing.T, userActions map[string]config.Action) (Board, *action.FakeExecutor) {
	t.Helper()
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	b := NewBoard(p, userActions, config.DefaultGitActions(), nil, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil, "", "")

	// Load a board with an empty column so board-scope actions can dispatch.
	m, _ := b.Update(boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{Title: "Empty", Cards: nil}},
	}})
	b = m.(Board)
	b.Width = 120
	b.Height = 40
	return b, fe
}

// Git defaults are menu-scoped: resolveAction (the normal-mode custom action
// path) must never surface them, keeping A-Z fully user-owned.
func TestResolveAction_DoesNotFallBackToGitDefault(t *testing.T) {
	b, _ := gitDefaultsBoard(t, nil)

	if act, ok := b.resolveAction("P"); ok {
		t.Fatalf("resolveAction(\"P\") = %+v, ok=true; git defaults must be git-menu-scoped, not normal-mode keys", act)
	}
}

func TestResolveAction_UserActionOnGitDefaultKey_ResolvesUserAction(t *testing.T) {
	userActions := map[string]config.Action{
		"P": {Name: "Custom P", Type: "shell", Command: "echo custom", Scope: "board"},
	}
	b, _ := gitDefaultsBoard(t, userActions)

	act, ok := b.resolveAction("P")
	if !ok {
		t.Fatal("resolveAction(\"P\") returned ok=false")
	}
	if act.Command != "echo custom" {
		t.Errorf("resolveAction(\"P\").Command = %q, want user action %q", act.Command, "echo custom")
	}
}

func TestGitDefaults_NotInHintBar(t *testing.T) {
	// A user board-scope action should surface in the hint bar; git defaults must not.
	userActions := map[string]config.Action{
		"Z": {Name: "Zap", Type: "shell", Command: "echo zap", Scope: "board"},
	}
	b, _ := gitDefaultsBoard(t, userActions)

	view := b.View()
	for _, name := range []string{"Push", "Pull (rebase)", "Mergetool"} {
		if strings.Contains(view, name) {
			t.Errorf("hint bar should NOT contain git default %q, got:\n%s", name, view)
		}
	}
	if !strings.Contains(view, "Zap") {
		t.Errorf("hint bar should contain user board-scope action %q, got:\n%s", "Zap", view)
	}
}

func TestGitDefaults_NormalModeUppercaseKey_DoesNotDispatch(t *testing.T) {
	b, fe := gitDefaultsBoard(t, nil)

	_, cmd := b.Update(keyMsg("P"))
	execCmds(cmd)

	if len(fe.RunShellCalls) != 0 {
		t.Fatalf("normal-mode P must not dispatch a git default, got RunShell calls: %v", fe.RunShellCalls)
	}
}

func TestBuildHelpContent_ListsGitMenuKeys(t *testing.T) {
	b, _ := gitDefaultsBoard(t, nil)

	content := b.buildHelpContent()
	if !strings.Contains(content, "Git Menu") {
		t.Fatalf("help content missing \"Git Menu\" section, got:\n%s", content)
	}
	for _, name := range []string{"Push", "Pull (rebase)", "Fetch", "Mergetool", "Stash push", "Stash pop"} {
		if !strings.Contains(content, name) {
			t.Errorf("help content missing git menu entry %q, got:\n%s", name, content)
		}
	}
}
