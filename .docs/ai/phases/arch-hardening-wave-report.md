# arch-hardening-wave — report (2026-06-10)

All five roadmap bullets (Now item 4) shipped, TDD-first, executed by parallel
subagents in isolated worktrees and cherry-picked onto main after review.
Plus one unplanned CI item. Suite + `-race` green; gofmt clean (modulo the
pre-existing notify/destinations_test.go).

| Item | Commit | Evidence |
|---|---|---|
| watcher `tick()` extraction + signature-gated digests + delta/CRUD tests | 8dbc9e3 | 8 new tests in tick_test.go; digests now recomputed only when changeFingerprint moves (fingerprint covers report + responses.json mtimes, so gating is sound) |
| scan-timing log | 8d4c68d | silent <100ms, info line 100–500ms, WARN >500ms; warn path tested via duration seam |
| assets bundle invariants + `</script` guard on RespondJS | 6488ca3 | RED: `RespondJSInline` didn't exist — the gap itself was the compile failure; bundle-order test pins keyboard precedence |
| schema `family@N` versioning | 2dbb266 | Parse lenient (same-family higher version renders w/ fallback panels; alien family errors), Validate strict ("newer than this binary supports — upgrade"); CONTRACT.md gained a Versioning section |
| registry cross-check test + absorb blockPrompt/blockText | 5232af7 | `TestRegistryCrossCheck` (render pkg) — injected fake type failed on template + defaultTitles + CONTRACT.md row; helpers moved verbatim to `manifest.BlockPrompt`/`manifest.BlockText` from server/push.go + server/search.go |
| (unplanned) GH Actions → Node 24 runtimes | faec79b | checkout v4→v6, setup-go v5→v6, goreleaser-action v6→v7; deadline was 2026-06-16 forced migration |

Operational note: the first v0.2.5 release run failed with `401 Requires
authentication` on the release POST — transient GitHub API flake (same 401
later hit `gh release view` interactively). `gh run rerun --failed` succeeded;
assets + tap formula verified at 0.2.5.
