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

// TestLoadBeadsConfig checks the opt-in beads gate parses from config.
func TestLoadBeadsConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"beads":{"enabled":true,"refresh_sec":20}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Beads.Enabled || c.Beads.RefreshSec != 20 {
		t.Errorf("Beads = %+v, want {Enabled:true RefreshSec:20}", c.Beads)
	}
}

// TestLoadBeadsWritable checks the separate write gate parses.
func TestLoadBeadsWritable(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"beads":{"enabled":true,"writable":true}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)
	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !c.Beads.Writable {
		t.Errorf("Beads.Writable = false, want true")
	}
}

// TestLoadBeadsDefaultDisabled checks beads is off with no config.
func TestLoadBeadsDefaultDisabled(t *testing.T) {
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
	if c.Beads.Enabled {
		t.Errorf("Beads.Enabled = true, want false by default")
	}
}

// TestPathExpandsTildeInOverride checks that a HARNESS_DECK_CONFIG override
// with a leading ~ is expanded to a home-relative path so the file is found.
func TestPathExpandsTildeInOverride(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", "~/hd/config.json")
	want := filepath.Join(home, "hd", "config.json")
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}
}

// TestPathPlainOverrideUnchanged checks that an override without a leading ~
// passes through untouched.
func TestPathPlainOverrideUnchanged(t *testing.T) {
	t.Setenv("HARNESS_DECK_CONFIG", "/tmp/hd-test/config.json")
	if got, want := Path(), "/tmp/hd-test/config.json"; got != want {
		t.Errorf("Path() = %q, want %q", got, want)
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

// TestBaseURL covers the canonical-URL resolution: PublicURL wins (trailing
// slash trimmed), the scheme follows TLS, and an unspecified bind collapses to
// loopback rather than the un-connectable listen address.
func TestBaseURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
		want string
	}{
		{"public url wins", Config{PublicURL: "https://scadrial.example.ts.net:7420", Bind: "0.0.0.0", Port: 7420}, "https://scadrial.example.ts.net:7420"},
		{"public url trailing slash trimmed", Config{PublicURL: "https://host:7420/", Port: 7420}, "https://host:7420"},
		{"http loopback default", Config{Bind: "127.0.0.1", Port: 7420}, "http://127.0.0.1:7420"},
		{"unspecified bind collapses to loopback", Config{Bind: "0.0.0.0", Port: 7420}, "http://127.0.0.1:7420"},
		{"tls flips scheme to https", Config{Bind: "127.0.0.1", Port: 7420, TLS: TLSConfig{Cert: "/c", Key: "/k"}}, "https://127.0.0.1:7420"},
		{"explicit host preserved", Config{Bind: "192.168.1.5", Port: 9000}, "http://192.168.1.5:9000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.BaseURL(); got != tc.want {
				t.Errorf("BaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}
