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

## Later / out of scope

- Harness-side integration (hooks/skill so harnesses emit manifests and pick up
  responses) — lives in `chezmoi-config`, separate task.
- Optional MCP report-builder server.
- v1a/v1b/v1c visual refinements (v1 original only for now).
- Multi-user / auth / cloud sync — local single-user only.

## Constraints

- Frontend stays vanilla HTML/CSS/JS, reusing the design files verbatim
  (`tokyo-night.css`, `vim-nav.js`, v1 layout). No frontend build step.
- The renderer owns all report HTML/CSS — manifests never contain layout, only
  content (except the deliberate `html` escape-hatch block).
- The harness→deck contract is a written file. MCP, if added, is a convenience
  wrapper, never the only path.
