package main

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type wizardStep int

const (
	providerStep wizardStep = iota
	repoStep
	doneStep
)

// ConfigWizard is a BubbleTea model that guides the user through initial configuration.
type ConfigWizard struct {
	step        wizardStep
	providers   []string
	providerIdx int
	repoInput   textinput.Model
	repoErr     string
	width       int
	height      int
	cancelled   bool
}

// Cancelled returns true if the user pressed Esc or Ctrl+C.
func (w ConfigWizard) Cancelled() bool {
	return w.cancelled
}

// Provider returns the selected provider name.
func (w ConfigWizard) Provider() string {
	return w.providers[w.providerIdx]
}

// Repo returns the repo value from the text input.
func (w ConfigWizard) Repo() string {
	return w.repoInput.Value()
}

// NewConfigWizard creates a ConfigWizard with optional pre-filled values.
func NewConfigWizard(provider, repo string) ConfigWizard {
	providers := []string{"github"}

	providerIdx := 0
	for i, p := range providers {
		if p == provider {
			providerIdx = i
			break
		}
	}

	ri := textinput.New()
	ri.Placeholder = "owner/repo"
	ri.Width = 40
	ri.CharLimit = 100
	if repo != "" {
		ri.SetValue(repo)
	}

	return ConfigWizard{
		step:        providerStep,
		providers:   providers,
		providerIdx: providerIdx,
		repoInput:   ri,
	}
}

func (w ConfigWizard) Init() tea.Cmd {
	return textinput.Blink
}

func (w ConfigWizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEscape:
			w.cancelled = true
			return w, tea.Quit
		}

		switch w.step {
		case providerStep:
			switch msg.String() {
			case "j", "down":
				if w.providerIdx < len(w.providers)-1 {
					w.providerIdx++
				}
			case "k", "up":
				if w.providerIdx > 0 {
					w.providerIdx--
				}
			case "enter":
				w.step = repoStep
				cmd := w.repoInput.Focus()
				return w, cmd
			}

		case repoStep:
			switch msg.Type {
			case tea.KeyEnter:
				value := w.repoInput.Value()
				parts := strings.SplitN(value, "/", 2)
				if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
					w.repoErr = "Must be in owner/repo format"
					return w, nil
				}
				w.step = doneStep
				return w, tea.Quit
			default:
				w.repoErr = ""
				var cmd tea.Cmd
				w.repoInput, cmd = w.repoInput.Update(msg)
				return w, cmd
			}
		}

	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
	}

	return w, nil
}

func (w ConfigWizard) View() string {
	if w.step == doneStep {
		return ""
	}

	var b strings.Builder
	b.WriteString("Lazyboards Configuration\n\n")

	switch w.step {
	case providerStep:
		b.WriteString("Provider:\n")
		for i, p := range w.providers {
			if i == w.providerIdx {
				b.WriteString("> " + p + "\n")
			} else {
				b.WriteString("  " + p + "\n")
			}
		}

	case repoStep:
		b.WriteString("Provider: " + w.providers[w.providerIdx] + "\n\n")
		b.WriteString("Repository:\n")
		b.WriteString(w.repoInput.View() + "\n")
		if w.repoErr != "" {
			b.WriteString(w.repoErr + "\n")
		}
	}

	b.WriteString("\nenter: next  esc: quit")

	return b.String()
}
