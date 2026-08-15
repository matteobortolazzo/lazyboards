package main

import (
	"errors"
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
	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
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
	// mutedRowStyle renders non-selected rows across every list-like surface
	// (#478) -- the counterpart to selectedCardStyle in the selectedRowStyle
	// choke point. Reuses gray 245, matching cardNumberStyle/subIssueStyle.
	mutedRowStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	leftPanelStyle        = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("15"))
	rightPanelStyle       = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	outerStyle            = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	helpStyle             = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	prIndicatorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("183"))
	workingIndicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	// Agent status badge styles (cenci card badges). All statuses render
	// as a single mark in plain foreground color -- no reverse/background --
	// so the badges read as one consistent family.
	agentRunningStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("75"))
	agentDoneStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	agentStoppedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	agentNeedInputStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	agentFailedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	// PR status badge styles (#431), same single-mark/solid-color family as
	// the agent status badges above. prIndicatorStyle (defined above) remains
	// the neutral/unknown fallback for the board glyph.
	prMergeableStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	prConflictingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	prBlockedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	prDraftStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cardNumberStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	// subIssueStyle renders GitHub's native sub-issue relationship lines
	// (#460) -- both the parent/has-children and child/is-sub-issue lines --
	// in a single muted gray, deliberately distinct from PR purple (183) and
	// the agent/action status hues so structural metadata isn't misread as
	// status.
	subIssueStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	// progressCompleteStyle marks a fully-complete progress bar (e.g.
	// milestone/sub-issue completion), reusing the same success green (114)
	// as agentDoneStyle/prMergeableStyle/gitAddedStyle/statusSuccessStyle,
	// deliberately distinct from PR purple (183) and the other status hues.
	progressCompleteStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	hintKeyStyle          = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	hintDescStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
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
	milestoneListMode
	agentListMode
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
// Not every status is named here — "done", "stopped", and "idle" are used as
// literals elsewhere (agentStatusSymbol, agentCounts) matching view.go's
// existing convention.
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
	filterByMilestone
)

// filterItem represents a single entry in the filter picker list.
type filterItem struct {
	itemType filterType
	value    string
	isHeader bool
}

// LinkedPR represents a pull request linked to a card.
//
// IsDraft/Mergeable/MergeStateStatus/State mirror provider.LinkedPR's raw
// GitHub fields verbatim; deriving a status/glyph/style from them is
// presentation logic that lives in view.go (prStatus, prStatusSymbol,
// prStatusStyle).
type LinkedPR struct {
	Number           int
	Title            string
	URL              string
	Branch           string
	IsDraft          bool
	Mergeable        string
	MergeStateStatus string
	State            string
}

// Blocker mirrors provider.Blocker's raw GitHub fields verbatim: a single
// issue from a card's blockedBy connection (#628, #630). Deriving a
// human-facing status glyph/style, or comparing RepoNameWithOwner against
// the board's own configured repo to detect a cross-repo blocker, is
// presentation logic that lives in view.go, not here.
type Blocker struct {
	Number            int
	State             string
	URL               string
	RepoNameWithOwner string
}

