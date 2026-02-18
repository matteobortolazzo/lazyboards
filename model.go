package main

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// Package-level styles.
var (
	activeBorderTitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	inactiveBorderTitleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedCardStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	detailTitleStyle  = lipgloss.NewStyle().Bold(true)
	leftPanelStyle    = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("15"))
	rightPanelStyle   = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	outerStyle        = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	prIndicatorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("5"))
	workingIndicatorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	cardNumberStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	hintKeyStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	hintDescStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// labelPalette contains 8 muted 256-color ANSI codes for label coloring.
var labelPalette = []lipgloss.Color{
	lipgloss.Color("168"), // rose
	lipgloss.Color("114"), // green
	lipgloss.Color("75"),  // blue
	lipgloss.Color("222"), // gold
	lipgloss.Color("174"), // salmon
	lipgloss.Color("152"), // mauve
	lipgloss.Color("80"),  // teal
	lipgloss.Color("215"), // orange
}

// semanticLabelColors maps common label names (lowercase) to specific palette colors.
var semanticLabelColors = map[string]lipgloss.Color{
	"bug":           lipgloss.Color("168"),
	"critical":      lipgloss.Color("168"),
	"feature":       lipgloss.Color("114"),
	"enhancement":   lipgloss.Color("114"),
	"design":        lipgloss.Color("75"),
	"question":      lipgloss.Color("75"),
	"docs":          lipgloss.Color("222"),
	"documentation": lipgloss.Color("222"),
	"infra":         lipgloss.Color("174"),
	"ops":           lipgloss.Color("174"),
	"chore":         lipgloss.Color("152"),
	"refactor":      lipgloss.Color("152"),
	"test":          lipgloss.Color("80"),
	"testing":       lipgloss.Color("80"),
	"backend":       lipgloss.Color("215"),
	"ui":            lipgloss.Color("215"),
}

// labelColor returns a deterministic color for a label.
// Semantic labels get fixed colors; unknown labels use FNV-32 hash.
func labelColor(label string) lipgloss.Color {
	lower := strings.ToLower(label)
	if c, ok := semanticLabelColors[lower]; ok {
		return c
	}
	h := fnv.New32a()
	h.Write([]byte(lower))
	return labelPalette[h.Sum32()%uint32(len(labelPalette))]
}

// normalModeHints are the default status bar hints shown in normal mode.
var normalModeHints = []Hint{
	{Key: "n", Desc: "New"},
	{Key: "c", Desc: "Config"},
	{Key: "r", Desc: "Refresh"},
	{Key: "q", Desc: "Quit"},
}

// detailFocusHints are the status bar hints shown when the detail panel is focused.
var detailFocusHints = []Hint{
	{Key: "j/k", Desc: "Scroll"},
	{Key: "h", Desc: "Back"},
	{Key: "esc", Desc: "Back"},
}

// prPickerHints are the status bar hints shown when the PR picker modal is open.
var prPickerHints = []Hint{
	{Key: "\u25c0/\u25b6", Desc: "Cycle"},
	{Key: "enter", Desc: "Select"},
	{Key: "esc", Desc: "Cancel"},
}

// boardMode represents the current interaction mode of the board.
type boardMode int

const (
	normalMode boardMode = iota
	createMode
	creatingMode
	loadingMode
	errorMode
	configMode
	prPickerMode
)

const (
	statusMessageDuration     = 3 * time.Second
	longStatusMessageDuration = 30 * time.Second
)

// LinkedPR represents a pull request linked to a card.
type LinkedPR struct {
	Number int
	Title  string
	URL    string
}

// Card represents a single Kanban card (e.g., a GitHub issue).
type Card struct {
	Number    int
	Title     string
	Labels    []string
	Body      string
	LinkedPRs []LinkedPR
}

// actionResultMsg is sent when an async shell action completes.
type actionResultMsg struct {
	success bool
	message string
}

// configSavedMsg is sent when a config file has been saved successfully.
type configSavedMsg struct{}

// configSaveErrorMsg is sent when saving a config file fails.
type configSaveErrorMsg struct{ err error }

// Column represents a Kanban column containing cards.
type Column struct {
	Title        string
	Cards        []Card
	Cursor       int
	ScrollOffset int
}

// boardFetchedMsg is sent when the provider successfully returns board data.
type boardFetchedMsg struct {
	board provider.Board
}

