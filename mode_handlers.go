package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// handleCreateModeKey routes create mode through the ModeCreate registry
// (textBinding, keymap_text.go): a resolved create.* command that's also
// currently eligible (createCommandActive -- the assignee cycle's fifth
// condition, gated on b.create.focus == 2 and a non-empty
// assigneeOptions) dispatches through runCreateCommand. Anything else --
// no match, OutcomePending, a non-command binding, a foreign command id, or
// a resolved-but-ineligible assignee-cycle command -- falls through to the
// focused field below, transcribed verbatim from the pre-#540 `default:`
// branch minus the hardcoded `case "left"/"right"` assignee-cycle switch:
// that logic now lives exclusively behind the eligibility-gated command
// path above, so an unbound/remapped-away left/right at focus 2 is a
// genuine no-op (mirroring TestCreateMode_UnbindAssigneeCycleKeys_
// LeftRightNoOpAtFocus2) instead of silently cycling via raw string
// matching.
func (b Board) handleCreateModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if binding, ok := b.textBinding(keymap.ModeCreate, msg); ok && binding.Kind == keymap.BindingCommand {
		switch binding.Command {
		case keymap.CommandCreateSubmit, keymap.CommandCreateCancel, keymap.CommandCreateNextField,
			keymap.CommandCreateAssigneePrev, keymap.CommandCreateAssigneeNext:
			if b.createCommandActive(binding.Command) {
				return b.runCreateCommand(binding.Command)
			}
		}
	}

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
	case 2: // assignee focused -- no textinput to forward to; the
		// assignee cycle only ever dispatches through the eligibility-
		// gated command path above, so any other key here is a no-op.
	}
	return b, cmd
}

// handleConfigModeKey routes config mode through the ModeConfig registry
// (textBinding, keymap_text.go): a resolved config.* command that's also
// currently eligible (configCommandActive -- the provider cycle's fifth
// condition, gated on b.config.focus == 0) dispatches through
// runConfigCommand. A resolved-but-ineligible provider-cycle command (left/
// right at focus == 1, repo input focused) is explicitly swallowed rather
// than forwarded to repoInput -- unlike create's default-branch text-
// forwarding shape, config's left/right were dedicated top-level cases in
// the pre-#540 handler that already returned (b, nil) whenever focus != 0,
// so this preserves that byte-identical swallow (the #540 plan's Q&A #2
// behavior-parity decision; TestConfigMode_LeftRight_AtRepoFocus_
// DoesNotReachInput is the named regression guard -- do not "fix" this to
// forward to repoInput). Anything else -- no match, OutcomePending, a
// non-command binding, or a foreign command id -- falls through to the
// verbatim pre-#540 `default:` branch below.
func (b Board) handleConfigModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if binding, ok := b.textBinding(keymap.ModeConfig, msg); ok && binding.Kind == keymap.BindingCommand {
		switch binding.Command {
		case keymap.CommandConfigSave, keymap.CommandConfigCancel, keymap.CommandConfigNextField,
			keymap.CommandConfigProviderPrev, keymap.CommandConfigProviderNext:
			if b.configCommandActive(binding.Command) {
				return b.runConfigCommand(binding.Command)
			}
			return b, nil
		}
	}

	if b.config.focus == 1 {
		var cmd tea.Cmd
		b.config.repoInput, cmd = b.config.repoInput.Update(msg)
		return b, cmd
	}
	return b, nil
}

func (b Board) handleNormalModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// A pending reference-navigation prompt consumes every key until it
	// resolves or cancels -- checked first (ahead of pendingSeq and the
	// detail-focused sub-state) so it covers both entry points identically.
	if len(b.pendingRefs) > 0 {
		return b.handlePendingRefKey(msg)
	}

	// A pending custom-action key sequence consumes every key until it
	// resolves or cancels -- checked before the detail-focused sub-state so
	// sequences behave identically in both focuses.
	if b.pendingSeq != "" {
		return b.handlePendingSeqKey(msg)
	}

	if b.detailFocused {
		return b.handleDetailFocusedKey(msg)
	}

	return b.dispatchKey(keymap.ModeNormal, msg)
}

