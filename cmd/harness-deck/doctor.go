package main

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/projects"
	usagemon "github.com/TaylorFinklea/harness-deck/internal/usage"
)

// checkStatus grades one doctor check.
type checkStatus string

const (
	statusOK   checkStatus = "ok"
	statusWarn checkStatus = "warn"
	statusFail checkStatus = "fail"
)

// checkResult is one line of doctor output. Fix, when set, is a concrete
// command or config change that clears the problem.
type checkResult struct {
	Name   string      `json:"name"`
	Status checkStatus `json:"status"`
	Detail string      `json:"detail"`
	Fix    string      `json:"fix,omitempty"`
}

// cmdDoctor runs preflight checks over config, TLS, push, the port, Tailscale,
// and — the one that bites hardest — the macOS Application Firewall, which
// silently drops inbound connections to ad-hoc-signed binaries on non-loopback
// interfaces. Every failure prints a concrete fix. Exit 1 on any FAIL.
func cmdDoctor(args []string) {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Println("usage: harness-deck doctor [--json]")
			return
		default:
			fmt.Fprintf(os.Stderr, "doctor: unknown flag %q\n", a)
			os.Exit(2)
		}
	}

	var results []checkResult
	add := func(r checkResult) { results = append(results, r) }

	cfg, err := config.Load()
	if err != nil {
		add(checkResult{Name: "config", Status: statusFail,
			Detail: fmt.Sprintf("%s does not load: %v", config.Path(), err),
			Fix:    "fix the JSON (or move the file aside to fall back to defaults)"})
		finish(results, asJSON)
		return
	}
	add(checkResult{Name: "config", Status: statusOK, Detail: config.Path() + " loads"})
	add(checkProviders(cfg.Usage.Providers))
	add(checkScanRoots(cfg))
	add(checkURLTLS(cfg.PublicURL, cfg.TLS.Enabled()))

	if cfg.TLS.Enabled() {
		results = append(results, checkTLSFiles(cfg)...)
	}
	add(checkVAPID(cfg))
	add(staleUnitResult(findStaleUnits(), runtime.GOOS))
	add(checkPort(cfg))
	if r, applicable := checkTailscale(cfg); applicable {
		add(r)
	}
	if needsFirewallProbe(cfg) && runtime.GOOS == "darwin" {
		bin, _ := os.Executable()
		add(checkFirewallProbe(firewallProbe(), bin))
	}

	finish(results, asJSON)
}

// finish prints results in the requested form and exits non-zero on any FAIL.
func finish(results []checkResult, asJSON bool) {
	if asJSON {
		out, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(out))
	} else {
		for _, r := range results {
			mark := map[checkStatus]string{statusOK: "  ok ", statusWarn: "WARN ", statusFail: "FAIL "}[r.Status]
			fmt.Printf("%s %-10s %s\n", mark, r.Name, r.Detail)
			if r.Fix != "" {
				fmt.Printf("        fix: %s\n", r.Fix)
			}
		}
	}
	if worstStatus(results) == statusFail {
		os.Exit(1)
	}
}

// worstStatus reduces results to the most severe status present.
func worstStatus(results []checkResult) checkStatus {
	worst := statusOK
	for _, r := range results {
		if r.Status == statusFail {
			return statusFail
		}
		if r.Status == statusWarn {
			worst = statusWarn
		}
	}
	return worst
}

// checkProviders flags usage.providers entries that Build would silently
// ignore — a typo there is otherwise an invisible no-op.
func checkProviders(names []string) checkResult {
	unk := usagemon.UnknownProviders(names)
	if len(unk) == 0 {
		return checkResult{Name: "usage", Status: statusOK,
			Detail: fmt.Sprintf("%d provider(s) recognized", len(names))}
	}
	return checkResult{Name: "usage", Status: statusFail,
		Detail: "unknown provider(s) in usage.providers: " + strings.Join(unk, ", ") + " (they are silently ignored)",
		Fix:    "valid names: codex, openrouter, claude-code, copilot, opencode"}
}

