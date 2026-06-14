package usage

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Monitor / Build ---

type fakeProvider struct {
	tool, label string
	s           Sample
}

func (f fakeProvider) Tool() string                  { return f.tool }
func (f fakeProvider) Label() string                 { return f.label }
func (f fakeProvider) Sample(context.Context) Sample { return f.s }

func TestBuildSelectsAndOrders(t *testing.T) {
	ps := Build(Options{Providers: []string{"codex", "openrouter", "bogus", "claude-code", "copilot", "opencode"}})
	want := []string{"codex", "openrouter", "claude-code", "copilot", "opencode"}
	if len(ps) != len(want) {
		t.Fatalf("got %d providers, want %d", len(ps), len(want))
	}
	for i, w := range want {
		if ps[i].Tool() != w {
			t.Errorf("provider[%d] = %q, want %q", i, ps[i].Tool(), w)
		}
	}
}

func TestNewMonitorNilWhenEmpty(t *testing.T) {
	if m := NewMonitor(nil, 0); m != nil {
		t.Errorf("NewMonitor(nil) = %v, want nil", m)
	}
	// nil monitor is safe to use
	var m *Monitor
	if got := m.Samples(); got != nil {
		t.Errorf("nil.Samples() = %v, want nil", got)
	}
	m.Start(context.Background()) // must not panic
}

func TestMonitorRefreshStoresInOrder(t *testing.T) {
	m := NewMonitor([]Provider{
		fakeProvider{tool: "codex", label: "CX", s: Sample{OK: true, Kind: KindWindow, Percent: pct(50)}},
		fakeProvider{tool: "openrouter", label: "OR", s: Sample{OK: true, Kind: KindBudget, Text: "$5/mo"}},
	}, time.Minute)
	m.refresh(context.Background())

	got := m.Samples()
	if len(got) != 2 {
		t.Fatalf("got %d samples, want 2", len(got))
	}
	if got[0].Tool != "codex" || got[1].Tool != "openrouter" {
		t.Errorf("order = %q,%q want codex,openrouter", got[0].Tool, got[1].Tool)
	}
	// refresh fills Tool/Label/Updated from the provider.
	if got[0].Label != "CX" || got[0].Updated == "" {
		t.Errorf("sample not annotated: %+v", got[0])
	}
}

// --- Codex (local files, no network) ---

func writeCodexRollout(t *testing.T, root, day, name, content string) {
	t.Helper()
	dir := filepath.Join(root, "sessions", day)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCodexReadsLatestRateLimits(t *testing.T) {
	root := t.TempDir()
	// The newest matching event is line 1; the last line (a "premium" event
	// with null windows) must be skipped, exercising the null-window guard.
	content := `{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"plan_type":"plus","primary":{"used_percent":62.5,"window_minutes":300,"resets_at":2000000000},"secondary":{"used_percent":76,"window_minutes":10080,"resets_at":2000500000}}}}
{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"limit_id":"premium","primary":null,"secondary":null}}}
`
	writeCodexRollout(t, root, "2026/06/14", "rollout-2026-06-14T10-00-00-abc.jsonl", content)
	t.Setenv("CODEX_HOME", root)

	pinNow(t, time.Unix(1999990000, 0).UTC()) // before resets_at
	s := codexProvider{}.Sample(context.Background())
	if !s.OK {
		t.Fatalf("codex sample not OK: %s", s.Err)
	}
	if s.Percent == nil || *s.Percent != 62.5 {
		t.Errorf("percent = %v, want 62.5", s.Percent)
	}
	wantReset := time.Unix(2000000000, 0).UTC().Format(time.RFC3339)
	if s.ResetAt != wantReset {
		t.Errorf("reset = %q, want %q", s.ResetAt, wantReset)
	}
	if !contains(s.Detail, "plan plus") || !contains(s.Detail, "weekly 76%") {
		t.Errorf("detail = %q, want plan + weekly", s.Detail)
	}
}

func TestCodexPastResetShowsZero(t *testing.T) {
	root := t.TempDir()
	content := `{"type":"event_msg","payload":{"type":"token_count","rate_limits":{"primary":{"used_percent":90,"window_minutes":300,"resets_at":1000000000}}}}` + "\n"
	writeCodexRollout(t, root, "2026/06/14", "rollout-x.jsonl", content)
	t.Setenv("CODEX_HOME", root)

	pinNow(t, time.Unix(1000000500, 0).UTC()) // after resets_at
	s := codexProvider{}.Sample(context.Background())
	if !s.OK || s.Percent == nil || *s.Percent != 0 {
		t.Errorf("after reset: OK=%v percent=%v, want OK + 0%%", s.OK, s.Percent)
	}
	if s.ResetAt != "" {
		t.Errorf("after reset: ResetAt = %q, want empty", s.ResetAt)
	}
}

func TestCodexNoSessions(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	if s := (codexProvider{}).Sample(context.Background()); s.OK {
		t.Errorf("empty CODEX_HOME should not be OK: %+v", s)
	}
}

// --- OpenRouter (httptest) ---

func TestOpenRouterCappedAndUncapped(t *testing.T) {
	t.Run("capped", func(t *testing.T) {
		srv := jsonServer(t, `{"data":{"usage":34,"limit":100,"limit_remaining":66,"usage_daily":1,"usage_weekly":5,"usage_monthly":20}}`)
		defer srv.Close()
		swap(&openRouterURL, srv.URL)
		s := openRouterProvider{key: "k"}.Sample(context.Background())
		if !s.OK || s.Percent == nil || *s.Percent != 34 {
			t.Fatalf("capped: OK=%v percent=%v", s.OK, s.Percent)
		}
		if s.Text != "$66 left" {
			t.Errorf("text = %q, want $66 left", s.Text)
		}
	})
	t.Run("uncapped", func(t *testing.T) {
		srv := jsonServer(t, `{"data":{"usage":12.5,"limit":null,"usage_monthly":12.5}}`)
		defer srv.Close()
		swap(&openRouterURL, srv.URL)
		s := openRouterProvider{key: "k"}.Sample(context.Background())
		if !s.OK || s.Percent != nil {
			t.Fatalf("uncapped should have no percent: %+v", s)
		}
		if s.Text != "$12.50/mo" {
			t.Errorf("text = %q, want $12.50/mo", s.Text)
		}
	})
	t.Run("no key", func(t *testing.T) {
		if s := (openRouterProvider{}).Sample(context.Background()); s.OK {
			t.Errorf("no key should not be OK: %+v", s)
		}
	})
}

// --- Claude (httptest + env token, no keychain) ---

func TestClaudeUsageViaEnvToken(t *testing.T) {
	srv := jsonServer(t, `{"five_hour":{"utilization":42,"resets_at":"2026-06-14T20:00:00Z"},"seven_day":{"utilization":10,"resets_at":"2026-06-20T00:00:00Z"}}`)
	defer srv.Close()
	swap(&claudeUsageURL, srv.URL)
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "sk-ant-oat01-test")

	s := claudeProvider{}.Sample(context.Background())
	if !s.OK || s.Percent == nil || *s.Percent != 42 {
		t.Fatalf("claude: OK=%v percent=%v err=%s", s.OK, s.Percent, s.Err)
	}
	if s.ResetAt != "2026-06-14T20:00:00Z" {
		t.Errorf("reset = %q", s.ResetAt)
	}
	if !contains(s.Detail, "weekly 10%") {
		t.Errorf("detail = %q, want weekly", s.Detail)
	}
}

