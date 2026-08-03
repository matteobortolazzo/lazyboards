package keymap

// Command ids for the seven confirm/text-input surfaces #538 catalogues:
// close_confirm, label_confirm, delete, create, config, search and comment
// -- the gap left when #508's PR 2/2 was never delivered. Each id is
// prefixed by its own Mode constant string verbatim (close_confirm.confirm,
// create.submit, ...), matching the convention modalCommands/systemCommands
// established. See defaults_text.go for the matching default Table entries.
//
// Two desc choices are deliberate rather than mechanical:
//   - delete.submit reads "Continue / Confirm": one id cannot carry the delete
//     flow's two per-step status-bar wordings ("Continue" on the comment step,
//     "Confirm" on the retype step), which the delete handler renders per step.
//   - config.provider_prev/.provider_next read "Cycle provider", deliberately
//     longer than create.assignee_prev/.assignee_next's "Cycle" -- today's UI
//     is itself inconsistent here and the catalogue preserves that.
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
// confirm/text-input modes. catalog.go merges this into the package-level
// catalog.
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
