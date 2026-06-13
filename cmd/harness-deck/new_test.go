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

// TestNewDefaultScaffoldValidates guards the no-template path (which the
// starterReport refactor still owns) through the strict validator, not just a
// structural map check.
func TestNewDefaultScaffoldValidates(t *testing.T) {
	central := runNew(t, []string{"--project", "demo", "--title", "t", "--id", "def"})
	data, err := os.ReadFile(filepath.Join(central, "demo", "def", "report.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	rep, err := manifest.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ps := rep.Validate(); len(ps) != 0 {
		t.Fatalf("default scaffold does not validate: %v", ps)
	}
}

// TestNewTemplatesValidate is the safety net for the hand-written template JSON
// in templates.go: every template must parse, strict-validate with zero
// problems, default its title and kind, and contain the block type that makes
// that template distinctive.
func TestNewTemplatesValidate(t *testing.T) {
	cases := []struct {
		name       string
		wantTitle  string
		wantBlocks int
		wantType   string // a distinctive block type that must be present
	}{
		{"audit", "Audit", 4, "table"},
		{"review", "Code review", 3, "approval"},
		{"progress", "Progress update", 2, "recommendations"},
		{"decision", "Decision", 2, "decision"},
		{"idea", "Idea", 2, "ask"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			central := runNew(t, []string{"--project", "demo", "--template", tc.name, "--id", "r-" + tc.name})
			data, err := os.ReadFile(filepath.Join(central, "demo", "r-"+tc.name, "report.json"))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			rep, err := manifest.Parse(data)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if ps := rep.Validate(); len(ps) != 0 {
				t.Fatalf("template %q does not validate: %v", tc.name, ps)
			}
			if rep.Kind != tc.name {
				t.Errorf("kind = %q, want %q (defaults to the template name)", rep.Kind, tc.name)
			}
			if rep.Title != tc.wantTitle {
				t.Errorf("title = %q, want %q (template default)", rep.Title, tc.wantTitle)
			}
			if len(rep.Blocks) != tc.wantBlocks {
				t.Errorf("blocks = %d, want %d", len(rep.Blocks), tc.wantBlocks)
			}
			found := false
			for _, b := range rep.Blocks {
				if b.Type == tc.wantType {
					found = true
				}
			}
			if !found {
				t.Errorf("template %q missing a %q block", tc.name, tc.wantType)
			}
		})
	}
}

// TestNewTemplateExplicitFlagsOverride confirms an explicit --title / --kind
// win over the template's defaults while the template's blocks still apply.
func TestNewTemplateExplicitFlagsOverride(t *testing.T) {
	central := runNew(t, []string{
		"--project", "demo", "--template", "decision", "--id", "ov",
		"--title", "Custom title", "--kind", "audit",
	})
	data, err := os.ReadFile(filepath.Join(central, "demo", "ov", "report.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	rep, err := manifest.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rep.Title != "Custom title" {
		t.Errorf("title = %q, want the explicit override", rep.Title)
	}
	if rep.Kind != "audit" {
		t.Errorf("kind = %q, want explicit audit (not the template name)", rep.Kind)
	}
	if len(rep.Blocks) != 2 {
		t.Errorf("blocks = %d, want the decision template's 2", len(rep.Blocks))
	}
}

// TestTemplateRegistrySync keeps templateOrder (drives --template help and the
// unknown-template error) and the reportTemplates map (drives lookup) from
// drifting — a template added to one but not the other is invisible in help or
// fails at use, and no other test would catch it. Mirrors TestRegistryCrossCheck.
func TestTemplateRegistrySync(t *testing.T) {
	if len(templateOrder) != len(reportTemplates) {
		t.Fatalf("templateOrder has %d entries, reportTemplates has %d", len(templateOrder), len(reportTemplates))
	}
	for _, name := range templateOrder {
		if _, ok := reportTemplates[name]; !ok {
			t.Errorf("templateOrder lists %q but reportTemplates has no such key", name)
		}
	}
	for name := range reportTemplates {
		found := false
		for _, n := range templateOrder {
			if n == name {
				found = true
			}
		}
		if !found {
			t.Errorf("reportTemplates has %q but templateOrder omits it (invisible in --template help)", name)
		}
	}
}

// TestNewTemplateWithOut confirms --template composes with an explicit --out
// path: the template's blocks land at the chosen file, independent of the
// central-dir routing the other tests exercise.
func TestNewTemplateWithOut(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"central_dir":"`+filepath.Join(dir, "reports")+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)
	out := filepath.Join(dir, "explicit", "report.json")
	cmdNew([]string{"--project", "demo", "--template", "audit", "--out", out})

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("expected file at --out path: %v", err)
	}
	rep, err := manifest.Parse(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ps := rep.Validate(); len(ps) != 0 {
		t.Fatalf("template via --out does not validate: %v", ps)
	}
	if len(rep.Blocks) != 4 {
		t.Errorf("audit template via --out: blocks = %d, want 4", len(rep.Blocks))
	}
}
