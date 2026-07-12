package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/agentwatch"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// subscribeAgentWatchCmd returns a tea.Cmd that reads the next snapshot from
// the agentwatch watcher, delivering agentSnapshotMsg on success or
// agentWatchErrorMsg on failure.
func subscribeAgentWatchCmd(w agentwatch.Watcher) tea.Cmd {
	return func() tea.Msg {
		snap, err := w.ReadNext()
		if err != nil {
			return agentWatchErrorMsg{err: err}
		}
		if snap == nil {
			// A nil snapshot with no error (e.g. a clean socket close or an
			// exhausted watcher) is not a usable read: route it through the
			// reconnect backoff instead of resetting and re-subscribing in a
			// tight loop.
			return agentWatchErrorMsg{err: errors.New("agentwatch: watcher returned nil snapshot")}
		}
		return agentSnapshotMsg{snapshot: snap}
	}
}

// fetchBoardCmd returns a tea.Cmd that fetches board data, collaborators,
// and the authenticated user concurrently from the provider.
func fetchBoardCmd(p provider.BoardProvider) tea.Cmd {
	return func() tea.Msg {
		type boardResult struct {
			board provider.Board
			err   error
		}
		type collabResult struct {
			collaborators []provider.Assignee
			err           error
		}
		type authResult struct {
			user string
			err  error
		}
		type labelResult struct {
			labels []string
			err    error
		}

		boardCh := make(chan boardResult, 1)
		collabCh := make(chan collabResult, 1)
		authCh := make(chan authResult, 1)
		labelCh := make(chan labelResult, 1)

		go func() {
			board, err := p.FetchBoard(context.Background())
			boardCh <- boardResult{board: board, err: err}
		}()
		go func() {
			collabs, err := p.FetchCollaborators(context.Background())
			collabCh <- collabResult{collaborators: collabs, err: err}
		}()
		go func() {
			user, err := p.GetAuthenticatedUser(context.Background())
			authCh <- authResult{user: user, err: err}
		}()
		go func() {
			labels, err := p.ListLabels(context.Background())
			labelCh <- labelResult{labels: labels, err: err}
		}()

		br := <-boardCh
		if br.err != nil {
			return boardFetchErrorMsg{err: br.err}
		}

		cr := <-collabCh
		ar := <-authCh
		lr := <-labelCh

		msg := boardFetchedMsg{board: br.board}

		if cr.err == nil {
			msg.collaborators = cr.collaborators
		} else {
			msg.collaboratorErr = cr.err
		}

		if ar.err == nil {
			msg.authenticatedUser = ar.user
		}

		if lr.err == nil {
			msg.repoLabels = lr.labels
		} else {
			msg.labelErr = lr.err
		}

		return msg
	}
}

// fetchGitStatusCmd returns a tea.Cmd that reads live git status from dir via
// reader, delivering gitStatusMsg with either the parsed Status or the read
// error. Exported behavior (not name) is reusable by future git-panel work
// (#271) as the same hook point for "refresh status after a git action".
func fetchGitStatusCmd(reader gitdetect.Reader, dir string) tea.Cmd {
	return func() tea.Msg {
		status, err := reader.Read(dir)
		if err != nil {
			return gitStatusMsg{err: err}
		}
		return gitStatusMsg{status: status}
	}
}

