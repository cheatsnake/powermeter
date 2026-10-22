package powermetrics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func fakeSudo(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "sudo")
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestRunnerStreamsSamples(t *testing.T) {
	dir := t.TempDir()
	fixture := filepath.Join(dir, "sample.plist")
	if err := os.WriteFile(fixture, []byte(sampleOne), 0o644); err != nil {
		t.Fatal(err)
	}

	fakeSudo(t, `
case "$1 $2" in
  "-n true") exit 0 ;;
esac
cat "`+fixture+`"
printf '\0'
cat "`+fixture+`"
printf '\0'
`)

	runner := NewRunner(2*time.Second, 2)
	var samples []Sample
	if err := runner.Run(context.Background(), func(sample Sample) {
		samples = append(samples, sample)
	}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("got %d samples, want 2", len(samples))
	}
	if samples[0].PowerMW != 1482 {
		t.Errorf("PowerMW = %v, want 1482", samples[0].PowerMW)
	}
}

func TestRunnerCancel(t *testing.T) {
	fakeSudo(t, `
case "$1 $2" in
  "-n true") exit 0 ;;
esac
while true; do sleep 1; done
`)

	runner := NewRunner(time.Second, 10)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- runner.Run(ctx, func(Sample) {})
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, ErrCanceled) {
			t.Fatalf("Run returned %v, want ErrCanceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
}
