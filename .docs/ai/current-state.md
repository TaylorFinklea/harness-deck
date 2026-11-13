# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: `codex/modernize-ui` (UI overhaul prepared for PR/local review).
- Last released: v0.2.15 (2026-07-13). The fork-friendliness wave on main
  (2026-07-16) remains unreleased; this work does not tag or publish a release.
- UI modernization preserves Tokyo Night, monospace, embedded vanilla assets,
  existing views, shortcuts, storage keys and report contracts. Adds shared UI
  styling, accessible native controls, modal focus management, readable themes,
  and responsive/dynamic-header fixes. See decisions.md, 2026-11-13.
- Correctness fixes: sorted cursor ordering, report g-chord/action precedence,
  project-history links, search cancellation/failure handling, and focus across
  SSE rerenders. Browser regressions live in `tests/browser.py`, with a separate
  CI job. CONTRIBUTING.md documents the test-only Playwright setup.

## Validation

- `go build ./...`, `go vet ./...`, `go test -race ./...`, formatting and diff checks pass.
- Chromium keyboard/responsive regression suite: 24 cases, using isolated,
  schema-validated fixtures; desktop widths 360–1440px.
- Manual visual review at 1536×1024 and 393×852. Axe WCAG A/AA checks pass on
  dashboard, projects and report surfaces in dark and light themes.

## Next

- Review/test the PR locally on macOS, including Safari and the normal daily
  keyboard workflow. Do not replace the Homebrew service until reviewed.
- No release or deployment performed.

## Blockers

- None.
