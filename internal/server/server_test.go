package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

const sampleReport = `{
  "schema": "harness-deck/report@1",
  "id": "0x4a2f", "project": "acme", "harness": "claude-code",
  "title": "readiness audit", "status": "awaiting-review",
  "created": "2026-05-18T18:39:50Z",
  "blocks": [{"type": "prose", "markdown": "all **clear**."}]
}`

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	central := t.TempDir()
	dir := filepath.Join(central, "acme", "0x4a2f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(sampleReport), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New(config.Config{CentralDir: central, Port: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.Handler()
}

func get(t *testing.T, h http.Handler, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Code, rec.Body.String()
}

func TestShellServed(t *testing.T) {
	code, body := get(t, newTestServer(t), "/")
	if code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", code)
	}
	for _, want := range []string{"harness-deck", `id="tree"`, `id="content"`, "VimNav.init"} {
		if !strings.Contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
}

func TestReportsAPI(t *testing.T) {
	code, body := get(t, newTestServer(t), "/api/reports")
	if code != http.StatusOK {
		t.Fatalf("GET /api/reports = %d, want 200", code)
	}
	for _, want := range []string{`"project":"acme"`, `"run":"0x4a2f"`, `"status":"awaiting-review"`} {
		if !strings.Contains(body, want) {
			t.Errorf("api response missing %q; body: %s", want, body)
		}
	}
}

func TestReportPageRendered(t *testing.T) {
	h := newTestServer(t)

	code, body := get(t, h, "/r/acme/0x4a2f")
	if code != http.StatusOK {
		t.Fatalf("GET /r/acme/0x4a2f = %d, want 200", code)
	}
	if !strings.Contains(body, `class="panel"`) || !strings.Contains(body, "<b>clear</b>") {
		t.Error("report page did not render the manifest")
	}

	if code, _ := get(t, h, "/r/acme/nope"); code != http.StatusNotFound {
		t.Errorf("GET unknown report = %d, want 404", code)
	}
}

const askReport = `{
  "schema": "harness-deck/report@1",
  "id": "0x1", "project": "acme", "harness": "claude-code",
  "title": "dark mode", "status": "awaiting-review",
  "created": "2026-05-20T14:30:00Z",
  "blocks": [{"type": "ask", "id": "q1", "prompt": "Which set?",
             "mode": "choice", "options": ["semantic", "raw"]}]
}`

func TestRespondRecordsAndShowsAnswer(t *testing.T) {
	central := t.TempDir()
	dir := filepath.Join(central, "acme", "0x1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(askReport), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New(config.Config{CentralDir: central})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := s.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/r/acme/0x1/respond",
		strings.NewReader(`{"block":"q1","value":"semantic"}`))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /respond = %d, want 200", rec.Code)
	}

	saved, err := os.ReadFile(filepath.Join(dir, "responses.json"))
	if err != nil {
		t.Fatalf("responses.json not written: %v", err)
	}
	if !strings.Contains(string(saved), `"semantic"`) {
		t.Errorf("responses.json missing the answer: %s", saved)
	}

	code, page := get(t, h, "/r/acme/0x1")
	if code != http.StatusOK || !strings.Contains(page, "ask-answered") {
		t.Error("report page should render the recorded answer after responding")
	}
}

func TestRoadmapRendersProjectFile(t *testing.T) {
	central := t.TempDir()
	proj := t.TempDir()
	aiDir := filepath.Join(proj, ".docs", "ai")
	if err := os.MkdirAll(aiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aiDir, "roadmap.md"),
		[]byte("# Plan\n\n- ship dark mode"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := New(config.Config{CentralDir: central, Projects: []string{proj}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	code, body := get(t, s.Handler(), "/api/roadmap")
	if code != http.StatusOK {
		t.Fatalf("GET /api/roadmap = %d, want 200", code)
	}
	var got struct {
		Projects []struct {
			HTML    string `json:"html"`
			HasFile bool   `json:"has_file"`
		} `json:"projects"`
	}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode roadmap response: %v", err)
	}
	if len(got.Projects) != 1 {
		t.Fatalf("got %d roadmap projects, want 1", len(got.Projects))
	}
	p := got.Projects[0]
	if !p.HasFile {
		t.Error("project should report has_file=true")
	}
	if !strings.Contains(p.HTML, "<h1>Plan</h1>") || !strings.Contains(p.HTML, "ship dark mode") {
		t.Errorf("rendered roadmap HTML missing content: %q", p.HTML)
	}
}

func TestEventsStreamOpens(t *testing.T) {
	srv := httptest.NewServer(newTestServer(t))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /events: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	buf := make([]byte, 128)
	n, _ := resp.Body.Read(buf)
	if !strings.Contains(string(buf[:n]), "event: hello") {
		t.Errorf("first SSE chunk = %q, want a hello event", buf[:n])
	}
}
