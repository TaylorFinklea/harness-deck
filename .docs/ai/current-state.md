# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main, synced with origin. Build/tests green.
- 2026-06-10: bug bash + arch review done → `.harness/20260610-bug-bash-audit/`
  (30 verified findings). Product review locked: release → fix highs → launch.
- **v0.2.4 released + verified** (binaries, tap formula, local brew upgrade,
  `hdeck contract` works). Next up: the fix milestone below, then v0.2.5.

## Plan (bug-fix milestone — spec: phases/bug-fix-wave-spec.md)

- [x] internal/jsonfile helper + migrate 5 writers + status-enum dedup — Verify: go test ./...
- [x] respond.Record atomic + mutex; surface respond.Load err in store — Verify: go test ./...
- [x] Cross-process report.json flock — Verify: go test ./...
- [x] MCP list_reports scan_roots parity — Verify: go test ./internal/mcp/...
- [x] DELETE blast-radius guard — Verify: go test ./internal/server/...
- [ ] Lenient known-block parse (Body nil on mismatch) — Verify: go test ./internal/manifest/... ./internal/store/...
- [ ] Markdown link scheme allowlist — Verify: go test ./internal/render/...
- [ ] live.js draft guard — Verify: human (browser)
- [ ] g-chord single owner — Verify: human (browser)

## Blockers

- None.

## Open questions

- None — 2026-06-10 product-review decisions are in decisions.md.
