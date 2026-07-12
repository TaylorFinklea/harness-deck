package main

import (
	"bytes"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// renewThreshold is how much validity may remain before --renew re-issues.
const renewThreshold = 30 * 24 * time.Hour

// cmdCert obtains a Tailscale HTTPS certificate for this machine and wires it
// into the config. It runs `tailscale cert --cert-file - --key-file -` and
// splits the combined stdout itself, because the Mac App Store build of
// Tailscale is sandboxed and cannot write cert files to any path — only the
// stdout form works on both the App Store and standalone builds. The cert +
// key land in <configdir>/tls/, and config.json gets tls.cert/tls.key (and
// public_url, when unset) patched in without disturbing other fields.
func cmdCert(args []string) {
	var host string
	renew, force := false, false
	for _, a := range args {
		switch a {
		case "--renew":
			renew = true
		case "--force", "-f":
			force = true
		case "-h", "--help":
			fmt.Println("usage: harness-deck cert [host.tailnet.ts.net] [--renew] [--force]")
			fmt.Println("  --renew  exit quietly if the existing cert is still valid >30 days")
			fmt.Println("  --force  re-issue even when --renew finds a fresh cert")
			return
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "cert: unknown flag %q\n", a)
				os.Exit(2)
			}
			if host != "" {
				fmt.Fprintln(os.Stderr, "usage: harness-deck cert [host.tailnet.ts.net] [--renew] [--force]")
				os.Exit(2)
			}
			host = a
		}
	}

	ts, err := findTailscale()
	if err != nil {
		fatal("cert", err)
	}
	if host == "" {
		host, err = tailscaleHost(ts)
		if err != nil {
			fatal("cert: resolving this machine's tailnet name", err)
		}
	}

	tlsDir := filepath.Join(config.Dir(), "tls")
	certPath := filepath.Join(tlsDir, host+".crt")
	keyPath := filepath.Join(tlsDir, host+".key")

	if renew && !force {
		if left, ok := certValidity(certPath); ok && left > renewThreshold {
			fmt.Printf("cert for %s still valid %dd — nothing to do (pass --force to re-issue)\n", host, int(left.Hours()/24))
			return
		}
	}

	// Only stdout carries PEM; tailscale logs progress to stderr, which is
	// passed through so ACME issuance doesn't look like a hang.
	cmd := exec.Command(ts, "cert", "--cert-file", "-", "--key-file", "-", host)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fatal("cert: tailscale cert", err)
	}

	certPEM, keyPEM, err := splitPEM(out.Bytes())
	if err != nil {
		fatal("cert: parsing tailscale output", err)
	}
	leaf, err := parseLeaf(certPEM)
	if err != nil {
		fatal("cert: validating certificate", err)
	}
	if err := leaf.VerifyHostname(host); err != nil {
		fatal("cert: certificate does not cover "+host, err)
	}

	if err := os.MkdirAll(tlsDir, 0o755); err != nil {
		fatal("cert: mkdir", err)
	}
	if err := writeFileAtomic(certPath, certPEM, 0o644); err != nil {
		fatal("cert: write cert", err)
	}
	if err := writeFileAtomic(keyPath, keyPEM, 0o600); err != nil {
		fatal("cert: write key", err)
	}
	fmt.Printf("wrote %s (expires %s)\n", certPath, leaf.NotAfter.Format("2006-01-02"))
	fmt.Printf("wrote %s\n", keyPath)

	cfg, err := config.Load()
	if err != nil {
		fatal("cert: config", err)
	}
	publicURL := fmt.Sprintf("https://%s:%d", host, cfg.Port)
	raw, err := os.ReadFile(config.Path())
	if err != nil && !os.IsNotExist(err) {
		fatal("cert: read config", err)
	}
	patched, err := patchConfigTLS(raw, tildify(certPath), tildify(keyPath), publicURL)
	if err != nil {
		fatal("cert: update config", err)
	}
	if err := os.MkdirAll(filepath.Dir(config.Path()), 0o755); err != nil {
		fatal("cert: mkdir config", err)
	}
	if err := writeFileAtomic(config.Path(), patched, 0o644); err != nil {
		fatal("cert: write config", err)
	}
	fmt.Printf("updated %s (tls.cert, tls.key%s)\n", config.Path(), publicURLNote(raw))
	fmt.Println("restart the server to pick up the new certificate (e.g. `brew services restart harness-deck`)")
}

