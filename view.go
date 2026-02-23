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
	panelHeight, leftContentWidth, rightContentWidth := b.layoutDimensions()

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
	// When a search query is active, display only filtered cards.
	displayCol := col
	if b.searchQuery != "" {
		filtered := b.filteredCards()
		cursor := col.Cursor
		if len(filtered) == 0 {
			cursor = 0
		} else if cursor >= len(filtered) {
			cursor = len(filtered) - 1
		}
		displayCol = Column{
			Title:        col.Title,
			Cards:        filtered,
			Cursor:       cursor,
			ScrollOffset: col.ScrollOffset,
		}
	}
	leftPanel := b.viewCardList(displayCol, panelHeight, leftContentWidth, leftStyle)
	rightPanel := b.viewCardDetail(displayCol, rightContentWidth, panelHeight, rightStyle)

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Help bar.
	helpBar := b.statusBar.View()
	if b.refreshing {
		helpBar = b.spinner.View() + " Refreshing..."
	}

	// Assemble inner content.
	inner := lipgloss.JoinVertical(lipgloss.Left, panels, helpBar)

	if b.mode == createMode || b.mode == creatingMode {
		return b.viewCreateModal()
	}

	if b.mode == prPickerMode {
		return b.viewPRPickerModal()
	}

	if b.mode == helpMode {
		return b.viewHelpModal()
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

// isHiddenLabel returns true if a label should be hidden from the colored dot display.
// "Working" (case-insensitive) and any label matching a column name are hidden.
func isHiddenLabel(label string, columnNames []string) bool {
	if strings.EqualFold(label, "Working") {
		return true
	}
	for _, col := range columnNames {
		if strings.EqualFold(label, col) {
			return true
		}
	}
	return false
}

// cardDisplayText builds the raw display text for a card: "#N title [PR icon] [Working icon] [label dots]".
// Returns the assembled text and the rune-length of the number prefix (for wrap indentation).
// columnNames controls which labels are hidden from the dot display.
func cardDisplayText(card Card, columnNames []string) (string, int) {
	prefix := fmt.Sprintf("#%d ", card.Number)
	text := prefix + card.Title
	if len(card.LinkedPRs) > 0 {
		text += " \ue728"
	}
	// Spinner icon uses exact match (case-sensitive) — only "Working" triggers it.
	// Dot filtering in isHiddenLabel uses EqualFold (case-insensitive) per ticket #124.
	for _, label := range card.Labels {
		if label.Name == "Working" {
			text += " \uf110"
			break
		}
	}
	for _, label := range card.Labels {
		if !isHiddenLabel(label.Name, columnNames) {
			text += " \u25cf"
		}
	}
	return text, len([]rune(prefix))
}

// cardLineCount returns the number of visual lines a card occupies
// when its title is wrapped to fit within contentWidth.
func cardLineCount(card Card, contentWidth int, columnNames []string) int {
	text, prefixLen := cardDisplayText(card, columnNames)
	return len(wrapTitle(text, contentWidth, prefixLen))
}

func (b *Board) clampScrollOffset() {
	if len(b.Columns) == 0 {
		return
	}
	col := &b.Columns[b.ActiveTab]

	// Use filtered cards when a search is active.
	cards := col.Cards
	if b.searchQuery != "" {
		cards = b.filteredCards()
	}
	totalCards := len(cards)
	if totalCards == 0 {
		col.ScrollOffset = 0
		return
	}

	panelHeight, contentWidth, _ := b.layoutDimensions()
	if contentWidth < 1 {
		contentWidth = 1
	}

	columnNames := make([]string, len(b.Columns))
	for i, c := range b.Columns {
		columnNames[i] = c.Title
	}

	// Compute total lines for all cards.
	totalLines := 0
	for i := 0; i < totalCards; i++ {
		totalLines += cardLineCount(cards[i], contentWidth, columnNames)
	}

	if totalLines <= panelHeight {
		col.ScrollOffset = 0
		return
	}

	// Iterate to find stable scroll position (converges in <=3 iterations).
	for iter := 0; iter < 3; iter++ {
		// Count lines visible from ScrollOffset.
		available := panelHeight
		if col.ScrollOffset > 0 {
			available-- // up indicator
		}

		// Count how many cards fit from ScrollOffset.
		linesUsed := 0
		lastVisible := col.ScrollOffset
		for lastVisible < totalCards {
			cl := cardLineCount(cards[lastVisible], contentWidth, columnNames)
			neededForDown := 0
			if lastVisible+1 < totalCards {
				neededForDown = 1
			}
			if linesUsed+cl > available-neededForDown {
				break
			}
			linesUsed += cl
			lastVisible++
		}
		// lastVisible is now one past the last fully visible card index.

		if col.Cursor < col.ScrollOffset {
			col.ScrollOffset = col.Cursor
		} else if col.Cursor >= lastVisible {
			// Scroll down so cursor card is the last visible.
			// Work backwards from cursor to find the ScrollOffset.
			col.ScrollOffset = col.Cursor
			linesFromCursor := cardLineCount(cards[col.Cursor], contentWidth, columnNames)
			avail := panelHeight - 1 // reserve 1 for up indicator (since we're scrolling down)
			for col.ScrollOffset > 0 {
				prevLines := cardLineCount(cards[col.ScrollOffset-1], contentWidth, columnNames)
				neededForDown := 0
				if col.Cursor+1 < totalCards {
					neededForDown = 1
				}
				if linesFromCursor+prevLines > avail-neededForDown {
					break
				}
				linesFromCursor += prevLines
				col.ScrollOffset--
			}
		} else {
			break
		}
	}

	// Final bounds clamp.
	if col.ScrollOffset < 0 {
		col.ScrollOffset = 0
	}
	maxOffset := totalCards - 1
	if col.ScrollOffset > maxOffset {
		col.ScrollOffset = maxOffset
	}
}

func (b Board) viewCardList(col Column, panelHeight, contentWidth int, style lipgloss.Style) string {
	columnNames := make([]string, len(b.Columns))
	for i, c := range b.Columns {
		columnNames[i] = c.Title
	}

	// Pre-compute wrapped lines for each card.
	type wrappedCard struct {
		lines    []string
		selected bool
	}
	var allCards []wrappedCard
	for j, card := range col.Cards {
		text, prefixLen := cardDisplayText(card, columnNames)
		hasPR := len(card.LinkedPRs) > 0
		hasWorking := false // Case-sensitive — see comment in cardDisplayText.
		for _, label := range card.Labels {
			if label.Name == "Working" {
				hasWorking = true
				break
			}
		}
		lines := wrapTitle(text, contentWidth, prefixLen)
		// Style PR indicator.
		if hasPR && len(lines) > 0 {
			last := len(lines) - 1
			lines[last] = strings.Replace(lines[last], "\ue728", prIndicatorStyle.Render("\ue728"), 1)
		}
		// Style Working indicator.
		if hasWorking && len(lines) > 0 {
			last := len(lines) - 1
			lines[last] = strings.Replace(lines[last], "\uf110", workingIndicatorStyle.Render("\uf110"), 1)
		}
		// Style label dots with per-label colors (skip hidden labels).
		for _, label := range card.Labels {
			if isHiddenLabel(label.Name, columnNames) {
				continue
			}
			styledDot := lipgloss.NewStyle().Foreground(labelColor(label)).Render("\u25cf")
			for li := range lines {
				if strings.Contains(lines[li], "\u25cf") {
					lines[li] = strings.Replace(lines[li], "\u25cf", styledDot, 1)
					break
				}
			}
		}
		// Dim card number on non-selected cards.
		if j != col.Cursor && len(lines) > 0 {
			prefix := fmt.Sprintf("#%d ", card.Number)
			lines[0] = strings.Replace(lines[0], prefix, cardNumberStyle.Render(prefix), 1)
		}
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
			lineCount := len(allCards[endIdx].lines)
			// Reserve 1 line for down indicator if there are more cards after.
			neededForDown := 0
			if endIdx+1 < len(allCards) {
				neededForDown = 1
			}
			if linesUsed+lineCount > available-neededForDown {
				break
			}
			linesUsed += lineCount
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
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	labelsText := strings.Join(labelNames, "  ")
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
		var styledLabels []string
		for _, label := range card.Labels {
			styledLabels = append(styledLabels, lipgloss.NewStyle().Foreground(labelColor(label)).Render(label.Name))
		}
		rightContent = detailTitleStyle.Render(fmt.Sprintf("#%d %s", card.Number, card.Title)) +
			"\n" + strings.Join(styledLabels, "  ")
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

func (b Board) renderModal(content string, width int) string {
	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("15")).
		Padding(1, 2).
		Width(width)

	modal := modalStyle.Render(content)
	return lipgloss.Place(b.Width, b.Height, lipgloss.Center, lipgloss.Center, modal)
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
			"Title:\n" + b.create.titleInput.View() + errLine + "\n\n" +
			"Label:\n" + b.create.labelInput.View() + "\n\n" +
			createHints.View()
	}

	return b.renderModal(modalContent, modalWidth)
}

func (b Board) viewConfigModal() string {
	modalWidth := 40
	var errLine string
	if b.validationErr != "" {
		errLine = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(b.validationErr)
	}

	providerDisplay := "< " + b.config.providerOptions[b.config.providerIndex] + " >"

	configHints := NewStatusBar([]Hint{
		{Key: "esc", Desc: "Cancel"},
		{Key: "tab", Desc: "Next"},
		{Key: "enter", Desc: "Save"},
	})

	repoView := b.config.repoInput.View()

	modalContent := "Configuration\n\n" +
		"Provider:\n" + providerDisplay + "\n\n" +
		"Repo:\n" + repoView + errLine + "\n\n" +
		configHints.View()

	return b.renderModal(modalContent, modalWidth)
}

func (b Board) viewPRPickerModal() string {
	col := b.Columns[b.ActiveTab]
	card := col.Cards[col.Cursor]
	pr := card.LinkedPRs[b.prPickerIndex]

	modalWidth := 50
	prDisplay := fmt.Sprintf("\u25c0 #%d %s \u25b6", pr.Number, pr.Title)

	pickerHints := NewStatusBar(prPickerHints)
	modalContent := "Select PR\n\n" +
		prDisplay + "\n\n" +
		pickerHints.View()

	return b.renderModal(modalContent, modalWidth)
}

func (b Board) buildHelpContent() string {
	var sb strings.Builder

	sb.WriteString("Help\n\n")

	// Normal Mode.
	sb.WriteString("Normal Mode\n")
	normalKeys := [][2]string{
		{"?", "Help"},
		{"q", "Quit"},
		{"n", "New card"},
		{"c", "Configuration"},
		{"o", "Open repository"},
		{"r", "Refresh"},
		{"p", "Open PR"},
		{"l/\u2192", "Detail panel"},
		{"j/k", "Navigate cards"},
		{"tab/s-tab", "Switch columns"},
		{"1-9", "Jump to column"},
	}
	for _, kv := range normalKeys {
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", kv[0], kv[1]))
	}

	// Detail Panel.
	sb.WriteString("\nDetail Panel\n")
	detailKeys := [][2]string{
		{"j/k", "Scroll body"},
		{"h/\u2190/esc", "Back to card list"},
		{"tab/s-tab", "Switch columns"},
		{"o", "Open repository"},
		{"r", "Refresh"},
		{"q", "Quit"},
		{"?", "Help"},
	}
	for _, kv := range detailKeys {
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", kv[0], kv[1]))
	}

	// Create Card.
	sb.WriteString("\nCreate Card\n")
	createKeys := [][2]string{
		{"esc", "Cancel"},
		{"tab", "Next field"},
		{"enter", "Submit"},
	}
	for _, kv := range createKeys {
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", kv[0], kv[1]))
	}

	// Configuration.
	sb.WriteString("\nConfiguration\n")
	configKeys := [][2]string{
		{"esc", "Cancel"},
		{"tab", "Next field"},
		{"\u2190/\u2192", "Cycle provider"},
		{"enter", "Save"},
	}
	for _, kv := range configKeys {
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", kv[0], kv[1]))
	}

	// PR Picker.
	sb.WriteString("\nPR Picker\n")
	prKeys := [][2]string{
		{"\u2190/\u2192", "Cycle PR"},
		{"enter", "Select"},
		{"esc", "Cancel"},
	}
	for _, kv := range prKeys {
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", kv[0], kv[1]))
	}

	// Error.
	sb.WriteString("\nError\n")
	errorKeys := [][2]string{
		{"r", "Retry"},
		{"q", "Quit"},
	}
	for _, kv := range errorKeys {
		sb.WriteString(fmt.Sprintf("  %-12s %s\n", kv[0], kv[1]))
	}

	// Custom Actions (global).
	hasGlobalActions := len(b.actions) > 0
	hasColumnActions := false
	for _, cc := range b.columnConfigs {
		if len(cc.Actions) > 0 {
			hasColumnActions = true
			break
		}
	}

	if hasGlobalActions || hasColumnActions {
		sb.WriteString("\nCustom Actions\n")
		for key, act := range b.actions {
			sb.WriteString(fmt.Sprintf("  %-12s %s (%s)\n", key, act.Name, act.Type))
		}
		// Column-specific actions.
		for _, cc := range b.columnConfigs {
			if len(cc.Actions) == 0 {
				continue
			}
			sb.WriteString(fmt.Sprintf("  %s:\n", cc.Name))
			for key, act := range cc.Actions {
				sb.WriteString(fmt.Sprintf("    %-10s %s (%s)\n", key, act.Name, act.Type))
			}
		}
	}

	// Usage.
	sb.WriteString("\nUsage\n")
	sb.WriteString("  Columns represent board states (e.g., New, Implementing).\n")
	sb.WriteString("  Press l or \u2192 to view card details.\n")
	sb.WriteString("  Custom actions are configured in .lazyboards.yml.\n")

	return sb.String()
}

func (b Board) viewHelpModal() string {
	modalWidth := 60
	content := b.buildHelpContent()
	contentLines := strings.Split(content, "\n")

	// Compute visible area: terminal height minus modal border/padding overhead.
	// renderModal uses Padding(1, 2) + rounded border: 1 top pad + 1 bottom pad + 1 top border + 1 bottom border = 4.
	// Plus outer centering margin ~4 lines. Total overhead = 8.
	// Reserve 2 lines for hints bar (blank line + hints).
	modalHeight := b.Height - 8
	if modalHeight < 5 {
		modalHeight = 5
	}
	visibleLines := modalHeight - 2
	if visibleLines < 1 {
		visibleLines = 1
	}

	// Clamp scroll offset (defensive — primary clamp is in handleHelpModeKey).
	maxOffset := len(contentLines) - visibleLines
	if maxOffset < 0 {
		maxOffset = 0
	}
	scrollOffset := b.helpScrollOffset
	if scrollOffset > maxOffset {
		scrollOffset = maxOffset
	}

	// Compute visible window.
	startLine := scrollOffset
	endLine := startLine + visibleLines
	if endLine > len(contentLines) {
		endLine = len(contentLines)
	}

	showUp := startLine > 0
	showDown := endLine < len(contentLines)

	// Reserve space for indicators within visibleLines.
	if showUp {
		startLine++
	}
	if showDown {
		endLine--
	}
	if startLine > endLine {
		startLine = endLine
	}

	var displayLines []string
	if showUp {
		displayLines = append(displayLines, helpStyle.Render("\u25b2"))
	}
	displayLines = append(displayLines, contentLines[startLine:endLine]...)
	if showDown {
		displayLines = append(displayLines, helpStyle.Render("\u25bc"))
	}

	// Add hints bar.
	hintsBar := NewStatusBar(helpModeHints)
	displayLines = append(displayLines, "", hintsBar.View())

	modalContent := strings.Join(displayLines, "\n")
	return b.renderModal(modalContent, modalWidth)
}
