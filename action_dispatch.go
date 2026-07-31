package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/debuglog"
	gitutil "github.com/matteobortolazzo/lazyboards/internal/git"
)

// dispatchActionWithAlt dispatches act, entering comment mode first when Alt
// was held and the action's template uses {comment} (the Alt+Shift+key
// comment flow, extended to key sequences where Alt may be held on any key).
func (b Board) dispatchActionWithAlt(act config.Action, alt bool) (tea.Model, tea.Cmd) {
	if alt && strings.Contains(act.URL+act.Command, "{comment}") {
		// Resolve the pending card (if card-scope or pr-scope) before
		// touching any state, so a "no card visible" refusal leaves b
		// untouched.
		var pendingCard Card
		if act.Scope != "board" {
			if len(b.visibleCards()) == 0 {
				return b, nil
			}
			pendingCard = b.selectedCard()
		}
		ci := textinput.New()
		ci.Placeholder = "Comment..."
		ci.CharLimit = 2000
		b.comment = commentState{
			input:             ci,
			pendingAction:     act,
			pendingCard:       pendingCard,
			boardScope:        act.Scope == "board",
			prScope:           act.Scope == "pr",
			fromDetailFocused: b.detailFocused,
		}
		b.detailFocused = false
		b.mode = commentMode
		b.statusBar.SetActionHints(commentModeHints)
		return b, b.comment.input.Focus()
	}
	// No Alt, or Alt on an action without {comment} -- execute normally.
	return b.dispatchResolvedAction(act)
}

// dispatchResolvedAction runs act against the currently selected card (or the
// whole board for board scope), applying the same scope gating used by both
// the plain-key and Alt+key custom-action dispatch paths.
func (b Board) dispatchResolvedAction(act config.Action) (tea.Model, tea.Cmd) {
	if act.Scope == "board" {
		return b.handleBoardActionKey(act)
	}
	if len(b.visibleCards()) == 0 {
		return b, nil
	}
	if act.Scope == "pr" {
		return b.handlePRActionKey(act, b.selectedCard())
	}
	return b.handleActionKey(act, b.selectedCard())
}

// dispatchExpandedAction expands act's URL/command template with vars and
// executes it (url -> OpenURL, shell -> runShellCmd). This is the shared leaf
// dispatch shared by every action scope (card, board, pr).
func (b Board) dispatchExpandedAction(act config.Action, vars map[string]string) (tea.Model, tea.Cmd) {
	switch act.Type {
	case "url":
		urlVars := action.BuildURLSafeVars(vars)
		expanded := action.ExpandTemplate(act.URL, urlVars)
		if err := b.executor.OpenURL(expanded); err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
			return b, cmd
		}
		return b, nil
	case "shell":
		shellVars := action.BuildShellSafeVars(vars)
		expanded := action.ExpandTemplate(act.Command, shellVars)
		cmd := b.statusBar.SetTimedMessage("Running...", StatusInfo, longStatusMessageDuration)
		return b, tea.Batch(cmd, runShellCmd(b.executor, expanded))
	}
	return b, nil
}

func (b Board) handleActionKeyWithComment(act config.Action, card Card, comment string) (tea.Model, tea.Cmd) {
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	window := b.resolveWindowName(card.Number, card.Title)
	vars := action.BuildTemplateVars(card.Number, card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, comment, window)
	return b.dispatchExpandedAction(act, vars)
}

func (b Board) handleBoardActionKeyWithComment(act config.Action, comment string) (tea.Model, tea.Cmd) {
	vars := action.BuildBoardTemplateVars(b.repoOwner, b.repoName, b.providerName, comment)
	return b.dispatchExpandedAction(act, vars)
}

// runPRAction is the leaf dispatcher for a scope: pr action against a
// specific card and one of its linked PRs. It layers PR-specific template
// variables on top of the card-scope base vars, then dispatches through
// dispatchExpandedAction like every other scope.
func (b Board) runPRAction(act config.Action, card Card, pr LinkedPR, comment string) (tea.Model, tea.Cmd) {
	labelNames := make([]string, len(card.Labels))
	for i, l := range card.Labels {
		labelNames[i] = l.Name
	}
	window := b.resolveWindowName(card.Number, card.Title)
	baseVars := action.BuildTemplateVars(card.Number, card.Title, labelNames, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, comment, window)
	return b.runPRActionWithVars(act, pr, baseVars)
}

