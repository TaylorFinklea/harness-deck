# harness-deck — roadmap

## Vision

One place where any AI coding harness can publish a consistently-formatted report
— progress, an opinion request, a decision, an idea — and where the user reviews
an upcoming roadmap. Reports are authored as a JSON block manifest; a local Go
server renders, aggregates, and live-updates them, and routes the user's
responses back to the harness.

## Milestones

- **Phase 0 — repo skeleton.** Repo, Go module, vendored design assets, handoff
  docs. _(done)_
- **Phase 1 — manifest + renderer.** Go manifest types; renderer for every
  block type incl. `html` escape hatch + Markdown; page shell; CLI `validate` /
  `render`. _(done — Go structs are the schema; see decisions.md)_
- **Phase 2 — discovery + aggregator.** Config, scan central + per-project dirs,
  aggregator shell page, `harness-deck serve`. _(done)_
- **Phase 3 — live updates.** Change detection → SSE → live tree/view updates.
  _(done)_
- **Phase 4 — response round-trip.** Interactive block controls, `/respond`,
  `responses.json`, notification command.
- **Phase 5 — roadmap view.** Render each project's `.docs/ai/roadmap.md` plus
  agent-appended roadmap items.

## Later / out of scope

- Harness-side integration (hooks/skill so harnesses emit manifests and pick up
  responses) — lives in `chezmoi-config`, separate task.
- Optional MCP report-builder server.
- v1a/v1b/v1c visual refinements (v1 original only for now).
- Multi-user / auth / cloud sync — local single-user only.

## Constraints

- Frontend stays vanilla HTML/CSS/JS, reusing the design files verbatim
  (`tokyo-night.css`, `vim-nav.js`, v1 layout). No frontend build step.
- The renderer owns all report HTML/CSS — manifests never contain layout, only
  content (except the deliberate `html` escape-hatch block).
- The harness→deck contract is a written file. MCP, if added, is a convenience
  wrapper, never the only path.
