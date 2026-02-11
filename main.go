package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v68/github"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
	"golang.org/x/oauth2"
)

func main() {
	globalPath, err := config.DefaultGlobalPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving config path: %v\n", err)
		os.Exit(1)
	}

	// Check for "config" subcommand.
	configSubcmd := len(os.Args) > 1 && os.Args[1] == "config"

	cfg, err := config.Load(globalPath, config.DefaultLocalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Run wizard if: "config" subcommand OR no config exists.
	needsWizard := configSubcmd || !config.Exists(globalPath, config.DefaultLocalPath)
	if needsWizard {
		wizard := NewConfigWizard(cfg.Provider, cfg.Repo)
		p := tea.NewProgram(wizard)
		finalModel, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		w, ok := finalModel.(ConfigWizard)
		if !ok {
			fmt.Fprintf(os.Stderr, "Unexpected wizard state\n")
			os.Exit(1)
		}
		if w.Cancelled() {
			os.Exit(0)
		}

		// Save config.
		cfg.Provider = w.Provider()
		cfg.Repo = w.Repo()
		if err := cfg.SaveGlobal(globalPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving global config: %v\n", err)
			os.Exit(1)
		}
		if err := cfg.SaveLocal(config.DefaultLocalPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving local config: %v\n", err)
			os.Exit(1)
		}

		// If explicit "config" subcommand, exit after saving.
		if configSubcmd {
			fmt.Println("Configuration saved.")
			os.Exit(0)
		}
	}

	// Normal board launch flow.
	var bp provider.BoardProvider
	switch cfg.Provider {
	case "":
		fmt.Fprintf(os.Stderr, "No provider configured. Run 'lazyboards config' to set up.\n")
		os.Exit(1)
	case "github":
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			fmt.Fprintf(os.Stderr, "GITHUB_TOKEN environment variable is required.\n")
			fmt.Fprintf(os.Stderr, "Set it in your shell profile:\n\n")
			fmt.Fprintf(os.Stderr, "  export GITHUB_TOKEN=<your-token>\n\n")
			os.Exit(1)
		}
		parts := strings.SplitN(cfg.Repo, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			fmt.Fprintf(os.Stderr, "Invalid repo format %q, expected \"owner/repo\"\n", cfg.Repo)
			os.Exit(1)
		}
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(context.Background(), ts)
		ghClient := github.NewClient(tc)
		bp = provider.NewGitHubProvider(ghClient.Issues, parts[0], parts[1], config.DefaultColumns)
	default:
		fmt.Fprintf(os.Stderr, "Unknown provider: %q\n", cfg.Provider)
		os.Exit(1)
	}

	board := NewBoard(bp)
	p := tea.NewProgram(board, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
