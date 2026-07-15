package main

import (
	"hash/fnv"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
	"github.com/muesli/termenv"
)

// Package-level styles.
// linkedPRGlyph is the Nerd Font glyph marking a linked pull request. It is
// rendered per-card (see cardDisplayText) and, prefixed to the aggregate count,
// in the status-bar PR indicator (see StatusBar.View).
const linkedPRGlyph = "\ue728"

var (
	activeBorderTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	inactiveBorderTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedCardStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	leftPanelStyle           = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("15"))
	rightPanelStyle          = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	outerStyle               = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	helpStyle                = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	prIndicatorStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("183"))
	workingIndicatorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	// Agent status badge styles (cenci card badges). All statuses render
	// as a single mark in plain foreground color -- no reverse/background --
	// so the badges read as one consistent family.
	agentRunningStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	agentDoneStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	agentStoppedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	agentNeedInputStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	agentFailedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	cardNumberStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	hintKeyStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	hintDescStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	// Git status segment styles (status bar), lazygit-style but muted to match
	// the rest of the palette: additions green, deletions red, push/pull
	// (ahead/behind) share one gentle orange since they're both just "sync"
	// state, not a warning.
	gitAddedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	gitDeletedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	gitAheadStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	gitBehindStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	// Status bar message styles use a dedicated renderer with forced ANSI256
	// so that colored messages always render, even in non-TTY environments.
	statusRenderer     = newStatusRenderer()
	statusErrorStyle   = statusRenderer.NewStyle().Foreground(lipgloss.Color("203"))
	statusWarningStyle = statusRenderer.NewStyle().Foreground(lipgloss.Color("222"))
	statusSuccessStyle = statusRenderer.NewStyle().Foreground(lipgloss.Color("114"))
	// dispatchSegmentStyle colors the normal ("on", no error) dispatch loop
	// status bar segment. The failing (LastError set) variant reuses
	// statusErrorStyle instead, consistent with other error states in the
	// status bar.
	dispatchSegmentStyle = statusRenderer.NewStyle().Foreground(lipgloss.Color("75"))
)

// newStatusRenderer creates a lipgloss renderer with ANSI256 forced,
// so status bar messages always display colors regardless of TTY detection.
func newStatusRenderer() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI256)
	return r
}

// labelPalette contains 8 muted 256-color ANSI codes for label coloring.
var labelPalette = []lipgloss.Color{
	lipgloss.Color("168"), // rose
	lipgloss.Color("114"), // green
	lipgloss.Color("75"),  // blue
	lipgloss.Color("222"), // gold
	lipgloss.Color("174"), // salmon
	lipgloss.Color("152"), // mauve
	lipgloss.Color("80"),  // teal
	lipgloss.Color("215"), // orange
}

// semanticLabelColors maps common label names (lowercase) to specific palette colors.
var semanticLabelColors = map[string]lipgloss.Color{
	"bug":           lipgloss.Color("168"),
	"critical":      lipgloss.Color("168"),
	"feature":       lipgloss.Color("114"),
	"enhancement":   lipgloss.Color("114"),
	"design":        lipgloss.Color("75"),
	"question":      lipgloss.Color("75"),
	"docs":          lipgloss.Color("222"),
	"documentation": lipgloss.Color("222"),
	"infra":         lipgloss.Color("174"),
	"ops":           lipgloss.Color("174"),
	"chore":         lipgloss.Color("152"),
	"refactor":      lipgloss.Color("152"),
	"test":          lipgloss.Color("80"),
	"testing":       lipgloss.Color("80"),
	"backend":       lipgloss.Color("215"),
	"ui":            lipgloss.Color("215"),
}

// labelColor returns a deterministic color for a label.
// If the label has a provider-supplied hex color, it is used directly.
// Otherwise, semantic labels get fixed colors; unknown labels use FNV-32 hash.
func labelColor(label Label) lipgloss.Color {
	if label.Color != "" {
		return lipgloss.Color("#" + label.Color)
	}
	lower := strings.ToLower(label.Name)
	if c, ok := semanticLabelColors[lower]; ok {
		return c
	}
	h := fnv.New32a()
	h.Write([]byte(lower))
	return labelPalette[h.Sum32()%uint32(len(labelPalette))]
}

// helpHint points users to the full help popup (toggled by `?`). It is the
// anti-stuck pointer, so it is placed left-most in the normal-mode hints to
// survive left-to-right truncation on narrow terminals.
var helpHint = Hint{Key: "?", Desc: "Help"}

// normalModeHints are the default status bar hints shown in normal mode.
var normalModeHints = []Hint{
	{Key: "n", Desc: "New"},
	{Key: "e", Desc: "Edit"},
}

// detailFocusHints are the status bar hints shown when the detail panel is focused.
var detailFocusHints = []Hint{
	{Key: "e", Desc: "Edit"},
	{Key: "j/k", Desc: "Scroll"},
	{Key: "h", Desc: "Back"},
	{Key: "esc", Desc: "Back"},
}

// searchModeHints are the status bar hints shown when search mode is active.
var searchModeHints = []Hint{
	{Key: "enter", Desc: "Apply"},
	{Key: "esc", Desc: "Clear"},
	{Key: "↑/↓", Desc: "Navigate"},
}

// prPickerHints are the status bar hints shown when the PR picker modal is open.
var prPickerHints = []Hint{
	{Key: "\u25c0/\u25b6", Desc: "Cycle"},
	{Key: "enter", Desc: "Select"},
	{Key: "esc", Desc: "Cancel"},
}

// commentModeHints are the status bar hints shown when the comment input is active.
var commentModeHints = []Hint{
	{Key: "esc", Desc: "Cancel"},
	{Key: "enter", Desc: "Submit"},
}

// deleteCommentHints are the status bar hints shown at the delete flow's
// optional-comment step.
var deleteCommentHints = []Hint{
	{Key: "esc", Desc: "Cancel"},
	{Key: "enter", Desc: "Continue"},
}

