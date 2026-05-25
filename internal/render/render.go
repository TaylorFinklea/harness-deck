// Package render turns a validated report manifest into a complete, themed
// HTML page in the v1 TUI dashboard style. The renderer owns every byte of
// layout and CSS; a manifest only ever supplies content, so all reports stay
// visually consistent and restyle together when the templates change.
package render

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"math"
	"strconv"
	"strings"

	"github.com/TaylorFinklea/harness-deck/internal/assets"
	"github.com/TaylorFinklea/harness-deck/internal/manifest"
	"github.com/TaylorFinklea/harness-deck/internal/respond"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

// Renderer holds the parsed template set. It is safe to reuse across renders.
type Renderer struct {
	tmpl *template.Template
}

// New parses the embedded templates and returns a ready Renderer.
func New() (*Renderer, error) {
	t := template.New("harness-deck").Funcs(funcMap())
	t, err := t.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Renderer{tmpl: t}, nil
}

// Report renders a full HTML report page. responses holds answers already
// recorded for the report's interactive blocks (keyed by block id); pass
// nil when there are none. sig is an opaque fingerprint of the report's
// current on-disk state — the page bakes it into window.HD_REPORT.sig so
// the live-reload JS can detect when the server's view differs from what
// the page is showing. Pass "" when no live-reload is wanted (e.g. the
// standalone `harness-deck render` CLI).
func (r *Renderer) Report(rep *manifest.Report, responses map[string]respond.Response, sig string) ([]byte, error) {
	pv := r.buildPage(rep, responses)
	pv.Sig = sig
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, "page", pv); err != nil {
		return nil, fmt.Errorf("render report: %w", err)
	}
	return buf.Bytes(), nil
}

// --- view models -----------------------------------------------------------

type pageView struct {
	Report *manifest.Report
	Banner bannerView
	TOC    []tocItem
	Run    []manifest.KV // sidebar "run" metadata section
	// OpenAsks summarizes the unanswered interactive blocks for the
	// pinned banner at the top of the report. Empty when nothing is
	// outstanding — the template hides the banner in that case.
	OpenAsks []openAsk
	Blocks   []template.HTML // pre-rendered block panels
	Sig      string          // server-side fingerprint for live-reload diffing
	CSS      template.CSS
	JS       template.JS
	Favicon  template.URL
}

// openAsk is one unanswered interactive block surfaced in the pinned
// banner. Num matches the §NN section number, Kind is "ask" / "decision"
// / "approval", Title is the block's human label.
type openAsk struct {
	Num, Anchor, Kind, Title string
}

type bannerView struct {
	Kicker, Scope, Title, Verdict string
	Meta                          []manifest.KV
}

type tocItem struct{ Num, Title, Anchor string }

// blockView is what every block template receives.
type blockView struct {
	Num, ID, Title string
	Pills          []manifest.Pill
	Body           any               // the concrete *manifest.XxxBlock, or fallbackView
	Answer         *respond.Response // recorded answer, for interactive blocks
}

// fallbackView backs block-fallback for unknown or unrenderable blocks.
type fallbackView struct {
	Type, Reason, Raw string
}

// --- page assembly ---------------------------------------------------------

func (r *Renderer) buildPage(rep *manifest.Report, responses map[string]respond.Response) pageView {
	pv := pageView{
		Report: rep,
		Banner: bannerView{
			Kicker:  rep.Kind,
			Scope:   rep.Scope,
			Title:   rep.Title,
			Verdict: rep.Verdict,
			Meta:    bannerMeta(rep),
		},
		Run:     runMeta(rep),
		CSS:     template.CSS(assets.ReportCSS),
		JS:      template.JS(assets.ReportJS),
		Favicon: template.URL(assets.FaviconDataURI),
	}
	for i, b := range rep.Blocks {
		title := blockTitle(b)
		num := fmt.Sprintf("%02d", i+1)
		anchor := fmt.Sprintf("sec-%d", i+1)
		pv.TOC = append(pv.TOC, tocItem{Num: num, Title: title, Anchor: anchor})
		pv.Blocks = append(pv.Blocks, r.renderBlock(i, b, title, responses))
		if manifest.InteractiveTypes[b.Type] {
			id := manifest.InteractiveID(b)
			if id != "" {
				if _, answered := responses[id]; answered {
					continue
				}
			}
			pv.OpenAsks = append(pv.OpenAsks, openAsk{Num: num, Anchor: anchor, Kind: b.Type, Title: title})
		}
	}
	return pv
}

// renderBlock renders one block panel. Unknown block types and blocks whose
// template fails are routed to renderFallback so a single bad block degrades
// gracefully instead of blanking the whole report.
func (r *Renderer) renderBlock(idx int, b manifest.Block, title string, responses map[string]respond.Response) template.HTML {
	bv := blockView{
		Num:   fmt.Sprintf("%02d", idx+1),
		ID:    fmt.Sprintf("sec-%d", idx+1),
		Title: title,
		Body:  b.Body,
	}
	if b.Body != nil {
		bv.Pills = b.Body.PanelPills()
	}
	if id := manifest.InteractiveID(b); id != "" {
		if got, ok := responses[id]; ok {
			answer := got
			bv.Answer = &answer
		}
	}
	if b.Body == nil {
		return r.renderFallback(bv, b, "unknown block type "+strconv.Quote(b.Type))
	}
	name := "block-" + b.Type
	if r.tmpl.Lookup(name) == nil {
		return r.renderFallback(bv, b, "no template for block type "+strconv.Quote(b.Type))
	}
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, name, bv); err != nil {
		return r.renderFallback(bv, b, "block failed to render: "+err.Error())
	}
	return template.HTML(buf.String())
}

