package keymap

import "testing"

// boardDefaultCase pins one (mode, key) -> CommandID default binding. Key is
// a canonical Sequence.String() form: a single key ("q") or a space-joined
// multi-key sequence ("g r") for the two-key go-prefix #502 introduces.
//
// Unaffected entries are transcribed directly from the current handler
// switch statements (handleNormalModeKey, mode_handlers.go;
// handleDetailFocusedKey, update.go). The nine #502-remapped
// entries are transcribed from the ticket's remap table instead --
// deliberately never copied from the pre-#502 internal/keymap/
// defaults_board.go, so a bug in the production remap can't also corrupt
// the expectation it's checked against.
type boardDefaultCase struct {
	key  string
	want CommandID
}

// normalDefaultCases enumerates every key handleNormalModeKey is expected to
// dispatch after the #502 remap (26 case blocks -- three of which bind two
// keys to the same command -- plus the 1-9 default branch, 38 entries
// total; two of those 38 are now two-key "g "-prefixed sequences rather
// than single keys, per the ticket's remap table:
//
//	card.delete:          t -> d
//	view.dispatch:        d -> D
//	nav.reference:        m -> "g r"
//	view.milestone_list:  i -> m
//	nav.agent:            s -> "g a"
//	board.sort_order:     u -> s
//	view.pr_list:         v -> P
//	view.agent_list:      w -> A
//	view.git_panel:       g -> G
//
// Every other key keeps its pre-#502 binding.
var normalDefaultCases = []boardDefaultCase{
	{"q", "app.quit"},
	{"?", "app.help"},
	{"c", "app.config"},
	{"n", "card.new"},
	{"e", "card.edit"},
	{"o", "card.open_ticket"},
	{"p", "card.open_pr"},
	{"x", "card.close"},
	{"d", "card.delete"}, // #502 remap: was "t"
	{"a", "card.assign"},
	{"r", "board.refresh"},
	{"/", "board.search"},
	{"f", "board.filter"},
	{"s", "board.sort_order"},    // #502 remap: was "u"
	{"P", "view.pr_list"},        // #502 remap: was "v"
	{"m", "view.milestone_list"}, // #502 remap: was "i"
	{"A", "view.agent_list"},     // #502 remap: was "w"
	{"G", "view.git_panel"},      // #502 remap: was "g"
	{"D", "view.dispatch"},       // #502 remap: was "d"
	{"g r", "nav.reference"},     // #502 remap: was "m"
	{"g a", "nav.agent"},         // #502 remap: was "s"
	{"l", "nav.detail_focus"},
	{"right", "nav.detail_focus"},
	{"j", "nav.cursor_down"},
	{"down", "nav.cursor_down"},
	{"k", "nav.cursor_up"},
	{"up", "nav.cursor_up"},
	{"tab", "nav.column_next"},
	{"shift+tab", "nav.column_prev"},
	{"1", "nav.column_1"},
	{"2", "nav.column_2"},
	{"3", "nav.column_3"},
	{"4", "nav.column_4"},
	{"5", "nav.column_5"},
	{"6", "nav.column_6"},
	{"7", "nav.column_7"},
	{"8", "nav.column_8"},
	{"9", "nav.column_9"},
}

// detailDefaultCases enumerates every key handleDetailFocusedKey is expected
// to dispatch after the #502 remap (plus its pre-switch esc/1-9 branches):
// it reuses app.quit, card.edit, board.refresh, card.open_ticket,
// nav.reference, card.open_pr, app.help, nav.column_next, nav.column_prev
// and nav.column_1..9 from the normal-mode catalogue, and adds detail.blur,
// detail.scroll_down, detail.scroll_up (25 keys total). Only nav.reference's
// key changes here (m -> "g r", mirroring the normal-mode remap); nav.agent
// is not bound in the detail panel today and #502 does not add it there.
var detailDefaultCases = []boardDefaultCase{
	{"q", "app.quit"},
	{"e", "card.edit"},
	{"r", "board.refresh"},
	{"o", "card.open_ticket"},
	{"g r", "nav.reference"}, // #502 remap: was "m"
	{"p", "card.open_pr"},
	{"?", "app.help"},
	{"h", "detail.blur"},
	{"left", "detail.blur"},
	{"esc", "detail.blur"},
	{"j", "detail.scroll_down"},
	{"down", "detail.scroll_down"},
	{"k", "detail.scroll_up"},
	{"up", "detail.scroll_up"},
	{"tab", "nav.column_next"},
	{"shift+tab", "nav.column_prev"},
	{"1", "nav.column_1"},
	{"2", "nav.column_2"},
	{"3", "nav.column_3"},
	{"4", "nav.column_4"},
	{"5", "nav.column_5"},
	{"6", "nav.column_6"},
	{"7", "nav.column_7"},
	{"8", "nav.column_8"},
	{"9", "nav.column_9"},
}

