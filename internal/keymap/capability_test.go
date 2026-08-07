package keymap

import "testing"

// TestUniversalCommands_ContainsExactlyCommandQuit pins universalCommands
// (capability.go) to a single-entry set: CommandQuit. #577's forthcoming
// per-mode capability index must derive app.quit's allowed set from this
// predicate rather than hand-writing a mode list -- if a future id is ever
// added here without a corresponding dispatch seam case, this test's exact-
// membership assertion (not just "contains CommandQuit") is what would catch
// a stray addition no integration test could otherwise observe (an unwired
// universal id simply no-ops silently at the seam).
func TestUniversalCommands_ContainsExactlyCommandQuit(t *testing.T) {
	if got := len(universalCommands); got != 1 {
		t.Fatalf("len(universalCommands) = %d, want 1", got)
	}
	if !universalCommands[CommandQuit] {
		t.Errorf("universalCommands does not contain CommandQuit, want it to")
	}
}

func TestIsUniversalCommand_CommandQuit_ReturnsTrue(t *testing.T) {
	if !IsUniversalCommand(CommandQuit) {
		t.Errorf("IsUniversalCommand(CommandQuit) = false, want true")
	}
}

// TestIsUniversalCommand_ModeScopedID_ReturnsFalse uses CommandCardDelete (a
// normal/detail-mode-scoped id, command_board.go) as a representative
// non-universal command: it must not be treated as dispatchable outside its
// own mode's runner.
func TestIsUniversalCommand_ModeScopedID_ReturnsFalse(t *testing.T) {
	if IsUniversalCommand(CommandCardDelete) {
		t.Errorf("IsUniversalCommand(CommandCardDelete) = true, want false (mode-scoped id, not universal)")
	}
}
