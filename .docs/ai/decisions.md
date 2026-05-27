# harness-deck — decisions

_ADR log. Newest at the bottom. Record non-obvious choices and why._

## 2026-05-20 — Separate repo, not part of chezmoi-config

harness-deck is its own repo. `chezmoi-config` is a deliberately thin dotfiles
overlay and explicitly not an app/orchestration/reporting layer. Harness-side
integration (hooks/skill) will later live in `chezmoi-config` as a thin pointer.

## 2026-05-20 — Go for the server

Single static binary, no runtime dependency, trivial to launch and manage in
`~/.local/bin`. Fits the user's single-binary CLI-tool ecosystem (chezmoi,
starship, atuin). Frontend stays vanilla HTML/CSS/JS — no build step.

## 2026-05-20 — Authoring contract is a JSON block manifest

Harnesses write `report.json`: an ordered list of typed blocks. The renderer
owns all HTML/CSS, so reports are consistent by construction and old reports
restyle when the renderer changes. Rejected: agent-writes-raw-HTML (no
consistency guarantee, un-restylable) and MCP-first (running-server dependency,
per-harness config burden). A raw-`html` block is the escape hatch for novel UI;
recurring `html` usage is the signal to promote a pattern to a typed block.
JSON is the single canonical format — no parallel Markdown-block format.

## 2026-05-20 — Both central and per-project report storage

Reports are discovered from a central dir (`~/.harness/reports/…`) and from
per-project `.harness/…` dirs of registered project roots. Per-project reports
travel with the repo; central is the catch-all.

## 2026-05-20 — Response round-trip via file + notification

Dashboard responses are written to `responses.json` beside the report and a
configured notification command fires. No live socket coupling required for the
harness to pick responses up — a file is universal across harnesses.

## 2026-05-20 — Go structs are the schema; no standalone report.schema.json

The plan listed a `report.schema.json`. Built instead with Go structs as the
single schema and a Go-native `Validate` (strict decode with
`DisallowUnknownFields` + semantic/enum checks). Reasons: the renderer needs
the structs regardless, a hand-written JSON Schema would drift from them, and a
JSON-Schema validation library is avoidable weight. `CONTRACT.md` (to be
written) is the human/agent-facing spec. A machine-readable schema can be
generated later if an MCP report-builder needs one.

## 2026-05-20 — Inline assets, with a </script guard

`render` produces a single self-contained HTML file: CSS and `vim-nav.js` are
inlined rather than linked. Because vim-nav.js mentions the literal `</script>`
in a header comment, the renderer rewrites `</script` → `<\/script` before
inlining (the HTML parser ends a script at `</script` regardless of JS
context). The vendored asset file itself stays verbatim for the server to
serve as a static file.

## 2026-05-20 — JSON config, not TOML

Config is JSON (`~/.config/harness-deck/config.json`), not the TOML the plan
named. Reason: TOML needs a third-party parser (a module download); JSON is
stdlib. Keeps the build hermetic — no `go get`. The whole project stays
zero-external-dependency on purpose (see also the in-house Markdown renderer).

## 2026-05-20 — Live updates by polling, not fsnotify

Phase 3 detects report changes by polling the report directories on an
interval rather than using `github.com/fsnotify/fsnotify`. Same reason as
above — no module download — and for a handful of local report directories a
~2s poll is indistinguishable from a watcher. The plan said fsnotify; the
observable behaviour (live SSE updates) is identical.

## 2026-05-20 — Roadmap view reuses .docs/ai/roadmap.md

The aggregator's roadmap view renders each registered project's existing
`.docs/ai/roadmap.md` (an established handoff convention) plus roadmap items
agents append via manifests. No new authoring surface.

## 2026-05-21 — Distribution via GoReleaser, GitHub Releases, and a Homebrew tap

Pushing a `v*` tag runs `.github/workflows/release.yml` → GoReleaser builds
static darwin/linux (amd64+arm64) binaries, publishes a GitHub Release, and
commits the formula to `TaylorFinklea/homebrew-tap`. Install paths: `brew
install taylorfinklea/tap/harness-deck`, `go install …`, or a release binary.

Rejected npm: it would force a Node toolchain plus a binary-download shim
package on users, contradicting the single-static-binary design. homebrew-core
proper needs notability the project lacks; a personal tap is the realistic
Homebrew route.