// runNormalCommand runs the built-in normal-mode command id resolves to.
// Case bodies are transcribed verbatim from the pre-#489 switch msg.String()
// (moved here unchanged, guard-for-guard) -- see docs/list-cursor-
// invariants.md for the 'u' sort-order cursor restore and
// docs/bubbletea-async-patterns.md for the tea.Cmd propagation both keep.
func (b Board) runNormalCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	if idx, ok := navColumnIndex(id); ok {
		if idx < len(b.Columns) {
			b.Columns[idx].Cursor = 0
			b.switchColumn(idx)
		}
		return b, nil
	}

	switch id {
	case keymap.CommandQuit:
		return b, tea.Quit
	case keymap.CommandCardNew:
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
			var tail []string
			for _, c := range b.collaborators {
				if !strings.EqualFold(c.Login, b.authenticatedUser) {
					tail = append(tail, c.Login)
				}
			}
			sortFoldStrings(tail)
			options = append(options, tail...)
			b.create.assigneeOptions = options
		} else {
			b.create.assigneeOptions = nil
		}
		b.recalcCreateInputs()
		cmd := b.create.titleInput.Focus()
		b.create.labelInput.Blur()
		return b, cmd
	case keymap.CommandCardEdit:
		if len(b.Columns) == 0 {
			return b, nil
		}
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		return b, openEditorCmd(b.selectedCard())
	case keymap.CommandConfig:
		b.enterConfigMode()
	case keymap.CommandBoardRefresh:
		if b.refreshing {
			return b, nil
		}
		b.refreshing = true
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider, true))
	case keymap.CommandCardOpenPR:
		if len(b.Columns) == 0 {
			return b, nil
		}
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		return b.handlePROpenKey(b.selectedCard())
	case keymap.CommandCardClose:
		if len(b.Columns) == 0 {
			return b, nil
		}
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		b.closeConfirm = closeConfirmState{card: b.selectedCard()}
		b.mode = closeConfirmMode
		return b, nil
	case keymap.CommandCardDelete:
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
		b.statusBar.SetActionHints(b.deleteCommentHints())
		return b, b.delete.commentInput.Focus()
	case keymap.CommandViewPRList:
		b.enterPRList()
		return b, fetchOpenPRsCmd(b.provider, b.prList.generation)
	case keymap.CommandViewMilestoneList:
		b.enterMilestoneList()
		return b, fetchMilestonesCmd(b.provider, b.milestoneList.generation)
	case keymap.CommandViewAgentList:
		b.enterAgentList()
		return b, nil
	case keymap.CommandNavAgent:
		if len(b.Columns) == 0 {
			return b, nil
		}
		if len(b.visibleCards()) == 0 {
			return b, nil
		}
		return b.handleAgentJumpKey(b.selectedCard())
	case keymap.CommandBoardSearch:
		b.mode = searchMode
		cmd := b.searchInput.Focus()
		b.statusBar.SetActionHints(b.searchHints())
		return b, cmd
	case keymap.CommandCardOpenTicket:
		return b.handleTicketOpenKey()
	case keymap.CommandNavReference:
		return b.handleReferenceNavKey()
	case keymap.CommandNavDetailFocus:
		b.detailFocused = true
		b.rebuildDetailHints()
	case keymap.CommandNavColumnPrev:
		b.switchColumn((b.ActiveTab - 1 + len(b.Columns)) % len(b.Columns))
	case keymap.CommandNavColumnNext:
		b.switchColumn((b.ActiveTab + 1) % len(b.Columns))
	case keymap.CommandNavCursorDown:
		col := &b.Columns[b.ActiveTab]
		col.Cursor = moveCursor(col.Cursor, len(b.visibleCards()), true)
		b.onCursorMoved()
	case keymap.CommandNavCursorUp:
		col := &b.Columns[b.ActiveTab]
		col.Cursor = moveCursor(col.Cursor, len(b.visibleCards()), false)
		b.onCursorMoved()
	case keymap.CommandCardAssign:
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
		start := 0
		if b.authenticatedUser != "" {
			items = append(items, assignItem{
				login:      b.authenticatedUser,
				isAssigned: assignedSet[strings.ToLower(b.authenticatedUser)],
				isMe:       true,
			})
			start = 1
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
		sortFold(items[start:], func(it assignItem) string { return it.login })

		b.assign = assignState{items: items, cursor: 0}
		b.mode = assignMode
		b.statusBar.SetActionHints(b.assignHints())
		return b, nil
	case keymap.CommandBoardFilter:
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
		b.statusBar.SetActionHints(b.filterHints())
		return b, nil
	case keymap.CommandHelp:
		b.helpFromDetailFocused = false
		b.helpScrollOffset = 0
		b.mode = helpMode
		b.statusBar.SetActionHints(b.helpHints())
		return b, nil
	case keymap.CommandViewGitPanel:
		b.enterGitPanel()
		return b, nil
	case keymap.CommandViewDispatch:
		b.dispatch = dispatchState{loading: true}
		b.mode = dispatchMode
		return b, queryDispatchStatusCmd(b.executor)
	case keymap.CommandBoardSortOrder:
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
		// The re-sort itself is synchronous; only remembering the choice for
		// the next launch is async (#503). With no state path there is
		// nowhere to save, so the toggle stays session-only.
		if b.statePath == "" {
			return b, nil
		}
		return b, saveSortOrderCmd(b.statePath, b.sortNewestFirst)
	}
	return b, nil
}