// Milestone represents a repository-wide GitHub milestone shown in the
// Milestones modal (i), independent of any board card. Distinct from
// Card.Milestone (a plain string naming the milestone a card belongs to,
// used for filter matching) -- this type carries the full aggregate data
// (progress, counts, due date, URL) for the repo-wide modal. Mirrors
// provider.Milestone field-for-field via mapMilestones, matching the
// LinkedPR/mapLinkedPRs convention.
type Milestone struct {
	Title              string
	URL                string
	DueOn              *time.Time
	OpenIssueCount     int
	ClosedIssueCount   int
	ProgressPercentage float64
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
//
// ParentNumber/SubIssueCount/SubIssueCompleted mirror provider.Card's
// sub-issue relationship fields (#460, #475): ParentNumber is this card's
// parent issue number (0 if none), SubIssueCount is this card's sub-issue
// count (0 if none), and SubIssueCompleted is how many of those sub-issues
// are closed (0 if none).
//
// BlockedByCount/TotalBlockedByCount/BlockingCount/TotalBlockingCount/
// Blockers mirror provider.Card's issue-dependency fields (#628, #630)
// verbatim: BlockedByCount/BlockingCount are GitHub's open-only counts,
// TotalBlockedByCount/TotalBlockingCount include closed issues too, and
// Blockers is the bounded (first 10) list of issues blocking this one, with
// no state filtering or dedup applied.
type Card struct {
	Number            int
	Title             string
	Labels            []Label
	Body              string
	URL               string
	LinkedPRs         []LinkedPR
	Assignees         []Assignee
	Milestone         string
	CreatedAt         time.Time
	ParentNumber      int
	SubIssueCount     int
	SubIssueCompleted int

	BlockedByCount      int
	TotalBlockedByCount int
	BlockingCount       int
	TotalBlockingCount  int
	Blockers            []Blocker
}

// refreshTickMsg is sent when the periodic refresh timer fires.
type refreshTickMsg struct{}

// actionResultMsg is sent when an async shell action completes.
type actionResultMsg struct {
	success bool
	message string
}

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

// configSavedMsg is sent when a config file has been saved successfully. It
// carries the provider name and "owner/repo" that were just written: a
// BoardProvider is bound to one owner/repo at construction, so the handler
// needs them to rebuild the provider and retarget the board. Without that,
// saving a different repo only refreshes the previous one.
type configSavedMsg struct {
	provider string
	repo     string
}

// configSaveErrorMsg is sent when saving a config file fails.
type configSaveErrorMsg struct{ err error }

// sortOrderSavedMsg is sent when the sort direction has been persisted to the
// runtime-state file successfully.
type sortOrderSavedMsg struct{}

// sortOrderSaveErrorMsg is sent when persisting the sort direction fails.
type sortOrderSaveErrorMsg struct{ err error }

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
	// openPRs is the repo-wide open pull request listing fetched alongside
	// the board (every cycle, not TTL-gated), feeding the status-bar PR
	// indicator. openPRsFetched distinguishes a successful listing — even an
	// empty one — from a failed/absent fetch (non-fatal, like metadata): when
	// false, the handler keeps the previously known count.
	openPRs        []provider.LinkedPR
	openPRsFetched bool
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

// gitPanelItem represents a single entry in the git panel picker list. action
// is the resolved config.Action key resolves to in the ModeGitPanel registry
// table (keymap_panels.go's gitPanelItemsFromKeymap), so runGitPanelCommand's
// CommandGitPanelRun can dispatch the cursor-selected entry without a second
// keymap lookup by key.
type gitPanelItem struct {
	// key is exempt from sanitizeSingleLine: every key value originates
	// from a ParseSequence-canonicalized Entry.Sequence table key (same
	// exemption rationale as Hint.Key in statusbar.go), so it can never
	// carry hostile content.
	key    string
	name   string
	action config.Action
}

// gitPanelState groups fields related to the git panel modal.
type gitPanelState struct {
	items  []gitPanelItem
	cursor int
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

// milestoneListState groups fields related to the Milestones modal.
//
// Rendering/handling precedence: loading -> err -> loaded. Unlike
// prListState, there is no card-linked fallback: entries stays empty (nil)
// in both the loading and err states, and is only populated on a successful
// fetch -- this is one fast repo-wide query with no board-derived substitute
// (a board-derived row would have no counts, no progress and no URL).
type milestoneListState struct {
	entries    []Milestone
	cursor     int
	loading    bool
	err        string
	generation uint64
}

// milestonesFetchedMsg is sent when fetchMilestonesCmd finishes listing the
// repository's open milestones for the Milestones modal.
type milestonesFetchedMsg struct {
	milestones []provider.Milestone
	err        error
	generation uint64
}

// agentListEntry is one row in the agents list modal: a cenci-watch window
// together with the board card it joins to. cardNumber is 0 when the window
// name doesn't join to any visible card (same join rule as
// agentStatusForNumber).
type agentListEntry struct {
	window      cenciwatch.WindowState
	cardNumber  int
	columnTitle string
}

// agentListState groups fields related to the agents list modal. Rows are not
// stored: they are derived live from the streamed snapshot by
// agentListEntries(), so the cursor must be re-clamped wherever the snapshot
// is replaced while the modal is open. cardNumber, when non-zero, scopes the
// modal to that card's windows (the multi-window case of the s jump); 0 is
// the global w modal.
type agentListState struct {
	cursor     int
	cardNumber int
}

// Agents-modal state messages, shared by viewAgentListModal and
// handleAgentJumpKey's zero-window branch so the modal and the s jump report
// the same cenciwatch state the same way.
const (
	agentListMsgNotEnabled = "cenci-watch is not enabled"
	agentListMsgWaiting    = "Waiting for cenci-watch daemon..."
	agentListMsgNoWindows  = "No agent windows"
)

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
	// (ticket #313). Upstream, that object is the same producer type as the
	// socket snapshot's "dispatch" object (cenci's watch.DispatchState), so
	// both wire boundaries decode into the one cenciwatch.DispatchState type
	// (#402). The dispatch modal renders this state and also toggles it on/off
	// via the built-in 'l' key (a confirmed toggleLoopCmd, #433). loop is nil
	// only when the top-level "loop" key was
	// entirely absent from the decoded JSON (a cenci binary that
	// predates this feature); in that case loopErr holds a guard message.
	loop    *cenciwatch.DispatchState
	loopErr string

	// confirmingLoop is true while the modal is showing the two-step
	// confirmation prompt for a loop on/off toggle. The loop is a persistent,
	// fleet-wide daemon setting shared by every enrolled repo, so flipping it
	// is gated behind an explicit y/n confirm in BOTH directions (#433).
	confirmingLoop bool
}

// dispatchStatusMsg is sent when queryDispatchStatusCmd finishes querying
// cenci for the current repo's dispatch enrollment status. loop carries the
// CLI's "loop" object, decoded into the shared cenciwatch.DispatchState
// wire type (see dispatchState.loop).
type dispatchStatusMsg struct {
	repo     string
	dir      string
	enrolled bool
	loop     *cenciwatch.DispatchState
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

// dispatchLoopToggleMsg is sent when toggleLoopCmd finishes turning the
// fleet-wide dispatch loop on or off. Like dispatchEnrollMsg it only carries
// an exec-status error; the authoritative new loop state is obtained by a
// follow-up queryDispatchStatusCmd re-query, not from the toggle's stdout.
type dispatchLoopToggleMsg struct {
	err string
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
	// pendingSeq holds the canonical, space-separated keys typed so far of an
	// unfinished multi-key sequence (e.g. "P" while waiting for the "f" of
	// "P f") -- stored in keymap.Sequence.String()'s canonical form, not the
	// rune-concatenated "Pf". Since #489 this is the registry's own
	// pending-sequence state (keymap_dispatch.go's dispatchKey/
	// handlePendingSeqKey): built-ins and inline actions both participate,
	// on any key, upper or lower -- there is no separate uppercase-only gate
	// anymore. While non-empty, normal-mode/detail-focused key handling
	// routes every key to handlePendingSeqKey. pendingSeqAlt records whether
	// Alt was held on any key of the sequence, so Alt+prefix triggers
	// comment mode exactly like Alt on a single-key action.
	pendingSeq    string
	pendingSeqAlt bool
	// pendingRefs holds the selected card's references (body #N refs plus
	// the card's open blockers) while the nav.reference (default "g r")
	// which-key prompt is active (see handleReferenceNavKey/
	// handlePendingRefKey/cardReferences in references.go). It is a
	// dedicated state, not a reuse of pendingSeq, so reference labels
	// ('a'-'z') can never collide with or be swallowed by an unrelated
	// in-flight keymap sequence.
	pendingRefs       []cardRef
	refreshing        bool
	refreshInterval   time.Duration
	lastMetadataFetch time.Time
	metadataTTL       time.Duration
	prevCards         map[int]prevCardInfo
	// cleanupBreakerWarning holds a status-bar warning set by
	// detectDepartures when the cleanup circuit breaker trips. It's a
	// transient hand-off: handleBoardFetched applies it as the timed status
	// message right after "Board refreshed"/"Filter has no matches" (which
	// would otherwise clobber it, since SetTimedMessage mutates the status
	// bar synchronously), then clears it. Empty means no trip occurred.
	cleanupBreakerWarning string
	// startupWarning holds a status-bar warning seeded from Config.Notices at
	// startup (e.g. an untrusted-local-config strip notice, #568). It's a
	// one-shot transient hand-off mirroring cleanupBreakerWarning: consumed
	// as a timed status message on the first successful board fetch
	// (handleBoardFetched, update.go), then cleared. Empty means nothing to
	// show.
	startupWarning              string
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
	// tmuxSession is the tmux session name this lazyboards instance runs in,
	// resolved once at startup (empty when not inside tmux). The agents list is
	// scoped to it so an instance surfaces only the agents in its own session
	// (#410); empty means "not in tmux", which disables the scoping.
	tmuxSession   string
	gitReader     gitdetect.Reader
	gitPanel      gitPanelState
	prList        prListState
	milestoneList milestoneListState
	agentList     agentListState
	dispatch      dispatchState
	// openPRCount is the repo-wide open-PR total shown by the status-bar PR
	// indicator, updated by every successful ListOpenPRs result (board fetch
	// cycles and the v modal's fetch). -1 is the "never fetched" sentinel:
	// prIndicatorCount falls back to the card-linked sum until the first
	// successful listing, mirroring prListState's fallback precedence.
	openPRCount int
	// sortNewestFirst controls the board-wide card sort order applied by
	// sortColumns: true sorts every column newest-created-first, false
	// oldest-first (the default, #503). Toggled at runtime by the 'u' key
	// (#412) and seeded at startup by config.ResolveSortNewestFirst.
	sortNewestFirst bool
	// statePath is the runtime-state file the 'u' toggle persists the sort
	// direction to (config.DefaultStatePath, #503). Empty means "nowhere to
	// save": toggling still works, it just won't survive a restart.
	statePath string
	// trustPath is the resolved trust-store file path (config.DefaultTrustPath,
	// #568), threaded into saveConfigCmd so an in-app config Save() carries
	// trust forward across the local config file rewrite (config.Save's
	// carry-forward). Empty means no trust store is available to update.
	trustPath string
	// updateCheckEnabled mirrors config.Config.UpdateCheckValue(): whether
	// Init() should kick off the startup version-update check (#444).
	updateCheckEnabled bool
	// keys is the active, immutable, resolved keymap that dispatchKey/
	// handlePendingSeqKey/registryHints consult (#489). *keymap.Keymap never
	// mutates after keymap.Resolve constructs it, so it is safe to share
	// across Board's value copies without a deep copy. NewBoard seeds it with
	// the built-in defaults (config.DefaultKeymap); main.go layers the loaded
	// config's own keymaps: on top once at startup, with the fully resolved
	// config.ResolveKeymap result via withKeymap.
	keys *keymap.Keymap
	// providerFactory builds a BoardProvider for a provider name and an
	// owner/repo pair. Providers are bound to one repository at construction,
	// so retargeting the board at a repo saved in the config modal requires
	// building a new one; main.go injects this after NewBoard (like statePath
	// and tmuxSession) so the factory can reuse the already-authenticated API
	// clients. Nil means "cannot retarget at runtime": a repo change then
	// reports that rather than silently refreshing the previous repo.
	providerFactory func(providerName, owner, repo string) (provider.BoardProvider, error)
}

// splitRepo splits a "owner/repo" identifier into its two halves. ok is false
// unless both halves are present and non-empty.
func splitRepo(repo string) (owner, name string, ok bool) {
	parts := strings.SplitN(repo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// NewBoard creates a Board in loadingMode (or configMode if firstLaunch).
// Call Init() to start fetching data.
func NewBoard(p provider.BoardProvider, defaultActions map[string]config.Action, columnConfigs []config.ColumnConfig, executor action.Executor, repoOwner, repoName, providerName string, sessionMaxLen int, refreshInterval time.Duration, workingLabel string, mouseEnabled bool, firstLaunch bool, watcher cenciwatch.Watcher, gitReader gitdetect.Reader, updateCheckEnabled bool) Board {
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

	// Seed the active keymap with the built-in defaults. main.go layers the
	// loaded config's keymaps: on top via withKeymap once config.Load() has
	// produced it; a Board built without that step (tests, the first-launch
	// config modal) still dispatches every built-in command.
	keys, err := config.DefaultKeymap()
	if err != nil {
		// Unreachable in practice: the built-in default tables always
		// resolve against themselves. Log rather than leave keys nil.
		debuglog.Errorf("keymap: failed to resolve the built-in default tables: %v", err)
	}

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
		defaultActions:     defaultActions,
		columnConfigs:      columnConfigs,
		executor:           executor,
		repoOwner:          repoOwner,
		repoName:           repoName,
		providerName:       providerName,
		sessionMaxLen:      sessionMaxLen,
		refreshInterval:    refreshInterval,
		metadataTTL:        metadataTTL,
		workingLabel:       workingLabel,
		mouseEnabled:       mouseEnabled,
		cenciWatcher:       watcher,
		gitReader:          gitReader,
		openPRCount:        -1,
		updateCheckEnabled: updateCheckEnabled,
		keys:               keys,
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

	// Card-scope hints are omitted because no columns/cards are loaded yet;
	// rebuildNormalHints adds them after the first board fetch.
	b.rebuildNormalHints()
	b.statusBar = NewStatusBar(b.normalHints)

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

// enterGitPanel opens the git menu modal, populating its items from the
// ModeGitPanel registry table (keymap_panels.go's gitPanelItemsFromKeymap).
// b.defaultActions (config.DefaultGitActions() when the working tree is a
// git repo, nil/empty otherwise) is consulted only as the "is this a git
// repo" availability gate -- it no longer supplies the item list itself, so
// if no default git actions are available this is a no-op and the panel
// does not open.
func (b *Board) enterGitPanel() {
	if len(b.defaultActions) == 0 {
		return
	}

	b.gitPanel = gitPanelState{items: b.gitPanelItemsFromKeymap(), cursor: 0}
	b.mode = gitPanelMode
	b.statusBar.SetActionHints(b.gitPanelHints())
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
				if pr.State == "CLOSED" || pr.State == "MERGED" {
					continue
				}
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
	b.statusBar.SetActionHints(b.prListHints())
}

// enterMilestoneList opens the repo-wide Milestones modal (i). Unlike
// enterPRList there is no card-linked fallback to seed: entries starts nil
// and stays that way until fetchMilestonesCmd's result lands (see
// milestoneListState's doc comment). It always opens, even before any
// milestones are known, so the modal can render its loading state.
func (b *Board) enterMilestoneList() {
	generation := b.milestoneList.generation + 1
	b.milestoneList = milestoneListState{entries: nil, cursor: 0, loading: true, generation: generation}
	b.mode = milestoneListMode
	b.statusBar.SetActionHints(b.milestoneListHints())
}

// enterAgentList opens the global agents list modal. Rows are derived live
// from the stored cenci-watch snapshot (agentListEntries), so unlike
// enterPRList there is no fetch to start and no generation to track. It
// always opens, even with no watcher or snapshot, so the modal can render its
// unavailable/empty states.
func (b *Board) enterAgentList() {
	b.enterAgentListScoped(0)
}

// enterAgentListForCard opens the agents list modal scoped to the given
// card's windows — the multi-window case of the s jump.
func (b *Board) enterAgentListForCard(number int) {
	b.enterAgentListScoped(number)
}

func (b *Board) enterAgentListScoped(cardNumber int) {
	b.agentList = agentListState{cursor: 0, cardNumber: cardNumber}
	b.mode = agentListMode
	hints := b.agentListHints()
	if len(b.agentListEntries()) == 0 {
		hints = b.agentListEmptyHints()
	}
	b.statusBar.SetActionHints(hints)
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

// rebuildNormalHints reconstructs b.normalHints from the active keymap: the
// curated built-in specs (help/new/edit) plus the scope-gated, config-order
// inline-action hints, both derived by registryHints (keymap_dispatch.go)
// from b.keys so a remap/unbind is reflected automatically (#489).
func (b *Board) rebuildNormalHints() {
	b.normalHints = b.registryHints(keymap.ModeNormal)
}

// rebuildDetailHints reconstructs and applies the status bar hints shown
// when the detail panel is focused, via the same registry-derived builder
// rebuildNormalHints uses (registryHints, keymap_dispatch.go) so the
// detail-focused bar carries the same "?" help pointer and scope-gated
// custom-action merge the card-list bar does. Unlike normalHints, the
// result isn't cached on Board: every call site that (re-)enters detail
// focus needs a fresh rebuild anyway (the gating depends on the selected
// card/column, which can change while unfocused), so there's no separate
// "restore the last-built hints" caller to justify a field.
func (b *Board) rebuildDetailHints() {
	b.statusBar.SetActionHints(b.registryHints(keymap.ModeDetail))
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

// sortFold stably sorts s in place by the case-folded (strings.ToLower) key
// returned by key, so entries differing only by case keep their original
// relative order (deterministic display ordering, see #477).
func sortFold[T any](s []T, key func(T) string) {
	sort.SliceStable(s, func(i, j int) bool {
		return strings.ToLower(key(s[i])) < strings.ToLower(key(s[j]))
	})
}

// sortFoldStrings stably sorts a []string case-insensitively in place.
func sortFoldStrings(s []string) {
	sortFold(s, func(v string) string { return v })
}

func mapLinkedPRs(prs []provider.LinkedPR) []LinkedPR {
	return mapSlice(prs, func(pr provider.LinkedPR) LinkedPR {
		return LinkedPR{
			Number:           pr.Number,
			Title:            pr.Title,
			URL:              pr.URL,
			Branch:           pr.Branch,
			IsDraft:          pr.IsDraft,
			Mergeable:        pr.Mergeable,
			MergeStateStatus: pr.MergeStateStatus,
			State:            pr.State,
		}
	})
}

// mapBlockers converts provider-layer blockers to app-layer Blocker values,
// mirroring the LinkedPR/mapLinkedPRs convention.
func mapBlockers(blockers []provider.Blocker) []Blocker {
	return mapSlice(blockers, func(bl provider.Blocker) Blocker {
		return Blocker{
			Number:            bl.Number,
			State:             bl.State,
			URL:               bl.URL,
			RepoNameWithOwner: bl.RepoNameWithOwner,
		}
	})
}

// mapMilestones converts provider-layer milestones to app-layer Milestone
// values, mirroring the LinkedPR/mapLinkedPRs convention.
func mapMilestones(milestones []provider.Milestone) []Milestone {
	return mapSlice(milestones, func(m provider.Milestone) Milestone {
		return Milestone{
			Title:              m.Title,
			URL:                m.URL,
			DueOn:              m.DueOn,
			OpenIssueCount:     m.OpenIssueCount,
			ClosedIssueCount:   m.ClosedIssueCount,
			ProgressPercentage: m.ProgressPercentage,
		}
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
		Number:            c.Number,
		Title:             c.Title,
		Labels:            mapLabels(c.Labels),
		Body:              c.Body,
		URL:               c.URL,
		LinkedPRs:         mapLinkedPRs(c.LinkedPRs),
		Assignees:         mapAssignees(c.Assignees),
		Milestone:         c.Milestone,
		CreatedAt:         c.CreatedAt,
		ParentNumber:      c.ParentNumber,
		SubIssueCount:     c.SubIssueCount,
		SubIssueCompleted: c.SubIssueCompleted,

		BlockedByCount:      c.BlockedByCount,
		TotalBlockedByCount: c.TotalBlockedByCount,
		BlockingCount:       c.BlockingCount,
		TotalBlockingCount:  c.TotalBlockingCount,
		Blockers:            mapBlockers(c.Blockers),
	}
}

// sortColumns reorders every column's Cards slice by CreatedAt, direction
// controlled by b.sortNewestFirst (#412). Uses sort.SliceStable so cards with
// equal (or zero) CreatedAt values keep the provider's original order. This
// is a pure reorder -- it never touches any column's Cursor; callers that
// invoke it while a cursor should track a specific card must resolve and
// restore that cursor themselves (see docs/list-cursor-invariants.md).
func (b *Board) sortColumns() {
	for i := range b.Columns {
		cards := b.Columns[i].Cards
		sort.SliceStable(cards, func(x, y int) bool {
			if b.sortNewestFirst {
				return cards[x].CreatedAt.After(cards[y].CreatedAt)
			}
			return cards[x].CreatedAt.Before(cards[y].CreatedAt)
		})
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
	case filterByMilestone:
		if card.Milestone == "" {
			return false
		}
		return strings.EqualFold(card.Milestone, b.activeFilterValue)
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

// applyFilter is the single choke point for applying a global filter
// (per docs/list-cursor-invariants.md): it sets the active filter fields and
// clamps the active column's cursor/scroll to the newly filtered card count.
// The active-tab guard exists for future repo-derived callers that may
// invoke this on a board with no columns yet (e.g. before the first fetch).
func (b *Board) applyFilter(itemType filterType, value string) {
	b.activeFilterType = itemType
	b.activeFilterValue = value
	if len(b.Columns) == 0 || b.ActiveTab < 0 || b.ActiveTab >= len(b.Columns) {
		return
	}
	filtered := b.filteredCards()
	col := &b.Columns[b.ActiveTab]
	if len(filtered) == 0 {
		col.Cursor = 0
	} else if col.Cursor >= len(filtered) {
		col.Cursor = len(filtered) - 1
	}
	col.ScrollOffset = 0
	b.clampScrollOffset()
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

// collectFilterItems scans all columns for unique labels, assignees, and
// milestones, returning a list of filterItems with section headers.
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

	// Collect unique milestones (case-insensitive dedup), skipping empty values.
	milestoneSeen := make(map[string]bool)
	var milestones []string
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			if card.Milestone == "" {
				continue
			}
			lower := strings.ToLower(card.Milestone)
			if !milestoneSeen[lower] {
				milestoneSeen[lower] = true
				milestones = append(milestones, card.Milestone)
			}
		}
	}

	if len(labels) == 0 && len(assignees) == 0 && len(milestones) == 0 {
		return nil
	}

	sortFoldStrings(labels)
	sortFoldStrings(assignees)
	sortFoldStrings(milestones)

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

	if len(milestones) > 0 {
		items = append(items, filterItem{isHeader: true, value: "Milestones"})
		for _, name := range milestones {
			items = append(items, filterItem{itemType: filterByMilestone, value: name})
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

// agentListEntries derives the agents list modal rows from the stored
// snapshot: every tracked window in snapshot order — matched to a card or not
// — annotated with the board card its name joins to. The join is the inverse
// of agentStatusForNumber's rule (window name "<number>" or "<number>-..."),
// so the modal and the card badges never disagree about which card a window
// belongs to.
func (b Board) agentListEntries() []agentListEntry {
	windows := b.sessionScopedWindows()
	entries := make([]agentListEntry, 0, len(windows))
	for _, w := range windows {
		num, joined := ticketNumberFromWindowName(w.WindowName)
		if b.agentList.cardNumber != 0 && (!joined || num != b.agentList.cardNumber) {
			continue
		}
		entry := agentListEntry{window: w}
		if joined {
			if ci, ii, found := b.findCard(num); found {
				entry.cardNumber = b.Columns[ci].Cards[ii].Number
				entry.columnTitle = b.Columns[ci].Title
			}
		}
		entries = append(entries, entry)
	}
	return entries
}

// cardAgentWindows returns every session-scoped window whose name joins to the
// given ticket number, in snapshot order — the same join rule as
// agentStatusForNumber, which returns only the single "best" window for the
// card badge. Scoping matches agentListEntries so the s jump acts on exactly
// the windows the agents modal would list.
func (b Board) cardAgentWindows(number int) []cenciwatch.WindowState {
	var windows []cenciwatch.WindowState
	for _, w := range b.sessionScopedWindows() {
		if n, ok := ticketNumberFromWindowName(w.WindowName); ok && n == number {
			windows = append(windows, w)
		}
	}
	return windows
}

// sessionScopedWindows returns the snapshot windows this lazyboards instance
// surfaces in its agents list: only those in the same tmux session as the
// instance itself (#410). When the instance's own session is unknown — it is
// not running inside tmux — there is no "same session" to scope to, so every
// tracked window is returned. Returns nil when no snapshot is stored yet.
func (b Board) sessionScopedWindows() []cenciwatch.WindowState {
	if b.agentSnapshot == nil {
		return nil
	}
	if b.tmuxSession == "" {
		return b.agentSnapshot.Windows
	}
	scoped := make([]cenciwatch.WindowState, 0, len(b.agentSnapshot.Windows))
	for _, w := range b.agentSnapshot.Windows {
		if w.Session == b.tmuxSession {
			scoped = append(scoped, w)
		}
	}
	return scoped
}

// ticketNumberFromWindowName parses the ticket number a window name joins to:
// the whole name ("42") or the segment before the first "-" ("42-implement").
// Reports false for non-numeric names, mirroring agentStatusForNumber's
// boundary rule so "420-x" never joins ticket #42. The round-trip check
// rejects non-canonical spellings Atoi would accept ("007", "+7"): they fail
// agentStatusForNumber's exact string match, so accepting them here would
// make the modal claim a card the badge join disagrees with.
func ticketNumberFromWindowName(name string) (int, bool) {
	num := name
	if i := strings.IndexByte(name, '-'); i >= 0 {
		num = name[:i]
	}
	n, err := strconv.Atoi(num)
	if err != nil || n <= 0 || strconv.Itoa(n) != num {
		return 0, false
	}
	return n, true
}

// switchToAgentWindow points tmux at the given agent window: select-window
// makes it the session's current window (this works even when that session is
// not attached), then switch-client moves the running client to the session.
// Both targets are shell-escaped — session and window identifiers originate
// outside the app (tmux state via the cenci-watch daemon) and are untrusted
// (docs/shell-and-url-safety.md). On failure tmux's stderr is returned as the
// error when present, since it names the actual problem (e.g. "no current
// client" when running outside tmux).
func (b Board) switchToAgentWindow(w cenciwatch.WindowState) error {
	target := action.ShellEscape(w.Session + ":" + w.WindowIndex)
	session := action.ShellEscape(w.Session)
	stderr, err := b.executor.RunShell("tmux select-window -t " + target + " && tmux switch-client -t " + session)
	if err != nil {
		if msg := strings.TrimSpace(stderr); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// agentWindowRef is the "session:index" reference shown for a window in the
// agents modal, so agents are identifiable by their tmux location (matching
// cenci's own status output). Missing pieces degrade gracefully: a paneless
// window with no tmux session/index yields "".
func agentWindowRef(w cenciwatch.WindowState) string {
	switch {
	case w.Session != "" && w.WindowIndex != "":
		return w.Session + ":" + w.WindowIndex
	case w.Session != "":
		return w.Session
	default:
		return w.WindowIndex
	}
}

// agentCounts returns how many live agent windows are in each of the six
// states the cenci-watch daemon reports (running, need-input, done, failed,
// stopped, idle). It iterates sessionScopedWindows() — the same session scope
// as agentListEntries (#410) — so the status-bar tally always matches exactly
// what the agents modal lists: a window in a different tmux session than this
// lazyboards instance is excluded, matched to a board card or not. When no
// snapshot is stored (cenci off/absent), all six counts are naturally zero.
func (b Board) agentCounts() (running, needInput, done, failed, stopped, idle int) {
	for _, w := range b.sessionScopedWindows() {
		switch w.Status {
		case agentStatusRunning:
			running++
		case agentStatusNeedInput:
			needInput++
		case "done":
			done++
		case agentStatusFailed:
			failed++
		case "stopped":
			stopped++
		case "idle":
			idle++
		}
	}
	return
}

// prCounts sums the linked pull requests across every card in every column —
// the card-linked fallback prIndicatorCount shows until a repo-wide open-PR
// listing succeeds. It is a raw count of linked PRs with no open/merged/closed
// filtering: LinkedPR now carries a State field (see enterPRList, which does
// filter it for the PR list modal fallback), but filtering prCounts's
// aggregate by state is out of scope for this ticket (#449).
func (b Board) prCounts() int {
	total := 0
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			total += len(card.LinkedPRs)
		}
	}
	return total
}

// prIndicatorCount returns the count for the status-bar PR indicator: the
// repo-wide open-PR total (the same population the v modal lists) once any
// ListOpenPRs fetch has succeeded, falling back to the card-linked sum before
// that. This mirrors prListState's precedence, where card-linked entries are
// the fallback until the repo-wide listing arrives; a later failed listing
// keeps the last known total rather than reverting to the fallback.
func (b Board) prIndicatorCount() int {
	if b.openPRCount >= 0 {
		return b.openPRCount
	}
	return b.prCounts()
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
	if shouldCheckForUpdate(appVersion(), b.updateCheckEnabled) {
		// Always targets lazyboards' own repo, not the board's tracked repo
		// (b.repoOwner/b.repoName may be an entirely different project).
		cmd = tea.Batch(cmd, checkForUpdateCmd(lazyboardsRepoOwner, lazyboardsRepoName))
	}
	return cmd
}
