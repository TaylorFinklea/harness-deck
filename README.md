<p align="center">
  <img src="harness-deck.png" alt="harness-deck" width="128" />
</p>

# harness-deck

A unified dashboard for AI coding work across multiple harnesses (Claude Code,
Pi Mono, OpenCode, …) and across many projects.

Any harness writes a structured **report manifest** (`report.json`); harness-deck
renders it into a consistent, terminal-styled HTML report and aggregates every
report across every project into one live dashboard. Reports can ask for your
opinion, surface a decision, or share an idea — and your responses flow back to
the harness as a `responses.json` file plus a notification.

## Screenshots

The aggregator dashboard — every report across every project and every harness,
with an inbox of the items that are awaiting your review:

![The harness-deck aggregator dashboard](docs/screenshots/dashboard.png)

A rendered report. The renderer turns a JSON block manifest into a consistent,
terminal-styled page — so every report looks like this, and old reports restyle
when the renderer changes:

![A rendered harness-deck report](docs/screenshots/report.png)

The projects view auto-discovers every project under your `scan_roots` (e.g.
`~/git`) and shows each one's `.docs/ai/current-state.md` and `roadmap.md`
together, alongside any reports published with `kind: "roadmap"`. A "tracked
projects" panel lets you uncheck the ones you don't care to follow:

![The harness-deck projects view](docs/screenshots/roadmap.png)

## Status

Early build. See [`.docs/ai/roadmap.md`](.docs/ai/roadmap.md) for the phased plan
and [`.docs/ai/current-state.md`](.docs/ai/current-state.md) for the latest
breadcrumb.

## How it works

```mermaid
flowchart TD
  H["AI harness<br/>Claude Code · Pi Mono · OpenCode · Codex"]
  H -->|"1 · publish a report"| R[("report.json<br/>in a run directory")]

  subgraph DECK["harness-deck serve — local Go server"]
    direction TB
    ST["store<br/>scans + watches every 2s"]
    RN["render<br/>manifest → themed HTML"]
    SR["HTTP + SSE"]
    ST --> SR
    RN --> SR
  end

  R -->|"2 · discovered"| ST
  SR -->|"3 · live dashboard + report pages"| B["Browser<br/>TUI dashboard"]
  B -->|"4 · approve / answer / decide"| SR
  SR -->|"5 · record answer + fire notify"| RESP[("responses.json<br/>beside the report")]
  RESP -.->|"6 · picked up next session"| H
```

- **Authoring contract:** a JSON block manifest (`report.json`) — an ordered list
  of typed blocks. The renderer owns all HTML/CSS, so every report looks
  consistent and old reports restyle when the renderer changes. A raw-`html`
  block is the escape hatch for UI the vocabulary doesn't cover yet. See
  [`CONTRACT.md`](CONTRACT.md) for the full schema, or
  [`docs/PUBLISHING.md`](docs/PUBLISHING.md) for a walkthrough aimed at tools
  that want to publish into the dashboard.
- **Visual design:** the "v1 TUI dashboard" — terminal chrome, sidebar tree +
  table of contents + run metadata, stacked content panels, vim navigation,
  Tokyo Night theme.

## Why this exists

AI coding agents are good at editing repos, but their human-facing artifacts can
sprawl: Markdown reports in chat, one-off HTML mockups, decisions buried in
scrollback, and review notes that disappear with the session. harness-deck gives
those artifacts one local place to land.

Think of it as a pane of glass for agent output. Chat is where you talk to the
agent; harness-deck is where the agent shows you things: reports, mockups,
comparisons, approvals, and decisions that should survive a context clear.

The launch article draft lives in
[`docs/launch/medium-pane-of-glass.md`](docs/launch/medium-pane-of-glass.md).

## Install

```sh
# Homebrew (macOS / Linux) — installs `harness-deck` and the short alias `hdeck`
brew install taylorfinklea/tap/harness-deck

# Go 1.26+
go install github.com/TaylorFinklea/harness-deck/cmd/harness-deck@latest
```

After `brew tap taylorfinklea/tap`, the bare `brew install harness-deck` also
works. Or download a prebuilt binary from the
[releases page](https://github.com/TaylorFinklea/harness-deck/releases).

## Host setup

If you are an AI agent asked to "set up harness-deck", follow
[`docs/SETUP.md`](docs/SETUP.md). It covers OS detection, Homebrew install,
`~/.config/harness-deck/config.json`, macOS LaunchAgent persistence, Linux
systemd user-service persistence with `loginctl enable-linger`, verification,
and optional Tailscale HTTPS / PWA push.

The short local-only setup is:

```sh
brew install taylorfinklea/tap/harness-deck
mkdir -p ~/.config/harness-deck
cat > ~/.config/harness-deck/config.json <<'JSON'
{
  "central_dir": "~/.harness/reports",
  "scan_roots": ["~/git"],
  "bind": "127.0.0.1",
  "port": 7420
}
JSON
hdeck serve
```

Use `scan_roots` for parent directories like `~/git`; harness-deck discovers
depth-1 children that contain `.docs/ai/`. For repos outside those roots, run
`hdeck register /path/to/project`.

## Build & run

```sh
go build ./...               # build from source
./harness-deck serve         # start the local dashboard
./harness-deck open          # open the dashboard in a dedicated app window
./harness-deck validate report.json
./harness-deck render report.json -o out.html
./harness-deck new --title "first report"  # scaffold a starter report.json
./harness-deck register /path/to/project   # add a project root to the config
./harness-deck vapid         # generate the VAPID keypair for phone push (one-time)
./harness-deck version       # print build metadata
```

## Mobile (PWA) + push notifications

harness-deck is also a PWA. Open it on your phone over Tailscale, tap _Add to
Home Screen_, and it installs as a standalone app. iOS lock-screen push
notifications work too — the requirements are tailnet reachability and HTTPS:

```jsonc
// ~/.config/harness-deck/config.json
{
  "bind": "0.0.0.0",                        // or your Tailscale IP
  "tls": {
    "cert": "/Users/me/laptop.tailnet.crt", // tailscale cert <host>
    "key":  "/Users/me/laptop.tailnet.key"
  }
}
```

Then `harness-deck vapid` once to generate the push identity, restart the
server, open the dashboard on your phone, visit the **settings** view, and
tap _Enable notifications on this browser_. New asks land on the lock screen
and deep-link straight to the report.

For exact Tailscale cert, `public_url`, service restart, and renewal notes, see
[`docs/SETUP.md`](docs/SETUP.md#6-optional-tailscale-https-pwa-and-push).

## Notification fan-out

Alongside Web Push, harness-deck can fan every new ask out to Slack, Discord,
or any HTTP webhook (n8n, Zapier, custom). Add destinations from the
**settings** view (or directly to `config.json`):

```jsonc
// ~/.config/harness-deck/config.json
{
  "public_url": "https://scadrial.tailceb58.ts.net:7420",
  "notifications": [
    {"name": "team-alerts", "type": "slack",   "url": "https://hooks.slack.com/services/…"},
    {"name": "my-server",   "type": "discord", "url": "https://discord.com/api/webhooks/…"},
    {"name": "n8n",         "type": "webhook", "url": "https://n8n.example.com/…",
     "projects": ["work-project"]}
  ]
}
```

`public_url` is the URL the chat-side link should point at — without it the
fallback is `<bind>:<port>` which won't resolve when `bind` is `0.0.0.0`.
Per-destination `projects` is an optional allowlist; omit it to fire for
every project. Sends are fire-and-forget with a per-attempt log line — no
retry queue, on the principle that the next still-open ask re-fires
automatically.

## License

MIT — see [LICENSE](LICENSE).
