package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestEncryptRoundTrip simulates a user agent: it generates its own ECDH
// keypair + auth secret, builds the matching Subscription, encrypts a
// payload as the application server would, then decrypts on the UA side
// using only data available in the request body and headers. If the
// original payload returns intact, all the HKDF derivations and the AES
// nonce/key/aad agree end-to-end.
func TestEncryptRoundTrip(t *testing.T) {
	curve := ecdh.P256()
	uaPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatal(err)
	}
	sub := SubscriptionKeys{
		P256dh: base64.RawURLEncoding.EncodeToString(uaPriv.PublicKey().Bytes()),
		Auth:   base64.RawURLEncoding.EncodeToString(authSecret),
	}

	plaintext := []byte(`{"title":"hello","body":"world"}`)
	body, err := encrypt(plaintext, sub)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	// Parse the RFC 8188 header.
	if len(body) < 16+4+1 {
		t.Fatalf("body too short: %d", len(body))
	}
	salt := body[:16]
	recordSize := binary.BigEndian.Uint32(body[16:20])
	idlen := int(body[20])
	if idlen != 65 {
		t.Fatalf("keyid length = %d, want 65", idlen)
	}
	asPubBytes := body[21 : 21+idlen]
	ciphertext := body[21+idlen:]
	if recordSize < uint32(len(ciphertext)) {
		t.Fatalf("recordSize %d < ciphertext %d", recordSize, len(ciphertext))
	}

	// UA-side ECDH against the application server's ephemeral public.
	asPub, err := curve.NewPublicKey(asPubBytes)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := uaPriv.ECDH(asPub)
	if err != nil {
		t.Fatal(err)
	}

	uaPubBytes := uaPriv.PublicKey().Bytes()
	prkKey := hkdfExtract(authSecret, shared)
	keyInfo := append([]byte("WebPush: info\x00"), uaPubBytes...)
	keyInfo = append(keyInfo, asPubBytes...)
	ikm := hkdfExpand(prkKey, keyInfo, 32)
	prk := hkdfExtract(salt, ikm)
	cek := hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce := hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)

	block, err := aes.NewCipher(cek)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	padded, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if len(padded) == 0 || padded[len(padded)-1] != 0x02 {
		t.Fatalf("missing 0x02 trailer: % x", padded)
	}
	got := padded[:len(padded)-1]
	if string(got) != string(plaintext) {
		t.Errorf("payload roundtrip mismatch\nwant: %s\ngot:  %s", plaintext, got)
	}
}

// TestVAPIDJWTSignatureVerifies generates a keypair, signs a JWT, then
// verifies the signature with the matching public key — the same check a
// push service performs before accepting the request.
func TestVAPIDJWTSignatureVerifies(t *testing.T) {
	keys, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	token, err := keys.SignVAPID("https://push.example", "mailto:nobody@example", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d parts, want 3", len(parts))
	}
	signingInput := parts[0] + "." + parts[1]
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	if len(sig) != 64 {
		t.Fatalf("sig len = %d, want 64", len(sig))
	}
	sum := sha256.Sum256([]byte(signingInput))
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: keys.priv.PublicKey.X, Y: keys.priv.PublicKey.Y}
	if !ecdsa.Verify(pub, sum[:], r, s) {
		t.Errorf("ecdsa.Verify failed")
	}

	// Header is the fixed ES256/JWT preamble.
	header, _ := base64.RawURLEncoding.DecodeString(parts[0])
	if !strings.Contains(string(header), `"alg":"ES256"`) {
		t.Errorf("header missing ES256: %s", header)
	}

	// Claims include aud + exp + sub.
	claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if claims["aud"] != "https://push.example" {
		t.Errorf("aud = %v", claims["aud"])
	}
	if claims["sub"] != "mailto:nobody@example" {
		t.Errorf("sub = %v", claims["sub"])
	}
	if _, ok := claims["exp"].(float64); !ok {
		t.Errorf("exp not numeric: %v", claims["exp"])
	}
}

// TestKeysSaveLoadRoundTrip ensures a generated keypair survives a file
// round-trip and the loaded key signs identically.
func TestKeysSaveLoadRoundTrip(t *testing.T) {
	keys, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "vapid.json")
	if err := keys.Save(path); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PublicB64URL() != keys.PublicB64URL() {
		t.Errorf("public key mismatch after reload")
	}
	if loaded.PrivateB64URL() != keys.PrivateB64URL() {
		t.Errorf("private key mismatch after reload")
	}
}

// TestAudienceOf trims the endpoint to scheme://host.
func TestAudienceOf(t *testing.T) {
	cases := map[string]string{
		"https://fcm.googleapis.com/fcm/send/abc":                "https://fcm.googleapis.com",
		"https://updates.push.services.mozilla.com/wpush/v2/xyz": "https://updates.push.services.mozilla.com",
	}
	for in, want := range cases {
		if got := audienceOf(in); got != want {
			t.Errorf("audienceOf(%q) = %q, want %q", in, got, want)
		}
	}
}
