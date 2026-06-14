package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// minimalReport writes a bare report.json with no asks.
func minimalReport(t *testing.T, dir, project, id string) {
	t.Helper()
	p := filepath.Join(dir, project, id)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema":"harness-deck/report@1","id":"` + id + `","project":"` + project + `","harness":"claude-code","title":"test","status":"done","created":"2026-01-01T00:00:00Z","blocks":[]}`
	if err := os.WriteFile(filepath.Join(p, "report.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// askReportFull writes a report.json containing one open ask block.
func askReportFull(t *testing.T, dir, project, id string) string {
	t.Helper()
	p := filepath.Join(dir, project, id)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema":"harness-deck/report@1","id":"` + id + `","project":"` + project + `","harness":"claude-code","title":"needs input","status":"awaiting-review","created":"2026-01-02T00:00:00Z","blocks":[{"type":"ask","id":"q1","prompt":"Pick one","mode":"choice","options":["a","b"]}]}`
	path := filepath.Join(p, "report.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// newTestServerFull returns a *Server (not just Handler) for tick tests.
func newTestServerFull(t *testing.T, cfg config.Config) *Server {
	t.Helper()
	isolateState(t)
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

// broadcastCount returns the number of times hub.broadcast was called by
// attaching a buffered SSE listener channel.
func listenBroadcasts(h *hub) (getCount func() int) {
	ch := h.add()
	var n int64
	go func() {
		for range ch {
			atomic.AddInt64(&n, 1)
		}
	}()
	return func() int { return int(atomic.LoadInt64(&n)) }
}

// TestTickNoChangeNoBroadcast asserts that when nothing on disk changes,
// a tick fires no SSE broadcast.
func TestTickNoChangeNoBroadcast(t *testing.T) {
	central := t.TempDir()
	minimalReport(t, central, "proj", "run1")
	s := newTestServerFull(t, config.Config{CentralDir: central})

	count := listenBroadcasts(s.hub)

	// seed initial state
	ws := s.initWatchState()
	// tick with identical disk state — nothing changed
	ws = s.tick(ws)

	// give the goroutine time to drain
	time.Sleep(10 * time.Millisecond)
	if got := count(); got != 0 {
		t.Errorf("tick with no change: broadcast called %d times, want 0", got)
	}
}

// TestTickNewReportBroadcasts asserts that adding a report between ticks
// fires exactly one SSE broadcast.
func TestTickNewReportBroadcasts(t *testing.T) {
	central := t.TempDir()
	s := newTestServerFull(t, config.Config{CentralDir: central})

	count := listenBroadcasts(s.hub)
	ws := s.initWatchState()

	// add a report between ticks
	minimalReport(t, central, "proj", "run1")

	ws = s.tick(ws)
	time.Sleep(10 * time.Millisecond)
	if got := count(); got != 1 {
		t.Errorf("tick after new report: broadcast called %d times, want 1", got)
	}

	// second tick with no further change — no additional broadcast
	ws = s.tick(ws)
	time.Sleep(10 * time.Millisecond)
	if got := count(); got != 1 {
		t.Errorf("second tick no change: broadcast called %d times, want 1", got)
	}
	_ = ws
}

// TestTickReportDisappearsBroadcasts asserts that removing a report fires a
// broadcast on the next tick.
func TestTickReportDisappearsBroadcasts(t *testing.T) {
	central := t.TempDir()
	minimalReport(t, central, "proj", "run1")
	s := newTestServerFull(t, config.Config{CentralDir: central})

	count := listenBroadcasts(s.hub)
	ws := s.initWatchState()
	// tick with initial state (no change)
	ws = s.tick(ws)
	time.Sleep(10 * time.Millisecond)
	if got := count(); got != 0 {
		t.Errorf("first tick no change: broadcast %d, want 0", got)
	}

	// remove the report
	if err := os.RemoveAll(filepath.Join(central, "proj")); err != nil {
		t.Fatal(err)
	}

	ws = s.tick(ws)
	time.Sleep(10 * time.Millisecond)
	if got := count(); got != 1 {
		t.Errorf("tick after removal: broadcast %d, want 1", got)
	}
	_ = ws
}

// TestTickNewAskFiresPushOnce asserts that a newly appearing open ask fires
// the push notifier exactly once, and a second tick with no change fires
// zero additional notifications.
func TestTickNewAskFiresPushOnce(t *testing.T) {
	central := t.TempDir()
	s := newTestServerFull(t, config.Config{CentralDir: central})

	// inject a test push counter seam
	var pushFires int64
	s.testNotifyFn = func() { atomic.AddInt64(&pushFires, 1) }

	ws := s.initWatchState()

	// add a report with an open ask
	askReportFull(t, central, "proj", "ask1")

	ws = s.tick(ws)
	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadInt64(&pushFires); got != 1 {
		t.Errorf("new ask: pushFires=%d, want 1", got)
	}

	// second tick — no change, no additional fire
	ws = s.tick(ws)
	time.Sleep(20 * time.Millisecond)
	if got := atomic.LoadInt64(&pushFires); got != 1 {
		t.Errorf("second tick no change: pushFires=%d, want 1", got)
	}
	_ = ws
}

// TestTickSignatureGatesDigestRecompute asserts that when the store signature
// does NOT change between ticks, currentAskDigests is not recomputed.
// We verify this by checking digest scan count via the digestCallCount seam.
func TestTickSignatureGatesDigestRecompute(t *testing.T) {
	central := t.TempDir()
	// plant a report with an open ask so digests are non-trivial
	askReportFull(t, central, "proj", "ask1")
	s := newTestServerFull(t, config.Config{CentralDir: central})

	var digestCalls int64
	s.testDigestCountFn = func() { atomic.AddInt64(&digestCalls, 1) }

	ws := s.initWatchState()
	// tick 1: sig changes (store is first populated)
	ws = s.tick(ws)
	callsAfterTick1 := atomic.LoadInt64(&digestCalls)

	// tick 2: nothing changed — digest must NOT be recomputed
	ws = s.tick(ws)
	callsAfterTick2 := atomic.LoadInt64(&digestCalls)

	if callsAfterTick2 != callsAfterTick1 {
		t.Errorf("digest recomputed on unchanged tick: calls after tick1=%d, after tick2=%d",
			callsAfterTick1, callsAfterTick2)
	}
	_ = ws
}

// TestNotificationsAddRemoveList exercises the CRUD round-trip through the
// real HTTP handlers: add a destination, list to confirm it appears (redacted),
// then delete it and list again to confirm it's gone.
func TestNotificationsAddRemoveList(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	// write a minimal valid config so Upsert has something to merge into
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	s, err := New(config.Config{CentralDir: t.TempDir(), Port: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := s.Handler()

	// add a webhook destination
	code, body := postJSON(t, h, "/api/notifications",
		`{"name":"team","type":"webhook","url":"https://hooks.example.com/token123"}`)
	if code != 200 {
		t.Fatalf("POST notifications = %d body=%s", code, body)
	}

	// list — should appear with URL redacted to host only
	code, body = get(t, h, "/api/notifications")
	if code != 200 {
		t.Fatalf("GET notifications = %d", code)
	}
	var resp struct {
		Destinations []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			URLHost string `json:"url_host"`
		} `json:"destinations"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Destinations) != 1 || resp.Destinations[0].Name != "team" {
		t.Fatalf("expected 1 destination 'team', got: %+v", resp.Destinations)
	}
	if resp.Destinations[0].URLHost != "https://hooks.example.com" {
		t.Errorf("url_host = %q, want https://hooks.example.com", resp.Destinations[0].URLHost)
	}

	// delete
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/api/notifications/team", nil)
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("DELETE notifications/team = %d", rec.Code)
	}

	// list again — empty
	code, body = get(t, h, "/api/notifications")
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Destinations) != 0 {
		t.Errorf("after delete, still have destinations: %+v", resp.Destinations)
	}
}

// TestNotificationsAddSurvivesReload confirms the add round-trip writes
// config.json to disk so a server restart picks up the destination.
func TestNotificationsAddSurvivesReload(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	s, err := New(config.Config{CentralDir: t.TempDir(), Port: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	code, body := postJSON(t, s.Handler(), "/api/notifications",
		`{"name":"ops","type":"webhook","url":"https://ops.example.com/hook"}`)
	if code != 200 {
		t.Fatalf("POST notifications = %d body=%s", code, body)
	}

	// read config.json back from disk to confirm it was persisted
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("decode persisted config: %v", err)
	}
	notifs, ok := cfg["notifications"].([]any)
	if !ok || len(notifs) != 1 {
		t.Fatalf("notifications on disk = %v, want 1 entry", cfg["notifications"])
	}
}

// TestProjectTogglePersistsAndSurvivesReload confirms that a project toggle
// writes the projects.json state file so a server restart preserves visibility.
func TestProjectTogglePersistsAndSurvivesReload(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("HARNESS_DECK_CONFIG", filepath.Join(cfgDir, "config.json"))

	gitDir := t.TempDir()
	mkAIDoc(t, gitDir, "larkline", "roadmap.md", "# Plan")

	s, err := New(config.Config{CentralDir: t.TempDir(), ScanRoots: []string{gitDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// toggle larkline off
	code, body := postJSON(t, s.Handler(), "/api/projects/toggle", `{"name":"larkline"}`)
	if code != 200 {
		t.Fatalf("toggle = %d body=%s", code, body)
	}

	// create a fresh server — projects.json must preserve disabled state
	s2, err := New(config.Config{CentralDir: t.TempDir(), ScanRoots: []string{gitDir}})
	if err != nil {
		t.Fatalf("New (reload): %v", err)
	}
	code, body = get(t, s2.Handler(), "/api/projects")
	if code != 200 {
		t.Fatalf("GET projects = %d", code)
	}
	var resp struct {
		Discovered []struct {
			Name    string `json:"name"`
			Enabled bool   `json:"enabled"`
		} `json:"discovered"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Discovered) != 1 || resp.Discovered[0].Enabled {
		t.Errorf("after reload discovered = %+v, want larkline enabled=false", resp.Discovered)
	}
}

// writeAskStatus writes (or overwrites) a report with one open ask "q1" at the
// given status — used to drive draft↔awaiting-review transitions.
func writeAskStatus(t *testing.T, dir, project, id, status string) {
	t.Helper()
	p := filepath.Join(dir, project, id)
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"schema":"harness-deck/report@1","id":"` + id + `","project":"` + project +
		`","harness":"claude-code","title":"needs input","status":"` + status +
		`","created":"2026-01-02T00:00:00Z","blocks":[{"type":"ask","id":"q1","prompt":"Pick one","mode":"choice","options":["a","b"]}]}`
	if err := os.WriteFile(filepath.Join(p, "report.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestTickDraftToLiveFiresPush guards the ask-retention × draft-gating
// interaction: a report seen as awaiting-review, drafted, then re-published as
// awaiting-review must fire its push again — the draft period must not leave
// the ask retained in the baseline as "already seen".
func TestTickDraftToLiveFiresPush(t *testing.T) {
	central := t.TempDir()
	s := newTestServerFull(t, config.Config{CentralDir: central})
	var fires int64
	s.testNotifyFn = func() { atomic.AddInt64(&fires, 1) }

	ws := s.initWatchState() // empty baseline

	// Ask appears (awaiting-review) → fires once.
	writeAskStatus(t, central, "proj", "ask1", "awaiting-review")
	ws = s.tick(ws)
	if got := atomic.LoadInt64(&fires); got != 1 {
		t.Fatalf("after first publish: fires=%d, want 1", got)
	}

	// Drafted → suppressed, no fire; the ask must NOT linger in the baseline.
	writeAskStatus(t, central, "proj", "ask1", "draft")
	ws = s.tick(ws)
	if got := atomic.LoadInt64(&fires); got != 1 {
		t.Fatalf("after draft: fires=%d, want 1 (draft suppressed)", got)
	}

	// Re-published → must fire again (the bug: it used to stay silent).
	writeAskStatus(t, central, "proj", "ask1", "awaiting-review")
	ws = s.tick(ws)
	if got := atomic.LoadInt64(&fires); got != 2 {
		t.Fatalf("after re-publish: fires=%d, want 2 (re-open fires)", got)
	}

	// Steady state — no further fire.
	ws = s.tick(ws)
	if got := atomic.LoadInt64(&fires); got != 2 {
		t.Errorf("steady state: fires=%d, want 2", got)
	}
	_ = ws
}

// TestMergeRetainedAsksExpiresAfterNTicks pins the retention boundary and the
// closed-key short-circuit directly.
func TestMergeRetainedAsksExpiresAfterNTicks(t *testing.T) {
	prev := map[string]askDigest{"k": {"x": "p"}}
	cur := map[string]askDigest{} // ask absent this tick
	noClosed := map[string]bool{} // transient: report not intentionally closed

	// Transient disappearance: retained until the askRetainTicks-th miss.
	misses := map[string]int{}
	for i := 1; i < askRetainTicks; i++ {
		merged, next := mergeRetainedAsks(prev, cur, misses, noClosed)
		if _, ok := merged["k"]["x"]; !ok {
			t.Fatalf("miss %d: ask should still be retained", i)
		}
		misses = next
	}
	if merged, _ := mergeRetainedAsks(prev, cur, misses, noClosed); merged["k"]["x"] != "" {
		t.Errorf("ask should expire on miss %d", askRetainTicks)
	}

	// Intentional close: dropped immediately regardless of miss count.
	if merged, _ := mergeRetainedAsks(prev, cur, map[string]int{}, map[string]bool{"k": true}); merged["k"]["x"] != "" {
		t.Errorf("a closed (draft/answered/archived) report's ask must not be retained")
	}
}
