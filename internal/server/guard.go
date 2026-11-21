package server

import (
	"crypto/x509"
	"encoding/pem"
	"log"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// maxBodyBytes caps every request body. The largest legitimate body is a
// free-text answer with a note; 1 MiB is generous and stops a client from
// streaming an unbounded body into a json.Decoder.
const maxBodyBytes = 1 << 20

// guard is the request filter in front of every route. harness-deck has no
// login: on loopback the trust boundary is "this machine", and beyond
// loopback it is the tailnet. guard enforces that boundary in code instead of
// leaving it to how the user configured "bind":
//
//  1. Network: a client must connect from loopback, from one of this
//     machine's own addresses, or from a trusted network (default: the
//     Tailscale ranges). "bind": "0.0.0.0" therefore reaches the tailnet,
//     not every Wi-Fi network the laptop joins.
//  2. Host: the Host header must name this machine. A DNS-rebinding page
//     (evil.example resolving to 127.0.0.1) sends Host: evil.example and is
//     refused, so a website can't read reports through the user's browser.
//  3. Cross-origin: state-changing requests from another origin are refused
//     (stdlib http.CrossOriginProtection, using Sec-Fetch-Site / Origin). A
//     page the user visits can't answer an approval or add a webhook.
//     Non-browser clients (curl, harness scripts) send neither header and
//     pass, which is the point: they already have local file access.
//  4. Body size: request bodies are capped at maxBodyBytes.
type guard struct {
	trusted []netip.Prefix
	hosts   map[string]bool // exact lowercase names that refer to this machine
	cop     *http.CrossOriginProtection
	next    http.Handler

	localMu      sync.Mutex
	localAddrs   map[netip.Addr]bool
	localFetched time.Time
}

// newGuard builds the filter from config. An unparseable trusted_networks
// list was already rejected by config.Load, so the error is ignored here and
// the safe default (Tailscale ranges) applies.
func newGuard(cfg config.Config, next http.Handler) *guard {
	trusted, err := cfg.TrustedNets()
	if err != nil {
		trusted, _ = config.Config{}.TrustedNets()
	}
	g := &guard{
		trusted: trusted,
		hosts:   map[string]bool{},
		cop:     http.NewCrossOriginProtection(),
		next:    next,
	}
	add := func(h string) {
		h = normalizeHost(h)
		if h != "" {
			g.hosts[h] = true
		}
	}
	if name, err := os.Hostname(); err == nil {
		add(name)
		short, _, _ := strings.Cut(name, ".")
		add(short)
		add(short + ".local")
	}
	if u, err := url.Parse(cfg.PublicURL); err == nil {
		add(u.Hostname())
	}
	for _, h := range cfg.AllowedHosts {
		add(h)
	}
	for _, h := range certNames(config.Expand(cfg.TLS.Cert)) {
		add(h)
	}
	g.cop.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "cross-origin request refused: harness-deck only accepts changes from its own pages", http.StatusForbidden)
	}))
	return g
}

func (g *guard) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !g.clientAllowed(r.RemoteAddr) {
		log.Printf("harness-deck: refused %s %s from untrusted address %s (add its network to trusted_networks to allow it)", r.Method, r.URL.Path, r.RemoteAddr)
		http.Error(w, "harness-deck: this network is not trusted — add it to trusted_networks in config.json", http.StatusForbidden)
		return
	}
	if !g.hostAllowed(r.Host) {
		log.Printf("harness-deck: refused %s %s with unrecognized Host %q (add it to allowed_hosts if it names this machine)", r.Method, r.URL.Path, r.Host)
		http.Error(w, "harness-deck: unrecognized Host — add it to allowed_hosts in config.json if it names this machine", http.StatusMisdirectedRequest)
		return
	}
	if r.Body != nil {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	}
	g.cop.Handler(g.next).ServeHTTP(w, r)
}

// clientAllowed reports whether the connecting address may use the server.
func (g *guard) clientAllowed(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		// Not an IP (e.g. a unix socket or a test harness's synthetic
		// address). There is no network peer to distrust.
		return true
	}
	addr = addr.Unmap().WithZone("")
	if addr.IsLoopback() {
		return true
	}
	for _, p := range g.trusted {
		if p.Contains(addr) {
			return true
		}
	}
	return g.isLocalAddr(addr)
}

// isLocalAddr reports whether addr is assigned to one of this machine's
// interfaces (a request from the machine to its own LAN address, as doctor's
// reachability probe makes). The interface list is cached for a minute.
func (g *guard) isLocalAddr(addr netip.Addr) bool {
	g.localMu.Lock()
	defer g.localMu.Unlock()
	if g.localAddrs == nil || time.Since(g.localFetched) > time.Minute {
		g.localAddrs = map[netip.Addr]bool{}
		if addrs, err := net.InterfaceAddrs(); err == nil {
			for _, a := range addrs {
				if ipn, ok := a.(*net.IPNet); ok {
					if ip, ok := netip.AddrFromSlice(ipn.IP); ok {
						g.localAddrs[ip.Unmap()] = true
					}
				}
			}
		}
		g.localFetched = time.Now()
	}
	return g.localAddrs[addr]
}

// hostAllowed reports whether the Host header names this machine. IP
// literals can't be rebound, localhost names never leave the machine, and
// *.ts.net names are issued by Tailscale (MagicDNS / HTTPS certs), so all of
// those pass along with the configured and discovered names.
func (g *guard) hostAllowed(hostport string) bool {
	h := normalizeHost(hostport)
	if h == "" {
		return true // HTTP/1.0 without Host; nothing a browser would send
	}
	if _, err := netip.ParseAddr(h); err == nil {
		return true
	}
	if h == "localhost" || strings.HasSuffix(h, ".localhost") || strings.HasSuffix(h, ".ts.net") {
		return true
	}
	return g.hosts[h]
}

// normalizeHost strips any port and brackets, lowercases, and drops a
// trailing dot, so "MyMac.local.:7420" and "mymac.local" compare equal.
func normalizeHost(h string) string {
	h = strings.TrimSpace(h)
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	if i := strings.IndexByte(h, '%'); i >= 0 {
		h = h[:i] // IPv6 zone
	}
	return strings.TrimSuffix(strings.ToLower(h), ".")
}

// certNames returns the DNS SANs of the PEM certificate at path, or nil.
func certNames(path string) []string {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil
	}
	return cert.DNSNames
}
