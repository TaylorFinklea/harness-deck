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
- **Search query language** _(2026-06-15)_ — Cmd+K upgraded from plain text to a
  JQL-like filter language (`status = …`, `project IN (…)`, `created >= -7d`,
  AND/OR/NOT, mixed with free text) + as-you-type autocomplete. New stdlib-only
  `internal/query` (lexer→parser→AST→lazy short-circuit eval), `/api/search`
  refactor + `/api/search/schema`. Built via a phased multi-agent workflow
  (build→adversarial-verify→server→client→review). decisions.md "Search query
  language"; phases/search-query-language-{spec,report}.md.
- **v0.2.7 release** _(2026-06-16)_ — **first release since v0.2.6 (2026-06-10)**;
  everything between was built but unreleased until now. Ships: usage monitors,
  perf wave (~6.9× warm scan), config live-reload, JS modularization (hd-dom +
  tree/help/settings split), search query language, and saved searches.
  _Correction: earlier handoff notes implied v0.2.6 carried usage monitors — it
  did not. v0.2.6 is the 2026-06-10 launch tag; these features reached Homebrew
  users only in v0.2.7._
- **Post-v0.2.7 assessment backlog — Waves 1–3** _(2026-06-16, in v0.2.8)_ —
  shipped from the 7-agent feature assessment. **Wave 1** (`8e9a0cc`): note
  field end-to-end (was persisted+POSTed but invisible), SSE `response` event
  (live harnesses get pushed the answer vs polling), notify `HD_RESPONSE_VALUE/
  _JSON`, and a **CI workflow** (`go test -race` on push/PR — there was none).
  **Wave 2** (`86c1841`): multi-select ask `mode:"multi"` + `Values[]` (checkbox
  picker, digit-toggle keyboard); also fixed a triage/note selector clash Wave 1
  introduced. **Wave 3** (`a3459d0`): the **activity timeline** — a 3rd dashboard
  view (cross-project/harness, day-grouped, `g l`). Each Sonnet-impl + Haiku-gate
  + 2×Sonnet-review + Opus browser-verify. decisions.md "Assessment Waves 1–3".
- **v0.2.8 release** _(2026-06-20)_ — ships the entire assessment backlog (Waves
  1–9). GoReleaser CI green (4 platforms + tap formula); the new push/PR CI
  workflow (W1) ran green too; installed + verified (scope-in-JQL, activity view,
  usage bar all live on 0.2.8).
- **Post-v0.2.7 assessment backlog — Waves 4–9** _(2026-06-16/19, in v0.2.8)_ —
  the rest of the list. **W4** (`9bdb6af`): search-text cache (Entry.SearchText,
  json:"-") — kills the per-query manifest re-read. **W5** (`72f6bcb`):
  cross-report `related[]` + "related" panel (the deferred half of W3). **W6**
  (`0b631e0`): response history (Response.Prior, capped) + responses.json
  `version`. **W7** (`0753e86`): `scope` as a JQL field + TLS cert-expiry warning
  (startup + 24h). **W8** (`e8380ab`): typed `card-grid` block (promotes the
  html-escape lane-grid). **W9** (`9bb9e10`): inbox sort pivot + per-project
  section collapse + `Cmd+S` save-search. Same Sonnet-impl + Haiku-gate +
  2×Sonnet-review + Opus browser/functional-verify per wave. decisions.md
  "Assessment Waves 4–9".
- **v0.2.9 release** _(2026-06-21)_ — ships Wave 10 (tags in JQL); GoReleaser +
  CI green; installed + verified (`tags` live in the search schema). Completes
  the assessment backlog on Homebrew.
- **Wave 10 — `tags` in JQL** _(2026-06-20, `d3de13b`, in v0.2.9)_ — the final
  deferred item. `Report.Tags []string` + the query engine extended for
  multi-value fields (Record gains `Fields()`, a `listFields` set, existential
  eval: `tags = X`/`IN` = any matches, `tags != X`/`NOT IN`/`!~` = none matches);
  chip render on the report page; autocomplete enumerates distinct tags.
  Live-verified the existential pipeline (incl. negation) + chip render.
  decisions.md "Wave 10".
- **opencode usage tile → local `opencode stats`** _(2026-06-26/27, in v0.2.10)_
  — replaced the fragile pasted-cookie web-scrape with a local `opencode stats
  --days N` shell-out (`KindBudget`, "OC $X", 7d default). No cookie, network,
  or browser. `900f137` (feat) + `4cf216c` (docs). Trade-off: shows local spend,
  not the cookie-only Zen plan-%. decisions.md "opencode usage tile".
