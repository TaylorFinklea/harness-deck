# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main, **ahead 5 + session commits — unpushed**. Build/tests green.
- 2026-06-10: bug bash + arch review done → `.harness/20260610-bug-bash-audit/`
  (30 verified findings). Product review locked: release → fix highs → launch.
- **v0.2.4 tag created locally, NOT pushed.** User to run
  `git push origin main v0.2.4` → GoReleaser release + tap formula. Then
  verify `hdeck contract` from a brew upgrade.

## Plan (bug-fix milestone — spec: phases/bug-fix-wave-spec.md)

- [ ] internal/jsonfile helper + migrate 5 writers + status-enum dedup — Verify: go test ./...
- [ ] respond.Record atomic + mutex; surface respond.Load err in store — Verify: go test ./...
- [ ] Cross-process report.json flock — Verify: go test ./...
- [ ] MCP list_reports scan_roots parity — Verify: go test ./internal/mcp/...
- [ ] DELETE blast-radius guard — Verify: go test ./internal/server/...
- [ ] Lenient known-block parse (Body nil on mismatch) — Verify: go test ./internal/manifest/... ./internal/store/...
- [ ] Markdown link scheme allowlist — Verify: go test ./internal/render/...
- [ ] live.js draft guard — Verify: human (browser)
- [ ] g-chord single owner — Verify: human (browser)

## Blockers

- None.

## Open questions

- None — 2026-06-10 product-review decisions are in decisions.md.
