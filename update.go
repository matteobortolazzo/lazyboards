package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func (b Board) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearStatusMsg:
		b.statusBar.ClearMessage()
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
		b.agentWatchConsecutiveErrors = 0
		b.statusBar.SetDispatchStatus(formatDispatchSegment(msg.snapshot.Dispatch))
		if b.agentWatcher == nil {
			return b, nil
		}
		return b, subscribeAgentWatchCmd(b.agentWatcher)

	case agentWatchErrorMsg:
		b.agentWatchConsecutiveErrors++
		debuglog.Errorf("agentwatch: %v", msg.err)
		if b.agentWatchConsecutiveErrors >= agentWatchClearThreshold {
			b.statusBar.SetDispatchStatus("")
		}
		if b.agentBackoff <= 0 {
			b.agentBackoff = agentWatchInitialBackoff
		} else {
			b.agentBackoff *= 2
			if b.agentBackoff > agentWatchMaxBackoff {
				b.agentBackoff = agentWatchMaxBackoff
			}
		}
		cmd := b.scheduleAgentWatchRetry()
		return b, cmd

	case agentWatchRetryMsg:
		if b.agentWatcher == nil {
			return b, nil
		}
		return b, subscribeAgentWatchCmd(b.agentWatcher)

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
			b.dispatch.loopErr = "agentwatch version too old for loop status — upgrade to use this feature"
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
		case filterMode:
			return b.handleFilterModeKey(msg)
		case assignMode:
			return b.handleAssignModeKey(msg)
		case gitPanelMode:
			return b.handleGitPanelKey(msg)
		case prListMode:
			return b.handlePRListModeKey(msg)
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

// scheduleAgentWatchRetry returns a tea.Cmd that fires an agentWatchRetryMsg
// after the current backoff duration, so the watcher can be re-subscribed.
func (b Board) scheduleAgentWatchRetry() tea.Cmd {
	return tea.Tick(b.agentBackoff, func(time.Time) tea.Msg {
		return agentWatchRetryMsg{}
	})
}

func (b Board) handleBoardFetched(msg boardFetchedMsg) (tea.Model, tea.Cmd) {
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
			b.statusBar.SetActionHints(detailFocusHints)
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

		// Guard A — agentwatch liveness: join by ticket number, so a title
		// rewrite (refine edits titles) can't hide a live agent's window.
		if b.agentSessionBusy(cardNum) {
			// A miss observed here still counts toward Guard C's debounce, so
			// cleanup runs on the first fetch after the agent finishes.
			prev.missingSeen = prev.missingSeen || !exists
			newCards[cardNum] = prev
			continue
		}

		// Guard B — working label: marks an in-flight agent even when
		// agentwatch is off or its snapshot lags behind.
		if exists && b.hasWorkingLabel(newInfo.labels) {
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

		// Card departed — expand template and collect command.
		window := b.resolveWindowName(cardNum, prev.title)
		vars := action.BuildTemplateVars(cardNum, prev.title, prev.labels, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, "", window)
		expanded := action.ExpandTemplate(cleanup, action.BuildShellSafeVars(vars))
		commands = append(commands, expanded)
	}

	if len(commands) == 0 {
		return nil
	}
	return runCleanupCmds(b.executor, commands)
}

// agentSessionBusy reports whether the agentwatch window joined to the card
// number has an agent that is running or waiting for input. Always false when
// no snapshot is stored (agentwatch off/absent).
func (b *Board) agentSessionBusy(cardNum int) bool {
	ws := b.agentStatusForNumber(cardNum)
	if ws == nil {
		return false
	}
	return ws.Status == agentStatusRunning || ws.Status == agentStatusNeedInput
}

// resolveWindowName resolves the {window} template variable: the live
// agentwatch window name joined to cardNum by ticket-number prefix (see
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

