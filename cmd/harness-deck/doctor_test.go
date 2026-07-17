package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

func TestCheckProviders(t *testing.T) {
	if r := checkProviders(nil); r.Status != statusOK {
		t.Errorf("no providers: %v, want ok", r.Status)
	}
	if r := checkProviders([]string{"codex", "claude-code"}); r.Status != statusOK {
		t.Errorf("valid providers: %v, want ok", r.Status)
	}
	r := checkProviders([]string{"codex", "ollama-cloud"})
	if r.Status != statusFail {
		t.Fatalf("typo provider: %v, want fail", r.Status)
	}
	if !strings.Contains(r.Detail, "ollama-cloud") {
		t.Errorf("detail should name the bad entry: %q", r.Detail)
	}
	if !strings.Contains(r.Fix, "codex") {
		t.Errorf("fix should list the valid names: %q", r.Fix)
	}
}

func TestCheckURLTLS(t *testing.T) {
	cases := []struct {
		url  string
		tls  bool
		want checkStatus
	}{
		{"", false, statusOK},
		{"https://h.ts.net:7420", true, statusOK},
		{"https://h.ts.net:7420", false, statusFail}, // https promised, no TLS configured
		{"http://h.ts.net:7420", true, statusWarn},   // TLS configured but URL says http
		{"http://h.ts.net:7420", false, statusOK},
	}
	for _, tc := range cases {
		if r := checkURLTLS(tc.url, tc.tls); r.Status != tc.want {
			t.Errorf("checkURLTLS(%q, tls=%v) = %v, want %v", tc.url, tc.tls, r.Status, tc.want)
		}
	}
}

func TestCheckCertExpiry(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		notAfter time.Time
		want     checkStatus
	}{
		{"fresh", now.AddDate(0, 3, 0), statusOK},
		{"expiring", now.Add(10 * 24 * time.Hour), statusWarn},
		{"expired", now.Add(-time.Hour), statusFail},
	}
	for _, tc := range cases {
		if r := checkCertExpiry(tc.notAfter, now); r.Status != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, r.Status, tc.want)
		}
	}
}

// selfSigned builds a throwaway cert covering the given DNS names.
func selfSigned(t *testing.T, dns ...string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dns[0]},
		DNSNames:     dns,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().AddDate(0, 3, 0),
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert
}

func TestCheckCertHost(t *testing.T) {
	cert := selfSigned(t, "my-mac.my-tailnet.ts.net")
	if r := checkCertHost(cert, "https://my-mac.my-tailnet.ts.net:7420"); r.Status != statusOK {
		t.Errorf("matching host: %v, want ok (%s)", r.Status, r.Detail)
	}
	if r := checkCertHost(cert, "https://other.my-tailnet.ts.net:7420"); r.Status != statusFail {
		t.Errorf("mismatched host: %v, want fail", r.Status)
	}
	if r := checkCertHost(cert, "://not a url"); r.Status != statusFail {
		t.Errorf("bad url: %v, want fail", r.Status)
	}
}

func TestCheckFirewallProbe(t *testing.T) {
	if r := checkFirewallProbe(nil, "/opt/x/harness-deck"); r.Status != statusOK {
		t.Errorf("probe ok: %v, want ok", r.Status)
	}
	r := checkFirewallProbe(errors.New("dial tcp 100.1.2.3:53422: i/o timeout"), "/opt/x/harness-deck")
	if r.Status != statusFail {
		t.Fatalf("probe timeout: %v, want fail", r.Status)
	}
	for _, want := range []string{"socketfilterfw", "--add", "--unblockapp", "/opt/x/harness-deck"} {
		if !strings.Contains(r.Fix, want) {
			t.Errorf("fix missing %q: %q", want, r.Fix)
		}
	}
}

func TestDoctorExitStatus(t *testing.T) {
	ok := checkResult{Name: "a", Status: statusOK}
	warn := checkResult{Name: "b", Status: statusWarn}
	fail := checkResult{Name: "c", Status: statusFail}
	if worstStatus([]checkResult{ok, warn}) != statusWarn {
		t.Error("ok+warn should be warn")
	}
	if worstStatus([]checkResult{ok, warn, fail}) != statusFail {
		t.Error("any fail should be fail")
	}
	if worstStatus(nil) != statusOK {
		t.Error("empty should be ok")
	}
}

func TestCheckBindTLSNeedsProbe(t *testing.T) {
	// The firewall probe only applies to non-loopback binds.
	for bind, want := range map[string]bool{
		"127.0.0.1":  false,
		"":           false,
		"0.0.0.0":    true,
		"100.99.1.2": true,
	} {
		cfg := config.Config{Bind: bind}
		if got := needsFirewallProbe(cfg); got != want {
			t.Errorf("needsFirewallProbe(bind=%q) = %v, want %v", bind, got, want)
		}
	}
}

func TestExternalReachResult(t *testing.T) {
	if r := externalReachResult("100.1.2.3", 7420, nil, "/opt/x/hd"); r.Status != statusOK {
		t.Errorf("reachable: %v, want ok", r.Status)
	}
	r := externalReachResult("100.1.2.3", 7420, errors.New("i/o timeout"), "/opt/x/hd")
	if r.Status != statusFail {
		t.Fatalf("unreachable: %v, want fail", r.Status)
	}
	if !strings.Contains(r.Detail, "loopback") || !strings.Contains(r.Detail, "100.1.2.3") {
		t.Errorf("detail should contrast loopback vs external: %q", r.Detail)
	}
	if !strings.Contains(r.Fix, "socketfilterfw") {
		t.Errorf("fix should include the allowlist command: %q", r.Fix)
	}
}

