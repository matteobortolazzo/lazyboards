package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func (b Board) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearStatusMsg:
		b.statusBar.ClearMessage()
		return b, nil

	case updateCheckMsg:
		if msg.err == nil && versionNewer(appVersion(), msg.latest) {
			b.statusBar.SetStickyMessage(
				"Update available: "+appVersion()+" → "+msg.latest+" · run go install github.com/matteobortolazzo/lazyboards@latest",
				StatusInfo,
			)
		}
		return b, nil

	case refreshTickMsg:
		return b.handleRefreshTick()

	case agentSnapshotMsg:
		b.agentSnapshot = msg.snapshot
		// Reset the backoff to the zero sentinel so the ladder restarts at the
		// initial delay (1s) on the next error, not a doubled value.
		b.agentBackoff = 0
		// A successful read means the watcher is healthy again: reset the
		// consecutive-error counter so a future error is treated as the first
		// (tolerated) strike, not a continuation of a prior run of errors.
		b.cenciWatchConsecutiveErrors = 0
		b.statusBar.SetDispatchStatus(formatDispatchSegment(msg.snapshot.Dispatch))
		// Live snapshots can shrink the agents list while its modal is open;
		// clamp at the mutation site (docs/list-cursor-invariants.md) and
		// keep the hints in step with the empty/non-empty branch the view
		// renders (docs/view-state-consistency.md) — a list that empties
		// mid-open must stop hinting enter/j/k, and vice versa.
		if b.mode == agentListMode {
			n := len(b.agentListEntries())
			if b.agentList.cursor >= n {
				b.agentList.cursor = n - 1
				if b.agentList.cursor < 0 {
					b.agentList.cursor = 0
				}
			}
			hints := agentListModeHints
			if n == 0 {
				hints = agentListEmptyHints
			}
			b.statusBar.SetActionHints(hints)
		}
		if b.cenciWatcher == nil {
			return b, nil
		}
		return b, subscribeCenciWatchCmd(b.cenciWatcher)

	case cenciWatchErrorMsg:
		b.cenciWatchConsecutiveErrors++
		debuglog.Errorf("cenci: %v", msg.err)
		if b.cenciWatchConsecutiveErrors >= cenciWatchClearThreshold {
			b.statusBar.SetDispatchStatus("")
		}
		if b.agentBackoff <= 0 {
			b.agentBackoff = cenciWatchInitialBackoff
		} else {
			b.agentBackoff *= 2
			if b.agentBackoff > cenciWatchMaxBackoff {
				b.agentBackoff = cenciWatchMaxBackoff
			}
		}
		cmd := b.scheduleCenciWatchRetry()
		return b, cmd

	case cenciWatchRetryMsg:
		if b.cenciWatcher == nil {
			return b, nil
		}
		return b, subscribeCenciWatchCmd(b.cenciWatcher)

	case gitStatusMsg:
		if msg.err != nil {
			b.statusBar.SetGitStatus("")
			return b, nil
		}
		b.statusBar.SetGitStatus(formatGitSegment(msg.status))
		return b, nil

	case gitStatusTickMsg:
		if b.gitReader == nil {
			return b, nil
		}
		return b, tea.Batch(fetchGitStatusCmd(b.gitReader, "."), scheduleGitStatusTick(b))

	case boardFetchedMsg:
		return b.handleBoardFetched(msg)

	case openPRsMsg:
		return b.handleOpenPRsFetched(msg)

	case boardFetchErrorMsg:
		if b.refreshing {
			b.refreshing = false
			b.pendingAutoRefresh = false
			cmd := b.statusBar.SetTimedMessage("Refresh failed: "+provider.SanitizeError(msg.err), StatusError, statusMessageDuration)
			if tickCmd := b.scheduleRefreshTick(); tickCmd != nil {
				cmd = tea.Batch(cmd, tickCmd)
			}
			return b, cmd
		}
		b.mode = errorMode
		b.loadErr = provider.SanitizeError(msg.err)
		b.statusBar.SetActionHints([]Hint{
			{Key: "r", Desc: "Retry"},
			{Key: "q", Desc: "Quit"},
		})
		return b, nil

	case cardCreatedMsg:
		return b.handleCardCreated(msg)

	case cardCreateErrorMsg:
		b.validationErr = provider.SanitizeError(msg.err)
		b.mode = createMode
		b.recalcCreateInputs()
		cmd := b.create.titleInput.Focus()
		b.create.labelInput.Blur()
		return b, cmd

	case configSavedMsg:
		if b.config.firstLaunch {
			b.config.configSaved = true
			return b, tea.Quit
		}
		b.mode = loadingMode
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider, true))

	case configSaveErrorMsg:
		b.validationErr = provider.SanitizeError(msg.err)
		b.mode = configMode
		return b, nil

	case actionResultMsg:
		level := StatusSuccess
		if !msg.success {
			level = StatusError
		}
		cmd := b.statusBar.SetTimedMessage(msg.message, level, statusMessageDuration)
		if msg.success && b.actionRefreshDelay > 0 {
			b.pendingAutoRefresh = true
			cmd = tea.Batch(cmd, tea.Tick(b.actionRefreshDelay, func(time.Time) tea.Msg {
				return autoRefreshMsg{}
			}))
		}
		// Broad refresh (per plan Q2): re-read git status after every successful
		// action, not just actions tagged as git-related.
		if msg.success && b.gitReader != nil {
			cmd = tea.Batch(cmd, fetchGitStatusCmd(b.gitReader, "."))
		}
		return b, cmd

	case autoRefreshMsg:
		if !b.pendingAutoRefresh || b.refreshing {
			return b, nil
		}
		b.pendingAutoRefresh = false
		b.refreshing = true
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider, b.metadataDue()))

	case cleanupResultMsg:
		if msg.count == 0 {
			return b, nil
		}
		cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Cleaned up %d sessions", msg.count), StatusSuccess, statusMessageDuration)
		return b, cmd

	case dispatchStatusMsg:
		b.dispatch.loading = false
		if msg.err != "" {
			b.dispatch.err = msg.err
			return b, nil
		}
		b.dispatch.repo = msg.repo
		b.dispatch.dir = msg.dir
		b.dispatch.enrolled = msg.enrolled
		b.dispatch.err = ""
		b.dispatch.loop = msg.loop
		if msg.loop == nil {
			b.dispatch.loopErr = "cenci version too old for loop status — upgrade to use this feature"
		} else {
			b.dispatch.loopErr = ""
		}
		return b, nil

	case dispatchEnrollMsg:
		if msg.err != "" {
			b.dispatch.loading = false
			b.dispatch.err = msg.err
			return b, nil
		}
		// enroll/unenroll only reports exit status; re-query status to get the
		// authoritative enrolled state. Keep loading=true until that lands.
		return b, queryDispatchStatusCmd(b.executor)

	case dispatchRunMsg:
		b.dispatch.running = false
		if msg.err != "" {
			b.dispatch.err = msg.err
		} else {
			b.dispatch.lastResult = msg.result
			b.dispatch.lastLines = msg.lines
		}
		return b, nil

	case dispatchLoopToggleMsg:
		if msg.err != "" {
			b.dispatch.loading = false
			b.dispatch.err = msg.err
			return b, nil
		}
		// The toggle only reports exit status; re-query for the authoritative
		// loop state. Keep loading=true until that lands (mirrors dispatchEnrollMsg).
		return b, queryDispatchStatusCmd(b.executor)

	case spinner.TickMsg:
		if b.mode == loadingMode || b.mode == creatingMode || b.refreshing {
			var cmd tea.Cmd
			b.spinner, cmd = b.spinner.Update(msg)
			return b, cmd
		}
		return b, nil

	case editorFinishedMsg:
		return b.handleEditorFinished(msg)

	case cardUpdatedMsg:
		return b.handleCardUpdated(msg)

	case cardUpdateErrorMsg:
		cmd := b.statusBar.SetTimedMessage("Update error: "+provider.SanitizeError(msg.err), StatusError, statusMessageDuration)
		return b, cmd

	case labelCreatedMsg:
		return b.handleLabelCreated()

	case labelCreateErrorMsg:
		// An "already exists" result is benign: the label is present in the repo,
		// so treat it as a successful creation and continue the card update.
		if errors.Is(msg.err, provider.ErrLabelExists) {
			return b.handleLabelCreated()
		}
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Error: "+provider.SanitizeError(msg.err), StatusError, statusMessageDuration)
		return b, cmd

	case assigneesUpdatedMsg:
		return b.handleAssigneesUpdated(msg)

	case assigneesUpdateErrorMsg:
		cmd := b.statusBar.SetTimedMessage("Assign error: "+provider.SanitizeError(msg.err), StatusError, statusMessageDuration)
		return b, cmd

	case cardClosedMsg:
		return b.handleCardClosed(msg)

	case cardCloseErrorMsg:
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Close error: "+provider.SanitizeError(msg.err), StatusError, statusMessageDuration)
		return b, cmd

	case deleteCommentPostedMsg:
		return b, deleteCardCmd(b.provider, msg.card, true)

	case deleteCommentErrorMsg:
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Comment error: "+provider.SanitizeError(msg.err), StatusError, statusMessageDuration)
		return b, cmd

	case cardDeletedMsg:
		return b.handleCardDeleted(msg)

	case cardDeleteErrorMsg:
		b.mode = normalMode
		if msg.commentPosted {
			cmd := b.statusBar.SetTimedMessage("Comment posted, but delete failed: "+provider.SanitizeError(msg.err), StatusError, longStatusMessageDuration)
			return b, cmd
		}
		cmd := b.statusBar.SetTimedMessage("Delete error: "+provider.SanitizeError(msg.err), StatusError, statusMessageDuration)
		return b, cmd

	case tea.MouseMsg:
		if !b.mouseEnabled || b.mode != normalMode {
			return b, nil
		}
		return b.handleMouseMsg(msg)

	case tea.KeyMsg:
		// ctrl+c always quits regardless of mode.
		if msg.String() == "ctrl+c" {
			return b, tea.Quit
		}

		// Any keypress dismisses the sticky update-available notice, without
		// swallowing the key -- its own normal action still runs below.
		b.statusBar.ClearStickyMessage()

		switch b.mode {
		case loadingMode, creatingMode:
			return b, nil
		case errorMode:
			switch msg.String() {
			case "q":
				return b, tea.Quit
			case "r":
				b.mode = loadingMode
				b.loadErr = ""
				return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider, true))
			}
			return b, nil
		case createMode:
			return b.handleCreateModeKey(msg)
		case configMode:
			return b.handleConfigModeKey(msg)
		case prPickerMode:
			return b.handlePRPickerModeKey(msg)
		case searchMode:
			return b.handleSearchModeKey(msg)
		case helpMode:
			return b.handleHelpModeKey(msg)
		case labelConfirmMode:
			return b.handleLabelConfirmModeKey(msg)
		case closeConfirmMode:
			return b.handleCloseConfirmModeKey(msg)
		case commentMode:
			return b.handleCommentModeKey(msg)
		case deleteMode:
			return b.handleDeleteModeKey(msg)
		case filterMode:
			return b.handleFilterModeKey(msg)
		case assignMode:
			return b.handleAssignModeKey(msg)
		case gitPanelMode:
			return b.handleGitPanelKey(msg)
		case prListMode:
			return b.handlePRListModeKey(msg)
		case agentListMode:
			return b.handleAgentListModeKey(msg)
		case dispatchMode:
			return b.handleDispatchModeKey(msg)
		default:
			return b.handleNormalModeKey(msg)
		}

	case tea.WindowSizeMsg:
		b.Width = msg.Width
		b.Height = msg.Height
		var cmd tea.Cmd
		if b.mode == createMode {
			b.recalcCreateInputs()
			// Reset viewport after height change (see keystroke path comment).
			_ = b.create.titleInput.View()
			b.create.titleInput, cmd = b.create.titleInput.Update(nil)
		}
		if len(b.Columns) > 0 {
			b.clampScrollOffset()
		}
		return b, cmd
	}
	return b, nil
}

