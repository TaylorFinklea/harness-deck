package server

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// scanWarnThreshold is the scan duration above which a WARN log line is emitted.
const scanWarnThreshold = 500 * time.Millisecond

// scanLogFloor is the minimum scan duration that produces any log output.
// Sub-floor scans are completely silent to avoid 2-second spam on healthy installs.
const scanLogFloor = 100 * time.Millisecond

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

// askRetainTicks is how many consecutive ticks an open ask may be missing
// from the recomputed digest before it is dropped from the notified baseline.
// A transiently-unreadable report (a momentary store.Get failure, a half-
// written file, a report that briefly vanishes from the scan) makes its asks
// disappear for a tick or two; retaining them keeps the reappearing ask out of
// the "new" delta so it doesn't re-fire a push. Retention applies ONLY to such
// transient disappearances — an ask whose report is still indexed but
// intentionally closed (draft, answered, or archived) is dropped immediately
// so a later re-open fires exactly once.
const askRetainTicks = 3

// watchState carries the inter-tick state threaded through tick() calls.
// Keeping it as a plain struct makes the loop body a pure function of
// inputs + side-effects, which is what lets tests drive it directly.
type watchState struct {
	lastSig     string                 // changeFingerprint after the last scan
	prevAsks    map[string]askDigest   // ask digest set from the previous tick
	prevEntries map[string]store.Entry // entry map from the previous tick
	askMisses   map[string]int         // key:id → consecutive ticks an ask has been missing from cur
	configMod   time.Time              // config.json mtime at the last tick (zero if absent)
}

// initWatchState captures the baseline snapshot — identical to what the
// watcher goroutine would read on startup — so the first real tick only
// fires for genuine deltas rather than re-firing the entire existing inbox.
func (s *Server) initWatchState() watchState {
	sig := s.changeFingerprint()
	prevAsks, prevEntries := s.currentAskDigests()
	return watchState{lastSig: sig, prevAsks: prevAsks, prevEntries: prevEntries, askMisses: map[string]int{}, configMod: configModTime()}
}

// configModTime is config.json's mtime, or the zero time if it is absent
// (running on defaults). The watcher compares it across ticks to reload the
// discovery roots when the config file changes.
func configModTime() time.Time {
	if fi, err := os.Stat(config.Path()); err == nil {
		return fi.ModTime()
	}
	return time.Time{}
}

// mergeRetainedAsks builds the next notified baseline from the freshly
// recomputed digests, carrying forward any open ask that was in the previous
// baseline but is transiently absent from cur — up to askRetainTicks ticks.
// This keeps a briefly-vanished ask out of the next tick's "new" delta so its
// reappearance does not re-fire a notification.
//
// closed is the set of keys whose report is still indexed but intentionally
// not open (draft, answered, or archived). Such a disappearance is NOT
// transient — the report was deliberately closed — so its asks are dropped
// from the baseline immediately rather than retained, ensuring a later re-open
// (e.g. flipping draft → awaiting-review) fires exactly once. Only a report
// that vanished from the index entirely, or is indexed-and-open but failed to
// read this tick, is treated as transient and retained.
//
// It returns the merged digest set and the updated miss counter.
func mergeRetainedAsks(prev, cur map[string]askDigest, misses map[string]int, closed map[string]bool) (map[string]askDigest, map[string]int) {
	merged := map[string]askDigest{}
	nextMisses := map[string]int{}
	for key, d := range cur {
		cp := askDigest{}
		for id, prompt := range d {
			cp[id] = prompt
		}
		merged[key] = cp
	}
	for key, d := range prev {
		if closed[key] {
			continue // intentionally closed — don't retain; let a re-open re-fire
		}
		for id, prompt := range d {
			if _, present := cur[key][id]; present {
				continue
			}
			tag := key + ":" + id
			if misses[tag]+1 >= askRetainTicks {
				continue
			}
			nextMisses[tag] = misses[tag] + 1
			if merged[key] == nil {
				merged[key] = askDigest{}
			}
			merged[key][id] = prompt
		}
	}
	return merged, nextMisses
}

