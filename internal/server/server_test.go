package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/push"
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

func TestContractEndpoint(t *testing.T) {
	h := newTestServer(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/contract.md", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /contract.md = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("Content-Type = %q, want text/markdown", ct)
	}
	if !strings.Contains(rec.Body.String(), "harness-deck/report@1") {
		t.Error("/contract.md body missing schema marker")
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

// TestReportsAPIExcludesSearchText confirms that the json:"-" tag on
// Entry.SearchText keeps it out of the /api/reports payload, so text
// precomputed for search is never shipped to the browser in listing calls.
func TestReportsAPIExcludesSearchText(t *testing.T) {
	// newTestServer uses sampleReport whose prose block contains "clear".
	code, body := get(t, newTestServer(t), "/api/reports")
	if code != http.StatusOK {
		t.Fatalf("GET /api/reports = %d, want 200", code)
	}
	if strings.Contains(body, "search_text") || strings.Contains(body, "SearchText") {
		t.Errorf("/api/reports must not expose the SearchText field; body: %s", body)
	}
	// The prose body "all **clear**." must not appear verbatim in the listing.
	if strings.Contains(body, "all **clear**") {
		t.Errorf("/api/reports must not include prose body in listing payload; body: %s", body)
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

// richReport carries every optional top-level field so the close handler's
// atomic rewrite is exercised against meaningful content to preserve.
const richReport = `{
  "schema": "harness-deck/report@1",
  "id": "rich1", "project": "acme", "harness": "claude-code",
  "agent": "claude-sonnet-4.5",
  "title": "complex report",
  "scope": "postgres",
  "kind": "audit",
  "status": "awaiting-review",
  "created": "2026-05-23T18:00:00Z",
  "verdict": "conditional-go",
  "meta": [{"key":"cost","value":"$1.84"},{"key":"scope","value":"14 services"}],
  "blocks": [
    {"type":"prose","markdown":"summary"},
    {"type":"metrics","metrics":[{"label":"queries","value":"312"}]}
  ]
}`

// writeRichReport drops richReport at central/acme/rich1/report.json and
// returns the run directory.
func writeRichReport(t *testing.T, central string) string {
	t.Helper()
	dir := filepath.Join(central, "acme", "rich1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(richReport), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// statusOf reads report.json and returns the top-level status string.
func statusOf(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	s, _ := got["status"].(string)
	return s
}

func TestCloseSetsStatusDone(t *testing.T) {
	isolateState(t)
	central := t.TempDir()
	dir := writeRichReport(t, central)
	s, err := New(config.Config{CentralDir: central})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r/acme/rich1/close", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("close = %d, want 200", rec.Code)
	}
	if got := statusOf(t, dir); got != "done" {
		t.Errorf("status = %q, want done", got)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	isolateState(t)
	central := t.TempDir()
	dir := writeRichReport(t, central)
	s, _ := New(config.Config{CentralDir: central})
	h := s.Handler()

	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r/acme/rich1/close", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("close pass %d = %d, want 200", i, rec.Code)
		}
	}
	if got := statusOf(t, dir); got != "done" {
		t.Errorf("status = %q, want done", got)
	}
}

func TestCloseAtomicWritePreservesFields(t *testing.T) {
	isolateState(t)
	central := t.TempDir()
	dir := writeRichReport(t, central)
	s, _ := New(config.Config{CentralDir: central})

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r/acme/rich1/close", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("close = %d, want 200", rec.Code)
	}

	data, _ := os.ReadFile(filepath.Join(dir, "report.json"))
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["verdict"] != "conditional-go" {
		t.Errorf("verdict lost: %v", got["verdict"])
	}
	if got["scope"] != "postgres" {
		t.Errorf("scope lost: %v", got["scope"])
	}
	if got["agent"] != "claude-sonnet-4.5" {
		t.Errorf("agent lost: %v", got["agent"])
	}
	if meta, ok := got["meta"].([]any); !ok || len(meta) != 2 {
		t.Errorf("meta lost: %v", got["meta"])
	}
	if blocks, ok := got["blocks"].([]any); !ok || len(blocks) != 2 {
		t.Errorf("blocks lost: %v", got["blocks"])
	}
}

func TestReopenSetsAwaitingReview(t *testing.T) {
	isolateState(t)
	central := t.TempDir()
	dir := writeRichReport(t, central)
	s, _ := New(config.Config{CentralDir: central})
	h := s.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r/acme/rich1/close", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("close = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r/acme/rich1/reopen", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("reopen = %d, want 200", rec.Code)
	}
	if got := statusOf(t, dir); got != "awaiting-review" {
		t.Errorf("status after reopen = %q, want awaiting-review", got)
	}
}

func TestCloseUnknownReport404(t *testing.T) {
	isolateState(t)
	s, _ := New(config.Config{CentralDir: t.TempDir()})

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/r/ghost/none/close", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("close unknown = %d, want 404", rec.Code)
	}
}

func TestDeleteRemovesRunDir(t *testing.T) {
	isolateState(t)
	central := t.TempDir()
	dir := writeRichReport(t, central)
	s, _ := New(config.Config{CentralDir: central})

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/r/acme/rich1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete = %d, want 200", rec.Code)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("run dir still exists after delete: %v", err)
	}
}

