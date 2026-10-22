package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"powermeter/internal/powermetrics"
	"powermeter/internal/storage"
)

// Sampler runs a measurement and calls onSample for every sample.
type Sampler interface {
	Run(ctx context.Context, onSample func(powermetrics.Sample)) error
}

// SamplerFactory creates a sampler for an interval and a sample count.
type SamplerFactory func(interval time.Duration, count int) Sampler

type view int

const (
	viewMenu view = iota
	viewSetup
	viewMeasure
	viewResults
	viewHistory
	viewAbout
)

type configSavedMsg struct {
	err error
}

type sessionsLoadedMsg struct {
	sessions []storage.Session
	err      error
}

type sessionLoadedMsg struct {
	path    string
	samples []powermetrics.Sample
	err     error
}

type measureSavedMsg struct {
	gen     int
	path    string
	samples []powermetrics.Sample
	err     error
}

// Model is the root bubbletea model.
type Model struct {
	view   view
	width  int
	height int

	store      *storage.Store
	newSampler SamplerFactory
	cfg        storage.Config

	menu    menuModel
	setup   setupModel
	measure measureModel
	results resultsModel
	history historyModel

	status string
}

// New creates the root model.
func New(store *storage.Store, cfg storage.Config, newSampler SamplerFactory) Model {
	return Model{
		view:       viewMenu,
		width:      80,
		height:     24,
		store:      store,
		newSampler: newSampler,
		cfg:        cfg,
		menu:       newMenuModel(),
		setup:      newSetupModel(cfg),
		measure:    newMeasureModel(),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.measure.progress.Width = clamp(m.width-26, 20, 60)
		m.results.resize(m.width, m.height)
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.cancelMeasurement()
			return m, tea.Quit
		}
		return m.handleKey(msg)

	case streamEventMsg:
		return m.handleStreamEvent(msg)

	case tickMsg:
		if m.view != viewMeasure || msg.gen != m.measure.gen {
			return m, nil
		}
		m.measure.elapsed = time.Since(m.measure.startedAt)
		if m.measure.running {
			return m, tickCmd(m.measure.gen)
		}
		return m, nil

	case configSavedMsg:
		if msg.err != nil {
			m.setup.err = "Could not save settings: " + msg.err.Error()
			return m, nil
		}
		m.view = viewMenu
		m.status = "Settings saved"
		return m, nil

	case measureSavedMsg:
		if msg.gen != m.measure.gen || m.view != viewMeasure {
			return m, nil
		}
		m.measure.saving = false
		if msg.err != nil {
			m.measure.err = "Could not save session: " + msg.err.Error()
			return m, nil
		}
		m.results = newResults(msg.samples, msg.path, viewMenu, m.width, m.height)
		m.view = viewResults
		return m, nil

	case sessionsLoadedMsg:
		m.history.loading = false
		if msg.err != nil {
			m.history.err = msg.err.Error()
			return m, nil
		}
		m.history.sessions = msg.sessions
		m.history.cursor = 0
		return m, nil

	case sessionLoadedMsg:
		if m.view != viewHistory {
			return m, nil
		}
		if msg.err != nil {
			m.history.err = "Could not read session: " + msg.err.Error()
			return m, nil
		}
		m.results = newResults(msg.samples, msg.path, viewHistory, m.width, m.height)
		m.view = viewResults
		return m, nil
	}

	return m, nil
}

// View implements tea.Model.
func (m Model) View() string {
	var body string
	switch m.view {
	case viewMenu:
		body = m.viewMenu()
	case viewSetup:
		body = m.viewSetup()
	case viewMeasure:
		body = m.viewMeasure()
	case viewResults:
		body = m.viewResults()
	case viewHistory:
		body = m.viewHistory()
	case viewAbout:
		body = m.viewAbout()
	}
	if m.status != "" {
		body += "\n\n" + successStyle.Render(m.status)
	}
	return appStyle.Render(body)
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "q" && m.view != viewSetup {
		m.cancelMeasurement()
		return m, tea.Quit
	}

	switch m.view {
	case viewMenu:
		return m.updateMenu(msg)
	case viewSetup:
		return m.updateSetup(msg)
	case viewMeasure:
		if msg.String() == "esc" {
			m.cancelMeasurement()
			m.view = viewMenu
			m.status = "Measurement canceled"
			return m, nil
		}
		return m, nil
	case viewResults:
		return m.updateResults(msg)
	case viewHistory:
		return m.updateHistory(msg)
	case viewAbout:
		if msg.String() == "esc" {
			m.view = viewMenu
			m.status = ""
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) cancelMeasurement() {
	if m.measure.cancel != nil {
		m.measure.cancel()
		m.measure.cancel = nil
	}
	m.measure.running = false
	m.measure.saving = false
	m.measure.gen++
}

func (m Model) saveConfigCmd(cfg storage.Config) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		return configSavedMsg{err: store.SaveConfig(cfg)}
	}
}

func (m Model) loadSessionsCmd() tea.Cmd {
	store := m.store
	return func() tea.Msg {
		sessions, err := store.ListSessions()
		return sessionsLoadedMsg{sessions: sessions, err: err}
	}
}

func (m Model) openSessionCmd(path string) tea.Cmd {
	store := m.store
	return func() tea.Msg {
		samples, _, err := store.LoadSession(path)
		return sessionLoadedMsg{path: path, samples: samples, err: err}
	}
}

func friendlyError(err error) string {
	if errors.Is(err, context.Canceled) || errors.Is(err, powermetrics.ErrCanceled) {
		return ""
	}
	return oneLine(err.Error())
}

func (m Model) footer(text string) string {
	if m.width > 0 {
		text = strings.TrimSpace(text)
	}
	return helpStyle.Render(text)
}

func (m Model) header(title, subtitle string) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(title))
	if subtitle != "" {
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render(subtitle))
	}
	b.WriteString("\n\n")
	return b.String()
}

func (m Model) centeredError(text string) string {
	return errorStyle.Render(text) + "\n"
}
