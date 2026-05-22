# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

harness-deck is a local Go dashboard for AI coding work. Any AI harness writes a
structured JSON **report manifest** (`report.json`); harness-deck renders it into
consistent themed HTML, aggregates every report into a live dashboard, and routes
the user's answers to interactive blocks back to the harness as `responses.json`.

## Commands

```sh
go build ./...                              # build everything
go test ./...                               # run all tests
go test ./internal/render -run TestReport   # run one package / one test
./harness-deck serve                        # start the dashboard (default :7420)
./harness-deck validate report.json         # check a manifest
./harness-deck render report.json -o out.html
```

There is no Makefile, linter config, or CI — `go build` / `go test` are the
whole toolchain. `gofmt` before committing.

Manual end-to-end testing uses fixtures outside the repo; point `serve` at an
alternate config with the env override:

```sh
HARNESS_DECK_CONFIG=/tmp/hd-test/config.json ./harness-deck serve
```

## Hard constraint: zero external dependencies

`go.mod` has **no `require` block** — the project is stdlib-only on purpose
(see `.docs/ai/decisions.md`). Do not `go get` anything. This is why the repo
has an in-house Markdown renderer (`internal/render/markdown.go`), JSON config
instead of TOML, and a 2s polling watcher instead of `fsnotify`. If a task
seems to need a library, reach for stdlib or stop and ask.

The frontend is likewise build-step-free: vanilla HTML/CSS/JS vendored under
`internal/assets/` and embedded with `go:embed`. No npm, no bundler.

## Architecture

The data flow is a one-way pipeline with a file-based response loop:

```
harness → report.json → store (discovers) → render → server (serves)
                                                         ↓
harness ← responses.json ← respond (writes) ← server (records answer) → notify
```

Packages (`internal/`), each with a package doc comment that is the best
starting point:

- **`manifest`** — the report schema. **Go structs are the schema** (no
  `report.schema.json`). `Parse` is lenient (unknown block types leave `Body`
  nil, unknown fields ignored) so a stale renderer degrades gracefully;
  `Validate` is strict (`DisallowUnknownFields` + enum/semantic checks).
- **`render`** — `html/template` → a complete v1-TUI HTML page. The renderer
  **owns every byte of layout and CSS**; manifests carry content only. One bad
  block renders a fallback error panel instead of failing the whole page.
- **`store`** — discovers `report.json` under the central dir and each
  registered project's `.harness/`; in-memory index; `Signature()` fingerprint
  drives change detection.
- **`server`** — aggregator shell, `/api/reports`, `/api/roadmap`, `/events`
  (SSE), `/r/{project}/{run}` report pages, `POST .../respond`. A 2s watcher
  polls the store and broadcasts SSE refreshes.
- **`respond`** / **`notify`** — `responses.json` read/write; the configured
  notify command fired when an answer is recorded.
- **`config`** — JSON config at `~/.config/harness-deck/config.json`; every
  field defaults, so it runs with no config file.
- **`assets`** — vendored frontend files embedded for self-contained output.

## Key conventions

- **`CONTRACT.md` is the agent-facing spec and must stay in sync with the
  `manifest` structs.** Any change to report/block fields updates both.
- **Adding a block type** touches four places: a `Block` struct + registry
  entry in `internal/manifest/`, a `block-<type>` template in
  `internal/render/templates/`, a default title in `render.go`'s
  `defaultTitles`, and a row in `CONTRACT.md`. The `html` block is the
  deliberate escape hatch — recurring `html` usage is the signal to promote a
  pattern to a real typed block.
- **Graceful degradation is a design rule**, not an accident: unknown block
  types, missing `responses.json`, and absent config files all resolve to a
  sensible fallback rather than an error.
- Trust boundary: harness-deck is a local single-user tool, so the `html`
  block and `safeHTML` template func intentionally bypass HTML escaping.

## Handoff docs (`.docs/ai/`)

`.docs/ai/{roadmap,current-state,decisions}.md` are the git-tracked source of
truth for cross-session continuity. Read them at the start of a session and
update them at the end — `current-state.md` is a short breadcrumb (blockers,
build status, recent progress), `decisions.md` is an append-only ADR log.
Phases 0–5 are complete; further plan context lives in `roadmap.md`.
