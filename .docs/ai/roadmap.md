# harness-deck — roadmap

## Vision

One place where any AI coding harness can publish a consistently-formatted report
— progress, an opinion request, a decision, an idea — and where the user reviews
an upcoming roadmap. Reports are authored as a JSON block manifest; a local Go
server renders, aggregates, and live-updates them, and routes the user's
responses back to the harness.

## Milestones

- **Phase 0 — repo skeleton.** _(done)_
- **Phase 1 — manifest + renderer.** _(done — Go structs are the schema)_
- **Phase 2 — discovery + aggregator.** _(done)_
- **Phase 3 — live updates.** _(done)_
- **Phase 4 — response round-trip.** _(done)_
- **Phase 5 — roadmap view.** _(done)_
- **Phase 6 — harness-side integration.** _(done — lives in `chezmoi-config`)_

## Post-MVP — shipped

One line each; detail in decisions.md + git log.

- Project discovery + tracking _(2026-05-22)_ — scan_roots, projects view, toggles
- Mobile PWA + Web Push, stdlib-only _(2026-05-23, v0.1.5)_
- Asks visibility, archive, code copy, command palette _(2026-05-24, v0.1.6)_
- Keyboard triage _(2026-05-24, v0.1.7)_ · Mobile polish _(v0.1.8/9)_
- Report-page live reload _(2026-05-25, v0.1.10)_ · Pull-to-refresh _(v0.1.11)_
- Richer Markdown: tables/quotes/links _(v0.1.12)_ · Roadmap polish _(v0.1.13)_
- `new` + `register` subcommands _(v0.1.14)_ · Cross-report search _(v0.1.15)_
- Theme switch + light mode _(v0.1.16)_ · Apple push 403 fix _(v0.1.19)_
- PNG PWA icons + docs/PUBLISHING.md _(2026-05-26)_
- "Extend usefulness" wave _(2026-05-26)_ — MCP report-builder server, live
  in-flight telemetry, per-project run history, notification fan-out
- **v0.2.0 cohesive redesign** _(2026-05-27)_ — 6 views → 2, keyboard chord
  system, inbox cursor; post-release keyboard polish; pins replace auto-tabs
- lark-plug-hdeck shipped read-only _(2026-05-27, separate repo)_
- `open` dedicated-window launcher + `config.BaseURL()` _(2026-06-05, v0.2.1–3)_
- html block isolated in shadow root _(2026-06-05)_
- docs/SETUP.md agent runbook _(2026-06-08)_ · launch drafts under docs/launch/
- **Self-describing binary** _(2026-06-09, unreleased)_ — embed CONTRACT.md +
  PUBLISHING.md; `contract` subcommand, MCP resources + initialize
  instructions, `GET /contract.md`
- **Report templates** _(2026-06-13)_ — `new --template
  audit|review|progress|decision|idea` pre-fills the block shapes each kind
  needs; title + kind default from the template. decisions.md "Report
  templates".
- **Draft reports don't surface as open asks** _(2026-06-13)_ — a `draft`
  report's interactive blocks no longer count in OpenAsks or fire push (they
  still render + stay answerable); aligns with the documented status lifecycle.
  decisions.md "Draft reports don't surface as open asks".
- **Config live-reload of project roots** _(2026-06-14, 0ac9ea6)_ — the watcher
  stats config.json each tick and reloads scan_roots/projects via
  Manager.SetRoots, so `register` is picked up without a restart (was a Next
  item; the --help claim is now true). Roots only; other config still restart.
- **aggregator.js split + pin/tabs decoupling** _(2026-06-14, 5445f85 + 6b45ad9)_
  — pin changes fire a `hd:pins-changed` CustomEvent (dashboard + report page
  both listen; was a single-slot callback); dropped the HDTabs shim
  (search.js → HDPins.open); extracted settings/push/destinations into
  aggregator-settings.js, assembled with aggregator.js in one IIFE Go-side.
  Browser-verified. decisions.md not needed (mechanical); seam pattern noted in
  the commit.
- **hd-dom.js shared DOM helpers** _(2026-06-14, 6d5f908)_ — extracted
  el()/htmlToNodes into a `window.HDDom` namespace (loaded first); aggregator.js
  + usage.js bind it instead of redefining/raw-createElement. Unblocks true
  module splits (separate IIFEs share one el()). Browser-verified. search.js
  adoption is a follow-up (needs HDDom in the report-page bundle).
- **Perf wave** _(2026-06-14, complete)_ — (1) incremental mtime-keyed scan in
  store.Scan, ~6.9× faster warm ticks (67b247b); (2) /api/projects history
  capped to 50 newest (+`?all=1`, `history_total` surfaced), responses loaded
  only for kept runs; (3) project roadmap/current-state markdown render cached
  by mtime. decisions.md "Perf wave".
- **Usage monitors** _(2026-06-14)_ — CodexBar-style footer for
  codex/openrouter/claude-code/copilot/opencode, opt-in via `usage.providers`;
  3-lens reviewed. decisions.md "Usage monitors"; spec
  phases/usage-monitors-spec.md; config docs in docs/SETUP.md §8.

## Now (sequenced 2026-06-10 — product review w/ audit)

Full audit: `.harness/20260610-bug-bash-audit/report.json` (30 verified
findings). Sequencing decision: release → fix highs → launch.

1. - [x] **Release v0.2.4** _(done 2026-06-10)_ — tag pushed, GoReleaser run
     succeeded (4 platform binaries), tap formula committed, verified via
     local `brew upgrade` → `hdeck version` 0.2.4 + `hdeck contract` prints
     the embedded schema.
