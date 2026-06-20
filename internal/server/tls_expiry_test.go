package server

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// makeCert generates a self-signed ECDSA certificate with the given NotAfter
// into a PEM file under dir, and returns its path.
func makeCert(t *testing.T, dir string, notAfter time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    notAfter.Add(-365 * 24 * time.Hour),
		NotAfter:     notAfter,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	path := filepath.Join(dir, "cert.pem")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create cert file: %v", err)
	}
	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		t.Fatalf("pem encode: %v", err)
	}
	f.Close()
	return path
}

func TestCertExpiryWarning(t *testing.T) {
	now := time.Now()

	t.Run("expires-in-10-days-warns", func(t *testing.T) {
		path := makeCert(t, t.TempDir(), now.Add(10*24*time.Hour))
		warn := certExpiryWarning(path, now)
		if warn == "" {
			t.Errorf("expected non-empty warning for cert expiring in 10 days, got empty")
		}
	})

	t.Run("expires-in-1-year-no-warn", func(t *testing.T) {
		path := makeCert(t, t.TempDir(), now.Add(365*24*time.Hour))
		warn := certExpiryWarning(path, now)
		if warn != "" {
			t.Errorf("expected empty warning for cert expiring in 1 year, got %q", warn)
		}
	})

	t.Run("already-expired-warns", func(t *testing.T) {
		path := makeCert(t, t.TempDir(), now.Add(-24*time.Hour))
		warn := certExpiryWarning(path, now)
		if warn == "" {
			t.Errorf("expected non-empty warning for already-expired cert, got empty")
		}
	})

	t.Run("missing-path-no-panic", func(t *testing.T) {
		warn := certExpiryWarning("/nonexistent/path/cert.pem", now)
		if warn != "" {
			t.Errorf("missing cert path should return empty warning, got %q", warn)
		}
	})

	t.Run("malformed-pem-no-panic", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "bad.pem")
		if err := os.WriteFile(path, []byte("not valid pem data"), 0o644); err != nil {
			t.Fatal(err)
		}
		warn := certExpiryWarning(path, now)
		if warn != "" {
			t.Errorf("malformed PEM should return empty warning, got %q", warn)
		}
	})
}
