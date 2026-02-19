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

	// Panel dimensions.
	// Left panel: ~40% of inner width, including its border (2 chars).
	leftTotal := innerWidth * 2 / 5
	leftContentWidth := leftTotal - 2
	// Right panel: remaining width, including its border (2 chars).
	rightTotal := innerWidth - leftTotal
	rightContentWidth := rightTotal - 2

	// Panel content height: total height - outer border (2) - help bar (1) - panel borders (2).
	panelHeight := b.Height - 5
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
	inner := lipgloss.JoinVertical(lipgloss.Left, panels, helpBar)

	if b.mode == createMode || b.mode == creatingMode {
		return b.viewCreateModal()
	}

	// Render with normal outer border, then replace the top line with the border title.
	rendered := outerStyle.Width(innerWidth).Render(inner)
	borderTitle := buildBorderTitle(b.Columns, b.ActiveTab, b.Width)
	lines := strings.SplitN(rendered, "\n", 2)
	if len(lines) == 2 {
		return borderTitle + "\n" + lines[1]
	}
	return rendered
}

// buildBorderTitle constructs the top border line with embedded column names.
// Format: ╭─ [1] Name ─ [2] Name ─...──╮
// When the terminal is too narrow, titles are progressively truncated with "…".
// If even truncated titles don't fit, falls back to just [N] per column.
func buildBorderTitle(columns []Column, activeTab, totalWidth int) string {
	borderFg := lipgloss.Color("240")
	borderStyle := lipgloss.NewStyle().Foreground(borderFg)

	prefixStr := borderStyle.Render("╭─ ")
	suffixChar := borderStyle.Render("╮")
	prefixWidth := lipgloss.Width(prefixStr)
	suffixWidth := lipgloss.Width(suffixChar)

	// Minimum fill is " ─" (2 chars visual).
	minFillWidth := 2
	availableForLabels := totalWidth - prefixWidth - suffixWidth - minFillWidth

	// renderLabels builds styled labels from text strings and joins them.
	renderLabels := func(texts []string) (string, int) {
		separator := borderStyle.Render(" ─ ")
		var styled []string
		for i, text := range texts {
			if i == activeTab {
				styled = append(styled, activeBorderTitleStyle.Render(text))
			} else {
				styled = append(styled, inactiveBorderTitleStyle.Render(text))
			}
		}
		joined := strings.Join(styled, separator)
		return joined, lipgloss.Width(joined)
	}

	// Try 1: Full titles — "[N] Title (C)"
	fullTexts := make([]string, len(columns))
	for i, col := range columns {
		fullTexts[i] = fmt.Sprintf("[%d] %s (%d)", i+1, col.Title, len(col.Cards))
	}
	joined, joinedWidth := renderLabels(fullTexts)

	if joinedWidth > availableForLabels {
		// Try 2: Truncated titles — "[N] Ti…"
		// Compute how much space separators take.
		sepWidth := 0
		if len(columns) > 1 {
			sepWidth = lipgloss.Width(borderStyle.Render(" ─ ")) * (len(columns) - 1)
		}
		perLabel := (availableForLabels - sepWidth) / len(columns)
		// Each label has "[N] " prefix overhead (4 chars for single-digit, 5 for double-digit).
		// Find max title chars after subtracting prefix overhead.
		truncTexts := make([]string, len(columns))
		canTruncate := true
		for i, col := range columns {
			numPrefix := fmt.Sprintf("[%d] ", i+1)
			prefixLen := len([]rune(numPrefix))
			countSuffix := fmt.Sprintf(" (%d)", len(col.Cards))
			countLen := len([]rune(countSuffix))
			maxTitleChars := perLabel - prefixLen - countLen
			if maxTitleChars < 1 {
				canTruncate = false
				break
			}
			titleRunes := []rune(col.Title)
			if len(titleRunes) > maxTitleChars {
				truncTexts[i] = numPrefix + string(titleRunes[:maxTitleChars-1]) + "\u2026" + countSuffix
			} else {
				truncTexts[i] = numPrefix + col.Title + countSuffix
			}
		}

		if canTruncate {
			joined, joinedWidth = renderLabels(truncTexts)
		}

		// Try 3: Numbers only — "[N] (C)"
		if !canTruncate || joinedWidth > availableForLabels {
			numTexts := make([]string, len(columns))
			for i, col := range columns {
				numTexts[i] = fmt.Sprintf("[%d] (%d)", i+1, len(col.Cards))
			}
			joined, joinedWidth = renderLabels(numTexts)
		}

		// Try 4: If even numbers-only exceeds available space, drop labels entirely.
		if joinedWidth > availableForLabels {
			joined = ""
			joinedWidth = 0
		}
	}

	// Fill remaining width with ─.
	fillWidth := totalWidth - prefixWidth - joinedWidth - suffixWidth - 1
	if fillWidth < 1 {
		fillWidth = 1
	}
	fill := borderStyle.Render(" " + strings.Repeat("─", fillWidth))

	return prefixStr + joined + fill + suffixChar
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
		if len(card.LinkedPRs) > 0 {
			text += " \u23c7"
		}
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

// countWrappedLines counts how many visual lines text occupies at a given width.
func countWrappedLines(text string, width int) int {
	if width <= 0 || text == "" {
		return 1
	}
	rendered := lipgloss.NewStyle().Width(width).Render(text)
	return strings.Count(rendered, "\n") + 1
}

// detailHeaderLineCount computes the actual header height for a card,
// accounting for title/label wrapping at the given content width.
func detailHeaderLineCount(card Card, contentWidth int) int {
	titleText := fmt.Sprintf("#%d %s", card.Number, card.Title)
	labelsText := fmt.Sprintf("Labels: %s", strings.Join(card.Labels, ", "))
	return countWrappedLines(titleText, contentWidth) + countWrappedLines(labelsText, contentWidth) + 1 // +1 blank separator
}

func renderBody(body string) string {
	if cachedGlamourRenderer != nil {
		if out, err := cachedGlamourRenderer.Render(body); err == nil {
			return strings.TrimSpace(out)
		}
	}
	return body
}

func (b Board) viewCardDetail(col Column, contentWidth, panelHeight int, style lipgloss.Style) string {
	var rightContent string
	if len(col.Cards) > 0 {
		card := col.Cards[col.Cursor]
		rightContent = detailTitleStyle.Render(fmt.Sprintf("#%d %s", card.Number, card.Title)) +
			"\n" + fmt.Sprintf("Labels: %s", strings.Join(card.Labels, ", "))
		if card.Body != "" {
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
			rendered := renderBody(card.Body)

			// Apply scroll offset and truncate to available panel height.
			lines := strings.Split(rendered, "\n")
			headerLines := detailHeaderLineCount(card, contentWidth)
			availableBodyLines := panelHeight - headerLines
			if availableBodyLines < 1 {
				availableBodyLines = 1
			}

			startLine := b.detailScrollOffset

			// Reserve space for up-arrow if scrolled past top.
			showUp := startLine > 0
			if showUp {
				availableBodyLines--
				if availableBodyLines < 1 {
					availableBodyLines = 1
				}
			}

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

			// Clamping may have zeroed startLine — reclaim up-arrow space.
			if startLine == 0 && showUp {
				showUp = false
				availableBodyLines++
			}

			endLine := startLine + availableBodyLines
			hasMore := endLine < len(lines)
			if hasMore {
				endLine = endLine - 1 // leave room for down-arrow indicator
			}
			if endLine > len(lines) {
				endLine = len(lines)
			}

			rightContent += "\n\n"
			if showUp {
				rightContent += helpStyle.Render("\u25b2") + "\n"
			}
			visibleLines := lines[startLine:endLine]
			rightContent += strings.Join(visibleLines, "\n")
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
