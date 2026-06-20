// Package query implements a JQL-like search query language for harness-deck:
// a lexer, a recursive-descent parser, a predicate AST, and a lazy
// short-circuit evaluator. It is pure and stdlib-only — it evaluates against
// the Record interface, never importing internal/store or internal/server, so
// it is unit-testable with fake records.
//
// A query mixes structural field filters with free text. Structural clauses
// (`status = done`, `project IN (a,b)`, `created >= -7d`) compare a report's
// indexed fields; text terms (`auth`, `"two words"`) match the report's
// searchable body+metadata text. The two combine with boolean operators whose
// precedence is NOT > AND > OR, where juxtaposition is an implicit AND. The
// evaluator is short-circuit: a structural predicate that fails first never
// reaches a text leaf, so the caller's lazy Text() is never opened.
package query

import (
	"fmt"
	"strings"
	"time"
)

// Record is the per-report view the evaluator sees. Field returns an indexed
// metadata value (cheap); Text returns the metadata+body searchable text and
// is expected to be fetched lazily and memoized by the caller, since it can
// require opening the full report.
type Record interface {
	// Field returns the value for one of the known field names:
	// "status","project","kind","scope","harness","title","agent","verdict","created".
	Field(name string) string
	// Text returns the metadata+body searchable text (lazy).
	Text() string
}

// Query is a compiled, evaluable query.
type Query struct {
	root node
}

// Parse compiles a query string into an evaluable Query. Empty or
// whitespace-only input is an error (callers special-case "" before calling).
// Invalid or partially-typed queries return a typed *Error carrying a short,
// human-readable message for a live-typing hint.
func Parse(q string) (Query, error) {
	if strings.TrimSpace(q) == "" {
		return Query{}, &Error{Msg: "empty query"}
	}
	toks, err := lex(q)
	if err != nil {
		return Query{}, &Error{Msg: err.Error()}
	}
	p := &parser{toks: toks}
	root, err := p.parse()
	if err != nil {
		return Query{}, err
	}
	return Query{root: root}, nil
}

// Match reports whether rec satisfies the query. now resolves relative
// `created` thresholds. Evaluation is short-circuit: a text leaf only opens
// rec.Text() when it is actually reached, so a structural predicate that fails
// first never reads the body.
func (q Query) Match(rec Record, now time.Time) bool {
	if q.root == nil {
		return false
	}
	return q.root.eval(rec, now)
}

// String renders the AST in a stable, parenthesized form for tests and
// debugging. Clauses render as "(field op value)" or "(field op [v1 v2])" for
// lists; text terms as a quoted string; boolean nodes as prefix forms
// "(AND a b)", "(OR a b)", "(NOT a)".
func (q Query) String() string {
	if q.root == nil {
		return ""
	}
	return q.root.String()
}

// HasText reports whether the query contains any text_term leaf. The server
// uses it to decide whether to score+snippet matches or order purely by
// recency.
func (q Query) HasText() bool { return len(collectText(q.root)) > 0 }

// TextTerms returns the text-term leaves in source order, for snippet capture
// by the server.
func (q Query) TextTerms() []string { return collectText(q.root) }

// Error is the typed parse error. Its message is short and human-readable so
// the client can surface it as a live-typing hint.
type Error struct {
	Msg string
}

func (e *Error) Error() string { return e.Msg }

// ----- fields & operator validity -----

// operator is one comparison operator in a clause.
type operator string

const (
	opEq    operator = "="
	opNe    operator = "!="
	opTilde operator = "~"
	opNTild operator = "!~"
	opGt    operator = ">"
	opGe    operator = ">="
	opLt    operator = "<"
	opLe    operator = "<="
	opIn    operator = "IN"
	opNotIn operator = "NOT IN"
)

// fieldSpec describes a known query field: the Record field name it reads and
// the set of operators valid on it. Order in allowedOps is the canonical order
// surfaced to autocomplete.
type fieldSpec struct {
	name       string
	allowedOps []operator
}

