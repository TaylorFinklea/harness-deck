package query

import (
	"strings"
	"testing"
	"time"
)

// fakeRecord is a test Record. Field is backed by a map; Text returns a
// fixed body and increments textCalls so the lazy short-circuit invariant
// can assert Text() is never opened when a structural predicate fails first.
type fakeRecord struct {
	fields    map[string]string
	text      string
	textCalls *int
}

func (r fakeRecord) Field(name string) string { return r.fields[name] }

func (r fakeRecord) Text() string {
	if r.textCalls != nil {
		*r.textCalls++
	}
	return r.text
}

// fixedNow is the reference instant for created-relative resolution tests.
// 2026-06-15T12:00:00Z keeps the arithmetic readable.
var fixedNow = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)

func TestParseAST(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // expected Query.String()
	}{
		// Single clauses, every operator shape.
		{"eq", `status = done`, `(status = done)`},
		{"neq", `status != draft`, `(status != draft)`},
		{"contains", `title ~ audit`, `(title ~ audit)`},
		{"ncontains", `title !~ audit`, `(title !~ audit)`},
		{"in", `status IN (draft, done)`, `(status IN [draft done])`},
		{"notin", `project NOT IN (a, b)`, `(project NOT IN [a b])`},
		{"gt", `created > -7d`, `(created > -7d)`},
		{"gte", `created >= -7d`, `(created >= -7d)`},
		{"lt", `created < 2026-01-01`, `(created < 2026-01-01)`},
		{"lte", `created <= 2026-01-01`, `(created <= 2026-01-01)`},

		// Case-insensitive field + keyword.
		{"upper-field", `STATUS = done`, `(status = done)`},
		{"upper-in", `status in (draft)`, `(status IN [draft])`},

		// Text terms.
		{"bare-text", `auth`, `"auth"`},
		{"quoted-text", `"two words"`, `"two words"`},
		{"known-field-no-op", `status`, `"status"`}, // field name with no op → text term

		// Quoted values.
		{"quoted-value", `title = "design approved"`, `(title = design approved)`},
		{"quoted-value-in-list", `project IN ("a b", c)`, `(project IN [a b c])`},

		// A reserved keyword is only a text_term or value when quoted (the
		// grammar's BARE rule forbids the bare form).
		{"quoted-keyword-text", `"and"`, `"and"`},
		{"quoted-keyword-value", `kind = "and"`, `(kind = and)`},
		{"quoted-keyword-in-list", `status IN (draft, "or")`, `(status IN [draft or])`},

		// Implicit AND (juxtaposition) and explicit AND, same precedence.
		{"implicit-and", `auth kind = audit`, `(AND "auth" (kind = audit))`},
		{"explicit-and", `auth AND kind = audit`, `(AND "auth" (kind = audit))`},
		{"two-clauses-implicit", `status = done kind = audit`, `(AND (status = done) (kind = audit))`},

		// OR is lower precedence than AND.
		{"or", `status = done OR status = draft`, `(OR (status = done) (status = draft))`},
		{"and-binds-tighter", `a AND b OR c`, `(OR (AND "a" "b") "c")`},
		{"or-with-and-right", `a OR b AND c`, `(OR "a" (AND "b" "c"))`},

		// NOT (unary boolean) binds tightest.
		{"not-clause", `NOT kind = audit`, `(NOT (kind = audit))`},
		{"not-text", `NOT auth`, `(NOT "auth")`},
		{"not-binds-tightest", `NOT a AND b`, `(AND (NOT "a") "b")`},
		{"double-not", `NOT NOT a`, `(NOT (NOT "a"))`},

		// Positional NOT IN vs unary NOT.
		{"notin-operator", `project NOT IN (a)`, `(project NOT IN [a])`},
		{"not-then-clause", `kind = roadmap NOT kind = audit`, `(AND (kind = roadmap) (NOT (kind = audit)))`},

		// Parentheses regroup.
		{"parens-or-then-and", `(a OR b) AND c`, `(AND (OR "a" "b") "c")`},
		{"parens-redundant", `(status = done)`, `(status = done)`},

		// Mixed real-world example from the spec.
		{"spec-example", `auth project IN (harness-deck, deck) AND kind = audit`,
			`(AND (AND "auth" (project IN [harness-deck deck])) (kind = audit))`},
		{"spec-created-not", `created >= -7d NOT kind = roadmap`,
			`(AND (created >= -7d) (NOT (kind = roadmap)))`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Parse(tc.in)
			if err != nil {
				t.Fatalf("Parse(%q) error: %v", tc.in, err)
			}
			if got := q.String(); got != tc.want {
				t.Errorf("Parse(%q).String() = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string // expected substring of the error message
	}{
		{"empty", ``, "empty"},
		{"whitespace", `   `, "empty"},
		{"unknown-field-with-op", `stauts = done`, `unknown field`},
		{"bad-op-for-field", `status ~ done`, "operator"},      // ~ not allowed on status
		{"created-bad-op", `created = 2026-01-01`, "operator"}, // = not allowed on created
		{"title-bad-op", `title IN (a, b)`, "operator"},        // IN not allowed on title
		{"missing-value", `status =`, "value"},
		{"missing-value-eof", `status = `, "value"},
		{"unbalanced-open", `(status = done`, "parenthes"},
		{"unbalanced-close", `status = done)`, "parenthes"},
		{"empty-list", `status IN ()`, "value"},
		{"unterminated-quote", `title = "open`, "quote"},
		{"created-bad-value", `created > nonsense`, "created"},
		{"dangling-and", `status = done AND`, ""},
		{"dangling-or", `a OR`, ""},
		{"dangling-not", `a AND NOT`, ""},

		// Reserved keywords (AND/OR/NOT/IN) are not valid bare text_terms.
		// Per the grammar's BARE rule a keyword in operand position is a parse
		// error, not a phantom literal-text leaf. (Regression: these used to
		// silently parse — e.g. `OR status = done` → (AND "OR" (status=done)).)
		{"keyword-and-alone", `AND`, ""},
		{"keyword-or-alone", `OR`, ""},
		{"keyword-not-alone", `NOT`, ""},
		{"keyword-in-alone", `IN`, ""},
		{"leading-or", `OR status = done`, ""},
		{"leading-and", `AND status = done`, ""},
		{"doubled-or", `a OR AND`, ""},
		{"doubled-and-explicit", `a AND OR`, ""},
		{"doubled-and-and", `a AND AND b`, ""},
		{"keyword-after-text", `auth OR`, ""},

		// Reserved keywords are not valid bare values either; they must be
		// quoted to be used literally. (Regression: `kind = and` used to parse
		// to the clause (kind = and) instead of erroring.)
		{"keyword-value-and", `kind = and`, "value"},
		{"keyword-value-or", `status = OR`, "value"},
		{"keyword-value-not", `kind = NOT`, "value"},
		{"keyword-value-in", `kind = IN`, "value"},
		{"keyword-in-list", `status IN (draft, OR)`, "value"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.in)
			if err == nil {
				t.Fatalf("Parse(%q) = nil error, want error", tc.in)
			}
			if tc.want != "" && !strings.Contains(err.Error(), tc.want) {
				t.Errorf("Parse(%q) error = %q, want substring %q", tc.in, err.Error(), tc.want)
			}
		})
	}
}

