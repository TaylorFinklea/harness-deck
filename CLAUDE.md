# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

harness-deck is a local Go dashboard for AI coding work. Any AI harness writes a
structured JSON **report manifest** (`report.json`); harness-deck renders it into
consistent themed HTML, aggregates every report into a live dashboard, and routes
the user's answers to interactive blocks back to the harness as `responses.json`.

## Commands

```sh
go build ./...                              # build everything
go test ./...                               # run all tests
go test ./internal/render -run TestReport   # run one package / one test
./harness-deck serve                        # start the dashboard (default :7420)
./harness-deck open                         # open the dashboard in a dedicated window
./harness-deck validate report.json         # check a manifest
./harness-deck render report.json -o out.html
./harness-deck new --title "first report"   # scaffold a starter report.json
./harness-deck register /path/to/project    # add a project root to the config
./harness-deck contract                     # print the embedded report contract (--publishing for the guide)
./harness-deck vapid                        # generate the VAPID keypair for push (one-time)
```

There is no Makefile, linter config, or CI — `go build` / `go test` are the
whole toolchain. `gofmt` before committing.

Manual end-to-end testing uses fixtures outside the repo; point `serve` at an
alternate config with the env override:

```sh
HARNESS_DECK_CONFIG=/tmp/hd-test/config.json ./harness-deck serve
```

## Host setup requests

When a user says "set up harness-deck" or asks for install help on a new host,
use [`docs/SETUP.md`](docs/SETUP.md) as the runbook. Do the setup for the
actual OS:

- Homebrew install when `brew` is available: `brew install
  taylorfinklea/tap/harness-deck`.
- macOS persistence: user LaunchAgent.
- Linux persistence: user systemd service plus `sudo loginctl enable-linger
  "$USER"`.
- Config path: `~/.config/harness-deck/config.json`.
- Local default: `bind` `127.0.0.1`, `port` `7420`, `scan_roots` like `~/git`.
- Phone/Tailscale default: `bind` `0.0.0.0`, set `public_url`, add TLS cert/key
  only when HTTPS or push is needed, run `hdeck vapid` for push.
- Verify with `hdeck open --print` and `/api/reports`.

Do not mix service managers: `loginctl` is Linux/systemd-only; `launchctl` is
macOS-only.

## Hard constraint: zero external dependencies

`go.mod` has **no `require` block** — the project is stdlib-only on purpose
(see `.docs/ai/decisions.md`). Do not `go get` anything. This is why the repo
has an in-house Markdown renderer (`internal/render/markdown.go`), JSON config
instead of TOML, and a 2s polling watcher instead of `fsnotify`. If a task
seems to need a library, reach for stdlib or stop and ask.

The frontend is likewise build-step-free: vanilla HTML/CSS/JS vendored under
`internal/assets/` and embedded with `go:embed`. No npm, no bundler.

## Architecture

The data flow is a one-way pipeline with a file-based response loop:

```
harness → report.json → store (discovers) → render → server (serves)
                                                         ↓
harness ← responses.json ← respond (writes) ← server (records answer) → notify
```

Packages (`internal/`), each with a package doc comment that is the best
starting point:

- **`manifest`** — the report schema. **Go structs are the schema** (no
  `report.schema.json`). `Parse` is lenient (unknown block types leave `Body`
  nil, unknown fields ignored) so a stale renderer degrades gracefully;
  `Validate` is strict (`DisallowUnknownFields` + enum/semantic checks).
- **`render`** — `html/template` → a complete v1-TUI HTML page. The renderer
  **owns every byte of layout and CSS**; manifests carry content only. One bad
  block renders a fallback error panel instead of failing the whole page.
- **`store`** — discovers `report.json` under the central dir and each
  given project's `.harness/`; in-memory index; `Signature()` fingerprint
  drives change detection.
- **`projects`** — discovers project roots (depth-1 children of `scan_roots`
  holding a `.docs/ai` dir, plus explicit `projects`); persists which ones the
  user hid in an app-owned `projects.json`; new projects default to enabled.
- **`push`** — Web Push delivery, stdlib-only: VAPID (RFC 8292) keypair gen
  + JWT signing, RFC 8291 `aes128gcm` payload encryption (HKDF + AES-GCM),
  HTTP POST to subscription endpoints, plus a JSON-backed subscription store.
  The watcher fires one push per newly-appeared open ask.
- **`server`** — aggregator shell, `/api/reports`, `/api/projects`, `POST
  /api/projects/toggle`, `/events` (SSE), `/r/{project}/{run}` report pages,
  `POST .../respond`. PWA endpoints: `/manifest.webmanifest`,
  `/service-worker.js`, `/icon.svg`. Push endpoints under `/api/push/*`. A 2s
  watcher polls store + projects and broadcasts SSE.
