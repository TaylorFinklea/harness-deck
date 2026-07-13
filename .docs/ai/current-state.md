# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main. v0.2.13 released; **macOS onboarding fix batch on main,
  unreleased** (hdeck cert + doctor, brew services block, gated notarization,
  SETUP/README rewrite). Spec/report: `phases/macos-onboarding-{spec,report}.md`.
- mandalore (this machine) fully set up + verified: doctor exit 0, phone URL
  200 with real cert validation, usage (codex+claude-code) + beads writable on.

## Plan

- [x] Roadmap Now #8 done — Developer ID cert issued (browser, Account
      Holder-only step), p12 assembled + chain-attached (quill), all 5
      MACOS_* secrets in GitHub. Locally verified signing works (codesign
      shows real chain; spctl correctly says unnotarized). Real notarization
      submission deliberately untested locally (see decisions.md) — first
      live test is at actual release time.
- [ ] Release v0.2.14 (roadmap Now #9 — read its THREE landmines first:
      old-LaunchAgent bootout; ALF allowlist pins the Cellar path; this is
      also the first live notarization submission, have the fallback ASC
      key (J79935N6P6) ready in case X3GUKCUJ9F lacks notary-submit rights).
      Verify: brew upgrade → doctor exit 0, formula has `service do`,
      codesign -dv shows Developer ID + notarized.

## Blockers

- None.

## Open questions

- None.
