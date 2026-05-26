// Package mcp implements a stdlib-only Model Context Protocol server for
// harness-deck.
//
// MCP is an optional wrapper around the file contract documented in
// CONTRACT.md. A harness with MCP support can emit reports via tool calls
// instead of writing JSON to a file directly; under the hood the tools
// perform the same atomic file writes the file path uses, so a dashboard
// running `harness-deck serve` picks them up via its existing 2s watcher.
//
// Transport is newline-delimited JSON-RPC 2.0 over stdio — the simplest
// MCP transport and what all major clients (Claude Code, Claude Desktop,
// VS Code Copilot Chat, etc.) understand. Each line is one JSON-RPC
// message; we never use Content-Length framing here.
//
// Diagnostics go to stderr. Stdout is reserved for the protocol — printing
// anything else would corrupt the stream.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
)

// protocolVersion is the MCP version this server speaks. We pick a stable
// dated revision (2025-06-18) — newer clients negotiate down, older clients
// see the field and decide. The protocol's negotiation step lets us bump
// this later without breaking a deployed harness.
const protocolVersion = "2025-06-18"

// JSON-RPC error codes we use. -32700/-32600/-32601/-32602/-32603 are
// reserved by the JSON-RPC 2.0 spec; -32000 onward is "server-defined."
const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
	errInternal       = -32603
)

// Request is one inbound JSON-RPC 2.0 message. ID is RawMessage so we can
// round-trip whatever shape the client sent (number or string) without
// converting. Method is required; Params is optional.
//
// A request with no ID is a notification — we process it but never reply.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification reports whether this request expects a response. The
// JSON-RPC spec defines a notification as a request with no id field.
func (r Request) IsNotification() bool { return len(r.ID) == 0 }

// Response is one outbound JSON-RPC 2.0 reply. Exactly one of Result or
// Error must be set — the json:omitempty on both, plus the discipline in
// writeResult / writeError, enforces this.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// RPCError is the JSON-RPC error envelope.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// ServerInfo is the {name, version} block the client sees during the
// initialize handshake.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// initializeResult is the payload of the initialize response. Capabilities
// uses map[string]any so we can declare empty objects (`{}`) for
// capabilities the server has but with no sub-fields — what the spec
// requires for "tools" support.
type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      ServerInfo     `json:"serverInfo"`
}

// Tool is one tool the server exposes via tools/list.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// toolsListResult is the payload of the tools/list response.
type toolsListResult struct {
	Tools []Tool `json:"tools"`
}

// ContentItem is one piece of a tool's structured response. We only ever
// emit text content here; the spec also allows image / resource_link /
// embedded_resource items, but text is universally supported and is what
// every CLI-style tool naturally produces.
type ContentItem struct {
	Type string `json:"type"` // always "text" for harness-deck tools
	Text string `json:"text"`
}