The cross-repo formula push needs a PAT in the `HOMEBREW_TAP_TOKEN` secret —
the built-in Actions `GITHUB_TOKEN` can't write to the separate tap repo.
Currently the broad keychain PAT; a fine-grained tap-only token is the
eventual swap.

`cmd/harness-deck` gained a `version` command; GoReleaser stamps
version/commit/date via `-ldflags -X main.*`. The pipeline keeps the
zero-dependency rule — GoReleaser is a build-time tool, not a module import.

## 2026-05-22 — Binary stays `harness-deck`; `hdeck` is a Homebrew symlink

Considered renaming the binary to `deck`. Rejected: Kong's decK CLI installs a
binary named `deck` via homebrew-core, so a `deck` in our formula would collide
on a `bin/` filename for anyone in the Kong ecosystem. Kept `harness-deck` as
the canonical name and added a short `hdeck` alias instead.

`hdeck` is created by `bin.install_symlink` in the GoReleaser `brews` block —
not a second binary. A separate `cmd/hdeck` would need `main` refactored into
an importable package and would double release artifacts (8 tarballs vs 4) for
something that is purely an alias. The CLI dispatches on `os.Args[1]` and
ignores `os.Args[0]`, so the symlink is behaviourally identical. Trade-off:
`hdeck` exists only for Homebrew installs, not `go install`/`go build`.

## 2026-05-22 — Project discovery + tracking (the `projects` package)

The roadmap view (decision 2026-05-20) required hand-listing every project in
`config.json`'s `projects` array. Replaced that with discovery: `scan_roots`
lists parent directories (e.g. `~/git`), and a depth-1 child holding a
`.docs/ai` directory is a project. The roadmap view became the **projects**
view — per project it now renders `current-state.md` *and* `roadmap.md`.

Toggle state (which discovered projects are hidden) lives in a separate
app-owned `projects.json`, **not** in `config.json`. Rejected writing back to
`config.json`: it would make harness-deck reformat a hand-authored file on
every toggle and put a fragile write on the user's real settings. The state
file records only the `disabled` exceptions, so a newly discovered project is
visible by default ("uncheck to hide") and a deleted one drops out. Writes are
atomic (temp file + rename) — harness-deck's first write path.

`store.Scan` was changed to take project roots as an argument instead of
reading `cfg.Projects` itself; the server passes the enabled set. This keeps
`store` decoupled from `projects` (no import) — the server orchestrates.

The frontend (projects view + toggle checkboxes) has no unit tests: the repo
has no JS test harness and the zero-dependency rule forbids adding one. It is
browser-verified instead, consistent with the rest of the frontend. The Go
side (`projects` package, `/api/projects`, toggle) is fully unit-tested.

---

## 2026-05-23 — Mobile PWA + Web Push, all stdlib

The dashboard needed to be usable from a phone. The space of plausible
choices: a native iOS app (rejected — second codebase, App Store dance,
and the renderer already owns all HTML); a PWA served from the existing
Go server (chosen); a cloud-mirrored variant that wouldn't need the
laptop online (rejected — violates local-tool ethos, doubles the moving
parts). Phone reaches the laptop over Tailscale, which keeps the
trust-boundary unchanged: the tailnet is the auth.

The push pipeline is stdlib-only. Go 1.26 ships everything needed:
`crypto/ecdsa` (VAPID JWT signing, ES256), `crypto/ecdh` (per-message
ECDH P-256), `crypto/hkdf` (key derivation), `crypto/aes` + `crypto/cipher`
(AES-128-GCM). A dedicated `internal/push` package handles VAPID key
gen/load/save, JWT signing, RFC 8291 `aes128gcm` payload encryption,
and the HTTP send; `push.Store` persists subscriptions as JSON. A
roundtrip test (simulate the UA, decrypt with its private key) gives
high confidence the encryption math is right despite the protocol's
notorious finickiness.

TLS is opt-in via a new `tls.cert` + `tls.key` config pair. The default
remains plain HTTP on `127.0.0.1`, so the threat model for someone who
just runs `harness-deck serve` does not change. Users wanting phone push
run `tailscale cert <host>` once and point config at the resulting
files; harness-deck never shells out to `tailscale`, so we add no
runtime dependency on the Tailscale CLI being installed.

