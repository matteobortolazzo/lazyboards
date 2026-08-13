package keymap

import "testing"

// findCandidate looks up a Candidate by its canonical sequence string, for
// assertions that don't want to depend on Result.Candidates' order.
func findCandidate(candidates []Candidate, seq string) (Candidate, bool) {
	for _, c := range candidates {
		if c.Sequence == seq {
			return c, true
		}
	}
	return Candidate{}, false
}

// TestOutcome_ValuesAreDistinct guards against a copy-paste that gives two
// Outcome constants the same underlying value, which would compile fine but
// silently merge two semantically different lookup results.
func TestOutcome_ValuesAreDistinct(t *testing.T) {
	outcomes := []Outcome{OutcomeNoMatch, OutcomeMatch, OutcomePending}
	seen := make(map[Outcome]bool)
	for _, o := range outcomes {
		if seen[o] {
			t.Errorf("Outcome value %v is reused by more than one constant", o)
		}
		seen[o] = true
	}
}

// TestOutcome_NoMatchIsZeroValue pins that OutcomeNoMatch is the zero value
// of Outcome, so a zero-value Result (as would appear from an unset
// variable) reads as "no match" without a separate "ok" check.
func TestOutcome_NoMatchIsZeroValue(t *testing.T) {
	if OutcomeNoMatch != 0 {
		t.Errorf("OutcomeNoMatch = %d, want 0", int(OutcomeNoMatch))
	}
}

// TestLookup_ExactMatch pins the exact-match outcome against a synthetic
// table: a key with a direct binding resolves to OutcomeMatch and carries
// that binding.
func TestLookup_ExactMatch(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("create.card")},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{"n"})
	if result.Outcome != OutcomeMatch {
		t.Fatalf("Lookup outcome = %v, want OutcomeMatch", result.Outcome)
	}
	if result.Binding.Kind != BindingCommand || result.Binding.Command != "create.card" {
		t.Errorf("Lookup binding = %+v, want CommandBinding(\"create.card\")", result.Binding)
	}
}

// TestLookup_PendingPrefix pins the pending outcome: a key that is a strict
// prefix of one or more longer bound sequences returns OutcomePending with
// every matching candidate.
func TestLookup_PendingPrefix(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {
			"g d": CommandBinding("go.down"),
			"g r": CommandBinding("go.right"),
		},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{"g"})
	if result.Outcome != OutcomePending {
		t.Fatalf("Lookup outcome = %v, want OutcomePending", result.Outcome)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("Lookup candidates = %+v, want 2 entries", result.Candidates)
	}
	gd, ok := findCandidate(result.Candidates, "g d")
	if !ok || gd.Binding.Command != "go.down" {
		t.Errorf("Candidates missing %q -> CommandBinding(\"go.down\"); got %+v", "g d", result.Candidates)
	}
	gr, ok := findCandidate(result.Candidates, "g r")
	if !ok || gr.Binding.Command != "go.right" {
		t.Errorf("Candidates missing %q -> CommandBinding(\"go.right\"); got %+v", "g r", result.Candidates)
	}
}

// TestLookup_NoMatch pins the no-match outcome: a key with no exact binding
// and no longer sequence starting with it returns OutcomeNoMatch with no
// candidates.
func TestLookup_NoMatch(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {
			"n":   CommandBinding("create.card"),
			"g d": CommandBinding("go.down"),
		},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{"z"})
	if result.Outcome != OutcomeNoMatch {
		t.Fatalf("Lookup outcome = %v, want OutcomeNoMatch", result.Outcome)
	}
	if len(result.Candidates) != 0 {
		t.Errorf("Lookup candidates = %+v, want none", result.Candidates)
	}
}

// TestLookup_ExactMatchBeatsPendingPrefix pins the precedence rule: when a
// key is both directly bound and a strict prefix of a longer sequence, the
// exact match wins -- mirroring resolveAction's "exact match first, then
// seqCandidates" order.
func TestLookup_ExactMatchBeatsPendingPrefix(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {
			"g":   CommandBinding("go"),
			"g d": CommandBinding("go.down"),
		},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{"g"})
	if result.Outcome != OutcomeMatch {
		t.Fatalf("Lookup outcome = %v, want OutcomeMatch (exact beats pending)", result.Outcome)
	}
	if result.Binding.Command != "go" {
		t.Errorf("Lookup binding = %+v, want CommandBinding(\"go\")", result.Binding)
	}
}

