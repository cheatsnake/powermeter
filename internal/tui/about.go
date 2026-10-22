package tui

import "strings"

const (
	projectURL = "https://github.com/cheatsnake/powermeter"
	appVersion = "2026-10-22-1"
)

func (m Model) viewAbout() string {
	var b strings.Builder
	b.WriteString(m.header("About Powermeter", "Version "+appVersion))

	b.WriteString("Powermeter shows how much power each process consumes on macOS.\n")
	b.WriteString("It reads data from the built-in powermetrics utility, so it needs\n")
	b.WriteString("administrator privileges for every measurement.\n\n")
	b.WriteString("Results are saved to ")
	b.WriteString(subtitleStyle.Render(m.store.Dir()))
	b.WriteString(" and stay available in History.\n\n")
	b.WriteString("In the results table, power is an estimate in milliwatts for each process.\n")
	b.WriteString("Press enter on a process to search for ways to reduce its power use.\n\n")
	b.WriteString(subtitleStyle.Render(projectURL))

	b.WriteString(m.footer("esc back · q quit"))
	return b.String()
}