// handleCommentModeKey routes comment mode through the ModeComment registry
// (textBinding, keymap_text.go): a resolved comment.* command dispatches
// through runCommentCommand -- neither of comment's two command ids is
// focus-gated, so there is no eligibility predicate to check, unlike
// create/config. Anything else -- no match, OutcomePending, a non-command
// binding, or a foreign command id -- falls through to the comment.input
// textinput below, transcribed verbatim from the pre-#540 `default:` branch.
func (b Board) handleCommentModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if binding, ok := b.textBinding(keymap.ModeComment, msg); ok && binding.Kind == keymap.BindingCommand {
		switch binding.Command {
		case keymap.CommandCommentSubmit, keymap.CommandCommentCancel:
			return b.runCommentCommand(binding.Command)
		}
	}

	var cmd tea.Cmd
	b.comment.input, cmd = b.comment.input.Update(msg)
	return b, cmd
}

func (b Board) handleFilterModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	result := b.lookupModalBinding(keymap.ModeFilter, msg)
	switch result.Outcome {
	case keymap.OutcomePending:
		// Q4: filter mode has no multi-key pending-sequence machinery; a key
		// that is only a prefix of a bound sequence is a silent no-op.
		return b, nil
	case keymap.OutcomeMatch:
		if result.Binding.Kind != keymap.BindingCommand {
			// A user config can legally bind an inline action inside
			// keymaps.filter (validation is not mode-scoped); this modal has
			// no inline-action dispatch path, so treat anything that isn't a
			// built-in command as a no-op instead of falling through to the
			// zero-value Command case below.
			return b, nil
		}
		switch result.Binding.Command {
		case keymap.CommandFilterClose:
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			return b, nil
		case keymap.CommandFilterSelect:
			if b.filterCursor < len(b.filterItems) && !b.filterItems[b.filterCursor].isHeader {
				item := b.filterItems[b.filterCursor]
				b.applyFilter(item.itemType, item.value)
			}
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			return b, nil
		case keymap.CommandFilterNext:
			b.filterMoveDown()
		case keymap.CommandFilterPrev:
			b.filterMoveUp()
		default:
			// Q4: a command id from a different domain (e.g. a stray
			// override binding board.refresh here) is not recognized inside
			// filter mode and must not dispatch.
		}
	}
	return b, nil
}

