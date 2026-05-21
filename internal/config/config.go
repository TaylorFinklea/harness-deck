// Package config loads harness-deck's settings: where reports live, which
// project roots to scan, and how to notify on responses. The config file is
// JSON at ~/.config/harness-deck/config.json; every field has a sensible
// default, so harness-deck runs with no config file at all.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Config is the harness-deck runtime configuration.
type Config struct {
	// CentralDir is the catch-all reports directory scanned for
	// <project>/<run>/report.json.
	CentralDir string `json:"central_dir"`
	// Projects are project roots; each is scanned for .harness/<run>/report.json.
	Projects []string `json:"projects"`
	// NotifyCommand runs when a response is recorded (Phase 4). The report
	// directory is appended as a final argument. Empty disables notification.
	NotifyCommand string `json:"notify_command"`
	// Port is the local HTTP port for `harness-deck serve`.
	Port int `json:"port"`
}

// Default returns the configuration used when no config file is present.
func Default() Config {
	return Config{
		CentralDir: "~/.harness/reports",
		Port:       7420,
	}
}

// Path is the config file location. The HARNESS_DECK_CONFIG env var overrides it.
func Path() string {
	if p := os.Getenv("HARNESS_DECK_CONFIG"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.json"
	}
	return filepath.Join(home, ".config", "harness-deck", "config.json")
}

// Load reads the config file, falling back to defaults for a missing file or
// any unset field. A malformed file is an error.
func Load() (Config, error) {
	c := Default()
	data, err := os.ReadFile(Path())
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if c.Port == 0 {
		c.Port = Default().Port
	}
	if c.CentralDir == "" {
		c.CentralDir = Default().CentralDir
	}
	return c, nil
}

// Expand resolves a leading ~ to the user's home directory.
func Expand(p string) string {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	return filepath.Join(home, p[2:])
}
