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

- **Report templates** (`new --template audit|review|progress|decision|idea`)
  — first post-launch feature (decided 2026-06-10); pre-fill the block shapes
  PUBLISHING.md recommends; mention in its 60-second smoke test.
- **Perf wave** (measure first via the scan-timing log): mtime-keyed
  incremental scan in store.Scan (measured ~6× tick reduction); cap
  /api/projects history (~50 runs + `?all=1`); cache rendered roadmap/
  current-state HTML by mtime.
- **aggregator.js split** along comment seams via Go-side concatenation
  (settings/push/destinations chunk first); `CustomEvent('hd:pins-changed')`
  replaces the single-slot HDPinsChanged; drop the HDTabs shim.
- **register/config reload asymmetry** — `register --help` claims the watcher
  picks it up; it doesn't. Either fix the text or stat config.json in tick()
  and reload roots.

## Backlog

All 20 audit items (2026-06-10 bug bash, mediums + lows) **cleared 2026-06-13**
— 9 file-disjoint clusters, each TDD-verified, full `go test -race ./...`
green; commits 2293e2b..67d5ae2. Rationale + the agents' non-obvious choices:
decisions.md "Audit-backlog clear-out". Original traces:
`.harness/20260610-bug-bash-audit/report.json`. (One pending: browser-verify
the 3 frontend fixes — Now item 5.)

New low-priority items surfaced during the clear-out:

- [ ] **Other hardcoded `harness-deck/report@1` literals** — grep fixtures /
  docs / MCP for the schema string; re-point Go code at `manifest.Schema`
  where applicable (non-Go references stay literal). `tier_floor`: junior.
  `complexity`: S.
- [ ] **Order-preserving JSON patch in `internal/jsonfile`** — only if
  authored top-level key order in report.json ever matters. Today
  update_status/update_live alphabetize via the `map[string]any` round-trip in
  jsonfile.Patch (documented trade-off, not a bug). A token-stream rewrite
  that replaces only the targeted scalar is the real fix. `tier_floor`:
  senior. `complexity`: M.

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
- **hd-dom.js** — shared el()/htmlToNodes so the no-innerHTML discipline is
  a helper, not a convention.
- **Micro-followups from the audit clear-out** (all tiny, do opportunistically):
  defense-in-depth size guard in `push.Sender.Send`; make `askRetainTicks`
  (sse.go) configurable / wall-clock-based; a test-only injection hook to make
  store.Scan's stale-walk clobber deterministically testable; refine the
  `Project.Name` "directory basename" doc comment; trim the now-redundant
  dashboard digit handler in tabs.js (still needed on report pages).

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
