package storage

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"powermeter/internal/powermetrics"
)

const (
	appDirName    = "powermeter"
	schemaVersion = "powermeter v1"

	MinTotalSeconds = 60
	MaxTotalSeconds = 86400
	MinIntervalMs   = 1000
	MaxIntervalMs   = 30000

	MinTotalMinutes    = float64(MinTotalSeconds) / 60
	MaxTotalMinutes    = float64(MaxTotalSeconds) / 60
	MinIntervalSeconds = float64(MinIntervalMs) / 1000
	MaxIntervalSeconds = float64(MaxIntervalMs) / 1000
)

// Config holds the measurement settings.
type Config struct {
	TotalSeconds int `json:"total_seconds"`
	IntervalMs   int `json:"interval_ms"`
}

// DefaultConfig returns the initial measurement settings.
func DefaultConfig() Config {
	return Config{TotalSeconds: 600, IntervalMs: 2000}
}

// Validate checks the measurement settings.
func (c Config) Validate() error {
	if c.TotalSeconds < MinTotalSeconds || c.TotalSeconds > MaxTotalSeconds {
		return fmt.Errorf("total time must be between %d and %d seconds", MinTotalSeconds, MaxTotalSeconds)
	}
	if c.IntervalMs < MinIntervalMs || c.IntervalMs > MaxIntervalMs {
		return fmt.Errorf("sample interval must be between %d and %d ms", MinIntervalMs, MaxIntervalMs)
	}
	if c.IntervalMs > c.TotalSeconds*1000 {
		return errors.New("sample interval must be shorter than total time")
	}
	return nil
}

// SampleCount returns the number of powermetrics samples for the config.
func (c Config) SampleCount() int {
	totalMs := c.TotalSeconds * 1000
	return (totalMs + c.IntervalMs - 1) / c.IntervalMs
}

// Session describes a saved measurement.
type Session struct {
	Path      string
	Name      string
	StartedAt time.Time
	Modified  time.Time
	Size      int64
}

// Store reads and writes application data in a directory.
type Store struct {
	dir string
}

// NewStore creates a store for the given directory.
func NewStore(dir string) *Store {
	return &Store{dir: dir}
}

// DefaultStore returns a store in ~/.cache/powermeter.
func DefaultStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home directory: %w", err)
	}
	return NewStore(filepath.Join(home, ".cache", appDirName)), nil
}

// Dir returns the data directory.
func (s *Store) Dir() string {
	return s.dir
}

func (s *Store) ensureDir() error {
	return os.MkdirAll(s.dir, 0o755)
}

// LoadConfig reads the settings, falling back to defaults.
func (s *Store) LoadConfig() (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(filepath.Join(s.dir, "config.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return DefaultConfig(), fmt.Errorf("parse config: %w", err)
	}
	if cfg.TotalSeconds <= 0 {
		cfg.TotalSeconds = DefaultConfig().TotalSeconds
	}
	if cfg.IntervalMs <= 0 {
		cfg.IntervalMs = DefaultConfig().IntervalMs
	}
	return cfg, nil
}

// SaveConfig writes the settings.
func (s *Store) SaveConfig(cfg Config) error {
	if err := s.ensureDir(); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	path := filepath.Join(s.dir, "config.json")
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

// SaveSession writes samples to a CSV file in the store directory.
func (s *Store) SaveSession(samples []powermetrics.Sample, interval time.Duration, startedAt time.Time) (string, error) {
	if len(samples) == 0 {
		return "", errors.New("no samples to save")
	}
	if err := s.ensureDir(); err != nil {
		return "", fmt.Errorf("create data directory: %w", err)
	}

	path := filepath.Join(s.dir, "power_log_"+startedAt.Format("20060102_150405")+".csv")
	file, err := os.Create(path)
	if err != nil {
		return "", fmt.Errorf("create session file: %w", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	write := func(record []string) error {
		if err := writer.Write(record); err != nil {
			return err
		}
		return nil
	}

	comments := [][]string{
		{"# " + schemaVersion},
		{"# session_start=" + startedAt.Format(time.RFC3339)},
		{"# sample_interval_ms=" + strconv.Itoa(int(interval.Milliseconds()))},
		{"# energy_impact_per_s is the OS per-process energy impact score (higher = more energy); combined_power_mw is CPU+GPU+ANE power."},
		{"record", "epoch", "iso_time", "pid", "name", "cpu_ms", "gpu_ms", "energy_impact_per_s", "wakeups", "combined_power_mw", "elapsed_ms"},
	}
	for _, row := range comments {
		if err := write(row); err != nil {
			return "", fmt.Errorf("write session header: %w", err)
		}
	}

	for _, sample := range samples {
		epoch := formatEpoch(sample.Timestamp)
		iso := sample.Timestamp.Format(time.RFC3339)
		systemPower := ""
		if sample.PowerMW > 0 {
			systemPower = formatFloat(sample.PowerMW)
		}
		elapsed := ""
		if sample.Elapsed > 0 {
			elapsed = formatFloat(float64(sample.Elapsed) / float64(time.Millisecond))
		}
		if err := write([]string{"S", epoch, iso, "", "", "", "", "", "", systemPower, elapsed}); err != nil {
			return "", fmt.Errorf("write session sample: %w", err)
		}
		for _, process := range sample.Processes {
			power := ""
			if process.PowerMW > 0 {
				power = formatFloat(process.PowerMW)
			}
			row := []string{
				"P", epoch, iso,
				strconv.Itoa(process.PID), process.Name,
				formatFloat(process.CPUTimeMs),
				formatFloat(process.GPUTimeMs),
				formatFloat(process.EnergyImpact),
				formatFloat(process.WakeupsPerSec),
				power, "",
			}
			if err := write(row); err != nil {
				return "", fmt.Errorf("write process sample: %w", err)
			}
		}
	}

	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", fmt.Errorf("write session file: %w", err)
	}
	return path, nil
}

// ListSessions returns saved measurements, newest first.
func (s *Store) ListSessions() ([]Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var sessions []Session
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, "power_log_") || !strings.HasSuffix(name, ".csv") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		session := Session{
			Path:      filepath.Join(s.dir, name),
			Name:      name,
			Modified:  info.ModTime(),
			Size:      info.Size(),
			StartedAt: sessionStart(name, info.ModTime()),
		}
		sessions = append(sessions, session)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].Modified.After(sessions[j].Modified)
	})
	return sessions, nil
}

