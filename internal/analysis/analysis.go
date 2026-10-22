package analysis

import (
	"fmt"
	"sort"
	"time"

	"powermeter/internal/powermetrics"
)

// ProcessStat is an aggregated per-process result.
type ProcessStat struct {
	PID         int
	Name        string
	Samples     int
	EnergyTotal float64
	EnergyAvg   float64
	CPUTotalMs  float64
	GPUTotalMs  float64
	WakeupsAvg  float64
	PowerAvgMW  float64
	HasPower    bool
}

// Summary is an aggregated session result.
type Summary struct {
	Samples     int
	StartedAt   time.Time
	Duration    time.Duration
	AvgPowerMW  float64
	PeakPowerMW float64
	TotalCPUMs  float64
}

// Aggregate merges samples by process and sorts them by average power. If no
// power data is available, it sorts by total energy impact.
func Aggregate(samples []powermetrics.Sample) []ProcessStat {
	byProcess := make(map[string]*ProcessStat)
	for _, sample := range samples {
		for _, process := range sample.Processes {
			key := fmt.Sprintf("%d\x00%s", process.PID, process.Name)
			stat, ok := byProcess[key]
			if !ok {
				stat = &ProcessStat{PID: process.PID, Name: process.Name}
				byProcess[key] = stat
			}
			stat.Samples++
			stat.EnergyTotal += process.EnergyImpact
			stat.CPUTotalMs += process.CPUTimeMs
			stat.GPUTotalMs += process.GPUTimeMs
			stat.WakeupsAvg += process.WakeupsPerSec
			if process.PowerMW > 0 {
				stat.PowerAvgMW += process.PowerMW
				stat.HasPower = true
			}
		}
	}

	stats := make([]ProcessStat, 0, len(byProcess))
	for _, stat := range byProcess {
		stat.EnergyAvg = stat.EnergyTotal / float64(stat.Samples)
		stat.WakeupsAvg /= float64(stat.Samples)
		if stat.HasPower {
			stat.PowerAvgMW /= float64(stat.Samples)
		}
		stats = append(stats, *stat)
	}

	sortByPower := false
	for _, stat := range stats {
		if stat.HasPower {
			sortByPower = true
			break
		}
	}

	sort.Slice(stats, func(i, j int) bool {
		if sortByPower {
			if stats[i].PowerAvgMW != stats[j].PowerAvgMW {
				return stats[i].PowerAvgMW > stats[j].PowerAvgMW
			}
		} else if stats[i].EnergyTotal != stats[j].EnergyTotal {
			return stats[i].EnergyTotal > stats[j].EnergyTotal
		}
		if stats[i].CPUTotalMs != stats[j].CPUTotalMs {
			return stats[i].CPUTotalMs > stats[j].CPUTotalMs
		}
		return stats[i].Name < stats[j].Name
	})
	return stats
}

// TopN returns at most n leading stats.
func TopN(stats []ProcessStat, n int) []ProcessStat {
	if n <= 0 || n > len(stats) {
		n = len(stats)
	}
	return stats[:n]
}

// Summarize aggregates system-wide values.
func Summarize(samples []powermetrics.Sample) Summary {
	summary := Summary{Samples: len(samples)}
	if len(samples) == 0 {
		return summary
	}
	summary.StartedAt = samples[0].Timestamp

	var powerSum float64
	var powerCount int
	for _, sample := range samples {
		summary.Duration += sample.Elapsed
		for _, process := range sample.Processes {
			summary.TotalCPUMs += process.CPUTimeMs
		}
		if sample.PowerMW > 0 {
			powerSum += sample.PowerMW
			powerCount++
			if sample.PowerMW > summary.PeakPowerMW {
				summary.PeakPowerMW = sample.PowerMW
			}
		}
	}
	if powerCount > 0 {
		summary.AvgPowerMW = powerSum / float64(powerCount)
	}
	if summary.Duration == 0 && len(samples) > 1 {
		last := samples[len(samples)-1]
		summary.Duration = last.Timestamp.Sub(samples[0].Timestamp) + last.Elapsed
	}
	return summary
}
