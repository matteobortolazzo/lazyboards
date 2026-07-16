package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

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
	// A pending custom-action key sequence consumes every key until it
	// resolves or cancels -- checked before the detail-focused sub-state so
	// sequences behave identically in both focuses.
	if b.pendingSeq != "" {
		return b.handlePendingSeqKey(msg)
	}

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
	case "t":
		if len(b.Columns) == 0 {
			return b, nil
		}
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		card := b.selectedCard()
		if len(card.LinkedPRs) > 0 {
			cmd := b.statusBar.SetTimedMessage("Delete is not supported for cards with linked PRs", StatusError, statusMessageDuration)
			return b, cmd
		}
		ci := textinput.New()
		ci.Placeholder = "Optional comment..."
		ci.CharLimit = 2000
		b.delete = deleteState{
			card:         card,
			step:         deleteStepComment,
			commentInput: ci,
		}
		b.mode = deleteMode
		b.statusBar.SetActionHints(deleteCommentHints)
		return b, b.delete.commentInput.Focus()
	case "v":
		b.enterPRList()
		return b, fetchOpenPRsCmd(b.provider, b.prList.generation)
	case "w":
		b.enterAgentList()
		return b, nil
	case "s":
		if len(b.Columns) == 0 {
			return b, nil
		}
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		return b.handleAgentJumpKey(b.selectedCard())
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
		b.onCursorMoved()
	case "k", "up":
		col := &b.Columns[b.ActiveTab]
		if col.Cursor > 0 {
			col.Cursor--
		}
		b.onCursorMoved()
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
	case "u":
		// Capture the selected card's identity (filter-aware) before flipping
		// the sort order, so the cursor can be restored to the same card by
		// Number afterward rather than by raw index (#412;
		// docs/list-cursor-invariants.md).
		hasSelection := len(b.visibleCards()) > 0
		selectedNumber := 0
		if hasSelection {
			selectedNumber = b.selectedCard().Number
		}

		b.sortNewestFirst = !b.sortNewestFirst
		b.sortColumns()

		if len(b.Columns) > 0 && b.ActiveTab < len(b.Columns) {
			col := &b.Columns[b.ActiveTab]
			visible := b.visibleCards()
			found := false
			if hasSelection {
				for i, c := range visible {
					if c.Number == selectedNumber {
						col.Cursor = i
						found = true
						break
					}
				}
			}
			if !found && col.Cursor >= len(visible) {
				col.Cursor = len(visible) - 1
				if col.Cursor < 0 {
					col.Cursor = 0
				}
			}
		}

		b.clampScrollOffset()
		b.rebuildNormalHints()
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
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
		b.assign.cursor = moveCursor(b.assign.cursor, len(b.assign.items), true)
	case "k", "up":
		b.assign.cursor = moveCursor(b.assign.cursor, len(b.assign.items), false)
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
		b.gitPanel.cursor = moveCursor(b.gitPanel.cursor, len(b.gitPanel.items), true)
		return b, nil
	case "k", "up":
		b.gitPanel.cursor = moveCursor(b.gitPanel.cursor, len(b.gitPanel.items), false)
		return b, nil
	}

	// Lazygit-style direct dispatch: pressing a menu item's key runs it
	// immediately without navigating to it first.
	if _, ok := b.defaultActions[msg.String()]; ok {
		return b.dispatchGitMenuKey(msg.String())
	}
	return b, nil
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

	// Defensive: an async board refresh can shrink the selected card's
	// LinkedPRs while the picker is still open, leaving prPickerIndex one or
	// more positions past the new end. Clamp it before any branch below reads
	// card.LinkedPRs[b.prPickerIndex].
	if b.prPickerIndex >= prCount {
		b.prPickerIndex = prCount - 1
		if b.prPickerIndex < 0 {
			b.prPickerIndex = 0
		}
	}

	// The picker can be opened from the card list or the detail panel; restore
	// the hint set matching where the user came from.
	restoreHints := func() {
		if b.detailFocused {
			b.statusBar.SetActionHints(detailFocusHints)
		} else {
			b.statusBar.SetActionHints(b.normalHints)
		}
	}

	// prCount == 0 is a separate case from the clamp above: no valid index
	// exists at all, so bail out with the same cleanup as the Escape path
	// instead of clamping to a nonexistent element.
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
		b.prList.cursor = moveCursor(b.prList.cursor, len(b.prList.entries), true)
		return b, nil
	case "k", "up":
		b.prList.cursor = moveCursor(b.prList.cursor, len(b.prList.entries), false)
		return b, nil
	}

	// Plain uppercase A-Z: global scope: pr custom actions against the
	// selected row (see handlePRListActionKey). Alt combinations are excluded:
	// the comment-action flow is normal-mode-only.
	if !msg.Alt && len(msg.Runes) == 1 && msg.Runes[0] >= 'A' && msg.Runes[0] <= 'Z' {
		return b.handlePRListActionKey(msg.String())
	}
	return b, nil
}

