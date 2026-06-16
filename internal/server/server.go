// Package server runs the harness-deck dashboard: the aggregator shell, the
// report-index API, and individual rendered report pages.
package server

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	harnessdeck "github.com/TaylorFinklea/harness-deck"
	"github.com/TaylorFinklea/harness-deck/internal/assets"
	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/notify"
	"github.com/TaylorFinklea/harness-deck/internal/projects"
	"github.com/TaylorFinklea/harness-deck/internal/push"
	"github.com/TaylorFinklea/harness-deck/internal/render"
	"github.com/TaylorFinklea/harness-deck/internal/respond"
	"github.com/TaylorFinklea/harness-deck/internal/store"
	"github.com/TaylorFinklea/harness-deck/internal/usage"
)

//go:embed shell.html.tmpl
var shellFS embed.FS

// pollInterval is how often the watcher rescans for report changes.
const pollInterval = 2 * time.Second

// Server wires the report store, the HTML renderer, and the HTTP routes.
type Server struct {
	cfg      config.Config
	store    *store.Store
	projects *projects.Manager
	renderer *render.Renderer
	shell    *template.Template
	hub      *hub
	mux      *http.ServeMux
	// Push state. pushKeys is nil until the user runs `harness-deck vapid`;
	// when nil the push endpoints return 503 and the watcher skips notifying.
	pushKeys *push.Keys
	subs     *push.Store
	// usage is the footer usage monitor, or nil when no usage providers are
	// configured. Serve starts it; /api/usage serves its cached samples.
	usage *usage.Monitor

	// docCache memoizes rendered project markdown (roadmap.md / current-state.md)
	// keyed by file path, invalidated by mtime, so /api/projects doesn't
	// re-render every doc on every poll. Guarded by docMu.
	docMu    sync.Mutex
	docCache map[string]docCacheEntry
	// notifMu guards cfg.Notifications + cfg.PublicURL against concurrent
	// reads from the watcher and writes from /api/notifications/* CRUD.
	notifMu sync.RWMutex

	// testNotifyFn, when non-nil, is called once per new-ask notification
	// instead of (in addition to) the real push/fanout path. Tests use this
	// seam to count notification fires without needing real VAPID keys.
	testNotifyFn func()

	// testDigestCountFn, when non-nil, is called each time currentAskDigests
	// is actually invoked. Tests use this seam to verify signature-gated
	// skipping behaviour.
	testDigestCountFn func()

	// testScanLogFn, when non-nil, receives scan-timing log messages instead
	// of (in addition to) the real log.Printf calls. Tests use this to assert
	// on the warn path without parsing real log output.
	testScanLogFn func(string)

	// testScanDuration, when non-zero, overrides the measured scan duration
	// reported in scan-timing log lines. Tests set this to trigger the
	// warn path without actually sleeping.
	testScanDuration time.Duration
}

