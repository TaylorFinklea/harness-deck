# Beads Backlog viewer — Phase 2 spec (actions: claim / close / create)

Status: **design — awaiting review**. Branch: `feat/beads-backlog-viewer` (continues Phase 1).
Bead: `harness-deck-5ph.2` (blocked-by the now-closed `5ph.1`).
Builds directly on `beads-backlog-viewer-spec.md` (Phase 1, read-only). Decisions
captured here + `decisions.md`. Approved: **separate `beads.writable` gate** +
**all three actions**.

## Goal

Add **claim / close / create** actions to the read-only Backlog view so the full
`bd` work-loop is drivable from the dashboard. Mutating, opt-in, safe by default.

## Constraints (inherited from Phase 1)

- Stdlib-only Go (no `require` block); vanilla JS (no npm). Never read `.beads/`
  directly — all writes go through `bd … ` (equals-form flags).
- Every `bd` subprocess gets a context timeout. Writes additionally serialize
  (see Concurrency).
- Graceful degradation: a write failure surfaces an error to the user; it never
  crashes the view or corrupts the snapshot.

## Gating — two flags

- New `config.BeadsConfig.Writable bool` (`json:"writable,omitempty"`, default
  **false**). Writes require `Beads.Enabled && Beads.Writable`.
- The server exposes writability to the page as `window.HD_BEADS_WRITABLE`
  (mirrors `HD_BEADS`, injected in `shell.html.tmpl` via `handleShell`), so
  action affordances render **only** when writable.
- Every write endpoint independently enforces it server-side (**403** when not
  writable) — the JS flag is UX, not the security boundary.

## Endpoints (mirror `handleRespond` — server.go)

Each: method `POST`, JSON body, validate → **status re-check** → write → force an
immediate snapshot refresh + broadcast the `beads` SSE event → `{"ok":true}` (or
`{"ok":true,"id":"<new>"}` for create). Registered in `New()`'s `mux.HandleFunc`
block next to the Phase-1 beads routes.

| Route | Body | bd invocation | Status codes |
|---|---|---|---|
| `POST /api/beads/{project}/{id}/claim` | — | `bd -C <root> update <id> --claim` | 200 · 400 bad id · 403 !writable · 404 unknown project/issue · 409 closed · 502 bd · 503 !enabled |
| `POST /api/beads/{project}/{id}/close` | `{"reason":"…"}` | `bd -C <root> close <id> --reason=<reason>` (omit flag if empty) | same + 409 if already closed |
| `POST /api/beads/{project}/create` | `{"title","type","priority","description"}` | `bd -C <root> create --title=<t> --type=<type> --priority=<p> --description=<d>` (omit `--description=` if empty) | 200 (+id) · 400 · 403 · 404 unknown project · 502 · 503 |

**Gate order** (so tests are deterministic): `beads.ValidID(id)` 400 → feature
enabled (`s.beadsMutator != nil`) 503 → writable (`cfg.Beads.Writable`) 403 →
project→root 404 → input validation 400 → re-check 409 → write.
(create has no `{id}`; it validates title/type/priority instead.)

## Input validation (create)

- `title`: non-empty after trim; reject a leading `-` is unnecessary (it is a
  flag *value*), but cap length (e.g. 500) and reject control chars.
- `type` ∈ {`bug`,`feature`,`task`,`epic`,`chore`} — reject anything else (400).
- `priority` ∈ {`0`,`1`,`2`,`3`,`4`} (string) — reject anything else (400).
- `description`: optional; passed as a flag value (multiline OK in argv).

## Argv safety

`exec.Command` runs `bd` with no shell, so there is no shell-injection surface.
The remaining risk is flag-smuggling. Mitigations: `id` guarded by
`beads.ValidID`; `type`/`priority` are closed enums; **all string values use the
`--flag=value` equals form** so a value that begins with `-` stays bound to its
flag rather than being reparsed as a new flag. No positional user input.

## Concurrency — Dolt single-writer

