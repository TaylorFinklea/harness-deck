package manifest

import (
	"fmt"
	"strconv"
	"strings"
)

// schemaFamily extracts the family portion from a "family@N" schema string.
// For "harness-deck/report@1" it returns "harness-deck/report".
// If the string has no "@" it is returned as-is.
func schemaFamily(s string) string {
	if i := strings.LastIndex(s, "@"); i >= 0 {
		return s[:i]
	}
	return s
}

// schemaVersion extracts the integer version from a "family@N" string.
// Returns -1 if the version segment is absent or not a positive integer.
func schemaVersion(s string) int {
	i := strings.LastIndex(s, "@")
	if i < 0 || i == len(s)-1 {
		return -1
	}
	n, err := strconv.Atoi(s[i+1:])
	if err != nil || n < 1 {
		return -1
	}
	return n
}

// acceptedSchemas is the set of schema values that Validate accepts.
// Parse is lenient (any version of the canonical family parses); Validate
// is strict (only this set passes).
var acceptedSchemas = map[string]bool{
	Schema: true,
}

// canonicalFamily is the schema family derived from the canonical Schema
// constant — computed once at init so it is never hardcoded separately.
var canonicalFamily = schemaFamily(Schema)

// checkSchema validates the schema field for Parse (lenient) and returns a
// non-nil error only when the document cannot be meaningfully decoded (wrong
// family). A higher version of the same family is allowed through so a stale
// binary still renders known blocks and falls back on unknown ones.
func checkSchemaLenient(schema string) error {
	if schema == "" {
		// Missing schema: let the caller degrade gracefully or let Validate
		// report it; not a fatal parse error.
		return nil
	}
	f := schemaFamily(schema)
	if f != canonicalFamily {
		return fmt.Errorf("manifest: unknown schema family %q (expected %q)", f, canonicalFamily)
	}
	return nil
}

// checkSchemaStrict returns Validate-level problems for the schema field.
func checkSchemaStrict(schema string) []Problem {
	var ps []Problem
	if schema == "" {
		ps = append(ps, Problem{"schema", "missing"})
		return ps
	}
	if acceptedSchemas[schema] {
		return nil // canonical version — OK
	}
	f := schemaFamily(schema)
	if f != canonicalFamily {
		ps = append(ps, Problem{"schema", fmt.Sprintf("expected %q, got %q", Schema, schema)})
		return ps
	}
	// Same family, but a version we don't know — give an actionable message.
	v := schemaVersion(schema)
	cv := schemaVersion(Schema)
	if v > cv {
		ps = append(ps, Problem{"schema", fmt.Sprintf(
			"schema version %d is newer than this binary supports (max %d) — upgrade harness-deck",
			v, cv,
		)})
	} else {
		// Older or malformed version of the same family.
		ps = append(ps, Problem{"schema", fmt.Sprintf("expected %q, got %q", Schema, schema)})
	}
	return ps
}
