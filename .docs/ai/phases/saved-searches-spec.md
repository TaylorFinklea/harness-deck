# Saved searches — spec

**Roadmap:** Now #6. **Decision:** Option A (open the Cmd+K palette pre-filled).
Mockup + sign-off: `.harness/reports/harness-deck/20260616-saved-searches-design/`.

## Goal

Let the user pin a JQL search query (the `internal/query` language shipped
2026-06-15) to a new `SAVED` sidebar section. Clicking a saved search opens the
Cmd+K palette **pre-filled with that query, run live**. Saving a search happens
from inside the palette. Pure client-side: **no server changes** — the query
language + `/api/search` already do all the matching.

## Architecture (one-way, mirrors pins)

```
search palette ──save──▶ HDSaved (localStorage) ──hd:saved-changed──▶ renderTree (SAVED section)
sidebar SAVED row ──click──▶ HDSearch.open(query)  (palette pre-filled + live)
```

`HDSaved` is the exact analogue of `HDPins` (tabs.js): a tiny localStorage
state module that fires a DOM `CustomEvent` on write; the sidebar listens and
repaints. No module imports either direction — they talk through the event +
`window` globals, same as pins.

## Scope decisions (locked)

- **Storage:** `localStorage`, new key, mirroring `HDPins`. NOT a server store.
- **Activation:** click → `HDSearch.open(query)`. The palette loads on every
  page, so a saved query *runs* anywhere; the clickable SAVED **list** is
  **dashboard-only** (like PINNED — `renderTree` only exists in the dashboard
  bundle).
- **Naming:** on save, `prompt()` for a name, **default = the query string** —
  Enter accepts query-as-name, or type a friendly label. Empty/cancel aborts.
- **No digit shortcuts.** Pins own digits 2–9; reusing them collides. Saved
  searches activate by **mouse click only** in v1. (Keyboard activation +
  `Cmd+S`-to-save are explicit follow-ups, not this scope.)
- **Management:** remove via a hover `✕` on the row. **No rename** (remove +
  re-save). Soft cap 30, drop-oldest like pins.
- **Dedup by query:** saving a query already stored is a no-op.

## Spec-derived (implement exactly)

### 1. New module `internal/assets/saved.js` → `window.HDSaved`

Mirror `tabs.js`'s structure (IIFE, try/catch around every `localStorage`
call, identical event-dispatch shape). **Read `tabs.js` first and follow it.**

