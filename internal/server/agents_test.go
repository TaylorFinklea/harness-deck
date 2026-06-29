package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/herdr"
	"github.com/TaylorFinklea/harness-deck/internal/push"
)

type fakeHerdr struct {
	agents  []herdr.Agent
	read    string
	readErr error
	listErr error
}

func (f *fakeHerdr) List(context.Context) ([]herdr.Agent, error) { return f.agents, f.listErr }
func (f *fakeHerdr) Read(_ context.Context, _ string, _ string) (string, bool, error) {
	return f.read, false, f.readErr
}
func (f *fakeHerdr) Send(context.Context, string, string) error { return nil }

func TestTickAgentsFiresOnNewBlock(t *testing.T) {
	fh := &fakeHerdr{read: "apply migration?"}
	s := &Server{agents: fh} // minimal server; see existing tests for the pattern
	var pushes int
	s.testAgentNotifyFn = func() { pushes++ }

	// idle → no block
	fh.agents = []herdr.Agent{{PaneID: "w1:p1", Status: "idle"}}
	st := s.tickAgents(context.Background(), agentState{blocked: map[string]BlockedAgent{}, misses: map[string]int{}})
	if pushes != 0 || len(st.blocked) != 0 {
		t.Fatalf("idle: pushes=%d blocked=%d, want 0/0", pushes, len(st.blocked))
	}

	// becomes blocked (not focused) → one push, captured question
	fh.agents = []herdr.Agent{{PaneID: "w1:p1", Status: "blocked"}}
	st = s.tickAgents(context.Background(), st)
	if pushes != 1 || len(st.blocked) != 1 {
		t.Fatalf("blocked: pushes=%d blocked=%d, want 1/1", pushes, len(st.blocked))
	}
	if st.blocked["w1:p1"].Question != "apply migration?" {
		t.Errorf("question = %q", st.blocked["w1:p1"].Question)
	}

	// still blocked → no re-fire
	st = s.tickAgents(context.Background(), st)
	if pushes != 1 {
		t.Fatalf("still blocked: pushes=%d, want 1 (no re-fire)", pushes)
	}

	// focused block is suppressed
	fh.agents = []herdr.Agent{{PaneID: "w2:p1", Status: "blocked", Focused: true}}
	st = s.tickAgents(context.Background(), agentState{blocked: map[string]BlockedAgent{}, misses: map[string]int{}})
	if pushes != 1 || len(st.blocked) != 0 {
		t.Fatalf("focused: pushes=%d blocked=%d, want unchanged/0", pushes, len(st.blocked))
	}
}

func TestNotifyBlockedAgentPayload(t *testing.T) {
	var got push.Payload
	s := &Server{testAgentPushFn: func(p push.Payload) { got = p }}
	s.notifyBlockedAgent(BlockedAgent{Agent: herdr.Agent{Label: "claude", Project: "refrigate", PaneID: "w1:p1"}, Question: "apply?"})
	if got.Tag != "w1:p1" || got.URL != "/agents" {
		t.Errorf("payload = %+v, want tag w1:p1 / url /agents", got)
	}
	if got.Body != "apply?" {
		t.Errorf("body = %q", got.Body)
	}
}

func TestHandleAgentsJSON(t *testing.T) {
	s := &Server{}
	s.setAgentSnapshot([]herdr.Agent{{PaneID: "w1:p1", Status: "blocked", Project: "refrigate"}},
		map[string]BlockedAgent{"w1:p1": {Agent: herdr.Agent{PaneID: "w1:p1", Project: "refrigate"}, Question: "apply?"}})
	rr := httptest.NewRecorder()
	s.handleAgents(rr, httptest.NewRequest("GET", "/api/agents", nil))
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	var body struct {
		Blocked []BlockedAgent `json:"blocked"`
	}
	json.NewDecoder(rr.Body).Decode(&body)
	if len(body.Blocked) != 1 || body.Blocked[0].Question != "apply?" {
		t.Errorf("blocked = %+v", body.Blocked)
	}
}

// sendSpyHerdr embeds fakeHerdr and records Send calls via an onSend hook.
type sendSpyHerdr struct {
	fakeHerdr
	onSend func(target, text string)
}

func (f *sendSpyHerdr) Send(_ context.Context, target, text string) error {
	if f.onSend != nil {
		f.onSend(target, text)
	}
	return nil
}

func TestAnswerDisabledWhenAgentsNil(t *testing.T) {
	s := &Server{} // s.agents is nil: feature disabled
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents/w1:p1/answer", strings.NewReader(`{"text":"yes"}`))
	req.SetPathValue("key", "w1:p1")
	s.handleAgentAnswer(rr, req)
	if rr.Code != 503 {
		t.Fatalf("status = %d, want 503 (agents feature disabled)", rr.Code)
	}
}

func TestAnswerRefusesUnblocked(t *testing.T) {
	fh := &fakeHerdr{agents: []herdr.Agent{{PaneID: "w1:p1", Status: "working"}}}
	s := &Server{agents: fh}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents/w1:p1/answer", strings.NewReader(`{"text":"yes"}`))
	req.SetPathValue("key", "w1:p1")
	s.handleAgentAnswer(rr, req)
	if rr.Code != 409 {
		t.Fatalf("status = %d, want 409 (no longer blocked)", rr.Code)
	}
}

func TestAnswerDeliversWhenBlocked(t *testing.T) {
	sent := ""
	fh := &sendSpyHerdr{fakeHerdr: fakeHerdr{agents: []herdr.Agent{{PaneID: "w1:p1", Status: "blocked"}}}, onSend: func(_, t string) { sent = t }}
	s := &Server{agents: fh}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/agents/w1:p1/answer", strings.NewReader(`{"text":"yes"}`))
	req.SetPathValue("key", "w1:p1")
	s.handleAgentAnswer(rr, req)
	if rr.Code != 200 || sent != "yes" {
		t.Fatalf("status=%d sent=%q, want 200/yes", rr.Code, sent)
	}
}