func (b Board) handleCardCreated(msg cardCreatedMsg) (tea.Model, tea.Cmd) {
	b.Columns[0].Cards = append(b.Columns[0].Cards, mapProviderCard(msg.card))
	b.create.titleInput.SetValue("")
	b.create.labelInput.SetValue("")
	b.validationErr = ""
	b.mode = normalMode

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

func (b Board) handleLabelConfirmModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Edit cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	switch msg.String() {
	case "y":
		label := b.labelConfirm.unknownLabels[b.labelConfirm.currentIdx]
		return b, createLabelCmd(b.provider, label)
	case "n":
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Edit cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	return b, nil
}

func (b Board) handleCloseConfirmModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Close cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	switch msg.String() {
	case "y":
		cmd := closeCardCmd(b.provider, b.closeConfirm.card.Number)
		b.mode = normalMode
		return b, cmd
	case "n":
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Close cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	return b, nil
}

// handleCardClosed removes the closed card from its column and, unless a
// cleanup guard blocks it (agent busy or working label present), fires the
// column's cleanup command immediately -- no debounce, unlike detectDepartures'
// background-fetch departure detection. The prevCards entry is unconditionally
// deleted regardless of guard outcome (locked decision #347 Q2: a guard-blocked
// close always deletes, it never defers).
func (b Board) handleCardClosed(msg cardClosedMsg) (tea.Model, tea.Cmd) {
	cardNum := msg.card.Number
	labelNames := make([]string, len(msg.card.Labels))
	for j, l := range msg.card.Labels {
		labelNames[j] = l.Name
	}

	var cleanupCmd tea.Cmd
	for ci := range b.Columns {
		for i := range b.Columns[ci].Cards {
			if b.Columns[ci].Cards[i].Number != cardNum {
				continue
			}
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
			break
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
	for ci := range b.Columns {
		for i := range b.Columns[ci].Cards {
			if b.Columns[ci].Cards[i].Number == msg.card.Number {
				b.Columns[ci].Cards[i] = Card{
					Number:    msg.card.Number,
					Title:     msg.card.Title,
					Body:      msg.card.Body,
					URL:       msg.card.URL,
					Labels:    mapLabels(msg.card.Labels),
					LinkedPRs: b.Columns[ci].Cards[i].LinkedPRs,
					Assignees: b.Columns[ci].Cards[i].Assignees,
				}
				cmd := b.statusBar.SetTimedMessage("Card updated", StatusSuccess, statusMessageDuration)
				return b, cmd
			}
		}
	}
	cmd := b.statusBar.SetTimedMessage("Card updated", StatusSuccess, statusMessageDuration)
	return b, cmd
}

func (b Board) handleCreateModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		return b, nil
	case tea.KeyEnter:
		title := strings.TrimSpace(strings.ReplaceAll(b.create.titleInput.Value(), "\n", " "))
		if title == "" {
			b.validationErr = "Title is required"
			return b, nil
		}
		label := strings.TrimSpace(b.create.labelInput.Value())
		for _, col := range b.Columns {
			if strings.EqualFold(col.Title, label) {
				b.validationErr = "Cannot use reserved column label"
				return b, nil
			}
		}
		// Store pending assignee if a real collaborator is selected (not "(none)").
		if len(b.create.assigneeOptions) > 1 && b.create.assigneeOptions[b.create.assigneeIndex] != noneAssignee {
			login := b.create.assigneeOptions[b.create.assigneeIndex]
			login = strings.TrimSuffix(login, " (me)")
			b.create.pendingAssignee = login
		} else {
			b.create.pendingAssignee = ""
		}
		b.mode = creatingMode
		b.create.titleInput.Blur()
		b.create.labelInput.Blur()
		return b, tea.Batch(b.spinner.Tick, createCardCmd(b.provider, title, label))
	case tea.KeyTab:
		var cmd tea.Cmd
		hasAssignee := len(b.create.assigneeOptions) > 1
		switch b.create.focus {
		case 0: // title -> label
			b.create.focus = 1
			b.create.titleInput.Blur()
			cmd = b.create.labelInput.Focus()
		case 1: // label -> assignee (if available) or title
			b.create.labelInput.Blur()
			if hasAssignee {
				b.create.focus = 2
			} else {
				b.create.focus = 0
				cmd = b.create.titleInput.Focus()
			}
		case 2: // assignee -> title
			b.create.focus = 0
			cmd = b.create.titleInput.Focus()
		}
		return b, cmd
	default:
		b.validationErr = ""
		var cmd tea.Cmd
		switch b.create.focus {
		case 0: // title focused
			b.create.titleInput, cmd = b.create.titleInput.Update(msg)
			b.recalcCreateInputs()
			// After recalcCreateInputs changes the textarea height, the
			// internal viewport's content lines may be stale, preventing
			// repositionView from scrolling correctly. Call View() to
			// update the viewport content (via the shared pointer), then
			// send a nil message through Update to trigger
			// repositionView, which scrolls the viewport to keep the
			// cursor visible.
			_ = b.create.titleInput.View()
			var repositionCmd tea.Cmd
			b.create.titleInput, repositionCmd = b.create.titleInput.Update(nil)
			cmd = tea.Batch(cmd, repositionCmd)
		case 1: // label focused
			b.create.labelInput, cmd = b.create.labelInput.Update(msg)
		case 2: // assignee focused
			if len(b.create.assigneeOptions) == 0 {
				return b, nil
			}
			switch msg.String() {
			case "left":
				b.create.assigneeIndex--
				if b.create.assigneeIndex < 0 {
					b.create.assigneeIndex = len(b.create.assigneeOptions) - 1
				}
			case "right":
				b.create.assigneeIndex++
				if b.create.assigneeIndex >= len(b.create.assigneeOptions) {
					b.create.assigneeIndex = 0
				}
			}
		}
		return b, cmd
	}
}

func (b Board) handleConfigModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		if b.config.firstLaunch {
			return b, tea.Quit
		}
		b.mode = normalMode
		return b, nil
	case tea.KeyEnter:
		provider := b.config.providerOptions[b.config.providerIndex]
		repo := strings.TrimSpace(b.config.repoInput.Value())
		if repo == "" {
			b.validationErr = "Repository is required"
			return b, nil
		}
		b.validationErr = ""
		return b, saveConfigCmd(b.config.localPath, provider, repo)
	case tea.KeyTab:
		if b.config.focus == 0 {
			b.config.focus = 1
			cmd := b.config.repoInput.Focus()
			return b, cmd
		}
		b.config.focus = 0
		b.config.repoInput.Blur()
		return b, nil
	case tea.KeyRight:
		if b.config.focus == 0 {
			b.config.providerIndex = (b.config.providerIndex + 1) % len(b.config.providerOptions)
		}
		return b, nil
	case tea.KeyLeft:
		if b.config.focus == 0 {
			b.config.providerIndex = (b.config.providerIndex - 1 + len(b.config.providerOptions)) % len(b.config.providerOptions)
		}
		return b, nil
	default:
		if b.config.focus == 1 {
			var cmd tea.Cmd
			b.config.repoInput, cmd = b.config.repoInput.Update(msg)
			return b, cmd
		}
		return b, nil
	}
}

