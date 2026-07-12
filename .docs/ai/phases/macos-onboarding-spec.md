# macOS onboarding fix — spec

Goal: the README's headline "Mobile (PWA) + push" path must work first-try for a
stranger on a fresh Mac. Article is live; sequencing matters.

## Confirmed root causes (fresh-machine evidence, 2026-07-12)

1. **Firewall drops the binary.** Release binaries are ad-hoc/linker-signed
   (`codesign -dv` → `Identifier=a.out`, `Signature=adhoc`). macOS Application
   Firewall auto-allows Apple/Developer-ID-signed software only; incoming
   connections to harness-deck on non-loopback interfaces are silently dropped
   (timeout, no error). Proven by control experiment: Apple-signed `nc`
   listening on the same host answered on the tailnet IP; harness-deck did not.
   Loopback is unaffected. launchd-started processes can't show the
   "accept incoming connections?" dialog — hence *silent*.
2. **`tailscale cert --cert-file <path>` cannot write anywhere** under the Mac
   App Store Tailscale build (`com.apple.security.app-sandbox`; MAS receipt
   present). Fails `operation not permitted` for `~/.config` and `/tmp` alike.
   SETUP.md §6 is broken for every MAS-Tailscale user. Workaround that works:
   `--cert-file - --key-file -` to stdout, caller splits and writes.
3. **Hand-pasted LaunchAgent** in SETUP.md (30 lines XML, author's personal
   label `com.tfinklea.harness-deck`). Formula is GoReleaser-generated
   (`DO NOT EDIT`) with no `service` block.
4. **`usage.providers` silently ignores unknown names**
   (`internal/usage/usage.go` Build switch) — a typo is a silent no-op.

## Decisions (user-approved 2026-07-12)

- **Signing**: Developer ID + notarization via GoReleaser (`notarize` block,
  cert + ASC API key as GitHub secrets). User enrolls in Apple Developer
  Program (his action; may take days).
- **Certs**: new `hdeck cert` subcommand (stdout-split approach). Renewal =
  documented scheduled `hdeck cert --renew`, NOT inside `serve`.
- **Persistence**: `brew services` via GoReleaser `brews[].service` block.
  Hand-rolled plist/systemd stay only as a fallback appendix for
  go-install/raw-binary users.
- **Diagnostics**: full `hdeck doctor` preflight. Needed even after signing:
  go-install binaries are ad-hoc signed too — signing does NOT fix them.

## Deliverables

### D1 — docs hotfix (ships today, zero code)

SETUP.md §6: replace the `--cert-file <path>` invocation with the
stdout-and-split pipeline (works on both Tailscale builds); add a macOS
firewall note with the `socketfilterfw --add/--unblockapp` pair and the
version-pinned-Cellar-path caveat.

### D2 — `hdeck cert` (`cmd/harness-deck/cert.go`)

- Locate `tailscale`: `$PATH`, then
  `/Applications/Tailscale.app/Contents/MacOS/Tailscale`.
- Host from `tailscale status --json` → `Self.DNSName` (trim trailing dot);
  overridable positional arg.
- Run `tailscale cert --cert-file - --key-file - <host>`; parse ALL PEM blocks
  from combined stdout and classify by block type — cert chain =
  `CERTIFICATE` blocks, key = the non-certificate block (`EC/RSA/PRIVATE
  KEY`). Never trust ordering.
- Validate leaf with `crypto/x509` (parses; SANs cover host).
- Write `<configdir>/tls/<host>.crt` + `.key` atomically (temp file + rename,
  same dir); key `0600`. Never print key material.
- Update config: set `tls.cert`, `tls.key`, and (if empty) `public_url` =
  `https://<host>:<port>`; preserve unknown fields when rewriting config.json
  (read as raw map, not the typed struct).
- `--renew`: exit 0 no-op if existing cert valid >30 days, unless `--force`.
- Flags/registration/help: mirror the `vapid` subcommand's registration in
  `cmd/harness-deck/main.go` and its output style.

### D3 — `hdeck doctor` (`cmd/harness-deck/doctor.go` + testable core)

Checks (each OK/WARN/FAIL + one-line fix; `--json` for agents; exit 1 on FAIL):

- config parses; unknown `usage.providers` vs the valid set exported from
  `internal/usage` (add `ValidProviders()` or equivalent next to the Build
  switch so the two can't drift); `https` `public_url` without `tls` and
  vice versa.
- TLS: files exist, key mode 0600, leaf parses, SANs cover `public_url` host,
  expiry (WARN <30d, FAIL expired).
- VAPID file present when `public_url` is https.
- Port: `bind:port` free, or held by a live harness-deck (probe
  `/api/reports`).
- Tailscale (darwin/linux, only when reachable): BackendState Running,
  ShieldsUp false.
- **Firewall self-probe** (darwin, only when bind ≠ loopback): open own
  listener on ephemeral port on the non-loopback IP, dial it back, 3s
  timeout. Because ALF decisions are per-binary this reproduces the failing
  condition exactly, without the server running and without sudo. On FAIL,
  print `sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add <path>
  && … --unblockapp <path>` with `os.Executable()` resolved. Note in output
  that a terminal run may pop the Allow dialog — clicking Allow also fixes it.
- Decision logic lives behind an interface/struct with injected probe results
  so it unit-tests without network/OS.

### D4 — release pipeline (`.goreleaser.yaml`)

- `brews[].service` block: `run [opt_bin/"harness-deck", "serve"]`, keep_alive,
  log paths under `var/log`. **Verify exact GoReleaser v2 syntax against its
  docs before writing** (codebase-derived; do not guess).
- `notarize` block wired to GitHub secrets (cert P12 + ASC API key). Verify
  schema + whether GoReleaser signs bare binaries or needs an archive step —
  read the docs first. Lands only when the user's Apple enrollment clears;
  keep in a separate commit so releases work without it meanwhile.

### D5 — docs rewrite

- SETUP.md happy path: `brew install` → `brew services start harness-deck` →
  `hdeck doctor` → (phone) `hdeck cert` → restart service → `hdeck doctor`.
- Plist/systemd XML → fallback appendix (go install / raw binary), with the
  explicit note that go-install binaries stay ad-hoc signed and will hit the
  firewall (run doctor).
- README Mobile section: point at `hdeck cert`; cron/launchd line for
  `hdeck cert --renew`.
- CONTRACT.md untouched (no manifest changes).

## Sequencing

1. D1 commit now.
2. D2+D3 with tests; D4 service block; D5. One commit per deliverable.
3. Notarization commit when Apple enrollment done (user action pending).
4. Release v0.2.14; re-verify on this machine end-to-end.

## Testing

- Table-driven: PEM split/classify (EC / PKCS#8 / RSA, key-first ordering,
  missing key, garbage between blocks); provider validation; doctor decision
  logic with injected results; config rewrite preserves unknown fields.
- `go build ./... && go test ./...` gate.
- Manual e2e on this machine: `hdeck cert` regenerates the pair; `hdeck
  doctor` FAILs the firewall probe pre-allowlist and passes after; phone URL
  reachable after fix.

## Non-goals

- Auto-renew inside `serve`; `hdeck service install`; new usage providers
  (ollama-cloud / opencode-go cloud spend → roadmap); Windows.
