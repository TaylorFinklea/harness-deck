// Package beads is a read-only adapter over the `bd` (beads) issue-tracker CLI.
//
// It shells `bd -C <root> … --json` for every `.beads/`-enabled repo and caches
// the priority-sorted ready queue, blocked items, and the dependency-graph edges
// so the dashboard can render a live Backlog view. It never reads the `.beads/`
// directory directly (that is a binary Dolt DB) — the CLI with --json is the only
// supported read path. Missing `bd`, a repo without `.beads/`, or a failing call
// all degrade to an empty/per-repo-error state rather than an error.
package beads

// Issue is one bd issue. Only the fields we consume are declared; unknown JSON
// keys are ignored so a newer bd stays forward-compatible.
type Issue struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	Status          string   `json:"status"`
	IssueType       string   `json:"issue_type"`
	Owner           string   `json:"owner,omitempty"`
	Priority        int      `json:"priority"`
	Labels          []string `json:"labels,omitempty"`
	DependencyCount int      `json:"dependency_count"`
	DependentCount  int      `json:"dependent_count"`
	CommentCount    int      `json:"comment_count"`
	Created         string   `json:"created_at,omitempty"`
	Updated         string   `json:"updated_at,omitempty"`
	Parent          string   `json:"parent,omitempty"`
	BlockedBy       []string `json:"blocked_by,omitempty"`
}

// Edge is a directed dependency-graph edge. Kind is "blocks" (From blocks To) or
// "parent" (From is the parent of To).
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// Counts is the per-repo summary from `bd status --json`.
type Counts struct {
	Open       int `json:"open"`
	Ready      int `json:"ready"`
	Blocked    int `json:"blocked"`
	InProgress int `json:"in_progress"`
	Total      int `json:"total"`
}

// RepoSnapshot is one repo's beads state for a refresh cycle. A non-empty Err
// isolates a repo whose `bd` calls failed; other repos still render.
type RepoSnapshot struct {
	Name    string  `json:"name"`
	Root    string  `json:"root"`
	Ready   []Issue `json:"ready"`
	Blocked []Issue `json:"blocked"`
	All     []Issue `json:"all"`
	Edges   []Edge  `json:"edges"`
	Counts  Counts  `json:"counts"`
	Err     string  `json:"err,omitempty"`
}

// Snapshot is the whole dashboard's cached beads state. Available is false when
// the `bd` binary is not found or the feature is disabled.
type Snapshot struct {
	Repos     []RepoSnapshot `json:"repos"`
	Updated   string         `json:"updated"`
	Available bool           `json:"available"`
}
