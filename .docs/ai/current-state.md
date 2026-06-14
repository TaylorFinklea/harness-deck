# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main; origin/main pushed through `89b3f5e`. Unpushed: `0ac9ea6`
  (config root live-reload) + this doc commit. Build + `go test -race ./...`
  green.
- **v0.2.6 released + approved** (2026-06-14 dashboard sign-off).
- **v0.2.6 era (2026-06-13):** audit backlog cleared, report templates,
  draft-gating.
- **2026-06-14:** usage monitors (CodexBar footer, opt-in `usage.providers`;
  decisions.md "Usage monitors"); perf wave complete (incremental scan ~6.9×,
  /api/projects history cap, mtime-cached doc render; "Perf wave"); **session-
  code audit + tiered clear-out** (5-lens audit → Opus/Sonnet/Haiku fixes;
  fixed a major draft↔awaiting-review re-notification bug + nits + test gaps;
  decisions.md "Session-code audit"); config live-reload of project roots
  (`register` now works without restart; 0ac9ea6).
- Backlog: now only deferred-with-rationale items (jsonfile order-preserving
  patch + a few micro-followups) — see roadmap Backlog.

## Plan

- [x] v0.2.6 six-check matrix — **approved 2026-06-14** (dashboard sign-off,
  report auto-archived).
- [?] awaiting human verify — 3 frontend audit fixes (commit 2b3c78a),
  roadmap Now item 5.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- Enable usage monitors: add tools to `usage.providers` in config (Claude needs
  a one-time Keychain allow; OpenCode needs a pasted cookie). docs/SETUP.md §8.
- Launch post (e) per docs/launch/community-posts.md.
