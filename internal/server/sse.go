package server

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// hub fans out change notifications to every connected SSE client.
type hub struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func newHub() *hub { return &hub{clients: make(map[chan string]struct{})} }

func (h *hub) add() chan string {
	ch := make(chan string, 4)
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
	return ch
}

func (h *hub) remove(ch chan string) {
	h.mu.Lock()
	if _, ok := h.clients[ch]; ok {
		delete(h.clients, ch)
		close(ch)
	}
	h.mu.Unlock()
}

func (h *hub) broadcast(msg string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.clients {
		select {
		case ch <- msg:
		default: // slow client — drop; it re-fetches on the next event anyway
		}
	}
}

// handleEvents streams change notifications to the browser as Server-Sent
// Events. The aggregator reconnects automatically (EventSource) if dropped.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.hub.add()
	defer s.hub.remove(ch)

	fmt.Fprint(w, "event: hello\ndata: connected\n\n")
	flusher.Flush()

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case msg, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// watchState carries the inter-tick state threaded through tick() calls.
// Keeping it as a plain struct makes the loop body a pure function of
// inputs + side-effects, which is what lets tests drive it directly.
type watchState struct {
	lastSig     string                 // changeFingerprint after the last scan
	prevAsks    map[string]askDigest   // ask digest set from the previous tick
	prevEntries map[string]store.Entry // entry map from the previous tick
}

// initWatchState captures the baseline snapshot — identical to what the
// watcher goroutine would read on startup — so the first real tick only
// fires for genuine deltas rather than re-firing the entire existing inbox.
func (s *Server) initWatchState() watchState {
	sig := s.changeFingerprint()
	prevAsks, prevEntries := s.currentAskDigests()
	return watchState{lastSig: sig, prevAsks: prevAsks, prevEntries: prevEntries}
}

// tick is one watcher iteration: scan, detect changes, broadcast SSE when
// something changed, fire ask notifications for newly-appeared open asks.
// Ask digests are only recomputed when the store signature changed —
// avoiding N full re-parses per 2 s in steady state.
//
// tick is deliberately free of time.Sleep so tests can drive it directly.
func (s *Server) tick(ws watchState) watchState {
	s.store.Scan(s.enabledRoots())
	cur := s.changeFingerprint()
	changed := cur != ws.lastSig

	var curAsks map[string]askDigest
	var curEntries map[string]store.Entry
	if changed {
		// Only recompute digests when the store actually changed — no-op
		// ticks cost nothing beyond the scan itself.
		if s.testDigestCountFn != nil {
			s.testDigestCountFn()
		}
		curAsks, curEntries = s.currentAskDigests()
		s.notifyNewAsks(ws.prevAsks, curAsks, curEntries)
		s.hub.broadcast("reports")
	} else {
		// No change — preserve the previous digest set unchanged.
		curAsks = ws.prevAsks
		curEntries = ws.prevEntries
	}

	return watchState{lastSig: cur, prevAsks: curAsks, prevEntries: curEntries}
}

// watch rescans on an interval and broadcasts when anything the dashboard
// reflects changes — reports, discovered projects, or their .docs/ai docs.
// Polling (rather than fsnotify) keeps the build dependency-free; a
// couple-second latency is imperceptible for a local dashboard.
//
// The first observed snapshot is treated as the baseline, so a backlog of
// existing open asks does not spam the phone at startup — only deltas
// after the first poll fire pushes.
func (s *Server) watch(interval time.Duration) {
	ws := s.initWatchState()
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		ws = s.tick(ws)
	}
}