2. - [x] **Bug-fix milestone: critical + 8 highs** _(done 2026-06-10)_ —
     all 9 items shipped TDD-first; frontend items browser-verified. See
     `.docs/ai/phases/bug-fix-wave-report.md`. v0.2.5 released (first run
     401'd — transient; rerun succeeded, tap formula at 0.2.5).
3. - [ ] **Launch sequence** — (a) ✅ 3 README screenshots retaken vs v0.2.x
     against a curated fixture (roadmap.png → projects.png), (b) ✅ Status
     line rewritten (daily use, Homebrew, versioned contract — commit
     74156bf), (c) ✅ Medium article published, (d) ✅
     docs/launch/community-posts.md links filled, (e) post per its checklist.
4. - [x] **Arch hardening wave** _(done 2026-06-10)_ — all five bullets +
     GH Actions Node 24 bump; commits faec79b..5232af7. See
     `.docs/ai/phases/arch-hardening-wave-report.md`.
5. - [ ] **Browser-verify the 3 frontend audit fixes** (commit 2b3c78a, no JS
     test harness): (a) move inbox cursor off the top row, hard-reload →
     cursor returns to same row; (b) pin 2+ reports, press digits 2-9 → jumps
     to each pinned report, 1 → dashboard; (c) digit nav still works after the
     dead-handler removal. Go-side bundle test already green.

## Next

_(empty — Now/Next cleared. Pull from Backlog or Later when starting fresh.)_

Possible follow-ups (not yet scheduled):

- **Further aggregator.js splits** — settings chunk extracted (5445f85,
  6b45ad9) and `hd-dom.js` now provides a shared `el()` (6d5f908), so further
  chunks can be true separate-IIFE modules binding `HDDom.el` (no more
  fragment-in-one-IIFE trick). Next candidate: the help-overlay or tree chunk.
- **search.js → HDDom.el** — migrate search.js (+ usage of HDDom on report
  pages: add HDDomJS to the ReportJS bundle) so its raw createElement drops the
  duplication; deferred from the hd-dom.js change to avoid report-page surface.

## Backlog

All 20 audit items (2026-06-10 bug bash, mediums + lows) **cleared 2026-06-13**
— 9 file-disjoint clusters, each TDD-verified, full `go test -race ./...`
green; commits 2293e2b..67d5ae2. Rationale + the agents' non-obvious choices:
decisions.md "Audit-backlog clear-out". Original traces:
`.harness/20260610-bug-bash-audit/report.json`. (One pending: browser-verify
the 3 frontend fixes — Now item 5.)

**Session-code audit + tiered clear-out 2026-06-14** (5-lens Sonnet audit of
the perf wave / draft-gating / usage internals; fixes routed Opus/Sonnet/Haiku;
commits d2b5a95, 6167957, e850943, 40ed9b3, fb0d173, e5d6ef0). decisions.md
"Session-code audit". Headline: a major draft↔awaiting-review re-notification
bug (ask-retention defeated draft-gating) — fixed. Plus renderDoc TOCTOU,
docCache eviction, push Send body cap, dead ?all=1 hint, footer overflow, and
the store-cache / projects / usage test gaps.

- [x] **Other hardcoded `harness-deck/report@1` literals** — audit found NO
  remaining Go sites (only the canonical `manifest.Schema` const + a doc
  comment); non-Go fixtures/docs stay literal. No-op.
- [ ] **Order-preserving JSON patch in `internal/jsonfile`** — DEFERRED (Lead):
  an in-house ordered JSON pretty-printer in the central atomic-write helper
  (blast radius: every report/response/config write) for a cosmetic benefit,
  on an item gated "only if it ever matters" (hasn't). The trade-off stays
  documented. `tier_floor`: senior. `complexity`: M. (Override to do it anyway.)
- Deferred micro-followups (Lead): make `askRetainTicks` configurable (YAGNI
  surface); a store.Scan stale-walk test hook (prod test-scaffolding for a
  marginal deterministic test); trim the redundant dashboard digit handler in
  tabs.js (harmless redundancy, browser-verify risk, zero functional gain).

## Later / parking lot

- **Search filters + saved searches** — Cmd+K filters by project / status /
  kind / time; saved searches pin to the sidebar PINNED section.
- **lark-plug-hdeck response writing** — re-deferred 2026-06-10. Trigger:
  *catching myself opening the browser just to tap yes/no on an ask.*
  Until then read-only stands.
- **herdr ↔ harness-deck active integration** — unchanged gate: requires a
  herdr extension; wait until used in anger.
- **responses.json evolution** — version field + `Values []string` for
  multi-select answers; pairs with any richer-answer-types work.
- **Search text-cache + archived exclusion from /api/reports** — only above
  ~1-2k active reports; measure first.
- **Micro-followups** — dispositioned 2026-06-14: push.Sender.Send size guard
  (done, 40ed9b3) and the Project.Name doc (done, e5d6ef0); the remaining three
  (askRetainTicks config, store stale-walk test hook, tabs.js digit-handler
  trim) deferred — see Backlog.

## Out of scope

- v1a/v1b/v1c visual refinements (v1 original only for now).
- Multi-user / auth / cloud sync — local single-user only.

## Constraints

- Frontend stays vanilla HTML/CSS/JS, reusing the design files verbatim
  (`tokyo-night.css`, `vim-nav.js`, v1 layout). No frontend build step.
- The renderer owns all report HTML/CSS — manifests never contain layout, only
  content (except the deliberate `html` escape-hatch block).
- The harness→deck contract is a written file. MCP is a convenience wrapper,
  never the only path.
- Zero external Go dependencies — stdlib only, no `require` block.
