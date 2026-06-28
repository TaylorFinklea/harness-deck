# herdr mobile inbox — spec

_Status: draft for review · 2026-06-28 · author: Opus (Claude Code)_

## Problem

The user runs a large fleet of AI agents under **herdr** (a terminal workspace
manager; ~11 agents live at spec time across separate repos). When an agent
**blocks** waiting for input, the user must be at the herdr terminal to notice
and answer. harness-deck already has the one thing herdr-in-a-terminal lacks —
**remote reach** (PWA + Web Push + Tailscale, already shipped). This feature
makes harness-deck the surface that pages the user when an agent blocks and lets
them answer from their phone, delivering the answer back into the agent.

## Scope

**In (v1):** detect a newly-`blocked` herdr agent → capture its question →
Web-Push the phone → user answers (quick-tap or free-text) → deliver the answer
into the agent via herdr. A minimal live "needs-you" surface (primarily mobile).

**Out:** a desktop fleet board (herdr already is that at the desk — explicitly
rejected by the user as low-value); historical analytics; multi-user; any cloud
dependency; managing/launching agents (read + answer only).

## Decisions (settled in brainstorming)

1. **Mobile-first remote unblock loop.** The board is not the product; reach is.
2. **Answer UX = quick buttons that _prefill_ an editable field + free-text + one
   confirm-send.** Buttons never blind-send (different agents take "yes"
   differently — numbered menu vs `y` vs a sentence); they prefill a best-guess
   token the user edits/confirms. One delivery path for everything: literal text
   (+ Enter) via herdr.
3. **Dedicated live agent channel**, NOT the report/`ask` pipeline. Blocked-agent
   state is live and ephemeral; model it as real-time state with its own
   endpoint + SSE + push trigger, rather than synthesizing durable `report.json`s.
4. **Data source = herdr adapter, stdlib-only.** Shell out to `herdr … --json`
   (or read the Unix socket at `~/.config/herdr/herdr.sock` directly). No new Go
   dependency; honors the repo's zero-`require` constraint.

## Architecture

A new `internal/herdr` adapter + a parallel agent-status watcher tick that
mirrors the existing report watcher, feeding a new live channel.

### `internal/herdr` (new package) — the adapter

Pure functions over the herdr CLI/socket; no server coupling. Surface:

- `List(ctx) ([]Agent, error)` — wraps `herdr agent list --json`; parses the
  `result.agents[]` array. Fields needed per agent (confirmed from live output):
  `agent` (label), `agent_status` (`idle|working|blocked|done|unknown`), `cwd`,
  `focused` (bool), `workspace_id`, `tab_id`, `pane_id`, `terminal_id`, and
  `agent_session.value` (claude session id, when present).
- `Read(ctx, target, opts) (string, error)` — wraps `herdr agent read <target>
  --source visible` to capture the on-screen question text when an agent blocks.
- `Send(ctx, target, text) error` — delivers the answer. Default path: literal
  text **+ Enter** (`herdr pane run <pane> <text>`); a no-Enter variant
  (`herdr agent send`) is available if a prompt needs raw keys.
- **TUI-hang guard:** all `exec.Command` calls leave `Stdin` nil (= /dev/null)
  and use `CommandContext` for timeout, per the repo's hard-won rule.
