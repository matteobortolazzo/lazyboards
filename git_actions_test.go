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

func TestResolveAction_FallsBackToGitDefault(t *testing.T) {
	b, _ := gitDefaultsBoard(t, nil)

	act, ok := b.resolveAction("P")
	if !ok {
		t.Fatal("resolveAction(\"P\") returned ok=false, want a git default")
	}
	if act.Command != "git push" || act.Scope != "board" {
		t.Errorf("resolveAction(\"P\") = %+v, want git push board-scope action", act)
	}
}

func TestResolveAction_UserActionOverridesGitDefault(t *testing.T) {
	userActions := map[string]config.Action{
		"P": {Name: "Custom P", Type: "shell", Command: "echo custom", Scope: "board"},
	}
	b, _ := gitDefaultsBoard(t, userActions)

	act, ok := b.resolveAction("P")
	if !ok {
		t.Fatal("resolveAction(\"P\") returned ok=false")
	}
	if act.Command != "echo custom" {
		t.Errorf("resolveAction(\"P\").Command = %q, want user override %q", act.Command, "echo custom")
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

func TestGitDefaults_PressPushRunsGitPush(t *testing.T) {
	b, fe := gitDefaultsBoard(t, nil)

	_, cmd := b.Update(keyMsg("P"))
	execCmds(cmd)

	if len(fe.RunShellCalls) == 0 {
		t.Fatal("expected RunShell to be called for git default P, got no calls")
	}
	if fe.RunShellCalls[0] != "git push" {
		t.Errorf("RunShell called with %q, want %q", fe.RunShellCalls[0], "git push")
	}
}

func TestBuildHelpContent_ListsGitDefaults(t *testing.T) {
	b, _ := gitDefaultsBoard(t, nil)

	content := b.buildHelpContent()
	if !strings.Contains(content, "Built-in Git Actions") {
		t.Fatalf("help content missing \"Built-in Git Actions\" section, got:\n%s", content)
	}
	for _, name := range []string{"Push", "Pull (rebase)", "Mergetool"} {
		if !strings.Contains(content, name) {
			t.Errorf("help content missing git default %q, got:\n%s", name, content)
		}
	}
}

func TestBuildHelpContent_OmitsOverriddenGitDefault(t *testing.T) {
	// User overrides P via a global action; it must not appear under Built-in Git Actions.
	userActions := map[string]config.Action{
		"P": {Name: "Custom Push", Type: "shell", Command: "echo custom", Scope: "board"},
	}
	b, _ := gitDefaultsBoard(t, userActions)

	content := b.buildHelpContent()
	gitSection := content[strings.Index(content, "Built-in Git Actions"):]
	if strings.Contains(gitSection, "Push (shell)") {
		t.Errorf("Built-in Git Actions section should omit overridden key P, got:\n%s", gitSection)
	}
	// The other defaults remain.
	if !strings.Contains(gitSection, "Mergetool") {
		t.Errorf("Built-in Git Actions section should still list Mergetool, got:\n%s", gitSection)
	}
}

func TestBuildHelpContent_OmitsColumnOverriddenGitDefault(t *testing.T) {
	// User overrides M via a per-column action; it must not appear under Built-in Git Actions.
	p := provider.NewFakeProvider()
	fe := &action.FakeExecutor{}
	columnConfigs := []config.ColumnConfig{
		{Name: "Empty", Actions: map[string]config.Action{
			"M": {Name: "Col Merge", Type: "shell", Command: "echo m", Scope: "board"},
		}},
	}
	b := NewBoard(p, nil, config.DefaultGitActions(), columnConfigs, fe, "matteobortolazzo", "lazyboards", "github", 0, 0, 0, "Working", false, false, nil, nil, "", "")
	m, _ := b.Update(boardFetchedMsg{board: provider.Board{
		Columns: []provider.Column{{Title: "Empty", Cards: nil}},
	}})
	b = m.(Board)

	content := b.buildHelpContent()
	gitSection := content[strings.Index(content, "Built-in Git Actions"):]
	if strings.Contains(gitSection, "Mergetool") {
		t.Errorf("Built-in Git Actions section should omit column-overridden key M, got:\n%s", gitSection)
	}
}
