package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

// boardMode represents the current interaction mode of the board.
type boardMode int

const (
	normalMode boardMode = iota
	createMode
)

// Card represents a single Kanban card (e.g., a GitHub issue).
type Card struct {
	Number int
	Title  string
	Label  string
}

// Column represents a Kanban column containing cards.
type Column struct {
	Title  string
	Cards  []Card
	Cursor int
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
}

// NewBoardFromProvider creates a Board by fetching data from the given provider.
func NewBoardFromProvider(p provider.BoardProvider) (Board, error) {
	board, err := p.FetchBoard(context.Background())
	if err != nil {
		return Board{}, fmt.Errorf("fetching board: %w", err)
	}

	cols := make([]Column, len(board.Columns))
	for i, pc := range board.Columns {
		cards := make([]Card, len(pc.Cards))
		for j, c := range pc.Cards {
			cards[j] = Card{Number: c.Number, Title: c.Title, Label: c.Label}
		}
		cols[i] = Column{Title: pc.Title, Cards: cards}
	}

	ti := textinput.New()
	ti.Placeholder = "Title"
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 40

	li := textinput.New()
	li.Placeholder = "Label"
	li.CharLimit = 50
	li.Width = 40

	return Board{
		Columns:    cols,
		titleInput: ti,
		labelInput: li,
		provider:   p,
	}, nil
}

func (b Board) Init() tea.Cmd {
	return textinput.Blink
}

func (b Board) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// ctrl+c always quits regardless of mode.
		if msg.String() == "ctrl+c" {
			return b, tea.Quit
		}

		switch b.mode {
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
				created, err := b.provider.CreateCard(context.Background(), title, label)
				if err != nil {
					b.validationErr = err.Error()
					return b, nil
				}
				newCard := Card{
					Number: created.Number,
					Title:  created.Title,
					Label:  created.Label,
				}
				b.Columns[0].Cards = append(b.Columns[0].Cards, newCard)
				b.titleInput.SetValue("")
				b.labelInput.SetValue("")
				b.validationErr = ""
				b.mode = normalMode
				return b, nil
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
			case "h", "left":
				if b.ActiveTab > 0 {
					b.ActiveTab--
				}
			case "l", "right":
				if b.ActiveTab < len(b.Columns)-1 {
					b.ActiveTab++
				}
			case "j", "down":
				col := &b.Columns[b.ActiveTab]
				if col.Cursor < len(col.Cards)-1 {
					col.Cursor++
				}
			case "k", "up":
				col := &b.Columns[b.ActiveTab]
				if col.Cursor > 0 {
					col.Cursor--
				}
			}
		}
	case tea.WindowSizeMsg:
		b.Width = msg.Width
		b.Height = msg.Height
	}
	return b, nil
}

func (b Board) View() string {
	if b.Width == 0 || len(b.Columns) == 0 {
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

	// Left panel: card list for active tab.
	col := b.Columns[b.ActiveTab]
	var leftLines []string
	for j, card := range col.Cards {
		cardText := fmt.Sprintf("#%d %s", card.Number, card.Title)
		if j == col.Cursor {
			cardText = selectedCardStyle.Render(cardText)
		}
		leftLines = append(leftLines, cardText)
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
			"\n" + fmt.Sprintf("Label: %s", card.Label)
	}
	rightPanel := rightPanelStyle.
		Width(rightContentWidth).
		Height(panelHeight).
		Render(rightContent)

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Help bar.
	helpBar := helpStyle.Render("h/l: switch tab  j/k: navigate  n: new  q: quit")

	// Assemble inner content.
	inner := lipgloss.JoinVertical(lipgloss.Left, tabBar, panels, helpBar)

	if b.mode == createMode {
		modalWidth := 40
		var errLine string
		if b.validationErr != "" {
			errLine = "\n" + lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render(b.validationErr)
		}
		modalContent := "New Card\n\n" +
			"Title:\n" + b.titleInput.View() + errLine + "\n\n" +
			"Label:\n" + b.labelInput.View()

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
