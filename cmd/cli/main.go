package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"powermeter/internal/powermetrics"
	"powermeter/internal/storage"
	"powermeter/internal/tui"
)

func main() {
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