func (b Board) handleRefreshTick() (tea.Model, tea.Cmd) {
	if b.refreshInterval <= 0 {
		return b, nil
	}
	if b.mode != normalMode || b.refreshing {
		return b, b.scheduleRefreshTick()
	}
	b.refreshing = true
	return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider, b.metadataDue()))
}

func (b Board) scheduleRefreshTick() tea.Cmd {
	if b.refreshInterval <= 0 {
		return nil
	}
	return tea.Tick(b.refreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
	})
}

// scheduleCenciWatchRetry returns a tea.Cmd that fires an cenciWatchRetryMsg
// after the current backoff duration, so the watcher can be re-subscribed.
func (b Board) scheduleCenciWatchRetry() tea.Cmd {
	return tea.Tick(b.agentBackoff, func(time.Time) tea.Msg {
		return cenciWatchRetryMsg{}
	})
}

func (b Board) handleBoardFetched(msg boardFetchedMsg) (tea.Model, tea.Cmd) {
	// A refresh can change columns/cards, invalidating a pending key
	// sequence's candidates and its hint bar (which the rebuilt hints below
	// would clobber anyway) -- cancel it.
	b.clearPendingSeq()

	cols := make([]Column, len(msg.board.Columns))
	for i, pc := range msg.board.Columns {
		cards := make([]Card, len(pc.Cards))
		for j, c := range pc.Cards {
			cards[j] = mapProviderCard(c)
		}
		cols[i] = Column{Title: pc.Title, Cards: cards}
	}

	// Build new card position map and detect departures for cleanup.
	newCards := buildCardMap(cols)
	cleanupCmd := b.detectDepartures(newCards)
	b.prevCards = newCards

	// Store collaborators if provided (non-fatal error handling).
	if msg.collaboratorErr == nil && msg.collaborators != nil {
		b.collaborators = mapAssignees(msg.collaborators)
	}
	if msg.authenticatedUser != "" {
		b.authenticatedUser = msg.authenticatedUser
	}
	// Store the repo label set (non-fatal). Placed before the refreshing/
	// non-refreshing split so both paths retain it. Guarded on non-nil (like
	// collaborators above), not just labelErr == nil: when a fetch cycle
	// skips metadata (includeMetadata=false), msg.labelErr is nil by zero
	// value too, so without the non-nil check a metadata-skipped refresh
	// would wipe the previously-known label set.
	if msg.labelErr == nil && msg.repoLabels != nil {
		b.repoLabels = msg.repoLabels
	}
	if msg.metadataRequested {
		b.lastMetadataFetch = time.Now()
	}
	// Adopt the repo-wide open-PR total only from a successful listing
	// (openPRsFetched=true covers the legitimate empty list); on a failed or
	// absent fetch the previous count is kept, so the indicator degrades to
	// stale rather than wrong — the same non-fatal treatment as metadata.
	if msg.openPRsFetched {
		b.openPRCount = len(msg.openPRs)
	}

	b.pendingAutoRefresh = false

	if b.refreshing {
		// Preserve ActiveTab and cursor position by card Number (only used when no filter active).
		savedTab := b.ActiveTab
		savedNumber := -1
		if b.activeFilterType == filterTypeNone && savedTab < len(b.Columns) {
			oldCol := b.Columns[savedTab]
			if len(oldCol.Cards) > 0 && oldCol.Cursor < len(oldCol.Cards) {
				savedNumber = oldCol.Cards[oldCol.Cursor].Number
			}
		}

		b.Columns = cols
		b.sortColumns()
		b.refreshing = false
		b.detailScrollOffset = 0

		// Rebuild filter items from refreshed data (labels/assignees may have changed).
		b.filterItems = b.collectFilterItems()

		// Clamp ActiveTab if columns were reduced.
		if b.ActiveTab >= len(b.Columns) {
			b.ActiveTab = len(b.Columns) - 1
			if b.ActiveTab < 0 {
				b.ActiveTab = 0
			}
		}

		if b.activeFilterType != filterTypeNone {
			// When filter is active, reset cursor and scroll to top for all columns.
			for i := range b.Columns {
				b.Columns[i].Cursor = 0
				b.Columns[i].ScrollOffset = 0
			}
		} else {
			// Restore cursor by card Number in the active column.
			if b.ActiveTab < len(b.Columns) {
				col := &b.Columns[b.ActiveTab]
				found := false
				if savedNumber >= 0 {
					for i, card := range col.Cards {
						if card.Number == savedNumber {
							col.Cursor = i
							found = true
							break
						}
					}
				}
				if !found {
					// Clamp cursor to valid range.
					if col.Cursor >= len(col.Cards) {
						col.Cursor = len(col.Cards) - 1
						if col.Cursor < 0 {
							col.Cursor = 0
						}
					}
				}
			}
		}

		b.clampScrollOffset()
		b.rebuildNormalHints()
		if b.detailFocused {
			b.rebuildDetailHints()
		} else {
			b.statusBar.SetActionHints(b.normalHints)
		}

		// Show no-matches hint if filter is active and zero cards match across all columns.
		var cmd tea.Cmd
		if b.activeFilterType != filterTypeNone && b.totalFilteredCards() == 0 {
			cmd = b.statusBar.SetTimedMessage("Filter has no matches \u2014 press f to clear", StatusWarning, statusMessageDuration)
		} else {
			cmd = b.statusBar.SetTimedMessage("Board refreshed", StatusSuccess, statusMessageDuration)
		}
		if b.cleanupBreakerWarning != "" {
			// Applied after the refreshed/filter message above so it isn't
			// clobbered -- SetTimedMessage mutates the status bar synchronously.
			cmd = b.statusBar.SetTimedMessage(b.cleanupBreakerWarning, StatusWarning, statusMessageDuration)
			b.cleanupBreakerWarning = ""
		}
		if cleanupCmd != nil {
			cmd = tea.Batch(cmd, cleanupCmd)
		}
		if tickCmd := b.scheduleRefreshTick(); tickCmd != nil {
			cmd = tea.Batch(cmd, tickCmd)
		}
		if b.gitReader != nil {
			cmd = tea.Batch(cmd, fetchGitStatusCmd(b.gitReader, "."))
		}
		return b, cmd
	}

	b.Columns = cols
	b.sortColumns()
	b.mode = normalMode
	b.detailScrollOffset = 0
	b.detailFocused = false

	// Rebuild filter items from new data.
	b.filterItems = b.collectFilterItems()

	// Reset cursor/scroll for all columns when filter is active.
	if b.activeFilterType != filterTypeNone {
		for i := range b.Columns {
			b.Columns[i].Cursor = 0
			b.Columns[i].ScrollOffset = 0
		}
	}

	var cmd tea.Cmd
	b.rebuildNormalHints()
	b.statusBar.SetActionHints(b.normalHints)
	if b.loaded {
		if b.activeFilterType != filterTypeNone && b.totalFilteredCards() == 0 {
			cmd = b.statusBar.SetTimedMessage("Filter has no matches \u2014 press f to clear", StatusWarning, statusMessageDuration)
		} else {
			cmd = b.statusBar.SetTimedMessage("Board refreshed", StatusSuccess, statusMessageDuration)
		}
	}
	if b.cleanupBreakerWarning != "" {
		// Applied after the refreshed/filter message above so it isn't
		// clobbered -- SetTimedMessage mutates the status bar synchronously.
		cmd = b.statusBar.SetTimedMessage(b.cleanupBreakerWarning, StatusWarning, statusMessageDuration)
		b.cleanupBreakerWarning = ""
	}
	b.loaded = true
	if cleanupCmd != nil {
		cmd = tea.Batch(cmd, cleanupCmd)
	}
	if tickCmd := b.scheduleRefreshTick(); tickCmd != nil {
		cmd = tea.Batch(cmd, tickCmd)
	}
	if b.gitReader != nil {
		cmd = tea.Batch(cmd, fetchGitStatusCmd(b.gitReader, "."))
	}
	return b, cmd
}

