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

// TestLoadDefaultsBind ensures the bind address falls back to 127.0.0.1
// when not set in the config file (preserving the local-only default).
func TestLoadDefaultsBind(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Bind != "127.0.0.1" {
		t.Errorf("Bind = %q, want %q", c.Bind, "127.0.0.1")
	}
}

// TestLoadReadsBindAndTLS verifies the new mobile-facing fields round-trip
// from the config file.
func TestLoadReadsBindAndTLS(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	body := `{"bind":"0.0.0.0","tls":{"cert":"/x/cert.pem","key":"/x/key.pem"}}`
	if err := os.WriteFile(cfgPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Bind != "0.0.0.0" {
		t.Errorf("Bind = %q", c.Bind)
	}
	if !c.TLS.Enabled() {
		t.Errorf("TLS.Enabled() = false, want true")
	}
	if c.TLS.Cert != "/x/cert.pem" || c.TLS.Key != "/x/key.pem" {
		t.Errorf("TLS = %+v", c.TLS)
	}
}