func TestMatchOperators(t *testing.T) {
	rec := func(fields map[string]string) fakeRecord {
		return fakeRecord{fields: fields, text: ""}
	}
	tests := []struct {
		name   string
		query  string
		fields map[string]string
		want   bool
	}{
		// = / != with case-insensitive fold.
		{"eq-match", `status = done`, map[string]string{"status": "done"}, true},
		{"eq-fold", `status = DONE`, map[string]string{"status": "done"}, true},
		{"eq-nomatch", `status = done`, map[string]string{"status": "draft"}, false},
		{"neq-match", `status != draft`, map[string]string{"status": "done"}, true},
		{"neq-nomatch", `status != done`, map[string]string{"status": "done"}, false},

		// ~ / !~ contains (case-insensitive).
		{"contains-match", `title ~ aud`, map[string]string{"title": "Readiness Audit"}, true},
		{"contains-nomatch", `title ~ xyz`, map[string]string{"title": "Readiness Audit"}, false},
		{"ncontains-match", `title !~ xyz`, map[string]string{"title": "Readiness Audit"}, true},
		{"ncontains-nomatch", `title !~ aud`, map[string]string{"title": "Readiness Audit"}, false},

		// IN / NOT IN.
		{"in-match", `status IN (draft, done)`, map[string]string{"status": "done"}, true},
		{"in-fold", `status IN (DRAFT, DONE)`, map[string]string{"status": "done"}, true},
		{"in-nomatch", `status IN (draft, answered)`, map[string]string{"status": "done"}, false},
		{"notin-match", `status NOT IN (draft, answered)`, map[string]string{"status": "done"}, true},
		{"notin-nomatch", `status NOT IN (draft, done)`, map[string]string{"status": "done"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.query, err)
			}
			if got := q.Match(rec(tc.fields), fixedNow); got != tc.want {
				t.Errorf("Match(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestMatchCreated(t *testing.T) {
	// fixedNow = 2026-06-15T12:00:00Z.
	tests := []struct {
		name    string
		query   string
		created string // RFC3339 Created field
		want    bool
	}{
		// Relative -7d → threshold 2026-06-08T12:00:00Z.
		{"within-7d", `created >= -7d`, "2026-06-10T00:00:00Z", true},
		{"older-than-7d", `created >= -7d`, "2026-06-01T00:00:00Z", false},
		{"exactly-on-7d-boundary", `created >= -7d`, "2026-06-08T12:00:00Z", true},
		{"lt-7d-older", `created < -7d`, "2026-06-01T00:00:00Z", true},
		{"lt-7d-newer", `created < -7d`, "2026-06-10T00:00:00Z", false},

		// Hours.
		{"within-24h", `created >= -24h`, "2026-06-15T06:00:00Z", true},
		{"older-than-24h", `created >= -24h`, "2026-06-13T00:00:00Z", false},

		// Weeks: -2w → 2026-06-01T12:00:00Z.
		{"within-2w", `created >= -2w`, "2026-06-05T00:00:00Z", true},
		{"older-than-2w", `created >= -2w`, "2026-05-20T00:00:00Z", false},

		// ISO date (local midnight; tests use UTC default location).
		{"iso-gt", `created > 2026-06-01`, "2026-06-10T00:00:00Z", true},
		{"iso-gt-false", `created > 2026-06-15`, "2026-06-10T00:00:00Z", false},
		{"iso-lte", `created <= 2026-06-10`, "2026-06-10T00:00:00Z", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.query, err)
			}
			r := fakeRecord{fields: map[string]string{"created": tc.created}}
			if got := q.Match(r, fixedNow); got != tc.want {
				t.Errorf("Match(%q) created=%s = %v, want %v", tc.query, tc.created, got, tc.want)
			}
		})
	}
}

