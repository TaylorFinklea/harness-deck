package query

import (
	"strconv"
	"strings"
	"time"
)

// eval evaluates a structural clause against rec. The created field is a time
// comparison; list fields (e.g. tags) use existential semantics; all other
// fields are string comparisons per the value-semantics table (= EqualFold,
// ~ Contains, IN fold-against-list, …).
func (n fieldPred) eval(rec Record, now time.Time) bool {
	if n.field == "created" {
		return evalCreated(rec.Field("created"), n.op, n.values[0], now)
	}
	if listFields[n.field] {
		return evalList(rec.Fields(n.field), n.op, n.values)
	}
	got := rec.Field(n.field)
	switch n.op {
	case opEq:
		return strings.EqualFold(got, n.values[0])
	case opNe:
		return !strings.EqualFold(got, n.values[0])
	case opTilde:
		return strings.Contains(strings.ToLower(got), strings.ToLower(n.values[0]))
	case opNTild:
		return !strings.Contains(strings.ToLower(got), strings.ToLower(n.values[0]))
	case opIn:
		return foldIn(got, n.values)
	case opNotIn:
		return !foldIn(got, n.values)
	}
	return false
}

// evalList applies existential semantics to a multi-value field. For positive
// operators (=, ~, IN) the clause matches when ANY value in vals satisfies it;
// for negative operators (!=, !~, NOT IN) the clause matches when NO value
// satisfies the positive counterpart (i.e. every value must fail).
func evalList(vals []string, op operator, queryVals []string) bool {
	switch op {
	case opEq:
		for _, v := range vals {
			if strings.EqualFold(v, queryVals[0]) {
				return true
			}
		}
		return false
	case opNe:
		for _, v := range vals {
			if strings.EqualFold(v, queryVals[0]) {
				return false
			}
		}
		return true
	case opTilde:
		lower := strings.ToLower(queryVals[0])
		for _, v := range vals {
			if strings.Contains(strings.ToLower(v), lower) {
				return true
			}
		}
		return false
	case opNTild:
		lower := strings.ToLower(queryVals[0])
		for _, v := range vals {
			if strings.Contains(strings.ToLower(v), lower) {
				return false
			}
		}
		return true
	case opIn:
		for _, v := range vals {
			if foldIn(v, queryVals) {
				return true
			}
		}
		return false
	case opNotIn:
		for _, v := range vals {
			if foldIn(v, queryVals) {
				return false
			}
		}
		return true
	}
	return false
}

// foldIn reports whether got case-insensitively equals any value in list.
func foldIn(got string, list []string) bool {
	for _, v := range list {
		if strings.EqualFold(got, v) {
			return true
		}
	}
	return false
}

// evalCreated parses the report's Created (RFC3339) and the clause value into
// times and applies op. An unparseable Created never satisfies the comparison
// (a report with no usable timestamp is excluded rather than matched).
func evalCreated(created string, op operator, value string, now time.Time) bool {
	c, err := time.Parse(time.RFC3339, created)
	if err != nil {
		return false
	}
	threshold, ok := resolveCreatedValue(value, now)
	if !ok {
		return false
	}
	switch op {
	case opGt:
		return c.After(threshold)
	case opGe:
		return !c.Before(threshold)
	case opLt:
		return c.Before(threshold)
	case opLe:
		return !c.After(threshold)
	}
	return false
}

// resolveCreatedValue resolves a created clause value into an absolute time.
// The value is either relative "-<N>[h|d|w]" (resolved against now) or an ISO
// date "YYYY-MM-DD" (treated as local midnight in now's location). ok is false
// when the value is neither form — parseCreatedValue at parse time already
// rejects those, so this is a defensive double-check.
func resolveCreatedValue(value string, now time.Time) (time.Time, bool) {
	if d, ok := parseRelative(value); ok {
		return now.Add(d), true
	}
	if t, ok := parseISODate(value, now.Location()); ok {
		return t, true
	}
	return time.Time{}, false
}

// parseRelative parses "-<N>[h|d|w]" into a (negative) duration offset. The
// leading minus is required (created values point into the past). Returns ok
// false if the form does not match.
func parseRelative(value string) (time.Duration, bool) {
	if len(value) < 3 || value[0] != '-' {
		return 0, false
	}
	unit := value[len(value)-1]
	numStr := value[1 : len(value)-1]
	n, err := strconv.Atoi(numStr)
	if err != nil || n < 0 {
		return 0, false
	}
	var per time.Duration
	switch unit {
	case 'h':
		per = time.Hour
	case 'd':
		per = 24 * time.Hour
	case 'w':
		per = 7 * 24 * time.Hour
	default:
		return 0, false
	}
	return -time.Duration(n) * per, true
}

// parseISODate parses "YYYY-MM-DD" as local midnight in loc. Returns ok false
// if the string is not a valid ISO date.
func parseISODate(value string, loc *time.Location) (time.Time, bool) {
	t, err := time.ParseInLocation("2006-01-02", value, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// validCreatedValue reports whether value is an acceptable created clause
// value (relative or ISO date). Used by the parser to reject malformed values
// at parse time so the live-typing hint is accurate.
func validCreatedValue(value string) bool {
	if _, ok := parseRelative(value); ok {
		return true
	}
	_, ok := parseISODate(value, time.UTC)
	return ok
}