// buildCardMap creates a map from card number to its column position and metadata.
func buildCardMap(cols []Column) map[int]prevCardInfo {
	m := make(map[int]prevCardInfo)
	for i, col := range cols {
		for _, card := range col.Cards {
			names := make([]string, len(card.Labels))
			for j, l := range card.Labels {
				names[j] = l.Name
			}
			m[card.Number] = prevCardInfo{
				colIdx: i,
				title:  card.Title,
				labels: names,
			}
		}
	}
	return m
}

// detectDepartures compares previous card positions with new positions and
// returns a tea.Cmd to run cleanup commands for cards that left their columns.
func (b *Board) detectDepartures(newCards map[int]prevCardInfo) tea.Cmd {
	if b.prevCards == nil || b.executor == nil {
		return nil
	}

	var commands []string
	for cardNum, prev := range b.prevCards {
		cleanup := b.columnCleanup(prev.colIdx)
		if cleanup == "" {
			continue
		}

		newInfo, exists := newCards[cardNum]
		if exists && newInfo.colIdx == prev.colIdx {
			continue // card stayed in same column
		}

		// Departure detected, but cleanup must never kill a window whose agent
		// is still working. Each guard defers instead of skipping: carrying the
		// prev entry into newCards (assigned to b.prevCards by the caller)
		// re-detects the same departure on the next fetch.

		// Guard A — cenci liveness: join by ticket number, so a title
		// rewrite (refine edits titles) can't hide a live agent's window.
		if b.agentSessionBusy(cardNum) {
			// A miss or move observed here still counts toward Guards C/D's
			// debounce, so cleanup runs on the first fetch after the agent
			// finishes rather than waiting an extra cycle once it's free.
			prev.missingSeen = prev.missingSeen || !exists
			prev.movedSeen = prev.movedSeen || (exists && newInfo.colIdx != prev.colIdx)
			newCards[cardNum] = prev
			continue
		}

		// Guard B — working label: marks an in-flight agent even when
		// cenci is off or its snapshot lags behind. Only reached when
		// exists && moved (the same-column case already continued above).
		if exists && b.hasWorkingLabel(newInfo.labels) {
			prev.movedSeen = true
			newCards[cardNum] = prev
			continue
		}

		// Guard C — missing-card debounce: a card can vanish from a single
		// fetch without leaving its column (e.g. pagination shifting while
		// issues close mid-fetch), so require two consecutive misses.
		if !exists && !prev.missingSeen {
			prev.missingSeen = true
			newCards[cardNum] = prev
			continue
		}

		// Guard D — moved-column debounce: a card can be misplaced by a
		// single bad fetch (e.g. a dropped-label fallback moving it to
		// column 0), so require the move to hold across two consecutive
		// fetches before cleanup fires. Guards A/B above accumulate
		// movedSeen too, so a liveness-deferred move still fires on the
		// first fetch after the agent becomes free, mirroring Guard C's
		// missing-card debounce semantics.
		if exists && !prev.movedSeen {
			prev.movedSeen = true
			newCards[cardNum] = prev
			continue
		}

		// Card departed — expand template and collect command.
		window := b.resolveWindowName(cardNum, prev.title)
		vars := action.BuildTemplateVars(cardNum, prev.title, prev.labels, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, "", window)
		expanded := action.ExpandTemplate(cleanup, action.BuildShellSafeVars(vars))
		commands = append(commands, expanded)
	}

	if len(commands) == 0 {
		return nil
	}

	trackedCount := len(b.prevCards)
	if cleanupCircuitBreakerTripped(len(commands), trackedCount) {
		// A single bad fetch that misplaces or drops many cards at once
		// (e.g. a label-fetch failure resetting columns to the fallback) is
		// far more likely than that many agents genuinely finishing in the
		// same refresh cycle. Skip every cleanup for this fetch rather than
		// firing any of them -- intentionally conservative: these cards
		// already passed the move/missing debounce and now sit at their new
		// position in newCards, so they won't automatically re-trigger
		// cleanup unless they move again. The warning is applied by the
		// caller, not here -- see cleanupBreakerWarning's doc comment.
		debuglog.Errorf("cleanup circuit breaker tripped: %d cleanups for %d tracked cards", len(commands), trackedCount)
		b.cleanupBreakerWarning = fmt.Sprintf("Cleanup skipped: %d cleanups looked implausible for one fetch", len(commands))
		return nil
	}

	return runCleanupCmds(b.executor, commands)
}

