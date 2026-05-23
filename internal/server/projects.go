package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/TaylorFinklea/harness-deck/internal/render"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// projectView is one tracked project as the dashboard shows it: its rendered
// .docs/ai docs plus any reports it published with kind "roadmap".
type projectView struct {
	Project          string        `json:"project"`
	CurrentStateHTML string        `json:"current_state_html"` // rendered current-state.md
	HasState         bool          `json:"has_state"`
	RoadmapHTML      string        `json:"roadmap_html"` // rendered roadmap.md
	HasRoadmap       bool          `json:"has_roadmap"`
	Reports          []store.Entry `json:"reports"` // reports of kind "roadmap"
}

// handleProjects renders the projects view: for every enabled project its
// current-state.md and roadmap.md, plus reports of kind "roadmap". It also
// returns the full discovered list so the settings panel can show toggles.
func (s *Server) handleProjects(w http.ResponseWriter, _ *http.Request) {
	// Group roadmap-kind reports by project name.
	roadmapReports := map[string][]store.Entry{}
	for _, e := range s.store.Entries() {
		if e.Kind == "roadmap" {
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
		pv := projectView{Project: p.Name, Reports: roadmapReports[p.Name]}
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
	// Projects seen only through roadmap-kind reports (e.g. a central-dir
	// report whose project has no discovered root) still get an entry — but
	// a discovered project the user hid stays hidden.
	for name, reports := range roadmapReports {
		if !known[name] {
			views = append(views, projectView{Project: name, Reports: reports})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"projects":   views,
		"discovered": discovered,
	})
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