func (b Board) handleAssignModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	result := b.lookupModalBinding(keymap.ModeAssign, msg)
	switch result.Outcome {
	case keymap.OutcomePending:
		// Q4: assign mode has no multi-key pending-sequence machinery; a key
		// that is only a prefix of a bound sequence is a silent no-op.
		return b, nil
	case keymap.OutcomeMatch:
		if result.Binding.Kind != keymap.BindingCommand {
			// A user config can legally bind an inline action inside
			// keymaps.assign (validation is not mode-scoped); this modal has
			// no inline-action dispatch path, so treat anything that isn't a
			// built-in command as a no-op instead of falling through to the
			// zero-value Command case below.
			return b, nil
		}
		switch result.Binding.Command {
		case keymap.CommandAssignClose:
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			return b, nil
		case keymap.CommandAssignToggle:
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
		case keymap.CommandAssignNext:
			b.assign.cursor = moveCursor(b.assign.cursor, len(b.assign.items), true)
		case keymap.CommandAssignPrev:
			b.assign.cursor = moveCursor(b.assign.cursor, len(b.assign.items), false)
		default:
			// Q4: a command id from a different domain (e.g. a stray
			// override binding board.refresh here) is not recognized inside
			// assign mode and must not dispatch.
		}
	}
	return b, nil
}

// handleGitPanelKey routes a git panel key through the ModeGitPanel registry
// table (#511): a resolved command runs through runGitPanelCommand
// (close/run-selected/next/prev, lazygit-style navigation and Enter-to-run),
// a resolved inline action dispatches directly through dispatchGitMenuAction
// (lazygit-style direct dispatch -- pressing a menu item's own key runs it
// immediately without navigating to it first), and anything unresolved
// (OutcomePending or OutcomeNoMatch -- panelBinding is single-key exact
// match only) is a silent no-op.
func (b Board) handleGitPanelKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	binding, ok := b.panelBinding(keymap.ModeGitPanel, msg)
	if !ok {
		return b, nil
	}

	switch binding.Kind {
	case keymap.BindingCommand:
		return b.runGitPanelCommand(binding.Command)
	case keymap.BindingAction:
		return b.dispatchGitMenuAction(binding.Action)
	}
	return b, nil
}

// handleDispatchModeKey handles key presses while the agent dispatch modal
// is open, dispatching through the ModeDispatch registry table
// (keymap_panels.go's handleDispatchModalKey, #511 PR 2/2). The modal is
// rendered by viewDispatchModal (#284).
func (b Board) handleDispatchModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return b.handleDispatchModalKey(msg)
}

// handleSearchModeKey routes search mode through the ModeSearch registry
// (textBinding, keymap_text.go): a resolved search.* command dispatches
// through runSearchCommand -- none of search's six command ids is focus-
// gated, so there is no eligibility predicate to check, unlike
// create/config. Anything else -- no match, OutcomePending, a non-command
// binding, or a foreign command id -- falls through to the searchInput
// textinput below, transcribed verbatim from the pre-#540 `default:`
// branch: queries can contain any character -- card-number search ("42")
// and titles with j/k must stay typeable.
func (b Board) handleSearchModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if binding, ok := b.textBinding(keymap.ModeSearch, msg); ok && binding.Kind == keymap.BindingCommand {
		switch binding.Command {
		case keymap.CommandSearchApply, keymap.CommandSearchCancel, keymap.CommandSearchNextResult,
			keymap.CommandSearchPrevResult, keymap.CommandSearchNextColumn, keymap.CommandSearchPrevColumn:
			return b.runSearchCommand(binding.Command)
		}
	}

	var cmd tea.Cmd
	b.searchInput, cmd = b.searchInput.Update(msg)
	b.searchQuery = b.searchInput.Value()
	// Reset cursor and scroll offset when filter changes.
	col := &b.Columns[b.ActiveTab]
	col.Cursor = 0
	col.ScrollOffset = 0
	b.statusBar.SetActionHints(b.searchHints())
	return b, cmd
}