// ToolCallResult is the payload of a tools/call response. IsError flips
// the result into a tool-level error (distinct from a protocol-level
// JSON-RPC error — see the spec) so clients can show it inline instead of
// surfacing a protocol failure.
type ToolCallResult struct {
	Content []ContentItem `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// toolCallParams is the params shape for tools/call.
type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolHandler implements one tool. It receives the raw arguments JSON
// (so the handler can decode into a tool-specific struct) and returns a
// ToolCallResult. Returning a non-nil error becomes a protocol-level
// JSON-RPC error; tool-level errors (e.g. validation failures the user
// should see) go through ToolCallResult with IsError=true.
type ToolHandler func(ctx context.Context, args json.RawMessage) (ToolCallResult, error)

// toolDef binds a Tool definition to its handler. We keep them paired so
// the registry stays the single source of truth for tool names.
type toolDef struct {
	Tool    Tool
	Handler ToolHandler
}

// Serve reads JSON-RPC requests from in and writes responses to out until
// in is closed (EOF). One request per line. Diagnostics — parse errors,
// unknown methods, panicked handlers — log to stderr via the standard log
// package; the caller wires stderr separately.
//
// The server is sequential by design: each request is processed before
// the next is read. MCP allows interleaved request/response pairs in
// principle, but our tools are quick and a sequential loop is dramatically
// simpler to reason about. If a tool becomes long-running we'll move it
// to goroutines + a write mutex.
func Serve(ctx context.Context, in io.Reader, out io.Writer, tools map[string]toolDef, info ServerInfo) error {
	scanner := bufio.NewScanner(in)
	// Reports can be large (long markdown blocks, big diffs). Default buf is
	// 64KB which trips on real-world manifests — bump to 4MB which covers
	// every report we've seen in fixtures and tests.
	scanner.Buffer(make([]byte, 0, 1<<16), 4<<20)

	enc := json.NewEncoder(out)
	for scanner.Scan() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			// Protocol-level parse error: we have no id to correlate so we
			// reply with a null id per spec. We only do this when the line
			// looked like JSON at all; obvious garbage gets dropped.
			writeError(enc, nil, errParse, "parse error: "+err.Error())
			continue
		}
		if req.JSONRPC != "2.0" {
			writeError(enc, req.ID, errInvalidRequest, `jsonrpc must be "2.0"`)
			continue
		}
		if err := dispatch(ctx, enc, req, tools, info); err != nil {
			log.Printf("mcp: dispatch %q: %v", req.Method, err)
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// dispatch routes one request to the right handler. Notifications (no id)
// are processed but get no reply.
func dispatch(ctx context.Context, enc *json.Encoder, req Request, tools map[string]toolDef, info ServerInfo) error {
	switch req.Method {
	case "initialize":
		return writeResult(enc, req.ID, initializeResult{
			ProtocolVersion: protocolVersion,
			Capabilities: map[string]any{
				// tools.listChanged would let us proactively notify the client
				// of tool-set changes. We never change tools at runtime, so the
				// empty object is the right signal.
				"tools": map[string]any{},
			},
			ServerInfo: info,
		})

	case "notifications/initialized", "initialized":
		// Notification — no response. We don't need to track init state;
		// every subsequent request just works.
		return nil

	case "tools/list":
		list := make([]Tool, 0, len(tools))
		for _, t := range tools {
			list = append(list, t.Tool)
		}
		return writeResult(enc, req.ID, toolsListResult{Tools: list})

	case "tools/call":
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return writeError(enc, req.ID, errInvalidParams, "bad params: "+err.Error())
		}
		t, ok := tools[params.Name]
		if !ok {
			return writeError(enc, req.ID, errInvalidParams, fmt.Sprintf("unknown tool %q", params.Name))
		}
		res, err := safeCall(ctx, t.Handler, params.Arguments)
		if err != nil {
			return writeError(enc, req.ID, errInternal, err.Error())
		}
		return writeResult(enc, req.ID, res)

	case "ping":
		// Spec ping: round-trip liveness check, empty result.
		return writeResult(enc, req.ID, map[string]any{})

	case "shutdown":
		// Older MCP drafts include shutdown — we accept it for forward
		// compat. The actual shutdown happens when the client closes stdin.
		return writeResult(enc, req.ID, map[string]any{})

	default:
		if req.IsNotification() {
			// Unknown notification: drop silently per spec.
			return nil
		}
		return writeError(enc, req.ID, errMethodNotFound, "method not found: "+req.Method)
	}
}

// safeCall invokes a tool handler with panic recovery, so a buggy tool
// can't crash the whole MCP server (and with it, the harness session).
func safeCall(ctx context.Context, h ToolHandler, args json.RawMessage) (res ToolCallResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("tool panicked: %v", r)
		}
	}()
	return h(ctx, args)
}

// writeResult emits a successful JSON-RPC response. Notifications (nil id)
// get no reply — callers shouldn't reach this for them, but we double-check.
func writeResult(enc *json.Encoder, id json.RawMessage, result any) error {
	if len(id) == 0 {
		return nil
	}
	return enc.Encode(Response{JSONRPC: "2.0", ID: id, Result: result})
}

// writeError emits a JSON-RPC error response. A nil id is allowed (for
// pre-id parse errors) and serialized as JSON null per spec.
func writeError(enc *json.Encoder, id json.RawMessage, code int, msg string) error {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return enc.Encode(Response{JSONRPC: "2.0", ID: id, Error: &RPCError{Code: code, Message: msg}})
}
