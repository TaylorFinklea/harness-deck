# Usage monitors — spec / research notes (2026-06-13)

**Status: shipped 2026-06-14** (commits 3292e4a + b074c6b; `internal/usage`).
Config docs: docs/SETUP.md §8. Rationale: decisions.md "Usage monitors".
This file remains the data-source reference (verified live) for future edits.

Goal: CodexBar-style usage indicators in the dashboard footer (the vim
statusline, `shell.html.tmpl:62-68`, next to the address segment). Tools to
support: Claude Code, Codex, OpenCode, Copilot, OpenRouter.

All data sources below were **verified live on this machine** by a 5-agent
research sweep (2026-06-13). Schemas are undocumented/internal for the
file-based ones — parse leniently (ignore unknown fields), mirroring the repo's
own lenient-Parse philosophy.

## Two kinds of "usage" (don't conflate)

- **window** — % of a rate-limit window consumed + reset time (the real
  CodexBar number). Codex, Claude (via auth), Copilot (monthly).
- **budget/spend** — cumulative $ / tokens or credit balance. OpenRouter,
  OpenCode, Claude (local proxy).

## Per-tool data sources

### Codex — BEST FIT (true window %, pure stdlib, no auth)
- File: `$CODEX_HOME/sessions/YYYY/MM/DD/rollout-*.jsonl` (`CODEX_HOME` default
  `~/.codex`); also `~/.codex/archived_sessions/*.jsonl`.
- Read: scan newest files backward (the YYYY/MM/DD tree sorts lexically),
  find the LAST line `payload.type=="token_count"` whose `payload.rate_limits.primary`
  is non-null (many events have null windows — skip them).
- Fields: `rate_limits.primary` = 5h `{used_percent float, window_minutes:300,
  resets_at: unix-epoch-seconds}`; `rate_limits.secondary` = weekly
  (window_minutes 10080); `rate_limits.credits {has_credits,unlimited,balance}`;
  `rate_limits.plan_type`.
- Staleness: numbers are only as fresh as the last Codex turn; if `now >
  resets_at`, show 0%/"reset".
- Ref: `github.com/xiangz19/codex-ratelimit` (local-file reader, ideal analog).
  CodexBar itself (`github.com/steipete/CodexBar`) prefers an OAuth API.

### OpenRouter — clean stdlib HTTP
- `GET https://openrouter.ai/api/v1/key`, `Authorization: Bearer <sk-or-v1-...>`.
- Response `data`: `usage`, `usage_daily/weekly/monthly`, `limit` (number|null —
  null = uncapped), `limit_remaining`, `is_free_tier`. `limit_reset` is a
  cadence WORD ("monthly"), NOT a timestamp — no reset countdown.
