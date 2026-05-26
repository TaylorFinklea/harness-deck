# harness-deck — roadmap

## Vision

One place where any AI coding harness can publish a consistently-formatted report
— progress, an opinion request, a decision, an idea — and where the user reviews
an upcoming roadmap. Reports are authored as a JSON block manifest; a local Go
server renders, aggregates, and live-updates them, and routes the user's
responses back to the harness.

## Milestones

- **Phase 0 — repo skeleton.** Repo, Go module, vendored design assets, handoff
  docs. _(done)_
- **Phase 1 — manifest + renderer.** Go manifest types; renderer for every
  block type incl. `html` escape hatch + Markdown; page shell; CLI `validate` /
  `render`. _(done — Go structs are the schema; see decisions.md)_
- **Phase 2 — discovery + aggregator.** Config, scan central + per-project dirs,
  aggregator shell page, `harness-deck serve`. _(done)_
- **Phase 3 — live updates.** Change detection → SSE → live tree/view updates.
  _(done)_
- **Phase 4 — response round-trip.** Interactive block controls, `/respond`,
  `responses.json`, notification command. _(done)_
- **Phase 5 — roadmap view.** Render each project's `.docs/ai/roadmap.md` plus
  reports of kind `roadmap`. _(done)_

All planned phases (0–5) are complete. The next step is Phase 6 — harness-side
integration — which lives in `chezmoi-config`, not this repo.

## Post-MVP additions

- **Project discovery + tracking** _(done — 2026-05-22)_ — `scan_roots`
  auto-discovers project roots; the roadmap view became the **projects** view
  (per-project current-state + roadmap) with a collapsible panel of visibility
  toggles persisted to `projects.json`. See decisions.md.
- **Mobile PWA + Web Push** _(done — 2026-05-23)_ — phone-friendly responsive
  CSS, installable manifest + service worker, stdlib-only Web Push pipeline
  (`internal/push`), `harness-deck vapid` subcommand, opt-in `bind` + `tls`
  config, settings view for per-browser subscribe/unsubscribe. Connectivity
  model is Tailscale; HTTPS via `tailscale cert`. See decisions.md.
- **Asks visibility, archive, in-app tabs, code copy** _(done — 2026-05-24)_
  — pinned unanswered-asks banner at the top of every report; aggregate
  open-asks counter in the titlebar + browser tab title; soft-archive
  with a recoverable archive view; fenced code blocks render with a copy
  button; recommendations bodies pass through full Markdown; `:` command
  palette gains wildmenu-style autocomplete; in-app tab strip pins
  multiple report URLs in localStorage with `gt`/`gT` to cycle.

- **Keyboard triage** _(done — 2026-05-24)_ — first unanswered ask
  auto-focuses on report load; digits pick option N, `y`/`n` map to
  yes/no or approve/changes-requested, `Enter` submits the highlighted
  choice, `i` jumps into the text-mode input, `Tab` / `Shift+Tab` skip
  between unanswered blocks, `Esc` defocuses. Page reload after submit
  carries focus naturally to the next unanswered ask, so the inner loop
  is "press digit, page reloads, press digit". `?` help overlay lists
  the triage shortcuts.
- **Mobile polish** _(done — 2026-05-24)_ — v0.1.8 + v0.1.9: real iPhone
  testing surfaced a CSS Grid `1fr` foot-gun (the default `minmax(auto,
  1fr)` lets a wide descendant push the whole page past the viewport)
  plus a wrong asset-bundle order. Fixed with `minmax(0, 1fr)`,
  `min-width: 0` on overflow containers, MobileCSS always last in the
  bundle, and an inbox-row layout that stacks vertically below 720px.
- **Report-page live reload** _(done — 2026-05-25)_ — store signature
  now hashes responses.json mtime so cross-device answers trigger SSE;
  `GET /r/{p}/{r}/sig` exposes a per-report fingerprint; `live.js` on
  the report page reloads when the server-side sig diverges (or
  redirects to / if the report is gone). 2s post-load grace prevents
  respond.js's own reload from triggering a second one.
- **Pull-to-refresh on PWAs** _(done — 2026-05-25)_ — both iOS Safari and
  Chrome strip native pull-to-refresh when display-mode is standalone, so
  installed PWAs had no way to force a reload. Added a touch-based PTR in
  mobile.js gated on standalone (no double-trigger with browser native PTR),
  damped pull with rubber-band feel, 70px threshold, 120ms confirmation
  flash before location.reload().
- **Richer Markdown** _(done — 2026-05-25)_ — GitHub-style tables
  (`| h | h |` header + `| --- |` separator + rows), `> ` blockquotes, and
  links (`[text](url)` inline + `<https://…>` autolinks) added to the
  in-house Markdown renderer. Style ride-along in deck.css; CONTRACT.md
  Markdown vocabulary updated. Stdlib-only — still zero external deps.
  Sample report at `~/.harness/reports/acme/markdown-demo/report.json`.
