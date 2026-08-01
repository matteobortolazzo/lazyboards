package keymap

// Command ids for the seven confirm/text-input surfaces #538 catalogues:
// close_confirm, label_confirm, delete, create, config, search and comment
// -- the gap left when #508's PR 2/2 was never delivered. Each id is
// prefixed by its own Mode constant string verbatim (close_confirm.confirm,
// create.submit, ...), matching the convention modalCommands/systemCommands
// established. See defaults_text.go for the matching default Table entries.
//
// desc sourcing (status-bar hint wins, helpSections is the fallback only
// where no status-bar hint exists -- same rule #508 PR 1 used):
//   - close_confirm.confirm/.cancel: helpSections "Close Confirm" (view.go,
//     no dedicated *Hints var exists for this mode)
//   - label_confirm.create/.cancel: helpSections "Label Confirm" (view.go,
//     no dedicated *Hints var exists for this mode)
//   - delete.submit: helpSections "Delete" ("Continue / Confirm") -- a
//     single id cannot carry the two per-step status-bar wordings
//     ("Continue" vs "Confirm"); those stay in deleteCommentHints/
//     deleteConfirmHints (model.go) and drive the hint bar directly until
//     the sibling conversion ticket. delete.cancel: deleteCommentHints/
//     deleteConfirmHints agree on "Cancel" for esc.
//   - create.submit/.cancel/.next_field/.assignee_prev/.assignee_next:
//     inline hints literal in viewCreateModal (view.go)
//   - config.save/.cancel/.next_field: inline hints literal in
//     viewConfigModal (view.go)
//   - config.provider_prev/.provider_next: helpSections fallback ("Cycle
//     provider") -- no status-bar hint exists for left/right in
//     viewConfigModal, same fallback #508 PR 1 used for error.retry/
//     dispatch.*. Deliberately longer than create.assignee_prev/next's
//     "Cycle" -- today's UI is itself inconsistent here.
//   - search.apply/.cancel/.next_result/.prev_result: searchModeHints
//     (model.go)
//   - search.next_column/.prev_column: helpSections fallback ("Switch
//     columns (clears search)") -- no status-bar hint exists for tab/
//     shift+tab in search mode.
//   - comment.submit/.cancel: commentModeHints (model.go)
const (
	CommandCloseConfirmConfirm CommandID = "close_confirm.confirm"
	CommandCloseConfirmCancel  CommandID = "close_confirm.cancel"

	CommandLabelConfirmCreate CommandID = "label_confirm.create"
	CommandLabelConfirmCancel CommandID = "label_confirm.cancel"

	CommandDeleteSubmit CommandID = "delete.submit"
	CommandDeleteCancel CommandID = "delete.cancel"

	CommandCreateSubmit       CommandID = "create.submit"
	CommandCreateCancel       CommandID = "create.cancel"
	CommandCreateNextField    CommandID = "create.next_field"
	CommandCreateAssigneePrev CommandID = "create.assignee_prev"
	CommandCreateAssigneeNext CommandID = "create.assignee_next"

	CommandConfigSave         CommandID = "config.save"
	CommandConfigCancel       CommandID = "config.cancel"
	CommandConfigNextField    CommandID = "config.next_field"
	CommandConfigProviderPrev CommandID = "config.provider_prev"
	CommandConfigProviderNext CommandID = "config.provider_next"

	CommandSearchApply      CommandID = "search.apply"
	CommandSearchCancel     CommandID = "search.cancel"
	CommandSearchNextResult CommandID = "search.next_result"
	CommandSearchPrevResult CommandID = "search.prev_result"
	CommandSearchNextColumn CommandID = "search.next_column"
	CommandSearchPrevColumn CommandID = "search.prev_column"

	CommandCommentSubmit CommandID = "comment.submit"
	CommandCommentCancel CommandID = "comment.cancel"
)

// textCommands is the Command catalogue entries (id + desc) for the seven
// confirm/text-input modes, sourced per the doc comment above so hint/help
// wording matches today's UI text exactly. catalog.go merges this into the
// package-level catalog.
var textCommands = []Command{
	{CommandCloseConfirmConfirm, "Confirm close"},
	{CommandCloseConfirmCancel, "Cancel"},

	{CommandLabelConfirmCreate, "Create label, continue"},
	{CommandLabelConfirmCancel, "Cancel edit"},

	{CommandDeleteSubmit, "Continue / Confirm"},
	{CommandDeleteCancel, "Cancel"},

	{CommandCreateSubmit, "Submit"},
	{CommandCreateCancel, "Cancel"},
	{CommandCreateNextField, "Next"},
	{CommandCreateAssigneePrev, "Cycle"},
	{CommandCreateAssigneeNext, "Cycle"},

	{CommandConfigSave, "Save"},
	{CommandConfigCancel, "Cancel"},
	{CommandConfigNextField, "Next"},
	{CommandConfigProviderPrev, "Cycle provider"},
	{CommandConfigProviderNext, "Cycle provider"},

	{CommandSearchApply, "Apply"},
	{CommandSearchCancel, "Clear"},
	{CommandSearchNextResult, "Navigate"},
	{CommandSearchPrevResult, "Navigate"},
	{CommandSearchNextColumn, "Switch columns (clears search)"},
	{CommandSearchPrevColumn, "Switch columns (clears search)"},

	{CommandCommentSubmit, "Submit"},
	{CommandCommentCancel, "Cancel"},
}
