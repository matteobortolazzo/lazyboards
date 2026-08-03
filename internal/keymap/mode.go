package keymap

import "fmt"

// Mode identifies a resolvable key surface: a distinct handler in the
// running app that dispatches through Lookup. Each constant's doc comment
// names the handler it corresponds to (all in mode_handlers.go unless
// noted).
type Mode string

const (
	// ModeNormal is the board's default key surface, handled by
	// handleNormalModeKey when no card is detail-focused.
	ModeNormal Mode = "normal"

	// ModeDetail is the detail-focused branch of handleNormalModeKey --
	// a distinct overlay target from ModeNormal per the plan's column
	// overlay design (both normal and detail can be overlaid per column).
	ModeDetail Mode = "detail"

	// ModeCreate is handleCreateModeKey, the card-creation form.
	ModeCreate Mode = "create"

	// ModeError is the errorMode key surface, handled by
	// handleErrorModeKey.
	ModeError Mode = "error"

	// ModeConfig is handleConfigModeKey, the first-launch/config form.
	ModeConfig Mode = "config"

	// ModePRPicker is handlePRPickerModeKey, the single/multi-PR picker
	// modal.
	ModePRPicker Mode = "pr_picker"

	// ModeSearch is handleSearchModeKey, the card search input.
	ModeSearch Mode = "search"

	// ModeHelp is handleHelpModeKey, the help modal.
	ModeHelp Mode = "help"

	// ModeLabelConfirm is handleLabelConfirmModeKey, the label-frontmatter
	// confirmation step.
	ModeLabelConfirm Mode = "label_confirm"

	// ModeCloseConfirm is handleCloseConfirmModeKey, the two-step close
	// confirmation flow.
	ModeCloseConfirm Mode = "close_confirm"

	// ModeComment is handleCommentModeKey, the comment-composition input.
	ModeComment Mode = "comment"

	// ModeDelete is handleDeleteModeKey, the two-step delete confirmation
	// flow.
	ModeDelete Mode = "delete"

	// ModeFilter is handleFilterModeKey, the label/assignee filter picker
	// modal.
	ModeFilter Mode = "filter"

	// ModeAssign is handleAssignModeKey, the assignee picker modal.
	ModeAssign Mode = "assign"

	// ModeGitPanel is handleGitPanelKey, the git menu panel.
	ModeGitPanel Mode = "git_panel"

	// ModeDispatch is handleDispatchModeKey, the agent dispatch modal.
	ModeDispatch Mode = "dispatch"

	// ModePRList is handlePRListModeKey, the global PR list modal.
	ModePRList Mode = "pr_list"

	// ModeMilestoneList is handleMilestoneListModeKey, the milestones list
	// modal.
	ModeMilestoneList Mode = "milestone_list"

	// ModeAgentList is handleAgentListModeKey, the agents list modal (all
	// cenci windows).
	ModeAgentList Mode = "agent_list"

	// ModeColumns is not a resolvable key surface -- it is a config
	// namespace ("columns.<name>") that overlays ModeNormal/ModeDetail for
	// a specific column, per Resolve's column-overlay rule. It is excluded
	// from Modes() but still accepted by ParseMode so #509 can route
	// keymaps.columns.<name> through the same name check as every other
	// mode. loadingMode and creatingMode (update.go) swallow every key and
	// get no Mode constant at all -- there is nothing to bind.
	ModeColumns Mode = "columns"
)

// modes lists every resolvable Mode in declaration order. It backs both
// Modes() and ParseMode's resolvable lookup.
var modes = []Mode{
	ModeNormal,
	ModeDetail,
	ModeCreate,
	ModeError,
	ModeConfig,
	ModePRPicker,
	ModeSearch,
	ModeHelp,
	ModeLabelConfirm,
	ModeCloseConfirm,
	ModeComment,
	ModeDelete,
	ModeFilter,
	ModeAssign,
	ModeGitPanel,
	ModeDispatch,
	ModePRList,
	ModeMilestoneList,
	ModeAgentList,
}

// Modes returns every resolvable key surface, in declaration order.
// ModeColumns is deliberately excluded -- it is a config namespace, not a
// surface Lookup callers iterate.
func Modes() []Mode {
	out := make([]Mode, len(modes))
	copy(out, modes)
	return out
}

// resolvableModes is a set built from modes for O(1) membership checks, so
// ParseMode and Resolvable share one lookup instead of each hand-rolling a
// linear scan over modes.
var resolvableModes = func() map[Mode]bool {
	set := make(map[Mode]bool, len(modes))
	for _, m := range modes {
		set[m] = true
	}
	return set
}()

// ParseMode parses a mode name into its Mode constant. It accepts
// "columns" (returning ModeColumns) even though ModeColumns is excluded
// from Modes() and is not itself Resolvable(), so #509 can route
// keymaps.columns.<name> through one name check. Unknown names, and the
// swallowing loadingMode/creatingMode surfaces (which never got a Mode
// constant), return an error naming the offending value.
func ParseMode(s string) (Mode, error) {
	if s == string(ModeColumns) {
		return ModeColumns, nil
	}
	if m := Mode(s); resolvableModes[m] {
		return m, nil
	}
	return "", fmt.Errorf("keymap: unknown mode %q", s)
}

// Resolvable reports whether m is a key surface Lookup can resolve against
// -- true for every constant in Modes(), false for ModeColumns.
func (m Mode) Resolvable() bool {
	return resolvableModes[m]
}

// textInputModes is the set of modes whose handler consumes every printable
// rune keypress as literal text input (see the handlers named in each
// Mode's own doc comment: handleCreateModeKey, handleConfigModeKey,
// handleSearchModeKey, handleCommentModeKey/comment_mode.go, and
// handleDeleteModeKey's confirmation text field). A key binding for a bare
// printable rune in one of these modes can never dispatch -- the input
// widget swallows the keypress before any lookup happens.
var textInputModes = map[Mode]bool{
	ModeCreate:  true,
	ModeConfig:  true,
	ModeSearch:  true,
	ModeComment: true,
	ModeDelete:  true,
}

// ConsumesPrintableRunes reports whether m is a text-input surface (create,
// config, search, comment, delete) that swallows every printable rune as
// literal text, so no bare printable-rune key can ever be bound there.
func (m Mode) ConsumesPrintableRunes() bool {
	return textInputModes[m]
}
