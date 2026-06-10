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

// postJSON drives a handler with a JSON body and returns status + body.
func postJSON(t *testing.T, h http.Handler, path, body string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestNotificationsAddRefusesCorruptConfig(t *testing.T) {
	// A hand-edit typo in config.json must make the save error out — not
	// silently rewrite the user's config down to the one key the handler
	// owns (dropping bind, scan_roots, tls, …).
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	corrupt := `{"central_dir":"/x","bind":"0.0.0.0",}` // trailing comma
	if err := os.WriteFile(cfgPath, []byte(corrupt), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	s, err := New(config.Config{CentralDir: t.TempDir(), Port: 0})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	code, _ := postJSON(t, s.Handler(), "/api/notifications",
		`{"name":"team","type":"webhook","url":"https://example.com/hook"}`)
	if code != http.StatusInternalServerError {
		t.Errorf("POST with corrupt config = %d, want 500", code)
	}
	data, _ := os.ReadFile(cfgPath)
	if string(data) != corrupt {
		t.Errorf("corrupt config was clobbered:\n%s", data)
	}
}
