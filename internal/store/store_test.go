package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

func writeReport(t *testing.T, dir, id, project, status string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rep := fmt.Sprintf(`{"schema":"harness-deck/report@1","id":%q,"project":%q,
	  "harness":"claude-code","title":"t","status":%q,
	  "created":"2026-05-18T18:39:50Z","blocks":[{"type":"prose","markdown":"x"}]}`,
		id, project, status)
	if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(rep), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanFindsCentralAndProjectReports(t *testing.T) {
	central := t.TempDir()
	proj := t.TempDir()
	writeReport(t, filepath.Join(central, "acme", "r1"), "r1", "acme", "awaiting-review")
	writeReport(t, filepath.Join(proj, ".harness", "r2"), "r2", "myproj", "done")

	s := New(config.Config{CentralDir: central})
	s.Scan([]string{proj})

	entries := s.Entries()
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	sources := map[string]string{}
	for _, e := range entries {
		sources[e.Run] = e.Source
	}
	if sources["r1"] != "central" || sources["r2"] != "project" {
		t.Errorf("sources = %v, want r1=central r2=project", sources)
	}

	rep, entry, err := s.Get("myproj", "r2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if rep.Title != "t" || entry.Status != "done" {
		t.Errorf("Get returned title=%q status=%q", rep.Title, entry.Status)
	}
}

// TestScanUsesProjectRootsArgument checks that Scan indexes the project
// roots it is given and no longer reads cfg.Projects itself — project
// selection now lives upstream (the enabled set from the projects package).
func TestScanUsesProjectRootsArgument(t *testing.T) {
	central := t.TempDir()
	argRoot := t.TempDir() // passed to Scan — should be indexed
	cfgRoot := t.TempDir() // in cfg.Projects — should be ignored
	writeReport(t, filepath.Join(argRoot, ".harness", "a1"), "a1", "argproj", "done")
	writeReport(t, filepath.Join(cfgRoot, ".harness", "c1"), "c1", "cfgproj", "done")

	s := New(config.Config{CentralDir: central, Projects: []string{cfgRoot}})
	s.Scan([]string{argRoot})

	runs := map[string]bool{}
	for _, e := range s.Entries() {
		runs[e.Run] = true
	}
	if !runs["a1"] {
		t.Error("report under the Scan argument root was not indexed")
	}
	if runs["c1"] {
		t.Error("report under cfg.Projects was indexed; Scan should use its argument")
	}
}

func TestSignatureChangesWhenReportsChange(t *testing.T) {
	central := t.TempDir()
	writeReport(t, filepath.Join(central, "p", "r1"), "r1", "p", "draft")
	s := New(config.Config{CentralDir: central})
	s.Scan(nil)
	before := s.Signature()

	writeReport(t, filepath.Join(central, "p", "r2"), "r2", "p", "draft")
	s.Scan(nil)
	if s.Signature() == before {
		t.Error("signature should change after a new report appears")
	}
}

func TestScanRecordsParseErrors(t *testing.T) {
	central := t.TempDir()
	bad := filepath.Join(central, "broken", "r1")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "report.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(config.Config{CentralDir: central})
	s.Scan(nil)

	if len(s.Entries()) != 0 {
		t.Errorf("a malformed report should not produce an entry")
	}
	if len(s.Errors()) != 1 {
		t.Fatalf("got %d scan errors, want 1", len(s.Errors()))
	}
}