func TestDeleteUnknownReport404(t *testing.T) {
	isolateState(t)
	s, _ := New(config.Config{CentralDir: t.TempDir()})

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/r/ghost/none", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("delete unknown = %d, want 404", rec.Code)
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

// TestPushDisabledByDefault confirms the push endpoints refuse work when
// no VAPID keypair has been generated — the server stays healthy and the
// rest of the dashboard continues to work.
func TestPushDisabledByDefault(t *testing.T) {
	h := newTestServer(t)

	code, _ := get(t, h, "/api/push/vapid-key")
	if code != http.StatusServiceUnavailable {
		t.Errorf("vapid-key without keys = %d, want 503", code)
	}

	code, body := get(t, h, "/api/push/status")
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200", code)
	}
	if !strings.Contains(body, `"enabled":false`) {
		t.Errorf("status body = %s, want enabled:false", body)
	}
}

// TestPushSubscribeRoundTrip generates a real VAPID keypair, subscribes
// a fake browser, confirms it lands in subscriptions.json, then
// unsubscribes and confirms the file is empty.
func TestPushSubscribeRoundTrip(t *testing.T) {
	isolateState(t)

	// Generate and stash VAPID keys where the server will find them.
	central := t.TempDir()
	dir := filepath.Join(central, "acme", "0x4a2f")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(sampleReport), 0o644); err != nil {
		t.Fatal(err)
	}
	keysPath := filepath.Join(filepath.Dir(os.Getenv("HARNESS_DECK_CONFIG")), "vapid.json")
	if err := writeFakeVAPID(keysPath); err != nil {
		t.Fatal(err)
	}
	s, err := New(config.Config{CentralDir: central, Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()

	code, body := get(t, h, "/api/push/vapid-key")
	if code != http.StatusOK {
		t.Fatalf("vapid-key = %d, body=%s", code, body)
	}
	if !strings.Contains(body, `"key":"`) {
		t.Errorf("vapid-key body missing key: %s", body)
	}

	subBody := `{"endpoint":"https://push.example/abc","keys":{"p256dh":"BPx","auth":"AAAA"}}`
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/push/subscribe", strings.NewReader(subBody)))
	if rec.Code != http.StatusOK {
		t.Fatalf("subscribe = %d, body=%s", rec.Code, rec.Body.String())
	}

	code, body = get(t, h, "/api/push/status")
	if !strings.Contains(body, `"subscription_count":1`) {
		t.Errorf("after subscribe status = %s", body)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/push/unsubscribe",
		strings.NewReader(`{"endpoint":"https://push.example/abc"}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("unsubscribe = %d", rec.Code)
	}
	code, body = get(t, h, "/api/push/status")
	if !strings.Contains(body, `"subscription_count":0`) {
		t.Errorf("after unsubscribe status = %s", body)
	}
}

// TestSearchEndpoint covers /api/search across metadata + body content.
// The fixture report has a known title and prose body so we can assert
// both axes match and that the snippet wraps the hit.
func TestSearchEndpoint(t *testing.T) {
	h := newTestServer(t)

	// Empty query — endpoint returns 200 with an empty matches array.
	code, body := get(t, h, "/api/search?q=")
	if code != http.StatusOK {
		t.Fatalf("empty q: code=%d", code)
	}
	if !strings.Contains(body, `"matches":[]`) {
		t.Errorf("empty q body = %s", body)
	}

	// Metadata hit — the sample title is "readiness audit".
	code, body = get(t, h, "/api/search?q=readiness")
	if code != http.StatusOK {
		t.Fatalf("title q: code=%d", code)
	}
	if !strings.Contains(body, `"title":"readiness audit"`) {
		t.Errorf("title hit missing title; body = %s", body)
	}

	// Body hit — the sample prose markdown contains "clear" (in `all **clear**`).
	code, body = get(t, h, "/api/search?q=clear")
	if code != http.StatusOK {
		t.Fatalf("body q: code=%d", code)
	}
	if !strings.Contains(body, `"snippet":`) {
		t.Errorf("body hit missing snippet; body = %s", body)
	}
	if !strings.Contains(body, `[[clear]]`) {
		t.Errorf("snippet should bracket the match; body = %s", body)
	}

	// Miss — no garbage matches.
	code, body = get(t, h, "/api/search?q=nonexistent-string-xyz")
	if !strings.Contains(body, `"matches":[]`) {
		t.Errorf("expected empty matches; body = %s", body)
	}
}

// searchSeedReport is a single report fixture for the query-language search
// tests. Fields map onto the structural filter axes (status/project/kind/
// harness/created) plus a prose body so text terms have something to hit.
type searchSeedReport struct {
	project, run, title, status, kind, harness, created, body string
}

// seedSearchServer builds a server whose central dir holds the given reports,
// each at central/<project>/<run>/report.json. It returns the handler so the
// query-language tests can exercise /api/search and /api/search/schema against
// a known set.
func seedSearchServer(t *testing.T, reports []searchSeedReport) http.Handler {
	t.Helper()
	isolateState(t)
	central := t.TempDir()
	for _, r := range reports {
		dir := filepath.Join(central, r.project, r.run)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		manifest := fmt.Sprintf(`{
  "schema": "harness-deck/report@1",
  "id": %q, "project": %q, "harness": %q,
  "title": %q, "status": %q, "kind": %q,
  "created": %q,
  "blocks": [{"type": "prose", "markdown": %q}]
}`, r.run, r.project, r.harness, r.title, r.status, r.kind, r.created, r.body)
		if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(manifest), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := New(config.Config{CentralDir: central, Port: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.Handler()
}

// searchMatchesResponse decodes the /api/search payload for the query tests.
type searchMatchesResponse struct {
	Matches []struct {
		Project string `json:"project"`
		Run     string `json:"run"`
		Title   string `json:"title"`
		Status  string `json:"status"`
		Snippet string `json:"snippet"`
	} `json:"matches"`
	Error string `json:"error"`
}

// searchRuns runs ?q= and returns the matched runs (in response order) plus the
// error field, decoding through the typed response.
func searchRuns(t *testing.T, h http.Handler, rawQuery string) ([]string, string) {
	t.Helper()
	code, body := get(t, h, "/api/search?q="+url.QueryEscape(rawQuery))
	if code != http.StatusOK {
		t.Fatalf("GET /api/search?q=%q = %d, want 200; body=%s", rawQuery, code, body)
	}
	var resp searchMatchesResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode search response: %v; body=%s", err, body)
	}
	runs := make([]string, len(resp.Matches))
	for i, m := range resp.Matches {
		runs[i] = m.Run
	}
	return runs, resp.Error
}

// searchCorpus is the seeded set for the query-language tests: three reports
// spanning two projects, three statuses, two kinds, and distinct created dates
// so structural filters select known subsets.
var searchCorpus = []searchSeedReport{
	{project: "harness-deck", run: "r1", title: "readiness audit", status: "awaiting-review",
		kind: "audit", harness: "claude-code", created: "2026-06-10T12:00:00Z",
		body: "auth flow looks solid"},
	{project: "harness-deck", run: "r2", title: "dark mode plan", status: "done",
		kind: "roadmap", harness: "codex", created: "2026-05-01T12:00:00Z",
		body: "auth was untouched here"},
	{project: "demo", run: "r3", title: "login refactor", status: "draft",
		kind: "progress", harness: "claude-code", created: "2026-06-14T12:00:00Z",
		body: "no keyword overlap"},
}

// TestSearchStructuralOnly proves a purely-structural query (no text terms)
// selects the right set without ever needing body text, and that a list/IN
// query and a created comparison filter as specified.
func TestSearchStructuralOnly(t *testing.T) {
	h := seedSearchServer(t, searchCorpus)

	// status = awaiting-review → only r1.
	runs, errMsg := searchRuns(t, h, "status = awaiting-review")
	if errMsg != "" {
		t.Fatalf("unexpected error: %q", errMsg)
	}
	if !reflect.DeepEqual(runs, []string{"r1"}) {
		t.Errorf("status filter runs = %v, want [r1]", runs)
	}

	// project IN (harness-deck) AND kind = audit → r1 only.
	runs, _ = searchRuns(t, h, "project IN (harness-deck) AND kind = audit")
	if !reflect.DeepEqual(runs, []string{"r1"}) {
		t.Errorf("project/kind filter runs = %v, want [r1]", runs)
	}

	// created >= an absolute ISO date (clock-independent, unlike -Nd) keeps the
	// two June reports (r1, r3) and drops the May one (r2). Order is newest-first
	// for a structural-only query.
	runs, _ = searchRuns(t, h, "created >= 2026-06-01")
	if !reflect.DeepEqual(runs, []string{"r3", "r1"}) {
		t.Errorf("created filter runs = %v, want [r3 r1] (newest first)", runs)
	}
}

// TestSearchMixedTextAndFilter proves a query mixing a text term with a
// structural clause keeps only entries satisfying both, and that the survivor
// carries a snippet from the text term.
func TestSearchMixedTextAndFilter(t *testing.T) {
	h := seedSearchServer(t, searchCorpus)

	// "auth" appears in r1 and r2 bodies; status = awaiting-review keeps r1.
	runs, errMsg := searchRuns(t, h, "auth status = awaiting-review")
	if errMsg != "" {
		t.Fatalf("unexpected error: %q", errMsg)
	}
	if !reflect.DeepEqual(runs, []string{"r1"}) {
		t.Fatalf("mixed query runs = %v, want [r1]", runs)
	}

	_, body := get(t, h, "/api/search?q="+url.QueryEscape("auth status = awaiting-review"))
	if !strings.Contains(body, "[[auth]]") {
		t.Errorf("mixed query should snippet the text term; body=%s", body)
	}
}

// TestSearchParseError proves an invalid/partial query returns 200 with the
// error field set and an empty matches array (so the client keeps last-good
// results and surfaces the hint).
func TestSearchParseError(t *testing.T) {
	h := seedSearchServer(t, searchCorpus)

	code, body := get(t, h, "/api/search?q="+url.QueryEscape("status ="))
	if code != http.StatusOK {
		t.Fatalf("parse-error query = %d, want 200; body=%s", code, body)
	}
	var resp searchMatchesResponse
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v; body=%s", err, body)
	}
	if resp.Error == "" {
		t.Errorf("expected an error message; body=%s", body)
	}
	if len(resp.Matches) != 0 {
		t.Errorf("parse error should yield no matches; got %d", len(resp.Matches))
	}
}

// TestSearchPlainTextRegression proves a bare term with no operators behaves
// exactly as the legacy full-text search did: it matches metadata + body and
// brackets the hit in a snippet.
func TestSearchPlainTextRegression(t *testing.T) {
	h := seedSearchServer(t, searchCorpus)

	// "auth" is body-only in r1 and r2.
	runs, errMsg := searchRuns(t, h, "auth")
	if errMsg != "" {
		t.Fatalf("unexpected error: %q", errMsg)
	}
	got := map[string]bool{}
	for _, r := range runs {
		got[r] = true
	}
	if !got["r1"] || !got["r2"] || got["r3"] {
		t.Errorf("plain-text 'auth' runs = %v, want {r1,r2}", runs)
	}

	_, body := get(t, h, "/api/search?q=auth")
	if !strings.Contains(body, "[[auth]]") {
		t.Errorf("plain-text hit should bracket the match; body=%s", body)
	}

	// Title metadata still matches (legacy behavior): "readiness" hits r1.
	runs, _ = searchRuns(t, h, "readiness")
	if !reflect.DeepEqual(runs, []string{"r1"}) {
		t.Errorf("title term runs = %v, want [r1]", runs)
	}
}

// TestSearchSchemaEndpoint proves GET /api/search/schema returns the field/op
// matrix, the static status enum (in order), and the distinct project/kind/
// harness values present in the seeded index.
func TestSearchSchemaEndpoint(t *testing.T) {
	h := seedSearchServer(t, searchCorpus)

	code, body := get(t, h, "/api/search/schema")
	if code != http.StatusOK {
		t.Fatalf("GET /api/search/schema = %d, want 200", code)
	}

	var resp struct {
		Fields []struct {
			Name string   `json:"name"`
			Ops  []string `json:"ops"`
		} `json:"fields"`
		Values       map[string][]string `json:"values"`
		CreatedHints []string            `json:"created_hints"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode schema: %v; body=%s", err, body)
	}

	// Field matrix: status carries exactly its four operators in order.
	var statusOps []string
	for _, f := range resp.Fields {
		if f.Name == "status" {
			statusOps = f.Ops
		}
	}
	if !reflect.DeepEqual(statusOps, []string{"=", "!=", "IN", "NOT IN"}) {
		t.Errorf("status ops = %v, want [= != IN NOT IN]", statusOps)
	}
	if len(resp.Fields) != 8 {
		t.Errorf("schema fields = %d, want 8", len(resp.Fields))
	}

	// status values are the static enum in stable order.
	if !reflect.DeepEqual(resp.Values["status"], []string{"draft", "awaiting-review", "answered", "done"}) {
		t.Errorf("status values = %v, want the static enum", resp.Values["status"])
	}

	// project/kind/harness values are the distinct seeded values, sorted.
	if !reflect.DeepEqual(resp.Values["project"], []string{"demo", "harness-deck"}) {
		t.Errorf("project values = %v, want [demo harness-deck]", resp.Values["project"])
	}
	if !reflect.DeepEqual(resp.Values["harness"], []string{"claude-code", "codex"}) {
		t.Errorf("harness values = %v, want [claude-code codex]", resp.Values["harness"])
	}
	if !reflect.DeepEqual(resp.Values["kind"], []string{"audit", "progress", "roadmap"}) {
		t.Errorf("kind values = %v, want [audit progress roadmap]", resp.Values["kind"])
	}

	if !reflect.DeepEqual(resp.CreatedHints, []string{"-24h", "-7d", "-2w", "YYYY-MM-DD"}) {
		t.Errorf("created_hints = %v", resp.CreatedHints)
	}
}

// TestReportSigEndpoint covers the live-reload fingerprint endpoint:
// existing report returns sig + status, unknown report returns
// {exists:false} so the page can redirect to /.
func TestReportSigEndpoint(t *testing.T) {
	h := newTestServer(t)

	code, body := get(t, h, "/r/acme/0x4a2f/sig")
	if code != http.StatusOK {
		t.Fatalf("sig = %d, body=%s", code, body)
	}
	for _, want := range []string{`"exists":true`, `"sig":"`, `"archived":false`, `"status":"awaiting-review"`} {
		if !strings.Contains(body, want) {
			t.Errorf("sig body missing %q; body=%s", want, body)
		}
	}

	code, body = get(t, h, "/r/acme/does-not-exist/sig")
	if code != http.StatusOK {
		t.Fatalf("missing-report sig = %d, body=%s", code, body)
	}
	if !strings.Contains(body, `"exists":false`) {
		t.Errorf("missing-report body = %s", body)
	}
}

// writeFakeVAPID drops a fresh VAPID keypair on disk for tests so the
// push endpoints become reachable.
func writeFakeVAPID(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	keys, err := push.Generate()
	if err != nil {
		return err
	}
	return keys.Save(path)
}
