package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// benchReports populates dir with n report run directories (half also carry a
// responses.json) for the scan benchmarks. Each manifest carries a realistic
// few-KB markdown body so the benchmark reflects real parse cost, not a
// degenerate one-line report.
func benchReports(b *testing.B, dir string, n int) {
	b.Helper()
	body := strings.Repeat("Lorem ipsum **dolor** sit amet, `consectetur` adipiscing elit. ", 40)
	for i := 0; i < n; i++ {
		d := filepath.Join(dir, fmt.Sprintf("proj%d", i%10), fmt.Sprintf("run%d", i))
		if err := os.MkdirAll(d, 0o755); err != nil {
			b.Fatal(err)
		}
		rep := fmt.Sprintf(`{"schema":"harness-deck/report@1","id":"run%d","project":"proj%d","harness":"h","title":"t","status":"awaiting-review","created":"2026-05-18T18:39:50Z","blocks":[{"type":"prose","title":"summary","markdown":%q},{"type":"prose","title":"detail","markdown":%q},{"type":"recommendations","items":[{"id":"r1","markdown":"do a thing"},{"id":"r2","markdown":"do another"}]},{"type":"ask","id":"a1","prompt":"ok?","mode":"yesno"}]}`, i, i%10, body, body)
		if err := os.WriteFile(filepath.Join(d, "report.json"), []byte(rep), 0o644); err != nil {
			b.Fatal(err)
		}
		if i%2 == 0 {
			_ = os.WriteFile(filepath.Join(d, "responses.json"), []byte(`{"run":"x","project":"y","responses":{}}`), 0o644)
		}
	}
}

// BenchmarkScanCold is a scan with an empty cache — every report is read and
// parsed. This is the first-scan cost and the pre-incremental-cache behavior.
func BenchmarkScanCold(b *testing.B) {
	dir := b.TempDir()
	benchReports(b, dir, 500)
	cfg := config.Config{CentralDir: dir}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		New(cfg).Scan(nil) // fresh store ⇒ empty cache ⇒ full re-parse
	}
}

// BenchmarkScanWarm is a repeated scan of an unchanged report set — the common
// watcher tick. After the first scan the mtime-keyed cache serves every entry,
// so only the walk + stats run.
func BenchmarkScanWarm(b *testing.B) {
	dir := b.TempDir()
	benchReports(b, dir, 500)
	s := New(config.Config{CentralDir: dir})
	s.Scan(nil) // warm the cache
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Scan(nil)
	}
}

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