- **`respond`** / **`notify`** — `responses.json` read/write; the configured
  notify command fired when an answer is recorded.
- **`config`** — JSON config at `~/.config/harness-deck/config.json`; every
  field defaults, so it runs with no config file. `scan_roots` lists
  directories searched for project roots. `bind` (default `127.0.0.1`)
  controls the listen interface — set it to a Tailscale IP or `0.0.0.0`
  to reach the dashboard from a phone. `tls.cert` + `tls.key` enable HTTPS
  (required for iOS push); generate certs once with `tailscale cert`.
- **`assets`** — vendored frontend files embedded for self-contained output.

## Key conventions

- **`CONTRACT.md` is the agent-facing spec and must stay in sync with the
  `manifest` structs.** Any change to report/block fields updates both.
- **The binary is self-describing.** `embed.go` at the repo root
  (`package harnessdeck`) `go:embed`s `CONTRACT.md` + `docs/PUBLISHING.md`, so
  the `contract` subcommand, the MCP `harness-deck://contract` resource, and
  the HTTP `GET /contract.md` route all serve the same embedded bytes —
  version-locked to the build, no repo clone needed. The MCP `initialize`
  handshake also returns an `instructions` string (`internal/mcp/resources.go`)
  so an MCP-connected agent learns when to publish without the skill file. Edit
  `CONTRACT.md` in place; the embed picks it up at compile time.
- **Adding a block type** touches four places: a `Block` struct + registry
  entry in `internal/manifest/`, a `block-<type>` template in
  `internal/render/templates/`, a default title in `render.go`'s
  `defaultTitles` (non-interactive types only), and a row in `CONTRACT.md`.
  The `html` block is the deliberate escape hatch — recurring `html` usage
  is the signal to promote a pattern to a real typed block.
  `TestRegistryCrossCheck` in `internal/render` enforces all four places
  automatically — if you add a registry entry without the rest, the test
  will name exactly what's missing. Interactive types (`ask`/`decision`/
  `approval`) are exempt from `defaultTitles` but must still have a
  template and a CONTRACT.md row. If the new type produces searchable prose,
  add a case to `manifest.BlockText`; if it's interactive, confirm
  `manifest.BlockPrompt` covers it — both helpers live in `internal/manifest/`
  and the cross-check test exercises them.
- **Graceful degradation is a design rule**, not an accident: unknown block
  types, missing `responses.json`, and absent config files all resolve to a
  sensible fallback rather than an error.
- Trust boundary: harness-deck is a local single-user tool, so the `html`
  block and `safeHTML` template func intentionally bypass HTML escaping.

## Handoff docs (`.docs/ai/`)

`.docs/ai/{roadmap,current-state,decisions}.md` are the git-tracked source of
truth for cross-session continuity. Read them at the start of a session and
update them at the end — `current-state.md` is a short breadcrumb (blockers,
build status, recent progress), `decisions.md` is an append-only ADR log.
Phases 0–5 are complete; further plan context lives in `roadmap.md`.

## Task tracking — beads pilot (2026-06-30)

**The forward backlog / "what to work on next" for this repo is piloted in [beads](https://github.com/steveyegge/beads) (`bd`), not the roadmap's Now/Next list.** Local-only stealth install — `.beads/` is git-excluded via `.git/info/exclude`; nothing is committed, so the pilot leaves no trace if dropped.

Agent loop (harness-agnostic — `bd` is just a CLI):
- `bd ready` — priority-sorted, dependency-aware queue of unblocked work (`--json`; `bd ready --claim --json` atomically claims the top item).
- `bd show <id>` — full detail before starting.
- `bd update <id> --claim` — set in_progress + assignee atomically.
- Run `go build ./... && go test ./...` first, then `bd close <id> --reason "…"`.
- `bd create "Title" -t task -p 2 -d "…"` — file work discovered mid-task; `bd dep add <new> <parent> -t discovered-from` records provenance.
- `bd dep add <issue> <blocker>` — `<issue>` is blocked-by `<blocker>` (hidden from `bd ready` until the blocker closes).

Layer split — beads owns ONLY the backlog/ready-queue. Do NOT migrate these into beads:
- **Rationale / ADRs → `.docs/ai/decisions.md`** (prose, unchanged).
- **Loop state → `.docs/ai/current-state.md`** (unchanged).
- `roadmap.md` keeps the durable narrative; new *actionable* work goes into `bd`.

`user-verify`-labeled issues = human merge/release/verify gates, not agent dev work. Part of a 5-repo pilot; see chezmoi-config `.docs/ai/phases/beads-pilot-spec.md`.