// checkScanRoots warns when nothing feeds the projects view. scan_roots has no
// default, so a config that omits it yields an empty projects view — a silent,
// confusing outcome that otherwise passes every other check. Roots that are
// set but match no projects (marker mismatch) are the same symptom and get
// the same treatment, via scanRootsResult.
func checkScanRoots(cfg config.Config) checkResult {
	if len(cfg.ScanRoots) == 0 && len(cfg.Projects) == 0 {
		return checkResult{Name: "projects", Status: statusWarn,
			Detail: "no scan_roots set — the projects view will be empty (reports still work)",
			Fix:    `add "scan_roots": ["~/git"] to ` + config.Path() + `, or run: harness-deck register <path>`}
	}
	n := len(projects.NewManager(cfg.ScanRoots, cfg.Projects, cfg.ProjectMarkers, projects.StatePath()).Discovered())
	return scanRootsResult(cfg, n)
}

// scanRootsResult grades the discovery outcome given the live project count.
// Separated from checkScanRoots so the zero-match warning is testable without
// real scan roots on disk (same pattern as staleUnitResult).
func scanRootsResult(cfg config.Config, discovered int) checkResult {
	if discovered > 0 {
		return checkResult{Name: "projects", Status: statusOK,
			Detail: fmt.Sprintf("%d project(s) from %d scan root(s), %d explicit", discovered, len(cfg.ScanRoots), len(cfg.Projects))}
	}
	markers := cfg.ProjectMarkers
	if len(markers) == 0 {
		markers = config.Default().ProjectMarkers
	}
	return checkResult{Name: "projects", Status: statusWarn,
		Detail: fmt.Sprintf("scan_roots matched no projects — no direct child holds a project marker (project_markers = %q)", markers),
		Fix:    `set "project_markers" in ` + config.Path() + ` to paths your repos actually contain, e.g. [".docs/ai", ".beads", ".git"]`}
}

// isStaleUnit reports whether a service-unit filename is a hand-rolled
// harness-deck unit from an older setup. Homebrew's own unit
// (homebrew.mxcl.harness-deck.*) is the one legitimate service and is spared;
// anything else naming harness-deck is a leftover that will start a second
// server at login.
func isStaleUnit(name string) bool {
	if !strings.HasSuffix(name, ".plist") && !strings.HasSuffix(name, ".service") {
		return false
	}
	if strings.HasPrefix(name, "homebrew.mxcl.") {
		return false
	}
	lower := strings.ToLower(name)
	return strings.Contains(lower, "harness-deck") || strings.Contains(lower, "harnessdeck")
}

// findStaleUnits lists hand-rolled harness-deck service units in the user's
// per-user service directory.
func findStaleUnits() []string {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(home, "Library", "LaunchAgents")
	case "linux":
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		dir = filepath.Join(home, ".config", "systemd", "user")
	default:
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var stale []string
	for _, e := range entries {
		if !e.IsDir() && isStaleUnit(e.Name()) {
			stale = append(stale, e.Name())
		}
	}
	return stale
}

// staleUnitResult grades leftover hand-rolled units. This is a FAIL rather
// than a warning because the failure it causes is invisible to every other
// check: both servers bind :7420 at login, the loser crash-loops, and the
// survivor still answers /api/reports — so the port check goes green while the
// install is quietly broken.
// goos is passed in rather than read from runtime so both platforms' fix text
// is testable on either OS — the first version of this hard-coded runtime.GOOS
// and its test only passed on macOS.
func staleUnitResult(names []string, goos string) checkResult {
	if len(names) == 0 {
		return checkResult{Name: "service", Status: statusOK,
			Detail: "no leftover hand-rolled service units"}
	}
	label := strings.TrimSuffix(strings.TrimSuffix(names[0], ".plist"), ".service")
	fix := fmt.Sprintf("launchctl bootout \"gui/$(id -u)/%s\" 2>/dev/null; rm -f ~/Library/LaunchAgents/%s   (then rerun; use rm -f — a bare rm can prompt and silently no-op)", label, names[0])
	if goos == "linux" {
		fix = fmt.Sprintf("systemctl --user disable --now %s; rm -f ~/.config/systemd/user/%s   (then rerun)", label, names[0])
	}
	return checkResult{Name: "service", Status: statusFail,
		Detail: "hand-rolled service unit(s) left over from an older setup: " + strings.Join(names, ", ") +
			" — these start a second server at login and fight brew services for the port",
		Fix: fix}
}

