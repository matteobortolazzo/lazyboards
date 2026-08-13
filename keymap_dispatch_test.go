package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// --- Registry dispatch seam (#489) ---
//
// handleNormalModeKey/handleDetailFocusedKey cut over from a hardcoded
// switch msg.String() to keymap.Keymap.Lookup. Default-table parity for
// every existing built-in binding is covered by the pre-existing
// suites (actions_test.go, navigation_test.go, detail_panel_test.go,
// key_sequence_test.go, close_mode_test.go, delete_mode_test.go) -- see the
// plan's Test Strategy. The tests here cover what the registry newly makes
// possible: user overrides winning over a built-in, explicit unbind,
// lowercase custom-action dispatch (the A-Z gate removal), built-ins
// participating in multi-key sequences, per-column overlay scoping, and the
// Alt-strip-and-retry fallback semantics (A3), including the required
// negative case.

// boardWithOverrideKeymap returns b with its active keymap replaced by one
// merging keymap.Defaults() (every built-in normal/detail binding) with the
// given user overrides, mirroring how main.go applies
// config.ResolveKeymap once at startup -- before any hints are shown. Hints
// are rebuilt immediately afterward so tests can assert on b.normalHints
// (or, once focused, b.statusBar.hints) without a separate rebuild call.
func boardWithOverrideKeymap(t *testing.T, b Board, modes map[keymap.Mode]keymap.Table, columns map[string]keymap.Table) Board {
	t.Helper()
	km, err := keymap.Resolve(keymap.Defaults(), keymap.Tables{Modes: modes, Columns: columns})
	if err != nil {
		t.Fatalf("keymap.Resolve() returned unexpected error: %v", err)
	}
	b = b.withKeymap(km)
	b.rebuildNormalHints()
	if b.detailFocused {
		b.rebuildDetailHints()
	}
	return b
}

// --- AC2: a user override wins over a normal-mode/detail-panel built-in ---

func TestKeymapDispatch_UserOverrideWinsOverBuiltinKey(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"n": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)

	b = sendKey(t, b, keyMsg("n"))

	if b.mode == createMode {
		t.Fatal("mode = createMode, want the user override (board.refresh) to win over the built-in card.new")
	}
	if !b.refreshing {
		t.Error("expected refreshing=true after 'n' was remapped to board.refresh")
	}
}

// --- `~` unbind is a no-op ---

func TestKeymapDispatch_ExplicitUnbindIsNoOp(t *testing.T) {
	b := newLoadedTestBoard(t)
	before := b.mode
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"n": keymap.UnboundBinding()},
	}, nil)

	b = sendKey(t, b, keyMsg("n"))

	if b.mode != before {
		t.Errorf("mode = %v after pressing an explicitly unbound key, want unchanged (%v)", b.mode, before)
	}
}

// --- AC3: the A-Z gate is gone -- a lowercase key can dispatch a custom action ---

func TestKeymapDispatch_LowercaseKeyDispatchesCustomAction(t *testing.T) {
	b, fe := newActionTestBoard(t, nil)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"z": keymap.ActionBinding(keymap.Action{Name: "Lowercase action", Type: "url", URL: "https://example.com/{number}"}),
		},
	}, nil)

	sendKey(t, b, keyMsg("z"))

	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("expected 1 OpenURL call for a lowercase-key custom action, got %d", len(fe.OpenURLCalls))
	}
}

// --- AC4: built-ins participate in multi-key sequences ---

func TestKeymapDispatch_BuiltinParticipatesInPendingSequence(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"z d": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)

	b = sendKey(t, b, keyMsg("z"))
	if b.pendingSeq != "z" {
		t.Fatalf("pendingSeq = %q, want %q after the prefix key of a built-in sequence", b.pendingSeq, "z")
	}
	b = sendKey(t, b, keyMsg("d"))

	if !b.refreshing {
		t.Error(`expected refreshing=true after completing the built-in sequence "z d"`)
	}
	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after dispatch, want empty", b.pendingSeq)
	}
}

