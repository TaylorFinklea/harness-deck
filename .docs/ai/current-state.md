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
  `/api/search?q=` (cross-report content search; metadata + body, returns
  top 20 with snippet around first hit), `POST /api/projects/toggle`,
  `POST /api/projects/reorder`, `/events` (SSE),
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

Richer Markdown shipped as `v0.1.12`: tables, blockquotes, and links
(inline `[text](url)` + `<https://…>` autolinks) added to
`internal/render/markdown.go`. CSS additions to deck.css; CONTRACT.md
Markdown vocabulary updated. Still stdlib-only.

Roadmap polish shipped as `v0.1.13`: GitHub task lists (`- [x]` /
`- [ ]`) render with green/yellow checkbox glyphs and dimmed-completed
text; `---` horizontal rules become section dividers; trailing
`(DONE)` / `(WIP)` / `(planned)` / `(blocked)` tokens in headings
become colored status pills.

`new` + `register` subcommands shipped as `v0.1.14`. `harness-deck new
--title T` scaffolds a starter report.json (auto id, draft status,
prose placeholder; --in-repo writes under <repo>/.harness/; --force
overwrites). `harness-deck register <path>` atomically adds/removes a
project root from the config's `projects` array; reads+writes as a
generic map so unknown fields stay preserved.

Cross-report search shipped as `v0.1.15`: `GET /api/search?q=` scores
metadata + block body content for every non-archived entry, returns
top 20 with snippet markers (`[[match]]`) the frontend renders as
`<mark>` spans. `search.js` is a Cmd+K / Ctrl+K palette overlay
(debounced live-fetch, ↑/↓ navigate, Enter opens via HDTabs, Esc
closes). Titlebar gets a `?` button for touch users.

Theme switch + light mode shipped as `v0.1.16`: Tokyo Night Day
palette added as CSS variable overrides under both `@media
(prefers-color-scheme: light)` and `[data-theme="light"]`. Three-way
settings picker (system / dark / light) persists choice to
`localStorage('harness-deck:theme')`. A tiny inline preamble script
applies the saved preference before render so there's no
flash-of-wrong-theme.

Manual test fixtures live in `/tmp/hd-test/` (config + sample reports):
`HARNESS_DECK_CONFIG=/tmp/hd-test/config.json harness-deck serve`.

Apple Push 403 fix shipped as `v0.1.19`: Apple's push service rejects
mailto VAPID `sub` claims that don't resolve to a real public TLD
(`mailto:harness-deck@localhost` → 403). Switched to the repo URL —
Apple validates URL syntax, not reachability, so no operator PII is
leaked and the form is universally accepted (FCM + Mozilla Autopush
already worked silently). Confirmed end-to-end on a real iPhone over
the tailnet HTTPS pipeline.

PNG PWA icons + `docs/PUBLISHING.md` shipped on `main` (post-v0.1.19,
unreleased): prerendered hd.svg → 180/192/512/1024 PNG via
`rsvg-convert`, embedded under `internal/assets/`, served at
`/icon-{size}.png`, manifest + apple-touch-icon switched over;
service-worker cache bumped to `harness-deck-v2`. Superseded the
earlier "no PNG variants" decision — see decisions.md. PUBLISHING.md
is the on-ramp for external publishers (60-second smoke test, MVP
manifest, the four blocks that cover 90% of reports). Linked from
README + CONTRACT.

**v0.2.0 cohesive redesign (in flight 2026-05-27)** — see
`.docs/ai/v0.2.0-spec.md`. Driven by product discovery: heavy daily
use, IA pain (6 tabs → 2), incoherent keyboard model. Shipped in
three rcs:

- **rc1 (v0.1.24-rc1)** — inbox cursor: focused-row state by report
  id, persisted across renders via sessionStorage; bindings
  j/k/Enter/o/a/x/dd; capture-phase keydown so vim-nav's page-scroll
  doesn't fire alongside; INSERT mode tracking in vim-nav via
  focusin/focusout so the statusline reflects when typing is active.