// boardFetchErrorMsg is sent when the provider fails to fetch board data.
type boardFetchErrorMsg struct {
	err error
}

// cardCreatedMsg is sent when the provider successfully creates a card.
type cardCreatedMsg struct {
	card provider.Card
}

// cardCreateErrorMsg is sent when the provider fails to create a card.
type cardCreateErrorMsg struct {
	err error
}

// Board is the top-level model implementing tea.Model.
type Board struct {
	Columns       []Column
	ActiveTab     int
	Width         int
	Height        int
	mode          boardMode
	titleInput    textinput.Model
	labelInput    textinput.Model
	validationErr string
	provider      provider.BoardProvider
	spinner       spinner.Model
	loadErr       string
	statusBar     StatusBar
	loaded        bool
	actions       map[string]config.Action
	columnConfigs []config.ColumnConfig
	executor      action.Executor
	repoOwner       string
	repoName        string
	providerName    string
	sessionMaxLen   int
	normalHints     []Hint
	providerOptions []string
	providerIndex   int
	repoInput       textinput.Model
	configFocus     int
	configLocalPath    string
	firstLaunch        bool
	ConfigSaved        bool
	detailFocused      bool
	detailScrollOffset int
	prPickerIndex      int
	refreshing         bool
}

// NewBoard creates a Board in loadingMode (or configMode if firstLaunch).
// Call Init() to start fetching data.
func NewBoard(p provider.BoardProvider, actions map[string]config.Action, columnConfigs []config.ColumnConfig, executor action.Executor, repoOwner, repoName, providerName string, sessionMaxLen int, firstLaunch bool) Board {
	ti := textinput.New()
	ti.Placeholder = "Title"
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 40

	li := textinput.New()
	li.Placeholder = "Label"
	li.CharLimit = 50
	li.Width = 40

	s := spinner.New()
	s.Spinner = spinner.Dot

	// Build normal-mode hints: defaults + action hints.
	hints := make([]Hint, len(normalModeHints))
	copy(hints, normalModeHints)
	for key, act := range actions {
		hints = append(hints, Hint{Key: key, Desc: act.Name})
	}

	sb := NewStatusBar(hints)

	ri := textinput.New()
	ri.Placeholder = "owner/repo"
	ri.CharLimit = 100
	ri.Width = 40

	b := Board{
		mode:            loadingMode,
		titleInput:      ti,
		labelInput:      li,
		provider:        p,
		spinner:         s,
		statusBar:       sb,
		actions:         actions,
		columnConfigs:   columnConfigs,
		executor:        executor,
		repoOwner:       repoOwner,
		repoName:        repoName,
		providerName:    providerName,
		sessionMaxLen:   sessionMaxLen,
		normalHints:     hints,
		providerOptions: []string{"github", "azure-devops"},
		providerIndex:   0,
		repoInput:       ri,
		configLocalPath: config.DefaultLocalPath,
		firstLaunch:     firstLaunch,
	}

	if firstLaunch {
		b.enterConfigMode()
	}

	return b
}

// enterConfigMode sets up configMode with pre-populated values from runtime.
func (b *Board) enterConfigMode() {
	b.mode = configMode
	b.configFocus = 0
	b.validationErr = ""
	b.repoInput.Blur()

	if b.repoOwner != "" && b.repoName != "" {
		b.repoInput.SetValue(b.repoOwner + "/" + b.repoName)
	} else {
		b.repoInput.SetValue("")
	}

	b.providerIndex = 0
	for i, opt := range b.providerOptions {
		if opt == b.providerName {
			b.providerIndex = i
			break
		}
	}
}

// layoutDimensions computes the panel layout dimensions.
// panelHeight = terminal height - outer border (2) - help bar (1) - panel borders (2) = Height - 5.
// leftContentWidth = left panel content area (40% of inner width, minus border).
// rightContentWidth = right panel content area (remaining width, minus border).
func (b Board) layoutDimensions() (panelHeight, leftContentWidth, rightContentWidth int) {
	innerWidth := b.Width - 2
	leftTotal := innerWidth * 2 / 5
	leftContentWidth = leftTotal - 2
	rightTotal := innerWidth - leftTotal
	rightContentWidth = rightTotal - 2
	panelHeight = b.Height - 5
	if panelHeight < 1 {
		panelHeight = 1
	}
	return
}