func (b Board) handleNormalModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if b.detailFocused {
		return b.handleDetailFocusedKey(msg)
	}

	switch msg.String() {
	case "q":
		return b, tea.Quit
	case "n":
		b.mode = createMode
		b.create.titleInput.SetValue("")
		b.create.labelInput.SetValue("")
		b.create.focus = 0
		b.create.assigneeIndex = 0
		b.create.pendingAssignee = ""
		if len(b.collaborators) > 0 {
			options := []string{noneAssignee}
			if b.authenticatedUser != "" {
				options = append(options, b.authenticatedUser+" (me)")
			}
			for _, c := range b.collaborators {
				if !strings.EqualFold(c.Login, b.authenticatedUser) {
					options = append(options, c.Login)
				}
			}
			b.create.assigneeOptions = options
		} else {
			b.create.assigneeOptions = nil
		}
		b.recalcCreateInputs()
		cmd := b.create.titleInput.Focus()
		b.create.labelInput.Blur()
		return b, cmd
	case "e":
		if len(b.Columns) == 0 {
			return b, nil
		}
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		return b, openEditorCmd(b.selectedCard())
	case "c":
		b.enterConfigMode()
	case "r":
		if b.refreshing {
			return b, nil
		}
		b.pendingAutoRefresh = false
		b.refreshing = true
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider, true))
	case "p":
		if len(b.Columns) == 0 {
			return b, nil
		}
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		return b.handlePROpenKey(b.selectedCard())
	case "x":
		if len(b.Columns) == 0 {
			return b, nil
		}
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		b.closeConfirm = closeConfirmState{card: b.selectedCard()}
		b.mode = closeConfirmMode
		return b, nil
	case "v":
		b.enterPRList()
		return b, nil
	case "/":
		b.mode = searchMode
		cmd := b.searchInput.Focus()
		b.statusBar.SetActionHints(searchModeHints)
		return b, cmd
	case "o":
		return b.handleTicketOpenKey()
	case "l", "right":
		b.detailFocused = true
		b.statusBar.SetActionHints(detailFocusHints)
	case "shift+tab":
		b.switchColumn((b.ActiveTab - 1 + len(b.Columns)) % len(b.Columns))
	case "tab":
		b.switchColumn((b.ActiveTab + 1) % len(b.Columns))
	case "j", "down":
		col := &b.Columns[b.ActiveTab]
		maxIdx := len(col.Cards) - 1
		if b.searchQuery != "" || b.activeFilterType != filterTypeNone {
			maxIdx = len(b.filteredCards()) - 1
		}
		if col.Cursor < maxIdx {
			col.Cursor++
		}
		b.detailScrollOffset = 0
		b.clampScrollOffset()
		b.rebuildNormalHints()
		b.statusBar.SetActionHints(b.normalHints)
	case "k", "up":
		col := &b.Columns[b.ActiveTab]
		if col.Cursor > 0 {
			col.Cursor--
		}
		b.detailScrollOffset = 0
		b.clampScrollOffset()
		b.rebuildNormalHints()
		b.statusBar.SetActionHints(b.normalHints)
	case "a":
		if len(b.Columns) == 0 || b.ActiveTab >= len(b.Columns) {
			return b, nil
		}
		cards := b.filteredCards()
		if len(cards) == 0 || len(b.collaborators) == 0 {
			return b, nil
		}

		card := b.selectedCard()
		assignedSet := make(map[string]bool)
		for _, a := range card.Assignees {
			assignedSet[strings.ToLower(a.Login)] = true
		}

		var items []assignItem
		if b.authenticatedUser != "" {
			items = append(items, assignItem{
				login:      b.authenticatedUser,
				isAssigned: assignedSet[strings.ToLower(b.authenticatedUser)],
				isMe:       true,
			})
		}
		for _, c := range b.collaborators {
			if strings.EqualFold(c.Login, b.authenticatedUser) {
				continue
			}
			items = append(items, assignItem{
				login:      c.Login,
				isAssigned: assignedSet[strings.ToLower(c.Login)],
			})
		}

		b.assign = assignState{items: items, cursor: 0}
		b.mode = assignMode
		b.statusBar.SetActionHints(assignModeHints)
		return b, nil
	case "f":
		if b.activeFilterType != filterTypeNone {
			b.clearFilter()
			b.clampScrollOffset()
			cmd := b.statusBar.SetTimedMessage("Filter cleared", StatusSuccess, statusMessageDuration)
			return b, cmd
		}
		items := b.collectFilterItems()
		if len(items) == 0 {
			return b, nil
		}
		b.filterItems = items
		// Set cursor to first selectable (non-header) item.
		b.filterCursor = 0
		for i, item := range items {
			if !item.isHeader {
				b.filterCursor = i
				break
			}
		}
		b.mode = filterMode
		b.statusBar.SetActionHints(filterModeHints)
		return b, nil
	case "?":
		b.helpFromDetailFocused = false
		b.helpScrollOffset = 0
		b.mode = helpMode
		b.statusBar.SetActionHints(helpModeHints)
		return b, nil
	case "g":
		b.enterGitPanel()
		return b, nil
	case "d":
		b.dispatch = dispatchState{loading: true}
		b.mode = dispatchMode
		b.statusBar.SetActionHints(dispatchModeHints)
		return b, queryDispatchStatusCmd(b.executor)
	default:
		// Check for number key navigation (1-9).
		if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' {
			idx := int(msg.Runes[0] - '1')
			if idx < len(b.Columns) {
				b.Columns[idx].Cursor = 0
				b.switchColumn(idx)
			}
			return b, nil
		}
		return b.handleCustomActionKey(msg)
	}
	return b, nil
}

