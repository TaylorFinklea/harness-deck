package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Problem is a single validation failure, located by a dotted/indexed path.
type Problem struct {
	Path string
	Msg  string
}

func (p Problem) String() string { return p.Path + ": " + p.Msg }

// statusValues are the allowed values of Report.Status.
var statusValues = enum("draft", "awaiting-review", "answered", "done")

// Validate checks a report manifest for structural and semantic problems and
// returns every problem found (empty slice means valid). It is intentionally
// strict — the `validate` command uses it to catch authoring mistakes such as
// misspelled fields, unknown block types, and out-of-range values.
func (r *Report) Validate() []Problem {
	var ps []Problem
	add := func(path, msg string) { ps = append(ps, Problem{path, msg}) }

	// Strict re-decode of the whole document catches unknown top-level keys
	// that the lenient Parse silently dropped, mirroring the per-block strict
	// re-decode below. Block decoding stays lenient (Block.UnmarshalJSON does
	// its own non-strict unmarshal), so unknown block fields are reported by
	// validateBlock, not here.
	if r.raw != nil {
		if err := strictDecode(r.raw, &Report{}); err != nil {
			add("", "invalid fields: "+err.Error())
		}
	}

	ps = append(ps, checkSchemaStrict(r.Schema)...)
	for field, val := range map[string]string{
		"id": r.ID, "project": r.Project, "harness": r.Harness,
		"title": r.Title, "status": r.Status, "created": r.Created,
	} {
		if val == "" {
			add(field, "missing")
		}
	}
	if r.Status != "" && !statusValues[r.Status] {
		add("status", fmt.Sprintf("not one of draft|awaiting-review|answered|done: %q", r.Status))
	}
	if r.Created != "" {
		if _, err := time.Parse(time.RFC3339, r.Created); err != nil {
			add("created", "not an RFC3339 timestamp")
		}
	}
	for i, kv := range r.Meta {
		if kv.Key == "" {
			add(fmt.Sprintf("meta[%d]", i), "missing key")
		}
	}
	for i, tag := range r.Tags {
		if strings.TrimSpace(tag) == "" {
			add(fmt.Sprintf("tags[%d]", i), "empty tag")
		}
	}
	for i, rel := range r.Related {
		if rel.ID == "" {
			add(fmt.Sprintf("related[%d]", i), "missing id")
		}
	}
	if len(r.Blocks) == 0 {
		add("blocks", "report has no blocks")
	}
	seenIDs := map[string]int{}
	for i, b := range r.Blocks {
		ps = append(ps, validateBlock(fmt.Sprintf("blocks[%d]", i), b)...)
		// Interactive block ids must be unique — they key responses.json.
		if id := InteractiveID(b); id != "" {
			if first, dup := seenIDs[id]; dup {
				add(fmt.Sprintf("blocks[%d]", i),
					fmt.Sprintf("interactive id %q already used by blocks[%d]", id, first))
			} else {
				seenIDs[id] = i
			}
		}
	}
	return ps
}

