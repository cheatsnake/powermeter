package analysis

import (
	"testing"
	"time"

	"powermeter/internal/powermetrics"
)

func sample(timestamp time.Time, processes []powermetrics.Process) powermetrics.Sample {
	return powermetrics.Sample{
		Timestamp: timestamp,
		Elapsed:   2 * time.Second,
		PowerMW:   1500,
		Processes: processes,
	}
}

func TestAggregateSortsAndSums(t *testing.T) {
	start := time.Now()
	samples := []powermetrics.Sample{
		sample(start, []powermetrics.Process{
			{PID: 1, Name: "launchd", EnergyImpact: 10, CPUTimeMs: 100, WakeupsPerSec: 2},
			{PID: 2, Name: "chrome", EnergyImpact: 50, CPUTimeMs: 400, WakeupsPerSec: 10},
		}),
		sample(start.Add(2*time.Second), []powermetrics.Process{
			{PID: 1, Name: "launchd", EnergyImpact: 20, CPUTimeMs: 300, WakeupsPerSec: 4},
			{PID: 2, Name: "chrome", EnergyImpact: 30, CPUTimeMs: 200, WakeupsPerSec: 6},
		}),
	}

	stats := Aggregate(samples)
	if len(stats) != 2 {
		t.Fatalf("got %d stats, want 2", len(stats))
	}
	if stats[0].Name != "chrome" {
		t.Errorf("first process = %s, want chrome", stats[0].Name)
	}
	if stats[0].EnergyTotal != 80 {
		t.Errorf("chrome energy total = %v, want 80", stats[0].EnergyTotal)
	}
	if stats[0].EnergyAvg != 40 {
		t.Errorf("chrome energy avg = %v, want 40", stats[0].EnergyAvg)
	}
	if stats[0].CPUTotalMs != 600 {
		t.Errorf("chrome cpu total = %v, want 600", stats[0].CPUTotalMs)
	}
	if stats[1].Samples != 2 || stats[1].EnergyTotal != 30 {
		t.Errorf("launchd stat = %+v", stats[1])
	}

	top := TopN(stats, 1)
	if len(top) != 1 || top[0].Name != "chrome" {
		t.Errorf("TopN = %+v", top)
	}
}

func TestAggregateKeepsProcessesWithReusedPID(t *testing.T) {
	start := time.Now()
	samples := []powermetrics.Sample{
		sample(start, []powermetrics.Process{{PID: 42, Name: "first", EnergyImpact: 1}}),
		sample(start.Add(2*time.Second), []powermetrics.Process{{PID: 42, Name: "second", EnergyImpact: 2}}),
	}

	stats := Aggregate(samples)
	if len(stats) != 2 {
		t.Fatalf("got %d stats, want 2", len(stats))
	}
}

func TestAggregateSortsByPower(t *testing.T) {
	start := time.Now()
	samples := []powermetrics.Sample{
		sample(start, []powermetrics.Process{
			{PID: 1, Name: "low-power", EnergyImpact: 100, PowerMW: 100},
			{PID: 2, Name: "high-power", EnergyImpact: 10, PowerMW: 900},
		}),
	}

	stats := Aggregate(samples)
	if len(stats) != 2 {
		t.Fatalf("got %d stats, want 2", len(stats))
	}
	if stats[0].Name != "high-power" {
		t.Errorf("first process = %s, want high-power", stats[0].Name)
	}
}

func TestAggregateAveragesProcessPower(t *testing.T) {
	start := time.Now()
	samples := []powermetrics.Sample{
		sample(start, []powermetrics.Process{
			{PID: 1, Name: "chrome", EnergyImpact: 10, PowerMW: 100},
			{PID: 2, Name: "launchd", EnergyImpact: 5},
		}),
		sample(start.Add(2*time.Second), []powermetrics.Process{
			{PID: 1, Name: "chrome", EnergyImpact: 20, PowerMW: 300},
		}),
	}

	stats := Aggregate(samples)
	if stats[0].Name != "chrome" {
		t.Fatalf("first process = %s, want chrome", stats[0].Name)
	}
	if !stats[0].HasPower || stats[0].PowerAvgMW != 200 {
		t.Errorf("chrome power = %v (has power %v), want 200", stats[0].PowerAvgMW, stats[0].HasPower)
	}
	if stats[1].Name != "launchd" || stats[1].HasPower {
		t.Errorf("launchd stat = %+v, want no power", stats[1])
	}
}

func TestSummarize(t *testing.T) {
	start := time.Now()
	samples := []powermetrics.Sample{
		sample(start, []powermetrics.Process{{PID: 1, Name: "a", CPUTimeMs: 100}}),
		sample(start.Add(2*time.Second), []powermetrics.Process{{PID: 1, Name: "a", CPUTimeMs: 150}}),
	}

	summary := Summarize(samples)
	if summary.Samples != 2 {
		t.Errorf("Samples = %d, want 2", summary.Samples)
	}
	if summary.Duration != 4*time.Second {
		t.Errorf("Duration = %v, want 4s", summary.Duration)
	}
	if summary.AvgPowerMW != 1500 {
		t.Errorf("AvgPowerMW = %v, want 1500", summary.AvgPowerMW)
	}
	if summary.TotalCPUMs != 250 {
		t.Errorf("TotalCPUMs = %v, want 250", summary.TotalCPUMs)
	}
}

func TestSummarizeEmpty(t *testing.T) {
	summary := Summarize(nil)
	if summary.Samples != 0 || summary.Duration != 0 {
		t.Errorf("unexpected summary: %+v", summary)
	}
}
