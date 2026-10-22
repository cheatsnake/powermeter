package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"powermeter/internal/storage"
)

type historyModel struct {
	sessions []storage.Session
	cursor   int
	loading  bool
	err      string
}

func (m Model) updateHistory(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.history.loading {
		return m, nil
	}
	onKey := msg.String()
	switch onKey {
	case "esc":
		m.view = viewMenu
		return m, nil
	case "up", "k":
		if len(m.history.sessions) > 0 {
			m.history.cursor = (m.history.cursor + len(m.history.sessions) - 1) % len(m.history.sessions)
		}
		return m, nil
	case "down", "j":
		if len(m.history.sessions) > 0 {
			m.history.cursor = (m.history.cursor + 1) % len(m.history.sessions)
		}
		return m, nil
	case "enter":
		if len(m.history.sessions) > 0 {
			m.history.err = ""
			return m, m.openSessionCmd(m.history.sessions[m.history.cursor].Path)
		}
		return m, nil
	}
	return m, nil
}

func (m Model) viewHistory() string {
	var b strings.Builder
	b.WriteString(m.header("History of measures", "Files in "+m.store.Dir()))
	b.WriteString("\n")

	switch {
	case m.history.loading:
		b.WriteString("Loading...")
	case m.history.err != "":
		b.WriteString(m.centeredError(m.history.err))
	case len(m.history.sessions) == 0:
		b.WriteString("No measurements yet.\n")
		b.WriteString(subtitleStyle.Render("Run a measure to see its results here."))
	default:
		for i, session := range m.history.sessions {
			line := fmt.Sprintf("%s   %8s   %s",
				session.StartedAt.Format("2006-01-02 15:04:05"),
				humanBytes(session.Size),
				session.Name,
			)
			if i == m.history.cursor {
				b.WriteString(selectedItemStyle.Render("▸ " + line))
			} else {
				b.WriteString(normalItemStyle.Render(line))
			}
			b.WriteString("\n")
		}
	}

	b.WriteString(m.footer("↑/↓ move · enter open · esc back · q quit"))
	return b.String()
}
