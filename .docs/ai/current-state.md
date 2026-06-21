# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main. **v0.2.8 released + installed + verified 2026-06-20** — ships
  Assessment Waves 1–9 (`8e9a0cc`…`9bb9e10`); GoReleaser + the new push/PR CI both
  green; `hdeck version` = 0.2.8, live features verified (scope-in-JQL, activity
  view, usage bar).
- **v0.2.9 released + installed + verified 2026-06-21** — ships **Wave 10
  (`tags` in JQL)** (`d3de13b`): `Report.Tags []string` + multi-value query
  support (existential `tags =`/`!=`/`IN`/`NOT IN`/`~`/`!~`) + chip render.
  GoReleaser + CI green; `hdeck version` = 0.2.9, `tags` in the live schema.
  **The assessment backlog is now 100% complete (Waves 1–10), all released.**
- **v0.2.7 released + installed 2026-06-16** — usage monitors, perf wave, config
  live-reload, JS modularization, search query language, saved searches. Usage
  bar live (codex ✅; claude-code wired; opencode awaits its `auth` cookie).
- **Assessment backlog COMPLETE 2026-06-16/19** — the 7-agent feature assessment
  shipped as Waves 1–9 (all unreleased): W1 response-loop (note field, SSE
  `response`, notify env, CI), W2 multi-select asks, W3 activity timeline, W4
  search-text cache, W5 cross-report `related[]`, W6 response history, W7 scope
  in JQL + TLS-expiry warning, W8 card-grid block, W9 inbox-sort/section-collapse/
  Cmd+S; **W10 tags in JQL**. decisions.md "Assessment Waves 1–3" + "Waves 4–9"
  + "Wave 10". Each Sonnet-impl + Haiku-gate + 2×Sonnet-review + Opus verify.

## Plan

- Empty. **Assessment backlog 100% done (Waves 1–10), nothing deferred.** Pull
  from Backlog / Later when starting fresh.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- Paste opencode.ai `auth` cookie into `usage.opencode_cookie` for the 3rd
  usage tile (codex + claude-code already work).
