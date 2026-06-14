# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main, **18 commits ahead of origin (unpushed — user pushes)**.
  Build + `go test -race ./...` green.
- **v0.2.6 released.** Since: audit backlog cleared, report templates,
  draft-gating (all 2026-06-13).
- **Usage monitors shipped 2026-06-14** — CodexBar-style footer for
  codex/openrouter/claude-code/copilot/opencode (opt-in `usage.providers`);
  3-lens reviewed; commits 3292e4a + b074c6b. decisions.md "Usage monitors";
  spec phases/usage-monitors-spec.md.
- **Perf wave complete 2026-06-14** — incremental scan (~6.9×, 67b247b) +
  /api/projects history cap (50, ?all=1) + mtime-cached doc render.
  decisions.md "Perf wave".

## Plan

- [?] awaiting human verify — v0.2.6 six-check matrix; approval in
  `.harness/20260610-v026-verify/`.
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
