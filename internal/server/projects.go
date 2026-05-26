package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/TaylorFinklea/harness-deck/internal/render"
	"github.com/TaylorFinklea/harness-deck/internal/respond"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// historyRun is one run as the project-history timeline shows it: the entry
// fields the inbox already exposes, plus the responses recorded against it
// so the retrospective view doesn't need a second round-trip per run.
type historyRun struct {
	Run       string             `json:"run"`
	Title     string             `json:"title"`
	Kind      string             `json:"kind,omitempty"`
	Status    string             `json:"status"`
	Created   string             `json:"created"`
	OpenAsks  int                `json:"open_asks"`
	Archived  bool               `json:"archived,omitempty"`
	Harness   string             `json:"harness,omitempty"`
	Responses []respond.Response `json:"responses"`
}

// projectView is one tracked project as the dashboard shows it: its rendered
// .docs/ai docs plus any reports it published with kind "roadmap" (the plan)
// and the full timeline of every run for the project (the history).
type projectView struct {
	Project          string        `json:"project"`
	CurrentStateHTML string        `json:"current_state_html"` // rendered current-state.md
	HasState         bool          `json:"has_state"`
	RoadmapHTML      string        `json:"roadmap_html"` // rendered roadmap.md
	HasRoadmap       bool          `json:"has_roadmap"`
	Reports          []store.Entry `json:"reports"` // reports of kind "roadmap"
	History          []historyRun  `json:"history"` // every run, newest first
}

// handleProjects renders the projects view: for every enabled project its
// current-state.md and roadmap.md, plus reports of kind "roadmap". It also
// returns the full discovered list so the settings panel can show toggles.
func (s *Server) handleProjects(w http.ResponseWriter, _ *http.Request) {
	// Group reports by project. Two slices per project:
	//   - roadmapReports: kind="roadmap", non-archived — the "plan" panel.
	//   - allReports: every entry (any kind, including archived) — the
	//     retrospective history timeline.
	// Both walk the same store.Entries() snapshot so they stay consistent.
	roadmapReports := map[string][]store.Entry{}
	allReports := map[string][]store.Entry{}
	for _, e := range s.store.Entries() {
		allReports[e.Project] = append(allReports[e.Project], e)
		if e.Kind == "roadmap" && !e.Archived {
			roadmapReports[e.Project] = append(roadmapReports[e.Project], e)
		}
	}

	discovered := s.projects.Discovered()
	known := map[string]bool{} // every discovered name, hidden or not
	views := []projectView{}
	for _, p := range discovered {
		known[p.Name] = true
		if !p.Enabled {
			continue
		}
		pv := projectView{
			Project: p.Name,
			Reports: roadmapReports[p.Name],
			History: buildHistory(allReports[p.Name]),
		}
		aiDir := filepath.Join(p.Path, ".docs", "ai")
		if data, err := os.ReadFile(filepath.Join(aiDir, "current-state.md")); err == nil {
			pv.HasState = true
			pv.CurrentStateHTML = string(render.Markdown(string(data)))
		}
		if data, err := os.ReadFile(filepath.Join(aiDir, "roadmap.md")); err == nil {
			pv.HasRoadmap = true
			pv.RoadmapHTML = string(render.Markdown(string(data)))
		}
		views = append(views, pv)
	}
	// Projects seen only through reports (e.g. a central-dir report whose
	// project has no discovered root) still get an entry — but a discovered
	// project the user hid stays hidden. The history needs the full report
	// set (not just roadmap-kind), so we key the orphan loop on allReports.
	for name, reports := range allReports {
		if known[name] {
			continue
		}
		views = append(views, projectView{
			Project: name,
			Reports: roadmapReports[name],
			History: buildHistory(reports),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"projects":   views,
		"discovered": discovered,
	})
}

// buildHistory turns a project's entry list into the retrospective timeline:
// every run newest-first, with its responses.json inlined. Each entry's
// responses load is a single file read — the watcher already touches every
// responses.json into the store fingerprint, so the kernel page cache makes
// this nearly free in practice.
func buildHistory(entries []store.Entry) []historyRun {
	runs := make([]historyRun, 0, len(entries))
	for _, e := range entries {
		var resps []respond.Response
		if file, err := respond.Load(e.Dir); err == nil {
			resps = make([]respond.Response, 0, len(file.Responses))
			for _, r := range file.Responses {
				resps = append(resps, r)
			}
			// Sort responses newest first so the most recent answer is
			// the first one the user sees under a run.
			sort.SliceStable(resps, func(i, j int) bool { return resps[i].At > resps[j].At })
		}
		runs = append(runs, historyRun{
			Run:       e.Run,
			Title:     e.Title,
			Kind:      e.Kind,
			Status:    e.Status,
			Created:   e.Created,
			OpenAsks:  e.OpenAsks,
			Archived:  e.Archived,
			Harness:   e.Harness,
			Responses: resps,
		})
	}
	// store.Entries() is already sorted by Created desc, but the per-project
	// slices were built in that traversal order; re-sort to be defensive.
	sort.SliceStable(runs, func(i, j int) bool { return runs[i].Created > runs[j].Created })
	return runs
}

// projectToggleRequest is the POST body for hiding or showing a project.
type projectToggleRequest struct {
	Name string `json:"name"`
}

// projectReorderRequest is the POST body for setting the display order.
type projectReorderRequest struct {
	Order []string `json:"order"`
}

// handleProjectReorder records the user's preferred project order and
// refreshes the dashboard so every project listing follows it.
func (s *Server) handleProjectReorder(w http.ResponseWriter, r *http.Request) {
	var req projectReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.projects.Reorder(req.Order); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.store.Scan(s.enabledRoots())
	s.hub.broadcast("reports")

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleProjectToggle flips a project's visibility, re-indexes reports for the
// new enabled set, and refreshes the dashboard.
func (s *Server) handleProjectToggle(w http.ResponseWriter, r *http.Request) {
	var req projectToggleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "missing project name", http.StatusBadRequest)
		return
	}
	if err := s.projects.Toggle(req.Name); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.store.Scan(s.enabledRoots())
	s.hub.broadcast("reports")

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
