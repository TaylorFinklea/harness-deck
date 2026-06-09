# Self-describing binary: contract delivery

**Status:** in progress · **Author:** Opus · **Date:** 2026-06-09

## Problem

`CONTRACT.md` (the agent-facing schema spec) lives only in the repo root.
Agent instructions that consume it — the user's chezmoi `AGENTS.md` and the
installed `harness-deck` SKILL.md — hardcode `~/git/harness-deck/CONTRACT.md`.
On any machine without the clone (a coworker, a fresh work box) that path is
dead. Coworkers who `brew install` the binary get nothing that teaches their
agent the schema, when to publish, or that harness-deck exists at all.

## Goal

Make the **binary self-describing** so a coworker with only the installed
binary (no repo clone, no dotfiles) can be fully equipped by registering one
MCP server. The contract ships *inside* the binary and is therefore
version-locked: you always get the contract that matches the binary you
installed — never a GitHub `HEAD` that describes block types your renderer
can't draw (the codebase's graceful-degradation rule, applied to docs).

## Design: one embed, four surfaces

### 1. Embed (the primitive)
- New root-level package (`embed.go`, `package harnessdeck`) at the module
  root `github.com/TaylorFinklea/harness-deck`.
- `//go:embed CONTRACT.md` → `var Contract string`;
  `//go:embed docs/PUBLISHING.md` → `var Publishing string`.
- Canonical files stay where they are; `go:embed` reads them at compile time.
  No duplication, no symlink, no `go:generate`, no sync test — single source.
- Guard test asserts both are non-empty and `Contract` contains the schema
  marker `harness-deck/report@1` (catches a moved/renamed file breaking the
  embed silently).

### 2. CLI — `harness-deck contract`
- New `case "contract"` in `cmd/harness-deck/main.go`; `cmdContract` prints
  `harnessdeck.Contract` to stdout. `--publishing` flag prints the gentler
  guide instead. One usage line.

### 3. MCP — resources + initialize instructions (the coworker payoff)
- `internal/mcp`: add `resources/list` + `resources/read` handling, exposing
  two static resources — `harness-deck://contract` and
  `harness-deck://publishing` (mimeType `text/markdown`).
- Advertise the `resources` capability (`{}`) in the `initialize` result.
- Add an `Instructions` field to `initializeResult` populated with a short,
  *stable* Go const: what harness-deck is, when to publish a report vs. ask a
  rich question, and "read the `harness-deck://contract` resource for the full
  schema." Stays tiny so it can't drift from the contract; detail lives in the
  resource.
- Thread static `resources []Resource` + `instructions string` through
  `Serve` / `dispatch` / `Server` the same way `info ServerInfo` is threaded.

### 4. HTTP — `GET /contract.md`
- `internal/server`: `mux.HandleFunc("GET /contract.md", s.handleContract)`
  serving `harnessdeck.Contract` as `text/markdown`, mirroring
  `handleManifest`. So a running deck can hand the contract to curl / a
  browser / any agent over HTTP too.

### 5. Close the loop (references)
- In-repo: note the embed/self-describe surfaces in `CLAUDE.md` Key
  conventions; refresh `.docs/ai` handoff files.
- Chezmoi (`/Users/tfinklea/git/chezmoi-config`): repoint `AGENTS.md` and
  `dot_claude/skills/harness-deck/SKILL.md` from `~/git/harness-deck/CONTRACT.md`
  to "run `harness-deck contract`" + mention the MCP `harness-deck://contract`
  resource. `chezmoi apply` after.

## Coworker onboarding (the acceptance test for the whole design)
1. `brew install taylorfinklea/tap/harness-deck`
2. Register one MCP server pointing at `harness-deck mcp`.
3. Agent now sees the emit tool, reads the contract resource for the schema,
   and gets when-to-use instructions on connect — no clone, no skill copy, no
   path edits, version-locked to the installed binary.

## Constraints
- Zero external dependencies (stdlib + `go:embed` only). ✓
- `go build ./...` and `go test ./...` are the whole toolchain; `gofmt`.
- TDD: failing test first for embed guard, MCP resources/instructions, HTTP
  endpoint.

## Out of scope
- No GitHub-fetch / auto-update mechanism (rejected: would let docs drift
  ahead of the binary's real capabilities).
- Distributing the full SKILL.md as a plugin — the MCP `instructions` + the
  contract resource cover the coworker path without it.