// closePRPickerNoPRs exits the PR picker back to normalMode when the
// selected card no longer has any linked PRs to show (e.g. an async board
// refresh cleared LinkedPRs while the picker was open). Shared by the
// keyboard (handlePRPickerModeKey) and wheel (handlePRPickerWheel) handlers
// so both apply the exact same cleanup: clear pendingPRAction and restore
// the hint set matching where the picker was opened from.
func (b *Board) closePRPickerNoPRs() {
	b.mode = normalMode
	b.pendingPRAction = nil
	if b.detailFocused {
		b.rebuildDetailHints()
	} else {
		b.statusBar.SetActionHints(b.normalHints)
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
			b.rebuildDetailHints()
		} else {
			b.statusBar.SetActionHints(b.normalHints)
		}
	}

	// prCount == 0 is a separate case from the clamp above: no valid index
	// exists at all, so bail out with the same cleanup as the Escape path
	// instead of clamping to a nonexistent element.
	if prCount == 0 {
		b.closePRPickerNoPRs()
		return b, nil
	}

	result := b.lookupModalBinding(keymap.ModePRPicker, msg)
	switch result.Outcome {
	case keymap.OutcomePending:
		// Q4: the PR picker has no multi-key pending-sequence machinery; a
		// key that is only a prefix of a bound sequence is a silent no-op.
		return b, nil
	case keymap.OutcomeMatch:
		if result.Binding.Kind != keymap.BindingCommand {
			// A user config can legally bind an inline action inside
			// keymaps.pr_picker (validation is not mode-scoped); this modal
			// has no inline-action dispatch path, so treat anything that
			// isn't a built-in command as a no-op instead of falling through
			// to the zero-value Command case below.
			return b, nil
		}
		switch result.Binding.Command {
		case keymap.CommandPRPickerClose:
			b.mode = normalMode
			b.pendingPRAction = nil
			restoreHints()
			return b, nil
		case keymap.CommandPRPickerPrev:
			b.prPickerIndex = (b.prPickerIndex - 1 + prCount) % prCount
			return b, nil
		case keymap.CommandPRPickerNext:
			b.prPickerIndex = (b.prPickerIndex + 1) % prCount
			return b, nil
		case keymap.CommandPRPickerSelect:
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
		default:
			// Q4: a command id from a different domain (e.g. a stray
			// override binding board.refresh here) is not recognized inside
			// the PR picker and must not dispatch.
		}
	}
	return b, nil
}

// handlePRListModeKey routes a global PR list key through the ModePRList
// registry table (#490 PR 7b), mirroring handleFilterModeKey/
// handleAssignModeKey's shape. Unlike those two modals, an inline action
// bound under keymaps.pr_list DOES dispatch here -- gated to scope: pr (Q2),
// mirroring the deleted raw A-Z scan's own scope check -- against the
// selected row via runPRListAction.
func (b Board) handlePRListModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	result := b.lookupModalBinding(keymap.ModePRList, msg)
	switch result.Outcome {
	case keymap.OutcomePending:
		// Q4: the PR list has no multi-key pending-sequence machinery; a key
		// that is only a prefix of a bound sequence is a silent no-op.
		return b, nil
	case keymap.OutcomeMatch:
		if result.Binding.Kind == keymap.BindingAction {
			act := configActionFromKeymap(result.Binding.Action)
			if config.DefaultScope(act.Scope) != "pr" {
				// Q2: only global scope: pr actions may dispatch inside the
				// PR list -- it is a repo-wide row view with no sensible
				// board/card target for any other scope.
				return b, nil
			}
			return b.runPRListAction(act)
		}
		switch result.Binding.Command {
		case keymap.CommandPRListClose:
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			return b, nil
		case keymap.CommandPRListOpen:
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
		case keymap.CommandPRListNext:
			b.prList.cursor = moveCursor(b.prList.cursor, len(b.prList.entries), true)
		case keymap.CommandPRListPrev:
			b.prList.cursor = moveCursor(b.prList.cursor, len(b.prList.entries), false)
		default:
			// Q4: a command id from a different domain (e.g. a stray
			// override binding board.refresh here) is not recognized inside
			// the PR list and must not dispatch.
		}
	}
	return b, nil
}

