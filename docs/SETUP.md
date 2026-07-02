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

- `Darwin` uses Homebrew plus a user LaunchAgent for persistence.
- Linux with `systemctl --user` uses Homebrew plus a user systemd service.
- Do not install Homebrew itself without the user's approval. If Homebrew is
  absent, use `go install` or a release binary only after confirming that path.

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
depth-1 children that contain `.docs/ai/`. For a repo outside those roots, run:

```sh
hdeck register /path/to/project
```

## 4. Install persistence

### macOS: LaunchAgent

```sh
HD_BIN="$(command -v hdeck)"
if [ -z "$HD_BIN" ]; then HD_BIN="$(command -v harness-deck)"; fi
mkdir -p "$HOME/Library/LaunchAgents" "$HOME/Library/Logs/harness-deck"
cat > "$HOME/Library/LaunchAgents/com.tfinklea.harness-deck.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.tfinklea.harness-deck</string>
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
launchctl bootout "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.tfinklea.harness-deck.plist" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$HOME/Library/LaunchAgents/com.tfinklea.harness-deck.plist"
launchctl enable "gui/$(id -u)/com.tfinklea.harness-deck"
launchctl kickstart -k "gui/$(id -u)/com.tfinklea.harness-deck"
```

Check it:

```sh
launchctl print "gui/$(id -u)/com.tfinklea.harness-deck"
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
```

Enable linger so the user service survives logout and can start without an
active interactive session:

```sh
sudo loginctl enable-linger "$USER"
loginctl show-user "$USER" -p Linger
```

Check it:

```sh
systemctl --user status harness-deck.service
journalctl --user -u harness-deck.service -n 40 --no-pager
```

## 5. Verify

```sh
hdeck open --print
curl -fsS "$(hdeck open --print)/api/reports" >/dev/null
```

If the dashboard is local-only, open `http://127.0.0.1:7420`. If `public_url`
is set, `hdeck open --print` should return that URL.

## 6. Optional: Tailscale HTTPS, PWA, and push

iOS push notifications require HTTPS. For a Tailscale host:

```sh
TS_HOST="HOSTNAME.tailnet.ts.net"
mkdir -p ~/.config/harness-deck/tls
tailscale cert \
  --cert-file "$HOME/.config/harness-deck/tls/${TS_HOST}.crt" \
  --key-file "$HOME/.config/harness-deck/tls/${TS_HOST}.key" \
  "$TS_HOST"
hdeck vapid
```

Then configure:

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
the home screen, then enable notifications in the settings view. `tailscale
cert` file output is not automatically renewed by harness-deck; renew or
automate it separately if the host needs long-lived HTTPS.

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

- Install with Homebrew when available.
- Write `~/.config/harness-deck/config.json`.
- Choose exactly one persistence path: LaunchAgent on macOS, systemd user
  service plus `loginctl enable-linger` on Linux.
- Restart the service after config or TLS changes.
- Verify `/api/reports` responds.
- For phone/PWA/push, set `public_url`, TLS cert/key, run `hdeck vapid`, then
  verify from the phone URL.