Notification trigger: the watcher already polls every 2s. It now also
records a per-report digest of unanswered ask block IDs, and fires one
push per id that appears in the new digest but not the previous one.
The first poll seeds the baseline, so a backlog of existing asks does
not spam the phone at startup. The notification payload contains the
project, report title, and the ask prompt — same boundary as the
dashboard, since anyone with phone access has tailnet access already.

Icons are the existing `hd.svg` reused via the manifest `image/svg+xml`
entries and as `apple-touch-icon`. We did not prerender PNG variants —
it would either require a runtime dep (rejected) or a build-time
toolchain (rejected). If older iOS home-screen rendering looks poor we
can revisit and commit prerendered PNGs.

The mobile layout reuses the existing CSS by layering a
`mobile.css` `@media (max-width: 720px)` overlay rather than forking
into a separate stylesheet — the desktop terminal aesthetic stays
pixel-identical above the breakpoint. A hamburger button + slide-in
drawer replaces the sidebar tree on phones; vim-nav and drag handles
hide on touch viewports per the mobile scope.

The settings view (5th aggregator view, key `5`) is the single
phone-relevant control surface today. Asking the server for status and
asking the browser for its `PushSubscription` are independent calls;
the view renders synchronously, then async-fills the two cells.

## 2026-05-26 — Prerendered PNG icons after all

Superseded the earlier "no PNG variants" decision. iOS Safari's PWA
install + lock-screen notification path renders SVG inconsistently
across versions, and real-device testing on iPhone showed the
home-screen icon looking soft. `rsvg-convert` is a one-shot CLI used
at icon-source-change time — not a runtime dependency, not a build
toolchain — so the original objection (toolchain creep) doesn't apply.
Generated 180/192/512/1024 PNGs are committed to `internal/assets/`,
embedded via `go:embed`, served at `/icon-{size}.png`. Total payload
~27 KB. The SVG stays as a fallback (favicon, last entry in
`manifest.webmanifest`), so nothing regresses for clients that
preferred it.

## 2026-05-26 — Next roadmap wave: ops pane + retrospective + fan-out

After the MVP + post-MVP polish (search, theme, mobile, push,
markdown, …) landed end-to-end on a real iPhone, four next-wave items
got picked together: MCP report-builder server, live in-flight
telemetry, per-project run history, notification fan-out. See
`roadmap.md` for the full list.

The MCP server is explicitly **optional** — the file contract
(`CONTRACT.md`) stays the canonical interface. MCP wraps the same file
writes; a harness without MCP support keeps working unchanged. This
keeps the "no live socket coupling required" property of the original
authoring decision (2026-05-20) intact while removing friction for
harnesses that already speak MCP.

## 2026-05-26 — Notification fan-out: fire-and-forget, redacted GETs

Fan-out destinations live in `config.json` (`notifications []`). The
settings view CRUD writes the same file via the round-trip-through-
`map[string]any` pattern, so future config fields survive — same shape
register.go uses.

Per-destination project allowlist is the only filter shipped in v1.
Rich filters (kind / status / time-of-day) were considered and deferred
— YAGNI until someone hits the case.

Reliability is **fire-and-forget with a log line on failure**. No retry
queue. Justification: the watcher's delta detection re-tests every 2s,
so an ask that's still open re-fires automatically (idempotent at the
event level — never the side-effect level, but a missed notification
isn't worth the queue's cost). Same reliability model as Web Push,
which we've now lived with end-to-end on a real iPhone.

GET /api/notifications **redacts URLs to scheme://host**. Slack and
Discord webhooks embed secrets in the URL path; if the full URL went
out in the GET, any browser session loading the settings view would
echo the secret over the wire. The full URL stays in config.json on
disk; the user re-enters it via the add form to edit. The test
endpoint takes a `name` (not a URL) for the same reason — accepting an
arbitrary URL would make the server an open relay.

A `notifMu sync.RWMutex` guards `cfg.Notifications` against concurrent
reads from the watcher and writes from CRUD handlers. The watcher
copies the slice under the read lock before calling `notify.Fanout` so
a mid-tick CRUD edit can't slice into a partially-written slice.

## 2026-05-26 — MCP transport: stdio JSON-RPC, not HTTP

