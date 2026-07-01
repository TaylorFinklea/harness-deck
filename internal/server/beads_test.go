package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TaylorFinklea/harness-deck/internal/beads"
	"github.com/TaylorFinklea/harness-deck/internal/config"
)

type fakeBeadsSource struct{ snap beads.Snapshot }

func (f fakeBeadsSource) Snapshot() beads.Snapshot { return f.snap }

type fakeDetailer struct {
	issue beads.Issue
	err   error
}

func (f fakeDetailer) Show(context.Context, string, string) (beads.Issue, error) {
	return f.issue, f.err
}
func (f fakeDetailer) DepList(context.Context, string, string) (string, error) {
	return "blockers", nil
}
func (f fakeDetailer) DepTree(context.Context, string, string, string) (string, error) {
	return "flowchart TD", nil
}
func (f fakeDetailer) Comments(context.Context, string, string) (string, error) {
	return "comments", nil
}

func TestHandleBeadsJSON(t *testing.T) {
	s := &Server{beads: fakeBeadsSource{snap: beads.Snapshot{
		Available: true,
		Repos:     []beads.RepoSnapshot{{Name: "harness-deck", Counts: beads.Counts{Ready: 10}}},
	}}}
	rr := httptest.NewRecorder()
	s.handleBeads(rr, httptest.NewRequest("GET", "/api/beads", nil))
	if rr.Code != 200 {
		t.Fatalf("code %d", rr.Code)
	}
	var got beads.Snapshot
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Available || len(got.Repos) != 1 || got.Repos[0].Name != "harness-deck" {
		t.Fatalf("bad body: %s", rr.Body)
	}
}

func TestHandleBeadsDisabledEmpty(t *testing.T) {
	s := &Server{} // no beads source
	rr := httptest.NewRecorder()
	s.handleBeads(rr, httptest.NewRequest("GET", "/api/beads", nil))
	if rr.Code != 200 {
		t.Fatalf("code %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"available":false`) {
		t.Fatalf("want available:false, got %s", rr.Body)
	}
	if !strings.Contains(rr.Body.String(), `"repos":[]`) {
		t.Fatalf("want repos:[], got %s", rr.Body)
	}
}

func beadsIssueReq(project, id string) *http.Request {
	req := httptest.NewRequest("GET", "/api/beads/"+project+"/"+id, nil)
	req.SetPathValue("project", project)
	req.SetPathValue("id", id)
	return req
}

func TestHandleBeadsIssueBadID(t *testing.T) {
	s := &Server{beads: fakeBeadsSource{}}
	rr := httptest.NewRecorder()
	s.handleBeadsIssue(rr, beadsIssueReq("harness-deck", "-rf"))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for flag-like id, got %d", rr.Code)
	}
}

func TestHandleBeadsIssueDisabled(t *testing.T) {
	s := &Server{} // beadsClient nil
	rr := httptest.NewRecorder()
	s.handleBeadsIssue(rr, beadsIssueReq("harness-deck", "harness-deck-5ph.1"))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 when disabled, got %d", rr.Code)
	}
}

func TestHandleBeadsIssueUnknownProject(t *testing.T) {
	s := &Server{beadsClient: fakeDetailer{}, cfg: config.Config{}} // no scan roots → no repos
	rr := httptest.NewRecorder()
	s.handleBeadsIssue(rr, beadsIssueReq("nope", "harness-deck-5ph.1"))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for unknown project, got %d", rr.Code)
	}
}

func TestHandleBeadsIssueOK(t *testing.T) {
	// A temp scan root with a .beads repo makes discovery deterministic.
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "myrepo", ".beads"), 0o755)
	s := &Server{
		cfg:         config.Config{ScanRoots: []string{root}},
		beadsClient: fakeDetailer{issue: beads.Issue{ID: "myrepo-abc", Title: "hi"}},
	}
	rr := httptest.NewRecorder()
	s.handleBeadsIssue(rr, beadsIssueReq("myrepo", "myrepo-abc"))
	if rr.Code != 200 {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body)
	}
	var got map[string]json.RawMessage
	json.Unmarshal(rr.Body.Bytes(), &got)
	for _, k := range []string{"issue", "blockers", "dependents", "comments"} {
		if _, ok := got[k]; !ok {
			t.Errorf("missing key %q in %s", k, rr.Body)
		}
	}
}
