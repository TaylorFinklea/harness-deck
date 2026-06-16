# Search query language (JQL-like filters + autocomplete) — spec

Status: design approved 2026-06-15 (brainstorming). Implementation: phased
workflow. Promotes the "Search filters" Later item to a Now milestone.

## Goal

Upgrade the Cmd+K search palette from plain full-text to a **JQL-like query
language** with **as-you-type autocomplete** of valid fields/operators/values.
A query mixes structural filters with free text:

```
status = awaiting-review
auth project IN (harness-deck, deck) AND kind = audit
created >= -7d NOT kind = roadmap
```

Plain text with no operators behaves exactly as today (no regression).

## Non-goals (v1)

- Saved searches (separate Later item).
- `ORDER BY`, `IS EMPTY`, `WAS`, function values (`now()`), history operators.
- Aliases / fuzzy value matching (decided: exact match, no aliases).
- Client-side filtering (decided: server is the source of truth).

## Grammar

Faithful JQL subset. Lexer → recursive-descent parser → predicate AST.

```
query      := or_expr
or_expr    := and_expr ( OR and_expr )*
and_expr   := unary ( AND? unary )*       # AND optional → juxtaposition = implicit AND
unary      := NOT unary | primary
primary    := '(' or_expr ')' | clause | text_term
clause     := FIELD op value_or_list      # only when FIELD is known AND an op follows
op         := '=' | '!=' | '~' | '!~' | '>' | '>=' | '<' | '<=' | 'IN' | 'NOT' 'IN'
value_or_list := value | '(' value (',' value)* ')'
value      := BARE | QUOTED
text_term  := BARE | QUOTED               # any primary not recognized as a clause
```

- Keywords `AND OR NOT IN` are case-insensitive; so are field names.
- Precedence: **NOT > AND > OR**. Implicit AND (juxtaposition) has the same
  precedence as explicit `AND`.
- **Field-vs-text disambiguation (lookahead):** a known field name is the start
  of a `clause` only if a valid operator follows it; otherwise the token is a
  `text_term`. So `status` alone → full-text search for "status"; `status =` →
  clause.
- **`NOT` is positional:** in operator position (right after a field, before
  `IN`) it forms the `NOT IN` operator (`project NOT IN (a,b)`); in unary
  position (start of a primary) it is boolean negation (`NOT kind = audit`).
- `BARE` = run of non-space, non-paren, non-comma chars that isn't a keyword.
  `QUOTED` = double-quoted, allows spaces; `\"` escapes a quote.

### Fields & operator validity

| Field | source (`store.Entry`) | allowed operators |
|---|---|---|
| `status` | `Status` (enum: draft, awaiting-review, answered, done) | `= != IN NOT IN` |
| `project` | `Project` | `= != ~ !~ IN NOT IN` |
| `kind` | `Kind` (progress/audit/review/decision/idea/roadmap, free-form) | `= != ~ !~ IN NOT IN` |
| `harness` | `Harness` | `= != ~ !~ IN NOT IN` |
| `title` | `Title` | `~ !~ = !=` |
| `agent` | `Agent` | `= != ~ !~ IN NOT IN` |
| `verdict` | `Verdict` | `= != ~ !~ IN NOT IN` |
| `created` | `Created` (ISO-8601 string) | `> >= < <=` |

A field used with a disallowed operator is a **parse error**.

### Value semantics (exact, no aliases)

- `=` : `strings.EqualFold(field, value)`
- `!=` : `!EqualFold`
- `~` : `strings.Contains(lower(field), lower(value))`
- `!~` : `!Contains`
- `IN` / `NOT IN` : EqualFold against any / none in the list
- `created` comparisons: parse `Created` as time; compare to the resolved
  threshold. Value is either **relative** `-<N>[h|d|w]` (e.g. `-7d`, `-24h`,
  `-2w`) resolved against `now`, or an **ISO date** `YYYY-MM-DD` (treated as
  local midnight). `created >= -7d` means "created in the last 7 days".

