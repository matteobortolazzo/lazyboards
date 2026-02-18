package main

import (
	"context"
	"strings"

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

const maxErrorOutputLen = 200

// runShellCmd returns a tea.Cmd that executes a shell command asynchronously.
func runShellCmd(executor action.Executor, command string) tea.Cmd {
	return func() tea.Msg {
		stderr, err := executor.RunShell(command)
		if err != nil {
			msg := "Error: " + truncateOutput(err.Error(), maxErrorOutputLen)
			if stderr != "" {
				msg = "Error: " + truncateOutput(stderr, maxErrorOutputLen)
			}
			return actionResultMsg{success: false, message: msg}
		}
		return actionResultMsg{success: true, message: "Done"}
	}
}

// truncateOutput truncates s to maxLen runes, appending "..." if truncated.
func truncateOutput(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
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

// wrapTitle wraps text at word boundaries to fit within maxWidth.
// First line uses full maxWidth; continuation lines are indented by indentWidth spaces.
// Falls back to character-break if a single word exceeds the available width.
// Returns at least one line.
func wrapTitle(text string, maxWidth int, indentWidth int) []string {
	if maxWidth <= 0 {
		return []string{text}
	}

	runes := []rune(text)
	if len(runes) <= maxWidth {
		return []string{text}
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	indent := strings.Repeat(" ", indentWidth)
	var lines []string
	isFirstLine := true

	// capacity returns the available character width for the current line.
	capacity := func() int {
		if isFirstLine {
			return maxWidth
		}
		cap := maxWidth - indentWidth
		if cap < 1 {
			cap = 1
		}
		return cap
	}

	// breakWord splits a word that exceeds cap into multiple lines.
	breakWord := func(word string, cap int) {
		wr := []rune(word)
		for len(wr) > 0 {
			take := cap
			if take > len(wr) {
				take = len(wr)
			}
			chunk := string(wr[:take])
			if !isFirstLine {
				chunk = indent + chunk
			}
			lines = append(lines, chunk)
			wr = wr[take:]
			isFirstLine = false
		}
	}

	var currentLine string
	currentLen := 0

	for _, word := range words {
		wordRunes := []rune(word)
		cap := capacity()

		if currentLen == 0 {
			// Starting a new line.
			if len(wordRunes) > cap {
				// Word is too long for the line -- character-break it.
				breakWord(word, cap)
				continue
			}
			if isFirstLine {
				currentLine = word
			} else {
				currentLine = indent + word
			}
			currentLen = len(wordRunes)
			continue
		}

		// Check if word fits on current line (with a space separator).
		if currentLen+1+len(wordRunes) <= cap {
			currentLine += " " + word
			currentLen += 1 + len(wordRunes)
		} else {
			// Flush current line.
			lines = append(lines, currentLine)
			isFirstLine = false
			currentLine = ""
			currentLen = 0

			cap = capacity()
			if len(wordRunes) > cap {
				breakWord(word, cap)
				continue
			}
			currentLine = indent + word
			currentLen = len(wordRunes)
		}
	}

	// Flush the last line.
	if currentLen > 0 {
		lines = append(lines, currentLine)
	}

	if len(lines) == 0 {
		return []string{""}
	}

	return lines
}
