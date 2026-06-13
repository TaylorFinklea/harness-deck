package notify

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeScript writes an executable shell script to a temp dir and returns its
// path, so a test can use it as a single-token notify command (Run appends the
// run dir as a trailing arg, which the scripts ignore).
func writeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notify.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestRunBlankIsNoop(t *testing.T) {
	if err := Run("   ", "/tmp", "proj", "run", "blk"); err != nil {
		t.Fatalf("blank command should be a no-op, got %v", err)
	}
}

func TestRunFastCommandSucceeds(t *testing.T) {
	script := writeScript(t, "exit 0")
	if err := Run(script, "/tmp", "proj", "run", "blk"); err != nil {
		t.Fatalf("fast command should succeed, got %v", err)
	}
}

func TestRunHangingCommandTimesOut(t *testing.T) {
	orig := runTimeout
	runTimeout = 150 * time.Millisecond
	defer func() { runTimeout = orig }()

	script := writeScript(t, "sleep 60")

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		done <- Run(script, "/tmp", "proj", "run", "blk")
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("hanging command should return an error after the timeout")
		}
		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Fatalf("Run took %v, expected to be bounded near the %v timeout", elapsed, runTimeout)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return within the timeout budget; it blocked")
	}
}
