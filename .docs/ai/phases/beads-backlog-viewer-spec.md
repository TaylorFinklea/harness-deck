# Beads backlog viewer — Phase 1 spec

Status: **design — awaiting review**. Branch: `feat/beads-backlog-viewer` (off `main`).
Bead: `harness-deck-5ph.1` (parent epic `harness-deck-5ph`; Phase 2 = `5ph.2`, blocked-by 5ph.1).
Decisions captured in `.harness/reports/harness-deck/20260630-beads-backlog-mockups`
(graph = **A, inline SVG DAG**; branch = **off main**).

## Goal

Add a **Backlog** view to the harness-deck dashboard that reads beads (`bd`) issues
from every `.beads/`-enabled repo under `scan_roots` and lets the user **visualize**
the priority-sorted ready queue + dependency graph and **drill into** individual
issues — the cockpit for the 5-repo beads pilot (tesela, simmersmith, harness-deck,
tga, portfolio-new). **Phase 1 is read-only.** Actions (claim/close/create) are
Phase 2 (`5ph.2`) and out of scope here.

## Non-negotiable constraints (inherited)

- **Stdlib-only.** No `go get`; `go.mod` stays with no `require` block. No npm/bundler;
  the graph is hand-rolled inline SVG built in vanilla JS.
- **Never read `.beads/` files.** It is a binary Dolt DB. The ONLY read path is the
  `bd` CLI with `--json`. Every call is `bd -C <root> … --json`.
- **Graceful degradation is a design rule.** `bd` absent, a repo with no `.beads/`,
  a `bd` call that errors → empty state / per-repo error, **never** a crashed page.
- **Discover by `.beads/`, not `.docs/ai`.** tga + portfolio-new have `.beads/` and
  **no** `.docs/ai`, so they are absent from `projects` discovery. Beads discovery is
  independent and keys on `.beads/`.
- Resolve `bd` via PATH with the launchd/systemd fallback dirs (it lives at
  `/opt/homebrew/bin/bd`), mirroring `usage.opencodeBin`.
- Every `bd` subprocess gets `cmd.Stdin = nil` (child `/dev/null`) — the TUI-hang guard,
  same as `usage`/`herdr`.

## Architecture — a live server-side view (NOT a report block)

Beads data is live and external, so it mirrors the **`usage` / `agents`** shape — a
server-side adapter + cached snapshot + `/api/…` endpoint + a dashboard view — **not**
a `report.json` block. (The block registry has no precedent for server-computed blocks;
confirmed during exploration.) So **no new manifest block type, no `TestRegistryCrossCheck`
involvement.**

```
bd CLI ──(shell, --json)──▶ internal/beads (adapter + parse)
                                   │
              internal/server: beads.Monitor (own goroutine, RWMutex snapshot,
              refresh every RefreshSec; fingerprint→OnChange callback)
                                   │
            GET /api/beads  ◀── cached Snapshot (all repos)
            GET /api/beads/{project}/{id} ◀── on-demand drill-in detail
                                   │
            OnChange ─▶ hub.broadcastEvent("beads","changed")  ─SSE▶ frontend
                                   │
            aggregator.js: 4th view "backlog" (g b / :backlog), refreshBeads(),
            client-side SVG DAG, j/k cursor, Enter drill-in panel
```

### Reference patterns to mirror (read these on `main` before coding — do not trust line numbers)

- **`internal/usage/usage.go`** — `Monitor` (own goroutine, `mu sync.RWMutex`, `Start(ctx)`,
  `Samples()`, nil-safe), `Options`/`Build`, per-sample timeout. The beads `Monitor` mirrors it.
- **`internal/usage/opencode.go`** — `opencodeBin()` PATH+fallback resolution; `exec.CommandContext`
  with nil stdin; `parseOpenCodeStats` tested on fixtures with **no** CLI call. The beads adapter
  + parse + tests mirror this exactly.
- **`internal/projects/projects.go`** — `discover(scanRoots, explicit)` (keys on `.docs/ai`;
  parallel it for `.beads/`), dedupe, `Fingerprint()` (FNV over mtimes) for change detection.
- **`internal/server/server.go`** — the `mux.HandleFunc` route block in `New()`; how `s.usage` is
  built (gated), `Start`ed, and served by `handleUsage`; `go s.watch(pollInterval)`.
