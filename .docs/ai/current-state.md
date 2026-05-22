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
- **`internal/store`** — discovers `report.json` under the central dir and the
  `.harness/` of each project root it is given; in-memory index; `Signature()`
  fingerprint for change detection; `OpenAsks` excludes answered blocks.
- **`internal/projects`** — discovers project roots (depth-1 children of
  `scan_roots` holding `.docs/ai`, plus explicit `projects`); `projects.json`
  records hidden ones (new = enabled); `Toggle` writes it atomically.
- **`internal/server`** — aggregator shell, `/api/reports`, `/api/projects`,
  `POST /api/projects/toggle`, `/events` (SSE), `/r/{project}/{run}` report
  pages, `POST …/respond`. A 2s watcher polls store + projects and broadcasts.
- **`internal/respond`** / **`internal/notify`** — `responses.json` read/write;
  the configured notify command.
- **`internal/config`** — JSON config at `~/.config/harness-deck/config.json`;
  `scan_roots` lists directories searched for project roots.
- **`internal/assets`** — vendored design CSS/JS + `aggregator.js` / `respond.js`.
  The 4th aggregator view is **projects**: per-project current-state + roadmap,
  with a collapsible "tracked projects" panel of discovery toggles.
- **`cmd/harness-deck`** — `validate`, `render`, `serve`, `version`.
- **`CONTRACT.md`** — the agent-facing report spec.
- **`CLAUDE.md`** — repo guidance for Claude Code (architecture, the
  zero-dependency constraint, the four-places block-type checklist).
- **Release pipeline** — `.goreleaser.yaml` + `.github/workflows/release.yml`:
  pushing a `v*` tag builds static darwin/linux binaries, publishes a GitHub
  Release, and commits the formula to `TaylorFinklea/homebrew-tap`. The formula
  installs `harness-deck` plus a short `hdeck` symlink alias. Install via `brew
  install taylorfinklea/tap/harness-deck` or `go install`. `v0.1.1` is the
  latest released tag (it carries the `hdeck` alias).

Deviations from the plan (all in decisions.md): zero external dependencies — Go
structs + `CONTRACT.md` instead of a `report.schema.json`; JSON config not TOML;
in-house Markdown renderer; live updates by polling, not fsnotify.

Project discovery + tracking (the `projects` package, `/api/projects`, the
projects view) was built on branch `feat/project-discovery` — see decisions.md.

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
