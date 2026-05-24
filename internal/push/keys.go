package push

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"os"
)

// Keys holds the application-server VAPID identity — a P-256 ECDSA keypair
// kept stable across restarts so existing subscriptions remain valid.
type Keys struct {
	priv *ecdsa.PrivateKey
}

// Generate creates a new VAPID keypair.
func Generate() (*Keys, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate vapid keypair: %w", err)
	}
	return &Keys{priv: priv}, nil
}

// PublicB64URL returns the uncompressed P-256 public key as base64url
// (no padding) — the form expected by the browser's applicationServerKey
// and by the VAPID Authorization header's `k=` parameter.
func (k *Keys) PublicB64URL() string {
	return base64.RawURLEncoding.EncodeToString(k.publicBytes())
}

// PrivateB64URL returns the 32-byte private scalar as base64url.
func (k *Keys) PrivateB64URL() string {
	d := k.priv.D.FillBytes(make([]byte, 32))
	return base64.RawURLEncoding.EncodeToString(d)
}

// publicBytes is the 65-byte uncompressed encoding (0x04 || X || Y).
func (k *Keys) publicBytes() []byte {
	x := k.priv.PublicKey.X.FillBytes(make([]byte, 32))
	y := k.priv.PublicKey.Y.FillBytes(make([]byte, 32))
	out := make([]byte, 0, 65)
	out = append(out, 0x04)
	out = append(out, x...)
	out = append(out, y...)
	return out
}

// fileFormat is what we write to vapid.json. We keep both halves
// base64url-encoded so the file is human-readable and trivially diffable.
type fileFormat struct {
	Public  string `json:"public"`
	Private string `json:"private"`
}

// Save writes the keypair to path with 0600 permissions.
func (k *Keys) Save(path string) error {
	data, err := json.MarshalIndent(fileFormat{
		Public:  k.PublicB64URL(),
		Private: k.PrivateB64URL(),
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Load reads a keypair previously saved with Save.
func Load(path string) (*Keys, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ff fileFormat
	if err := json.Unmarshal(data, &ff); err != nil {
		return nil, fmt.Errorf("parse vapid file: %w", err)
	}
	priv, err := base64.RawURLEncoding.DecodeString(ff.Private)
	if err != nil {
		return nil, fmt.Errorf("decode private: %w", err)
	}
	if len(priv) != 32 {
		return nil, fmt.Errorf("vapid private key wrong length: got %d, want 32", len(priv))
	}
	k := &ecdsa.PrivateKey{}
	k.Curve = elliptic.P256()
	k.D = new(big.Int).SetBytes(priv)
	k.PublicKey.X, k.PublicKey.Y = elliptic.P256().ScalarBaseMult(priv)
	return &Keys{priv: k}, nil
}

// LoadOrMissing returns the keys if present, or nil, false (no error) when
// the file simply doesn't exist — push features stay dormant until the
// user runs `harness-deck vapid`.
func LoadOrMissing(path string) (*Keys, bool, error) {
	k, err := Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return k, true, nil
}