// checkURLTLS catches a public_url whose scheme contradicts the tls block.
func checkURLTLS(publicURL string, tlsEnabled bool) checkResult {
	switch {
	case publicURL == "":
		return checkResult{Name: "public_url", Status: statusOK, Detail: "not set (local-only)"}
	case strings.HasPrefix(publicURL, "https://") && !tlsEnabled:
		return checkResult{Name: "public_url", Status: statusFail,
			Detail: publicURL + " promises HTTPS but no tls cert/key is configured",
			Fix:    "run `harness-deck cert` (Tailscale) or set tls.cert + tls.key"}
	case strings.HasPrefix(publicURL, "http://") && tlsEnabled:
		return checkResult{Name: "public_url", Status: statusWarn,
			Detail: "tls is configured but public_url is plain http — phones will hit the wrong scheme",
			Fix:    "change public_url to https://"}
	default:
		return checkResult{Name: "public_url", Status: statusOK, Detail: publicURL}
	}
}

// checkTLSFiles validates the configured cert + key on disk.
func checkTLSFiles(cfg config.Config) []checkResult {
	certPath := config.Expand(cfg.TLS.Cert)
	keyPath := config.Expand(cfg.TLS.Key)
	var rs []checkResult

	if _, err := tls.LoadX509KeyPair(certPath, keyPath); err != nil {
		return append(rs, checkResult{Name: "tls", Status: statusFail,
			Detail: fmt.Sprintf("cert/key pair does not load: %v", err),
			Fix:    "re-issue with `harness-deck cert`"})
	}
	rs = append(rs, checkResult{Name: "tls", Status: statusOK, Detail: "cert + key load as a pair"})

	if fi, err := os.Stat(keyPath); err == nil && fi.Mode().Perm()&0o077 != 0 {
		rs = append(rs, checkResult{Name: "tls-key", Status: statusWarn,
			Detail: fmt.Sprintf("%s is mode %o — readable beyond the owner", keyPath, fi.Mode().Perm()),
			Fix:    "chmod 600 " + keyPath})
	}

	data, err := os.ReadFile(certPath)
	if err != nil {
		return rs
	}
	leaf, err := parseLeaf(data)
	if err != nil {
		return append(rs, checkResult{Name: "tls-cert", Status: statusFail,
			Detail: "certificate does not parse: " + err.Error(), Fix: "re-issue with `harness-deck cert`"})
	}
	rs = append(rs, checkCertExpiry(leaf.NotAfter, time.Now()))
	if cfg.PublicURL != "" {
		rs = append(rs, checkCertHost(leaf, cfg.PublicURL))
	}
	return rs
}

// checkCertExpiry grades remaining certificate validity: FAIL expired, WARN
// under 30 days (tailscale certs live 90 and nothing renews them by itself).
func checkCertExpiry(notAfter, now time.Time) checkResult {
	left := notAfter.Sub(now)
	switch {
	case left <= 0:
		return checkResult{Name: "tls-expiry", Status: statusFail,
			Detail: "certificate expired " + notAfter.Format("2006-01-02"),
			Fix:    "harness-deck cert --force"}
	case left < renewThreshold:
		return checkResult{Name: "tls-expiry", Status: statusWarn,
			Detail: fmt.Sprintf("certificate expires in %dd (%s)", int(left.Hours()/24), notAfter.Format("2006-01-02")),
			Fix:    "harness-deck cert --renew (add it to cron/launchd to never see this again)"}
	default:
		return checkResult{Name: "tls-expiry", Status: statusOK,
			Detail: fmt.Sprintf("certificate valid %dd more", int(left.Hours()/24))}
	}
}