- **v0.2.11 release** _(2026-06-28)_ — ships the launchd-PATH fix (`e977d7e`:
  `opencodeBin()` falls back from $PATH to common install dirs) for the opencode
  tile, closing the v0.2.10 regression where the LaunchAgent's minimal PATH
  (no /opt/homebrew/bin) made `exec.LookPath` fail → "CLI not found". GoReleaser
  + CI green; installed + live-verified — `/api/usage` now returns the opencode
  budget tile on the real dashboard. decisions.md landmine note.
- **Beads Backlog viewer — Phase 1** _(2026-06-30, branch `feat/beads-backlog-viewer`, unreleased)_
  — read-only Backlog view over `bd` issues: `internal/beads` adapter (mirrors
  `usage`) + `Monitor` + `/api/beads` + `/api/beads/{project}/{id}` + a 4th
  dashboard view (`g b`) with a hand-rolled inline-SVG dependency graph +
  drill-in, live-refreshed over an SSE `beads` event. Opt-in (`beads.enabled`),
  discovers `.beads/` repos (independent of `.docs/ai`), stdlib-only. Bead
  `harness-deck-5ph.1`. Browser-verified across ~23 pilot repos (2 bugs caught +
  fixed live). decisions.md "Beads Backlog viewer"; spec/plan in
  `phases/beads-backlog-viewer-{spec,plan}.md`. Phase 2 (actions) = `5ph.2`.
- **Beads Backlog viewer — Phase 2 (actions)** _(2026-07-02, same branch, unreleased)_
  — claim/close/create from the UI behind a separate `beads.writable` flag
  (default false; read-only stays the default). Three `POST /api/beads/...`
  endpoints (argv equals-form, write mutex for Dolt, status re-check→409,
  immediate RefreshNow+SSE); detail-panel Claim/Close + `+ new` create form +
  keys `c`/`x`/`n`, all gated by `HD_BEADS_WRITABLE`. Bead `harness-deck-5ph.2`.
  Browser-verified create→claim→close + 403-when-read-only. decisions.md "Beads
  actions"; spec/plan `phases/beads-actions-{spec,plan}.md`.

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
3. - [x] **Launch sequence** _(done 2026-06-16)_ — (a) ✅ 3 README screenshots
     retaken vs v0.2.x against a curated fixture (roadmap.png → projects.png),
     (b) ✅ Status line rewritten (daily use, Homebrew, versioned contract —
     commit 74156bf), (c) ✅ Medium article published, (d) ✅
     docs/launch/community-posts.md links filled, (e) ✅ community post done by
     the user 2026-06-16.
4. - [x] **Arch hardening wave** _(done 2026-06-10)_ — all five bullets +
     GH Actions Node 24 bump; commits faec79b..5232af7. See
     `.docs/ai/phases/arch-hardening-wave-report.md`.
5. - [x] **Browser-verify the 3 frontend audit fixes** _(done 2026-06-15,
     chrome-devtools MCP)_: (a) cursor moved off top row (demo/r1→r2),
     hard-reload restored to demo/r2 (not snapped to top) ✓; (b) pinned 2
     reports, digit 2→first pin, 3→second pin, 1→dashboard ✓; (c) digit nav
     works on both the dashboard (switchToTabN) and report pages (tabs.js
     handler). Doubled as a regression check of the IIFE modularization —
     cursor-persist + digit-nav survive the new HDTree/HDHelp/HDDom boundaries;
     zero console errors.

The 2026-06-10 batch above is fully shipped, and so is the next item:

6. - [x] **Saved searches** _(done 2026-06-16)_ — pin a JQL query to a new SAVED
     sidebar section; click opens the Cmd+K palette pre-filled + live (Option A).
     localStorage (`window.HDSaved`, mirrors HDPins); click-only activation
     (no digit collision with pins). decisions.md "Saved searches"; spec
     phases/saved-searches-spec.md; mockup report `20260616-saved-searches-design`.
     Built Sonnet-impl + Haiku-gate + 2×Sonnet-review; browser-verify caught a
     CSS↔inline-style bug (☆ never showed) that static review missed.

**Batch 2026-07-12 — macOS onboarding fix** (spec/report:
`phases/macos-onboarding-{spec,report}.md`; code all on main, unreleased):

7. - [x] **hdeck cert + hdeck doctor + brew services + gated notarization**
     _(done 2026-07-12)_ — fresh-machine install surfaced two confirmed
     onboarding breaks on the README's headline phone/Tailscale path (ALF
     silent drop of ad-hoc binaries; MAS-sandboxed `tailscale cert` can't
     write files). Shipped: SETUP.md hotfix, `hdeck cert`, `hdeck doctor`,
     GoReleaser `brews[].service` + env-gated `notarize`, docs rewrite.
