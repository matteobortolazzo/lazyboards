package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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

	col := b.Columns[b.ActiveTab]
	leftPanel := b.viewCardList(col, panelHeight, leftContentWidth)
	rightPanel := b.viewCardDetail(col, rightContentWidth, panelHeight)

	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Help bar.
	helpBar := helpStyle.Render(b.statusBar.View())

	// Assemble inner content.
	inner := lipgloss.JoinVertical(lipgloss.Left, tabBar, panels, helpBar)

	if b.mode == createMode || b.mode == creatingMode {
		return b.viewCreateModal()
	}

	return outerStyle.Width(innerWidth).Render(inner)
}

func (b Board) viewCardList(col Column, panelHeight, contentWidth int) string {
	var leftLines []string
	totalCards := len(col.Cards)

	if totalCards <= panelHeight {
		for j, card := range col.Cards {
			cardText := fmt.Sprintf("#%d %s", card.Number, card.Title)
			cardText = truncateTitle(cardText, contentWidth)
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
			cardText = truncateTitle(cardText, contentWidth)
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
	return leftPanelStyle.
		Width(contentWidth).
		Height(panelHeight).
		Render(leftContent)
}

func (b Board) viewCardDetail(col Column, contentWidth, panelHeight int) string {
	var rightContent string
	if len(col.Cards) > 0 {
		card := col.Cards[col.Cursor]
		rightContent = detailTitleStyle.Render(fmt.Sprintf("#%d %s", card.Number, card.Title)) +
			"\n" + fmt.Sprintf("Labels: %s", strings.Join(card.Labels, ", "))
	}
	return rightPanelStyle.
		Width(contentWidth).
		Height(panelHeight).
		Render(rightContent)
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
			"Title:\n" + b.titleInput.View() + errLine + "\n\n" +
			"Label:\n" + b.labelInput.View() + "\n\n" +
			helpStyle.Render(createHints.View())
	}

	modalStyle := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("15")).
		Padding(1, 2).
		Width(modalWidth)

	modal := modalStyle.Render(modalContent)
	return lipgloss.Place(b.Width, b.Height, lipgloss.Center, lipgloss.Center, modal)
}

func (b Board) viewConfigModal() string {
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
		BorderForeground(lipgloss.Color("15")).
		Padding(1, 2).
		Width(modalWidth)

	modal := modalStyle.Render(modalContent)
	return lipgloss.Place(b.Width, b.Height, lipgloss.Center, lipgloss.Center, modal)
}
