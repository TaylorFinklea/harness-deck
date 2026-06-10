# Community launch posts

Use these as starting points after the Medium article is published. Adapt each
post to the target community's rules and norms. Do not cross-post the same text
verbatim everywhere.

## Hacker News / Lobsters style

Title:

```text
I built a local pane of glass for AI coding agents
```

Post:

```text
I have been using multiple AI coding harnesses across a bunch of repos, and I
kept running into the same artifact-sprawl problem: Markdown reports in chat,
one-off HTML mockups in random directories, decisions buried in scrollback, and
no consistent place for agents to show human-facing work.

So I built harness-deck: a local Go dashboard where any harness can publish a
small report manifest. It renders Markdown, tables, diffs, comparisons, raw HTML
mockups, and interactive asks/approvals. Responses get written back to
responses.json next to the report, so the next agent run can pick them up.

The goal is not another chat UI or an agent orchestrator. It is a presentation
layer for coding agents: chat is where we talk, harness-deck is where agents
show their work.

Article: https://medium.com/@taylor.finklea/i-built-a-pane-of-glass-for-my-ai-coding-agents-caca1d47e0a4
Repo: https://github.com/TaylorFinklea/harness-deck
```

## Reddit style

Title:

```text
I built a local dashboard so AI coding agents have somewhere to show their work
```

Post:

```text
I use several AI coding tools across different repos, and the thing that kept
breaking down was not code editing. It was review artifacts.

One agent would leave a Markdown report in chat. Another would generate an HTML
mockup somewhere under the repo. A third would ask a decision question in a
session I was about to clear. Useful output, but scattered everywhere.

I built harness-deck to give agents a shared local presentation surface. Any
harness can write a report.json manifest; the dashboard renders it as a report
with Markdown, diffs, tables, comparisons, raw HTML mockups, and interactive
asks/approvals. When I answer, it writes responses.json next to the report.

It is local-first, harness-neutral, and intentionally not another chat UI. It is
just the pane of glass agents use when they need to show me something.

I wrote up the reasoning here: https://medium.com/@taylor.finklea/i-built-a-pane-of-glass-for-my-ai-coding-agents-caca1d47e0a4
Repo: https://github.com/TaylorFinklea/harness-deck
```

## Substack / personal blog note

```text
I wrote about a small tool I built for my AI coding workflow: harness-deck.

The core idea is that AI coding agents need a presentation layer. Chat is fine
for conversation, and Markdown is fine for prose, but agents increasingly need
to show richer human-facing artifacts: product mockups, audit reports, review
summaries, decisions, approvals, and visual comparisons.

I kept getting those artifacts scattered across chat transcripts and one-off
HTML files, so I built a local dashboard where any harness can publish a report
manifest. It is a pane of glass for agent output, not another chat app.

Article: https://medium.com/@taylor.finklea/i-built-a-pane-of-glass-for-my-ai-coding-agents-caca1d47e0a4
Repo: https://github.com/TaylorFinklea/harness-deck
```

## Short social post

```text
AI coding agents are good at editing repos, but the artifacts they show humans
are still scattered across chat, Markdown, and random HTML files.

I built harness-deck as a local pane of glass where agents can publish reports,
mockups, asks, decisions, and approvals.

Article: https://medium.com/@taylor.finklea/i-built-a-pane-of-glass-for-my-ai-coding-agents-caca1d47e0a4
Repo: https://github.com/TaylorFinklea/harness-deck
```

## Launch checklist

- Medium URL filled after publishing.
- GitHub URL filled with the public repository URL.
- Add one screenshot or GIF before posting anywhere visual.
- For Reddit, pick one community first and participate in comments before
  posting broadly.
- For HN/Lobsters style sites, keep the title concrete and avoid marketing
  language.
