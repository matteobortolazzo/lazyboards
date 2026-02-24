package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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

// cleanupResultMsg is sent when cleanup commands finish executing.
type cleanupResultMsg struct {
	count int
}

// runCleanupCmds returns a tea.Cmd that executes cleanup shell commands.
func runCleanupCmds(executor action.Executor, commands []string) tea.Cmd {
	return func() tea.Msg {
		count := 0
		for _, cmd := range commands {
			_, _ = executor.RunShell(cmd)
			count++
		}
		return cleanupResultMsg{count: count}
	}
}

// composeFrontmatter builds a YAML frontmatter string with title, labels (always included, bare key when empty), and body.
func composeFrontmatter(title string, labels []string, body string) string {
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("title: " + title + "\n")
	if len(labels) > 0 {
		sb.WriteString("labels: " + strings.Join(labels, ", ") + "\n")
	} else {
		sb.WriteString("labels:\n")
	}
	sb.WriteString("---\n")
	if body != "" {
		sb.WriteString(body)
	}
	return sb.String()
}

// parseFrontmatter extracts title, labels, and body from a frontmatter string.
// Returns error if the format is invalid or title is blank.
func parseFrontmatter(content string) (title string, labels []string, body string, err error) {
	// Split on "\n---\n" to avoid breaking on "---" inside the title value.
	// composeFrontmatter produces "---\ntitle: ...\n---\n..." so the closing
	// delimiter always appears as "\n---\n" (or "\n---" at EOF).
	const delim = "\n---\n"
	idx := strings.Index(content, delim)
	if idx < 0 {
		// Try "\n---" at EOF (no trailing newline after closing delimiter).
		if strings.HasSuffix(content, "\n---") {
			idx = len(content) - 4 // len("\n---")
		} else {
			return "", nil, "", errors.New("invalid frontmatter: missing closing delimiter")
		}
	}
	header := content[:idx]
	bodyStart := idx + len(delim)
	if bodyStart > len(content) {
		bodyStart = len(content)
	}

	for _, line := range strings.Split(header, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
		}
		if strings.HasPrefix(line, "labels:") {
			labelsVal := strings.TrimSpace(strings.TrimPrefix(line, "labels:"))
			if labelsVal != "" {
				for _, l := range strings.Split(labelsVal, ",") {
					trimmed := strings.TrimSpace(l)
					if trimmed != "" {
						labels = append(labels, trimmed)
					}
				}
			}
		}
	}
	if labels == nil {
		labels = []string{}
	}
	if title == "" {
		return "", nil, "", errors.New("title is required")
	}
	body = strings.TrimSpace(content[bodyStart:])
	return title, labels, body, nil
}

// resolveEditor returns the user's preferred editor.
// Checks $VISUAL, then $EDITOR, then falls back to "vi".
func resolveEditor() string {
	if v := os.Getenv("VISUAL"); v != "" {
		return v
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	return "vi"
}

// updateCardCmd returns a tea.Cmd that updates a card via the provider.
func updateCardCmd(p provider.BoardProvider, number int, title, body string, labels []string) tea.Cmd {
	return func() tea.Msg {
		card, err := p.UpdateCard(context.Background(), number, title, body, labels)
		if err != nil {
			return cardUpdateErrorMsg{err: err}
		}
		return cardUpdatedMsg{card: card}
	}
}

// openEditorCmd creates a temp file with card frontmatter, opens it in the
// user's editor via tea.ExecProcess, and returns an editorFinishedMsg on close.
func openEditorCmd(card Card) tea.Cmd {
	editor := resolveEditor()
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	originalContent := composeFrontmatter(card.Title, labelNames, card.Body)

	tmpFile, err := os.CreateTemp("", "lazyboards-*.md")
	if err != nil {
		return func() tea.Msg {
			return editorFinishedMsg{err: fmt.Errorf("failed to create temp file: %w", err), card: card}
		}
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.WriteString(originalContent); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return func() tea.Msg {
			return editorFinishedMsg{err: fmt.Errorf("failed to write temp file: %w", err), card: card}
		}
	}
	_ = tmpFile.Close()

	editorArgs := strings.Fields(editor)
	c := exec.Command(editorArgs[0], append(editorArgs[1:], tmpPath)...)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		defer func() { _ = os.Remove(tmpPath) }()
		if err != nil {
			return editorFinishedMsg{err: err, card: card}
		}
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return editorFinishedMsg{err: fmt.Errorf("failed to read temp file: %w", readErr), card: card}
		}
		editedContent := string(data)
		return editorFinishedMsg{
			editedContent:   editedContent,
			originalContent: originalContent,
			card:            card,
		}
	})
}

// createLabelCmd returns a tea.Cmd that creates a label via the provider.
func createLabelCmd(p provider.BoardProvider, name string) tea.Cmd {
	return func() tea.Msg {
		err := p.CreateLabel(context.Background(), name)
		if err != nil {
			return labelCreateErrorMsg{err: err}
		}
		return labelCreatedMsg{}
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
