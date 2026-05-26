# Publishing to harness-deck

A walkthrough for anyone — human or agent — who wants their project to show up
in the local harness-deck dashboard. By the end you'll have a `report.json`
that renders, lives under a project the user has registered, and can ask the
user a question whose answer comes back as a file.

The full block vocabulary lives in [`CONTRACT.md`](../CONTRACT.md). This guide
is the on-ramp.

## What you're producing

A **report** is a single JSON file (`report.json`) that describes one run of
your tool. harness-deck:

1. discovers the file under either the central reports directory or the
   project's `.harness/`,
2. renders it into a consistent themed HTML page,
3. lists it on the dashboard (with an open-asks counter if your report
   contains interactive blocks),
4. writes the user's answers to `responses.json` next to your report.

You only own step 1. Everything else is the renderer's job — your job is to
emit content, never layout. There is no HTML in a manifest (except the
deliberate `html` escape-hatch block).

## 60-second smoke test

```sh
# 1. Install harness-deck if you don't already have it.
brew install taylorfinklea/tap/harness-deck

# 2. From inside your project's root:
harness-deck register .

# 3. Write a starter manifest. --in-repo puts it in `.harness/<id>/` next to
#    your code, so it ships with the repo rather than landing in the central
#    reports dir.
harness-deck new --in-repo --title "hello from <your-tool>"

# 4. Validate it before publishing — strict schema check.
harness-deck validate .harness/<id>/report.json

# 5. Look at it.
harness-deck serve   # then open http://127.0.0.1:7420
```

You should see your project appear in the sidebar and a single report under
it. Edit the JSON file and the dashboard live-reloads (~2s).

## Where to write the file

You choose one of two locations, whichever fits your run model:

| Location | Path | When to use |
|---|---|---|
| **Per-project** | `<project-root>/.harness/<run>/report.json` | Reports that belong to a repo, are short-lived, and you want auto-discovered along with the project. |
| **Central** | `~/.harness/reports/<project>/<run>/report.json` | Cross-repo work, ad-hoc runs, or runs that outlive any one checkout. |

`<run>` is any stable id you want — a timestamp (`20260526-104210`), a hash,
or your harness's session id. harness-deck never invents one for you; the
directory is yours and may also contain artifacts (logs, diffs,
intermediate files).

A project shows up in the dashboard once it's been registered with
`harness-deck register <path>` **and** it has a `.docs/ai/` directory.
(The `.docs/ai/` requirement is intentional — it's how harness-deck
distinguishes a real project from any other directory under `scan_roots`.)

## The minimum viable manifest

This is the smallest legal report. Save it as `report.json`:

```json
{
  "schema":  "harness-deck/report@1",
  "id":      "20260526-104210",
  "project": "your-project",
  "harness": "your-tool",
  "title":   "First run",
  "status":  "awaiting-review",
  "created": "2026-05-26T10:42:10Z",
  "blocks": [
    {
      "type": "prose",
      "title": "what happened",
      "markdown": "Ran the **initial pass**. Found 3 things worth surfacing.\n\n- one\n- two\n- three"
    }
  ]
}
```

That's it. The renderer will give you a fully themed page.

Run `harness-deck validate report.json` after every write — the validator is
strict (unknown fields are errors; enum values are checked) so it catches
typos that the lenient parser would silently drop.

Atomic writes are recommended (write to a temp file, then `rename(2)`):
harness-deck polls every 2s and may catch a partial write otherwise. Every
tool inside harness-deck does this already; do the same.

## The most useful blocks (in order)

You can ship 90% of useful reports with just these four block types.
The rest of the vocabulary is in [`CONTRACT.md`](../CONTRACT.md).

### `prose` — Markdown panel

Drop-in for any narrative section. Supports headings, lists, **bold**,
*italic*, `inline code`, fenced code blocks (rendered with a copy button),
tables (`| h | h |` / `| - | - |` / rows), `> ` blockquotes, GitHub task
lists (`- [x]` / `- [ ]`), `---` horizontal rules, and inline links
(`[text](url)` or `<https://…>`).