8. - [x] **Apple Developer enrollment + notarization secrets** _(done
     2026-07-12)_ — account was already enrolled. Developer ID Application
     cert issued via developer.apple.com (browser, Account-Holder-only
     operation — 2 ASC API keys got 403 trying it headlessly, confirming
     Apple restricts this regardless of API role). CSR/key/cert/p12 live in
     `~/.appstoreconnect/` (key + p12 never left this machine; user ran the
     p12-assembly script themselves per a deny-rule prompt on `Write(**/*.p12)`
     — see decisions.md). Chain attached + verified via `quill p12
     attach-chain`/`describe` (anchore/quill v0.7.1, checksum-verified
     release binary): leaf → Developer ID G2 → Apple Root CA. **Locally
     verified the p12 actually signs**: `quill sign` on a copy of the dev
     binary → `codesign -dv` shows a real chain + hardened runtime + genuine
     Apple timestamp; `spctl` correctly says "rejected, Unnotarized Developer
     ID" (expected pre-notarization state). All 5 `MACOS_*` secrets uploaded
     via `gh secret set` (piped from Keychain/openssl, never printed);
     confirmed present via `gh secret list`. **Not yet done: a real
     notarization submission** — GoReleaser's notarize pipe has no
     snapshot-skip (`Skip()` only checks an explicit `--skip=notarize` flag
     or empty config), so even a local test build with these secrets
     exported would hit Apple's live notary API; deliberately left for the
     real v0.2.14 release rather than spent on a speculative local test.
     ASC key `X3GUKCUJ9F` used for notary auth (confirmed live via the
     earlier read-probe; not yet proven to have notary-submit rights
     specifically — first release will confirm).
9. - [x] **Release v0.2.14 RELEASED + verified 2026-07-12** — tag pushed,
     GoReleaser green (`gh run view` log: `signing` → `notarizing and waiting`
     → `notarized` for both darwin binaries, ~17s each, first try, no
     fallback key needed — X3GUKCUJ9F has notary-submit rights after all).
     GitHub Release + tap formula (`service do` block, v0.2.14) both live.
     Installed on mandalore: old hand-pasted LaunchAgent booted out + its
     plist deleted (superseded by brew services — landmine cleared), `brew
     upgrade` → 0.2.14, `codesign -dv` on the real installed binary shows
     Developer ID Application → Developer ID Certification Authority → Apple
     Root CA with a genuine timestamp matching the CI run. **`brew services
     start harness-deck` reached the phone URL (10.15.109.208:7420 +
     `mandalore.tailceb58.ts.net`) with ZERO manual `socketfilterfw`
     commands** — the actual proof the signing fix works; removed the
     now-stale ALF entry for the deleted 0.2.13 path. `hdeck doctor` exit 0,
     10/10 ok. `spctl -t execute` shows "rejected (does not seem to be an
     app)" for the bare CLI binary — expected/harmless (Gatekeeper's
     app-launch check doesn't apply to non-bundle binaries; orthogonal to
     notarization, which the CI log already proved succeeded).

