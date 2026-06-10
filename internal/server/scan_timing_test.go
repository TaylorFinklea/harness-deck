package server

import (
	"strings"
	"testing"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// TestScanTimingWarnPath confirms that when a scan exceeds the 500ms warn
// threshold, the server emits a log line containing "WARN" and the scan
// duration. We inject a fake clock (testNowFn + testScanDurationFn seam) that
// returns a duration well above the threshold.
func TestScanTimingWarnPath(t *testing.T) {
	central := t.TempDir()
	// No real reports needed — we're testing the logging path only.
	s, err := New(config.Config{CentralDir: central})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// capture log output via the test seam
	var logged []string
	s.testScanLogFn = func(msg string) { logged = append(logged, msg) }

	// inject a fake scan duration above the 500ms threshold
	s.testScanDuration = 600 * time.Millisecond

	ws := s.initWatchState()
	ws = s.tick(ws)

	// The warn path must have fired.
	var warnLine string
	for _, l := range logged {
		if strings.Contains(strings.ToUpper(l), "WARN") {
			warnLine = l
			break
		}
	}
	if warnLine == "" {
		t.Errorf("expected a WARN log line for >500ms scan; got logs: %v", logged)
	}
}

// TestScanTimingQuietBelowFloor confirms that a fast scan (below the 100ms
// quiet floor) emits no log line at all.
func TestScanTimingQuietBelowFloor(t *testing.T) {
	central := t.TempDir()
	s, err := New(config.Config{CentralDir: central})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	var logged []string
	s.testScanLogFn = func(msg string) { logged = append(logged, msg) }
	s.testScanDuration = 5 * time.Millisecond // well below 100ms floor

	ws := s.initWatchState()
	ws = s.tick(ws)

	if len(logged) != 0 {
		t.Errorf("expected no log for fast scan; got: %v", logged)
	}
	_ = ws
}
