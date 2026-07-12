# macOS onboarding fix — report (2026-07-12)

Spec: `macos-onboarding-spec.md`. All deliverables shipped on main, one commit
each; unreleased (v0.2.14 pending).

## Shipped

- **D1** `fff51c8` — SETUP.md hotfix (stdout-split cert pipeline, tested
  verbatim on this machine; firewall section).
- **D2** `8a32cf8` — `hdeck cert`: finds tailscale ($PATH → MAS bundle path),
  host from `Self.DNSName`, PEM classified by block type, atomic writes, key
  0600, config patched as raw map (~-relative paths), `--renew`/`--force`.
  Table-driven tests: EC/PKCS#8/RSA keys, key-first order, chain-of-4, dup
  keys, garbage between blocks, config unknown-field preservation.
- **D3** `2c50bcf` — `hdeck doctor`: config/providers/url-tls/tls files/
  expiry/SAN/key-mode/VAPID/port/tailscale/firewall, `--json`, exit 1 on FAIL.
  Firewall check dials the running server on a non-loopback IP (true phone
  path); self-listen probe only when the server is down. `usage.
  UnknownProviders` pinned to Build's switch by test.
- **D4** `cccaa21` — GoReleaser: `brews[].service` (verified against the
  formula template: lines wrapped in `service do…end`; opt_bin survives
  upgrades) + `notarize.macos` gated on MACOS_SIGN_P12; release.yml exports
  the 5 secrets. `goreleaser check` valid; snapshot build skips sign cleanly.
- **D5** `00549f6` — SETUP.md happy path = brew install → brew services start
  → doctor → cert → doctor; hand-rolled units to Appendix A (neutral label
  `com.harnessdeck.serve`); README mobile section rewritten.

## E2E verification (this machine, mandalore)

`cert --renew` no-op at 89d; `cert --force` re-issued + config diff showed
only key reordering (beads/usage/bind all preserved); doctor all-ok exit 0
including the external-interface dial; phone URL 200 with real cert
validation. Negative paths exercised: provider typo + https-without-tls both
FAIL with fixes, exit 1.

## Investigation notes (what the spec's evidence section doesn't say)

The ALF block did NOT reproduce after the machine's ruleset was touched — an
unallowlisted ad-hoc dev binary passed under terminal AND launchd. The
morning's per-binary block (nc control) was real; the verdict is stateful.
Hence doctor prefers dialing the running server over trusting a fresh
self-probe, and docs say "at risk", not "always dropped". Full chain in
decisions.md 2026-07-12.

## Left open

- Apple enrollment + secrets, then v0.2.14 — roadmap Now #8/#9 (landmines
  inline there).
- ollama-cloud usage provider (roadmap Later) — requested on setup, doesn't
  exist; doctor now names it instead of silently ignoring it.