// writeAskReport writes a report carrying one unanswered yes/no ask at the
// given status — used to exercise the OpenAsks counter.
func writeAskReport(t *testing.T, dir, id, project, status string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	rep := fmt.Sprintf(`{"schema":"harness-deck/report@1","id":%q,"project":%q,
	  "harness":"claude-code","title":"t","status":%q,
	  "created":"2026-05-18T18:39:50Z",
	  "blocks":[{"type":"ask","id":"a1","prompt":"ok?","mode":"yesno"}]}`,
		id, project, status)
	if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(rep), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestScanDraftReportSuppressesOpenAsks guards the lifecycle rule: a draft
// report's interactive blocks do not count as open asks (so they neither
// surface in the inbox/projects/MCP counters nor fire a push), while the same
// report at awaiting-review does count.
func TestScanDraftReportSuppressesOpenAsks(t *testing.T) {
	central := t.TempDir()
	writeAskReport(t, filepath.Join(central, "draft-r"), "draft-r", "demo", "draft")
	writeAskReport(t, filepath.Join(central, "live-r"), "live-r", "demo", "awaiting-review")

	s := New(config.Config{CentralDir: central})
	s.Scan(nil)

	got := map[string]int{}
	for _, e := range s.Entries() {
		got[e.Run] = e.OpenAsks
	}
	if got["draft-r"] != 0 {
		t.Errorf("draft report OpenAsks = %d, want 0 (draft must not surface asks)", got["draft-r"])
	}
	if got["live-r"] != 1 {
		t.Errorf("awaiting-review report OpenAsks = %d, want 1", got["live-r"])
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

// TestScanReportsProjectRunCollision checks that two reports mapping to the
// same (project,run) key from different files surface the collision in
// Errors() rather than the second silently shadowing the first.
func TestScanReportsProjectRunCollision(t *testing.T) {
	central := t.TempDir()
	// Same project + id, two distinct run directories => same key,
	// different Path.
	writeReport(t, filepath.Join(central, "dup-a"), "r1", "acme", "done")
	writeReport(t, filepath.Join(central, "dup-b"), "r1", "acme", "done")

	s := New(config.Config{CentralDir: central})
	s.Scan(nil)

	if len(s.Entries()) != 1 {
		t.Fatalf("entries = %d, want 1 (first wins, second shadowed)", len(s.Entries()))
	}
	found := false
	for _, e := range s.Errors() {
		if strings.Contains(e, "duplicate (project,run)") && strings.Contains(e, "acme/r1") {
			found = true
		}
	}
	if !found {
		t.Errorf("collision not surfaced in Errors(): %v", s.Errors())
	}
}

// TestScanSerializesConcurrentCalls runs many overlapping Scan calls against
// the same store, concurrent with Entries()/Signature() reads. scanMu must
// serialize each scan's walk+commit so the final committed index is a complete
// one (all 8 reports), and -race must find no data race between the
// concurrent scans and the index reads.
func TestScanSerializesConcurrentCalls(t *testing.T) {
	central := t.TempDir()
	for i := 0; i < 8; i++ {
		writeReport(t, filepath.Join(central, fmt.Sprintf("p%d", i)), fmt.Sprintf("r%d", i), "proj", "done")
	}
	s := New(config.Config{CentralDir: central})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Scan(nil)
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Entries()
			_ = s.Signature()
			_ = s.Errors()
		}()
	}
	wg.Wait()

	// A complete scan indexes all 8 reports; a clobbered/partial commit
	// would leave fewer.
	if got := len(s.Entries()); got != 8 {
		t.Fatalf("entries = %d, want 8 (a partial walk clobbered a complete index)", got)
	}
}

func TestScanSurfacesCorruptResponsesWithoutDroppingReport(t *testing.T) {
	// A corrupt responses.json must not silently flip answered asks back
	// to open: the report stays indexed (graceful degradation), but the
	// problem surfaces in Errors() instead of being swallowed.
	central := t.TempDir()
	dir := filepath.Join(central, "acme", "run-1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `{"schema":"harness-deck/report@1","id":"run-1","project":"acme",` +
		`"harness":"claude-code","title":"t","status":"awaiting-review",` +
		`"created":"2026-06-10T00:00:00Z",` +
		`"blocks":[{"type":"ask","id":"a1","prompt":"ok?","mode":"yesno"}]}`
	if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "responses.json"), []byte(`{"resp`), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(config.Config{CentralDir: central})
	s.Scan(nil)

	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1 (report must stay indexed)", len(entries))
	}
	found := false
	for _, e := range s.Errors() {
		if strings.Contains(e, "responses.json") {
			found = true
		}
	}
	if !found {
		t.Errorf("corrupt responses.json not surfaced in Errors(): %v", s.Errors())
	}
}

