package manifest

import (
	"strings"
	"testing"
)

// minimalReport returns a minimal valid report JSON with the given schema value.
func minimalReport(schema string) string {
	return `{"schema":"` + schema + `","id":"x","project":"p","harness":"h",` +
		`"title":"t","status":"draft","created":"2026-06-10T00:00:00Z",` +
		`"blocks":[{"type":"prose","markdown":"hello"}]}`
}

// TestParseSchemaFamily verifies that Parse accepts the canonical schema value
// and also accepts a higher version of the same family (forward-compat lenient parse).
func TestParseSchemaFamily(t *testing.T) {
	// Canonical version must parse cleanly.
	r, err := Parse([]byte(minimalReport(Schema)))
	if err != nil {
		t.Fatalf("Parse canonical schema: %v", err)
	}
	if r.Schema != Schema {
		t.Errorf("Schema = %q, want %q", r.Schema, Schema)
	}

	// A higher version of the same family must also parse (lenient).
	v2 := schemaFamily(Schema) + "@2"
	r2, err := Parse([]byte(minimalReport(v2)))
	if err != nil {
		t.Fatalf("Parse v2 schema: %v", err)
	}
	if r2.Schema != v2 {
		t.Errorf("v2 Schema = %q, want %q", r2.Schema, v2)
	}
}

// TestParseDifferentFamilyErrors verifies that Parse rejects a completely
// different schema family (not just a different version).
func TestParseDifferentFamilyErrors(t *testing.T) {
	_, err := Parse([]byte(minimalReport("other-tool/report@1")))
	if err == nil {
		t.Error("Parse should return an error for a different schema family")
	}
}

// TestValidateAcceptsOnlyKnownVersion verifies that Validate is strict:
// only the accepted version set passes; a higher version of the same
// family fails with a message that tells the user to upgrade.
func TestValidateAcceptsOnlyKnownVersion(t *testing.T) {
	// Canonical version must pass Validate.
	r, err := Parse([]byte(minimalReport(Schema)))
	if err != nil {
		t.Fatalf("Parse canonical: %v", err)
	}
	if ps := r.Validate(); hasProblem(ps, "schema") {
		t.Errorf("Validate flagged canonical schema: %v", ps)
	}

	// A higher version of the same family must FAIL Validate with an
	// actionable "newer than this binary" message.
	v2 := schemaFamily(Schema) + "@2"
	r2, err := Parse([]byte(minimalReport(v2)))
	if err != nil {
		t.Fatalf("Parse v2 schema: %v", err)
	}
	ps2 := r2.Validate()
	if !hasProblem(ps2, "newer than this binary") {
		t.Errorf("Validate should flag newer schema with upgrade hint; got: %v", ps2)
	}
}

// TestForwardCompatV2ParseAndFallback verifies the forward-compat contract:
// a @2 document with one known block and one unknown block type must parse
// successfully (lenient), and the unknown block must produce a fallback panel
// (Body == nil) while the known block decodes normally.
func TestForwardCompatV2ParseAndFallback(t *testing.T) {
	v2 := schemaFamily(Schema) + "@2"
	src := `{"schema":"` + v2 + `","id":"x","project":"p","harness":"h",` +
		`"title":"t","status":"draft","created":"2026-06-10T00:00:00Z",` +
		`"blocks":[` +
		`{"type":"prose","markdown":"known"},` +
		`{"type":"future-widget","spin":"up"}` +
		`]}`

	r, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("Parse v2 forward-compat: %v", err)
	}
	if len(r.Blocks) != 2 {
		t.Fatalf("want 2 blocks, got %d", len(r.Blocks))
	}
	if _, ok := r.Blocks[0].Body.(*ProseBlock); !ok {
		t.Errorf("block 0: got %T, want *ProseBlock", r.Blocks[0].Body)
	}
	if r.Blocks[1].Body != nil {
		t.Errorf("block 1 (unknown type): Body should be nil (fallback panel), got %T", r.Blocks[1].Body)
	}

	// Validate should flag the unknown block type AND the newer schema.
	ps := r.Validate()
	if !hasProblem(ps, "unknown block type") {
		t.Error("Validate should flag the unknown block type in a v2 document")
	}
	if !hasProblem(ps, "newer than this binary") {
		t.Error("Validate should flag the newer schema version")
	}
}

// TestSchemaFamilyHelper verifies that schemaFamily correctly extracts the
// family portion from a family@N string.
func TestSchemaFamilyHelper(t *testing.T) {
	// Derive expected family from the canonical constant — never hardcode.
	want := strings.Split(Schema, "@")[0]
	got := schemaFamily(Schema)
	if got != want {
		t.Errorf("schemaFamily(%q) = %q, want %q", Schema, got, want)
	}

	if got2 := schemaFamily("other-tool/report@3"); got2 != "other-tool/report" {
		t.Errorf("schemaFamily(other-tool/report@3) = %q, want other-tool/report", got2)
	}
}