- **rc2 (v0.1.24-rc2)** — IA collapse: 6 views → 2 (inbox +
  projects); settings becomes a modal overlay (gear button in
  titlebar opens it); archive becomes a chip on the inbox metric
  strip; old URLs (?v=overview/latest/archive/settings) migrate via
  history.replaceState. Each view gets an operational metric strip
  (awaiting · open asks · in-flight · today · archived for inbox;
  projects · updated this week · with asks · latest update for
  projects).
- **v0.2.0** — keyboard chord system: Space leader (Space-s settings,
  Space-t theme, Space-? cheat), g-prefix jumps (g-i inbox, g-p
  projects, g-a archive, g-g top), 1–9 → in-app tab N (Chrome-style),
  context-aware `?` help overlay listing every binding in six
  sections (movement / row actions / jumps / leader / tabs /
  commands). `:`-palette commands `:inbox`, `:projects`, `:archive`,
  `:settings`, `:cheat`, `:theme` registered via the new
  `VimNav.addCommand(name, fn, desc)` API. Chord timeout is 1500ms
  matching vim's default `timeoutlen`.

## Next

**Next roadmap wave (selected 2026-05-26)** — see roadmap.md:

- **MCP report-builder server** _(done — 2026-05-26)_ — stdio JSON-RPC
  server (`harness-deck mcp`) wrapping the file contract. Six tools:
  `publish_report`, `validate_report`, `get_responses`, `list_reports`,
  `update_status`, `update_live`. File contract stays canonical — MCP
  delegates to the same atomic-write path. `internal/mcp/` package,
  stdlib only.
- **Live in-flight telemetry** _(done — 2026-05-26)_ — optional `live`
  field on the manifest (`{updated, step, elapsed_ms, tokens, cost_usd,
  progress}`). Report page renders a pulsing banner above the open-asks
  list while `updated` is within 60 seconds; the inbox dot pulses for
  the same window. `update_live` MCP tool merges telemetry without
  rewriting unrelated fields — cheap to call every few seconds.
- **Per-project run history** _(done — 2026-05-26)_ — every run for a
  project, newest-first, with responses inlined. `/api/projects` grew a
  `history []historyRun` field per project; `viewProjects()` renders a
  "history" subsection with status dot + title + meta + chip-style
  responses (`block → value · time`). Also fixed the desktop `.layout`
  `1fr` foot-gun (`minmax(0, 1fr)` + `.content { min-width: 0 }`) so
  wide markdown / code can't push the content column past the viewport;
  same fix that mobile got, now applied to desktop too.
- **Notification fan-out** _(done — 2026-05-26)_ — Slack incoming
  webhook, Discord webhook, and generic POST webhook destinations
  configured under `notifications[]` in config.json. The 2s watcher
  fires `notify.Fanout(...)` alongside Web Push for every newly-open
  ask; per-destination optional `projects[]` allowlist filters routing.
  Fire-and-forget with per-attempt log line, no retry. Settings view
  gains a "notification destinations" panel with list/add/test/remove
  CRUD via `/api/notifications/*` endpoints — atomic config rewrite
  preserving unknown fields. URLs in the GET response are host-redacted
  so webhook tokens don't echo. `public_url` config field controls the
  link in chat (falls back to bind+port). Done across the v0.1.5 push
  pipeline + the v0.1.21 history surface, this closes the "extend
  usefulness" wave (MCP + live + history + fan-out).

Parking lot (not picked but logged): report templates
(`new --template <name>`), search filters + saved searches.

Phase 6 — harness-side integration — still lives in `chezmoi-config`,
not here. Hooks for Claude Code / Codex / OpenCode / Pi Mono landed
there during the v0.1.x cycle.

## Blockers / open questions

- None. The next-wave items above are unblocked; pick whichever is
  most useful to start.
