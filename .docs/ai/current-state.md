# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main; origin/main was caught up through `c30cc7e`. **Unpushed: the
  search query-language milestone** (`fa88dce` internal/query pkg, `bcf4e97`
  server, `6c25e3c` client, + docs) — JQL-like Cmd+K filters + autocomplete.
  Build + `go test ./...` + gofmt + vet + node-check all green; browser-verified
  on dashboard + report page. Tree clean, push-ready.
- **v0.2.6 released + approved** (2026-06-14 dashboard sign-off).
- **v0.2.6 era (2026-06-13):** audit backlog cleared, report templates,
  draft-gating.
- **2026-06-14:** usage monitors (CodexBar footer, opt-in `usage.providers`;
  decisions.md "Usage monitors"); perf wave complete (incremental scan ~6.9×,
  /api/projects history cap, mtime-cached doc render; "Perf wave"); **session-
  code audit + tiered clear-out** (5-lens audit → Opus/Sonnet/Haiku fixes;
  fixed a major draft↔awaiting-review re-notification bug + nits + test gaps;
  decisions.md "Session-code audit"); config live-reload of project roots
  (`register` works without restart; 0ac9ea6); aggregator.js pin/tabs decouple
  (CustomEvent, drop HDTabs) + settings-chunk split (5445f85, 6b45ad9);
  hd-dom.js shared el()/htmlToNodes namespace (6d5f908).
- Roadmap **Now**: only the human-gated launch post (3e) remains. **Search
  filters shipped** as the JQL-like query language (this session). Next feature
  candidate: **saved searches** (Later; pin a query to the sidebar — needs a
  design call, now cheap on the query language). Rest is Backlog (deferred) +
  Later.
- Backlog: now only deferred-with-rationale items (jsonfile order-preserving
  patch + a few micro-followups) — see roadmap Backlog.

## Plan

- [x] v0.2.6 six-check matrix — **approved 2026-06-14** (dashboard sign-off,
  report auto-archived).
- [x] 3 frontend audit fixes (commit 2b3c78a), roadmap Now item 5 — browser-
  verified 2026-06-15 (cursor-persist + digit-nav 2/3→pins, 1→dashboard); also
  cleared the modularization regression question. Plan empty.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- Enable usage monitors: add tools to `usage.providers` in config (Claude needs
  a one-time Keychain allow; OpenCode needs a pasted cookie). docs/SETUP.md §8.
- Launch post (e) per docs/launch/community-posts.md.