// renderFallback produces the placeholder panel shown for a block the renderer
// cannot handle. Strategy: render an error-styled panel that names the problem
// and shows the block's raw JSON, so the report still loads and the author can
// see exactly what went wrong (rather than failing the whole render or
// silently dropping the block).
func (r *Renderer) renderFallback(bv blockView, b manifest.Block, reason string) template.HTML {
	bv.Body = fallbackView{Type: b.Type, Reason: reason, Raw: string(b.Raw)}
	var buf bytes.Buffer
	if err := r.tmpl.ExecuteTemplate(&buf, "block-fallback", bv); err != nil {
		return template.HTML("<!-- harness-deck: fallback render failed -->")
	}
	return template.HTML(buf.String())
}

// --- helpers ---------------------------------------------------------------

var defaultTitles = map[string]string{
	manifest.TypeProse: "summary", manifest.TypeMetrics: "key metrics",
	manifest.TypeRisks: "risk register", manifest.TypeDiff: "representative diffs",
	manifest.TypeTimeline: "timeline", manifest.TypeCompare: "comparison",
	manifest.TypeRecommendations: "recommendations", manifest.TypeCallout: "note",
	manifest.TypeBarchart: "breakdown", manifest.TypeTable: "table",
	manifest.TypeHTML: "detail",
}

func blockTitle(b manifest.Block) string {
	if b.Body != nil && b.Body.PanelTitle() != "" {
		return b.Body.PanelTitle()
	}
	if d, ok := defaultTitles[b.Type]; ok {
		return d
	}
	if b.Type == "" {
		return "block"
	}
	return b.Type
}

// bannerMeta is the right-aligned metadata block in the report banner.
func bannerMeta(rep *manifest.Report) []manifest.KV {
	var m []manifest.KV
	add := func(k, v string) {
		if v != "" {
			m = append(m, manifest.KV{Key: k, Value: v})
		}
	}
	add("run", rep.ID)
	add("created", rep.Created)
	add("agent", rep.Agent)
	add("verdict", rep.Verdict)
	return m
}

// runMeta is the sidebar "run" section: identity plus author-supplied metadata.
func runMeta(rep *manifest.Report) []manifest.KV {
	m := []manifest.KV{}
	add := func(k, v string) {
		if v != "" {
			m = append(m, manifest.KV{Key: k, Value: v})
		}
	}
	add("id", rep.ID)
	add("harness", rep.Harness)
	add("model", rep.Agent)
	m = append(m, rep.Meta...)
	add("status", rep.Status)
	return m
}

// --- template functions ----------------------------------------------------

type asciiBarParts struct{ Filled, Empty string }

type diffStats struct{ Added, Removed, Files int }

func funcMap() template.FuncMap {
	return template.FuncMap{
		"md":        renderMarkdown,
		"mdInline":  renderMarkdownInline,
		"upper":     strings.ToUpper,
		"pct":       pctText,
		"sparkPath": sparkPath,
		"asciiBar":  asciiBar,
		"diffStat":  diffStat,
		"glyph":     timelineGlyph,
		"riskCount": riskCount,
		"add":       func(a, b int) int { return a + b },
		// safeHTML marks an html-block's body as trusted. The html block is a
		// deliberate escape hatch for agent-authored markup; harness-deck is a
		// local single-user tool, so this is an accepted trust boundary.
		"safeHTML": func(s string) template.HTML { return template.HTML(s) },
	}
}

// pctText formats a percentage without a trailing ".0".
func pctText(p float64) string {
	if p == math.Trunc(p) {
		return strconv.Itoa(int(p))
	}
	return strconv.FormatFloat(p, 'f', 1, 64)
}

// sparkPath maps sparkline points to an SVG path within an 80x16 viewBox.
func sparkPath(pts []float64) string {
	if len(pts) < 2 {
		return ""
	}
	lo, hi := pts[0], pts[0]
	for _, p := range pts {
		lo, hi = math.Min(lo, p), math.Max(hi, p)
	}
	span := hi - lo
	if span == 0 {
		span = 1
	}
	var b strings.Builder
	for i, p := range pts {
		x := float64(i) / float64(len(pts)-1) * 80
		y := 14 - (p-lo)/span*12 // invert: 14 = bottom, 2 = top
		cmd := "L"
		if i == 0 {
			cmd = "M"
		}
		fmt.Fprintf(&b, "%s%.1f,%.1f ", cmd, x, y)
	}
	return strings.TrimSpace(b.String())
}

// asciiBar splits a 52-cell bar into filled (█) and empty (·) runs.
func asciiBar(pct float64) asciiBarParts {
	const total = 52
	n := int(math.Round(pct / 100 * total))
	if n < 0 {
		n = 0
	}
	if n > total {
		n = total
	}
	return asciiBarParts{strings.Repeat("█", n), strings.Repeat("·", total-n)}
}

// diffStat counts added/removed lines and files across a diff block.
func diffStat(d *manifest.DiffBlock) diffStats {
	s := diffStats{Files: len(d.Files)}
	for _, f := range d.Files {
		for _, ln := range f.Lines {
			switch ln.Kind {
			case "add":
				s.Added++
			case "del":
				s.Removed++
			}
		}
	}
	return s
}

// timelineGlyph picks the box-drawing glyph for a timeline row.
func timelineGlyph(i, n int, kind string) string {
	switch kind {
	case "ok":
		return "✓"
	case "crit":
		return "!"
	}
	switch i {
	case 0:
		return "┬"
	case n - 1:
		return "┴"
	default:
		return "├"
	}
}

// riskCount counts risks of a given severity, for the panel-header pills.
func riskCount(rs []manifest.Risk, sev string) int {
	n := 0
	for _, r := range rs {
		if r.Severity == sev {
			n++
		}
	}
	return n
}