// handleAgentJumpKey jumps to the selected card's agent windows: a single
// matching window switches tmux directly, several open the agents modal
// scoped to the card, none reports so. Mirrors handlePROpenKey's single/multi
// split.
func (b Board) handleAgentJumpKey(card Card) (tea.Model, tea.Cmd) {
	windows := b.cardAgentWindows(card.Number)
	switch len(windows) {
	case 0:
		// Full cenciwatch state precedence, matching viewAgentListModal:
		// "no windows for this card" is only true when a daemon snapshot is
		// actually connected — otherwise report the real reason.
		msg := fmt.Sprintf("%s for #%d", agentListMsgNoWindows, card.Number)
		switch {
		case b.cenciWatcher == nil:
			msg = agentListMsgNotEnabled
		case b.agentSnapshot == nil:
			msg = agentListMsgWaiting
		}
		cmd := b.statusBar.SetTimedMessage(msg, StatusInfo, statusMessageDuration)
		return b, cmd
	case 1:
		if err := b.switchToAgentWindow(windows[0]); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
			return b, cmd
		}
		cmd := b.statusBar.SetTimedMessage("Switched to "+windows[0].WindowName, StatusSuccess, statusMessageDuration)
		return b, cmd
	default:
		b.enterAgentListForCard(card.Number)
		return b, nil
	}
}

// handleAgentListModeKey handles keys in the agents list modal. Enter closes
// the modal and switches the tmux client to the selected agent's window; the
// empty-list guard mirrors viewAgentListModal's empty/unavailable states
// (docs/view-state-consistency.md).
func (b Board) handleAgentListModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	entries := b.agentListEntries()
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	case tea.KeyEnter:
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		if len(entries) == 0 || b.agentList.cursor >= len(entries) {
			return b, nil
		}
		w := entries[b.agentList.cursor].window
		if err := b.switchToAgentWindow(w); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
			return b, cmd
		}
		cmd := b.statusBar.SetTimedMessage("Switched to "+w.WindowName, StatusSuccess, statusMessageDuration)
		return b, cmd
	}

	switch msg.String() {
	case "j", "down":
		b.agentList.cursor = moveCursor(b.agentList.cursor, len(entries), true)
	case "k", "up":
		b.agentList.cursor = moveCursor(b.agentList.cursor, len(entries), false)
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

// handleDeleteModeKey drives the two-step delete-confirm flow: an optional
// comment step, then a retype-to-confirm step. Esc cancels the whole flow
// (discarding any comment typed in step 1) from either step. The
// retype-to-confirm step only accepts an exact string match of the card
// number; a mismatch shows an inline message and remains in the step rather
// than auto-cancelling.
func (b Board) handleDeleteModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc {
		b.mode = normalMode
		b.statusBar.SetActionHints(b.normalHints)
		cmd := b.statusBar.SetTimedMessage("Delete cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	}

	switch b.delete.step {
	case deleteStepComment:
		if msg.Type == tea.KeyEnter {
			ci := textinput.New()
			ci.Placeholder = strconv.Itoa(b.delete.card.Number)
			ci.CharLimit = 20
			b.delete.step = deleteStepConfirm
			b.delete.confirmInput = ci
			b.delete.mismatchMsg = ""
			b.statusBar.SetActionHints(deleteConfirmHints)
			return b, b.delete.confirmInput.Focus()
		}
		var cmd tea.Cmd
		b.delete.commentInput, cmd = b.delete.commentInput.Update(msg)
		return b, cmd
	case deleteStepConfirm:
		if msg.Type == tea.KeyEnter {
			card := b.delete.card
			if b.delete.confirmInput.Value() != strconv.Itoa(card.Number) {
				b.delete.mismatchMsg = fmt.Sprintf("Doesn't match #%d — try again or Esc to cancel", card.Number)
				return b, nil
			}
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			comment := strings.TrimSpace(b.delete.commentInput.Value())
			if comment != "" {
				return b, addCommentForDeleteCmd(b.provider, card, comment)
			}
			return b, deleteCardCmd(b.provider, card, false)
		}
		var cmd tea.Cmd
		b.delete.confirmInput, cmd = b.delete.confirmInput.Update(msg)
		return b, cmd
	}
	return b, nil
}