// handleCustomActionKey resolves msg against the user's custom action system:
// Alt+letter enters comment mode (if the action's template uses {comment}) or
// dispatches immediately, and a plain uppercase letter dispatches directly.
// Shared by normal mode and detail-focused mode so custom actions behave
// identically in both -- b.detailFocused (already accurate at call time,
// since detail-focused mode is a sub-state routed to before this is ever
// reached) is threaded onto the pending comment so returning from comment
// mode restores the focus it was triggered from, mirroring the
// helpFromDetailFocused pattern. Scope routing (board/card/pr) is delegated
// to dispatchResolvedAction so every dispatch path shares one gating rule.
// Returns b unchanged if msg is not a recognized custom action key.
func (b Board) handleCustomActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Alt+Shift+key: check for comment mode trigger (uppercase A-Z only).
	if msg.Alt && len(msg.Runes) == 1 && msg.Runes[0] >= 'A' && msg.Runes[0] <= 'Z' {
		baseKey := string(msg.Runes)
		if act, ok := b.resolveAction(baseKey); ok {
			template := act.URL + act.Command
			if strings.Contains(template, "{comment}") {
				// Resolve the pending card (if card-scope or pr-scope) before
				// touching any state, so a "no card visible" refusal leaves b
				// untouched.
				var pendingCard Card
				if act.Scope != "board" {
					if len(b.visibleCards()) == 0 {
						return b, nil
					}
					pendingCard = b.selectedCard()
				}
				ci := textinput.New()
				ci.Placeholder = "Comment..."
				ci.CharLimit = 2000
				b.comment = commentState{
					input:             ci,
					pendingAction:     act,
					pendingCard:       pendingCard,
					boardScope:        act.Scope == "board",
					prScope:           act.Scope == "pr",
					fromDetailFocused: b.detailFocused,
				}
				b.detailFocused = false
				b.mode = commentMode
				b.statusBar.SetActionHints(commentModeHints)
				return b, b.comment.input.Focus()
			}
			// Alt on action without {comment} -- execute normally.
			return b.dispatchResolvedAction(act)
		}
		return b, nil
	}
	// Check if it's a custom action key (uppercase A-Z only).
	if len(msg.Runes) == 1 && msg.Runes[0] >= 'A' && msg.Runes[0] <= 'Z' {
		if act, ok := b.resolveAction(msg.String()); ok {
			return b.dispatchResolvedAction(act)
		}
	}
	return b, nil
}

// restoreModeHints resets the status bar hints after leaving commentMode,
// restoring detail-panel focus if the comment was triggered from there
// (mirrors the helpFromDetailFocused pattern).
func (b *Board) restoreModeHints() {
	if b.comment.fromDetailFocused {
		b.detailFocused = true
		b.statusBar.SetActionHints(detailFocusHints)
		return
	}
	b.statusBar.SetActionHints(b.normalHints)
}

