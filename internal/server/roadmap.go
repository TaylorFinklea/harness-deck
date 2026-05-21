package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/render"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// roadmapProject is one project's roadmap: its rendered .docs/ai/roadmap.md
// plus any reports it published with kind "roadmap".
type roadmapProject struct {
	Project string        `json:"project"`
	HTML    string        `json:"html"`    // rendered roadmap.md ("" if none)
	HasFile bool          `json:"has_file"`
	Reports []store.Entry `json:"reports"` // reports of kind "roadmap"
}

// handleRoadmap renders each registered project's .docs/ai/roadmap.md and
// gathers reports of kind "roadmap" — the two roadmap sources the user chose.
func (s *Server) handleRoadmap(w http.ResponseWriter, _ *http.Request) {
	// Group roadmap-kind reports by project name.
	roadmapReports := map[string][]store.Entry{}
	for _, e := range s.store.Entries() {
		if e.Kind == "roadmap" {
			roadmapReports[e.Project] = append(roadmapReports[e.Project], e)
		}
	}

	out := []roadmapProject{}
	covered := map[string]bool{}
	for _, projPath := range s.cfg.Projects {
		root := config.Expand(projPath)
		name := filepath.Base(root)
		covered[name] = true
		rp := roadmapProject{Project: name, Reports: roadmapReports[name]}
		if data, err := os.ReadFile(filepath.Join(root, ".docs", "ai", "roadmap.md")); err == nil {
			rp.HasFile = true
			rp.HTML = string(render.Markdown(string(data)))
		}
		out = append(out, rp)
	}
	// Projects that only appear via roadmap-kind reports (e.g. central-dir
	// reports with no registered root) still get a roadmap entry.
	for name, reports := range roadmapReports {
		if !covered[name] {
			out = append(out, roadmapProject{Project: name, Reports: reports})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"projects": out})
}
