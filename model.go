package main

import (
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
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
// If the label has a provider-supplied hex color, it is used directly.
// Otherwise, semantic labels get fixed colors; unknown labels use FNV-32 hash.
func labelColor(label Label) lipgloss.Color {
	if label.Color != "" {
		return lipgloss.Color("#" + label.Color)
	}
	lower := strings.ToLower(label.Name)
	if c, ok := semanticLabelColors[lower]; ok {
		return c
	}
	h := fnv.New32a()
	h.Write([]byte(lower))
	return labelPalette[h.Sum32()%uint32(len(labelPalette))]
}

// normalModeHints are the default status bar hints shown in normal mode.
var normalModeHints = []Hint{
	{Key: "o", Desc: "Open"},
	{Key: "e", Desc: "Edit"},
	{Key: "n", Desc: "New"},
}

// detailFocusHints are the status bar hints shown when the detail panel is focused.
var detailFocusHints = []Hint{
	{Key: "e", Desc: "Edit"},
	{Key: "j/k", Desc: "Scroll"},
	{Key: "h", Desc: "Back"},
	{Key: "esc", Desc: "Back"},
}

// searchModeHints are the status bar hints shown when search mode is active.
var searchModeHints = []Hint{
	{Key: "esc", Desc: "Clear"},
}

// prPickerHints are the status bar hints shown when the PR picker modal is open.
var prPickerHints = []Hint{
	{Key: "\u25c0/\u25b6", Desc: "Cycle"},
	{Key: "enter", Desc: "Select"},
	{Key: "esc", Desc: "Cancel"},
}

// helpModeHints are the status bar hints shown in help mode.
var helpModeHints = []Hint{
	{Key: "esc/?", Desc: "Close"},
	{Key: "j/k", Desc: "Scroll"},
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
	searchMode
	helpMode
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

// Label represents a card label with an optional hex color.
type Label struct {
	Name  string
	Color string
}

// Card represents a single Kanban card (e.g., a GitHub issue).
type Card struct {
	Number    int
	Title     string
	Labels    []Label
	Body      string
	LinkedPRs []LinkedPR
}

// refreshTickMsg is sent when the periodic refresh timer fires.
type refreshTickMsg struct{}

// actionResultMsg is sent when an async shell action completes.
type actionResultMsg struct {
	success bool
	message string
}

// autoRefreshMsg is sent when the auto-refresh delay timer fires.
type autoRefreshMsg struct{}

// configSavedMsg is sent when a config file has been saved successfully.
type configSavedMsg struct{}

// configSaveErrorMsg is sent when saving a config file fails.
type configSaveErrorMsg struct{ err error }

// prevCardInfo stores a card's column position and metadata for departure detection.
type prevCardInfo struct {
	colIdx int
	title  string
	labels []string
}

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

// editorFinishedMsg is sent when the external editor process closes.
type editorFinishedMsg struct {
	editedContent   string
	originalContent string
	card            Card
	err             error
}

// cardUpdatedMsg is sent when the provider successfully updates a card.
type cardUpdatedMsg struct {
	card provider.Card
}

// cardUpdateErrorMsg is sent when the provider fails to update a card.
type cardUpdateErrorMsg struct {
	err error
}

// configState groups fields related to the config modal.
type configState struct {
	providerOptions []string
	providerIndex   int
	repoInput       textinput.Model
	focus           int
	localPath       string
	firstLaunch     bool
	configSaved     bool
}

// createState groups fields related to the create-card modal.
type createState struct {
	titleInput textarea.Model
	labelInput textinput.Model
}

// Board is the top-level model implementing tea.Model.
type Board struct {
	Columns       []Column
	ActiveTab     int
	Width         int
	Height        int
	mode          boardMode
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
	config          configState
	create          createState
	detailFocused      bool
	detailScrollOffset int
	prPickerIndex      int
	refreshing             bool
	refreshInterval        time.Duration
	actionRefreshDelay     time.Duration
	pendingAutoRefresh     bool
	prevCards              map[int]prevCardInfo
	searchQuery            string
	searchInput            textinput.Model
	helpScrollOffset       int
	helpFromDetailFocused  bool
	workingLabel           string
	mouseEnabled           bool
}

// NewBoard creates a Board in loadingMode (or configMode if firstLaunch).
// Call Init() to start fetching data.
func NewBoard(p provider.BoardProvider, actions map[string]config.Action, columnConfigs []config.ColumnConfig, executor action.Executor, repoOwner, repoName, providerName string, sessionMaxLen int, refreshInterval time.Duration, actionRefreshDelay time.Duration, workingLabel string, mouseEnabled bool, firstLaunch bool) Board {
	ti := textarea.New()
	ti.Placeholder = "Title"
	ti.CharLimit = 0
	ti.ShowLineNumbers = false
	ti.KeyMap.InsertNewline.SetEnabled(false)

	li := textinput.New()
	li.Placeholder = "Label"
	li.CharLimit = 50

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

	si := textinput.New()
	si.Placeholder = "Search..."
	si.CharLimit = 100
	si.Prompt = "/ "

	b := Board{
		mode:            loadingMode,
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
		refreshInterval:    refreshInterval,
		actionRefreshDelay: actionRefreshDelay,
		workingLabel:       workingLabel,
		mouseEnabled:      mouseEnabled,
		normalHints:     hints,
		config: configState{
			providerOptions: []string{"github", "azure-devops"},
			providerIndex:   0,
			repoInput:       ri,
			localPath:       config.DefaultLocalPath,
			firstLaunch:     firstLaunch,
		},
		create: createState{
			titleInput: ti,
			labelInput: li,
		},
		searchInput: si,
	}

	if firstLaunch {
		b.enterConfigMode()
	}

	return b
}

// enterConfigMode sets up configMode with pre-populated values from runtime.
func (b *Board) enterConfigMode() {
	b.mode = configMode
	b.config.focus = 0
	b.validationErr = ""
	b.config.repoInput.Blur()

	if b.repoOwner != "" && b.repoName != "" {
		b.config.repoInput.SetValue(b.repoOwner + "/" + b.repoName)
	} else {
		b.config.repoInput.SetValue("")
	}

	b.config.providerIndex = 0
	for i, opt := range b.config.providerOptions {
		if opt == b.providerName {
			b.config.providerIndex = i
			break
		}
	}
}

// createModalWidth returns the modal width for the create-card dialog (60% of terminal width, min 20).
func (b Board) createModalWidth() int {
	w := b.Width * 60 / 100
	if w < 20 {
		w = 20
	}
	return w
}

// recalcCreateInputs updates the title textarea and label input widths and
// the textarea height based on current terminal dimensions and content.
func (b *Board) recalcCreateInputs() {
	modalWidth := b.createModalWidth()
	// renderModal uses Padding(1, 2): 2 chars left + 2 chars right = 4 chars padding
	// Plus border: 1 char left + 1 char right = 2 chars
	// Total horizontal overhead = 6
	// The textarea.Width() getter subtracts the prompt width (2 chars for "> "),
	// so we add that back when calling SetWidth to get the desired Width() value.
	innerWidth := modalWidth - 6
	if innerWidth < 1 {
		innerWidth = 1
	}

	promptWidth := lipgloss.Width(b.create.titleInput.Prompt)
	b.create.titleInput.SetWidth(innerWidth + promptWidth)
	b.create.labelInput.Width = innerWidth

	// Auto-expand textarea height based on visual (wrapped) line count.
	// LineCount() returns logical lines (separated by newlines), but since
	// newline insertion is disabled, we need to count wrapped visual lines.
	contentWidth := b.create.titleInput.Width()
	if contentWidth < 1 {
		contentWidth = 1
	}
	visualLines := 0
	value := b.create.titleInput.Value()
	if value == "" {
		visualLines = 1
	} else {
		for _, line := range strings.Split(value, "\n") {
			w := lipgloss.Width(line)
			if w == 0 {
				visualLines++
			} else {
				visualLines += (w + contentWidth - 1) / contentWidth
			}
		}
	}
	maxHeight := b.Height * 50 / 100
	if maxHeight < 1 {
		maxHeight = 1
	}
	if visualLines > maxHeight {
		visualLines = maxHeight
	}
	b.create.titleInput.SetHeight(visualLines)
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
	hints := make([]Hint, 0, len(normalModeHints)+len(b.actions)+1)

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

func mapLabels(labels []provider.Label) []Label {
	if len(labels) == 0 {
		return nil
	}
	result := make([]Label, len(labels))
	for i, l := range labels {
		result[i] = Label{Name: l.Name, Color: l.Color}
	}
	return result
}

// filteredCards returns the cards in the active column that match the current
// search query. If the query is empty, all cards are returned.
func (b *Board) filteredCards() []Card {
	col := b.Columns[b.ActiveTab]
	if b.searchQuery == "" {
		return col.Cards
	}
	query := strings.ToLower(b.searchQuery)
	var result []Card
	for _, card := range col.Cards {
		if matchesSearch(card, query) {
			result = append(result, card)
		}
	}
	return result
}

// matchesSearch returns true if a card matches the search query.
// It checks the card title, card number, and label names (all case-insensitive).
func matchesSearch(card Card, query string) bool {
	if strings.Contains(strings.ToLower(card.Title), query) {
		return true
	}
	if strings.Contains(strconv.Itoa(card.Number), query) {
		return true
	}
	for _, label := range card.Labels {
		if strings.Contains(strings.ToLower(label.Name), query) {
			return true
		}
	}
	return false
}

// clearSearch resets the search state: clears the query, input, and resets
// cursor/scroll for the active column.
func (b *Board) clearSearch() {
	b.searchQuery = ""
	b.searchInput.SetValue("")
	b.searchInput.Blur()
	col := &b.Columns[b.ActiveTab]
	col.Cursor = 0
	col.ScrollOffset = 0
}

func (b Board) Init() tea.Cmd {
	if b.config.firstLaunch {
		return nil
	}
	return tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
}
