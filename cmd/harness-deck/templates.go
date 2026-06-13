package main

import "strings"

// A report template is a named scaffold: a default title plus the JSON value
// of the manifest's "blocks" array, pre-filled with placeholder blocks the
// user fills in. Templates pre-fill the block shapes docs/PUBLISHING.md
// recommends so the common report kinds start from a useful skeleton instead
// of a single empty prose panel.
//
// The blocks JSON is hand-written (type-first field order, same rationale as
// starterReport — the file is the first example the user edits) and indented
// to nest directly under a two-space top level. TestNewTemplatesValidate
// parses and strict-validates every template, so a typo here fails the build.
type reportTemplate struct {
	title       string // default --title when the user gives none
	blocks      string // JSON value of the "blocks" array
	interactive bool   // ships an ask/decision/approval block (drives the next-step hint)
}

// templateOrder is the human-facing order for usage text and error messages
// (roughly: report-on-past-work first, then forward-looking).
var templateOrder = []string{"audit", "review", "progress", "decision", "idea"}

var reportTemplates = map[string]reportTemplate{
	"audit":    {title: "Audit", blocks: auditBlocks},
	"review":   {title: "Code review", blocks: reviewBlocks, interactive: true},
	"progress": {title: "Progress update", blocks: progressBlocks},
	"decision": {title: "Decision", blocks: decisionBlocks, interactive: true},
	"idea":     {title: "Idea", blocks: ideaBlocks, interactive: true},
}

// templateNames returns the templates as a "a | b | c" string in templateOrder
// for the --template flag help and the unknown-template error.
func templateNames() string { return strings.Join(templateOrder, " | ") }

// defaultBlocks is the no-template scaffold: a single placeholder prose block.
// Interpreted string (not raw) because the guidance text uses `code` spans.
const defaultBlocks = `[
    {
      "type": "prose",
      "title": "summary",
      "markdown": "Replace this prose block with the actual report content.\n\n- Add bullets, **bold**, ` + "`code`" + `.\n- Switch ` + "`status`" + ` to ` + "`awaiting-review`" + ` once you add interactive blocks (ask/decision/approval)."
    }
  ]`

// The template block sets below are raw strings, so a literal \n is the JSON
// newline escape and the placeholder content deliberately avoids backticks.
// Angle-bracketed <fill me in> placeholders render literally (the markdown
// renderer HTML-escapes them).

const auditBlocks = `[
    {
      "type": "prose",
      "title": "summary",
      "markdown": "What was audited and the headline finding.\n\n- **Scope:** <what you looked at>\n- **Verdict:** <go / conditional / no-go>"
    },
    {
      "type": "table",
      "title": "findings",
      "columns": ["#", "severity", "finding", "where"],
      "rows": [
        ["1", "high", "<describe the finding>", "<file:line>"],
        ["2", "med", "<describe the finding>", "<file:line>"]
      ]
    },
    {
      "type": "recommendations",
      "title": "fixes",
      "items": [
        {"id": "r1", "markdown": "<the first fix to make>"},
        {"id": "r2", "markdown": "<the next fix>"}
      ]
    },
    {
      "type": "callout",
      "level": "warn",
      "tag": "before you act",
      "markdown": "<the one risk the user must weigh first>"
    }
  ]`

const reviewBlocks = `[
    {
      "type": "prose",
      "title": "what was reviewed",
      "markdown": "Branch, PR, or files under review and the overall read.\n\n- **Scope:** <what changed>\n- **Recommendation:** <approve / request changes>"
    },
    {
      "type": "recommendations",
      "title": "review comments",
      "items": [
        {"id": "c1", "markdown": "<file:line — the issue and the suggested change>"},
        {"id": "c2", "markdown": "<the next comment>"}
      ]
    },
    {
      "type": "approval",
      "title": "sign-off",
      "id": "review-signoff",
      "prompt": "Approve these changes, or request changes? Add specifics in the note."
    }
  ]`

const progressBlocks = `[
    {
      "type": "prose",
      "title": "what happened",
      "markdown": "Where the work stands now.\n\n- **Done:** <what landed>\n- **In progress:** <what is active>\n- **Blocked:** <anything stuck, or none>"
    },
    {
      "type": "recommendations",
      "title": "next steps",
      "items": [
        {"id": "n1", "markdown": "<the next action>"},
        {"id": "n2", "markdown": "<the action after that>"}
      ]
    }
  ]`

const decisionBlocks = `[
    {
      "type": "prose",
      "title": "context",
      "markdown": "The decision to make and why it matters now.\n\n- **Question:** <what are we deciding>\n- **Constraints:** <what bounds the choice>"
    },
    {
      "type": "decision",
      "title": "the decision",
      "id": "the-call",
      "prompt": "Which option should we take?",
      "a": {
        "tag": "A",
        "title": "<option A>",
        "items": [
          {"kind": "pro", "text": "<an upside of A>"},
          {"kind": "con", "text": "<a downside of A>"}
        ]
      },
      "b": {
        "tag": "B",
        "title": "<option B>",
        "items": [
          {"kind": "pro", "text": "<an upside of B>"},
          {"kind": "con", "text": "<a downside of B>"}
        ]
      }
    }
  ]`

const ideaBlocks = `[
    {
      "type": "prose",
      "title": "the idea",
      "markdown": "The pitch in a few lines.\n\n- **Problem:** <what is annoying today>\n- **Proposal:** <what to build or change>\n- **Why now:** <the trigger>"
    },
    {
      "type": "ask",
      "title": "go / no-go",
      "id": "pursue",
      "prompt": "Worth pursuing?",
      "mode": "yesno"
    }
  ]`