- Storage key: `harness-deck:saved-searches`.
- State shape: array of `{ name, query }` in insertion order.
- `MAX_SAVED = 30`; on `add`, `while (list.length > MAX_SAVED) list.shift()`.
- On every write, `localStorage.setItem` then dispatch
  `new CustomEvent('hd:saved-changed')` on `window` (copy the exact guard +
  pattern from `tabs.js`'s `save()` / `hd:pins-changed`).
- Public API on `window.HDSaved`:
  - `load()` → array (always an array; bad/missing JSON → `[]`).
  - `list()` → alias of `load()` (read-only consumers).
  - `add(name, query)` → bool. No-op+false if `query` already present
    (dedup by exact `query` string). Trims inputs; ignores empty `query`.
  - `remove(query)` → bool. Removes the entry whose `query` matches.
  - `isSaved(query)` → bool.
- **No keydown listener** — this is a pure storage module, so its bundle
  position is irrelevant to keyboard precedence.

### 2. `internal/assets/assets.go` wiring

- `//go:embed saved.js` → `var SavedJS string`.
- `var SavedJSInline = strings.ReplaceAll(SavedJS, "</script", ` + "`<\\/script`" + `)`
  (every bundle member goes through the `</script` escape — see the file's
  existing vars).
- Add `SavedJSInline` to the **`ReportJS`** concatenation so report pages get
  `window.HDSaved` for the palette save button. Position: **after
  `TabsJSInline`** (sibling state module), before `SearchJSInline`. Because it
  has no keydown listener, it does not change keyboard precedence — but keep it
  adjacent to Tabs for readability.

### 3. Dashboard shell — `internal/server/server.go` + `internal/server/shell.html.tmpl`

- In `server.go`, find where the shell template data is populated with
  `SearchJS` / `TabsJS` (they map to `assets.SearchJSInline` / `assets.TabsJSInline`
  or the raw vars — **read it and mirror exactly**). Add a `SavedJS` field fed
  the same way from `assets.SavedJSInline`.
- In `shell.html.tmpl`, add `<script>{{.SavedJS}}</script>` immediately after
  the existing `<script>{{.TabsJS}}</script>` (line ~75), so `HDSaved` exists
  before any user click. (`AppJS`/`renderTree` runs at interaction time, after
  all `<script>`s load, so absolute order only needs HDSaved present in the
  document — adjacent to Tabs is the clean choice.)

### 4. `internal/assets/search.js` — pre-fill open + save affordance

Two changes, both surgical. **Read the file first.**

- **`open(initialQuery)`** — currently `open()` hard-clears `input.value = ""`.
  Accept an optional `initialQuery`: when truthy, set `input.value` to it, then
  run the live query immediately (call the same path `onInput`/`runQuery` use —
  do NOT duplicate fetch logic; reuse `runQuery(q)` after setting the value, and
  still `updateSuggestions()` so the caret state is sane). When falsy, keep the
  current clear-to-empty behavior. Keep `input.focus()`.
  - Update the exported `window.HDSearch = { open, close }` — `open` already
    exposed; the new arg is backward-compatible (the titlebar `?` button calls
    `HDSearch.open()` with no arg — must still clear to empty).
- **Save affordance** — a `☆` control in `.search-input-wrap` (next to
  `.search-status`). Visibility rule: shown only when the **current query is
  non-empty AND parsed clean**. Track a `queryValid` flag: set `true` at the
  top of the no-error branch in `runQuery` (where `status.classList.remove
  ("search-status-hint")` runs), `false` on the parse-error branch and on the
  empty-input branch in `onInput`. Toggle the control's visibility from
  `queryValid && input.value.trim() !== ""`.
  - On activate (click): `var q = input.value.trim(); var name = prompt("Name
    this search", q); if (name == null) return; name = name.trim() || q;
    window.HDSaved && HDSaved.add(name, q);` then a quiet confirmation in
    `status` (e.g. `status.textContent = "saved ✓"`). Do not close the palette.
  - Build the control with `HDDom.el` (search.js already uses `el`); no
    `innerHTML`.

### 5. `internal/assets/aggregator.js` — SAVED section + click branch + listener

- **`renderTree()`** — build a `SAVED` section and unshift it as the **first**
  `sidebar-section` (above PINNED, above REPORTS). Gate on
  `window.HDSaved && HDSaved.list().length`. Each row:
  - class `row saved` (NOT `row run` — must stay out of `aggregator-tree.js`'s
    `#tree .row.run` tree-focus walk and out of the generic `[data-url]` nav).
  - `data: { saved: item.query }` (carries the query; NO `url`).
  - children: a `⌕` glyph span (`saved-glyph`), the name in a `.label` span
    (reuse the ellipsis style), and a `✕` remove button `el('button', { class:
    'saved-del', title: 'remove' }, ['✕'])`.
  - Section title: `saved` (no `· s` hint — there is no key binding).
- **Delegated click** (the `document.addEventListener('click', …)` at ~line
  680) — add a branch **before** the generic `var row = e.target.closest
  ('[data-url]')` block:
  ```
  var del = e.target.closest('.saved-del');
  if (del) { var r = del.closest('[data-saved]');
             if (r && window.HDSaved) HDSaved.remove(r.dataset.saved);
             return; }
  var sv = e.target.closest('[data-saved]');
  if (sv) { if (window.HDSearch) HDSearch.open(sv.dataset.saved); return; }
  ```
  (Order matters: the `.saved-del` check precedes the row check so the ✕ does
  not also trigger activation.)
- **Repaint listener** — next to the existing
  `window.addEventListener('hd:pins-changed', renderTree)`, add
  `window.addEventListener('hd:saved-changed', function () { renderTree(); });`.

### 6. `internal/assets/v1.css` — styles

Mirror the PINNED block (lines ~110–136). Add a `.saved-tree` / `.row.saved`
group:
- `.row.saved` — same padding/flex as `.row.run`; `cursor: pointer`.
- `.saved-glyph` — `color: var(--tn-cyan); margin-right: 4px; font-size: 10px;`
  (the magnifier).
- `.saved-del` — unstyled button: transparent bg, no border, `color:
  var(--tn-comment)`, `cursor: pointer`, `margin-left: auto`, small font;
  `.row.saved:hover .saved-del { color: var(--tn-red); }` and hide it until row
  hover (`.saved-del { visibility: hidden }` → `.row.saved:hover .saved-del {
  visibility: visible }`).
- `.search-save` (the palette `☆`) — small, `color: var(--tn-comment)`,
  `cursor: pointer`, `:hover { color: var(--tn-yellow) }`; hidden by default
  (`display:none`), shown when `queryValid`.

## Invariants / edge cases

- `localStorage` unavailable / quota / malformed → silent no-op returning `[]`
  (copy tabs.js try/catch). Never throw to the page.
- Empty input or parse-error query → save control hidden; you cannot save a
  broken query.
- Saved rows never carry `data-url` → they never navigate; `aggregator-tree.js`
  tree-focus (`.row.run`) never selects them.
- `HDSearch.open()` with no arg still clears to empty (titlebar `?` button +
  Cmd+K unchanged).
- Removing the active/last saved search just repaints; no navigation.

## Verification (all must pass)

1. `gofmt -l internal/ | head` → empty (no unformatted Go; only server.go changed).
2. `go build ./...`
3. `go test ./...` (must stay green; `TestBundlesAreValidJS` runs `node --check`
   over `ReportJS` incl. the new `saved.js`).
4. `go vet ./...`
5. `node --check` on `saved.js` standalone (belt-and-suspenders; it's also a
   dashboard `<script>` tag, not only bundled).
6. **Browser-verify** (chrome-devtools MCP, dashboard at https://127.0.0.1:7420):
   - Cmd+K, type `status = awaiting-review`, ☆ appears → click → name prompt →
     accept → SAVED section appears in sidebar with the row.
   - Click the saved row → palette opens pre-filled with the query, results
     listed live.
   - Hover row → `✕` appears → click → row removed, section hides when empty.
   - Type a deliberately broken query (`status =`) → ☆ hidden.
   - Titlebar `?` button still opens an EMPTY palette (no regression).
   - Zero console errors.

## Out of scope (record as follow-ups)

- Digit/keyboard activation of saved searches; `Cmd+S` save shortcut.
- Inline (non-`prompt`) naming UI; rename.
- Saved-list UI on report pages (palette save already works there).
- Server-side store / cross-device sync.