// closedAskKeys is the set of indexed reports that are intentionally not open
// this tick — OpenAsks==0 (draft or fully answered) or archived — keyed the
// same way as the ask digests (project/run). mergeRetainedAsks uses it to tell
// an intentional close from a transient disappearance.
func (s *Server) closedAskKeys() map[string]bool {
	closed := map[string]bool{}
	for _, e := range s.store.Entries() {
		if e.OpenAsks == 0 || e.Archived {
			closed[e.Project+"/"+e.Run] = true
		}
	}
	return closed
}

// tick is one watcher iteration: scan, detect changes, broadcast SSE when
// something changed, fire ask notifications for newly-appeared open asks.
// Ask digests are only recomputed when the store signature changed —
// avoiding N full re-parses per 2 s in steady state.
//
// tick is deliberately free of time.Sleep so tests can drive it directly.
func (s *Server) tick(ws watchState) watchState {
	// Reload discovery roots when config.json changed (e.g. `register` added a
	// project) so the dashboard picks it up without a restart. Only the roots
	// are reloaded live; other config (bind, TLS, notifications) still needs a
	// restart.
	nextConfigMod := ws.configMod
	if mod := configModTime(); !mod.Equal(ws.configMod) {
		if cfg, err := config.Load(); err == nil {
			s.projects.SetRoots(cfg.ScanRoots, cfg.Projects)
		}
		nextConfigMod = mod
	}

	scanStart := time.Now()
	s.store.Scan(s.enabledRoots())
	elapsed := time.Since(scanStart)
	// testScanDuration overrides the measured value so tests can exercise
	// the warn path without actually sleeping.
	if s.testScanDuration > 0 {
		elapsed = s.testScanDuration
	}
	s.logScanTiming(elapsed, len(s.store.Entries()))

	cur := s.changeFingerprint()
	changed := cur != ws.lastSig

	nextAsks := ws.prevAsks
	nextEntries := ws.prevEntries
	nextMisses := ws.askMisses
	if changed {
		// Only recompute digests when the store actually changed — no-op
		// ticks cost nothing beyond the scan itself.
		if s.testDigestCountFn != nil {
			s.testDigestCountFn()
		}
		curAsks, curEntries := s.currentAskDigests()
		s.notifyNewAsks(ws.prevAsks, curAsks, curEntries)
		s.hub.broadcast("reports")
		// Carry forward briefly-vanished open asks so a transient read
		// failure does not re-fire their push when they reappear — but not
		// asks whose report was intentionally closed (draft/answered/archived),
		// so flipping such a report back to open fires exactly once.
		nextAsks, nextMisses = mergeRetainedAsks(ws.prevAsks, curAsks, ws.askMisses, s.closedAskKeys())
		nextEntries = curEntries
	}

	return watchState{lastSig: cur, prevAsks: nextAsks, prevEntries: nextEntries, askMisses: nextMisses, configMod: nextConfigMod}
}

// logScanTiming emits a scan-duration log line when the scan took longer
// than the quiet floor (100ms). Scans above the warn threshold (500ms) get
// a "WARN" prefix so slow-disk or large-repo situations are easy to grep.
// Sub-floor scans are silent so typical installs (millisecond scans) never
// produce per-tick noise.
func (s *Server) logScanTiming(d time.Duration, n int) {
	if d < scanLogFloor {
		return
	}
	var msg string
	if d >= scanWarnThreshold {
		msg = fmt.Sprintf("harness-deck: WARN slow scan: %v for %d entries", d.Round(time.Millisecond), n)
	} else {
		msg = fmt.Sprintf("harness-deck: scan: %v for %d entries", d.Round(time.Millisecond), n)
	}
	if s.testScanLogFn != nil {
		s.testScanLogFn(msg)
		return
	}
	log.Print(msg)
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
