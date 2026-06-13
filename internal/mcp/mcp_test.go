package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// runOne pipes one JSON-RPC request line into a fresh server and returns
// the parsed response. The test helper takes care of the initialize
// handshake skipping — most tests just want to exercise one tool call.
func runOne(t *testing.T, cfg config.Config, reqLine string) Response {
	t.Helper()
	srv := New(cfg, "test")
	var out bytes.Buffer
	if err := srv.Run(context.Background(), strings.NewReader(reqLine+"\n"), &out); err != nil {
		t.Fatalf("server: %v", err)
	}
	// Take the first non-empty line as the response.
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var r Response
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("parse response %q: %v", line, err)
		}
		return r
	}
	t.Fatal("no response from server")
	return Response{}
}

func TestInitializeHandshake(t *testing.T) {
	r := runOne(t, config.Default(), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if r.Error != nil {
		t.Fatalf("initialize returned error: %+v", r.Error)
	}
	// Result is map[string]any when decoded into Response.Result (any).
	res, ok := r.Result.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T, want object", r.Result)
	}
	if res["protocolVersion"] != protocolVersion {
		t.Errorf("protocolVersion = %v, want %v", res["protocolVersion"], protocolVersion)
	}
	caps, _ := res["capabilities"].(map[string]any)
	if _, hasTools := caps["tools"]; !hasTools {
		t.Errorf("capabilities missing 'tools': %v", caps)
	}
	info, _ := res["serverInfo"].(map[string]any)
	if info["name"] != "harness-deck" {
		t.Errorf("serverInfo.name = %v", info["name"])
	}
}