- **`internal/server/sse.go`** — `hub.broadcastEvent(event, data)`; `tick()`/`watch()`.
- **`internal/assets/aggregator.js`** — `VIEWS`, `BUILDERS`, `renderContent`/`showView`, the
  inbox **cursor** model (`visibleRows`/`moveCursor`/`openFocused`), `HDKeys.chord`,
  `registerCommands`, `connectEvents`. **`g b` is unused on main.**
- **`internal/assets/hd-dom.js`** — `HDDom.el()` DOM idiom (no innerHTML). SVG needs a sibling
  `svgEl()` (createElementNS) — `el()` can't make SVG nodes.
- **`internal/herdr/`** (on the herdr branch, not in this tree — read via
  `git show feat/herdr-mobile-inbox:internal/herdr/herdr.go`) — the `flagLike` argv-guard and the
  fake-binary/interface test seam are worth mirroring for the drill-in `{id}` handler and tests.

## Data model (Go structs are the schema)

`internal/beads/beads.go`. Field set verified against real `bd … --json` output; the implementer
re-confirms JSON tags against live `bd` before finalizing.

```
type Issue struct {
    ID, Title, Description string
    Status, IssueType, Owner string
    Priority int            // 0..4, 0=highest
    Labels []string
    DependencyCount, DependentCount, CommentCount int
    Created, Updated string // RFC3339
    BlockedBy []string      // populated from `blocked --json` (blocked_by[])
    Parent string           // hierarchical parent id if present
}
type Edge struct { From, To, Kind string } // Kind: "blocks" | "parent"
type Counts struct { Open, Ready, Blocked, InProgress, Total int }
type RepoSnapshot struct {
    Name, Root string       // Name = filepath.Base(Root); groups in the UI / {project}
    Ready, Blocked, All []Issue
    Edges []Edge
    Counts Counts
    Err string              // bd failed for THIS repo; others still render
}
type Snapshot struct { Repos []RepoSnapshot; Updated string; Available bool }
```

`Available=false` ⇒ `bd` binary not found / feature disabled (frontend shows empty state).

### Edge acquisition (the one fiddly CLI-derived detail — verified, but re-check field names)

Build the graph from **structured JSON**, not by parsing mermaid:

- **blocks edges:** from `bd -C <root> blocked --json`, each issue `B` with `blocked_by=[A,…]`
  ⇒ `Edge{From:A, To:B, Kind:"blocks"}`. (Captures all *open* blocking relationships, which is
  exactly what the graph should show.)
- **parent edges:** from `bd -C <root> list --json` (all open issues), each issue with a
  `parent` ⇒ `Edge{From:parent, To:issue, Kind:"parent"}`.
- **nodes:** union of `ready ∪ blocked ∪ all` issue ids referenced by edges. `list --json`
  supplies node attributes (title/priority/status) for any blocker that is itself neither ready
  nor blocked (e.g. an in-progress blocker).

Per-repo `bd` calls per refresh: `ready`, `blocked`, `list`, `status` (counts) = **4 reads**.
(`bd dep tree … --format=mermaid` is available and is the fallback if structured parent/blocked_by
turns out insufficient for a richer graph — but Phase 1 uses the structured path.)

Drill-in detail (`/api/beads/{project}/{id}`) shells on demand: `show <id> --json`,
`dep list <id>` (blockers), `dep tree <id> --direction=up` (dependents), `comments <id>`.
`bd show --json` omits edges/comments — they must be fetched separately (landmine).

## Adapter — `internal/beads/`

- `New() (*Client, bool)` — `resolveBin("bd", fallbacks)`; `ok=false` ⇒ feature stays dark.
- Methods, each shelling `bd -C <root> … --json`, ctx-timeout, nil stdin, lenient unmarshal
  (unknown fields ignored for forward-compat): `Ready`, `Blocked`, `List`, `Status`, `Show`,
  `DepList`, `DepTree(dir)`, `Comments`.
- `Discover(scanRoots, explicit []string) []Repo` — depth-1 children of scan roots holding a
  `.beads/` dir, plus explicit roots; dedupe by abs path. Independent of `projects` toggles in
  Phase 1 (every `.beads/` repo shows).
- `flagLike(s)` guard reused for the `{id}` path param (reject leading `-`; constrain to the bd
  id charset `[A-Za-z0-9._-]`) so a hostile id can't smuggle a `bd` flag.
