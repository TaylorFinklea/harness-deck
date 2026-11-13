# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: `codex/modernize-ui` (PR #1). UI fixes pushed as `8b08af1`;
  the local checkout tracks the PR branch. No merge or public release.
- Installed baseline before this update: v0.2.16 (`e77023c`). The PR build is
  an unreleased local development build.
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
- Doctor recognizes current `sh.brew.*` and legacy `homebrew.mxcl.*` service
  namespaces; real Homebrew services are no longer mislabeled for deletion.
  Setup guidance requires confirming ownership before removing obsolete units.

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

- Local service activation, signed build/version, preservation checks and the
  rollback path are recorded in Beads `harness-deck-pr1-qa-12r`. The build uses
  the existing Developer ID identity without Apple notarization; a later brew
  upgrade/reinstall replaces the local override. No public release is included.
- PR review/merge remains separate from the authorized push and local update.
- Existing broader case left out of scope: if an arbitrary keyboard-focused
  dynamic control is replaced while a dialog is open, closing falls back to
  visible content rather than resolving an equivalent replacement.

## Blockers

- None.
