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

## 2026-06-09 — Self-describing binary: embed CONTRACT.md, don't fetch it

A coworker installing the binary (no repo clone, no dotfiles) had no
way to get `CONTRACT.md` — and the user's chezmoi `AGENTS.md` + the
installed `SKILL.md` both hardcoded `~/git/harness-deck/CONTRACT.md`,
a dead path off the dev box. Fixed by making the binary carry its own
docs.

A new root-level package (`embed.go`, `package harnessdeck`) embeds
`CONTRACT.md` + `docs/PUBLISHING.md` via `go:embed` at the module root
(`github.com/TaylorFinklea/harness-deck`), so any package can read them
with no duplication, symlink, or `go:generate`. Three surfaces expose
them, all reading the same bytes: `harness-deck contract [--publishing]`
(CLI), MCP resources `harness-deck://contract` + `://publishing` plus an
`instructions` string in the `initialize` handshake (the coworker
payoff — one MCP registration equips an agent fully), and HTTP
`GET /contract.md`.

Rejected: fetching `CONTRACT.md` from GitHub `HEAD` at runtime. Embedding
makes the contract *version-locked to the binary* — you always get the
contract that matches the renderer you installed. A GitHub-latest fetch
could describe block types the installed binary can't render, which is
exactly the graceful-degradation footgun the project already guards
against. Offline-safe and zero-dependency as a bonus.

## 2026-06-10 — Bug bash audit + release-first sequencing

Multi-agent audit (8 bug lenses + 5 architecture reviewers, findings
adversarially verified against source) produced 30 confirmed findings;
durable record in `.harness/20260610-bug-bash-audit/report.json`. Product
review locked these decisions:

- **Tag v0.2.4 immediately, before fixes.** The self-describing binary is
  referenced by committed README/SETUP and the repointed chezmoi
  AGENTS.md/SKILL.md but exists in no tag — every installed binary fails
  `hdeck contract`. Nothing in the tag is worse than what's installed, so
  release first. Full sequencing: v0.2.4 → crit/high fix milestone →
  v0.2.5 → Medium launch (readers shouldn't hit known highs).
- **Fix-milestone scope: critical + all 8 highs.** Mediums/lows became
  self-contained roadmap Backlog items rather than a campaign.
- **Root-cause framing over per-site patches:** 7 findings share one
  pattern — uncoordinated read-modify-write on shared JSON. One
  `internal/jsonfile` helper (unique temp names, UseNumber, refuse to
  clobber unparseable files) fixes the critical, the config clobber, the
  tmp collision, and the float64 mangling in a single move.
- **`.harness/` is committed**, not gitignored — decision records travel
  with the repo and the flagship repo dogfoods its own in-repo publishing
  doctrine.
- **lark-plug-hdeck response writing re-deferred** with a concrete
  trigger ("catching myself opening the browser just to tap yes/no")
  replacing the expired time-boxed condition.
- **Report templates** picked as the first post-launch feature (compounds
  the launch funnel; scaffolding already exists).
- **Arch adoptions:** watcher tick() + delta/CRUD tests, registry
  cross-check test (+ absorbing blockPrompt/blockText into manifest),
  assets bundle-invariants test, schema versioning mechanism.

## 2026-06-13 — Audit-backlog clear-out (20 items, 9 clusters)

Cleared the entire mediums+lows backlog from the 2026-06-10 audit in one wave.
Commits 2293e2b..67d5ae2; full `go test -race ./...` green.

**Orchestration.** Grouped the 20 items into 9 **file-disjoint** clusters so
implementation could fan out in parallel without collisions, then integrated
serially. Each cluster ran as a subagent in an **isolated git worktree**
(implement → TDD → `go build && go test`), and the authoritative diffs were
pulled straight from the worktree filesystems rather than the agents' returned
JSON (the round-tripped diff strings were HTML-entity-escaped — `&amp;` for
`&` — and would have corrupted Go source if applied). Integration (apply →
full-suite verify → one commit per cluster) stayed in the main loop for
oversight. The only cross-cluster file overlap was `mcp_test.go` (C1+C3, both
appending test funcs); `git apply`'s context search placed them without
conflict.

**Per-cluster choices worth keeping:**

- **C1 (manifest/mcp).** `update_status`/`update_live` alphabetize report.json
  keys because the reorder originates in `jsonfile.Patch`'s `map[string]any`
  round-trip. Chose to **document** the trade-off at both call sites rather
  than re-marshal from the struct — re-marshaling would defeat jsonfile.Patch's
  two guarantees (preserve unmodeled forward-compat fields; keep number
  literals byte-exact via UseNumber). Strict top-level Validate is implemented
  by retaining the document bytes (`Report.raw`) in Parse and strict-decoding
  them — mirrors the per-block `Block.Raw` pattern. Note the interaction:
  publish_report now runs Validate before writing, so an unmodeled top-level
  key is rejected at publish (was silently dropped); byte-fidelity is therefore
  about authored **field order**, not smuggling unknown keys.
- **C2 (push/server).** Truncation lives at the Body-construction site
  (`notifyNewAsks`), **not** `encrypt()` — encrypt receives opaque marshaled
  bytes that can't be cut. Ask-delta re-fire fixed with a **decaying retention
  window** (`askRetainTicks=3`) over a permanent notified-Tag set, because a
  permanent set would break the documented "answered ask replaced by a fresh
  same-id ask still re-fires" behavior. `publicReportURL`→`BaseURL()` changes
  the URL for an unspecified bind from `0.0.0.0` to loopback (intended).
- **C4 (store).** `scanMu` held across walk+commit over a generation counter
  (smaller, obviously correct; separate from `s.mu` so index reads stay
  uncontended — no lock-order inversion). Collision policy: first-seen wins
  (central scanned first) **and** the collision surfaces in `Errors()`.
  Honest caveat: the last-writer-wins defect is a **logical** clobber, not a
  data race (the existing `s.mu` already passes `-race`), so its test is a
  regression guard, not a fail-before/pass-after — a true repro needs an
  out-of-scope timing hook (parked in Later).
- **C5 (render).** NUL-delimited placeholders (`\x00N\x00`) park links/
  autolinks/code spans before the emphasis passes; `html.EscapeString` never
  emits NUL so the markers are unambiguous. For `[text](url)` only the opening
  tag is parked, so `*italic*` inside link text still renders.
- **C7 (projects/config).** Kept `Project.Name` as the projects.json
  persistence key but made it **unique-on-collision** (parent-dir hint, numeric
  fallback) — no on-disk schema change, no migration, and the common
  no-collision case is byte-identical. Avoided adding a new key field that
  would ripple into the server/frontend toggle/reorder identifiers.
- **C8 (notify/server).** Applied **both** offered fixes: a 10s
  `context.WithTimeout` + `exec.CommandContext` in `notify.Run` (signature
  unchanged; `runTimeout` a package var as a test seam) **and** moved the
  notify call after `Scan`+`broadcast` so a slow-but-bounded command can't
  delay the SSE refresh.

## 2026-06-13 — Report templates (`new --template`)

First post-launch feature (the launch-funnel compounder picked 2026-06-10).
`new --template audit|review|progress|decision|idea` pre-fills the block shapes
each kind usually needs. Built + adversarially reviewed (3-lens workflow) before
commit.

- **Opt-in, not default.** No `--template` still yields the single placeholder
  prose block (the `TestNewScaffoldsValidManifest` invariant). Templates are a
  richer opt-in scaffold, so backward compat is exact.
- **One-flag UX.** A template supplies a default `--title` and defaults `--kind`
  to the template name (both overridden by an explicit flag, detected via
  `fs.Visit`/`flagSet`). `new --template audit` works alone. `kind` is
  free-form (not enum-validated), so `review`/`decision` are valid kinds.
- **Hand-built JSON, type-first.** Template blocks are hand-written JSON
  constants (matching `starterReport`'s existing %q style and field order — the
  file is the first example the user edits), not struct-marshaled: the body
  structs embed an **unexported** `blockHead`, so package `main` can't
  construct them, and `map[string]any` marshaling would alphabetize keys
  (type-last). The safety net is `TestNewTemplatesValidate`, which parses +
  strict-validates every template to zero problems — a typo fails the build.
  `templateOrder` vs the `reportTemplates` map are kept in sync by
  `TestTemplateRegistrySync` (mirrors `TestRegistryCrossCheck`).
- **Interactive templates stay `draft` + flag the status flip.** review/decision/
  idea ship an interactive block but scaffold as `draft` (awaiting-review would
  falsely assert the placeholder question is real). The success message branches
  on `interactive` to remind the user to flip status. Interactive blocks carry a
  `title` so they don't render as the bare type name.
- **Declined (deliberate):** kept the audit findings `table` over a `risks`
  block — the `where` (file:line) column is high-value and a free-text severity
  is friendlier in a scaffold than the `risks` severity enum (a mistyped
  severity would fail validation). Left the no-template default's backtick/
  imperative voice as-is (it showcases code spans). Did not refactor cmdNew's
  `os.Exit` error paths into a pure helper for testability (consistent with the
  file's existing style; the realistic drift — template list vs map — is covered
  by the cross-check test instead).
- **Discovered, routed not fixed:** a `draft` report with an interactive block
  still counts as an open ask and fires push — so an interactive template
  scaffolded into a watched dir notifies for placeholder content. App-wide
  behavior change, so it was a roadmap Next item (decision needed), not folded
  into this feature. → resolved same day, see below.

## 2026-06-13 — Draft reports don't surface as open asks

Follow-up to the templates review (user chose "fix it now"). A `status: draft`
report's interactive blocks no longer count toward `Entry.OpenAsks`, so they
don't show in the inbox/projects/MCP open-ask counters and don't fire Web Push
or fan-out. The asks still render and stay answerable on the report page; they
start surfacing the moment the author flips status off `draft`.

- **Gated at the single computation point** (`internal/store/store.go`,
  `entryFromReport`: `if e.Status == "draft" { e.OpenAsks = 0 }`) rather than at
  each consumer. Every surface (the `/api/reports` inbox badge, `push.go`'s
  `currentAskDigests` gate, `projects.go`, `mcp` `list_reports`) reads
  `OpenAsks`, so one change covers them all and redefines the field to "open
  asks that are ready for the user." The report-page ask list is computed
  separately in `render.go`, so a draft's asks still render on its own page.
- **Why this is correct, not just convenient:** it aligns behavior with the
  status lifecycle already documented in docs/PUBLISHING.md ("draft = not ready
  for the user yet"). Flipping to `awaiting-review` makes the asks appear in the
  digest and fires the push once — the intended "now it's ready" signal.
- No test relied on a draft surfacing asks (full suite + `-race` stayed green);
  `TestScanDraftReportSuppressesOpenAsks` is the new guard.

## 2026-06-14 — Usage monitors (CodexBar-style footer)

Added a footer usage indicator (next to the address) for five tools:
codex, openrouter, claude-code, copilot, opencode. New `internal/usage`
package: a `Provider` per tool, a `Monitor` that refreshes them concurrently
and caches `Sample`s, served at `GET /api/usage` and rendered by `usage.js` in
the statusline. Full data-source research + per-tool fields:
`phases/usage-monitors-spec.md`. Config: `docs/SETUP.md` §8.

- **Opt-in per provider.** Nothing reads credentials or hits the network
  unless the tool is listed in `config.usage.providers`. Several providers
  touch a Keychain (claude) or a remote API (openrouter/copilot/opencode), so
  listing *is* the consent. Empty list ⇒ feature off (the poller doesn't even
  start). Graceful degradation: a provider with no data returns
  `Sample{OK:false}` and is omitted from the footer.
- **Two usage shapes, one Sample.** `window` (rate-limit % + reset: codex,
  claude, copilot) vs `budget` (spend/credits: openrouter; copilot
  free/unlimited). The footer renders `% ` (severity-tinted) or `text`.
- **Stdlib-only, like the rest of the repo.** os/exec (`security` for the
  Claude Keychain token) + net/http + encoding/json. No SQLite driver, no SDKs.
  This is why OpenCode uses the web `_server` scrape (cookie) over its local
  SQLite db — a driver would breach the zero-dep rule.
- **Provider-specific, decided with the user after CodexBar-source research:**
  - claude-code true % needs the OAuth token, which on macOS lives **only** in
    the Keychain → one-time `security` "Always Allow", then silent (token
    auto-refreshes). `$CLAUDE_CODE_OAUTH_TOKEN` / file creds avoid the prompt.
    No `claude` CLI usage command exists (open upstream request).
  - copilot uses the **undocumented** `copilot_internal/user` endpoint — the
    only per-user source (GitHub's billing API is org/enterprise-only); flagged
    as ToS-gray in `copilot.go` and SETUP.md; opt-in is the consent.
  - opencode has **no** usage API (open request anomalyco/opencode#10448); the
    `_server` hash IDs are opencode.ai build fingerprints that drift on their
    deploys, so this provider is expected to break periodically and degrades to
    hidden — accepted trade-off for the CodexBar-style subscription %.
- **Review fixes (3-lens adversarial pass):** meter OpenRouter off
  `limit_remaining/limit` (not lifetime `usage`, which drifts past a topped-up
  cap); bound the OpenCode regex to the target block's braces so a partial
  block can't borrow a sibling's numbers; and **drop the external response body
  from `httpError`** — `Sample.Err` is served on the unauthenticated
  `/api/usage`, so echoing a provider's raw 4xx/5xx body would broaden
  disclosure beyond the credential host.
- **Drive-by:** the footer address was hardcoded `127.0.0.1`; now shows the
  real bind/URL via `statusAddr(cfg.BaseURL())`.

## 2026-06-14 — Perf wave (scan + projects endpoint)

Three changes, all measure-first / cache-by-mtime, no behavior change:

- **Incremental scan** (`store.Scan`) — reuse the previous scan's parsed Entry
  when a report's `(path, mtime, respMtime)` is unchanged, keying on the exact
  tuple `Signature()` already trusts, so no new staleness. ~6.9× faster warm
  ticks on realistic manifests (BenchmarkScanCold/Warm). The retained soft
  warning (corrupt responses.json) is cached too so it keeps surfacing.
- **/api/projects history cap** — `buildHistory` sorts, caps to `historyCap`
  (50) by default, then loads `responses.json` only for the kept runs (the cap
  is applied *before* the per-run file reads — the actual win, since the
  endpoint is re-fetched on every SSE refresh). `?all=1` removes the cap;
  `history_total` is surfaced so the footer/panel shows "newest 50 of N".
- **Rendered-doc cache** — project `roadmap.md`/`current-state.md` markdown is
  rendered once and memoized by file mtime (`Server.docCache`, guarded by
  `docMu`), instead of re-running the in-house renderer for every project on
  every poll. Invalidates when mtime changes; cache is bounded by project count.

Tests: BenchmarkScanCold/Warm + the store suite; `TestProjectsHistoryCapped`
and `TestRenderDocCachesByMtime` (the latter pins both cache-hit-by-mtime and
invalidation via `os.Chtimes`). Full suite + `-race` green.

## 2026-06-14 — Session-code audit + tiered backlog clear-out

A 5-lens Sonnet bug-bash audit of this session's under-audited code (perf wave,
draft-gating, usage internals — the per-feature reviews hadn't covered their
interaction). Findings triaged by the Lead (Opus) and routed by tier per the
user's mapping (Haiku = junior, Sonnet = senior, Opus = lead). Commits
d2b5a95, 6167957, e850943, 40ed9b3, fb0d173, e5d6ef0.

- **Headline (major, Opus-implemented):** ask-retention (C2) defeated
  draft-gating (this session). A report seen awaiting-review → drafted →
  re-published never re-fired its push: the retained ask looked "already seen."
  The subtle part is distinguishing a *transient* disappearance (report gone
  from the index — must retain, guards TestTickReappearingAskNoRefire) from an
  *intentional* close (report indexed but OpenAsks==0/archived — must drop so a
  re-open fires). Fix keys on `closedAskKeys()` (the full store), not
  `curEntries` (which omits draft reports). Lead implemented this directly
  rather than delegating — too subtle for the junior tier and a delegation
  round-trip wasn't worth it for the critical fix.
- **Routing:** B (store-cache correctness tests) → Sonnet; C (renderDoc
  TOCTOU + docCache eviction + tests), D (push body cap + usage error/edge
  tests), E (frontend hint/overflow) → Haiku. Deferred with rationale: the
  jsonfile order-preserving patch (risk≫benefit on the central write helper),
  askRetainTicks-config (YAGNI), a store stale-walk test hook (prod
  test-scaffolding), and the tabs.js digit-handler trim (harmless redundancy).
  B1 (hardcoded schema literals) was a no-op — no Go sites remained.
- **Workflow gotcha (important for future runs):** `isolation: 'worktree'`
  branches worktrees from a stale ref (here `1d68164`, the last-pushed-ish
  base), NOT current `HEAD`. The Sonnet B agent noticed and fast-forwarded (its
  diff applied cleanly); the Haiku C/D/E agents worked on the stale base, so
  their returned diffs reverted most of the session and were unusable. The Lead
  re-implemented C/D/E directly on `main` from the specs (the agents' work was
  reference only). Takeaway: for worktree fan-outs on an unpushed branch,
  verify each worktree's base or have agents fast-forward first.

## 2026-06-15 — Search query language (JQL-like) over field-prefix filters

The "Search filters" Later item was scoped to `field:value` prefix tokens, but
the user wanted a JQL-like DSL (`status = open AND project IN (harness-deck)`)
matching their Tesela/Jira muscle memory. Decided for a faithful JQL **subset**
(not full JQL): `= != ~ !~ IN "NOT IN"`, `created >/>=/</<=`, `AND/OR/NOT` +
parens (NOT>AND>OR), implicit-AND by juxtaposition, bare/quoted tokens as
full-text leaves. Dropped as YAGNI: ORDER BY, IS EMPTY/WAS, function values.

Key decisions:
- **Exact match, no aliases.** `=`/`IN` are EqualFold against literal values;
  `~` is substring. `status = open` returns nothing (the real statuses are
  draft/awaiting-review/answered/done) — the friction is paid back by
  autocomplete, which surfaces the real values as you type.
- **Server-side parser, not client.** Filtering must run where the data + the
  20-cap live so `status = answered` with no text can list all such reports
  (faceted browse) — a client filter could only narrow the ≤20 text results.
  Parser is Go (testable; repo has no JS test harness). Client tokenizes only
  to drive autocomplete; the raw query string is what's sent.
- **New `internal/query` package, pure.** Evaluates against a `Record`
  interface (Field cheap, Text lazy), no store/server import. **Lazy
  short-circuit**: a structural predicate that fails first never opens the
  report body — order-sensitive (structural-left), capped anyway by the
  server's memoized Text(). Net: filtered queries are *faster* than the old
  open-every-report scan.
- **`query.Schema()` is the single source of truth** for the field/operator
  matrix, shared by the parser and `/api/search/schema`. The first build put a
  static duplicate in the server; the Lead refactored to derive from the
  package (TestSchema asserts every advertised op parses) — silent
  autocomplete↔parser drift was otherwise un-test-caught.
- **Autocomplete CSS in deck.css, not aggregator.css** (the spec said
  aggregator.css). deck.css ships in BOTH the report-page and dashboard
  bundles; aggregator.css is dashboard-only — so the spec's file would have
  left the dropdown unstyled on report pages. The build agent caught the spec
  typo and corrected it; browser-verified styled on both surfaces.
- **Parse-error UX:** a mid-typing invalid query returns 200 `{matches:[],
  error}`; the palette keeps last-good results (no flash) and shows a low-key
  hint. Tab accepts a suggestion, Enter always opens the active result.

**Process:** built via a phased multi-agent workflow (build internal/query TDD →
3 parallel adversarial verifiers + repair loop → server → client → review +
health). 1 bug found+fixed in verify round 1; final review APPROVE, all findings
non-defects. Lead independently re-verified (build/test/vet/fmt/node-check + full
browser pass on dashboard & report page) before committing — subagent output is
not trusted on say-so. Spec/report: phases/search-query-language-{spec,report}.md.

## 2026-06-16 — Saved searches: Option A (palette pre-filled), localStorage

Pin a JQL query to a new SAVED sidebar section; clicking opens the Cmd+K palette
**pre-filled and run live** (`HDSearch.open(initialQuery)`). Chose A over an
inline results view (B, large: new view+routing+live-refresh) and an in-place
tree filter (C, scoped to tree rows only, no body/snippet matches) — A reuses
the entire existing search overlay and the shipped query language, smallest
surface. Mockup + sign-off: report `20260616-saved-searches-design`.

- **Storage = localStorage, mirroring HDPins**, not a server store. New
  `window.HDSaved` module (`saved.js`) is a structural twin of `tabs.js`: key
  `harness-deck:saved-searches`, `[{name,query}]`, dedup by query, MAX 30,
  fires `hd:saved-changed` → `renderTree` repaints. Saved-list UI is
  dashboard-only (like PINNED); the palette save works anywhere.
- **No digit shortcuts.** Pins own digits 2–9; reusing them collides. Saved
  rows activate by **click only** (carry `data-saved`, never `data-url`, so
  they stay out of tree-focus `.row.run` nav and the generic `[data-url]`
  handler). `Cmd+S`/keyboard activation are recorded follow-ups, not v1.
- **Save affordance = a ☆ button in the palette**, shown only when the query is
  non-empty AND parses clean (a `queryValid` flag set in `runQuery`'s
  no-error / parse-error / empty branches). Name via `prompt()` defaulting to
  the query (Enter = query-as-name). Button-only (no `Cmd+S`) to avoid fighting
  the browser save-page default.
- **Kept parity with pins on the `setItem`-throws-still-dispatches-event
  pattern** (adversarial reviewer flagged it). `tabs.js` does the same, and the
  repaint reflects the *actually-persisted* state (a failed add simply doesn't
  appear) — so it shows truth, not stale data. Fixing only `saved.js` would
  diverge from pins; left both as-is.
- **`.saved-del` goes red on button-hover, not row-hover** (superseding the
  spec's literal text). The ✕ is comment-gray when the row is hovered
  (discoverable) and red only when you're about to click it — the conventional
  delete-affordance pattern.

**Process:** Lead (Opus) wrote the spec; **Sonnet implemented**; **Haiku**
re-ran the build/test gate; **two Sonnet reviewers** did spec-conformance +
adversarial passes. All Go gates green. **Browser-verify (chrome-devtools) was
decisive** — it caught a bug all three static reviewers missed: `updateSaveBtn`
set `style.display = ""` on show, which falls back to the `.search-save`
`display:none` CSS default, so the ☆ *never appeared*. Fixed to set an explicit
`inline-block`. Lesson reaffirmed: runtime browser verification catches
CSS↔inline-style interactions static review cannot. Spec:
phases/saved-searches-spec.md.

## 2026-06-16 — Assessment Waves 1–3 (richer response loop, multi-select, timeline)

After a 7-agent feature assessment (verdict: core loop is feature-complete; gaps
= a shallow interactive layer + no cross-report narrative), shipped the top-4
picks as three waves, each Sonnet-impl + Haiku-build-gate + 2×Sonnet-review +
Opus adjudication & browser-verify. Commits `8e9a0cc` / `86c1841` / `a3459d0`.

- **Wave 1 — richer response loop.** Wired the latent `note` field (persisted +
  POSTed since forever but rendered nowhere); added an SSE `response` event so a
  live harness gets pushed the answer (the hub frame was generalized to carry an
  event name, default `change` — existing `broadcast("reports")` + live.js
  untouched); added notify `HD_RESPONSE_VALUE`/`HD_RESPONSE_JSON`; added the
  first **CI workflow** (`go test -race` on push/PR — there was none, which is
  how the v0.2.6→v0.2.7 backlog accreted ungated). Note input is on the
  button-based prompts only (choice/yes-no/decision/approval), not free-text ask
  (its answer is already prose). SSE payload is `{project,run,block,value}` (no
  note — it's in responses.json + HD_RESPONSE_JSON; kept the SSE frame lean).
- **Wave 2 — multi-select asks.** New ask `mode:"multi"` with `Values[]`
  alongside the joined `value` (additive, backward-compatible). Server
  canonicalizes: client sends `values[]`, server joins into `value`. Keyboard:
  digit N **toggles** checkbox N (never auto-submits), Enter submits.
  **Cross-feature regression caught by the adversarial reviewer + fixed:** Wave
  1's `.hd-note` (class `hd-input hd-note`) collided with triage.js's `input()`
  = `.hd-input` selector, so `i` wrongly focused the note on choice/approval
  panels. Narrowed `input()` to `.ask-text-input`. (Lesson: a new shared class
  can silently re-target an existing query — adversarial review across features
  earns its keep.)
- **Wave 3 — activity timeline.** A third dashboard view (`g l`, `?v=activity`):
  all non-archived reports, cross-project/harness, day-grouped newest-first,
  rows reuse the `[data-url]` click-nav. Created value from the existing corpus —
  zero new authoring by harnesses, no server changes. Adjudicated two reviewer
  findings: day grouping now uses the **local** day (was UTC `slice(0,10)`) to
  match the local `HH:MM` shown; removed a dead `:first-of-type` CSS rule. The
  cross-report `related[]` field (the other half of the "big bet") was
  **deferred** to keep the wave focused — see roadmap Next.

Browser-verified every wave end-to-end (chrome-devtools, isolated `central_dir`
fixtures + the real corpus for the timeline): note record/render + SSE events,
mouse + keyboard multi-select, timeline render + nav + no inbox/projects
regression. All Go gates green; model scorecard updated per wave.

## 2026-06-19 — Assessment Waves 4–9 (the rest of the backlog)

Finished the assessment backlog. Same harness throughout: Lead (Opus) writes
the spec + grounds it in the actual code, a Sonnet implements, a Haiku re-runs
the build gate, two Sonnet reviewers do conformance + adversarial passes, and
Opus adjudicates + independently verifies (browser via chrome-devtools for UI,
live functional checks for backend) before committing. Commits `9bdb6af` /
`72f6bcb` / `0b631e0` / `0753e86` / `e8380ab` / `9bb9e10`.

- **W4 — search-text cache.** `store.Entry.SearchText` (`json:"-"`, so out of
  /api/reports) computed once in loadEntry where the manifest is already parsed;
  the existing parse-cache carries it on cache-hit. `searchRecord.Text()` reads
  it instead of `store.Get()` + `BlockText()` — identical text, zero per-query
  disk I/O. Memory: report-count × body-size, documented.
- **W5 — cross-report `related[]`.** `RelatedReport{id, project?, rel?, label?}`;
  a "related" panel after the banner links `/r/{project}/{id}` (url.PathEscape;
  project/label default from the host report). Forward links to not-yet-
  published reports are valid. Added an empty-project link guard. The
  `related_to` JQL query stays future.
- **W6 — response history.** `Response.Prior []PriorAnswer` (recursion-free,
  newest-first, cap 20) + responses.json `version`. `Record()` prepends the
  superseded answer under recordMu. Additive; legacy files load fine. The
  answered state renders the prior chain (value · note · time).
- **W7 — scope in JQL + TLS-expiry.** `scope` wired through the single
  query.fields/fieldOrder source (parser + autocomplete) + store.Entry +
  Field/Text. `certExpiryWarning()` parses the cert NotAfter; Serve() logs a
  WARNING at startup + on a 24h ticker tied to Serve's lifetime via a done
  channel (no goroutine leak). `tags` in JQL deferred (needs a tags field +
  list-membership query support).
- **W8 — typed `card-grid` block.** The 4-place block add (struct+registry+
  BlockText, validate, template+defaultTitles, CONTRACT) enforced by
  TestRegistryCrossCheck. Reuses the shared `md` + `pills` templates (no
  safeHTML bypass). `.card-grid` uses `auto-fill` so a lone card keeps its
  width. Promotes the recurring html-escape lane-grid pattern.
- **W9 — frontend polish.** Inbox sort pivot (recent/asks/project,
  sessionStorage); per-project current-state/roadmap section collapse (▾/▸,
  sessionStorage); `Cmd+S` in the palette saves the current query (saveCurrent()
  factored, preventDefault stops browser save).

Recurring lesson held: adversarial Sonnet review caught real cross-feature/spec
issues (e.g. the W2 triage/note clash, the W6 prior-note omission, the W7
goroutine leak), but runtime-only defects (W3 UTC-vs-local grouping, the earlier
saved-search CSS bug) only fell to Opus browser-verify. The division of labor —
Sonnet implement + Sonnet adversarial + Haiku gate + Opus browser/functional
verify — is the cost-efficient sweet spot for bounded Go+frontend features.
`tags`-in-JQL is the only assessment sliver left (roadmap Next).

## 2026-06-20 — Wave 10: tags + multi-value query fields

The deferred sliver, and the first **multi-value** query field. Design choices:

- **Record interface gains `Fields(name) []string`** (alongside `Field`) rather
  than encoding a list into the single-value `Field` string. A `listFields` set
  marks which fields are multi-valued; `eval` branches to existential semantics
  for them and leaves every single-value path byte-for-byte unchanged.
- **Existential semantics** (the load-bearing decision): `tags = X` / `IN` match
  when ANY tag satisfies; `tags != X` / `NOT IN` / `!~` match when NO tag
  satisfies. Corollary: an **untagged** report matches every negative predicate
  (`tags != x` is vacuously true) — documented, and the natural read of "has no
  tag equal to x". This is the classic negation-over-list trap; locked with
  unit tests for all six ops incl. NOT IN / !~ + empty-tags, and live-verified
  end-to-end through JSON→Entry→Fields→evalList→Match.
- **Autocomplete dedup stays case-sensitive** (as-stored), consistent with
  project/kind/scope — the matcher is EqualFold but the value list shows the
  literal tags; normalizing only tags would diverge from the sibling fields.
- Tags render as chips on the report page; join into the free-text `Text()` +
  the metadata score pass (so a tag-matching text term scores like a metadata
  hit, not a body hit).

This **completes the assessment backlog** (Waves 1–10). Waves 1–9 in v0.2.8;
Wave 10 unreleased.

## 2026-06-26 — opencode usage tile: local `opencode stats`, not the web cookie

The opencode footer tile scraped opencode.ai's web `_server` RPC with a manually-
pasted, expiring browser `auth` cookie + build-fingerprint hashes — fragile by its
own admission. Brainstormed a better source.

- **Investigated how CodexBar does it** (read its app binary):
  `OpenCodeProviderImplementation.swift` hits the **same** `https://opencode.ai/_server`
  with a **session cookie** — there is **no api-key/REST endpoint** for the Zen
  plan-% (the opencode-go api key is for model *inference*, not billing; my
  `api.opencode.ai/billing/usage` guess was GitHub/OpenAI strings bleeding into
  the opencode binary). CodexBar's only edge is `OpenCodeCookieImporter` —
  auto-importing the cookie from the browser + Keychain, a native-macOS trick.
- **Conclusion:** the Zen plan-% is cookie-only. Auto-importing encrypted browser
  cookies (Chrome AES+Keychain / Safari binarycookies + a SQLite dep) is OS-
  specific and against harness-deck's portable, zero-dep, no-browser nature.
- **Decision (user-chosen):** switch the tile to the **local** `opencode stats
  --days N` CLI (which OpenCode now ships — the old "no usage API" note was
  stale), reported as a `KindBudget` tile ("OC $X"). Robust (local DB, no auth,
  unbreakable), dependency-free (shell out, parse). **Trade-off accepted:** it
  shows local *spend*, not the plan-%, because spend is the only thing available
  headlessly without the cookie. A cheap/free *model* was never needed — fetching
  usage is an auth problem, and the local CLI sidesteps auth.
- TUI-hang guard: `cmd.Stdin` left nil (= /dev/null) so a terminal-capability
  query can't wedge the poll; `exec.CommandContext` lets the 12s Monitor timeout
  kill a hung child. Parser is pure + unit-tested (binary-free). `opencode_cookie`
  / `opencode_workspace_id` config kept parseable but deprecated.

Commit `900f137`. Sonnet-impl + Haiku-gate + Sonnet-review + Opus live-verify
(/api/usage returns the budget tile end-to-end).

**Landmine (fixed `e977d7e`):** any usage provider that shells out to a CLI must
NOT rely on `exec.LookPath` alone — the dashboard runs under launchd/systemd
with a minimal PATH that omits `/opt/homebrew/bin` & `~/.local/bin`, so a bare
lookup fails even when the tool is installed. `opencodeBin()` probes common
install dirs and execs the absolute path. v0.2.10 shipped with this bug (caught
immediately by the post-release live `/api/usage` check, NOT by the workflow —
the in-shell test had Homebrew on PATH); v0.2.11 carries the fix.

## 2026-06-28 — opencode usage tile disabled behind a feature flag

The local-`opencode stats` redesign (v0.2.10/v0.2.11) was the **wrong data
source** for this user's actual usage, surfaced by the post-release live check:
the tile read **$0.00 / 0 sessions** even though opencode is used heavily.

**Root cause (investigated, confirmed):** `opencode stats` tallies only local
opencode *TUI* sessions (`~/.local/share/opencode/storage`). This machine had 4
local sessions, newest **Feb 6, 2026** (~141 days stale). The real spend runs
through the **opencode-go / Zen cloud plan** (`auth.json` has an `api`-type
`opencode-go` key serving `opencode-go/*` models — glm-5, kimi-k2.6, …), driven
by `orchestra`/`pi`, which call the cloud API **directly** and never write local
session files. That usage is **account-scoped on `opencode.ai/zen`** (binary
strings show `credits` / `addCredits`) and is not exposed by any local CLI/SDK/
config surface (checked the vendored `@opencode-ai/sdk` — local-server client
only, no billing/usage methods). CodexBar reads it via the **browser session**,
which the user explicitly rejected.

**Decision:** drop the opencode tile from the live footer (codex + claude-code
only) but keep the provider code behind a feature flag rather than deleting it.
New `usage.opencode_enabled` (config) → `Options.OpenCodeEnabled` (off by
default). Listing `"opencode"` in `providers` now does nothing unless the flag
is also set — `Build`'s `opencode` case is gated. Re-enabling later is a
one-line flag flip, no code change.

**Why a flag, not a delete:** the work is sound for *local* TUI users and the
investigation is captured here; a future revisit (headless Zen-API endpoint with
the stored opencode-go key, or the web session) flips the flag back on. Test
`TestOpenCodeFeatureFlagged` pins the gate. `opencode_cookie` /
`opencode_workspace_id` / `opencode_days` stay parseable.

**Open follow-up (roadmap Later):** a real cloud-Zen usage source. Unknown
whether the opencode-go api key can read account balance/usage headlessly, or
whether that endpoint requires the web session (CodexBar's path). One
authenticated probe with the stored key would answer it — deferred.
