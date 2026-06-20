package render

import (
	"strings"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/manifest"
	"github.com/TaylorFinklea/harness-deck/internal/respond"
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
	out, err := r.Report(rep, nil, "")
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
		`class="panel"`,       // panels rendered
		`<b>ready</b>`,        // markdown bold
		`class="metric-grid"`, // metrics block
		`class="delta pos"`,   // metric trend
		`<svg class="spark"`,  // sparkline
		`class="sev crit"`,    // risk severity
		`<hd-html><template>`, // html block wrapped for shadow-DOM isolation
		`<i id=esc>raw</i>`,   // html escape-hatch passed through verbatim
		`VimNav.init`,         // vim navigation wired
		`class="statusline"`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered output missing %q", want)
		}
	}
}

func TestInteractiveBlockShowsControlsThenAnswer(t *testing.T) {
	src := `{
	  "schema": "harness-deck/report@1",
	  "id": "0x1", "project": "p", "harness": "h", "title": "t",
	  "status": "awaiting-review", "created": "2026-05-18T18:39:50Z",
	  "blocks": [{"type": "ask", "id": "q1", "prompt": "Ship it?", "mode": "yesno"}]
	}`
	rep, err := manifest.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	r, err := New()
	if err != nil {
		t.Fatalf("renderer: %v", err)
	}

	unanswered, err := r.Report(rep, nil, "")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(string(unanswered), `data-block="q1"`) {
		t.Error("unanswered ask block should render response controls")
	}

	answered, err := r.Report(rep, map[string]respond.Response{
		"q1": {Block: "q1", Value: "yes", At: "2026-05-20T10:00:00Z"},
	}, "")
	if err != nil {
		t.Fatalf("render answered: %v", err)
	}
	s := string(answered)
	if !strings.Contains(s, "ask-answered") || !strings.Contains(s, "<b>yes</b>") {
		t.Error("answered ask block should show the recorded answer")
	}
	if strings.Contains(s, `data-block="q1"`) {
		t.Error("response controls should be gone once the block is answered")
	}
}

func TestMarkdownRendersHeadingsListsAndInline(t *testing.T) {
	out := string(Markdown("# Title\n\n## Section\n\nBody **bold** and `code`.\n\n- one\n- two"))
	for _, want := range []string{
		"<h1>Title</h1>",
		"<h2>Section</h2>",
		"<p>Body <b>bold</b> and <code>code</code>.</p>",
		"<ul><li>one</li><li>two</li></ul>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown output missing %q\ngot: %s", want, out)
		}
	}
}

// TestMarkdownTable confirms GitHub-style tables produce a <table.md-table>
// with <thead>/<tbody>, header cells in <th>, data cells in <td>, and that
// cell content still runs through inlineMarkdown (so **bold** in a cell
// becomes <b>).
func TestMarkdownTable(t *testing.T) {
	src := "Before.\n\n| col1 | col2 |\n| ---- | ---- |\n| **a** | b |\n| c | d |\n\nAfter."
	out := string(Markdown(src))
	for _, want := range []string{
		`<p>Before.</p>`,
		`<table class="md-table">`,
		`<thead><tr><th>col1</th><th>col2</th></tr></thead>`,
		`<tbody>`,
		`<tr><td><b>a</b></td><td>b</td></tr>`,
		`<tr><td>c</td><td>d</td></tr>`,
		`</tbody></table>`,
		`<p>After.</p>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown output missing %q\ngot: %s", want, out)
		}
	}
}

// TestMarkdownBlockquote covers `> ` blockquotes, multi-line joins with a
// space, and inline marks inside the quote text.
func TestMarkdownBlockquote(t *testing.T) {
	src := "> first **bold** line\n> second line\n\nAfter."
	out := string(Markdown(src))
	for _, want := range []string{
		`<blockquote><p>first <b>bold</b> line second line</p></blockquote>`,
		`<p>After.</p>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown output missing %q\ngot: %s", want, out)
		}
	}
}

// TestMarkdownTaskList covers GitHub task lists: a run of `- [x] …` /
// `- [ ] …` bullets renders with a .task-list class on the <ul>, and
// each <li> gains .task-list-item.done or .open plus a checkbox glyph.
func TestMarkdownTaskList(t *testing.T) {
	src := "- [x] shipped item\n- [ ] open item\n- [X] also done"
	out := string(Markdown(src))
	for _, want := range []string{
		`<ul class="task-list">`,
		`<li class="task-list-item done">`,
		`<li class="task-list-item open">`,
		`<span class="checkbox" aria-hidden="true">☑</span>`,
		`<span class="checkbox" aria-hidden="true">☐</span>`,
		`shipped item`,
		`open item`,
		`also done`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown output missing %q\ngot: %s", want, out)
		}
	}
}

