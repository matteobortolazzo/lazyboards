package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (b Board) handleMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown:
		return b.handleMouseWheel(msg)
	case tea.MouseButtonLeft:
		// Left-click stays gated to normalMode -- clicks never leak into
		// modals, unlike wheel scrolling which is extended to all of them.
		if msg.Action == tea.MouseActionPress && b.mode == normalMode {
			return b.handleMouseClick(msg)
		}
	}
	return b, nil
}

// handleMouseWheel routes a wheel event by mode, mirroring each surface's
// keyboard navigation model: normalMode scrolls the card list/detail panel,
// the list modals move their row cursor (clamped, not wrapped, unlike their
// j/k handlers), filterMode skips header rows while clamping, helpMode
// scrolls its viewport, and every other mode (no scrollable content) is a
// no-op.
func (b Board) handleMouseWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch b.mode {
	case normalMode:
		return b.handleMouseScroll(msg)
	case helpMode:
		return b.handleHelpWheel(msg)
	case prListMode:
		return b.handlePRListWheel(msg)
	case agentListMode:
		return b.handleAgentListWheel(msg)
	case assignMode:
		return b.handleAssignWheel(msg)
	case gitPanelMode:
		return b.handleGitPanelWheel(msg)
	case prPickerMode:
		return b.handlePRPickerWheel(msg)
	case filterMode:
		b.filterMoveClamp(msg.Button == tea.MouseButtonWheelDown)
		return b, nil
	}
	return b, nil
}

func (b Board) handleHelpWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Button == tea.MouseButtonWheelDown {
		maxOffset := b.helpMaxScrollOffset()
		if b.helpScrollOffset < maxOffset {
			b.helpScrollOffset++
		}
	} else if b.helpScrollOffset > 0 {
		b.helpScrollOffset--
	}
	return b, nil
}

func (b Board) handlePRListWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	b.prList.cursor = clampCursor(b.prList.cursor, len(b.prList.entries), msg.Button == tea.MouseButtonWheelDown)
	return b, nil
}

func (b Board) handleAgentListWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Recompute live rather than using a cached length: agentListEntries()
	// reflects the current tmux/cenci state, which can change between
	// renders (see docs/list-cursor-invariants.md).
	entries := b.agentListEntries()
	b.agentList.cursor = clampCursor(b.agentList.cursor, len(entries), msg.Button == tea.MouseButtonWheelDown)
	return b, nil
}

func (b Board) handleAssignWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	b.assign.cursor = clampCursor(b.assign.cursor, len(b.assign.items), msg.Button == tea.MouseButtonWheelDown)
	return b, nil
}

func (b Board) handleGitPanelWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	b.gitPanel.cursor = clampCursor(b.gitPanel.cursor, len(b.gitPanel.items), msg.Button == tea.MouseButtonWheelDown)
	return b, nil
}

func (b Board) handlePRPickerWheel(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	// Read LinkedPRs live rather than a cached count: an async board refresh
	// can shrink the selected card's PRs while the picker is open, mirroring
	// the defensive clamp in handlePRPickerModeKey (mode_handlers.go).
	prCount := len(b.selectedCard().LinkedPRs)
	// prCount == 0 mirrors handlePRPickerModeKey: no valid index exists at
	// all, so close the picker instead of clamping to a nonexistent element.
	if prCount == 0 {
		b.closePRPickerNoPRs()
		return b, nil
	}
	b.prPickerIndex = clampCursor(b.prPickerIndex, prCount, msg.Button == tea.MouseButtonWheelDown)
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
		b.onCursorMoved()
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
			countStr = fmt.Sprintf("(%d/%d) ●", fc, len(col.Cards))
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
		lines := b.cardLineCount(cards[i], leftContentWidth, columnNames)
		if msg.Y >= currentY && msg.Y < currentY+lines {
			col.Cursor = i
			b.onCursorMoved()
			return b, nil
		}
		currentY += lines
	}

	return b, nil
}