func TestMatchCreatedUnparseable(t *testing.T) {
	// A report whose Created is not a parseable timestamp can never satisfy
	// a created comparison.
	q, err := Parse(`created >= -7d`)
	if err != nil {
		t.Fatal(err)
	}
	r := fakeRecord{fields: map[string]string{"created": "not-a-time"}}
	if q.Match(r, fixedNow) {
		t.Errorf("created comparison against unparseable Created should not match")
	}
}

func TestMatchBooleanShapes(t *testing.T) {
	tests := []struct {
		name   string
		query  string
		fields map[string]string
		text   string
		want   bool
	}{
		{"and-both-true", `status = done kind = audit`,
			map[string]string{"status": "done", "kind": "audit"}, "", true},
		{"and-one-false", `status = done kind = audit`,
			map[string]string{"status": "done", "kind": "review"}, "", false},
		{"or-one-true", `status = done OR status = draft`,
			map[string]string{"status": "draft"}, "", true},
		{"or-none-true", `status = done OR status = draft`,
			map[string]string{"status": "answered"}, "", false},
		{"not-true", `NOT status = draft`,
			map[string]string{"status": "done"}, "", true},
		{"not-false", `NOT status = draft`,
			map[string]string{"status": "draft"}, "", false},
		{"text-match", `auth`, map[string]string{}, "user authentication flow", true},
		{"text-nomatch", `auth`, map[string]string{}, "deployment pipeline", false},
		{"text-case-insensitive", `AUTH`, map[string]string{}, "user authentication", true},
		{"mixed-struct-and-text", `status = done auth`,
			map[string]string{"status": "done"}, "auth subsystem", true},
		{"mixed-struct-fail", `status = done auth`,
			map[string]string{"status": "draft"}, "auth subsystem", false},
		{"parens-group", `(status = done OR status = draft) kind = audit`,
			map[string]string{"status": "draft", "kind": "audit"}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.query, err)
			}
			r := fakeRecord{fields: tc.fields, text: tc.text}
			if got := q.Match(r, fixedNow); got != tc.want {
				t.Errorf("Match(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// TestLazyShortCircuit is the critical invariant: Text() must not be opened
// when a structural predicate short-circuits the evaluation.
func TestLazyShortCircuit(t *testing.T) {
	// status = done AND auth — status fails first, so the text leaf is
	// never reached and Text() must not be called.
	t.Run("and-structural-fails-first", func(t *testing.T) {
		var calls int
		q, err := Parse(`status = done auth`)
		if err != nil {
			t.Fatal(err)
		}
		r := fakeRecord{fields: map[string]string{"status": "draft"}, text: "auth", textCalls: &calls}
		if q.Match(r, fixedNow) {
			t.Fatalf("expected no match (status != done)")
		}
		if calls != 0 {
			t.Errorf("Text() called %d times; want 0 (structural predicate short-circuits)", calls)
		}
	})

	// OR short-circuits the other way: a true structural leaf on the left
	// means the text leaf on the right is never evaluated.
	t.Run("or-structural-true-first", func(t *testing.T) {
		var calls int
		q, err := Parse(`status = done OR auth`)
		if err != nil {
			t.Fatal(err)
		}
		r := fakeRecord{fields: map[string]string{"status": "done"}, text: "auth", textCalls: &calls}
		if !q.Match(r, fixedNow) {
			t.Fatalf("expected match (status = done)")
		}
		if calls != 0 {
			t.Errorf("Text() called %d times; want 0 (OR short-circuits on true left)", calls)
		}
	})

	// When the text leaf IS reached, Text() may be called — sanity that the
	// counter mechanism works and isn't spuriously zero.
	t.Run("text-leaf-reached-calls-text", func(t *testing.T) {
		var calls int
		q, err := Parse(`status = done auth`)
		if err != nil {
			t.Fatal(err)
		}
		r := fakeRecord{fields: map[string]string{"status": "done"}, text: "auth", textCalls: &calls}
		if !q.Match(r, fixedNow) {
			t.Fatalf("expected match")
		}
		if calls == 0 {
			t.Errorf("Text() never called though text leaf was reached")
		}
	})
}

func TestHasTextAndTextTerms(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantHas   bool
		wantTerms []string
	}{
		{"structural-only", `status = done kind = audit`, false, nil},
		{"single-text", `auth`, true, []string{"auth"}},
		{"mixed", `status = done auth deploy`, true, []string{"auth", "deploy"}},
		{"quoted-text", `"two words"`, true, []string{"two words"}},
		{"text-under-not", `NOT auth`, true, []string{"auth"}},
		{"created-not-text", `created >= -7d`, false, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tc.query, err)
			}
			if got := q.HasText(); got != tc.wantHas {
				t.Errorf("HasText(%q) = %v, want %v", tc.query, got, tc.wantHas)
			}
			got := q.TextTerms()
			if len(got) != len(tc.wantTerms) {
				t.Fatalf("TextTerms(%q) = %v, want %v", tc.query, got, tc.wantTerms)
			}
			for i := range got {
				if got[i] != tc.wantTerms[i] {
					t.Errorf("TextTerms(%q)[%d] = %q, want %q", tc.query, i, got[i], tc.wantTerms[i])
				}
			}
		})
	}
}

