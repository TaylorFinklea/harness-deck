package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/jsonfile"
	"github.com/TaylorFinklea/harness-deck/internal/manifest"
	"github.com/TaylorFinklea/harness-deck/internal/respond"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// safeNameRe gates the project / run / status path components. We accept
// dotted, dashed, underscored alphanumerics — no slashes, no `..`, no
// shell metacharacters — so a malicious caller can't escape the central
// reports dir via the same path joining the real harness uses.
var safeNameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// safePathComponent validates one segment. Empty + over-long + non-matching
// inputs all fail; callers should treat the boolean as authoritative.
func safePathComponent(s string) bool {
	if s == "" || len(s) > 200 {
		return false
	}
	if s == "." || s == ".." {
		return false
	}
	return safeNameRe.MatchString(s)
}

// registerDefaults binds the built-in tool set. Each tool's input schema
// is a small JSON Schema fragment; clients use it to coach the user
// (Claude Code, for example, surfaces it as a popover on the tool call).
func (s *Server) registerDefaults() {
	s.register(Tool{
		Name: "publish_report",
		Description: "Publish a harness-deck report. Atomically writes report.json " +
			"either to the central reports dir (~/.harness/reports/<project>/<id>/) " +
			"or to <repo_path>/.harness/<id>/ when repo_path is set. Validates the " +
			"manifest first; validation failures return a tool error with the " +
			"problem list and no file is written.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"manifest": map[string]any{
					"type":        "object",
					"description": "The full report manifest (matching CONTRACT.md). Required fields: schema, id, project, harness, title, status, created.",
				},
				"repo_path": map[string]any{
					"type":        "string",
					"description": "Optional — when set, write under <repo_path>/.harness/<id>/ instead of the central dir.",
				},
			},
			"required": []string{"manifest"},
		},
	}, s.toolPublishReport)

	s.register(Tool{
		Name:        "validate_report",
		Description: "Validate a harness-deck manifest without writing it. Returns the list of problems (empty if valid).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"manifest": map[string]any{
					"type":        "object",
					"description": "The report manifest to validate.",
				},
			},
			"required": []string{"manifest"},
		},
	}, s.toolValidateReport)

	s.register(Tool{
		Name: "get_responses",
		Description: "Read responses.json for a published report. Returns " +
			"an empty responses object if the user hasn't answered any " +
			"interactive blocks yet — not an error.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project":   map[string]any{"type": "string"},
				"run":       map[string]any{"type": "string", "description": "the report id"},
				"repo_path": map[string]any{"type": "string", "description": "optional — look in <repo_path>/.harness/<run>/ instead of the central dir"},
			},
			"required": []string{"project", "run"},
		},
	}, s.toolGetResponses)

	s.register(Tool{
		Name:        "list_reports",
		Description: "List every indexed report — title, status, open_asks, kind, created — optionally filtered to one project.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project": map[string]any{"type": "string", "description": "optional filter"},
			},
		},
	}, s.toolListReports)

	s.register(Tool{
		Name: "update_live",
		Description: "Push in-flight telemetry to a published report. Atomically " +
			"merges {step, elapsed_ms, tokens, cost_usd, progress} into the " +
			"manifest's `live` field; `updated` is set to the current UTC time " +
			"automatically. Cheap to call repeatedly (every few seconds) — every " +
			"other manifest field is preserved.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project":    map[string]any{"type": "string"},
				"run":        map[string]any{"type": "string"},
				"step":       map[string]any{"type": "string", "description": "short human description of the current step"},
				"elapsed_ms": map[string]any{"type": "integer", "description": "milliseconds since the run started"},
				"tokens":     map[string]any{"type": "integer", "description": "cumulative token count"},
				"cost_usd":   map[string]any{"type": "string", "description": "free-form dollar amount (e.g. \"0.42\" or \"$1.84\")"},
				"progress":   map[string]any{"type": "number", "description": "0..1 progress fraction (optional)"},
				"repo_path":  map[string]any{"type": "string"},
			},
			"required": []string{"project", "run"},
		},
	}, s.toolUpdateLive)

	s.register(Tool{
		Name: "update_status",
		Description: "Set the status of a published report (draft / awaiting-review / answered / done). " +
			"Reads the manifest, mutates the status field, writes atomically — every other field is preserved.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"project":   map[string]any{"type": "string"},
				"run":       map[string]any{"type": "string"},
				"status":    map[string]any{"type": "string", "enum": []string{"draft", "awaiting-review", "answered", "done"}},
				"repo_path": map[string]any{"type": "string", "description": "optional, see publish_report"},
			},
			"required": []string{"project", "run", "status"},
		},
	}, s.toolUpdateStatus)
}

