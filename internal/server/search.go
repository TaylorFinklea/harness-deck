package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/TaylorFinklea/harness-deck/internal/manifest"
	"github.com/TaylorFinklea/harness-deck/internal/store"
)

// searchMaxResults caps the response so a wide query doesn't ship the
// whole report set back to the browser.
const searchMaxResults = 20

// searchSnippetRadius is how many characters of context to keep on each
// side of the first content match.
const searchSnippetRadius = 60

// searchHit is one result row sent to the frontend.
type searchHit struct {
	Project string `json:"project"`
	Run     string `json:"run"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Status  string `json:"status"`
	Created string `json:"created"`
	// Snippet is content around the first body-text match, "" if the match
	// was only on metadata.
	Snippet string `json:"snippet"`
	// Score determines result order. Metadata hits weigh less than body
	// hits — most users typing a query are looking for content, not the
	// title they already remember.
	Score int `json:"score"`
}

// handleSearch returns reports matching ?q=<query>. Empty q returns 200
// with an empty matches array so the frontend's live-search handler can
// distinguish "no query" from "query but no matches".
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "application/json")

	if q == "" {
		_, _ = w.Write([]byte(`{"matches":[]}`))
		return
	}
	needle := strings.ToLower(q)

	hits := []searchHit{}
	for _, e := range s.store.Entries() {
		if e.Archived {
			continue
		}
		hit := scoreEntry(s, e, needle)
		if hit.Score > 0 {
			hits = append(hits, hit)
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		// Tie-break newest first so a stale dupe doesn't bury today's hit.
		return hits[i].Created > hits[j].Created
	})
	if len(hits) > searchMaxResults {
		hits = hits[:searchMaxResults]
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"matches": hits})
}

// scoreEntry adds points for each searchable surface of e that contains
// needle. Body-content matches are 5x metadata matches, and capture a
// snippet for display. Returns a zero-Score hit when nothing matched.
func scoreEntry(s *Server, e store.Entry, needle string) searchHit {
	out := searchHit{
		Project: e.Project, Run: e.Run, Title: e.Title,
		Kind: e.Kind, Status: e.Status, Created: e.Created,
	}

	// Metadata pass — cheap, runs against fields already in the index.
	for _, field := range []string{e.Title, e.Project, e.Kind, e.Status, e.Harness, e.Agent, e.Verdict} {
		if field != "" && strings.Contains(strings.ToLower(field), needle) {
			out.Score++
		}
	}

	// Body pass — open the report, walk blocks, search their text. Only
	// run when metadata didn't already match strongly, to keep wide
	// queries cheap. (A title-only query still runs body pass; the limit
	// is the small "report fetch cost per match".)
	rep, _, err := s.store.Get(e.Project, e.Run)
	if err == nil && rep != nil {
		text := blockText(rep)
		idx := strings.Index(strings.ToLower(text), needle)
		if idx >= 0 {
			out.Score += 5
			out.Snippet = snippet(text, idx, len(needle))
		}
	}
	return out
}

// blockText concatenates every block's plain-text content for search.
// Skips the html block (raw markup is rarely what a user is searching
// for, and the cost of stripping tags isn't worth it).
func blockText(rep *manifest.Report) string {
	var b strings.Builder
	for _, blk := range rep.Blocks {
		switch body := blk.Body.(type) {
		case *manifest.ProseBlock:
			b.WriteString(body.Markdown)
			b.WriteByte('\n')
		case *manifest.RecommendationsBlock:
			for _, item := range body.Items {
				b.WriteString(item.Markdown)
				b.WriteByte('\n')
			}
		case *manifest.CalloutBlock:
			b.WriteString(body.Markdown)
			b.WriteByte('\n')
		case *manifest.TimelineBlock:
			for _, ev := range body.Events {
				b.WriteString(ev.Markdown)
				b.WriteByte('\n')
			}
		case *manifest.AskBlock:
			b.WriteString(body.Prompt)
			b.WriteByte('\n')
			for _, opt := range body.Options {
				b.WriteString(opt)
				b.WriteByte('\n')
			}
		case *manifest.DecisionBlock:
			b.WriteString(body.Prompt)
			b.WriteByte('\n')
			b.WriteString(body.A.Title)
			b.WriteByte('\n')
			b.WriteString(body.B.Title)
			b.WriteByte('\n')
		case *manifest.ApprovalBlock:
			b.WriteString(body.Prompt)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// snippet returns ~120 chars of context around the match, with "…"
// markers when truncated, and the matched range wrapped in [[ ]] so the
// frontend can highlight it. Newlines are collapsed to spaces.
func snippet(text string, idx, matchLen int) string {
	start := idx - searchSnippetRadius
	if start < 0 {
		start = 0
	}
	end := idx + matchLen + searchSnippetRadius
	if end > len(text) {
		end = len(text)
	}
	leading := ""
	if start > 0 {
		leading = "…"
	}
	trailing := ""
	if end < len(text) {
		trailing = "…"
	}
	before := text[start:idx]
	match := text[idx : idx+matchLen]
	after := text[idx+matchLen : end]
	collapsed := strings.ReplaceAll(leading+before+"[["+match+"]]"+after+trailing, "\n", " ")
	return strings.TrimSpace(collapsed)
}
