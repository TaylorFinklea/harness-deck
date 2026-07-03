# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- **v0.2.13 (releasing now, on `main`): beads Backlog viewer (Phase 1+2) + usage
  per-window bars — both MERGED + browser-verified.** Beads `5ph.1`/`5ph.2`/epic
  `5ph` closed. `internal/beads` adapter+Monitor, `/api/beads` (+ `{id}` drill-in +
  claim/close/create POSTs), 4th view (`g b`) with inline-SVG dep graph + drill-in +
  actions, live SSE; two flags `beads.enabled`/`beads.writable` (both default off).
  Usage footer: `Sample.Windows` (5h+wk) → mini bar per window for codex/claude-code.
  TDD + adversarial review (beads: 4 review findings fixed); build/test/gofmt green,
  `go.mod` unchanged. decisions.md "Beads Backlog viewer" + "Beads actions" +
  "Usage footer: per-window bars".
- **opencode usage tile DROPPED behind a feature flag** (shipped v0.2.12).
  Live verify of v0.2.11 showed the tile reads $0.00 because
  `opencode stats` only sees local TUI sessions (4, newest Feb 6); real spend is
  the opencode-go/Zen **cloud** plan (orchestra/pi), invisible to local CLI +
  account-scoped on opencode.ai. Decision: footer = codex + claude-code only;
  opencode gated by `usage.opencode_enabled` (default false), code kept for a
  future cloud-Zen source. New `Options.OpenCodeEnabled`; `Build` opencode case
  gated; `TestOpenCodeFeatureFlagged` pins it. decisions.md "opencode usage tile
  disabled"; roadmap Later "opencode usage tile — needs a cloud-Zen source".
  Build/test/vet green. Shipped in v0.2.12.
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

- **v0.2.13 RELEASED + installed + deployed 2026-07-02** — beads viewer (P1+P2) +
  usage per-window bars. GoReleaser green, `brew upgrade` 0.2.12→0.2.13, LaunchAgent
  restarted. Live-verified on scadrial:7420: usage codex 5h+wk bars render; beads
  Backlog view enabled+writable, 33 `.beads/` repos discovered (0 errored). Config
  gained `beads:{enabled:true,writable:true}` (backup at config.json.pre-beads.bak).
  Next: `bd ready`.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- **Hard-reload the dashboard PWA** (Cmd-Shift-R / reopen) — the service worker can
  serve the old cached shell, hiding the new footer bars + Backlog tab.
- **claude-code (CC) usage bars** need a one-time macOS Keychain "Always Allow" for
  the new 0.2.13 binary (per-binary grant); codex is unaffected. Trigger it with an
  interactive `hdeck` usage read if you want CC bars in the background service.