// cleanupCircuitBreakerMinCount and cleanupCircuitBreakerFraction are
// hardcoded (no config surface): a fetch with this many cleanup commands, or
// this fraction of all tracked cards, is treated as an implausible mass
// departure rather than genuine agent completions.
const (
	cleanupCircuitBreakerMinCount = 5
	cleanupCircuitBreakerFraction = 0.5
)

// cleanupCircuitBreakerTripped reports whether cmdCount would-fire cleanup
// commands for a single fetch is implausible given trackedCount tracked
// cards: either the absolute floor is met, or cmdCount exceeds half of all
// tracked cards on a smaller board.
func cleanupCircuitBreakerTripped(cmdCount, trackedCount int) bool {
	if cmdCount >= cleanupCircuitBreakerMinCount {
		return true
	}
	return trackedCount > 0 && float64(cmdCount) > cleanupCircuitBreakerFraction*float64(trackedCount)
}

// agentSessionBusy reports whether the cenci window joined to the card
// number has an agent that is running or waiting for input. Fails closed
// (reports busy) when cenci is enabled but no snapshot has been
// delivered yet -- daemon down/restarting, or a startup race -- so cleanup
// never fires against stale "not busy" information. Always false when
// cenci is off/absent (no watcher configured).
func (b *Board) agentSessionBusy(cardNum int) bool {
	if b.cenciWatcher != nil && b.agentSnapshot == nil {
		return true
	}
	ws := b.agentStatusForNumber(cardNum)
	if ws == nil {
		return false
	}
	return ws.Status == agentStatusRunning || ws.Status == agentStatusNeedInput
}

