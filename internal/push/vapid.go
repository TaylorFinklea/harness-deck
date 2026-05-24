package push

import (
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// SignVAPID produces a VAPID JWT (RFC 8292) authorizing one push request:
// header is fixed {alg:ES256, typ:JWT}; payload has aud (push origin),
// exp (now+ttl, capped by the spec at 24h), and sub (operator contact).
func (k *Keys) SignVAPID(audience, subject string, ttl time.Duration) (string, error) {
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	header := map[string]string{"typ": "JWT", "alg": "ES256"}
	claims := map[string]any{
		"aud": audience,
		"exp": time.Now().Add(ttl).Unix(),
		"sub": subject,
	}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." +
		base64.RawURLEncoding.EncodeToString(claimsJSON)

	sum := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, k.priv, sum[:])
	if err != nil {
		return "", fmt.Errorf("ecdsa sign: %w", err)
	}
	// ES256 signature is fixed-width r || s, each 32 bytes.
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}
