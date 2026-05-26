package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/mcp"
)

// cmdMCP starts a stdio MCP server. The harness spawns this as a
// subprocess and speaks JSON-RPC 2.0 over the pipe.
//
// We deliberately separate logging (stderr) from protocol (stdout):
// any stray Printf to stdout would corrupt the JSON-RPC stream, so the
// log package gets pointed at stderr explicitly before we hand off.
func cmdMCP(_ []string) {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ltime | log.Lshortfile)

	cfg, err := config.Load()
	if err != nil {
		fatal("mcp: config", err)
	}

	srv := mcp.New(cfg, version)

	// Cancel on SIGINT/SIGTERM so the server unwinds cleanly when the
	// parent harness exits or is interrupted. Closing stdin (the normal
	// shutdown path) also returns from Run via scanner EOF.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := srv.Run(ctx, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "harness-deck: mcp: %v\n", err)
		os.Exit(1)
	}
}
