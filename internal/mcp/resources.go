package mcp

import harnessdeck "github.com/TaylorFinklea/harness-deck"

// instructions is the short, stable guidance the MCP server hands a client
// during the initialize handshake. It explains *when* to use harness-deck and
// points at the contract resource for the full schema — so it stays small and
// can't drift from CONTRACT.md (the detail lives in the resource, not here).
const instructions = `harness-deck is the user's local dashboard for AI coding work. Two jobs:

• Publish a report (publish_report tool) to leave a durable record of major
  work — a feature, refactor, audit, or investigation — or a multi-session
  roadmap (set kind: "roadmap"). Skip trivial or exploratory work.
• Ask a rich question with an interactive block (ask / decision / approval)
  when the options carry too much context for a plain inline picker: a mock-up
  to react to, a paragraph per option, or an answer that must survive a
  context clear. The user answers in the dashboard; read it back with
  get_responses. Don't use this for short, self-explanatory choices — ask
  those in the harness's native inline picker.

For the full report schema and every block type, read the
harness-deck://contract resource (or run ` + "`harness-deck contract`" + `).
The harness-deck://publishing resource is a gentler walkthrough.`

// defaultResources exposes the embedded agent-facing docs as MCP resources, so
// a client can pull the schema with no repo clone — version-locked to this
// binary. Bodies come from the root harnessdeck package's go:embed vars.
func defaultResources() []resourceDef {
	return []resourceDef{
		{
			Resource: Resource{
				URI:         "harness-deck://contract",
				Name:        "report-contract",
				Description: "The full harness-deck report schema (CONTRACT.md): every top-level field and block type.",
				MimeType:    "text/markdown",
			},
			Text: harnessdeck.Contract,
		},
		{
			Resource: Resource{
				URI:         "harness-deck://publishing",
				Name:        "publishing-guide",
				Description: "Gentler walkthrough (PUBLISHING.md): minimum-viable manifest and the common blocks.",
				MimeType:    "text/markdown",
			},
			Text: harnessdeck.Publishing,
		},
	}
}