// TestLookup_UnboundExactKeyIsNoMatch pins that an explicitly unbound key
// (BindingUnbound) is treated as absent for exact matching -- it does not
// fall back to the default binding it replaced (Resolve already discarded
// that default), and it must not appear as a match.
func TestLookup_UnboundExactKeyIsNoMatch(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("create.card")},
	}}
	user := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": UnboundBinding()},
	}}
	km := resolveOrFatal(t, defaults, user)

	result := km.Lookup(ModeNormal, "", Sequence{"n"})
	if result.Outcome != OutcomeNoMatch {
		t.Fatalf("Lookup outcome = %v, want OutcomeNoMatch for an unbound key", result.Outcome)
	}
}

// TestLookup_UnboundKeyDropsPendingCandidate pins that unbinding a longer
// sequence removes it from another key's pending-candidate list, rather
// than stranding a dangling pending state -- unbinding "g d" (with no other
// sequence starting with "g") makes "g" a no-match, not a pending prefix
// with zero real candidates.
func TestLookup_UnboundKeyDropsPendingCandidate(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"g d": CommandBinding("go.down")},
	}}
	user := Tables{Modes: map[Mode]Table{
		ModeNormal: {"g d": UnboundBinding()},
	}}
	km := resolveOrFatal(t, defaults, user)

	result := km.Lookup(ModeNormal, "", Sequence{"g"})
	if result.Outcome != OutcomeNoMatch {
		t.Fatalf("Lookup outcome = %v, want OutcomeNoMatch once the only candidate under \"g\" is unbound", result.Outcome)
	}
	if len(result.Candidates) != 0 {
		t.Errorf("Lookup candidates = %+v, want none", result.Candidates)
	}
}

// TestLookup_UnboundKeyDropsOnlyItsOwnCandidate mirrors the drop above but
// confirms a sibling candidate under the same prefix survives -- unbinding
// "g d" must not also drop "g r".
func TestLookup_UnboundKeyDropsOnlyItsOwnCandidate(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {
			"g d": CommandBinding("go.down"),
			"g r": CommandBinding("go.right"),
		},
	}}
	user := Tables{Modes: map[Mode]Table{
		ModeNormal: {"g d": UnboundBinding()},
	}}
	km := resolveOrFatal(t, defaults, user)

	result := km.Lookup(ModeNormal, "", Sequence{"g"})
	if result.Outcome != OutcomePending {
		t.Fatalf("Lookup outcome = %v, want OutcomePending (one bound sibling remains)", result.Outcome)
	}
	if _, ok := findCandidate(result.Candidates, "g d"); ok {
		t.Errorf("Candidates still contains unbound %q; want it dropped", "g d")
	}
	if _, ok := findCandidate(result.Candidates, "g r"); !ok {
		t.Errorf("Candidates missing bound sibling %q; want it retained", "g r")
	}
}

// TestLookup_ColumnOverlayWinsForNormalAndDetail pins that a column overlay
// wins on conflict for both ModeNormal and ModeDetail lookups against that
// column.
func TestLookup_ColumnOverlayWinsForNormalAndDetail(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("normal.default")},
		ModeDetail: {"n": CommandBinding("detail.default")},
	}}
	user := Tables{Columns: map[string]Table{
		"backlog": {"n": CommandBinding("column.override")},
	}}
	km := resolveOrFatal(t, defaults, user)

	for _, mode := range []Mode{ModeNormal, ModeDetail} {
		result := km.Lookup(mode, "backlog", Sequence{"n"})
		if result.Outcome != OutcomeMatch || result.Binding.Command != "column.override" {
			t.Errorf("Lookup(%q, \"backlog\", \"n\") = %+v, want the column override", mode, result)
		}
	}
}

// TestLookup_ColumnOverlayLeavesOtherColumnsUntouched asserts a different
// column (or no column) still resolves against the un-overlaid mode table.
func TestLookup_ColumnOverlayLeavesOtherColumnsUntouched(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"n": CommandBinding("normal.default")},
	}}
	user := Tables{Columns: map[string]Table{
		"backlog": {"n": CommandBinding("column.override")},
	}}
	km := resolveOrFatal(t, defaults, user)

	for _, column := range []string{"", "in-progress"} {
		result := km.Lookup(ModeNormal, column, Sequence{"n"})
		if result.Outcome != OutcomeMatch || result.Binding.Command != "normal.default" {
			t.Errorf("Lookup(ModeNormal, %q, \"n\") = %+v, want the un-overlaid default", column, result)
		}
	}
}

