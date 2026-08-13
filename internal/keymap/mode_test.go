package keymap

import "testing"

// TestModes_AreUnique guards against a copy-pasted constant that compiles
// fine but silently shadows another mode's key surface.
func TestModes_AreUnique(t *testing.T) {
	seen := make(map[Mode]bool)
	for _, m := range Modes() {
		if seen[m] {
			t.Errorf("Modes() contains duplicate value %q", m)
		}
		seen[m] = true
	}
}

// TestModes_ExcludesColumns pins the plan's explicit carve-out: ModeColumns
// is a config namespace, not a resolvable key surface, so it must never
// appear in the enumeration Lookup callers iterate.
func TestModes_ExcludesColumns(t *testing.T) {
	for _, m := range Modes() {
		if m == ModeColumns {
			t.Fatalf("Modes() must not include ModeColumns")
		}
	}
}

// TestModes_NonTrivialCount catches an accidentally empty or truncated
// Modes() slice, which would otherwise fail silently (every other test in
// this file iterates it, so an empty slice makes them vacuously pass).
func TestModes_NonTrivialCount(t *testing.T) {
	if len(Modes()) < 15 {
		t.Fatalf("Modes() returned %d modes, want at least 15 (one per non-swallowing handler)", len(Modes()))
	}
}

// TestParseMode_RoundTripsEveryResolvableMode asserts every constant
// returned by Modes() parses back to itself via ParseMode, and that its
// Resolvable() reports true.
func TestParseMode_RoundTripsEveryResolvableMode(t *testing.T) {
	for _, m := range Modes() {
		got, err := ParseMode(string(m))
		if err != nil {
			t.Errorf("ParseMode(%q) returned unexpected error: %v", m, err)
			continue
		}
		if got != m {
			t.Errorf("ParseMode(%q) = %q, want %q", m, got, m)
		}
		if !m.Resolvable() {
			t.Errorf("Mode %q returned from Modes() must be Resolvable()", m)
		}
	}
}

// TestParseMode_AcceptsColumns pins the Q&A decision: ParseMode accepts
// "columns" (so #509 can route keymaps.columns.<name> through one name
// check) even though it is excluded from Modes() and is not itself
// resolvable.
func TestParseMode_AcceptsColumns(t *testing.T) {
	got, err := ParseMode("columns")
	if err != nil {
		t.Fatalf("ParseMode(\"columns\") returned unexpected error: %v", err)
	}
	if got != ModeColumns {
		t.Errorf("ParseMode(\"columns\") = %q, want %q", got, ModeColumns)
	}
	if got.Resolvable() {
		t.Errorf("ModeColumns.Resolvable() = true, want false")
	}
}

// TestParseMode_RejectsUnknown asserts ParseMode errors, naming the
// offending value, for a mode name that doesn't exist.
func TestParseMode_RejectsUnknown(t *testing.T) {
	_, err := ParseMode("nope")
	if err == nil {
		t.Fatal("ParseMode(\"nope\") returned nil error, want an error")
	}
}

// TestParseMode_RejectsLoadingAndCreating pins the assumption that
// loadingMode/creatingMode (which swallow every key per update.go's
// dispatcher) get no Mode constant at all.
func TestParseMode_RejectsLoadingAndCreating(t *testing.T) {
	for _, name := range []string{"loading", "creating"} {
		if _, err := ParseMode(name); err == nil {
			t.Errorf("ParseMode(%q) returned nil error, want an error (loading/creating swallow every key and have no Mode constant)", name)
		}
	}
}

// TestModeError_IsResolvable pins the Q&A decision to include an "error"
// Mode constant now (for future q/r bindings), distinct from the
// loading/creating exclusion above.
func TestModeError_IsResolvable(t *testing.T) {
	found := false
	for _, m := range Modes() {
		if m == ModeError {
			found = true
		}
	}
	if !found {
		t.Fatal("Modes() does not include ModeError")
	}
	if !ModeError.Resolvable() {
		t.Fatal("ModeError.Resolvable() = false, want true")
	}
}

// TestModeNames_AreSnakeCase pins the Q&A naming decision for multi-word
// modes (close_confirm, label_confirm, pr_list, pr_picker, milestone_list,
// agent_list, git_panel), superseding the epic body's informal "prlist"
// spelling.
func TestModeNames_AreSnakeCase(t *testing.T) {
	want := []Mode{
		ModeCloseConfirm,
		ModeLabelConfirm,
		ModePRList,
		ModePRPicker,
		ModeMilestoneList,
		ModeAgentList,
		ModeGitPanel,
	}
	wantNames := map[Mode]string{
		ModeCloseConfirm:  "close_confirm",
		ModeLabelConfirm:  "label_confirm",
		ModePRList:        "pr_list",
		ModePRPicker:      "pr_picker",
		ModeMilestoneList: "milestone_list",
		ModeAgentList:     "agent_list",
		ModeGitPanel:      "git_panel",
	}
	for _, m := range want {
		if string(m) != wantNames[m] {
			t.Errorf("mode constant = %q, want %q", m, wantNames[m])
		}
	}
}

