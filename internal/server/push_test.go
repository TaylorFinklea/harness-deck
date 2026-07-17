package server

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// TestTruncateBodyCapsLongPrompts asserts an oversized body is trimmed to a
// rune-boundary-safe preview under the push cap, while a short body is left
// untouched. The cap matters because Web Push silently drops a payload whose
// encrypted record exceeds the aes128gcm limit.
func TestTruncateBodyCapsLongPrompts(t *testing.T) {
	short := "pick one"
	if got := truncateBody(short); got != short {
		t.Errorf("short body altered: got %q, want %q", got, short)
	}

	long := strings.Repeat("a", maxPushBody*2)
	got := truncateBody(long)
	if len(got) > maxPushBody {
		t.Errorf("truncated body = %d bytes, want <= %d", len(got), maxPushBody)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("truncated body missing ellipsis: %q", got[len(got)-8:])
	}

	// A run of multi-byte runes must not be split mid-rune.
	multibyte := strings.Repeat("é", maxPushBody) // 'é' is 2 bytes
	gotMB := truncateBody(multibyte)
	if len(gotMB) > maxPushBody {
		t.Errorf("multibyte truncation = %d bytes, want <= %d", len(gotMB), maxPushBody)
	}
	if !utf8.ValidString(gotMB) {
		t.Errorf("multibyte truncation split a rune: %q", gotMB)
	}
}

// TestTickReappearingAskNoRefire is the subtle case: an open ask that
// transiently vanishes for a tick or two (a momentary read failure, modeled
// here as the report file disappearing) and then returns must NOT re-fire the
// push notifier. The initial appearance fires exactly once; the round trip
// adds nothing.
func TestTickReappearingAskNoRefire(t *testing.T) {
	central := t.TempDir()
	s := newTestServerFull(t, config.Config{CentralDir: central})

	var pushFires int64
	s.testNotifyFn = func() { atomic.AddInt64(&pushFires, 1) }

	// Baseline is empty (report does not exist yet), mirroring startup.
	ws := s.initWatchState()

	// Ask appears — fires exactly once.
	askReportFull(t, central, "proj", "ask1")
	ws = s.tick(ws)
	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadInt64(&pushFires); got != 1 {
		t.Fatalf("after ask appears: pushFires=%d, want 1", got)
	}

	// Ask transiently disappears (report removed from disk).
	if err := os.RemoveAll(filepath.Join(central, "proj")); err != nil {
		t.Fatal(err)
	}
	ws = s.tick(ws)
	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadInt64(&pushFires); got != 1 {
		t.Fatalf("after transient disappearance: pushFires=%d, want 1", got)
	}

	// Same ask reappears — must be retained in the baseline, so no re-fire.
	askReportFull(t, central, "proj", "ask1")
	ws = s.tick(ws)
	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadInt64(&pushFires); got != 1 {
		t.Errorf("after reappearance: pushFires=%d, want 1 (no re-fire)", got)
	}
	_ = ws
}

// TestPushSubject checks the VAPID "sub" claim source: push_subject from the
// config wins, and the built-in repo-URL contact is only the fallback — a
// fork's pushes shouldn't identify the upstream project as the operator.
func TestPushSubject(t *testing.T) {
	s := &Server{cfg: config.Config{PushSubject: "https://example.com/my-fork"}}
	if got := s.pushSubject(); got != "https://example.com/my-fork" {
		t.Errorf("pushSubject() = %q, want configured value", got)
	}
	s = &Server{}
	if got := s.pushSubject(); got != defaultPushSubject {
		t.Errorf("pushSubject() = %q, want defaultPushSubject fallback", got)
	}
}
