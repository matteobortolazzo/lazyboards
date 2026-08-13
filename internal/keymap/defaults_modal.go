package keymap

// prListDefaults is the default ModePRList table, transcribed from
// handlePRListModeKey (mode_handlers.go).
var prListDefaults = Table{
	"esc":   CommandBinding(CommandPRListClose),
	"enter": CommandBinding(CommandPRListOpen),
	"j":     CommandBinding(CommandPRListNext),
	"down":  CommandBinding(CommandPRListNext),
	"k":     CommandBinding(CommandPRListPrev),
	"up":    CommandBinding(CommandPRListPrev),
}

// milestoneListDefaults is the default ModeMilestoneList table, transcribed
// from handleMilestoneListModeKey (mode_handlers.go).
var milestoneListDefaults = Table{
	"esc":   CommandBinding(CommandMilestoneListClose),
	"enter": CommandBinding(CommandMilestoneListFilter),
	"j":     CommandBinding(CommandMilestoneListNext),
	"down":  CommandBinding(CommandMilestoneListNext),
	"k":     CommandBinding(CommandMilestoneListPrev),
	"up":    CommandBinding(CommandMilestoneListPrev),
	"o":     CommandBinding(CommandMilestoneListOpen),
}

// agentListDefaults is the default ModeAgentList table, transcribed from
// handleAgentListModeKey (mode_handlers.go).
var agentListDefaults = Table{
	"esc":   CommandBinding(CommandAgentListClose),
	"enter": CommandBinding(CommandAgentListGoToWindow),
	"j":     CommandBinding(CommandAgentListNext),
	"down":  CommandBinding(CommandAgentListNext),
	"k":     CommandBinding(CommandAgentListPrev),
	"up":    CommandBinding(CommandAgentListPrev),
}

// filterDefaults is the default ModeFilter table, transcribed from
// handleFilterModeKey (mode_handlers.go).
var filterDefaults = Table{
	"esc":   CommandBinding(CommandFilterClose),
	"enter": CommandBinding(CommandFilterSelect),
	"j":     CommandBinding(CommandFilterNext),
	"down":  CommandBinding(CommandFilterNext),
	"k":     CommandBinding(CommandFilterPrev),
	"up":    CommandBinding(CommandFilterPrev),
}

// assignDefaults is the default ModeAssign table, transcribed from
// handleAssignModeKey (mode_handlers.go).
var assignDefaults = Table{
	"esc":   CommandBinding(CommandAssignClose),
	"enter": CommandBinding(CommandAssignToggle),
	"j":     CommandBinding(CommandAssignNext),
	"down":  CommandBinding(CommandAssignNext),
	"k":     CommandBinding(CommandAssignPrev),
	"up":    CommandBinding(CommandAssignPrev),
}

// prPickerDefaults is the default ModePRPicker table, transcribed from
// handlePRPickerModeKey (mode_handlers.go).
var prPickerDefaults = Table{
	"esc":   CommandBinding(CommandPRPickerClose),
	"left":  CommandBinding(CommandPRPickerPrev),
	"right": CommandBinding(CommandPRPickerNext),
	"enter": CommandBinding(CommandPRPickerSelect),
}

// modalDefaultTables aggregates the six navigable modal default tables by
// Mode. catalog.go merges this into the package-level defaultModeTables.
var modalDefaultTables = map[Mode]Table{
	ModePRList:        prListDefaults,
	ModeMilestoneList: milestoneListDefaults,
	ModeAgentList:     agentListDefaults,
	ModeFilter:        filterDefaults,
	ModeAssign:        assignDefaults,
	ModePRPicker:      prPickerDefaults,
}