// TestConsumesPrintableRunes_TextInputModes pins the exact five-mode set
// named by #510's acceptance criteria: create, config, search, comment,
// delete each swallow every printable rune as literal text input.
func TestConsumesPrintableRunes_TextInputModes(t *testing.T) {
	want := []Mode{ModeCreate, ModeConfig, ModeSearch, ModeComment, ModeDelete}
	for _, m := range want {
		if !m.ConsumesPrintableRunes() {
			t.Errorf("Mode(%q).ConsumesPrintableRunes() = false, want true", m)
		}
	}
}

// TestConsumesPrintableRunes_OtherModes_False guards against the set
// silently growing to include a non-text-input mode (which would wrongly
// reject a bare printable-rune binding there too).
func TestConsumesPrintableRunes_OtherModes_False(t *testing.T) {
	textInput := map[Mode]bool{
		ModeCreate:  true,
		ModeConfig:  true,
		ModeSearch:  true,
		ModeComment: true,
		ModeDelete:  true,
	}
	for _, m := range Modes() {
		if textInput[m] {
			continue
		}
		if m.ConsumesPrintableRunes() {
			t.Errorf("Mode(%q).ConsumesPrintableRunes() = true, want false", m)
		}
	}
}

// TestDispatchesKeySequences_SequenceCapableModes pins the ticket's
// Decision (#578): multi-key sequences remain supported only in
// ModeNormal, ModeDetail, and ModeColumns (the pending-sequence dispatch
// seam is handlePendingSeqKey, keymap_dispatch.go; ModeColumns overlays
// onto ModeNormal/ModeDetail rather than dispatching on its own -- see
// keymap.go's Resolve).
func TestDispatchesKeySequences_SequenceCapableModes(t *testing.T) {
	want := []Mode{ModeNormal, ModeDetail, ModeColumns}
	for _, m := range want {
		if !m.DispatchesKeySequences() {
			t.Errorf("Mode(%q).DispatchesKeySequences() = false, want true", m)
		}
	}
}

// TestDispatchesKeySequences_OtherModes_False guards against the set
// silently growing to include a mode whose dispatch seam has no
// pending-sequence machinery -- panelBinding (keymap_panels.go),
// textBinding (keymap_text.go), and every modal's single-key Lookup all
// resolve a single key by exact match only and discard a multi-key
// OutcomePending result.
func TestDispatchesKeySequences_OtherModes_False(t *testing.T) {
	sequenceCapable := map[Mode]bool{
		ModeNormal:  true,
		ModeDetail:  true,
		ModeColumns: true,
	}
	for _, m := range Modes() {
		if sequenceCapable[m] {
			continue
		}
		if m.DispatchesKeySequences() {
			t.Errorf("Mode(%q).DispatchesKeySequences() = true, want false", m)
		}
	}
}

// TestDefaultTables_NoSequenceKeyOutsideSequenceCapableModes is the #578
// drift guard: only a DispatchesKeySequences() mode's default table may
// bind a multi-key sequence -- today that's just CommandNavReference
// (default "g r") and CommandNavAgent (default "g a") in
// normalDefaults/detailDefaults (defaults_board.go). If a future default
// table for a non-sequence-capable mode ever bound a sequence, #578's
// config-load validator would reject it for every user who never touched
// that key, making the shipped default itself the offending config. Uses
// ParseSequence to count elements (never strings.Contains(key, " ")),
// mirroring internal/config's own validator requirement.
func TestDefaultTables_NoSequenceKeyOutsideSequenceCapableModes(t *testing.T) {
	defaults := Defaults()
	for mode, table := range defaults.Modes {
		if mode.DispatchesKeySequences() {
			continue
		}
		for key := range table {
			seq, err := ParseSequence(key)
			if err != nil {
				t.Fatalf("Defaults().Modes[%q] has an unparsable key %q: %v", mode, key, err)
			}
			if len(seq) > 1 {
				t.Errorf("Defaults().Modes[%q] binds multi-key sequence %q, but mode %q does not DispatchesKeySequences()", mode, key, mode)
			}
		}
	}
}

// TestModeDetail_IsDistinctFromNormal pins that "detail" is its own
// resolvable surface (the detail-focused branch of handleNormalModeKey),
// separate from "normal" -- both are overlay targets per the plan's
// "Column overlay" design decision.
func TestModeDetail_IsDistinctFromNormal(t *testing.T) {
	if ModeDetail == ModeNormal {
		t.Fatal("ModeDetail must not equal ModeNormal")
	}
	if string(ModeDetail) != "detail" {
		t.Errorf("ModeDetail = %q, want %q", ModeDetail, "detail")
	}
	if string(ModeNormal) != "normal" {
		t.Errorf("ModeNormal = %q, want %q", ModeNormal, "normal")
	}
}