10. - [x] **v0.2.15 RELEASED + verified 2026-07-13** — a cold-read agent
     review (fresh subagent, no context, "install harness-deck on my machine")
     found v0.2.14's `doctor` graded a stopped server WARN while `finish()`
     exits 1 only on FAIL — so the exit-0 gate the setup docs tell agents to
     trust **certified installs where the service never started**. Fixed: down
     server = FAIL; leftover hand-rolled unit = FAIL (it fights brew services
     for :7420 at login and the survivor still answers, so every other check
     went green — this was live on mandalore); empty `scan_roots` = WARN.
     Added **AGENTS.md** (canonical harness-neutral entry point; only CLAUDE.md
     existed, so Codex/OpenCode/Cursor/Copilot found nothing, and README linked
     a file that would have 404'd) — CLAUDE.md now imports it. Install docs
     rewritten (stale-unit cleanup, upgrade + no-Homebrew branches, config
     documented as optional). First release attempt FAILED at GoReleaser's
     `go test` pre-hook: the stale-unit test asserted macOS `launchctl` text
     while the fn branched on `runtime.GOOS` — untestable across platforms;
     OS is now a parameter, both platforms covered, nothing was published by
     the failed run. Verified on the shipped binary: doctor 12/12 exit 0, exit
     1 on a never-started install, Developer ID chain intact.

## Next

**Feature assessment 2026-06-16** (7-agent sweep; verdict: core loop
feature-complete). The whole assessment backlog is **shipped as Waves 1–10**
(see "Post-MVP shipped" + decisions.md "Assessment Waves 1–3" / "Waves 4–9" /
"Wave 10"). Substantial picks → Waves 1–6; polish → 7–9; `tags`-in-JQL → 10.

**The list is fully clear — nothing deferred, everything released.** Waves 1–9
in v0.2.8, Wave 10 in v0.2.9. Pull from Backlog / Later when starting fresh.

Possible follow-ups (not yet scheduled):

- **Further aggregator.js splits** — settings chunk extracted (5445f85,
  6b45ad9) and `hd-dom.js` provides a shared `el()` (6d5f908), so chunks can be
  true separate-IIFE modules. Done so far:
  - `aggregator-help.js` (`window.HDHelp`, d9b2789) — `?` cheat sheet; appended
    *after* the core IIFE (referenced only from deferred callbacks).
  - `aggregator-tree.js` (`window.HDTree`, latest) — sidebar tree-focus mode
    (Space-e; j/k/Enter/p/Esc). **Prepended *before* the core IIFE** because
    `HDTree.paint()` runs during core init (render → refresh). Owns
    `treeFocused`/`treeActiveKey`; talks to core only via `HDPins` + the DOM.
    `pin()` reads the title from the row's `.label` (was `data.reports`) to stay
    data-free. The view builder `renderTree()` stays in core (coupled to
    `data`/`activeReports`/`isPinned`/`reportURL`).
  - Bundle order (assets.go): tree → core(+settings) IIFE → help.
  No obvious next chunk — remaining core (inbox/projects views, render loop,
  keydown router) is tightly data-coupled. Stop here unless a new seam appears.
- ~~**search.js → HDDom.el**~~ — DONE (next commit). Migrated search.js's
  ensureOverlay/renderList/snippet builder to `HDDom.el`; prepended
  `HDDomJSInline` to the `ReportJS` bundle (shell already loads it first) so
  report pages get `window.HDDom` before search runs. Bundle-order test +
  browser-verified search palette renders/highlights/navigates on **both** the
  dashboard and a report page.

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

- **opencode usage tile — needs a cloud-Zen source** _(deferred 2026-06-28)_ —
  the tile is feature-flagged **off** (`usage.opencode_enabled`, default false;
  ships in v0.2.12) because `opencode stats` sees only local TUI sessions and
  reads $0 for cloud (opencode-go/Zen) usage. To revive: find a headless way to
  read account-scoped Zen usage/balance — probe the opencode.ai endpoint with
  the stored `opencode-go` api key (auth.json), and if that needs the web
  session instead, fall back to CodexBar's cookie path. One authenticated probe
  answers the api-key question. Then flip the flag on. Code + investigation in
  `internal/usage/opencode.go` + decisions.md "opencode usage tile disabled".
- **ollama-cloud usage provider** _(requested 2026-07-12)_ — the user listed
  `ollama-cloud` in `usage.providers` on a fresh setup; no such provider
  exists (`hdeck doctor` now names it instead of silently ignoring it). Would
  read Ollama Cloud plan usage via `$OLLAMA_API_KEY`; check whether ollama.com
  exposes a usage/quota endpoint before scheduling.

- ~~**Search filters**~~ — **DONE 2026-06-15** as a JQL-like query language
  (`status = awaiting-review`, `project IN (a,b) AND kind = audit`,
  `created >= -7d`, mixed with free text) + as-you-type autocomplete. New
  `internal/query` package (parser+eval), `/api/search` refactor +
  `/api/search/schema`, palette autocomplete. Spec + report in
  `.docs/ai/phases/search-query-language-{spec,report}.md`; ADR in decisions.md.
- ~~**Saved searches**~~ — **DONE 2026-06-16** (Now item 6). Follow-ups, not
  scheduled: (a) keyboard activation + `Cmd+S`-to-save (v1 is click + ☆-button
  only); (b) inline (non-`prompt`) naming + rename; (c) saved-list UI on report
  pages (the palette save already works there); (d) server-side store for
  cross-device sync. **Discovered:** `aggregator-tree.js`'s Space-e focus ring
  targets the first `#tree .tree` — now the SAVED sub-tree when present (was the
  PINNED sub-tree) rather than the REPORTS tree. Pre-existing cosmetic quirk
  (j/k still walk `.row.run` only); fix = scope the querySelector to the reports
  tree. Low priority.
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
