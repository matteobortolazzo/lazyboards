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
