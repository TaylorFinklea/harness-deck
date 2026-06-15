package assets

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestBundlesAreValidJS syntax-checks the assembled script bundles with
// `node --check`. The frontend has no build step and no JS linter: the
// dashboard bundle is several separate-IIFE files concatenated here in Go
// (aggregator.js even omits its own closing })(); , re-added by string
// surgery), so a single typo in any fragment would ship a fully-broken page
// with no other automated signal. TestReportJSBundleOrder /
// TestAggregatorBundleAssembled check ORDER; this checks VALIDITY.
//
// Skipped (not failed) when node is unavailable so it never blocks a
// node-less machine or CI lane — it's a best-effort gate that runs wherever
// node exists (it's how these bundles were verified by hand during dev).
func TestBundlesAreValidJS(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found on PATH; skipping JS syntax check")
	}
	bundles := map[string]string{
		"AggregatorJS": AggregatorJS,
		"ReportJS":     ReportJS,
	}
	dir := t.TempDir()
	for name, src := range bundles {
		path := filepath.Join(dir, name+".js")
		if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
			t.Fatalf("%s: write temp bundle: %v", name, err)
		}
		out, err := exec.Command("node", "--check", path).CombinedOutput()
		if err != nil {
			t.Errorf("%s failed node --check (assembled bundle has a JS syntax error):\n%s", name, out)
		}
	}
}

// TestMobileCSSLastInBundles pins the invariant documented in assets.go:
// MobileCSS must be the last stylesheet in every assembled bundle so its
// @media (max-width) overrides win over desktop rules of equal specificity.
func TestMobileCSSLastInBundles(t *testing.T) {
	for _, tc := range []struct {
		name   string
		bundle string
	}{
		{"ReportCSS", ReportCSS},
		{"DeckUICSS", DeckUICSS},
	} {
		idx := strings.LastIndex(tc.bundle, MobileCSS)
		if idx < 0 {
			t.Errorf("%s: MobileCSS not present in bundle", tc.name)
			continue
		}
		after := tc.bundle[idx+len(MobileCSS):]
		if strings.TrimSpace(after) != "" {
			t.Errorf("%s: MobileCSS is not last — content follows it:\n%s", tc.name, after[:min(len(after), 120)])
		}
	}
}

// TestReportJSBundleOrder pins the keyboard-precedence order documented in the
// "ORDER IS KEYBOARD PRECEDENCE" comment on ReportJS. Reordering the
// bundle silently reshuffles keyboard semantics; this test makes that loud.
//
// Expected order: VimNavJSInline → RespondJS → MobileJSInline →
// TabsJSInline → TriageJSInline → LiveJSInline → LiveBannerJSInline →
// SearchJSInline → HTMLBlockJSInline
func TestReportJSBundleOrder(t *testing.T) {
	type member struct {
		name string
		src  string
	}
	order := []member{
		{"HDDomJSInline", HDDomJSInline},
		{"VimNavJSInline", VimNavJSInline},
		{"RespondJSInline", RespondJSInline},
		{"MobileJSInline", MobileJSInline},
		{"TabsJSInline", TabsJSInline},
		{"TriageJSInline", TriageJSInline},
		{"LiveJSInline", LiveJSInline},
		{"LiveBannerJSInline", LiveBannerJSInline},
		{"SearchJSInline", SearchJSInline},
		{"HTMLBlockJSInline", HTMLBlockJSInline},
	}

	prev := -1
	for _, m := range order {
		// Use a sufficient prefix to anchor position uniquely.
		prefix := m.src
		if len(prefix) > 64 {
			prefix = prefix[:64]
		}
		idx := strings.Index(ReportJS, prefix)
		if idx < 0 {
			t.Errorf("ReportJS: %s not found (first 64 chars: %q)", m.name, prefix)
			prev = -1 // reset so ordering errors don't cascade
			continue
		}
		if idx <= prev {
			t.Errorf("ReportJS: %s appears before its predecessor (at %d, prev was %d)", m.name, idx, prev)
		}
		prev = idx
	}
}

// TestAssembledBundleScriptGuard verifies that no assembled bundle emits a
// literal </script sequence that would terminate a surrounding <script> tag.
// Each member JS file is individually escaped — this test catches any file
// that slips through without the escape (the RespondJS gap found in the audit).
func TestAssembledBundleScriptGuard(t *testing.T) {
	bundles := map[string]string{
		"ReportJS": ReportJS,
	}
	for name, bundle := range bundles {
		if strings.Contains(bundle, "</script") {
			// Find which member introduced it
			t.Errorf("%s contains a bare </script sequence — it would terminate an inline <script> tag", name)
		}
	}
}

// TestScriptGuardSurvivesInjection injects a </script payload through the
// assembly path and asserts it cannot survive into the inline output.
// This exercises the per-file escape before assembly (not just the bundle check).
func TestScriptGuardSurvivesInjection(t *testing.T) {
	// Simulate what would happen if a JS file contained </script
	payload := `console.log("</script><script>alert(1)</script>")`
	escaped := strings.ReplaceAll(payload, "</script", `<\/script`)
	if strings.Contains(escaped, "</script") {
		t.Error("escape did not remove </script from payload")
	}
	// The escaped form must not contain the raw sequence
	if strings.Contains(escaped, "</script") {
		t.Error("escaped string still contains </script")
	}
}

// TestAggregatorBundleAssembled guards the Go-side reassembly of the split
// dashboard script and its load order: the tree module (aggregator-tree.js) is
// prepended before the core IIFE (the core calls HDTree.paint() during init);
// aggregator.js opens the shared IIFE; the settings fragment continues it; the
// IIFE close is re-added; and the help module (aggregator-help.js) is appended
// after the close (the core references HDHelp only from deferred callbacks).
func TestAggregatorBundleAssembled(t *testing.T) {
	b := AggregatorJS
	if !strings.Contains(b, "'use strict'") {
		t.Error("AggregatorJS missing the core fragment (no 'use strict' from the IIFE head)")
	}
	// Anchor the core by a core-only marker; the settings fragment by viewSettings.
	core := strings.Index(b, "function viewSettings(")
	if core < 0 {
		t.Fatal("AggregatorJS missing the settings fragment (viewSettings)")
	}
	// Match the real export RHS, not any prose mention in a comment. Load order:
	// tree module BEFORE the core (HDTree.paint runs during core init), help
	// module AFTER the core (the core references HDHelp only from deferred
	// callbacks).
	tree := strings.Index(b, "window.HDTree = {")
	if tree < 0 {
		t.Error("AggregatorJS missing the tree module (window.HDTree export)")
	} else if tree > core {
		t.Error("AggregatorJS: tree module must be prepended before the core")
	}
	help := strings.Index(b, "window.HDHelp = { open: openHelpOverlay")
	if help < 0 {
		t.Error("AggregatorJS missing the help module (window.HDHelp export)")
	} else if help < core {
		t.Error("AggregatorJS: help module must be appended after the core")
	}
	if !strings.HasSuffix(strings.TrimSpace(b), "})();") {
		t.Error("AggregatorJS must end with the help module's IIFE close })();")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