### Text terms

A `text_term` leaf matches if the report's searchable text contains it
(case-insensitive). "Searchable text" = the same surfaces today's search scores:
metadata fields (title/project/kind/status/harness/agent/verdict) **and** body
text (`manifest.BlockText`). A quoted term matches the phrase verbatim.

## Package: `internal/query` (new, stdlib-only, pure)

No dependency on `store`/`server` — evaluates against an interface, so it is
unit-testable with fake records.

```go
// Parse compiles a query string into an evaluable Query. Empty/whitespace
// input is an error (callers special-case "" before calling).
func Parse(q string) (Query, error)

// Record is the per-report view the evaluator sees. Field is cheap (index);
// Text is the body, fetched lazily and memoized by the caller.
type Record interface {
    Field(name string) string // "status","project","kind","harness","title","agent","verdict","created"
    Text() string             // metadata+body searchable text (lazy)
}

// Match reports whether rec satisfies the query. now resolves relative
// created thresholds. Evaluation is SHORT-CIRCUIT: Text() is only called
// when a text leaf is actually reached, so a structural predicate that
// fails first never opens the report.
func (q Query) Match(rec Record, now time.Time) bool

// HasText reports whether the query contains any text_term leaf (drives
// whether the server scores/snippets or orders purely by recency).
func (q Query) HasText() bool

// TextTerms returns the text leaves, for snippet capture by the server.
func (q Query) TextTerms() []string
```

AST node types (unexported): `andNode`, `orNode`, `notNode`, `fieldPred{field,
op, values}`, `textPred{term}`. `Parse` returns a typed error with a short,
human message (`expected a value after '='`, `unknown field "stauts"`,
`unbalanced parentheses`) for the live-typing hint.

**Error model:** invalid/incomplete queries return an error; partially-typed
queries (the common live case) are errors too — the server surfaces the message
and the client keeps last-good results (see Client).

## Server changes (`internal/server`)

### `handleSearch` refactor (`search.go`)

1. `q == ""` → `{"matches":[]}` (unchanged).
2. `parsed, err := query.Parse(q)`. On error → `200` with
   `{"matches":[], "error": err.Error()}`.
3. `now := time.Now()`. For each non-archived `store.Entry`:
   - build a `record` bridging the entry + a **memoized** body-text closure
     (`Text()` calls `s.store.Get` + `manifest.BlockText` at most once).
   - if `parsed.Match(rec, now)` → it's a hit.
4. Scoring/snippets **preserved** for ordering: if `parsed.HasText()`, reuse the
   existing body-vs-metadata scoring + `snippet()` against the query's
   `TextTerms()` (first body match drives the snippet). If no text terms, all
   survivors score equal → ordered newest-first (`Created` desc). Cap
   `searchMaxResults` (20).

`scoreEntry`/`snippet` are refactored to serve survivors (scoring/snippet),
**not** to be the filter. `searchHit` JSON shape is unchanged.

### New `GET /api/search/schema` (autocomplete vocabulary)

Response (computed from `s.store.Entries()`, archived excluded):

```json
{
  "fields": [
    {"name":"status","ops":["=","!=","IN","NOT IN"]},
    {"name":"project","ops":["=","!=","~","!~","IN","NOT IN"]},
    ...
    {"name":"created","ops":[">",">=","<","<="]}
  ],
  "values": {
    "status":  ["draft","awaiting-review","answered","done"],
    "project": ["harness-deck","demo", ...],   // distinct, sorted
    "kind":    ["audit","progress", ...],
    "harness": ["claude-code","codex", ...]
  },
  "created_hints": ["-24h","-7d","-2w","YYYY-MM-DD"]
}
```

`status` values are the static enum (stable order). `project/kind/harness`
values are distinct non-empty values present in the index, sorted. Cheap;
recompute per request (entries are in memory). Registered next to
`GET /api/search` in `server.go`.

## Client changes (`internal/assets/search.js`)