// milestoneStatusTitleMaxLen bounds a milestone title before it reaches a
// timed status-bar message: unlike the modal's row text (which the terminal
// width already bounds), timed messages render untruncated
// (statusbar.go), so an untrusted GitHub title must be capped here too.
const milestoneStatusTitleMaxLen = 60

// handleMilestoneListModeKey routes a Milestones modal (i) key through the
// ModeMilestoneList registry table (#490 PR 7b), mirroring
// handleFilterModeKey/handleAssignModeKey's shape -- this modal has no
// inline-action dispatch path (Q4). Enter closes the modal and, mirroring
// handlePRListModeKey's "close first, then guard" shape, applies the
// selected milestone as the board filter (applyFilter) only when entries is
// non-empty and cursor is in range (loading/err/empty all leave entries nil,
// per milestoneListState's doc comment, so this one guard covers every
// non-loaded state uniformly) -- the modal always closes on the bound key,
// but no filter or status message is applied when there is nothing to
// select. Esc cancels without applying anything. o opens the selected
// milestone's URL and leaves the modal open, mirroring
// handleTicketOpenKey's empty-URL guard ("URL not available",
// StatusWarning).
func (b Board) handleMilestoneListModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	result := b.lookupModalBinding(keymap.ModeMilestoneList, msg)
	switch result.Outcome {
	case keymap.OutcomePending:
		// Q4: the Milestones modal has no multi-key pending-sequence
		// machinery; a key that is only a prefix of a bound sequence is a
		// silent no-op.
		return b, nil
	case keymap.OutcomeMatch:
		if result.Binding.Kind != keymap.BindingCommand {
			// A user config can legally bind an inline action inside
			// keymaps.milestone_list (validation is not mode-scoped); this
			// modal has no inline-action dispatch path, so treat anything
			// that isn't a built-in command as a no-op instead of falling
			// through to the zero-value Command case below.
			return b, nil
		}
		switch result.Binding.Command {
		case keymap.CommandMilestoneListClose:
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			return b, nil
		case keymap.CommandMilestoneListFilter:
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			if len(b.milestoneList.entries) == 0 || b.milestoneList.cursor >= len(b.milestoneList.entries) {
				return b, nil
			}
			m := b.milestoneList.entries[b.milestoneList.cursor]
			b.applyFilter(filterByMilestone, m.Title)
			title := truncateOutput(sanitizeSingleLine(m.Title), milestoneStatusTitleMaxLen)
			cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Filtered by milestone: %s", title), StatusSuccess, statusMessageDuration)
			return b, cmd
		case keymap.CommandMilestoneListNext:
			b.milestoneList.cursor = moveCursor(b.milestoneList.cursor, len(b.milestoneList.entries), true)
		case keymap.CommandMilestoneListPrev:
			b.milestoneList.cursor = moveCursor(b.milestoneList.cursor, len(b.milestoneList.entries), false)
		case keymap.CommandMilestoneListOpen:
			if len(b.milestoneList.entries) == 0 || b.milestoneList.cursor >= len(b.milestoneList.entries) {
				return b, nil
			}
			m := b.milestoneList.entries[b.milestoneList.cursor]
			title := truncateOutput(sanitizeSingleLine(m.Title), milestoneStatusTitleMaxLen)
			if m.URL == "" {
				cmd := b.statusBar.SetTimedMessage("URL not available", StatusWarning, statusMessageDuration)
				return b, cmd
			}
			if err := b.executor.OpenURL(m.URL); err != nil {
				cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
				return b, cmd
			}
			cmd := b.statusBar.SetTimedMessage(fmt.Sprintf("Opened milestone %s", title), StatusSuccess, statusMessageDuration)
			return b, cmd
		default:
			// Q4: a command id from a different domain (e.g. a stray
			// override binding board.refresh here) is not recognized inside
			// the Milestones modal and must not dispatch.
		}
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

// handleAgentListModeKey routes an agents list modal key through the
// ModeAgentList registry table (#490 PR 7b), mirroring handleFilterModeKey/
// handleAssignModeKey's shape -- this modal has no inline-action dispatch
// path (Q4). entries is derived live from the streamed snapshot BEFORE
// dispatch, exactly as before the conversion (agentListState's doc comment):
// a snapshot arriving mid-modal can shrink the row count, so every branch
// below must re-clamp against this fresh entries, not a stale count. Enter
// closes the modal and switches the tmux client to the selected agent's
// window; the empty-list guard mirrors viewAgentListModal's
// empty/unavailable states (docs/view-state-consistency.md).
func (b Board) handleAgentListModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	entries := b.agentListEntries()

	result := b.lookupModalBinding(keymap.ModeAgentList, msg)
	switch result.Outcome {
	case keymap.OutcomePending:
		// Q4: the agents list has no multi-key pending-sequence machinery; a
		// key that is only a prefix of a bound sequence is a silent no-op.
		return b, nil
	case keymap.OutcomeMatch:
		if result.Binding.Kind != keymap.BindingCommand {
			// A user config can legally bind an inline action inside
			// keymaps.agent_list (validation is not mode-scoped); this modal
			// has no inline-action dispatch path, so treat anything that
			// isn't a built-in command as a no-op instead of falling through
			// to the zero-value Command case below.
			return b, nil
		}
		switch result.Binding.Command {
		case keymap.CommandAgentListClose:
			b.mode = normalMode
			b.statusBar.SetActionHints(b.normalHints)
			return b, nil
		case keymap.CommandAgentListGoToWindow:
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
		case keymap.CommandAgentListNext:
			b.agentList.cursor = moveCursor(b.agentList.cursor, len(entries), true)
		case keymap.CommandAgentListPrev:
			b.agentList.cursor = moveCursor(b.agentList.cursor, len(entries), false)
		default:
			// Q4: a command id from a different domain (e.g. a stray
			// override binding board.refresh here) is not recognized inside
			// the agents list and must not dispatch.
		}
	}
	return b, nil
}

