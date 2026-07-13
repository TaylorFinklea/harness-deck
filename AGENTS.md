# AGENTS.md

Instructions for any AI coding agent working with this repository (Claude Code,
Codex, OpenCode, Cursor, Copilot, Pi, Gemini, …). `CLAUDE.md` imports this file.

**Route yourself first:**

- Asked to **install / set up harness-deck** on the user's machine → [Installing
  harness-deck](#installing-harness-deck) below. You do **not** need to build
  from source or understand the architecture.
- Asked to **publish a report** to a running harness-deck → you don't need this
  repo at all. Run `hdeck contract` (the schema ships inside the binary), or read
  [`CONTRACT.md`](CONTRACT.md) / [`docs/PUBLISHING.md`](docs/PUBLISHING.md).
- Asked to **change harness-deck's code** → [Architecture](#architecture) and
  everything below it.

## What this is

harness-deck is a local Go dashboard for AI coding work. Any AI harness writes a
structured JSON **report manifest** (`report.json`); harness-deck renders it into
consistent themed HTML, aggregates every report into a live dashboard, and routes
the user's answers to interactive blocks back to the harness as `responses.json`.

## Installing harness-deck

Full runbook: [`docs/SETUP.md`](docs/SETUP.md). The short version, which is
correct on both macOS and Linux:

```sh
brew install taylorfinklea/tap/harness-deck   # installs `harness-deck` + `hdeck` alias
brew services start harness-deck              # start now + on login (launchd/systemd)
hdeck doctor                                  # verify; every failure prints its fix
hdeck open                                    # open the dashboard (--print over SSH: no GUI)
```

Ask the user where their code lives (e.g. `~/git`) and set `scan_roots`
accordingly — see the config rule below. Without it the projects view is empty,
which is the first thing they'll notice.

Then confirm `hdeck doctor` exits 0 before telling the user it's done. Do not
report success on a doctor run you didn't actually read — a FAIL there is the
difference between a working install and one the user discovers is broken later.

Rules for doing this well:

- **Check for an existing install first.** If `hdeck version` already reports a
  version, this is an upgrade: use `brew upgrade harness-deck`. A host set up
  before v0.2.14 also has a **hand-rolled LaunchAgent/systemd unit that must be
  removed**, or it will start a second server at login and fight the Homebrew
  service for the port (the loser crash-loops). Homebrew's own unit is
  `homebrew.mxcl.harness-deck`; anything else matching `harness` in
  `~/Library/LaunchAgents` or `~/.config/systemd/user` is stale. Removal
  commands: [SETUP.md step 4](docs/SETUP.md). Use `rm -f` — a bare `rm` can hit
  an interactive prompt, silently no-op, and leave the landmine in place.
- **`brew services` is the persistence path on both OSes.** Do not hand-write a
  LaunchAgent plist or a systemd unit when Homebrew is available — the formula
  ships a service definition. Hand-rolled units are the fallback for `go install`
  / raw-binary hosts only ([SETUP.md Appendix A](docs/SETUP.md)). On Linux also
  run `sudo loginctl enable-linger "$USER"` so the service survives logout.
- **`hdeck doctor` is the verification step**, not `curl`. It checks the config
  (including `usage.providers` typos and an empty `scan_roots`), TLS cert
  validity/expiry/hostname, push keys, whether the server actually answers —
  including on the non-loopback interface a phone uses — Tailscale, and the macOS
  firewall. Every failure prints a concrete fix. `--json` for machine-readable
  output; exit 1 on any FAIL. `brew services start` is asynchronous, so if doctor
  reports `nothing listening` immediately after starting the service, wait a
  second and rerun once before treating it as real.
- **It runs with no config file at all** (every field defaults: `127.0.0.1:7420`,
  reports in `~/.harness/reports`). Only write
  `~/.config/harness-deck/config.json` to change something — most commonly
  `scan_roots` (e.g. `["~/git"]`) so the projects view discovers repos, since that
  has no default. Doctor warns when it's missing.
- **Never `go get` a dependency** to make an install work. See the zero-dependency
  constraint below.

### Phone / Tailscale / push (optional)

HTTPS is required for iOS push. Set `"bind": "0.0.0.0"` in the config, then:

```sh
hdeck cert                        # Tailscale HTTPS cert; writes files + patches config
hdeck vapid                       # one-time push identity
brew services restart harness-deck
hdeck doctor
```

Do **not** run `tailscale cert --cert-file <path>` directly — the Mac App Store
build of Tailscale is sandboxed and cannot write to any path (it fails with
`operation not permitted` even for `/tmp`). `hdeck cert` reads the PEM over stdout
and writes the files itself, which works on every Tailscale build.

Certs last 90 days and nothing auto-renews them: schedule `hdeck cert --renew`
(a no-op while >30 days remain) or act on doctor's expiry warning.

### macOS firewall

If the dashboard answers on `127.0.0.1` but times out from a phone, the macOS
Application Firewall is dropping inbound connections. Signed release binaries
(v0.2.14+) are auto-allowed and need nothing. Binaries built locally with
`go install` are ad-hoc signed and can still be blocked — silently, with no log
entry. `hdeck doctor` detects this and prints the `socketfilterfw` fix.

### MCP (optional)

Lets an agent publish reports without shelling out. Not needed for the dashboard
to run.

```sh
claude mcp add harness-deck -- hdeck mcp        # Claude Code
```

Other MCP-capable harnesses: register a stdio server named `harness-deck`, command
`hdeck`, args `["mcp"]`.

## Commands

```sh
go build ./...                              # build everything
go test ./...                               # run all tests
go test ./internal/render -run TestReport   # run one package / one test
./harness-deck serve                        # start the dashboard (default :7420)
./harness-deck open                         # open the dashboard in a dedicated window
./harness-deck doctor                       # preflight checks; every FAIL prints its fix
./harness-deck cert                         # Tailscale HTTPS cert, wired into config
./harness-deck vapid                        # generate the VAPID keypair for push (one-time)
./harness-deck validate report.json         # check a manifest
./harness-deck render report.json -o out.html
./harness-deck new --title "first report"   # scaffold a starter report.json
./harness-deck register /path/to/project    # add a project root to the config
./harness-deck contract                     # print the embedded report contract (--publishing for the guide)
./harness-deck version
```

There is no Makefile or linter config — `go build` / `go test` are the whole
toolchain. `gofmt` before committing.

Manual end-to-end testing uses fixtures outside the repo; point `serve` at an
alternate config with the env override:

```sh
HARNESS_DECK_CONFIG=/tmp/hd-test/config.json ./harness-deck serve
```

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
- **`usage`** — opt-in footer usage monitors. `Build`'s switch is the list of
  valid provider ids; `UnknownProviders` (pinned to it by test) is what lets
  `doctor` name a typo instead of silently ignoring it.
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
  (required for iOS push); issue them with `hdeck cert`.
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
  sensible fallback rather than an error. This extends to writes — `hdeck cert`
  patches `config.json` as a raw map so fields it doesn't know survive.
- Trust boundary: harness-deck is a local single-user tool, so the `html`
  block and `safeHTML` template func intentionally bypass HTML escaping.

## Release

Tag `vX.Y.Z` and push the tag; `.github/workflows/release.yml` runs GoReleaser,
which builds the binaries, **signs + notarizes the macOS ones** (Developer ID,
via quill — gated on the `MACOS_SIGN_P12` secret), publishes the GitHub Release,
and commits the formula to `TaylorFinklea/homebrew-tap`.

Signing is load-bearing, not cosmetic: the macOS Application Firewall auto-allows
Developer-ID-signed binaries and silently drops inbound connections to ad-hoc
ones, which is what makes the phone/Tailscale path work out of the box.

## Handoff docs (`.docs/ai/`)

`.docs/ai/{roadmap,current-state,decisions}.md` are the git-tracked source of
truth for cross-session continuity. Read them at the start of a session and
update them at the end — `current-state.md` is a short breadcrumb (blockers,
build status, recent progress), `decisions.md` is an append-only ADR log.
Multi-session design work lives in `.docs/ai/phases/<slug>-{spec,report}.md`.
