package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/herdr"
	"github.com/TaylorFinklea/harness-deck/internal/push"
)

type fakeHerdr struct {
	agents []herdr.Agent
	read   string
}

func (f *fakeHerdr) List(context.Context) ([]herdr.Agent, error) { return f.agents, nil }
func (f *fakeHerdr) Read(context.Context, string) (string, bool, error) {
	return f.read, false, nil
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
