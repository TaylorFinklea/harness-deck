# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: `main`, tracking `origin/main`. PR #1 merged as `ac3915a`;
  public v0.2.17 tags `03f3c7f`, including UI fix `8b08af1` and doctor fix
  `6980074`. Release workflow and Go/browser CI pass.
- Official v0.2.17 is installed through Homebrew, replacing the local
  development override. The tap formula matches the published checksums.
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
- Local Homebrew service now runs official `0.2.17` on port 7420.
  Installed doctor exits 0; config state and all report/response hashes are
  unchanged. Its remaining warning is no configured scan roots. Both macOS
  release binaries have verified Developer ID signatures and CDHashes matching
  accepted Apple notarization tickets. Prior keg and backups are retained.
- Original UI audit: widths 360–1536px, eight sample panels, zero fallbacks or
  horizontal overflow; dark/light desktop/mobile review and Axe checks passed.

## Next

- Release, credential-repair approval, artifact verification, local upgrade and
  rollback evidence are recorded in Beads `harness-deck-pr1-qa-gxy`. Prior
  local-build evidence: `harness-deck-pr1-qa-22p` / `harness-deck-pr1-qa-12r`.
- Existing broader case left out of scope: if an arbitrary keyboard-focused
  dynamic control is replaced while a dialog is open, closing falls back to
  visible content rather than resolving an equivalent replacement.

## Blockers

- None.
