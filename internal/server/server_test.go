package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
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

// isolateState points the projects-state file at a temp dir so tests never
// touch the real ~/.config/harness-deck/projects.json.
func isolateState(t *testing.T) {
	t.Helper()
	t.Setenv("HARNESS_DECK_CONFIG", filepath.Join(t.TempDir(), "config.json"))
}

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	isolateState(t)
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
	isolateState(t)
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

// projectsResponse mirrors the /api/projects payload for test decoding.
type projectsResponse struct {
	Projects []struct {
		Project          string `json:"project"`
		CurrentStateHTML string `json:"current_state_html"`
		HasState         bool   `json:"has_state"`
		RoadmapHTML      string `json:"roadmap_html"`
		HasRoadmap       bool   `json:"has_roadmap"`
	} `json:"projects"`
	Discovered []struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	} `json:"discovered"`
}

// mkAIDoc writes a .docs/ai doc for a project under a scan root.
func mkAIDoc(t *testing.T, scanRoot, project, doc, body string) {
	t.Helper()
	aiDir := filepath.Join(scanRoot, project, ".docs", "ai")
	if err := os.MkdirAll(aiDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aiDir, doc), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectsViewRendersRoadmapAndState(t *testing.T) {
	isolateState(t)
	gitDir := t.TempDir()
	mkAIDoc(t, gitDir, "larkline", "roadmap.md", "# Plan\n\n- ship dark mode")
	mkAIDoc(t, gitDir, "larkline", "current-state.md", "## State\n\nbuild is green")

	s, err := New(config.Config{CentralDir: t.TempDir(), ScanRoots: []string{gitDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	code, body := get(t, s.Handler(), "/api/projects")
	if code != http.StatusOK {
		t.Fatalf("GET /api/projects = %d, want 200", code)
	}
	var got projectsResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	if len(got.Projects) != 1 {
		t.Fatalf("got %d projects, want 1; body: %s", len(got.Projects), body)
	}
	p := got.Projects[0]
	if p.Project != "larkline" {
		t.Errorf("project = %q, want larkline", p.Project)
	}
	if !p.HasRoadmap || !strings.Contains(p.RoadmapHTML, "<h1>Plan</h1>") {
		t.Errorf("roadmap not rendered: has=%v html=%q", p.HasRoadmap, p.RoadmapHTML)
	}
	if !p.HasState || !strings.Contains(p.CurrentStateHTML, "build is green") {
		t.Errorf("current-state not rendered: has=%v html=%q", p.HasState, p.CurrentStateHTML)
	}
	if len(got.Discovered) != 1 || got.Discovered[0].Name != "larkline" || !got.Discovered[0].Enabled {
		t.Errorf("discovered = %+v, want [{larkline true}]", got.Discovered)
	}
}

func TestProjectToggleHidesProject(t *testing.T) {
	isolateState(t)
	gitDir := t.TempDir()
	mkAIDoc(t, gitDir, "larkline", "roadmap.md", "# Plan")

	s, err := New(config.Config{CentralDir: t.TempDir(), ScanRoots: []string{gitDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := s.Handler()

	post := func(body string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/projects/toggle",
			strings.NewReader(body)))
		return rec.Code
	}

	if code := post(`{"name":"larkline"}`); code != http.StatusOK {
		t.Fatalf("POST toggle = %d, want 200", code)
	}

	_, body := get(t, h, "/api/projects")
	var got projectsResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Projects) != 0 {
		t.Errorf("hidden project still in projects list: %+v", got.Projects)
	}
	if len(got.Discovered) != 1 || got.Discovered[0].Enabled {
		t.Errorf("discovered = %+v, want larkline enabled=false", got.Discovered)
	}

	if code := post(`{"name":"ghost"}`); code != http.StatusBadRequest {
		t.Errorf("toggle of unknown project = %d, want 400", code)
	}
}

func TestProjectsReorderUpdatesOrder(t *testing.T) {
	isolateState(t)
	gitDir := t.TempDir()
	mkAIDoc(t, gitDir, "alpha", "roadmap.md", "# a")
	mkAIDoc(t, gitDir, "beta", "roadmap.md", "# b")
	mkAIDoc(t, gitDir, "gamma", "roadmap.md", "# g")

	s, err := New(config.Config{CentralDir: t.TempDir(), ScanRoots: []string{gitDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/projects/reorder",
		strings.NewReader(`{"order":["gamma","alpha","beta"]}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST reorder = %d, want 200", rec.Code)
	}

	_, body := get(t, h, "/api/projects")
	var got projectsResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var names []string
	for _, p := range got.Discovered {
		names = append(names, p.Name)
	}
	if want := []string{"gamma", "alpha", "beta"}; !reflect.DeepEqual(names, want) {
		t.Errorf("discovered order = %v, want %v", names, want)
	}
	var viewNames []string
	for _, p := range got.Projects {
		viewNames = append(viewNames, p.Project)
	}
	if want := []string{"gamma", "alpha", "beta"}; !reflect.DeepEqual(viewNames, want) {
		t.Errorf("projects view order = %v, want %v", viewNames, want)
	}
}

func TestProjectsReorderRejectsUnknownNames(t *testing.T) {
	isolateState(t)
	gitDir := t.TempDir()
	mkAIDoc(t, gitDir, "alpha", "roadmap.md", "# a")

	s, err := New(config.Config{CentralDir: t.TempDir(), ScanRoots: []string{gitDir}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/projects/reorder",
		strings.NewReader(`{"order":["alpha","ghost"]}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("reorder with ghost = %d, want 400", rec.Code)
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
