package query

import (
	"fmt"
	"strings"
)

// tokenKind classifies a lexed token.
type tokenKind int

const (
	tokEOF    tokenKind = iota
	tokBare             // unquoted run: keyword candidate, field, value, or text
	tokQuoted           // double-quoted string (already unescaped)
	tokOp               // an operator symbol: = != ~ !~ > >= < <=
	tokLParen           // (
	tokRParen           // )
	tokComma            // ,
)

// token is one lexed unit. For tokQuoted, quoted is true so the parser can
// distinguish a quoted value/text from a bare one (a quoted token is never a
// keyword or field, only a value or text term).
type token struct {
	kind   tokenKind
	val    string
	quoted bool
}

// lex tokenizes a query string. Quoted strings allow spaces and \" escapes;
// everything else is split on whitespace and the structural punctuation
// ( ) , and the operator characters = ! ~ > <. An unterminated quote is the
// only lexer-level error.
func lex(input string) ([]token, error) {
	var toks []token
	runes := []rune(input)
	i := 0
	n := len(runes)
	for i < n {
		c := runes[i]
		switch {
		case isSpace(c):
			i++
		case c == '(':
			toks = append(toks, token{kind: tokLParen, val: "("})
			i++
		case c == ')':
			toks = append(toks, token{kind: tokRParen, val: ")"})
			i++
		case c == ',':
			toks = append(toks, token{kind: tokComma, val: ","})
			i++
		case c == '"':
			s, next, err := lexQuoted(runes, i)
			if err != nil {
				return nil, err
			}
			toks = append(toks, token{kind: tokQuoted, val: s, quoted: true})
			i = next
		case isOpStart(c):
			op, next := lexOp(runes, i)
			toks = append(toks, token{kind: tokOp, val: op})
			i = next
		default:
			s, next := lexBare(runes, i)
			toks = append(toks, token{kind: tokBare, val: s})
			i = next
		}
	}
	toks = append(toks, token{kind: tokEOF})
	return toks, nil
}

// lexQuoted reads a double-quoted string starting at runes[i] (the opening
// quote). It returns the unescaped contents, the index just past the closing
// quote, and an error if the quote is never closed.
func lexQuoted(runes []rune, i int) (string, int, error) {
	var b strings.Builder
	i++ // skip opening quote
	n := len(runes)
	for i < n {
		c := runes[i]
		if c == '\\' && i+1 < n && runes[i+1] == '"' {
			b.WriteRune('"')
			i += 2
			continue
		}
		if c == '"' {
			return b.String(), i + 1, nil
		}
		b.WriteRune(c)
		i++
	}
	return "", i, fmt.Errorf("unterminated quote")
}

// lexOp reads an operator token starting at runes[i]. It greedily matches the
// two-character operators (!=, !~, >=, <=) before the single-character ones.
// A lone '!' not followed by '=' or '~' is returned as "!" so the parser can
// reject it as an unknown operator rather than the lexer swallowing context.
func lexOp(runes []rune, i int) (string, int) {
	n := len(runes)
	c := runes[i]
	if i+1 < n {
		two := string(c) + string(runes[i+1])
		switch two {
		case "!=", "!~", ">=", "<=":
			return two, i + 2
		}
	}
	return string(c), i + 1
}

// lexBare reads a bare token: a run of characters that are not whitespace,
// parentheses, commas, quotes, or operator-start characters.
func lexBare(runes []rune, i int) (string, int) {
	start := i
	n := len(runes)
	for i < n {
		c := runes[i]
		if isSpace(c) || c == '(' || c == ')' || c == ',' || c == '"' || isOpStart(c) {
			break
		}
		i++
	}
	return string(runes[start:i]), i
}

func isSpace(c rune) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' }

// isOpStart reports whether c begins an operator symbol. '-' is deliberately
// excluded so relative created values like "-7d" lex as a single bare token.
func isOpStart(c rune) bool {
	switch c {
	case '=', '!', '~', '>', '<':
		return true
	}
	return false
}
