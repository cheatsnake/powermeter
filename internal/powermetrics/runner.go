package powermetrics

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrCanceled reports that the user stopped a measurement.
var ErrCanceled = errors.New("measurement canceled")

// Runner streams samples from the powermetrics utility.
type Runner struct {
	Interval time.Duration
	Count    int
}

// NewRunner creates a runner that takes count samples every interval.
func NewRunner(interval time.Duration, count int) *Runner {
	return &Runner{Interval: interval, Count: count}
}

// Run authenticates, starts powermetrics and calls onSample for every sample.
// It blocks until the requested sample count is reached or ctx is canceled.
func (r *Runner) Run(ctx context.Context, onSample func(Sample)) error {
	if err := authenticate(ctx); err != nil {
		return err
	}

	args := []string{"-n", "powermetrics"}
	args = append(args,
		"-i", strconv.Itoa(int(r.Interval.Milliseconds())),
		"-n", strconv.Itoa(r.Count),
		"-b", "0",
		"--samplers", "tasks,cpu_power,gpu_power,ane_power",
		"--show-process-energy",
		"--show-process-gpu",
		"--handle-invalid-values",
		"-f", "plist",
	)

	cmd := exec.CommandContext(ctx, "sudo", args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
	cmd.WaitDelay = 3 * time.Second

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("connect to powermetrics: %w", err)
	}
	var stderr syncBuffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start powermetrics: %w", err)
	}

	readErr := readSamples(bufio.NewReaderSize(stdout, 1<<20), onSample)
	if readErr != nil {
		_ = cmd.Cancel()
	}
	waitErr := cmd.Wait()

	if ctx.Err() != nil {
		return ErrCanceled
	}
	if readErr != nil {
		return readErr
	}
	if waitErr != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("powermetrics failed: %s", msg)
		}
		return fmt.Errorf("powermetrics failed: %w", waitErr)
	}
	return nil
}

func readSamples(r *bufio.Reader, onSample func(Sample)) error {
	for {
		chunk, err := r.ReadBytes(0)
		blob := bytes.TrimSpace(chunk)
		if len(blob) > 0 && (bytes.Contains(blob, []byte("<plist")) || bytes.HasPrefix(blob, []byte("<?xml"))) {
			sample, parseErr := parseSample(blob)
			if parseErr != nil {
				return parseErr
			}
			onSample(sample)
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read powermetrics output: %w", err)
		}
	}
}

func authenticate(ctx context.Context) error {
	if exec.CommandContext(ctx, "sudo", "-n", "true").Run() == nil {
		return nil
	}

	dir, err := os.MkdirTemp("", "powermeter-askpass-")
	if err != nil {
		return fmt.Errorf("create askpass helper: %w", err)
	}
	defer os.RemoveAll(dir)

	helper := filepath.Join(dir, "askpass.sh")
	script := `#!/bin/sh
osascript -e 'text returned of (display dialog "Powermeter needs administrator privileges to read per-process power data." default answer "" with hidden answer buttons {"Cancel", "OK"} default button "OK" with title "Powermeter")'
`
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		return fmt.Errorf("create askpass helper: %w", err)
	}

	cmd := exec.CommandContext(ctx, "sudo", "-A", "-v")
	cmd.Env = append(os.Environ(), "SUDO_ASKPASS="+helper)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
	cmd.WaitDelay = 3 * time.Second

	var stderr syncBuffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ErrCanceled
		}
		msg := strings.TrimSpace(stderr.String())
		if strings.Contains(strings.ToLower(msg), "cancel") {
			return ErrCanceled
		}
		if msg != "" {
			return fmt.Errorf("administrator privileges are required: %s", msg)
		}
		return fmt.Errorf("administrator privileges are required: %w", err)
	}
	return nil
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
