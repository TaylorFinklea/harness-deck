# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main. **Unpushed** (origin/main last caught up through `c30cc7e`):
  the search query-language milestone (`fa88dce`→`dddd2e8`, 6 commits) **plus
  saved searches** — `7dbf4b0` (spec), `3e75aff` (feature), + this doc commit.
  All push-ready; user reviews + pushes.
- **Saved searches shipped 2026-06-16** (roadmap Now #6, the whole Now list is
  now clear). Option A: pin a JQL query to a new SAVED sidebar section → click
  opens the Cmd+K palette pre-filled + live. localStorage (`window.HDSaved`,
  mirrors HDPins), click-only activation. Built Sonnet-impl + Haiku-gate +
  2×Sonnet-review (model scorecard updated). gofmt/build/test/vet/node-check all
  green; **browser-verified** (chrome-devtools) full save→sidebar→activate→
  remove — caught + fixed a CSS↔inline-style bug static review missed.
  decisions.md "Saved searches"; spec phases/saved-searches-spec.md.
- Prior context (roadmap shipped list): v0.2.6 era, perf wave, usage monitors,
  session-code audit, search query language.

## Plan

- Empty. Roadmap Now is fully shipped — pull from Backlog (deferred items) or
  Later (saved-searches follow-ups) when starting fresh.

## Blockers

- None.

## Open questions

- None.

## Out (human-gated)

- Push the unpushed commits (search query-language milestone + saved searches)
  after review.
- Enable usage monitors: add tools to `usage.providers` in config (Claude needs
  a one-time Keychain allow; OpenCode needs a pasted cookie). docs/SETUP.md §8.
