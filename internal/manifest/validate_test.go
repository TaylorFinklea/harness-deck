package manifest

import (
	"strings"
	"testing"
)

func TestValidStatus(t *testing.T) {
	for _, s := range []string{"draft", "awaiting-review", "answered", "done"} {
		if !ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = false, want true", s)
		}
	}
	for _, s := range []string{"", "bogus", "Done"} {
		if ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = true, want false", s)
		}
	}
}

func parseReportWithBlock(t *testing.T, blockJSON string) *Report {
	t.Helper()
	src := `{"schema":"harness-deck/report@1","id":"r1","project":"p","harness":"h",` +
		`"title":"t","status":"draft","created":"2026-01-01T00:00:00Z",` +
		`"blocks":[` + blockJSON + `]}`
	r, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return r
}

func hasValidationProblem(ps []Problem, substr string) bool {
	for _, p := range ps {
		if strings.Contains(p.String(), substr) {
			return true
		}
	}
	return false
}

func TestValidateMultiAskWithOptions(t *testing.T) {
	r := parseReportWithBlock(t, `{"type":"ask","id":"q1","prompt":"Pick some?","mode":"multi","options":["a","b","c"]}`)
	if ps := r.Validate(); len(ps) != 0 {
		t.Fatalf("valid multi ask should have no problems; got: %v", ps)
	}
}

func TestValidateMultiAskWithoutOptionsIsInvalid(t *testing.T) {
	r := parseReportWithBlock(t, `{"type":"ask","id":"q1","prompt":"Pick some?","mode":"multi"}`)
	ps := r.Validate()
	if !hasValidationProblem(ps, "choice/multi mode needs options") {
		t.Errorf("multi ask without options should fail validation; got: %v", ps)
	}
}

// parseReportWithRelated builds a minimal valid report with a "related" field.
func parseReportWithRelated(t *testing.T, relatedJSON string) *Report {
	t.Helper()
	src := `{"schema":"harness-deck/report@1","id":"r1","project":"p","harness":"h",` +
		`"title":"t","status":"draft","created":"2026-01-01T00:00:00Z",` +
		`"related":` + relatedJSON + `,` +
		`"blocks":[{"type":"prose","markdown":"body"}]}`
	r, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return r
}

func TestValidateRelatedWithIDIsValid(t *testing.T) {
	r := parseReportWithRelated(t, `[{"id":"spec-run-01","rel":"spec"},{"id":"audit-07","project":"other","label":"Audit"}]`)
	if ps := r.Validate(); len(ps) != 0 {
		t.Fatalf("valid related entries should have no problems; got: %v", ps)
	}
}

func TestValidateRelatedWithEmptyIDIsInvalid(t *testing.T) {
	r := parseReportWithRelated(t, `[{"id":""},{"id":"ok-run"}]`)
	ps := r.Validate()
	if !hasValidationProblem(ps, "related[0]") || !hasValidationProblem(ps, "missing id") {
		t.Errorf("related entry with empty id should fail validation; got: %v", ps)
	}
	// The entry with a valid id should not produce a problem.
	for _, p := range ps {
		if strings.Contains(p.String(), "related[1]") {
			t.Errorf("related[1] (valid entry) should not produce a problem; got: %v", p)
		}
	}
}

// --- card-grid validation tests ---

func TestValidateCardGridValid(t *testing.T) {
	r := parseReportWithBlock(t, `{"type":"card-grid","cards":[{"title":"Alpha","markdown":"body"},{"title":"Beta","pills":[{"text":"done","level":"ok"}]}]}`)
	if ps := r.Validate(); len(ps) != 0 {
		t.Fatalf("valid card-grid should have no problems; got: %v", ps)
	}
}

func TestValidateCardGridNoCards(t *testing.T) {
	r := parseReportWithBlock(t, `{"type":"card-grid","cards":[]}`)
	ps := r.Validate()
	if !hasValidationProblem(ps, "card-grid: no cards") {
		t.Errorf("card-grid with no cards should fail validation; got: %v", ps)
	}
}

func TestValidateCardGridCardMissingTitle(t *testing.T) {
	r := parseReportWithBlock(t, `{"type":"card-grid","cards":[{"title":""},{"title":"Beta"}]}`)
	ps := r.Validate()
	if !hasValidationProblem(ps, "card-grid: card missing title") {
		t.Errorf("card with empty title should fail validation; got: %v", ps)
	}
}

func TestValidateCardGridBadPillLevel(t *testing.T) {
	r := parseReportWithBlock(t, `{"type":"card-grid","cards":[{"title":"Alpha","pills":[{"text":"x","level":"bad"}]}]}`)
	ps := r.Validate()
	if !hasValidationProblem(ps, `"bad"`) {
		t.Errorf("card with invalid pill level should fail validation; got: %v", ps)
	}
}

func TestValidateRelatedUnknownFieldIsRejected(t *testing.T) {
	r := parseReportWithRelated(t, `[{"id":"run-01","unknown_field":"oops"}]`)
	ps := r.Validate()
	if !hasValidationProblem(ps, "invalid fields") {
		t.Errorf("related entry with unknown field should fail strict decode; got: %v", ps)
	}
}