// TestDefaults_NormalModeBindings asserts one test case per key
// handleNormalModeKey is expected to dispatch after the #502 remap:
// resolving Defaults() against an empty user layer and looking the key (or
// two-key sequence) up against ModeNormal must produce an OutcomeMatch
// carrying exactly the expected command id.
func TestDefaults_NormalModeBindings(t *testing.T) {
	if len(normalDefaultCases) < 37 {
		t.Fatalf("normalDefaultCases has %d entries, want at least 37 (one per current handleNormalModeKey binding)", len(normalDefaultCases))
	}

	km := resolveOrFatal(t, Defaults(), Tables{})
	for _, tc := range normalDefaultCases {
		t.Run(tc.key, func(t *testing.T) {
			seq, err := ParseSequence(tc.key)
			if err != nil {
				t.Fatalf("ParseSequence(%q) error: %v", tc.key, err)
			}
			result := km.Lookup(ModeNormal, "", seq)
			if result.Outcome != OutcomeMatch {
				t.Fatalf("Lookup(ModeNormal, \"\", %q) outcome = %v, want OutcomeMatch", tc.key, result.Outcome)
			}
			if result.Binding.Kind != BindingCommand || result.Binding.Command != tc.want {
				t.Errorf("Lookup(ModeNormal, \"\", %q) binding = %+v, want CommandBinding(%q)", tc.key, result.Binding, tc.want)
			}
		})
	}
}

// TestDefaults_DetailModeBindings asserts one test case per key
// handleDetailFocusedKey is expected to dispatch after the #502 remap, the
// same way TestDefaults_NormalModeBindings does for ModeNormal.
func TestDefaults_DetailModeBindings(t *testing.T) {
	if len(detailDefaultCases) < 25 {
		t.Fatalf("detailDefaultCases has %d entries, want at least 25 (one per current handleDetailFocusedKey binding)", len(detailDefaultCases))
	}

	km := resolveOrFatal(t, Defaults(), Tables{})
	for _, tc := range detailDefaultCases {
		t.Run(tc.key, func(t *testing.T) {
			seq, err := ParseSequence(tc.key)
			if err != nil {
				t.Fatalf("ParseSequence(%q) error: %v", tc.key, err)
			}
			result := km.Lookup(ModeDetail, "", seq)
			if result.Outcome != OutcomeMatch {
				t.Fatalf("Lookup(ModeDetail, \"\", %q) outcome = %v, want OutcomeMatch", tc.key, result.Outcome)
			}
			if result.Binding.Kind != BindingCommand || result.Binding.Command != tc.want {
				t.Errorf("Lookup(ModeDetail, \"\", %q) binding = %+v, want CommandBinding(%q)", tc.key, result.Binding, tc.want)
			}
		})
	}
}

// TestDefaults_NormalModeFreedKeysAreUnbound pins the #502 remap's freed
// keys: i (was view.milestone_list), t (was card.delete), u (was
// board.sort_order), v (was view.pr_list) and w (was view.agent_list) all
// move to new keys and nothing else claims their old ones, so each must now
// resolve to OutcomeNoMatch in normal mode.
func TestDefaults_NormalModeFreedKeysAreUnbound(t *testing.T) {
	freedKeys := []string{"i", "t", "u", "v", "w"}

	km := resolveOrFatal(t, Defaults(), Tables{})
	for _, key := range freedKeys {
		t.Run(key, func(t *testing.T) {
			result := km.Lookup(ModeNormal, "", Sequence{Key(key)})
			if result.Outcome != OutcomeNoMatch {
				t.Fatalf("Lookup(ModeNormal, \"\", %q) outcome = %v, want OutcomeNoMatch (freed by #502 remap)", key, result.Outcome)
			}
		})
	}
}

