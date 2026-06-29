package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/herdr"
	"github.com/TaylorFinklea/harness-deck/internal/push"
)

// agentSnapshot is the mutex-guarded latest state served by GET /api/agents.
// setAgentSnapshot writes it; handleAgents reads it.
type agentSnapshot struct {
	mu      sync.RWMutex
	agents  []herdr.Agent
	blocked map[string]BlockedAgent
}

// herdrClient is the interface the agent watcher uses to interact with herdr.
// *herdr.Client satisfies it; tests inject a fakeHerdr.
type herdrClient interface {
	List(context.Context) ([]herdr.Agent, error)
	Read(context.Context, string, string) (string, bool, error)
	Send(context.Context, string, string) error
}

// BlockedAgent is a herdr agent that is waiting on user input.
type BlockedAgent struct {
	herdr.Agent
	Question string    // captured pane text (trimmed)
	Since    time.Time // first observed blocked (server clock)
}

// agentState carries the inter-tick state threaded through tickAgents() calls.
// Mirroring watchState for the report watcher, keeping it as a plain struct
// makes tickAgents a pure function of inputs + side-effects, testable without
// goroutines.
type agentState struct {
	blocked map[string]BlockedAgent // pane_id → blocked agent
	misses  map[string]int          // pane_id → consecutive ticks absent from blocked set
}

// agentMergeRetained is the agent-channel sibling of mergeRetainedAsks.
// It carries forward any blocked agent from prev that is transiently absent
// from cur (up to askRetainTicks) to prevent a 1-tick status flicker from
// clearing the entry and re-paging on the next reappearance.
//
// excluded is the set of keys that are present in the live herdr agent list
// but intentionally not in cur this tick (blocked+focused: user is already at
// the terminal). Such an exclusion is NOT transient — the agent was suppressed
// on purpose — so those keys are dropped from the retained baseline immediately,
// mirroring how mergeRetainedAsks uses the closed set in sse.go.
func agentMergeRetained(prev, cur map[string]BlockedAgent, misses map[string]int, excluded map[string]bool) (map[string]BlockedAgent, map[string]int) {
	merged := map[string]BlockedAgent{}
	nextMisses := map[string]int{}
	for key, b := range cur {
		merged[key] = b
	}
	for key, b := range prev {
		if excluded[key] {
			continue // intentionally focused — drop immediately, not after retain window
		}
		if _, present := cur[key]; present {
			continue
		}
		if misses[key]+1 >= askRetainTicks {
			continue // expired: drop from the retained set
		}
		nextMisses[key] = misses[key] + 1
		merged[key] = b
	}
	return merged, nextMisses
}

// tickAgents is one agent-watcher iteration: diff the current herdr status
// against prev, fire notifyBlockedAgent for each newly-blocked agent, and
// apply a debounce on clearing to avoid re-paging on transient status flickers.
//
// tickAgents is deliberately free of time.Sleep so tests can drive it directly,
// mirroring the tick/watchState pattern in sse.go.
func (s *Server) tickAgents(ctx context.Context, prev agentState) agentState {
	agents, err := s.agents.List(ctx)
	if err != nil {
		// herdr unreachable or down: keep the previous state, don't clear the
		// inbox on a transient failure. Log once so the operator can investigate.
		log.Printf("harness-deck: herdr list: %v", err)
		return prev
	}

	// Build the new blocked set: blocked AND not focused (focused = user is
	// already at the terminal, no notification needed).
	cur := map[string]BlockedAgent{}
	for _, a := range agents {
		if a.Blocked() && !a.Focused {
			cur[a.Key()] = BlockedAgent{Agent: a}
		}
	}

	// Detect newly-blocked agents (in cur but NOT in prev.blocked) and fire
	// push. Agents that are still blocked carry their existing Question and Since.
	for key, b := range cur {
		if existing, existed := prev.blocked[key]; existed {
			// Still blocked: preserve the captured question and initial timestamp.
			cur[key] = existing
		} else {
			// Newly blocked: read the pane to capture the question.
			const questionPlaceholder = "(question unavailable — open the agent in herdr)"
			text, truncated, err := s.agents.Read(ctx, key, "visible")
			if err != nil || text == "" {
				b.Question = questionPlaceholder
				log.Printf("harness-deck: herdr read %s: %v", key, err)
			} else {
				// herdr signals truncation when the viewport clipped the question;
				// retry once with the scrollback window to recover the full text.
				if truncated {
					if text2, _, err2 := s.agents.Read(ctx, key, "recent"); err2 == nil && text2 != "" {
						text = text2
					}
				}
				b.Question = text
			}
			b.Since = time.Now()
			cur[key] = b
			s.notifyBlockedAgent(b)
		}
	}

	// Compute the intentionally-excluded set: blocked+focused agents that were
	// suppressed from cur this tick. Unlike a transient disappearance (e.g. a
	// 1-tick herdr hiccup), a focused agent should NOT be retained — the user is
	// already at that terminal, so dropping the inbox entry immediately is correct.
	excluded := map[string]bool{}
	for _, a := range agents {
		if a.Blocked() && a.Focused {
			excluded[a.Key()] = true
		}
	}

	// Debounce clearing: carry forward recently-vanished blocked agents up to
	// askRetainTicks ticks so a 1-tick status flicker doesn't drop + re-page.
	// Intentionally-excluded (focused) agents are NOT retained.
	merged, nextMisses := agentMergeRetained(prev.blocked, cur, prev.misses, excluded)
	s.setAgentSnapshot(agents, merged)
	return agentState{blocked: merged, misses: nextMisses}
}

