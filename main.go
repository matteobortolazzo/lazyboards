package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/google/go-github/v68/github"
	"github.com/matteobortolazzo/lazyboards/internal/action"
	"github.com/matteobortolazzo/lazyboards/internal/auth"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	gitdetect "github.com/matteobortolazzo/lazyboards/internal/git"
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

	// Auto-detect provider and repo from git remote
	gitInfo := gitdetect.DetectRemote(".git/config")

	// Config overrides git-detected values
	prov := cfg.Provider
	if prov == "" {
		prov = gitInfo.Provider
	}
	repo := cfg.Repo
	if repo == "" {
		repo = gitInfo.Repo
	}

	// Split repo early for reuse
	repoOwner, repoNameOnly := "", ""
	if parts := strings.SplitN(repo, "/", 2); len(parts) == 2 {
		repoOwner = parts[0]
		repoNameOnly = parts[1]
	}

	// First-launch flow: show config popup before creating provider
	if !config.LocalExists(config.DefaultLocalPath) {
		board := NewBoard(nil, nil, nil, nil, repoOwner, repoNameOnly, prov, 0, 0, true)
		p := tea.NewProgram(board, tea.WithAltScreen())
		m, err := p.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		b := m.(Board)
		if !b.config.configSaved {
			fmt.Fprintf(os.Stderr, "Configuration required. Exiting.\n")
			os.Exit(1)
		}
		// Reload config with saved values
		cfg, err = config.Load(globalPath, config.DefaultLocalPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
			os.Exit(1)
		}
		prov = cfg.Provider
		if prov == "" {
			prov = gitInfo.Provider
		}
		repo = cfg.Repo
		if repo == "" {
			repo = gitInfo.Repo
		}
		repoOwner, repoNameOnly = "", ""
		if parts := strings.SplitN(repo, "/", 2); len(parts) == 2 {
			repoOwner = parts[0]
			repoNameOnly = parts[1]
		}
	}

	var bp provider.BoardProvider
	switch prov {
	case "":
		fmt.Fprintf(os.Stderr, "No provider detected.\n\n")
		fmt.Fprintf(os.Stderr, "Ensure you are in a git repository with a GitHub or Azure DevOps remote,\n")
		fmt.Fprintf(os.Stderr, "or create a .lazyboards.yml with:\n\n")
		fmt.Fprintf(os.Stderr, "  provider: github\n")
		fmt.Fprintf(os.Stderr, "  repo: owner/repo\n\n")
		os.Exit(1)
	case "github":
		token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
		if token == "" {
			out, err := exec.Command("gh", "auth", "token").Output()
			if err == nil {
				token = strings.TrimSpace(string(out))
			}
		}
		if token == "" {
			fmt.Fprintf(os.Stderr, "GitHub token not found.\n\n")
			fmt.Fprintf(os.Stderr, "Either set GITHUB_TOKEN or authenticate with: gh auth login\n")
			os.Exit(1)
		}
		if err := auth.ValidateGitHubToken(token); err != nil {
			fmt.Fprintf(os.Stderr, "Invalid GitHub token format.\n\n")
			fmt.Fprintf(os.Stderr, "Ensure GITHUB_TOKEN or `gh auth token` provides a valid token.\n")
			os.Exit(1)
		}
		parts := strings.SplitN(repo, "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			fmt.Fprintf(os.Stderr, "Invalid repo format %q, expected \"owner/repo\"\n", repo)
			os.Exit(1)
		}
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		tc := oauth2.NewClient(context.Background(), ts)
		ghClient := github.NewClient(tc)
		bp = provider.NewGitHubProvider(ghClient.Issues, parts[0], parts[1], cfg.ColumnNames())
	default:
		fmt.Fprintf(os.Stderr, "Unknown provider: %q\n", prov)
		os.Exit(1)
	}

	board := NewBoard(bp, cfg.Actions, cfg.Columns, action.DefaultExecutor{}, repoOwner, repoNameOnly, prov, cfg.SessionMaxLength, time.Duration(cfg.RefreshInterval)*time.Minute, false)

	p := tea.NewProgram(board, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
