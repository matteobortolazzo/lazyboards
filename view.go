package main

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	"github.com/matteobortolazzo/lazyboards/internal/keymap"
)

// Package-level glamour renderer cache.
// Safe because BubbleTea is single-threaded (all View/Update calls on main goroutine).
var (
	cachedGlamourRenderer      *glamour.TermRenderer
	cachedGlamourRendererWidth int
)

func (b Board) View() string {
	// Persist the stack of any panic during rendering before BubbleTea's
	// recovery restores the terminal and wipes it. See Board.Update.
	defer debuglog.RecoverCrash("View")

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
		helpBar = fmt.Sprintf("Label %q doesn't exist. Create it?%s", label, promptParenthetical(b.keys.Entries(keymap.ModeLabelConfirm, ""), keymap.CommandLabelConfirmCreate, keymap.CommandLabelConfirmCancel))
	}
	if b.mode == closeConfirmMode {
		card := b.closeConfirm.card
		helpBar = fmt.Sprintf("Close #%d %q?%s", card.Number, sanitizeSingleLine(card.Title), promptParenthetical(b.keys.Entries(keymap.ModeCloseConfirm, ""), keymap.CommandCloseConfirmConfirm, keymap.CommandCloseConfirmCancel))
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

	if b.mode == milestoneListMode {
		return b.viewMilestoneListModal()
	}

	if b.mode == agentListMode {
		return b.viewAgentListModal()
	}

	if b.mode == dispatchMode {
		return b.viewDispatchModal()
	}

	// Render with normal outer border, then replace the top line with the border title.
	rendered := outerStyle.Width(innerWidth).Render(inner)
	borderTitle := buildBorderTitle(b.Columns, b.ActiveTab, b.Width, b.borderTitleCounts())
	lines := strings.SplitN(rendered, "\n", 2)
	if len(lines) == 2 {
		return borderTitle + "\n" + lines[1]
	}
	return rendered
}

// tabZone describes one column's rendered tab-bar label: its text (exactly
// as buildBorderTitle rendered it, at whatever truncation rung the ladder
// selected) and its cell-position/width within the rendered border-title
// line. It is the single source of truth shared by buildBorderTitle
// (rendering) and handleTabClick (hit-testing) so the two can never disagree
// about label text or geometry (#519).
type tabZone struct {
	label string
	start int
	width int
}

