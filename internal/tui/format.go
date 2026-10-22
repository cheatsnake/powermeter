package tui

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func clamp(value, low, high int) int {
	if high < low {
		high = low
	}
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func shortDuration(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	if hours > 0 {
		return fmt.Sprintf("%dh %02dm", hours, minutes)
	}
	return fmt.Sprintf("%dm %02ds", minutes, seconds)
}

func formatMillis(ms float64) string {
	if ms < 1000 {
		return fmt.Sprintf("%.0fms", ms)
	}
	return shortDuration(time.Duration(ms * float64(time.Millisecond)))
}

func formatPowerMW(value float64) string {
	if value <= 0 {
		return "0 mW"
	}
	if value < 100 {
		return fmt.Sprintf("%.1f mW", math.Round(value*10)/10)
	}
	return fmt.Sprintf("%.0f mW", value)
}

func formatMinutes(seconds int) string {
	return strconv.FormatFloat(float64(seconds)/60, 'f', -1, 64)
}

func formatSeconds(ms int) string {
	return strconv.FormatFloat(float64(ms)/1000, 'f', -1, 64)
}

func humanBytes(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}

func oneLine(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
