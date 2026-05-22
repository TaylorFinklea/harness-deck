package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// TestLoadReadsScanRoots checks that Load populates ScanRoots from the
// scan_roots field of the config file.
func TestLoadReadsScanRoots(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"scan_roots":["~/git","/tmp/projects"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"~/git", "/tmp/projects"}
	if !reflect.DeepEqual(c.ScanRoots, want) {
		t.Errorf("ScanRoots = %v, want %v", c.ScanRoots, want)
	}
}
