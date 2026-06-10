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
     `.docs/ai/phases/bug-fix-wave-report.md`. **v0.2.5 tagged locally —
     user to push** (`git push origin main v0.2.5`).
3. - [ ] **Launch sequence** — (a) retake the 3 README screenshots against
     the v0.2.x UI (rename roadmap.png → projects.png; consider an html-block
     mockup shot), (b) reconsider README "Status: Early build" line,
     (c) publish the Medium article, (d) fill `<MEDIUM_URL>`/`<GITHUB_URL>`
     placeholders in docs/launch/community-posts.md, (e) post per its
     checklist. Order matters: after v0.2.5 so readers don't hit known highs.
4. - [ ] **Arch hardening wave** (adopted 2026-06-10):
     - [ ] watcher `tick()` extraction + signature-gated digests + delta/CRUD
       tests (0% coverage on the config-rewriting handlers today)
     - [ ] registry cross-check test (template + defaultTitle + CONTRACT.md
       presence per block type) + absorb `blockPrompt`/`blockText` into
       `manifest` (hidden places 5–6 of the four-places checklist)
     - [ ] `assets_test.go` bundle invariants (MobileCSS-last, `</script`
       guard applied to the assembled bundle — closes the RespondJS gap)
     - [ ] schema versioning: parse `report@N` family+version, accepted set,
       CONTRACT.md Versioning section, forward-compat test
     - [ ] scan-timing log line (duration + entry count, warn >500ms)

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

Self-contained items from the 2026-06-10 audit (mediums, then lows). Full
traces in `.harness/20260610-bug-bash-audit/report.json`. Default Verify:
`go build ./... && go test ./...` unless noted.

- [ ] **Validate top-level strictness + publish_report byte fidelity** —
  strict-decode the top-level doc in Validate (shadow struct); make MCP
  publish_report write the validated `args.Manifest` bytes (json.Indent)
  instead of re-marshaling the struct. Files: internal/manifest/validate.go,
  internal/mcp/tools.go:212. Sonnet.
- [ ] **Ask-delta re-fire on transient disappearance** — merge cur into prev
  (retain vanished keys a few ticks) or keep a notified-set keyed by Tag.
  Files: internal/server/sse.go:100, push.go:176-187. Sonnet — coordinate
  with the watcher tick() refactor.
- [ ] **Markdown italic pass corrupts hrefs** — placeholder-token the
  link/code spans before bold/italic; require non-space inside `*…*`.
  Files: internal/render/markdown.go:49-57. Sonnet.
- [ ] **Inbox cursor not restored on initial load** — call restoreFocus()
  before the first ensureFocused() at bootstrap. Files:
  internal/assets/aggregator.js:1059-1111. Verify: browser. Sonnet.
- [ ] **MCP: oversized line kills session** — detect bufio.ErrTooLong, drain
  the line, emit one -32700, continue. Files: internal/mcp/protocol.go:201,
  228-230. Sonnet.
- [ ] **Push payload >4KB silently rejected** — truncate Body to a preview
  length before encrypt (also caps Slack/Discord bodies). Files:
  internal/push/encrypt.go:72-99, internal/server/push.go:200. Sonnet.
- [ ] **410-prune can delete a fresh re-subscription** — RemoveIfMatches
  (endpoint + keys) instead of Remove(endpoint). Files:
  internal/server/push.go:246-265, internal/push/store.go. Sonnet.
- [ ] **publicReportURL diverges from BaseURL** — delete the hand-rolled
  builder; call s.cfg.BaseURL() + path. Files: internal/server/push.go:231.
  Haiku candidate.
- [ ] **Project discovery dedups by basename** — key on absolute path,
  disambiguate display names, surface dropped duplicates. Files:
  internal/projects/projects.go:234-258. Sonnet — touches persistence keys.
- [ ] **(project,run) collisions silently shadow** — when seen[key] differs
  in Path, append to errs. Files: internal/store/store.go:96-100. Haiku
  candidate.
- [ ] **store.Scan last-writer-wins** — scanMu across walk+commit, or
  generation counter. Files: internal/store/store.go:72-116. Sonnet.
- [ ] **notify.Run no timeout blocks respond handler** — CommandContext w/
  10s timeout (mirror handleNotificationsTest) or run after broadcast.
  Files: internal/notify/notify.go, internal/server/server.go:277. Haiku.
- [ ] **switchToTabN reads deleted legacy key** — point at HDPins.load() or
  delete the branch. Files: internal/assets/aggregator.js:1370-1384.
  Verify: browser digits 2-9. Haiku.
- [ ] **JSON-RPC: don't reply to malformed notifications** — gate writeError
  on IsNotification. Files: internal/mcp/protocol.go:220-223. Haiku.
- [ ] **update tools alphabetize report.json keys** — decide: struct
  re-marshal for canonical order vs document the trade-off. Files:
  internal/mcp/tools.go:364-373,410-436. Sonnet.
- [ ] **addPaddingTrim doc lies** — make it convert standard→urlsafe (doc
  says it does) or fix the doc. Files: internal/push/encrypt.go:123-132.
  Haiku.
- [ ] **currentAskDigests missing promised log line** — add log.Printf in
  the err branch. Files: internal/server/push.go:120-123. Haiku.
- [ ] **HARNESS_DECK_CONFIG not tilde-expanded** — run through
  config.Expand in config.Path() and projects.StatePath(). Files:
  internal/config/config.go:77, internal/projects/projects.go:42. Haiku.
- [ ] **Dead legacy view-switch handler** — delete aggregator.js:1246-1253.
  Verify: browser digit nav. Haiku.
- [ ] **new.go hardcodes the schema literal** — reference manifest.Schema;
  update new_test.go:36. Haiku.

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
