package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func guardedRequest(t *testing.T, g http.Handler, method, remote, host string, hdr map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(method, "/r/demo/run/respond", strings.NewReader(`{"block":"a","value":"approved"}`))
	req.RemoteAddr = remote
	req.Host = host
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req)
	return rec.Code
}

func TestGuardNetworks(t *testing.T) {
	g := newGuard(config.Default(), okHandler())
	cases := []struct {
		remote string
		want   int
	}{
		{"127.0.0.1:5000", http.StatusOK},
		{"[::1]:5000", http.StatusOK},
		{"100.101.102.103:5000", http.StatusOK},            // Tailscale CGNAT
		{"[fd7a:115c:a1e0::1234]:5000", http.StatusOK},     // Tailscale IPv6
		{"192.0.2.10:5000", http.StatusForbidden},          // arbitrary LAN/Internet peer
		{"[2001:db8::1]:5000", http.StatusForbidden},       // arbitrary IPv6 peer
		{"[::ffff:192.0.2.10]:5000", http.StatusForbidden}, // v4-mapped stays untrusted
	}
	for _, c := range cases {
		if got := guardedRequest(t, g, http.MethodGet, c.remote, "127.0.0.1:7420", nil); got != c.want {
			t.Errorf("remote %s: got %d, want %d", c.remote, got, c.want)
		}
	}
}

func TestGuardTrustedNetworksConfig(t *testing.T) {
	cfg := config.Default()
	cfg.TrustedNetworks = []string{"192.0.2.0/24"}
	g := newGuard(cfg, okHandler())
	if got := guardedRequest(t, g, http.MethodGet, "192.0.2.10:5000", "127.0.0.1", nil); got != http.StatusOK {
		t.Errorf("configured LAN range: got %d, want 200", got)
	}
	// An explicit list replaces the Tailscale default.
	if got := guardedRequest(t, g, http.MethodGet, "100.101.102.103:5000", "127.0.0.1", nil); got != http.StatusForbidden {
		t.Errorf("tailnet with explicit list: got %d, want 403", got)
	}
}

func TestGuardHost(t *testing.T) {
	cfg := config.Default()
	cfg.PublicURL = "https://deck.example.com:7420"
	cfg.AllowedHosts = []string{"Box.Home.Arpa"}
	g := newGuard(cfg, okHandler())
	allowed := []string{
		"127.0.0.1:7420", "[::1]:7420", "100.64.0.9:7420", "localhost:7420",
		"app.localhost", "mymac.tail1234.ts.net:7420", "deck.example.com",
		"box.home.arpa:7420", "LOCALHOST.:7420",
	}
	for _, h := range allowed {
		if got := guardedRequest(t, g, http.MethodGet, "127.0.0.1:5000", h, nil); got != http.StatusOK {
			t.Errorf("host %q: got %d, want 200", h, got)
		}
	}
	// DNS rebinding: an attacker's name pointed at 127.0.0.1.
	for _, h := range []string{"evil.example:7420", "localhost.evil.example", "ts.net.evil.example"} {
		if got := guardedRequest(t, g, http.MethodGet, "127.0.0.1:5000", h, nil); got != http.StatusMisdirectedRequest {
			t.Errorf("host %q: got %d, want 421", h, got)
		}
	}
}

func TestGuardCrossOrigin(t *testing.T) {
	g := newGuard(config.Default(), okHandler())
	const remote, host = "127.0.0.1:5000", "127.0.0.1:7420"
	cases := []struct {
		name   string
		method string
		hdr    map[string]string
		want   int
	}{
		{"cross-site POST", http.MethodPost, map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}, http.StatusForbidden},
		{"origin-only cross POST", http.MethodPost, map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"cross-site DELETE", http.MethodDelete, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusForbidden},
		{"same-origin POST", http.MethodPost, map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": "http://127.0.0.1:7420"}, http.StatusOK},
		{"origin matches host", http.MethodPost, map[string]string{"Origin": "http://127.0.0.1:7420"}, http.StatusOK},
		{"non-browser POST (curl)", http.MethodPost, nil, http.StatusOK},
		{"cross-site GET", http.MethodGet, map[string]string{"Sec-Fetch-Site": "cross-site"}, http.StatusOK},
	}
	for _, c := range cases {
		if got := guardedRequest(t, g, c.method, remote, host, c.hdr); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestGuardBodyLimit(t *testing.T) {
	var readErr error
	g := newGuard(config.Default(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 64<<10)
		for readErr == nil {
			_, readErr = r.Body.Read(buf)
		}
	}))
	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(strings.Repeat("a", maxBodyBytes+1)))
	req.RemoteAddr = "127.0.0.1:1"
	req.Host = "127.0.0.1"
	g.ServeHTTP(httptest.NewRecorder(), req)
	if readErr == nil || !strings.Contains(readErr.Error(), "too large") {
		t.Fatalf("oversized body: read error %v, want request body too large", readErr)
	}
}

func TestTrustedNetsRejectsGarbage(t *testing.T) {
	cfg := config.Default()
	cfg.TrustedNetworks = []string{"not-a-network"}
	if _, err := cfg.TrustedNets(); err == nil {
		t.Fatal("expected an error for an invalid trusted_networks entry")
	}
	cfg.TrustedNetworks = []string{"10.0.0.7"}
	nets, err := cfg.TrustedNets()
	if err != nil || len(nets) != 1 || nets[0].Bits() != 32 {
		t.Fatalf("bare IP: got %v, %v; want one /32", nets, err)
	}
}