- **Roadmap polish** _(done — 2026-05-25)_ — GitHub task lists
  (`- [x]` / `- [ ]`) render with green/yellow checkbox glyphs and
  dimmed-completed body text; `---` horizontal rules become section
  dividers; trailing `(DONE)` / `(WIP)` / `(planned)` / `(blocked)`
  tokens in headings render as colored status pills. `.roadmap-md` gets
  more breathing room around H2/H3 with a subtle bottom border on H2 so
  wave/phase sections feel structured.
- **CLI: `new` + `register` subcommands** _(done — 2026-05-25)_ —
  `harness-deck new --title T` scaffolds a starter report.json with
  sensible defaults (auto id, draft status, prose placeholder).
  `harness-deck register <path>` atomically adds a project root to
  the config's `projects` array; `--remove` removes it. Both use the
  shared atomic-write pattern and preserve unknown config fields.
- **Cross-report search** _(done — 2026-05-25)_ — new `GET /api/search?q=`
  walks all non-archived entries, scores metadata + block body content
  (case-insensitive substring), returns top 20 matches with a snippet
  around the first body hit (markers around the match for client-side
  `<mark>` highlighting). Frontend: `search.js` adds a Cmd+K / Ctrl+K
  palette overlay (debounced live-fetch, ↑/↓ navigate, Enter opens via
  HDTabs, Esc closes). Titlebar gets a `🔎` button for touch users.
- **Theme switch + light mode** _(done — 2026-05-25)_ — added Tokyo Night
  Day palette overrides under both `@media (prefers-color-scheme: light)`
  and `[data-theme="light"]` so default behavior follows the OS. Settings
  view picks up a three-way segmented control (system / dark / light)
  persisted to `localStorage('harness-deck:theme')`. A tiny inline
  preamble script applies the saved preference before render to avoid
  flash-of-wrong-theme.

## Next up — extend usefulness (2026-05-26)

Selected together as the next four leverage points. The MVP loop is done;
these turn harness-deck from "renders agent output" into "live ops pane +
retrospective + multi-channel notifier."

- **MCP report-builder server** — an MCP server that wraps the file
  contract so harnesses with MCP support can emit reports via tool
  calls instead of writing JSON directly. Explicitly **optional**: the
  file path stays the canonical, durable interface (`CONTRACT.md`).
  MCP is a thin convenience layer that translates tool calls into the
  same file writes a harness would have done by hand. Same atomic-write
  + validate pipeline underneath.
- **Live in-flight telemetry** — extend the manifest with an optional
  `live` field (or a typed `live` block): current step, elapsed,
  token/$ usage, last-update timestamp. Dashboard renders an active
  pulse + progress indicator for runs whose `status: in-progress` and
  whose `live.updated` is recent. Turns the dashboard into a live ops
  pane, not just a result viewer.
- **Per-project run history** _(done — 2026-05-26)_ — every report
  under a project, newest-first, with the user's responses inlined
  beneath each run. `/api/projects` grew a `history` field of
  `historyRun` records (entry summary + inlined `responses.json`).
  `viewProjects()` renders a "history" subsection per project: status
  dot + title + meta on the right (time / kind / harness / archived /
  open-asks pills) + response chips (`block → value · timestamp ·
  optional note`). The flat inbox stays the triage surface; the
  per-project timeline is the retrospective surface. Ride-along
  fix: extended the `minmax(0, 1fr)` grid-track defense to the
  desktop `.layout` (was only at the 720px breakpoint), so wide
  markdown / code can't push the content column past the viewport.
- **Notification fan-out** — destinations beyond Web Push: Slack /
  Discord webhooks, generic POST webhook, optional email. Plug into
  the existing watcher → push delivery path. Per-destination on/off in
  the settings view; payload shape mirrors the existing push payload so
  one delivery driver hands off to a list of senders.

Also on the table (lower priority — not picked yet but logged):

- **Report templates** — `harness-deck new --template audit | review |
  progress | decision | idea` scaffolds opinionated starters with the
  right block shapes pre-filled. Pairs with `docs/PUBLISHING.md`.
- **Search filters + saved searches** — Cmd+K filters by project /
  status / kind / block-type / time range; saved searches pin as
  tabs in the in-app tab strip.

## Later / out of scope

- Harness-side integration (hooks/skill so harnesses emit manifests and pick up
  responses) — lives in `chezmoi-config`, separate task.
- v1a/v1b/v1c visual refinements (v1 original only for now).
- Multi-user / auth / cloud sync — local single-user only.

## Constraints

- Frontend stays vanilla HTML/CSS/JS, reusing the design files verbatim
  (`tokyo-night.css`, `vim-nav.js`, v1 layout). No frontend build step.
- The renderer owns all report HTML/CSS — manifests never contain layout, only
  content (except the deliberate `html` escape-hatch block).
- The harness→deck contract is a written file. MCP, if added, is a convenience
  wrapper, never the only path.