// deleteConfirmHints are the status bar hints shown at the delete flow's
// retype-to-confirm step.
var deleteConfirmHints = []Hint{
	{Key: "esc", Desc: "Cancel"},
	{Key: "enter", Desc: "Confirm"},
}

// filterModeHints are the status bar hints shown in filter mode.
var filterModeHints = []Hint{
	{Key: "esc", Desc: "Cancel"},
	{Key: "j/k", Desc: "Navigate"},
	{Key: "enter", Desc: "Select"},
}

// helpModeHints are the status bar hints shown in help mode.
var helpModeHints = []Hint{
	{Key: "esc/?", Desc: "Close"},
	{Key: "j/k", Desc: "Scroll"},
}

// boardMode represents the current interaction mode of the board.
type boardMode int

const (
	normalMode boardMode = iota
	createMode
	creatingMode
	loadingMode
	errorMode
	configMode
	prPickerMode
	searchMode
	helpMode
	labelConfirmMode
	closeConfirmMode
	commentMode
	deleteMode
	filterMode
	assignMode
	gitPanelMode
	dispatchMode
	prListMode
)

const (
	statusMessageDuration     = 3 * time.Second
	longStatusMessageDuration = 30 * time.Second
	noneAssignee              = "(none)"
)

const (
	cenciWatchInitialBackoff = 1 * time.Second
	cenciWatchMaxBackoff     = 30 * time.Second
)

// cenciWatchClearThreshold is the number of consecutive cenci-watch watcher
// errors, with no intervening successful snapshot, required before the
// dispatch status-bar segment is cleared live. A lone transient blip (1
// error) is tolerated since the reconnect backoff ladder self-heals within
// ~1s; only a second consecutive error clears the segment (#333).
const cenciWatchClearThreshold = 2

// gitStatusPollInterval is the fixed interval for the background git status
// poll (a fallback for out-of-app changes), independent of any fetch/refresh
// completion so it can't spin on an ambiguous read result.
const gitStatusPollInterval = 12 * time.Second

// metadataTTLMultiplier and minMetadataTTL together determine how long
// collaborators/authenticated-user/repo-labels are cached before an
// automatic refresh cycle (periodic tick, post-action auto-refresh) is
// allowed to re-fetch them. The TTL is a multiple of refreshInterval, floored
// at minMetadataTTL so a short refresh_interval (e.g. 1m) can't cause
// metadata thrash. Explicit user actions (manual 'r', config save, error
// retry) always bypass this TTL and force a full metadata fetch.
const (
	metadataTTLMultiplier = 6
	minMetadataTTL        = 30 * time.Minute
)

// Agent window status values reported by the cenci-watch daemon (plain strings).
// Only the two surfaced as status-bar counts are named here.
const (
	agentStatusRunning   = "running"
	agentStatusNeedInput = "need-input"
	agentStatusFailed    = "failed"
)

// filterType represents the category of a filter selection.
type filterType int

const (
	filterTypeNone filterType = iota
	filterByLabel
	filterByAssignee
)

// filterItem represents a single entry in the filter picker list.
type filterItem struct {
	itemType filterType
	value    string
	isHeader bool
}

// LinkedPR represents a pull request linked to a card.
type LinkedPR struct {
	Number int
	Title  string
	URL    string
	Branch string
}

// Label represents a card label with an optional hex color.
type Label struct {
	Name  string
	Color string
}

// Assignee represents a user assigned to a card.
type Assignee struct {
	Login string
}

// Card represents a single Kanban card (e.g., a GitHub issue).
type Card struct {
	Number    int
	Title     string
	Labels    []Label
	Body      string
	URL       string
	LinkedPRs []LinkedPR
	Assignees []Assignee
}

// refreshTickMsg is sent when the periodic refresh timer fires.
type refreshTickMsg struct{}

// actionResultMsg is sent when an async shell action completes.
type actionResultMsg struct {
	success bool
	message string
}

// autoRefreshMsg is sent when the auto-refresh delay timer fires.
type autoRefreshMsg struct{}

// agentSnapshotMsg is sent when the cenci-watch watcher delivers a new state snapshot.
type agentSnapshotMsg struct {
	snapshot *cenciwatch.StateSnapshot
}

// cenciWatchErrorMsg is sent when reading from the cenci-watch watcher fails.
type cenciWatchErrorMsg struct {
	err error
}

// cenciWatchRetryMsg is sent when the cenci-watch reconnect backoff timer fires.
type cenciWatchRetryMsg struct{}

// gitStatusMsg is sent when a git status read completes (success or failure).
type gitStatusMsg struct {
	status gitdetect.Status
	err    error
}

// gitStatusTickMsg is sent when the background git status poll timer fires.
type gitStatusTickMsg struct{}

// configSavedMsg is sent when a config file has been saved successfully.
type configSavedMsg struct{}

// configSaveErrorMsg is sent when saving a config file fails.
type configSaveErrorMsg struct{ err error }

// prevCardInfo stores a card's column position and metadata for departure detection.
type prevCardInfo struct {
	colIdx int
	title  string
	labels []string
	// missingSeen marks a card already absent from one fetch; a missing card
	// only counts as departed once it stays missing on a second consecutive
	// fetch, so transient fetch glitches don't trigger cleanup.
	missingSeen bool
	// movedSeen marks a card already observed in a different column on one
	// fetch; a moved card only counts as departed once the move holds on a
	// second consecutive fetch, so a single bad fetch that misplaces cards
	// (e.g. a dropped-label fallback) can't trigger cleanup board-wide.
	movedSeen bool
}

// Column represents a Kanban column containing cards.
type Column struct {
	Title        string
	Cards        []Card
	Cursor       int
	ScrollOffset int
}

// boardFetchedMsg is sent when the provider successfully returns board data.
type boardFetchedMsg struct {
	board             provider.Board
	collaborators     []provider.Assignee
	authenticatedUser string
	collaboratorErr   error
	repoLabels        []string
	labelErr          error
	// metadataRequested records whether this fetch cycle asked fetchBoardCmd
	// to include collaborators/authenticated-user/labels, so the handler
	// knows whether to advance lastMetadataFetch.
	metadataRequested bool
}

