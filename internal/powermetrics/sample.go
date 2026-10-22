package powermetrics

import "time"

// Sample is a single powermetrics sample.
type Sample struct {
	Timestamp time.Time
	Elapsed   time.Duration
	PowerMW   float64
	Processes []Process
}

// Process is the per-process part of a sample.
type Process struct {
	PID           int
	Name          string
	CPUTimeMs     float64
	GPUTimeMs     float64
	EnergyImpact  float64
	WakeupsPerSec float64
	PowerMW       float64
}
