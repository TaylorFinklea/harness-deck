package manifest

import (
	"strings"
	"testing"
)

const validReport = `{
  "schema": "harness-deck/report@1",
  "id": "0x4a2f",
  "project": "acme-api",
  "harness": "claude-code",
  "agent": "claude-sonnet-4.5",
  "title": "readiness audit",
  "status": "awaiting-review",
  "created": "2026-05-18T18:39:50Z",
  "meta": [{"key": "cost", "value": "$1.84"}],
  "blocks": [
    {"type": "prose", "markdown": "The cluster is broadly ready."},
    {"type": "metrics", "metrics": [{"label": "queries", "value": "312", "trend": "pos"}]},
    {"type": "risks", "risks": [{"severity": "crit", "label": "drift", "pct": 92}]},
    {"type": "html", "html": "<b>custom</b>"}
  ]
}`

func TestParseDispatchesBlockTypes(t *testing.T) {
	r, err := Parse([]byte(validReport))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(r.Blocks) != 4 {
		t.Fatalf("got %d blocks, want 4", len(r.Blocks))
	}
	if _, ok := r.Blocks[0].Body.(*ProseBlock); !ok {
		t.Errorf("block 0: got %T, want *ProseBlock", r.Blocks[0].Body)
	}
	if _, ok := r.Blocks[1].Body.(*MetricsBlock); !ok {
		t.Errorf("block 1: got %T, want *MetricsBlock", r.Blocks[1].Body)
	}
	if _, ok := r.Blocks[3].Body.(*HTMLBlock); !ok {
		t.Errorf("block 3: got %T, want *HTMLBlock", r.Blocks[3].Body)
	}
}

func TestValidateCleanReport(t *testing.T) {
	r, err := Parse([]byte(validReport))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if ps := r.Validate(); len(ps) != 0 {
		t.Fatalf("expected no problems, got: %v", ps)
	}
}

func TestUnknownBlockTypeIsLenient(t *testing.T) {
	// A block type the renderer does not know must still parse — a stale
	// dashboard should degrade, not crash, on a manifest from a newer harness.
	src := `{"schema":"harness-deck/report@1","id":"x","project":"p","harness":"h",
	         "title":"t","status":"draft","created":"2026-05-18T18:39:50Z",
	         "blocks":[{"type":"future-widget","prompt":"go?"}]}`
	r, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse should not fail on unknown block type: %v", err)
	}
	if r.Blocks[0].Type != "future-widget" || r.Blocks[0].Body != nil {
		t.Errorf("unknown block: Type=%q Body=%v, want Type=future-widget Body=nil",
			r.Blocks[0].Type, r.Blocks[0].Body)
	}
	// Validate, however, must flag it.
	if !hasProblem(r.Validate(), "unknown block type") {
		t.Error("Validate should flag an unknown block type")
	}
}

func TestValidateCatchesProblems(t *testing.T) {
	// Use a same-family schema value that isn't an accepted version so Validate
	// flags it. "wrong" would be caught at Parse now (different family); using
	// a future version makes the intent clear and exercises Validate's strict path.
	bad := `{
	  "schema": "harness-deck/report@999",
	  "project": "p",
	  "harness": "h",
	  "title": "t",
	  "status": "bogus",
	  "created": "not-a-date",
	  "blocks": [
	    {"type": "risks", "risks": [{"severity": "extreme", "label": "x", "pct": 150}]},
	    {"type": "prose", "markdown": "ok", "markdwn": "typo"}
	  ]
	}`
	r, err := Parse([]byte(bad))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	ps := r.Validate()
	for _, want := range []string{
		"newer than this binary", // schema: future version of known family
		"missing",                // id missing
		"draft|awaiting-review",
		"RFC3339",
		"invalid value \"extreme\"",
		"out of range",
		"invalid fields", // unknown field "markdwn"
	} {
		if !hasProblem(ps, want) {
			t.Errorf("expected a problem containing %q; problems: %v", want, ps)
		}
	}
}

func hasProblem(ps []Problem, substr string) bool {
	for _, p := range ps {
		if strings.Contains(p.String(), substr) {
			return true
		}
	}
	return false
}

func TestParseLenientOnKnownBlockFieldTypeMismatch(t *testing.T) {
	// The graceful-degradation rule: an unknown block TYPE keeps the report
	// alive with a fallback panel. A known type with one mistyped field must
	// degrade the same way — not abort the whole Parse and silently drop the
	// report from the dashboard.
	data := []byte(`{"schema":"harness-deck/report@1","id":"r1","project":"acme",
		"harness":"claude-code","title":"t","status":"draft",
		"created":"2026-06-10T00:00:00Z",
		"blocks":[{"type":"prose","markdown":123},{"type":"prose","markdown":"fine"}]}`)
	rep, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse should tolerate a mistyped field in one block: %v", err)
	}
	if rep.Blocks[0].Body != nil {
		t.Error("mistyped block should leave Body nil (fallback panel)")
	}
	if rep.Blocks[1].Body == nil {
		t.Error("healthy sibling block must still decode")
	}
	if len(rep.Validate()) == 0 {
		t.Error("Validate must still flag the mistyped field")
	}
}