// boardFetchErrorMsg is sent when the provider fails to fetch board data.
type boardFetchErrorMsg struct {
	err error
}

// cardCreatedMsg is sent when the provider successfully creates a card.
type cardCreatedMsg struct {
	card provider.Card
}

// cardCreateErrorMsg is sent when the provider fails to create a card.
type cardCreateErrorMsg struct {
	err error
}

// editorFinishedMsg is sent when the external editor process closes.
type editorFinishedMsg struct {
	editedContent   string
	originalContent string
	card            Card
	err             error
}

// cardUpdatedMsg is sent when the provider successfully updates a card.
type cardUpdatedMsg struct {
	card provider.Card
}

// cardUpdateErrorMsg is sent when the provider fails to update a card.
type cardUpdateErrorMsg struct {
	err error
}

// labelCreatedMsg is sent when a label has been created successfully.
type labelCreatedMsg struct{}

// labelCreateErrorMsg is sent when creating a label fails.
type labelCreateErrorMsg struct{ err error }

// labelConfirmState groups fields related to the label confirmation prompt.
type labelConfirmState struct {
	card          Card
	title         string
	body          string
	allLabels     []string
	unknownLabels []string
	currentIdx    int
}

// closeConfirmState groups fields related to the close-card confirmation prompt.
type closeConfirmState struct {
	card Card
}

// cardClosedMsg is sent when a card has been closed successfully.
type cardClosedMsg struct {
	card Card
}

// cardCloseErrorMsg is sent when closing a card fails.
type cardCloseErrorMsg struct {
	err error
}

// deleteStep represents which step of the two-step delete-confirm flow is active.
type deleteStep int

const (
	deleteStepComment deleteStep = iota
	deleteStepConfirm
)

// deleteState groups fields related to the delete-confirm modal's two steps:
// an optional-comment step and a retype-to-confirm step.
type deleteState struct {
	card         Card
	step         deleteStep
	commentInput textinput.Model
	confirmInput textinput.Model
	mismatchMsg  string
}

// deleteCommentPostedMsg is sent when addCommentForDeleteCmd successfully
// posts the delete flow's optional comment.
type deleteCommentPostedMsg struct {
	card Card
}

// deleteCommentErrorMsg is sent when addCommentForDeleteCmd fails to post the
// delete flow's optional comment. The delete must not proceed.
type deleteCommentErrorMsg struct {
	err error
}

// cardDeletedMsg is sent when deleteCardCmd successfully deletes a card.
type cardDeletedMsg struct {
	card Card
}

// cardDeleteErrorMsg is sent when deleteCardCmd fails to delete a card.
type cardDeleteErrorMsg struct {
	err error
	// commentPosted is true when this failure was reached via the
	// comment-then-delete chain (the comment successfully posted before
	// DeleteCard failed), indicating a partial-success state.
	commentPosted bool
}

// commentState groups fields related to the comment input modal.
type commentState struct {
	input             textinput.Model
	pendingAction     config.Action
	pendingCard       Card
	boardScope        bool
	prScope           bool
	fromDetailFocused bool
}

// pendingPRAction carries a scope: pr action (and any comment already
// gathered for it) while the PR picker is open, awaiting the user's PR
// selection. A nil pendingPRAction on Board means the picker's Enter key
// falls back to its original open-URL behavior.
type pendingPRAction struct {
	action  config.Action
	comment string
}

// assignItem represents a single entry in the assignee picker list.
type assignItem struct {
	login      string
	isAssigned bool
	isMe       bool
}

// assignState groups fields related to the assignee picker modal.
type assignState struct {
	items  []assignItem
	cursor int
}

// assigneesUpdatedMsg is sent when assignees have been updated successfully.
type assigneesUpdatedMsg struct {
	card provider.Card
}

// assigneesUpdateErrorMsg is sent when updating assignees fails.
type assigneesUpdateErrorMsg struct {
	err error
}

// assignModeHints are the status bar hints shown in assign mode.
var assignModeHints = []Hint{
	{Key: "esc", Desc: "Cancel"},
	{Key: "j/k", Desc: "Navigate"},
	{Key: "enter", Desc: "Toggle"},
}

// gitPanelItem represents a single entry in the git panel picker list.
type gitPanelItem struct {
	key  string
	name string
}

// gitPanelState groups fields related to the git panel modal.
type gitPanelState struct {
	items  []gitPanelItem
	cursor int
}

// gitPanelModeHints are the status bar hints shown in git panel mode.
var gitPanelModeHints = []Hint{
	{Key: "esc", Desc: "Cancel"},
	{Key: "j/k", Desc: "Navigate"},
	{Key: "enter", Desc: "Run"},
}

// prListEntry is one row in the global PR list: a linked PR together with the
// card and column it belongs to, so rows stay disambiguated across the board.
type prListEntry struct {
	pr          LinkedPR
	cardNumber  int
	columnTitle string
}

// prListState groups fields related to the global PR list modal.
//
// Rendering/handling precedence: loading -> err -> loaded. While loading,
// entries holds the card-linked fallback aggregated from the board; when the
// repo-wide fetch succeeds, entries is replaced with every open PR in the
// repository; on error, the fallback entries are kept and err records the
// sanitized failure.
type prListState struct {
	entries    []prListEntry
	cursor     int
	loading    bool
	err        string
	generation uint64
}

// openPRsMsg is sent when fetchOpenPRsCmd finishes listing the repository's
// open pull requests for the PR list modal.
type openPRsMsg struct {
	prs        []provider.LinkedPR
	err        error
	generation uint64
}

// prListModeHints are the base status bar hints shown in PR list mode; see
// prListActionHints for the full set including custom-action hints.
var prListModeHints = []Hint{
	{Key: "esc", Desc: "Cancel"},
	{Key: "j/k", Desc: "Navigate"},
	{Key: "enter", Desc: "Open"},
}