// --- Copilot (httptest + apps.json in a temp HOME) ---

func TestCopilotUsage(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".config", "github-copilot")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "apps.json"),
		[]byte(`{"github.com:Iv1.abc":{"oauth_token":"ghu_test"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	srv := jsonServer(t, `{"copilot_plan":"individual","quota_reset_date":"2026-07-01","quota_snapshots":{"premium_interactions":{"entitlement":300,"remaining":120,"percent_remaining":40}}}`)
	defer srv.Close()
	swap(&copilotUsageURL, srv.URL)

	s := copilotProvider{}.Sample(context.Background())
	if !s.OK || s.Percent == nil || *s.Percent != 60 {
		t.Fatalf("copilot: OK=%v percent=%v err=%s", s.OK, s.Percent, s.Err)
	}
	if s.ResetAt != "2026-07-01T00:00:00Z" {
		t.Errorf("reset = %q", s.ResetAt)
	}
	if !contains(s.Detail, "120 of 300") || !contains(s.Detail, "individual") {
		t.Errorf("detail = %q", s.Detail)
	}
}

// --- OpenCode usage extractor (the tolerant scan) ---

func TestExtractOpenCodeUsage(t *testing.T) {
	body := `whatever({"rollingUsage":{"usagePercent":55,"resetInSec":3600},"weeklyUsage":{"usagePercent":80.5,"resetInSec":86400}})`
	p, r, ok := extractOpenCodeUsage(body, "rollingUsage")
	if !ok || p != 55 || r != 3600 {
		t.Errorf("rolling = (%v,%v,%v), want 55,3600,true", p, r, ok)
	}
	wp, wr, ok := extractOpenCodeUsage(body, "weeklyUsage")
	if !ok || wp != 80.5 || wr != 86400 {
		t.Errorf("weekly = (%v,%v,%v), want 80.5,86400,true", wp, wr, ok)
	}
	if _, _, ok := extractOpenCodeUsage(body, "missing"); ok {
		t.Error("missing key should not be ok")
	}
}

func TestOpenCodeNoCookie(t *testing.T) {
	if s := (openCodeProvider{}).Sample(context.Background()); s.OK {
		t.Errorf("no cookie should not be OK: %+v", s)
	}
}

// --- helpers ---

func TestMoneyFormat(t *testing.T) {
	for _, tc := range []struct {
		in   float64
		want string
	}{{0, "$0"}, {5, "$5"}, {12.5, "$12.50"}, {12.34, "$12.34"}} {
		if got := money(tc.in); got != tc.want {
			t.Errorf("money(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMonthlyReset(t *testing.T) {
	if got := monthlyReset("2026-07-01"); got != "2026-07-01T00:00:00Z" {
		t.Errorf("monthlyReset = %q", got)
	}
	if got := monthlyReset("garbage"); got != "" {
		t.Errorf("monthlyReset(garbage) = %q, want empty", got)
	}
}

// --- test utilities ---

func pinNow(t *testing.T, at time.Time) {
	t.Helper()
	prev := nowUTC
	nowUTC = func() time.Time { return at }
	t.Cleanup(func() { nowUTC = prev })
}

func swap(p *string, v string) {
	*p = v // tests are sequential per package var; httptest URLs differ per run
}

func jsonServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
