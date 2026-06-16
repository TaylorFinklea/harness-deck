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
- Roadmap **Now**: 2026-06-10 batch fully shipped (launch post done by user
  2026-06-16). **Active next item: saved searches** (roadmap Now #6) — pin a
  JQL query to the sidebar; **needs a design brainstorm first** (dashboard-only
  vs palette-everywhere; HDPins-localStorage vs server store; activation UX).
  User will kick it off in a fresh session. Rest is Backlog (deferred) + Later.
- Backlog: now only deferred-with-rationale items (jsonfile order-preserving
  patch + a few micro-followups) — see roadmap Backlog.

## Plan

- Empty. Next session: saved searches (roadmap Now #6) — start with a design
  brainstorm, then implement → verify → commit.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- Enable usage monitors: add tools to `usage.providers` in config (Claude needs
  a one-time Keychain allow; OpenCode needs a pasted cookie). docs/SETUP.md §8.
- Push the 6 unpushed commits (search query-language milestone) after review.
