package keymap

import "testing"

// modalBindingCase pins one (mode, key) -> resolved Binding default, per the
// #508 PR 1 Command Set table (navigable modals, git panel, dispatch, help,
// error). Transcribed directly from the current handler switch statements
// (mode_handlers.go:459, :484, :526, :561, :709, :776, :830, :913, :944;
// update.go:326) -- never from a production keymap data file, so a bug in
// the catalogue/defaults data can't also corrupt the expectation, mirroring
// boardDefaultCase (defaults_board_test.go).
//
// wantKind distinguishes a resolved built-in command (BindingCommand) from
// an inline action (BindingAction, used only by the git panel's six
// lazygit-style action keys, seeded from config.DefaultGitActions() --
// internal/keymap cannot import internal/config, so the action literals
// below are duplicated by hand and held together by the root
// git_keymap_defaults_test.go drift guard).
type modalBindingCase struct {
	name        string
	mode        Mode
	key         string
	wantKind    BindingKind
	wantCommand CommandID
	wantAction  Action
}

var modalBindingCases = []modalBindingCase{
	// pr_list (handlePRListModeKey, mode_handlers.go:776)
	{"pr_list/esc", ModePRList, "esc", BindingCommand, "pr_list.close", Action{}},
	{"pr_list/enter", ModePRList, "enter", BindingCommand, "pr_list.open", Action{}},
	{"pr_list/j", ModePRList, "j", BindingCommand, "pr_list.next", Action{}},
	{"pr_list/down", ModePRList, "down", BindingCommand, "pr_list.next", Action{}},
	{"pr_list/k", ModePRList, "k", BindingCommand, "pr_list.prev", Action{}},
	{"pr_list/up", ModePRList, "up", BindingCommand, "pr_list.prev", Action{}},

	// milestone_list (handleMilestoneListModeKey, mode_handlers.go:830)
	{"milestone_list/esc", ModeMilestoneList, "esc", BindingCommand, "milestone_list.close", Action{}},
	{"milestone_list/enter", ModeMilestoneList, "enter", BindingCommand, "milestone_list.filter", Action{}},
	{"milestone_list/j", ModeMilestoneList, "j", BindingCommand, "milestone_list.next", Action{}},
	{"milestone_list/down", ModeMilestoneList, "down", BindingCommand, "milestone_list.next", Action{}},
	{"milestone_list/k", ModeMilestoneList, "k", BindingCommand, "milestone_list.prev", Action{}},
	{"milestone_list/up", ModeMilestoneList, "up", BindingCommand, "milestone_list.prev", Action{}},
	{"milestone_list/o", ModeMilestoneList, "o", BindingCommand, "milestone_list.open", Action{}},

	// agent_list (handleAgentListModeKey, mode_handlers.go:913)
	{"agent_list/esc", ModeAgentList, "esc", BindingCommand, "agent_list.close", Action{}},
	{"agent_list/enter", ModeAgentList, "enter", BindingCommand, "agent_list.go_to_window", Action{}},
	{"agent_list/j", ModeAgentList, "j", BindingCommand, "agent_list.next", Action{}},
	{"agent_list/down", ModeAgentList, "down", BindingCommand, "agent_list.next", Action{}},
	{"agent_list/k", ModeAgentList, "k", BindingCommand, "agent_list.prev", Action{}},
	{"agent_list/up", ModeAgentList, "up", BindingCommand, "agent_list.prev", Action{}},

	// filter (handleFilterModeKey, mode_handlers.go:459)
	{"filter/esc", ModeFilter, "esc", BindingCommand, "filter.close", Action{}},
	{"filter/enter", ModeFilter, "enter", BindingCommand, "filter.select", Action{}},
	{"filter/j", ModeFilter, "j", BindingCommand, "filter.next", Action{}},
	{"filter/down", ModeFilter, "down", BindingCommand, "filter.next", Action{}},
	{"filter/k", ModeFilter, "k", BindingCommand, "filter.prev", Action{}},
	{"filter/up", ModeFilter, "up", BindingCommand, "filter.prev", Action{}},

	// assign (handleAssignModeKey, mode_handlers.go:484)
	{"assign/esc", ModeAssign, "esc", BindingCommand, "assign.close", Action{}},
	{"assign/enter", ModeAssign, "enter", BindingCommand, "assign.toggle", Action{}},
	{"assign/j", ModeAssign, "j", BindingCommand, "assign.next", Action{}},
	{"assign/down", ModeAssign, "down", BindingCommand, "assign.next", Action{}},
	{"assign/k", ModeAssign, "k", BindingCommand, "assign.prev", Action{}},
	{"assign/up", ModeAssign, "up", BindingCommand, "assign.prev", Action{}},

	// pr_picker (handlePRPickerModeKey, mode_handlers.go:709)
	{"pr_picker/esc", ModePRPicker, "esc", BindingCommand, "pr_picker.close", Action{}},
	{"pr_picker/left", ModePRPicker, "left", BindingCommand, "pr_picker.prev", Action{}},
	{"pr_picker/right", ModePRPicker, "right", BindingCommand, "pr_picker.next", Action{}},
	{"pr_picker/enter", ModePRPicker, "enter", BindingCommand, "pr_picker.select", Action{}},

	// git_panel navigation (handleGitPanelKey, mode_handlers.go:526)
	{"git_panel/esc", ModeGitPanel, "esc", BindingCommand, "git_panel.close", Action{}},
	{"git_panel/enter", ModeGitPanel, "enter", BindingCommand, "git_panel.run", Action{}},
	{"git_panel/j", ModeGitPanel, "j", BindingCommand, "git_panel.next", Action{}},
	{"git_panel/down", ModeGitPanel, "down", BindingCommand, "git_panel.next", Action{}},
	{"git_panel/k", ModeGitPanel, "k", BindingCommand, "git_panel.prev", Action{}},
	{"git_panel/up", ModeGitPanel, "up", BindingCommand, "git_panel.prev", Action{}},

	// git_panel action keys, seeded from config.DefaultGitActions()
	// (internal/config/config.go:161); duplicated here per binding.go's
	// no-import rule and cross-checked by git_keymap_defaults_test.go.
	{"git_panel/P", ModeGitPanel, "P", BindingAction, "", Action{Name: "Push", Type: "shell", Command: "git push", Scope: "board"}},
	{"git_panel/p", ModeGitPanel, "p", BindingAction, "", Action{Name: "Pull (rebase)", Type: "shell", Command: "git pull --rebase", Scope: "board"}},
	{"git_panel/f", ModeGitPanel, "f", BindingAction, "", Action{Name: "Fetch", Type: "shell", Command: "git fetch", Scope: "board"}},
	{"git_panel/m", ModeGitPanel, "m", BindingAction, "", Action{Name: "Mergetool", Type: "shell", Command: "git mergetool", Scope: "board"}},
	{"git_panel/s", ModeGitPanel, "s", BindingAction, "", Action{Name: "Stash push", Type: "shell", Command: "git stash push", Scope: "board"}},
	{"git_panel/S", ModeGitPanel, "S", BindingAction, "", Action{Name: "Stash pop", Type: "shell", Command: "git stash pop", Scope: "board"}},

	// dispatch (handleDispatchModeKey, mode_handlers.go:561); y/n resolve
	// unconditionally at the keymap layer -- the confirmingLoop state gate
	// stays in the handler (docs/view-state-consistency.md), not the table.
	{"dispatch/esc", ModeDispatch, "esc", BindingCommand, "dispatch.close", Action{}},
	{"dispatch/enter", ModeDispatch, "enter", BindingCommand, "dispatch.toggle_enroll", Action{}},
	{"dispatch/o", ModeDispatch, "o", BindingCommand, "dispatch.once", Action{}},
	{"dispatch/l", ModeDispatch, "l", BindingCommand, "dispatch.toggle_loop", Action{}},
	{"dispatch/y", ModeDispatch, "y", BindingCommand, "dispatch.confirm_loop", Action{}},
	{"dispatch/n", ModeDispatch, "n", BindingCommand, "dispatch.cancel_loop", Action{}},

	// help (handleHelpModeKey, mode_handlers.go:944); q reuses the
	// already-catalogued app.quit id (allow-listed, Q5).
	{"help/esc", ModeHelp, "esc", BindingCommand, "help.close", Action{}},
	{"help/?", ModeHelp, "?", BindingCommand, "help.close", Action{}},
	{"help/j", ModeHelp, "j", BindingCommand, "help.scroll_down", Action{}},
	{"help/down", ModeHelp, "down", BindingCommand, "help.scroll_down", Action{}},
	{"help/k", ModeHelp, "k", BindingCommand, "help.scroll_up", Action{}},
	{"help/up", ModeHelp, "up", BindingCommand, "help.scroll_up", Action{}},
	{"help/q", ModeHelp, "q", BindingCommand, CommandQuit, Action{}},

	// error (update.go:326-334); q reuses app.quit (allow-listed, Q3/Q5).
	{"error/q", ModeError, "q", BindingCommand, CommandQuit, Action{}},
	{"error/r", ModeError, "r", BindingCommand, "error.retry", Action{}},
}

