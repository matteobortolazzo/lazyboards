package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
)

// Package-level glamour renderer cache.
// Safe because BubbleTea is single-threaded (all View/Update calls on main goroutine).
var (
	cachedGlamourRenderer      *glamour.TermRenderer
	cachedGlamourRendererWidth int
)

func (b Board) View() string {
	if b.Width == 0 {
		return ""
	}

	if b.mode == loadingMode {
		loadingText := b.spinner.View() + " Loading board..."
		return lipgloss.Place(b.Width, b.Height, lipgloss.Center, lipgloss.Center, loadingText)
	}

	if b.mode == errorMode {
		errorText := "Error: " + b.loadErr + "\n\n" + b.statusBar.View()
		return lipgloss.Place(b.Width, b.Height, lipgloss.Center, lipgloss.Center, errorText)
	}

	if b.mode == configMode {
		return b.viewConfigModal()
	}

	if len(b.Columns) == 0 {
		return ""
	}

	// Outer border consumes 2 chars width, 2 lines height.
	innerWidth := b.Width - 2

	// Tab bar: all tab names joined horizontally.
	var tabs []string
	for i, col := range b.Columns {
		if i == b.ActiveTab {
			tabs = append(tabs, activeTabStyle.Render("["+col.Title+"]"))
		} else {
			tabs = append(tabs, inactiveTabStyle.Render("["+col.Title+"]"))
		}
	}
	tabBar := strings.Join(tabs, " ")

	// Panel dimensions.
	// Left panel: ~40% of inner width, including its border (2 chars).
	leftTotal := innerWidth * 2 / 5
	leftContentWidth := leftTotal - 2
	// Right panel: remaining width, including its border (2 chars).
	rightTotal := innerWidth - leftTotal
	rightContentWidth := rightTotal - 2

	// Panel content height: total height - outer border (2) - tab bar (1) - help bar (1) - panel borders (2).
	panelHeight := b.Height - 6
	if panelHeight < 1 {
		panelHeight = 1
	}

	// Set panel border styles based on detail focus.
	var leftStyle, rightStyle lipgloss.Style
	if b.detailFocused {
		leftStyle = leftPanelStyle.BorderForeground(lipgloss.Color("240"))
		rightStyle = rightPanelStyle.BorderForeground(lipgloss.Color("15"))
	} else {
		leftStyle = leftPanelStyle
		rightStyle = rightPanelStyle
	}

	col := b.Columns[b.ActiveTab]
	leftPanel := b.viewCardList(col, panelHeight, leftContentWidth, leftStyle)
	rightPanel := b.viewCardDetail(col, rightContentWidth, panelHeight, rightStyle)

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Help bar.
	helpBar := helpStyle.Render(b.statusBar.View())

	// Assemble inner content.
	inner := lipgloss.JoinVertical(lipgloss.Left, tabBar, panels, helpBar)

	if b.mode == createMode || b.mode == creatingMode {
		return b.viewCreateModal()
	}

	return outerStyle.Width(innerWidth).Render(inner)
}

func (b Board) viewCardList(col Column, panelHeight, contentWidth int, style lipgloss.Style) string {
	// Pre-compute wrapped lines for each card.
	type wrappedCard struct {
		lines    []string
		selected bool
	}
	var allCards []wrappedCard
	for j, card := range col.Cards {
		prefix := fmt.Sprintf("#%d ", card.Number)
		text := prefix + card.Title
		lines := wrapTitle(text, contentWidth, len([]rune(prefix)))
		allCards = append(allCards, wrappedCard{lines: lines, selected: j == col.Cursor})
	}

	// Compute total line count for all cards.
	totalLines := 0
	for _, wc := range allCards {
		totalLines += len(wc.lines)
	}

	var leftLines []string

	if totalLines <= panelHeight {
		// All cards fit -- render everything.
		for _, wc := range allCards {
			for _, line := range wc.lines {
				if wc.selected {
					line = selectedCardStyle.Render(line)
				}
				leftLines = append(leftLines, line)
			}
		}
	} else {
		// Need scrolling -- determine which cards are visible.
		showUp := col.ScrollOffset > 0

		// Available lines for card content.
		available := panelHeight
		if showUp {
			available--
		}

		// Render cards starting from ScrollOffset, fitting within available lines.
		linesUsed := 0
		endIdx := col.ScrollOffset
		for endIdx < len(allCards) {
			cardLineCount := len(allCards[endIdx].lines)
			// Reserve 1 line for down indicator if there are more cards after.
			neededForDown := 0
			if endIdx+1 < len(allCards) {
				neededForDown = 1
			}
			if linesUsed+cardLineCount > available-neededForDown {
				break
			}
			linesUsed += cardLineCount
			endIdx++
		}

		showDown := endIdx < len(allCards)

		if showUp {
			leftLines = append(leftLines, "\u25b2")
		}
		for j := col.ScrollOffset; j < endIdx; j++ {
			wc := allCards[j]
			for _, line := range wc.lines {
				if wc.selected {
					line = selectedCardStyle.Render(line)
				}
				leftLines = append(leftLines, line)
			}
		}
		if showDown {
			leftLines = append(leftLines, "\u25bc")
		}
	}

	leftContent := strings.Join(leftLines, "\n")
	return style.
		Width(contentWidth).
		Height(panelHeight).
		Render(leftContent)
}

