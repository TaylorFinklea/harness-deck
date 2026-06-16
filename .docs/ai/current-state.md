# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main; **origin/main caught up through `c30cc7e`** (the JS
  modularization arc — hd-dom/help/tree IIFE split, search.js→HDDom — + the
  harden sweep: gofmt, `node --check` bundle gate, dead-countBy removal — all
  pushed). Working tree clean. Build + `go test ./...` green (incl.
  TestBundlesAreValidJS, skips w/o node).
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
- Roadmap **Now**: only the human-gated launch post (3e) remains — all
  codeable items done (item 5 verified 2026-06-15). **Next** follow-ups all
  done (search.js→HDDom; aggregator help + tree splits; node-check gate). No
  obvious next aggregator chunk — rest of core is data-coupled. The genuine
  next *feature* is **search filters** (Later; M, additive, low-risk). Rest is
  Backlog (deferred) + Later.
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
