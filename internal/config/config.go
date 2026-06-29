// Package config loads harness-deck's settings: where reports live, which
// project roots to scan, and how to notify on responses. The config file is
// JSON at ~/.config/harness-deck/config.json; every field has a sensible
// default, so harness-deck runs with no config file at all.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/TaylorFinklea/harness-deck/internal/notify"
)

// Config is the harness-deck runtime configuration.
type Config struct {
	// CentralDir is the catch-all reports directory scanned for
	// <project>/<run>/report.json.
	CentralDir string `json:"central_dir"`
	// Projects are project roots; each is scanned for .harness/<run>/report.json.
	Projects []string `json:"projects"`
	// ScanRoots are directories searched (depth-1) for project roots: a
	// direct child holding a .docs/ai directory is a discovered project.
	ScanRoots []string `json:"scan_roots"`
	// NotifyCommand runs when a response is recorded (Phase 4). The report
	// directory is appended as a final argument. Empty disables notification.
	NotifyCommand string `json:"notify_command"`
	// Port is the local HTTP port for `harness-deck serve`.
	Port int `json:"port"`
	// Bind is the interface the server listens on. Default "127.0.0.1" keeps
	// the dashboard private to the local machine; set "0.0.0.0" or a specific
	// interface address (e.g. a Tailscale IP) to make it reachable from a phone.
	Bind string `json:"bind"`
	// TLS holds optional cert + key paths. When both are set the server
	// listens with HTTPS, which is required for iOS web push notifications.
	// Generate certs once with `tailscale cert <hostname>`.
	TLS TLSConfig `json:"tls"`
	// PublicURL is the externally-reachable base URL of the dashboard,
	// e.g. "https://scadrial.tailceb58.ts.net:7420". Used by notification
	// fan-out (Slack/Discord/webhook) to build clickable links to reports.
	// Empty falls back to best-effort construction from Bind + Port + TLS;
	// that fallback works for localhost but produces "0.0.0.0:7420" for
	// 0.0.0.0 binds — links won't resolve externally, set this explicitly.
	PublicURL string `json:"public_url,omitempty"`
	// Notifications are fan-out destinations (Slack / Discord / generic
	// webhook) fired alongside Web Push whenever a new ask appears.
	// Validation happens at Load time; a malformed entry is fatal so
	// misconfig surfaces immediately rather than silently dropping
	// notifications later.
	Notifications []notify.Destination `json:"notifications,omitempty"`
	// Usage configures the footer usage monitors (CodexBar-style). Opt-in:
	// nothing reads credentials or hits the network unless a provider is
	// listed in Usage.Providers.
	Usage UsageConfig `json:"usage,omitempty"`
	// Agents drives the herdr mobile-inbox feature. Opt-in: nothing shells
	// out to herdr unless Agents.Enabled is true.
	Agents AgentsConfig `json:"agents,omitempty"`
}

// UsageConfig drives the footer usage indicators. It is fully opt-in: an empty
// Providers list means the feature is off.
type UsageConfig struct {
	// Providers lists the tools to monitor, in footer order. Known ids:
	// "codex", "openrouter", "claude-code", "copilot", "opencode". Several
	// touch credentials (Keychain) or the network, so listing is the consent.
	Providers []string `json:"providers,omitempty"`
	// OpenRouterKey authenticates the OpenRouter usage GET; falls back to the
	// OPENROUTER_API_KEY environment variable when empty.
	OpenRouterKey string `json:"openrouter_key,omitempty"`
	// OpenCodeCookie is deprecated and unused. opencode usage now reads
	// `opencode stats` locally — no cookie is needed. This field is kept so
	// existing config files still parse without error.
	OpenCodeCookie string `json:"opencode_cookie,omitempty"`
	// OpenCodeWorkspaceID is deprecated and unused. opencode usage now reads
	// `opencode stats` locally. This field is kept so existing configs parse.
	OpenCodeWorkspaceID string `json:"opencode_workspace_id,omitempty"`
	// OpenCodeDays is the rolling window in days for the opencode spend tile
	// (reads `opencode stats --days N`). Default 7.
	OpenCodeDays int `json:"opencode_days,omitempty"`
	// OpenCodeEnabled is a feature flag for the opencode tile, off by default.
	// `opencode stats` only sees local TUI sessions, so the tile reads $0 for
	// anyone whose real spend runs through the opencode-go/Zen cloud plan. Even
	// with "opencode" in Providers, the tile stays hidden unless this is true.
	// Set it on to revisit once a cloud-usage source lands. See decisions.md.
	OpenCodeEnabled bool `json:"opencode_enabled,omitempty"`
	// RefreshSec is the poll cadence in seconds (default 60). HTTP providers
	// count against their service's rate limits, so keep it sane.
	RefreshSec int `json:"refresh_sec,omitempty"`
}

// AgentsConfig drives the herdr mobile-inbox feature. Opt-in: nothing shells
// out to herdr unless Enabled. RefreshSec is the herdr poll cadence (default
// 2s, matching the report watcher) when zero.
type AgentsConfig struct {
	// Enabled turns on herdr agent polling, the /api/agents channel, and
	// block→push. Off by default — the dashboard never touches herdr otherwise.
	Enabled bool `json:"enabled,omitempty"`
	// RefreshSec is the herdr poll cadence in seconds (default 2 when zero).
	RefreshSec int `json:"refresh_sec,omitempty"`
}

// TLSConfig points at the cert + key files used when HTTPS is enabled.
type TLSConfig struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
}

// Enabled reports whether TLS is configured.
func (t TLSConfig) Enabled() bool { return t.Cert != "" && t.Key != "" }

// Default returns the configuration used when no config file is present.
func Default() Config {
	return Config{
		CentralDir: "~/.harness/reports",
		Port:       7420,
		Bind:       "127.0.0.1",
	}
}

// Path is the config file location. The HARNESS_DECK_CONFIG env var overrides
// it; a leading ~ in the override is expanded to the user's home directory.
func Path() string {
	if p := os.Getenv("HARNESS_DECK_CONFIG"); p != "" {
		return Expand(p)
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
	if c.Bind == "" {
		c.Bind = Default().Bind
	}
	for i, d := range c.Notifications {
		if err := d.Validate(); err != nil {
			return c, fmt.Errorf("notifications[%d] (%q): %w", i, d.Name, err)
		}
	}
	return c, nil
}

// Dir returns the directory the config file lives in, e.g.
// ~/.config/harness-deck. Companion state (VAPID keys, push subscriptions)
// is stored alongside it.
func Dir() string { return filepath.Dir(Path()) }

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

// BaseURL returns the canonical base URL a client should use to reach the
// dashboard, without a trailing slash. It prefers PublicURL when set;
// otherwise it builds scheme://host:port, substituting a loopback host for an
// unspecified bind ("0.0.0.0"/"::"/"") since those are listen addresses, not
// ones a client can connect to. Note: with TLS and an unspecified bind, the
// loopback fallback will mismatch a hostname cert — set PublicURL to the
// cert's hostname (e.g. a Tailscale name) for a clean HTTPS URL.
func (c Config) BaseURL() string {
	if u := strings.TrimRight(c.PublicURL, "/"); u != "" {
		return u
	}
	scheme := "http"
	if c.TLS.Enabled() {
		scheme = "https"
	}
	host := c.Bind
	switch host {
	case "", "0.0.0.0", "::", "[::]":
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, host, c.Port)
}
