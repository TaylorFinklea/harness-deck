# harness-deck — decisions

_ADR log. Newest at the bottom. Record non-obvious choices and why._

## 2026-05-20 — Separate repo, not part of chezmoi-config

harness-deck is its own repo. `chezmoi-config` is a deliberately thin dotfiles
overlay and explicitly not an app/orchestration/reporting layer. Harness-side
integration (hooks/skill) will later live in `chezmoi-config` as a thin pointer.

## 2026-05-20 — Go for the server

Single static binary, no runtime dependency, trivial to launch and manage in
`~/.local/bin`. Fits the user's single-binary CLI-tool ecosystem (chezmoi,
starship, atuin). Frontend stays vanilla HTML/CSS/JS — no build step.

## 2026-05-20 — Authoring contract is a JSON block manifest

Harnesses write `report.json`: an ordered list of typed blocks. The renderer
owns all HTML/CSS, so reports are consistent by construction and old reports
restyle when the renderer changes. Rejected: agent-writes-raw-HTML (no
consistency guarantee, un-restylable) and MCP-first (running-server dependency,
per-harness config burden). A raw-`html` block is the escape hatch for novel UI;
recurring `html` usage is the signal to promote a pattern to a typed block.
JSON is the single canonical format — no parallel Markdown-block format.

## 2026-05-20 — Both central and per-project report storage

Reports are discovered from a central dir (`~/.harness/reports/…`) and from
per-project `.harness/…` dirs of registered project roots. Per-project reports
travel with the repo; central is the catch-all.

## 2026-05-20 — Response round-trip via file + notification

Dashboard responses are written to `responses.json` beside the report and a
configured notification command fires. No live socket coupling required for the
harness to pick responses up — a file is universal across harnesses.

## 2026-05-20 — Go structs are the schema; no standalone report.schema.json

The plan listed a `report.schema.json`. Built instead with Go structs as the
single schema and a Go-native `Validate` (strict decode with
`DisallowUnknownFields` + semantic/enum checks). Reasons: the renderer needs
the structs regardless, a hand-written JSON Schema would drift from them, and a
JSON-Schema validation library is avoidable weight. `CONTRACT.md` (to be
written) is the human/agent-facing spec. A machine-readable schema can be
generated later if an MCP report-builder needs one.

## 2026-05-20 — Inline assets, with a </script guard

`render` produces a single self-contained HTML file: CSS and `vim-nav.js` are
inlined rather than linked. Because vim-nav.js mentions the literal `</script>`
in a header comment, the renderer rewrites `</script` → `<\/script` before
inlining (the HTML parser ends a script at `</script` regardless of JS
context). The vendored asset file itself stays verbatim for the server to
serve as a static file.

## 2026-05-20 — JSON config, not TOML

Config is JSON (`~/.config/harness-deck/config.json`), not the TOML the plan
named. Reason: TOML needs a third-party parser (a module download); JSON is
stdlib. Keeps the build hermetic — no `go get`. The whole project stays
zero-external-dependency on purpose (see also the in-house Markdown renderer).

## 2026-05-20 — Live updates by polling, not fsnotify

Phase 3 detects report changes by polling the report directories on an
interval rather than using `github.com/fsnotify/fsnotify`. Same reason as
above — no module download — and for a handful of local report directories a
~2s poll is indistinguishable from a watcher. The plan said fsnotify; the
observable behaviour (live SSE updates) is identical.

## 2026-05-20 — Roadmap view reuses .docs/ai/roadmap.md

The aggregator's roadmap view renders each registered project's existing
`.docs/ai/roadmap.md` (an established handoff convention) plus roadmap items
agents append via manifests. No new authoring surface.
