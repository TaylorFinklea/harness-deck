# harness-deck — current state

_Last-session breadcrumb. Not a journal — keep it short._

## Where things are

Phases 0–3 complete. `go build ./... && go test ./...` green.

Phase 3 additions:
- `internal/store`: each Entry carries the report's ModTime; `Signature()` is a
  fingerprint of the indexed files (path + modtime).
- `internal/server/sse.go`: an SSE `hub`, the `GET /events` endpoint, and a
  `watch` goroutine that polls every 2s and broadcasts `change` when the store
  signature moves. `/api/reports` no longer rescans — the watcher owns scanning.
- `aggregator.js`: connects to `/events` via EventSource and reloads on change.
Verified: adding a report file makes it appear in the tree/inbox with no reload.

Phase 2 additions:
- `internal/config`: JSON config at `~/.config/harness-deck/config.json`
  (central_dir, projects, notify_command, port); all fields defaulted.
- `internal/store`: scans the central dir + each project's `.harness/` for
  `report.json` files; in-memory index; counts interactive blocks as `OpenAsks`.
- `internal/server`: aggregator shell, `GET /api/reports` (rescans per request),
  `GET /r/{project}/{run}` report pages; `harness-deck serve` works.
- `internal/assets`: `aggregator.css` + `aggregator.js` — the dashboard frontend
  builds the sidebar tree and 4 switchable home views (inbox/overview/latest/
  roadmap) with safe DOM construction (no innerHTML).

Verified end-to-end with sample reports in `/tmp/hd-test/` (kept for Phase 3/4
testing): `HARNESS_DECK_CONFIG=/tmp/hd-test/config.json harness-deck serve`.

## Earlier (Phases 0–1)

- Go module `github.com/TaylorFinklea/harness-deck` (Go 1.26). Builds and
  tests clean: `go build ./... && go test ./...`.
- Design assets vendored into `internal/assets/` (embedded): `tokyo-night.css`,
  `v1.css`, `vim-nav.js`, plus `deck.css` for table/fallback styling.
- `internal/manifest`: report + block union types, lenient parse, strict
  `Validate`. 11 block types (prose, metrics, risks, diff, timeline, compare,
  recommendations, callout, barchart, table, html). Unknown types parse
  leniently (Body nil) but Validate flags them.
- `internal/render`: `html/template` renderer → full v1 TUI HTML page. Unknown
  or unrenderable blocks degrade to a `block-fallback` error panel. Tiny
  dependency-free Markdown renderer in `markdown.go`.
- `cmd/harness-deck`: `validate` and `render` subcommands work; `serve` stubbed.
- `samples/postgres-audit.report.json` mirrors the design reference content;
  renders and visually matches `v1-tui-dashboard.html`.

Note: deviated from the plan's `report.schema.json` — Go structs +
`CONTRACT.md` are the schema; `validate` is Go-native (no JSON Schema lib /
drift). See decisions.md.

## Next

Phase 4 — response round-trip:

1. Register interactive block types in `internal/manifest`: `ask`, `decision`,
   `approval` (currently they hit the fallback panel).
2. Render them as interactive controls in `internal/render`.
3. `POST /r/{project}/{run}/respond` → write `responses.json` into the run dir.
4. `internal/notify`: run the configured `notify_command`.
5. Report-page JS to POST responses.

Also outstanding: write `CONTRACT.md` (agent-facing manifest spec).

## Blockers / open questions

- None.
