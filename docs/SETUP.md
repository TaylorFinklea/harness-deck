# Setting up harness-deck on a host

Use this runbook when a user says "set up harness-deck" or asks an agent to
install it on a new machine. The goal is a running dashboard that starts on
login/boot, scans the user's repos, and can be reached from the right browser
or phone URL.

## 1. Detect the host

```sh
uname -s
command -v brew
command -v hdeck || command -v harness-deck
```

- Both macOS and Linux use Homebrew plus `brew services` for persistence
  (hand-rolled LaunchAgent/systemd units only for non-Homebrew installs —
  Appendix A).
- **Already installed?** If `hdeck version` reports a version, this is an
  upgrade, not an install: run `brew upgrade harness-deck` instead of `brew
  install`, and still do steps 4–5 (a host set up before v0.2.14 has a
  hand-rolled service unit that must be removed — see step 4).
- **No Homebrew?** Do not install Homebrew without the user's approval. Ask
  first, and offer `go install` (below) as the alternative. Binaries built by
  `go install` are ad-hoc signed, so on macOS they can be silently blocked by
  the Application Firewall — `hdeck doctor` catches that and prints the fix.

## 2. Install the binary

```sh
brew install taylorfinklea/tap/harness-deck
hdeck version
```

The Homebrew formula installs the canonical `harness-deck` binary and the short
`hdeck` symlink. After `brew tap taylorfinklea/tap`, `brew install
harness-deck` is equivalent.

Fallback if Homebrew is not available:

```sh
go install github.com/TaylorFinklea/harness-deck/cmd/harness-deck@latest
harness-deck version
```

`go install` does not create the `hdeck` alias.

## 3. Write the config

Optional — every field defaults and harness-deck runs with no config file at all
(`127.0.0.1:7420`, reports in `~/.harness/reports`). Write one to change
something. The usual reason is `scan_roots`, which has no default: without it the
projects view stays empty.

Local-only dashboard:

```sh
mkdir -p ~/.config/harness-deck
cat > ~/.config/harness-deck/config.json <<'JSON'
{
  "central_dir": "~/.harness/reports",
  "scan_roots": ["~/git"],
  "bind": "127.0.0.1",
  "port": 7420
}
JSON
```

Dashboard reachable over Tailscale, without push notifications:

```json
{
  "central_dir": "~/.harness/reports",
  "scan_roots": ["~/git"],
  "bind": "0.0.0.0",
  "port": 7420,
  "public_url": "http://HOSTNAME.tailnet.ts.net:7420"
}
```

Use `scan_roots` for parent directories like `~/git`. harness-deck discovers
depth-1 children that contain `.docs/ai/` — override the marker with
`project_markers` if your repos follow a different convention (any listed
path, directory or file, qualifies a child):

```json
{ "scan_roots": ["~/git"], "project_markers": [".docs/ai", ".beads", ".git"] }
```

For a repo outside those roots, run:

```sh
hdeck register /path/to/project
```

Other fork/self-host knobs: `push_subject` sets the contact embedded in Web
Push VAPID JWTs (defaults to the harness-deck repo URL — set your own if you
fork or self-host). It must be an `https://` URL or a `mailto:` address —
validated at load time, since push services (Apple especially) reject other
forms only at delivery time. The config file itself lives at
`~/.config/harness-deck/config.json`, or under `$XDG_CONFIG_HOME/harness-deck/`
when that variable is set (absolute paths only; an existing legacy
`~/.config` install keeps winning until a config exists at the XDG location);
`HARNESS_DECK_CONFIG` overrides both.

## 4. Install persistence

**First, remove any hand-rolled unit from a previous setup.** Older revisions of
this runbook had you write a LaunchAgent/systemd unit by hand. If one is left
behind it will start a *second* server at login and fight the Homebrew service
for the port; the loser crash-loops. Homebrew's own unit is
`homebrew.mxcl.harness-deck` — leave that one alone, remove anything else:

```sh
# macOS — old labels include com.tfinklea.harness-deck and com.harnessdeck.serve
ls ~/Library/LaunchAgents | grep -i harness      # anything not homebrew.mxcl.* is stale
launchctl bootout "gui/$(id -u)/<stale-label>" 2>/dev/null || true
rm -f ~/Library/LaunchAgents/<stale-label>.plist  # -f: a bare `rm` may prompt and silently no-op
ls ~/Library/LaunchAgents | grep -i harness      # verify it is actually gone

# Linux
systemctl --user disable --now harness-deck.service 2>/dev/null || true
rm -f ~/.config/systemd/user/harness-deck.service
```

Then start the service. Homebrew installs (formula v0.2.14+) ship a service
definition, so this is the whole step on both macOS and Linux:

```sh
brew services start harness-deck
```

