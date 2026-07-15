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

// handleCustomActionKey resolves msg against the user's custom action system:
// Alt+letter enters comment mode (if the action's template uses {comment}) or
// dispatches immediately, and a plain uppercase letter dispatches directly.
// Shared by normal mode and detail-focused mode so custom actions behave
// identically in both -- b.detailFocused (already accurate at call time,
// since detail-focused mode is a sub-state routed to before this is ever
// reached) is threaded onto the pending comment so returning from comment
// mode restores the focus it was triggered from, mirroring the
// helpFromDetailFocused pattern. Scope routing (board/card/pr) is delegated
// to dispatchResolvedAction so every dispatch path shares one gating rule.
// Returns b unchanged if msg is not a recognized custom action key.
func (b Board) handleCustomActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Alt+Shift+key: check for comment mode trigger (uppercase A-Z only).
	if msg.Alt && len(msg.Runes) == 1 && msg.Runes[0] >= 'A' && msg.Runes[0] <= 'Z' {
		baseKey := string(msg.Runes)
		if act, ok := b.resolveAction(baseKey); ok {
			template := act.URL + act.Command
			if strings.Contains(template, "{comment}") {
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
			// Alt on action without {comment} -- execute normally.
			return b.dispatchResolvedAction(act)
		}
		return b, nil
	}
	// Check if it's a custom action key (uppercase A-Z only).
	if len(msg.Runes) == 1 && msg.Runes[0] >= 'A' && msg.Runes[0] <= 'Z' {
		if act, ok := b.resolveAction(msg.String()); ok {
			return b.dispatchResolvedAction(act)
		}
	}
	return b, nil
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
//   - 0 PRs: no-op (defensive; resolveAction should already gate this out).
//   - 1 PR: runs the action immediately against that PR's data.
//   - 2+ PRs: stashes the action (and any comment) as pendingPRAction and
//     opens prPickerMode; the picker's Enter key consumes it.
func (b Board) handlePRActionKeyWithComment(act config.Action, card Card, comment string) (tea.Model, tea.Cmd) {
	switch len(card.LinkedPRs) {
	case 0:
		debuglog.Errorf("scope:pr action %q dispatched against a card with 0 linked PRs (resolveAction gate bypassed)", act.Name)
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
