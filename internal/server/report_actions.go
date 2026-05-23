package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// setReportStatus rewrites report.json with a new top-level status, preserving
// every other field via a map round-trip. The write is atomic (temp + rename)
// so a crash mid-write cannot truncate the agent's report.
func setReportStatus(dir, newStatus string) error {
	path := filepath.Join(dir, "report.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	doc["status"] = newStatus
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// handleReportClose marks a report as done. Idempotent — closing an
// already-done report is a no-op success.
func (s *Server) handleReportClose(w http.ResponseWriter, r *http.Request) {
	s.mutateReportStatus(w, r, "done")
}

// handleReportReopen flips a closed report back to awaiting-review so it
// returns to the inbox. Idempotent.
func (s *Server) handleReportReopen(w http.ResponseWriter, r *http.Request) {
	s.mutateReportStatus(w, r, "awaiting-review")
}

// mutateReportStatus is the shared close/reopen body.
func (s *Server) mutateReportStatus(w http.ResponseWriter, r *http.Request, newStatus string) {
	_, entry, err := s.store.Get(r.PathValue("project"), r.PathValue("run"))
	if err != nil {
		http.Error(w, "report not found: "+err.Error(), http.StatusNotFound)
		return
	}
	if err := setReportStatus(entry.Dir, newStatus); err != nil {
		http.Error(w, "could not update status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.store.Scan(s.enabledRoots())
	s.hub.broadcast("reports")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleReportDelete removes a report's run directory — report.json,
// responses.json, and any artifacts the agent left alongside.
func (s *Server) handleReportDelete(w http.ResponseWriter, r *http.Request) {
	_, entry, err := s.store.Get(r.PathValue("project"), r.PathValue("run"))
	if err != nil {
		http.Error(w, "report not found: "+err.Error(), http.StatusNotFound)
		return
	}
	if err := os.RemoveAll(entry.Dir); err != nil {
		http.Error(w, "could not delete: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.store.Scan(s.enabledRoots())
	s.hub.broadcast("reports")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
