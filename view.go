package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
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
		errorText := "Error: " + b.loadErr + "\n\n" + b.statusBar.View(b.Width, 0, 0)
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
	// When a search query or global filter is active, display only filtered cards.
	// Compute filtered cards once and reuse throughout View().
	displayCol := col
	var filtered []Card
	if b.searchQuery != "" || b.activeFilterType != filterTypeNone {
		filtered = b.filteredCards()
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

	// Help bar. Session-scoped agent counts (all six statuses) and the
	// repo-wide open-PR total render as an always-visible prefix.
	running, needInput, done, failed, stopped, idle := b.agentCounts()
	helpBar := b.statusBar.View(innerWidth, running, needInput, b.prIndicatorCount(), done, failed, stopped, idle)
	if b.refreshing {
		helpBar = b.spinner.View() + " Refreshing..."
	}
	if b.mode == labelConfirmMode && b.labelConfirm.currentIdx < len(b.labelConfirm.unknownLabels) {
		label := b.labelConfirm.unknownLabels[b.labelConfirm.currentIdx]
		helpBar = fmt.Sprintf("Label %q doesn't exist. Create it? (y/n)", label)
	}
	if b.mode == closeConfirmMode {
		card := b.closeConfirm.card
		helpBar = fmt.Sprintf("Close #%d %q? (y/n)", card.Number, card.Title)
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

	if b.mode == commentMode {
		return b.viewCommentModal()
	}

	if b.mode == deleteMode {
		return b.viewDeleteModal()
	}

	if b.mode == filterMode {
		return b.viewFilterModal()
	}

	if b.mode == assignMode {
		return b.viewAssignModal()
	}

	if b.mode == gitPanelMode {
		return b.viewGitPanelModal()
	}

	if b.mode == prListMode {
		return b.viewPRListModal()
	}

	if b.mode == agentListMode {
		return b.viewAgentListModal()
	}

	if b.mode == dispatchMode {
		return b.viewDispatchModal()
	}

	// Render with normal outer border, then replace the top line with the border title.
	rendered := outerStyle.Width(innerWidth).Render(inner)
	var borderTitle string
	if b.searchQuery != "" {
		fc := make([]int, len(b.Columns))
		for i := range fc {
			fc[i] = -1
		}
		fc[b.ActiveTab] = len(filtered)
		borderTitle = buildBorderTitle(b.Columns, b.ActiveTab, b.Width, fc)
	} else if b.activeFilterType != filterTypeNone {
		fc := make([]int, len(b.Columns))
		for i := range b.Columns {
			fc[i] = b.filteredCardsForColumn(i)
		}
		borderTitle = buildBorderTitle(b.Columns, b.ActiveTab, b.Width, fc)
	} else {
		borderTitle = buildBorderTitle(b.Columns, b.ActiveTab, b.Width)
	}
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
// filteredCounts is optional: when non-nil, filteredCounts[i] >= 0 means show
// "filteredCounts[i]/len(col.Cards)" instead of "(len(col.Cards))" for column i.
func buildBorderTitle(columns []Column, activeTab, totalWidth int, filteredCounts ...[]int) string {
	var fc []int
	if len(filteredCounts) > 0 {
		fc = filteredCounts[0]
	}
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

	// countSuffix returns "(filtered/total) ●" when a filtered count is set,
	// or "(total)" otherwise.
	countSuffix := func(i int, total int) string {
		if fc != nil && i < len(fc) && fc[i] >= 0 {
			return fmt.Sprintf("(%d/%d) \u25cf", fc[i], total)
		}
		return fmt.Sprintf("(%d)", total)
	}

	// Try 1: Full titles — "[N] Title (C)"
	fullTexts := make([]string, len(columns))
	for i, col := range columns {
		fullTexts[i] = fmt.Sprintf("[%d] %s %s", i+1, col.Title, countSuffix(i, len(col.Cards)))
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
			cntSuffix := " " + countSuffix(i, len(col.Cards))
			countLen := len([]rune(cntSuffix))
			maxTitleChars := perLabel - prefixLen - countLen
			if maxTitleChars < 1 {
				canTruncate = false
				break
			}
			titleRunes := []rune(col.Title)
			if len(titleRunes) > maxTitleChars {
				truncTexts[i] = numPrefix + string(titleRunes[:maxTitleChars-1]) + "\u2026" + cntSuffix
			} else {
				truncTexts[i] = numPrefix + col.Title + cntSuffix
			}
		}

		if canTruncate {
			joined, joinedWidth = renderLabels(truncTexts)
		}

		// Try 3: Numbers only — "[N] (C)"
		if !canTruncate || joinedWidth > availableForLabels {
			numTexts := make([]string, len(columns))
			for i, col := range columns {
				numTexts[i] = fmt.Sprintf("[%d] %s", i+1, countSuffix(i, len(col.Cards)))
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
// The configured working label (case-insensitive) and any label matching a column name are hidden.
func isHiddenLabel(label string, columnNames []string, workingLabel string) bool {
	if workingLabel != "" && strings.EqualFold(label, workingLabel) {
		return true
	}
	for _, col := range columnNames {
		if strings.EqualFold(label, col) {
			return true
		}
	}
	return false
}

// agentBadgeKindWidth is the fixed rune width the agent kind is padded/truncated
// to, so badges align across cards regardless of agent-name length.
const agentBadgeKindWidth = 6

// agentStatusSymbol maps a cenci window status to its badge symbol.
// Returns "" for idle and any unknown status (no badge).
func agentStatusSymbol(status string) string {
	switch status {
	case "running":
		return "▶" // ▶
	case "done":
		return "✓" // ✓
	case "stopped":
		return "■" // ■
	case "need-input":
		return "!" // ! (single mark, consistent with the other statuses)
	case "failed":
		return "✗" // ✗
	default:
		return ""
	}
}

// agentBadgeText returns the fixed-width badge text "<kind> <symbol>" for a
// window status/agent, or "" when the status has no badge. When agent is empty
// the symbol is returned alone. The kind is truncated/space-padded to a stable
// rune width (content build-up, not layout measurement — []rune is correct here).
func agentBadgeText(status, agent string) string {
	symbol := agentStatusSymbol(status)
	if symbol == "" {
		return ""
	}
	if agent == "" {
		return symbol
	}
	runes := []rune(agent)
	if len(runes) > agentBadgeKindWidth {
		runes = runes[:agentBadgeKindWidth]
	} else {
		for len(runes) < agentBadgeKindWidth {
			runes = append(runes, ' ')
		}
	}
	return string(runes) + " " + symbol
}

// agentBadgeStyle maps a cenci window status to its badge style.
func agentBadgeStyle(status string) lipgloss.Style {
	switch status {
	case "running":
		return agentRunningStyle
	case "done":
		return agentDoneStyle
	case "stopped":
		return agentStoppedStyle
	case "need-input":
		return agentNeedInputStyle
	case "failed":
		return agentFailedStyle
	default:
		return lipgloss.NewStyle()
	}
}

// prStatus derives a single-PR status from GitHub's raw isDraft/mergeable/
// mergeStateStatus fields: one of "draft", "mergeable", "conflicting",
// "blocked", "unstable", or "unknown".
//
// Mergeable == "UNKNOWN" short-circuits to "unknown" before the draft/blocked
// checks -- an unresolved mergeability calculation must never be misreported
// as draft/blocked just because the PR also happens to match one of those
// signals. Draft is derived from IsDraft (not mergeStateStatus == "DRAFT")
// per the ticket, and is checked before mergeStateStatus's blocked-family
// values so a draft PR always reports as draft regardless of its
// mergeStateStatus.
func prStatus(pr LinkedPR) string {
	if pr.Mergeable == "UNKNOWN" {
		return "unknown"
	}
	if pr.IsDraft {
		return "draft"
	}
	if pr.Mergeable == "CONFLICTING" {
		return "conflicting"
	}
	switch pr.MergeStateStatus {
	case "BLOCKED", "BEHIND":
		return "blocked"
	case "UNSTABLE":
		return "unstable"
	case "DIRTY":
		// DIRTY means "the merge commit cannot be cleanly created" -- the
		// same real-world condition as Mergeable == "CONFLICTING" above.
		// Checked explicitly here (not left to fall through to Mergeable)
		// so conflict detection doesn't silently depend on Mergeable and
		// MergeStateStatus agreeing -- GitHub-side staleness/propagation
		// lag can leave Mergeable == "MERGEABLE" while MergeStateStatus
		// has already moved to DIRTY.
		return "conflicting"
	}
	// HAS_HOOKS ("mergeable with passing status and pre-receive hooks") and
	// CLEAN intentionally fall through to the Mergeable check below.
	if pr.Mergeable == "MERGEABLE" {
		return "mergeable"
	}
	return "unknown"
}

// prStatusSymbol maps a prStatus value to its badge glyph. Returns "" for
// "unknown" and any other unrecognized status (no glyph), mirroring
// agentStatusSymbol's blank-for-idle convention.
func prStatusSymbol(status string) string {
	switch status {
	case "draft":
		return "●" // ●
	case "mergeable":
		return "✓" // ✓
	case "conflicting":
		return "✗" // ✗
	case "blocked":
		return "!"
	case "unstable":
		return "●"
	default:
		return ""
	}
}

// prStatusStyle maps a prStatus value to its badge style.
func prStatusStyle(status string) lipgloss.Style {
	switch status {
	case "draft":
		return prDraftStyle
	case "mergeable":
		return prMergeableStyle
	case "conflicting":
		return prConflictingStyle
	case "blocked", "unstable":
		return prBlockedStyle
	default:
		return prIndicatorStyle
	}
}

// prStatusSymbolWidth is the rendered cell width of every known-status
// glyph (●, ✓, ✗, !) -- all single-width per go-runewidth.
const prStatusSymbolWidth = 1

// prStatusPrefix renders the fixed-width prefix column for a PR list row:
// the purple linkedPRGlyph marker followed by the status glyph, padded to a
// fixed rendered width (prStatusSymbolWidth + a separator space) so
// unknown-status rows (no glyph) occupy exactly the same column width as
// known-status rows -- otherwise the "#NN" column jitters left/right
// depending on whether that row's status is known. Width is measured with
// lipgloss.Width, not len(), per docs/terminal-rendering.md.
//
// This is the single choke point used by both viewPRListModal and
// cardStatusLines, so the purple marker prefixes every PR row/line for all
// statuses, including "unknown".
func prStatusPrefix(status string) string {
	symbol := prStatusSymbol(status)
	pad := prStatusSymbolWidth - lipgloss.Width(symbol)
	if pad < 0 {
		pad = 0
	}
	statusColumn := strings.Repeat(" ", pad) + prStatusStyle(status).Render(symbol) + " "
	return prIndicatorStyle.Render(linkedPRGlyph) + " " + statusColumn
}

// cardDisplayText builds the raw display text for a card's title line:
// "#N title [Working icon] [label dots]". Agent status and linked-PR status
// no longer render inline here -- they render as separate lines beneath the
// title via cardStatusLines (#439).
// Returns the assembled text and the rune-length of the number prefix (for wrap indentation).
// columnNames controls which labels are hidden from the dot display.
// workingLabel is the configured label name that triggers the spinner icon.
func cardDisplayText(card Card, columnNames []string, workingLabel string) (string, int) {
	prefix := fmt.Sprintf("#%d ", card.Number)
	text := prefix + card.Title
	// Spinner icon uses case-insensitive match against the configured working label.
	for _, label := range card.Labels {
		if workingLabel != "" && strings.EqualFold(label.Name, workingLabel) {
			text += " \uf110"
			break
		}
	}
	for _, label := range card.Labels {
		if !isHiddenLabel(label.Name, columnNames, workingLabel) {
			text += " \u25cf"
		}
	}
	return text, len([]rune(prefix))
}

// cardStatusLines returns the status lines rendered under a card's title:
// one line per non-idle agent window joined to the card (agent lines first),
// then one line per linked PR (PR lines last), each prefixed with indentWidth
// spaces to align under the title text -- the same continuation indent
// wrapTitle uses for the "#N " prefix. Idle/badge-less agent windows are
// skipped entirely (no line, no vertical cost).
func (b Board) cardStatusLines(card Card, indentWidth int) []string {
	indent := strings.Repeat(" ", indentWidth)
	var lines []string
	for _, w := range b.cardAgentWindows(card.Number) {
		badge := agentBadgeText(w.Status, w.Agent)
		if badge == "" {
			continue
		}
		lines = append(lines, indent+agentBadgeStyle(w.Status).Render(badge))
	}
	for _, pr := range card.LinkedPRs {
		status := prStatus(pr)
		lines = append(lines, indent+prStatusPrefix(status)+fmt.Sprintf("#%d", pr.Number))
	}
	return lines
}

// cardLineCount returns the number of visual lines a card occupies: its
// (possibly wrapped) title lines plus its agent/PR status lines
// (cardStatusLines). This is the single source of truth shared by
// clampScrollOffset, viewCardList, and handleCardClick
// (docs/list-cursor-invariants.md) -- they must never disagree about a
// card's rendered height.
func (b Board) cardLineCount(card Card, contentWidth int, columnNames []string) int {
	text, prefixLen := cardDisplayText(card, columnNames, b.workingLabel)
	return len(wrapTitle(text, contentWidth, prefixLen)) + len(b.cardStatusLines(card, prefixLen))
}

func (b *Board) clampScrollOffset() {
	if len(b.Columns) == 0 {
		return
	}
	col := &b.Columns[b.ActiveTab]

	// Use filtered cards when a search or global filter is active.
	cards := col.Cards
	if b.searchQuery != "" || b.activeFilterType != filterTypeNone {
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
		totalLines += b.cardLineCount(cards[i], contentWidth, columnNames)
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
			cl := b.cardLineCount(cards[lastVisible], contentWidth, columnNames)
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
			linesFromCursor := b.cardLineCount(cards[col.Cursor], contentWidth, columnNames)
			avail := panelHeight - 1 // reserve 1 for up indicator (since we're scrolling down)
			for col.ScrollOffset > 0 {
				prevLines := b.cardLineCount(cards[col.ScrollOffset-1], contentWidth, columnNames)
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

	// When search mode is active, render the search input at the top.
	var searchLine string
	if b.mode == searchMode {
		searchLine = b.searchInput.View()
		panelHeight -= 2 // 1 for input, 1 for separator blank line
		if panelHeight < 1 {
			panelHeight = 1
		}
	}

	// Show empty state when search or global filter matches no cards.
	if len(col.Cards) == 0 && ((b.mode == searchMode && b.searchQuery != "") || b.activeFilterType != filterTypeNone) {
		leftContent := "No matching cards"
		actualHeight := panelHeight
		if searchLine != "" {
			leftContent = searchLine + "\n\n" + leftContent
			actualHeight += 2
		}
		return style.
			Width(contentWidth).
			Height(actualHeight).
			Render(leftContent)
	}

	// Pre-compute wrapped lines for each card.
	type wrappedCard struct {
		lines    []string
		selected bool
	}
	var allCards []wrappedCard
	for j, card := range col.Cards {
		text, prefixLen := cardDisplayText(card, columnNames, b.workingLabel)
		hasWorking := false
		for _, label := range card.Labels {
			if b.workingLabel != "" && strings.EqualFold(label.Name, b.workingLabel) {
				hasWorking = true
				break
			}
		}
		lines := wrapTitle(text, contentWidth, prefixLen)
		// Style Working indicator.
		if hasWorking && len(lines) > 0 {
			last := len(lines) - 1
			lines[last] = strings.Replace(lines[last], "\uf110", workingIndicatorStyle.Render("\uf110"), 1)
		}
		// Style label dots with per-label colors (skip hidden labels).
		for _, label := range card.Labels {
			if isHiddenLabel(label.Name, columnNames, b.workingLabel) {
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
		lines = append(lines, b.cardStatusLines(card, prefixLen)...)
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
	if searchLine != "" {
		leftContent = searchLine + "\n\n" + leftContent
	}
	actualHeight := panelHeight
	if b.mode == searchMode {
		actualHeight += 2 // restore the 2 lines we subtracted for search input
	}
	return style.
		Width(contentWidth).
		Height(actualHeight).
		Render(leftContent)
}

func renderBody(body string) string {
	if cachedGlamourRenderer != nil {
		if out, err := cachedGlamourRenderer.Render(body); err == nil {
			return strings.TrimSpace(out)
		}
	}
	return body
}

// escapeMarkdown escapes markdown-special characters to prevent
// unintended formatting when rendered by glamour.
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		`*`, `\*`,
		`_`, `\_`,
		"`", "\\`",
		`[`, `\[`,
		`]`, `\]`,
		`~`, `\~`,
	)
	return replacer.Replace(s)
}

// composeDetailMarkdown builds a markdown string for the detail panel.
// Card metadata is rendered as markdown text followed by a --- horizontal rule.
// A "labels:" field is always shown: label names when present, "(none)" when empty.
// A "created:" field is always shown: the creation date, or "(unknown)" when CreatedAt is zero.
// If the card has a body, it is appended after the horizontal rule.
func composeDetailMarkdown(card Card) string {
	var sb strings.Builder

	// Escape markdown chars in title. No YAML quoting — title is displayed as-is.
	safeTitle := escapeMarkdown(card.Title)
	fmt.Fprintf(&sb, "title: #%d %s\n\n", card.Number, safeTitle)

	if len(card.Labels) > 0 {
		labelNames := make([]string, len(card.Labels))
		for i, l := range card.Labels {
			labelNames[i] = l.Name
		}
		sb.WriteString("labels: " + strings.Join(labelNames, ", ") + "\n\n")
	} else {
		sb.WriteString("labels: (none)\n\n")
	}

	if len(card.Assignees) > 0 {
		logins := make([]string, len(card.Assignees))
		for i, a := range card.Assignees {
			logins[i] = a.Login
		}
		sb.WriteString("assignees: " + strings.Join(logins, ", ") + "\n\n")
	} else {
		sb.WriteString("assignees: (none)\n\n")
	}

	if card.CreatedAt.IsZero() {
		sb.WriteString("created: (unknown)\n\n")
	} else {
		sb.WriteString("created: " + card.CreatedAt.Format("2006-01-02") + "\n\n")
	}

	sb.WriteString("---")
	if card.Body != "" {
		sb.WriteString("\n\n" + card.Body)
	}
	return sb.String()
}

func (b Board) viewCardDetail(col Column, contentWidth, panelHeight int, style lipgloss.Style) string {
	var rightContent string
	if len(col.Cards) > 0 {
		card := col.Cards[col.Cursor]

		// Initialize glamour renderer if needed.
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

		// Build and render the full markdown (frontmatter + body).
		fullMarkdown := composeDetailMarkdown(card)
		rendered := renderBody(fullMarkdown)

		// Apply unified scroll: the entire rendered content scrolls as one unit.
		lines := strings.Split(rendered, "\n")
		availableLines := panelHeight

		startLine := b.detailScrollOffset

		// Reserve space for up-arrow if scrolled past top.
		showUp := startLine > 0
		if showUp {
			availableLines--
			if availableLines < 1 {
				availableLines = 1
			}
		}

		maxOffset := len(lines) - availableLines
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
			availableLines++
		}

		endLine := startLine + availableLines
		hasMore := endLine < len(lines)
		if hasMore {
			endLine = endLine - 1 // leave room for down-arrow indicator
		}
		if endLine > len(lines) {
			endLine = len(lines)
		}

		if showUp {
			rightContent += helpStyle.Render("\u25b2") + "\n"
		}
		visibleLines := lines[startLine:endLine]
		rightContent += strings.Join(visibleLines, "\n")
		if hasMore {
			rightContent += "\n" + helpStyle.Render("\u25bc")
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
	modalWidth := b.createModalWidth()
	var modalContent string
	if b.mode == creatingMode {
		modalContent = "New Card\n\n" + b.spinner.View() + " Creating card..."
	} else {
		var errLine string
		if b.validationErr != "" {
			errLine = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(b.validationErr)
		}
		hints := []Hint{
			{Key: "esc", Desc: "Cancel"},
			{Key: "tab", Desc: "Next"},
		}
		if b.create.focus == 2 {
			hints = append(hints, Hint{Key: "\u25c0/\u25b6", Desc: "Cycle"})
		}
		hints = append(hints, Hint{Key: "enter", Desc: "Submit"})
		createHints := NewStatusBar(hints)

		var assigneeLine string
		if len(b.create.assigneeOptions) > 1 {
			assigneeDisplay := "< " + b.create.assigneeOptions[b.create.assigneeIndex] + " >"
			assigneeLine = "\n\nAssignee:\n" + assigneeDisplay
		}

		modalContent = "New Card\n\n" +
			"Title:\n" + b.create.titleInput.View() + errLine + "\n\n" +
			"Label:\n" + b.create.labelInput.View() +
			assigneeLine + "\n\n" +
			createHints.View(modalWidth, 0, 0)
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
		configHints.View(modalWidth, 0, 0)

	return b.renderModal(modalContent, modalWidth)
}

func (b Board) viewPRPickerModal() string {
	col := b.Columns[b.ActiveTab]
	card := col.Cards[col.Cursor]
	pr := card.LinkedPRs[b.prPickerIndex]

	modalWidth := 50
	status := prStatus(pr)
	var prPrefix string
	if symbol := prStatusSymbol(status); symbol != "" {
		prPrefix = prStatusStyle(status).Render(symbol) + " "
	}
	prDisplay := prPrefix + fmt.Sprintf("\u25c0 #%d %s \u25b6", pr.Number, pr.Title)

	pickerHints := NewStatusBar(prPickerHints)
	modalContent := "Select PR\n\n" +
		prDisplay + "\n\n" +
		pickerHints.View(modalWidth, 0, 0)

	return b.renderModal(modalContent, modalWidth)
}

// helpSection groups a display name with its keybinding entries for the help popup.
type helpSection struct {
	title string
	keys  [][2]string
}

// helpSections is the ordered list of static help sections.
// Custom Actions and Usage are appended dynamically by buildHelpContent().
// When adding a new mode, add its section here so keybindings appear in the help popup.
// The six j/k-navigated lists (card list, PR list, agents list, assignee
// picker, filter picker, git menu) all wrap from last item to first and back
// (#426); their entries here intentionally keep the "Navigate" wording rather
// than switching to "Cycle X" like the Left/Right pickers below, since they
// remain navigation controls -- wraparound is a behavior refinement, not a
// change of purpose.
var helpSections = []helpSection{
	{"Normal Mode", [][2]string{
		{"?", "Help"},
		{"q", "Quit"},
		{"n", "New card"},
		{"e", "Edit card"},
		{"c", "Configuration"},
		{"o", "Open ticket"},
		{"r", "Refresh"},
		{"p", "Open PR"},
		{"x", "Close card"},
		{"t", "Delete card"},
		{"v", "Open PRs"},
		{"w", "Agents (cenci)"},
		{"s", "Go to agent (cenci)"},
		{"/", "Search"},
		{"a", "Assign"},
		{"g", "Git menu"},
		{"d", "Dispatch (cenci)"},
		{"u", "Sort order"},
		{"f", "Filter (toggle)"},
		{"l/\u2192", "Detail panel"},
		{"j/k", "Navigate cards"},
		{"tab/s-tab", "Switch columns"},
		{"1-9", "Jump to column"},
		{"A-Z", "Custom action"},
		{"A-Z..", "Custom action key sequence"},
		{"alt+shift+key", "Comment action"},
	}},
	{"Detail Panel", [][2]string{
		{"e", "Edit card"},
		{"j/k", "Scroll body"},
		{"h/\u2190/esc", "Back to card list"},
		{"tab/s-tab", "Switch columns"},
		{"o", "Open ticket"},
		{"p", "Open PR"},
		{"r", "Refresh"},
		{"q", "Quit"},
		{"?", "Help"},
		{"A-Z", "Custom action"},
		{"A-Z..", "Custom action key sequence"},
		{"alt+shift+key", "Comment action"},
	}},
	{"Create Card", [][2]string{
		{"esc", "Cancel"},
		{"tab", "Next field"},
		{"←/→", "Cycle assignee"},
		{"enter", "Submit"},
	}},
	{"Configuration", [][2]string{
		{"esc", "Cancel"},
		{"tab", "Next field"},
		{"\u2190/\u2192", "Cycle provider"},
		{"enter", "Save"},
	}},
	{"PR Picker", [][2]string{
		{"\u2190/\u2192", "Cycle PR"},
		{"enter", "Select"},
		{"esc", "Cancel"},
	}},
	{"Pull Requests", [][2]string{
		{"v", "Open PRs (repo-wide)"},
		{"j/k", "Navigate"},
		{"enter", "Open PR"},
		{"A-Z", "Custom action (scope: pr)"},
		{"esc", "Cancel"},
	}},
	{"Agents (cenci)", [][2]string{
		{"w", "Agents (all cenci windows)"},
		{"s", "Go to agent (from Normal Mode)"},
		{"j/k", "Navigate"},
		{"enter", "Go to tmux window"},
		{"esc", "Cancel"},
	}},
	{"Comment", [][2]string{
		{"esc", "Cancel"},
		{"enter", "Submit"},
	}},
	{"Delete", [][2]string{
		{"t", "Open (from Normal Mode)"},
		{"enter", "Continue / Confirm"},
		{"esc", "Cancel"},
	}},
	{"Close Confirm", [][2]string{
		{"x", "Open (from Normal Mode)"},
		{"y", "Confirm close"},
		{"n/esc", "Cancel"},
	}},
	{"Filter", [][2]string{
		{"f", "Filter (toggle)"},
		{"j/k", "Navigate"},
		{"enter", "Select"},
		{"esc", "Cancel"},
	}},
	{"Search", [][2]string{
		{"↑/↓", "Navigate results"},
		{"ctrl+n/p", "Navigate results"},
		{"tab/s-tab", "Switch columns (clears search)"},
		{"enter", "Apply search"},
		{"esc", "Clear search"},
	}},
	{"Assign", [][2]string{
		{"j/k", "Navigate"},
		{"enter", "Toggle assignee"},
		{"esc", "Cancel"},
	}},
	{"Git Menu", [][2]string{
		{"g", "Open (from Normal Mode)"},
		{"P", "Push"},
		{"p", "Pull (rebase)"},
		{"f", "Fetch"},
		{"m", "Mergetool"},
		{"s", "Stash push"},
		{"S", "Stash pop"},
		{"j/k", "Navigate"},
		{"enter", "Run selected"},
		{"esc", "Cancel"},
	}},
	{"Dispatch (cenci)", [][2]string{
		{"d", "Open (from Normal Mode)"},
		{"enter", "Enroll/Unenroll"},
		{"o", "Dispatch once"},
		{"l", "Toggle loop on/off (all enrolled repos)"},
		{"y/n", "Confirm/cancel loop toggle"},
		{"esc", "Close"},
	}},
	{"Label Confirm", [][2]string{
		{"y", "Create label, continue"},
		{"n", "Cancel edit"},
		{"esc", "Cancel edit"},
	}},
	{"Error", [][2]string{
		{"r", "Retry"},
		{"q", "Quit"},
	}},
	// Always-visible status-bar prefix indicators (not keys). Each is omitted
	// when its count is zero.
	{"Status Bar", [][2]string{
		{"▶N", "Agents running"},
		{"!N", "Agents awaiting input"},
		{linkedPRGlyph + "N", "Open PRs in the repository"},
	}},
}

func (b Board) buildHelpContent() string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Help — lazyboards %s\n\n", appVersion())

	for i, section := range helpSections {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(section.title + "\n")
		for _, kv := range section.keys {
			fmt.Fprintf(&sb, "  %-12s %s\n", kv[0], kv[1])
		}
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
		globalKeys := make([]string, 0, len(b.actions))
		for key := range b.actions {
			globalKeys = append(globalKeys, key)
		}
		sort.Strings(globalKeys)
		for _, key := range globalKeys {
			act := b.actions[key]
			fmt.Fprintf(&sb, "  %-12s %s (%s)\n", key, act.Name, act.Type)
		}
		// Column-specific actions.
		for _, cc := range b.columnConfigs {
			if len(cc.Actions) == 0 {
				continue
			}
			fmt.Fprintf(&sb, "  %s:\n", cc.Name)
			columnKeys := make([]string, 0, len(cc.Actions))
			for key := range cc.Actions {
				columnKeys = append(columnKeys, key)
			}
			sort.Strings(columnKeys)
			for _, key := range columnKeys {
				act := cc.Actions[key]
				fmt.Fprintf(&sb, "    %-10s %s (%s)\n", key, act.Name, act.Type)
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
	displayLines = append(displayLines, "", hintsBar.View(modalWidth, 0, 0))

	modalContent := strings.Join(displayLines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

func (b Board) viewCommentModal() string {
	modalWidth := b.createModalWidth()
	commentHints := NewStatusBar(commentModeHints)
	modalContent := b.comment.pendingAction.Name + "\n\n" +
		b.comment.input.View() + "\n\n" +
		commentHints.View(modalWidth, 0, 0)
	return b.renderModal(modalContent, modalWidth)
}

// viewDeleteModal renders the two-step delete-confirm modal: the
// optional-comment step or the retype-to-confirm step, mirroring
// viewCommentModal's structure (prompt, active textinput, hints), plus a
// card identifier and an inline mismatch message when present.
func (b Board) viewDeleteModal() string {
	modalWidth := b.createModalWidth()
	card := b.delete.card

	var prompt, activeInputView string
	var hints StatusBar
	switch b.delete.step {
	case deleteStepConfirm:
		prompt = fmt.Sprintf("Type %d to permanently delete #%d %q (Esc to cancel):", card.Number, card.Number, card.Title)
		activeInputView = b.delete.confirmInput.View()
		hints = NewStatusBar(deleteConfirmHints)
	default:
		prompt = fmt.Sprintf("Delete #%d %q — optional comment (Enter to continue, Esc to cancel):", card.Number, card.Title)
		activeInputView = b.delete.commentInput.View()
		hints = NewStatusBar(deleteCommentHints)
	}

	modalContent := prompt + "\n\n" + activeInputView
	if b.delete.mismatchMsg != "" {
		modalContent += "\n\n" + b.delete.mismatchMsg
	}
	modalContent += "\n\n" + hints.View(modalWidth, 0, 0)
	return b.renderModal(modalContent, modalWidth)
}

func (b Board) viewFilterModal() string {
	modalWidth := 50

	var lines []string
	lines = append(lines, "Filter")
	lines = append(lines, "")

	for i, item := range b.filterItems {
		if item.isHeader {
			lines = append(lines, helpStyle.Render(item.value))
			continue
		}
		display := "  " + item.value
		if i == b.filterCursor {
			display = selectedCardStyle.Render(display)
		}
		lines = append(lines, display)
	}

	lines = append(lines, "")
	filterHints := NewStatusBar(filterModeHints)
	lines = append(lines, filterHints.View(modalWidth, 0, 0))

	modalContent := strings.Join(lines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

func (b Board) viewAssignModal() string {
	modalWidth := 50

	var lines []string
	lines = append(lines, "Assign")
	lines = append(lines, "")

	for i, item := range b.assign.items {
		prefix := "  "
		if item.isAssigned {
			prefix = "* "
		}
		display := prefix + item.login
		if i == b.assign.cursor {
			display = selectedCardStyle.Render(display)
		}
		lines = append(lines, display)
	}

	lines = append(lines, "")
	assignHints := NewStatusBar(assignModeHints)
	lines = append(lines, assignHints.View(modalWidth, 0, 0))

	modalContent := strings.Join(lines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

func (b Board) viewGitPanelModal() string {
	modalWidth := 50

	var lines []string
	lines = append(lines, "Git Menu")
	lines = append(lines, "")

	for i, item := range b.gitPanel.items {
		display := "  " + item.key + "  " + item.name
		if i == b.gitPanel.cursor {
			display = selectedCardStyle.Render(display)
		}
		lines = append(lines, display)
	}

	lines = append(lines, "")
	gitPanelHints := NewStatusBar(gitPanelModeHints)
	lines = append(lines, gitPanelHints.View(modalWidth, 0, 0))

	modalContent := strings.Join(lines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

// viewPRListModal renders the global PR list: every open PR in the
// repository in one navigable list. Each row shows the PR number and its
// (truncated) title; rows linked to a board card also carry the owning
// column + card so they stay disambiguated, while unlinked PRs (cardNumber
// 0) render without a card reference. State precedence mirrors prListState's
// (loading -> err -> loaded): while the repo-wide fetch is in flight the
// card-linked fallback entries render with a loading note; on fetch error
// the fallback is kept with an explicit degraded-view note; each empty-list
// state gets its own message.
func (b Board) viewPRListModal() string {
	modalWidth := 60

	var lines []string
	lines = append(lines, "Open Pull Requests")
	lines = append(lines, "")

	if len(b.prList.entries) == 0 {
		switch {
		case b.prList.loading:
			lines = append(lines, "Loading open PRs...")
		case b.prList.err != "":
			lines = append(lines, "No linked PRs")
		default:
			lines = append(lines, "No open PRs")
		}
	} else {
		noteLines := 0
		if b.prList.loading || b.prList.err != "" {
			noteLines = 2
		}
		maxRowLines := b.Height - 8 - noteLines
		if maxRowLines < 1 {
			maxRowLines = 1
		}
		start, end := 0, len(b.prList.entries)
		showUp, showDown := false, false
		if len(b.prList.entries) > maxRowLines {
			entryLines := maxRowLines - 2
			if entryLines < 1 {
				entryLines = 1
			}
			start = b.prList.cursor - entryLines/2
			if start < 0 {
				start = 0
			}
			end = start + entryLines
			if end > len(b.prList.entries) {
				end = len(b.prList.entries)
				start = end - entryLines
			}
			showUp = start > 0
			showDown = end < len(b.prList.entries)
			// At very small terminal heights there is only room for the
			// selected row and one directional indicator.
			if maxRowLines < 3 && showUp && showDown {
				showDown = false
			}
		}
		if showUp {
			lines = append(lines, helpStyle.Render("▲"))
		}
		for i := start; i < end; i++ {
			entry := b.prList.entries[i]
			title := truncateOutput(entry.pr.Title, 32)
			status := prStatus(entry.pr)
			prefix := prStatusPrefix(status)
			display := fmt.Sprintf("%s  #%d  %s", prefix, entry.pr.Number, title)
			if entry.cardNumber != 0 {
				display += fmt.Sprintf("  —  %s #%d", entry.columnTitle, entry.cardNumber)
			}
			if i == b.prList.cursor {
				display = selectedCardStyle.Render(display)
			}
			lines = append(lines, display)
		}
		if showDown {
			lines = append(lines, helpStyle.Render("▼"))
		}
		if b.prList.loading {
			lines = append(lines, "")
			lines = append(lines, "Loading all open PRs...")
		}
	}
	if b.prList.err != "" {
		lines = append(lines, "")
		lines = append(lines, truncateOutput("Couldn't load open PRs — showing linked PRs only", modalWidth-4))
	}

	lines = append(lines, "")
	prListHints := NewStatusBar(b.prListActionHints())
	lines = append(lines, prListHints.View(modalWidth, 0, 0))

	modalContent := strings.Join(lines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

// viewAgentListModal renders the agents list modal. State precedence mirrors
// every state the cenciwatch wiring distinguishes: watcher disabled -> daemon
// not connected yet -> connected with no windows -> window list, plus a stale
// note when the same consecutive-error threshold that clears the status-bar
// dispatch segment has been reached (the list then shows the last known
// snapshot). dispatchLoopSource applies the same threshold as its
// live-vs-CLI trust gate for the dispatch modal's Loop line. The
// empty/unavailable branches deliberately render no enter hint, matching
// handleAgentListModeKey's empty-list guard
// (docs/view-state-consistency.md).
func (b Board) viewAgentListModal() string {
	modalWidth := 60

	var lines []string
	title := "Agents"
	if b.agentList.cardNumber != 0 {
		title = fmt.Sprintf("Agents — #%d", b.agentList.cardNumber)
	}
	lines = append(lines, title)
	lines = append(lines, "")

	entries := b.agentListEntries()
	disconnected := b.agentSnapshot != nil && b.cenciWatchConsecutiveErrors >= cenciWatchClearThreshold
	if len(entries) == 0 {
		switch {
		case b.cenciWatcher == nil:
			lines = append(lines, agentListMsgNotEnabled)
		case b.agentSnapshot == nil:
			lines = append(lines, agentListMsgWaiting)
		default:
			lines = append(lines, agentListMsgNoWindows)
		}
	} else {
		noteLines := 0
		if disconnected {
			noteLines = 2
		}
		maxRowLines := b.Height - 8 - noteLines
		if maxRowLines < 1 {
			maxRowLines = 1
		}
		start, end := 0, len(entries)
		showUp, showDown := false, false
		if len(entries) > maxRowLines {
			entryLines := maxRowLines - 2
			if entryLines < 1 {
				entryLines = 1
			}
			start = b.agentList.cursor - entryLines/2
			if start < 0 {
				start = 0
			}
			end = start + entryLines
			if end > len(entries) {
				end = len(entries)
				start = end - entryLines
			}
			showUp = start > 0
			showDown = end < len(entries)
			// At very small terminal heights there is only room for the
			// selected row and one directional indicator.
			if maxRowLines < 3 && showUp && showDown {
				showDown = false
			}
		}
		if showUp {
			lines = append(lines, helpStyle.Render("▲"))
		}
		for i := start; i < end; i++ {
			entry := entries[i]
			symbol := agentStatusSymbol(entry.window.Status)
			if symbol == "" {
				// The modal lists every window, so idle/unknown (badge-less
				// elsewhere) still gets a neutral marker to keep rows aligned.
				symbol = "·"
			}
			display := fmt.Sprintf("  %s %s", symbol, truncateOutput(entry.window.WindowName, 24))
			if ref := agentWindowRef(entry.window); ref != "" {
				display = fmt.Sprintf("  %s %s  %s", symbol, truncateOutput(ref, 16), truncateOutput(entry.window.WindowName, 24))
			}
			if entry.window.Agent != "" {
				display += "  " + truncateOutput(entry.window.Agent, agentBadgeKindWidth)
			}
			if entry.cardNumber != 0 {
				display += fmt.Sprintf("  —  %s #%d", entry.columnTitle, entry.cardNumber)
			}
			if i == b.agentList.cursor {
				display = selectedCardStyle.Render(display)
			}
			lines = append(lines, display)
		}
		if showDown {
			lines = append(lines, helpStyle.Render("▼"))
		}
	}
	if disconnected {
		lines = append(lines, "")
		lines = append(lines, truncateOutput("cenci-watch disconnected — showing last known agents", modalWidth-4))
	}

	lines = append(lines, "")
	hints := agentListModeHints
	if len(entries) == 0 {
		hints = agentListEmptyHints
	}
	agentListHints := NewStatusBar(hints)
	lines = append(lines, agentListHints.View(modalWidth, 0, 0))

	modalContent := strings.Join(lines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

// viewDispatchModal renders the agent dispatch modal. State precedence:
// loading -> running -> err -> ready (ready shows an optional "Last run"
// summary line when dispatch.lastResult is populated).
func (b Board) viewDispatchModal() string {
	// Wider than the other modals' usual 50: the common "cenci not found
	// on PATH" classifyCenciError message (57 chars) wraps at width 60
	// but fits on one line at 65. Longer classified messages (e.g. the
	// git-repo-not-resolvable case) may still wrap onto a second line, which
	// is acceptable — this width targets the common case, not every case.
	modalWidth := 65

	var lines []string
	lines = append(lines, "Agent Dispatch")
	lines = append(lines, "")

	var hints []Hint
	switch {
	case b.dispatch.loading:
		lines = append(lines, b.spinner.View()+" Checking dispatch status...")
		hints = []Hint{{Key: "esc", Desc: "Close"}}
	case b.dispatch.running:
		lines = append(lines, b.spinner.View()+" Running dispatch...")
		hints = []Hint{{Key: "esc", Desc: "Close"}}
	case b.dispatch.err != "":
		if b.dispatch.repo != "" {
			lines = append(lines, "Repo: "+b.dispatch.repo)
		}
		if b.dispatch.dir != "" {
			lines = append(lines, "Dir: "+b.dispatch.dir)
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(b.dispatch.err))
		hints = []Hint{{Key: "esc", Desc: "Close"}}
	case b.dispatch.repo == "":
		// Zero-value/unset dispatchState (e.g. modal opened before the status
		// query resolves, or an unexpected empty repo from the query) is not
		// a ready state — render it as such rather than showing blank fields.
		lines = append(lines, "No repository detected.")
		hints = []Hint{{Key: "esc", Desc: "Close"}}
	default:
		lines = append(lines, "Repo: "+b.dispatch.repo)
		lines = append(lines, "Dir: "+b.dispatch.dir)
		enrolledText := "no"
		if b.dispatch.enrolled {
			enrolledText = "yes"
		}
		lines = append(lines, "Enrolled: "+enrolledText)
		if b.dispatch.lastResult != "" {
			lines = append(lines, "")
			lines = append(lines, "Last run: "+b.dispatch.lastResult)

			// Render up to 8 per-issue decision lines so skips are
			// explainable; no scrolling, just a truncation notice past the
			// cap (ticket #302).
			const maxDecisionLines = 8
			shown := b.dispatch.lastLines
			var moreCount int
			if len(shown) > maxDecisionLines {
				moreCount = len(shown) - maxDecisionLines
				shown = shown[:maxDecisionLines]
			}
			for _, decisionLine := range shown {
				lines = append(lines, wrapTitle(decisionLine, modalWidth, 0)...)
			}
			if moreCount > 0 {
				lines = append(lines, fmt.Sprintf("… and %d more", moreCount))
			}
		}

		lines = append(lines, "")
		loop, loopErr := b.dispatchLoopSource()
		lines = append(lines, renderLoopLine(loop, loopErr))

		if b.dispatch.confirmingLoop {
			// Fleet-wide, persistent toggle: confirm in both directions and spell
			// out the blast radius so a single keypress doesn't silently commit
			// every enrolled repo (#433).
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("Turn dispatch loop %s? Affects all enrolled repos.", loopToggleTarget(loop)))
			hints = []Hint{{Key: "y", Desc: "Confirm"}, {Key: "n/esc", Desc: "Cancel"}}
		} else {
			// The loop toggle needs a known current state to pick a direction;
			// omit the affordance entirely when the loop state is unknown. It
			// lives on its own line under the Loop line (rather than the bottom
			// hint bar) both because it reads as an action on the state directly
			// above it, and because a fourth hint overflows the modal width.
			if loop != nil {
				lines = append(lines, fmt.Sprintf("  l: Turn loop %s", loopToggleTarget(loop)))
			}

			enterDesc := "Enroll"
			if b.dispatch.enrolled {
				enterDesc = "Unenroll"
			}
			hints = []Hint{{Key: "esc", Desc: "Close"}, {Key: "enter", Desc: enterDesc}}
			if b.dispatch.enrolled {
				hints = append(hints, Hint{Key: "o", Desc: "Dispatch once"})
			}
		}
	}

	lines = append(lines, "")
	dispatchHints := NewStatusBar(hints)
	lines = append(lines, dispatchHints.View(modalWidth, 0, 0))

	modalContent := strings.Join(lines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

// dispatchLoopSource selects which decoded DispatchState the dispatch
// modal's Loop line renders (#403). The dispatch loop is fleet-wide, so the
// live state pushed over the daemon socket (agentSnapshot.Dispatch) is
// authoritative whenever it is present AND currently trusted -- trusted
// meaning the watcher's consecutive-error count is below the same
// cenciWatchClearThreshold gate that clears the status-bar segment and
// marks viewAgentListModal's list disconnected. Once that threshold is
// reached, the live value may be arbitrarily stale, so the line falls back
// to the independently-fetched `cenci dispatch status --json` result rather
// than rendering a silently-stale live value; a later successful snapshot
// resets the counter (update.go) and flips the source back to live.
func (b Board) dispatchLoopSource() (*cenciwatch.DispatchState, string) {
	if b.agentSnapshot != nil &&
		b.agentSnapshot.Dispatch != nil &&
		b.cenciWatchConsecutiveErrors < cenciWatchClearThreshold {
		return b.agentSnapshot.Dispatch, ""
	}
	return b.dispatch.loop, b.dispatch.loopErr
}

// loopToggleTarget returns the direction ("on"/"off") that a toggle would move
// the fleet-wide dispatch loop, given its current state. A nil loop (unknown
// state) reports "on" defensively, but callers gate the toggle on loop != nil,
// so that fallback is never surfaced to the user.
func loopToggleTarget(loop *cenciwatch.DispatchState) string {
	if loop != nil && loop.Enabled {
		return "off"
	}
	return "on"
}

// renderLoopLine renders the "Loop: ..." status line describing the
// daemon-owned background dispatch loop, sourced by dispatchLoopSource from
// either the live socket snapshot or the "loop" object in
// `cenci dispatch status --json` (ticket #313) -- both decode into the
// shared cenciwatch.DispatchState wire type (#402). lazyboards renders this
// state read-only here; toggling the loop on/off is the modal's built-in 'l'
// key (a confirmed toggleLoopCmd), not a render concern (#433). Precedence:
// old-binary guard >
// nil loop (defensive) > last_error > enabled/off > daemon-not-running >
// never-run > normal summary.
func renderLoopLine(loop *cenciwatch.DispatchState, loopErr string) string {
	errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	if loop == nil {
		if loopErr == "" {
			loopErr = "status unavailable — upgrade cenci"
		}
		return errStyle.Render("Loop: " + loopErr)
	}

	if loop.LastError != "" {
		return errStyle.Render("Loop: error — " + loop.LastError)
	}
	if !loop.Enabled {
		return "Loop: off"
	}

	intervalSuffix := ""
	if loop.Interval != "" {
		intervalSuffix = " (" + loop.Interval + ")"
	}

	if !loop.DaemonRunning {
		return fmt.Sprintf("Loop: on%s — daemon not running", intervalSuffix)
	}
	if loop.LastRunAt == "" {
		return fmt.Sprintf("Loop: on%s — no runs yet", intervalSuffix)
	}

	return fmt.Sprintf("Loop: on%s — last run %s, %d dispatched / %d skipped",
		intervalSuffix, formatLoopRunTime(loop.LastRunAt), loop.LastDispatched, loop.LastSkipped)
}

// formatLoopRunTime parses raw as RFC3339 and formats it as a local HH:MM
// string. If raw cannot be parsed, it is returned unchanged (never NaN,
// never a panic) -- a regression guard for malformed last_run_at values.
func formatLoopRunTime(raw string) string {
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return raw
	}
	return parsed.Local().Format("15:04")
}