// publicURLNote says whether the config patch also set public_url, purely for
// the success message.
func publicURLNote(raw []byte) string {
	var m map[string]any
	if json.Unmarshal(raw, &m) == nil {
		if s, _ := m["public_url"].(string); s != "" {
			return ""
		}
	}
	return ", public_url"
}

// splitPEM separates `tailscale cert` stdout into the certificate chain and
// the single private key, classifying blocks by PEM type rather than trusting
// their order. Non-PEM bytes between blocks are ignored.
func splitPEM(data []byte) (certPEM, keyPEM []byte, err error) {
	var certs, keys [][]byte
	for rest := data; ; {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			break
		}
		enc := pem.EncodeToMemory(b)
		switch {
		case b.Type == "CERTIFICATE":
			certs = append(certs, enc)
		case strings.HasSuffix(b.Type, "PRIVATE KEY"):
			keys = append(keys, enc)
		}
	}
	if len(keys) > 1 {
		return nil, nil, fmt.Errorf("more than one private key block (%d)", len(keys))
	}
	if len(certs) == 0 {
		return nil, nil, fmt.Errorf("no certificate block found")
	}
	if len(keys) == 0 {
		return nil, nil, fmt.Errorf("no private key block found")
	}
	return bytes.Join(certs, nil), keys[0], nil
}

// patchConfigTLS sets tls.cert/tls.key in raw config JSON, plus public_url
// when it is empty or absent, preserving every other field (known or not) by
// editing the document as a generic map. An unreadably malformed config is an
// error — never a silent wipe. A nil/empty raw starts a fresh config object.
func patchConfigTLS(raw []byte, certPath, keyPath, publicURL string) ([]byte, error) {
	m := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, fmt.Errorf("existing config does not parse (fix it or delete it first): %w", err)
		}
	}
	tlsMap, _ := m["tls"].(map[string]any)
	if tlsMap == nil {
		tlsMap = map[string]any{}
	}
	tlsMap["cert"] = certPath
	tlsMap["key"] = keyPath
	m["tls"] = tlsMap
	if s, _ := m["public_url"].(string); s == "" && publicURL != "" {
		m["public_url"] = publicURL
	}
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// parseLeaf returns the first certificate in the chain.
func parseLeaf(certPEM []byte) (*x509.Certificate, error) {
	b, _ := pem.Decode(certPEM)
	if b == nil {
		return nil, fmt.Errorf("no PEM block")
	}
	return x509.ParseCertificate(b.Bytes)
}

// certValidity reports how long the cert at path remains valid, when readable.
func certValidity(path string) (time.Duration, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	leaf, err := parseLeaf(data)
	if err != nil {
		return 0, false
	}
	return time.Until(leaf.NotAfter), true
}

// writeFileAtomic writes via a temp file + rename in the destination
// directory, so a crash never leaves a truncated cert or key behind.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// tildify rewrites a path under the user's home as ~/… so config.json stays
// portable across machines; config.Expand reverses it at load time.
func tildify(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if rel, err := filepath.Rel(home, p); err == nil && !strings.HasPrefix(rel, "..") {
		return "~/" + rel
	}
	return p
}

// findTailscale locates the tailscale CLI: $PATH first, then the Mac App
// Store bundle, whose binary is not symlinked anywhere on PATH by default.
func findTailscale() (string, error) {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p, nil
	}
	mas := "/Applications/Tailscale.app/Contents/MacOS/Tailscale"
	if _, err := os.Stat(mas); err == nil {
		return mas, nil
	}
	return "", fmt.Errorf("tailscale CLI not found on PATH or at %s — is Tailscale installed?", mas)
}

// tailscaleHost asks the local tailscaled for this machine's MagicDNS name.
func tailscaleHost(ts string) (string, error) {
	out, err := exec.Command(ts, "status", "--json").Output()
	if err != nil {
		return "", fmt.Errorf("tailscale status: %w", err)
	}
	var st struct {
		Self struct {
			DNSName string
		}
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return "", err
	}
	host := strings.TrimSuffix(st.Self.DNSName, ".")
	if host == "" {
		return "", fmt.Errorf("tailscale reports no MagicDNS name for this machine (is it connected?)")
	}
	return host, nil
}
