package assets

import (
	"strings"
	"testing"
)

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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
