# harness-deck — current state

_Last-session breadcrumb. Not a journal — keep it short._

## Where things are

**All planned phases (0–5) are complete.** `go build ./... && go test ./...`
is green. The whole MVP works end-to-end and was verified in a browser.

What exists:

- **`internal/manifest`** — report manifest + 14 block types (11 content + 3
  interactive: ask/decision/approval). Lenient parse, strict `Validate`.
- **`internal/render`** — `html/template` renderer → the v1 TUI HTML page.
  Unknown/broken blocks degrade to a fallback panel. Dependency-free Markdown
  renderer (`markdown.go`, exported as `Markdown`).
- **`internal/store`** — discovers `report.json` under the central dir and each
  project's `.harness/`; in-memory index; `Signature()` fingerprint for change
  detection; `OpenAsks` excludes answered blocks.
- **`internal/server`** — aggregator shell, `/api/reports`, `/api/roadmap`,
  `/events` (SSE), `/r/{project}/{run}` report pages, `POST …/respond`. A 2s
  watcher polls + broadcasts.
- **`internal/respond`** / **`internal/notify`** — `responses.json` read/write;
  the configured notify command.
- **`internal/config`** — JSON config at `~/.config/harness-deck/config.json`.
- **`internal/assets`** — vendored design CSS/JS + `aggregator.js` / `respond.js`.
- **`cmd/harness-deck`** — `validate`, `render`, `serve`.
- **`CONTRACT.md`** — the agent-facing report spec.

Deviations from the plan (all in decisions.md): zero external dependencies — Go
structs + `CONTRACT.md` instead of a `report.schema.json`; JSON config not TOML;
in-house Markdown renderer; live updates by polling, not fsnotify.

Manual test fixtures live in `/tmp/hd-test/` (config + sample reports):
`HARNESS_DECK_CONFIG=/tmp/hd-test/config.json harness-deck serve`.

## Next

Phase 6 — harness-side integration — lives in `chezmoi-config`, not here: thin
hooks/skill so Claude Code / Pi Mono / OpenCode emit manifests and read
`responses.json`, plus an optional MCP report-builder. Not started.

Possible follow-ups in this repo: `harness-deck register <path>` and `new`
(scaffold) subcommands; richer Markdown; report-page live reload.

## Blockers / open questions

- None.
