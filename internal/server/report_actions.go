package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
)

// patchReport rewrites report.json after applying changes to its
// top-level fields, preserving every other field via a map round-trip.
// The write is atomic (temp + rename) so a crash mid-write cannot
// truncate the agent's report.
func patchReport(dir string, changes map[string]any) error {
	path := filepath.Join(dir, "report.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return err
	}
	for k, v := range changes {
		if v == nil {
			delete(doc, k)
		} else {
			doc[k] = v
		}
	}
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
	if err := patchReport(entry.Dir, map[string]any{"status": newStatus}); err != nil {
		http.Error(w, "could not update status: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.store.Scan(s.enabledRoots())
	s.hub.broadcast("reports")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleReportArchive flips the archived flag so the report drops out of
// every default view. Files stay on disk; the archive view lists them
// for restore or permanent delete.
func (s *Server) handleReportArchive(w http.ResponseWriter, r *http.Request) {
	s.mutateArchived(w, r, true)
}

// handleReportUnarchive restores an archived report to its prior state.
func (s *Server) handleReportUnarchive(w http.ResponseWriter, r *http.Request) {
	s.mutateArchived(w, r, false)
}

func (s *Server) mutateArchived(w http.ResponseWriter, r *http.Request, archived bool) {
	_, entry, err := s.store.Get(r.PathValue("project"), r.PathValue("run"))
	if err != nil {
		http.Error(w, "report not found: "+err.Error(), http.StatusNotFound)
		return
	}
	// Omit the field entirely when clearing — keeps report.json clean rather
	// than carrying an "archived: false" tombstone in unarchived files.
	var v any
	if archived {
		v = true
	} else {
		v = nil
	}
	if err := patchReport(entry.Dir, map[string]any{"archived": v}); err != nil {
		http.Error(w, "could not update archived flag: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.store.Scan(s.enabledRoots())
	s.hub.broadcast("reports")
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleReportSig returns the current fingerprint for a report. The
// report page polls this on every SSE change event to decide whether to
// reload. Returning {exists: false} signals that the report was deleted
// or archived to a hidden state — the page redirects to / in that case.
func (s *Server) handleReportSig(w http.ResponseWriter, r *http.Request) {
	_, entry, err := s.store.Get(r.PathValue("project"), r.PathValue("run"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		_, _ = w.Write([]byte(`{"exists":false}`))
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"exists":   true,
		"sig":      entry.Sig(),
		"archived": entry.Archived,
		"status":   entry.Status,
	})
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
