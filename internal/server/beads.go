package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

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
		http.Error(w, "show: "+err.Error(), beadsShowStatus(err))
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

// beadsShowStatus maps a Show error to an HTTP status: a genuinely-missing issue
// is 404; a transient bd failure (exec error / timeout / bad JSON) is 502, so a
// wedged bd isn't silently reported as "not found".
func beadsShowStatus(err error) int {
	if errors.Is(err, os.ErrNotExist) {
		return http.StatusNotFound
	}
	return http.StatusBadGateway
}

// --- mutations (Phase 2). Gated by beads.enabled + beads.writable. ---

// beadsMutator is the write surface; *beads.Client satisfies it. An interface so
// handler tests inject a fake. The pre-write status re-check reuses beadsClient.Show.
type beadsMutator interface {
	Claim(ctx context.Context, root, id string) error
	Close(ctx context.Context, root, id, reason string) error
	Create(ctx context.Context, root, title, itype, priority, description string) (string, error)
}

// beadsWriteGate enforces the mutation preconditions (feature enabled, writable)
// and resolves the repo root. It writes the error response and returns ok=false.
func (s *Server) beadsWriteGate(w http.ResponseWriter, project string) (string, bool) {
	if s.beadsMutator == nil {
		http.Error(w, "beads disabled", http.StatusServiceUnavailable)
		return "", false
	}
	if !s.cfg.Beads.Writable {
		http.Error(w, "beads is read-only (set beads.writable)", http.StatusForbidden)
		return "", false
	}
	root, ok := s.beadsRepoRoot(project)
	if !ok {
		http.Error(w, "unknown project", http.StatusNotFound)
		return "", false
	}
	return root, true
}

// beadsRefreshAndBroadcast forces an immediate snapshot refresh so the mutation
// shows at once, then broadcasts the SSE event other tabs listen for.
func (s *Server) beadsRefreshAndBroadcast(ctx context.Context) {
	if s.beadsMonitor != nil {
		s.beadsMonitor.RefreshNow(ctx)
	}
	s.hub.broadcastEvent("beads", "changed")
}

// handleBeadsClaim claims an issue (in_progress + assigned). 409 if already closed.
func (s *Server) handleBeadsClaim(w http.ResponseWriter, r *http.Request) {
	project, id := r.PathValue("project"), r.PathValue("id")
	if !beads.ValidID(id) {
		http.Error(w, "bad issue id", http.StatusBadRequest)
		return
	}
	root, ok := s.beadsWriteGate(w, project)
	if !ok {
		return
	}
	ctx := r.Context()
	issue, err := s.beadsClient.Show(ctx, root, id)
	if err != nil {
		http.Error(w, "show: "+err.Error(), beadsShowStatus(err))
		return
	}
	if issue.Status == "closed" {
		http.Error(w, "issue is closed", http.StatusConflict)
		return
	}
	if err := s.beadsMutator.Claim(ctx, root, id); err != nil {
		http.Error(w, "bd: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.beadsRefreshAndBroadcast(ctx)
	writeBeadsOK(w, nil)
}

// handleBeadsClose closes an issue with an optional reason. 409 if already closed.
func (s *Server) handleBeadsClose(w http.ResponseWriter, r *http.Request) {
	project, id := r.PathValue("project"), r.PathValue("id")
	if !beads.ValidID(id) {
		http.Error(w, "bad issue id", http.StatusBadRequest)
		return
	}
	root, ok := s.beadsWriteGate(w, project)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	// An empty/absent body is fine — close without a reason.
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	ctx := r.Context()
	issue, err := s.beadsClient.Show(ctx, root, id)
	if err != nil {
		http.Error(w, "show: "+err.Error(), beadsShowStatus(err))
		return
	}
	if issue.Status == "closed" {
		http.Error(w, "issue is already closed", http.StatusConflict)
		return
	}
	if err := s.beadsMutator.Close(ctx, root, id, req.Reason); err != nil {
		http.Error(w, "bd: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.beadsRefreshAndBroadcast(ctx)
	writeBeadsOK(w, nil)
}

// handleBeadsCreate creates a new issue in the repo and returns its id.
func (s *Server) handleBeadsCreate(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	root, ok := s.beadsWriteGate(w, project)
	if !ok {
		return
	}
	var req struct {
		Title       string `json:"title"`
		Type        string `json:"type"`
		Priority    string `json:"priority"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request body", http.StatusBadRequest)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if !beads.ValidTitle(req.Title) {
		http.Error(w, "title required (1-500 chars, no control characters)", http.StatusBadRequest)
		return
	}
	if !beads.ValidType(req.Type) {
		http.Error(w, "bad type (bug|feature|task|epic|chore)", http.StatusBadRequest)
		return
	}
	if !beads.ValidPriority(req.Priority) {
		http.Error(w, "bad priority (0-4)", http.StatusBadRequest)
		return
	}
	id, err := s.beadsMutator.Create(r.Context(), root, req.Title, req.Type, req.Priority, req.Description)
	if err != nil {
		http.Error(w, "bd: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.beadsRefreshAndBroadcast(r.Context())
	writeBeadsOK(w, map[string]string{"id": id})
}

// writeBeadsOK writes {"ok":true} plus any extra fields.
func writeBeadsOK(w http.ResponseWriter, extra map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	out := map[string]any{"ok": true}
	for k, v := range extra {
		out[k] = v
	}
	_ = json.NewEncoder(w).Encode(out)
}