// New builds a Server and performs the initial report scan.
func New(cfg config.Config) (*Server, error) {
	renderer, err := render.New()
	if err != nil {
		return nil, err
	}
	shell, err := template.New("shell").ParseFS(shellFS, "shell.html.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parse shell template: %w", err)
	}
	pm := projects.NewManager(cfg.ScanRoots, cfg.Projects, projects.StatePath())
	st := store.New(cfg)

	// Push is optional. A missing vapid.json means the user hasn't run
	// `harness-deck vapid` yet; everything else keeps working without it.
	vapidPath := filepath.Join(config.Dir(), "vapid.json")
	keys, _, kerr := push.LoadOrMissing(vapidPath)
	if kerr != nil {
		log.Printf("harness-deck: vapid keys: %v — push disabled", kerr)
	}
	subs := push.NewStore(filepath.Join(config.Dir(), "subscriptions.json"))

	s := &Server{cfg: cfg, store: st, projects: pm, renderer: renderer, shell: shell, hub: newHub(), pushKeys: keys, subs: subs, docCache: map[string]docCacheEntry{}}
	s.store.Scan(s.enabledRoots())

	// Footer usage monitors (CodexBar-style). nil when no providers configured.
	s.usage = usage.NewMonitor(usage.Build(usage.Options{
		Providers:           cfg.Usage.Providers,
		OpenRouterKey:       cfg.Usage.OpenRouterKey,
		OpenCodeCookie:      cfg.Usage.OpenCodeCookie,
		OpenCodeWorkspaceID: cfg.Usage.OpenCodeWorkspaceID,
	}), time.Duration(cfg.Usage.RefreshSec)*time.Second)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleShell)
	mux.HandleFunc("GET /api/reports", s.handleReports)
	mux.HandleFunc("GET /api/projects", s.handleProjects)
	mux.HandleFunc("POST /api/projects/toggle", s.handleProjectToggle)
	mux.HandleFunc("POST /api/projects/reorder", s.handleProjectReorder)
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.HandleFunc("GET /r/{project}/{run}", s.handleReport)
	mux.HandleFunc("POST /r/{project}/{run}/respond", s.handleRespond)
	mux.HandleFunc("GET /r/{project}/{run}/sig", s.handleReportSig)
	mux.HandleFunc("POST /r/{project}/{run}/close", s.handleReportClose)
	mux.HandleFunc("POST /r/{project}/{run}/reopen", s.handleReportReopen)
	mux.HandleFunc("POST /r/{project}/{run}/archive", s.handleReportArchive)
	mux.HandleFunc("POST /r/{project}/{run}/unarchive", s.handleReportUnarchive)
	mux.HandleFunc("DELETE /r/{project}/{run}", s.handleReportDelete)
	mux.HandleFunc("GET /api/search", s.handleSearch)
	mux.HandleFunc("GET /api/search/schema", s.handleSearchSchema)
	mux.HandleFunc("GET /api/usage", s.handleUsage)
	mux.HandleFunc("GET /api/push/vapid-key", s.handleVAPIDKey)
	mux.HandleFunc("GET /api/push/status", s.handlePushStatus)
	mux.HandleFunc("POST /api/push/subscribe", s.handlePushSubscribe)
	mux.HandleFunc("POST /api/push/unsubscribe", s.handlePushUnsubscribe)
	mux.HandleFunc("GET /api/notifications", s.handleNotificationsList)
	mux.HandleFunc("POST /api/notifications", s.handleNotificationsAdd)
	mux.HandleFunc("DELETE /api/notifications/{name}", s.handleNotificationsDelete)
	mux.HandleFunc("POST /api/notifications/test", s.handleNotificationsTest)
	mux.HandleFunc("GET /contract.md", s.handleContract)
	mux.HandleFunc("GET /manifest.webmanifest", s.handleManifest)
	mux.HandleFunc("GET /service-worker.js", s.handleServiceWorker)
	mux.HandleFunc("GET /icon.svg", s.handleIcon)
	mux.HandleFunc("GET /icon-180.png", s.handleIconPNG(assets.IconPNG180))
	mux.HandleFunc("GET /icon-192.png", s.handleIconPNG(assets.IconPNG192))
	mux.HandleFunc("GET /icon-512.png", s.handleIconPNG(assets.IconPNG512))
	mux.HandleFunc("GET /icon-1024.png", s.handleIconPNG(assets.IconPNG1024))
	s.mux = mux
	return s, nil
}

// enabledRoots is the filesystem paths of the projects the user is tracking.
func (s *Server) enabledRoots() []string {
	enabled := s.projects.Enabled()
	roots := make([]string, len(enabled))
	for i, p := range enabled {
		roots[i] = p.Path
	}
	return roots
}

// changeFingerprint digests everything the dashboard reflects — the report
// index plus discovered projects and their docs — so the watcher can detect
// any of it changing.
func (s *Server) changeFingerprint() string {
	return s.store.Signature() + "|" + s.projects.Fingerprint()
}

// Handler exposes the routes (used by tests).
func (s *Server) Handler() http.Handler { return s.mux }

// Serve starts the change watcher and the HTTP server. When the config has
// both a TLS cert and key, it serves HTTPS — required for iOS web push.
// Otherwise it serves plain HTTP. Bind defaults to 127.0.0.1; set it to a
// reachable interface (e.g. a Tailscale address or "0.0.0.0") to expose the
// dashboard to a phone.
func (s *Server) Serve() error {
	s.usage.Start(context.Background())
	go s.watch(pollInterval)
	addr := fmt.Sprintf("%s:%d", s.cfg.Bind, s.cfg.Port)
	if s.cfg.TLS.Enabled() {
		// Expand ~ in cert/key paths so config files can use the standard
		// shorthand (e.g. "~/.config/tailscale-certs/host.crt").
		return http.ListenAndServeTLS(addr,
			config.Expand(s.cfg.TLS.Cert),
			config.Expand(s.cfg.TLS.Key),
			s.mux)
	}
	return http.ListenAndServe(addr, s.mux)
}