- **Binary resolution:** reuse the `opencodeBin()` lesson — resolve `herdr` via
  `$PATH` then probe `/opt/homebrew/bin`, `/usr/local/bin`, `~/.local/bin`
  (the dashboard runs under launchd's minimal PATH). Factor a shared
  `resolveBin(name, …)` helper.

### Watcher — a parallel tick

Mirror the existing report watcher exactly. `s.watch(pollInterval)` (server.go)
already runs every `pollInterval` (2s) and calls `s.tick(watchState)`
(sse.go) which fires Web Push for **newly-appeared open asks** with an
`askRetainTicks` debounce against flicker. Add a sibling:

- Extend `watchState` (or add a parallel state) with the last-seen set of
  blocked agents, keyed by a stable agent key (`pane_id`, already globally unique — it embeds
  the workspace, e.g. `w6544b3b0f2d752:p1`).
- Each tick: `herdr.List`. Diff against last-seen. For each agent that is
  **newly** `blocked` (status transitioned into blocked, not seen-blocked last
  tick), and is **not** `focused` (the user is already looking at it):
  `herdr.Read` the pane → build a `BlockedAgent` item → fire **one** Web Push
  (reuse the `push` package + `subs` store; mirror the dedupe in `push.go` so a
  still-blocked agent doesn't re-page every 2s) → `hub.broadcastEvent` so any
  open live view updates.
- Clear the item when the agent leaves `blocked`. Apply the same retain-ticks
  debounce so a 1-tick status flicker doesn't drop/re-page.
- herdr unreachable (socket down / binary absent) degrades gracefully: empty
  list, no error surfaced to the user — consistent with the usage providers.

### Live channel — endpoint + view

- `GET /api/agents` — current agent list + the open blocked items (JSON). Live
  updates via the existing `/events` SSE (`hub`), a new event name (e.g.
  `agents`) so report consumers ignore it.
- A minimal **needs-you** surface, phone-first: list of blocked agents, each
  showing project + agent label + captured question + the answer control
  (buttons-prefill + free-text + confirm-send). At the desk this can be a small
  inbox affordance; it is deliberately not a full fleet board.

### Answer → delivery (write-back)

- The answer control POSTs to a new `POST /api/agents/{key}/answer` with the
  final (confirmed, possibly edited) text.
- Server **re-reads** the agent's status immediately before send; if it's no
  longer `blocked`, reject with a clear "agent already moved on" so a stale
  answer never lands in a live session.
- On success: `herdr.Send(target, text)` (text + Enter). Broadcast the cleared
  state.

## Data model

```
type Agent struct {
  Key         string // = PaneID; globally unique, embeds workspace
  Label       string // "claude" | "codex" | "hermes" | …
  Status      string // idle|working|blocked|done|unknown
  Project     string // basename(cwd)
  Cwd         string
  Focused     bool
  PaneID      string
  WorkspaceID string
  SessionID   string // agent_session.value, optional
}

type BlockedAgent struct {
  Agent
  Question string    // captured pane text (trimmed)
  Since    time.Time // first observed blocked (server clock)
}
```

## Policies / defaults (vetoable)

- Page on each **new** block; **suppress the agent currently `focused`** in herdr.
- One push per block (no re-page while still blocked); `askRetainTicks`-style
  debounce on both appear and clear.
- Re-check status right before send; refuse stale answers.

## Risks + mitigations

1. **Keystroke fidelity** (a blind "yes" sends the wrong keys to some agents) →
   buttons prefill, human confirms, one literal-text+Enter path.
2. **Noise at ~11 agents** → focused-suppression + page-once + (future knob) a
   per-project allowlist if it proves noisy. v1 pages all non-focused blocks.
3. **Stale answer** (agent advanced before you tapped) → pre-send status re-check.
4. **herdr API drift** (CLI/socket shape changes across herdr versions) → isolate
   all parsing in `internal/herdr`; tolerate unknown fields; degrade to empty.
5. **Security/trust** — local single-user tool; the write-back sends text into a
   local terminal the user owns. No new external surface; keep the answer
   endpoint same-origin like the existing respond route.

## Open questions

- Direct Unix-socket protocol vs shelling `herdr … --json`: default to shelling
  (simplest, version-tolerant); revisit only if latency/parsing forces it.
- Whether `herdr agent read --source visible` reliably contains the full prompt
  for every integrated agent (claude/codex differ) — validate against live
  blocked agents during the build; may need `--lines N` tuning.

## Acceptance / verify

- `internal/herdr` unit tests parse a captured `agent list --json` fixture and a
  `read` fixture (binary-free, like the opencode stats parser).
- Watcher tick test: a fixtured status transition idle→blocked fires exactly one
  push and one SSE; staying blocked does not re-fire; blocked→working clears.
- `GET /api/agents` returns the live set; `POST …/answer` calls the adapter and
  refuses when status≠blocked (table test with a fake adapter).
- `go build ./... && go test ./...` green; `gofmt` clean; zero new `go.mod`
  requires.
- Live end-to-end: with a real blocked agent, the phone receives a push, the
  answer delivers, and the agent resumes (manual, human-verified).