func TestKeymapDispatch_BuiltinThreeKeySequenceDispatches(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"z b c": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)

	b = sendKey(t, b, keyMsg("z"))
	if b.pendingSeq != "z" {
		t.Fatalf("pendingSeq = %q, want %q", b.pendingSeq, "z")
	}
	b = sendKey(t, b, keyMsg("b"))
	if b.pendingSeq != "z b" {
		t.Fatalf("pendingSeq = %q, want %q", b.pendingSeq, "z b")
	}
	b = sendKey(t, b, keyMsg("c"))

	if !b.refreshing {
		t.Error(`expected refreshing=true after completing the built-in 3-key sequence "z b c"`)
	}
}

func TestKeymapDispatch_PendingSequenceEscRestoresHintsAfterBuiltinPrefix(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"z d": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)

	before := append([]Hint{}, b.normalHints...)
	b = sendKey(t, b, keyMsg("z"))
	if b.pendingSeq != "z" {
		t.Fatalf("precondition: pendingSeq = %q, want %q after the prefix key of a built-in sequence", b.pendingSeq, "z")
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after esc, want empty", b.pendingSeq)
	}
	if len(b.statusBar.hints) != len(before) {
		t.Errorf("hints = %+v after esc, want the normal-mode hints restored: %+v", b.statusBar.hints, before)
	}
	// The continuation key must not still complete the cancelled sequence.
	b = sendKey(t, b, keyMsg("d"))
	if b.refreshing {
		t.Error("expected refreshing=false: 'd' after a cancelled sequence must not complete it")
	}
}

func TestKeymapDispatch_UnmatchedContinuationAfterBuiltinPrefixWarnsAndCancels(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"z d": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)

	b = sendKey(t, b, keyMsg("z"))
	if b.pendingSeq != "z" {
		t.Fatalf("precondition: pendingSeq = %q, want %q after the prefix key of a built-in sequence", b.pendingSeq, "z")
	}
	b = sendKey(t, b, keyMsg("y")) // "z y" is not bound to anything

	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after an unmatched continuation, want empty", b.pendingSeq)
	}
	if b.refreshing {
		t.Error("refreshing = true, want the unmatched continuation to not dispatch board.refresh")
	}
	if b.statusBar.message == "" {
		t.Error("expected a status-bar warning message after an unmatched continuation, got none")
	}
}

// --- #502: the "g" go-prefix is a built-in multi-key sequence, generalized
// through the same registry/dispatch machinery as any other prefix key ---

// TestKeymapDispatch_BareGShowsWhichKeyHintsForGoPrefix covers the #502
// acceptance criterion that bare "g" in normal mode (against the shipped
// defaults, no override needed -- "g a"/"g r" are default bindings) enters a
// pending state and shows which-key hints listing both completions, in
// canonical sorted order.
func TestKeymapDispatch_BareGShowsWhichKeyHintsForGoPrefix(t *testing.T) {
	b := newLoadedTestBoard(t)

	b = sendKey(t, b, keyMsg("g"))

	if b.pendingSeq != "g" {
		t.Fatalf("pendingSeq = %q, want %q after the built-in go-prefix key", b.pendingSeq, "g")
	}
	wantHints := []Hint{
		{Key: "g a", Desc: mustFindCommand(t, keymap.CommandNavAgent).Desc},
		{Key: "g r", Desc: mustFindCommand(t, keymap.CommandNavReference).Desc},
		{Key: "esc", Desc: "cancel"},
	}
	hints := b.statusBar.hints
	if len(hints) != len(wantHints) {
		t.Fatalf("hints after bare 'g' = %v, want %v", hints, wantHints)
	}
	for i, want := range wantHints {
		if hints[i] != want {
			t.Errorf("hints[%d] after bare 'g' = %v, want %v", i, hints[i], want)
		}
	}
}

