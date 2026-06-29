# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- **herdr mobile inbox COMPLETE 2026-06-29** — all 11 tasks committed on main.
  `internal/herdr` adapter (List/Read/Send/resolveBin), `config.Agents` opt-in
  block, agent-status watcher tick (push + SSE on newly-blocked), `GET /api/agents`,
  `POST /api/agents/{key}/answer` (re-check before send), needs-you view + answer
  UX (`/agents`), docs (SETUP.md §9, decisions.md ADR, roadmap Later marked done).
  Build/test green. Not yet released.
- Branch: main. **opencode usage tile DROPPED behind a feature flag** — `opencode
  stats` sees only local TUI sessions ($0 for cloud/Zen spend). Footer = codex +
  claude-code only; `usage.opencode_enabled` flag (default false). Build/test green.
- **v0.2.11 released + installed + live-verified 2026-06-28** — launchd-PATH fix
  (`e977d7e`). opencode tile's $0 is a separate cloud-blindness issue, not this fix.

## Plan

- Empty. herdr mobile inbox 100% done (Tasks 1–11). Pull from Backlog / Later.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- Cut **v0.2.12** to ship: opencode-tile feature-flag drop + herdr mobile inbox.
  After release: upgrade + restart, confirm `/api/usage` shows only codex +
  claude-code, and `/api/agents` is live.