func TestQuotedTextPhraseMatch(t *testing.T) {
	// A quoted term matches the phrase verbatim (case-insensitive), not its
	// individual words.
	q, err := Parse(`"design approved"`)
	if err != nil {
		t.Fatal(err)
	}
	if !q.Match(fakeRecord{text: "Status: design approved 2026-06-15"}, fixedNow) {
		t.Errorf("quoted phrase should match contiguous occurrence")
	}
	if q.Match(fakeRecord{text: "design then later approved"}, fixedNow) {
		t.Errorf("quoted phrase should not match non-contiguous words")
	}
}

func TestSchema(t *testing.T) {
	// Schema is the single source of truth shared with the server's
	// /api/search/schema. Lock its canonical order and a representative op set so
	// drift from the parser's field matrix is caught here.
	got := Schema()
	wantOrder := []string{"status", "project", "kind", "harness", "title", "agent", "verdict", "created"}
	if len(got) != len(wantOrder) {
		t.Fatalf("Schema() returned %d fields, want %d", len(got), len(wantOrder))
	}
	for i, name := range wantOrder {
		if got[i].Name != name {
			t.Errorf("Schema()[%d].Name = %q, want %q", i, got[i].Name, name)
		}
	}
	// Every advertised op must actually be accepted by the parser for that field
	// (no autocomplete-suggests-what-parser-rejects drift). Build a valid value
	// for each op shape: list ops need parens, created needs a date-ish value.
	for _, f := range got {
		for _, op := range f.Ops {
			val := "x"
			if op == "IN" || op == "NOT IN" {
				val = "(x)"
			} else if f.Name == "created" {
				val = "-7d"
			}
			if _, err := Parse(f.Name + " " + op + " " + val); err != nil {
				t.Errorf("Schema advertises %s %s but parser rejects it: %v", f.Name, op, err)
			}
		}
	}
}
