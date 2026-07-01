package beads

import (
	"context"
	"testing"
)

type fakeFetcher struct {
	ready, blocked, all []Issue
	counts              Counts
	err                 error // fails ready/blocked/list
	statusErr           error // fails only status
}

func (f *fakeFetcher) Ready(context.Context, string) ([]Issue, error)   { return f.ready, f.err }
func (f *fakeFetcher) Blocked(context.Context, string) ([]Issue, error) { return f.blocked, f.err }
func (f *fakeFetcher) List(context.Context, string) ([]Issue, error)    { return f.all, f.err }
func (f *fakeFetcher) Status(context.Context, string) (Counts, error)   { return f.counts, f.statusErr }

func TestMonitorRefreshCachesAndFiresOnce(t *testing.T) {
	f := &fakeFetcher{ready: []Issue{{ID: "a", Status: "open"}}, counts: Counts{Ready: 1, Open: 1}}
	repos := func() []Repo { return []Repo{{Name: "r", Root: "/r"}} }
	fires := 0
	m := NewMonitor(f, repos, 0, func() { fires++ })

	m.refreshOnce(context.Background())
	snap := m.Snapshot()
	if !snap.Available || len(snap.Repos) != 1 || len(snap.Repos[0].Ready) != 1 {
		t.Fatalf("bad snapshot: %+v", snap)
	}
	if snap.Repos[0].Counts.Ready != 1 {
		t.Errorf("want counts.ready 1, got %+v", snap.Repos[0].Counts)
	}
	if fires != 1 {
		t.Fatalf("want 1 onChange, got %d", fires)
	}

	m.refreshOnce(context.Background()) // identical → no new fire
	if fires != 1 {
		t.Fatalf("identical refresh must not fire, got %d", fires)
	}

	f.ready = []Issue{{ID: "a", Status: "closed"}} // real change
	m.refreshOnce(context.Background())
	if fires != 2 {
		t.Fatalf("changed refresh must fire, got %d", fires)
	}

	// A priority-only change (bd may not bump updated_at) must still fire.
	f.ready = []Issue{{ID: "a", Status: "closed", Priority: 2}}
	m.refreshOnce(context.Background())
	if fires != 3 {
		t.Fatalf("priority change must fire onChange, got %d", fires)
	}
}

func TestMonitorRepoErrorIsolated(t *testing.T) {
	f := &fakeFetcher{err: context.DeadlineExceeded}
	m := NewMonitor(f, func() []Repo { return []Repo{{Name: "r", Root: "/r"}} }, 0, nil)
	m.refreshOnce(context.Background())
	snap := m.Snapshot()
	if len(snap.Repos) != 1 || snap.Repos[0].Err == "" {
		t.Fatalf("want per-repo Err, got %+v", snap)
	}
}

func TestMonitorStatusFailureKeepsIssues(t *testing.T) {
	f := &fakeFetcher{
		ready:     []Issue{{ID: "a", Status: "open"}, {ID: "b", Status: "open"}},
		blocked:   []Issue{{ID: "c", Status: "open"}},
		all:       []Issue{{ID: "a"}, {ID: "b"}, {ID: "c"}},
		statusErr: context.DeadlineExceeded, // only Status fails
	}
	m := NewMonitor(f, func() []Repo { return []Repo{{Name: "r", Root: "/r"}} }, 0, nil)
	m.refreshOnce(context.Background())
	repo := m.Snapshot().Repos[0]
	if repo.Err != "" {
		t.Fatalf("a status-only failure must NOT mark the repo errored: %q", repo.Err)
	}
	if len(repo.Ready) != 2 || len(repo.Blocked) != 1 {
		t.Fatalf("issues must survive a status failure: %+v", repo)
	}
	if repo.Counts.Ready != 2 || repo.Counts.Blocked != 1 || repo.Counts.Open != 3 {
		t.Errorf("counts should fall back to list lengths, got %+v", repo.Counts)
	}
}

func TestMonitorNilSnapshotSafe(t *testing.T) {
	var m *Monitor
	if snap := m.Snapshot(); snap.Available || snap.Repos != nil {
		t.Errorf("nil monitor should give empty snapshot, got %+v", snap)
	}
}

func TestMonitorEdgesDerived(t *testing.T) {
	f := &fakeFetcher{
		all:     []Issue{{ID: "child", Parent: "parent"}},
		blocked: []Issue{{ID: "b", BlockedBy: []string{"a"}}},
	}
	m := NewMonitor(f, func() []Repo { return []Repo{{Name: "r", Root: "/r"}} }, 0, nil)
	m.refreshOnce(context.Background())
	edges := m.Snapshot().Repos[0].Edges
	if len(edges) != 2 {
		t.Fatalf("want 2 edges (blocks + parent), got %d: %+v", len(edges), edges)
	}
}
