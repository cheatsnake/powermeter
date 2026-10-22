package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"powermeter/internal/powermetrics"
	"powermeter/internal/storage"
)

func testModel(t *testing.T) Model {
	t.Helper()
	store := storage.NewStore(t.TempDir())
	return New(store, storage.DefaultConfig(), func(time.Duration, int) Sampler {
		return nil
	})
}

func update(t *testing.T, m Model, msg tea.Msg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(msg)
	model, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return model, cmd
}

func runeKey(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

func TestMenuNavigation(t *testing.T) {
	m := testModel(t)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})

	if view := m.View(); !strings.Contains(view, "Powermeter") || !strings.Contains(view, "Start measure") {
		t.Fatalf("unexpected menu view:\n%s", view)
	}

	for i := 0; i < 3; i++ {
		m, _ = update(t, m, runeKey('j'))
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if view := m.View(); !strings.Contains(view, "About Powermeter") {
		t.Fatalf("expected about view:\n%s", view)
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if view := m.View(); !strings.Contains(view, "Start measure") {
		t.Fatalf("expected menu view:\n%s", view)
	}
}

func TestSetupValidation(t *testing.T) {
	m := testModel(t)
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = update(t, m, runeKey('j'))
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if view := m.View(); !strings.Contains(view, "Measure setup") {
		t.Fatalf("expected setup view:\n%s", view)
	}

	m.setup.inputs[0].SetValue("0.5")
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.setup.err == "" || !strings.Contains(m.View(), "total time must be between") {
		t.Fatalf("expected validation error, got %q", m.setup.err)
	}

	m.setup.inputs[0].SetValue("60")
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.view != viewSetup {
		t.Fatalf("expected to stay in setup until save completes")
	}

	m, _ = update(t, m, configSavedMsg{})
	if m.view != viewMenu {
		t.Fatalf("expected menu after saving, got %v", m.view)
	}
	if !strings.Contains(m.View(), "Settings saved") {
		t.Errorf("expected status message")
	}
}

func TestSetupFloatUnits(t *testing.T) {
	m := testModel(t)
	m.setup.inputs[0].SetValue("1.5")
	m.setup.inputs[1].SetValue("2.5")

	cfg, err := m.setup.config()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	if cfg.TotalSeconds != 90 {
		t.Errorf("TotalSeconds = %d, want 90", cfg.TotalSeconds)
	}
	if cfg.IntervalMs != 2500 {
		t.Errorf("IntervalMs = %d, want 2500", cfg.IntervalMs)
	}

	m.setup.inputs[1].SetValue("0.5")
	if _, err := m.setup.config(); err == nil || !strings.Contains(err.Error(), "sample interval must be between") {
		t.Errorf("expected interval error, got %v", err)
	}
}

func TestResultsView(t *testing.T) {
	samples := []powermetrics.Sample{
		{
			Timestamp: time.Now(),
			Elapsed:   2 * time.Second,
			PowerMW:   1500,
			Processes: []powermetrics.Process{
				{PID: 1, Name: "chrome", EnergyImpact: 100, CPUTimeMs: 2000, PowerMW: 1364},
				{PID: 2, Name: "launchd", EnergyImpact: 10, CPUTimeMs: 500, PowerMW: 136},
			},
		},
	}
	m := testModel(t)
	m.results = newResults(samples, "/tmp/power_log_test.csv", viewMenu, 120, 40)
	m.view = viewResults

	view := m.View()
	for _, want := range []string{"Results", "chrome", "launchd", "avg system power", "Power mW", "1364 mW", "136 mW"} {
		if !strings.Contains(view, want) {
			t.Errorf("results view is missing %q:\n%s", want, view)
		}
	}
	if !strings.Contains(view, "1.50 W") {
		t.Errorf("results view misses formatted power:\n%s", view)
	}
}

func TestHistoryView(t *testing.T) {
	m := testModel(t)
	now := time.Now()
	m.history = historyModel{sessions: []storage.Session{
		{Name: "power_log_20261020_101010.csv", StartedAt: now, Size: 2048},
		{Name: "power_log_20261019_191931.csv", StartedAt: now.Add(-24 * time.Hour), Size: 1024},
	}}
	m.view = viewHistory

	view := m.View()
	if !strings.Contains(view, "power_log_20261020_101010.csv") {
		t.Fatalf("history view misses file names:\n%s", view)
	}
	if !strings.Contains(view, "2.0 KB") {
		t.Errorf("history view misses formatted size:\n%s", view)
	}

	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.history.cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.history.cursor)
	}
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.history.cursor != 0 {
		t.Errorf("cursor should wrap to 0, got %d", m.history.cursor)
	}
}

type fakeSampler struct {
	samples []powermetrics.Sample
}

func (f fakeSampler) Run(ctx context.Context, onSample func(powermetrics.Sample)) error {
	for _, sample := range f.samples {
		onSample(sample)
	}
	return nil
}

func TestMeasurementFlow(t *testing.T) {
	start := time.Now()
	samples := []powermetrics.Sample{
		{
			Timestamp: start,
			Elapsed:   2 * time.Second,
			PowerMW:   1200,
			Processes: []powermetrics.Process{
				{PID: 1, Name: "chrome", EnergyImpact: 50, CPUTimeMs: 1000},
			},
		},
		{
			Timestamp: start.Add(2 * time.Second),
			Elapsed:   2 * time.Second,
			PowerMW:   900,
			Processes: []powermetrics.Process{
				{PID: 1, Name: "chrome", EnergyImpact: 70, CPUTimeMs: 1200},
			},
		},
	}

	m := New(storage.NewStore(t.TempDir()), storage.Config{TotalSeconds: 600, IntervalMs: 2000},
		func(time.Duration, int) Sampler {
			return fakeSampler{samples: samples}
		})
	m, _ = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 40})
	m, _ = update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.view != viewMeasure {
		t.Fatalf("expected measure view, got %v", m.view)
	}

	cmd := m.measure.waitCmd(t)
	for step := 0; step < 100 && m.view == viewMeasure; step++ {
		msg := cmd()
		m, cmd = update(t, m, msg)
		if m.measure.saving {
			break
		}
	}

	if !m.measure.saving {
		t.Fatalf("measurement did not finish, err=%q", m.measure.err)
	}

	m, cmd = update(t, m, cmd())
	if m.view != viewResults {
		t.Fatalf("expected results view, got %v (err: %s)", m.view, m.results.err)
	}
	view := m.View()
	if !strings.Contains(view, "chrome") || !strings.Contains(view, "Results") {
		t.Fatalf("unexpected results view:\n%s", view)
	}
	if m.results.path == "" {
		t.Error("expected a saved session path")
	}
}

func (m *measureModel) waitCmd(t *testing.T) tea.Cmd {
	t.Helper()
	cmd := waitForEvent(m.ch, m.gen)
	if cmd == nil {
		t.Fatal("expected a wait command")
	}
	return cmd
}
