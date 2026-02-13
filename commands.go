package main

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

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
