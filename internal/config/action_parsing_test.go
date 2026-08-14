package config

import "testing"

func TestDefaultGitActions_LazygitStyleKeys(t *testing.T) {
	actions := DefaultGitActions()

	cases := []struct {
		key     string
		command string
	}{
		{"P", "git push"},
		{"p", "git pull --rebase"},
		{"f", "git fetch"},
		{"m", "git mergetool"},
		{"s", "git stash push"},
		{"S", "git stash pop"},
	}

	for _, c := range cases {
		act, ok := actions[c.key]
		if !ok {
			t.Fatalf("DefaultGitActions() missing key %q", c.key)
		}
		if act.Command != c.command {
			t.Errorf("DefaultGitActions()[%q].Command = %q, want %q", c.key, act.Command, c.command)
		}
		if act.Type != "shell" {
			t.Errorf("DefaultGitActions()[%q].Type = %q, want %q", c.key, act.Type, "shell")
		}
		if act.Scope != "board" {
			t.Errorf("DefaultGitActions()[%q].Scope = %q, want %q", c.key, act.Scope, "board")
		}
	}
}

func TestCardSpecificVarPattern_MatchesWindow(t *testing.T) {
	if !cardSpecificVarPattern.MatchString("{window}") {
		t.Error("cardSpecificVarPattern should match {window}: it is card-derived (per-card cenci window name) and documented as card-specific in the README")
	}
}
