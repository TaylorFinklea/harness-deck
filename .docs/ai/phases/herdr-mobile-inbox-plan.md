# herdr Mobile Inbox Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Page the user's phone when a herdr-managed agent blocks, let them answer remotely, and deliver the answer back into the agent.

**Architecture:** A new stdlib-only `internal/herdr` adapter wraps the herdr CLI (`agent list --json`, `agent read`, `pane run`). A new server watcher tick — mirroring the existing report watcher in `internal/server/sse.go` — diffs agent statuses each poll, fires one Web Push per newly-`blocked` non-focused agent (reusing `deliverPush`), and exposes the live set at `GET /api/agents` (live via the existing SSE `hub`, new event name `agents`). A phone-first "needs-you" view answers with prefill-buttons + free-text + one confirm-send; `POST /api/agents/{key}/answer` re-checks status then calls `herdr pane run`.

**Tech Stack:** Go (stdlib only), `html/template`, vanilla embedded JS/CSS (`go:embed`), the herdr CLI.

## Global Constraints

- **Zero external dependencies.** `go.mod` has NO `require` block. Do not `go get` anything. Use stdlib only (copied verbatim from CLAUDE.md). [[decisions.md]]
- **`gofmt` before every commit.** `go build ./...` and `go test ./...` are the whole toolchain.
- **TUI-hang guard:** every `exec.Command`/`exec.CommandContext` to herdr leaves `Stdin` nil (Go gives the child `/dev/null`) so a terminal-capability query can't wedge the poll goroutine.
- **launchd PATH:** the dashboard runs under a launchd/systemd minimal PATH that omits `/opt/homebrew/bin` and `~/.local/bin`; never rely on `exec.LookPath` alone — probe install dirs (see Task 3, mirrors `internal/usage/opencode.go`'s `opencodeBin`).
- **Opt-in:** the whole feature is gated by `config.Agents.Enabled` (default `false`); nothing shells out to herdr unless it is enabled.
- **Graceful degradation:** herdr unreachable (socket down, binary absent) yields an empty agent list and no surfaced error — consistent with the usage providers.
- Spec: `.docs/ai/phases/herdr-mobile-inbox-spec.md`.

---

## File Structure

- `internal/herdr/herdr.go` (new) — adapter: `Agent` type, `Client`, `List`/`Read`/`Send`, `resolveBin`.
- `internal/herdr/parse.go` (new) — pure parsers `parseAgentList`, `parseRead` (binary-free, unit-tested).
- `internal/herdr/parse_test.go` (new) — fixture-driven parser tests.
- `internal/herdr/herdr_test.go` (new) — `resolveBin` test.
- `internal/config/config.go` (modify) — add `Agents AgentsConfig` block.
- `internal/config/config_test.go` (modify) — defaults/parse test for the new block.
- `internal/server/agents.go` (new) — agent-status watcher tick, `/api/agents` handler, `POST …/answer` handler, push-on-block.
- `internal/server/agents_test.go` (new) — tick/answer tests with a fake herdr client.
- `internal/server/server.go` (modify) — construct the herdr client + agent watcher in `New`, register routes (gated by config).
- `internal/assets/` (modify) — a `needs-you` view module + answer UX; registered like the existing views. **Read the existing view wiring first** (the Wave-3 activity timeline view is the closest analog).
- `docs/SETUP.md` (modify) — a "herdr mobile inbox" section.
- `.docs/ai/{roadmap,current-state,decisions}.md` (modify) — handoff updates.

**Plan convention (per AGENTS.md "Writing implementation plans"):** spec-derived details (the herdr JSON shapes — verified live — the data model, test invariants, acceptance) are specified exactly with full code. Codebase-derived idioms (the watcher tick, SSE/push wiring, route registration, the frontend view) are given as *behavior + the exact file/pattern to mirror*, NOT prescribed code — the implementer reads the named file and follows it. Do not invent server/JS code that contradicts the named pattern.

---

## Task 1: herdr adapter — `Agent` type + `List`

**Files:**
- Create: `internal/herdr/herdr.go`
- Create: `internal/herdr/parse.go`
- Test: `internal/herdr/parse_test.go`

**Interfaces:**
- Produces: `type Agent struct { Label, Status, Cwd, Project string; Focused bool; PaneID, TabID, WorkspaceID, TerminalID, SessionID string }`; `func (a Agent) Key() string` (= `PaneID`); `func (a Agent) Blocked() bool` (= `Status == "blocked"`); `func parseAgentList(raw []byte) ([]Agent, error)`; `func (c *Client) List(ctx context.Context) ([]Agent, error)`.

- [ ] **Step 1: Write the failing parser test**

Fixture is the verified live shape of `herdr agent list --json` (envelope `{"id":…,"result":{"agents":[…],"type":"agent_list"}}`).

```go
package herdr

import "testing"

const listFixture = `{"id":"cli:agent:list","result":{"agents":[
{"agent":"claude","agent_status":"working","cwd":"/Users/t/git/tesela","focused":false,"pane_id":"w1:p1","tab_id":"w1:t1","workspace_id":"w1","terminal_id":"term_1","agent_session":{"agent":"claude","kind":"id","source":"herdr:claude","value":"uuid-abc"}},
{"agent":"codex","agent_status":"blocked","cwd":"/Users/t/codex","focused":true,"pane_id":"w7:p1","tab_id":"w7:t1","workspace_id":"w7","terminal_id":"term_7"}
],"type":"agent_list"}}`

func TestParseAgentList(t *testing.T) {
	got, err := parseAgentList([]byte(listFixture))
	if err != nil {
		t.Fatalf("parseAgentList: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d agents, want 2", len(got))
	}
	a := got[0]
	if a.Label != "claude" || a.Status != "working" || a.Project != "tesela" {
		t.Errorf("agent[0] = %+v, want claude/working/tesela", a)
	}
	if a.Key() != "w1:p1" {
		t.Errorf("Key() = %q, want w1:p1", a.Key())
	}
	if a.SessionID != "uuid-abc" {
		t.Errorf("SessionID = %q, want uuid-abc", a.SessionID)
	}
	if !got[1].Blocked() || got[1].Project != "codex" || !got[1].Focused {
		t.Errorf("agent[1] = %+v, want blocked/codex/focused", got[1])
	}
}

func TestParseAgentListEmpty(t *testing.T) {
	got, err := parseAgentList([]byte(`{"result":{"agents":[],"type":"agent_list"}}`))
	if err != nil || len(got) != 0 {
		t.Fatalf("got (%v,%v), want empty,nil", got, err)
	}
}
```

- [ ] **Step 2: Run it, verify it fails**

Run: `go test ./internal/herdr/ -run TestParseAgentList -v`
Expected: FAIL — `parseAgentList` / `Agent` undefined.

- [ ] **Step 3: Write `parse.go` (parser) and `herdr.go` (types)**

`internal/herdr/parse.go`:

```go
package herdr

import (
	"encoding/json"
	"path/filepath"
)

// rawList mirrors the `herdr agent list --json` envelope. Unknown fields are
// ignored so a herdr version bump that adds fields does not break parsing.
type rawList struct {
	Result struct {
		Agents []struct {
			Agent        string `json:"agent"`
			AgentStatus  string `json:"agent_status"`
			Cwd          string `json:"cwd"`
			Focused      bool   `json:"focused"`
			PaneID       string `json:"pane_id"`
			TabID        string `json:"tab_id"`
			WorkspaceID  string `json:"workspace_id"`
			TerminalID   string `json:"terminal_id"`
			AgentSession *struct {
				Value string `json:"value"`
			} `json:"agent_session"`
		} `json:"agents"`
	} `json:"result"`
}

// parseAgentList turns the list envelope into []Agent. Project is the basename
// of cwd. A nil agent_session (non-claude agents) yields an empty SessionID.
func parseAgentList(raw []byte) ([]Agent, error) {
	var rl rawList
	if err := json.Unmarshal(raw, &rl); err != nil {
		return nil, err
	}
	out := make([]Agent, 0, len(rl.Result.Agents))
	for _, a := range rl.Result.Agents {
		ag := Agent{
			Label:       a.Agent,
			Status:      a.AgentStatus,
			Cwd:         a.Cwd,
			Project:     filepath.Base(a.Cwd),
			Focused:     a.Focused,
			PaneID:      a.PaneID,
			TabID:       a.TabID,
			WorkspaceID: a.WorkspaceID,
			TerminalID:  a.TerminalID,
		}
		if a.AgentSession != nil {
			ag.SessionID = a.AgentSession.Value
		}
		out = append(out, ag)
	}
	return out, nil
}
```

`internal/herdr/herdr.go` (types + `Client.List`; `Read`/`Send`/`resolveBin` land in Tasks 2–3):

```go
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
```

- [ ] **Step 4: Run tests, verify pass**

Run: `go test ./internal/herdr/ -run TestParseAgentList -v`
Expected: PASS (both cases). NOTE: `herdr.go` references `resolveBin` only in Tasks 2–3; if the package does not compile because `c.bin` is unused, that's fine — `List` uses it. If `go vet` flags an unused field before Task 3, proceed; Task 3 adds the constructor.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w internal/herdr/
go build ./internal/herdr/ && go test ./internal/herdr/
git add internal/herdr/
git commit -m "feat(herdr): adapter Agent type + agent list parser"
```

---

## Task 2: herdr adapter — `Read` (captures the blocked question)

**Files:**
- Modify: `internal/herdr/parse.go`, `internal/herdr/herdr.go`
- Test: `internal/herdr/parse_test.go`

**Interfaces:**
- Produces: `func parseRead(raw []byte) (text string, truncated bool, err error)`; `func (c *Client) Read(ctx context.Context, target string) (text string, truncated bool, err error)`.

**Verified shape:** `herdr agent read <target> --source visible` returns `{"result":{"read":{"text":"…","truncated":false,"source":"visible",…},"type":…}}`.

- [ ] **Step 1: Write the failing test**

```go
func TestParseRead(t *testing.T) {
	const readFixture = `{"id":"cli:agent:read","result":{"read":{"format":"text","pane_id":"w1:p1","revision":0,"source":"visible","tab_id":"w1:t1","text":"Do you want to apply migration 0007? (y/n)","truncated":false,"workspace_id":"w1"},"type":"agent_read"}}`
	text, truncated, err := parseRead([]byte(readFixture))
	if err != nil {
		t.Fatalf("parseRead: %v", err)
	}
	if truncated {
		t.Errorf("truncated = true, want false")
	}
	if text != "Do you want to apply migration 0007? (y/n)" {
		t.Errorf("text = %q", text)
	}
}
```

- [ ] **Step 2: Run, verify FAIL** — `go test ./internal/herdr/ -run TestParseRead -v` → `parseRead` undefined.

- [ ] **Step 3: Implement**

Append to `internal/herdr/parse.go`:

```go
type rawRead struct {
	Result struct {
		Read struct {
			Text      string `json:"text"`
			Truncated bool   `json:"truncated"`
		} `json:"read"`
	} `json:"result"`
}

// parseRead extracts the captured pane text and herdr's truncation flag.
func parseRead(raw []byte) (text string, truncated bool, err error) {
	var rr rawRead
	if err = json.Unmarshal(raw, &rr); err != nil {
		return "", false, err
	}
	return rr.Result.Read.Text, rr.Result.Read.Truncated, nil
}
```

Append to `internal/herdr/herdr.go`:

```go
// Read returns the visible pane text for an agent plus herdr's truncated flag.
// On truncated==true the caller may retry with a larger window; see the spec.
func (c *Client) Read(ctx context.Context, target string) (string, bool, error) {
	cmd := exec.CommandContext(ctx, c.bin, "agent", "read", target, "--source", "visible")
	out, err := cmd.Output()
	if err != nil {
		return "", false, err
	}
	return parseRead(out)
}
```

- [ ] **Step 4: Run, verify PASS** — `go test ./internal/herdr/ -run TestParseRead -v`.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w internal/herdr/
git add internal/herdr/
git commit -m "feat(herdr): Read captures blocked question + truncation flag"
```

---

## Task 3: herdr adapter — `resolveBin` + `Send` + `New`

**Files:**
- Modify: `internal/herdr/herdr.go`
- Test: `internal/herdr/herdr_test.go`

**Interfaces:**
- Produces: `func New() (*Client, bool)` (resolves the herdr binary; `false` when not found); `func (c *Client) Send(ctx context.Context, target, text string) error`.

- [ ] **Step 1: VERIFY the herdr send invocation before coding.** Run `herdr pane run --help < /dev/null` and `herdr agent send --help < /dev/null`. Confirm which delivers *text + Enter* to a pane (the spec assumes `pane run`). Use the verified argv in Step 3. Do NOT guess — if `pane run` takes `<pane_id> -- <argv...>`, the Send below must match it.

- [ ] **Step 2: Write the failing `resolveBin` test**

```go
package herdr

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBinProbesDir(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "herdr")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, ok := resolveBin("herdr", []string{filepath.Join(dir, "nope"), bin})
	if !ok || got != bin {
		t.Fatalf("resolveBin = (%q,%v), want (%q,true)", got, ok, bin)
	}
}

func TestResolveBinMissing(t *testing.T) {
	if got, ok := resolveBin("herdr-nonexistent-xyz", []string{"/no/such/path"}); ok {
		t.Fatalf("resolveBin found %q, want not found", got)
	}
}
```

- [ ] **Step 3: Run, verify FAIL** — `go test ./internal/herdr/ -run TestResolveBin -v`.

- [ ] **Step 4: Implement `resolveBin`, `New`, `Send`** — mirror `opencodeBin()` in `internal/usage/opencode.go` (read it first for the exact `$PATH`-then-probe shape).

```go
import (
	"os"
	"path/filepath"
)

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

// Send delivers text (followed by Enter) into the target pane so a blocked
// agent receives the user's answer. ARGV VERIFIED IN STEP 1 — adjust to match.
func (c *Client) Send(ctx context.Context, target, text string) error {
	cmd := exec.CommandContext(ctx, c.bin, "pane", "run", target, "--", text)
	return cmd.Run()
}
```

- [ ] **Step 5: Run, verify PASS** — `go test ./internal/herdr/`.

- [ ] **Step 6: gofmt + commit**

```bash
gofmt -w internal/herdr/
go build ./... && go test ./internal/herdr/
git add internal/herdr/
git commit -m "feat(herdr): resolveBin (launchd PATH) + New + Send"
```

---

## Task 4: config — `Agents` opt-in block

**Files:**
- Modify: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `type AgentsConfig struct { Enabled bool; RefreshSec int }`; field `Agents AgentsConfig` on `Config`.

- [ ] **Step 1: Write the failing test** (mirror the existing config parse tests' style — read `config_test.go` first).

```go
func TestAgentsConfigParse(t *testing.T) {
	c := Default()
	if c.Agents.Enabled {
		t.Errorf("Agents.Enabled default = true, want false (opt-in)")
	}
	const j = `{"agents":{"enabled":true,"refresh_sec":3}}`
	if err := json.Unmarshal([]byte(j), &c); err != nil {
		t.Fatal(err)
	}
	if !c.Agents.Enabled || c.Agents.RefreshSec != 3 {
		t.Errorf("Agents = %+v, want enabled/3", c.Agents)
	}
}
```

- [ ] **Step 2: Run, verify FAIL** — `go test ./internal/config/ -run TestAgentsConfig -v`.

- [ ] **Step 3: Implement** — add to `config.go` near `UsageConfig`:

```go
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
```

Add `Agents AgentsConfig `json:"agents,omitempty"`` to the `Config` struct.

- [ ] **Step 4: Run, verify PASS** — `go test ./internal/config/`.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w internal/config/
git add internal/config/
git commit -m "feat(config): opt-in agents block for herdr mobile inbox"
```

---

## Task 5: server — agent-status watcher tick (detect newly-blocked)

**Files:**
- Create: `internal/server/agents.go`
- Test: `internal/server/agents_test.go`

**Interfaces:**
- Consumes: `herdr.Agent`, `herdr.Client` (Task 1–3); the SSE `hub` and `deliverPush` (existing).
- Produces: `type herdrClient interface { List(context.Context) ([]herdr.Agent, error); Read(context.Context, string) (string, bool, error); Send(context.Context, string, string) error }` (lets tests inject a fake); `type BlockedAgent struct { herdr.Agent; Question string; Since time.Time }`; `type agentState struct { blocked map[string]BlockedAgent; misses map[string]int }`; `func (s *Server) tickAgents(ctx context.Context, prev agentState) agentState`.

**Behavior (spec-derived — implement exactly):**
- Poll `s.agents.List(ctx)`. Build the new blocked set = agents where `Blocked()` AND NOT `Focused` (focused = user is already there).
- A *newly*-blocked agent (key in new set, NOT in `prev.blocked`) → `Read` its pane for the `Question`, set `Since = time.Now()`, and fire push (Task 6) + `hub.broadcastEvent("agents", …)`.
- An agent that left blocked → drop it, applying an `askRetainTicks`-style miss debounce so a 1-tick flicker does not drop+re-page. **Reuse the existing `askRetainTicks` constant and mirror `mergeRetainedAsks` in `sse.go`** (read it; do not re-invent the debounce).
- herdr error → treat as empty list, log once, keep `prev` (don't clear the inbox on a transient herdr hiccup).

**Implementation note:** mirror `internal/server/sse.go`'s `tick`/`watchState` structure — `tickAgents` is the agent-channel sibling of `tick`, pure of `time.Sleep` so the test drives it directly. Wire it into `watch` (Task 9). Do NOT prescribe the body here; follow `sse.go`.

- [ ] **Step 1: Write the failing test** (fake client; no real herdr, no real push — use the existing `s.testNotifyFn` seam pattern from `push_test.go`, read it first).

```go
package server

import (
	"context"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/herdr"
)

type fakeHerdr struct {
	agents []herdr.Agent
	read   string
}

func (f *fakeHerdr) List(context.Context) ([]herdr.Agent, error) { return f.agents, nil }
func (f *fakeHerdr) Read(context.Context, string) (string, bool, error) { return f.read, false, nil }
func (f *fakeHerdr) Send(context.Context, string, string) error { return nil }

func TestTickAgentsFiresOnNewBlock(t *testing.T) {
	fh := &fakeHerdr{read: "apply migration?"}
	s := &Server{agents: fh} // minimal server; see existing tests for the pattern
	var pushes int
	s.testAgentNotifyFn = func() { pushes++ }

	// idle → no block
	fh.agents = []herdr.Agent{{PaneID: "w1:p1", Status: "idle"}}
	st := s.tickAgents(context.Background(), agentState{blocked: map[string]BlockedAgent{}, misses: map[string]int{}})
	if pushes != 0 || len(st.blocked) != 0 {
		t.Fatalf("idle: pushes=%d blocked=%d, want 0/0", pushes, len(st.blocked))
	}

	// becomes blocked (not focused) → one push, captured question
	fh.agents = []herdr.Agent{{PaneID: "w1:p1", Status: "blocked"}}
	st = s.tickAgents(context.Background(), st)
	if pushes != 1 || len(st.blocked) != 1 {
		t.Fatalf("blocked: pushes=%d blocked=%d, want 1/1", pushes, len(st.blocked))
	}
	if st.blocked["w1:p1"].Question != "apply migration?" {
		t.Errorf("question = %q", st.blocked["w1:p1"].Question)
	}

	// still blocked → no re-fire
	st = s.tickAgents(context.Background(), st)
	if pushes != 1 {
		t.Fatalf("still blocked: pushes=%d, want 1 (no re-fire)", pushes)
	}

	// focused block is suppressed
	fh.agents = []herdr.Agent{{PaneID: "w2:p1", Status: "blocked", Focused: true}}
	st = s.tickAgents(context.Background(), agentState{blocked: map[string]BlockedAgent{}, misses: map[string]int{}})
	if pushes != 1 || len(st.blocked) != 0 {
		t.Fatalf("focused: pushes=%d blocked=%d, want unchanged/0", pushes, len(st.blocked))
	}
}
```

- [ ] **Step 2: Run, verify FAIL** — `go test ./internal/server/ -run TestTickAgents -v` (won't compile: `Server.agents`, `testAgentNotifyFn`, `tickAgents`, `agentState`, `BlockedAgent` undefined).

- [ ] **Step 3: Implement `agents.go`** — define the types/interface above; add `agents herdrClient` and `testAgentNotifyFn func()` fields to `Server` (in `server.go`); implement `tickAgents` mirroring `sse.go`'s `tick`+`mergeRetainedAsks` (debounce via `askRetainTicks`), calling the push helper from Task 6 (stub it to just call `testAgentNotifyFn` if set, then `deliverPush`, until Task 6).

- [ ] **Step 4: Run, verify PASS** — `go test ./internal/server/ -run TestTickAgents -v`.

- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w internal/server/
go build ./... && go test ./internal/server/ -run TestTickAgents
git add internal/server/agents.go internal/server/server.go internal/server/agents_test.go
git commit -m "feat(server): herdr agent-status tick — detect newly-blocked"
```

---

## Task 6: server — push + SSE on a new block

**Files:**
- Modify: `internal/server/agents.go`
- Test: `internal/server/agents_test.go`

**Interfaces:**
- Produces: `func (s *Server) notifyBlockedAgent(b BlockedAgent)` — fires the test seam, then (when push is enabled) `deliverPush` with a payload pointing at the needs-you view, and `hub.broadcastEvent("agents", "blocked")`.

**Behavior:** mirror `notifyNewAsks` in `push.go` (read it). Payload: `Title = b.Project + " — " + b.Label + " needs you"`, `Body = truncateBody(b.Question)`, `Tag = b.Key()`, `URL = "/agents"` (the needs-you view route from Task 10). Fire the `testAgentNotifyFn` seam once before any real push so the Task 5 test counts without VAPID keys.

- [ ] **Step 1: Write the failing test** — assert `notifyBlockedAgent` broadcasts an `agents` SSE event. Subscribe a hub client (mirror an existing SSE test if present; otherwise assert via the `testAgentNotifyFn` count already covered — in that case this step adds a focused test for the payload `Tag`/`URL` via a `testAgentPushFn func(push.Payload)` seam).

```go
func TestNotifyBlockedAgentPayload(t *testing.T) {
	var got push.Payload
	s := &Server{testAgentPushFn: func(p push.Payload) { got = p }}
	s.notifyBlockedAgent(BlockedAgent{Agent: herdr.Agent{Label: "claude", Project: "refrigate", PaneID: "w1:p1"}, Question: "apply?"})
	if got.Tag != "w1:p1" || got.URL != "/agents" {
		t.Errorf("payload = %+v, want tag w1:p1 / url /agents", got)
	}
	if got.Body != "apply?" {
		t.Errorf("body = %q", got.Body)
	}
}
```

- [ ] **Step 2: Run, verify FAIL.**
- [ ] **Step 3: Implement `notifyBlockedAgent`** — add the `testAgentPushFn func(push.Payload)` seam to `Server`; route through it when set, else `go s.deliverPush(p)`; always `hub.broadcastEvent("agents", "blocked")`. Have `tickAgents` call `notifyBlockedAgent` (replace the Task 5 stub).
- [ ] **Step 4: Run, verify PASS** — `go test ./internal/server/ -run 'TestTickAgents|TestNotifyBlockedAgent'`.
- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w internal/server/
git add internal/server/
git commit -m "feat(server): Web Push + SSE on newly-blocked herdr agent"
```

---

## Task 7: server — `GET /api/agents`

**Files:**
- Modify: `internal/server/agents.go`, `internal/server/server.go` (route)
- Test: `internal/server/agents_test.go`

**Interfaces:**
- Produces: `func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request)` — JSON `{ "blocked": [BlockedAgent…], "agents": [Agent…] }` from the latest tick state (store the latest `agentState` + last full list on `Server` under a mutex; mirror how `s.usage` snapshots are served by `/api/usage` — read that handler first).

- [ ] **Step 1: Write the failing test** — seed the server's latest agent snapshot, `httptest` GET `/api/agents`, assert the blocked agent is present in the JSON.

```go
func TestHandleAgentsJSON(t *testing.T) {
	s := &Server{}
	s.setAgentSnapshot([]herdr.Agent{{PaneID: "w1:p1", Status: "blocked", Project: "refrigate"}},
		map[string]BlockedAgent{"w1:p1": {Agent: herdr.Agent{PaneID: "w1:p1", Project: "refrigate"}, Question: "apply?"}})
	rr := httptest.NewRecorder()
	s.handleAgents(rr, httptest.NewRequest("GET", "/api/agents", nil))
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	var body struct {
		Blocked []BlockedAgent `json:"blocked"`
	}
	json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Blocked) != 1 || body.Blocked[0].Question != "apply?" {
		t.Errorf("blocked = %+v", body.Blocked)
	}
}
```

- [ ] **Step 2: Run, verify FAIL.**
- [ ] **Step 3: Implement** `handleAgents` + `setAgentSnapshot`/`agentSnapshot` (mutex-guarded fields on `Server`). Register `mux.HandleFunc("GET /api/agents", s.handleAgents)` in `server.go` next to `GET /api/usage`.
- [ ] **Step 4: Run, verify PASS.**
- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w internal/server/
git add internal/server/
git commit -m "feat(server): GET /api/agents live channel"
```

