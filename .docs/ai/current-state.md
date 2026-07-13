# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main, in sync with origin. **v0.2.14 RELEASED + installed +
  verified 2026-07-12** — macOS onboarding fix batch complete: hdeck cert +
  doctor, brew services persistence, Developer ID signing/notarization all
  live and proven end to end on mandalore. Spec/report:
  `phases/macos-onboarding-{spec,report}.md`.
- The whole point is proven: brand-new signed binary reached the phone URL
  with **zero manual firewall commands** — `hdeck doctor` 10/10 ok.

## Plan

- [ ] Release v0.2.15 — doctor: down-server now FAIL (v0.2.14's exit-0 gate
      certified broken installs), empty-scan_roots WARN, leftover hand-rolled
      service unit = FAIL. Plus AGENTS.md (new cross-harness entry point) +
      install-doc rewrite. SETUP.md already claims these as v0.2.15 behavior,
      so the docs are ahead of the binary until this ships.
      Verify: brew upgrade → hdeck version 0.2.15, doctor exit 0 here.

## Blockers

- None.

## Open questions

- None.
