package keymap

// Command ids for the normal-mode and detail-panel surfaces (#507). Each
// constant here is the CommandID a default (or future user) binding
// resolves to; see catalog.go for the matching Command entries and
// defaults_board.go for the key bindings themselves. CommandQuit ("app.quit")
// is declared in command.go and reused here rather than redeclared.
const (
	CommandHelp              CommandID = "app.help"
	CommandConfig            CommandID = "app.config"
	CommandCardNew           CommandID = "card.new"
	CommandCardEdit          CommandID = "card.edit"
	CommandCardOpenTicket    CommandID = "card.open_ticket"
	CommandCardOpenPR        CommandID = "card.open_pr"
	CommandCardClose         CommandID = "card.close"
	CommandCardDelete        CommandID = "card.delete"
	CommandCardAssign        CommandID = "card.assign"
	CommandBoardRefresh      CommandID = "board.refresh"
	CommandBoardSearch       CommandID = "board.search"
	CommandBoardFilter       CommandID = "board.filter"
	CommandBoardSortOrder    CommandID = "board.sort_order"
	CommandViewPRList        CommandID = "view.pr_list"
	CommandViewMilestoneList CommandID = "view.milestone_list"
	CommandViewAgentList     CommandID = "view.agent_list"
	CommandViewGitPanel      CommandID = "view.git_panel"
	CommandViewDispatch      CommandID = "view.dispatch"
	CommandNavReference      CommandID = "nav.reference"
	CommandNavAgent          CommandID = "nav.agent"
	CommandNavDetailFocus    CommandID = "nav.detail_focus"
	CommandNavCursorDown     CommandID = "nav.cursor_down"
	CommandNavCursorUp       CommandID = "nav.cursor_up"
	CommandNavColumnNext     CommandID = "nav.column_next"
	CommandNavColumnPrev     CommandID = "nav.column_prev"
	CommandNavColumn1        CommandID = "nav.column_1"
	CommandNavColumn2        CommandID = "nav.column_2"
	CommandNavColumn3        CommandID = "nav.column_3"
	CommandNavColumn4        CommandID = "nav.column_4"
	CommandNavColumn5        CommandID = "nav.column_5"
	CommandNavColumn6        CommandID = "nav.column_6"
	CommandNavColumn7        CommandID = "nav.column_7"
	CommandNavColumn8        CommandID = "nav.column_8"
	CommandNavColumn9        CommandID = "nav.column_9"
	CommandDetailBlur        CommandID = "detail.blur"
	CommandDetailScrollDown  CommandID = "detail.scroll_down"
	CommandDetailScrollUp    CommandID = "detail.scroll_up"
)
