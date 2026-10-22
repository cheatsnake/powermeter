package tui

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"powermeter/internal/storage"
)

type setupModel struct {
	inputs [2]textinput.Model
	focus  int
	err    string
}

func newSetupModel(cfg storage.Config) setupModel {
	total := textinput.New()
	total.Prompt = "Total time (min): "
	total.SetValue(formatMinutes(cfg.TotalSeconds))
	total.CharLimit = 8
	total.Width = 8
	total.Focus()

	interval := textinput.New()
	interval.Prompt = "Sample interval (s): "
	interval.SetValue(formatSeconds(cfg.IntervalMs))
	interval.CharLimit = 6
	interval.Width = 6

	return setupModel{inputs: [2]textinput.Model{total, interval}}
}

func (m *setupModel) next() {
	m.inputs[m.focus].Blur()
	m.focus = (m.focus + 1) % len(m.inputs)
	m.inputs[m.focus].Focus()
}

func (m setupModel) config() (storage.Config, error) {
	minutes, err := strconv.ParseFloat(strings.TrimSpace(m.inputs[0].Value()), 64)
	if err != nil {
		return storage.Config{}, errors.New("total time must be a number")
	}
	if minutes < storage.MinTotalMinutes || minutes > storage.MaxTotalMinutes {
		return storage.Config{}, fmt.Errorf(
			"total time must be between %g and %g minutes",
			storage.MinTotalMinutes, storage.MaxTotalMinutes,
		)
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(m.inputs[1].Value()), 64)
	if err != nil {
		return storage.Config{}, errors.New("sample interval must be a number")
	}
	if seconds < storage.MinIntervalSeconds || seconds > storage.MaxIntervalSeconds {
		return storage.Config{}, fmt.Errorf(
			"sample interval must be between %g and %g seconds",
			storage.MinIntervalSeconds, storage.MaxIntervalSeconds,
		)
	}

	cfg := storage.Config{
		TotalSeconds: int(math.Round(minutes * 60)),
		IntervalMs:   int(math.Round(seconds * 1000)),
	}
	if err := cfg.Validate(); err != nil {
		return storage.Config{}, err
	}
	return cfg, nil
}

func (m Model) updateSetup(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.setup = newSetupModel(m.cfg)
		m.view = viewMenu
		return m, nil
	case "tab", "shift+tab", "up", "down":
		m.setup.next()
		return m, textinput.Blink
	case "enter":
		cfg, err := m.setup.config()
		if err != nil {
			m.setup.err = err.Error()
			return m, nil
		}
		m.setup.err = ""
		m.cfg = cfg
		return m, m.saveConfigCmd(cfg)
	}

	var cmd tea.Cmd
	m.setup.inputs[m.setup.focus], cmd = m.setup.inputs[m.setup.focus].Update(msg)
	return m, cmd
}

func (m Model) viewSetup() string {
	var b strings.Builder
	b.WriteString(m.header("Measure setup", "Settings are saved to "+m.store.Dir()))
	for i := range m.setup.inputs {
		b.WriteString(m.setup.inputs[i].View())
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf(
		"Total time (%g–%g min) · sample interval (%g–%g s)",
		storage.MinTotalMinutes, storage.MaxTotalMinutes,
		storage.MinIntervalSeconds, storage.MaxIntervalSeconds,
	)))
	if m.setup.err != "" {
		b.WriteString("\n\n")
		b.WriteString(m.centeredError(m.setup.err))
	}
	b.WriteString(m.footer("tab/↑/↓ switch · enter save · esc cancel"))
	return b.String()
}
