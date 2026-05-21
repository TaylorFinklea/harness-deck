package render

import (
	"strings"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/manifest"
)

func renderJSON(t *testing.T, src string) string {
	t.Helper()
	rep, err := manifest.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := New()
	if err != nil {
		t.Fatalf("renderer: %v", err)
	}
	out, err := r.Report(rep)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	return string(out)
}

func TestReportRendersBlocks(t *testing.T) {
	html := renderJSON(t, `{
	  "schema": "harness-deck/report@1",
	  "id": "0x4a2f", "project": "acme-api", "harness": "claude-code",
	  "title": "readiness audit", "scope": "postgres",
	  "status": "awaiting-review", "created": "2026-05-18T18:39:50Z",
	  "blocks": [
	    {"type": "prose", "markdown": "Cluster is **ready** to migrate."},
	    {"type": "metrics", "metrics": [
	      {"label": "queries", "value": "312", "delta": "+27", "trend": "pos", "spark": [1,2,3,5]}]},
	    {"type": "risks", "risks": [{"severity": "crit", "label": "drift", "pct": 92}]},
	    {"type": "html", "html": "<i id=esc>raw</i>"}
	  ]
	}`)

	for _, want := range []string{
		`<title>harness-deck · readiness audit</title>`,
		`class="report-banner"`,
		`<span class="scope">postgres</span>`,
		`class="panel"`,            // panels rendered
		`<b>ready</b>`,             // markdown bold
		`class="metric-grid"`,      // metrics block
		`class="delta pos"`,        // metric trend
		`<svg class="spark"`,       // sparkline
		`class="sev crit"`,         // risk severity
		`<i id=esc>raw</i>`,        // html escape-hatch passed through
		`VimNav.init`,              // vim navigation wired
		`class="statusline"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered output missing %q", want)
		}
	}
}

func TestUnknownBlockRendersFallback(t *testing.T) {
	html := renderJSON(t, `{
	  "schema": "harness-deck/report@1",
	  "id": "x", "project": "p", "harness": "h", "title": "t",
	  "status": "draft", "created": "2026-05-18T18:39:50Z",
	  "blocks": [{"type": "quantum-widget", "spin": "up"}]
	}`)

	for _, want := range []string{
		"unrenderable block",
		"unknown block type",
		"quantum-widget",
		`class="fallback-raw"`, // raw JSON block shown so the author can see what failed
		"spin",                 // the offending field survives into the raw dump
	} {
		if !strings.Contains(html, want) {
			t.Errorf("fallback output missing %q", want)
		}
	}
}
