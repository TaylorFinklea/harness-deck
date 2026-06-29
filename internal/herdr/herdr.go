// Package herdr adapts the herdr CLI (a terminal workspace manager for AI
// agents) into typed Go calls. It shells out to `herdr … --json` — no network,
// no auth, stdlib only. All calls leave Stdin nil (= /dev/null) so a terminal
// query can't wedge a caller goroutine, and use CommandContext for timeouts.
package herdr

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

// Agent is one herdr-managed agent terminal.
type Agent struct {
	Label       string // "claude" | "codex" | "hermes" | …
	Status      string // idle|working|blocked|done|unknown
	Cwd         string
	Project     string // basename(Cwd)
	Focused     bool
	PaneID      string
	TabID       string
	WorkspaceID string
	TerminalID  string
	SessionID   string // agent_session.value, empty for non-claude
}

// Key is the stable per-agent identity. PaneID is globally unique (it embeds
// the workspace, e.g. "w6544b3b0f2d752:p1").
func (a Agent) Key() string { return a.PaneID }

// Blocked reports whether the agent is waiting on user input.
func (a Agent) Blocked() bool { return a.Status == "blocked" }

// Client wraps a resolved herdr binary path.
type Client struct {
	bin string // resolved herdr path; see resolveBin (Task 3)
}

// List returns every herdr-managed agent. A herdr that is absent or down
// yields (nil, err); callers degrade to an empty fleet.
func (c *Client) List(ctx context.Context) ([]Agent, error) {
	cmd := exec.CommandContext(ctx, c.bin, "agent", "list", "--json")
	out, err := cmd.Output() // Stdin nil = /dev/null (TUI-hang guard)
	if err != nil {
		return nil, err
	}
	return parseAgentList(out)
}

// Read returns the pane text for an agent plus herdr's truncated flag.
// source selects the buffer to read: "visible" returns the on-screen viewport,
// "recent" returns a larger scrollback window useful when visible is truncated,
// and "recent-unwrapped" is the same without terminal line-wrap artifacts.
// On truncated==true the caller may retry with source "recent"; see agents.go.
func (c *Client) Read(ctx context.Context, target, source string) (string, bool, error) {
	cmd := exec.CommandContext(ctx, c.bin, "agent", "read", target, "--source", source)
	out, err := cmd.Output() // Stdin nil = /dev/null (TUI-hang guard)
	if err != nil {
		return "", false, err
	}
	return parseRead(out)
}

// resolveBin finds an executable: $PATH first, then the given fallback paths
// (absolute). Mirrors usage.opencodeBin — the dashboard runs under launchd's
// minimal PATH, so a bare LookPath fails even when herdr is installed.
func resolveBin(name string, fallbacks []string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	for _, c := range fallbacks {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, true
		}
	}
	return "", false
}

// New resolves the herdr binary. ok=false means herdr is not installed; the
// caller should leave the feature dark.
func New() (*Client, bool) {
	fallbacks := []string{"/opt/homebrew/bin/herdr", "/usr/local/bin/herdr"}
	if h, err := os.UserHomeDir(); err == nil {
		fallbacks = append(fallbacks, filepath.Join(h, ".local", "bin", "herdr"))
	}
	bin, ok := resolveBin("herdr", fallbacks)
	if !ok {
		return nil, false
	}
	return &Client{bin: bin}, true
}

// Send delivers text + Enter into the target pane so a blocked agent receives
// the user's answer. Argv verified: `herdr pane run <pane_id> <text>` (Enter
// included; no `--` separator).
func (c *Client) Send(ctx context.Context, target, text string) error {
	cmd := exec.CommandContext(ctx, c.bin, "pane", "run", target, text)
	return cmd.Run()
}
