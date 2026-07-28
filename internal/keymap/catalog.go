package keymap

// catalog is every catalogued Command for the normal-mode and detail-panel
// surfaces (#507). #508 extends this same slice for the remaining modes --
// keep this file to the two data literals (catalog, defaultModeTables) and
// three accessors (Commands, FindCommand, Defaults) -- #508 adds data, not
// more accessors.
var catalog = []Command{
	{CommandQuit, "Quit"},
	{CommandHelp, "Help"},
	{CommandConfig, "Configuration"},
	{CommandCardNew, "New card"},
	{CommandCardEdit, "Edit card"},
	{CommandCardOpenTicket, "Open ticket"},
	{CommandCardOpenPR, "Open PR"},
	{CommandCardClose, "Close card"},
	{CommandCardDelete, "Delete card"},
	{CommandCardAssign, "Assign"},
	{CommandBoardRefresh, "Refresh"},
	{CommandBoardSearch, "Search"},
	{CommandBoardFilter, "Filter (toggle)"},
	{CommandBoardSortOrder, "Sort order"},
	{CommandViewPRList, "Open PRs"},
	{CommandViewMilestoneList, "Milestones (repo-wide)"},
	{CommandViewAgentList, "Agents (cenci)"},
	{CommandViewGitPanel, "Git menu"},
	{CommandViewDispatch, "Dispatch (cenci)"},
	{CommandNavReference, "Go to reference"},
	{CommandNavAgent, "Go to agent (cenci)"},
	{CommandNavDetailFocus, "Detail panel"},
	{CommandNavCursorDown, "Navigate cards"},
	{CommandNavCursorUp, "Navigate cards"},
	{CommandNavColumnNext, "Switch columns"},
	{CommandNavColumnPrev, "Switch columns"},
	{CommandNavColumn1, "Jump to column 1"},
	{CommandNavColumn2, "Jump to column 2"},
	{CommandNavColumn3, "Jump to column 3"},
	{CommandNavColumn4, "Jump to column 4"},
	{CommandNavColumn5, "Jump to column 5"},
	{CommandNavColumn6, "Jump to column 6"},
	{CommandNavColumn7, "Jump to column 7"},
	{CommandNavColumn8, "Jump to column 8"},
	{CommandNavColumn9, "Jump to column 9"},
	{CommandDetailBlur, "Back to card list"},
	{CommandDetailScrollDown, "Scroll body"},
	{CommandDetailScrollUp, "Scroll body"},
}

// defaultModeTables backs Defaults(): the normal/detail default Table
// literals from defaults_board.go, keyed by the Mode they bind. #508 adds
// entries for every remaining mode.
var defaultModeTables = map[Mode]Table{
	ModeNormal: normalDefaults,
	ModeDetail: detailDefaults,
}

// Commands returns every catalogued Command, in declaration order. It
// returns a defensive copy, mirroring Modes() (mode.go).
func Commands() []Command {
	out := make([]Command, len(catalog))
	copy(out, catalog)
	return out
}

// FindCommand looks up a catalogued Command by id.
func FindCommand(id CommandID) (Command, bool) {
	for _, c := range catalog {
		if c.ID == id {
			return c, true
		}
	}
	return Command{}, false
}

// Defaults returns the built-in default Tables: the normal-mode and
// detail-panel tables for #507. #508 fills in every remaining mode. It
// returns a defensive copy, mirroring Commands() (above), Modes() (mode.go)
// and Entries() (keymap.go) -- callers cannot mutate the package-level
// defaultModeTables (or its nested per-mode Tables) through the result.
func Defaults() Tables {
	modes := make(map[Mode]Table, len(defaultModeTables))
	for mode, table := range defaultModeTables {
		copied := make(Table, len(table))
		for seq, binding := range table {
			copied[seq] = binding
		}
		modes[mode] = copied
	}
	return Tables{Modes: modes}
}
