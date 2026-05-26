package mcp

import (
	"context"
	"fmt"
	"io"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// Server is the MCP server entry point. It owns the loaded config (so
// tools know where the central reports dir is and which roots to scan)
// and the tool registry.
type Server struct {
	cfg   config.Config
	tools map[string]toolDef
	info  ServerInfo
}

// New constructs an MCP server with the default tool set wired to the
// passed config. The version string is stamped into the initialize
// handshake response so clients can show which build they're talking to.
func New(cfg config.Config, version string) *Server {
	s := &Server{
		cfg:   cfg,
		tools: map[string]toolDef{},
		info:  ServerInfo{Name: "harness-deck", Version: version},
	}
	s.registerDefaults()
	return s
}

// Run starts the JSON-RPC loop over the given pipes. Closing in (EOF)
// returns nil; any other read error returns it.
func (s *Server) Run(ctx context.Context, in io.Reader, out io.Writer) error {
	return Serve(ctx, in, out, s.tools, s.info)
}

// register adds one tool to the registry. We panic on a duplicate name —
// it's a programming error, not a runtime condition the user can fix.
func (s *Server) register(t Tool, h ToolHandler) {
	if _, exists := s.tools[t.Name]; exists {
		panic(fmt.Sprintf("mcp: duplicate tool %q", t.Name))
	}
	s.tools[t.Name] = toolDef{Tool: t, Handler: h}
}
