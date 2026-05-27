package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDestinationValidate(t *testing.T) {
	cases := []struct {
		name    string
		d       Destination
		wantErr string // substring; "" means no error
	}{
		{"ok slack", Destination{Name: "x", Type: "slack", URL: "https://hooks.slack.com/x"}, ""},
		{"ok webhook", Destination{Name: "x", Type: "webhook", URL: "http://localhost:1"}, ""},
		{"missing name", Destination{Type: "slack", URL: "https://x"}, "name is required"},
		{"missing url", Destination{Name: "x", Type: "slack"}, "url is required"},
		{"bad type", Destination{Name: "x", Type: "email", URL: "https://x"}, "must be one of"},
		{"bad scheme", Destination{Name: "x", Type: "slack", URL: "ftp://x"}, "scheme must be http"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.d.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("want ok, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestMatchesProjectAllowlist(t *testing.T) {
	d := Destination{Name: "x", Type: "slack", URL: "https://x"}
	if !d.Matches("anything") {
		t.Error("empty allowlist should match every project")
	}
	d.Projects = []string{"a", "b"}
	if !d.Matches("a") || !d.Matches("b") {
		t.Error("allowlist should match listed projects")
	}
	if d.Matches("c") {
		t.Error("allowlist should not match unlisted projects")
	}
}

// captureServer is a httptest server that records every POST it gets so
// tests can assert on payload shape + headers.
func captureServer(t *testing.T) (*httptest.Server, *[]map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var got []map[string]any
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		got = append(got, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(s.Close)
	return s, &got
}

func TestSlackPayloadShape(t *testing.T) {
	srv, captured := captureServer(t)
	d := Destination{Name: "ops", Type: "slack", URL: srv.URL}
	if err := d.Send(context.Background(), Notification{Title: "T", Body: "B", URL: "https://x/r/p/1"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(*captured) != 1 {
		t.Fatalf("captures = %d, want 1", len(*captured))
	}
	text, _ := (*captured)[0]["text"].(string)
	if !strings.Contains(text, "*T*") || !strings.Contains(text, "B") || !strings.Contains(text, "https://x/r/p/1") {
		t.Errorf("slack text missing parts: %q", text)
	}
}

func TestDiscordPayloadShape(t *testing.T) {
	srv, captured := captureServer(t)
	d := Destination{Name: "myserver", Type: "discord", URL: srv.URL}
	if err := d.Send(context.Background(), Notification{Title: "T", Body: "B", URL: "https://x"}); err != nil {
		t.Fatalf("send: %v", err)
	}
	content, _ := (*captured)[0]["content"].(string)
	if !strings.Contains(content, "T") || !strings.Contains(content, "https://x") {
		t.Errorf("discord content missing parts: %q", content)
	}
}

func TestWebhookPayloadShape(t *testing.T) {
	srv, captured := captureServer(t)
	d := Destination{Name: "n8n", Type: "webhook", URL: srv.URL}
	n := Notification{Title: "T", Body: "B", URL: "https://x", Project: "p", Run: "r1", Tag: "p:r1:b"}
	if err := d.Send(context.Background(), n); err != nil {
		t.Fatalf("send: %v", err)
	}
	got := (*captured)[0]
	for _, key := range []string{"title", "body", "url", "project", "run", "tag"} {
		if _, has := got[key]; !has {
			t.Errorf("webhook payload missing %q: %v", key, got)
		}
	}
}

func TestSendErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	d := Destination{Name: "x", Type: "slack", URL: srv.URL}
	if err := d.Send(context.Background(), Notification{Title: "t"}); err == nil {
		t.Error("expected error on 500")
	}
}

func TestFanoutFiltersAndLogsErrors(t *testing.T) {
	okSrv, _ := captureServer(t)
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer failSrv.Close()
	dests := []Destination{
		{Name: "ok", Type: "slack", URL: okSrv.URL},                              // fires
		{Name: "skipped", Type: "slack", URL: okSrv.URL, Projects: []string{"x"}}, // filtered out
		{Name: "broken", Type: "slack", URL: failSrv.URL},                        // fires + fails
	}
	var logCount int32
	logf := func(format string, args ...any) {
		atomic.AddInt32(&logCount, 1)
	}
	Fanout(context.Background(), Notification{Project: "p"}, dests, logf)
	// Fanout goroutines must finish; sleep is the simplest sync here.
	time.Sleep(300 * time.Millisecond)
	if got := atomic.LoadInt32(&logCount); got != 1 {
		t.Errorf("logCount = %d, want 1 (only broken destination should have errored)", got)
	}
}