---

## Task 8: server — `POST /api/agents/{key}/answer` (write-back with re-check)

**Files:**
- Modify: `internal/server/agents.go`, `internal/server/server.go` (route)
- Test: `internal/server/agents_test.go`

**Interfaces:**
- Produces: `func (s *Server) handleAgentAnswer(w http.ResponseWriter, r *http.Request)` — body `{ "text": "…" }`; path value `{key}` = pane id.

**Behavior (spec-derived):**
1. Decode `{text}`. Reject empty 400.
2. **Re-check status:** `s.agents.List(ctx)`; find the agent by key. If not found or NOT `blocked`, return **409** `{"error":"agent no longer blocked"}` — never deliver a stale answer into a live session.
3. Else `s.agents.Send(ctx, key, text)`; on success `{"ok":true}` and `hub.broadcastEvent("agents","answered")`.

- [ ] **Step 1: Write the failing tests** (fake client with a settable status).

```go
func TestAnswerRefusesUnblocked(t *testing.T) {
	fh := &fakeHerdr{agents: []herdr.Agent{{PaneID: "w1:p1", Status: "working"}}}
	s := &Server{agents: fh}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents/w1:p1/answer", strings.NewReader(`{"text":"yes"}`))
	req.SetPathValue("key", "w1:p1")
	s.handleAgentAnswer(rr, req)
	if rr.Code != 409 {
		t.Fatalf("status = %d, want 409 (no longer blocked)", rr.Code)
	}
}

func TestAnswerDeliversWhenBlocked(t *testing.T) {
	sent := ""
	fh := &sendSpyHerdr{fakeHerdr: fakeHerdr{agents: []herdr.Agent{{PaneID: "w1:p1", Status: "blocked"}}}, onSend: func(_, t string) { sent = t }}
	s := &Server{agents: fh}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents/w1:p1/answer", strings.NewReader(`{"text":"yes"}`))
	req.SetPathValue("key", "w1:p1")
	s.handleAgentAnswer(rr, req)
	if rr.Code != 200 || sent != "yes" {
		t.Fatalf("status=%d sent=%q, want 200/yes", rr.Code, sent)
	}
}
```