That is the whole step, on both macOS (launchd) and Linux (systemd). Check it
with `brew services info harness-deck`; logs land in
`$(brew --prefix)/var/log/harness-deck.log`.

On Linux, additionally enable linger so the user service survives logout and
starts without an interactive session:

```sh
sudo loginctl enable-linger "$USER"
loginctl show-user "$USER" -p Linger
```

Installed via `go install` or a raw release binary — or on a formula older
than v0.2.14? Use the hand-rolled units in
[Appendix A](#appendix-a-persistence-without-brew-services).

## 5. Verify

```sh
hdeck doctor
```

`doctor` (v0.2.15+) checks the config (including `usage.providers` typos and an
empty `scan_roots`, both otherwise silent), TLS cert validity/expiry/hostname,
VAPID presence, leftover hand-rolled service units, whether the server actually
answers — including on the non-loopback interface, the path a phone uses — and,
on macOS, whether the Application Firewall is dropping inbound connections.
Every failure prints the exact fix. `--json` for agents; exit 1 on any FAIL.

`brew services start` is asynchronous: if doctor reports `nothing listening`
immediately after you started the service, wait a second and rerun once before
treating it as a real failure.

On v0.2.14, `doctor` graded a stopped server as a warning and still exited 0 —
so on that version, don't treat exit 0 alone as proof the install works.

Manual equivalent:

```sh
hdeck open --print
curl -fsS "$(hdeck open --print)/api/reports" >/dev/null
```

If the dashboard is local-only, open `http://127.0.0.1:7420`. If `public_url`
is set, `hdeck open --print` should return that URL.

## 6. Optional: Tailscale HTTPS, PWA, and push

iOS push notifications require HTTPS. For a Tailscale host (v0.2.14+):

```sh
hdeck cert        # issue the cert, write it to the config dir, patch config.json
hdeck vapid       # one-time push identity
brew services restart harness-deck
hdeck doctor      # confirm everything, including the phone path
```

`hdeck cert` resolves this machine's MagicDNS name, obtains the cert + key,
stores them under `~/.config/harness-deck/tls/`, and fills in `tls.cert`,
`tls.key`, and (when unset) `public_url`. Renewal: certs live 90 days and
nothing renews them automatically — schedule `hdeck cert --renew` (a no-op
while the cert is valid >30 days) in cron/launchd, or rerun it when `hdeck
doctor` warns.

Why not `tailscale cert --cert-file <path>` directly? The Mac App Store build
of Tailscale is sandboxed and cannot write to any location outside its
container — it fails with `operation not permitted` for every path, `/tmp`
included. `hdeck cert` reads the PEM over stdout and does the writing itself,
which works on both the App Store and standalone builds. On a pre-v0.2.14
binary, do the same by hand:

```sh
TS_HOST="HOSTNAME.tailnet.ts.net"   # find yours: tailscale status --json | grep DNSName
TLS_DIR="$HOME/.config/harness-deck/tls"
mkdir -p "$TLS_DIR"
tailscale cert --cert-file - --key-file - "$TS_HOST" \
  | awk -v crt="$TLS_DIR/$TS_HOST.crt" -v key="$TLS_DIR/$TS_HOST.key" '
      /-----BEGIN .*PRIVATE KEY-----/ { out = key }
      { if (out == key) print > key; else print > crt }'
chmod 600 "$TLS_DIR/$TS_HOST.key"
```

If `tailscale` is not on your `PATH`, the App Store build's CLI lives at
`/Applications/Tailscale.app/Contents/MacOS/Tailscale`.

### macOS firewall: the dashboard loads locally but times out from the phone

If `https://127.0.0.1:7420` works on the Mac but the tailnet URL times out
from every other device, the macOS Application Firewall is dropping the
connections. The firewall auto-allows Apple/Developer-ID-signed software;
for other binaries its verdict is per-binary and stateful, and a service
started by launchd cannot show the "accept incoming connections?" dialog —
the observed result is a silent timeout: no error, no log entry. Ad-hoc
signed binaries (releases before signing landed, and anything built locally
with `go install`) are the ones at risk.

`hdeck doctor` detects this — it dials the running server on a non-loopback
interface, exactly what a phone does — and prints the fix:

```sh
HD_BIN="$(readlink -f "$(command -v harness-deck)")"
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add "$HD_BIN"
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp "$HD_BIN"
```

The allowlist entry pins the resolved binary path (for Homebrew that is a
versioned Cellar path), so a `brew upgrade` installs a new binary that is not
allowlisted — rerun `hdeck doctor` after upgrading if the phone URL stops
responding.

`hdeck cert` writes this configuration for you; done by hand it looks like:

```json
{
  "central_dir": "~/.harness/reports",
  "scan_roots": ["~/git"],
  "bind": "0.0.0.0",
  "port": 7420,
  "public_url": "https://HOSTNAME.tailnet.ts.net:7420",
  "tls": {
    "cert": "~/.config/harness-deck/tls/HOSTNAME.tailnet.ts.net.crt",
    "key": "~/.config/harness-deck/tls/HOSTNAME.tailnet.ts.net.key"
  }
}
```

Restart the persistent service, open `public_url` on the phone, add the PWA to
the home screen, then enable notifications in the settings view.

## 7. Optional: harness publishing integration

Agents can always publish by writing `report.json` under
`<repo>/.harness/<run>/` or `~/.harness/reports/<project>/<run>/`.
MCP publishing is optional and does not require the dashboard server to be
running.

The report schema travels with the binary, so an agent never needs to clone
this repo to learn it:

- `hdeck contract` prints the full contract (`--publishing` prints the gentler
  walkthrough).
- Over MCP, the server exposes it as the `harness-deck://contract` resource
  (plus `harness-deck://publishing`), and its `initialize` handshake returns
  short instructions on when to publish.
- A running dashboard also serves it at `GET /contract.md`.

Claude Code:

```sh
claude mcp add harness-deck -- hdeck mcp
```

Other MCP-capable harnesses, including Codex, should register a stdio MCP
server named `harness-deck` with command `hdeck` and args `["mcp"]`, or command
`harness-deck` and args `["mcp"]` when installed without Homebrew.

## 8. Optional: usage monitors (footer)

The dashboard footer can show CodexBar-style usage for AI coding tools next to
the address. It is **opt-in**: nothing reads credentials or hits the network
unless a provider is listed in `usage.providers`. A provider with no data or
credential simply drops off the footer. Window-kind providers (codex,
claude-code) render a small progress bar per rate-limit window — the **5-hour
and the weekly limit** — severity-colored (≥90% red, ≥70% yellow); budget-kind
providers show a text value.

```jsonc
"usage": {
  "providers": ["codex", "openrouter", "claude-code", "copilot"],
  "openrouter_key": "",            // else read from $OPENROUTER_API_KEY
  "opencode_days": 7,              // rolling window for opencode spend tile (default 7)
  "opencode_enabled": false,       // feature flag for the opencode tile (default off)
  "refresh_sec": 60                // poll cadence (default 60)
}
```

`opencode_cookie` and `opencode_workspace_id` are **deprecated and ignored** —
they were used by the old web-scrape approach. Remove them from your config if
present.

The **`opencode` tile is feature-flagged off by default** (`opencode_enabled:
false`). `opencode stats` only counts local opencode *TUI* sessions, so the tile
reads `$0` for anyone whose real spend runs through the opencode-go/Zen **cloud**
plan (e.g. driven by `orchestra`/`pi`) — that usage is account-scoped on
opencode.ai and invisible to the local CLI. Listing `"opencode"` in `providers`
does nothing unless you also set `opencode_enabled: true`; only do that if you
use the local opencode TUI and want its local spend. See `.docs/ai/decisions.md`.

Per provider:

| id | shows | setup |
|---|---|---|
| `codex` | true 5h + weekly % + reset | none — reads `~/.codex` session logs |
| `openrouter` | credit budget + spend | `OPENROUTER_API_KEY` env, or `usage.openrouter_key` |
| `claude-code` | true 5h + weekly % + reset | reads the OAuth token from the macOS **Keychain** (`security`) — one "Always Allow" prompt on first read, silent after. `$CLAUDE_CODE_OAUTH_TOKEN` or file creds (`~/.claude/.credentials.json`) avoid the prompt. |
| `copilot` | premium-request % + monthly reset | reads the local `ghu_` token (`~/.config/github-copilot/apps.json`). **Uses GitHub's undocumented `copilot_internal/user` endpoint — may be against Copilot ToS for non-official clients; opt in knowingly.** |
| `opencode` | N-day cumulative **local** spend (cost) | **feature-flagged off** (`opencode_enabled`, default false) — local TUI sessions only, blind to opencode-go/Zen cloud spend. When enabled: shells out to `opencode stats --days N` (default 7, via `opencode_days`); no cookie/network; degrades gracefully when the binary is absent. |

Restart the service after changing `usage`. Verify with `GET /api/usage`.

## 9. Optional: beads Backlog view

The dashboard can show a read-only **Backlog** view over the [beads](https://github.com/steveyegge/beads)
(`bd`) issue tracker — the priority-sorted ready queue, blocked items with their
blockers, and a per-repo dependency graph, with drill-in to one issue. It is
**opt-in** and requires the `bd` binary on `PATH` (or a common install dir).

```jsonc
"beads": {
  "enabled": true,       // off by default; shows the Backlog view (g b / :backlog)
  "writable": false,     // off by default; required to claim/close/create from the UI
  "refresh_sec": 15      // how often bd is re-read across repos (default 15)
}
```

- **Discovery** is by a `.beads/` directory: every depth-1 child of a `scan_roots`
  entry (plus explicit `projects`) that holds `.beads/` appears — independent of
  the `.docs/ai` keying used by the projects view, so a repo with beads but no
  `.docs/ai` still shows up.
- **Never touches `.beads/` directly** (it is a binary Dolt DB); all reads go
  through `bd … --json`.
- **Graceful degradation:** if `bd` is not installed the view is dark and
  `/api/beads` reports `available:false`; a repo whose `bd` calls fail is shown
  with its error and does not sink the others.
- Live-refreshes over SSE (a `beads` event) like the rest of the dashboard.
- **Actions are opt-in** via `beads.writable` (default false). With it off the
  view is strictly read-only — no action buttons render and the write endpoints
  return 403, so it's safe to browse from a phone. With it on, the drill-in panel
  gains Claim + Close (with a reason) and each repo card a `+ new` create form;
  keyboard `c`/`x`/`n` on the focused row. Restart to apply a `writable` change.

Restart the service after changing `beads`. Verify with `GET /api/beads` and the
`g b` view in the dashboard.

## Agent checklist

- Check for an existing install (`hdeck version`) — if present, `brew upgrade`,
  not `brew install`.
- Install with Homebrew when available; ask before installing Homebrew itself.
- **Remove any leftover hand-rolled service unit** (step 4) before starting the
  service. `hdeck doctor` FAILs on one, but check first — two servers fighting
  over the port is the most confusing failure mode there is.
- The config file is **optional** (everything defaults). Write one only to
  change something. The usual reason is `scan_roots` — ask the user where their
  repos live (e.g. `~/git`); without it the projects view is empty and doctor
  warns.
- Persistence: `brew services start harness-deck` (both OSes; plus `sudo
  loginctl enable-linger "$USER"` on Linux). Hand-rolled units (Appendix A)
  only for go-install/raw-binary hosts.
- Restart the service after config or TLS changes.
- Run `hdeck doctor` and clear every FAIL. It covers the server actually
  answering (including on the phone-facing interface), TLS, push keys, leftover
  units, and the macOS firewall. `brew services start` is async — if doctor says
  `nothing listening` right after starting it, wait a second and rerun once.
- For phone/PWA/push: `hdeck cert`, `hdeck vapid`, restart, `hdeck doctor`,
  then verify from the phone URL.
- Over SSH / headless, use `hdeck open --print` (prints the URL) rather than
  `hdeck open`, which wants a GUI session.

## Appendix A: persistence without brew services

For `go install` or raw-binary hosts (or a formula older than v0.2.14). Note
that locally built binaries are ad-hoc signed — after starting the service,
run `hdeck doctor` to catch the macOS firewall silently dropping the
phone path (see the firewall section above).

### macOS: LaunchAgent

```sh
HD_BIN="$(command -v hdeck)"
if [ -z "$HD_BIN" ]; then HD_BIN="$(command -v harness-deck)"; fi
mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs/harness-deck"
cat > "$HOME/Library/LaunchAgents/com.harnessdeck.serve.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.harnessdeck.serve</string>
  <key>ProgramArguments</key>
  <array>
    <string>${HD_BIN}</string>
    <string>serve</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>${HOME}/Library/Logs/harness-deck/stdout.log</string>
  <key>StandardErrorPath</key>
  <string>${HOME}/Library/Logs/harness-deck/stderr.log</string>
</dict>
</plist>
EOF
launchctl bootout "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.harnessdeck.serve.plist" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.harnessdeck.serve.plist"
launchctl enable "gui/$(id -u)/com.harnessdeck.serve"
launchctl kickstart -k "gui/$(id -u)/com.harnessdeck.serve"
```

Check it:

```sh
launchctl print "gui/$(id -u)/com.harnessdeck.serve"
tail -n 40 "$HOME/Library/Logs/harness-deck/stderr.log"
```

### Linux: systemd user service

```sh
HD_BIN="$(command -v hdeck)"
if [ -z "$HD_BIN" ]; then HD_BIN="$(command -v harness-deck)"; fi
mkdir -p ~/.config/systemd/user
cat > ~/.config/systemd/user/harness-deck.service <<EOF
[Unit]
Description=harness-deck dashboard

[Service]
Type=simple
ExecStart=${HD_BIN} serve
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF
systemctl --user daemon-reload
systemctl --user enable --now harness-deck.service
sudo loginctl enable-linger "$USER"
```

Check it:

```sh
systemctl --user status harness-deck.service
journalctl --user -u harness-deck.service -n 40 --no-pager
```
