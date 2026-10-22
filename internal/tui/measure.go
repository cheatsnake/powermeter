package tui

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"

	"powermeter/internal/powermetrics"
	"powermeter/internal/storage"
)

type streamEvent struct {
	sample *powermetrics.Sample
	err    error
	done   bool
}

type streamEventMsg struct {
	gen    int
	sample *powermetrics.Sample
	err    error
	done   bool
}

type tickMsg struct {
	gen int
}

func tickCmd(gen int) tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg {
		return tickMsg{gen: gen}
	})
}

func waitForEvent(ch <-chan streamEvent, gen int) tea.Cmd {
	return func() tea.Msg {
		event, ok := <-ch
		if !ok {
			return streamEventMsg{gen: gen, done: true}
		}
		return streamEventMsg{
			gen:    gen,
			sample: event.sample,
			err:    event.err,
			done:   event.done,
		}
	}
}

type measureModel struct {
	cfg     storage.Config
	factory SamplerFactory

	progress  progress.Model
	samples   []powermetrics.Sample
	got       int
	count     int
	gen       int
	startedAt time.Time
	elapsed   time.Duration

	cancel  context.CancelFunc
	ch      chan streamEvent
	running bool
	saving  bool
	err     string
}

func newMeasureModel() measureModel {
	return measureModel{
		progress: progress.New(progress.WithDefaultGradient(), progress.WithWidth(40)),
	}
}

func (m Model) startMeasurement() (tea.Model, tea.Cmd) {
	if err := m.cfg.Validate(); err != nil {
		m.status = err.Error()
		return m, nil
	}

	m.measure = newMeasureModel()
	m.measure.cfg = m.cfg
	m.measure.factory = m.newSampler
	m.measure.count = m.cfg.SampleCount()
	m.measure.progress.Width = clamp(m.width-26, 20, 60)
	m.view = viewMeasure
	m.status = ""

	cmd := m.measure.start()
	return m, tea.Batch(cmd, tickCmd(m.measure.gen))
}

func (m *measureModel) start() tea.Cmd {
	m.gen++
	m.running = true
	m.saving = false
	m.startedAt = time.Now()
	m.elapsed = 0
	m.samples = nil
	m.got = 0
	m.err = ""

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.ch = make(chan streamEvent, 64)

	sampler := m.factory(time.Duration(m.cfg.IntervalMs)*time.Millisecond, m.count)
	ch := m.ch
	gen := m.gen

	go func() {
		defer close(ch)
		err := sampler.Run(ctx, func(sample powermetrics.Sample) {
			select {
			case ch <- streamEvent{sample: &sample}:
			case <-ctx.Done():
			}
		})
		select {
		case ch <- streamEvent{err: err, done: true}:
		case <-ctx.Done():
		}
	}()

	return waitForEvent(ch, gen)
}

func (m Model) handleStreamEvent(msg streamEventMsg) (tea.Model, tea.Cmd) {
	if m.view != viewMeasure || msg.gen != m.measure.gen {
		return m, nil
	}
	if msg.sample != nil {
		m.measure.samples = append(m.measure.samples, *msg.sample)
		m.measure.got++
	}
	if !msg.done {
		return m, waitForEvent(m.measure.ch, m.measure.gen)
	}

	m.measure.running = false
	if msg.err != nil {
		if errors.Is(msg.err, powermetrics.ErrCanceled) || errors.Is(msg.err, context.Canceled) {
			m.view = viewMenu
			m.status = "Measurement canceled"
			return m, nil
		}
		m.measure.err = friendlyError(msg.err)
		return m, nil
	}
	if len(m.measure.samples) == 0 {
		m.measure.err = "powermetrics returned no samples"
		return m, nil
	}

	m.measure.saving = true
	return m, m.saveMeasureCmd()
}

func (m Model) saveMeasureCmd() tea.Cmd {
	store := m.store
	samples := m.measure.samples
	interval := time.Duration(m.measure.cfg.IntervalMs) * time.Millisecond
	startedAt := m.measure.startedAt
	return func() tea.Msg {
		path, err := store.SaveSession(samples, interval, startedAt)
		return measureSavedMsg{gen: m.measure.gen, path: path, samples: samples, err: err}
	}
}

func (m Model) viewMeasure() string {
	var b strings.Builder
	b.WriteString(m.header("Measuring system activity", ""))

	if m.measure.err != "" {
		b.WriteString(m.centeredError(m.measure.err))
		b.WriteString(m.footer("esc back to menu · q quit"))
		return b.String()
	}

	total := time.Duration(m.measure.cfg.TotalSeconds) * time.Second
	elapsed := m.measure.elapsed
	if elapsed == 0 {
		elapsed = time.Since(m.measure.startedAt)
	}

	percent := 0.0
	if m.measure.count > 0 {
		percent = float64(m.measure.got) / float64(m.measure.count)
	}
	if m.measure.running && total > 0 {
		if timePercent := float64(elapsed) / float64(total); timePercent > percent {
			percent = timePercent
		}
		if percent > 0.99 {
			percent = 0.99
		}
	}
	if m.measure.saving || m.measure.got >= m.measure.count {
		percent = 1
	}
	percent = math.Min(1, math.Max(0, percent))

	b.WriteString(m.measure.progress.ViewAs(percent))
	b.WriteString("\n\n")

	if m.measure.saving {
		b.WriteString(subtitleStyle.Render("Saving results..."))
	} else {
		remaining := total - time.Duration(float64(total)*percent)
		if remaining < 0 {
			remaining = 0
		}
		b.WriteString(fmt.Sprintf(
			"sample %d/%d  ·  elapsed %s  ·  remaining %s",
			m.measure.got, m.measure.count,
			shortDuration(elapsed), shortDuration(remaining),
		))
	}

	b.WriteString("\n\n")
	b.WriteString(subtitleStyle.Render("A macOS password prompt may appear. Leave this window open and use your Mac as usual."))
	b.WriteString(m.footer("esc cancel · q quit"))
	return b.String()
}
