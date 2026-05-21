// Package server runs the harness-deck dashboard: the aggregator shell, the
// report-index API, and individual rendered report pages.
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"

	"github.com/TaylorFinklea/harness-deck/internal/assets"
	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/render"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

//go:embed shell.html.tmpl
var shellFS embed.FS

// Server wires the report store, the HTML renderer, and the HTTP routes.
type Server struct {
	cfg      config.Config
	store    *store.Store
	renderer *render.Renderer
	shell    *template.Template
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

	s := &Server{cfg: cfg, store: st, renderer: renderer, shell: shell}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.handleShell)
	mux.HandleFunc("GET /api/reports", s.handleReports)
	mux.HandleFunc("GET /r/{project}/{run}", s.handleReport)
	s.mux = mux
	return s, nil
}

// Handler exposes the routes (used by tests).
func (s *Server) Handler() http.Handler { return s.mux }

// Serve starts the HTTP server on the configured port.
func (s *Server) Serve() error {
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

// handleReports rescans and returns the report index as JSON. Rescanning per
// request keeps the index fresh before Phase 3 replaces polling with fsnotify.
func (s *Server) handleReports(w http.ResponseWriter, _ *http.Request) {
	s.store.Scan()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"reports": s.store.Entries(),
		"errors":  s.store.Errors(),
	})
}

// handleReport renders one report to a full HTML page.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	rep, _, err := s.store.Get(r.PathValue("project"), r.PathValue("run"))
	if err != nil {
		http.Error(w, "report not found: "+err.Error(), http.StatusNotFound)
		return
	}
	html, err := s.renderer.Report(rep)
	if err != nil {
		http.Error(w, "render failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(html)
}