// handleHelpModeKey handles key presses while the help modal is open,
// dispatching through the ModeHelp registry table (keymap_panels.go's
// panelBinding/runHelpCommand, #511 PR 2/2), mirroring handleGitPanelKey's
// shape.
func (b Board) handleHelpModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	binding, ok := b.panelBinding(keymap.ModeHelp, msg)
	if !ok || binding.Kind != keymap.BindingCommand {
		return b, nil
	}
	return b.runHelpCommand(binding.Command)
}

// handleLabelConfirmModeKey routes a key press in labelConfirmMode through
// the ModeLabelConfirm registry (textBinding, keymap_text.go), mirroring
// handleHelpModeKey's binding->dispatch->run shape (#539 PR 1/2). Unlike the
// pre-#539 handler, there is no literal `msg.Type == tea.KeyEsc` pre-check:
// esc resolves through the table onto label_confirm.cancel, the same
// command id "n" resolves to, so an explicit unbind of esc alone genuinely
// disables it (see keymap_text_test.go's EscUnbound regression coverage).
func (b Board) handleLabelConfirmModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	binding, ok := b.textBinding(keymap.ModeLabelConfirm, msg)
	if !ok || binding.Kind != keymap.BindingCommand {
		return b, nil
	}
	return b.runLabelConfirmCommand(binding.Command)
}