// LoadSession reads a saved measurement back into samples.
func (s *Store) LoadSession(path string) ([]powermetrics.Sample, time.Duration, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open session: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	var samples []powermetrics.Sample
	var interval time.Duration
	var current *powermetrics.Sample

	for {
		row, err := reader.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, 0, fmt.Errorf("parse session: %w", err)
		}
		if len(row) == 0 {
			continue
		}
		first := row[0]
		if strings.HasPrefix(first, "#") {
			if value, ok := strings.CutPrefix(first, "# sample_interval_ms="); ok {
				if ms, err := strconv.Atoi(value); err == nil {
					interval = time.Duration(ms) * time.Millisecond
				}
			}
			continue
		}
		if first == "record" {
			continue
		}

		switch first {
		case "S":
			sample := powermetrics.Sample{
				Timestamp: parseTimestamp(field(row, 1), field(row, 2)),
				PowerMW:   parseFloat(field(row, 9)),
				Elapsed:   time.Duration(parseFloat(field(row, 10)) * float64(time.Millisecond)),
			}
			samples = append(samples, sample)
			current = &samples[len(samples)-1]
		case "P":
			if current == nil {
				samples = append(samples, powermetrics.Sample{
					Timestamp: parseTimestamp(field(row, 1), field(row, 2)),
				})
				current = &samples[len(samples)-1]
			}
			current.Processes = append(current.Processes, powermetrics.Process{
				PID:           int(parseFloat(field(row, 3))),
				Name:          field(row, 4),
				CPUTimeMs:     parseFloat(field(row, 5)),
				GPUTimeMs:     parseFloat(field(row, 6)),
				EnergyImpact:  parseFloat(field(row, 7)),
				WakeupsPerSec: parseFloat(field(row, 8)),
				PowerMW:       parseFloat(field(row, 9)),
			})
		}
	}

	return samples, interval, nil
}

func sessionStart(name string, fallback time.Time) time.Time {
	trimmed := strings.TrimSuffix(strings.TrimPrefix(name, "power_log_"), ".csv")
	parsed, err := time.ParseInLocation("20060102_150405", trimmed, time.Local)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseTimestamp(epoch, iso string) time.Time {
	if value, err := strconv.ParseFloat(epoch, 64); err == nil {
		seconds := int64(value)
		nanos := int64((value - float64(seconds)) * float64(time.Second))
		return time.Unix(seconds, nanos)
	}
	if parsed, err := time.Parse(time.RFC3339, iso); err == nil {
		return parsed
	}
	return time.Time{}
}

func field(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

func parseFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}

func formatEpoch(t time.Time) string {
	return formatFloat(float64(t.UnixNano()) / float64(time.Second))
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 3, 64)
}