```json
{
  "type": "prose",
  "title": "summary",
  "markdown": "Hot path is **84% covered**.\n\n```bash\nrun tests\n```"
}
```

### `recommendations` — numbered next-steps

When you want the user to see "do these N things next."

```json
{
  "type": "recommendations",
  "items": [
    {"id": "r1", "owner": "platform",  "markdown": "Reindex `users.email` before cutover."},
    {"id": "r2", "owner": "data",      "markdown": "Backfill `events.tags` opclass on a replica first."}
  ]
}
```

### `callout` — info / warn / error aside

For one-line "heads up" content.

```json
{
  "type": "callout",
  "level": "warn",
  "tag":   "rollback",
  "markdown": "WAL diverges after the writer flip. **Decide before** you run the cutover."
}
```

### `ask` — pose a question

This is what makes the report interactive. The user's answer comes back to
you in `responses.json` (see below).

```json
{
  "type": "ask",
  "id":   "ship-decision",
  "prompt": "Ship now, or wait until after the freeze?",
  "mode":   "choice",
  "options": ["ship", "wait"]
}
```

`mode` is one of:

- `choice` — user picks from `options[]`.
- `yesno` — user picks yes or no. No `options` needed.
- `text` — free-form text response.

The two other interactive blocks (`decision`, `approval`) are documented
in `CONTRACT.md` — same response shape, different UI affordance.

## Reading the user's answer

When the user answers, harness-deck writes a sibling file to your report:

```jsonc
// <run-dir>/responses.json
{
  "run": "20260526-104210",
  "project": "your-project",
  "updated": "2026-05-26T10:47:01Z",
  "responses": {
    "ship-decision": {
      "block": "ship-decision",
      "value": "ship",
      "note":  "",
      "at":    "2026-05-26T10:47:01Z"
    }
  }
}
```

The `responses` map is keyed by your interactive block's `id`. Read it at
the start of your next run (or as a session-start hook) to pick up answers
the user gave between invocations.

If `responses.json` doesn't exist, the user hasn't answered yet — no error,
just an empty answer set.

## Driving the lifecycle from the harness

The `status` field changes the report's accent in the inbox. You're free to
set it however your harness models a run; a typical progression is:

- `draft` — written but not ready for the user yet.
- `awaiting-review` — questions are live, the user should look.
- `answered` — user has responded, harness has not yet acted on it.
- `done` — terminal state.

You can also opt into a `kind`: most are just labels (`audit`, `progress`,
`idea`, …), but `kind: "roadmap"` lands the report in the **projects** view
alongside that project's `.docs/ai/roadmap.md`.

## Optional but recommended

- **Validate in CI** — `harness-deck validate path/to/report.json` is exit-0
  on success. Hook it into whatever guards your harness's outputs.
- **Use absolute, RFC 3339 `created` timestamps** — relative or vague ones
  break ordering in the inbox.
- **Reach for an existing block before `html`** — the `html` escape hatch
  exists so you're never blocked, but a real typed block survives renderer
  upgrades. If you find yourself reaching for `html` repeatedly, that's a
  signal a new block type should be promoted.
- **Keep one report per run.** Updating a manifest in place is fine; the
  store fingerprints content + mtime and re-renders. Multiple runs go in
  separate `<run>` directories.

## A complete worked example

For a non-trivial report that exercises ~10 block types, see
[`samples/postgres-audit.report.json`](../samples/postgres-audit.report.json).
That file is the renderer's reference fixture and is kept current with
the manifest schema.

## Getting unstuck

- `harness-deck validate <file>` — strict schema check.
- `harness-deck render <file> -o out.html` — render without the dashboard.
- `harness-deck serve` then watch `/api/reports` — the live index.
- `~/.config/harness-deck/config.json` — bind, TLS, scan roots, optional
  `notify_command` that fires whenever the user records an answer.

Found a rough edge? File it at
<https://github.com/TaylorFinklea/harness-deck/issues>.
