package herdr

import "testing"

const listFixture = `{"id":"cli:agent:list","result":{"agents":[
{"agent":"claude","agent_status":"working","cwd":"/Users/t/git/tesela","focused":false,"pane_id":"w1:p1","tab_id":"w1:t1","workspace_id":"w1","terminal_id":"term_1","agent_session":{"agent":"claude","kind":"id","source":"herdr:claude","value":"uuid-abc"}},
{"agent":"codex","agent_status":"blocked","cwd":"/Users/t/codex","focused":true,"pane_id":"w7:p1","tab_id":"w7:t1","workspace_id":"w7","terminal_id":"term_7"}
],"type":"agent_list"}}`

func TestParseAgentList(t *testing.T) {
	got, err := parseAgentList([]byte(listFixture))
	if err != nil {
		t.Fatalf("parseAgentList: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d agents, want 2", len(got))
	}
	a := got[0]
	if a.Label != "claude" || a.Status != "working" || a.Project != "tesela" {
		t.Errorf("agent[0] = %+v, want claude/working/tesela", a)
	}
	if a.Key() != "w1:p1" {
		t.Errorf("Key() = %q, want w1:p1", a.Key())
	}
	if a.SessionID != "uuid-abc" {
		t.Errorf("SessionID = %q, want uuid-abc", a.SessionID)
	}
	if !got[1].Blocked() || got[1].Project != "codex" || !got[1].Focused {
		t.Errorf("agent[1] = %+v, want blocked/codex/focused", got[1])
	}
}

func TestParseAgentListEmpty(t *testing.T) {
	got, err := parseAgentList([]byte(`{"result":{"agents":[],"type":"agent_list"}}`))
	if err != nil || len(got) != 0 {
		t.Fatalf("got (%v,%v), want empty,nil", got, err)
	}
}
