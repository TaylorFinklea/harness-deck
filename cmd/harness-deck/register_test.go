package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestRegisterAddsAndRemoves(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"central_dir":"/x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(dir, "proj")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	cmdRegister([]string{projectDir})
	cmdRegister([]string{projectDir}) // idempotent re-add — should not error

	got, _ := loadConfigMap(cfgPath)
	projs := stringSlice(got["projects"])
	if len(projs) != 1 || projs[0] != projectDir {
		t.Fatalf("after add: projects = %v", projs)
	}
	if got["central_dir"] != "/x" {
		t.Errorf("central_dir was clobbered: %v", got["central_dir"])
	}

	cmdRegister([]string{"--remove", projectDir})
	got, _ = loadConfigMap(cfgPath)
	projs = stringSlice(got["projects"])
	if len(projs) != 0 {
		t.Errorf("after remove: projects = %v", projs)
	}
}

func TestRegisterPreservesUnknownFields(t *testing.T) {
	// register reads and writes the config as a generic map so it never
	// loses fields it doesn't recognize (forward-compat with future schema
	// additions like `tls.*` or scan_roots).
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	body := `{"central_dir":"/x","scan_roots":["~/git"],"bind":"0.0.0.0","tls":{"cert":"/c","key":"/k"}}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(dir, "proj")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	cmdRegister([]string{projectDir})

	got, _ := loadConfigMap(cfgPath)
	for _, k := range []string{"central_dir", "scan_roots", "bind", "tls"} {
		if _, ok := got[k]; !ok {
			t.Errorf("register dropped %q from the config", k)
		}
	}
	if data, _ := json.Marshal(got["tls"]); string(data) != `{"cert":"/c","key":"/k"}` {
		t.Errorf("tls = %s", data)
	}
}

func TestLoadConfigMapRefusesCorruptConfig(t *testing.T) {
	// A corrupt config must abort the register command — degrading to an
	// empty map would make cmdRegister rewrite the user's config with only
	// a "projects" key.
	cfgPath := filepath.Join(t.TempDir(), "config.json")
	corrupt := `{"central_dir":"/x",}` // trailing comma
	if err := os.WriteFile(cfgPath, []byte(corrupt), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := loadConfigMap(cfgPath); err == nil {
		t.Fatal("loadConfigMap on a corrupt file should error, not degrade to empty")
	}
	// A genuinely missing file is still the start-fresh case.
	m, err := loadConfigMap(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("missing file should yield an empty map, got %v", m)
	}
}