// prListActionHints returns the PR list mode hints: the base navigation
// hints plus one named hint per global scope: pr custom action, mirroring
// how normal mode surfaces action names. Only scope: pr actions appear
// because only they can fire inside the modal (handlePRListActionKey) —
// hinting other scopes would advertise keys that silently no-op.
func (b Board) prListActionHints() []Hint {
	hints := append([]Hint{}, prListModeHints...)
	keys := make([]string, 0, len(b.actions))
	for key := range b.actions {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		act := b.actions[key]
		if config.DefaultScope(act.Scope) == "pr" {
			hints = append(hints, Hint{Key: key, Desc: act.Name})
		}
	}
	return hints
}

// dispatchState groups fields related to the agent dispatch modal.
type dispatchState struct {
	loading    bool
	err        string
	running    bool
	repo       string
	dir        string
	enrolled   bool
	lastResult string
	lastLines  []string

	// loop is the daemon-owned background dispatch loop state, decoded
	// verbatim from the "loop" object in `cenci dispatch status --json`
	// (ticket #313). lazyboards is a pure reader of this state -- starting
	// and stopping the loop is a user-configured custom shell action, not a
	// code path here. loop is nil only when the top-level "loop" key was
	// entirely absent from the decoded JSON (a cenci binary that
	// predates this feature); in that case loopErr holds a guard message.
	loop    *dispatchLoopInfo
	loopErr string
}

// dispatchLoopInfo mirrors the "loop" object decoded from
// `cenci dispatch status --json`. See queryDispatchStatusCmd, which
// unmarshals directly into this type.
type dispatchLoopInfo struct {
	Enabled        bool   `json:"enabled"`
	DaemonRunning  bool   `json:"daemon_running"`
	Interval       string `json:"interval"`
	PassRunning    bool   `json:"pass_running"`
	LastRunAt      string `json:"last_run_at"`
	LastDispatched int    `json:"last_dispatched"`
	LastSkipped    int    `json:"last_skipped"`
	LastError      string `json:"last_error"`
}

// dispatchModeHints are the status bar hints shown in dispatch mode.
var dispatchModeHints = []Hint{
	{Key: "enter", Desc: "Enroll/Unenroll"},
	{Key: "o", Desc: "Dispatch once"},
	{Key: "esc", Desc: "Close"},
}

// dispatchStatusMsg is sent when queryDispatchStatusCmd finishes querying
// cenci for the current repo's dispatch enrollment status.
type dispatchStatusMsg struct {
	repo     string
	dir      string
	enrolled bool
	loop     *dispatchLoopInfo
	err      string
}

// dispatchEnrollMsg is sent when toggleEnrollCmd finishes enrolling or
// unenrolling the current repo with cenci.
type dispatchEnrollMsg struct {
	err string
}

// dispatchRunMsg is sent when dispatchOnceCmd finishes running a fleet-wide
// dispatch pass.
type dispatchRunMsg struct {
	result string
	err    string
	lines  []string
}

// configState groups fields related to the config modal.
type configState struct {
	providerOptions []string
	providerIndex   int
	repoInput       textinput.Model
	focus           int
	localPath       string
	firstLaunch     bool
	configSaved     bool
}

// createState groups fields related to the create-card modal.
type createState struct {
	titleInput      textarea.Model
	labelInput      textinput.Model
	assigneeOptions []string // ["(none)", "user (me)", "collab1", ...]
	assigneeIndex   int      // currently selected index
	pendingAssignee string   // login to assign after card creation
	focus           int      // 0=title, 1=label, 2=assignee
}

// Board is the top-level model implementing tea.Model.
type Board struct {
	Columns            []Column
	ActiveTab          int
	Width              int
	Height             int
	mode               boardMode
	validationErr      string
	provider           provider.BoardProvider
	spinner            spinner.Model
	loadErr            string
	statusBar          StatusBar
	loaded             bool
	actions            map[string]config.Action
	defaultActions     map[string]config.Action
	columnConfigs      []config.ColumnConfig
	executor           action.Executor
	repoOwner          string
	repoName           string
	providerName       string
	sessionMaxLen      int
	normalHints        []Hint
	comment            commentState
	assign             assignState
	config             configState
	create             createState
	detailFocused      bool
	detailScrollOffset int
	prPickerIndex      int
	pendingPRAction    *pendingPRAction
	// pendingSeq holds the keys typed so far of an unfinished custom-action
	// key sequence (e.g. "P" while waiting for the "f" of "Pf"). While
	// non-empty, normal-mode/detail-focused key handling routes every key to
	// handlePendingSeqKey. pendingSeqAlt records whether Alt was held on any
	// key of the sequence, so Alt+prefix triggers comment mode exactly like
	// Alt on a single-key action.
	pendingSeq         string
	pendingSeqAlt      bool
	refreshing         bool
	refreshInterval    time.Duration
	actionRefreshDelay time.Duration
	lastMetadataFetch  time.Time
	metadataTTL        time.Duration
	pendingAutoRefresh bool
	prevCards          map[int]prevCardInfo
	// cleanupBreakerWarning holds a status-bar warning set by
	// detectDepartures when the cleanup circuit breaker trips. It's a
	// transient hand-off: handleBoardFetched applies it as the timed status
	// message right after "Board refreshed"/"Filter has no matches" (which
	// would otherwise clobber it, since SetTimedMessage mutates the status
	// bar synchronously), then clears it. Empty means no trip occurred.
	cleanupBreakerWarning       string
	searchQuery                 string
	searchInput                 textinput.Model
	helpScrollOffset            int
	helpFromDetailFocused       bool
	workingLabel                string
	mouseEnabled                bool
	labelConfirm                labelConfirmState
	closeConfirm                closeConfirmState
	delete                      deleteState
	filterItems                 []filterItem
	filterCursor                int
	activeFilterType            filterType
	activeFilterValue           string
	collaborators               []Assignee
	authenticatedUser           string
	repoLabels                  []string
	cenciWatcher                cenciwatch.Watcher
	agentSnapshot               *cenciwatch.StateSnapshot
	agentBackoff                time.Duration
	cenciWatchConsecutiveErrors int
	gitReader                   gitdetect.Reader
	gitPanel                    gitPanelState
	prList                      prListState
	dispatch                    dispatchState
}

