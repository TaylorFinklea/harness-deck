package query

import "strings"

// parser is a recursive-descent parser over the lexed token stream. It
// implements the grammar:
//
//	query   := or_expr
//	or_expr := and_expr ( OR and_expr )*
//	and_expr:= unary ( AND? unary )*      # juxtaposition = implicit AND
//	unary   := NOT unary | primary
//	primary := '(' or_expr ')' | clause | text_term
//	clause  := FIELD op value_or_list     # only when FIELD known AND op follows
//
// Precedence is NOT > AND > OR. Implicit AND (juxtaposition) shares AND's
// precedence. A known field is the start of a clause only when a valid
// operator follows it (lookahead); otherwise it is a text term.
type parser struct {
	toks []token
	pos  int
}

func (p *parser) peek() token { return p.toks[p.pos] }

func (p *parser) next() token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) atEOF() bool { return p.toks[p.pos].kind == tokEOF }

// parse is the entry point: a full or_expr that must consume every token.
func (p *parser) parse() (node, error) {
	n, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if !p.atEOF() {
		// The only token that can dangle here is an unmatched ')'.
		if p.peek().kind == tokRParen {
			return nil, errf("unbalanced parentheses")
		}
		return nil, errf("unexpected %q", p.peek().val)
	}
	return n, nil
}

// parseOr := and_expr ( OR and_expr )*
func (p *parser) parseOr() (node, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.isKeyword("OR") {
		p.next()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = orNode{left: left, right: right}
	}
	return left, nil
}

// parseAnd := unary ( AND? unary )* — explicit AND or juxtaposition, both the
// same precedence. It loops while another unary can start (a primary token or
// a leading NOT) so adjacency means implicit AND.
func (p *parser) parseAnd() (node, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for {
		if p.isKeyword("AND") {
			p.next()
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = andNode{left: left, right: right}
			continue
		}
		if p.startsUnary() {
			right, err := p.parseUnary()
			if err != nil {
				return nil, err
			}
			left = andNode{left: left, right: right}
			continue
		}
		break
	}
	return left, nil
}

// parseUnary := NOT unary | primary. A leading NOT here is boolean negation
// (positional NOT IN is handled inside clause parsing, after a field).
func (p *parser) parseUnary() (node, error) {
	if p.isKeyword("NOT") {
		p.next()
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return notNode{operand: operand}, nil
	}
	return p.parsePrimary()
}

// parsePrimary := '(' or_expr ')' | clause | text_term.
func (p *parser) parsePrimary() (node, error) {
	t := p.peek()
	switch t.kind {
	case tokLParen:
		p.next()
		inner, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, errf("unbalanced parentheses")
		}
		p.next()
		return inner, nil
	case tokBare, tokQuoted:
		return p.parseClauseOrText()
	case tokEOF:
		return nil, errf("expected a search term")
	default:
		return nil, errf("unexpected %q", t.val)
	}
}

// parseClauseOrText implements the field-vs-text disambiguation. A bare token
// that names a known field becomes a clause only when a valid operator follows
// it; otherwise (and always for quoted tokens) it is a text term.
func (p *parser) parseClauseOrText() (node, error) {
	t := p.peek()
	if t.kind == tokBare {
		if op, isOp := p.lookaheadOp(); isOp {
			fs, ok := knownField(t.val)
			if !ok {
				// A bare token in operator position that isn't a known field
				// is a typo, not a text term — surface it for the live hint.
				return nil, errf("unknown field %q", t.val)
			}
			return p.parseClause(fs, op)
		}
		// A reserved keyword (AND/OR/NOT/IN) cannot be a text_term: per the
		// grammar's BARE rule it is not a valid bare token, and reaching it
		// here means it sits where an operand was expected (e.g. a leading or
		// doubled connector). Surface a parse error for the live hint rather
		// than injecting a phantom literal-text leaf.
		if isAnyKeywordTok(t) {
			return nil, errf("unexpected %q", t.val)
		}
	}
	// Text term: a known field name with no operator after it (e.g. lone
	// "status") or any other non-keyword bare/quoted token.
	p.next()
	return textPred{term: t.val}, nil
}

// lookaheadOp inspects the token(s) after the current field token without
// consuming anything, returning the operator that would follow. It recognizes
// the symbol operators, the `IN` keyword, and the positional `NOT IN` pair.
func (p *parser) lookaheadOp() (operator, bool) {
	nxt := p.toks[p.pos+1]
	switch {
	case nxt.kind == tokOp:
		return operator(nxt.val), true
	case isKeywordTok(nxt, "IN"):
		return opIn, true
	case isKeywordTok(nxt, "NOT"):
		// Positional NOT only forms an operator as `NOT IN`.
		if isKeywordTok(p.toks[p.pos+2], "IN") {
			return opNotIn, true
		}
	}
	return "", false
}

