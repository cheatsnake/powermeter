package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type menuModel struct {
	cursor int
}

var menuItems = []string{
	"Start measure",
	"Change measure setup",
	"History of measures",
	"About",
	"Exit",
}

func newMenuModel() menuModel {
	return menuModel{}
}

func (m Model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.menu.cursor = (m.menu.cursor + len(menuItems) - 1) % len(menuItems)
	case "down", "j":
		m.menu.cursor = (m.menu.cursor + 1) % len(menuItems)
	case "enter":
		return m.selectMenuItem()
	}
	return m, nil
}

func (m Model) selectMenuItem() (tea.Model, tea.Cmd) {
	m.status = ""
	switch m.menu.cursor {
	case 0:
		return m.startMeasurement()
	case 1:
		m.setup = newSetupModel(m.cfg)
		m.view = viewSetup
		return m, textinput.Blink
	case 2:
		m.history = historyModel{loading: true}
		m.view = viewHistory
		return m, m.loadSessionsCmd()
	case 3:
		m.view = viewAbout
		return m, nil
	case 4:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) viewMenu() string {
	var b strings.Builder
	b.WriteString(m.header("Powermeter", "Measure per-process power consumption on macOS"))
	for i, item := range menuItems {
		if i == m.menu.cursor {
			b.WriteString(selectedItemStyle.Render("▸ " + item))
		} else {
			b.WriteString(normalItemStyle.Render(item))
		}
		b.WriteString("\n")
	}
	b.WriteString("\n")
	b.WriteString(subtitleStyle.Render(fmt.Sprintf(
		"Total time: %s min   Sample interval: %s s   Samples: %d",
		formatMinutes(m.cfg.TotalSeconds), formatSeconds(m.cfg.IntervalMs), m.cfg.SampleCount(),
	)))
	b.WriteString(m.footer("↑/↓ move · enter select · q quit"))
	return b.String()
}