// checkCertHost verifies the cert covers the public_url hostname.
func checkCertHost(leaf *x509.Certificate, publicURL string) checkResult {
	u, err := url.Parse(publicURL)
	if err != nil || u.Hostname() == "" {
		return checkResult{Name: "tls-host", Status: statusFail,
			Detail: fmt.Sprintf("public_url %q does not parse as a URL", publicURL)}
	}
	if err := leaf.VerifyHostname(u.Hostname()); err != nil {
		return checkResult{Name: "tls-host", Status: statusFail,
			Detail: fmt.Sprintf("certificate does not cover %s (SANs: %s)", u.Hostname(), strings.Join(leaf.DNSNames, ", ")),
			Fix:    "harness-deck cert " + u.Hostname()}
	}
	return checkResult{Name: "tls-host", Status: statusOK, Detail: "certificate covers " + u.Hostname()}
}

// checkVAPID warns when push would be wanted (HTTPS public_url) but no VAPID
// keypair exists yet.
func checkVAPID(cfg config.Config) checkResult {
	path := config.Dir() + "/vapid.json"
	_, err := os.Stat(path)
	switch {
	case err == nil:
		return checkResult{Name: "push", Status: statusOK, Detail: "VAPID keypair present"}
	case strings.HasPrefix(cfg.PublicURL, "https://"):
		return checkResult{Name: "push", Status: statusWarn,
			Detail: "no VAPID keypair — push notifications cannot be enabled",
			Fix:    "harness-deck vapid"}
	default:
		return checkResult{Name: "push", Status: statusOK, Detail: "no VAPID keypair (push not in use)"}
	}
}

// serverDownResult grades "nothing is listening" as FAIL. The setup docs tell
// agents to treat `doctor` exit 0 as the install-is-done gate, so the most
// likely install failure of all — the service never came up — has to be able to
// fail that gate. (`brew services start` is async: a doctor run immediately
// after it can legitimately race the server, so an agent seeing this should
// retry once before reporting it.)
func serverDownResult(port int) checkResult {
	return checkResult{Name: "server", Status: statusFail,
		Detail: fmt.Sprintf("nothing listening on :%d", port),
		Fix:    "brew services start harness-deck (or `harness-deck serve`); if you just started it, give it a second and rerun"}
}

// checkPort reports whether the configured port is free, ours, or contested.
func checkPort(cfg config.Config) checkResult {
	addr := net.JoinHostPort("127.0.0.1", fmt.Sprint(cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return serverDownResult(cfg.Port)
	}
	conn.Close()
	scheme := "http"
	if cfg.TLS.Enabled() {
		scheme = "https"
	}
	// The dial goes to loopback, so the cert (issued for the tailnet name)
	// can't match the address. Verify the chain against the public_url
	// hostname when one is configured; only a nameless setup skips
	// verification, and then this check only identifies the service —
	// cert validity is graded separately by the tls checks above.
	tlsCfg := &tls.Config{InsecureSkipVerify: true}
	if u, err := url.Parse(cfg.PublicURL); err == nil && u.Hostname() != "" {
		tlsCfg = &tls.Config{ServerName: u.Hostname()}
	}
	client := &http.Client{Timeout: 3 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg}}
	resp, err := client.Get(fmt.Sprintf("%s://%s/api/reports", scheme, addr))
	if err != nil {
		return checkResult{Name: "server", Status: statusFail,
			Detail: fmt.Sprintf("port %d is taken but does not answer %s /api/reports — another program?", cfg.Port, scheme),
			Fix:    fmt.Sprintf("lsof -nP -iTCP:%d -sTCP:LISTEN", cfg.Port)}
	}
	resp.Body.Close()
	if needsFirewallProbe(cfg) {
		if ip := nonLoopbackIP(); ip != "" {
			extResp, extErr := client.Get(fmt.Sprintf("%s://%s/api/reports", scheme, net.JoinHostPort(ip, fmt.Sprint(cfg.Port))))
			if extErr == nil {
				extResp.Body.Close()
			}
			bin, _ := os.Executable()
			return externalReachResult(ip, cfg.Port, extErr, bin)
		}
	}
	return checkResult{Name: "server", Status: statusOK,
		Detail: fmt.Sprintf("harness-deck answering on :%d", cfg.Port)}
}