func (b Board) viewCardDetail(col Column, contentWidth, panelHeight int, style lipgloss.Style) string {
	var rightContent string
	if len(col.Cards) > 0 {
		card := col.Cards[col.Cursor]
		rightContent = detailTitleStyle.Render(fmt.Sprintf("#%d %s", card.Number, card.Title)) +
			"\n" + fmt.Sprintf("Labels: %s", strings.Join(card.Labels, ", "))
		if card.Body != "" {
			rendered := card.Body
			if cachedGlamourRenderer == nil || cachedGlamourRendererWidth != contentWidth {
				mdStyle := styles.DarkStyleConfig
				mdStyle.Document.Color = nil
				mdStyle.Document.BackgroundColor = nil
				mdStyle.Paragraph.Color = nil
				mdStyle.Paragraph.BackgroundColor = nil
				mdStyle.Text.Color = nil
				r, err := glamour.NewTermRenderer(
					glamour.WithStyles(mdStyle),
					glamour.WithWordWrap(contentWidth),
				)
				if err == nil {
					cachedGlamourRenderer = r
					cachedGlamourRendererWidth = contentWidth
				}
			}
			if cachedGlamourRenderer != nil {
				if out, renderErr := cachedGlamourRenderer.Render(card.Body); renderErr == nil {
					rendered = strings.TrimSpace(out)
				}
			}

			// Apply scroll offset and truncate to available panel height.
			lines := strings.Split(rendered, "\n")
			headerLines := 3 // title + labels + blank separator
			availableBodyLines := panelHeight - headerLines
			if availableBodyLines < 1 {
				availableBodyLines = 1
			}

			startLine := b.detailScrollOffset
			maxOffset := len(lines) - availableBodyLines
			if maxOffset < 0 {
				maxOffset = 0
			}
			if startLine > maxOffset {
				startLine = maxOffset
			}
			if startLine < 0 {
				startLine = 0
			}

			endLine := startLine + availableBodyLines
			hasMore := endLine < len(lines)
			if hasMore {
				endLine = endLine - 1 // leave room for scroll indicator
			}
			if endLine > len(lines) {
				endLine = len(lines)
			}

			visibleLines := lines[startLine:endLine]
			rightContent += "\n\n" + strings.Join(visibleLines, "\n")
			if hasMore {
				rightContent += "\n" + helpStyle.Render("\u25bc")
			}
		}
	}
	return style.
		Width(contentWidth).
		Height(panelHeight).
		Render(rightContent)
}

func (b Board) viewCreateModal() string {
	modalWidth := 40
	var modalContent string
	if b.mode == creatingMode {
		modalContent = "New Card\n\n" + b.spinner.View() + " Creating card..."
	} else {
		var errLine string
		if b.validationErr != "" {
			errLine = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(b.validationErr)
		}
		createHints := NewStatusBar([]Hint{
			{Key: "esc", Desc: "Cancel"},
			{Key: "tab", Desc: "Next"},
			{Key: "enter", Desc: "Submit"},
		})
		modalContent = "New Card\n\n" +
			"Title:\n" + b.titleInput.View() + errLine + "\n\n" +
			"Label:\n" + b.labelInput.View() + "\n\n" +
			helpStyle.Render(createHints.View())
	}

	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("15")).
		Padding(1, 2).
		Width(modalWidth)

	modal := modalStyle.Render(modalContent)
	return lipgloss.Place(b.Width, b.Height, lipgloss.Center, lipgloss.Center, modal)
}

func (b Board) viewConfigModal() string {
	modalWidth := 40
	var errLine string
	if b.validationErr != "" {
		errLine = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(b.validationErr)
	}

	providerDisplay := "< " + b.providerOptions[b.providerIndex] + " >"

	configHints := NewStatusBar([]Hint{
		{Key: "esc", Desc: "Cancel"},
		{Key: "tab", Desc: "Next"},
		{Key: "enter", Desc: "Save"},
	})

	repoView := b.repoInput.View()

	modalContent := "Configuration\n\n" +
		"Provider:\n" + providerDisplay + "\n\n" +
		"Repo:\n" + repoView + errLine + "\n\n" +
		helpStyle.Render(configHints.View())

	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("15")).
		Padding(1, 2).
		Width(modalWidth)

	modal := modalStyle.Render(modalContent)
	return lipgloss.Place(b.Width, b.Height, lipgloss.Center, lipgloss.Center, modal)
}