// borderTitleZones runs the same progressive-truncation ladder buildBorderTitle
// uses and returns one zone per column with the label text it chose and that
// label's cell start/width within the rendered border-title line (measured
// from the left edge, i.e. including the "prefix" glyphs). Returns nil if the
// ladder drops labels entirely (even numbers-only doesn't fit) or if columns
// is empty. filteredCounts is optional: when non-nil, filteredCounts[i] >= 0
// means show "filteredCounts[i]/len(col.Cards)" instead of "(len(col.Cards))"
// for column i.
func borderTitleZones(columns []Column, totalWidth int, filteredCounts ...[]int) []tabZone {
	if len(columns) == 0 {
		return nil
	}

	var fc []int
	if len(filteredCounts) > 0 {
		fc = filteredCounts[0]
	}
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	prefixWidth := lipgloss.Width(borderStyle.Render("╭─ "))
	suffixWidth := lipgloss.Width(borderStyle.Render("╮"))
	sepWidth := lipgloss.Width(borderStyle.Render(" ─ "))

	// Minimum fill is 2 cells visual.
	minFillWidth := 2
	availableForLabels := totalWidth - prefixWidth - suffixWidth - minFillWidth

	widthOf := func(texts []string) int {
		w := 0
		for i, text := range texts {
			if i > 0 {
				w += sepWidth
			}
			w += lipgloss.Width(text)
		}
		return w
	}

	// countSuffix returns "(filtered/total) ●" when a filtered count is set,
	// or "(total)" otherwise.
	countSuffix := func(i int, total int) string {
		if fc != nil && i < len(fc) && fc[i] >= 0 {
			return fmt.Sprintf("(%d/%d) ●", fc[i], total)
		}
		return fmt.Sprintf("(%d)", total)
	}

	// Try 1: Full titles.
	// col.Title comes from the repo-local, untrusted .lazyboards.yml config,
	// so it is sanitized here (before composition/truncation) and reused
	// below wherever the raw title would otherwise be rendered (#500).
	sanitizedTitles := make([]string, len(columns))
	for i, col := range columns {
		sanitizedTitles[i] = sanitizeSingleLine(col.Title)
	}
	texts := make([]string, len(columns))
	for i, col := range columns {
		texts[i] = fmt.Sprintf("[%d] %s %s", i+1, sanitizedTitles[i], countSuffix(i, len(col.Cards)))
	}
	textsWidth := widthOf(texts)

	if textsWidth > availableForLabels {
		// Try 2: Truncated titles.
		// Compute how much space separators take.
		totalSepWidth := 0
		if len(columns) > 1 {
			totalSepWidth = sepWidth * (len(columns) - 1)
		}
		perLabel := (availableForLabels - totalSepWidth) / len(columns)
		// Each label has "[N] " prefix overhead (4 cells for single-digit, 5 for double-digit).
		// Find max title cells after subtracting prefix overhead.
		truncTexts := make([]string, len(columns))
		canTruncate := true
		for i, col := range columns {
			numPrefix := fmt.Sprintf("[%d] ", i+1)
			prefixCells := lipgloss.Width(numPrefix)
			cntSuffix := " " + countSuffix(i, len(col.Cards))
			suffixCells := lipgloss.Width(cntSuffix)
			maxTitleCells := perLabel - prefixCells - suffixCells
			if maxTitleCells < 1 {
				canTruncate = false
				break
			}
			truncTexts[i] = numPrefix + ansi.Truncate(sanitizedTitles[i], maxTitleCells, "…") + cntSuffix
		}

		if canTruncate {
			texts = truncTexts
			textsWidth = widthOf(texts)
		}

		// Try 3: Numbers only.
		if !canTruncate || textsWidth > availableForLabels {
			numTexts := make([]string, len(columns))
			for i, col := range columns {
				numTexts[i] = fmt.Sprintf("[%d] %s", i+1, countSuffix(i, len(col.Cards)))
			}
			texts = numTexts
			textsWidth = widthOf(texts)
		}

		// Try 4: If even numbers-only exceeds available space, drop labels entirely.
		if textsWidth > availableForLabels {
			return nil
		}
	}

	zones := make([]tabZone, len(texts))
	pos := prefixWidth
	for i, text := range texts {
		if i > 0 {
			pos += sepWidth
		}
		w := lipgloss.Width(text)
		zones[i] = tabZone{label: text, start: pos, width: w}
		pos += w
	}
	return zones
}

// buildBorderTitle constructs the top border line with embedded column names.
// Format: prefix [1] Name sep [2] Name sep...fill suffix.
// When the terminal is too narrow, titles are progressively truncated.
// If even truncated titles don't fit, falls back to just [N] per column.
// filteredCounts is optional: when non-nil, filteredCounts[i] >= 0 means show
// "filteredCounts[i]/len(col.Cards)" instead of "(len(col.Cards))" for column i.
func buildBorderTitle(columns []Column, activeTab, totalWidth int, filteredCounts ...[]int) string {
	borderFg := lipgloss.Color("240")
	borderStyle := lipgloss.NewStyle().Foreground(borderFg)

	prefixStr := borderStyle.Render("╭─ ")
	suffixChar := borderStyle.Render("╮")
	prefixWidth := lipgloss.Width(prefixStr)
	suffixWidth := lipgloss.Width(suffixChar)

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

	zones := borderTitleZones(columns, totalWidth, filteredCounts...)
	texts := make([]string, len(zones))
	for i, z := range zones {
		texts[i] = z.label
	}
	joined, joinedWidth := renderLabels(texts)

	// Fill remaining width with ─.
	fillWidth := totalWidth - prefixWidth - joinedWidth - suffixWidth - 1
	if fillWidth < 1 {
		fillWidth = 1
	}
	fill := borderStyle.Render(" " + strings.Repeat("─", fillWidth))

	return prefixStr + joined + fill + suffixChar
}

