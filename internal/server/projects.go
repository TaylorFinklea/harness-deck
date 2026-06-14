package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/render"
	"github.com/TaylorFinklea/harness-deck/internal/respond"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// historyCap bounds the per-project run timeline in /api/projects by default;
// the full set is available with ?all=1. Capping keeps a project with a deep
// history from loading every responses.json on every poll.
const historyCap = 50

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
	Reports          []store.Entry `json:"reports"`       // reports of kind "roadmap"
	History          []historyRun  `json:"history"`       // runs, newest first (capped unless ?all=1)
	HistoryTotal     int           `json:"history_total"` // total runs before the cap
}

// docCacheEntry is one rendered project markdown file, memoized by mtime.
type docCacheEntry struct {
	mod  time.Time
	html string
}

// renderDoc returns the rendered HTML of a project markdown file, reusing the
// cached render when the file's mtime is unchanged. ok is false when the file
// is absent or unreadable.
func (s *Server) renderDoc(path string) (string, bool) {
	// Stat under the lock so the mtime stored in the cache always matches the
	// content read below — no TOCTOU window where a concurrent write leaves a
	// pre-write mtime keying post-write content (a permanently-missing slot).
	s.docMu.Lock()
	defer s.docMu.Unlock()
	fi, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	if c, ok := s.docCache[path]; ok && c.mod.Equal(fi.ModTime()) {
		return c.html, true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	html := string(render.Markdown(string(data)))
	s.docCache[path] = docCacheEntry{mod: fi.ModTime(), html: html}
	return html, true
}

// pruneDocCache drops cached renders whose path is not in the active set, so a
// hidden or removed project's docs don't linger in the cache forever.
func (s *Server) pruneDocCache(active map[string]bool) {
	s.docMu.Lock()
	defer s.docMu.Unlock()
	for path := range s.docCache {
		if !active[path] {
			delete(s.docCache, path)
		}
	}
}

// handleProjects renders the projects view: for every enabled project its
// current-state.md and roadmap.md, plus reports of kind "roadmap". It also
// returns the full discovered list so the settings panel can show toggles.
func (s *Server) handleProjects(w http.ResponseWriter, r *http.Request) {
	limit := historyCap
	if r.URL.Query().Get("all") == "1" {
		limit = 0 // 0 = no cap
	}
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
	known := map[string]bool{}      // every discovered name, hidden or not
	activeDocs := map[string]bool{} // doc paths rendered this request (for cache pruning)
	views := []projectView{}
	for _, p := range discovered {
		known[p.Name] = true
		if !p.Enabled {
			continue
		}
		hist, total := buildHistory(allReports[p.Name], limit)
		pv := projectView{
			Project:      p.Name,
			Reports:      roadmapReports[p.Name],
			History:      hist,
			HistoryTotal: total,
		}
		aiDir := filepath.Join(p.Path, ".docs", "ai")
		csPath := filepath.Join(aiDir, "current-state.md")
		rmPath := filepath.Join(aiDir, "roadmap.md")
		activeDocs[csPath] = true
		activeDocs[rmPath] = true
		if html, ok := s.renderDoc(csPath); ok {
			pv.HasState = true
			pv.CurrentStateHTML = html
		}
		if html, ok := s.renderDoc(rmPath); ok {
			pv.HasRoadmap = true
			pv.RoadmapHTML = html
		}
		views = append(views, pv)
	}
	s.pruneDocCache(activeDocs)
	// Projects seen only through reports (e.g. a central-dir report whose
	// project has no discovered root) still get an entry — but a discovered
	// project the user hid stays hidden. The history needs the full report
	// set (not just roadmap-kind), so we key the orphan loop on allReports.
	for name, reports := range allReports {
		if known[name] {
			continue
		}
		hist, total := buildHistory(reports, limit)
		views = append(views, projectView{
			Project:      name,
			Reports:      roadmapReports[name],
			History:      hist,
			HistoryTotal: total,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"projects":   views,
		"discovered": discovered,
	})
}

// buildHistory turns a project's entry list into the retrospective timeline:
// runs newest-first, with each run's responses.json inlined. It sorts first,
// then applies limit (0 = no cap), then loads responses only for the kept runs
// — so a deep history doesn't read every responses.json on every poll. It
// returns the kept runs plus the total before capping (for "N of M" display).
func buildHistory(entries []store.Entry, limit int) (runs []historyRun, total int) {
	sorted := append([]store.Entry(nil), entries...)
	// store.Entries() is already sorted by Created desc, but the per-project
	// slices were built in that traversal order; re-sort to be defensive.
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Created > sorted[j].Created })
	total = len(sorted)
	if limit > 0 && len(sorted) > limit {
		sorted = sorted[:limit]
	}
	runs = make([]historyRun, 0, len(sorted))
	for _, e := range sorted {
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
	return runs, total
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