`bd`'s embedded Dolt is single-writer per repo. Add a `sync.Mutex` to
`beads.Client` that guards the **write** methods (Claim/Close/Create) so two
mutations never run concurrently. Reads (the Monitor's Ready/Blocked/List/Status)
stay unlocked — writes are low-frequency + user-initiated, so a global write lock
is simplest and sufficient; if read-during-write ever proves flaky, escalate to a
per-repo RWMutex (documented, not built now — YAGNI).

## Freshness

`beads.Monitor` gains `RefreshNow(ctx)` (exported wrapper over `refreshOnce`) so a
write handler forces an immediate re-read + fingerprint diff → `OnChange` → SSE,
instead of waiting up to `refresh_sec` (15s). The frontend also calls
`refreshBeads()` on a successful POST (belt-and-suspenders; the SSE covers other
open tabs).

## Adapter — `internal/beads`

New `Client` methods (each acquires the write mutex, sets a ctx timeout, nil
stdin):
- `Claim(ctx, root, id) error`
- `Close(ctx, root, id, reason string) error`
- `Create(ctx, root, title, itype, priority, description string) (string, error)`
  — parses the new id from `bd create` output. **Verify** the output shape first
  (`bd create --json` if it returns the issue, else parse the `Created issue: <id>`
  line); mirror the parse-on-fixture test style.

## Server — `internal/server`

- `beadsMutator` interface (`Claim`/`Close`/`Create` only) so handler tests inject
  a fake without real `bd`; `*beads.Client` satisfies it. The pre-write **re-check
  reuses the existing `beadsDetailer.Show`** (`s.beadsClient`) rather than
  duplicating Show on the mutator.
- `s.beadsMutator beadsMutator` field, set in `New()` only when
  `cfg.Beads.Enabled && bd found` (same construction as `beadsClient`; the writable
  gate is checked per-request, not at construction, so toggling writable needs only
  a config reload of the flag — but note Phase 1 established config is restart-level
  except roots; document that writable takes effect on restart).
- Handlers `handleBeadsClaim`, `handleBeadsClose`, `handleBeadsCreate`. On success
  call `s.beadsMonitor.RefreshNow(r.Context())` then `s.hub.broadcastEvent("beads","changed")`.

## Frontend — `internal/assets`

Writable-only affordances (guard every one on `window.HD_BEADS_WRITABLE`):
- **Detail panel** (`HDBacklog` drill-in): a **Claim** button (shown when the issue
  is not closed; `bd --claim` is idempotent so re-claiming is harmless) and a
  **Close** control — an inline
  reason `<input>` + **Close** button. Post to the endpoints; on ok, `refreshBeads()`
  + reopen/refresh the detail.
- **Create**: a `+` affordance on each repo-card header opens a small **create form**
  (title `<input>`, type `<select>`, priority `<select>`, description `<textarea>`,
  Create/Cancel). Submit → `POST /api/beads/{project}/create`.
- **Keyboard** (backlog view, writable): `c` claim focused row, `x` close focused
  (opens the reason field focused), `n` new-issue form for the focused row's repo.
  Wired in `aggregator.js`'s backlog keydown block next to j/k/Enter/Esc.
- Errors: a POST failure shows the response text inline in the panel/form (mirror
  `reportAction`'s tolerant `.catch`), never a crash.
- CSS: `.bk-act*` classes in `aggregator.css`.

## Testing

- `internal/server/beads_test.go` — a `fakeMutator` (satisfies `beadsMutator`):
  - claim/close/create happy paths (200, and create returns an id);
  - gates: 400 (bad id / bad type / bad priority / empty title), 403 (not
    writable), 404 (unknown project), 409 (close an already-closed issue via the
    Show re-check returning a closed status), 503 (feature disabled).
- `internal/beads` — unit-test the create-output parse on a captured fixture; a
  `flagLike`/enum-validation helper test if extracted. Write-method shelling itself
  is covered by browser/manual verify.
- Frontend: browser-verify (chrome-devtools) against a **throwaway** issue —
  create it, claim it, close it — then confirm it left no residue (the created
  issue is closed, not deleted; note it in the verify log).

## Acceptance

- With `beads.writable:true`: claim/close/create all work from the UI and the view
  reflects the change within ~1s (immediate refresh), across a real repo.
- With `beads.writable:false` (default): no action affordances render, and a direct
  `POST` to any write endpoint returns 403.
- Every guard returns its documented status code.
- `go build ./...` + `go test ./...` green; `gofmt -l .` empty; `go.mod` still has
  **no `require`**.

## Verify

```sh
cd ~/git/harness-deck
go build ./... && go test ./...
gofmt -l .
# with beads.enabled+beads.writable in a test config, browser-verify claim/close/create
HARNESS_DECK_CONFIG=/tmp/hd-beads/config.json ./harness-deck serve
```

## Task sequence (for the plan)

1. `config.BeadsConfig.Writable` + `internal/beads` write methods (Claim/Close/Create
   + write mutex) + create-output parse test. **Verify:** `go test ./internal/beads ./internal/config`.
2. `Monitor.RefreshNow`; `beadsMutator` interface + the three handlers + gating +
   route registration + `HD_BEADS_WRITABLE` shell flag + server tests. **Verify:** `go test ./internal/server`.
3. Frontend: detail-panel Claim/Close + create form + `c`/`x`/`n` keys + CSS.
   **Verify:** browser (create→claim→close a throwaway issue; 403 when not writable).
4. Docs (SETUP §9 `writable`, decisions.md ADR) + `bd close harness-deck-5ph.2`.

## Out of scope (future beads)

- edit / reopen / delete / dependency-editing from the UI; bulk actions; a repo
  picker for create beyond "the focused row's repo".
