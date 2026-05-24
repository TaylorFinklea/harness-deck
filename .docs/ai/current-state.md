# harness-deck — current state

_Last-session breadcrumb. Not a journal — keep it short._

## Where things are

**All planned phases (0–5) are complete.** `go build ./... && go test ./...`
is green. The whole MVP works end-to-end and was verified in a browser.

What exists:

- **`internal/manifest`** — report manifest + 14 block types (11 content + 3
  interactive: ask/decision/approval). Lenient parse, strict `Validate`.
- **`internal/render`** — `html/template` renderer → the v1 TUI HTML page.
  Unknown/broken blocks degrade to a fallback panel. Dependency-free Markdown
  renderer (`markdown.go`, exported as `Markdown`).
- **`internal/store`** — discovers `report.json` under the central dir and the
  `.harness/` of each project root it is given; in-memory index; `Signature()`
  fingerprint for change detection; `OpenAsks` excludes answered blocks.
- **`internal/projects`** — discovers project roots (depth-1 children of
  `scan_roots` holding `.docs/ai`, plus explicit `projects`); `projects.json`
  records hidden ones (new = enabled); `Toggle` writes it atomically.
- **`internal/server`** — aggregator shell, `/api/reports`, `/api/projects`,
  `POST /api/projects/toggle`, `POST /api/projects/reorder`, `/events` (SSE),
  `/r/{project}/{run}` report pages, `POST …/respond`, plus report lifecycle:
  `POST …/close` (status→done), `POST …/reopen` (status→awaiting-review,
  preserves all other fields via map[string]any round-trip + atomic write),
  `DELETE …` (removes the run directory). PWA: `/manifest.webmanifest`,
  `/service-worker.js`, `/icon.svg`. Push: `GET /api/push/vapid-key`,
  `GET /api/push/status`, `POST /api/push/subscribe`, `POST
  /api/push/unsubscribe`. A 2s watcher polls store + projects, broadcasts
  SSE, and diffs open-ask block IDs to fire one push per newly-appeared
  ask (first poll seeds the baseline so startup never spams).
- **`internal/respond`** / **`internal/notify`** — `responses.json` read/write;
  the configured notify command.
- **`internal/config`** — JSON config at `~/.config/harness-deck/config.json`;
  `scan_roots` lists directories searched for project roots; `bind`
  (default `127.0.0.1`) picks the listen interface so a phone on Tailscale
  can reach the dashboard; `tls.cert`/`tls.key` enable HTTPS (needed for
  iOS push, generated once via `tailscale cert`).
- **`internal/push`** — stdlib-only Web Push: VAPID keypair gen + ES256 JWT
  (`vapid.json` next to config.json), RFC 8291 `aes128gcm` payload
  encryption, HTTP sender, and a JSON-backed `Store` for browser
  subscriptions (`subscriptions.json`). Encryption is roundtrip-tested
  against the spec.
- **`internal/assets`** — vendored design CSS/JS + `aggregator.js` / `respond.js`
  + PWA bundle (`manifest.webmanifest`, `service-worker.js`, `mobile.css`,
  `mobile.js`; `hd.svg` doubles as the manifest + apple-touch icon).
  The 4th aggregator view is **projects**: per-project current-state + roadmap,
  with a collapsible "tracked projects" panel of discovery toggles and HTML5
  drag-and-drop reordering (⋮⋮ handle; order persists to `projects.json` and
  propagates to the projects view, the toggle panel, and the sidebar tree).
  The 5th view is **settings**: phone-push enable / disable for the current
  browser, with a server-status pill and per-device subscription state.
  Inbox rows have a hover-revealed ✕ close action; the report page top bar
  carries ✕ close / ↺ reopen / ⌦ delete (delete confirms natively).
- **`cmd/harness-deck`** — `validate`, `render`, `serve`, `vapid`, `version`.
- **`CONTRACT.md`** — the agent-facing report spec.
- **`CLAUDE.md`** — repo guidance for Claude Code (architecture, the
  zero-dependency constraint, the four-places block-type checklist).
- **Release pipeline** — `.goreleaser.yaml` + `.github/workflows/release.yml`:
  pushing a `v*` tag builds static darwin/linux binaries, publishes a GitHub
  Release, and commits the formula to `TaylorFinklea/homebrew-tap`. The formula
  installs `harness-deck` plus a short `hdeck` symlink alias. Install via `brew
  install taylorfinklea/tap/harness-deck` or `go install`. `v0.1.1` is the
  latest released tag (it carries the `hdeck` alias).

Deviations from the plan (all in decisions.md): zero external dependencies — Go
structs + `CONTRACT.md` instead of a `report.schema.json`; JSON config not TOML;
in-house Markdown renderer; live updates by polling, not fsnotify.

Project discovery + tracking (the `projects` package, `/api/projects`, the
projects view) was built on branch `feat/project-discovery` — see decisions.md.
Drag-and-drop reordering and report close/reopen/delete landed on `main`
after the projects view shipped (commits `0a8bfbb`, `df97c80`).

The mobile PWA + Web Push pipeline (`internal/push`, the PWA asset bundle,
the settings view, the `vapid` subcommand, opt-in TLS) shipped as `v0.1.5`
— see decisions.md.

The visibility/archive/tabs/code-copy round shipped as `v0.1.6`: pinned
open-asks banner + titlebar counter, soft-archive flag + archive view
(`POST /r/{p}/{r}/archive` + `unarchive`), fenced code block rendering
with copy button, recommendations through full Markdown, `:` command
palette autocomplete (wildmenu-style with Tab/↑/↓), in-app tab strip
persisted to localStorage with `gt`/`gT` cycling.

Keyboard triage (commit pending): `internal/assets/triage.js`. First
unanswered ask auto-focuses; digits pick option N, y/n shortcut yes/no,
Enter submits highlighted, i focuses text input, Tab/Shift+Tab skip
between asks, Esc defocuses. Help overlay (`?`) lists the bindings.

Manual test fixtures live in `/tmp/hd-test/` (config + sample reports):
`HARNESS_DECK_CONFIG=/tmp/hd-test/config.json harness-deck serve`.

## Next

Phase 6 — harness-side integration — lives in `chezmoi-config`, not here: thin
hooks/skill so Claude Code / Pi Mono / OpenCode emit manifests and read
`responses.json`, plus an optional MCP report-builder. Not started.

Possible follow-ups in this repo: `harness-deck register <path>` and `new`
(scaffold) subcommands; richer Markdown; report-page live reload;
prerendered PNG PWA icons (currently SVG-only — may render rough on
older iOS home screens); auto-renew for Tailscale TLS certs.

## Blockers / open questions

- None. Mobile PWA + push need real-device testing: Tailscale connect from
  phone, `tailscale cert` + cfg, install to home screen, enable
  notifications in /settings, trigger an ask, confirm the lock-screen
  push and the deep-link.