// handleShell serves the aggregator dashboard page.
func (s *Server) handleShell(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := s.shell.ExecuteTemplate(w, "shell", struct {
		CSS                                                                 template.CSS
		HDDomJS, VimJS, AppJS, MobileJS, TabsJS, SavedJS, SearchJS, UsageJS template.JS
		Favicon                                                             template.URL
		Addr                                                                string
	}{
		CSS:      template.CSS(assets.DeckUICSS),
		HDDomJS:  template.JS(assets.HDDomJSInline),
		VimJS:    template.JS(assets.VimNavJSInline),
		AppJS:    template.JS(assets.AggregatorJS),
		MobileJS: template.JS(assets.MobileJSInline),
		TabsJS:   template.JS(assets.TabsJSInline),
		SavedJS:  template.JS(assets.SavedJSInline),
		SearchJS: template.JS(assets.SearchJSInline),
		UsageJS:  template.JS(assets.UsageJSInline),
		Favicon:  template.URL(assets.FaviconDataURI),
		Addr:     statusAddr(s.cfg),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleUsage serves the cached footer usage samples (CodexBar-style). The
// array is empty when no usage providers are configured; entries with ok:false
// carry an err for diagnostics and are skipped by the footer.
func (s *Server) handleUsage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	samples := s.usage.Samples() // nil-safe
	if samples == nil {
		samples = []usage.Sample{}
	}
	_ = json.NewEncoder(w).Encode(samples)
}

// statusAddr is the host:port shown in the footer — the dashboard's reachable
// address, scheme stripped (the footer's previous value was hardcoded).
func statusAddr(cfg config.Config) string {
	u := cfg.BaseURL()
	if i := strings.Index(u, "://"); i >= 0 {
		return u[i+3:]
	}
	return u
}

// handleManifest serves the PWA web app manifest.
func (s *Server) handleManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/manifest+json")
	_, _ = w.Write([]byte(assets.ManifestJSON))
}

// handleContract serves the embedded report contract (CONTRACT.md) as
// markdown, so an agent can fetch the schema from a running deck without a
// repo clone. It's the HTTP twin of `harness-deck contract` and the MCP
// harness-deck://contract resource — all three read the same embedded bytes.
func (s *Server) handleContract(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	_, _ = w.Write([]byte(harnessdeck.Contract))
}

// handleServiceWorker serves the service worker script. Browsers require
// it to be served from same-origin and with a JS MIME type, so we set
// the Content-Type explicitly. The Service-Worker-Allowed header lets the
// SW control the whole site even if browsers ever serve it from a
// subdirectory.
func (s *Server) handleServiceWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Service-Worker-Allowed", "/")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(assets.ServiceWorkerJS))
}

// handleIcon serves the hd monogram as the PWA icon (manifest icons +
// apple-touch-icon both fetch this).
func (s *Server) handleIcon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write([]byte(assets.FaviconSVG))
}

// handleIconPNG returns a closure that serves a single prerendered PNG icon.
// Each size gets its own route so the manifest can declare exact dimensions,
// and so iOS / Android caches can fetch the closest match without resampling.
func (s *Server) handleIconPNG(payload []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(payload)
	}
}

// handleReports returns the report index as JSON. The watcher keeps the store
// fresh, so this serves the current snapshot without rescanning.
func (s *Server) handleReports(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"reports": s.store.Entries(),
		"errors":  s.store.Errors(),
	})
}

// handleReport renders one report to a full HTML page, including any responses
// already recorded for its interactive blocks.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	rep, entry, err := s.store.Get(r.PathValue("project"), r.PathValue("run"))
	if err != nil {
		http.Error(w, "report not found: "+err.Error(), http.StatusNotFound)
		return
	}
	answers, _ := respond.Load(entry.Dir) // missing responses file is fine
	html, err := s.renderer.Report(rep, answers.Responses, entry.Sig())
	if err != nil {
		http.Error(w, "render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(html)
}

// respondRequest is the POST body for recording one answer.
type respondRequest struct {
	Block  string   `json:"block"`
	Value  string   `json:"value"`
	Values []string `json:"values"`
	Note   string   `json:"note"`
}

// handleRespond records the user's answer to an interactive block into the
// run's responses.json, fires the notify command, and refreshes the dashboard.
func (s *Server) handleRespond(w http.ResponseWriter, r *http.Request) {
	project, run := r.PathValue("project"), r.PathValue("run")
	_, entry, err := s.store.Get(project, run)
	if err != nil {
		http.Error(w, "report not found: "+err.Error(), http.StatusNotFound)
		return
	}
	var req respondRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Block == "" {
		http.Error(w, "missing block id", http.StatusBadRequest)
		return
	}
	// For multi-select asks, join the selected values into a single summary
	// string and record both the joined value and the individual values slice.
	value := req.Value
	var values []string
	if len(req.Values) > 0 {
		value = strings.Join(req.Values, ", ")
		values = req.Values
	}
	if _, err := respond.Record(entry.Dir, project, run, respond.Response{
		Block: req.Block, Value: value, Values: values, Note: req.Note,
	}); err != nil {
		http.Error(w, "could not record response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	// Reindex so OpenAsks counts drop, and tell the dashboard to refresh.
	s.store.Scan(s.enabledRoots())
	s.hub.broadcast("reports")
	// Emit a distinct SSE event so a live harness can be pushed the answer
	// instead of polling responses.json.
	if evtData, jerr := json.Marshal(map[string]string{
		"project": project,
		"run":     run,
		"block":   req.Block,
		"value":   value,
	}); jerr == nil {
		s.hub.broadcastEvent("response", string(evtData))
	}
	// Fire the notify command after the broadcast so a slow (but
	// timeout-bounded) command can't delay the user-visible refresh.
	if err := notify.Run(s.cfg.NotifyCommand, entry.Dir, project, run, req.Block, value, req.Note); err != nil {
		log.Printf("harness-deck: notify command failed: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
