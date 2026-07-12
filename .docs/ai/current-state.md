# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main. v0.2.13 released; **macOS onboarding fix batch on main,
  unreleased** (hdeck cert + doctor, brew services block, gated notarization,
  SETUP/README rewrite). Spec/report: `phases/macos-onboarding-{spec,report}.md`.
- mandalore (this machine) fully set up + verified: doctor exit 0, phone URL
  200 with real cert validation, usage (codex+claude-code) + beads writable on.

## Plan

- [ ] Apple Developer enrollment + the 5 MACOS_* repo secrets (roadmap Now #8,
      USER action). Verify: release logs non-skipped sign & notarize.
- [ ] Release v0.2.14 (roadmap Now #9 — read its two landmines first:
      old-LaunchAgent bootout; ALF allowlist pins the Cellar path).
      Verify: brew upgrade → doctor exit 0, formula has `service do`.

## Blockers

- None.

## Open questions

- None.