// TestLookup_ColumnOverlayIgnoredOutsideNormalAndDetail pins that the
// column overlay never applies to modes other than normal/detail, even when
// a column name is passed in.
func TestLookup_ColumnOverlayIgnoredOutsideNormalAndDetail(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeSearch: {"n": CommandBinding("search.default")},
	}}
	user := Tables{Columns: map[string]Table{
		"backlog": {"n": CommandBinding("column.override")},
	}}
	km := resolveOrFatal(t, defaults, user)

	result := km.Lookup(ModeSearch, "backlog", Sequence{"n"})
	if result.Outcome != OutcomeMatch || result.Binding.Command != "search.default" {
		t.Errorf("Lookup(ModeSearch, \"backlog\", \"n\") = %+v, want the un-overlaid ModeSearch default", result)
	}
}

// TestLookup_ColumnOverlayMatchesCaseInsensitively pins that the column
// name passed into Lookup is matched case-insensitively against the
// (lowercased-on-entry) Tables.Columns keys.
func TestLookup_ColumnOverlayMatchesCaseInsensitively(t *testing.T) {
	user := Tables{Columns: map[string]Table{
		"Backlog": {"n": CommandBinding("column.override")},
	}}
	km := resolveOrFatal(t, Tables{}, user)

	for _, column := range []string{"Backlog", "BACKLOG", "backlog"} {
		result := km.Lookup(ModeNormal, column, Sequence{"n"})
		if result.Outcome != OutcomeMatch || result.Binding.Command != "column.override" {
			t.Errorf("Lookup(ModeNormal, %q, \"n\") = %+v, want the column override", column, result)
		}
	}
}

// TestLookup_CtrlCResolvesToQuitAgainstEmptyTable pins the hard-wired
// exception: ctrl+c resolves to app.quit even against a table with no
// bindings at all.
func TestLookup_CtrlCResolvesToQuitAgainstEmptyTable(t *testing.T) {
	km := resolveOrFatal(t, Tables{}, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{"ctrl+c"})
	if result.Outcome != OutcomeMatch {
		t.Fatalf("Lookup outcome = %v, want OutcomeMatch", result.Outcome)
	}
	if result.Binding.Kind != BindingCommand || result.Binding.Command != CommandQuit {
		t.Errorf("Lookup binding = %+v, want CommandBinding(CommandQuit)", result.Binding)
	}
}

// TestLookup_CtrlCResolvesToQuitEvenWhenRebound pins that ctrl+c resolves
// to app.quit regardless of table contents -- even a table that explicitly
// rebinds ctrl+c to something else cannot override the hard-wire.
func TestLookup_CtrlCResolvesToQuitEvenWhenRebound(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"ctrl+c": CommandBinding("custom.command")},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{"ctrl+c"})
	if result.Outcome != OutcomeMatch || result.Binding.Command != CommandQuit {
		t.Errorf("Lookup binding = %+v, want CommandBinding(CommandQuit) despite the table rebinding ctrl+c", result.Binding)
	}
}

// TestLookup_CtrlCResolvesToQuitMidPending pins the "last key, not whole
// sequence" rule: a multi-key sequence ending in ctrl+c resolves to
// app.quit even mid-pending (e.g. after "g"), so a pending prefix can never
// strand a user without a way to quit.
func TestLookup_CtrlCResolvesToQuitMidPending(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"g d": CommandBinding("go.down")},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{"g", "ctrl+c"})
	if result.Outcome != OutcomeMatch || result.Binding.Command != CommandQuit {
		t.Errorf("Lookup(\"g ctrl+c\") = %+v, want CommandBinding(CommandQuit)", result)
	}
}

// TestLookup_CtrlCOnlyShortCircuitsAsLastKey pins the converse of the "last
// key" rule: ctrl+c earlier in the sequence (not as the last key) does not
// trigger the quit short-circuit -- only the sequence's final key is
// checked.
func TestLookup_CtrlCOnlyShortCircuitsAsLastKey(t *testing.T) {
	km := resolveOrFatal(t, Tables{}, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{"ctrl+c", "d"})
	if result.Outcome == OutcomeMatch && result.Binding.Command == CommandQuit {
		t.Errorf("Lookup(\"ctrl+c d\") = %+v, want it not to short-circuit to quit (ctrl+c is not the last key)", result)
	}
}

// TestLookup_BracketedPasteShapedKeyIsNoMatch pins the #547 whitespace
// guard: a Key shaped like bubbletea's bracketed-paste wrapping ("[g d]",
// a single Key containing an embedded space) can never appear in a
// validated table -- ParseKey rejects any key containing a space -- but a
// runtime Sequence is built directly from msg.String() with no validation
// (keymap_panels.go, keymap_text.go). Lookup itself must refuse to resolve
// such a Key rather than relying on the fact that no table entry could ever
// match it by construction.
func TestLookup_BracketedPasteShapedKeyIsNoMatch(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"g d": CommandBinding("go.down")},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{Key("[g d]")})
	if result.Outcome != OutcomeNoMatch {
		t.Fatalf("Lookup(Sequence{\"[g d]\"}) outcome = %v, want OutcomeNoMatch", result.Outcome)
	}
}

