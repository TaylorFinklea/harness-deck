package render

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/manifest"
)

// repoRoot returns the repository root by walking up from this file's location.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file is …/internal/render/registry_crosscheck_test.go; root is two dirs up.
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// TestRegistryCrossCheck asserts that every registered block type is wired up
// in all required places. The test must FAIL when a registry entry is added
// without the accompanying template, defaultTitle, CONTRACT.md row, and
// manifest helper coverage.
//
// Places checked per block type:
//  1. internal/manifest registry (source — iterated via manifest.Types())
//  2. block-<type> template in internal/render/templates/
//  3. defaultTitles entry in render.go (required for non-interactive types;
//     interactive types are exempt — they have titles baked into panel heads)
//  4. CONTRACT.md block-type table row
//  5. manifest.BlockPrompt handles every interactive type (non-empty return)
//  6. manifest.BlockText includes every textual block type (non-empty return)
func TestRegistryCrossCheck(t *testing.T) {
	contractBytes, err := os.ReadFile(filepath.Join(repoRoot(t), "CONTRACT.md"))
	if err != nil {
		t.Fatalf("read CONTRACT.md: %v", err)
	}
	contractMD := string(contractBytes)

	r, err := New()
	if err != nil {
		t.Fatalf("New renderer: %v", err)
	}

	for _, typ := range manifest.Types() {
		typ := typ // capture

		t.Run(typ, func(t *testing.T) {
			// 1. Template must exist.
			tmplName := "block-" + typ
			if r.tmpl.Lookup(tmplName) == nil {
				t.Errorf("missing template %q — add {{define %q}} to templates/blocks.tmpl", tmplName, tmplName)
			}

			// 2. Non-interactive types must have a defaultTitles entry.
			//    Interactive types are allowed to be absent (they carry their
			//    own panel-head copy in the template).
			if !manifest.InteractiveTypes[typ] {
				if _, ok := defaultTitles[typ]; !ok {
					t.Errorf("missing defaultTitles entry for %q in render.go", typ)
				}
			}

			// 3. CONTRACT.md must mention the type as a block row.
			//    The contract table uses `| `type` |` formatting.
			rowMarker := "| `" + typ + "`"
			if !strings.Contains(contractMD, rowMarker) {
				t.Errorf("CONTRACT.md missing block-table row for %q (expected %q)", typ, rowMarker)
			}
		})
	}

	// 4. manifest.BlockPrompt must return a non-empty string for every
	//    interactive block type. An empty return would silently produce
	//    blank push notifications and fanout messages.
	for _, typ := range manifest.Types() {
		if !manifest.InteractiveTypes[typ] {
			continue
		}
		b := syntheticInteractiveBlock(typ)
		got := manifest.BlockPrompt(b)
		if got == "" {
			t.Errorf("manifest.BlockPrompt returned empty for interactive type %q", typ)
		}
	}

	// 5. manifest.BlockText must return a non-empty string for a report that
	//    contains one block of every textual type (those with searchable
	//    prose content). The types covered by BlockText are a subset —
	//    metric/diff/barchart/table/compare/html are intentionally skipped.
	//    We assert the covered types are non-empty and that the function
	//    handles ALL registered types without panicking.
	textualTypes := []string{
		manifest.TypeProse, manifest.TypeRecommendations, manifest.TypeCallout,
		manifest.TypeTimeline, manifest.TypeAsk, manifest.TypeDecision,
		manifest.TypeApproval, manifest.TypeCardGrid,
	}
	for _, typ := range textualTypes {
		rep := syntheticReportWithBlock(typ)
		got := manifest.BlockText(rep)
		if got == "" {
			t.Errorf("manifest.BlockText returned empty for textual type %q", typ)
		}
	}

	// Smoke: every registered type must not panic in BlockText.
	for _, typ := range manifest.Types() {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("manifest.BlockText panicked for type %q: %v", typ, r)
				}
			}()
			manifest.BlockText(syntheticReportWithBlock(typ))
		}()
	}
}

// syntheticInteractiveBlock builds a minimal parsed Block of the given
// interactive type with a non-empty Prompt field so BlockPrompt has something
// to return. The JSON is constructed per known interactive block shapes.
func syntheticInteractiveBlock(typ string) manifest.Block {
	var raw string
	switch typ {
	case manifest.TypeAsk:
		raw = `{"type":"ask","id":"test","prompt":"Is this ok?"}`
	case manifest.TypeDecision:
		raw = `{"type":"decision","id":"test","prompt":"Which path?","a":{"tag":"A","title":"Alpha","items":[]},"b":{"tag":"B","title":"Beta","items":[]}}`
	case manifest.TypeApproval:
		raw = `{"type":"approval","id":"test","prompt":"Approve this?"}`
	default:
		raw = `{"type":"` + typ + `","id":"test","prompt":"prompt?"}`
	}
	rep, _ := manifest.Parse([]byte(`{"schema":"harness-deck/report@1","id":"x","project":"p","harness":"h","title":"t","status":"draft","created":"2026-01-01T00:00:00Z","blocks":[` + raw + `]}`))
	if rep == nil || len(rep.Blocks) == 0 {
		return manifest.Block{Type: typ}
	}
	return rep.Blocks[0]
}

// syntheticReportWithBlock builds a minimal Report containing one block of the
// given type, with enough content that BlockText should return a non-empty string.
func syntheticReportWithBlock(typ string) *manifest.Report {
	var blockJSON string
	switch typ {
	case manifest.TypeProse:
		blockJSON = `{"type":"prose","markdown":"hello world"}`
	case manifest.TypeRecommendations:
		blockJSON = `{"type":"recommendations","items":[{"markdown":"do this"}]}`
	case manifest.TypeCallout:
		blockJSON = `{"type":"callout","level":"info","markdown":"watch out"}`
	case manifest.TypeTimeline:
		blockJSON = `{"type":"timeline","events":[{"time":"now","markdown":"it happened"}]}`
	case manifest.TypeAsk:
		blockJSON = `{"type":"ask","id":"q1","prompt":"Ship it?","options":["yes","no"]}`
	case manifest.TypeDecision:
		blockJSON = `{"type":"decision","id":"d1","prompt":"Which?","a":{"tag":"A","title":"Alpha","items":[]},"b":{"tag":"B","title":"Beta","items":[]}}`
	case manifest.TypeApproval:
		blockJSON = `{"type":"approval","id":"a1","prompt":"Approve?"}`
	case manifest.TypeCardGrid:
		blockJSON = `{"type":"card-grid","cards":[{"title":"Alpha","markdown":"first card"},{"title":"Beta"}]}`
	default:
		// For types without text content, just build a valid block.
		blockJSON = `{"type":"` + typ + `"}`
	}
	src := `{"schema":"harness-deck/report@1","id":"x","project":"p","harness":"h","title":"t","status":"draft","created":"2026-01-01T00:00:00Z","blocks":[` + blockJSON + `]}`
	rep, _ := manifest.Parse([]byte(src))
	if rep == nil {
		return &manifest.Report{}
	}
	return rep
}
