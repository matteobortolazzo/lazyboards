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

	cfg, err := config.Load(globalPath, config.DefaultLocalPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	var bp provider.BoardProvider
	switch cfg.Provider {
	case "":
		fmt.Fprintf(os.Stderr, "No provider configured.\n\n")
		fmt.Fprintf(os.Stderr, "Create a .lazyboards.yml file with:\n\n")
		fmt.Fprintf(os.Stderr, "  provider: github\n")
		fmt.Fprintf(os.Stderr, "  repo: owner/repo\n\n")
		fmt.Fprintf(os.Stderr, "Then set GITHUB_TOKEN in your environment.\n")
		os.Exit(1)
	case "github":
		token := os.Getenv("GITHUB_TOKEN")
		if token == "" {
			fmt.Fprintf(os.Stderr, "GITHUB_TOKEN environment variable is required\n")
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