// runDir resolves the directory a report's files live in. With repo_path
// set it's <repo_path>/.harness/<run>/; otherwise it's the central
// reports dir from config. Both project and run go through
// safePathComponent.
func (s *Server) runDir(project, run, repoPath string) (string, error) {
	if !safePathComponent(project) {
		return "", fmt.Errorf("invalid project name %q", project)
	}
	if !safePathComponent(run) {
		return "", fmt.Errorf("invalid run id %q", run)
	}
	if repoPath != "" {
		// Resolve repoPath but don't enforce that it exists — the writer
		// will MkdirAll. We do require an absolute path so a relative
		// "../etc" can't sneak past the joiner.
		abs, err := filepath.Abs(repoPath)
		if err != nil {
			return "", fmt.Errorf("repo_path: %w", err)
		}
		return filepath.Join(abs, ".harness", run), nil
	}
	return filepath.Join(config.Expand(s.cfg.CentralDir), project, run), nil
}

// publishArgs is the input shape for publish_report. Manifest stays as
// RawMessage so we can validate the original bytes (strict-mode
// json.Decoder catches typos that a re-encode would silently smooth over).
type publishArgs struct {
	Manifest json.RawMessage `json:"manifest"`
	RepoPath string          `json:"repo_path,omitempty"`
}

func (s *Server) toolPublishReport(_ context.Context, raw json.RawMessage) (ToolCallResult, error) {
	var args publishArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolErr("bad arguments: " + err.Error()), nil
	}
	if len(args.Manifest) == 0 {
		return toolErr("missing manifest"), nil
	}
	// Parse + strict-validate. Parse is lenient (tolerates unknown blocks),
	// Validate is strict. Both run because we want the bytes shape known
	// good AND the semantic constraints satisfied.
	rep, err := manifest.Parse(args.Manifest)
	if err != nil {
		return toolErr("parse: " + err.Error()), nil
	}
	if problems := rep.Validate(); len(problems) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "validation failed (%d problem(s)):\n", len(problems))
		for _, p := range problems {
			fmt.Fprintf(&b, "  · %s\n", p)
		}
		return toolErr(b.String()), nil
	}
	dir, err := s.runDir(rep.Project, rep.ID, args.RepoPath)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return toolErr("mkdir: " + err.Error()), nil
	}
	target := filepath.Join(dir, "report.json")
	// Pretty-print on the way out so a human reading the file sees the same
	// shape the file-path harnesses produce. Marshal of *Report uses the
	// canonical field order we've documented in CONTRACT.md.
	pretty, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		return toolErr("marshal: " + err.Error()), nil
	}
	if err := jsonfile.AtomicWrite(target, append(pretty, '\n'), 0o644); err != nil {
		return toolErr("write: " + err.Error()), nil
	}
	return toolOK(fmt.Sprintf("wrote %s (%d block(s))", target, len(rep.Blocks))), nil
}

// validateArgs is shared with toolValidateReport.
type validateArgs struct {
	Manifest json.RawMessage `json:"manifest"`
}

func (s *Server) toolValidateReport(_ context.Context, raw json.RawMessage) (ToolCallResult, error) {
	var args validateArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolErr("bad arguments: " + err.Error()), nil
	}
	if len(args.Manifest) == 0 {
		return toolErr("missing manifest"), nil
	}
	rep, err := manifest.Parse(args.Manifest)
	if err != nil {
		return toolErr("parse: " + err.Error()), nil
	}
	problems := rep.Validate()
	if len(problems) == 0 {
		return toolOK(fmt.Sprintf("ok — %d block(s), no problems", len(rep.Blocks))), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d problem(s):\n", len(problems))
	for _, p := range problems {
		fmt.Fprintf(&b, "  · %s\n", p)
	}
	return ToolCallResult{
		Content: []ContentItem{{Type: "text", Text: b.String()}},
		IsError: true,
	}, nil
}

type getRespArgs struct {
	Project  string `json:"project"`
	Run      string `json:"run"`
	RepoPath string `json:"repo_path,omitempty"`
}

func (s *Server) toolGetResponses(_ context.Context, raw json.RawMessage) (ToolCallResult, error) {
	var args getRespArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolErr("bad arguments: " + err.Error()), nil
	}
	dir, err := s.runDir(args.Project, args.Run, args.RepoPath)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	file, err := respond.Load(dir)
	if err != nil {
		return toolErr("read: " + err.Error()), nil
	}
	// Marshal back to JSON. A run with no answers serializes to a non-error
	// payload with an empty Responses map — that's the documented "not an
	// error" state.
	out, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return toolErr("marshal: " + err.Error()), nil
	}
	return toolOK(string(out)), nil
}

type listArgs struct {
	Project string `json:"project,omitempty"`
}

