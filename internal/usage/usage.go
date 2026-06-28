// Package usage reads AI coding-tool usage — rate-limit windows or
// credit/spend — from each tool's local files and HTTP APIs, and exposes a
// normalized snapshot the dashboard footer renders (CodexBar-style).
//
// Each tool is a Provider. A Monitor refreshes the enabled providers on an
// interval and caches the latest Sample per tool; the server serves the cache
// at GET /api/usage. Everything is opt-in (config lists the providers) because
// several providers read credentials or hit the network — nothing happens for
// a tool that isn't listed.
//
// Zero external dependencies, like the rest of harness-deck: providers use
// os.ReadFile / os/exec / net/http and encoding/json only. Parsing is lenient
// (the on-disk JSONL/JSON schemas are undocumented and drift) — a provider that
// can't read its source returns Sample{OK: false} and is simply omitted from
// the footer rather than erroring.
package usage

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Kind distinguishes the two shapes of usage a footer segment renders.
type Kind string

const (
	// KindWindow is a rate-limit window: Percent used (0–100) + ResetAt.
	KindWindow Kind = "window"
	// KindBudget is cumulative spend / a credit balance: Text carries it.
	KindBudget Kind = "budget"
)

// Sample is one provider's latest reading, normalized for the footer. A
// provider with no data this cycle returns {OK: false} (Err explains why, for
// diagnostics — it is not shown to the user).
type Sample struct {
	Tool    string   `json:"tool"`               // stable id: "codex", "claude-code", …
	Label   string   `json:"label"`              // short footer label: "CX", "CC", …
	OK      bool     `json:"ok"`                 // data available this cycle
	Kind    Kind     `json:"kind,omitempty"`     // window | budget
	Percent *float64 `json:"percent,omitempty"`  // 0–100 used (window kind)
	ResetAt string   `json:"reset_at,omitempty"` // RFC3339 reset (window kind)
	Text    string   `json:"text,omitempty"`     // short value (budget kind), e.g. "$12.30"
	Detail  string   `json:"detail,omitempty"`   // tooltip: secondary window, plan, breakdown
	Err     string   `json:"err,omitempty"`      // why OK is false (diagnostics only)
	Updated string   `json:"updated,omitempty"`  // RFC3339 fetch time
}

// Provider reads one tool's usage. Sample should honor ctx for timeouts; the
// Monitor calls providers concurrently but never the same provider twice at
// once, so a provider needs no internal locking.
type Provider interface {
	Tool() string
	Label() string
	Sample(ctx context.Context) Sample
}

// Options configures which providers Build constructs and supplies their
// credentials. Mirrors config.UsageConfig but keeps this package decoupled
// from config (the server translates).
type Options struct {
	Providers     []string // enabled tool ids, in display order
	OpenRouterKey string   // else $OPENROUTER_API_KEY
	OpenCodeDays  int      // window in days for the opencode spend tile; default 7
	// OpenCodeEnabled is a feature flag gating the opencode tile. It is off by
	// default and kept separate from Providers on purpose: `opencode stats`
	// only sees local TUI sessions, so the tile reads $0 for anyone whose real
	// spend runs through the opencode-go/Zen cloud plan (orchestra/pi). Listing
	// "opencode" in Providers does nothing unless this is also true. Flip it on
	// to revisit once a cloud-usage source exists. See decisions.md.
	OpenCodeEnabled bool
}

// Build constructs the providers named in o.Providers, in order. Unknown names
// are ignored. The OpenRouter key falls back to $OPENROUTER_API_KEY.
func Build(o Options) []Provider {
	orKey := o.OpenRouterKey
	if orKey == "" {
		orKey = os.Getenv("OPENROUTER_API_KEY")
	}
	var ps []Provider
	seen := map[string]bool{}
	add := func(p Provider) {
		if seen[p.Tool()] { // a tool listed twice (or via an alias) shows once
			return
		}
		seen[p.Tool()] = true
		ps = append(ps, p)
	}
	for _, name := range o.Providers {
		switch strings.TrimSpace(name) {
		case "codex":
			add(&codexProvider{})
		case "openrouter":
			add(&openRouterProvider{key: orKey})
		case "claude-code", "claude":
			add(&claudeProvider{})
		case "copilot":
			add(&copilotProvider{})
		case "opencode":
			if o.OpenCodeEnabled { // feature-flagged off by default; see Options.OpenCodeEnabled
				add(&openCodeProvider{days: o.OpenCodeDays})
			}
		}
	}
	return ps
}

