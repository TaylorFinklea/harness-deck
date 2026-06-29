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
	Read(context.Context, string) (string, bool, error)
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
func agentMergeRetained(prev, cur map[string]BlockedAgent, misses map[string]int) (map[string]BlockedAgent, map[string]int) {
	merged := map[string]BlockedAgent{}
	nextMisses := map[string]int{}
	for key, b := range cur {
		merged[key] = b
	}
	for key, b := range prev {
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
			text, _, _ := s.agents.Read(ctx, key)
			b.Question = text
			b.Since = time.Now()
			cur[key] = b
			s.notifyBlockedAgent(b)
		}
	}

	// Debounce clearing: carry forward recently-vanished blocked agents up to
	// askRetainTicks ticks so a 1-tick status flicker doesn't drop + re-page.
	merged, nextMisses := agentMergeRetained(prev.blocked, cur, prev.misses)
	return agentState{blocked: merged, misses: nextMisses}
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