- Parsing lives in `parse.go` and is unit-tested on captured JSON fixtures with **no** subprocess.

## Server wiring — `internal/server/`

- `config.BeadsConfig{ Enabled bool; RefreshSec int }`, added to `Config` as
  `Beads BeadsConfig json:"beads,omitempty"`, modeled on `UsageConfig`. Default **disabled**
  (opt-in, like usage/agents). `RefreshSec` default **15** (4 reads × 5 repos every 15s is light;
  2s like the report watcher would be wasteful).
- `beads.Monitor` lives in **`internal/beads`** (mirrors `usage.Monitor`; never imports
  `server`/`hub`): holds the client + discovered repos, refreshes every `RefreshSec`, caches
  `Snapshot` under RWMutex, computes an FNV fingerprint (ids+status+updated across repos), and on
  change invokes an injected `OnChange func()`. Repos are re-discovered each tick (cheap dir stat)
  so a newly-`bd init`'d repo appears live. Handlers + the injectable interface + fake live in
  `internal/server/beads.go`.
- In `New()`: if `cfg.Beads.Enabled` and `beads.New()` ok, construct the Monitor with
  `OnChange = func(){ s.hub.broadcastEvent("beads","changed") }`; else leave nil (dark).
  `Start(ctx)` it in `Serve()` next to `s.usage.Start`.
- Routes (added to the `mux.HandleFunc` block):
  - `GET /api/beads` → cached `Snapshot` JSON (nil-safe: `{repos:[],available:false}` when off).
  - `GET /api/beads/{project}/{id}` → on-demand detail JSON
    `{issue, blockers[], dependents[], comments[]}`; 404 unknown project/id, 400 bad id,
    503 when disabled. Maps `{project}`→root via the Monitor's discovered repos.
- A small server→JS feature flag (`beadsEnabled`) injected the same way the shell passes other
  server state to the page, so the frontend only registers the Backlog view when enabled.
  (Find the existing mechanism in `shell.html.tmpl`/`handleShell`; mirror it. If none fits, the
  fallback is: always register the view, show an "enable beads.enabled" empty state when
  `available=false`.)

## Frontend — the Backlog view

Place the graph + detail rendering in a new module `aggregator-backlog.js` (`window.HDBacklog`,
bundled like the other `aggregator-*` modules in `assets.go`); keep view *registration* in core
`aggregator.js`, following the existing module seams. Mirror the inbox cursor model.

- Register: add `{id:'backlog', label:'backlog'}` to `VIEWS`; `backlog: viewBacklog` to `BUILDERS`;
  `HDKeys.chord('g','b', …showView('backlog'))`; `:backlog` in `registerCommands`.
- Data: `var beadsData = {repos:[],available:false}`; `refreshBeads()` fetches `/api/beads`
  (`cache:'no-store'`, tolerant `.catch`); call on view-switch + once at startup; add
  `es.addEventListener('beads', refreshBeads)` in `connectEvents` (live SSE refresh).
- Layout per repo: header strip (name · ready/blocked/open counts) → two columns
  **READY** (priority-sorted) / **BLOCKED** (each with its blocked-by) → the **inline SVG
  dependency graph** panel (collapsible). Build rows with `HDDom.el` (no innerHTML); build SVG
  with a new `svgEl()` (createElementNS) and `textContent` labels (no injection).
- **SVG graph (Option A, matches the approved mockup):** layered DAG.
  Layer assignment = longest-path: `layer(n)=0` if no in-edges within the repo subgraph, else
  `max(layer(parents))+1`; cycle-guard with a visited set + depth cap (deps are a DAG, but never
  trust input). Position `x=layer*COL`, `y=indexInLayer*ROW`; nodes = rounded `<rect>`+`<text>`
  (id bold + P# pill + truncated title), edges = `<line>`/`<path>` with an arrowhead `<marker>`,
  labeled `blocks`/`parent`. Color by status (ready=green, blocked=red, epic=purple,
  in-progress=yellow) + priority pill, using the `--tn-*` theme vars. Empty graph (no edges) ⇒
  hide the panel, show "no dependencies".
- **Keyboard:** `j`/`k` move a cursor over the visible ready+blocked rows (mirror inbox
  `visibleRows`/`moveCursor`); `Enter`/`o` open the focused issue's **detail panel** (fetch
  `/api/beads/{project}/{id}` → description, blocked-by, blocks/dependents, comments, a focused
  mini dep-graph); `Esc` closes it. No new page route — the detail is an in-view panel.
