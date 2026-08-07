package keymap

// boardCommands holds the Command catalogue entries for the normal-mode and
// detail-panel surfaces (#507). modalCommands (command_modal.go),
// panelCommands (command_panel.go) and systemCommands (command_system.go)
// hold the #508 PR 1 surfaces (the six navigable modals, the git panel, the
// dispatch modal, help and errorMode); textCommands (command_text.go) holds
// the #538 surfaces (the seven confirm/text-input modes: close_confirm,
// label_confirm, delete, create, config, search, comment). init (below)
// merges every group into catalog -- a new mode group only needs a var here
// (or its own file) plus one line in init, not a new accessor alongside
// Commands/FindCommand/Defaults.
var boardCommands = []Command{
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

var catalog []Command

// defaultModeTables backs Defaults(): the normal/detail default Table
// literals from defaults_board.go (#507), merged with modalDefaultTables
// (defaults_modal.go), panelDefaultTables (defaults_panel.go) and
// systemDefaultTables (defaults_system.go) for #508 PR 1, and
// textDefaultTables (defaults_text.go) for #538, keyed by the Mode each
// binds. Every mode in Modes() now has a non-empty entry here.
var defaultModeTables map[Mode]Table

// init populates catalog and defaultModeTables from the per-group vars
// declared above and in the other command_*.go/defaults_*.go files -- a new
// mode group only needs to add itself to the two append/assignment blocks
// below, not touch any other part of this file.
func init() {
	catalog = append(catalog, boardCommands...)
	catalog = append(catalog, modalCommands...)
	catalog = append(catalog, panelCommands...)
	catalog = append(catalog, systemCommands...)
	catalog = append(catalog, textCommands...)

	defaultModeTables = map[Mode]Table{
		ModeNormal: normalDefaults,
		ModeDetail: detailDefaults,
	}
	for mode, table := range modalDefaultTables {
		defaultModeTables[mode] = table
	}
	for mode, table := range panelDefaultTables {
		defaultModeTables[mode] = table
	}
	for mode, table := range systemDefaultTables {
		defaultModeTables[mode] = table
	}
	for mode, table := range textDefaultTables {
		defaultModeTables[mode] = table
	}

	// buildCommandModeIndex (capability.go) is called from here, not from
	// its own init in capability.go, because Go runs a package's init
	// functions in filename order: capability.go sorts before catalog.go,
	// so an init in capability.go itself would run first and read a nil
	// defaultModeTables, silently building an empty (over-rejecting) index.
	buildCommandModeIndex()
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
// detail-panel tables for #507, the six navigable modals/git panel/dispatch
// modal/help/errorMode for #508 PR 1, and the seven confirm/text-input
// modes (close_confirm, label_confirm, delete, create, config, search,
// comment) for #538 -- every mode in Modes() now has a non-empty default
// table. It returns a defensive copy, mirroring Commands() (above), Modes()
// (mode.go) and Entries() (keymap.go) -- callers cannot mutate the
// package-level defaultModeTables (or its nested per-mode Tables) through
// the result.
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
