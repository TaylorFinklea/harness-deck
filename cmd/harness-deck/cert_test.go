package main

import (
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
)

// block builds a PEM block of the given type with junk bytes — splitPEM
// classifies by block type only; x509 validation happens on the real output.
func block(typ string) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: []byte("fake-der")})
}

func TestSplitPEM(t *testing.T) {
	cert := block("CERTIFICATE")
	ec := block("EC PRIVATE KEY")
	pkcs8 := block("PRIVATE KEY")
	rsa := block("RSA PRIVATE KEY")

	cases := []struct {
		name      string
		in        []byte
		wantCerts int
		wantKey   string // expected key block type, "" when an error is wanted
		wantErr   string
	}{
		{"cert then ec key", join(cert, ec), 1, "EC PRIVATE KEY", ""},
		{"chain of four then key", join(cert, cert, cert, cert, ec), 4, "EC PRIVATE KEY", ""},
		{"pkcs8 key", join(cert, pkcs8), 1, "PRIVATE KEY", ""},
		{"rsa key", join(cert, rsa), 1, "RSA PRIVATE KEY", ""},
		{"key before cert", join(ec, cert), 1, "EC PRIVATE KEY", ""},
		{"garbage between blocks", join(cert, []byte("# comment\n"), ec), 1, "EC PRIVATE KEY", ""},
		{"no key", join(cert, cert), 0, "", "no private key"},
		{"no cert", join(ec), 0, "", "no certificate"},
		{"two keys", join(cert, ec, pkcs8), 0, "", "more than one private key"},
		{"empty input", nil, 0, "", "no certificate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			certPEM, keyPEM, err := splitPEM(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := strings.Count(string(certPEM), "-----BEGIN CERTIFICATE-----"); got != tc.wantCerts {
				t.Errorf("cert blocks = %d, want %d", got, tc.wantCerts)
			}
			b, rest := pem.Decode(keyPEM)
			if b == nil || b.Type != tc.wantKey {
				t.Errorf("key block = %+v, want type %q", b, tc.wantKey)
			}
			if len(rest) != 0 {
				t.Errorf("key output has %d trailing bytes, want none", len(rest))
			}
		})
	}
}

func join(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestPatchConfigTLS(t *testing.T) {
	t.Run("preserves unknown fields and existing values", func(t *testing.T) {
		raw := []byte(`{
  "central_dir": "~/.harness/reports",
  "future_field": {"nested": true},
  "port": 7420,
  "public_url": "https://already.set:7420",
  "usage": {"providers": ["codex"]}
}`)
		out, err := patchConfigTLS(raw, "/x/h.crt", "/x/h.key", "https://new.example:7420")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatalf("output is not valid JSON: %v", err)
		}
		if m["future_field"] == nil {
			t.Error("unknown field future_field dropped")
		}
		if m["public_url"] != "https://already.set:7420" {
			t.Errorf("public_url overwritten: %v", m["public_url"])
		}
		tls, _ := m["tls"].(map[string]any)
		if tls["cert"] != "/x/h.crt" || tls["key"] != "/x/h.key" {
			t.Errorf("tls not patched: %v", m["tls"])
		}
		if m["usage"] == nil {
			t.Error("usage section dropped")
		}
	})

	t.Run("empty file creates config and sets public_url", func(t *testing.T) {
		out, err := patchConfigTLS(nil, "/x/h.crt", "/x/h.key", "https://h.ts.net:7420")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(out, &m); err != nil {
			t.Fatal(err)
		}
		if m["public_url"] != "https://h.ts.net:7420" {
			t.Errorf("public_url = %v", m["public_url"])
		}
		tls, _ := m["tls"].(map[string]any)
		if tls["cert"] != "/x/h.crt" {
			t.Errorf("tls.cert = %v", tls["cert"])
		}
	})

	t.Run("malformed config is an error not a wipe", func(t *testing.T) {
		if _, err := patchConfigTLS([]byte("{broken"), "/c", "/k", ""); err == nil {
			t.Fatal("want error for malformed config")
		}
	})
}
