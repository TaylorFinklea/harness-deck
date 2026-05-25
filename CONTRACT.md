# The harness-deck report contract

This is the spec a harness (Claude Code, Pi Mono, OpenCode, …) follows to
publish a report to harness-deck. A report is a single JSON file; harness-deck
renders it into a consistent themed page and aggregates it into the dashboard.

## Where to write the file

Write `report.json` into a run directory, in **either** location:

- **Central:** `~/.harness/reports/<project>/<run>/report.json`
- **Per-project:** `<project-root>/.harness/<run>/report.json`

`<run>` is any stable id for the run. The directory may also hold artifacts;
harness-deck writes `responses.json` next to `report.json` (see below).

Validate before publishing: `harness-deck validate path/to/report.json`.

## Top-level fields

```jsonc
{
  "schema":  "harness-deck/report@1",   // required, exact value
  "id":      "0x4a2f",                  // required, the run id
  "project": "acme-platform",           // required, groups reports
  "harness": "claude-code",             // required, who produced it
  "agent":   "claude-sonnet-4.5",       // optional, model id
  "title":   "14 → 16 readiness audit", // required
  "scope":   "postgres",                // optional, highlighted before the title
  "kind":    "audit",                   // optional: audit | progress | idea | roadmap | …
  "status":  "awaiting-review",         // required: draft | awaiting-review | answered | done
  "created": "2026-05-18T18:39:50Z",    // required, RFC3339
  "verdict": "conditional-go",          // optional, headline conclusion
  "meta":    [{"key": "cost", "value": "$1.84"}],   // optional, ordered run metadata
  "blocks":  [ /* ordered content blocks — see below */ ]
}
```

A report with `kind: "roadmap"` also appears in the dashboard's roadmap view.

## Content blocks

`blocks` is an ordered list. Every block has a `type`; most accept an optional
`title` (panel heading) and `pills` (`[{"text": "...", "level": "ok|warn|err"}]`).
The renderer owns all layout — blocks carry content, never markup. An unknown
block type degrades to a visible error panel rather than breaking the report.

| type | purpose | key fields |
|---|---|---|
| `prose` | Markdown text panel | `markdown` |
| `metrics` | metric grid + optional bars | `metrics[]` (`label,value,unit,delta,trend,spark,color`), `bars[]` |
| `risks` | severity register | `risks[]` (`severity: crit\|high\|med\|low`, `label`, `pct`), `callouts[]` |
| `diff` | code diffs | `files[]` (`path,lang,lines[]` where line `kind: ctx\|add\|del\|hunk`) |
| `timeline` | event log | `events[]` (`time,kind,markdown`) |
| `compare` | A/B comparison | `a`, `b` (each `tag,title,items[]` with item `kind: pro\|con\|neu`) |
| `recommendations` | numbered actions | `items[]` (`id,owner,markdown`) |
| `callout` | info/warn/err aside | `level`, `tag`, `markdown` |
| `barchart` | labeled bars | `bars[]` (`label,pct,color`) |
| `table` | columnar data | `columns[]`, `rows[][]` |
| `html` | **escape hatch** — raw HTML inside panel chrome | `html` |

Markdown fields support paragraphs, `# headings`, `- lists`, `**bold**`,
`*italic*`, `` `code` ``, fenced ``` ```lang ``` ``` blocks (rendered with a
copy button), GitHub-style tables (`| h | h |` / `| - | - |` / rows),
`> ` blockquotes, and links (`[text](url)` or `<https://…>`). See
`samples/postgres-audit.report.json` for a complete worked example.

### Interactive blocks

These pose a question; the user's answer is recorded in `responses.json`. Each
needs a unique `id` within the report.

- **`ask`** — `{id, prompt, mode: "choice"|"yesno"|"text", options[]}` (options
  required for `choice`).
- **`decision`** — `{id, prompt, a, b}` — same A/B shape as `compare`; records
  the chosen side's `tag`.
- **`approval`** — `{id, prompt}` — records `approved` or `changes-requested`.

## Consuming responses

When the user answers, harness-deck writes `responses.json` into the run
directory:

```jsonc
{
  "run": "0x4a2f", "project": "acme-platform",
  "updated": "2026-05-20T00:44:27Z",
  "responses": {
    "token-strategy": { "block": "token-strategy", "value": "semantic tokens",
                        "note": "", "at": "2026-05-20T00:44:27Z" }
  }
}
```

A harness reads `responses.json` from the run directory (e.g. in a session-start
hook) to pick up the user's answers, keyed by interactive-block `id`.