// dispatchResolvedAction runs act against the currently selected card (or the
// whole board for board scope), applying the same scope gating used by both
// the plain-key and Alt+key custom-action dispatch paths.
func (b Board) dispatchResolvedAction(act config.Action) (tea.Model, tea.Cmd) {
	if act.Scope == "board" {
		return b.handleBoardActionKey(act)
	}
	if len(b.visibleCards()) == 0 {
		return b, nil
	}
	if act.Scope == "pr" {
		return b.handlePRActionKey(act, b.selectedCard())
	}
	return b.handleActionKey(act, b.selectedCard())
}

func (b Board) handleCommentModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.restoreModeHints()
		return b, nil
	case tea.KeyEnter:
		b.mode = normalMode
		b.restoreModeHints()
		comment := b.comment.input.Value()
		act := b.comment.pendingAction
		if b.comment.boardScope {
			return b.handleBoardActionKeyWithComment(act, comment)
		}
		if b.comment.prScope {
			return b.handlePRActionKeyWithComment(act, b.comment.pendingCard, comment)
		}
		return b.handleActionKeyWithComment(act, b.comment.pendingCard, comment)
	default:
		var cmd tea.Cmd
		b.comment.input, cmd = b.comment.input.Update(msg)
		return b, cmd
	}
}

func (b Board) handleFilterModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case tea.KeyEnter:
		if b.filterCursor < len(b.filterItems) && !b.filterItems[b.filterCursor].isHeader {
			item := b.filterItems[b.filterCursor]
			b.activeFilterType = item.itemType
			b.activeFilterValue = item.value
			// Clamp cursor to filtered card count.
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
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	}

	switch msg.String() {
	case "j", "down":
		b.filterMoveDown()
	case "k", "up":
		b.filterMoveUp()
	}
	return b, nil
}

// filterMoveDown moves the filter cursor to the next selectable (non-header) item.
func (b *Board) filterMoveDown() {
	for i := b.filterCursor + 1; i < len(b.filterItems); i++ {
		if !b.filterItems[i].isHeader {
			b.filterCursor = i
			return
		}
	}
}

// filterMoveUp moves the filter cursor to the previous selectable (non-header) item.
func (b *Board) filterMoveUp() {
	for i := b.filterCursor - 1; i >= 0; i-- {
		if !b.filterItems[i].isHeader {
			b.filterCursor = i
			return
		}
	}
}

func (b Board) handleAssignModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case tea.KeyEnter:
		if len(b.assign.items) == 0 || b.assign.cursor >= len(b.assign.items) {
			return b, nil
		}
		item := b.assign.items[b.assign.cursor]
		card := b.selectedCard()

		newLogins := []string{}
		if item.isAssigned {
			for _, a := range card.Assignees {
				if !strings.EqualFold(a.Login, item.login) {
					newLogins = append(newLogins, a.Login)
				}
			}
		} else {
			for _, a := range card.Assignees {
				newLogins = append(newLogins, a.Login)
			}
			newLogins = append(newLogins, item.login)
		}

		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		statusCmd := b.statusBar.SetTimedMessage("Updating assignees...", StatusInfo, longStatusMessageDuration)
		return b, tea.Batch(statusCmd, setAssigneesCmd(b.provider, card.Number, newLogins))
	}

	switch msg.String() {
	case "j", "down":
		if b.assign.cursor < len(b.assign.items)-1 {
			b.assign.cursor++
		}
	case "k", "up":
		if b.assign.cursor > 0 {
			b.assign.cursor--
		}
	}
	return b, nil
}

func (b Board) handleGitPanelKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case tea.KeyEnter:
		if len(b.gitPanel.items) == 0 || b.gitPanel.cursor >= len(b.gitPanel.items) {
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			return b, nil
		}
		return b.dispatchGitMenuKey(b.gitPanel.items[b.gitPanel.cursor].key)
	}

	switch msg.String() {
	case "j", "down":
		if b.gitPanel.cursor < len(b.gitPanel.items)-1 {
			b.gitPanel.cursor++
		}
		return b, nil
	case "k", "up":
		if b.gitPanel.cursor > 0 {
			b.gitPanel.cursor--
		}
		return b, nil
	}

	// Lazygit-style direct dispatch: pressing a menu item's key runs it
	// immediately without navigating to it first.
	if _, ok := b.defaultActions[msg.String()]; ok {
		return b.dispatchGitMenuKey(msg.String())
	}
	return b, nil
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

// handleDispatchModeKey handles key presses while the agent dispatch modal
// is open. Enter toggles enrollment for the current repo; "o" runs a
// fleet-wide dispatch pass. The modal is rendered by viewDispatchModal (#284).
func (b Board) handleDispatchModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case tea.KeyEnter:
		if b.dispatch.loading || b.dispatch.err != "" || b.dispatch.running {
			return b, nil
		}
		if b.dispatch.repo == "" {
			return b, nil
		}
		b.dispatch.loading = true
		return b, toggleEnrollCmd(b.executor, b.dispatch.enrolled)
	}

	switch msg.String() {
	case "o":
		if b.dispatch.loading || b.dispatch.err != "" || b.dispatch.running {
			return b, nil
		}
		if !b.dispatch.enrolled {
			return b, nil
		}
		b.dispatch.running = true
		return b, dispatchOnceCmd(b.executor)
	}

	return b, nil
}

