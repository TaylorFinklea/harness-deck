// Package server runs the harness-deck dashboard: the aggregator shell, the
// report-index API, and individual rendered report pages.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/assets"
	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/notify"
	"github.com/TaylorFinklea/harness-deck/internal/render"
	"github.com/TaylorFinklea/harness-deck/internal/respond"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

//go:embed shell.html.tmpl
var shellFS embed.FS

// pollInterval is how often the watcher rescans for report changes.
const pollInterval = 2 * time.Second

// Server wires the report store, the HTML renderer, and the HTTP routes.
type Server struct {
	cfg      config.Config
	store    *store.Store
	renderer *render.Renderer
	shell    *template.Template
	hub      *hub
	mux      *http.ServeMux
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
	st := store.New(cfg)
	st.Scan()

	s := &Server{cfg: cfg, store: st, renderer: renderer, shell: shell, hub: newHub()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleShell)
	mux.HandleFunc("GET /api/reports", s.handleReports)
	mux.HandleFunc("GET /events", s.handleEvents)
	mux.HandleFunc("GET /r/{project}/{run}", s.handleReport)
	mux.HandleFunc("POST /r/{project}/{run}/respond", s.handleRespond)
	s.mux = mux
	return s, nil
}

// Handler exposes the routes (used by tests).
func (s *Server) Handler() http.Handler { return s.mux }

// Serve starts the change watcher and the HTTP server on the configured port.
func (s *Server) Serve() error {
	go s.watch(pollInterval)
	return http.ListenAndServe(fmt.Sprintf("127.0.0.1:%d", s.cfg.Port), s.mux)
}

// handleShell serves the aggregator dashboard page.
func (s *Server) handleShell(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	err := s.shell.ExecuteTemplate(w, "shell", struct {
		CSS          template.CSS
		VimJS, AppJS template.JS
	}{
		CSS:   template.CSS(assets.DeckUICSS),
		VimJS: template.JS(assets.VimNavJSInline),
		AppJS: template.JS(assets.AggregatorJS),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
	html, err := s.renderer.Report(rep, answers.Responses)
	if err != nil {
		http.Error(w, "render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(html)
}

// respondRequest is the POST body for recording one answer.
type respondRequest struct {
	Block string `json:"block"`
	Value string `json:"value"`
	Note  string `json:"note"`
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
	if _, err := respond.Record(entry.Dir, project, run, respond.Response{
		Block: req.Block, Value: req.Value, Note: req.Note,
	}); err != nil {
		http.Error(w, "could not record response: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := notify.Run(s.cfg.NotifyCommand, entry.Dir, project, run, req.Block); err != nil {
		log.Printf("harness-deck: notify command failed: %v", err)
	}
	// Reindex so OpenAsks counts drop, and tell the dashboard to refresh.
	s.store.Scan()
	s.hub.broadcast("reports")

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
