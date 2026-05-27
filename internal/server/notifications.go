package server

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/notify"
)

// notificationsResponse is the GET /api/notifications payload. We expose
// destinations with the URL host-only (not the full URL with token/path
// secrets) — the settings UI shows the host as identification, never
// echoes the secret back. The user can edit the URL via the add flow
// but the existing value never round-trips through the wire.
type notificationsResponse struct {
	PublicURL    string                  `json:"public_url"`
	Destinations []destinationListEntry  `json:"destinations"`
}

// destinationListEntry redacts the URL down to "<scheme>://<host>" so
// secrets in the path (Slack/Discord webhook tokens) don't leak via the
// settings GET. The full URL stays in config.json on disk.
type destinationListEntry struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	URLHost  string   `json:"url_host"`
	Projects []string `json:"projects,omitempty"`
}

// handleNotificationsList returns the configured destinations. The full
// webhook URLs are intentionally not in the response — see
// destinationListEntry.
func (s *Server) handleNotificationsList(w http.ResponseWriter, _ *http.Request) {
	s.notifMu.RLock()
	dests := append([]notify.Destination(nil), s.cfg.Notifications...)
	publicURL := s.cfg.PublicURL
	s.notifMu.RUnlock()

	list := make([]destinationListEntry, 0, len(dests))
	for _, d := range dests {
		list = append(list, destinationListEntry{
			Name:     d.Name,
			Type:     d.Type,
			URLHost:  redactURL(d.URL),
			Projects: d.Projects,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(notificationsResponse{
		PublicURL: publicURL, Destinations: list,
	})
}

// handleNotificationsAdd accepts a full Destination JSON, validates it,
// dedupes by name (replace-on-name), and rewrites config.json atomically.
// The watcher picks up changes on its next tick — no server restart.
func (s *Server) handleNotificationsAdd(w http.ResponseWriter, r *http.Request) {
	var d notify.Destination
	if err := json.NewDecoder(r.Body).Decode(&d); err != nil {
		http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := d.Validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.notifMu.Lock()
	defer s.notifMu.Unlock()
	// Replace-on-name so a "test, fix, save" loop doesn't pile up
	// near-duplicates with bumped names. The user edits the destination
	// by re-POSTing with the same name.
	dests := make([]notify.Destination, 0, len(s.cfg.Notifications)+1)
	replaced := false
	for _, existing := range s.cfg.Notifications {
		if existing.Name == d.Name {
			dests = append(dests, d)
			replaced = true
			continue
		}
		dests = append(dests, existing)
	}
	if !replaced {
		dests = append(dests, d)
	}
	if err := saveNotifications(dests); err != nil {
		http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg.Notifications = dests

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleNotificationsDelete removes by name. Idempotent — a missing
// name returns 200 so the settings UI doesn't need to track local state.
func (s *Server) handleNotificationsDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}

	s.notifMu.Lock()
	defer s.notifMu.Unlock()
	dests := s.cfg.Notifications[:0:0]
	for _, d := range s.cfg.Notifications {
		if d.Name == name {
			continue
		}
		dests = append(dests, d)
	}
	if err := saveNotifications(dests); err != nil {
		http.Error(w, "save: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.cfg.Notifications = dests

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// handleNotificationsTest fires one sample notification at the named
// destination so the user can verify the URL + token before saving. The
// destination must already be configured — we don't accept a URL inline
// here because that would let any browser session POST to any third-
// party URL via this origin (open-relay risk).
func (s *Server) handleNotificationsTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "missing name", http.StatusBadRequest)
		return
	}

	s.notifMu.RLock()
	var dest *notify.Destination
	for _, d := range s.cfg.Notifications {
		if d.Name == req.Name {
			d := d
			dest = &d
			break
		}
	}
	s.notifMu.RUnlock()
	if dest == nil {
		http.Error(w, "destination not found", http.StatusNotFound)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	err := dest.Send(ctx, notify.Notification{
		Title:   "harness-deck test",
		Body:    "Test notification — " + time.Now().UTC().Format(time.RFC3339),
		URL:     s.publicReportURL("harness-deck", "test"),
		Project: "harness-deck",
		Run:     "test",
		Tag:     "test:" + req.Name,
	})

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": err.Error()})
		return
	}
	_, _ = w.Write([]byte(`{"ok":true}`))
}

// saveNotifications writes the destination list back to config.json,
// round-tripping the rest of the file through map[string]any so unknown
// future fields survive. Same pattern as cmd/harness-deck/register.go.
func saveNotifications(dests []notify.Destination) error {
	path := config.Path()
	var current map[string]any
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, &current)
	}
	if current == nil {
		current = map[string]any{}
	}
	// Marshal the typed slice through JSON so the destination shape stays
	// consistent with what config.Load expects to see.
	encoded, err := json.Marshal(dests)
	if err != nil {
		return err
	}
	var decoded any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		return err
	}
	current["notifications"] = decoded

	body, err := json.MarshalIndent(current, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(body, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// redactURL returns "<scheme>://<host>" so the settings UI shows what
// destination the user is dealing with without echoing webhook tokens.
// Falls back to the full URL when parsing fails (defensive: don't drop
// the field entirely).
func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	// Tiny manual split — avoids importing net/url for a one-line
	// purpose; the URL was already validated at config load.
	scheme := ""
	rest := raw
	if i := indexOfStr(raw, "://"); i >= 0 {
		scheme = raw[:i] + "://"
		rest = raw[i+3:]
	}
	if i := indexOfByte(rest, '/'); i >= 0 {
		rest = rest[:i]
	}
	return scheme + rest
}

// Tiny helpers used by redactURL — pulled out so the function reads as
// a single intent rather than mixing index math with the return value.
func indexOfStr(s, sub string) int {
	if sub == "" {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func indexOfByte(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

