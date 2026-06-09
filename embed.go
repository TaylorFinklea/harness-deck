// Package harnessdeck embeds the canonical agent-facing docs into the binary
// so harness-deck can describe itself without a repo clone.
//
// CONTRACT.md (the report schema) and docs/PUBLISHING.md (the gentler
// walkthrough) live at the repo root as the single source of truth; this
// package reads them at compile time via go:embed. Because they ship inside
// the binary, an installed harness-deck always serves the contract that
// matches its own version — the `contract` subcommand, the MCP
// `harness-deck://contract` resource, and the HTTP /contract.md endpoint all
// read from here.
package harnessdeck

import _ "embed"

// Contract is the full report contract (CONTRACT.md) — the exhaustive schema
// reference an agent follows to publish a report.
//
//go:embed CONTRACT.md
var Contract string

// Publishing is the gentler walkthrough (docs/PUBLISHING.md) — minimum-viable
// manifest, the smoke test, and the handful of blocks covering most reports.
//
//go:embed docs/PUBLISHING.md
var Publishing string
