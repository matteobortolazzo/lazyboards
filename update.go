package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

func (b Board) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearStatusMsg:
		b.statusBar.ClearMessage()
		return b, nil

	case refreshTickMsg:
		return b.handleRefreshTick()

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
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))

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
		return b, cmd

	case autoRefreshMsg:
		if !b.pendingAutoRefresh || b.refreshing {
			return b, nil
		}
		b.pendingAutoRefresh = false
		b.refreshing = true
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))

	case cleanupResultMsg:
		if msg.count == 0 {
			return b, nil
		}
		cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Cleaned up %d sessions", msg.count), StatusSuccess, statusMessageDuration)
		return b, cmd

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
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Error: "+provider.SanitizeError(msg.err), StatusError, statusMessageDuration)
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
				return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
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
		case commentMode:
			return b.handleCommentModeKey(msg)
		case filterMode:
			return b.handleFilterModeKey(msg)
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
	return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
}

func (b Board) scheduleRefreshTick() tea.Cmd {
	if b.refreshInterval <= 0 {
		return nil
	}
	return tea.Tick(b.refreshInterval, func(time.Time) tea.Msg {
		return refreshTickMsg{}
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
			cmd = b.statusBar.SetTimedMessage("Filter has no matches \u2014 press F to clear", StatusWarning, statusMessageDuration)
		} else {
			cmd = b.statusBar.SetTimedMessage("Board refreshed", StatusSuccess, statusMessageDuration)
		}
		if cleanupCmd != nil {
			cmd = tea.Batch(cmd, cleanupCmd)
		}
		if tickCmd := b.scheduleRefreshTick(); tickCmd != nil {
			cmd = tea.Batch(cmd, tickCmd)
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
			cmd = b.statusBar.SetTimedMessage("Filter has no matches \u2014 press F to clear", StatusWarning, statusMessageDuration)
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

		// Card departed — expand template and collect command.
		vars := action.BuildTemplateVars(cardNum, prev.title, prev.labels, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, "")
		expanded := action.ExpandTemplate(cleanup, action.BuildShellSafeVars(vars))
		commands = append(commands, expanded)
	}

	if len(commands) == 0 {
		return nil
	}
	return runCleanupCmds(b.executor, commands)
}

// columnCleanup returns the cleanup command for the column at colIdx, matched by title.
func (b *Board) columnCleanup(colIdx int) string {
	if colIdx >= len(b.Columns) {
		return ""
	}
	colTitle := b.Columns[colIdx].Title
	for _, cc := range b.columnConfigs {
		if strings.EqualFold(cc.Name, colTitle) {
			return cc.Cleanup
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
	return b, nil
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
		b.mode = creatingMode
		b.create.titleInput.Blur()
		b.create.labelInput.Blur()
		return b, tea.Batch(b.spinner.Tick, createCardCmd(b.provider, title, label))
	case tea.KeyTab:
		var cmd tea.Cmd
		if b.create.titleInput.Focused() {
			b.create.titleInput.Blur()
			cmd = b.create.labelInput.Focus()
		} else {
			b.create.labelInput.Blur()
			cmd = b.create.titleInput.Focus()
		}
		return b, cmd
	default:
		b.validationErr = ""
		var cmd tea.Cmd
		if b.create.titleInput.Focused() {
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
		} else if b.create.labelInput.Focused() {
			b.create.labelInput, cmd = b.create.labelInput.Update(msg)
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
		b.recalcCreateInputs()
		cmd := b.create.titleInput.Focus()
		b.create.labelInput.Blur()
		return b, cmd
	case "e":
		if len(b.Columns) == 0 {
			return b, nil
		}
		col := b.Columns[b.ActiveTab]
		if len(col.Cards) == 0 {
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
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
	case "p":
		if len(b.Columns) == 0 {
			return b, nil
		}
		col := b.Columns[b.ActiveTab]
		if len(col.Cards) == 0 {
			return b, nil
		}
		return b.handlePROpenKey(b.selectedCard())
	case "/":
		b.mode = searchMode
		cmd := b.searchInput.Focus()
		b.statusBar.SetActionHints(searchModeHints)
		return b, cmd
	case "o":
		return b.handleTicketOpenKey()
	case "O":
		return b.handleRepoOpenKey()
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
		if b.activeFilterType != filterTypeNone {
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
	case "f":
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
	case "F":
		b.clearFilter()
		b.clampScrollOffset()
		cmd := b.statusBar.SetTimedMessage("Filter cleared", StatusSuccess, statusMessageDuration)
		return b, cmd
	case "?":
		b.helpFromDetailFocused = false
		b.helpScrollOffset = 0
		b.mode = helpMode
		b.statusBar.SetActionHints(helpModeHints)
		return b, nil
	default:
		// Alt+key: check for comment mode trigger.
		if msg.Alt && len(msg.Runes) == 1 {
			baseKey := string(msg.Runes)
			if act, ok := b.resolveAction(baseKey); ok {
				template := act.URL + act.Command
				if strings.Contains(template, "{comment}") {
					// Enter comment mode.
					ci := textinput.New()
					ci.Placeholder = "Comment..."
					ci.CharLimit = 2000
					b.comment = commentState{
						input:         ci,
						pendingAction: act,
						boardScope:    act.Scope == "board",
					}
					// Store pending card if card-scope; refuse if column is empty.
					if act.Scope != "board" {
						col := b.Columns[b.ActiveTab]
						if len(col.Cards) == 0 {
							return b, nil
						}
						b.comment.pendingCard = b.selectedCard()
					}
					b.mode = commentMode
					b.statusBar.SetActionHints(commentModeHints)
					return b, b.comment.input.Focus()
				}
				// Alt on action without {comment} -- execute normally.
				col := b.Columns[b.ActiveTab]
				if act.Scope == "board" {
					return b.handleBoardActionKey(act)
				}
				if len(col.Cards) == 0 {
					return b, nil
				}
				return b.handleActionKey(act, b.selectedCard())
			}
			return b, nil
		}
		// Check for number key navigation (1-9).
		if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' {
			idx := int(msg.Runes[0] - '1')
			if idx < len(b.Columns) {
				b.Columns[idx].Cursor = 0
				b.switchColumn(idx)
			}
			return b, nil
		}
		// Check if it's a custom action key.
		if act, ok := b.resolveAction(msg.String()); ok {
			col := b.Columns[b.ActiveTab]
			if act.Scope == "board" {
				return b.handleBoardActionKey(act)
			}
			if len(col.Cards) == 0 {
				return b, nil
			}
			return b.handleActionKey(act, b.selectedCard())
		}
	}
	return b, nil
}

func (b Board) handleCommentModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case tea.KeyEnter:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		comment := b.comment.input.Value()
		act := b.comment.pendingAction
		if b.comment.boardScope {
			return b.handleBoardActionKeyWithComment(act, comment)
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

func (b Board) handleActionKeyWithComment(act config.Action, card Card, comment string) (tea.Model, tea.Cmd) {
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	vars := action.BuildTemplateVars(card.Number, card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, comment)

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

func (b Board) handleBoardActionKeyWithComment(act config.Action, comment string) (tea.Model, tea.Cmd) {
	vars := action.BuildBoardTemplateVars(b.repoOwner, b.repoName, b.providerName, comment)

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

func (b Board) handleRepoOpenKey() (tea.Model, tea.Cmd) {
	if b.providerName != "github" {
		cmd := b.statusBar.SetTimedMessage("Repository URL not available for this provider", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	if b.repoOwner == "" || b.repoName == "" {
		cmd := b.statusBar.SetTimedMessage("Repository info not available", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	url := "https://github.com/" + b.repoOwner + "/" + b.repoName
	if err := b.executor.OpenURL(url); err != nil {
		cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
		return b, cmd
	}
	cmd := b.statusBar.SetTimedMessage("Opened repository", StatusSuccess, statusMessageDuration)
	return b, cmd
}

func (b Board) handleTicketOpenKey() (tea.Model, tea.Cmd) {
	if len(b.Columns) == 0 {
		return b, nil
	}
	col := b.Columns[b.ActiveTab]
	if len(col.Cards) == 0 {
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
		col := b.Columns[b.ActiveTab]
		if len(col.Cards) == 0 {
			return b, nil
		}
		return b, openEditorCmd(b.selectedCard())
	case "r":
		if b.refreshing {
			return b, nil
		}
		b.pendingAutoRefresh = false
		b.refreshing = true
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
	case "o":
		return b.handleTicketOpenKey()
	case "O":
		return b.handleRepoOpenKey()
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
	}
	return b, nil
}

// scrollDetailDown increments the detail panel scroll offset by one line,
// respecting the rendered content height and panel dimensions.
func (b *Board) scrollDetailDown() {
	col := b.Columns[b.ActiveTab]
	if len(col.Cards) == 0 {
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
	default:
		// Check for number keys (1-9) for column switching.
		if len(msg.Runes) == 1 && msg.Runes[0] >= '1' && msg.Runes[0] <= '9' {
			colIdx := int(msg.Runes[0] - '1')
			if colIdx < len(b.Columns) {
				b.clearSearch()
				b.mode = normalMode
				b.switchColumn(colIdx)
				return b, nil
			}
		}

		// Handle j/k navigation on filtered list.
		if len(msg.Runes) == 1 {
			switch msg.Runes[0] {
			case 'j':
				col := &b.Columns[b.ActiveTab]
				filtered := b.filteredCards()
				if col.Cursor < len(filtered)-1 {
					col.Cursor++
				}
				b.detailScrollOffset = 0
				b.clampScrollOffset()
				return b, nil
			case 'k':
				col := &b.Columns[b.ActiveTab]
				if col.Cursor > 0 {
					col.Cursor--
				}
				b.detailScrollOffset = 0
				b.clampScrollOffset()
				return b, nil
			}
		}

		// Forward to textinput.
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

	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
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
		b.statusBar.SetActionHints(b.normalHints)
		if err := b.executor.OpenURL(pr.URL); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
			return b, cmd
		}
		cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Opened PR #%d", pr.Number), StatusSuccess, statusMessageDuration)
		return b, cmd
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
			if b.activeFilterType != filterTypeNone {
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

	prefixWidth := 3  // "╭─ "
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

	// Use filtered cards when a filter is active.
	cards := col.Cards
	if b.activeFilterType != filterTypeNone {
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
		lines := cardLineCount(cards[i], leftContentWidth, columnNames, b.workingLabel)
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