Builds on the existing palette (HDDom.el, no innerHTML). The **raw query string
is still sent to `/api/search` unchanged** — the client parser exists only to
drive autocomplete; the server parser is authoritative.

1. **Schema fetch:** on palette open, `GET /api/search/schema` once per session
   (memoized); tolerate failure (autocomplete simply goes quiet).
2. **Suggestion engine:** tokenize the input up to the caret, classify caret
   position → suggest:
   - field position (start, or after `AND`/`OR`/`NOT`/`(`): field names.
   - after a known field: that field's operators.
   - after an operator (or inside `IN ( … )`): that field's values (for
     `created`: the `created_hints`).
   - bare text otherwise: no suggestions (it's a text term).
   Render a dropdown under the input (HDDom.el, `.search-suggest` list).
3. **Accept keys:** **Tab** (or click) inserts the highlighted suggestion at the
   caret; **↑/↓** move the active suggestion; **Esc** closes the dropdown first
   (second Esc closes the palette). **Enter** still opens the active *result*
   (unchanged) — never hijacked by the dropdown.
4. **Parse-error UX:** the search response may carry `error`. On error, **keep
   the currently-rendered results** (no clear/flash), show the message in the
   `.search-status` area as a non-alarming hint. On a successful (possibly
   empty) `matches`, replace results as today.

CSS: add `.search-suggest` + `.search-suggest-item.active` rules to
`aggregator.css` (palette is dashboard + report page; styles already shared via
the bundles). No new bundle members; `search.js` stays one file.

## Testing

- **`internal/query` (table-driven):**
  - parse: valid queries → expected AST (stringified); invalid → expected error
    substrings (precedence, quoting, lists, created relative/ISO, unknown field,
    bad operator-for-field, unbalanced parens, implicit vs explicit AND).
  - eval: predicate trees vs fake `Record`s for every operator + boolean shape;
    `created` relative/ISO vs a fixed `now`.
  - **lazy short-circuit invariant:** a `Record` whose `Text()` increments a
    counter (or `t.Fatal`s) proves `Text()` is NOT called when a structural
    predicate short-circuits (`status = x AND <text>` with non-matching status).
- **`internal/server`:** `handleSearch` for structural-only (`status = …` with no
  text returns the right set), mixed (`auth status = …`), parse error → `error`
  field + 200, regression (plain text unchanged); `GET /api/search/schema`
  returns the enum + distinct values from a seeded store.
- **`internal/assets`:** existing `TestBundlesAreValidJS` + `node --check` guard
  search.js syntax; bundle order unchanged.
- **Browser (chrome-devtools MCP, end):** type `status = ` → dropdown lists the
  4 statuses; Tab accepts; `status = awaiting-review` returns the right reports
  with no text; `created >= -7d` filters; a mid-type partial keeps last results
  + shows a hint; Enter opens; zero console errors. On both dashboard + a report
  page.

## Constraints (inherited)

- Zero external Go deps (stdlib only). Frontend: no build step, no innerHTML
  (HDDom.el / textContent), vanilla JS.
- The renderer owns all HTML/CSS; this adds palette UI only.
- Keep `internal/query` pure (interface-only deps) for isolation + testability.

## File-by-file

- `internal/query/query.go` (+ `lex.go`/`parse.go`/`eval.go` if it reads
  cleaner) — new package.
- `internal/query/query_test.go` — parse + eval + lazy-eval tests.
- `internal/server/search.go` — `handleSearch` refactor; record bridge; reuse
  `scoreEntry`/`snippet` for survivors.
- `internal/server/schema.go` (or in `search.go`) — `handleSearchSchema`.
- `internal/server/server.go` — register `GET /api/search/schema`.
- `internal/server/server_test.go` — handler + schema tests.
- `internal/assets/search.js` — schema fetch, suggestion engine, dropdown,
  Tab-accept, parse-error hint.
- `internal/assets/aggregator.css` — `.search-suggest*` styles.
- `CONTRACT.md` — only if a user-facing surface changed (search is internal
  tooling; likely no change — confirm).