// Monitor refreshes a set of providers on an interval and caches the latest
// Sample for each. Safe for concurrent use.
type Monitor struct {
	providers []Provider
	interval  time.Duration

	mu      sync.RWMutex
	samples map[string]Sample
}

// NewMonitor returns a Monitor over the given providers. A non-positive
// interval defaults to 60s. Returns nil when there are no providers, so the
// caller can treat "usage disabled" as a nil monitor.
func NewMonitor(providers []Provider, interval time.Duration) *Monitor {
	if len(providers) == 0 {
		return nil
	}
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Monitor{providers: providers, interval: interval, samples: map[string]Sample{}}
}

// Start refreshes once immediately, then on every interval until ctx is done.
// Non-blocking: it launches a background goroutine. Safe to call on a nil
// Monitor (no-op).
func (m *Monitor) Start(ctx context.Context) {
	if m == nil {
		return
	}
	go func() {
		m.refresh(ctx)
		t := time.NewTicker(m.interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.refresh(ctx)
			}
		}
	}()
}

// refresh samples every provider concurrently (each bounded by perSampleTimeout)
// and commits the results.
func (m *Monitor) refresh(ctx context.Context) {
	const perSampleTimeout = 12 * time.Second
	results := make([]Sample, len(m.providers))
	var wg sync.WaitGroup
	for i, p := range m.providers {
		wg.Add(1)
		go func(i int, p Provider) {
			defer wg.Done()
			sctx, cancel := context.WithTimeout(ctx, perSampleTimeout)
			defer cancel()
			s := p.Sample(sctx)
			s.Tool, s.Label = p.Tool(), p.Label()
			s.Updated = nowUTC().Format(time.RFC3339)
			results[i] = s
		}(i, p)
	}
	wg.Wait()
	m.mu.Lock()
	for _, s := range results {
		m.samples[s.Tool] = s
	}
	m.mu.Unlock()
}

// Samples returns the latest sample for each provider in display order. A
// provider not yet sampled is omitted. Safe on a nil Monitor (returns nil).
func (m *Monitor) Samples() []Sample {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Sample, 0, len(m.providers))
	for _, p := range m.providers {
		if s, ok := m.samples[p.Tool()]; ok {
			out = append(out, s)
		}
	}
	return out
}

// --- shared provider helpers ---

// nowUTC is the clock, indirected so tests can pin reset-window math.
var nowUTC = func() time.Time { return time.Now().UTC() }

// httpClient is the shared client for the HTTP-backed providers. Per-request
// deadlines come from the context; this timeout is a backstop.
var httpClient = &http.Client{Timeout: 15 * time.Second}

// getJSON performs an authenticated GET and decodes the JSON body into v.
// headers are applied verbatim. A non-2xx status is an error.
func getJSON(ctx context.Context, url string, headers map[string]string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, val := range headers {
		req.Header.Set(k, val)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &httpError{status: resp.StatusCode}
	}
	return json.Unmarshal(body, v)
}

// httpError carries only the status code — deliberately NOT the response body.
// A Sample's Err is served over the unauthenticated /api/usage endpoint, so
// echoing an external API's raw error body (which can carry account/org/rate
// detail) would broaden disclosure beyond the host that holds the credential.
type httpError struct {
	status int
}

func (e *httpError) Error() string {
	return "http " + itoa(e.status)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// home returns the user's home directory, or "" if it can't be resolved.
func home() string {
	h, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return h
}

// readJSONFile reads and decodes a JSON file into v.
func readJSONFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// pct returns a pointer to a percentage clamped to [0,100].
func pct(p float64) *float64 {
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return &p
}

// newestFirst sorts file paths by modification time, newest first; unreadable
// files sort last. Used by the local-file providers to find the latest data.
func newestFirst(paths []string) {
	mod := func(p string) int64 {
		fi, err := os.Stat(p)
		if err != nil {
			return -1
		}
		return fi.ModTime().UnixNano()
	}
	sort.Slice(paths, func(i, j int) bool { return mod(paths[i]) > mod(paths[j]) })
}

// jsonlLinesReverse returns the lines of a (possibly large) file, last line
// first, so a provider can find the most-recent matching record without
// scanning the whole thing into structs.
func jsonlLinesReverse(path string) [][]byte {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := splitLines(data)
	for i, j := 0, len(lines)-1; i < j; i, j = i+1, j-1 {
		lines[i], lines[j] = lines[j], lines[i]
	}
	return lines
}

func splitLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i := 0; i < len(data); i++ {
		if data[i] == '\n' {
			if i > start {
				out = append(out, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}
