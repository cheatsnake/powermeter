package powermetrics

import (
	"bufio"
	"math"
	"strings"
	"testing"
	"time"
)

const sampleOne = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>elapsed_ns</key>
	<integer>2000000000</integer>
	<key>processor</key>
	<dict>
		<key>cpu_power</key>
		<real>1300</real>
		<key>gpu_power</key>
		<real>182</real>
		<key>ane_power</key>
		<real>0</real>
		<key>combined_power</key>
		<real>1482</real>
	</dict>
	<key>tasks</key>
	<array>
		<dict>
			<key>pid</key>
			<integer>860</integer>
			<key>name</key>
			<string>appstoreagent</string>
			<key>cputime_ms_per_s</key>
			<real>975.5145</real>
			<key>gputime_ms_per_s</key>
			<real>0</real>
			<key>energy_impact_per_s</key>
			<real>458.906</real>
			<key>intr_wakeups_per_s</key>
			<real>13</real>
			<key>idle_wakeups_per_s</key>
			<real>2.982</real>
		</dict>
		<dict>
			<key>pid</key>
			<integer>169</integer>
			<key>name</key>
			<string>WindowServer</string>
			<key>cputime_ms_per_s</key>
			<real>89.553</real>
			<key>gputime_ms_per_s</key>
			<real>12.5</real>
			<key>energy_impact_per_s</key>
			<real>29.16</real>
			<key>intr_wakeups_per_s</key>
			<real>120</real>
			<key>idle_wakeups_per_s</key>
			<real>58</real>
		</dict>
	</array>
</dict>
</plist>`

const legacySample = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>elapsed_ns</key>
	<integer>2000000000</integer>
	<key>combined_power</key>
	<integer>1482</integer>
	<key>tasks</key>
	<array>
		<dict>
			<key>pid</key>
			<integer>860</integer>
			<key>name</key>
			<string>appstoreagent</string>
			<key>cpu_time</key>
			<integer>1951029000</integer>
			<key>energy_impact</key>
			<integer>458</integer>
			<key>wakeups_per_s</key>
			<real>15.982</real>
		</dict>
	</array>
</dict>
</plist>`

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 0.001
}

func TestParseSample(t *testing.T) {
	sample, err := parseSample([]byte(sampleOne))
	if err != nil {
		t.Fatalf("parseSample: %v", err)
	}

	if sample.Elapsed != 2*time.Second {
		t.Errorf("Elapsed = %v, want 2s", sample.Elapsed)
	}
	if sample.PowerMW != 1482 {
		t.Errorf("PowerMW = %v, want 1482", sample.PowerMW)
	}
	if len(sample.Processes) != 2 {
		t.Fatalf("got %d processes, want 2", len(sample.Processes))
	}

	first := sample.Processes[0]
	if first.PID != 860 || first.Name != "appstoreagent" {
		t.Errorf("first process = %+v", first)
	}
	if !almostEqual(first.CPUTimeMs, 1951.029) {
		t.Errorf("CPUTimeMs = %v, want 1951.029", first.CPUTimeMs)
	}
	if !almostEqual(first.EnergyImpact, 458.906) {
		t.Errorf("EnergyImpact = %v, want 458.906", first.EnergyImpact)
	}
	if !almostEqual(first.WakeupsPerSec, 15.982) {
		t.Errorf("WakeupsPerSec = %v, want 15.982", first.WakeupsPerSec)
	}

	totalImpact := 458.906 + 29.16
	if want := 1482 * 458.906 / totalImpact; !almostEqual(first.PowerMW, want) {
		t.Errorf("PowerMW = %v, want %v", first.PowerMW, want)
	}

	second := sample.Processes[1]
	if want := 1482 * 29.16 / totalImpact; !almostEqual(second.PowerMW, want) {
		t.Errorf("second PowerMW = %v, want %v", second.PowerMW, want)
	}
	if !almostEqual(second.GPUTimeMs, 25) {
		t.Errorf("GPUTimeMs = %v, want 25", second.GPUTimeMs)
	}
	if !almostEqual(second.WakeupsPerSec, 178) {
		t.Errorf("aggregated wakeups = %v, want 178", second.WakeupsPerSec)
	}
}

func TestParseLegacyKeys(t *testing.T) {
	sample, err := parseSample([]byte(legacySample))
	if err != nil {
		t.Fatalf("parseSample: %v", err)
	}
	if sample.PowerMW != 1482 {
		t.Errorf("PowerMW = %v, want 1482", sample.PowerMW)
	}
	if len(sample.Processes) != 1 {
		t.Fatalf("got %d processes, want 1", len(sample.Processes))
	}
	process := sample.Processes[0]
	if !almostEqual(process.CPUTimeMs, 1951.029) {
		t.Errorf("CPUTimeMs = %v, want 1951.029", process.CPUTimeMs)
	}
	if process.EnergyImpact != 458 {
		t.Errorf("EnergyImpact = %v, want 458", process.EnergyImpact)
	}
	if !almostEqual(process.WakeupsPerSec, 15.982) {
		t.Errorf("WakeupsPerSec = %v, want 15.982", process.WakeupsPerSec)
	}
	if !almostEqual(process.PowerMW, 1482) {
		t.Errorf("PowerMW = %v, want 1482", process.PowerMW)
	}
}

func TestReadSamples(t *testing.T) {
	input := sampleOne + "\x00" + sampleOne + "\x00"
	reader := bufio.NewReader(strings.NewReader(input))

	var samples []Sample
	if err := readSamples(reader, func(sample Sample) {
		samples = append(samples, sample)
	}); err != nil {
		t.Fatalf("readSamples: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}
}

func TestParseSampleRejectsBrokenPlist(t *testing.T) {
	if _, err := parseSample([]byte("<plist><dict>")); err == nil {
		t.Fatal("expected an error for a broken plist")
	}
}