// resolveWindowName resolves the {window} template variable: the live
// cenci window name joined to cardNum by ticket-number prefix (see
// agentStatusForNumber), falling back to the {session} value when no
// snapshot is stored or no window matches.
func (b Board) resolveWindowName(cardNum int, title string) string {
	if ws := b.agentStatusForNumber(cardNum); ws != nil && ws.WindowName != "" {
		return ws.WindowName
	}
	return action.BuildSessionName(cardNum, title, b.sessionMaxLen)
}

// hasWorkingLabel reports whether any label matches the configured working
// label (case-insensitive). Always false when no working label is configured.
func (b *Board) hasWorkingLabel(labels []string) bool {
	if b.workingLabel == "" {
		return false
	}
	for _, l := range labels {
		if strings.EqualFold(l, b.workingLabel) {
			return true
		}
	}
	return false
}

// findCard searches all columns, in column then card order, for a card whose
// Number matches. Returns the column and card index of the first match found
// and ok=true, or ok=false if no card has that number.
func (b *Board) findCard(number int) (colIdx, cardIdx int, ok bool) {
	for ci := range b.Columns {
		for i := range b.Columns[ci].Cards {
			if b.Columns[ci].Cards[i].Number == number {
				return ci, i, true
			}
		}
	}
	return 0, 0, false
}

// columnCleanup returns the cleanup command for the column at colIdx, matched by title.
func (b *Board) columnCleanup(colIdx int) string {
	if colIdx >= len(b.Columns) {
		return ""
	}
	colTitle := b.Columns[colIdx].Title
	for _, cc := range b.columnConfigs {
		if strings.EqualFold(cc.Name, colTitle) {
			return cc.CleanupValue()
		}
	}
	return ""
}

// handleOpenPRsFetched applies the repo-wide open-PR fetch result to the PR
// list modal, following the prListState precedence (loading -> err ->
// loaded). Results are scoped to a modal generation, so a response that lands
// after the user closes or reopens the modal cannot overwrite the current
// request. On success, entries are replaced with the repo-wide list in
// provider order, each PR annotated with the first board card that links it
// (cardNumber 0 marks an unlinked PR); on error, the card-linked fallback
// entries built by enterPRList are kept.
func (b Board) handleOpenPRsFetched(msg openPRsMsg) (tea.Model, tea.Cmd) {
	// A successful current-generation listing is fresh repo-wide data whether
	// or not the modal is still open, so the status-bar PR indicator adopts
	// its total before the modal-state guard below. Stale generations (an
	// older request superseded by a newer one) are still dropped: they could
	// arrive out of order and roll the count backwards.
	if msg.err == nil && msg.generation == b.prList.generation {
		b.openPRCount = len(msg.prs)
	}
	if b.mode != prListMode || msg.generation != b.prList.generation {
		return b, nil
	}
	b.prList.loading = false
	if msg.err != nil {
		b.prList.err = provider.SanitizeError(msg.err)
		return b, nil
	}

	type cardRef struct {
		cardNumber  int
		columnTitle string
	}
	refs := make(map[int]cardRef)
	for _, col := range b.Columns {
		for _, card := range col.Cards {
			for _, pr := range card.LinkedPRs {
				if _, ok := refs[pr.Number]; !ok {
					refs[pr.Number] = cardRef{cardNumber: card.Number, columnTitle: col.Title}
				}
			}
		}
	}

	entries := make([]prListEntry, 0, len(msg.prs))
	for _, pr := range mapLinkedPRs(msg.prs) {
		ref := refs[pr.Number]
		entries = append(entries, prListEntry{
			pr:          pr,
			cardNumber:  ref.cardNumber,
			columnTitle: ref.columnTitle,
		})
	}
	b.prList.entries = entries
	if b.prList.cursor >= len(entries) {
		b.prList.cursor = len(entries) - 1
		if b.prList.cursor < 0 {
			b.prList.cursor = 0
		}
	}
	return b, nil
}

