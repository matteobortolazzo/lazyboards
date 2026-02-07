package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	Columns      []Column
	ActiveColumn int
	Width        int
	Height       int
}

func (b Board) Init() tea.Cmd {
	return nil
}

func (b Board) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return b, tea.Quit
		case "h", "left":
			if b.ActiveColumn > 0 {
				b.ActiveColumn--
			}
		case "l", "right":
			if b.ActiveColumn < len(b.Columns)-1 {
				b.ActiveColumn++
			}
		case "j", "down":
			col := &b.Columns[b.ActiveColumn]
			if col.Cursor < len(col.Cards)-1 {
				col.Cursor++
			}
		case "k", "up":
			col := &b.Columns[b.ActiveColumn]
			if col.Cursor > 0 {
				col.Cursor--
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

	borderWidth := 2 // left + right border characters
	colWidth := (b.Width / len(b.Columns)) - borderWidth

	activeStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Width(colWidth)

	inactiveStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Width(colWidth)

	selectedCardStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	headerStyle := lipgloss.NewStyle().
		Bold(true)

	var cols []string
	for i, col := range b.Columns {
		isActive := i == b.ActiveColumn

		var lines []string
		lines = append(lines, headerStyle.Render(col.Title))
		lines = append(lines, strings.Repeat("-", colWidth-4))

		for j, card := range col.Cards {
			cardText := fmt.Sprintf("#%d %s [%s]", card.Number, card.Title, card.Label)
			if isActive && j == col.Cursor {
				cardText = selectedCardStyle.Render(cardText)
			}
			lines = append(lines, cardText)
		}

		content := strings.Join(lines, "\n")

		if isActive {
			cols = append(cols, activeStyle.Render(content))
		} else {
			cols = append(cols, inactiveStyle.Render(content))
		}
	}

	board := lipgloss.JoinHorizontal(lipgloss.Top, cols...)
	helpBar := "h/l: switch panel  j/k: navigate  q: quit"

	return lipgloss.JoinVertical(lipgloss.Left, board, helpBar)
}