func (s *Server) toolListReports(_ context.Context, raw json.RawMessage) (ToolCallResult, error) {
	var args listArgs
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &args); err != nil {
			return toolErr("bad arguments: " + err.Error()), nil
		}
	}
	// Fresh store scan — the MCP process doesn't share state with a
	// long-running `harness-deck serve`, so we re-discover on demand. Cheap
	// in practice because the filesystem walks the same dirs the watcher
	// already keeps warm.
	st := store.New(s.cfg)
	roots := make([]string, 0, len(s.cfg.Projects))
	for _, p := range s.cfg.Projects {
		roots = append(roots, p)
	}
	st.Scan(roots)
	entries := st.Entries()

	type row struct {
		Project  string `json:"project"`
		Run      string `json:"run"`
		Title    string `json:"title"`
		Kind     string `json:"kind,omitempty"`
		Status   string `json:"status"`
		Created  string `json:"created"`
		OpenAsks int    `json:"open_asks"`
		Archived bool   `json:"archived,omitempty"`
	}
	out := make([]row, 0, len(entries))
	for _, e := range entries {
		if args.Project != "" && e.Project != args.Project {
			continue
		}
		out = append(out, row{
			Project: e.Project, Run: e.Run, Title: e.Title, Kind: e.Kind,
			Status: e.Status, Created: e.Created, OpenAsks: e.OpenAsks,
			Archived: e.Archived,
		})
	}
	body, err := json.MarshalIndent(map[string]any{"reports": out}, "", "  ")
	if err != nil {
		return toolErr("marshal: " + err.Error()), nil
	}
	return toolOK(string(body)), nil
}

type updateStatusArgs struct {
	Project  string `json:"project"`
	Run      string `json:"run"`
	Status   string `json:"status"`
	RepoPath string `json:"repo_path,omitempty"`
}

func (s *Server) toolUpdateStatus(_ context.Context, raw json.RawMessage) (ToolCallResult, error) {
	var args updateStatusArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolErr("bad arguments: " + err.Error()), nil
	}
	if !manifest.ValidStatus(args.Status) {
		return toolErr(fmt.Sprintf("invalid status %q (must be draft / awaiting-review / answered / done)", args.Status)), nil
	}
	dir, err := s.runDir(args.Project, args.Run, args.RepoPath)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	// jsonfile.Patch preserves any field we don't know about (newer
	// manifest field added by a future renderer, future Live block, …)
	// and keeps number literals exact across the rewrite.
	err = jsonfile.Patch(filepath.Join(dir, "report.json"), func(doc map[string]any) error {
		doc["status"] = args.Status
		return nil
	})
	if err != nil {
		return toolErr(err.Error()), nil
	}
	return toolOK(fmt.Sprintf("status = %q at %s", args.Status, time.Now().UTC().Format(time.RFC3339))), nil
}

type updateLiveArgs struct {
	Project   string   `json:"project"`
	Run       string   `json:"run"`
	Step      *string  `json:"step,omitempty"`
	ElapsedMs *int64   `json:"elapsed_ms,omitempty"`
	Tokens    *int64   `json:"tokens,omitempty"`
	CostUSD   *string  `json:"cost_usd,omitempty"`
	Progress  *float64 `json:"progress,omitempty"`
	RepoPath  string   `json:"repo_path,omitempty"`
}

// toolUpdateLive merges live telemetry into a report.json without
// rewriting unrelated fields. The pointer types in updateLiveArgs let us
// distinguish "client didn't send this field" from "client sent zero":
// missing fields are preserved as-is, explicit zeros overwrite. This is
// what makes the tool cheap to call every few seconds — a harness can
// push just `step` and `tokens` while leaving cost alone.
func (s *Server) toolUpdateLive(_ context.Context, raw json.RawMessage) (ToolCallResult, error) {
	var args updateLiveArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolErr("bad arguments: " + err.Error()), nil
	}
	dir, err := s.runDir(args.Project, args.Run, args.RepoPath)
	if err != nil {
		return toolErr(err.Error()), nil
	}
	updated := time.Now().UTC().Format(time.RFC3339)
	err = jsonfile.Patch(filepath.Join(dir, "report.json"), func(doc map[string]any) error {
		live, _ := doc["live"].(map[string]any)
		if live == nil {
			live = map[string]any{}
		}
		live["updated"] = updated
		if args.Step != nil {
			live["step"] = *args.Step
		}
		if args.ElapsedMs != nil {
			live["elapsed_ms"] = *args.ElapsedMs
		}
		if args.Tokens != nil {
			live["tokens"] = *args.Tokens
		}
		if args.CostUSD != nil {
			live["cost_usd"] = *args.CostUSD
		}
		if args.Progress != nil {
			live["progress"] = *args.Progress
		}
		doc["live"] = live
		return nil
	})
	if err != nil {
		return toolErr(err.Error()), nil
	}
	return toolOK(fmt.Sprintf("live updated at %s", updated)), nil
}

// toolOK is the standard "happy path" tool result.
func toolOK(text string) ToolCallResult {
	return ToolCallResult{Content: []ContentItem{{Type: "text", Text: text}}}
}

// toolErr is a tool-level (not protocol-level) error — surfaces inline in
// the client UI with isError set.
func toolErr(text string) ToolCallResult {
	return ToolCallResult{Content: []ContentItem{{Type: "text", Text: text}}, IsError: true}
}
