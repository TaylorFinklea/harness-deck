package push

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
)

// encrypt produces an aes128gcm-encoded Web Push body (RFC 8291) for
// payload, addressed to the UA identified by keys. The returned bytes go
// straight into the POST request body with Content-Encoding: aes128gcm.
//
// Layout:
//
//	header     ::= salt(16) || record_size(4, big-endian) || idlen(1) || as_public(idlen)
//	ciphertext ::= AES-128-GCM(CEK, NONCE, payload || 0x02)  + 16-byte tag
//	body       ::= header || ciphertext
//
// CEK and NONCE are derived per RFC 8291 §3.4 using HKDF-SHA256 in two
// stages mixed with the UA's auth secret and the shared ECDH secret.
func encrypt(payload []byte, keys SubscriptionKeys) ([]byte, error) {
	uaPubBytes, err := base64.RawURLEncoding.DecodeString(addPaddingTrim(keys.P256dh))
	if err != nil {
		return nil, fmt.Errorf("decode p256dh: %w", err)
	}
	authSecret, err := base64.RawURLEncoding.DecodeString(addPaddingTrim(keys.Auth))
	if err != nil {
		return nil, fmt.Errorf("decode auth: %w", err)
	}
	if len(authSecret) != 16 {
		return nil, fmt.Errorf("auth secret wrong length: got %d, want 16", len(authSecret))
	}

	curve := ecdh.P256()
	uaPub, err := curve.NewPublicKey(uaPubBytes)
	if err != nil {
		return nil, fmt.Errorf("parse ua public key: %w", err)
	}
	asPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral key: %w", err)
	}
	asPubBytes := asPriv.PublicKey().Bytes() // 65 bytes uncompressed
	shared, err := asPriv.ECDH(uaPub)
	if err != nil {
		return nil, fmt.Errorf("ecdh: %w", err)
	}

	// Stage 1: mix the auth secret into the ECDH output, binding the IKM
	// to both endpoints' public keys (RFC 8291 §3.4).
	prkKey := hkdfExtract(authSecret, shared)
	keyInfo := append([]byte("WebPush: info\x00"), uaPubBytes...)
	keyInfo = append(keyInfo, asPubBytes...)
	ikm := hkdfExpand(prkKey, keyInfo, 32)

	// Stage 2: derive CEK + NONCE from a random salt per RFC 8188.
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	prk := hkdfExtract(salt, ikm)
	cek := hkdfExpand(prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce := hkdfExpand(prk, []byte("Content-Encoding: nonce\x00"), 12)

	// Pad to a single record. 0x02 is the last-record delimiter (RFC 8188).
	padded := make([]byte, 0, len(payload)+1)
	padded = append(padded, payload...)
	padded = append(padded, 0x02)

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nil, nonce, padded, nil)

	// recordSize is the encrypted record size cap. 4096 is the typical
	// browser-supported maximum; our payloads are tiny, but the field
	// must exceed the actual record length.
	var recordSize uint32 = 4096

	body := make([]byte, 0, 16+4+1+len(asPubBytes)+len(ciphertext))
	body = append(body, salt...)
	rs := make([]byte, 4)
	binary.BigEndian.PutUint32(rs, recordSize)
	body = append(body, rs...)
	body = append(body, byte(len(asPubBytes)))
	body = append(body, asPubBytes...)
	body = append(body, ciphertext...)
	return body, nil
}

// hkdfExtract is HKDF-SHA256-Extract: HMAC(salt, ikm).
func hkdfExtract(salt, ikm []byte) []byte {
	h := hmac.New(sha256.New, salt)
	h.Write(ikm)
	return h.Sum(nil)
}

// hkdfExpand is HKDF-SHA256-Expand truncated to length bytes. Web Push
// only ever asks for a single 32-byte block, so the loop variant in
// RFC 5869 is unnecessary; we still write the trailing 0x01 counter
// because the IKM/CEK/NONCE info strings are defined without it.
func hkdfExpand(prk, info []byte, length int) []byte {
	if length > sha256.Size {
		panic("push: hkdfExpand only supports lengths ≤ 32 bytes (single block)")
	}
	h := hmac.New(sha256.New, prk)
	h.Write(info)
	h.Write([]byte{0x01})
	return h.Sum(nil)[:length]
}

// addPaddingTrim accepts either standard or url-safe base64, with or
// without padding, and returns a form RawURLEncoding can decode. Browsers
// hand us un-padded url-safe; some libraries pad.
func addPaddingTrim(s string) string {
	// Strip any padding the caller included.
	for len(s) > 0 && s[len(s)-1] == '=' {
		s = s[:len(s)-1]
	}
	return s
}
