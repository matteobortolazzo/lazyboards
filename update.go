package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
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
			cmd := b.statusBar.SetTimedMessage("Refresh failed: "+provider.SanitizeError(msg.err), statusMessageDuration)
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
		cmd := b.statusBar.SetTimedMessage(msg.message, statusMessageDuration)
		if msg.success {
			b.pendingAutoRefresh = true
			cmd = tea.Batch(cmd, tea.Tick(autoRefreshDelay, func(time.Time) tea.Msg {
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
		cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Cleaned up %d sessions", msg.count), statusMessageDuration)
		return b, cmd

	case spinner.TickMsg:
		if b.mode == loadingMode || b.mode == creatingMode || b.refreshing {
			var cmd tea.Cmd
			b.spinner, cmd = b.spinner.Update(msg)
			return b, cmd
		}
		return b, nil

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
		default:
			return b.handleNormalModeKey(msg)
		}

	case tea.WindowSizeMsg:
		b.Width = msg.Width
		b.Height = msg.Height
		if len(b.Columns) > 0 {
			b.clampScrollOffset()
		}
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
			cards[j] = Card{Number: c.Number, Title: c.Title, Labels: c.Labels, Body: c.Body, LinkedPRs: mapLinkedPRs(c.LinkedPRs)}
		}
		cols[i] = Column{Title: pc.Title, Cards: cards}
	}

	// Build new card position map and detect departures for cleanup.
	newCards := buildCardMap(cols)
	cleanupCmd := b.detectDepartures(newCards)
	b.prevCards = newCards

	b.pendingAutoRefresh = false

	if b.refreshing {
		// Preserve ActiveTab and cursor position by card Number.
		savedTab := b.ActiveTab
		savedNumber := -1
		if savedTab < len(b.Columns) {
			oldCol := b.Columns[savedTab]
			if len(oldCol.Cards) > 0 && oldCol.Cursor < len(oldCol.Cards) {
				savedNumber = oldCol.Cards[oldCol.Cursor].Number
			}
		}

		b.Columns = cols
		b.refreshing = false
		b.detailScrollOffset = 0

		// Clamp ActiveTab if columns were reduced.
		if b.ActiveTab >= len(b.Columns) {
			b.ActiveTab = len(b.Columns) - 1
			if b.ActiveTab < 0 {
				b.ActiveTab = 0
			}
		}

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

		b.clampScrollOffset()
		b.rebuildNormalHints()
		if b.detailFocused {
			b.statusBar.SetActionHints(detailFocusHints)
		} else {
			b.statusBar.SetActionHints(b.normalHints)
		}
		cmd := b.statusBar.SetTimedMessage("Board refreshed", statusMessageDuration)
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
	var cmd tea.Cmd
	b.rebuildNormalHints()
	b.statusBar.SetActionHints(b.normalHints)
	if b.loaded {
		cmd = b.statusBar.SetTimedMessage("Board refreshed", statusMessageDuration)
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
			m[card.Number] = prevCardInfo{
				colIdx: i,
				title:  card.Title,
				labels: card.Labels,
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
		vars := action.BuildTemplateVars(cardNum, prev.title, prev.labels, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen)
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
	newCard := Card{
		Number:    msg.card.Number,
		Title:     msg.card.Title,
		Labels:    msg.card.Labels,
		Body:      msg.card.Body,
		LinkedPRs: mapLinkedPRs(msg.card.LinkedPRs),
	}
	b.Columns[0].Cards = append(b.Columns[0].Cards, newCard)
	b.create.titleInput.SetValue("")
	b.create.labelInput.SetValue("")
	b.validationErr = ""
	b.mode = normalMode
	return b, nil
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
		cmd := b.create.titleInput.Focus()
		b.create.labelInput.Blur()
		return b, cmd
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
		return b.handlePROpenKey(col.Cards[col.Cursor])
	case "o":
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
		if col.Cursor < len(col.Cards)-1 {
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
		// Check if it's a custom action key.
		if act, ok := b.resolveAction(msg.String()); ok {
			col := b.Columns[b.ActiveTab]
			if len(col.Cards) == 0 {
				return b, nil
			}
			return b.handleActionKey(act, col.Cards[col.Cursor])
		}
	}
	return b, nil
}

func (b Board) handlePROpenKey(card Card) (tea.Model, tea.Cmd) {
	switch len(card.LinkedPRs) {
	case 0:
		cmd := b.statusBar.SetTimedMessage("No linked PRs", statusMessageDuration)
		return b, cmd
	case 1:
		pr := card.LinkedPRs[0]
		if err := b.executor.OpenURL(pr.URL); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), statusMessageDuration)
			return b, cmd
		}
		cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Opened PR #%d", pr.Number), statusMessageDuration)
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
		cmd := b.statusBar.SetTimedMessage("Repository URL not available for this provider", statusMessageDuration)
		return b, cmd
	}
	if b.repoOwner == "" || b.repoName == "" {
		cmd := b.statusBar.SetTimedMessage("Repository info not available", statusMessageDuration)
		return b, cmd
	}
	url := "https://github.com/" + b.repoOwner + "/" + b.repoName
	if err := b.executor.OpenURL(url); err != nil {
		cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), statusMessageDuration)
		return b, cmd
	}
	cmd := b.statusBar.SetTimedMessage("Opened repository", statusMessageDuration)
	return b, cmd
}