// borderTitleCounts derives the filteredCounts override passed to
// buildBorderTitle for the tab bar's "(N)"/"(f/N)" count suffix. Search wins
// over a global filter: when a search query is active, only the active
// column gets a non-negative count (b.filteredCards(), which combines any
// active filter with the search), and every other column gets the -1
// sentinel (no override). Otherwise, when a global filter is active (and no
// search), every column gets its own filtered count via
// b.filteredCardsForColumn. When neither is active, returns nil so
// buildBorderTitle falls back to its plain "(total)" rung.
func (b *Board) borderTitleCounts() []int {
	if b.searchQuery != "" {
		fc := make([]int, len(b.Columns))
		for i := range fc {
			fc[i] = -1
		}
		fc[b.ActiveTab] = len(b.filteredCards())
		return fc
	}
	if b.activeFilterType != filterTypeNone {
		fc := make([]int, len(b.Columns))
		for i := range b.Columns {
			fc[i] = b.filteredCardsForColumn(i)
		}
		return fc
	}
	return nil
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
//
// Unlike sanitizeSingleLine (built for natural-language single-line fields,
// where an embedded newline is replaced with a space to preserve word
// boundaries), agent is a compact kind token (e.g. "claude", "codex") that
// never legitimately contains internal whitespace, so embedded
// whitespace/control runs are stripped entirely here rather than replaced
// with a space -- otherwise the synthetic separator space would itself
// consume one rune of the fixed agentBadgeKindWidth budget, truncating real
// content that would otherwise fit.
func agentBadgeText(status, agent string) string {
	symbol := agentStatusSymbol(status)
	if symbol == "" {
		return ""
	}
	agent = strings.Join(strings.Fields(sanitizeControlSequences(agent)), "")
	agent = strings.Map(func(r rune) rune {
		if isBidiOrZeroWidthRune(r) {
			return -1
		}
		return r
	}, agent)
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
//
// The prefix keeps its purple marker and status color on every row,
// selected or not (#493): the glyph is the at-a-glance signal
// for "this ticket has a linked PR, in this state", so it is deliberately
// exempt from the non-selected muting convention that grays plain row text
// (see selectedRowStyle).
func prStatusPrefix(status string) string {
	symbol := prStatusSymbol(status)
	pad := prStatusSymbolWidth - lipgloss.Width(symbol)
	if pad < 0 {
		pad = 0
	}
	statusColumn := strings.Repeat(" ", pad) + prStatusStyle(status).Render(symbol) + " "
	return prIndicatorStyle.Render(linkedPRGlyph) + " " + statusColumn
}

// renderProgressBar renders a `▓▓▓░░` style progress bar of the given cell
// width for the given percentage. A complete (100%), non-muted bar renders
// in success green (progressCompleteStyle); every other case renders in the
// existing muted gray (mutedRowStyle), matching prStatusPrefix's convention
// of baking color in at construction time rather than via an outer wrap.
// Returns "" for a non-positive width.
func renderProgressBar(percentage float64, width int, muted bool) string {
	if width <= 0 {
		return ""
	}
	if percentage < 0 {
		percentage = 0
	} else if percentage > 100 {
		percentage = 100
	}
	fill := int(math.Round(percentage / 100 * float64(width)))
	if fill < 0 {
		fill = 0
	} else if fill > width {
		fill = width
	}
	bar := strings.Repeat("▓", fill) + strings.Repeat("░", width-fill)
	style := mutedRowStyle
	if !muted && percentage == 100 {
		style = progressCompleteStyle
	}
	return style.Render(bar)
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
	text := prefix + sanitizeSingleLine(card.Title)
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

// subIssueParentGlyph marks a card that has sub-issues (a parent), followed
// by its completed/total sub-issue count -- e.g. "󰙅 2/3" (#460, #475).
const subIssueParentGlyph = "\U000F0645"

// subIssueChildGlyph marks a card that has a parent issue (a child),
// followed by "#<parentNumber>" -- e.g. "󱞫 #12" (#460). Points up-right
// (nf-md-arrow_right_top) toward the parent, rather than down, so the
// direction reads as "this points to its parent" at a glance.
const subIssueChildGlyph = "\U000F17AB"

// cardStatusLines returns the status lines rendered under a card's title:
// sub-issue relationship lines first (parent line, then child line -- #460,
// structural context takes precedence per CLAUDE.md's state-struct
// precedence rule), then one line per non-idle agent window joined to the
// card (agent lines), then one line per linked PR (PR lines last), each
// prefixed with indentWidth spaces to align under the title text -- the same
// continuation indent wrapTitle uses for the "#N " prefix. Idle/badge-less
// agent windows and a card with neither sub-issue relationship are skipped
// entirely (no line, no vertical cost).
//
// Status lines keep their own colors on every card, focused or not
// (#493): agent badges and PR glyphs are what make "this
// ticket has an agent / a linked PR" readable at a glance across the whole
// board, so they are deliberately exempt from the non-selected muting
// convention that grays plain row text (see selectedRowStyle). Sub-issue
// lines render in their own muted gray on every card, as they always have.
func (b Board) cardStatusLines(card Card, indentWidth int) []string {
	indent := strings.Repeat(" ", indentWidth)
	var lines []string
	if card.SubIssueCount > 0 {
		lines = append(lines, indent+subIssueStyle.Render(fmt.Sprintf("%s %d/%d", subIssueParentGlyph, card.SubIssueCompleted, card.SubIssueCount)))
	}
	if card.ParentNumber > 0 {
		lines = append(lines, indent+subIssueStyle.Render(fmt.Sprintf("%s #%d", subIssueChildGlyph, card.ParentNumber)))
	}
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

// selectedRowStyle renders text with selectedCardStyle when selected is
// true, or mutes it to gray via mutedRowStyle otherwise (#478). This is the
// single choke point for the selected-row highlight convention shared by
// every list-like UI element (card list, PR list, filter picker, assignee
// picker, git menu, agents list, PR picker) -- a future list surface
// inherits the muted-gray treatment by default. Note: an outer wrap here
// only recolors plain text -- pre-colored status glyphs (prStatusPrefix,
// agent badges, sub-issue markers) keep the color baked in at their own
// construction site and stay colored on non-selected rows by design
// (#493, see docs/terminal-rendering.md).
func selectedRowStyle(text string, selected bool) string {
	if selected {
		return selectedCardStyle.Render(text)
	}
	return mutedRowStyle.Render(text)
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
				line = selectedRowStyle(line, wc.selected)
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
				line = selectedRowStyle(line, wc.selected)
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
// A "milestone:" field is always shown: the milestone title when present, "(none)" when empty.
// A "created:" field is always shown: the creation date, or "(unknown)" when CreatedAt is zero.
// Title, label names, milestone, assignees, and body are all untrusted GitHub content:
// each is sanitized (see sanitizeControlSequences) to strip terminal control
// sequences before it is escaped/joined into markdown. The body is appended
// after the horizontal rule with cross-reference annotations.
func composeDetailMarkdown(card Card) string {
	var sb strings.Builder

	// Escape markdown chars in title. No YAML quoting — title is displayed as-is.
	safeTitle := escapeMarkdown(sanitizeControlSequences(card.Title))
	fmt.Fprintf(&sb, "title: #%d %s\n\n", card.Number, safeTitle)

	if len(card.Labels) > 0 {
		labelNames := mapSlice(card.Labels, func(l Label) string { return l.Name })
		sortFoldStrings(labelNames)
		labelNames = mapSlice(labelNames, sanitizeControlSequences)
		sb.WriteString("labels: " + strings.Join(labelNames, ", ") + "\n\n")
	} else {
		sb.WriteString("labels: (none)\n\n")
	}

	if len(card.Assignees) > 0 {
		logins := mapSlice(card.Assignees, func(a Assignee) string { return a.Login })
		sortFoldStrings(logins)
		logins = mapSlice(logins, sanitizeControlSequences)
		sb.WriteString("assignees: " + strings.Join(logins, ", ") + "\n\n")
	} else {
		sb.WriteString("assignees: (none)\n\n")
	}

	if card.Milestone != "" {
		sb.WriteString("milestone: " + escapeMarkdown(sanitizeControlSequences(card.Milestone)) + "\n\n")
	} else {
		sb.WriteString("milestone: (none)\n\n")
	}

	if card.CreatedAt.IsZero() {
		sb.WriteString("created: (unknown)\n\n")
	} else {
		sb.WriteString("created: " + card.CreatedAt.Format("2006-01-02") + "\n\n")
	}

	sb.WriteString("---")
	if card.Body != "" {
		sb.WriteString("\n\n" + annotateBodyRefs(sanitizeControlSequences(card.Body)))
	}
	return sb.String()
}

// renderDetailLines composes the card's frontmatter+body markdown, renders
// it through glamour, and hard-wraps the result to contentWidth, returning
// the final display lines. Glamour never breaks long unbreakable tokens
// (e.g. a long URL), so a rendered line can come out wider than
// contentWidth; ansi.Hardwrap makes the returned line count and per-line
// width match what lipgloss will actually render (the per-line SGR/style
// handling itself is done by lipgloss.Style.Render(), not by Hardwrap).
//
// Both viewCardDetail (rendering) and scrollDetailDown (scroll-offset math)
// must call this same helper so their line counts cannot drift apart --
// see docs/view-state-consistency.md.
func renderDetailLines(card Card, contentWidth int) []string {
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

	fullMarkdown := composeDetailMarkdown(card)
	rendered := renderBody(fullMarkdown)

	if contentWidth >= 1 {
		rendered = ansi.Hardwrap(rendered, contentWidth, true)
	}

	return strings.Split(rendered, "\n")
}

func (b Board) viewCardDetail(col Column, contentWidth, panelHeight int, style lipgloss.Style) string {
	var rightContent string
	if len(col.Cards) > 0 {
		card := col.Cards[col.Cursor]

		// Apply unified scroll: the entire rendered content scrolls as one unit.
		lines := renderDetailLines(card, contentWidth)
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
		createHints := NewStatusBar(b.createModalHints())

		var assigneeLine string
		if len(b.create.assigneeOptions) > 1 {
			assigneeDisplay := "< " + sanitizeSingleLine(b.create.assigneeOptions[b.create.assigneeIndex]) + " >"
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

	configHints := NewStatusBar(b.configModalHints())

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
	// Picker shows only the currently browsed PR — always selected, no cursor to compare.
	prText := selectedRowStyle(fmt.Sprintf("#%d %s", pr.Number, sanitizeSingleLine(pr.Title)), true)
	prDisplay := prPrefix + "\u25c0 " + prText + " \u25b6"

	pickerHints := NewStatusBar(b.prPickerHints())
	modalContent := "Select PR\n\n" +
		prDisplay + "\n\n" +
		pickerHints.View(modalWidth, 0, 0)

	return b.renderModal(modalContent, modalWidth)
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
	hintsBar := NewStatusBar(b.helpHints())
	displayLines = append(displayLines, "", hintsBar.View(modalWidth, 0, 0))

	modalContent := strings.Join(displayLines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

func (b Board) viewCommentModal() string {
	modalWidth := b.createModalWidth()
	commentHints := NewStatusBar(b.commentHints())
	// pendingAction.Name is a config.Action.Name -- repo-local .lazyboards.yml
	// data, the same untrusted type #511 sanitizes at the git menu and help
	// modal render sites.
	modalContent := sanitizeSingleLine(b.comment.pendingAction.Name) + "\n\n" +
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
		prompt = fmt.Sprintf("Type %d to permanently delete #%d %q%s:", card.Number, card.Number, sanitizeSingleLine(card.Title), b.deleteConfirmPromptSuffix())
		activeInputView = b.delete.confirmInput.View()
		hints = NewStatusBar(b.deleteConfirmHints())
	default:
		prompt = fmt.Sprintf("Delete #%d %q — optional comment%s:", card.Number, sanitizeSingleLine(card.Title), b.deleteCommentPromptSuffix())
		activeInputView = b.delete.commentInput.View()
		hints = NewStatusBar(b.deleteCommentHints())
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
		display := "  " + sanitizeSingleLine(item.value)
		display = selectedRowStyle(display, i == b.filterCursor)
		lines = append(lines, display)
	}

	lines = append(lines, "")
	filterHints := NewStatusBar(b.filterHints())
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
		display := prefix + sanitizeSingleLine(item.login)
		display = selectedRowStyle(display, i == b.assign.cursor)
		lines = append(lines, display)
	}

	lines = append(lines, "")
	assignHints := NewStatusBar(b.assignHints())
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
		display := "  " + item.key + "  " + sanitizeSingleLine(item.name)
		display = selectedRowStyle(display, i == b.gitPanel.cursor)
		lines = append(lines, display)
	}

	lines = append(lines, "")
	gitPanelHintsBar := NewStatusBar(b.gitPanelHints())
	lines = append(lines, gitPanelHintsBar.View(modalWidth, 0, 0))

	modalContent := strings.Join(lines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

// scrollWindow computes the visible [start, end) row range for a
// cursor-centered, height-limited list, plus whether to render the ▲/▼
// scroll indicators. Shared by viewPRListModal, viewMilestoneListModal, and
// viewAgentListModal so their windowing math can't drift apart: a list that
// fits within maxRowLines renders in full with no indicators; a longer list
// scrolls to keep cursor in view, and at very small heights (maxRowLines < 3)
// only one directional indicator is shown so there's still room for the
// selected row.
func scrollWindow(cursor, total, maxRowLines int) (start, end int, showUp, showDown bool) {
	start, end = 0, total
	if total <= maxRowLines {
		return start, end, false, false
	}
	entryLines := maxRowLines - 2
	if entryLines < 1 {
		entryLines = 1
	}
	start = cursor - entryLines/2
	if start < 0 {
		start = 0
	}
	end = start + entryLines
	if end > total {
		end = total
		start = end - entryLines
	}
	showUp = start > 0
	showDown = end < total
	if maxRowLines < 3 && showUp && showDown {
		showDown = false
	}
	return start, end, showUp, showDown
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
		start, end, showUp, showDown := scrollWindow(b.prList.cursor, len(b.prList.entries), maxRowLines)
		if showUp {
			lines = append(lines, helpStyle.Render("▲"))
		}
		for i := start; i < end; i++ {
			entry := b.prList.entries[i]
			title := truncateOutput(sanitizeSingleLine(entry.pr.Title), 32)
			status := prStatus(entry.pr)
			prefix := prStatusPrefix(status)
			display := fmt.Sprintf("%s  #%d  %s", prefix, entry.pr.Number, title)
			if entry.cardNumber != 0 {
				display += fmt.Sprintf("  —  %s #%d", sanitizeSingleLine(entry.columnTitle), entry.cardNumber)
			}
			display = selectedRowStyle(display, i == b.prList.cursor)
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
	prListHints := NewStatusBar(b.prListHints())
	lines = append(lines, prListHints.View(modalWidth, 0, 0))

	modalContent := strings.Join(lines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

// milestoneModalWidth is the fixed total width of the Milestones modal
// (view.go:1036's renderModal Padding(1, 2) subtracts 4 for content =
// 72-cell content width). The plan deliberately does not clamp this to
// narrow terminals (no narrow-terminal width clamping in scope).
const milestoneModalWidth = 76

// Fixed-width columns of a milestone row, in cells (see the plan's column
// math: 30 + 12 + 4 + 7 + 11 + 4x2 separators = 72, the modal's content
// width). The title column is the only elastic one, computed as whatever is
// left after these fixed columns and separators.
const (
	milestoneBarWidth    = 12
	milestonePctWidth    = 4
	milestoneCountsWidth = 7
	milestoneDueWidth    = 11
	// milestoneColumnGap is the 2-space separator rendered between every pair
	// of adjacent columns; there are 4 gaps (title|bar|pct|counts|due).
	milestoneColumnGap = "  "
)

// milestoneTitleWidth returns the elastic title column width for the fixed
// 72-cell content area (milestoneModalWidth minus renderModal's 4-cell
// padding overhead).
func milestoneTitleWidth() int {
	contentWidth := milestoneModalWidth - 4
	fixed := milestoneBarWidth + milestonePctWidth + milestoneCountsWidth + milestoneDueWidth + 4*len(milestoneColumnGap)
	return contentWidth - fixed
}

// viewMilestoneListModal renders the repo-wide Milestones modal (i): every
// open milestone in the repository, one per line (title, progress bar,
// percentage, closed/total counts, due date). State precedence mirrors
// milestoneListState's (loading -> err -> loaded); unlike viewPRListModal
// there is no fallback list and no degraded-view note on error -- the error
// state renders only the fixed "Couldn't load milestones" message, never the
// raw/sanitized provider error text (see the plan's Resolved Decisions).
func (b Board) viewMilestoneListModal() string {
	modalWidth := milestoneModalWidth
	titleWidth := milestoneTitleWidth()

	var lines []string
	lines = append(lines, "Milestones")
	lines = append(lines, "")

	switch {
	case b.milestoneList.loading:
		lines = append(lines, "Loading milestones...")
	case b.milestoneList.err != "":
		lines = append(lines, "Couldn't load milestones")
	case len(b.milestoneList.entries) == 0:
		lines = append(lines, "No open milestones")
	default:
		entries := b.milestoneList.entries
		// b.Height <= 0 means the terminal size is not (yet) known -- rather
		// than clamp to a nonsensical 1-row window, show every entry
		// unwindowed. Real usage always has a positive Height by the time
		// View() renders this modal (set by the initial tea.WindowSizeMsg).
		maxRowLines := len(entries)
		if b.Height > 0 {
			maxRowLines = b.Height - 8
			if maxRowLines < 1 {
				maxRowLines = 1
			}
		}
		start, end, showUp, showDown := scrollWindow(b.milestoneList.cursor, len(entries), maxRowLines)
		if showUp {
			lines = append(lines, helpStyle.Render("▲"))
		}
		for i := start; i < end; i++ {
			m := entries[i]
			selected := i == b.milestoneList.cursor

			title := truncateOutput(sanitizeSingleLine(m.Title), titleWidth-3)
			titleCell := padCell(title, titleWidth)

			bar := renderProgressBar(m.ProgressPercentage, milestoneBarWidth, !selected)

			pct := int(math.Round(m.ProgressPercentage))
			pctCell := padCell(fmt.Sprintf("%3d%%", pct), milestonePctWidth)

			total := m.ClosedIssueCount + m.OpenIssueCount
			countsCell := padCell(fmt.Sprintf("%d/%d", m.ClosedIssueCount, total), milestoneCountsWidth)

			due := "no due date"
			if m.DueOn != nil {
				due = m.DueOn.UTC().Format("2006-01-02")
			}
			dueCell := padCell(due, milestoneDueWidth)

			display := strings.Join([]string{titleCell, bar, pctCell, countsCell, dueCell}, milestoneColumnGap)
			display = selectedRowStyle(display, selected)
			lines = append(lines, display)
		}
		if showDown {
			lines = append(lines, helpStyle.Render("▼"))
		}
	}

	lines = append(lines, "")
	milestoneHints := NewStatusBar(b.milestoneListHints())
	lines = append(lines, milestoneHints.View(modalWidth, 0, 0))

	modalContent := strings.Join(lines, "\n")
	return b.renderModal(modalContent, modalWidth)
}

// padCell pads s with trailing spaces to exactly width terminal cells,
// measured via lipgloss.Width (never len(), per docs/terminal-rendering.md),
// or hard-clamps it to width cells when s is already wider. This protects
// the Milestones modal's fixed column grid from wrapping onto a second
// physical line: truncateOutput truncates by runes and can return more
// cells than requested (its "..." suffix, or a wide CJK/emoji rune), and a
// naive rune-count clamp can land mid-rune on a wide-rune boundary, which
// this pads back up to the exact target width. Returns "" for a
// non-positive width.
func padCell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	w := lipgloss.Width(s)
	if w == width {
		return s
	}
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width {
		runes = runes[:len(runes)-1]
	}
	out := string(runes)
	if got := lipgloss.Width(out); got < width {
		out += strings.Repeat(" ", width-got)
	}
	return out
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
		start, end, showUp, showDown := scrollWindow(b.agentList.cursor, len(entries), maxRowLines)
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
			display := fmt.Sprintf("  %s %s", symbol, truncateOutput(sanitizeSingleLine(entry.window.WindowName), 24))
			if ref := agentWindowRef(entry.window); ref != "" {
				display = fmt.Sprintf("  %s %s  %s", symbol, truncateOutput(sanitizeSingleLine(ref), 16), truncateOutput(sanitizeSingleLine(entry.window.WindowName), 24))
			}
			if entry.window.Agent != "" {
				display += "  " + truncateOutput(sanitizeSingleLine(entry.window.Agent), agentBadgeKindWidth)
			}
			if entry.cardNumber != 0 {
				display += fmt.Sprintf("  —  %s #%d", sanitizeSingleLine(entry.columnTitle), entry.cardNumber)
			}
			display = selectedRowStyle(display, i == b.agentList.cursor)
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
	hints := b.agentListHints()
	if len(entries) == 0 {
		hints = b.agentListEmptyHints()
	}
	agentListStatusBar := NewStatusBar(hints)
	lines = append(lines, agentListStatusBar.View(modalWidth, 0, 0))

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

	loop, loopErr := b.dispatchLoopSource()
	hints := b.dispatchModalHints()

	switch {
	case b.dispatch.loading:
		lines = append(lines, b.spinner.View()+" Checking dispatch status...")
	case b.dispatch.running:
		lines = append(lines, b.spinner.View()+" Running dispatch...")
	case b.dispatch.err != "":
		if b.dispatch.repo != "" {
			lines = append(lines, "Repo: "+b.dispatch.repo)
		}
		if b.dispatch.dir != "" {
			lines = append(lines, "Dir: "+b.dispatch.dir)
		}
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(b.dispatch.err))
	case b.dispatch.repo == "":
		// Zero-value/unset dispatchState (e.g. modal opened before the status
		// query resolves, or an unexpected empty repo from the query) is not
		// a ready state — render it as such rather than showing blank fields.
		lines = append(lines, "No repository detected.")
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
		lines = append(lines, renderLoopLine(loop, loopErr))

		if b.dispatch.confirmingLoop {
			// Fleet-wide, persistent toggle: confirm in both directions and spell
			// out the blast radius so a single keypress doesn't silently commit
			// every enrolled repo (#433).
			lines = append(lines, "")
			lines = append(lines, fmt.Sprintf("Turn dispatch loop %s? Affects all enrolled repos.", loopToggleTarget(loop)))
		} else {
			// The loop toggle needs a known current state to pick a direction;
			// omit the affordance entirely when the loop state is unknown or its
			// key is unbound. It lives on its own line under the Loop line
			// (rather than the bottom hint bar) both because it reads as an
			// action on the state directly above it, and because a fourth hint
			// overflows the modal width.
			toggleKey := panelHintKey(b.panelEntries(keymap.ModeDispatch), keymap.CommandDispatchToggleLoop)
			if loop != nil && toggleKey != "" {
				lines = append(lines, fmt.Sprintf("  %s: Turn loop %s", toggleKey, loopToggleTarget(loop)))
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
