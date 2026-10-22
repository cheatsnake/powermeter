package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"powermeter/internal/powermetrics"
)

func TestConfigDefaults(t *testing.T) {
	store := NewStore(t.TempDir())
	cfg, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg != DefaultConfig() {
		t.Errorf("cfg = %+v, want defaults", cfg)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	want := Config{TotalSeconds: 120, IntervalMs: 5000}
	if err := store.SaveConfig(want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := store.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got != want {
		t.Errorf("cfg = %+v, want %+v", got, want)
	}
}

func TestConfigValidate(t *testing.T) {
	valid := DefaultConfig()
	if err := valid.Validate(); err != nil {
		t.Errorf("default config: %v", err)
	}

	cases := []Config{
		{TotalSeconds: MinTotalSeconds - 1, IntervalMs: MinIntervalMs},
		{TotalSeconds: MaxTotalSeconds + 1, IntervalMs: MinIntervalMs},
		{TotalSeconds: 60, IntervalMs: MinIntervalMs - 1},
		{TotalSeconds: 60, IntervalMs: MaxIntervalMs + 1},
		{TotalSeconds: 10, IntervalMs: 20000},
	}
	for _, cfg := range cases {
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate(%+v) = nil, want error", cfg)
		}
	}
}

func TestSampleCount(t *testing.T) {
	cfg := Config{TotalSeconds: 600, IntervalMs: 3000}
	if got := cfg.SampleCount(); got != 200 {
		t.Errorf("SampleCount = %d, want 200", got)
	}
}

func TestSessionRoundTrip(t *testing.T) {
	store := NewStore(t.TempDir())
	started := time.Date(2026, 10, 20, 9, 43, 19, 0, time.Local)
	samples := []powermetrics.Sample{
		{
			Timestamp: started.Add(2 * time.Second),
			Elapsed:   2 * time.Second,
			PowerMW:   1482.91,
			Processes: []powermetrics.Process{
				{PID: 860, Name: "appstoreagent", CPUTimeMs: 1951.029, EnergyImpact: 458.906, WakeupsPerSec: 15.982},
				{PID: 99, Name: "Weird, Name", CPUTimeMs: 10, GPUTimeMs: 5, EnergyImpact: 1.5},
			},
		},
		{
			Timestamp: started.Add(4 * time.Second),
			Elapsed:   2 * time.Second,
			PowerMW:   1200,
			Processes: []powermetrics.Process{
				{PID: 860, Name: "appstoreagent", CPUTimeMs: 100, EnergyImpact: 20},
			},
		},
	}

	path, err := store.SaveSession(samples, 2*time.Second, started)
	if err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	if filepath.Ext(path) != ".csv" {
		t.Errorf("path = %s, want a .csv file", path)
	}

	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	if !sessions[0].StartedAt.Equal(started) {
		t.Errorf("StartedAt = %v, want %v", sessions[0].StartedAt, started)
	}

	loaded, interval, err := store.LoadSession(path)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if interval != 2*time.Second {
		t.Errorf("interval = %v, want 2s", interval)
	}
	if len(loaded) != 2 {
		t.Fatalf("got %d samples, want 2", len(loaded))
	}
	if loaded[0].PowerMW != 1482.91 {
		t.Errorf("PowerMW = %v, want 1482.91", loaded[0].PowerMW)
	}
	if delta := loaded[0].Timestamp.Sub(samples[0].Timestamp); delta > time.Millisecond || delta < -time.Millisecond {
		t.Errorf("Timestamp delta = %v", delta)
	}
	if len(loaded[0].Processes) != 2 {
		t.Fatalf("got %d processes, want 2", len(loaded[0].Processes))
	}
	process := loaded[0].Processes[0]
	if process.Name != "appstoreagent" || process.CPUTimeMs != 1951.029 || process.EnergyImpact != 458.906 {
		t.Errorf("process = %+v", process)
	}
	if loaded[0].Processes[1].Name != "Weird, Name" {
		t.Errorf("quoted name = %q", loaded[0].Processes[1].Name)
	}
	if loaded[0].Elapsed != 2*time.Second {
		t.Errorf("Elapsed = %v, want 2s", loaded[0].Elapsed)
	}
}

func TestLoadLegacySession(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)
	legacy := `# powerstats v1
# session_start=2026-10-19T19:19:31+03:00
# sample_interval_ms=2000
# energy_impact_per_s is the OS per-process energy impact score (higher = more energy); combined_power_mw is CPU+GPU+ANE power.
record,epoch,iso_time,pid,name,cpu_ms,gpu_ms,energy_impact_per_s,wakeups,combined_power_mw,elapsed_ms
S,1792426775.845,2026-10-19T19:19:35+03:00,,,,,,,1482.910,2012.3
P,1792426775.845,2026-10-19T19:19:35+03:00,860,appstoreagent,1951.029,0.000,458.906,15.982,,
`
	path := filepath.Join(dir, "power_log_20261019_191931.csv")
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	samples, interval, err := store.LoadSession(path)
	if err != nil {
		t.Fatalf("LoadSession: %v", err)
	}
	if interval != 2*time.Second {
		t.Errorf("interval = %v, want 2s", interval)
	}
	if len(samples) != 1 {
		t.Fatalf("got %d samples, want 1", len(samples))
	}
	if samples[0].PowerMW != 1482.91 {
		t.Errorf("PowerMW = %v", samples[0].PowerMW)
	}
	if len(samples[0].Processes) != 1 || samples[0].Processes[0].Name != "appstoreagent" {
		t.Errorf("processes = %+v", samples[0].Processes)
	}
}

func TestListSessionsEmptyDir(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "missing"))
	sessions, err := store.ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 0 {
		t.Errorf("got %d sessions, want 0", len(sessions))
	}
}
