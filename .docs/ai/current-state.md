# harness-deck — current state

_Last-session breadcrumb. Not a journal — keep it short._

## Where things are

Phase 0 + Phase 1 complete.

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

Phase 2 — discovery + aggregator:

1. `internal/config`: `~/.config/harness-deck/config.toml` (central_dir,
   projects, notify_command, port).
2. `internal/store`: scan central dir + per-project `.harness/` dirs.
3. `internal/server` + aggregator shell page (sidebar tree, home overview).
4. `harness-deck serve`.

Also outstanding: write `CONTRACT.md` (agent-facing manifest spec).

## Blockers / open questions

- None.
