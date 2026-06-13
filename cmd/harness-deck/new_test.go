package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/manifest"
)

// runNew invokes cmdNew with args inside a fresh tempdir-isolated config
// and returns the path the file landed at.
func runNew(t *testing.T, args []string) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	central := filepath.Join(dir, "reports")
	if err := os.WriteFile(cfgPath, []byte(`{"central_dir":"`+central+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)
	cmdNew(args)
	return central
}

func TestNewScaffoldsValidManifest(t *testing.T) {
	central := runNew(t, []string{"--project", "demo", "--title", "first", "--id", "r1"})
	target := filepath.Join(central, "demo", "r1", "report.json")
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("expected file at %s: %v", target, err)
	}
	var rep map[string]any
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rep["schema"] != manifest.Schema {
		t.Errorf("schema = %v", rep["schema"])
	}
	if rep["project"] != "demo" || rep["title"] != "first" || rep["id"] != "r1" {
		t.Errorf("identifiers = %+v", rep)
	}
	if rep["status"] != "draft" || rep["kind"] != "progress" || rep["harness"] != "manual" {
		t.Errorf("defaults wrong = %+v", rep)
	}
	blocks, ok := rep["blocks"].([]any)
	if !ok || len(blocks) != 1 {
		t.Fatalf("blocks = %v", rep["blocks"])
	}
	block := blocks[0].(map[string]any)
	if block["type"] != "prose" {
		t.Errorf("first block should be prose, got %v", block["type"])
	}
}

func TestNewAgentFieldOmittedWhenEmpty(t *testing.T) {
	central := runNew(t, []string{"--project", "demo", "--title", "t", "--id", "r2"})
	data, _ := os.ReadFile(filepath.Join(central, "demo", "r2", "report.json"))
	var rep map[string]any
	_ = json.Unmarshal(data, &rep)
	if _, present := rep["agent"]; present {
		t.Errorf("agent should be omitted when --agent is empty; got %v", rep["agent"])
	}
}

func TestNewAgentFieldPresentWhenSet(t *testing.T) {
	central := runNew(t, []string{"--project", "demo", "--title", "t", "--id", "r3", "--agent", "claude-opus-4-7"})
	data, _ := os.ReadFile(filepath.Join(central, "demo", "r3", "report.json"))
	var rep map[string]any
	_ = json.Unmarshal(data, &rep)
	if rep["agent"] != "claude-opus-4-7" {
		t.Errorf("agent = %v", rep["agent"])
	}
}