// NewBoard creates a Board in loadingMode (or configMode if firstLaunch).
// Call Init() to start fetching data.
func NewBoard(p provider.BoardProvider, actions map[string]config.Action, defaultActions map[string]config.Action, columnConfigs []config.ColumnConfig, executor action.Executor, repoOwner, repoName, providerName string, sessionMaxLen int, refreshInterval time.Duration, actionRefreshDelay time.Duration, workingLabel string, mouseEnabled bool, firstLaunch bool, watcher cenciwatch.Watcher, gitReader gitdetect.Reader) Board {
	ti := textarea.New()
	ti.Placeholder = "Title"
	ti.CharLimit = 0
	ti.ShowLineNumbers = false
	ti.KeyMap.InsertNewline.SetEnabled(false)

	li := textinput.New()
	li.Placeholder = "Label"
	li.CharLimit = 50

	s := spinner.New()
	s.Spinner = spinner.Dot

	// Build normal-mode hints: defaults + board-scope action hints.
	// Card-scope hints are omitted because no columns/cards are loaded yet;
	// rebuildNormalHints adds them after the first board fetch.
	hints := make([]Hint, 0, len(normalModeHints)+1)
	hints = append(hints, helpHint)
	hints = append(hints, normalModeHints...)
	for key, act := range actions {
		scope := config.DefaultScope(act.Scope)
		if scope == "board" {
			hints = append(hints, Hint{Key: key, Desc: act.Name})
		}
	}

	sb := NewStatusBar(hints)

	ri := textinput.New()
	ri.Placeholder = "owner/repo"
	ri.CharLimit = 100
	ri.Width = 40

	si := textinput.New()
	si.Placeholder = "Search..."
	si.CharLimit = 100
	si.Prompt = "/ "

	metadataTTL := refreshInterval * metadataTTLMultiplier
	if metadataTTL < minMetadataTTL {
		metadataTTL = minMetadataTTL
	}

	b := Board{
		mode:               loadingMode,
		provider:           p,
		spinner:            s,
		statusBar:          sb,
		actions:            actions,
		defaultActions:     defaultActions,
		columnConfigs:      columnConfigs,
		executor:           executor,
		repoOwner:          repoOwner,
		repoName:           repoName,
		providerName:       providerName,
		sessionMaxLen:      sessionMaxLen,
		refreshInterval:    refreshInterval,
		actionRefreshDelay: actionRefreshDelay,
		metadataTTL:        metadataTTL,
		workingLabel:       workingLabel,
		mouseEnabled:       mouseEnabled,
		normalHints:        hints,
		cenciWatcher:       watcher,
		gitReader:          gitReader,
		config: configState{
			providerOptions: []string{"github", "azure-devops"},
			providerIndex:   0,
			repoInput:       ri,
			localPath:       config.DefaultLocalPath,
			firstLaunch:     firstLaunch,
		},
		create: createState{
			titleInput: ti,
			labelInput: li,
		},
		searchInput: si,
	}

	if firstLaunch {
		b.enterConfigMode()
	}

	return b
}

// metadataDue reports whether collaborators/authenticated-user/repo-labels
// should be re-fetched: either they have never been fetched, or the
// metadataTTL has elapsed since the last successful metadata fetch.
func (b Board) metadataDue() bool {
	return b.lastMetadataFetch.IsZero() || time.Since(b.lastMetadataFetch) >= b.metadataTTL
}

// enterConfigMode sets up configMode with pre-populated values from runtime.
func (b *Board) enterConfigMode() {
	b.mode = configMode
	b.config.focus = 0
	b.validationErr = ""
	b.config.repoInput.Blur()

	if b.repoOwner != "" && b.repoName != "" {
		b.config.repoInput.SetValue(b.repoOwner + "/" + b.repoName)
	} else {
		b.config.repoInput.SetValue("")
	}

	b.config.providerIndex = 0
	for i, opt := range b.config.providerOptions {
		if opt == b.providerName {
			b.config.providerIndex = i
			break
		}
	}
}

// gitPanelBuiltinOrder is the fixed display/dispatch order of the git menu's
// built-in shortcuts: Push, Pull, Fetch, Mergetool, Stash push, Stash pop.
// This must hold regardless of Go map iteration order over defaultActions.
var gitPanelBuiltinOrder = []string{"P", "p", "f", "m", "s", "S"}

// enterGitPanel opens the git menu modal, populating its items from
// b.defaultActions in a fixed order (not map iteration order). If no default
// git actions are available (e.g. outside a git repo), this is a no-op and
// the panel does not open.
func (b *Board) enterGitPanel() {
	if len(b.defaultActions) == 0 {
		return
	}

	items := make([]gitPanelItem, 0, len(gitPanelBuiltinOrder))
	for _, key := range gitPanelBuiltinOrder {
		act, ok := b.defaultActions[key]
		if !ok {
			continue
		}
		items = append(items, gitPanelItem{key: key, name: act.Name})
	}

	b.gitPanel = gitPanelState{items: items, cursor: 0}
	b.mode = gitPanelMode
	b.statusBar.SetActionHints(gitPanelModeHints)
}

// enterPRList opens the global PR list modal, which surveys every open PR in
// the repository. The card-linked PRs aggregated here (across all columns and
// cards, deliberately ignoring any active search/filter) render immediately
// as a fallback while the caller's repo-wide fetch (fetchOpenPRsCmd) is in
// flight; handleOpenPRsFetched then replaces them with the full repo-wide
// list. Fallback order is column, then card, then PR within the card. It
// always opens, even with no linked PRs, so the modal can render its
// loading/empty states.
func (b *Board) enterPRList() {
	generation := b.prList.generation + 1
	var entries []prListEntry
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			for _, pr := range card.LinkedPRs {
				entries = append(entries, prListEntry{
					pr:          pr,
					cardNumber:  card.Number,
					columnTitle: col.Title,
				})
			}
		}
	}

	b.prList = prListState{entries: entries, cursor: 0, loading: true, generation: generation}
	b.mode = prListMode
	b.statusBar.SetActionHints(b.prListActionHints())
}

