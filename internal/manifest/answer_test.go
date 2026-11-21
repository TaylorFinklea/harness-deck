package manifest

import "testing"

func TestCheckAnswer(t *testing.T) {
	rep, err := Parse([]byte(`{"schema":"harness-deck/report@1","id":"r","title":"t","project":"p","harness":"h","created":"2026-01-01T00:00:00Z","status":"awaiting-review","blocks":[
		{"type":"ask","id":"pick","prompt":"which?","options":["red","blue"]},
		{"type":"ask","id":"yn","prompt":"ok?","mode":"yesno"},
		{"type":"ask","id":"many","prompt":"which?","mode":"multi","options":["a","b","c"]},
		{"type":"ask","id":"free","prompt":"why?"},
		{"type":"decision","id":"path","a":{"tag":"A","title":"x","items":[]},"b":{"tag":"B","title":"y","items":[]}},
		{"type":"approval","id":"ship"},
		{"type":"prose","markdown":"hi"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		block  string
		value  string
		values []string
		ok     bool
	}{
		{"choice option", "pick", "red", nil, true},
		{"choice off-menu", "pick", "green", nil, false},
		{"yes", "yn", "yes", nil, true},
		{"yesno other", "yn", "maybe", nil, false},
		{"multi subset", "many", "", []string{"a", "c"}, true},
		{"multi empty", "many", "", nil, false},
		{"multi off-menu", "many", "", []string{"a", "z"}, false},
		{"text", "free", "because", nil, true},
		{"text blank", "free", "   ", nil, false},
		{"decision tag", "path", "B", nil, true},
		{"decision other", "path", "C", nil, false},
		{"approve", "ship", ApprovalApproved, nil, true},
		{"request changes", "ship", ApprovalChangesRequested, nil, true},
		{"approval free text", "ship", "approve", nil, false},
		{"unknown block", "nope", "x", nil, false},
	}
	for _, c := range cases {
		err := CheckAnswer(rep, c.block, c.value, c.values)
		if (err == nil) != c.ok {
			t.Errorf("%s: err=%v, want ok=%v", c.name, err, c.ok)
		}
	}
}
