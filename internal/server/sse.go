package server

import (
	"fmt"
	"net/http"
	"sync"
	"time"
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

// watch rescans on an interval and broadcasts when anything the dashboard
// reflects changes — reports, discovered projects, or their .docs/ai docs.
// Polling (rather than fsnotify) keeps the build dependency-free; a
// couple-second latency is imperceptible for a local dashboard.
func (s *Server) watch(interval time.Duration) {
	last := s.changeFingerprint()
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		s.store.Scan(s.enabledRoots())
		if cur := s.changeFingerprint(); cur != last {
			last = cur
			s.hub.broadcast("reports")
		}
	}
}
