package server

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/TaylorFinklea/harness-deck/internal/beads"
)

// beadsSource is the read side handleBeads needs; *beads.Monitor satisfies it.
// It is an interface so tests inject a fake snapshot without a live monitor.
type beadsSource interface {
	Snapshot() beads.Snapshot
}

// beadsDetailer is the on-demand drill-in surface; *beads.Client satisfies it.
type beadsDetailer interface {
	Show(ctx context.Context, root, id string) (beads.Issue, error)
	DepList(ctx context.Context, root, id string) (string, error)
	DepTree(ctx context.Context, root, id, dir string) (string, error)
	Comments(ctx context.Context, root, id string) (string, error)
}

// handleBeads serves the cached beads snapshot across all discovered repos.
// Nil-safe: with the feature off it returns {repos:[],available:false}.
func (s *Server) handleBeads(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var snap beads.Snapshot
	if s.beads != nil {
		snap = s.beads.Snapshot()
	}
	if snap.Repos == nil {
		snap.Repos = []beads.RepoSnapshot{}
	}
	_ = json.NewEncoder(w).Encode(snap)
}

// handleBeadsIssue serves one issue's drill-in detail, shelled on demand.
// 400 bad id · 503 disabled · 404 unknown project/issue.
func (s *Server) handleBeadsIssue(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	id := r.PathValue("id")
	if !beads.ValidID(id) {
		http.Error(w, "bad issue id", http.StatusBadRequest)
		return
	}
	if s.beadsClient == nil {
		http.Error(w, "beads disabled", http.StatusServiceUnavailable)
		return
	}
	root, ok := s.beadsRepoRoot(project)
	if !ok {
		http.Error(w, "unknown project", http.StatusNotFound)
		return
	}
	ctx := r.Context()
	issue, err := s.beadsClient.Show(ctx, root, id)
	if err != nil {
		http.Error(w, "issue not found", http.StatusNotFound)
		return
	}
	// Blockers/dependents/comments are best-effort: a failure on any one leaves
	// its field empty rather than sinking the whole detail response.
	blockers, _ := s.beadsClient.DepList(ctx, root, id)
	dependents, _ := s.beadsClient.DepTree(ctx, root, id, "up")
	comments, _ := s.beadsClient.Comments(ctx, root, id)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"issue":      issue,
		"blockers":   blockers,
		"dependents": dependents,
		"comments":   comments,
	})
}

// beadsRepoRoot maps a discovered repo's display name to its root. Discovery is
// a cheap dir-stat, done fresh so a newly-added repo resolves without restart.
func (s *Server) beadsRepoRoot(project string) (string, bool) {
	for _, r := range beads.Discover(s.cfg.ScanRoots, s.cfg.Projects) {
		if r.Name == project {
			return r.Root, true
		}
	}
	return "", false
}
