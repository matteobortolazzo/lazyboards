package main

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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
		lines := cardLineCount(cards[i], leftContentWidth, columnNames, b.workingLabel, b.agentBadgeFor(cards[i]))
		if msg.Y >= currentY && msg.Y < currentY+lines {
			col.Cursor = i
			b.onCursorMoved()
			return b, nil
		}
		currentY += lines
	}

	return b, nil
}