// createModalWidth returns the modal width for the create-card dialog (60% of terminal width, min 20).
func (b Board) createModalWidth() int {
	w := b.Width * 60 / 100
	if w < 20 {
		w = 20
	}
	return w
}

// recalcCreateInputs updates the title textarea and label input widths and
// the textarea height based on current terminal dimensions and content.
func (b *Board) recalcCreateInputs() {
	modalWidth := b.createModalWidth()
	// renderModal uses Padding(1, 2): 2 chars left + 2 chars right = 4 chars padding
	// Plus border: 1 char left + 1 char right = 2 chars
	// Total horizontal overhead = 6
	// The textarea.Width() getter subtracts the prompt width (2 chars for "> "),
	// so we add that back when calling SetWidth to get the desired Width() value.
	innerWidth := modalWidth - 6
	if innerWidth < 1 {
		innerWidth = 1
	}

	promptWidth := lipgloss.Width(b.create.titleInput.Prompt)
	b.create.titleInput.SetWidth(innerWidth + promptWidth)
	b.create.labelInput.Width = innerWidth

	// Auto-expand textarea height based on visual (wrapped) line count.
	// LineCount() returns logical lines (separated by newlines), but since
	// newline insertion is disabled, we need to count wrapped visual lines.
	contentWidth := b.create.titleInput.Width()
	if contentWidth < 1 {
		contentWidth = 1
	}
	visualLines := 0
	value := b.create.titleInput.Value()
	if value == "" {
		visualLines = 1
	} else {
		for _, line := range strings.Split(value, "\n") {
			w := lipgloss.Width(line)
			if w == 0 {
				visualLines++
			} else {
				visualLines += (w + contentWidth - 1) / contentWidth
			}
		}
	}
	maxHeight := b.Height * 50 / 100
	if maxHeight < 1 {
		maxHeight = 1
	}
	if visualLines > maxHeight {
		visualLines = maxHeight
	}
	b.create.titleInput.SetHeight(visualLines)
}

// layoutDimensions computes the panel layout dimensions.
// panelHeight = terminal height - outer border (2) - help bar (1) - panel borders (2) - bottom row guard (1) = Height - 6.
// leftContentWidth = left panel content area (40% of inner width, minus border).
// rightContentWidth = right panel content area (remaining width, minus border).
func (b Board) layoutDimensions() (panelHeight, leftContentWidth, rightContentWidth int) {
	innerWidth := b.Width - 2
	leftTotal := innerWidth * 2 / 5
	leftContentWidth = leftTotal - 2
	rightTotal := innerWidth - leftTotal
	rightContentWidth = rightTotal - 2
	panelHeight = b.Height - 6
	if panelHeight < 1 {
		panelHeight = 1
	}
	return
}

// resolveAction looks up an action by key, checking the active column's
// per-column actions first (if any), then falling back to global actions.
// scope: pr actions are only returned when the active card has at least one
// linked PR (mirrors the gating on the hardcoded "p" open-PR hint/key).
func (b *Board) resolveAction(key string) (config.Action, bool) {
	if len(b.Columns) > 0 && b.ActiveTab < len(b.Columns) {
		colTitle := b.Columns[b.ActiveTab].Title
		for _, cc := range b.columnConfigs {
			if strings.EqualFold(cc.Name, colTitle) {
				if act, ok := cc.Actions[key]; ok {
					if b.prScopeGated(act) {
						return config.Action{}, false
					}
					return act, true
				}
				break
			}
		}
	}
	if act, ok := b.actions[key]; ok {
		if b.prScopeGated(act) {
			return config.Action{}, false
		}
		return act, true
	}
	// No fallback to b.defaultActions: built-in git actions are scoped to the
	// git menu (see handleGitPanelKey) so normal-mode keys stay user-owned.
	return config.Action{}, false
}

// prScopeGated reports whether act is a scope: pr action that must be
// hidden/refused because the active card has no linked PRs (mirrors the
// gating on the hardcoded "p" open-PR hint/key).
func (b *Board) prScopeGated(act config.Action) bool {
	return act.Scope == "pr" && len(b.selectedCard().LinkedPRs) == 0
}

// rebuildNormalHints reconstructs the normalHints slice by merging global
// actions with the active column's per-column actions (column overrides global).
func (b *Board) rebuildNormalHints() {
	hints := make([]Hint, 0, len(normalModeHints)+len(b.actions)+2)

	// Help pointer stays left-most so it survives left-to-right truncation.
	hints = append(hints, helpHint)

	// Determine if the active column has cards.
	hasCards := false
	if len(b.Columns) > 0 && b.ActiveTab < len(b.Columns) {
		hasCards = len(b.Columns[b.ActiveTab].Cards) > 0
	}

	// Default mode hints.
	hints = append(hints, normalModeHints...)

	// Collect action hints with their scopes: start with global, overlay column-specific.
	type actionEntry struct {
		hint  Hint
		scope string
	}
	actionEntries := make(map[string]actionEntry)
	for key, act := range b.actions {
		scope := config.DefaultScope(act.Scope)
		actionEntries[key] = actionEntry{hint: Hint{Key: key, Desc: act.Name}, scope: scope}
	}

	// Overlay active column's actions.
	if len(b.Columns) > 0 && b.ActiveTab < len(b.Columns) {
		colTitle := b.Columns[b.ActiveTab].Title
		for _, cc := range b.columnConfigs {
			if strings.EqualFold(cc.Name, colTitle) {
				for key, act := range cc.Actions {
					scope := config.DefaultScope(act.Scope)
					actionEntries[key] = actionEntry{hint: Hint{Key: key, Desc: act.Name}, scope: scope}
				}
				break
			}
		}
	}

	// Filter: show board-scope hints always; card-scope hints only when column
	// has cards; pr-scope hints only when the selected card has a linked PR
	// (same gate as the hardcoded "p" open-PR hint above).
	hasLinkedPR := hasCards && len(b.selectedCard().LinkedPRs) > 0
	for _, entry := range actionEntries {
		switch entry.scope {
		case "board":
			hints = append(hints, entry.hint)
		case "pr":
			if hasLinkedPR {
				hints = append(hints, entry.hint)
			}
		default:
			if hasCards {
				hints = append(hints, entry.hint)
			}
		}
	}

	b.normalHints = hints
}

