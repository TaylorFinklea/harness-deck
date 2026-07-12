# harness-deck — current state

_Loop state only. Shipped-work history → roadmap.md; rationale → decisions.md._

- Branch: main. v0.2.13 released; **macOS onboarding fix batch on main,
  unreleased** (hdeck cert + doctor, brew services block, gated notarization,
  SETUP/README rewrite). Spec/report: `phases/macos-onboarding-{spec,report}.md`.
- mandalore (this machine) fully set up + verified: doctor exit 0, phone URL
  200 with real cert validation, usage (codex+claude-code) + beads writable on.

## Plan

- [?] awaiting human verify — Developer ID cert via browser (roadmap Now #8, in
      progress): account IS enrolled; CSR ready at
      `~/.appstoreconnect/developer-id-application.csr` (key local, 0600). API
      creation exhausted: Apple limits Developer ID certs to the Account
      Holder in-browser (2× 403; keys 7L49/LL48 dead 401). USER: upload CSR at
      developer.apple.com → Certificates → Developer ID Application, download
      .cer. Then agent: assemble .p12 (password → Keychain), `gh secret set`
      the 5 MACOS_*, prove with a signed snapshot build. Existing account cert
      QN93Y65886 (exp 2027-02) has no local private key — unusable, left alone.
- [ ] Release v0.2.14 (roadmap Now #9 — read its two landmines first:
      old-LaunchAgent bootout; ALF allowlist pins the Cellar path).
      Verify: brew upgrade → doctor exit 0, formula has `service do`.

## Blockers

- None.

## Open questions

- None.