// watchAgents polls herdr on an interval, calling tickAgents each tick to
// detect newly-blocked agents and fire push/SSE. It is the agent-channel
// sibling of watch in sse.go and follows the same tick/state pattern.
// Only called when s.agents != nil (gated by cfg.Agents.Enabled in New).
func (s *Server) watchAgents(interval time.Duration) {
	st := agentState{blocked: map[string]BlockedAgent{}, misses: map[string]int{}}
	t := time.NewTicker(interval)
	defer t.Stop()
	for range t.C {
		st = s.tickAgents(context.Background(), st)
	}
}

// setAgentSnapshot stores the latest tick output so GET /api/agents can serve
// it without holding the watcher goroutine. Mirrors how usage.Monitor.Samples()
// caches usage snapshots for /api/usage.
func (s *Server) setAgentSnapshot(agents []herdr.Agent, blocked map[string]BlockedAgent) {
	s.agentsSnap.mu.Lock()
	s.agentsSnap.agents = agents
	s.agentsSnap.blocked = blocked
	s.agentsSnap.mu.Unlock()
}

// handleAgents serves the live agent snapshot as JSON:
//
//	{ "blocked": [...BlockedAgent], "agents": [...Agent] }
//
// "blocked" is the subset waiting on user input (not focused); "agents" is the
// full fleet from the last tick. Both are empty arrays (never null) when herdr
// has not reported any agents yet.
func (s *Server) handleAgents(w http.ResponseWriter, _ *http.Request) {
	s.agentsSnap.mu.RLock()
	agents := s.agentsSnap.agents
	blockedMap := s.agentsSnap.blocked
	s.agentsSnap.mu.RUnlock()

	// Convert blocked map to a stable slice for JSON serialisation.
	blocked := make([]BlockedAgent, 0, len(blockedMap))
	for _, b := range blockedMap {
		blocked = append(blocked, b)
	}
	if agents == nil {
		agents = []herdr.Agent{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"blocked": blocked,
		"agents":  agents,
	})
}

// handleAgentAnswer handles POST /api/agents/{key}/answer.
// Body: { "text": "…" }. It re-checks the agent's live status before
// delivering so a stale answer never gets sent into an active session.
//
// 400 — missing or empty text.
// 409 — agent not found in herdr's current list, or no longer blocked.
// 503 — agents feature disabled (s.agents is nil).
// 200 — answer delivered; broadcasts SSE "agents" / "answered".
func (s *Server) handleAgentAnswer(w http.ResponseWriter, r *http.Request) {
	if s.agents == nil {
		http.Error(w, "agents feature disabled", http.StatusServiceUnavailable)
		return
	}
	key := r.PathValue("key")

	var body struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if body.Text == "" {
		http.Error(w, "text is required", http.StatusBadRequest)
		return
	}

	// Re-check live status before delivering to avoid sending stale answers.
	ctx := r.Context()
	agents, err := s.agents.List(ctx)
	if err != nil {
		http.Error(w, "herdr unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	var target *herdr.Agent
	for i := range agents {
		if agents[i].Key() == key {
			target = &agents[i]
			break
		}
	}
	if target == nil || !target.Blocked() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"agent no longer blocked"}`))
		return
	}

	if err := s.agents.Send(ctx, key, body.Text); err != nil {
		http.Error(w, "herdr send failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	if s.hub != nil {
		s.hub.broadcastEvent("agents", "answered")
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// notifyBlockedAgent fires on a newly-blocked agent: fires the test seam
// (for Task 5 test compatibility), sends a Web Push to all subscriptions, and
// broadcasts an SSE "agents" event so any open live view updates immediately.
// Mirrors notifyNewAsks in push.go.
func (s *Server) notifyBlockedAgent(b BlockedAgent) {
	// Fire the count seam first so Task 5 tests pass without VAPID keys.
	if s.testAgentNotifyFn != nil {
		s.testAgentNotifyFn()
	}

	p := push.Payload{
		Title: b.Project + " — " + b.Label + " needs you",
		Body:  truncateBody(b.Question),
		Tag:   b.Key(),
		URL:   "/agents",
	}

	if s.testAgentPushFn != nil {
		// Payload seam: lets tests assert on Tag/URL/Body without real VAPID.
		s.testAgentPushFn(p)
	} else if s.pushEnabled() && s.subs != nil && s.subs.Count() > 0 {
		go s.deliverPush(p)
	}

	// Broadcast so any connected browser view refreshes immediately.
	if s.hub != nil {
		s.hub.broadcastEvent("agents", "blocked")
	}
}
