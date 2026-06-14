package server

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

func getProjects(t *testing.T, s *Server, target string) []projectView {
	t.Helper()
	req := httptest.NewRequest("GET", target, nil)
	w := httptest.NewRecorder()
	s.handleProjects(w, req)
	if w.Code != 200 {
		t.Fatalf("GET %s = %d", target, w.Code)
	}
	var resp struct {
		Projects []projectView `json:"projects"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Projects
}

func findProject(views []projectView, name string) *projectView {
	for i := range views {
		if views[i].Project == name {
			return &views[i]
		}
	}
	return nil
}

// TestProjectsHistoryCapped checks the per-project history is capped to
// historyCap by default, reports the true total, and is uncapped with ?all=1.
func TestProjectsHistoryCapped(t *testing.T) {
	central := t.TempDir()
	const n = historyCap + 5
	for i := 0; i < n; i++ {
		minimalReport(t, central, "big", "run"+itoa3(i))
	}
	s := newTestServerFull(t, config.Config{CentralDir: central})

	def := findProject(getProjects(t, s, "/api/projects"), "big")
	if def == nil {
		t.Fatal("project 'big' not in default view")
	}
	if len(def.History) != historyCap {
		t.Errorf("default history len = %d, want %d", len(def.History), historyCap)
	}
	if def.HistoryTotal != n {
		t.Errorf("history_total = %d, want %d", def.HistoryTotal, n)
	}

	all := findProject(getProjects(t, s, "/api/projects?all=1"), "big")
	if all == nil || len(all.History) != n {
		t.Fatalf("?all=1 history len = %d, want %d", len(all.History), n)
	}
	if all.HistoryTotal != n {
		t.Errorf("?all=1 history_total = %d, want %d", all.HistoryTotal, n)
	}
}

// TestProjectsHistoryAtExactCap checks the boundary: exactly historyCap runs
// are NOT truncated (the cap is a strict `len > limit`), and history_total
// equals the count.
func TestProjectsHistoryAtExactCap(t *testing.T) {
	central := t.TempDir()
	for i := 0; i < historyCap; i++ {
		minimalReport(t, central, "exact", "run"+itoa3(i))
	}
	s := newTestServerFull(t, config.Config{CentralDir: central})

	p := findProject(getProjects(t, s, "/api/projects"), "exact")
	if p == nil {
		t.Fatal("project 'exact' not found")
	}
	if len(p.History) != historyCap {
		t.Errorf("history len = %d, want %d (exactly cap → no truncation)", len(p.History), historyCap)
	}
	if p.HistoryTotal != historyCap {
		t.Errorf("history_total = %d, want %d", p.HistoryTotal, historyCap)
	}
}

// TestRenderDocCachesByMtime verifies the doc cache serves a memoized render
// while the file's mtime is unchanged and re-renders once it changes.
func TestRenderDocCachesByMtime(t *testing.T) {
	s := newTestServerFull(t, config.Config{CentralDir: t.TempDir()})
	path := filepath.Join(t.TempDir(), "roadmap.md")
	base := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

	mustWrite(t, path, "# First")
	mustChtimes(t, path, base)
	first, ok := s.renderDoc(path)
	if !ok || !containsStr(first, "First") {
		t.Fatalf("first render = %q ok=%v", first, ok)
	}

	// Overwrite the content but pin mtime back: a cache hit must serve the
	// OLD render (proving it didn't re-read the file).
	mustWrite(t, path, "# Second")
	mustChtimes(t, path, base)
	cached, _ := s.renderDoc(path)
	if cached != first {
		t.Errorf("mtime unchanged should hit cache; got %q want %q", cached, first)
	}

	// Advance mtime: the cache must invalidate and pick up the new content.
	mustChtimes(t, path, base.Add(time.Second))
	fresh, _ := s.renderDoc(path)
	if !containsStr(fresh, "Second") {
		t.Errorf("after mtime change render = %q, want new content", fresh)
	}

	// A missing file is not ok.
	if _, ok := s.renderDoc(filepath.Join(t.TempDir(), "nope.md")); ok {
		t.Error("missing file should not be ok")
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustChtimes(t *testing.T, path string, mod time.Time) {
	t.Helper()
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func itoa3(n int) string {
	return string(rune('0'+n/100%10)) + string(rune('0'+n/10%10)) + string(rune('0'+n%10))
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
