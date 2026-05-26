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
