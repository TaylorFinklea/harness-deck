# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- **Branch: `feat/usage-weekly-bars` (off main). Usage footer per-window bars
  COMPLETE + browser-verified 2026-07-02.** Weekly (`seven_day`) limit surfaced as
  structured `Sample.Windows` (5h + wk) for codex + claude-code; footer renders a
  mini progress bar per window (severity-colored), like Claude Code's /usage.
  `usage.js` + `v1.css`; TDD (window assertions in codex/claude tests). build/test
  green, gofmt clean. decisions.md "Usage footer: per-window bars". **Not merged.**
  (Separate from the beads work on `feat/beads-backlog-viewer`.)
- _(below is main's baseline)_ Branch: main. **opencode usage tile DROPPED behind a feature flag** (uncommitted
  → needs v0.2.12). Live verify of v0.2.11 showed the tile reads $0.00 because
  `opencode stats` only sees local TUI sessions (4, newest Feb 6); real spend is
  the opencode-go/Zen **cloud** plan (orchestra/pi), invisible to local CLI +
  account-scoped on opencode.ai. Decision: footer = codex + claude-code only;
  opencode gated by `usage.opencode_enabled` (default false), code kept for a
  future cloud-Zen source. New `Options.OpenCodeEnabled`; `Build` opencode case
  gated; `TestOpenCodeFeatureFlagged` pins it. decisions.md "opencode usage tile
  disabled"; roadmap Later "opencode usage tile — needs a cloud-Zen source".
  Build/test/vet green. **NOT yet committed/released.**
- **v0.2.11 released + installed + live-verified 2026-06-28** — launchd-PATH fix
  (`e977d7e`: `opencodeBin()` probes install dirs) for the opencode tile, closing
  the v0.2.10 "CLI not found" regression. The PATH fix is correct; the tile's $0
  is the separate cloud-blindness issue above, not this fix.
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

- Cut **v0.2.12** to ship the opencode-tile feature-flag drop (commit pending).
  After release: upgrade + restart, confirm `/api/usage` shows only codex +
  claude-code (no opencode).