// mapSlice transforms each element of in with f, returning nil when in is
// empty (never an empty non-nil slice) so callers preserve nil-vs-empty
// semantics for downstream comparisons.
func mapSlice[T, U any](in []T, f func(T) U) []U {
	if len(in) == 0 {
		return nil
	}
	result := make([]U, len(in))
	for i, v := range in {
		result[i] = f(v)
	}
	return result
}

func mapLinkedPRs(prs []provider.LinkedPR) []LinkedPR {
	return mapSlice(prs, func(pr provider.LinkedPR) LinkedPR {
		return LinkedPR{Number: pr.Number, Title: pr.Title, URL: pr.URL, Branch: pr.Branch}
	})
}

func mapLabels(labels []provider.Label) []Label {
	return mapSlice(labels, func(l provider.Label) Label {
		return Label{Name: l.Name, Color: l.Color}
	})
}

func mapAssignees(assignees []provider.Assignee) []Assignee {
	return mapSlice(assignees, func(a provider.Assignee) Assignee {
		return Assignee{Login: a.Login}
	})
}

// mapProviderCard converts a provider.Card to a main-package Card.
func mapProviderCard(c provider.Card) Card {
	return Card{
		Number:    c.Number,
		Title:     c.Title,
		Labels:    mapLabels(c.Labels),
		Body:      c.Body,
		URL:       c.URL,
		LinkedPRs: mapLinkedPRs(c.LinkedPRs),
		Assignees: mapAssignees(c.Assignees),
	}
}

// selectedCard returns the card currently under the cursor, accounting for
// active search and global filters. When either is active, the cursor indexes
// into the filtered list; otherwise it indexes into the raw column cards.
func (b *Board) selectedCard() Card {
	cards := b.visibleCards()
	if len(cards) == 0 {
		return Card{}
	}
	cursor := b.Columns[b.ActiveTab].Cursor
	if cursor >= len(cards) {
		return cards[len(cards)-1]
	}
	if cursor < 0 {
		return cards[0]
	}
	return cards[cursor]
}

// visibleCards returns the active column's cards after applying any active
// search query or global filter.
func (b *Board) visibleCards() []Card {
	if len(b.Columns) == 0 || b.ActiveTab < 0 || b.ActiveTab >= len(b.Columns) {
		return nil
	}
	if b.searchQuery != "" || b.activeFilterType != filterTypeNone {
		return b.filteredCards()
	}
	return b.Columns[b.ActiveTab].Cards
}

// matchesGlobalFilter returns true if a card matches the active global filter.
// Uses case-insensitive comparison (strings.EqualFold) per lessons-learned.
func (b *Board) matchesGlobalFilter(card Card) bool {
	switch b.activeFilterType {
	case filterByLabel:
		for _, label := range card.Labels {
			if strings.EqualFold(label.Name, b.activeFilterValue) {
				return true
			}
		}
		return false
	case filterByAssignee:
		for _, a := range card.Assignees {
			if strings.EqualFold(a.Login, b.activeFilterValue) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// filteredCards returns the cards in the active column that match the current
// global filter and search query. If neither is active, all cards are returned.
func (b *Board) filteredCards() []Card {
	col := b.Columns[b.ActiveTab]
	cards := col.Cards

	// Apply global filter first.
	if b.activeFilterType != filterTypeNone {
		var filtered []Card
		for _, card := range cards {
			if b.matchesGlobalFilter(card) {
				filtered = append(filtered, card)
			}
		}
		cards = filtered
	}

	// Then apply search filter.
	if b.searchQuery == "" {
		return cards
	}
	query := strings.ToLower(b.searchQuery)
	var result []Card
	for _, card := range cards {
		if matchesSearch(card, query) {
			result = append(result, card)
		}
	}
	return result
}

// totalFilteredCards returns the total number of cards across all columns
// that match the active global filter. Returns 0 if no filter is active
// or no cards match.
func (b *Board) totalFilteredCards() int {
	total := 0
	for i := range b.Columns {
		count := b.filteredCardsForColumn(i)
		if count > 0 {
			total += count
		}
	}
	return total
}

// filteredCardsForColumn returns the number of cards in the given column
// that match the active global filter. Returns -1 if no filter is active.
func (b *Board) filteredCardsForColumn(colIdx int) int {
	if b.activeFilterType == filterTypeNone {
		return -1
	}
	if colIdx < 0 || colIdx >= len(b.Columns) {
		return 0
	}
	count := 0
	for _, card := range b.Columns[colIdx].Cards {
		if b.matchesGlobalFilter(card) {
			count++
		}
	}
	return count
}

// clearFilter resets the global filter state and clamps cursor/scroll for the active column.
func (b *Board) clearFilter() {
	b.activeFilterType = filterTypeNone
	b.activeFilterValue = ""
	if len(b.Columns) > 0 && b.ActiveTab < len(b.Columns) {
		col := &b.Columns[b.ActiveTab]
		if col.Cursor >= len(col.Cards) {
			col.Cursor = len(col.Cards) - 1
			if col.Cursor < 0 {
				col.Cursor = 0
			}
		}
		col.ScrollOffset = 0
	}
}

// matchesSearch returns true if a card matches the search query.
// It checks the card title, card number, and label names (all case-insensitive).
func matchesSearch(card Card, query string) bool {
	if strings.Contains(strings.ToLower(card.Title), query) {
		return true
	}
	if strings.Contains(strconv.Itoa(card.Number), query) {
		return true
	}
	for _, label := range card.Labels {
		if strings.Contains(strings.ToLower(label.Name), query) {
			return true
		}
	}
	return false
}

// clearSearch resets the search state: clears the query, input, and resets
// cursor/scroll for the active column.
func (b *Board) clearSearch() {
	b.searchQuery = ""
	b.searchInput.SetValue("")
	b.searchInput.Blur()
	col := &b.Columns[b.ActiveTab]
	col.Cursor = 0
	col.ScrollOffset = 0
}

// collectFilterItems scans all columns for unique labels and assignees,
// returning a list of filterItems with section headers.
func (b *Board) collectFilterItems() []filterItem {
	// Build a set of column titles for exclusion (case-insensitive).
	columnNames := make(map[string]bool, len(b.Columns))
	for _, col := range b.Columns {
		columnNames[strings.ToLower(col.Title)] = true
	}

	// Collect unique labels (case-insensitive dedup), excluding column names.
	labelSeen := make(map[string]bool)
	var labels []string
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			for _, label := range card.Labels {
				lower := strings.ToLower(label.Name)
				if columnNames[lower] {
					continue
				}
				if !labelSeen[lower] {
					labelSeen[lower] = true
					labels = append(labels, label.Name)
				}
			}
		}
	}

	// Collect unique assignees (case-insensitive dedup).
	assigneeSeen := make(map[string]bool)
	var assignees []string
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			for _, a := range card.Assignees {
				lower := strings.ToLower(a.Login)
				if !assigneeSeen[lower] {
					assigneeSeen[lower] = true
					assignees = append(assignees, a.Login)
				}
			}
		}
	}

	if len(labels) == 0 && len(assignees) == 0 {
		return nil
	}

	var items []filterItem

	if len(labels) > 0 {
		items = append(items, filterItem{isHeader: true, value: "Labels"})
		for _, name := range labels {
			items = append(items, filterItem{itemType: filterByLabel, value: name})
		}
	}

	if len(assignees) > 0 {
		items = append(items, filterItem{isHeader: true, value: "Assignees"})
		for _, login := range assignees {
			items = append(items, filterItem{itemType: filterByAssignee, value: login})
		}
	}

	return items
}

