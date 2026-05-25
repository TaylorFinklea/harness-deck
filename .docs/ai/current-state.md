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
- **`cmd/harness-deck`** — `validate`, `render`, `serve`, `new`,
  `register`, `vapid`, `version`. `new` scaffolds a starter report.json
  (id auto = `YYYYMMDD-HHMMSS`, status `draft`, prose placeholder
  block; `--in-repo` writes to `<repo>/.harness/<id>/` instead of the
  central reports dir). `register` atomically adds/removes a project
  root from the config's `projects` array, preserving every other
  field (forward-compat with unknown future config keys).
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

Keyboard triage shipped as `v0.1.7` (`internal/assets/triage.js`). First
unanswered ask auto-focuses; digits pick option N, y/n shortcut yes/no,
Enter submits highlighted, i focuses text input, Tab/Shift+Tab skip
between asks, Esc defocuses. Help overlay (`?`) lists the bindings.

Mobile polish shipped as `v0.1.8` + `v0.1.9`: real-device testing on
iPhone Safari surfaced multiple bugs — view-tab strip overflowing
viewport, CSS bundle order making mobile overrides lose to desktop
selectors, and (the real one) `.layout` `grid-template-columns: 1fr`
resolving to `minmax(auto, 1fr)` so a wide descendant pushed the page
past the viewport. Fixed via `minmax(0, 1fr)` plus `min-width: 0` on
overflow-bearing containers; asset bundle reordered so MobileCSS is
always last (`baseCSS + AggregatorCSS + MobileCSS`).

Report-page live reload shipped as `v0.1.10`: the watcher now hashes
responses.json mtime into the store signature so cross-device answers
fire SSE. New `GET /r/{p}/{r}/sig` returns `{exists, sig, archived,
status}`. `internal/assets/live.js` opens EventSource on the report
page, fetches /sig on each change event, and reloads if the
fingerprint diverges (or redirects to / if the report is gone). 2s
post-load grace plus single-in-flight debounce prevents the
respond.js double-reload.

Pull-to-refresh shipped as `v0.1.11`: both iOS Safari and Chrome strip
native PTR when display-mode is standalone, so installed PWAs had no
gesture for forcing a reload. `mobile.js` adds a touch-based PTR gated
on standalone (no native-PTR double trigger), damped rubber-band pull
with 70px threshold, 120ms confirmation flash before reload.

Richer Markdown (commit pending): tables, blockquotes, and links
(inline `[text](url)` + `<https://…>` autolinks) added to
`internal/render/markdown.go`. CSS additions to deck.css; CONTRACT.md
Markdown vocabulary updated. Still stdlib-only.

Manual test fixtures live in `/tmp/hd-test/` (config + sample reports):
`HARNESS_DECK_CONFIG=/tmp/hd-test/config.json harness-deck serve`.

## Next

Phase 6 — harness-side integration — lives in `chezmoi-config`, not here: thin
hooks/skill so Claude Code / Pi Mono / OpenCode emit manifests and read
`responses.json`, plus an optional MCP report-builder. Not started.

Possible follow-ups in this repo: `harness-deck register <path>` and `new`
(scaffold) subcommands; richer Markdown (tables, links, blockquotes);
prerendered PNG PWA icons (currently SVG-only — may render rough on
older iOS home screens); auto-renew for Tailscale TLS certs.

## Blockers / open questions

- None. Mobile PWA + push need real-device testing: Tailscale connect from
  phone, `tailscale cert` + cfg, install to home screen, enable
  notifications in /settings, trigger an ask, confirm the lock-screen
  push and the deep-link.
