package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/matteobortolazzo/lazyboards/internal/config"
	"github.com/matteobortolazzo/lazyboards/internal/provider"
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
	case "fake":
		bp = provider.NewFakeProvider()
	default:
		fmt.Fprintf(os.Stderr, "Unknown provider: %q\n", cfg.Provider)
		os.Exit(1)
	}

	board, err := NewBoardFromProvider(bp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading board: %v\n", err)
		os.Exit(1)
	}

	p := tea.NewProgram(board, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
