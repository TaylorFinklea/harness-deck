package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/jsonfile"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// patchReport rewrites report.json after applying changes to its
// top-level fields (nil deletes the key), preserving every other field —
// including exact number literals — via jsonfile.Patch's atomic
// read-modify-write.
func patchReport(dir string, changes map[string]any) error {
	return jsonfile.Patch(filepath.Join(dir, "report.json"), func(doc map[string]any) error {
		for k, v := range changes {
			if v == nil {
				delete(doc, k)
			} else {
				doc[k] = v
			}
		}
		return nil
	})
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
// responses.json, and any artifacts the agent left alongside. The guard
// matters: discovery indexes a report.json at ANY depth, so a manifest
// misplaced at a shared root would make entry.Dir the whole central dir
// (or a repo's entire .harness/) and RemoveAll would take every other
// report with it.
func (s *Server) handleReportDelete(w http.ResponseWriter, r *http.Request) {
	_, entry, err := s.store.Get(r.PathValue("project"), r.PathValue("run"))
	if err != nil {
		http.Error(w, "report not found: "+err.Error(), http.StatusNotFound)
		return
	}
	if reason := s.unsafeToDelete(entry); reason != "" {
		http.Error(w, "refusing to delete: "+reason, http.StatusConflict)
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

// unsafeToDelete reports why entry.Dir must not be RemoveAll'd, or "" when
// the delete is safe. Two refusal conditions: the dir IS a shared scan
// root (central dir or a project's .harness root), or another indexed
// report lives anywhere beneath it.
func (s *Server) unsafeToDelete(entry store.Entry) string {
	dir := filepath.Clean(entry.Dir)
	if dir == filepath.Clean(config.Expand(s.cfg.CentralDir)) {
		return "this report.json sits at the central reports root, not in its own run directory"
	}
	for _, root := range s.enabledRoots() {
		if dir == filepath.Clean(filepath.Join(config.Expand(root), ".harness")) {
			return "this report.json sits at a project's .harness root, not in its own run directory"
		}
	}
	var other string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || other != "" {
			return nil
		}
		if d.Name() == "report.json" && path != entry.Path {
			other = path
		}
		return nil
	})
	if other != "" {
		return "another report (" + other + ") lives inside this directory"
	}
	return ""
}
