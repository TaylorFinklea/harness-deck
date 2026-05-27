package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Notification is one in-flight notification fan-out payload. Fields
// mirror push.Payload by design — the same upstream event (a new
// unanswered ask) fans out to Web Push AND any configured destinations,
// and using the same shape lets the watcher build one source-of-truth.
//
// notify is deliberately decoupled from internal/push (separate concern,
// no import dependency) so the package can be reused for fan-out paths
// that have nothing to do with Web Push later.
type Notification struct {
	Title   string
	Body    string
	URL     string // absolute, externally-reachable link to the report
	Tag     string // dedup key (project:run:block-id)
	Project string
	Run     string
}

// Destination is one configured notification destination. Loaded from
// config.json and round-trippable via the settings-view CRUD endpoints.
// Name is the user-facing label (also used in log lines + the dedup key
// for the test endpoint). Projects is an optional allowlist — when empty
// the destination fires for every project.
type Destination struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`               // "slack" | "discord" | "webhook"
	URL      string   `json:"url"`                // POST target
	Projects []string `json:"projects,omitempty"` // optional allowlist
}

// validTypes is the closed set of destination types v1 supports. New
// types (email, ntfy, pushover, …) add a case in send + an entry here.
var validTypes = map[string]bool{
	"slack":   true,
	"discord": true,
	"webhook": true,
}

// Validate checks one destination at config load (and on POST). Returns
// the first problem found so the user can fix one thing at a time.
func (d Destination) Validate() error {
	if strings.TrimSpace(d.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if !validTypes[d.Type] {
		return fmt.Errorf("type %q must be one of slack, discord, webhook", d.Type)
	}
	if strings.TrimSpace(d.URL) == "" {
		return fmt.Errorf("url is required")
	}
	u, err := url.Parse(d.URL)
	if err != nil {
		return fmt.Errorf("url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q", u.Scheme)
	}
	return nil
}

// Matches reports whether this destination fires for the given project.
// An empty allowlist fires for every project — that's the "no filter"
// default. A non-empty allowlist must contain the project name exactly.
func (d Destination) Matches(project string) bool {
	if len(d.Projects) == 0 {
		return true
	}
	for _, p := range d.Projects {
		if p == project {
			return true
		}
	}
	return false
}

// httpClient is shared across fan-out sends so we benefit from connection
// reuse to popular endpoints (one Slack workspace, one Discord webhook).
// The timeout caps any one send so a hung server can't pile up
// goroutines forever.
var httpClient = &http.Client{Timeout: 10 * time.Second}

// Fanout fires one POST per matching destination, concurrently. Failures
// are logged via logf — no retry, no queue. The watcher's next tick
// re-evaluates open asks; an ask that's still open keeps firing until
// answered, so a transient outage self-heals.
//
// logf is plumbed in (rather than calling log.Printf directly) so tests
// can capture lines and the caller can route them to whatever logger.
func Fanout(ctx context.Context, n Notification, dests []Destination, logf func(format string, args ...any)) {
	for _, d := range dests {
		if !d.Matches(n.Project) {
			continue
		}
		d := d // capture per iteration
		go func() {
			if err := d.Send(ctx, n); err != nil {
				if logf != nil {
					logf("notify[%s] %s: %v", d.Type, d.Name, err)
				}
			}
		}()
	}
}

// Send dispatches one notification to one destination. Returns the first
// error encountered (network, non-2xx response, etc.).
func (d Destination) Send(ctx context.Context, n Notification) error {
	switch d.Type {
	case "slack":
		return postJSON(ctx, d.URL, map[string]any{"text": formatText(n)})
	case "discord":
		// Discord caps content at 2000 chars. The format helper builds
		// short payloads (title + body line + URL); we still clamp
		// defensively in case a future title or step blows up.
		text := formatText(n)
		if len(text) > 2000 {
			text = text[:1997] + "..."
		}
		return postJSON(ctx, d.URL, map[string]any{"content": text})
	case "webhook":
		// Generic webhooks pass the structured Notification through so
		// downstream automation (n8n, Zapier, custom) can pull whichever
		// fields it needs without parsing prose.
		return postJSON(ctx, d.URL, map[string]any{
			"title":   n.Title,
			"body":    n.Body,
			"url":     n.URL,
			"project": n.Project,
			"run":     n.Run,
			"tag":     n.Tag,
		})
	default:
		return fmt.Errorf("unknown destination type %q", d.Type)
	}
}

// formatText is the shared text shape for chat-style destinations
// (Slack, Discord). Both render *bold*, line breaks, and bare URLs as
// clickable links — Discord with markdown-ish parsing, Slack with mrkdwn.
// We aim for a single line that reads as a sentence with the report
// link at the end.
func formatText(n Notification) string {
	var b strings.Builder
	if n.Title != "" {
		b.WriteString("*" + n.Title + "*")
	}
	if n.Body != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(n.Body)
	}
	if n.URL != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString("→ " + n.URL)
	}
	return strings.TrimSpace(b.String())
}

// postJSON marshals body, POSTs it to url, and treats any non-2xx
// response as an error. Used by every sender — the only thing that
// differs across destination types is the body shape.
func postJSON(ctx context.Context, target string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", target, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "harness-deck/notify")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}
