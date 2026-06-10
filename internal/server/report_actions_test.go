package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// strayReport returns a minimal manifest with the given project/id —
// used to plant report.json files at hostile locations.
func strayReport(project, id string) string {
	return `{"schema":"harness-deck/report@1","id":"` + id + `","project":"` + project + `",` +
		`"harness":"claude-code","title":"stray","status":"draft",` +
		`"created":"2026-06-10T00:00:00Z","blocks":[]}`
}

func TestDeleteRefusesSharedRootDir(t *testing.T) {
	// A report.json misplaced directly at the central reports root makes
	// its entry.Dir the WHOLE central dir. Deleting that entry must refuse
	// — RemoveAll would wipe every other report on the machine.
	isolateState(t)
	central := t.TempDir()
	if err := os.WriteFile(filepath.Join(central, "report.json"), []byte(strayReport("stray", "top")), 0o644); err != nil {
		t.Fatal(err)
	}
	innocent := filepath.Join(central, "acme", "run-1")
	if err := os.MkdirAll(innocent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(innocent, "report.json"), []byte(strayReport("acme", "run-1")), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _ := New(config.Config{CentralDir: central})

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/r/stray/top", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("delete of root-level report = %d, want 409", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(innocent, "report.json")); err != nil {
		t.Fatalf("innocent sibling report destroyed: %v", err)
	}
}

func TestDeleteRefusesDirContainingOtherReports(t *testing.T) {
	// entry.Dir must be a dedicated run dir. If another indexed report
	// lives anywhere beneath it, deleting would take that report with it.
	isolateState(t)
	central := t.TempDir()
	outer := filepath.Join(central, "acme", "run-outer")
	inner := filepath.Join(outer, "sub")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outer, "report.json"), []byte(strayReport("acme", "run-outer")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, "report.json"), []byte(strayReport("acme", "run-inner")), 0o644); err != nil {
		t.Fatal(err)
	}
	s, _ := New(config.Config{CentralDir: central})

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/r/acme/run-outer", nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("delete of dir holding another report = %d, want 409", rec.Code)
	}
	if _, err := os.Stat(filepath.Join(inner, "report.json")); err != nil {
		t.Fatalf("nested report destroyed: %v", err)
	}
}