// TestMarkdownHRule confirms `---` (or `***`) on its own line emits <hr>.
func TestMarkdownHRule(t *testing.T) {
	src := "Before.\n\n---\n\nAfter."
	out := string(Markdown(src))
	for _, want := range []string{`<p>Before.</p>`, `<hr />`, `<p>After.</p>`} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown output missing %q\ngot: %s", want, out)
		}
	}
}

// TestMarkdownHeadingStatusPill confirms trailing `(DONE)` / `(WIP)` /
// `(planned)` etc. tokens become a styled .status-pill inside the
// heading.
func TestMarkdownHeadingStatusPill(t *testing.T) {
	src := "## M16: Feed Management (DONE)\n## M17: Web app (WIP)\n## M18: Theme switch (planned)"
	out := string(Markdown(src))
	for _, want := range []string{
		`<span class="status-pill done">DONE</span>`,
		`<span class="status-pill wip">WIP</span>`,
		`<span class="status-pill planned">planned</span>`,
		`M16: Feed Management`,
		`M17: Web app`,
		`M18: Theme switch`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown output missing %q\ngot: %s", want, out)
		}
	}
}

// TestMarkdownLinks covers inline `[text](url)` links and `<https://…>`
// autolinks. Both should yield <a href="…"> with rel="noopener".
func TestMarkdownLinks(t *testing.T) {
	src := "See [the docs](https://example.com/x) or <https://example.com/y>."
	out := string(Markdown(src))
	for _, want := range []string{
		`<a href="https://example.com/x" rel="noopener">the docs</a>`,
		`<a href="https://example.com/y" rel="noopener">https://example.com/y</a>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown output missing %q\ngot: %s", want, out)
		}
	}
}

// TestMarkdownFencedCodeRendersCopyable confirms ```lang ... ``` produces
// the copy-button wrapper the frontend wires up, with the body escaped
// but never mistakenly run through inline markdown.
func TestMarkdownFencedCodeRendersCopyable(t *testing.T) {
	src := "Prefix paragraph.\n\n```python\nfor i in range(3):\n    print(i * 2)\n```\n\nTrailing line."
	out := string(Markdown(src))
	for _, want := range []string{
		`<div class="code-block">`,
		`<span class="lang">python</span>`,
		`<button class="copy-btn"`,
		`<code class="lang-python">`,
		`print(i * 2)`, // body present verbatim, not markdown-italicized
		`<p>Prefix paragraph.</p>`,
		`<p>Trailing line.</p>`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Markdown output missing %q\ngot: %s", want, out)
		}
	}
	if strings.Contains(out, "<i>") {
		t.Errorf("fenced code body should not be italicized\ngot: %s", out)
	}
}

// TestRelatedPanelRendersLinks confirms that a report with a "related" array
// renders the .related-panel with correct hrefs, applies project-default
// when project is omitted, and uses id as label when label is omitted.
func TestRelatedPanelRendersLinks(t *testing.T) {
	html := renderJSON(t, `{
	  "schema": "harness-deck/report@1",
	  "id": "impl-run", "project": "my-proj", "harness": "claude-code",
	  "title": "impl", "status": "done", "created": "2026-05-18T18:39:50Z",
	  "related": [
	    {"id": "spec-run", "rel": "spec"},
	    {"id": "audit-run", "project": "other-proj", "rel": "audit", "label": "Security audit"}
	  ],
	  "blocks": [{"type": "prose", "markdown": "done"}]
	}`)

	for _, want := range []string{
		`class="related-panel"`,
		`class="related-head"`,
		// project-default: my-proj used because entry has no "project"
		`href="/r/my-proj/spec-run"`,
		// label defaults to id when not set
		`spec-run</a>`,
		// explicit project
		`href="/r/other-proj/audit-run"`,
		// explicit label
		`Security audit</a>`,
		// rel tags rendered
		`class="related-rel"`,
		`>spec</span>`,
		`>audit</span>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("related panel: output missing %q", want)
		}
	}
}

// TestNoRelatedPanelWhenEmpty confirms the panel div is absent when Related is nil.
func TestNoRelatedPanelWhenEmpty(t *testing.T) {
	html := renderJSON(t, `{
	  "schema": "harness-deck/report@1",
	  "id": "x", "project": "p", "harness": "h", "title": "t",
	  "status": "draft", "created": "2026-05-18T18:39:50Z",
	  "blocks": [{"type": "prose", "markdown": "body"}]
	}`)
	// The CSS in <style> contains "related-panel" as a selector; use the
	// data-vim-section attribute (only present on the rendered div) to
	// confirm the panel div itself is not emitted.
	if strings.Contains(html, `class="related-panel"`) {
		t.Error("report with no related field should not render .related-panel div")
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
