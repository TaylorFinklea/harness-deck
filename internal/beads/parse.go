package beads

import "encoding/json"

// parseIssues decodes a `bd … --json` array (ready / list / show).
func parseIssues(b []byte) ([]Issue, error) {
	var xs []Issue
	if err := json.Unmarshal(b, &xs); err != nil {
		return nil, err
	}
	return xs, nil
}

// parseBlocked decodes `bd blocked --json`; items carry blocked_by[], so the
// Issue shape is reused.
func parseBlocked(b []byte) ([]Issue, error) { return parseIssues(b) }

// parseStatus decodes the {summary:{…}} envelope from `bd status --json`.
func parseStatus(b []byte) (Counts, error) {
	var s struct {
		Summary struct {
			Open       int `json:"open_issues"`
			Ready      int `json:"ready_issues"`
			Blocked    int `json:"blocked_issues"`
			InProgress int `json:"in_progress_issues"`
			Total      int `json:"total_issues"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return Counts{}, err
	}
	return Counts{
		Open:       s.Summary.Open,
		Ready:      s.Summary.Ready,
		Blocked:    s.Summary.Blocked,
		InProgress: s.Summary.InProgress,
		Total:      s.Summary.Total,
	}, nil
}

// deriveEdges builds the dependency graph from structured data (not by parsing
// mermaid): open blocking relationships from blocked[].blocked_by, and
// parent-child links from all[].parent. Edges are deduped.
func deriveEdges(all, blocked []Issue) []Edge {
	var edges []Edge
	seen := map[string]bool{}
	add := func(from, to, kind string) {
		if from == "" || to == "" {
			return
		}
		k := kind + "|" + from + "|" + to
		if seen[k] {
			return
		}
		seen[k] = true
		edges = append(edges, Edge{From: from, To: to, Kind: kind})
	}
	for _, b := range blocked {
		for _, blocker := range b.BlockedBy {
			add(blocker, b.ID, "blocks")
		}
	}
	for _, i := range all {
		if i.Parent != "" {
			add(i.Parent, i.ID, "parent")
		}
	}
	return edges
}
