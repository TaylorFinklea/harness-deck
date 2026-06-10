package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/manifest"
	"github.com/TaylorFinklea/harness-deck/internal/notify"
	"github.com/TaylorFinklea/harness-deck/internal/push"
	"github.com/TaylorFinklea/harness-deck/internal/respond"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// pushEnabled reports whether VAPID keys are loaded. Endpoints return 503
// when disabled so the settings UI can suggest running `harness-deck vapid`.
func (s *Server) pushEnabled() bool { return s.pushKeys != nil }

// handleVAPIDKey returns the application-server public key so the browser
// can pass it to PushManager.subscribe().
func (s *Server) handleVAPIDKey(w http.ResponseWriter, _ *http.Request) {
	if !s.pushEnabled() {
		http.Error(w, "push not configured — run `harness-deck vapid`", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"key": s.pushKeys.PublicB64URL(),
	})
}

// handlePushSubscribe stores a browser-side PushSubscription. The body is
// whatever subscription.toJSON() produced; we store it verbatim.
func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	if !s.pushEnabled() {
		http.Error(w, "push not configured", http.StatusServiceUnavailable)
		return
	}
	var sub push.Subscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if sub.Endpoint == "" || sub.Keys.P256dh == "" || sub.Keys.Auth == "" {
		http.Error(w, "subscription missing endpoint or keys", http.StatusBadRequest)
		return
	}
	if err := s.subs.Add(sub); err != nil {
		http.Error(w, "store subscription: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// pushUnsubscribeRequest is the POST body for removing a subscription.
type pushUnsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

// handlePushUnsubscribe drops the subscription identified by its endpoint.
// Idempotent.
func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	var req pushUnsubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Endpoint == "" {
		http.Error(w, "missing endpoint", http.StatusBadRequest)
		return
	}
	if err := s.subs.Remove(req.Endpoint); err != nil {
		http.Error(w, "remove: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handlePushStatus reports whether push is wired up and how many
// subscriptions are stored — used by the settings view header.
func (s *Server) handlePushStatus(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"enabled":            s.pushEnabled(),
		"subscription_count": s.subs.Count(),
	}
	if s.pushEnabled() {
		resp["public_key"] = s.pushKeys.PublicB64URL()
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// askDigest is the fingerprint of one report's open interactive blocks:
// block_id → prompt text. The watcher diffs digests between polls and
// sends a push for every block id that appears in the new digest but not
// the previous one. Keying on id rather than count means answering one
// ask never re-fires a push, and an answered ask being replaced by a
// fresh ask still does.
type askDigest map[string]string

// currentAskDigests builds a digest for every indexed entry that has
// open asks, plus a quick lookup table from key → entry. A read failure
// for one report is logged and silently dropped — the next poll will
// retry, and the missed push is worth less than the risk of a watcher
// crash.
func (s *Server) currentAskDigests() (map[string]askDigest, map[string]store.Entry) {
	digests := map[string]askDigest{}
	entries := map[string]store.Entry{}
	for _, e := range s.store.Entries() {
		if e.OpenAsks == 0 || e.Archived {
			continue
		}
		key := e.Project + "/" + e.Run
		entries[key] = e
		rep, _, err := s.store.Get(e.Project, e.Run)
		if err != nil || rep == nil {
			continue
		}
		answers, _ := respond.Load(e.Dir)
		d := askDigest{}
		for _, b := range rep.Blocks {
			if !manifest.InteractiveTypes[b.Type] {
				continue
			}
			id := manifest.InteractiveID(b)
			if id == "" {
				continue
			}
			if _, answered := answers.Responses[id]; answered {
				continue
			}
			d[id] = blockPrompt(b)
		}
		if len(d) > 0 {
			digests[key] = d
		}
	}
	return digests, entries
}

// blockPrompt returns the human-readable question text for an interactive
// block, falling back to the block title if no prompt is set.
func blockPrompt(b manifest.Block) string {
	switch body := b.Body.(type) {
	case *manifest.AskBlock:
		if body.Prompt != "" {
			return body.Prompt
		}
	case *manifest.DecisionBlock:
		if body.Prompt != "" {
			return body.Prompt
		}
	case *manifest.ApprovalBlock:
		if body.Prompt != "" {
			return body.Prompt
		}
	}
	if b.Body != nil {
		if t := b.Body.PanelTitle(); t != "" {
			return t
		}
	}
	return b.Type
}

// notifyNewAsks sends one push per ask that appeared in cur but not prev
// AND fans the same notification out to every configured destination
// (Slack / Discord / webhook). First-tick prev is empty, so an existing
// inbox of asks does not spam either channel at startup — only delta
// from the first observed snapshot.
func (s *Server) notifyNewAsks(prev, cur map[string]askDigest, entries map[string]store.Entry) {
	havePush := s.pushEnabled() && s.subs.Count() > 0
	s.notifMu.RLock()
	haveFanout := len(s.cfg.Notifications) > 0
	s.notifMu.RUnlock()
	haveTestSeam := s.testNotifyFn != nil
	if !havePush && !haveFanout && !haveTestSeam {
		return
	}
	for key, curAsks := range cur {
		prevAsks := prev[key]
		for id, prompt := range curAsks {
			if _, existed := prevAsks[id]; existed {
				continue
			}
			// Fire the test seam, if any, once per new ask (before any real
			// push/fanout so tests can count without needing real VAPID keys).
			if s.testNotifyFn != nil {
				s.testNotifyFn()
			}
			entry := entries[key]
			title := entry.Title
			if title == "" {
				title = entry.Run
			}
			reportPath := "/r/" + entry.Project + "/" + entry.Run
			if havePush {
				go s.deliverPush(push.Payload{
					Title:   entry.Project + " — " + title,
					Body:    prompt,
					Tag:     key + ":" + id,
					URL:     reportPath, // service worker resolves against origin
					Project: entry.Project,
					Run:     entry.Run,
				})
			}
			if haveFanout {
				// Snapshot under the read lock so a CRUD edit happening
				// mid-tick doesn't slice into a half-written slice.
				s.notifMu.RLock()
				dests := append([]notify.Destination(nil), s.cfg.Notifications...)
				s.notifMu.RUnlock()
				notify.Fanout(context.Background(), notify.Notification{
					Title:   entry.Project + " — " + title,
					Body:    prompt,
					Tag:     key + ":" + id,
					URL:     s.publicReportURL(entry.Project, entry.Run),
					Project: entry.Project,
					Run:     entry.Run,
				}, dests, log.Printf)
			}
		}
	}
}

// publicReportURL builds an absolute, externally-reachable URL for a
// report — what Slack / Discord need so a click in chat lands on the
// dashboard page. Prefers cfg.PublicURL when set; otherwise falls back
// to bind+port+TLS-scheme, which works for localhost but produces
// "0.0.0.0:7420" for an open bind (links won't resolve externally — the
// PublicURL field exists to fix that).
func (s *Server) publicReportURL(project, run string) string {
	base := s.cfg.PublicURL
	if base == "" {
		scheme := "http"
		if s.cfg.TLS.Enabled() {
			scheme = "https"
		}
		base = fmt.Sprintf("%s://%s:%d", scheme, s.cfg.Bind, s.cfg.Port)
	}
	return base + "/r/" + project + "/" + run
}

// deliverPush sends one payload to every stored subscription, dropping
// any the push service reports as Gone (404/410) so stale entries don't
// accumulate forever.
func (s *Server) deliverPush(payload push.Payload) {
	subs := s.subs.All()
	if len(subs) == 0 || s.pushKeys == nil {
		return
	}
	sender := push.NewSender(s.pushKeys, defaultPushSubject)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, sub := range subs {
		status, err := sender.Send(ctx, sub, payload)
		switch {
		case err != nil:
			log.Printf("harness-deck: push send error: %v", err)
		case status == 404 || status == 410:
			log.Printf("harness-deck: dropping gone subscription %s", sub.Endpoint)
			_ = s.subs.Remove(sub.Endpoint)
		case status >= 400:
			log.Printf("harness-deck: push rejected %d for %s", status, sub.Endpoint)
		}
	}
}

// defaultPushSubject is the contact embedded in the VAPID JWT's "sub"
// claim per RFC 8292. Apple's Push Service is the strictest validator
// of this field — it rejects with HTTP 403 when the sub is a mailto:
// without a real public TLD ("mailto:harness-deck@localhost" fails,
// for example). The repo URL form is universally accepted: Apple
// only checks URL syntax, not reachability, so no live endpoint is
// needed and we still leak no operator PII.
const defaultPushSubject = "https://github.com/TaylorFinklea/harness-deck"
