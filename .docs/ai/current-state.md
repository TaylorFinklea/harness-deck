# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main, pushed (origin caught up through `7f32600`). **v0.2.7 released
  + installed + verified 2026-06-16** — first release since v0.2.6 (2026-06-10).
  GoReleaser CI green (4 platforms + tap formula); `brew upgrade` done, service
  restarted, `hdeck version` = 0.2.7. Verified live: `/api/usage` 200 +
  `/api/search/schema` 200. Ships the whole post-launch backlog: usage monitors,
  perf wave, config live-reload, JS modularization, search query language,
  saved searches.
- **Doc correction:** earlier notes implied v0.2.6 shipped usage monitors — it
  did not; v0.2.6 is the launch tag, these reached users only in v0.2.7.
- **Usage bar live** (`~/.config/harness-deck/config.json` → `usage.providers
  = [claude-code, codex, opencode]`). codex ✅ green (`weekly 76%`); claude-code
  wired (got a transient `http 429`, no Keychain block — populates on a
  non-rate-limited poll); opencode awaits the opencode.ai `auth` cookie in
  `opencode_cookie`.
- **Feature assessment done 2026-06-16** (7-agent workflow). Verdict: core loop
  feature-complete; gaps = shallow interactive layer (single-string answers) +
  no cross-report narrative. Top picks → roadmap Backlog (note-field wire-up,
  SSE response event, notify response env, multi-select asks, activity
  timeline, CI workflow). Not yet started.

## Plan

- Empty. Next: pull from roadmap Backlog (assessment top picks) or Later.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- Paste opencode.ai `auth` cookie into `usage.opencode_cookie` for the 3rd
  usage tile (codex + claude-code work without it).