// collectKnownLabels returns a set of all label names (lowercased) across the board.
func (b *Board) collectKnownLabels() map[string]bool {
	known := make(map[string]bool)
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			for _, label := range card.Labels {
				known[strings.ToLower(label.Name)] = true
			}
		}
	}
	// Include the repository's full label set so labels that exist but are not
	// attached to any visible card are still recognized as known.
	for _, name := range b.repoLabels {
		known[strings.ToLower(name)] = true
	}
	return known
}

// agentStatusFor returns the cenci window state joined to card by ticket
// number, or nil if no snapshot is stored yet or no window matches.
func (b Board) agentStatusFor(card Card) *cenciwatch.WindowState {
	return b.agentStatusForNumber(card.Number)
}

// agentStatusForNumber returns the cenci window state whose name joins to
// the given ticket number, or nil if no snapshot is stored yet or no window
// matches. A window joins when its name is exactly "<number>" or starts with
// "<number>-" (cenci names dispatched windows "<number>-<skill>", e.g.
// "230-refine"). The trailing "-" is a boundary, so card #23 never matches
// "230-...". This is backward-compatible with cenci's older
// "<number>-<title-slug>" names. When several windows share the number, an
// active one (running / need_input) wins over any other status, else the first
// match in snapshot order.
func (b Board) agentStatusForNumber(number int) *cenciwatch.WindowState {
	if b.agentSnapshot == nil {
		return nil
	}
	num := strconv.Itoa(number)
	prefix := num + "-"
	var match *cenciwatch.WindowState
	for i := range b.agentSnapshot.Windows {
		w := &b.agentSnapshot.Windows[i]
		if w.WindowName != num && !strings.HasPrefix(w.WindowName, prefix) {
			continue
		}
		if w.Status == agentStatusRunning || w.Status == agentStatusNeedInput {
			return w
		}
		if match == nil {
			match = w
		}
	}
	return match
}

// agentBadgeFor returns the fixed-width badge text for the card's live agent
// window, or "" when there is no match or the status is idle/unknown.
func (b Board) agentBadgeFor(card Card) string {
	ws := b.agentStatusFor(card)
	if ws == nil {
		return ""
	}
	return agentBadgeText(ws.Status, ws.Agent)
}

// agentCounts returns how many cards on the current board have a live agent
// window in the running / need_input states. Counts are board-scoped: only
// windows that join to a visible card (via agentStatusFor) contribute, keeping
// the status-bar summary consistent with the per-card badges. When no snapshot
// is stored (cenci off/absent), agentStatusFor returns nil for every card
// and both counts are naturally zero.
func (b Board) agentCounts() (running, needInput int) {
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			ws := b.agentStatusFor(card)
			if ws == nil {
				continue
			}
			switch ws.Status {
			case agentStatusRunning:
				running++
			case agentStatusNeedInput:
				needInput++
			}
		}
	}
	return
}

// prCounts sums the linked pull requests across every card in every column,
// producing the board-wide total shown in the status-bar PR indicator. It is a
// raw count of linked PRs with no open/merged/closed filtering (LinkedPR has no
// state today). Mirrors agentCounts' full-board iteration.
func (b Board) prCounts() int {
	total := 0
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			total += len(card.LinkedPRs)
		}
	}
	return total
}

func (b Board) Init() tea.Cmd {
	if b.config.firstLaunch {
		return nil
	}
	cmd := tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider, true))
	if b.cenciWatcher != nil {
		cmd = tea.Batch(cmd, subscribeCenciWatchCmd(b.cenciWatcher))
	}
	if b.gitReader != nil {
		cmd = tea.Batch(cmd, fetchGitStatusCmd(b.gitReader, "."), scheduleGitStatusTick(b))
	}
	return cmd
}