// TestLookup_UnbracketedSpaceKeyNeverCollidesWithMultiKeySequence pins the
// concrete collision the AC's "checked per-Key (never against
// seq.String())" clause guards against, and is the case that actually fails
// without the guard (unlike the bracket-wrapped case above, which can never
// collide with a real canonical key by construction): a single Key whose
// own string happens to equal a bound two-key canonical sequence --
// Sequence{Key("g d")}, one Key containing an embedded space, as opposed to
// the legitimate Sequence{Key("g"), Key("d")} -- must not resolve via
// seq.String()'s naive space-join producing the same "g d" text a real
// two-keypress sequence would. Today, before the guard, Lookup's
// query := seq.String() collapses this one-Key sequence to the exact string
// "g d" and incorrectly returns OutcomeMatch on the bound "go.down" command.
func TestLookup_UnbracketedSpaceKeyNeverCollidesWithMultiKeySequence(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"g d": CommandBinding("go.down")},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{Key("g d")})
	if result.Outcome != OutcomeNoMatch {
		t.Fatalf("Lookup(Sequence{\"g d\"} as a single whitespace-bearing Key) outcome = %v, want OutcomeNoMatch (it must never collide with the bound two-key sequence \"g\" \"d\")", result.Outcome)
	}
}

// TestLookup_WhitespaceBearingEarlierKeyStillQuitsOnCtrlC pins the mandatory
// risk-coverage guard: the whitespace guard must sit AFTER the ctrl+c
// short-circuit (which only inspects the sequence's last key), so a
// whitespace-bearing key earlier in the sequence can never strand a user
// without a way to quit.
func TestLookup_WhitespaceBearingEarlierKeyStillQuitsOnCtrlC(t *testing.T) {
	km := resolveOrFatal(t, Tables{}, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{Key("[g d]"), Key("ctrl+c")})
	if result.Outcome != OutcomeMatch || result.Binding.Command != CommandQuit {
		t.Errorf("Lookup(Sequence{\"[g d]\", \"ctrl+c\"}) = %+v, want CommandBinding(CommandQuit)", result)
	}
}

// TestLookup_LegitimateTwoKeySequenceStillResolves confirms the whitespace
// guard is scoped to per-Key whitespace (never Sequence.String(), whose
// space-joined canonical form is expected and legitimate): an ordinary
// two-key sequence with no whitespace inside either individual Key still
// resolves normally.
func TestLookup_LegitimateTwoKeySequenceStillResolves(t *testing.T) {
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {"g r": CommandBinding("go.right")},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{Key("g"), Key("r")})
	if result.Outcome != OutcomeMatch || result.Binding.Command != "go.right" {
		t.Errorf("Lookup(Sequence{\"g\", \"r\"}) = %+v, want CommandBinding(\"go.right\")", result)
	}
}

// TestLookup_PendingCandidatesStructurallyIdenticalForActionAndCommand pins
// the outcome-purity design decision: a pending result mixing an
// action-rhs and a command-rhs candidate under the same prefix carries no
// "is this a custom action" distinction anywhere in Result -- both are
// plain Candidate{Sequence, Binding} entries, differing only in
// Binding.Kind.
func TestLookup_PendingCandidatesStructurallyIdenticalForActionAndCommand(t *testing.T) {
	action := Action{Name: "Open PR", Type: "url", URL: "https://example.com"}
	defaults := Tables{Modes: map[Mode]Table{
		ModeNormal: {
			"g d": CommandBinding("go.down"),
			"g r": ActionBinding(action),
		},
	}}
	km := resolveOrFatal(t, defaults, Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{"g"})
	if result.Outcome != OutcomePending {
		t.Fatalf("Lookup outcome = %v, want OutcomePending", result.Outcome)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("Lookup candidates = %+v, want 2 entries", result.Candidates)
	}

	commandCandidate, ok := findCandidate(result.Candidates, "g d")
	if !ok || commandCandidate.Binding.Kind != BindingCommand || commandCandidate.Binding.Command != "go.down" {
		t.Errorf("Candidates[%q] = %+v, want CommandBinding(\"go.down\")", "g d", commandCandidate)
	}
	actionCandidate, ok := findCandidate(result.Candidates, "g r")
	if !ok || actionCandidate.Binding.Kind != BindingAction || actionCandidate.Binding.Action != action {
		t.Errorf("Candidates[%q] = %+v, want ActionBinding(%+v)", "g r", actionCandidate, action)
	}
}
