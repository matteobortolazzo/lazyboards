package main

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/action"
)

func (b Board) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case clearStatusMsg:
		b.statusBar.ClearMessage()
		return b, nil

	case boardFetchedMsg:
		return b.handleBoardFetched(msg)

	case boardFetchErrorMsg:
		b.mode = errorMode
		b.loadErr = msg.err.Error()
		b.statusBar.SetActionHints([]Hint{
			{Key: "r", Desc: "Retry"},
			{Key: "q", Desc: "Quit"},
		})
		return b, nil

	case cardCreatedMsg:
		return b.handleCardCreated(msg)

	case cardCreateErrorMsg:
		b.validationErr = msg.err.Error()
		b.mode = createMode
		cmd := b.titleInput.Focus()
		b.labelInput.Blur()
		return b, cmd

	case configSavedMsg:
		if b.firstLaunch {
			b.ConfigSaved = true
			return b, tea.Quit
		}
		b.mode = loadingMode
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))

	case configSaveErrorMsg:
		b.validationErr = msg.err.Error()
		b.mode = configMode
		return b, nil

	case actionResultMsg:
		cmd := b.statusBar.SetTimedMessage(msg.message, 3*time.Second)
		return b, cmd

	case spinner.TickMsg:
		if b.mode == loadingMode || b.mode == creatingMode {
			var cmd tea.Cmd
			b.spinner, cmd = b.spinner.Update(msg)
			return b, cmd
		}
		return b, nil

	case tea.KeyMsg:
		// ctrl+c always quits regardless of mode.
		if msg.String() == "ctrl+c" {
			return b, tea.Quit
		}

		switch b.mode {
		case loadingMode, creatingMode:
			return b, nil
		case errorMode:
			switch msg.String() {
			case "q":
				return b, tea.Quit
			case "r":
				b.mode = loadingMode
				b.loadErr = ""
				return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
			}
			return b, nil
		case createMode:
			return b.handleCreateModeKey(msg)
		case configMode:
			return b.handleConfigModeKey(msg)
		default:
			return b.handleNormalModeKey(msg)
		}

	case tea.WindowSizeMsg:
		b.Width = msg.Width
		b.Height = msg.Height
		if len(b.Columns) > 0 {
			b.clampScrollOffset()
		}
	}
	return b, nil
}

func (b Board) handleBoardFetched(msg boardFetchedMsg) (tea.Model, tea.Cmd) {
	cols := make([]Column, len(msg.board.Columns))
	for i, pc := range msg.board.Columns {
		cards := make([]Card, len(pc.Cards))
		for j, c := range pc.Cards {
			cards[j] = Card{Number: c.Number, Title: c.Title, Labels: c.Labels, Body: c.Body}
		}
		cols[i] = Column{Title: pc.Title, Cards: cards}
	}
	b.Columns = cols
	b.mode = normalMode
	b.detailScrollOffset = 0
	b.detailFocused = false
	var cmd tea.Cmd
	if b.loaded {
		b.statusBar.SetActionHints(b.normalHints)
		cmd = b.statusBar.SetTimedMessage("Board refreshed", 3*time.Second)
	}
	b.loaded = true
	return b, cmd
}

func (b Board) handleCardCreated(msg cardCreatedMsg) (tea.Model, tea.Cmd) {
	newCard := Card{
		Number: msg.card.Number,
		Title:  msg.card.Title,
		Labels: msg.card.Labels,
		Body:   msg.card.Body,
	}
	b.Columns[0].Cards = append(b.Columns[0].Cards, newCard)
	b.titleInput.SetValue("")
	b.labelInput.SetValue("")
	b.validationErr = ""
	b.mode = normalMode
	return b, nil
}

func (b Board) handleCreateModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		b.mode = normalMode
		return b, nil
	case tea.KeyEnter:
		title := strings.TrimSpace(b.titleInput.Value())
		if title == "" {
			b.validationErr = "Title is required"
			return b, nil
		}
		label := strings.TrimSpace(b.labelInput.Value())
		for _, col := range b.Columns {
			if strings.EqualFold(col.Title, label) {
				b.validationErr = "Cannot use reserved column label"
				return b, nil
			}
		}
		b.mode = creatingMode
		b.titleInput.Blur()
		b.labelInput.Blur()
		return b, tea.Batch(b.spinner.Tick, createCardCmd(b.provider, title, label))
	case tea.KeyTab:
		var cmd tea.Cmd
		if b.titleInput.Focused() {
			b.titleInput.Blur()
			cmd = b.labelInput.Focus()
		} else {
			b.labelInput.Blur()
			cmd = b.titleInput.Focus()
		}
		return b, cmd
	default:
		b.validationErr = ""
		var cmd tea.Cmd
		if b.titleInput.Focused() {
			b.titleInput, cmd = b.titleInput.Update(msg)
		} else if b.labelInput.Focused() {
			b.labelInput, cmd = b.labelInput.Update(msg)
		}
		return b, cmd
	}
}