- Budget meter: `limit_remaining/limit` %, plus daily/weekly/monthly spend.
- Key: `OPENROUTER_API_KEY` env (CodexBar reads exactly this) or hdeck config.
- `/api/v1/credits` (balance) needs a management key → avoid; use `/key`.
- Poll cadence ≥30–60s (counts against the key's rate limit).

### Claude Code — local proxy (no auth) OR true % (auth)
- Local proxy: `~/.claude/projects/<url-encoded-cwd>/<sessionId>.jsonl`,
  one JSON/line; assistant lines carry `message.usage` {input_tokens,
  output_tokens, cache_creation_input_tokens, cache_read_input_tokens}. Sum
  across lines in the last 300 min for a rolling-5h consumption proxy; dedup on
  `message.id`+`requestId`. Files are 2–18 MB → raise bufio buffer or decode
  per line. This is `ccusage blocks`. **Consumption proxy, NOT the server's %.**
- True % (the real number): authenticated `GET
  https://claude.ai/api/organizations/{orgId}/usage` (`Cookie: sessionKey=...`)
  → `five_hour {utilization 0-100, resets_at}` + weekly. The user's own
  `~/.claude/fetch-claude-usage.swift` statusline helper already does this.
  Credential on macOS lives in the **login Keychain** (generic-password service
  "Claude Code-credentials", account = $USER) — reading it triggers a security
  prompt + is a privacy boundary. NOT cached in any local file by Claude Code.

### Copilot — stdlib HTTP, undocumented endpoint
- `GET https://api.github.com/copilot_internal/user`, `Authorization: token
  <ghu_...>` (+ `Editor-Version`, `Copilot-Integration-Id` headers to be safe).
- Token: plaintext in `~/.config/github-copilot/apps.json` (key
  `github.com:Iv1...`, field `oauth_token`; fallback `hosts.json`,
  `~/.copilot/config.json`). Or shell out to `gh api /copilot_internal/user`.
- Metric: `quota_snapshots.premium_interactions {percent_remaining, remaining,
  entitlement}` + top-level `quota_reset_date` (monthly, resets 1st 00:00 UTC).
- ⚠️ Undocumented internal endpoint; GitHub says non-official-client use may
  violate Copilot ToS. Unversioned; GitHub migrating to "AI Credits" billing
  2026-06-01 (semantics may shift). The documented REST API is org/enterprise
  only — no public per-user-self endpoint.

### OpenCode — SQLite, breaks pure-stdlib-Go
- DB: `$OPENCODE_DATA_DIR/opencode.db` (default
  `~/.local/share/opencode/opencode.db`, WAL mode). `session` table columns:
  `cost REAL, tokens_input/output/reasoning/cache_read/cache_write INT, model
  TEXT(JSON!), time_updated (epoch ms)`. Cumulative per session.
- **No rate-limit/reset concept** — BYOK, provider-agnostic. Only cumulative
  $/tokens (per session/model/day).
- Zero-dep tension: reading SQLite needs a driver (CGO or modernc) → violates
  the no-`require` rule. Stdlib-only options: (a) shell out to `sqlite3 -json`
  (ubiquitous CLI, but a runtime-binary requirement); (b) read the LEGACY
  `storage/{session,message}/*.json` (stale after the v1.x SQLite migration →
  undercounts; what ccusage still reads); (c) hand-roll a SQLite page reader
  (too much). `session.model` is JSON-encoded — parse for provider/model.
  Query only the `session` aggregate columns (message/part blobs hold prompt
  content — privacy).

## Proposed architecture (pending scope decisions)

- New `internal/usage` package: a `Provider` interface returning a `Sample`
  {Tool, OK, Kind: "window"|"budget", Label, Percent *float64, ResetAt *time,
  Detail, Err}. One provider per enabled tool.
- A `usage.Monitor` goroutine refreshes each provider on its own cadence
  (local-file: ~10–30s or piggyback the watcher; HTTP: ~60s) and holds the
  latest samples behind a mutex.
- Server: `GET /api/usage` returns cached samples; push updates over the
  existing SSE channel. Config `usage` block: enabled providers + OpenRouter
  key (or env). Graceful degradation: a provider with no data/credential
  yields `OK:false` and is simply omitted from the footer (design rule).
- Frontend: render `seg` spans in the statusline next to the address; window
  kind → "CX 62% ⟳3:15" style, budget kind → "OR $12.30".

## Decisions (locked 2026-06-13)

All five providers, opt-in via config (nothing reads creds / hits network unless
the provider is listed):

- **Codex** — local JSONL, true 5h+weekly % + reset. No auth.
- **OpenRouter** — `GET https://openrouter.ai/api/v1/key`, budget meter
  (limit_remaining/limit + daily/weekly/monthly spend). Key from
  `OPENROUTER_API_KEY` env, else config `openrouter_key`.
- **Copilot** — `GET https://api.github.com/copilot_internal/user`, premium
  % + monthly reset. Token from `~/.config/github-copilot/apps.json`
  (fallback hosts.json, ~/.copilot/config.json). **Undocumented/ToS-gray —
  documented as such in code + docs.**
- **Claude Code** — true %: shell out `security find-generic-password -s
  "Claude Code-credentials" -w` (stdlib os/exec) → JSON `.claudeAiOauth.accessToken`
  → `GET https://api.anthropic.com/api/oauth/usage` with `Authorization: Bearer`,
  `anthropic-beta: oauth-2025-04-20`, `User-Agent: claude-code/<ver>` →
  `five_hour`/`seven_day` {utilization, resets_at}. One macOS Keychain
  "Always Allow" on first read, silent after; token auto-refreshes in place.
  Endpoint is **api.anthropic.com/api/oauth/usage** (NOT claude.ai/...). Needs
  `user:profile` scope (inference-only tokens 4xx). Handle missing windows.
- **OpenCode** — CodexBar-style subscription %: `GET https://opencode.ai/_server?id=<subHash>&args=["<wrk_id>"]`
  with the opencode.ai **`auth` session cookie** (pasted into config
  `opencode_cookie`), browser User-Agent/Origin/Referer headers; parse
  rollingUsage/weeklyUsage {usagePercent, resetInSec}. Get <wrk_id> from the
  workspaces `_server?id=<wrkHash>` call. **The `_server` hash IDs are
  opencode.ai build fingerprints that change on their deploys** — keep them in
  one place + a config override; degrade to OK:false when they 404/shift. (No
  Zen balance API exists — open feature request anomalyco/opencode#10448.)

Architecture as in the section above. Build order: core + Codex + OpenRouter
first (clean, end-to-end), then Claude + Copilot + OpenCode.
