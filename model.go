package main

import (
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
	activeTabStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	inactiveTabStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	selectedCardStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	detailTitleStyle  = lipgloss.NewStyle().Bold(true)
	leftPanelStyle    = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205"))
	rightPanelStyle   = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	outerStyle        = lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

// normalModeHints are the default status bar hints shown in normal mode.
var normalModeHints = []Hint{
	{Key: "n", Desc: "New"},
	{Key: "c", Desc: "Config"},
	{Key: "r", Desc: "Refresh"},
	{Key: "q", Desc: "Quit"},
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
)

// Card represents a single Kanban card (e.g., a GitHub issue).
type Card struct {
	Number int
	Title  string
	Labels []string
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
	executor      action.Executor
	repoOwner       string
	repoName        string
	providerName    string
	normalHints     []Hint
	providerOptions []string
	providerIndex   int
	repoInput       textinput.Model
	configFocus     int
	configLocalPath string
	firstLaunch     bool
	ConfigSaved     bool
}

// NewBoard creates a Board in loadingMode (or configMode if firstLaunch).
// Call Init() to start fetching data.
func NewBoard(p provider.BoardProvider, actions map[string]config.Action, executor action.Executor, repoOwner, repoName, providerName string, firstLaunch bool) Board {
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
		executor:        executor,
		repoOwner:       repoOwner,
		repoName:        repoName,
		providerName:    providerName,
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

func (b *Board) clampScrollOffset() {
	if len(b.Columns) == 0 {
		return
	}
	col := &b.Columns[b.ActiveTab]
	totalCards := len(col.Cards)
	panelHeight := b.Height - 6
	if panelHeight < 1 {
		panelHeight = 1
	}

	if totalCards <= panelHeight {
		col.ScrollOffset = 0
		return
	}

	// Iterate to find stable scroll position (converges in <=3 iterations)
	for i := 0; i < 3; i++ {
		visible := panelHeight
		if col.ScrollOffset > 0 {
			visible-- // up indicator
		}
		if col.ScrollOffset+visible < totalCards {
			visible-- // down indicator
		}
		if visible < 1 {
			visible = 1
		}

		if col.Cursor < col.ScrollOffset {
			col.ScrollOffset = col.Cursor
		} else if col.Cursor >= col.ScrollOffset+visible {
			col.ScrollOffset = col.Cursor - visible + 1
		} else {
			break
		}
	}

	// Final bounds clamp
	if col.ScrollOffset < 0 {
		col.ScrollOffset = 0
	}
	maxOffset := totalCards - 1
	if col.ScrollOffset > maxOffset {
		col.ScrollOffset = maxOffset
	}
}

func (b Board) Init() tea.Cmd {
	if b.firstLaunch {
		return nil
	}
	return tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
}