func (b Board) handleConfigModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		if b.firstLaunch {
			return b, tea.Quit
		}
		b.mode = normalMode
		return b, nil
	case tea.KeyEnter:
		provider := b.providerOptions[b.providerIndex]
		repo := strings.TrimSpace(b.repoInput.Value())
		if repo == "" {
			b.validationErr = "Repository is required"
			return b, nil
		}
		b.validationErr = ""
		return b, saveConfigCmd(b.configLocalPath, provider, repo)
	case tea.KeyTab:
		if b.configFocus == 0 {
			b.configFocus = 1
			cmd := b.repoInput.Focus()
			return b, cmd
		}
		b.configFocus = 0
		b.repoInput.Blur()
		return b, nil
	case tea.KeyRight:
		if b.configFocus == 0 {
			b.providerIndex = (b.providerIndex + 1) % len(b.providerOptions)
		}
		return b, nil
	case tea.KeyLeft:
		if b.configFocus == 0 {
			b.providerIndex = (b.providerIndex - 1 + len(b.providerOptions)) % len(b.providerOptions)
		}
		return b, nil
	default:
		if b.configFocus == 1 {
			var cmd tea.Cmd
			b.repoInput, cmd = b.repoInput.Update(msg)
			return b, cmd
		}
		return b, nil
	}
}

func (b Board) handleNormalModeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if b.detailFocused {
		return b.handleDetailFocusedKey(msg)
	}

	switch msg.String() {
	case "q":
		return b, tea.Quit
	case "n":
		b.mode = createMode
		b.titleInput.SetValue("")
		b.labelInput.SetValue("")
		b.titleInput.Focus()
		b.labelInput.Blur()
	case "c":
		b.enterConfigMode()
	case "r":
		b.mode = loadingMode
		b.statusBar.ClearMessage()
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
	case "l":
		b.detailFocused = true
		b.statusBar.SetActionHints(detailFocusHints)
	case "shift+tab", "left":
		if b.ActiveTab > 0 {
			b.ActiveTab--
			b.Columns[b.ActiveTab].ScrollOffset = 0
			b.detailScrollOffset = 0
			b.clampScrollOffset()
		}
	case "tab", "right":
		if b.ActiveTab < len(b.Columns)-1 {
			b.ActiveTab++
			b.Columns[b.ActiveTab].ScrollOffset = 0
			b.detailScrollOffset = 0
			b.clampScrollOffset()
		}
	case "j", "down":
		col := &b.Columns[b.ActiveTab]
		if col.Cursor < len(col.Cards)-1 {
			col.Cursor++
		}
		b.detailScrollOffset = 0
		b.clampScrollOffset()
	case "k", "up":
		col := &b.Columns[b.ActiveTab]
		if col.Cursor > 0 {
			col.Cursor--
		}
		b.detailScrollOffset = 0
		b.clampScrollOffset()
	default:
		// Check if it's a custom action key.
		if act, ok := b.actions[msg.String()]; ok {
			col := b.Columns[b.ActiveTab]
			if len(col.Cards) == 0 {
				return b, nil
			}
			card := col.Cards[col.Cursor]
			vars := action.BuildTemplateVars(card.Number, card.Title, card.Labels, b.repoOwner, b.repoName, b.providerName)

			switch act.Type {
			case "url":
				expanded := action.ExpandTemplate(act.URL, vars)
				if err := b.executor.OpenURL(expanded); err != nil {
					cmd := b.statusBar.SetTimedMessage("Error: "+err.Error(), 3*time.Second)
					return b, cmd
				}
				return b, nil
			case "shell":
				expanded := action.ExpandTemplate(act.Command, action.BuildShellSafeVars(vars))
				cmd := b.statusBar.SetTimedMessage("Running...", 30*time.Second)
				return b, tea.Batch(cmd, runShellCmd(b.executor, expanded))
			}
		}
	}
	return b, nil
}

func (b Board) handleDetailFocusedKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Handle Escape via msg.Type first.
	if msg.Type == tea.KeyEsc {
		b.detailFocused = false
		b.statusBar.SetActionHints(b.normalHints)
		return b, nil
	}

	switch msg.String() {
	case "q":
		return b, tea.Quit
	case "r":
		b.mode = loadingMode
		b.statusBar.ClearMessage()
		return b, tea.Batch(b.spinner.Tick, fetchBoardCmd(b.provider))
	case "h":
		b.detailFocused = false
		b.statusBar.SetActionHints(b.normalHints)
	case "j", "down":
		col := b.Columns[b.ActiveTab]
		if len(col.Cards) > 0 {
			card := col.Cards[col.Cursor]
			if card.Body != "" {
				maxLines := strings.Count(card.Body, "\n") + 1
				panelHeight := b.Height - 6
				if panelHeight < 1 {
					panelHeight = 1
				}
				headerLines := 3
				availableBodyLines := panelHeight - headerLines
				if availableBodyLines < 1 {
					availableBodyLines = 1
				}
				maxOffset := maxLines - availableBodyLines
				if maxOffset < 0 {
					maxOffset = 0
				}
				if b.detailScrollOffset < maxOffset {
					b.detailScrollOffset++
				}
			}
		}
	case "k", "up":
		if b.detailScrollOffset > 0 {
			b.detailScrollOffset--
		}
	case "tab", "right":
		if b.ActiveTab < len(b.Columns)-1 {
			b.detailFocused = false
			b.detailScrollOffset = 0
			b.statusBar.SetActionHints(b.normalHints)
			b.ActiveTab++
			b.Columns[b.ActiveTab].ScrollOffset = 0
			b.clampScrollOffset()
		}
	case "shift+tab", "left":
		if b.ActiveTab > 0 {
			b.detailFocused = false
			b.detailScrollOffset = 0
			b.statusBar.SetActionHints(b.normalHints)
			b.ActiveTab--
			b.Columns[b.ActiveTab].ScrollOffset = 0
			b.clampScrollOffset()
		}
	}
	return b, nil
}