// TestScanCacheHitServesStaleUntilMtimeChanges proves that the mtime-keyed
// cache reuses a prior Entry when report.json's mtime is unchanged (cache hit)
// and re-parses only after the mtime advances (cache invalidation).
func TestScanCacheHitServesStaleUntilMtimeChanges(t *testing.T) {
	central := t.TempDir()
	dir := filepath.Join(central, "proj", "r1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(dir, "report.json")

	// Write the initial report with title "old-title".
	v1 := `{"schema":"harness-deck/report@1","id":"r1","project":"proj",` +
		`"harness":"claude-code","title":"old-title","status":"awaiting-review",` +
		`"created":"2026-05-18T18:39:50Z","blocks":[{"type":"prose","markdown":"x"}]}`
	if err := os.WriteFile(reportPath, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	// Warm the cache.
	s := New(config.Config{CentralDir: central})
	s.Scan(nil)

	// Capture the mtime the cache keyed on.
	fi, err := os.Stat(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	originalMtime := fi.ModTime()

	// Overwrite with a different title but pin the mtime back to the original.
	v2 := `{"schema":"harness-deck/report@1","id":"r1","project":"proj",` +
		`"harness":"claude-code","title":"new-title","status":"awaiting-review",` +
		`"created":"2026-05-18T18:39:50Z","blocks":[{"type":"prose","markdown":"x"}]}`
	if err := os.WriteFile(reportPath, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(reportPath, originalMtime, originalMtime); err != nil {
		t.Fatal(err)
	}

	// Scan: mtime unchanged → cache hit → must still show old-title.
	s.Scan(nil)
	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Title != "old-title" {
		t.Errorf("cache hit: title = %q, want %q (cache must serve the prior parse)", entries[0].Title, "old-title")
	}

	// Advance the mtime so the cache key changes → must re-parse and see new-title.
	future := originalMtime.Add(2 * time.Second)
	if err := os.Chtimes(reportPath, future, future); err != nil {
		t.Fatal(err)
	}

	s.Scan(nil)
	entries = s.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Title != "new-title" {
		t.Errorf("cache invalidation: title = %q, want %q (mtime change must trigger re-parse)", entries[0].Title, "new-title")
	}
}

// TestScanCacheInvalidatesOnResponsesChange proves that when responses.json is
// written (answering an ask), the next Scan re-parses the entry (because the
// responses-mtime component of the cache key changed) and returns OpenAsks==0.
func TestScanCacheInvalidatesOnResponsesChange(t *testing.T) {
	central := t.TempDir()
	dir := filepath.Join(central, "proj", "r1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(dir, "report.json")
	responsesPath := filepath.Join(dir, "responses.json")

	// Write a report with one unanswered ask at awaiting-review (not draft,
	// so OpenAsks is counted).
	report := `{"schema":"harness-deck/report@1","id":"r1","project":"proj",` +
		`"harness":"claude-code","title":"t","status":"awaiting-review",` +
		`"created":"2026-05-18T18:39:50Z",` +
		`"blocks":[{"type":"ask","id":"a1","prompt":"ok?","mode":"yesno"}]}`
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	// Warm the cache; the ask must be open.
	s := New(config.Config{CentralDir: central})
	s.Scan(nil)
	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].OpenAsks != 1 {
		t.Fatalf("before response: OpenAsks = %d, want 1", entries[0].OpenAsks)
	}

	// Write responses.json answering the ask (bumps its mtime).
	resp := `{"run":"r1","project":"proj","updated":"2026-05-18T19:00:00Z",` +
		`"responses":{"a1":{"block":"a1","value":"yes","at":"2026-05-18T19:00:00Z"}}}`
	if err := os.WriteFile(responsesPath, []byte(resp), 0o644); err != nil {
		t.Fatal(err)
	}

	// Scan again; the responses-mtime changed so the cache is invalid and the
	// entry is re-parsed — OpenAsks must now be 0.
	s.Scan(nil)
	entries = s.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].OpenAsks != 0 {
		t.Errorf("after response: OpenAsks = %d, want 0 (cache must re-parse on responses change)", entries[0].OpenAsks)
	}
}

// TestScanPopulatesSearchText proves that loadEntry sets SearchText from the
// report's body blocks, and that a second scan with the file unchanged still
// carries the same SearchText (cache-hit reuse).
func TestScanPopulatesSearchText(t *testing.T) {
	central := t.TempDir()
	dir := filepath.Join(central, "proj", "r1")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `{"schema":"harness-deck/report@1","id":"r1","project":"proj",` +
		`"harness":"claude-code","title":"t","status":"awaiting-review",` +
		`"created":"2026-05-18T18:39:50Z",` +
		`"blocks":[{"type":"prose","markdown":"unique body content xyz"}]}`
	if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(config.Config{CentralDir: central})
	s.Scan(nil)

	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if !strings.Contains(entries[0].SearchText, "unique body content xyz") {
		t.Errorf("SearchText = %q, want it to contain the prose body", entries[0].SearchText)
	}

	// Second scan with unchanged file: cache hit must still carry SearchText.
	s.Scan(nil)
	entries = s.Entries()
	if len(entries) != 1 {
		t.Fatalf("after second scan: entries = %d, want 1", len(entries))
	}
	if !strings.Contains(entries[0].SearchText, "unique body content xyz") {
		t.Errorf("cache-hit SearchText = %q, want body text preserved", entries[0].SearchText)
	}
}

// TestScanDeletedReportDropsFromCache proves that a run directory removed
// between scans is not served from the stale cache — the deleted report must
// vanish from Entries() on the next scan.
func TestScanDeletedReportDropsFromCache(t *testing.T) {
	central := t.TempDir()
	dir1 := filepath.Join(central, "proj", "r1")
	dir2 := filepath.Join(central, "proj", "r2")
	writeReport(t, dir1, "r1", "proj", "done")
	writeReport(t, dir2, "r2", "proj", "done")

	// Warm: both reports indexed.
	s := New(config.Config{CentralDir: central})
	s.Scan(nil)
	if got := len(s.Entries()); got != 2 {
		t.Fatalf("after warm: entries = %d, want 2", got)
	}

	// Delete one run directory.
	if err := os.RemoveAll(dir2); err != nil {
		t.Fatal(err)
	}

	// Scan again: the deleted report must not appear (the cache must not serve
	// a path that no longer exists on disk).
	s.Scan(nil)
	entries := s.Entries()
	if len(entries) != 1 {
		t.Fatalf("after delete: entries = %d, want 1", len(entries))
	}
	if entries[0].Run != "r1" {
		t.Errorf("after delete: remaining run = %q, want %q", entries[0].Run, "r1")
	}
}