func (b Board) handleCardCreated(msg cardCreatedMsg) (tea.Model, tea.Cmd) {
	const targetCol = 0 // create mode has no column picker; new cards always land here
	b.Columns[targetCol].Cards = append(b.Columns[targetCol].Cards, mapProviderCard(msg.card))
	b.create.titleInput.SetValue("")
	b.create.labelInput.SetValue("")
	b.validationErr = ""
	b.mode = normalMode

	// Focus the newly created card: switch to its column, drop any active
	// search/filter that could hide it, then select it explicitly -- this
	// runs after clearSearch/clearFilter because they reset the column
	// cursor to 0, which we then override to point at the appended card.
	b.ActiveTab = targetCol
	b.clearSearch()
	b.clearFilter()
	col := &b.Columns[targetCol]
	col.Cursor = len(col.Cards) - 1
	b.onCursorMoved()

	var cmd tea.Cmd
	if b.create.pendingAssignee != "" {
		cmd = tea.Batch(
			b.statusBar.SetTimedMessage("Setting assignee...", StatusInfo, longStatusMessageDuration),
			setAssigneesCmd(b.provider, msg.card.Number, []string{b.create.pendingAssignee}),
		)
		b.create.pendingAssignee = ""
	}
	return b, cmd
}

func (b Board) handleEditorFinished(msg editorFinishedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		cmd := b.statusBar.SetTimedMessage("Error: "+msg.err.Error(), StatusError, statusMessageDuration)
		return b, cmd
	}
	if msg.editedContent == "" || msg.editedContent == msg.originalContent {
		cmd := b.statusBar.SetTimedMessage("Edit cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	title, labels, body, err := parseFrontmatter(msg.editedContent)
	if err != nil {
		cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
		return b, cmd
	}

	// Check for unknown labels.
	known := b.collectKnownLabels()
	var unknownLabels []string
	for _, l := range labels {
		if !known[strings.ToLower(l)] {
			unknownLabels = append(unknownLabels, l)
		}
	}

	if len(unknownLabels) > 0 {
		b.mode = labelConfirmMode
		b.labelConfirm = labelConfirmState{
			card:          msg.card,
			title:         title,
			body:          body,
			allLabels:     labels,
			unknownLabels: unknownLabels,
			currentIdx:    0,
		}
		return b, nil
	}

	return b, updateCardCmd(b.provider, msg.card.Number, title, body, labels)
}

// handleCardDeleted removes the deleted card from its column and, unless a
// cleanup guard blocks it (agent busy or working label present), fires the
// column's cleanup command immediately -- no debounce, mirroring
// handleCardClosed's full guard precedence in full (#174/#234 lessons). The
// prevCards entry is unconditionally deleted regardless of guard outcome.
func (b Board) handleCardDeleted(msg cardDeletedMsg) (tea.Model, tea.Cmd) {
	b.clearPendingSeq()

	cardNum := msg.card.Number
	labelNames := make([]string, len(msg.card.Labels))
	for j, l := range msg.card.Labels {
		labelNames[j] = l.Name
	}

	var cleanupCmd tea.Cmd
	if ci, i, ok := b.findCard(cardNum); ok {
		cleanup := b.columnCleanup(ci)
		if cleanup != "" && b.executor != nil && !b.agentSessionBusy(cardNum) && !b.hasWorkingLabel(labelNames) {
			window := b.resolveWindowName(cardNum, msg.card.Title)
			vars := action.BuildTemplateVars(cardNum, msg.card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, "", window)
			expanded := action.ExpandTemplate(cleanup, action.BuildShellSafeVars(vars))
			cleanupCmd = runCleanupCmds(b.executor, []string{expanded})
		}
		b.Columns[ci].Cards = append(b.Columns[ci].Cards[:i], b.Columns[ci].Cards[i+1:]...)
		if b.Columns[ci].Cursor >= len(b.Columns[ci].Cards) {
			b.Columns[ci].Cursor = len(b.Columns[ci].Cards) - 1
			if b.Columns[ci].Cursor < 0 {
				b.Columns[ci].Cursor = 0
			}
		}
	}

	delete(b.prevCards, cardNum)

	b.clampScrollOffset()
	b.rebuildNormalHints()
	cmd := b.statusBar.SetTimedMessage("Card deleted", StatusSuccess, statusMessageDuration)
	if cleanupCmd != nil {
		cmd = tea.Batch(cmd, cleanupCmd)
	}
	return b, cmd
}

// handleCardClosed removes the closed card from its column and, unless a
// cleanup guard blocks it (agent busy or working label present), fires the
// column's cleanup command immediately -- no debounce, unlike detectDepartures'
// background-fetch departure detection. The prevCards entry is unconditionally
// deleted regardless of guard outcome (locked decision #347 Q2: a guard-blocked
// close always deletes, it never defers).
func (b Board) handleCardClosed(msg cardClosedMsg) (tea.Model, tea.Cmd) {
	b.clearPendingSeq()

	cardNum := msg.card.Number
	labelNames := make([]string, len(msg.card.Labels))
	for j, l := range msg.card.Labels {
		labelNames[j] = l.Name
	}

	var cleanupCmd tea.Cmd
	if ci, i, ok := b.findCard(cardNum); ok {
		cleanup := b.columnCleanup(ci)
		if cleanup != "" && b.executor != nil && !b.agentSessionBusy(cardNum) && !b.hasWorkingLabel(labelNames) {
			window := b.resolveWindowName(cardNum, msg.card.Title)
			vars := action.BuildTemplateVars(cardNum, msg.card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, "", window)
			expanded := action.ExpandTemplate(cleanup, action.BuildShellSafeVars(vars))
			cleanupCmd = runCleanupCmds(b.executor, []string{expanded})
		}
		b.Columns[ci].Cards = append(b.Columns[ci].Cards[:i], b.Columns[ci].Cards[i+1:]...)
		if b.Columns[ci].Cursor >= len(b.Columns[ci].Cards) {
			b.Columns[ci].Cursor = len(b.Columns[ci].Cards) - 1
			if b.Columns[ci].Cursor < 0 {
				b.Columns[ci].Cursor = 0
			}
		}
	}

	delete(b.prevCards, cardNum)

	b.clampScrollOffset()
	b.rebuildNormalHints()
	cmd := b.statusBar.SetTimedMessage("Card closed", StatusSuccess, statusMessageDuration)
	if cleanupCmd != nil {
		cmd = tea.Batch(cmd, cleanupCmd)
	}
	return b, cmd
}

func (b Board) handleLabelCreated() (tea.Model, tea.Cmd) {
	b.labelConfirm.currentIdx++
	if b.labelConfirm.currentIdx < len(b.labelConfirm.unknownLabels) {
		// More unknown labels to confirm.
		return b, nil
	}
	// All labels created, proceed with update.
	b.mode = normalMode
	lc := b.labelConfirm
	return b, updateCardCmd(b.provider, lc.card.Number, lc.title, lc.body, lc.allLabels)
}

func (b Board) handleCardUpdated(msg cardUpdatedMsg) (tea.Model, tea.Cmd) {
	if ci, i, ok := b.findCard(msg.card.Number); ok {
		b.Columns[ci].Cards[i] = Card{
			Number:    msg.card.Number,
			Title:     msg.card.Title,
			Body:      msg.card.Body,
			URL:       msg.card.URL,
			Labels:    mapLabels(msg.card.Labels),
			LinkedPRs: b.Columns[ci].Cards[i].LinkedPRs,
			Assignees: b.Columns[ci].Cards[i].Assignees,
			CreatedAt: b.Columns[ci].Cards[i].CreatedAt,
		}
	}
	cmd := b.statusBar.SetTimedMessage("Card updated", StatusSuccess, statusMessageDuration)
	return b, cmd
}

// restoreModeHints resets the status bar hints after leaving commentMode,
// restoring detail-panel focus if the comment was triggered from there
// (mirrors the helpFromDetailFocused pattern).
func (b *Board) restoreModeHints() {
	if b.comment.fromDetailFocused {
		b.detailFocused = true
		b.rebuildDetailHints()
		return
	}
	b.statusBar.SetActionHints(b.normalHints)
}

// filterMoveDown cycles the filter cursor to the next selectable (non-header)
// item, wrapping from the last selectable item to the first.
func (b *Board) filterMoveDown() {
	b.filterMove(true)
}

// filterMoveUp cycles the filter cursor to the previous selectable
// (non-header) item, wrapping from the first selectable item to the last.
func (b *Board) filterMoveUp() {
	b.filterMove(false)
}

// filterMove steps the filter cursor one position via moveCursor, skipping
// header rows, until it lands on a selectable item or the loop's bound
// (len(filterItems) iterations) is reached -- guarding against an infinite
// loop if every item were somehow a header. A list with no selectable items
// is a no-op.
func (b *Board) filterMove(down bool) {
	for range b.filterItems {
		b.filterCursor = moveCursor(b.filterCursor, len(b.filterItems), down)
		if !b.filterItems[b.filterCursor].isHeader {
			return
		}
	}
}

// moveCursor returns cursor moved one step within [0, length-1], cycling
// around the ends: down from the last item wraps to the first, up from the
// first item wraps to the last. Lists with 0 or 1 items are a no-op (cursor
// is returned unchanged, avoiding a divide-by-zero).
func moveCursor(cursor, length int, down bool) int {
	if length <= 1 {
		return cursor
	}
	if down {
		return (cursor + 1) % length
	}
	return (cursor - 1 + length) % length
}

// dispatchGitMenuKey closes the git menu and runs the built-in action bound to
// key. It dispatches from b.defaultActions directly (not resolveAction): git
// menu keys are menu-scoped, so normal-mode custom actions on the same letter
// never shadow them (and vice versa).
func (b Board) dispatchGitMenuKey(key string) (tea.Model, tea.Cmd) {
	b.mode = normalMode
	b.statusBar.SetActionHints(b.normalHints)

	act, ok := b.defaultActions[key]
	if !ok {
		return b, nil
	}
	return b.handleBoardActionKey(act)
}

func (b Board) handleAssigneesUpdated(msg assigneesUpdatedMsg) (tea.Model, tea.Cmd) {
	updated := mapProviderCard(msg.card)
	if ci, i, ok := b.findCard(updated.Number); ok {
		b.Columns[ci].Cards[i].Assignees = updated.Assignees
	}
	cmd := b.statusBar.SetTimedMessage("Assignees updated", StatusSuccess, statusMessageDuration)
	return b, cmd
}

func (b Board) handlePROpenKey(card Card) (tea.Model, tea.Cmd) {
	switch len(card.LinkedPRs) {
	case 0:
		cmd := b.statusBar.SetTimedMessage("No linked PRs", StatusWarning, statusMessageDuration)
		return b, cmd
	case 1:
		pr := card.LinkedPRs[0]
		if err := b.executor.OpenURL(pr.URL); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
			return b, cmd
		}
		cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Opened PR #%d", pr.Number), StatusSuccess, statusMessageDuration)
		return b, cmd
	default:
		b.prPickerIndex = 0
		b.mode = prPickerMode
		b.statusBar.SetActionHints(prPickerHints)
		return b, nil
	}
}