func (b Board) handleActionKey(act config.Action, card Card) (tea.Model, tea.Cmd) {
	vars := action.BuildTemplateVars(card.Number, card.Title, card.Labels, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen)

	switch act.Type {
	case "url":
		expanded := action.ExpandTemplate(act.URL, action.BuildURLSafeVars(vars))
		if err := b.executor.OpenURL(expanded); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), statusMessageDuration)
			return b, cmd
		}
		return b, nil
	case "shell":
		expanded := action.ExpandTemplate(act.Command, action.BuildShellSafeVars(vars))
		cmd := b.statusBar.SetTimedMessage("Running...", longStatusMessageDuration)
		return b, tea.Batch(cmd, runShellCmd(b.executor, expanded))
	}
	return b, nil
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
	case "r":
		if b.refreshing {
			return b, nil
		}
		b.pendingAutoRefresh = false
		b.refreshing = true
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
	case "o":
		return b.handleRepoOpenKey()
	case "h", "left":
		b.detailFocused = false
		b.statusBar.SetActionHints(b.normalHints)
	case "j", "down":
		col := b.Columns[b.ActiveTab]
		if len(col.Cards) > 0 {
			card := col.Cards[col.Cursor]
			if card.Body != "" {
				rendered := renderBody(card.Body)
				maxLines := len(strings.Split(rendered, "\n"))
				panelHeight, _, rightContentWidth := b.layoutDimensions()
				headerLines := detailHeaderLineCount(card, rightContentWidth)
				availableBodyLines := panelHeight - headerLines
				if availableBodyLines < 1 {
					availableBodyLines = 1
				}
				// Account for up-arrow indicator when scrolled.
				if b.detailScrollOffset > 0 {
					availableBodyLines--
					if availableBodyLines < 1 {
						availableBodyLines = 1
					}
				}
				maxOffset := maxLines - availableBodyLines
				if maxOffset < 0 {
					maxOffset = 0
				}
				if b.detailScrollOffset < maxOffset {
					b.detailScrollOffset++
				}
			}
		}
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

func (b *Board) switchColumn(idx int) {
	b.ActiveTab = idx
	b.Columns[b.ActiveTab].ScrollOffset = 0
	b.detailScrollOffset = 0
	b.clampScrollOffset()
	b.rebuildNormalHints()
	b.statusBar.SetActionHints(b.normalHints)
}

func (b Board) handlePRPickerModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	col := b.Columns[b.ActiveTab]
	card := col.Cards[col.Cursor]
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
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), statusMessageDuration)
			return b, cmd
		}
		cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Opened PR #%d", pr.Number), statusMessageDuration)
		return b, cmd
	}
	return b, nil
}