// TestTickAgentsReadError verifies FIX 1: when fakeHerdr.Read returns an error
// the agent is still marked blocked, Question is set to the placeholder, and
// exactly one push fires (not zero, not more).
func TestTickAgentsReadError(t *testing.T) {
	fh := &fakeHerdr{readErr: errors.New("herdr read unavailable")}
	s := &Server{agents: fh}
	var pushes int
	s.testAgentNotifyFn = func() { pushes++ }

	fh.agents = []herdr.Agent{{PaneID: "w1:p1", Status: "blocked"}}
	st := s.tickAgents(context.Background(), agentState{blocked: map[string]BlockedAgent{}, misses: map[string]int{}})

	if pushes != 1 {
		t.Fatalf("pushes = %d, want 1 (error path still pages)", pushes)
	}
	if len(st.blocked) != 1 {
		t.Fatalf("blocked = %d, want 1 (agent still blocked)", len(st.blocked))
	}
	const wantQ = "(question unavailable — open the agent in herdr)"
	if st.blocked["w1:p1"].Question != wantQ {
		t.Errorf("Question = %q, want placeholder %q", st.blocked["w1:p1"].Question, wantQ)
	}
}

// TestTickAgentsFocusedClearsImmediately verifies FIX 2: a blocked+focused agent
// is dropped from the snapshot on the very next tick — it must NOT linger for
// askRetainTicks ticks like a transient disappearance would.
func TestTickAgentsFocusedClearsImmediately(t *testing.T) {
	fh := &fakeHerdr{read: "question?"}
	s := &Server{agents: fh}
	s.testAgentNotifyFn = func() {}

	// Tick 1: blocked+unfocused → enters snapshot.
	fh.agents = []herdr.Agent{{PaneID: "w1:p1", Status: "blocked", Focused: false}}
	st := s.tickAgents(context.Background(), agentState{blocked: map[string]BlockedAgent{}, misses: map[string]int{}})
	if len(st.blocked) != 1 {
		t.Fatalf("tick 1: blocked = %d, want 1", len(st.blocked))
	}

	// Tick 2: same agent now blocked+focused (intentional exclusion) →
	// must be dropped immediately, not retained for askRetainTicks ticks.
	fh.agents = []herdr.Agent{{PaneID: "w1:p1", Status: "blocked", Focused: true}}
	st = s.tickAgents(context.Background(), st)
	if len(st.blocked) != 0 {
		t.Fatalf("tick 2: blocked = %d, want 0 (focused agent cleared immediately)", len(st.blocked))
	}
}

// TestTickAgentsListError verifies FIX 3a: when fakeHerdr.List returns an error
// tickAgents returns prev unchanged, fires no push, and does not panic.
func TestTickAgentsListError(t *testing.T) {
	fh := &fakeHerdr{listErr: errors.New("herdr down")}
	s := &Server{agents: fh}
	var pushes int
	s.testAgentNotifyFn = func() { pushes++ }

	prev := agentState{
		blocked: map[string]BlockedAgent{
			"w1:p1": {Agent: herdr.Agent{PaneID: "w1:p1"}, Question: "q?"},
		},
		misses: map[string]int{},
	}
	st := s.tickAgents(context.Background(), prev)

	if pushes != 0 {
		t.Fatalf("pushes = %d, want 0 (no new agents on list error)", pushes)
	}
	if len(st.blocked) != 1 {
		t.Fatalf("blocked = %d, want 1 (prev preserved on list error)", len(st.blocked))
	}
	if st.blocked["w1:p1"].Question != "q?" {
		t.Errorf("Question = %q, want q? (question preserved on error)", st.blocked["w1:p1"].Question)
	}
}

// TestTickAgentsClearOnUnblock verifies FIX 3b: a blocked→working transition
// clears the agent from the snapshot after the askRetainTicks debounce window
// (not before, not later).
func TestTickAgentsClearOnUnblock(t *testing.T) {
	fh := &fakeHerdr{read: "question?"}
	s := &Server{agents: fh}
	var pushes int
	s.testAgentNotifyFn = func() { pushes++ }

	// Tick 1: agent blocked → enters snapshot, fires push.
	fh.agents = []herdr.Agent{{PaneID: "w1:p1", Status: "blocked"}}
	st := s.tickAgents(context.Background(), agentState{blocked: map[string]BlockedAgent{}, misses: map[string]int{}})
	if len(st.blocked) != 1 {
		t.Fatalf("tick 1: blocked = %d, want 1", len(st.blocked))
	}
	if pushes != 1 {
		t.Fatalf("tick 1: pushes = %d, want 1", pushes)
	}

	// Agent transitions to working. Drive askRetainTicks ticks through the
	// debounce window; after that the agent must be gone from the snapshot.
	fh.agents = []herdr.Agent{{PaneID: "w1:p1", Status: "working"}}
	for i := 0; i < askRetainTicks; i++ {
		st = s.tickAgents(context.Background(), st)
		if i < askRetainTicks-1 && len(st.blocked) == 0 {
			t.Fatalf("tick %d (inside debounce window): blocked prematurely cleared", i+2)
		}
	}
	if len(st.blocked) != 0 {
		t.Fatalf("after debounce window: blocked = %d, want 0", len(st.blocked))
	}
	// No additional pushes should fire during unblock.
	if pushes != 1 {
		t.Fatalf("pushes = %d after unblock, want 1 (no re-page)", pushes)
	}
}
