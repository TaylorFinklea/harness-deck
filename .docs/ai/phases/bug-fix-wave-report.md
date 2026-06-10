# bug-fix-wave — report (2026-06-10)

All 9 spec items shipped, one commit each, TDD throughout (every fix has a
test that failed first demonstrating the bug). Spec: `bug-fix-wave-spec.md`;
findings detail: `.harness/20260610-bug-bash-audit/report.json`.

| # | Item | Commit | Failing-test evidence |
|---|---|---|---|
| 1 | internal/jsonfile (AtomicWrite/Patch/Upsert) + migrate all writers + ValidStatus dedup | ab19589 | update_live mangled 9007199254740993→…992; corrupt config clobbered to one key |
| 2 | respond.Record mutex + atomic write; store surfaces corrupt responses.json | a251b5d | 49 of 50 concurrent answers lost; green under -race |
| 3 | flock in jsonfile.patch — cross-process serialization | a9d3535 | 1 of 50 concurrent Patch increments survived |
| 4 | MCP list_reports mirrors server.enabledRoots | 24e7e6c | scan_roots-discovered report → "reports": [] |
| 5 | DELETE blast-radius guard (409 on shared roots / nested reports) | c109db9 | test literally destroyed a nested report pre-fix |
| 6 | Lenient known-block parse (Body nil on mistyped field) | b645880 | one "markdown":123 killed the whole report's Parse |
| 7 | Markdown link scheme allowlist | cabb96f | javascript:/data:/vbscript: rendered as live hrefs |
| 8 | live.js draft guard + tap-to-reload banner | (this commit) | Playwright: draft survives heartbeat, banner shows, click reloads, no-draft auto-reload intact |
| 9 | g-chord single owner (HDKeys registry in tabs.js) | (this commit) | Playwright: dashboard g p/g i in-place, g t cycles pins (was dead), gg+a no longer double-dispatches |

Deviations from spec:

- Item 3's lock lives inside `jsonfile.patch()` rather than per-call-site,
  so config.json writers (register vs notifications CRUD) got cross-process
  safety for free.
- Item 8's affordance is a click-to-reload chip, not "press r" — `r` is
  already bound to reopen on report pages.
- Item 9 added `HDKeys.pendingPrefix` so the aggregator's capture-phase
  handler stands down during a chord (capture-phase would otherwise eat
  completion keys before bubble-phase tabs.js sees them). Keyboard
  precedence = bundle order, now documented in assets.go.

Verification: `go build ./... && go test ./...` green (plus -race on the new
concurrency tests); `node --check` on edited JS; items 8-9 driven end-to-end
in a real browser against a fixture server (typing guard, banner round-trip,
chord matrix). v0.2.5 tagged at the milestone head.
