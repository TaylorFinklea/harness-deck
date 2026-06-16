# Search query language — report

Shipped 2026-06-15. Spec: `search-query-language-spec.md`. ADR: decisions.md
"Search query language". Commits: `fa88dce` (query pkg), `bcf4e97` (server),
`6c25e3c` (client), + docs.

## What shipped

Cmd+K search upgraded from plain full-text to a JQL-like query language with
as-you-type autocomplete. Examples that now work:

- `status = awaiting-review` (faceted browse — no text needed)
- `project IN (demo) AND kind = audit`
- `kind = audit OR kind = progress`
- `created >= -7d NOT kind = roadmap`
- `auth project = demo` (free text AND structural)

## Layers

- **`internal/query`** (new, pure, stdlib-only) — lexer (`lex.go`), recursive-
  descent parser (`parse.go`), AST + public API + field matrix (`query.go`),
  evaluator (`eval.go`). `Parse(q) (Query, error)`, `Query.Match(rec, now)`
  (lazy short-circuit against a `Record` interface), `HasText`/`TextTerms`,
  `Schema()` (single source of truth for autocomplete). 1241 lines incl. tests.
- **server** — `handleSearch` parses → pre-filters entries (cheap index fields)
  → opens report bodies lazily+memoized only for survivors that reach a text
  leaf → scores survivors for ordering (snippet preserved) → newest-first for
  purely-structural. New `GET /api/search/schema` (derives fields from
  `query.Schema()`, distinct index values, status enum, created hints).
- **client** (`search.js` + `deck.css`) — schema fetch on open, caret-position
  tokenizer → field/operator/value suggestions, dropdown (HDDom.el, no
  innerHTML), Tab-accept / Enter-opens, lenient parse-error hint. CSS in
  deck.css so it styles on dashboard + report pages.

## Process

Phased multi-agent workflow (`build-search-query-language`): TDD-build query pkg
→ 3 parallel adversarial verifiers (precedence/boolean, eval+lazy+created,
lexing/values) + repair loop → server integration → client → parallel
code-review + health gate. 12 agents, ~932k tokens, ~38 min. 1 real bug found
and fixed in verify round 1 (0 in round 2). Review verdict APPROVE — 4 findings,
all non-defects (incl. the correct deck.css-vs-aggregator.css deviation).

## Lead post-work (not trusted on say-so)

- Closed the field-matrix duplication: added `query.Schema()` + `TestSchema`,
  rewired the server to derive from it (was a static duplicate → silent drift
  risk).
- Independently re-ran build / `go test ./...` / `gofmt -l` / `go vet` /
  `node --check`.
- Full browser pass (chrome-devtools MCP) on dashboard **and** a report page:
  field/operator/value autocomplete, prefix-filter, Tab-accept, parse-error
  hint (keeps results), Enter-opens, structural + `IN`/`OR`/`NOT`/`created`
  queries via the API. Dropdown styled on both surfaces. Zero console errors.

## Not done (parked)

- **Saved searches** — pin a query to the sidebar. Needs a design call
  (dashboard-only vs everywhere + storage key). Cheap to build on this now.
- `~`/`!~` for `created`, `+N` future offsets, ORDER BY — out of scope by
  design.