(Define `sendSpyHerdr` embedding `fakeHerdr` with an `onSend` hook in the test file.)

- [ ] **Step 2: Run, verify FAIL.**
- [ ] **Step 3: Implement** `handleAgentAnswer`; register `mux.HandleFunc("POST /api/agents/{key}/answer", s.handleAgentAnswer)`.
- [ ] **Step 4: Run, verify PASS** — `go test ./internal/server/ -run TestAnswer`.
- [ ] **Step 5: gofmt + commit**

```bash
gofmt -w internal/server/
git add internal/server/
git commit -m "feat(server): POST answer — re-check status then herdr send"
```

---

## Task 9: server — wire adapter + watcher into `New` (gated)

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: Wire it up** (no new behavior test — covered by Tasks 5–8; this is construction). In `New`, after the usage monitor block:

```go
if cfg.Agents.Enabled {
	if hc, ok := herdr.New(); ok {
		s.agents = hc
	} else {
		log.Print("harness-deck: agents enabled but herdr not found — feature dark")
	}
}
```

Start the agent watcher inside `Start`/`watch` (mirror `go s.watch(pollInterval)`): a sibling goroutine polling at `cfg.Agents.RefreshSec` (default `pollInterval`) that calls `tickAgents` and `setAgentSnapshot`, only when `s.agents != nil`. Read the existing `watch` (sse.go:297) and follow it.

