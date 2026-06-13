# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main, **12 commits ahead of origin (unpushed — user pushes)**.
  Build + `go test -race ./...` green.
- **v0.2.6 released.** Audit backlog **fully cleared 2026-06-13** (9 clusters,
  commits 2293e2b..67d5ae2).
- **Report templates shipped 2026-06-13** — `new --template
  audit|review|progress|decision|idea`; reviewed via 3-lens workflow.
  decisions.md "Report templates"; phases/report-templates-report.md.

## Plan

- [?] awaiting human verify — v0.2.6 six-check matrix; approval in
  `.harness/20260610-v026-verify/`.
- [?] awaiting human verify — 3 frontend audit fixes (commit 2b3c78a),
  roadmap Now item 5.

## Blockers

- None.

## Open questions

- Draft-gating (roadmap Next): should `draft` reports stop counting as open
  asks / firing push? Surfaced by the templates review; awaiting user decision.

## Out (human-gated)

- Launch post (e) per docs/launch/community-posts.md.
