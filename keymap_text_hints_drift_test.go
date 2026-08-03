package main

import (
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// hintDesc looks up the Desc of the Hint in hints whose Key matches key,
// failing the test immediately if no such Hint exists -- so a typo'd key in
// this file's own expectations surfaces as a clear test-setup failure
// instead of a silent empty-string comparison.
func hintDesc(t *testing.T, hints []Hint, key string) string {
	t.Helper()
	for _, h := range hints {
		if h.Key == key {
			return h.Desc
		}
	}
	t.Fatalf("hints has no entry with Key %q", key)
	return ""
}

// TestTextCommandDescs_MatchHintBarWording is the cross-package drift guard
// named in the #538 plan (Q3): internal/keymap cannot import package main,
// so command_text.go's desc strings for comment/search/delete are
// hand-duplicated literals copied from the *Hints vars in model.go (pre-#540)
// / keymap_text.go's hint builders (#540). This test lives in package main --
// the only package that can import internal/keymap *and* see
// b.commentHints()/b.searchHints()/deleteCommentHints/deleteConfirmHints --
// so it can assert the duplicated descs against the real producer values
// rather than hardcoding a second copy of its own, per
// .claude/rules/testing.md's "never copy an expected value from the
// implementation" rule. Mirrors TestGitPanelDefaults_MatchDefaultGitActions
// (git_keymap_defaults_test.go).
func TestTextCommandDescs_MatchHintBarWording(t *testing.T) {
	b := newTestBoard(t)

	cases := []struct {
		id   keymap.CommandID
		want string
	}{
		{"comment.submit", hintDesc(t, b.commentHints(), "enter")},
		{"comment.cancel", hintDesc(t, b.commentHints(), "esc")},

		{"search.apply", hintDesc(t, b.searchHints(), "enter")},
		{"search.cancel", hintDesc(t, b.searchHints(), "esc")},
		{"search.next_result", hintDesc(t, b.searchHints(), "↑/↓")},
		{"search.prev_result", hintDesc(t, b.searchHints(), "↑/↓")},

		{"delete.cancel", hintDesc(t, b.deleteCommentHints(), "esc")},
		{"delete.cancel", hintDesc(t, b.deleteConfirmHints(), "esc")},

		// delete.submit's desc is composed from the two per-step producers
		// rather than hardcoded as "Continue / Confirm" -- renaming either
		// step's hint must fail this test.
		{"delete.submit", hintDesc(t, b.deleteCommentHints(), "enter") + " / " + hintDesc(t, b.deleteConfirmHints(), "enter")},

		// #557: errorHints() is the registry-derived producer for errorMode's
		// hint bar (keymap_panels_test.go).
		{"error.retry", hintDesc(t, b.errorHints(), "r")},
		{"app.quit", hintDesc(t, b.errorHints(), "q")},
	}

	for _, tc := range cases {
		cmd, ok := keymap.FindCommand(tc.id)
		if !ok {
			t.Errorf("keymap.FindCommand(%q) returned ok == false, want a catalogued Command", tc.id)
			continue
		}
		if cmd.Desc != tc.want {
			t.Errorf("keymap.FindCommand(%q).Desc = %q, want %q (today's hint-bar wording)", tc.id, cmd.Desc, tc.want)
		}
	}
}
