package config

import (
	"strings"
	"testing"

	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// --- #577 regression pin: app.quit is not walled off by any mode-capability
// check (#589) ---
//
// Neither test below is a red-then-green TDD test: #589 (this ticket)
// intentionally adds NO mode-capability validation wall to internal/config --
// today, keymaps.<mode>.<key>: app.quit already loads successfully in every
// mode (and in keymaps.columns.<name>) simply because no such wall exists
// yet, so every case passes immediately on first write. Their purpose is
// forward-looking: #577 (the parent ticket, not yet implemented) will add a
// per-mode capability index that DOES reject unrecognized command ids per
// mode, and that index must consult keymap.IsUniversalCommand for app.quit's
// allowed-mode set (see internal/keymap/capability.go's doc comment) rather
// than deriving it from keymap.Defaults() -- Defaults() only reports the
// four modes that bind app.quit by default (normal, detail, help, error),
// which would wrongly re-introduce the exact validation wall #589 exists to
// remove (including a git_panel regression -- see #589's ticket body). This
// file pins today's un-walled acceptance and today's still-active printable-
// rune rejection so a future #577 implementation that gets either boundary
// wrong fails immediately here instead of shipping silently.

// TestLoad_AppQuit_AcceptedInEveryModeAndColumns asserts
// keymaps.<mode>.ctrl+q: app.quit loads cleanly for every mode
// keymap.Modes() reports (all 19), plus keymaps.columns.<name>.
func TestLoad_AppQuit_AcceptedInEveryModeAndColumns(t *testing.T) {
	for _, mode := range keymap.Modes() {
		t.Run(string(mode), func(t *testing.T) {
			yamlContent := `provider: github
keymaps:
  ` + string(mode) + `:
    ctrl+q: app.quit
`
			if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
				t.Errorf("Load() returned unexpected error for keymaps.%s.ctrl+q: app.quit: %v", mode, err)
			}
		})
	}

	t.Run("columns", func(t *testing.T) {
		yamlContent := `provider: github
keymaps:
  columns:
    Doing:
      ctrl+q: app.quit
`
		if _, err := loadConfigFromStrings(t, yamlContent, ""); err != nil {
			t.Errorf("Load() returned unexpected error for keymaps.columns.Doing.ctrl+q: app.quit: %v", err)
		}
	})
}

// TestLoad_AppQuit_BarePrintableRune_StillRejectedInTextInputModes pins the
// other half of the #577 boundary: validatePrintableRuneBindings
// (keymap_semantic_validate.go) must keep rejecting a BARE printable-rune key
// bound to app.quit in the five ConsumesPrintableRunes() modes, exactly as it
// already does for every other command id (#526) -- app.quit's universal
// status is not an exemption from this mode-swallows-every-rune rule.
func TestLoad_AppQuit_BarePrintableRune_StillRejectedInTextInputModes(t *testing.T) {
	for _, mode := range []string{"create", "config", "search", "comment", "delete"} {
		t.Run(mode, func(t *testing.T) {
			yamlContent := `provider: github
keymaps:
  ` + mode + `:
    q: app.quit
`
			_, err := loadConfigFromStrings(t, yamlContent, "")
			if err == nil {
				t.Fatalf("Load() returned nil error, want error for a bare printable-rune key bound to app.quit in mode %q", mode)
			}
			if !strings.Contains(err.Error(), "printable rune") {
				t.Errorf("error = %q, want it to explain that this mode consumes every printable rune", err.Error())
			}
		})
	}
}
