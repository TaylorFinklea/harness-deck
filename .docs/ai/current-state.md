# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main, pushed. **v0.2.8 released + installed + verified 2026-06-20** —
  ships Assessment Waves 1–9 (`8e9a0cc`…`9bb9e10`). GoReleaser CI green (4
  platforms); the new push/PR CI workflow (W1) also ran green on its first push.
  `brew upgrade` done, service restarted, `hdeck version` = 0.2.8. Verified live:
  scope-in-JQL schema, activity view, usage bar.
- **v0.2.7 released + installed 2026-06-16** — usage monitors, perf wave, config
  live-reload, JS modularization, search query language, saved searches. Usage
  bar live (codex ✅; claude-code wired; opencode awaits its `auth` cookie).
- **Assessment backlog COMPLETE 2026-06-16/19** — the 7-agent feature assessment
  shipped as Waves 1–9 (all unreleased): W1 response-loop (note field, SSE
  `response`, notify env, CI), W2 multi-select asks, W3 activity timeline, W4
  search-text cache, W5 cross-report `related[]`, W6 response history, W7 scope
  in JQL + TLS-expiry warning, W8 card-grid block, W9 inbox-sort/section-collapse/
  Cmd+S. decisions.md "Assessment Waves 1–3" + "Waves 4–9". Each Sonnet-impl +
  Haiku-gate + 2×Sonnet-review + Opus verify.

## Plan

- Empty. The assessment list is clear. Only deferred sliver: `tags` in JQL
  (roadmap Next). Else pull from Backlog / Later.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- Paste opencode.ai `auth` cookie into `usage.opencode_cookie` for the 3rd
  usage tile (codex + claude-code already work).