func (b Board) handleTicketOpenKey() (tea.Model, tea.Cmd) {
	if len(b.Columns) == 0 {
		return b, nil
	}
	if len(b.visibleCards()) == 0 {
		return b, nil
	}
	card := b.selectedCard()

	if card.URL == "" {
		cmd := b.statusBar.SetTimedMessage("URL not available", StatusWarning, statusMessageDuration)
		return b, cmd
	}

	if err := b.executor.OpenURL(card.URL); err != nil {
		cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
		return b, cmd
	}

	msg := fmt.Sprintf("Opened #%d", card.Number)
	cmd := b.statusBar.SetTimedMessage(msg, StatusSuccess, statusMessageDuration)
	return b, cmd
}

func (b Board) handleDetailFocusedKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle Escape via msg.Type first.
	if msg.Type == tea.KeyEsc {
		b.detailFocused = false
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	}

	// Check for number key navigation (1-9).
	if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' {
		idx := int(msg.Runes[0] - '1')
		if idx < len(b.Columns) {
			b.detailFocused = false
			b.Columns[idx].Cursor = 0
			b.switchColumn(idx)
		}
		return b, nil
	}

	switch msg.String() {
	case "q":
		return b, tea.Quit
	case "e":
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		return b, openEditorCmd(b.selectedCard())
	case "r":
		if b.refreshing {
			return b, nil
		}
		b.pendingAutoRefresh = false
		b.refreshing = true
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider, true))
	case "o":
		return b.handleTicketOpenKey()
	case "p":
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		return b.handlePROpenKey(b.selectedCard())
	case "?":
		b.helpFromDetailFocused = true
		b.detailFocused = false
		b.helpScrollOffset = 0
		b.mode = helpMode
		b.statusBar.SetActionHints(helpModeHints)
		return b, nil
	case "h", "left":
		b.detailFocused = false
		b.statusBar.SetActionHints(b.normalHints)
	case "j", "down":
		b.scrollDetailDown()
	case "k", "up":
		if b.detailScrollOffset > 0 {
			b.detailScrollOffset--
		}
	case "tab":
		b.detailFocused = false
		b.switchColumn((b.ActiveTab + 1) % len(b.Columns))
	case "shift+tab":
		b.detailFocused = false
		b.switchColumn((b.ActiveTab - 1 + len(b.Columns)) % len(b.Columns))
	default:
		return b.handleCustomActionKey(msg)
	}
	return b, nil
}