// A server that isn't running must FAIL, not WARN: AGENTS.md/SETUP.md tell
// agents to treat `doctor` exit 0 as the install-is-done gate, and "the
// service never started" is the single most likely install failure. Grading it
// WARN made that gate certify a broken install.
func TestServerDownIsFail(t *testing.T) {
	r := serverDownResult(7420)
	if r.Status != statusFail {
		t.Errorf("nothing listening = %v, want fail (exit 0 must not certify a dead server)", r.Status)
	}
	if !strings.Contains(r.Fix, "brew services start") {
		t.Errorf("fix should tell the user how to start it: %q", r.Fix)
	}
	if worstStatus([]checkResult{{Status: statusOK}, r}) != statusFail {
		t.Error("a down server must drive the overall exit status to fail")
	}
}

func TestCheckScanRoots(t *testing.T) {
	// No scan_roots and no explicit projects: the projects view will be empty,
	// which is the symptom a user actually notices. Warn, don't fail — a
	// central-dir-only install is legitimate.
	r := checkScanRoots(config.Config{})
	if r.Status != statusWarn {
		t.Errorf("no scan_roots = %v, want warn", r.Status)
	}
	if !strings.Contains(r.Fix, "scan_roots") {
		t.Errorf("fix should name scan_roots: %q", r.Fix)
	}
}

// scanRootsResult grades the discovery outcome given the live project count,
// so the marker-mismatch warning is testable without real scan roots (same
// pattern as staleUnitResult).
func TestScanRootsResult(t *testing.T) {
	if r := scanRootsResult(config.Config{ScanRoots: []string{"~/git"}}, 3); r.Status != statusOK {
		t.Errorf("roots with projects = %v, want ok", r.Status)
	}
	if r := scanRootsResult(config.Config{Projects: []string{"/x/y"}}, 1); r.Status != statusOK {
		t.Errorf("explicit projects should satisfy it: %v, want ok", r.Status)
	}

	// scan_roots set but nothing matched a marker: the projects view is empty
	// while every other check passes — warn and name project_markers, the
	// knob that fixes it for repos not using the .docs/ai convention.
	r := scanRootsResult(config.Config{ScanRoots: []string{"~/git"}, ProjectMarkers: []string{".docs/ai"}}, 0)
	if r.Status != statusWarn {
		t.Errorf("roots with no matches = %v, want warn", r.Status)
	}
	if !strings.Contains(r.Detail+r.Fix, "project_markers") {
		t.Errorf("warning should name project_markers: detail=%q fix=%q", r.Detail, r.Fix)
	}
}

// A hand-rolled unit left over from an older setup starts a second server at
// login and fights the Homebrew service for the port; the loser crash-loops,
// and the survivor still answers /api/reports — so every other check goes
// green. Prose in the runbook is not enough: doctor has to name it.
func TestStaleUnitResult(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		if r := staleUnitResult(nil, goos); r.Status != statusOK {
			t.Errorf("%s: no stale units = %v, want ok", goos, r.Status)
		}
	}

	// macOS: launchctl + the rm -f caveat that bit us for real.
	mac := staleUnitResult([]string{"com.tfinklea.harness-deck.plist"}, "darwin")
	if mac.Status != statusFail {
		t.Fatalf("darwin stale unit = %v, want fail", mac.Status)
	}
	if !strings.Contains(mac.Detail, "com.tfinklea.harness-deck.plist") {
		t.Errorf("detail must name the file: %q", mac.Detail)
	}
	for _, want := range []string{"launchctl bootout", "rm -f"} {
		if !strings.Contains(mac.Fix, want) {
			t.Errorf("darwin fix missing %q: %q", want, mac.Fix)
		}
	}

	// Linux: systemd, not launchctl. Mixing the two service managers is the
	// exact confusion the setup docs warn about.
	lin := staleUnitResult([]string{"harness-deck.service"}, "linux")
	if lin.Status != statusFail {
		t.Fatalf("linux stale unit = %v, want fail", lin.Status)
	}
	if !strings.Contains(lin.Fix, "systemctl --user disable") || !strings.Contains(lin.Fix, "rm -f") {
		t.Errorf("linux fix should use systemctl: %q", lin.Fix)
	}
	if strings.Contains(lin.Fix, "launchctl") {
		t.Errorf("linux fix must not mention launchctl: %q", lin.Fix)
	}
}

func TestIsStaleUnit(t *testing.T) {
	cases := map[string]bool{
		"homebrew.mxcl.harness-deck.plist":   false, // Homebrew's own — leave alone
		"com.tfinklea.harness-deck.plist":    true,  // the label that actually bit us
		"com.harnessdeck.serve.plist":        true,  // the label Appendix A used
		"harness-deck.service":               true,  // hand-rolled systemd unit
		"homebrew.mxcl.harness-deck.service": false,
		"com.apple.something.plist":          false, // unrelated
		"my-harness-notes.txt":               false, // not a unit file
	}
	for name, want := range cases {
		if got := isStaleUnit(name); got != want {
			t.Errorf("isStaleUnit(%q) = %v, want %v", name, got, want)
		}
	}
}