func (b Board) handleAssigneesUpdated(msg assigneesUpdatedMsg) (tea.Model, tea.Cmd) {
	updated := mapProviderCard(msg.card)
	for ci := range b.Columns {
		for i := range b.Columns[ci].Cards {
			if b.Columns[ci].Cards[i].Number == updated.Number {
				b.Columns[ci].Cards[i].Assignees = updated.Assignees
				cmd := b.statusBar.SetTimedMessage("Assignees updated", StatusSuccess, statusMessageDuration)
				return b, cmd
			}
		}
	}
	cmd := b.statusBar.SetTimedMessage("Assignees updated", StatusSuccess, statusMessageDuration)
	return b, cmd
}

// dispatchExpandedAction expands act's URL/command template with vars and
// executes it (url -> OpenURL, shell -> runShellCmd). This is the shared leaf
// dispatch shared by every action scope (card, board, pr).
func (b Board) dispatchExpandedAction(act config.Action, vars map[string]string) (tea.Model, tea.Cmd) {
	switch act.Type {
	case "url":
		urlVars := action.BuildURLSafeVars(vars)
		expanded := action.ExpandTemplate(act.URL, urlVars)
		if err := b.executor.OpenURL(expanded); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
			return b, cmd
		}
		return b, nil
	case "shell":
		shellVars := action.BuildShellSafeVars(vars)
		expanded := action.ExpandTemplate(act.Command, shellVars)
		cmd := b.statusBar.SetTimedMessage("Running...", StatusInfo, longStatusMessageDuration)
		return b, tea.Batch(cmd, runShellCmd(b.executor, expanded))
	}
	return b, nil
}

func (b Board) handleActionKeyWithComment(act config.Action, card Card, comment string) (tea.Model, tea.Cmd) {
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	window := b.resolveWindowName(card.Number, card.Title)
	vars := action.BuildTemplateVars(card.Number, card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, comment, window)
	return b.dispatchExpandedAction(act, vars)
}

func (b Board) handleBoardActionKeyWithComment(act config.Action, comment string) (tea.Model, tea.Cmd) {
	vars := action.BuildBoardTemplateVars(b.repoOwner, b.repoName, b.providerName, comment)
	return b.dispatchExpandedAction(act, vars)
}

// runPRAction is the leaf dispatcher for a scope: pr action against a
// specific card and one of its linked PRs. It layers PR-specific template
// variables on top of the card-scope base vars, then dispatches through
// dispatchExpandedAction like every other scope.
func (b Board) runPRAction(act config.Action, card Card, pr LinkedPR, comment string) (tea.Model, tea.Cmd) {
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	window := b.resolveWindowName(card.Number, card.Title)
	baseVars := action.BuildTemplateVars(card.Number, card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, comment, window)
	vars := action.BuildPRTemplateVars(baseVars, pr.Number, pr.Title, pr.URL, pr.Branch)
	return b.dispatchExpandedAction(act, vars)
}

// handlePRActionKeyWithComment implements the full 0/1/2+ linked-PR
// precedence for a scope: pr action, mirroring handlePROpenKey (the
// built-in "p" key's precedence anchor):
//   - 0 PRs: no-op (defensive; resolveAction should already gate this out).
//   - 1 PR: runs the action immediately against that PR's data.
//   - 2+ PRs: stashes the action (and any comment) as pendingPRAction and
//     opens prPickerMode; the picker's Enter key consumes it.
func (b Board) handlePRActionKeyWithComment(act config.Action, card Card, comment string) (tea.Model, tea.Cmd) {
	switch len(card.LinkedPRs) {
	case 0:
		debuglog.Errorf("scope:pr action %q dispatched against a card with 0 linked PRs (resolveAction gate bypassed)", act.Name)
		return b, nil
	case 1:
		return b.runPRAction(act, card, card.LinkedPRs[0], comment)
	default:
		b.pendingPRAction = &pendingPRAction{action: act, comment: comment}
		b.prPickerIndex = 0
		b.mode = prPickerMode
		b.statusBar.SetActionHints(prPickerHints)
		return b, nil
	}
}

