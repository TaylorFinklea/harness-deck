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