// TestKeymapDispatch_BareGEscCancelsAndRestoresHints is the go-prefix analog
// of TestKeymapDispatch_PendingSequenceEscRestoresHintsAfterBuiltinPrefix:
// esc during the pending "g" state cancels it and restores the normal-mode
// hints, and the stale continuation key must not still complete it.
func TestKeymapDispatch_BareGEscCancelsAndRestoresHints(t *testing.T) {
	b := newBoardWithCollaborators(t)
	before := append([]Hint{}, b.normalHints...)

	b = sendKey(t, b, keyMsg("g"))
	if b.pendingSeq != "g" {
		t.Fatalf("precondition: pendingSeq = %q, want %q", b.pendingSeq, "g")
	}
	b = sendKey(t, b, arrowMsg(tea.KeyEsc))

	if b.pendingSeq != "" {
		t.Errorf("pendingSeq = %q after esc, want empty", b.pendingSeq)
	}
	if len(b.statusBar.hints) != len(before) {
		t.Errorf("hints = %+v after esc, want the normal-mode hints restored: %+v", b.statusBar.hints, before)
	}
	// The stale continuation key must not still complete the cancelled
	// go-prefix sequence: "a" alone is card.assign (enters assignMode), a
	// different outcome than completing "g a" (nav.agent) would produce, so
	// landing in assignMode is a strong positive signal "a" was NOT
	// swallowed as a continuation of the stale "g" prefix.
	b = sendKey(t, b, keyMsg("a"))
	if b.mode != assignMode {
		t.Errorf("mode after 'a' post-cancel = %v, want assignMode (card.assign fired fresh, not as a stale 'g' continuation)", b.mode)
	}
}

// --- #502 AC6: a user config can restore an old, pre-remap key ---

// TestKeymapDispatch_UserConfigRestoresPreRemapKey covers AC6: a user config
// binding `keymaps.normal: t: card.delete` restores the old, pre-#502 key
// for card.delete (its default moved to "d") -- exercised through the real
// config.Load -> config.ResolveKeymap -> withKeymap pipeline
// (newKeymapConfigLoadedTestBoard), mirroring main.go's actual startup
// wiring.
func TestKeymapDispatch_UserConfigRestoresPreRemapKey(t *testing.T) {
	b := newKeymapConfigLoadedTestBoard(t, `keymaps:
  normal:
    t: card.delete
`)

	b = sendKey(t, b, keyMsg("t"))

	if b.mode != deleteMode {
		t.Errorf("mode after 't' with the restoring keymaps: override = %v, want deleteMode", b.mode)
	}
}

// --- #502 AC7: a user inline action can shadow a new built-in default ---

// TestKeymapDispatch_InlineActionShadowsRemappedBuiltinDefaults covers AC7: a
// keymaps.normal inline action claiming any of D/P/A/G (#502's new built-in
// defaults) loads without error and shadows the built-in, since a user
// binding always wins over a default. Exercised through
// newConfigLoadedActionTestBoard's real config.Load -> ResolveKeymap ->
// withKeymap wiring with a FakeExecutor attached, so the shadowing is
// asserted by what the shell action actually runs.
func TestKeymapDispatch_InlineActionShadowsRemappedBuiltinDefaults(t *testing.T) {
	localYAML := `provider: github
keymaps:
  normal:
    D:
      name: Custom D
      type: shell
      command: "echo custom-d"
      scope: board
    P:
      name: Custom P
      type: shell
      command: "echo custom-p"
      scope: board
    A:
      name: Custom A
      type: shell
      command: "echo custom-a"
      scope: board
    G:
      name: Custom G
      type: shell
      command: "echo custom-g"
      scope: board
`
	b, fe := newConfigLoadedActionTestBoard(t, localYAML)

	for _, tc := range []struct {
		key         string
		wantCommand string
	}{
		{"D", "echo custom-d"},
		{"P", "echo custom-p"},
		{"A", "echo custom-a"},
		{"G", "echo custom-g"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			m, cmd := b.Update(keyMsg(tc.key))
			board, ok := m.(Board)
			if !ok {
				t.Fatalf("Update(%q) returned %T, want Board", tc.key, m)
			}
			if cmd == nil {
				t.Fatalf("Update(%q) returned nil cmd, want the shell action's cmd", tc.key)
			}
			execCmds(cmd)

			if board.mode != normalMode {
				t.Errorf("mode after %q = %v, want normalMode (the legacy action, not the built-in modal, must fire)", tc.key, board.mode)
			}
			found := false
			for _, call := range fe.RunShellCalls {
				if call == tc.wantCommand {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("RunShellCalls after %q = %v, want it to contain %q (the legacy action shadowing the built-in default)", tc.key, fe.RunShellCalls, tc.wantCommand)
			}
		})
	}
}

