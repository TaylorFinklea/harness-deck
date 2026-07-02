package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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

func TestHandleBeadsIssueShowErrors(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "myrepo", ".beads"), 0o755)
	cfg := config.Config{ScanRoots: []string{root}}
	cases := []struct {
		name    string
		showErr error
		want    int
	}{
		{"missing issue -> 404", os.ErrNotExist, http.StatusNotFound},
		{"transient bd failure -> 502", errors.New("bd: exit status 1"), http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{cfg: cfg, beadsClient: fakeDetailer{err: tc.showErr}}
			rr := httptest.NewRecorder()
			s.handleBeadsIssue(rr, beadsIssueReq("myrepo", "myrepo-abc"))
			if rr.Code != tc.want {
				t.Fatalf("want %d, got %d", tc.want, rr.Code)
			}
		})
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

// fakeBeadsRW satisfies both beadsDetailer (Show for the re-check) and beadsMutator.
type fakeBeadsRW struct {
	issue     beads.Issue
	showErr   error
	claimErr  error
	closeErr  error
	createID  string
	createErr error
	closedIDs []string
	claimed   int
}

func (f *fakeBeadsRW) Show(context.Context, string, string) (beads.Issue, error) {
	return f.issue, f.showErr
}
func (f *fakeBeadsRW) DepList(context.Context, string, string) (string, error) { return "", nil }
func (f *fakeBeadsRW) DepTree(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeBeadsRW) Comments(context.Context, string, string) (string, error) { return "", nil }
func (f *fakeBeadsRW) Claim(context.Context, string, string) error {
	f.claimed++
	return f.claimErr
}
func (f *fakeBeadsRW) Close(_ context.Context, _, id, _ string) error {
	f.closedIDs = append(f.closedIDs, id)
	return f.closeErr
}
func (f *fakeBeadsRW) Create(context.Context, string, string, string, string, string) (string, error) {
	return f.createID, f.createErr
}

func newRWServer(t *testing.T, f *fakeBeadsRW, writable bool) *Server {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "myrepo", ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Server{
		cfg:         config.Config{ScanRoots: []string{root}, Beads: config.BeadsConfig{Enabled: true, Writable: writable}},
		beadsClient: f, beadsMutator: f, hub: newHub(),
	}
}

func TestClaimHappyPath(t *testing.T) {
	f := &fakeBeadsRW{issue: beads.Issue{ID: "myrepo-a", Status: "open"}}
	s := newRWServer(t, f, true)
	rr := httptest.NewRecorder()
	s.handleBeadsClaim(rr, beadsIssueReq("myrepo", "myrepo-a"))
	if rr.Code != 200 {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body)
	}
	if f.claimed != 1 {
		t.Errorf("Claim not called")
	}
}

func TestClaimForbiddenWhenNotWritable(t *testing.T) {
	f := &fakeBeadsRW{issue: beads.Issue{ID: "myrepo-a", Status: "open"}}
	s := newRWServer(t, f, false)
	rr := httptest.NewRecorder()
	s.handleBeadsClaim(rr, beadsIssueReq("myrepo", "myrepo-a"))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rr.Code)
	}
	if f.claimed != 0 {
		t.Errorf("must not mutate when not writable")
	}
}

func TestClaimDisabled503(t *testing.T) {
	s := &Server{cfg: config.Config{Beads: config.BeadsConfig{Enabled: false}}, hub: newHub()}
	rr := httptest.NewRecorder()
	s.handleBeadsClaim(rr, beadsIssueReq("myrepo", "myrepo-a"))
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestCloseAlreadyClosed409(t *testing.T) {
	f := &fakeBeadsRW{issue: beads.Issue{ID: "myrepo-a", Status: "closed"}}
	s := newRWServer(t, f, true)
	rr := httptest.NewRecorder()
	req := beadsIssueReq("myrepo", "myrepo-a")
	req.Body = io.NopCloser(strings.NewReader(`{"reason":"done"}`))
	s.handleBeadsClose(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", rr.Code)
	}
	if len(f.closedIDs) != 0 {
		t.Errorf("must not Close an already-closed issue")
	}
}

func TestCloseHappyPath(t *testing.T) {
	f := &fakeBeadsRW{issue: beads.Issue{ID: "myrepo-a", Status: "open"}}
	s := newRWServer(t, f, true)
	rr := httptest.NewRecorder()
	req := beadsIssueReq("myrepo", "myrepo-a")
	req.Body = io.NopCloser(strings.NewReader(`{"reason":"shipped"}`))
	s.handleBeadsClose(rr, req)
	if rr.Code != 200 {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body)
	}
	if len(f.closedIDs) != 1 {
		t.Errorf("Close not called")
	}
}

func createReq(project, body string) *http.Request {
	r := httptest.NewRequest("POST", "/api/beads/"+project+"/create", strings.NewReader(body))
	r.SetPathValue("project", project)
	return r
}

func TestCreateValidatesInput(t *testing.T) {
	f := &fakeBeadsRW{createID: "myrepo-new"}
	s := newRWServer(t, f, true)
	// bad type
	rr := httptest.NewRecorder()
	s.handleBeadsCreate(rr, createReq("myrepo", `{"title":"x","type":"nope","priority":"2"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad type want 400, got %d", rr.Code)
	}
	// bad priority
	rr = httptest.NewRecorder()
	s.handleBeadsCreate(rr, createReq("myrepo", `{"title":"x","type":"task","priority":"9"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad priority want 400, got %d", rr.Code)
	}
	// empty title
	rr = httptest.NewRecorder()
	s.handleBeadsCreate(rr, createReq("myrepo", `{"title":"  ","type":"task","priority":"2"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty title want 400, got %d", rr.Code)
	}
	// happy path returns id
	rr = httptest.NewRecorder()
	s.handleBeadsCreate(rr, createReq("myrepo", `{"title":"hi","type":"task","priority":"2"}`))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "myrepo-new") {
		t.Fatalf("want 200 + id, got %d %s", rr.Code, rr.Body)
	}
}

func TestCreateForbiddenWhenNotWritable(t *testing.T) {
	s := newRWServer(t, &fakeBeadsRW{}, false)
	rr := httptest.NewRecorder()
	s.handleBeadsCreate(rr, createReq("myrepo", `{"title":"hi","type":"task","priority":"2"}`))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rr.Code)
	}
}