// runLabelConfirmCommand runs the label_confirm command id resolves to.
// Case bodies are transcribed verbatim from the pre-#539
// handleLabelConfirmModeKey (mode_handlers.go), guard for guard.
func (b Board) runLabelConfirmCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandLabelConfirmCreate:
		label := b.labelConfirm.unknownLabels[b.labelConfirm.currentIdx]
		return b, createLabelCmd(b.provider, label)
	case keymap.CommandLabelConfirmCancel:
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Edit cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	return b, nil
}

// handleCloseConfirmModeKey routes a key press in closeConfirmMode through
// the ModeCloseConfirm registry (textBinding, keymap_text.go), mirroring
// handleHelpModeKey's binding->dispatch->run shape (#539 PR 1/2). Unlike the
// pre-#539 handler, there is no literal `msg.Type == tea.KeyEsc` pre-check:
// esc resolves through the table onto close_confirm.cancel, the same
// command id "n" resolves to, so an explicit unbind of esc alone genuinely
// disables it (see keymap_text_test.go's EscUnbound regression coverage).
func (b Board) handleCloseConfirmModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	binding, ok := b.textBinding(keymap.ModeCloseConfirm, msg)
	if !ok || binding.Kind != keymap.BindingCommand {
		return b, nil
	}
	return b.runCloseConfirmCommand(binding.Command)
}

// runCloseConfirmCommand runs the close_confirm command id resolves to.
// Case bodies are transcribed verbatim from the pre-#539
// handleCloseConfirmModeKey (mode_handlers.go), guard for guard.
func (b Board) runCloseConfirmCommand(id keymap.CommandID) (tea.Model, tea.Cmd) {
	switch id {
	case keymap.CommandCloseConfirmConfirm:
		cmd := closeCardCmd(b.provider, b.closeConfirm.card.Number)
		b.mode = normalMode
		return b, cmd
	case keymap.CommandCloseConfirmCancel:
		b.mode = normalMode
		cmd := b.statusBar.SetTimedMessage("Close cancelled", StatusWarning, statusMessageDuration)
		return b, cmd
	}
	return b, nil
}

// handleDeleteModeKey drives the two-step delete-confirm flow: an optional
// comment step, then a retype-to-confirm step. It routes through the
// ModeDelete registry (textBinding, keymap_text.go) but deliberately
// diverges from close_confirm/label_confirm's binding->dispatch->run shape
// (#539 PR 2/2): those modes no-op on anything that isn't a recognized
// command, but delete's ConsumesPrintableRunes() is true and its handler
// owns every keystroke that isn't one of its two commands
// (delete.submit/delete.cancel), so any other outcome -- no match,
// OutcomePending, a resolved non-command (BindingAction) binding, or a
// command id outside that set -- must fall through to the active step's
// textinput.Update(msg) instead of silently no-oping, byte-identical to
// today's behavior where everything except esc/enter reaches the textinput.
func (b Board) handleDeleteModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	binding, ok := b.textBinding(keymap.ModeDelete, msg)
	if ok && binding.Kind == keymap.BindingCommand &&
		(binding.Command == keymap.CommandDeleteSubmit || binding.Command == keymap.CommandDeleteCancel) {
		return b.runDeleteCommand(binding.Command)
	}

	switch b.delete.step {
	case deleteStepComment:
		var cmd tea.Cmd
		b.delete.commentInput, cmd = b.delete.commentInput.Update(msg)
		return b, cmd
	case deleteStepConfirm:
		var cmd tea.Cmd
		b.delete.confirmInput, cmd = b.delete.confirmInput.Update(msg)
		return b, cmd
	}
	return b, nil
}
