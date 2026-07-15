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
	"github.com/matteobortolazzo/lazyboards/internal/cenciwatch"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
)

// subscribeCenciWatchCmd returns a tea.Cmd that reads the next snapshot from
// the cenci-watch watcher, delivering agentSnapshotMsg on success or
// cenciWatchErrorMsg on failure.
func subscribeCenciWatchCmd(w cenciwatch.Watcher) tea.Cmd {
	return func() tea.Msg {
		snap, err := w.ReadNext()
		if err != nil {
			return cenciWatchErrorMsg{err: err}
		}
		if snap == nil {
			// A nil snapshot with no error (e.g. a clean socket close or an
			// exhausted watcher) is not a usable read: route it through the
			// reconnect backoff instead of resetting and re-subscribing in a
			// tight loop.
			return cenciWatchErrorMsg{err: errors.New("cenci: watcher returned nil snapshot")}
		}
		return agentSnapshotMsg{snapshot: snap}
	}
}

// fetchBoardCmd returns a tea.Cmd that fetches board data from the provider,
// and, when includeMetadata is true, concurrently fetches collaborators, the
// authenticated user, and repo labels too. When includeMetadata is false,
// those three metadata calls are skipped entirely (metadata is gated behind
// a TTL -- see Board.metadataDue -- so most refresh cycles only need the
// board itself). The returned boardFetchedMsg.metadataRequested records
// which mode this fetch ran in, so the handler knows whether to advance
// lastMetadataFetch.
func fetchBoardCmd(p provider.BoardProvider, includeMetadata bool) tea.Cmd {
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

		go func() {
			board, err := p.FetchBoard(context.Background())
			boardCh <- boardResult{board: board, err: err}
		}()

		if !includeMetadata {
			br := <-boardCh
			if br.err != nil {
				return boardFetchErrorMsg{err: br.err}
			}
			return boardFetchedMsg{board: br.board, metadataRequested: false}
		}

		collabCh := make(chan collabResult, 1)
		authCh := make(chan authResult, 1)
		labelCh := make(chan labelResult, 1)

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

		msg := boardFetchedMsg{board: br.board, metadataRequested: true}

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

// fetchOpenPRsCmd returns a tea.Cmd that lists the repository's open pull
// requests via the provider, delivering openPRsMsg for the PR list modal's
// repo-wide view.
func fetchOpenPRsCmd(p provider.BoardProvider, generation uint64) tea.Cmd {
	return func() tea.Msg {
		prs, err := p.ListOpenPRs(context.Background())
		if err != nil {
			return openPRsMsg{err: err, generation: generation}
		}
		return openPRsMsg{prs: prs, generation: generation}
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

// runCleanupCmds returns a tea.Cmd that executes cleanup shell commands. Each
// command's outcome is logged via debuglog so cleanup executions are
// forensically traceable after an incident (#361); the returned count and
// message contract is unchanged.
func runCleanupCmds(executor action.Executor, commands []string) tea.Cmd {
	return func() tea.Msg {
		count := 0
		for _, cmd := range commands {
			stderr, err := executor.RunShell(cmd)
			if err != nil {
				debuglog.Log(fmt.Sprintf("cleanup: command failed: %s: %v (stderr: %s)", cmd, err, stderr))
			} else {
				debuglog.Log(fmt.Sprintf("cleanup: executed: %s", cmd))
			}
			count++
		}
		return cleanupResultMsg{count: count}
	}
}

// cenciTooOldMsg and cenciNoOutputMsg are the user-facing messages
// for the two ways an old/misbehaving cenci binary can be detected.
const (
	cenciTooOldMsg   = "cenci version too old — upgrade to use dispatch enrollment"
	cenciNoOutputMsg = "cenci produced no output — check that the correct binary is on PATH"
)

// isCenciNotFound reports whether err/stderr indicate that the
// cenci binary itself could not be found/executed on PATH (as opposed
// to running but failing for some other reason).
func isCenciNotFound(err error, stderr string) bool {
	// exit 127 is "command not found" from sh; match the exact os/exec format
	// ("exit status 127") rather than a bare "127" substring that could appear
	// in unrelated error text.
	notFound := err != nil && strings.Contains(err.Error(), "exit status 127")
	if strings.Contains(stderr, "not found") || strings.Contains(stderr, "command not found") {
		notFound = true
	}
	return notFound
}

// classifyCenciError maps a shell execution error/stderr pair from an
// cenci invocation into a user-facing message.
func classifyCenciError(err error, stderr string) string {
	if isCenciNotFound(err, stderr) {
		return "cenci not found on PATH — install it to use dispatch"
	}

	if strings.Contains(stderr, "is not a git repository") || strings.Contains(stderr, "getting origin remote url") {
		return "could not resolve repository — run dispatch from inside a git repo with an origin remote"
	}

	if stderr != "" {
		return "cenci: " + truncateOutput(strings.TrimSpace(stderr), maxErrorOutputLen)
	}
	if err != nil {
		return "cenci: " + truncateOutput(err.Error(), maxErrorOutputLen)
	}
	return "cenci: unknown error"
}

// resolveCenciPathSuffix runs a side-effect-free `command -v cenci`
// lookup and returns a " (using <path>)" suffix (leading space) so error
// messages can name the resolved binary -- making PATH shadowing (e.g. a
// stale ~/go/bin build shadowing the plugin-installed binary) instantly
// visible. Returns "" when resolution fails or produces no output; callers
// should only invoke this on error paths, never on success.
func resolveCenciPathSuffix(executor action.Executor) string {
	stdout, _, err := executor.RunShellOutput("command -v cenci")
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(stdout)
	if path == "" {
		return ""
	}
	return " (using " + path + ")"
}

// queryDispatchStatusCmd returns a tea.Cmd that queries cenci for the
// current working directory's repo/dir/enrollment status.
//
// It first runs a side-effect-free `cenci version` probe. An old
// cenci binary that predates the `dispatch status` verb does not fail
// cleanly on an unrecognized subcommand -- Go's flag parsing stops at the
// first positional argument and silently discards the rest of argv, so
// `dispatch status --json --dir <cwd>` degrades, on such a binary, into a
// REAL `dispatch` pass with real side effects (dispatching tickets,
// creating tmux windows). Probing first, and never running `dispatch
// status` when the probe fails for any reason other than "binary not
// found", prevents a read-only status poll from ever accidentally
// dispatching (#299).
func queryDispatchStatusCmd(executor action.Executor) tea.Cmd {
	return func() tea.Msg {
		cwd, err := os.Getwd()
		if err != nil {
			return dispatchStatusMsg{err: classifyCenciError(err, "")}
		}

		_, versionStderr, versionErr := executor.RunShellOutput("cenci version")
		if versionErr != nil {
			if isCenciNotFound(versionErr, versionStderr) {
				// The binary doesn't exist at all -- a resolved path would be
				// misleading, so skip the path-suffix lookup entirely.
				return dispatchStatusMsg{err: classifyCenciError(versionErr, versionStderr)}
			}
			return dispatchStatusMsg{err: cenciTooOldMsg + resolveCenciPathSuffix(executor)}
		}

		command := "cenci dispatch status --json --dir " + action.ShellEscape(cwd)
		stdout, stderr, err := executor.RunShellOutput(command)
		if err != nil {
			return dispatchStatusMsg{err: classifyCenciError(err, stderr)}
		}

		if strings.TrimSpace(stdout) == "" {
			return dispatchStatusMsg{err: cenciNoOutputMsg + resolveCenciPathSuffix(executor)}
		}

		var v struct {
			Repo     string                    `json:"repo"`
			Dir      string                    `json:"dir"`
			Enrolled bool                      `json:"enrolled"`
			Loop     *cenciwatch.DispatchState `json:"loop"`
		}
		if err := json.Unmarshal([]byte(stdout), &v); err != nil {
			return dispatchStatusMsg{err: cenciTooOldMsg + resolveCenciPathSuffix(executor)}
		}

		return dispatchStatusMsg{repo: v.Repo, dir: v.Dir, enrolled: v.Enrolled, loop: v.Loop}
	}
}

// toggleEnrollCmd returns a tea.Cmd that enrolls or unenrolls the current
// working directory's repo with cenci, based on the current enrolled
// state. It only checks the exec exit status; stdout content is ignored.
func toggleEnrollCmd(executor action.Executor, enrolled bool) tea.Cmd {
	return func() tea.Msg {
		cwd, err := os.Getwd()
		if err != nil {
			return dispatchEnrollMsg{err: classifyCenciError(err, "")}
		}

		sub := "enroll"
		if enrolled {
			sub = "unenroll"
		}
		command := "cenci dispatch " + sub + " --dir " + action.ShellEscape(cwd)
		_, stderr, err := executor.RunShellOutput(command)
		if err != nil {
			return dispatchEnrollMsg{err: classifyCenciError(err, stderr)}
		}
		return dispatchEnrollMsg{}
	}
}

// dispatchOnceCmd returns a tea.Cmd that runs a single fleet-wide cenci
// dispatch pass across all enrolled repos, parsing the dispatched/skipped
// counts from its stdout.
func dispatchOnceCmd(executor action.Executor) tea.Cmd {
	return func() tea.Msg {
		command := "cenci dispatch --once"
		stdout, stderr, err := executor.RunShellOutput(command)
		if err != nil {
			return dispatchRunMsg{err: classifyCenciError(err, stderr)}
		}

		// cenci prints one line per repo: "#N dispatch (...)" or
		// "#N skip: ...". Count those two forms; any other line (headers,
		// summaries, or future format additions) is deliberately ignored so a
		// minor output-format drift degrades gracefully to a count rather than
		// an error (ticket #283, Q3). The matching is intentionally
		// prefix-agnostic (strings.Contains, not anchored on a leading "#")
		// so it survives cenci's output changing from "#N …" to
		// "owner/repo#N …" (ticket #302) without requiring an update here.
		dispatched := 0
		skipped := 0
		var lines []string
		for _, line := range strings.Split(stdout, "\n") {
			switch {
			case strings.Contains(line, " dispatch "):
				dispatched++
				lines = append(lines, strings.TrimSpace(line))
			case strings.Contains(line, " skip:"):
				skipped++
				lines = append(lines, strings.TrimSpace(line))
			}
		}

		result := fmt.Sprintf("%d dispatched, %d skipped (all enrolled repos)", dispatched, skipped)
		return dispatchRunMsg{result: result, lines: lines}
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

// closeCardCmd returns a tea.Cmd that closes a card via the provider.
func closeCardCmd(p provider.BoardProvider, number int) tea.Cmd {
	return func() tea.Msg {
		card, err := p.CloseCard(context.Background(), number)
		if err != nil {
			return cardCloseErrorMsg{err: err}
		}
		return cardClosedMsg{card: mapProviderCard(card)}
	}
}

// addCommentForDeleteCmd returns a tea.Cmd that posts the delete flow's
// optional comment via the provider. AddComment returns only an error (no
// Card), so the Card is carried through the closure rather than derived from
// a provider response.
func addCommentForDeleteCmd(p provider.BoardProvider, card Card, comment string) tea.Cmd {
	return func() tea.Msg {
		if err := p.AddComment(context.Background(), card.Number, comment); err != nil {
			return deleteCommentErrorMsg{err: err}
		}
		return deleteCommentPostedMsg{card: card}
	}
}

// deleteCardCmd returns a tea.Cmd that permanently deletes a card via the
// provider. DeleteCard returns only an error (no Card), so the Card is
// carried through the closure rather than derived from a provider response.
// commentPosted indicates whether this call was reached via the
// comment-then-delete chain, so a failure can be reported as a partial
// success (comment landed but delete failed).
func deleteCardCmd(p provider.BoardProvider, card Card, commentPosted bool) tea.Cmd {
	return func() tea.Msg {
		if err := p.DeleteCard(context.Background(), card.Number); err != nil {
			return cardDeleteErrorMsg{err: err, commentPosted: commentPosted}
		}
		return cardDeletedMsg{card: card}
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