func TestInitializeAdvertisesResourcesAndInstructions(t *testing.T) {
	r := runOne(t, config.Default(), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	if r.Error != nil {
		t.Fatalf("initialize returned error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	caps, _ := res["capabilities"].(map[string]any)
	if _, hasResources := caps["resources"]; !hasResources {
		t.Errorf("capabilities missing 'resources': %v", caps)
	}
	instr, _ := res["instructions"].(string)
	if instr == "" {
		t.Error("initialize result missing instructions string")
	}
	if !strings.Contains(instr, "harness-deck://contract") {
		t.Errorf("instructions should point at the contract resource: %q", instr)
	}
}

func TestResourcesListIncludesContract(t *testing.T) {
	r := runOne(t, config.Default(), `{"jsonrpc":"2.0","id":2,"method":"resources/list"}`)
	if r.Error != nil {
		t.Fatalf("resources/list error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	resources, _ := res["resources"].([]any)
	uris := map[string]bool{}
	for _, x := range resources {
		m := x.(map[string]any)
		uris[m["uri"].(string)] = true
	}
	for _, want := range []string{"harness-deck://contract", "harness-deck://publishing"} {
		if !uris[want] {
			t.Errorf("resources/list missing %q (have %v)", want, uris)
		}
	}
}

func TestResourcesReadContractReturnsSchema(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"harness-deck://contract"}}`
	r := runOne(t, config.Default(), body)
	if r.Error != nil {
		t.Fatalf("resources/read error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	contents, _ := res["contents"].([]any)
	if len(contents) == 0 {
		t.Fatal("resources/read returned no contents")
	}
	first := contents[0].(map[string]any)
	if first["mimeType"] != "text/markdown" {
		t.Errorf("mimeType = %v, want text/markdown", first["mimeType"])
	}
	text, _ := first["text"].(string)
	if !strings.Contains(text, "harness-deck/report@1") {
		t.Error("contract resource body missing schema marker")
	}
}

func TestResourcesReadUnknownURIErrors(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"harness-deck://nope"}}`
	r := runOne(t, config.Default(), body)
	if r.Error == nil {
		t.Errorf("expected an error for an unknown resource uri, got result %v", r.Result)
	}
}

func TestToolsListIncludesAllDefaults(t *testing.T) {
	r := runOne(t, config.Default(), `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if r.Error != nil {
		t.Fatalf("tools/list error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	tools, _ := res["tools"].([]any)
	names := map[string]bool{}
	for _, t := range tools {
		m := t.(map[string]any)
		names[m["name"].(string)] = true
	}
	for _, want := range []string{"publish_report", "validate_report", "get_responses", "list_reports", "update_status"} {
		if !names[want] {
			t.Errorf("tools/list missing %q (have %v)", want, names)
		}
	}
}

func TestValidateReportRejectsBadManifest(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"validate_report","arguments":{"manifest":{"schema":"harness-deck/report@1"}}}}`
	r := runOne(t, config.Default(), body)
	if r.Error != nil {
		t.Fatalf("protocol error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	if res["isError"] != true {
		t.Errorf("expected isError=true for incomplete manifest: %v", res)
	}
}

func TestValidateReportAcceptsValid(t *testing.T) {
	m := minimalManifest("acme", "run-1", "Hello")
	args, _ := json.Marshal(map[string]any{"manifest": json.RawMessage(m)})
	req := map[string]any{"jsonrpc": "2.0", "id": 4, "method": "tools/call", "params": map[string]any{"name": "validate_report", "arguments": json.RawMessage(args)}}
	line, _ := json.Marshal(req)
	r := runOne(t, config.Default(), string(line))
	if r.Error != nil {
		t.Fatalf("error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	if got, _ := res["isError"].(bool); got {
		// isError omits when false but if present it should be false
		t.Errorf("isError = true on valid manifest, content: %v", res["content"])
	}
}

func TestPublishReportWritesFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.CentralDir = dir // bypass ~/.harness/reports

	m := minimalManifest("acme", "run-42", "Publish me")
	args, _ := json.Marshal(map[string]any{"manifest": json.RawMessage(m)})
	req := map[string]any{"jsonrpc": "2.0", "id": 5, "method": "tools/call", "params": map[string]any{"name": "publish_report", "arguments": json.RawMessage(args)}}
	line, _ := json.Marshal(req)

	r := runOne(t, cfg, string(line))
	if r.Error != nil {
		t.Fatalf("protocol error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	if got, _ := res["isError"].(bool); got {
		t.Fatalf("publish failed: %v", res["content"])
	}

	want := filepath.Join(dir, "acme", "run-42", "report.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected file at %s: %v", want, err)
	}
}

func TestPublishReportPreservesInputBytes(t *testing.T) {
	// publish_report must write the publisher's own bytes through (only
	// re-indenting), not re-marshal the parsed struct. Re-marshaling *Report
	// would re-emit fields in struct-declaration order; writing the input
	// bytes preserves the publisher's authored order exactly, formatting
	// aside.
	dir := t.TempDir()
	cfg := config.Default()
	cfg.CentralDir = dir

	// Deliberately non-canonical field order: title and status lead, schema
	// trails — the opposite of the struct's declaration order.
	manifest := `{"title":"Order matters","status":"draft","schema":"harness-deck/report@1",` +
		`"id":"run-bytes","project":"acme","harness":"test",` +
		`"created":"2026-05-26T10:00:00Z",` +
		`"blocks":[{"type":"prose","markdown":"hello"}]}`
	args, _ := json.Marshal(map[string]any{"manifest": json.RawMessage(manifest)})
	req := map[string]any{"jsonrpc": "2.0", "id": 9, "method": "tools/call", "params": map[string]any{"name": "publish_report", "arguments": json.RawMessage(args)}}
	line, _ := json.Marshal(req)

	r := runOne(t, cfg, string(line))
	if r.Error != nil {
		t.Fatalf("protocol error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	if got, _ := res["isError"].(bool); got {
		t.Fatalf("publish failed: %v", res["content"])
	}

	data, err := os.ReadFile(filepath.Join(dir, "acme", "run-bytes", "report.json"))
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	// Re-indent the original input the same way the tool does and compare
	// byte-for-byte: identical means the publisher's exact bytes (and field
	// order) round-tripped, formatting aside. A struct re-marshal would lead
	// with "schema" and fail this.
	var want bytes.Buffer
	if err := json.Indent(&want, []byte(manifest), "", "  "); err != nil {
		t.Fatal(err)
	}
	want.WriteByte('\n')
	if string(data) != want.String() {
		t.Errorf("published bytes differ from indented input\n got: %s\nwant: %s", data, want.String())
	}
}

func TestPublishReportRejectsPathTraversal(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.CentralDir = dir

	// Project name contains "..": should be refused by safePathComponent.
	m := minimalManifest("../escape", "run-1", "Escape attempt")
	args, _ := json.Marshal(map[string]any{"manifest": json.RawMessage(m)})
	req := map[string]any{"jsonrpc": "2.0", "id": 6, "method": "tools/call", "params": map[string]any{"name": "publish_report", "arguments": json.RawMessage(args)}}
	line, _ := json.Marshal(req)

	r := runOne(t, cfg, string(line))
	if r.Error != nil {
		t.Fatalf("protocol error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	if got, _ := res["isError"].(bool); !got {
		// Note: the validate step may catch this first as a schema problem,
		// which is also fine — either layer refusing it is acceptable.
		t.Errorf("expected isError=true for traversal attempt, got %v", res)
	}
}

// minimalManifest builds the smallest valid report.json for tests.
func minimalManifest(project, id, title string) []byte {
	m := map[string]any{
		"schema":  "harness-deck/report@1",
		"id":      id,
		"project": project,
		"harness": "test",
		"title":   title,
		"status":  "draft",
		"created": "2026-05-26T10:00:00Z",
		"blocks": []any{
			map[string]any{"type": "prose", "markdown": "hello"},
		},
	}
	b, _ := json.Marshal(m)
	return b
}

func TestUpdateLivePreservesLargeIntegerLiterals(t *testing.T) {
	// update_live round-trips the whole document; decoding numbers into
	// float64 silently corrupts integers past 2^53 anywhere in the report.
	dir := t.TempDir()
	cfg := config.Default()
	cfg.CentralDir = dir

	runDir := filepath.Join(dir, "acme", "run-9")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `{"schema":"harness-deck/report@1","id":"run-9","project":"acme",` +
		`"harness":"claude-code","title":"big numbers","status":"draft",` +
		`"created":"2026-06-10T00:00:00Z","big_id":9007199254740993,"blocks":[]}`
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	args, _ := json.Marshal(map[string]any{"project": "acme", "run": "run-9", "step": "compiling"})
	req := map[string]any{"jsonrpc": "2.0", "id": 7, "method": "tools/call", "params": map[string]any{"name": "update_live", "arguments": json.RawMessage(args)}}
	line, _ := json.Marshal(req)

	r := runOne(t, cfg, string(line))
	if r.Error != nil {
		t.Fatalf("protocol error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	if got, _ := res["isError"].(bool); got {
		t.Fatalf("update_live failed: %v", res["content"])
	}

	data, err := os.ReadFile(filepath.Join(runDir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "9007199254740993") {
		t.Errorf("large integer mangled by update_live round-trip:\n%s", data)
	}
}

func TestListReportsSeesScanRootsProjects(t *testing.T) {
	// The dashboard scans projects discovered under scan_roots; list_reports
	// must see the same world, or an agent that publishes into a discovered
	// repo can't find its own report and concludes the publish failed.
	t.Setenv("HARNESS_DECK_CONFIG", filepath.Join(t.TempDir(), "config.json"))

	root := t.TempDir() // plays the role of ~/git
	proj := filepath.Join(root, "myproj")
	if err := os.MkdirAll(filepath.Join(proj, ".docs", "ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	runDir := filepath.Join(proj, ".harness", "run-7")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `{"schema":"harness-deck/report@1","id":"run-7","project":"myproj",` +
		`"harness":"claude-code","title":"in-repo report","status":"draft",` +
		`"created":"2026-06-10T00:00:00Z","blocks":[]}`
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.CentralDir = t.TempDir()
	cfg.ScanRoots = []string{root}
	cfg.Projects = nil

	req := map[string]any{"jsonrpc": "2.0", "id": 8, "method": "tools/call", "params": map[string]any{"name": "list_reports", "arguments": json.RawMessage(`{}`)}}
	line, _ := json.Marshal(req)
	r := runOne(t, cfg, string(line))
	if r.Error != nil {
		t.Fatalf("protocol error: %+v", r.Error)
	}
	res := r.Result.(map[string]any)
	if got, _ := res["isError"].(bool); got {
		t.Fatalf("list_reports failed: %v", res["content"])
	}
	body, _ := json.Marshal(res["content"])
	if !strings.Contains(string(body), "run-7") {
		t.Errorf("scan_roots-discovered report missing from list_reports:\n%s", body)
	}
}
