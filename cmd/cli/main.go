package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"powermeter/internal/powermetrics"
	"powermeter/internal/storage"
	"powermeter/internal/tui"
	"powermeter/internal/version"
)

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("powermeter %s\n", version.Version)
		return
	}

	store, err := storage.DefaultStore()
	if err != nil {
		fmt.Fprintln(os.Stderr, "powermeter:", err)
		os.Exit(1)
	}

	cfg, err := store.LoadConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "powermeter: using default settings:", err)
		cfg = storage.DefaultConfig()
	}

	model := tui.New(store, cfg, func(interval time.Duration, count int) tui.Sampler {
		return powermetrics.NewRunner(interval, count)
	})

	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "powermeter:", err)
		os.Exit(1)
	}
}