// validateBlock checks one block: type known, no unknown fields, semantics sound.
func validateBlock(path string, b Block) []Problem {
	var ps []Problem
	add := func(msg string) { ps = append(ps, Problem{path, msg}) }

	if b.Type == "" {
		add("missing type")
		return ps
	}
	if !KnownType(b.Type) {
		add(fmt.Sprintf("unknown block type %q", b.Type))
		return ps
	}
	// Strict re-decode catches misspelled or extra fields that the lenient
	// parse silently dropped.
	if err := strictDecode(b.Raw, registry[b.Type]()); err != nil {
		add("invalid fields: " + err.Error())
		return ps
	}

	switch body := b.Body.(type) {
	case *ProseBlock:
		if body.Markdown == "" {
			add("prose: empty markdown")
		}
	case *MetricsBlock:
		if len(body.Metrics) == 0 {
			add("metrics: no metrics")
		}
		for i, m := range body.Metrics {
			mp := fmt.Sprintf("%s metrics[%d]", path, i)
			if m.Label == "" || m.Value == "" {
				ps = append(ps, Problem{mp, "label and value are required"})
			}
			ps = append(ps, checkEnum(mp+".trend", m.Trend, "", "pos", "neg")...)
			ps = append(ps, checkEnum(mp+".color", m.Color, "", "ok", "warn", "err", "info")...)
		}
		ps = append(ps, checkBars(path, body.Bars)...)
	case *RisksBlock:
		if len(body.Risks) == 0 {
			add("risks: no risks")
		}
		for i, rk := range body.Risks {
			rp := fmt.Sprintf("%s risks[%d]", path, i)
			ps = append(ps, checkEnum(rp+".severity", rk.Severity, "crit", "high", "med", "low")...)
			ps = append(ps, checkPct(rp, rk.Pct)...)
		}
		for i, c := range body.Callouts {
			ps = append(ps, checkCallout(fmt.Sprintf("%s callouts[%d]", path, i), c)...)
		}
	case *DiffBlock:
		if len(body.Files) == 0 {
			add("diff: no files")
		}
		for i, f := range body.Files {
			fp := fmt.Sprintf("%s files[%d]", path, i)
			if f.Path == "" {
				ps = append(ps, Problem{fp, "missing path"})
			}
			for j, ln := range f.Lines {
				ps = append(ps, checkEnum(fmt.Sprintf("%s lines[%d].kind", fp, j),
					ln.Kind, "ctx", "add", "del", "hunk")...)
			}
		}
	case *TimelineBlock:
		if len(body.Events) == 0 {
			add("timeline: no events")
		}
		for i, e := range body.Events {
			ep := fmt.Sprintf("%s events[%d]", path, i)
			if e.Time == "" || e.Markdown == "" {
				ps = append(ps, Problem{ep, "time and markdown are required"})
			}
			ps = append(ps, checkEnum(ep+".kind", e.Kind, "", "ok", "crit")...)
		}
	case *CompareBlock:
		for tag, side := range map[string]CompareSide{"a": body.A, "b": body.B} {
			if len(side.Items) == 0 {
				add("compare: side " + tag + " has no items")
			}
			for i, it := range side.Items {
				ps = append(ps, checkEnum(fmt.Sprintf("%s %s.items[%d].kind", path, tag, i),
					it.Kind, "pro", "con", "neu")...)
			}
		}
	case *RecommendationsBlock:
		if len(body.Items) == 0 {
			add("recommendations: no items")
		}
		for i, it := range body.Items {
			if it.Markdown == "" {
				ps = append(ps, Problem{fmt.Sprintf("%s items[%d]", path, i), "empty markdown"})
			}
		}
	case *CalloutBlock:
		ps = append(ps, checkCallout(path, body.Callout)...)
	case *BarchartBlock:
		if len(body.Bars) == 0 {
			add("barchart: no bars")
		}
		ps = append(ps, checkBars(path, body.Bars)...)
	case *TableBlock:
		if len(body.Columns) == 0 {
			add("table: no columns")
		}
		for i, row := range body.Rows {
			if len(row) != len(body.Columns) {
				ps = append(ps, Problem{fmt.Sprintf("%s rows[%d]", path, i),
					fmt.Sprintf("has %d cells, expected %d", len(row), len(body.Columns))})
			}
		}
	case *HTMLBlock:
		if body.HTML == "" {
			add("html: empty html")
		}
	case *CardGridBlock:
		if len(body.Cards) == 0 {
			add("card-grid: no cards")
		}
		for i, card := range body.Cards {
			cp := fmt.Sprintf("%s cards[%d]", path, i)
			if card.Title == "" {
				ps = append(ps, Problem{cp, "card-grid: card missing title"})
			}
			for j, pill := range card.Pills {
				ps = append(ps, checkEnum(fmt.Sprintf("%s pills[%d].level", cp, j),
					pill.Level, "", "ok", "warn", "err")...)
			}
		}
	case *AskBlock:
		if body.ID == "" {
			add("ask: missing id")
		}
		if body.Prompt == "" {
			add("ask: empty prompt")
		}
		ps = append(ps, checkEnum(path+".mode", body.Mode, "", "choice", "yesno", "text", "multi")...)
		if (body.ResolvedMode() == "choice" || body.ResolvedMode() == "multi") && len(body.Options) == 0 {
			add("ask: choice/multi mode needs options")
		}
	case *DecisionBlock:
		if body.ID == "" {
			add("decision: missing id")
		}
		for tag, side := range map[string]CompareSide{"a": body.A, "b": body.B} {
			if len(side.Items) == 0 {
				add("decision: side " + tag + " has no items")
			}
			for i, it := range side.Items {
				ps = append(ps, checkEnum(fmt.Sprintf("%s %s.items[%d].kind", path, tag, i),
					it.Kind, "pro", "con", "neu")...)
			}
		}
	case *ApprovalBlock:
		if body.ID == "" {
			add("approval: missing id")
		}
	}
	return ps
}

func checkCallout(path string, c Callout) []Problem {
	ps := checkEnum(path+".level", c.Level, "info", "warn", "err")
	if c.Markdown == "" {
		ps = append(ps, Problem{path, "callout: empty markdown"})
	}
	return ps
}

func checkBars(path string, bars []Bar) []Problem {
	var ps []Problem
	for i, b := range bars {
		ps = append(ps, checkPct(fmt.Sprintf("%s bars[%d]", path, i), b.Pct)...)
	}
	return ps
}

func checkPct(path string, pct float64) []Problem {
	if pct < 0 || pct > 100 {
		return []Problem{{path, fmt.Sprintf("pct %.1f out of range 0..100", pct)}}
	}
	return nil
}

// checkEnum reports a problem if val is not one of allowed.
func checkEnum(path, val string, allowed ...string) []Problem {
	for _, a := range allowed {
		if val == a {
			return nil
		}
	}
	return []Problem{{path, fmt.Sprintf("invalid value %q", val)}}
}

func enum(vals ...string) map[string]bool {
	m := make(map[string]bool, len(vals))
	for _, v := range vals {
		m[v] = true
	}
	return m
}

func strictDecode(raw []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	return dec.Decode(v)
}

// ValidStatus reports whether s is one of the allowed Report.Status values.
func ValidStatus(s string) bool { return statusValues[s] }