// scrollDetailDown increments the detail panel scroll offset by one line,
// respecting the rendered content height and panel dimensions.
func (b *Board) scrollDetailDown() {
	if len(b.visibleCards()) == 0 {
		return
	}
	card := b.selectedCard()
	fullMarkdown := composeDetailMarkdown(card)
	rendered := renderBody(fullMarkdown)
	totalLines := len(strings.Split(rendered, "\n"))
	panelHeight, _, _ := b.layoutDimensions()
	availableLines := panelHeight
	if b.detailScrollOffset > 0 {
		availableLines--
		if availableLines < 1 {
			availableLines = 1
		}
	}
	maxOffset := totalLines - availableLines
	if maxOffset < 0 {
		maxOffset = 0
	}
	if b.detailScrollOffset < maxOffset {
		b.detailScrollOffset++
	}
}

// onCursorMoved resets the detail scroll position, clamps the card list
// scroll offset to the new cursor position, and rebuilds/reapplies the
// normal-mode action hints (the hint set can depend on the selected card,
// e.g. scope:pr custom actions only show when the card has linked PRs).
// Only appropriate for callers displaying normal-mode hints -- search mode's
// arrow-key navigation does NOT use this helper; see the F3 commit message
// for why (calling it there would clobber the search-mode hint bar).
func (b *Board) onCursorMoved() {
	// Keyboard keys are consumed by a pending key sequence, so only mouse
	// events can move the cursor mid-sequence -- the selected card (and with
	// it the pr-scope gating of the sequence's candidates) changed, and the
	// hint reset below replaces the pending hint bar, so cancel the sequence
	// to keep handler state and view in sync.
	b.clearPendingSeq()
	b.detailScrollOffset = 0
	b.clampScrollOffset()
	b.rebuildNormalHints()
	b.statusBar.SetActionHints(b.normalHints)
}

func (b *Board) switchColumn(idx int) {
	b.ActiveTab = idx
	b.Columns[b.ActiveTab].ScrollOffset = 0
	b.onCursorMoved()
}

// helpMaxScrollOffset computes the maximum scroll offset for the help modal content.
func (b Board) helpMaxScrollOffset() int {
	content := b.buildHelpContent()
	contentLines := strings.Split(content, "\n")
	// Match viewHelpModal layout: modal overhead 8, reserve 2 for hints bar + blank line.
	modalHeight := b.Height - 8
	if modalHeight < 5 {
		modalHeight = 5
	}
	visibleLines := modalHeight - 2
	if visibleLines < 1 {
		visibleLines = 1
	}
	maxOffset := len(contentLines) - visibleLines
	if maxOffset < 0 {
		maxOffset = 0
	}
	return maxOffset
}

// closeHelp exits helpMode and restores the previous mode (normal or detail-focused).
func (b *Board) closeHelp() {
	b.mode = normalMode
	if b.helpFromDetailFocused {
		b.detailFocused = true
		b.rebuildDetailHints()
	} else {
		b.statusBar.SetActionHints(b.normalHints)
	}
}