func (b Board) handlePRActionKey(act config.Action, card Card) (tea.Model, tea.Cmd) {
	return b.handlePRActionKeyWithComment(act, card, "")
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

func (b Board) handleActionKey(act config.Action, card Card) (tea.Model, tea.Cmd) {
	return b.handleActionKeyWithComment(act, card, "")
}

func (b Board) handleBoardActionKey(act config.Action) (tea.Model, tea.Cmd) {
	return b.handleBoardActionKeyWithComment(act, "")
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

func (b *Board) switchColumn(idx int) {
	b.ActiveTab = idx
	b.Columns[b.ActiveTab].ScrollOffset = 0
	b.detailScrollOffset = 0
	b.clampScrollOffset()
	b.rebuildNormalHints()
	b.statusBar.SetActionHints(b.normalHints)
}

func (b Board) handleSearchModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		b.clearSearch()
		b.mode = normalMode
		b.rebuildNormalHints()
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case tea.KeyEnter:
		b.searchInput.Blur()
		b.mode = normalMode
		b.clampScrollOffset()
		b.rebuildNormalHints()
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case tea.KeyTab:
		b.clearSearch()
		b.mode = normalMode
		b.switchColumn((b.ActiveTab + 1) % len(b.Columns))
		return b, nil
	case tea.KeyShiftTab:
		b.clearSearch()
		b.mode = normalMode
		b.switchColumn((b.ActiveTab - 1 + len(b.Columns)) % len(b.Columns))
		return b, nil
	// Result navigation while typing: arrows plus ctrl+n/ctrl+p (the
	// neovim/fzf convention). Bare j/k must reach the textinput so queries
	// containing those letters stay typeable.
	case tea.KeyDown, tea.KeyCtrlN:
		col := &b.Columns[b.ActiveTab]
		filtered := b.filteredCards()
		if col.Cursor < len(filtered)-1 {
			col.Cursor++
		}
		b.detailScrollOffset = 0
		b.clampScrollOffset()
		return b, nil
	case tea.KeyUp, tea.KeyCtrlP:
		col := &b.Columns[b.ActiveTab]
		if col.Cursor > 0 {
			col.Cursor--
		}
		b.detailScrollOffset = 0
		b.clampScrollOffset()
		return b, nil
	default:
		// Forward everything else (including digits and j/k) to the textinput:
		// queries can contain any character — card-number search ("42") and
		// titles with j/k must stay typeable. Column switching while searching
		// is available via tab/shift+tab.
		var cmd tea.Cmd
		b.searchInput, cmd = b.searchInput.Update(msg)
		b.searchQuery = b.searchInput.Value()
		// Reset cursor and scroll offset when filter changes.
		col := &b.Columns[b.ActiveTab]
		col.Cursor = 0
		col.ScrollOffset = 0
		b.statusBar.SetActionHints(searchModeHints)
		return b, cmd
	}
}

func (b Board) handlePRPickerModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	card := b.selectedCard()
	prCount := len(card.LinkedPRs)

	// The picker can be opened from the card list or the detail panel; restore
	// the hint set matching where the user came from.
	restoreHints := func() {
		if b.detailFocused {
			b.statusBar.SetActionHints(detailFocusHints)
		} else {
			b.statusBar.SetActionHints(b.normalHints)
		}
	}

	// Defensive: an async board refresh (boardFetchedMsg) can shrink the
	// selected card's LinkedPRs to 0 while the picker is still open, which
	// would otherwise divide-by-zero on the modulo below or panic on the
	// index. Bail out with the same cleanup as the Escape path.
	if prCount == 0 {
		b.mode = normalMode
		b.pendingPRAction = nil
		restoreHints()
		return b, nil
	}

	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.pendingPRAction = nil
		restoreHints()
		return b, nil
	case tea.KeyLeft:
		b.prPickerIndex = (b.prPickerIndex - 1 + prCount) % prCount
		return b, nil
	case tea.KeyRight:
		b.prPickerIndex = (b.prPickerIndex + 1) % prCount
		return b, nil
	case tea.KeyEnter:
		pr := card.LinkedPRs[b.prPickerIndex]
		b.mode = normalMode
		restoreHints()
		// Dual-purpose: if a scope: pr custom action is pending, run it
		// against the selected PR and clear the pending state. Otherwise fall
		// back to the original open-URL behavior (built-in "p" key).
		if b.pendingPRAction != nil {
			pending := b.pendingPRAction
			b.pendingPRAction = nil
			return b.runPRAction(pending.action, card, pr, pending.comment)
		}
		if err := b.executor.OpenURL(pr.URL); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
			return b, cmd
		}
		cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Opened PR #%d", pr.Number), StatusSuccess, statusMessageDuration)
		return b, cmd
	}
	return b, nil
}

func (b Board) handlePRListModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case tea.KeyEnter:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		if len(b.prList.entries) == 0 || b.prList.cursor >= len(b.prList.entries) {
			return b, nil
		}
		pr := b.prList.entries[b.prList.cursor].pr
		if err := b.executor.OpenURL(pr.URL); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
			return b, cmd
		}
		cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Opened PR #%d", pr.Number), StatusSuccess, statusMessageDuration)
		return b, cmd
	}

	switch msg.String() {
	case "j", "down":
		if b.prList.cursor < len(b.prList.entries)-1 {
			b.prList.cursor++
		}
		return b, nil
	case "k", "up":
		if b.prList.cursor > 0 {
			b.prList.cursor--
		}
		return b, nil
	}
	return b, nil
}

