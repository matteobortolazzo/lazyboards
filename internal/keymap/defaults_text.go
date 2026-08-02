package keymap

// closeConfirmDefaults is the default ModeCloseConfirm table, transcribed
// from handleCloseConfirmModeKey (mode_handlers.go).
var closeConfirmDefaults = Table{
	"y":   CommandBinding(CommandCloseConfirmConfirm),
	"n":   CommandBinding(CommandCloseConfirmCancel),
	"esc": CommandBinding(CommandCloseConfirmCancel),
}

// labelConfirmDefaults is the default ModeLabelConfirm table, transcribed
// from handleLabelConfirmModeKey (mode_handlers.go).
var labelConfirmDefaults = Table{
	"y":   CommandBinding(CommandLabelConfirmCreate),
	"n":   CommandBinding(CommandLabelConfirmCancel),
	"esc": CommandBinding(CommandLabelConfirmCancel),
}

// deleteDefaults is the default ModeDelete table, transcribed from
// handleDeleteModeKey (mode_handlers.go). delete stays one Mode: enter
// resolves to the single id delete.submit regardless of which step
// (comment/confirm) the handler is currently in -- step branching, and the
// per-step status-bar wording ("Continue" vs "Confirm"), stay handler-side
// until the sibling conversion ticket.
var deleteDefaults = Table{
	"enter": CommandBinding(CommandDeleteSubmit),
	"esc":   CommandBinding(CommandDeleteCancel),
}

// createDefaults is the default ModeCreate table, transcribed from
// handleCreateModeKey (mode_handlers.go). left/right are bound
// unconditionally here even though the handler only acts on them when the
// assignee field is focused (mode_handlers.go) -- the focus gate
// stays handler-side for the sibling conversion ticket, same pattern
// applied to config.provider_prev/next below.
var createDefaults = Table{
	"enter": CommandBinding(CommandCreateSubmit),
	"esc":   CommandBinding(CommandCreateCancel),
	"tab":   CommandBinding(CommandCreateNextField),
	"left":  CommandBinding(CommandCreateAssigneePrev),
	"right": CommandBinding(CommandCreateAssigneeNext),
}

// configDefaults is the default ModeConfig table, transcribed from
// handleConfigModeKey (mode_handlers.go). left/right are bound
// unconditionally here even though the handler only acts on them when the
// provider field is focused (mode_handlers.go) -- the focus gate
// stays handler-side for the sibling conversion ticket.
var configDefaults = Table{
	"enter": CommandBinding(CommandConfigSave),
	"esc":   CommandBinding(CommandConfigCancel),
	"tab":   CommandBinding(CommandConfigNextField),
	"left":  CommandBinding(CommandConfigProviderPrev),
	"right": CommandBinding(CommandConfigProviderNext),
}

// searchDefaults is the default ModeSearch table, transcribed from
// handleSearchModeKey (mode_handlers.go).
var searchDefaults = Table{
	"enter":     CommandBinding(CommandSearchApply),
	"esc":       CommandBinding(CommandSearchCancel),
	"down":      CommandBinding(CommandSearchNextResult),
	"ctrl+n":    CommandBinding(CommandSearchNextResult),
	"up":        CommandBinding(CommandSearchPrevResult),
	"ctrl+p":    CommandBinding(CommandSearchPrevResult),
	"tab":       CommandBinding(CommandSearchNextColumn),
	"shift+tab": CommandBinding(CommandSearchPrevColumn),
}

// commentDefaults is the default ModeComment table, transcribed from
// handleCommentModeKey (mode_handlers.go).
var commentDefaults = Table{
	"enter": CommandBinding(CommandCommentSubmit),
	"esc":   CommandBinding(CommandCommentCancel),
}

// textDefaultTables aggregates the seven confirm/text-input default tables
// by Mode. catalog.go merges this into the package-level defaultModeTables.
var textDefaultTables = map[Mode]Table{
	ModeCloseConfirm: closeConfirmDefaults,
	ModeLabelConfirm: labelConfirmDefaults,
	ModeDelete:       deleteDefaults,
	ModeCreate:       createDefaults,
	ModeConfig:       configDefaults,
	ModeSearch:       searchDefaults,
	ModeComment:      commentDefaults,
}
