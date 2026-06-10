# The harness-deck report contract

This is the spec a harness (Claude Code, Pi Mono, OpenCode, …) follows to
publish a report to harness-deck. A report is a single JSON file; harness-deck
renders it into a consistent themed page and aggregates it into the dashboard.

> New here? [`docs/PUBLISHING.md`](docs/PUBLISHING.md) is the gentler
> walkthrough — minimum viable manifest, the 60-second smoke test, and the
> handful of blocks that cover 90% of reports. This file is the exhaustive
> reference.

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

### Optional: `live` — in-flight telemetry

An active harness can publish a `live` block to surface real-time progress
on the report page (a pulsing banner) and in the inbox (a pulsing dot
with the current step). Every field except `updated` is optional.

```jsonc
{
  // ... other top-level fields ...
  "live": {
    "updated":    "2026-05-26T10:42:10Z", // required when `live` is set
    "step":       "running tests · pkg/parser",
    "elapsed_ms": 42000,                  // since the run started
    "tokens":     14230,                  // cumulative
    "cost_usd":   "0.42",                 // string for precision
    "progress":   0.63                    // 0..1 fraction
  }
}
```

The dashboard treats a run as **live** when `live.updated` is within the
last 60 seconds; older reports still show their last reported state but
the pulse stops. Re-write the manifest (or call the MCP `update_live`
tool) every few seconds while the run is active.

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
| `html` | raw HTML/CSS/SVG canvas — full control inside panel chrome | `html` |

Markdown fields support paragraphs, `# headings`, `- lists`, `**bold**`,
`*italic*`, `` `code` ``, fenced ``` ```lang ``` ``` blocks (rendered with a
copy button), GitHub-style tables (`| h | h |` / `| - | - |` / rows),
`> ` blockquotes, and links (`[text](url)` or `<https://…>`). See
`samples/postgres-audit.report.json` for a complete worked example.

### The `html` block

The `html` field is rendered **verbatim** as the contents of an isolated
**shadow root** inside the themed panel — arbitrary HTML, inline `<style>`, and
`<svg>` all work. It's the canvas for one-off rich content (custom layouts,
rendered mock-ups, inline charts) that markdown and the typed blocks can't
express; prefer a typed block for any *recurring* shape so it restyles with the
renderer and stays consistent.

Because the block is isolated:

- **Your `<style>` and selectors stay inside the block** — they can't leak out
  and restyle the dashboard, and the page's CSS won't bleed into your markup.
  Style freely with bare selectors (`div { … }`) or inline `style="…"`.
- **`<script>` does not execute** — html blocks are for layout/visuals, not
  interactive JS widgets. (Need interactivity? That's a feature request for a
  typed block.)
- **The Tokyo Night theme variables are available** (CSS custom properties
  inherit through the shadow boundary), so use them for colors that adapt to
  light/dark and restyle with the theme instead of hardcoding hex:

  `--tn-bg` `--tn-bg-dark` `--tn-bg-highlight` · `--tn-fg` `--tn-fg-dark`
  `--tn-comment` · `--tn-blue` `--tn-cyan` `--tn-purple` `--tn-green`
  `--tn-yellow` `--tn-orange` `--tn-red` `--tn-magenta` `--tn-teal` ·
  semantic: `--tn-ok` `--tn-warn` `--tn-err` `--tn-info` · `--tn-rule` (hairline).

  ```json
  { "type": "html", "html": "<div style=\"color:var(--tn-fg);border:1px solid var(--tn-rule);padding:10px\">themed <b style=\"color:var(--tn-green)\">ok</b></div>" }
  ```

  Images, SVG, video, and tables are capped to the panel width and the block
  scrolls horizontally if its content is wider, so a wide layout won't break
  the page.

### Interactive blocks

These pose a question; the user's answer is recorded in `responses.json`. Each
needs a unique `id` within the report.

- **`ask`** — `{id, prompt, mode: "choice"|"yesno"|"text", options[]}` (options
  required for `choice`).
- **`decision`** — `{id, prompt, a, b}` — same A/B shape as `compare`; records
  the chosen side's `tag`.
- **`approval`** — `{id, prompt}` — records `approved` or `changes-requested`.

## Versioning

The `schema` field follows the `<family>@<N>` format — currently
`harness-deck/report@1`. The family is `harness-deck/report`; the version is a
positive integer.

**Lenient parse / strict validate split:**

- `Parse` accepts any version of the canonical family. A manifest written by a
  newer harness (e.g. `harness-deck/report@2`) will still load: known blocks
  decode normally; unknown block types degrade to visible fallback panels. A
  different family (e.g. `other-tool/report@1`) is rejected immediately.
- `Validate` (run by `harness-deck validate`) only accepts the exact set of
  versions this binary knows. A newer version produces an error that says the
  schema is "newer than this binary supports — upgrade harness-deck".

**For publishers:** always write the exact schema string shown in the top-level
fields example above. Only upgrade `@N` when you intentionally target a newer
harness-deck release that documents the new version.

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
