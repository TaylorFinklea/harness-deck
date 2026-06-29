// Package herdr adapts the herdr CLI (a terminal workspace manager for AI
// agents) into typed Go calls. It shells out to `herdr … --json` — no network,
// no auth, stdlib only. All calls leave Stdin nil (= /dev/null) so a terminal
// query can't wedge a caller goroutine, and use CommandContext for timeouts.
package herdr

import (
	"context"
	"os/exec"
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