// fields is the field/operator matrix. Field names are lowercase; lookups
// lower-case the input first (field names are case-insensitive).
var fields = map[string]fieldSpec{
	"status":  {"status", []operator{opEq, opNe, opIn, opNotIn}},
	"project": {"project", []operator{opEq, opNe, opTilde, opNTild, opIn, opNotIn}},
	"kind":    {"kind", []operator{opEq, opNe, opTilde, opNTild, opIn, opNotIn}},
	"scope":   {"scope", []operator{opEq, opNe, opTilde, opNTild, opIn, opNotIn}},
	"harness": {"harness", []operator{opEq, opNe, opTilde, opNTild, opIn, opNotIn}},
	"title":   {"title", []operator{opTilde, opNTild, opEq, opNe}},
	"agent":   {"agent", []operator{opEq, opNe, opTilde, opNTild, opIn, opNotIn}},
	"verdict": {"verdict", []operator{opEq, opNe, opTilde, opNTild, opIn, opNotIn}},
	"created": {"created", []operator{opGt, opGe, opLt, opLe}},
}

// fieldOrder is the canonical field order surfaced to autocomplete (Schema).
var fieldOrder = []string{"status", "project", "kind", "scope", "harness", "title", "agent", "verdict", "created"}

// FieldSchema is one field's public autocomplete vocabulary: its name and the
// operators valid on it, in canonical order.
type FieldSchema struct {
	Name string
	Ops  []string
}

// Schema returns the field/operator matrix in canonical order. It is the single
// source of truth for both parsing (the same fields map drives validation) and
// the server's /api/search/schema autocomplete vocabulary, so the two can never
// drift — adding a field here updates the parser and autocomplete together.
func Schema() []FieldSchema {
	out := make([]FieldSchema, 0, len(fieldOrder))
	for _, name := range fieldOrder {
		fs := fields[name]
		ops := make([]string, len(fs.allowedOps))
		for i, op := range fs.allowedOps {
			ops[i] = string(op)
		}
		out = append(out, FieldSchema{Name: name, Ops: ops})
	}
	return out
}

// knownField reports whether name (case-insensitive) is a query field, and
// returns its spec.
func knownField(name string) (fieldSpec, bool) {
	fs, ok := fields[strings.ToLower(name)]
	return fs, ok
}

// opAllowed reports whether op is valid for the field.
func (fs fieldSpec) opAllowed(op operator) bool {
	for _, a := range fs.allowedOps {
		if a == op {
			return true
		}
	}
	return false
}

// isListOp reports whether op takes a parenthesized value list.
func isListOp(op operator) bool { return op == opIn || op == opNotIn }

// ----- AST -----

// node is one node in the predicate AST.
type node interface {
	eval(rec Record, now time.Time) bool
	String() string
}

// andNode is a short-circuit conjunction: left then right.
type andNode struct{ left, right node }

func (n andNode) eval(rec Record, now time.Time) bool {
	return n.left.eval(rec, now) && n.right.eval(rec, now)
}
func (n andNode) String() string { return "(AND " + n.left.String() + " " + n.right.String() + ")" }

// orNode is a short-circuit disjunction: left then right.
type orNode struct{ left, right node }

func (n orNode) eval(rec Record, now time.Time) bool {
	return n.left.eval(rec, now) || n.right.eval(rec, now)
}
func (n orNode) String() string { return "(OR " + n.left.String() + " " + n.right.String() + ")" }

// notNode is boolean negation of its operand.
type notNode struct{ operand node }

func (n notNode) eval(rec Record, now time.Time) bool { return !n.operand.eval(rec, now) }
func (n notNode) String() string                      { return "(NOT " + n.operand.String() + ")" }

// fieldPred is a structural clause: field op value(s).
type fieldPred struct {
	field  string // lowercase field name
	op     operator
	values []string
}

func (n fieldPred) String() string {
	if isListOp(n.op) {
		return "(" + n.field + " " + string(n.op) + " [" + strings.Join(n.values, " ") + "])"
	}
	return "(" + n.field + " " + string(n.op) + " " + strings.Join(n.values, " ") + ")"
}

// textPred is a free-text leaf matched against rec.Text().
type textPred struct{ term string }

func (n textPred) String() string { return "\"" + n.term + "\"" }

func (n textPred) eval(rec Record, now time.Time) bool {
	return strings.Contains(strings.ToLower(rec.Text()), strings.ToLower(n.term))
}

// collectText walks the AST and returns its text-term leaves in source order.
func collectText(n node) []string {
	var out []string
	var walk func(node)
	walk = func(n node) {
		switch v := n.(type) {
		case andNode:
			walk(v.left)
			walk(v.right)
		case orNode:
			walk(v.left)
			walk(v.right)
		case notNode:
			walk(v.operand)
		case textPred:
			out = append(out, v.term)
		}
	}
	walk(n)
	return out
}

// errf builds a typed parse error.
func errf(format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}