- [ ] **Step 2: Build + full test** — `go build ./... && go test ./...`. Expected: all green.

- [ ] **Step 3: gofmt + commit**

```bash
gofmt -w internal/server/
git add internal/server/
git commit -m "feat(server): construct herdr client + agent watcher when enabled"
```

---

## Task 10: frontend — the needs-you view + answer UX

**Files:**
- Modify: `internal/assets/` (a new view module + its registration). **Read the existing view wiring first** — the Wave-3 activity timeline (`g l`) is the closest analog for adding a 3rd/4th view; follow its IIFE module + `HDDom.el` pattern. Do NOT prescribe JS here.

**Behavior (spec-derived — the UX contract):**
- A `/agents` route (or a view toggle) renders a **needs-you** list from `GET /api/agents`, refreshed on the SSE `agents` event (subscribe via the existing `EventSource`).
- Each blocked agent card: project · agent label · captured `Question`.
- Answer control: quick buttons (Yes / No / Approve — configurable defaults) that **prefill** an editable text field (they never auto-submit); a free-text field; a single **Send** that POSTs `{text}` to `/api/agents/{key}/answer`.
- On a 409 (`agent no longer blocked`), show a non-blocking notice and refresh the list.
- Phone-first layout (this is the primary surface); desktop shows the same view compactly. Reuse Tokyo-Night CSS variables.

