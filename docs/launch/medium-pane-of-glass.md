# I Built a Pane of Glass for My AI Coding Agents

AI coding agents have become good at doing work across a repository. They can
read code, make edits, run tests, inspect failures, draft plans, and come back
with a summary. But as I started using more of them, I ran into a smaller and
stranger problem:

They did not have a good place to show me things.

Claude Code would leave a long Markdown report in chat. Codex would generate a
local HTML mockup. Another harness would ask for a decision halfway down a
transcript I was about to clear. A product review would live in one session, a
visual comparison in another, a status note in a third. All of it was useful.
None of it had a home.

The agents were getting better at building things, but the human-facing layer
was still improvised.

That is why I built harness-deck: a local pane of glass where any AI coding
harness can publish the artifacts it wants a human to inspect.

## The artifact sprawl problem

If you use AI coding tools heavily, you probably recognize the shape of this.

The agent finishes a task and gives you a summary in chat. That is fine until
you want to compare it to what another agent did yesterday.

The agent builds a UI mockup and writes an HTML file somewhere under `/tmp` or
inside the repo. That is fine until you have five of them, from three tools,
with no shared index.

The agent asks a question in the middle of a long session: "Should I take
approach A or B?" That is fine until the context gets cleared, the terminal
scrollback disappears, or you want to answer from your phone later.

Markdown is a good note format. Chat is a good conversation format. Neither is
a very good durable presentation layer for agent work.

What I wanted was a place where the agent could say:

> I need the human to look at this.

And then put the thing somewhere consistent.

## Before and after

Before harness-deck, the flow looked like this:

```text
agent output
  -> chat summary
  -> loose Markdown
  -> one-off HTML page
  -> screenshot
  -> decision buried in scrollback
```

After harness-deck, the flow looks like this:

```text
agent output
  -> report.json
  -> local dashboard
  -> rendered report, mockup, ask, decision, approval
  -> responses.json
```

The important part is not the JSON. The important part is the shared contract.

Every harness gets the same way to publish a human-facing artifact. The
dashboard owns the rendering. The report owns the content. The human gets one
place to review it.

## harness-deck is not another chat UI

I do not want another chat surface. Chat is already where I talk to agents.
harness-deck is for the moments when the agent needs to show its work.

That can mean:

- a code review summary
- an architecture decision
- a product mockup
- a before/after visual
- a test or audit report
- a launch plan
- a question that should survive a context clear

It can be Markdown. It can be a structured table. It can be a comparison. It
can be an interactive approval request. It can also be raw HTML rendered inside
an isolated block, which matters because sometimes the useful artifact is not
prose. Sometimes the agent needs to show a small interface, diagram, chart, or
mockup.

That was the original itch: my agents were generating HTML pages to show me
things, and those pages were scattered everywhere. harness-deck gave that habit
a center.

## The contract

A harness-deck report is a JSON manifest written to disk. A local Go server
discovers it, renders it, and adds it to the dashboard.

The manifest is intentionally simple:

```json
{
  "schema": "harness-deck/report@1",
  "id": "20260608-product-review",
  "project": "my-app",
  "harness": "codex",
  "title": "Checkout redesign review",
  "status": "awaiting-review",
  "created": "2026-06-08T21:00:00Z",
  "blocks": [
    {
      "type": "prose",
      "markdown": "The new checkout flow is ready for review."
    },
    {
      "type": "ask",
      "id": "ship-or-revise",
      "prompt": "Ship this version or revise the payment step?",
      "mode": "choice",
      "options": ["ship", "revise payment step"]
    }
  ]
}
```

When I answer the question in the dashboard, harness-deck writes a
`responses.json` next to the report. The next agent run can read it and keep
going.

There is no cloud dependency in that loop. It is local files and a local server.
That matters because the artifacts belong to my repos, my work sessions, and my
machine.

## The HTML block matters

Markdown covers a lot, but not everything.

For a normal status report, Markdown is enough. For a code review, a typed
report block is better. But for a UI mockup, a flow diagram, or a visual
comparison, the agent needs a canvas.

harness-deck has an `html` block for that. It renders arbitrary HTML, inline
CSS, and SVG inside an isolated shadow root. Scripts do not run. The block is a
visual surface, not an application runtime.

That gives agents a safe-ish, practical way to show richer artifacts without
dumping random standalone pages across the filesystem.

The distinction is useful:

- chat is for conversation
- Markdown is for prose
- code diffs are for implementation
- harness-deck is for human-facing artifacts

## Why harness-neutral matters

I use more than one coding harness. That is becoming normal.

One tool may be better for a long autonomous repo pass. Another may be better
for fast local edits. Another may have a better browser or mobile story. The
agent ecosystem is moving quickly, and I do not want my review surface tied to
one of them.

harness-deck is deliberately harness-neutral. Claude Code, Codex, OpenCode, Pi
Mono, or a custom script can all publish the same manifest. The dashboard does
not care who wrote it.

That makes it feel less like a feature of one AI tool and more like a local
workbench primitive.

## What it does today

The current version of harness-deck can:

- discover reports across many repos
- render Markdown, tables, metrics, timelines, diffs, comparisons, and raw HTML
- show open asks, decisions, and approval requests
- write responses back to `responses.json`
- group work by project
- show per-project current state and roadmap docs
- live-update as reports change
- run locally as a small Go server
- install with Homebrew

It is not trying to orchestrate agents. It is not a remote collaboration
platform. It is not a general-purpose BI dashboard. It is the pane of glass
where coding agents can show me things.

That boundary is important. The narrower the job, the more useful it becomes.

## Trying it

Install:

```sh
brew install taylorfinklea/tap/harness-deck
```

Run:

```sh
hdeck serve
```

Open:

```sh
hdeck open
```

Publish a starter report from a repo:

```sh
hdeck register .
hdeck new --in-repo --title "hello from my agent"
```

The full contract lives in the repository. A harness can publish by writing
`report.json` directly, or by using the optional MCP server.

## The larger pattern

The more capable agents become, the more they need interfaces that are not
chat.

Not every artifact should be a message. Some should be dashboards. Some should
be mockups. Some should be approvals. Some should be durable records that
survive the session where they were produced.

That is the pattern I am trying to name with harness-deck:

AI coding agents need a presentation layer.

The human still decides. The agent still works in the repo. harness-deck is the
shared surface between those two facts.

If your agents are already producing reports, mockups, plans, and decisions,
you may not need a smarter chat window. You may need a better place for them to
show their work.
