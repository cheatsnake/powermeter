package tui

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"powermeter/internal/analysis"
	"powermeter/internal/powermetrics"
)

type resultsModel struct {
	table      table.Model
	stats      []analysis.ProcessStat
	summary    analysis.Summary
	path       string
	returnView view
	err        string
}

func newResults(samples []powermetrics.Sample, path string, returnView view, width, height int) resultsModel {
	model := resultsModel{
		stats:      analysis.TopN(analysis.Aggregate(samples), 20),
		summary:    analysis.Summarize(samples),
		path:       path,
		returnView: returnView,
	}
	model.resize(width, height)
	return model
}

func (m *resultsModel) resize(width, height int) {
	if len(m.stats) == 0 {
		return
	}
	cursor := 0
	if m.table.Cursor() > 0 {
		cursor = m.table.Cursor()
	}

	nameWidth := clamp(width-48, 16, 44)
	columns := []table.Column{
		{Title: "#", Width: 3},
		{Title: "Process", Width: nameWidth},
		{Title: "Power mW", Width: 10},
		{Title: "CPU", Width: 10},
		{Title: "GPU", Width: 10},
		{Title: "Wakeups/s", Width: 10},
	}

	rows := make([]table.Row, 0, len(m.stats))
	for i, stat := range m.stats {
		power := "—"
		if stat.HasPower {
			power = formatPowerMW(stat.PowerAvgMW)
		}
		rows = append(rows, table.Row{
			fmt.Sprintf("%d", i+1),
			stat.Name,
			power,
			formatMillis(stat.CPUTotalMs),
			formatMillis(stat.GPUTotalMs),
			fmt.Sprintf("%.0f", stat.WakeupsAvg),
		})
	}

	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		Bold(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true)
	styles.Selected = styles.Selected.
		Bold(false).
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57"))

	m.table = table.New(
		table.WithColumns(columns),
		table.WithRows(rows),
		table.WithFocused(true),
		table.WithHeight(clamp(len(rows)+1, 3, height-12)),
		table.WithStyles(styles),
	)
	m.table.SetCursor(cursor)
}

func (m Model) updateResults(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		if m.results.returnView == viewHistory {
			m.view = viewHistory
		} else {
			m.view = viewMenu
		}
		return m, nil
	case "enter":
		row := m.results.table.SelectedRow()
		if len(row) > 1 {
			if err := openProcessQuery(row[1]); err != nil {
				m.results.err = err.Error()
			}
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.results.table, cmd = m.results.table.Update(msg)
	return m, cmd
}

func (m Model) viewResults() string {
	var b strings.Builder

	title := "Results"
	if m.results.path != "" {
		title = "Results · " + filepath.Base(m.results.path)
	}
	b.WriteString(m.header(title, ""))

	summary := m.results.summary
	parts := []string{fmt.Sprintf("%d samples", summary.Samples)}
	if summary.Duration > 0 {
		parts = append(parts, shortDuration(summary.Duration))
	}
	if summary.AvgPowerMW > 0 {
		parts = append(parts, fmt.Sprintf("avg system power %.2f W", summary.AvgPowerMW/1000))
	}
	if summary.PeakPowerMW > 0 {
		parts = append(parts, fmt.Sprintf("peak %.2f W", summary.PeakPowerMW/1000))
	}
	b.WriteString(subtitleStyle.Render(strings.Join(parts, "  ·  ")))
	b.WriteString("\n\n")

	if len(m.results.stats) == 0 {
		b.WriteString("No process data was recorded.")
	} else {
		b.WriteString(m.results.table.View())
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Power = estimated per-process draw in milliwatts, allocated from system power by energy impact share."))
	}

	if m.results.err != "" {
		b.WriteString("\n")
		b.WriteString(m.centeredError(m.results.err))
	}
	if m.results.path != "" {
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Saved: " + prettyPath(m.results.path)))
	}
	b.WriteString(m.footer("↑/↓ move · enter explain in browser · esc back · q quit"))
	return b.String()
}

func openProcessQuery(process string) error {
	query := fmt.Sprintf("Why %s consumes too much power on my mac?", process)
	link := "https://www.google.com/search?q=" + url.QueryEscape(query)
	if err := exec.Command("open", link).Start(); err != nil {
		return fmt.Errorf("could not open browser: %w", err)
	}
	return nil
}

func prettyPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return strings.Replace(path, home, "~", 1)
}
