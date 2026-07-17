# Contributing to harness-deck

Thanks for your interest! This file covers the ground rules for humans;
[`AGENTS.md`](AGENTS.md) is the deeper companion (architecture, package map,
conventions) and applies to you too — it's just written agent-first.

## Build, test, format

```sh
go build ./...    # build everything
go test ./...     # run all tests (CI runs go test -race)
gofmt -l .        # must print nothing before you commit
```

There is no Makefile and no linter config — this is the whole toolchain.

## The one hard constraint: zero dependencies

`go.mod` has **no `require` block**, on purpose (see
`.docs/ai/decisions.md`). Do not `go get` anything. This is why the repo has
an in-house Markdown renderer, JSON config instead of TOML, and a 2s polling
watcher instead of fsnotify. If your change seems to need a library, use the
stdlib or open an issue first.

The frontend is equally build-step-free: vanilla HTML/CSS/JS under
`internal/assets/`, embedded with `go:embed`. No npm, no bundler.

## Making changes

- **Tests come with the change.** The suite is the review gate; new behavior
  needs a test that fails without it.
- **`CONTRACT.md` must stay in sync with the `manifest` structs** — the Go
  structs are the schema, and the contract file is embedded into the binary
  at build time.
- **Adding a report block type** touches four places (manifest struct +
  registry, render template, default title, CONTRACT.md row).
  `TestRegistryCrossCheck` in `internal/render` names exactly what's missing
  if you forget one.
- **Graceful degradation is a design rule**: unknown block types, missing
  config, missing responses.json all resolve to a sensible fallback, never an
  error. New code should behave the same way.
- Scope note: harness-deck is deliberately a **local, single-user** tool.
  Multi-user, auth, and cloud sync are out of scope.

## Forking / self-hosting

Everything user-facing is configurable — see the config section of
[`docs/SETUP.md`](docs/SETUP.md). The knobs forks most often need:

- `project_markers` — which paths mark a repo as a project (default
  `[".docs/ai"]`; set e.g. `[".git"]` to discover every repo).
- `push_subject` — the contact URL embedded in Web Push VAPID JWTs
  (defaults to this repo's URL; set your own).
- `$XDG_CONFIG_HOME` and `HARNESS_DECK_CONFIG` relocate the config file.

To publish your own builds, edit `.goreleaser.yaml`: `brews[].repository`
(tap owner) and `homepage` point at this repo. macOS signing/notarization is
env-gated on the `MACOS_*` secrets — without them releases still work, just
unsigned (the macOS Application Firewall then silently drops inbound LAN
connections; `hdeck doctor` prints the fix). Renaming the Go module path is
optional — only needed if you want `go install` from your fork.

## License

MIT. By contributing you agree your contributions are licensed under it.
