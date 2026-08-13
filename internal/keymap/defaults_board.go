package keymap

// normalDefaults is the default ModeNormal table. Keys are already in
// canonical Sequence.String() form (single BubbleTea key names, or
// space-joined multi-key sequences for the "g"-prefixed go-navigation
// commands). #502 remapped nine commands to neovim-style mnemonics; see
// the ticket's remap table for the old->new key history.
var normalDefaults = Table{
	"q":         CommandBinding(CommandQuit),
	"?":         CommandBinding(CommandHelp),
	"c":         CommandBinding(CommandConfig),
	"n":         CommandBinding(CommandCardNew),
	"e":         CommandBinding(CommandCardEdit),
	"o":         CommandBinding(CommandCardOpenTicket),
	"p":         CommandBinding(CommandCardOpenPR),
	"x":         CommandBinding(CommandCardClose),
	"d":         CommandBinding(CommandCardDelete),
	"a":         CommandBinding(CommandCardAssign),
	"r":         CommandBinding(CommandBoardRefresh),
	"/":         CommandBinding(CommandBoardSearch),
	"f":         CommandBinding(CommandBoardFilter),
	"s":         CommandBinding(CommandBoardSortOrder),
	"P":         CommandBinding(CommandViewPRList),
	"m":         CommandBinding(CommandViewMilestoneList),
	"A":         CommandBinding(CommandViewAgentList),
	"G":         CommandBinding(CommandViewGitPanel),
	"D":         CommandBinding(CommandViewDispatch),
	"g r":       CommandBinding(CommandNavReference),
	"g a":       CommandBinding(CommandNavAgent),
	"l":         CommandBinding(CommandNavDetailFocus),
	"right":     CommandBinding(CommandNavDetailFocus),
	"j":         CommandBinding(CommandNavCursorDown),
	"down":      CommandBinding(CommandNavCursorDown),
	"k":         CommandBinding(CommandNavCursorUp),
	"up":        CommandBinding(CommandNavCursorUp),
	"tab":       CommandBinding(CommandNavColumnNext),
	"shift+tab": CommandBinding(CommandNavColumnPrev),
	"1":         CommandBinding(CommandNavColumn1),
	"2":         CommandBinding(CommandNavColumn2),
	"3":         CommandBinding(CommandNavColumn3),
	"4":         CommandBinding(CommandNavColumn4),
	"5":         CommandBinding(CommandNavColumn5),
	"6":         CommandBinding(CommandNavColumn6),
	"7":         CommandBinding(CommandNavColumn7),
	"8":         CommandBinding(CommandNavColumn8),
	"9":         CommandBinding(CommandNavColumn9),
}

// detailDefaults is the default ModeDetail table. It reuses several
// normal-mode command ids (app.quit, app.help, card.edit, card.open_ticket,
// card.open_pr, board.refresh, nav.reference, nav.column_next,
// nav.column_prev, nav.column_1..9) and adds the detail-only ids. #502
// remapped nav.reference's key here too (m -> "g r"), mirroring the
// normal-mode remap; nav.agent is not bound in the detail panel.
var detailDefaults = Table{
	"q":         CommandBinding(CommandQuit),
	"e":         CommandBinding(CommandCardEdit),
	"r":         CommandBinding(CommandBoardRefresh),
	"o":         CommandBinding(CommandCardOpenTicket),
	"g r":       CommandBinding(CommandNavReference),
	"p":         CommandBinding(CommandCardOpenPR),
	"?":         CommandBinding(CommandHelp),
	"h":         CommandBinding(CommandDetailBlur),
	"left":      CommandBinding(CommandDetailBlur),
	"esc":       CommandBinding(CommandDetailBlur),
	"j":         CommandBinding(CommandDetailScrollDown),
	"down":      CommandBinding(CommandDetailScrollDown),
	"k":         CommandBinding(CommandDetailScrollUp),
	"up":        CommandBinding(CommandDetailScrollUp),
	"tab":       CommandBinding(CommandNavColumnNext),
	"shift+tab": CommandBinding(CommandNavColumnPrev),
	"1":         CommandBinding(CommandNavColumn1),
	"2":         CommandBinding(CommandNavColumn2),
	"3":         CommandBinding(CommandNavColumn3),
	"4":         CommandBinding(CommandNavColumn4),
	"5":         CommandBinding(CommandNavColumn5),
	"6":         CommandBinding(CommandNavColumn6),
	"7":         CommandBinding(CommandNavColumn7),
	"8":         CommandBinding(CommandNavColumn8),
	"9":         CommandBinding(CommandNavColumn9),
}