// runPRActionWithVars layers the PR-specific template variables (including
// on-demand {pr_worktree} resolution) on top of baseVars and dispatches act.
// Shared by runPRAction (card-derived base vars) and the PR list modal's
// card-less dispatch path.
func (b Board) runPRActionWithVars(act config.Action, pr LinkedPR, baseVars map[string]string) (tea.Model, tea.Cmd) {
	prWorktree := ""
	if strings.Contains(act.URL+act.Command, "{pr_worktree}") {
		var err error
		prWorktree, err = b.resolvePRWorktree(pr.Branch)
		if err != nil {
			cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), StatusError, statusMessageDuration)
			return b, cmd
		}
	}
	vars := action.BuildPRTemplateVars(baseVars, pr.Number, pr.Title, pr.URL, pr.Branch, prWorktree)
	return b.dispatchExpandedAction(act, vars)
}

// handlePRListActionKey dispatches an uppercase custom-action key pressed
// inside the PR list modal against the selected PR row. Eligibility is
// deliberately narrower than normal mode's registry dispatch (dispatchBinding
// in keymap_dispatch.go): only GLOBAL
// scope: pr actions apply — the modal is a repo-wide view with no active
// column, so per-column overrides and card/board scopes have no sensible
// target here. Rows linked to a board card dispatch with the same full
// card+PR template variables as a normal-mode scope: pr action; unlinked
// rows expand the card-derived variables ({number}, {title}, {tags},
// {session}, {window}) to empty strings since there is no card to derive
// them from. The modal stays open so several PRs can be acted on in a row.
func (b Board) handlePRListActionKey(key string) (tea.Model, tea.Cmd) {
	act, ok := b.actions[key]
	if !ok || config.DefaultScope(act.Scope) != "pr" {
		return b, nil
	}
	if len(b.prList.entries) == 0 || b.prList.cursor >= len(b.prList.entries) {
		return b, nil
	}
	entry := b.prList.entries[b.prList.cursor]

	if entry.cardNumber != 0 {
		if ci, i, ok := b.findCard(entry.cardNumber); ok {
			return b.runPRAction(act, b.Columns[ci].Cards[i], entry.pr, "")
		}
	}

	baseVars := action.BuildTemplateVars(0, "", nil, b.repoOwner, b.repoName, b.providerName, b.sessionMaxLen, "", "")
	for _, cardVar := range []string{"number", "title", "tags", "session", "window"} {
		baseVars[cardVar] = ""
	}
	return b.runPRActionWithVars(act, entry.pr, baseVars)
}

// resolvePRWorktree returns the registered worktree for branch. Git's
// porcelain output is used instead of assuming a project-specific directory
// convention.
func (b Board) resolvePRWorktree(branch string) (string, error) {
	stdout, stderr, err := b.executor.RunShellOutput("git worktree list --porcelain")
	if err != nil {
		detail := strings.TrimSpace(stderr)
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("could not list Git worktrees: %s", detail)
	}
	worktree := gitutil.WorktreeForBranch(stdout, branch)
	if worktree == "" {
		return "", fmt.Errorf("no Git worktree found for branch %q", branch)
	}
	return worktree, nil
}

// handlePRActionKeyWithComment implements the full 0/1/2+ linked-PR
// precedence for a scope: pr action, mirroring handlePROpenKey (the
// built-in "p" key's precedence anchor):
//   - 0 PRs: no-op (defensive; dispatchBinding's pr-scope gate in
//     keymap_dispatch.go should already refuse this before it ever reaches
//     here).
//   - 1 PR: runs the action immediately against that PR's data.
//   - 2+ PRs: stashes the action (and any comment) as pendingPRAction and
//     opens prPickerMode; the picker's Enter key consumes it.
func (b Board) handlePRActionKeyWithComment(act config.Action, card Card, comment string) (tea.Model, tea.Cmd) {
	switch len(card.LinkedPRs) {
	case 0:
		debuglog.Errorf("scope:pr action %q dispatched against a card with 0 linked PRs (dispatchBinding's pr-scope gate bypassed)", act.Name)
		cmd := b.statusBar.SetTimedMessage("No linked PRs", StatusWarning, statusMessageDuration)
		return b, cmd
	case 1:
		return b.runPRAction(act, card, card.LinkedPRs[0], comment)
	default:
		b.pendingPRAction = &pendingPRAction{action: act, comment: comment}
		b.prPickerIndex = 0
		b.mode = prPickerMode
		b.statusBar.SetActionHints(prPickerHints)
		return b, nil
	}
}

func (b Board) handlePRActionKey(act config.Action, card Card) (tea.Model, tea.Cmd) {
	return b.handlePRActionKeyWithComment(act, card, "")
}

func (b Board) handleActionKey(act config.Action, card Card) (tea.Model, tea.Cmd) {
	return b.handleActionKeyWithComment(act, card, "")
}

func (b Board) handleBoardActionKey(act config.Action) (tea.Model, tea.Cmd) {
	return b.handleBoardActionKeyWithComment(act, "")
}