// cardLineCount returns the number of visual lines a card occupies
// when its title is wrapped to fit within contentWidth.
func cardLineCount(card Card, contentWidth int) int {
	// Build the display text exactly as the view renders it
	prefix := fmt.Sprintf("#%d ", card.Number)
	text := prefix + card.Title
	if len(card.LinkedPRs) > 0 {
		text += " \ue728"
	}
	for _, label := range card.Labels {
		if label == "Working" {
			text += " \uf110"
			break
		}
	}
	for range card.Labels {
		text += " \u25cf"
	}
	return len(wrapTitle(text, contentWidth, len([]rune(prefix))))
}

func (b *Board) clampScrollOffset() {
	if len(b.Columns) == 0 {
		return
	}
	col := &b.Columns[b.ActiveTab]
	totalCards := len(col.Cards)
	if totalCards == 0 {
		col.ScrollOffset = 0
		return
	}

	panelHeight, contentWidth, _ := b.layoutDimensions()
	if contentWidth < 1 {
		contentWidth = 1
	}

	// Compute total lines for all cards.
	totalLines := 0
	for i := 0; i < totalCards; i++ {
		totalLines += cardLineCount(col.Cards[i], contentWidth)
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
			cl := cardLineCount(col.Cards[lastVisible], contentWidth)
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
			linesFromCursor := cardLineCount(col.Cards[col.Cursor], contentWidth)
			avail := panelHeight - 1 // reserve 1 for up indicator (since we're scrolling down)
			for col.ScrollOffset > 0 {
				prevLines := cardLineCount(col.Cards[col.ScrollOffset-1], contentWidth)
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

// resolveAction looks up an action by key, checking the active column's
// per-column actions first (if any), then falling back to global actions.
func (b *Board) resolveAction(key string) (config.Action, bool) {
	if len(b.Columns) > 0 && b.ActiveTab < len(b.Columns) {
		colTitle := b.Columns[b.ActiveTab].Title
		for _, cc := range b.columnConfigs {
			if strings.EqualFold(cc.Name, colTitle) {
				if act, ok := cc.Actions[key]; ok {
					return act, true
				}
				break
			}
		}
	}
	act, ok := b.actions[key]
	return act, ok
}

// rebuildNormalHints reconstructs the normalHints slice by merging global
// actions with the active column's per-column actions (column overrides global).
func (b *Board) rebuildNormalHints() {
	hints := make([]Hint, 0, len(normalModeHints)+len(b.actions)+4)

	// Number navigation hint (if columns loaded).
	if len(b.Columns) > 0 {
		hints = append(hints, Hint{Key: fmt.Sprintf("1-%d", len(b.Columns)), Desc: "Column"})
	}

	// Conditional PR hint: only show when the selected card has linked PRs.
	if len(b.Columns) > 0 && b.ActiveTab < len(b.Columns) {
		col := b.Columns[b.ActiveTab]
		if len(col.Cards) > 0 && col.Cursor < len(col.Cards) && len(col.Cards[col.Cursor].LinkedPRs) > 0 {
			hints = append(hints, Hint{Key: "p", Desc: "Open PR"})
		}
	}

	// Default mode hints.
	hints = append(hints, normalModeHints...)

	// Collect action hints: start with global, overlay column-specific.
	actionHints := make(map[string]Hint)
	for key, act := range b.actions {
		actionHints[key] = Hint{Key: key, Desc: act.Name}
	}

	// Overlay active column's actions.
	if len(b.Columns) > 0 && b.ActiveTab < len(b.Columns) {
		colTitle := b.Columns[b.ActiveTab].Title
		for _, cc := range b.columnConfigs {
			if strings.EqualFold(cc.Name, colTitle) {
				for key, act := range cc.Actions {
					actionHints[key] = Hint{Key: key, Desc: act.Name}
				}
				break
			}
		}
	}

	for _, h := range actionHints {
		hints = append(hints, h)
	}

	b.normalHints = hints
}

func mapLinkedPRs(prs []provider.LinkedPR) []LinkedPR {
	if len(prs) == 0 {
		return nil
	}
	result := make([]LinkedPR, len(prs))
	for i, pr := range prs {
		result[i] = LinkedPR{Number: pr.Number, Title: pr.Title, URL: pr.URL}
	}
	return result
}

func (b Board) Init() tea.Cmd {
	if b.firstLaunch {
		return nil
	}
	return tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
}
