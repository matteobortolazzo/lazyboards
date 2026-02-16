package main

import (
	"context"
	"fmt"
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
}

// NewBoard creates a Board in loadingMode. Call Init() to start fetching data.
func NewBoard(p provider.BoardProvider, actions map[string]config.Action, executor action.Executor, repoOwner, repoName, providerName string) Board {
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

	return Board{
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
	}
}

// fetchBoardCmd returns a tea.Cmd that fetches board data from the provider.
func fetchBoardCmd(p provider.BoardProvider) tea.Cmd {
	return func() tea.Msg {
		board, err := p.FetchBoard(context.Background())
		if err != nil {
			return boardFetchErrorMsg{err: err}
		}
		return boardFetchedMsg{board: board}
	}
}

// createCardCmd returns a tea.Cmd that creates a card via the provider.
func createCardCmd(p provider.BoardProvider, title, label string) tea.Cmd {
	return func() tea.Msg {
		card, err := p.CreateCard(context.Background(), title, label)
		if err != nil {
			return cardCreateErrorMsg{err: err}
		}
		return cardCreatedMsg{card: card}
	}
}

// runShellCmd returns a tea.Cmd that executes a shell command asynchronously.
func runShellCmd(executor action.Executor, command string) tea.Cmd {
	return func() tea.Msg {
		stderr, err := executor.RunShell(command)
		if err != nil {
			msg := "Error: " + err.Error()
			if stderr != "" {
				msg = "Error: " + stderr
			}
			return actionResultMsg{success: false, message: msg}
		}
		return actionResultMsg{success: true, message: "Done"}
	}
}

// saveConfigCmd returns a tea.Cmd that saves the config file.
func saveConfigCmd(path, provider, repo string) tea.Cmd {
	return func() tea.Msg {
		if err := config.Save(path, provider, repo); err != nil {
			return configSaveErrorMsg{err: err}
		}
		return configSavedMsg{}
	}
}

func truncateTitle(s string, maxWidth int) string {
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s
	}
	if maxWidth <= 3 {
		return string(runes[:maxWidth])
	}
	return string(runes[:maxWidth-3]) + "..."
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
	return tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
}

