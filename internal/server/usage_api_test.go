package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// TestUsageAPIEmptyWhenNoProviders exercises handleUsage's nil-Monitor path:
// with no usage providers configured the endpoint must return a JSON empty
// array, not null or an error.
func TestUsageAPIEmptyWhenNoProviders(t *testing.T) {
	s := newTestServerFull(t, config.Config{CentralDir: t.TempDir()})
	req := httptest.NewRequest("GET", "/api/usage", nil)
	w := httptest.NewRecorder()
	s.handleUsage(w, req)

	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := strings.TrimSpace(w.Body.String()); got != "[]" {
		t.Errorf("body = %q, want []", got)
	}
}
