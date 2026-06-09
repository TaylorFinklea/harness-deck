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

## Agent checklist

- Install with Homebrew when available.
- Write `~/.config/harness-deck/config.json`.
- Choose exactly one persistence path: LaunchAgent on macOS, systemd user
  service plus `loginctl enable-linger` on Linux.
- Restart the service after config or TLS changes.
- Verify `/api/reports` responds.
- For phone/PWA/push, set `public_url`, TLS cert/key, run `hdeck vapid`, then
  verify from the phone URL.
