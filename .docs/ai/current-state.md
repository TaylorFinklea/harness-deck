# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

> **Backlog/ready-queue → beads (`bd ready`) as of 2026-06-30 (pilot).** New actionable work is filed in beads, not roadmap Now or this file. `.beads/` is git-excluded (stealth, local-only); decisions/loop-state stay prose. See CLAUDE.md → "Task tracking — beads pilot".

- **herdr mobile inbox COMPLETE + verified 2026-06-29** — on branch
  **`feat/herdr-mobile-inbox`** (NOT yet on main; user reviews + merges). 11
  TDD tasks (per-task Opus adversarial review) + a hardening pass + a security
  fix + live browser-verify. `internal/herdr` adapter (List/Read/Send/resolveBin),
  `config.Agents` opt-in block, agent-status watcher tick (push + SSE on
  newly-blocked, focused-suppression, retain debounce), `GET /api/agents`,
  `POST /api/agents/{key}/answer` (status re-check before send), needs-you view +
  answer UX (`/agents`, prefill-not-submit), docs (SETUP.md §9, decisions.md ADR,
  roadmap Later done). **Security:** argv flag-smuggling guard — herdr's parser
  is non-POSIX (no `--`), so `-`-leading target/text is rejected at the boundary
  (decisions.md landmine). Browser-verified end-to-end via a fake-herdr instance:
  card renders, prefill fills-not-submits, Send delivers, `-`-answer→400, 0
  console errors. `go build`/`go test ./...` green; `go.mod` unchanged. Not released.
- **v0.2.12 released + verified 2026-06-29** — opencode usage tile DROPPED
  behind `usage.opencode_enabled` (default false; `opencode stats` sees only
  local TUI sessions, $0 for cloud/Zen spend). Live `/api/usage` confirmed
  footer = codex + claude-code only.
- **v0.2.11 released + installed + live-verified 2026-06-28** — launchd-PATH fix
  (`e977d7e`). opencode tile's $0 is a separate cloud-blindness issue, not this fix.

## Plan

- Empty. herdr mobile inbox 100% done (Tasks 1–11). Pull from Backlog / Later.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- **Review + merge `feat/herdr-mobile-inbox`** (14 commits; build/test green,
  browser-verified). Then release (v0.2.13) and, to use it: set
  `agents.enabled: true` in config, ensure push is configured (`hdeck vapid` +
  TLS for iOS), restart. True end-to-end (real blocked agent → phone push →
  answer → resume) still needs one human check against a live herdr block —
  the build was verified via a fake-herdr instance, which can't exercise push
  to a real device.