func (b Board) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearStatusMsg:
		b.statusBar.ClearMessage()
		return b, nil

	case boardFetchedMsg:
		cols := make([]Column, len(msg.board.Columns))
		for i, pc := range msg.board.Columns {
			cards := make([]Card, len(pc.Cards))
			for j, c := range pc.Cards {
				cards[j] = Card{Number: c.Number, Title: c.Title, Labels: c.Labels}
			}
			cols[i] = Column{Title: pc.Title, Cards: cards}
		}
		b.Columns = cols
		b.mode = normalMode
		var cmd tea.Cmd
		if b.loaded {
			b.statusBar.SetActionHints(b.normalHints)
			cmd = b.statusBar.SetTimedMessage("Board refreshed", 3*time.Second)
		}
		b.loaded = true
		return b, cmd

	case boardFetchErrorMsg:
		b.mode = errorMode
		b.loadErr = msg.err.Error()
		b.statusBar.SetActionHints([]Hint{
			{Key: "r", Desc: "Retry"},
			{Key: "q", Desc: "Quit"},
		})
		return b, nil

	case cardCreatedMsg:
		newCard := Card{
			Number: msg.card.Number,
			Title:  msg.card.Title,
			Labels: msg.card.Labels,
		}
		b.Columns[0].Cards = append(b.Columns[0].Cards, newCard)
		b.titleInput.SetValue("")
		b.labelInput.SetValue("")
		b.validationErr = ""
		b.mode = normalMode
		return b, nil

	case cardCreateErrorMsg:
		b.validationErr = msg.err.Error()
		b.mode = createMode
		cmd := b.titleInput.Focus()
		b.labelInput.Blur()
		return b, cmd

	case configSavedMsg:
		b.mode = loadingMode
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))

	case configSaveErrorMsg:
		b.validationErr = msg.err.Error()
		b.mode = configMode
		return b, nil

	case actionResultMsg:
		cmd := b.statusBar.SetTimedMessage(msg.message, 3*time.Second)
		return b, cmd

	case spinner.TickMsg:
		if b.mode == loadingMode || b.mode == creatingMode {
			var cmd tea.Cmd
			b.spinner, cmd = b.spinner.Update(msg)
			return b, cmd
		}
		return b, nil

	case tea.KeyMsg:
		// ctrl+c always quits regardless of mode.
		if msg.String() == "ctrl+c" {
			return b, tea.Quit
		}

		switch b.mode {
		case loadingMode, creatingMode:
			// Ignore all keys while loading or creating.
			return b, nil

		case errorMode:
			switch msg.String() {
			case "q":
				return b, tea.Quit
			case "r":
				b.mode = loadingMode
				b.loadErr = ""
				return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
			}
			return b, nil

		case createMode:
			switch msg.Type {
			case tea.KeyEscape:
				b.mode = normalMode
				return b, nil
			case tea.KeyEnter:
				title := strings.TrimSpace(b.titleInput.Value())
				if title == "" {
					b.validationErr = "Title is required"
					return b, nil
				}
				label := strings.TrimSpace(b.labelInput.Value())
				for _, col := range b.Columns {
					if strings.EqualFold(col.Title, label) {
						b.validationErr = "Cannot use reserved column label"
						return b, nil
					}
				}
				b.mode = creatingMode
				b.titleInput.Blur()
				b.labelInput.Blur()
				return b, tea.Batch(b.spinner.Tick, createCardCmd(b.provider, title, label))
			case tea.KeyTab:
				var cmd tea.Cmd
				if b.titleInput.Focused() {
					b.titleInput.Blur()
					cmd = b.labelInput.Focus()
				} else {
					b.labelInput.Blur()
					cmd = b.titleInput.Focus()
				}
				return b, cmd
			default:
				b.validationErr = ""
				var cmd tea.Cmd
				if b.titleInput.Focused() {
					b.titleInput, cmd = b.titleInput.Update(msg)
				} else if b.labelInput.Focused() {
					b.labelInput, cmd = b.labelInput.Update(msg)
				}
				return b, cmd
			}

		case configMode:
			switch msg.Type {
			case tea.KeyEscape:
				b.mode = normalMode
				return b, nil
			case tea.KeyEnter:
				provider := b.providerOptions[b.providerIndex]
				repo := strings.TrimSpace(b.repoInput.Value())
				if repo == "" {
					b.validationErr = "Repository is required"
					return b, nil
				}
				b.validationErr = ""
				return b, saveConfigCmd(b.configLocalPath, provider, repo)
			case tea.KeyTab:
				if b.configFocus == 0 {
					b.configFocus = 1
					cmd := b.repoInput.Focus()
					return b, cmd
				} else {
					b.configFocus = 0
					b.repoInput.Blur()
					return b, nil
				}
			case tea.KeyRight:
				if b.configFocus == 0 {
					b.providerIndex = (b.providerIndex + 1) % len(b.providerOptions)
				}
				return b, nil
			case tea.KeyLeft:
				if b.configFocus == 0 {
					b.providerIndex = (b.providerIndex - 1 + len(b.providerOptions)) % len(b.providerOptions)
				}
				return b, nil
			default:
				if b.configFocus == 1 {
					var cmd tea.Cmd
					b.repoInput, cmd = b.repoInput.Update(msg)
					return b, cmd
				}
				return b, nil
			}

		default: // normalMode
			switch msg.String() {
			case "q":
				return b, tea.Quit
			case "n":
				b.mode = createMode
				b.titleInput.SetValue("")
				b.labelInput.SetValue("")
				b.titleInput.Focus()
				b.labelInput.Blur()
			case "c":
				b.mode = configMode
				b.configFocus = 0
				b.repoInput.SetValue("")
				b.providerIndex = 0
				b.repoInput.Blur()
				b.validationErr = ""
			case "r":
				b.mode = loadingMode
				b.statusBar.ClearMessage()
				return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
			case "h", "left":
				if b.ActiveTab > 0 {
					b.ActiveTab--
					b.Columns[b.ActiveTab].ScrollOffset = 0
					b.clampScrollOffset()
				}
			case "l", "right":
				if b.ActiveTab < len(b.Columns)-1 {
					b.ActiveTab++
					b.Columns[b.ActiveTab].ScrollOffset = 0
					b.clampScrollOffset()
				}
			case "j", "down":
				col := &b.Columns[b.ActiveTab]
				if col.Cursor < len(col.Cards)-1 {
					col.Cursor++
				}
				b.clampScrollOffset()
			case "k", "up":
				col := &b.Columns[b.ActiveTab]
				if col.Cursor > 0 {
					col.Cursor--
				}
				b.clampScrollOffset()
			default:
				// Check if it's a custom action key.
				if act, ok := b.actions[msg.String()]; ok {
					col := b.Columns[b.ActiveTab]
					if len(col.Cards) == 0 {
						return b, nil
					}
					card := col.Cards[col.Cursor]
					vars := action.BuildTemplateVars(card.Number, card.Title, card.Labels, b.repoOwner, b.repoName, b.providerName)

					switch act.Type {
					case "url":
						expanded := action.ExpandTemplate(act.URL, vars)
						if err := b.executor.OpenURL(expanded); err != nil {
							cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), 3*time.Second)
							return b, cmd
						}
						return b, nil
					case "shell":
						expanded := action.ExpandTemplate(act.Command, action.BuildShellSafeVars(vars))
						cmd := b.statusBar.SetTimedMessage("Running...", 30*time.Second)
						return b, tea.Batch(cmd, runShellCmd(b.executor, expanded))
					}
				}
			}
		}

	case tea.WindowSizeMsg:
		b.Width = msg.Width
		b.Height = msg.Height
		if len(b.Columns) > 0 {
			b.clampScrollOffset()
		}
	}
	return b, nil
}

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

	// Left panel: card list for active tab with scrolling.
	col := b.Columns[b.ActiveTab]
	var leftLines []string
	totalCards := len(col.Cards)

	if totalCards <= panelHeight {
		// All cards fit — no scrolling needed
		for j, card := range col.Cards {
			cardText := fmt.Sprintf("#%d %s", card.Number, card.Title)
			cardText = truncateTitle(cardText, leftContentWidth)
			if j == col.Cursor {
				cardText = selectedCardStyle.Render(cardText)
			}
			leftLines = append(leftLines, cardText)
		}
	} else {
		visible := panelHeight
		showUp := col.ScrollOffset > 0
		if showUp {
			visible--
		}
		showDown := col.ScrollOffset+visible < totalCards
		if showDown {
			visible--
		}
		if visible < 1 {
			visible = 1
		}

		endIdx := col.ScrollOffset + visible
		if endIdx > totalCards {
			endIdx = totalCards
		}

		if showUp {
			leftLines = append(leftLines, "\u25b2")
		}
		for j := col.ScrollOffset; j < endIdx; j++ {
			card := col.Cards[j]
			cardText := fmt.Sprintf("#%d %s", card.Number, card.Title)
			cardText = truncateTitle(cardText, leftContentWidth)
			if j == col.Cursor {
				cardText = selectedCardStyle.Render(cardText)
			}
			leftLines = append(leftLines, cardText)
		}
		if showDown {
			leftLines = append(leftLines, "\u25bc")
		}
	}
	leftContent := strings.Join(leftLines, "\n")
	leftPanel := leftPanelStyle.
		Width(leftContentWidth).
		Height(panelHeight).
		Render(leftContent)

	// Right panel: detail for selected card.
	var rightContent string
	if len(col.Cards) > 0 {
		card := col.Cards[col.Cursor]
		rightContent = detailTitleStyle.Render(fmt.Sprintf("#%d %s", card.Number, card.Title)) +
			"\n" + fmt.Sprintf("Labels: %s", strings.Join(card.Labels, ", "))
	}
	rightPanel := rightPanelStyle.
		Width(rightContentWidth).
		Height(panelHeight).
		Render(rightContent)

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Help bar.
	helpBar := helpStyle.Render(b.statusBar.View())

	// Assemble inner content.
	inner := lipgloss.JoinVertical(lipgloss.Left, tabBar, panels, helpBar)

	if b.mode == createMode || b.mode == creatingMode {
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
			BorderForeground(lipgloss.Color("205")).
			Padding(1, 2).
			Width(modalWidth)

		modal := modalStyle.Render(modalContent)

		return lipgloss.Place(b.Width, b.Height, lipgloss.Center, lipgloss.Center, modal)
	}

	if b.mode == configMode {
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
			BorderForeground(lipgloss.Color("205")).
			Padding(1, 2).
			Width(modalWidth)

		modal := modalStyle.Render(modalContent)
		return lipgloss.Place(b.Width, b.Height, lipgloss.Center, lipgloss.Center, modal)
	}

	return outerStyle.Width(innerWidth).Render(inner)
}
