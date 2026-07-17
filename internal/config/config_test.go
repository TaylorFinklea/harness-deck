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

// TestLoadDefaultsProjectMarkers checks that project discovery falls back to
// the historical .docs/ai marker when the config file doesn't set one.
func TestLoadDefaultsProjectMarkers(t *testing.T) {
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
	if want := []string{".docs/ai"}; !reflect.DeepEqual(c.ProjectMarkers, want) {
		t.Errorf("ProjectMarkers = %v, want %v", c.ProjectMarkers, want)
	}
}

// TestLoadReadsProjectMarkers checks that project_markers replaces the
// default discovery marker set.
func TestLoadReadsProjectMarkers(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"project_markers":[".beads","go.mod"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{".beads", "go.mod"}; !reflect.DeepEqual(c.ProjectMarkers, want) {
		t.Errorf("ProjectMarkers = %v, want %v", c.ProjectMarkers, want)
	}
}

// TestLoadEmptyProjectMarkersFallsBack checks that an explicit empty list
// degrades to the default rather than making discovery match nothing.
func TestLoadEmptyProjectMarkersFallsBack(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"project_markers":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{".docs/ai"}; !reflect.DeepEqual(c.ProjectMarkers, want) {
		t.Errorf("ProjectMarkers = %v, want %v", c.ProjectMarkers, want)
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

// TestLoadDropsBlankProjectMarkers checks that blank/whitespace marker
// entries are stripped at load time, so downstream consumers (discovery,
// doctor's warning text) all see the effective set — and an all-blank list
// degrades to the default rather than match-all.
func TestLoadDropsBlankProjectMarkers(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"project_markers":["", " ", ".beads"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{".beads"}; !reflect.DeepEqual(c.ProjectMarkers, want) {
		t.Errorf("ProjectMarkers = %v, want %v", c.ProjectMarkers, want)
	}

	if err := os.WriteFile(cfgPath, []byte(`{"project_markers":[" "]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := []string{".docs/ai"}; !reflect.DeepEqual(c.ProjectMarkers, want) {
		t.Errorf("all-blank ProjectMarkers = %v, want default %v", c.ProjectMarkers, want)
	}
}

// TestLoadReadsPushSubject checks that push_subject parses; empty means the
// server falls back to its built-in contact URL.
func TestLoadReadsPushSubject(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"push_subject":"https://example.com/my-fork"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.PushSubject != "https://example.com/my-fork" {
		t.Errorf("PushSubject = %q, want %q", c.PushSubject, "https://example.com/my-fork")
	}
}

// TestLoadRejectsInvalidPushSubject checks that a malformed push_subject is a
// load-time error (matching notifications validation): the field feeds VAPID
// JWTs where a bad value means silently failed push delivery much later.
func TestLoadRejectsInvalidPushSubject(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	t.Setenv("HARNESS_DECK_CONFIG", cfgPath)

	for _, bad := range []string{"not-a-uri", "http://example.com", "mailto:", "   "} {
		if err := os.WriteFile(cfgPath, []byte(`{"push_subject":"`+bad+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(); err == nil {
			t.Errorf("push_subject %q: Load succeeded, want error", bad)
		}
	}

	for _, good := range []string{"https://example.com/my-fork", "mailto:ops@example.com"} {
		if err := os.WriteFile(cfgPath, []byte(`{"push_subject":"`+good+`"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := Load()
		if err != nil {
			t.Errorf("push_subject %q: Load: %v", good, err)
		}
		if c.PushSubject != good {
			t.Errorf("PushSubject = %q, want %q", c.PushSubject, good)
		}
	}
}

// TestPathHonorsXDGConfigHome checks that an absolute $XDG_CONFIG_HOME
// relocates the config file (Linux convention). HARNESS_DECK_CONFIG still
// wins when both are set.
func TestPathHonorsXDGConfigHome(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("HARNESS_DECK_CONFIG", "")
	t.Setenv("HOME", t.TempDir()) // fresh machine: no legacy ~/.config install
	t.Setenv("XDG_CONFIG_HOME", xdg)
	want := filepath.Join(xdg, "harness-deck", "config.json")
	if got := Path(); got != want {
		t.Errorf("Path() = %q, want %q", got, want)
	}

	t.Setenv("HARNESS_DECK_CONFIG", "/tmp/hd-test/config.json")
	if got, want := Path(), "/tmp/hd-test/config.json"; got != want {
		t.Errorf("Path() with override = %q, want %q", got, want)
	}
}

// TestPathXDGKeepsLegacyInstall checks the upgrade path for users who had
// XDG_CONFIG_HOME set before harness-deck honored it: an existing
// ~/.config/harness-deck/config.json keeps winning until a config exists at
// the XDG location, so upgrading the binary can't silently orphan their
// config and projects.json. Fresh installs (no file anywhere) use XDG.
func TestPathXDGKeepsLegacyInstall(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HARNESS_DECK_CONFIG", "")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", xdg)

	legacy := filepath.Join(home, ".config", "harness-deck", "config.json")
	xdgPath := filepath.Join(xdg, "harness-deck", "config.json")

	// Fresh install: nothing on disk → XDG location.
	if got := Path(); got != xdgPath {
		t.Errorf("fresh install Path() = %q, want %q", got, xdgPath)
	}

	// Legacy install: only ~/.config has a config → keep using it.
	if err := os.MkdirAll(filepath.Dir(legacy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacy, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Path(); got != legacy {
		t.Errorf("legacy install Path() = %q, want %q", got, legacy)
	}

	// Migrated: a config at the XDG location wins over the legacy one.
	if err := os.MkdirAll(filepath.Dir(xdgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(xdgPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := Path(); got != xdgPath {
		t.Errorf("migrated install Path() = %q, want %q", got, xdgPath)
	}
}

// TestPathIgnoresRelativeXDGConfigHome checks that a non-absolute
// $XDG_CONFIG_HOME is ignored per the basedir spec, falling back to
// ~/.config so the config file can't land relative to a random cwd.
func TestPathIgnoresRelativeXDGConfigHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home dir: %v", err)
	}
	t.Setenv("HARNESS_DECK_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", "relative/path")
	want := filepath.Join(home, ".config", "harness-deck", "config.json")
	if got := Path(); got != want {
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
