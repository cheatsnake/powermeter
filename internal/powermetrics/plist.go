package powermetrics

import (
	"fmt"
	"strconv"
	"time"

	"howett.net/plist"
)

// parseSample decodes one XML plist sample.
//
// powermetrics writes NUL-separated XML plists. Key names differ between macOS
// releases, so every lookup accepts a list of known aliases.
func parseSample(blob []byte) (Sample, error) {
	var root map[string]any
	if _, err := plist.Unmarshal(blob, &root); err != nil {
		return Sample{}, fmt.Errorf("decode plist: %w", err)
	}

	sample := Sample{
		Timestamp: time.Now(),
		Elapsed:   elapsed(root),
		PowerMW:   systemPower(root),
	}
	if ts, ok := root["timestamp"].(time.Time); ok {
		sample.Timestamp = ts
	}

	tasks, ok := root["tasks"].([]any)
	if !ok {
		return sample, nil
	}

	seconds := sample.Elapsed.Seconds()
	var totalImpact float64
	for _, raw := range tasks {
		task, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		name, _ := first(task, "name", "command").(string)
		if name == "" {
			continue
		}
		process := Process{
			PID:           int(number(first(task, "pid"))),
			Name:          name,
			CPUTimeMs:     rateToMs(first(task, "cputime_ms_per_s"), seconds, first(task, "cpu_time", "cpu_time_ns", "cpu_ms")),
			GPUTimeMs:     rateToMs(first(task, "gputime_ms_per_s"), seconds, first(task, "gpu_time", "gpu_time_ns", "gpu_ms")),
			EnergyImpact:  number(first(task, "energy_impact_per_s", "energy_impact")),
			WakeupsPerSec: wakeups(task),
		}
		totalImpact += process.EnergyImpact
		sample.Processes = append(sample.Processes, process)
	}

	allocatePower(sample.Processes, sample.PowerMW, totalImpact)

	return sample, nil
}

// allocatePower splits the system power among processes in proportion to their
// energy impact. powermetrics reports power only for the whole system, so the
// per-process value is an estimate.
func allocatePower(processes []Process, systemPowerMW, totalImpact float64) {
	if systemPowerMW <= 0 || totalImpact <= 0 {
		return
	}
	for i := range processes {
		share := processes[i].EnergyImpact / totalImpact
		if share < 0 {
			share = 0
		}
		processes[i].PowerMW = systemPowerMW * share
	}
}

func elapsed(root map[string]any) time.Duration {
	if ns := number(first(root, "elapsed_ns")); ns > 0 {
		return time.Duration(ns)
	}
	if ms := number(first(root, "elapsed_ms")); ms > 0 {
		return time.Duration(ms * float64(time.Millisecond))
	}
	return 0
}

func systemPower(root map[string]any) float64 {
	if processor, ok := root["processor"].(map[string]any); ok {
		if power := number(first(processor, "combined_power", "combined_power_mw")); power > 0 {
			return power
		}
	}
	return number(first(root, "combined_power", "combined_power_mw"))
}

// rateToMs converts a per-second rate to a value for the whole sample window.
// If no rate is available, it falls back to an absolute value.
func rateToMs(rate any, seconds float64, fallback any) float64 {
	if rate != nil {
		return number(rate) * seconds
	}
	switch fallback {
	case nil:
		return 0
	}
	value := number(fallback)
	if value > 100000 {
		return nanosToMs(value)
	}
	return value
}

func wakeups(task map[string]any) float64 {
	var total float64
	var found bool
	for _, key := range []string{"intr_wakeups_per_s", "idle_wakeups_per_s"} {
		if v, ok := task[key]; ok {
			total += number(v)
			found = true
		}
	}
	if found {
		return total
	}
	for _, key := range []string{"interrupt_wakeups", "package_idle_wakeups"} {
		if v, ok := task[key]; ok {
			total += number(v)
			found = true
		}
	}
	if found {
		return total
	}
	return number(first(task, "wakeups_per_s", "wakeups"))
}

func first(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok && v != nil {
			return v
		}
	}
	return nil
}

func number(v any) float64 {
	switch n := v.(type) {
	case uint64:
		return float64(n)
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case uint:
		return float64(n)
	case int32:
		return float64(n)
	case uint32:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	case string:
		f, err := strconv.ParseFloat(n, 64)
		if err != nil {
			return 0
		}
		return f
	}
	return 0
}

func nanosToMs(ns float64) float64 {
	return ns / float64(time.Millisecond)
}