- CSS: a `/* backlog */` section in `aggregator.css` reusing `.panel`/`.metric-strip` tokens;
  mobile rules in `mobile.css` (graph collapses by default on ≤720px).

## Graceful degradation matrix

| Condition | Behavior |
|---|---|
| `bd` not installed | Monitor nil; `/api/beads`→`{available:false}`; view shows "bd not found" empty state |
| `beads.enabled` false | feature dark; Backlog view not registered (or empty state via fallback) |
| repo `bd` call errors | that `RepoSnapshot.Err` set; **other repos still render**; page never fails |
| repo has no open issues | repo card shows "no open issues"; still listed |
| no `.beads/` repos found | view shows "no beads repos under scan_roots" |
| drill-in id unknown / bad | 404 / 400; the rest of the view is unaffected |

## Testing (mirror `usage` + `herdr`; stdlib `testing` only)

- `internal/beads/parse_test.go` — `parseReady/Blocked/List/Status/DepList/Comments` on captured
  JSON/text fixtures (real `bd` output samples), **no subprocess**. Include an empty + a malformed
  fixture (lenient degrade).
- `internal/beads/beads_test.go` — `resolveBin` fallback probing; `flagLike` guard; `Discover`
  against temp dirs (some with `.beads/`, some with `.docs/ai` only, some with neither) — assert
  tga/portfolio-new-style (`.beads/`, no `.docs/ai`) are found and `.docs/ai`-only are not.
- `internal/beads/monitor_test.go` — a fake client injected into `Monitor`: snapshot caches,
  fingerprint changes only on real data change, `OnChange` fires exactly on change.
- `internal/server/beads_test.go` — handlers against an injected fake snapshot/Monitor (no real
  `bd`): `/api/beads` JSON shape; `/api/beads/{project}/{id}` happy path + 400 (bad id) +
  404 (unknown) + 503 (disabled).
- Frontend: no JS test harness exists; verify in-browser (chrome-devtools MCP / live dashboard).

## Acceptance (Phase 1)

- Backlog view lists each `.beads/` repo with ready/blocked/all issues (id, title, priority, type,
  labels, blocked-by + blocks counts), a per-repo inline-SVG dependency graph, and drill-in to one
  issue (description, blocked-by, blocks, dependents, comments).
- Keyboard nav matches the dashboard (j/k cursor, Enter drill-in, Esc, `g b`/`:backlog`); the view
  live-refreshes via SSE.
- tga + portfolio-new (`.beads/`, no `.docs/ai`) appear; `bd` absent → graceful empty state, never
  a crash; a per-repo `bd` failure isolates to that repo.
- The harness-deck chain `i8t→eoz→7ne` renders as a graph and the blocked items read as blocked;
  the `5ph` epic renders its parent-child chain.
- `go build ./...` and `go test ./...` pass; `gofmt -l .` empty; `go.mod` still has **no `require`**.

## Verify

```sh
cd ~/git/harness-deck
go build ./... && go test ./...
gofmt -l .                 # expect empty
# enable in a test config, run, browser-verify the Backlog view across the 5 repos:
HARNESS_DECK_CONFIG=/tmp/hd-beads/config.json ./harness-deck serve   # beads.enabled:true
```

## Suggested task sequence (for the implementation plan)

1. `internal/beads` adapter + parse + tests (fixtures only). **Verify:** `go test ./internal/beads`.
2. `config.BeadsConfig`; `internal/server/beads.go` Monitor + snapshot + `/api/beads` +
   `/api/beads/{project}/{id}` + wiring + server tests (fake client). **Verify:** `go test ./internal/server`.
3. Frontend: `aggregator-backlog.js` + core registration + CSS + SSE listener + bundle/embed.
   **Verify:** browser — view renders across 5 repos, graph draws, drill-in + keyboard work, live SSE.
4. Docs: `docs/SETUP.md` beads-config section; ADR in `.docs/ai/decisions.md`; roadmap/current-state
   routing; `bd close harness-deck-5ph.1`.

## Out of scope (Phase 2 / later)

- Actions: claim (`bd update --claim`), close (`bd close --reason`), create (`bd create`) from the UI
  — `harness-deck-5ph.2`, blocked-by this.
- Respecting `projects.json` hide/show toggles for beads repos; cross-repo aggregated views.