MCP supports stdio and Streamable HTTP. Picked stdio for harness-deck's
server because:

- Simplest implementation (newline-delimited JSON-RPC; no Content-Length
  framing, no SSE, no auth layer).
- Standard MCP pattern — every major client (Claude Code, Claude
  Desktop, VS Code Copilot Chat) speaks stdio.
- Decoupled from the dashboard process. `harness-deck mcp` works
  whether or not `harness-deck serve` is running; the dashboard picks
  the file up via its 2s watcher.
- Zero external deps (stdlib `encoding/json` + the existing manifest /
  respond / store packages).

Stdout is reserved for protocol; diagnostics route to stderr (`log` is
reconfigured in `cmd/harness-deck/mcp.go` before handoff). A stray
`fmt.Println` to stdout would corrupt the JSON-RPC stream.

The six tools wrap the same atomic-write + validate path the file
contract uses. The file contract (`CONTRACT.md`) stays canonical —
`update_status` and `update_live` round-trip the manifest through
`map[string]any` so any field they don't know about (a future block
type, a future top-level extension) survives the rewrite.

## 2026-05-26 — Live telemetry: top-level field, not a block

`live` is metadata about the run (current step, elapsed, tokens, cost,
progress), not content the renderer should position among the blocks.
Made it an optional top-level field on `Report`. Rejected alternative:
a typed `live` block — would have made the schema list-positional
(where in the blocks array does it belong?) and would have changed
every render every tick.

Freshness ("live" vs "stale") is computed client-side. The server emits
the `updated` ISO timestamp; a small inline script (`live-banner.js`)
re-evaluates every second so the indicator stays honest while the page
stays open. The live window is 60 seconds — long enough to absorb
network blips and short manifest-rewrite races, short enough that a
truly hung harness goes stale fast. Stale state keeps the last
reported data visible; only the pulse + the "working" label change.

The `update_live` MCP tool uses pointer-typed args (`*string`, `*int64`,
`*float64`) to distinguish "client didn't send this field" from "client
sent zero" — so a harness can push just `step` + `tokens` without
clobbering an unrelated cost field. The merge happens through
`map[string]any` round-trip, same pattern as `update_status`.

## 2026-05-27 — Pins replace auto-tabs (manual, sidebar-resident)

The v0.1.x in-app tab strip auto-pinned every opened report. Direct
user feedback: "I don't like the tabs, they just aren't useful to
me." Killed the tabbar; replaced with manual pinning surfaced in a
dedicated PINNED section at the top of the sidebar tree.

Three design choices, all locked after structured discovery:

1. **Manual pin verb (`p`)** — nothing auto-pins. Opening a report
   is transient; only an explicit `p` makes it stick. Inverts the
   default — opt-in instead of opt-out. The pin list stays curated.
2. **Dedicated section + ★ marker in the main tree** — pins show
   as a flat list above REPORTS (no project nesting) so digit keys
   map cleanly. The same report's appearance in the project tree
   gets a ★ glyph for cross-section discoverability.
3. **Digits 1-9 = pinned items** — 1 returns to dashboard, 2-9
   jump to pinned report N-1. Chrome-tab muscle memory preserved.

State persists in localStorage as `harness-deck:pins`. One-time
migration from the old `harness-deck:tabs` key runs on first load
of the new code (copy + delete legacy). API surface is
`window.HDPins.{load,pin,unpin,toggle,isPinned,open}`; the
`HDTabs.open()` shim stays so `search.js` keeps working.

The duplicate-key gotcha worth remembering: a pinned report shows
in BOTH the PINNED list AND its project subtree. Both `<div
class="row run">` elements had identical `data-url`, so
`treeKeyOf()` collapsed them into one key and tree-focus j/k could
never reach the deeper twin past the pinned one. Fix: section-
scoped keys (`p:` for pinned, `t:` for tree). Same lesson as the
NUL-in-CSS-selector bug from v0.2.0 — when keys come from
content, watch for content that collides across views.

## 2026-05-27 — Vim-faithful keyboard model is opinionated about modes

Product discovery surfaced "I keep having to re-learn shortcuts" as
the dominant friction. Picked vim-faithful (modal, hjkl, g-prefix,
Space-leader) over modern-app or hybrid. Trade-off: alien to
non-vim users, but the target audience is vim-fluent power users —
the discovery question that set this was explicit.