// externalReachResult grades the running server's reachability on a
// non-loopback interface — the true phone-path test. Loopback answering while
// the external dial times out is the Application Firewall signature; the ALF
// verdict is stateful and per-binary, so the fix names the binary. If the
// server runs a different binary than doctor (brew vs dev build), allowlist
// the serving one.
func externalReachResult(ip string, port int, dialErr error, binPath string) checkResult {
	if dialErr == nil {
		return checkResult{Name: "server", Status: statusOK,
			Detail: fmt.Sprintf("harness-deck answering on loopback and %s:%d", ip, port)}
	}
	return checkResult{Name: "server", Status: statusFail,
		Detail: fmt.Sprintf("server answers on loopback but not on %s:%d — macOS Application Firewall signature (%v)", ip, port, dialErr),
		Fix:    firewallFix(binPath)}
}

// firewallFix is the allowlist command pair for one binary path.
func firewallFix(binPath string) string {
	return fmt.Sprintf("sudo /usr/libexec/ApplicationFirewall/socketfilterfw --add %s && "+
		"sudo /usr/libexec/ApplicationFirewall/socketfilterfw --unblockapp %s  (rerun after brew upgrade)", binPath, binPath)
}

// checkTailscale reports daemon state when a tailnet URL is in play. The
// second return is false when the check does not apply (no tailscale in use).
func checkTailscale(cfg config.Config) (checkResult, bool) {
	usesTS := strings.Contains(cfg.PublicURL, ".ts.net")
	ts, err := findTailscale()
	if err != nil {
		if usesTS {
			return checkResult{Name: "tailscale", Status: statusFail,
				Detail: "public_url is a tailnet name but the tailscale CLI is not installed"}, true
		}
		return checkResult{}, false
	}
	out, err := exec.Command(ts, "status", "--json").Output()
	if err != nil {
		return checkResult{Name: "tailscale", Status: statusWarn,
			Detail: "tailscale status failed: " + err.Error()}, usesTS
	}
	var st struct {
		BackendState string
		Self         struct{ Online bool }
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return checkResult{Name: "tailscale", Status: statusWarn, Detail: "unparseable tailscale status"}, usesTS
	}
	if st.BackendState != "Running" {
		return checkResult{Name: "tailscale", Status: statusFail,
			Detail: "tailscale backend is " + st.BackendState,
			Fix:    "start Tailscale and connect"}, true
	}
	return checkResult{Name: "tailscale", Status: statusOK, Detail: "backend running"}, true
}

// needsFirewallProbe reports whether the server is exposed beyond loopback —
// the only case where the Application Firewall verdict matters.
func needsFirewallProbe(cfg config.Config) bool {
	switch cfg.Bind {
	case "", "127.0.0.1", "localhost", "::1":
		return false
	}
	if ip := net.ParseIP(cfg.Bind); ip != nil && ip.IsLoopback() {
		return false
	}
	return true
}

// firewallProbe listens on an ephemeral port on a non-loopback interface and
// dials itself. The macOS Application Firewall filters inbound connections
// per binary, so this reproduces exactly what a phone hitting `serve` gets —
// without needing the server up, and without sudo. A terminal run may pop the
// "accept incoming connections?" dialog; clicking Allow also fixes `serve`.
func firewallProbe() error {
	ip := nonLoopbackIP()
	if ip == "" {
		return nil // no external interface up; nothing to probe
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, "0"))
	if err != nil {
		return fmt.Errorf("listen on %s: %w", ip, err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 3*time.Second)
	if err != nil {
		return err
	}
	conn.Close()
	return nil
}

// checkFirewallProbe turns the probe outcome into a result with the exact
// allowlist commands for this binary.
func checkFirewallProbe(probeErr error, binPath string) checkResult {
	if probeErr == nil {
		return checkResult{Name: "firewall", Status: statusOK,
			Detail: "inbound connections reach this binary on non-loopback interfaces"}
	}
	return checkResult{Name: "firewall", Status: statusFail,
		Detail: "the macOS Application Firewall is dropping inbound connections to this binary (" + probeErr.Error() + ")",
		Fix:    firewallFix(binPath)}
}

// nonLoopbackIP returns one global-unicast IPv4 of this machine, "" if none.
func nonLoopbackIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipn, ok := a.(*net.IPNet)
		if !ok || ipn.IP.IsLoopback() || ipn.IP.To4() == nil {
			continue
		}
		return ipn.IP.String()
	}
	return ""
}
