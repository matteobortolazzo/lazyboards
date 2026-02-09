package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	Columns   []Column
	ActiveTab int
	Width     int
	Height    int
	mode      boardMode
}

func (b Board) Init() tea.Cmd {
	return nil
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
			}
			// All other keys in createMode are blocked (no-op).
			return b, nil

		default: // normalMode
			switch msg.String() {
			case "q":
				return b, tea.Quit
			case "n":
				b.mode = createMode
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
	helpBar := helpStyle.Render("h/l: switch tab  j/k: navigate  q: quit")

	// Assemble inner content.
	inner := lipgloss.JoinVertical(lipgloss.Left, tabBar, panels, helpBar)

	return outerStyle.Width(innerWidth).Render(inner)
}
