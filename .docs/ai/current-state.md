# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main. origin/main caught up through `3963bea` (v0.2.7 release docs).
  **Unpushed: Assessment Waves 1–3** + their doc commits — `8e9a0cc` (Wave 1),
  `86c1841` (Wave 2), `a3459d0` (Wave 3), + this handoff commit. All gates green;
  each browser-verified. Push-ready; user reviews + pushes, then a v0.2.8 release
  would put them on Homebrew.
- **v0.2.7 released + installed 2026-06-16** — first release since v0.2.6
  (2026-06-10); shipped usage monitors, perf wave, config live-reload, JS
  modularization, search query language, saved searches. Usage bar live (codex
  ✅; claude-code wired, transient 429; opencode awaits its `auth` cookie).
- **Assessment Waves 1–3 shipped 2026-06-16** (unreleased) — from the 7-agent
  feature assessment (verdict: core loop feature-complete). W1: note field
  end-to-end + SSE `response` event + notify `HD_RESPONSE_*` + first CI workflow.
  W2: multi-select asks (`mode:"multi"`, `Values[]`) + fixed a triage/note
  selector clash. W3: activity timeline (3rd view, `g l`, cross-project,
  day-grouped). decisions.md "Assessment Waves 1–3". Each Sonnet-impl +
  Haiku-gate + 2×Sonnet-review + Opus browser-verify.

## Plan

- Empty. Next: roadmap Next — remaining assessment items (text-search cache,
  cross-report `related[]`, response history) or the polish list.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- Push the unpushed commits (Waves 1–3); optionally cut **v0.2.8** to release
  them (push tag `v0.2.8` → GoReleaser → `brew upgrade`).
- Paste opencode.ai `auth` cookie into `usage.opencode_cookie` for the 3rd
  usage tile (codex + claude-code already work).
