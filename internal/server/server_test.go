package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