// --- AC5: per-column overrides scope to that column only ---

func TestKeymapDispatch_ColumnOverlayScopesToItsOwnColumnOnly(t *testing.T) {
	b, fe := newActionTestBoardWithColumns(t, nil, []provider.Column{
		{Title: "Col A", Cards: []provider.Card{{Number: 1, Title: "Card"}}},
		{Title: "Col B", Cards: []provider.Card{{Number: 2, Title: "Card"}}},
	})
	b = boardWithOverrideKeymap(t, b, nil, map[string]keymap.Table{
		"col a": {"z": keymap.ActionBinding(keymap.Action{Name: "Col A only", Type: "url", URL: "https://example.com/{number}"})},
	})

	// Column 0 ("Col A") is overlaid: "z" fires.
	b = sendKey(t, b, keyMsg("z"))
	if len(fe.OpenURLCalls) != 1 {
		t.Fatalf("expected 1 OpenURL call on the overlaid column, got %d", len(fe.OpenURLCalls))
	}

	// Column 1 ("Col B") has no overlay: "z" is not bound there.
	b = sendKey(t, b, arrowMsg(tea.KeyTab))
	sendKey(t, b, keyMsg("z"))
	if len(fe.OpenURLCalls) != 1 {
		t.Errorf("expected no additional OpenURL calls outside the overlaid column, got %d total", len(fe.OpenURLCalls))
	}
}

// --- Alt-strip-and-retry fallback (A3) ---

func TestKeymapDispatch_AltExplicitBindingWins(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {"alt+n": keymap.CommandBinding(keymap.CommandBoardRefresh)},
	}, nil)

	b = sendKey(t, b, altKeyMsg("n"))

	if b.mode == createMode {
		t.Fatal("mode = createMode, want the explicit alt+n binding (board.refresh) to win, not the base 'n' key's card.new")
	}
	if !b.refreshing {
		t.Error("expected refreshing=true: an explicit alt+n binding must dispatch directly, not fall back")
	}
}

func TestKeymapDispatch_AltStrippedFallbackFiresInlineAction(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"z": keymap.ActionBinding(keymap.Action{Name: "Zeta", Type: "shell", Command: "echo {comment}"}),
		},
	}, nil)

	b = sendKey(t, b, altKeyMsg("z"))

	if b.mode != commentMode {
		t.Fatalf(`mode = %v, want commentMode after alt+z (unbound) falls back to the inline action bound to "z"`, b.mode)
	}
}

// TestKeymapDispatch_AltStrippedFallbackDoesNotFireBuiltin is the required
// explicit-risk-coverage test (.claude/rules/testing.md): without the A3
// restriction, alt+n would fall back to the unmodified 'n' key and fire the
// built-in card.new command. alt+n is unbound (no explicit alt+n entry
// anywhere in the resolved table) and the base key 'n' resolves to a
// built-in command, not an inline action -- the fallback must never reach
// it.
func TestKeymapDispatch_AltStrippedFallbackDoesNotFireBuiltin(t *testing.T) {
	b := newLoadedTestBoard(t) // default keymap only: 'n' -> card.new, no alt+n bound

	before := b.mode
	b = sendKey(t, b, altKeyMsg("n"))

	if b.mode == createMode {
		t.Fatal("mode = createMode, want alt+n (unbound) to NOT fall back and fire the built-in card.new command")
	}
	if b.mode != before {
		t.Errorf("mode = %v, want unchanged (%v) -- the alt-stripped fallback must never dispatch a built-in command", b.mode, before)
	}
}

func TestKeymapDispatch_AltStrippedFallbackEntersPendingWhenAllCandidatesAreInline(t *testing.T) {
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"z a": keymap.ActionBinding(keymap.Action{Name: "Za", Type: "shell", Command: "echo za"}),
		},
	}, nil)

	b = sendKey(t, b, altKeyMsg("z"))

	if b.pendingSeq != "z" {
		t.Fatalf("pendingSeq = %q, want %q after alt+z falls back to the inline-action-only prefix", b.pendingSeq, "z")
	}
}