// parseClause consumes FIELD op value_or_list. The field token and the
// operator tokens are consumed here; op is the operator lookaheadOp resolved.
func (p *parser) parseClause(fs fieldSpec, op operator) (node, error) {
	p.next() // field
	// Consume the operator tokens.
	switch op {
	case opIn:
		p.next() // IN
	case opNotIn:
		p.next() // NOT
		p.next() // IN
	default:
		p.next() // symbol operator
	}

	if !fs.opAllowed(op) {
		return nil, errf("operator %q is not valid for field %q", string(op), fs.name)
	}

	values, err := p.parseValueOrList(isListOp(op))
	if err != nil {
		return nil, err
	}

	if fs.name == "created" {
		if !validCreatedValue(values[0]) {
			return nil, errf("created value %q must be relative (-7d, -24h, -2w) or a date (YYYY-MM-DD)", values[0])
		}
	}

	return fieldPred{field: fs.name, op: op, values: values}, nil
}

// parseValueOrList parses a single value, or a parenthesized comma list when
// list is true (IN / NOT IN). A list must hold at least one value.
func (p *parser) parseValueOrList(list bool) ([]string, error) {
	if list {
		if p.peek().kind != tokLParen {
			return nil, errf("expected '(' to start a value list")
		}
		p.next()
		var values []string
		for {
			v, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			values = append(values, v)
			if p.peek().kind == tokComma {
				p.next()
				continue
			}
			break
		}
		if p.peek().kind != tokRParen {
			return nil, errf("unbalanced parentheses in value list")
		}
		p.next()
		return values, nil
	}
	v, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return []string{v}, nil
}

// parseValue consumes one BARE or QUOTED value token. Per the grammar's BARE
// rule (value := BARE | QUOTED, BARE excludes keywords), a bare keyword
// (AND/OR/NOT/IN) is not a valid value — it must be quoted to be used
// literally (`kind = "and"`). Quoted tokens carry any word verbatim; only
// structural tokens, EOF, and bare keywords are rejected.
func (p *parser) parseValue() (string, error) {
	t := p.peek()
	switch t.kind {
	case tokBare:
		if isAnyKeywordTok(t) {
			return "", errf("expected a value, got keyword %q (quote it to use as a literal)", t.val)
		}
		p.next()
		return t.val, nil
	case tokQuoted:
		p.next()
		return t.val, nil
	default:
		return "", errf("expected a value")
	}
}

// startsUnary reports whether the current token can begin a unary (and thus an
// implicit-AND continuation): a primary token or a leading NOT keyword. The
// binary connectors AND and OR never start a unary — they are handled by their
// own precedence loops, so juxtaposition must not swallow them as text terms.
func (p *parser) startsUnary() bool {
	t := p.peek()
	switch t.kind {
	case tokLParen, tokQuoted:
		return true
	case tokBare:
		// AND/OR are binary connectors, not unary starts.
		if isKeywordTok(t, "AND") || isKeywordTok(t, "OR") {
			return false
		}
		// A leading NOT begins a negated unary; any other bare token (field
		// or text) begins a primary.
		return true
	default:
		return false
	}
}

// isKeyword reports whether the current token is the given keyword
// (case-insensitive bare token).
func (p *parser) isKeyword(kw string) bool { return isKeywordTok(p.peek(), kw) }

// isKeywordTok reports whether t is a bare token equal to kw, case-insensitive.
func isKeywordTok(t token, kw string) bool {
	return t.kind == tokBare && strings.EqualFold(t.val, kw)
}

// isAnyKeywordTok reports whether t is one of the reserved keywords AND, OR,
// NOT, or IN (case-insensitive bare token). Per the grammar's BARE definition
// — "a run of non-space, non-paren, non-comma chars that isn't a keyword" — a
// keyword can never stand as a text_term or a value; only a quoted token can
// carry those words. This guards the two positions (operand and value) where a
// bare keyword would otherwise be silently swallowed as a literal.
func isAnyKeywordTok(t token) bool {
	if t.kind != tokBare {
		return false
	}
	return strings.EqualFold(t.val, "AND") ||
		strings.EqualFold(t.val, "OR") ||
		strings.EqualFold(t.val, "NOT") ||
		strings.EqualFold(t.val, "IN")
}