// TestDefaults_NormalModeBareGIsGoPrefix pins that bare "g" is no longer the
// git menu (which moves to "G") but a two-key go-prefix: OutcomePending
// with exactly two candidates, "g a" (nav.agent) and "g r" (nav.reference),
// in canonical sorted order.
func TestDefaults_NormalModeBareGIsGoPrefix(t *testing.T) {
	km := resolveOrFatal(t, Defaults(), Tables{})

	result := km.Lookup(ModeNormal, "", Sequence{Key("g")})
	if result.Outcome != OutcomePending {
		t.Fatalf("Lookup(ModeNormal, \"\", \"g\") outcome = %v, want OutcomePending", result.Outcome)
	}
	if len(result.Candidates) != 2 {
		t.Fatalf("Lookup(ModeNormal, \"\", \"g\") candidates = %+v, want exactly 2", result.Candidates)
	}

	ga, ok := findCandidate(result.Candidates, "g a")
	if !ok || ga.Binding.Kind != BindingCommand || ga.Binding.Command != CommandNavAgent {
		t.Errorf("Candidates missing %q -> CommandBinding(%q); got %+v", "g a", CommandNavAgent, result.Candidates)
	}
	gr, ok := findCandidate(result.Candidates, "g r")
	if !ok || gr.Binding.Kind != BindingCommand || gr.Binding.Command != CommandNavReference {
		t.Errorf("Candidates missing %q -> CommandBinding(%q); got %+v", "g r", CommandNavReference, result.Candidates)
	}

	if result.Candidates[0].Sequence != "g a" || result.Candidates[1].Sequence != "g r" {
		t.Errorf("Candidates = %+v, want canonical sorted order [\"g a\", \"g r\"]", result.Candidates)
	}
}

// TestDefaults_DetailModeMIsFreed pins that the detail panel's "m" no
// longer resolves to anything after the #502 remap moves nav.reference to
// "g r" there too -- unlike normal mode, no other command claims "m" in the
// detail table, so it must be OutcomeNoMatch.
func TestDefaults_DetailModeMIsFreed(t *testing.T) {
	km := resolveOrFatal(t, Defaults(), Tables{})

	result := km.Lookup(ModeDetail, "", Sequence{Key("m")})
	if result.Outcome != OutcomeNoMatch {
		t.Fatalf("Lookup(ModeDetail, \"\", \"m\") outcome = %v, want OutcomeNoMatch (freed by #502 remap)", result.Outcome)
	}
}

// TestDefaults_ColumnJumpsAreIndividuallyBoundNotComputed pins the specific
// acceptance criterion that 1-9 must be nine distinct catalogue ids/default
// bindings, not a single computed-offset default branch: unbinding
// nav.column_1 alone (leaving nav.column_2..9 untouched) must not disturb
// "2"'s resolution.
func TestDefaults_ColumnJumpsAreIndividuallyBoundNotComputed(t *testing.T) {
	user := Tables{Modes: map[Mode]Table{
		ModeNormal: {"1": UnboundBinding()},
	}}
	km := resolveOrFatal(t, Defaults(), user)

	result := km.Lookup(ModeNormal, "", Sequence{Key("1")})
	if result.Outcome != OutcomeNoMatch {
		t.Fatalf("Lookup(ModeNormal, \"\", \"1\") outcome = %v after unbinding nav.column_1, want OutcomeNoMatch", result.Outcome)
	}

	result = km.Lookup(ModeNormal, "", Sequence{Key("2")})
	if result.Outcome != OutcomeMatch || result.Binding.Command != "nav.column_2" {
		t.Fatalf("Lookup(ModeNormal, \"\", \"2\") = %+v after unbinding only nav.column_1, want an untouched OutcomeMatch(nav.column_2)", result)
	}
}