// scheduleGitStatusTick returns a tea.Cmd that fires a gitStatusTickMsg after
// gitStatusPollInterval, as an independent-interval poll (not chained off the
// fetch's completion) so it can't spin unthrottled on an ambiguous read
// result. Returns nil when the board has no git reader configured.
func scheduleGitStatusTick(b Board) tea.Cmd {
	if b.gitReader == nil {
		return nil
	}
	return tea.Tick(gitStatusPollInterval, func(time.Time) tea.Msg {
		return gitStatusTickMsg{}
	})
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

// classifyAgentwatchError maps a shell execution error/stderr pair from an
// agentwatch invocation into a user-facing message.
func classifyAgentwatchError(err error, stderr string) string {
	// exit 127 is "command not found" from sh; match the exact os/exec format
	// ("exit status 127") rather than a bare "127" substring that could appear
	// in unrelated error text.
	notFound := err != nil && strings.Contains(err.Error(), "exit status 127")
	if strings.Contains(stderr, "not found") || strings.Contains(stderr, "command not found") {
		notFound = true
	}
	if notFound {
		return "agentwatch not found on PATH — install it to use dispatch"
	}

	if strings.Contains(stderr, "is not a git repository") || strings.Contains(stderr, "getting origin remote url") {
		return "could not resolve repository — run dispatch from inside a git repo with an origin remote"
	}

	if stderr != "" {
		return "agentwatch: " + truncateOutput(strings.TrimSpace(stderr), maxErrorOutputLen)
	}
	if err != nil {
		return "agentwatch: " + truncateOutput(err.Error(), maxErrorOutputLen)
	}
	return "agentwatch: unknown error"
}

// queryDispatchStatusCmd returns a tea.Cmd that queries agentwatch for the
// current working directory's repo/dir/enrollment status.
func queryDispatchStatusCmd(executor action.Executor) tea.Cmd {
	return func() tea.Msg {
		cwd, err := os.Getwd()
		if err != nil {
			return dispatchStatusMsg{err: classifyAgentwatchError(err, "")}
		}

		command := "agentwatch dispatch status --json --dir " + action.ShellEscape(cwd)
		stdout, stderr, err := executor.RunShellOutput(command)
		if err != nil {
			return dispatchStatusMsg{err: classifyAgentwatchError(err, stderr)}
		}

		if strings.TrimSpace(stdout) == "" {
			return dispatchStatusMsg{err: "agentwatch produced no output — check that the correct binary is on PATH"}
		}

		var v struct {
			Repo     string `json:"repo"`
			Dir      string `json:"dir"`
			Enrolled bool   `json:"enrolled"`
		}
		if err := json.Unmarshal([]byte(stdout), &v); err != nil {
			return dispatchStatusMsg{err: "agentwatch version too old — upgrade to use dispatch enrollment"}
		}

		return dispatchStatusMsg{repo: v.Repo, dir: v.Dir, enrolled: v.Enrolled}
	}
}

// toggleEnrollCmd returns a tea.Cmd that enrolls or unenrolls the current
// working directory's repo with agentwatch, based on the current enrolled
// state. It only checks the exec exit status; stdout content is ignored.
func toggleEnrollCmd(executor action.Executor, enrolled bool) tea.Cmd {
	return func() tea.Msg {
		cwd, err := os.Getwd()
		if err != nil {
			return dispatchEnrollMsg{err: classifyAgentwatchError(err, "")}
		}

		sub := "enroll"
		if enrolled {
			sub = "unenroll"
		}
		command := "agentwatch dispatch " + sub + " --dir " + action.ShellEscape(cwd)
		_, stderr, err := executor.RunShellOutput(command)
		if err != nil {
			return dispatchEnrollMsg{err: classifyAgentwatchError(err, stderr)}
		}
		return dispatchEnrollMsg{}
	}
}

// dispatchOnceCmd returns a tea.Cmd that runs a single fleet-wide agentwatch
// dispatch pass across all enrolled repos, parsing the dispatched/skipped
// counts from its stdout.
func dispatchOnceCmd(executor action.Executor) tea.Cmd {
	return func() tea.Msg {
		command := "agentwatch dispatch --once"
		stdout, stderr, err := executor.RunShellOutput(command)
		if err != nil {
			return dispatchRunMsg{err: classifyAgentwatchError(err, stderr)}
		}

		// agentwatch prints one line per repo: "#N dispatch (...)" or
		// "#N skip: ...". Count those two forms; any other line (headers,
		// summaries, or future format additions) is deliberately ignored so a
		// minor output-format drift degrades gracefully to a count rather than
		// an error (ticket #283, Q3).
		dispatched := 0
		skipped := 0
		for _, line := range strings.Split(stdout, "\n") {
			switch {
			case strings.Contains(line, " dispatch "):
				dispatched++
			case strings.Contains(line, " skip:"):
				skipped++
			}
		}

		result := fmt.Sprintf("%d dispatched, %d skipped (all enrolled repos)", dispatched, skipped)
		return dispatchRunMsg{result: result}
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

// setAssigneesCmd returns a tea.Cmd that sets assignees on a card via the provider.
func setAssigneesCmd(p provider.BoardProvider, number int, logins []string) tea.Cmd {
	return func() tea.Msg {
		card, err := p.SetAssignees(context.Background(), number, logins)
		if err != nil {
			return assigneesUpdateErrorMsg{err: err}
		}
		return assigneesUpdatedMsg{card: card}
	}
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