- [ ] **Step 1:** Build the view following the activity-timeline module pattern; wire `GET /api/agents` + the SSE `agents` event + the answer POST.
- [ ] **Step 2: Browser-verify** (chrome-devtools MCP, isolated instance via `HARNESS_DECK_CONFIG`): with a fake `/api/agents` payload (or a real blocked agent), the needs-you view lists it, a quick button prefills the field, edit + Send POSTs, and a 409 path shows the notice. Zero console errors.
- [ ] **Step 3: commit**

```bash
git add internal/assets/
git commit -m "feat(ui): needs-you view — prefill-buttons + free-text + confirm-send"
```

---

## Task 11: docs + handoff

**Files:**
- Modify: `docs/SETUP.md`, `.docs/ai/roadmap.md`, `.docs/ai/current-state.md`, `.docs/ai/decisions.md`

- [ ] **Step 1: `docs/SETUP.md`** — add a "herdr mobile inbox" section: opt-in `agents.enabled`, what it needs (herdr installed + the agent integrations), the `/api/agents` channel, that push must be configured (`hdeck vapid` + TLS for iOS), and the focused-suppression behavior.
- [ ] **Step 2: handoff docs** — `decisions.md` ADR (the three brainstorm decisions + the verified read-fidelity finding); `roadmap.md` mark the "herdr ↔ harness-deck" Later item in-progress/done; `current-state.md` breadcrumb.
- [ ] **Step 3: commit**

```bash
git add docs/SETUP.md .docs/ai/
git commit -m "docs: herdr mobile inbox setup + handoff"
```

---

## Final verification

- [ ] `gofmt -l internal/ | head` → no output.
- [ ] `go build ./... && go test ./...` → all green; zero new `go.mod` requires (`git diff go.mod` empty).
- [ ] Manual end-to-end (human-verified): enable `agents` in an isolated config, induce a real blocked agent, confirm the phone receives a push, the needs-you view shows the question, Send delivers via herdr, and the agent resumes. Re-confirm a focused block does NOT page.
