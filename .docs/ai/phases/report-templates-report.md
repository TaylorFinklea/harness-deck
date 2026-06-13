# Report templates — completion report (2026-06-13)

First post-launch feature. `harness-deck new --template <kind>` scaffolds a
report.json pre-filled with the block shapes a given report kind usually needs.

## What shipped

- `--template audit|review|progress|decision|idea` on the `new` subcommand.
- Each template pre-fills purposeful blocks (see decisions.md "Report
  templates" / docs/PUBLISHING.md "Start from a template" for the per-template
  block list).
- One-flag UX: template supplies a default `--title` and defaults `--kind` to
  the template name; both overridable (explicit flag wins, detected via
  `fs.Visit`).
- No-template path unchanged (single placeholder prose block) → backward
  compatible.

## Files

- `cmd/harness-deck/templates.go` (new) — `reportTemplate`, `reportTemplates`,
  `templateOrder`, `templateNames()`, block-JSON constants.
- `cmd/harness-deck/new.go` — `--template` flag, validation, title/kind
  defaulting, `flagSet` helper, `starterReport(...,blocksJSON)` splice,
  interactive-aware success message.
- `cmd/harness-deck/main.go` — top-level usage banner mentions `--template`.
- `cmd/harness-deck/new_test.go` — `TestNewTemplatesValidate`,
  `TestNewTemplateExplicitFlagsOverride`, `TestNewDefaultScaffoldValidates`,
  `TestTemplateRegistrySync`, `TestNewTemplateWithOut`.
- `docs/PUBLISHING.md` — "Start from a template" section + smoke-test pointer.

## Verification

- `go build ./...`, `go vet ./cmd/harness-deck`, `go test ./...` green.
- Every template + the default scaffold: `harness-deck validate` → zero
  problems; `harness-deck render` → zero fallback panels; interactive blocks
  render with real panel titles.
- Adversarial 3-lens review workflow (correctness / template-UX / edge-cases);
  all actionable findings addressed (success-message status reminder,
  interactive-block titles, registry cross-check test, usage/doc gaps).

## Follow-up (routed to roadmap Next, not done here)

- **Draft-gating decision:** a `draft` report with an interactive block still
  counts as an open ask and fires push, so scaffolding an interactive template
  into a watched dir notifies for placeholder content. App-wide behavior change
  → roadmap Next, decision needed.