func (b Board) handleHelpModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q":
		return b, tea.Quit
	case "?":
		b.closeHelp()
		return b, nil
	case "j", "down":
		maxOffset := b.helpMaxScrollOffset()
		if b.helpScrollOffset < maxOffset {
			b.helpScrollOffset++
		}
		return b, nil
	case "k", "up":
		if b.helpScrollOffset > 0 {
			b.helpScrollOffset--
		}
		return b, nil
	}

	if msg.Type == tea.KeyEsc {
		b.closeHelp()
		return b, nil
	}

	return b, nil
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
		b.statusBar.SetActionHints(detailFocusHints)
	} else {
		b.statusBar.SetActionHints(b.normalHints)
	}
}

func (b Board) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		return b.handleMouseScroll(msg)
	case tea.MouseButtonLeft:
		if msg.Action == tea.MouseActionPress {
			return b.handleMouseClick(msg)
		}
	}
	return b, nil
}

func (b Board) handleMouseScroll(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	innerWidth := b.Width - 2
	leftTotal := innerWidth * 2 / 5

	if msg.X <= leftTotal {
		// Left panel: scroll card list by moving cursor (like j/k).
		col := &b.Columns[b.ActiveTab]
		if len(col.Cards) == 0 {
			return b, nil
		}
		if msg.Button == tea.MouseButtonWheelDown {
			maxIdx := len(col.Cards) - 1
			if b.searchQuery != "" || b.activeFilterType != filterTypeNone {
				maxIdx = len(b.filteredCards()) - 1
			}
			if col.Cursor < maxIdx {
				col.Cursor++
			}
		} else {
			if col.Cursor > 0 {
				col.Cursor--
			}
		}
		b.detailScrollOffset = 0
		b.clampScrollOffset()
		b.rebuildNormalHints()
		b.statusBar.SetActionHints(b.normalHints)
	} else {
		// Right panel: scroll detail body.
		if msg.Button == tea.MouseButtonWheelDown {
			b.scrollDetailDown()
		} else {
			if b.detailScrollOffset > 0 {
				b.detailScrollOffset--
			}
		}
	}
	return b, nil
}

func (b Board) handleMouseClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Row 0 = border title bar (tab labels).
	if msg.Y == 0 {
		return b.handleTabClick(msg)
	}

	// Left panel card click.
	innerWidth := b.Width - 2
	leftTotal := innerWidth * 2 / 5
	if msg.X <= leftTotal {
		return b.handleCardClick(msg)
	}

	return b, nil
}

func (b Board) handleTabClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	numCols := len(b.Columns)
	if numCols == 0 {
		return b, nil
	}

	prefixWidth := 3    // "╭─ "
	separatorWidth := 3 // " ─ "

	x := msg.X
	pos := prefixWidth
	for i, col := range b.Columns {
		countStr := fmt.Sprintf("(%d)", len(col.Cards))
		if b.activeFilterType != filterTypeNone {
			fc := b.filteredCardsForColumn(i)
			countStr = fmt.Sprintf("(%d/%d) \u25cf", fc, len(col.Cards))
		}
		labelText := fmt.Sprintf("[%d] %s %s", i+1, col.Title, countStr)
		labelWidth := lipgloss.Width(labelText)

		if x >= pos && x < pos+labelWidth {
			if i != b.ActiveTab {
				b.switchColumn(i)
			}
			return b, nil
		}
		pos += labelWidth
		if i < numCols-1 {
			pos += separatorWidth
		}
	}

	return b, nil
}

func (b Board) handleCardClick(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if len(b.Columns) == 0 || b.ActiveTab >= len(b.Columns) {
		return b, nil
	}
	col := &b.Columns[b.ActiveTab]

	// Use filtered cards when search or a filter is active.
	cards := col.Cards
	if b.searchQuery != "" || b.activeFilterType != filterTypeNone {
		cards = b.filteredCards()
	}
	if len(cards) == 0 {
		return b, nil
	}

	// Card content starts at Y=2 (row 0=outer border title, row 1=panel top border).
	// Account for scroll offset and up-arrow indicator.
	contentStartY := 2
	if col.ScrollOffset > 0 {
		contentStartY++ // up-arrow indicator takes 1 row
	}

	// Determine card widths for line count calculation.
	_, leftContentWidth, _ := b.layoutDimensions()
	columnNames := make([]string, len(b.Columns))
	for i, c := range b.Columns {
		columnNames[i] = c.Title
	}

	// Iterate through visible cards to find which card was clicked.
	currentY := contentStartY
	for i := col.ScrollOffset; i < len(cards); i++ {
		lines := cardLineCount(cards[i], leftContentWidth, columnNames, b.workingLabel, b.agentBadgeFor(cards[i]))
		if msg.Y >= currentY && msg.Y < currentY+lines {
			col.Cursor = i
			b.detailScrollOffset = 0
			b.clampScrollOffset()
			b.rebuildNormalHints()
			b.statusBar.SetActionHints(b.normalHints)
			return b, nil
		}
		currentY += lines
	}

	return b, nil
}
