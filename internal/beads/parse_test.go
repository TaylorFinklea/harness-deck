package beads

import "testing"

const readyFixture = `[
 {"id":"harness-deck-i8t","title":"Review and merge","status":"open","priority":0,"issue_type":"task","dependent_count":1,"labels":["merge"]},
 {"id":"harness-deck-5ph.1","title":"Beads viewer Phase 1","status":"open","priority":2,"issue_type":"feature","parent":"harness-deck-5ph"}
]`

const blockedFixture = `[
 {"id":"harness-deck-7ne","title":"Verify herdr e2e","status":"open","priority":1,"issue_type":"task","blocked_by":["harness-deck-eoz"]},
 {"id":"harness-deck-eoz","title":"Release v0.2.13","status":"open","priority":1,"issue_type":"task","blocked_by":["harness-deck-i8t"]}
]`

const statusFixture = `{"schema_version":1,"summary":{"open_issues":13,"ready_issues":10,"blocked_issues":3,"in_progress_issues":0,"total_issues":13}}`

func TestParseIssues(t *testing.T) {
	got, err := parseIssues([]byte(readyFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if got[0].ID != "harness-deck-i8t" || got[0].Priority != 0 {
		t.Errorf("bad first: %+v", got[0])
	}
	if got[0].DependentCount != 1 || len(got[0].Labels) != 1 {
		t.Errorf("bad deps/labels: %+v", got[0])
	}
	if got[1].Parent != "harness-deck-5ph" {
		t.Errorf("want parent, got %q", got[1].Parent)
	}
}

func TestParseBlockedFillsBlockedBy(t *testing.T) {
	got, err := parseBlocked([]byte(blockedFixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if len(got[0].BlockedBy) != 1 || got[0].BlockedBy[0] != "harness-deck-eoz" {
		t.Errorf("want blocked_by eoz, got %+v", got[0].BlockedBy)
	}
}

func TestParseStatus(t *testing.T) {
	c, err := parseStatus([]byte(statusFixture))
	if err != nil {
		t.Fatal(err)
	}
	if c.Ready != 10 || c.Blocked != 3 || c.Open != 13 || c.Total != 13 {
		t.Errorf("bad counts: %+v", c)
	}
}

func TestDeriveEdges(t *testing.T) {
	all, _ := parseIssues([]byte(readyFixture))
	blk, _ := parseBlocked([]byte(blockedFixture))
	edges := deriveEdges(all, blk)
	has := func(from, to, kind string) bool {
		for _, e := range edges {
			if e.From == from && e.To == to && e.Kind == kind {
				return true
			}
		}
		return false
	}
	if !has("harness-deck-eoz", "harness-deck-7ne", "blocks") {
		t.Error("missing blocks eoz->7ne")
	}
	if !has("harness-deck-i8t", "harness-deck-eoz", "blocks") {
		t.Error("missing blocks i8t->eoz")
	}
	if !has("harness-deck-5ph", "harness-deck-5ph.1", "parent") {
		t.Error("missing parent 5ph->5ph.1")
	}
}

func TestDeriveEdgesDedupes(t *testing.T) {
	all := []Issue{{ID: "b", Parent: "a"}, {ID: "b", Parent: "a"}}
	if got := deriveEdges(all, nil); len(got) != 1 {
		t.Errorf("want 1 deduped edge, got %d", len(got))
	}
}

func TestParseIssuesEmptyAndGarbage(t *testing.T) {
	if got, err := parseIssues([]byte(`[]`)); err != nil || len(got) != 0 {
		t.Errorf("empty: %v %v", got, err)
	}
	if _, err := parseIssues([]byte(`not json`)); err == nil {
		t.Error("want error on garbage")
	}
}
