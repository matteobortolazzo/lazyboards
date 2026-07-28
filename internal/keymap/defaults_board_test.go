package keymap

import "testing"

// boardDefaultCase pins one (mode, key) -> CommandID default binding,
// transcribed directly from the current handler switch statements
// (handleNormalModeKey, mode_handlers.go:169-430; handleDetailFocusedKey,
// update.go:1289-1357) -- never from a production keymap data file, so a
// bug introduced into the catalogue/defaults data can't also corrupt the
// expectation.
type boardDefaultCase struct {
	key  string
	want CommandID
}

// normalDefaultCases enumerates every key handleNormalModeKey's switch
// dispatches today (26 case blocks -- three of which bind two keys to the
// same command -- plus the 1-9 default branch, 38 keys total).
var normalDefaultCases = []boardDefaultCase{
	{"q", "app.quit"},
	{"?", "app.help"},
	{"c", "app.config"},
	{"n", "card.new"},
	{"e", "card.edit"},
	{"o", "card.open_ticket"},
	{"p", "card.open_pr"},
	{"x", "card.close"},
	{"t", "card.delete"},
	{"a", "card.assign"},
	{"r", "board.refresh"},
	{"/", "board.search"},
	{"f", "board.filter"},
	{"u", "board.sort_order"},
	{"v", "view.pr_list"},
	{"i", "view.milestone_list"},
	{"w", "view.agent_list"},
	{"g", "view.git_panel"},
	{"d", "view.dispatch"},
	{"m", "nav.reference"},
	{"s", "nav.agent"},
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

// detailDefaultCases enumerates every key handleDetailFocusedKey's switch
// (plus its pre-switch esc/1-9 branches) dispatches today: it reuses
// app.quit, card.edit, board.refresh, card.open_ticket, nav.reference,
// card.open_pr, app.help, nav.column_next, nav.column_prev and
// nav.column_1..9 from the normal-mode catalogue, and adds detail.blur,
// detail.scroll_down, detail.scroll_up (25 keys total).
var detailDefaultCases = []boardDefaultCase{
	{"q", "app.quit"},
	{"e", "card.edit"},
	{"r", "board.refresh"},
	{"o", "card.open_ticket"},
	{"m", "nav.reference"},
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

// TestDefaults_NormalModeBindings asserts one test case per key currently
// handled by handleNormalModeKey: resolving Defaults() against an empty
// user layer and looking the key up against ModeNormal must produce an
// OutcomeMatch carrying exactly the expected command id.
func TestDefaults_NormalModeBindings(t *testing.T) {
	if len(normalDefaultCases) < 37 {
		t.Fatalf("normalDefaultCases has %d entries, want at least 37 (one per current handleNormalModeKey binding)", len(normalDefaultCases))
	}

	km := resolveOrFatal(t, Defaults(), Tables{})
	for _, tc := range normalDefaultCases {
		t.Run(tc.key, func(t *testing.T) {
			result := km.Lookup(ModeNormal, "", Sequence{Key(tc.key)})
			if result.Outcome != OutcomeMatch {
				t.Fatalf("Lookup(ModeNormal, \"\", %q) outcome = %v, want OutcomeMatch", tc.key, result.Outcome)
			}
			if result.Binding.Kind != BindingCommand || result.Binding.Command != tc.want {
				t.Errorf("Lookup(ModeNormal, \"\", %q) binding = %+v, want CommandBinding(%q)", tc.key, result.Binding, tc.want)
			}
		})
	}
}

// TestDefaults_DetailModeBindings asserts one test case per key currently
// handled by handleDetailFocusedKey (including keys that differ from
// normal mode), the same way TestDefaults_NormalModeBindings does for
// ModeNormal.
func TestDefaults_DetailModeBindings(t *testing.T) {
	if len(detailDefaultCases) < 25 {
		t.Fatalf("detailDefaultCases has %d entries, want at least 25 (one per current handleDetailFocusedKey binding)", len(detailDefaultCases))
	}

	km := resolveOrFatal(t, Defaults(), Tables{})
	for _, tc := range detailDefaultCases {
		t.Run(tc.key, func(t *testing.T) {
			result := km.Lookup(ModeDetail, "", Sequence{Key(tc.key)})
			if result.Outcome != OutcomeMatch {
				t.Fatalf("Lookup(ModeDetail, \"\", %q) outcome = %v, want OutcomeMatch", tc.key, result.Outcome)
			}
			if result.Binding.Kind != BindingCommand || result.Binding.Command != tc.want {
				t.Errorf("Lookup(ModeDetail, \"\", %q) binding = %+v, want CommandBinding(%q)", tc.key, result.Binding, tc.want)
			}
		})
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
