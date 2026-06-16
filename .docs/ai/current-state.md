# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main. **v0.2.7 cut 2026-06-16** — first release since v0.2.6
  (2026-06-10). Pushed main + tagged `v0.2.7`; GoReleaser (GH Actions, on
  `v*` tag) builds 4 platforms, publishes the GitHub Release, commits the
  homebrew-tap formula. Ships the whole post-launch backlog: usage monitors,
  perf wave, config live-reload, JS modularization, search query language,
  saved searches. **Verify after CI:** `brew upgrade harness-deck` →
  `hdeck version` = 0.2.7, then `/api/usage` works + saved searches appear.
- **Doc correction:** earlier notes implied v0.2.6 shipped usage monitors — it
  did not; v0.2.6 is the launch tag, these reached users only in v0.2.7.
- **Usage bar configured** (`~/.config/harness-deck/config.json` → `usage.providers
  = [claude-code, codex, opencode]`, empty `opencode_cookie`). codex needs
  nothing; claude-code triggers a one-time Keychain "Always Allow"; opencode
  needs the opencode.ai `auth` cookie pasted in. Lit up once the 0.2.7 binary
  is installed (the running 0.2.6 had no `/api/usage` route).
- **Feature assessment done 2026-06-16** (7-agent workflow). Verdict: core loop
  feature-complete; gaps = shallow interactive layer (single-string answers) +
  no cross-report narrative. Top picks → roadmap Backlog (note-field wire-up,
  SSE response event, notify response env, multi-select asks, activity
  timeline, CI workflow). Not yet started.

## Plan

- Empty. Next: pull from roadmap Backlog (assessment top picks) or Later.

## Blockers

- None. (v0.2.7 release CI to be verified, then brew upgrade.)

## Open questions

- None.

## Out (human-gated)

- `brew upgrade harness-deck` to 0.2.7 + restart LaunchAgent after CI completes.
- Paste opencode.ai `auth` cookie into `usage.opencode_cookie` for the 3rd
  usage tile (codex + claude-code work without it).
