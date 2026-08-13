package keymap

import "testing"

// textBindingCase pins one (mode, key) -> resolved CommandID default for the
// seven confirm/text-input modes #538 catalogues: close_confirm,
// label_confirm, delete, create, config, search, comment. Transcribed
// directly from the current handler switch statements
// (handleCloseConfirmModeKey, handleLabelConfirmModeKey, handleDeleteModeKey,
// handleCreateModeKey, handleConfigModeKey, handleSearchModeKey,
// handleCommentModeKey in mode_handlers.go) -- never from defaults_text.go,
// so a bug in the catalogue/defaults data can't also corrupt the
// expectation, mirroring modalBindingCase (catalog_pr1_bindings_test.go).
// Every binding in these seven modes resolves to a built-in command --
// none of them dispatches an inline Action -- so unlike modalBindingCase
// there is no wantKind/wantAction field.
type textBindingCase struct {
	name        string
	mode        Mode
	key         string
	wantCommand CommandID
}

var textBindingCases = []textBindingCase{
	// close_confirm (handleCloseConfirmModeKey, mode_handlers.go)
	{"close_confirm/y", ModeCloseConfirm, "y", "close_confirm.confirm"},
	{"close_confirm/n", ModeCloseConfirm, "n", "close_confirm.cancel"},
	{"close_confirm/esc", ModeCloseConfirm, "esc", "close_confirm.cancel"},

	// label_confirm (handleLabelConfirmModeKey, mode_handlers.go)
	{"label_confirm/y", ModeLabelConfirm, "y", "label_confirm.create"},
	{"label_confirm/n", ModeLabelConfirm, "n", "label_confirm.cancel"},
	{"label_confirm/esc", ModeLabelConfirm, "esc", "label_confirm.cancel"},

	// delete (handleDeleteModeKey, mode_handlers.go); delete stays one
	// Mode -- enter resolves to a single id (delete.submit) regardless of
	// which step (comment/confirm) the handler is in; step branching stays
	// handler-side (docs/view-state-consistency.md).
	{"delete/enter", ModeDelete, "enter", "delete.submit"},
	{"delete/esc", ModeDelete, "esc", "delete.cancel"},

	// create (handleCreateModeKey, mode_handlers.go)
	{"create/enter", ModeCreate, "enter", "create.submit"},
	{"create/esc", ModeCreate, "esc", "create.cancel"},
	{"create/tab", ModeCreate, "tab", "create.next_field"},
	{"create/left", ModeCreate, "left", "create.assignee_prev"},
	{"create/right", ModeCreate, "right", "create.assignee_next"},

	// config (handleConfigModeKey, mode_handlers.go)
	{"config/enter", ModeConfig, "enter", "config.save"},
	{"config/esc", ModeConfig, "esc", "config.cancel"},
	{"config/tab", ModeConfig, "tab", "config.next_field"},
	{"config/left", ModeConfig, "left", "config.provider_prev"},
	{"config/right", ModeConfig, "right", "config.provider_next"},

	// search (handleSearchModeKey, mode_handlers.go)
	{"search/enter", ModeSearch, "enter", "search.apply"},
	{"search/esc", ModeSearch, "esc", "search.cancel"},
	{"search/down", ModeSearch, "down", "search.next_result"},
	{"search/ctrl+n", ModeSearch, "ctrl+n", "search.next_result"},
	{"search/up", ModeSearch, "up", "search.prev_result"},
	{"search/ctrl+p", ModeSearch, "ctrl+p", "search.prev_result"},
	{"search/tab", ModeSearch, "tab", "search.next_column"},
	{"search/shift+tab", ModeSearch, "shift+tab", "search.prev_column"},

	// comment (handleCommentModeKey, mode_handlers.go)
	{"comment/enter", ModeComment, "enter", "comment.submit"},
	{"comment/esc", ModeComment, "esc", "comment.cancel"},
}

// textBindingCommandIDs returns every distinct CommandID textBindingCases
// expects a command binding to resolve to, for catalog_text_test.go's
// desc-presence check (TestCatalogue_TextCommandIDsHaveDesc), mirroring
// modalBindingCommandIDs (catalog_pr1_bindings_test.go).
func textBindingCommandIDs() []CommandID {
	seen := make(map[CommandID]bool)
	var out []CommandID
	for _, tc := range textBindingCases {
		if !seen[tc.wantCommand] {
			seen[tc.wantCommand] = true
			out = append(out, tc.wantCommand)
		}
	}
	return out
}

// TestDefaults_TextBindings asserts one test case per key currently handled
// by the seven confirm/text-input handlers: resolving Defaults() against an
// empty user layer and looking the key up against its mode must produce the
// exact expected command id, mirroring TestDefaults_ModalBindings
// (catalog_pr1_bindings_test.go).
func TestDefaults_TextBindings(t *testing.T) {
	if len(textBindingCases) != 28 {
		t.Fatalf("textBindingCases has %d entries, want exactly 28 (one per current text-mode binding)", len(textBindingCases))
	}

	km := resolveOrFatal(t, Defaults(), Tables{})
	for _, tc := range textBindingCases {
		t.Run(tc.name, func(t *testing.T) {
			result := km.Lookup(tc.mode, "", Sequence{Key(tc.key)})
			if result.Outcome != OutcomeMatch {
				t.Fatalf("Lookup(%q, \"\", %q) outcome = %v, want OutcomeMatch", tc.mode, tc.key, result.Outcome)
			}
			if result.Binding.Kind != BindingCommand {
				t.Fatalf("Lookup(%q, \"\", %q) binding kind = %v, want BindingCommand", tc.mode, tc.key, result.Binding.Kind)
			}
			if result.Binding.Command != tc.wantCommand {
				t.Errorf("Lookup(%q, \"\", %q) binding = %+v, want CommandBinding(%q)", tc.mode, tc.key, result.Binding, tc.wantCommand)
			}
		})
	}
}