Specific bindings landed:
- j/k cursor on inbox rows + tree-focus (Space-e)
- h/l walk choice options + text-input ↔ submit slot
- digits 1-9 = pinned items (was: view switching, now g i / g p / g a)
- a/x/dd row actions (matching dashboard ↔ report-page semantics)
- p = pin / unpin (context: inbox row, tree-active, current report)
- q on report = back to dashboard (vim convention)
- Space leader (preventDefault on browser-scroll; INSERT mode
  passes through)
- g prefix (gi/gp/ga/gd/gh/gt/gT/gx/gg, 1500ms timeoutlen)

The chord timeout (1500ms) came from Playwright friction during
synthetic-event testing but landed at the right human value too —
700ms was tight for deliberate Space-then-key pauses, 1500ms
absorbs them without feeling laggy.

## 2026-05-27 — lark-plug-hdeck as the larkline integration shape

Separate repo (`~/git/lark-plug-hdeck/`, public on GitHub) rather
than living inside the harness-deck or larkline trees. Mirrors how
`lark.nvim` was extracted out of the larkline monorepo on
2026-04-25.

Read-only by design. The plugin GETs `/api/reports`, filters
client-side, opens report URLs in `$BROWSER`. It does NOT POST to
`/respond` or call the MCP server. The trade-off: response writing
would fork the response UI between browser and terminal picker
contexts. For yes/no it'd be cheap (alt actions). For choice and
text it'd need real picker-mode design that doesn't yet exist.
Defer until a week of read-only usage shows the gap.

Configurable via `HARNESS_DECK_URL` env (defaults to
`http://127.0.0.1:7420` which works for users with no TLS but
fails for the TLS+tailnet setup — those need the env var). The
plugin's HTTP layer is larkline's `lark.http.get`, which validates
TLS; tailnet certs validate fine because Tailscale signs via Let's
Encrypt.

Indirect Neovim integration is free via `lark.nvim`: `:Lark` opens
larkline in a floating terminal, picker dispatches like any other
larkline command. A dedicated `hdeck.nvim` would only earn its
keep with in-buffer rendering or LSP-style ask popups — neither
needed today.

## 2026-05-26 — Per-project history is "all runs," not "all kinds"

The projects view already had `Reports []store.Entry` for `kind:
roadmap` reports — "the plan." Rather than overload that field to
include every kind, added a parallel `History []historyRun` field
that lists every run (any kind, including archived) for the project,
newest first, with `responses.json` inlined per run. Two distinct
fields keeps the original semantics intact and lets the frontend
render two visually distinct sections ("reports" = the plan,
"history" = the timeline).

Responses are inlined server-side rather than fetched per-run by the
frontend because the watcher already touches every `responses.json`
into the store fingerprint — they're warm in the kernel page cache.
The alternative (a per-project endpoint the frontend hits on demand)
was rejected: the projects view is the natural location, and a
second round-trip per project on every render would be slower than
the current single bulk read.

## 2026-05-26 — Desktop `.layout` got the same `minmax(0, 1fr)` fix

The mobile polish round (`v0.1.9`) fixed a CSS Grid foot-gun where
plain `1fr` resolves to `minmax(auto, 1fr)`, letting a wide descendant
inflate the track past the viewport. That fix was only applied at the
720px breakpoint. On desktop, `.layout` was still `grid-template-columns:
240px 1fr` — same bug, just invisible because a wide-enough monitor
hid it. Playwright at 1280px viewport revealed `.content` rendering at
4346px wide. Extended the fix to desktop: `240px minmax(0, 1fr)` plus
`.content { min-width: 0 }`. Same logic, durable across viewport
widths now.

## 2026-05-26 — docs/PUBLISHING.md on-ramp

`CONTRACT.md` is the exhaustive reference and stays that way.
`docs/PUBLISHING.md` is the new on-ramp for an external publisher —
60-second smoke test, central vs per-project choice, minimum viable
manifest, the four blocks that cover 90% of reports, response-loop
read-back, and the recommended polish (atomic writes, validate in CI,
prefer typed blocks over `html`). Linked from `README.md` and the
top of `CONTRACT.md`. The motivating force was the cowork project
asking how to publish — "read CONTRACT.md" works, but the gradient
was too steep.