// --- Untrusted hint label sanitization (security review, #489) ---

// TestKeymapDispatch_PendingSeqHintSanitizesControlBytesInActionName covers
// the describeBinding sink used by seqHints (the which-key pending-sequence
// hint bar): an inline action's Name reaching that bar via a multi-key
// prefix must be sanitized exactly like inlineActionHints' own Desc field.
func TestKeymapDispatch_PendingSeqHintSanitizesControlBytesInActionName(t *testing.T) {
	rawName := "Evil\x1b[31m\nAction\x07"
	b := newLoadedTestBoard(t)
	b = boardWithOverrideKeymap(t, b, map[keymap.Mode]keymap.Table{
		keymap.ModeNormal: {
			"z a": keymap.ActionBinding(keymap.Action{Name: rawName, Type: "url", URL: "https://example.com/{number}"}),
		},
	}, nil)

	b = sendKey(t, b, keyMsg("z"))
	if b.pendingSeq != "z" {
		t.Fatalf("precondition: pendingSeq = %q, want %q", b.pendingSeq, "z")
	}

	view := b.statusBar.View(200)
	if strings.Count(view, "\n") != 0 {
		t.Errorf("View() = %q, want a single physical line (0 embedded newlines), got %d", view, strings.Count(view, "\n"))
	}
	want := sanitizeSingleLine(rawName)
	found := false
	for _, h := range b.statusBar.hints {
		if h.Desc == want {
			found = true
		}
		if strings.ContainsAny(h.Desc, "\n\x1b\x07") {
			t.Errorf("pending-sequence hint Desc = %q, still contains raw control/ANSI bytes", h.Desc)
		}
	}
	if !found {
		t.Errorf("pending-sequence hints = %+v, want a sanitized Desc %q", b.statusBar.hints, want)
	}
}

// --- Unit: the alt-fallback eligibility decision (state machine) ---
//
// Justified per the plan's Test Strategy: the strip-and-retry precedence has
// four outcomes an end-to-end test can only sample, and A3's negative case
// is invisible in happy-path flows. altFallbackEligible is the pure
// decision predicate lookupWithAltFallback consults after re-looking-up the
// stripped sequence: eligible only for an exact inline-action match, or a
// pending prefix whose candidates are ALL inline actions.
func TestAltFallbackEligible_FourOutcomes(t *testing.T) {
	tests := []struct {
		name   string
		result keymap.Result
		want   bool
	}{
		{
			name:   "exact match to a built-in command must not fire",
			result: keymap.Result{Outcome: keymap.OutcomeMatch, Binding: keymap.CommandBinding(keymap.CommandCardNew)},
			want:   false,
		},
		{
			name:   "exact match to an inline action fires",
			result: keymap.Result{Outcome: keymap.OutcomeMatch, Binding: keymap.ActionBinding(keymap.Action{Name: "X"})},
			want:   true,
		},
		{
			name: "pending prefix whose candidates are all inline actions is eligible",
			result: keymap.Result{Outcome: keymap.OutcomePending, Candidates: []keymap.Candidate{
				{Sequence: "z a", Binding: keymap.ActionBinding(keymap.Action{Name: "Za"})},
				{Sequence: "z b", Binding: keymap.ActionBinding(keymap.Action{Name: "Zb"})},
			}},
			want: true,
		},
		{
			name: "pending prefix with a mixed built-in candidate must not fire",
			result: keymap.Result{Outcome: keymap.OutcomePending, Candidates: []keymap.Candidate{
				{Sequence: "z a", Binding: keymap.ActionBinding(keymap.Action{Name: "Za"})},
				{Sequence: "z b", Binding: keymap.CommandBinding(keymap.CommandCardNew)},
			}},
			want: false,
		},
		{
			name:   "no match at all",
			result: keymap.Result{Outcome: keymap.OutcomeNoMatch},
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := altFallbackEligible(tc.result); got != tc.want {
				t.Errorf("altFallbackEligible(%+v) = %v, want %v", tc.result, got, tc.want)
			}
		})
	}
}
