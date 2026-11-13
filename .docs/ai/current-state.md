# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: `codex/fix-pr1-keyboard-qa`, based on PR #1 head `856ad46`.
  Local follow-up fixes; no push or release.
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
- macOS QA follow-up: dashboard Space chords work on focused native controls;
  Enter activates them. Forms and dialogs keep native Space behavior. Pointer-
  opened dialogs restore their launcher in Safari, including saved-search
  launchers rebuilt by a live update; missing/hidden launchers use visible
  content as a fallback. Rationale: decisions.md, "Dashboard Space chords and
  Safari dialog launchers". Beads: `harness-deck-pr1-qa-2du.1` / `.2`.

## Validation

- `go build ./...`, `go vet ./...`, `go test -race ./...`, formatting and diff checks pass.
- Exact Playwright 1.62.0: Chromium 30/30 and WebKit 30/30 pass with isolated,
  schema-validated fixtures. New regressions reproduce the original failures
  before the fixes. No application dependencies were added.
- Actual Safari 27.0.1 verified tree toggle, pointer Settings/Search focus,
  Space chords from button focus, Enter activation, modal native Space,
  incomplete-chord cancellation, and saved-search focus through a real SSE
  sidebar refresh. Sample data/config stay isolated on port 7421.
- Independent GPT-6.1 Terra review completed with no remaining findings.
- Original UI audit: widths 360–1536px, eight sample panels, zero fallbacks or
  horizontal overflow; dark/light desktop/mobile review and Axe checks passed.

## Next

- The local fix branch can be integrated into PR #1 when the owner authorizes
  a push. The Homebrew service was not replaced; no release/deployment occurred.
- Existing broader case left out of scope: if an arbitrary keyboard-focused
  dynamic control is replaced while a dialog is open, closing falls back to
  visible content rather than resolving an equivalent replacement.

## Blockers

- None.