// modalBindingCommandIDs returns every distinct CommandID modalBindingCases
// expects a command binding to resolve to, for catalog_pr1_test.go's
// desc-presence check.
func modalBindingCommandIDs() []CommandID {
	seen := make(map[CommandID]bool)
	var out []CommandID
	for _, tc := range modalBindingCases {
		if tc.wantKind != BindingCommand {
			continue
		}
		if !seen[tc.wantCommand] {
			seen[tc.wantCommand] = true
			out = append(out, tc.wantCommand)
		}
	}
	return out
}

// TestDefaults_ModalBindings asserts one test case per key currently handled
// by the ten PR-1 modal/panel/help/error handlers: resolving Defaults()
// against an empty user layer and looking the key up against its mode must
// produce the exact expected outcome and Binding.
func TestDefaults_ModalBindings(t *testing.T) {
	if len(modalBindingCases) < 55 {
		t.Fatalf("modalBindingCases has %d entries, want at least 55 (one per current PR-1 binding)", len(modalBindingCases))
	}

	km := resolveOrFatal(t, Defaults(), Tables{})
	for _, tc := range modalBindingCases {
		t.Run(tc.name, func(t *testing.T) {
			result := km.Lookup(tc.mode, "", Sequence{Key(tc.key)})
			if result.Outcome != OutcomeMatch {
				t.Fatalf("Lookup(%q, \"\", %q) outcome = %v, want OutcomeMatch", tc.mode, tc.key, result.Outcome)
			}
			if result.Binding.Kind != tc.wantKind {
				t.Fatalf("Lookup(%q, \"\", %q) binding kind = %v, want %v", tc.mode, tc.key, result.Binding.Kind, tc.wantKind)
			}
			switch tc.wantKind {
			case BindingCommand:
				if result.Binding.Command != tc.wantCommand {
					t.Errorf("Lookup(%q, \"\", %q) binding = %+v, want CommandBinding(%q)", tc.mode, tc.key, result.Binding, tc.wantCommand)
				}
			case BindingAction:
				got := result.Binding.Action
				want := tc.wantAction
				if got.Name != want.Name || got.Type != want.Type || got.Command != want.Command || got.Scope != want.Scope {
					t.Errorf("Lookup(%q, \"\", %q) action = %+v, want Name/Type/Command/Scope matching %+v", tc.mode, tc.key, got, want)
				}
			}
		})
	}
}
