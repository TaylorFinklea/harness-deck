// Package push delivers Web Push notifications to subscribed browsers,
// implementing VAPID (RFC 8292) authorization and the aes128gcm content
// encoding (RFC 8291 + RFC 8188) on top of the Go standard library —
// keeping harness-deck dependency-free.
//
// Two long-lived state objects live in the config directory next to
// config.json:
//
//   - vapid.json         the ECDSA P-256 application-server identity
//     keypair, generated once via `harness-deck vapid`.
//   - subscriptions.json the list of phones / browsers that asked to be
//     notified.
//
// For every notification we generate a fresh ephemeral ECDH P-256 keypair,
// derive a content-encryption key + nonce per RFC 8291, encrypt the JSON
// payload, and POST it to the subscription endpoint with a VAPID-signed JWT
// in the Authorization header.
package push

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
	"unicode/utf8"
)

// Subscription is the shape the browser's PushSubscription.toJSON()
// produces — what we receive from /api/push/subscribe and store verbatim.
type Subscription struct {
	Endpoint string           `json:"endpoint"`
	Keys     SubscriptionKeys `json:"keys"`
}

// SubscriptionKeys carries the UA's P-256 ECDH public key (uncompressed,
// base64url) and a 16-byte shared auth secret (base64url).
type SubscriptionKeys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// Payload is the JSON object delivered to the service worker as the
// notification body. We keep it small and explicit; the service worker
// renders it as a system notification.
type Payload struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	Tag     string `json:"tag,omitempty"`
	URL     string `json:"url,omitempty"`
	Project string `json:"project,omitempty"`
	Run     string `json:"run,omitempty"`
}

// Sender pushes payloads to subscriptions using a single long-lived VAPID
// identity. It is safe for concurrent use.
type Sender struct {
	keys    *Keys
	subject string // mailto: or https:// contact URL embedded in the JWT
	client  *http.Client
}

// NewSender builds a sender around an existing VAPID keypair. The `subject`
// is the contact (mailto: or https:// URL) push services may use to reach
// the operator about misbehaving notifications.
func NewSender(keys *Keys, subject string) *Sender {
	return &Sender{
		keys:    keys,
		subject: subject,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

// Send delivers payload to sub. It returns the HTTP status code so the
// caller can prune subscriptions the push service reports as Gone (404/410).
func (s *Sender) Send(ctx context.Context, sub Subscription, payload Payload) (int, error) {
	// Defense-in-depth: bound the body so the encrypted payload stays under the
	// aes128gcm record cap (see recordSize in encrypt.go) — a push service
	// silently drops an oversized payload. The dashboard already caps the body
	// before calling Send; this protects any other caller.
	payload.Body = boundBody(payload.Body)
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("marshal payload: %w", err)
	}
	encrypted, err := encrypt(body, sub.Keys)
	if err != nil {
		return 0, fmt.Errorf("encrypt: %w", err)
	}
	jwt, err := s.keys.SignVAPID(audienceOf(sub.Endpoint), s.subject, 12*time.Hour)
	if err != nil {
		return 0, fmt.Errorf("sign vapid jwt: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(encrypted))
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.Itoa(int((24 * time.Hour).Seconds())))
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+s.keys.PublicB64URL())

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// maxPayloadBody bounds a notification body before encryption. The aes128gcm
// record is capped (recordSize in encrypt.go) and a push service silently
// drops an oversized payload; 3000 bytes leaves ample room for the title, tag,
// URL, and JSON + encryption overhead within that record.
const maxPayloadBody = 3000

// boundBody returns body unchanged when it fits maxPayloadBody bytes, else a
// rune-boundary-safe prefix with a trailing ellipsis.
func boundBody(body string) string {
	if len(body) <= maxPayloadBody {
		return body
	}
	const ellipsis = "…"
	cut := maxPayloadBody - len(ellipsis)
	for cut > 0 && !utf8.RuneStart(body[cut]) {
		cut--
	}
	return body[:cut] + ellipsis
}

// audienceOf returns scheme://host for use as the JWT "aud" claim.
func audienceOf(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return endpoint
	}
	return u.Scheme + "://" + u.Host
}
