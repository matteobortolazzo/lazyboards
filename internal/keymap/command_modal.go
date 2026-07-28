package keymap

// Command ids for the six navigable modal surfaces cataloged by #508 PR 1:
// the global PR list, Milestones, agents list, filter picker, assignee
// picker and PR picker. Each id is prefixed by its own Mode constant string
// verbatim (pr_list.close, filter.select, ...) -- see defaults_modal.go for
// the matching default Table entries and modalCommands (below) for the
// Command entries (desc strings) these ids resolve against.
const (
	CommandPRListClose CommandID = "pr_list.close"
	CommandPRListOpen  CommandID = "pr_list.open"
	CommandPRListNext  CommandID = "pr_list.next"
	CommandPRListPrev  CommandID = "pr_list.prev"

	CommandMilestoneListClose  CommandID = "milestone_list.close"
	CommandMilestoneListFilter CommandID = "milestone_list.filter"
	CommandMilestoneListNext   CommandID = "milestone_list.next"
	CommandMilestoneListPrev   CommandID = "milestone_list.prev"
	CommandMilestoneListOpen   CommandID = "milestone_list.open"

	CommandAgentListClose      CommandID = "agent_list.close"
	CommandAgentListGoToWindow CommandID = "agent_list.go_to_window"
	CommandAgentListNext       CommandID = "agent_list.next"
	CommandAgentListPrev       CommandID = "agent_list.prev"

	CommandFilterClose  CommandID = "filter.close"
	CommandFilterSelect CommandID = "filter.select"
	CommandFilterNext   CommandID = "filter.next"
	CommandFilterPrev   CommandID = "filter.prev"

	CommandAssignClose  CommandID = "assign.close"
	CommandAssignToggle CommandID = "assign.toggle"
	CommandAssignNext   CommandID = "assign.next"
	CommandAssignPrev   CommandID = "assign.prev"

	CommandPRPickerClose  CommandID = "pr_picker.close"
	CommandPRPickerPrev   CommandID = "pr_picker.prev"
	CommandPRPickerNext   CommandID = "pr_picker.next"
	CommandPRPickerSelect CommandID = "pr_picker.select"
)

// modalCommands is the Command catalogue entries (id + desc) for the six
// navigable modal surfaces, sourced from their *Hints vars (model.go:177
// prPickerHints, :205 filterModeHints, :611 assignModeHints, :669
// prListModeHints, :721 milestoneListModeHints, :760 agentListModeHints) so
// hint wording matches today's status bar exactly. catalog.go merges this
// into the package-level catalog.
var modalCommands = []Command{
	{CommandPRListClose, "Cancel"},
	{CommandPRListOpen, "Open"},
	{CommandPRListNext, "Navigate"},
	{CommandPRListPrev, "Navigate"},

	{CommandMilestoneListClose, "Cancel"},
	{CommandMilestoneListFilter, "Filter board"},
	{CommandMilestoneListNext, "Navigate"},
	{CommandMilestoneListPrev, "Navigate"},
	{CommandMilestoneListOpen, "Open in browser"},

	{CommandAgentListClose, "Cancel"},
	{CommandAgentListGoToWindow, "Go to window"},
	{CommandAgentListNext, "Navigate"},
	{CommandAgentListPrev, "Navigate"},

	{CommandFilterClose, "Cancel"},
	{CommandFilterSelect, "Select"},
	{CommandFilterNext, "Navigate"},
	{CommandFilterPrev, "Navigate"},

	{CommandAssignClose, "Cancel"},
	{CommandAssignToggle, "Toggle"},
	{CommandAssignNext, "Navigate"},
	{CommandAssignPrev, "Navigate"},

	{CommandPRPickerClose, "Cancel"},
	{CommandPRPickerPrev, "Cycle"},
	{CommandPRPickerNext, "Cycle"},
	{CommandPRPickerSelect, "Select"},
}
