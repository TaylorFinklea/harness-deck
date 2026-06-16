package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/manifest"
	"github.com/TaylorFinklea/harness-deck/internal/query"
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

// searchRecord bridges a store.Entry to the query.Record interface. Field
// reads come straight from the index; Text() opens the report and computes
// the metadata+body searchable text at most once, memoizing the result so a
// query with several text leaves only pays the report-fetch cost a single
// time.
type searchRecord struct {
	s     *Server
	e     store.Entry
	text  string
	known bool // whether text has been computed yet
}

// Field returns the indexed value for one of the known query field names.
func (r *searchRecord) Field(name string) string {
	switch name {
	case "status":
		return r.e.Status
	case "project":
		return r.e.Project
	case "kind":
		return r.e.Kind
	case "harness":
		return r.e.Harness
	case "title":
		return r.e.Title
	case "agent":
		return r.e.Agent
	case "verdict":
		return r.e.Verdict
	case "created":
		return r.e.Created
	}
	return ""
}

// Text returns the metadata+body searchable text, computing it lazily on the
// first call and reusing it thereafter. This mirrors what scoreEntry searches:
// the metadata fields plus the report body via manifest.BlockText.
func (r *searchRecord) Text() string {
	if r.known {
		return r.text
	}
	r.known = true
	var b strings.Builder
	for _, field := range []string{r.e.Title, r.e.Project, r.e.Kind, r.e.Status, r.e.Harness, r.e.Agent, r.e.Verdict} {
		if field != "" {
			b.WriteString(field)
			b.WriteByte('\n')
		}
	}
	if rep, _, err := r.s.store.Get(r.e.Project, r.e.Run); err == nil && rep != nil {
		b.WriteString(manifest.BlockText(rep))
	}
	r.text = b.String()
	return r.text
}

// handleSearch returns reports matching ?q=<query>. Empty q returns 200 with
// an empty matches array so the frontend's live-search handler can distinguish
// "no query" from "query but no matches". A non-empty q is parsed as a
// structured query (query.Parse); a parse error returns 200 with
// {"matches":[], "error": msg} so the client can keep its last-good results and
// surface the hint without an HTTP failure. Otherwise each non-archived entry
// is bridged to a query.Record and kept when parsed.Match reports true; the
// survivors are then scored/snippeted (when the query carries text terms) or
// ordered newest-first (purely structural queries).
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	w.Header().Set("Content-Type", "application/json")

	if q == "" {
		_, _ = w.Write([]byte(`{"matches":[]}`))
		return
	}

	parsed, err := query.Parse(q)
	if err != nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"matches": []searchHit{},
			"error":   err.Error(),
		})
		return
	}

	now := time.Now()
	hasText := parsed.HasText()
	terms := parsed.TextTerms()

	hits := []searchHit{}
	for _, e := range s.store.Entries() {
		if e.Archived {
			continue
		}
		rec := &searchRecord{s: s, e: e}
		if !parsed.Match(rec, now) {
			continue
		}
		hits = append(hits, scoreSurvivor(rec, hasText, terms))
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		// Tie-break newest first so a stale dupe doesn't bury today's hit.
		// Purely-structural queries score equal, so this is the only ordering.
		return hits[i].Created > hits[j].Created
	})
	if len(hits) > searchMaxResults {
		hits = hits[:searchMaxResults]
	}

	_ = json.NewEncoder(w).Encode(map[string]any{"matches": hits})
}

// scoreSurvivor builds the result row for an entry that already matched the
// query. Scoring exists only to order survivors, never to filter them. When the
// query has text terms, each metadata field containing a term scores 1 and the
// first body-text match scores 5 and supplies the snippet (mirroring the old
// metadata-vs-body weighting). A purely-structural query has no terms, so every
// survivor scores 0 and ordering falls through to newest-first.
func scoreSurvivor(rec *searchRecord, hasText bool, terms []string) searchHit {
	e := rec.e
	out := searchHit{
		Project: e.Project, Run: e.Run, Title: e.Title,
		Kind: e.Kind, Status: e.Status, Created: e.Created,
	}
	if !hasText {
		return out
	}

	// Metadata pass — cheap, against fields already in the index.
	for _, field := range []string{e.Title, e.Project, e.Kind, e.Status, e.Harness, e.Agent, e.Verdict} {
		lower := strings.ToLower(field)
		for _, term := range terms {
			if field != "" && strings.Contains(lower, strings.ToLower(term)) {
				out.Score++
			}
		}
	}

	// Body pass — the first term whose text appears in the body wins the
	// snippet and the body weight. Text() is memoized on rec, so the report is
	// opened at most once across Match + this pass.
	text := rec.Text()
	lowerText := strings.ToLower(text)
	for _, term := range terms {
		idx := strings.Index(lowerText, strings.ToLower(term))
		if idx >= 0 {
			out.Score += 5
			if out.Snippet == "" {
				out.Snippet = snippet(text, idx, len(term))
			}
			break
		}
	}
	return out
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

// searchFieldSchema is one field's autocomplete vocabulary: its name and the
// operators valid on it, in canonical order. It is the JSON view of
// query.Schema(), the single source of truth shared with the parser.
type searchFieldSchema struct {
	Name string   `json:"name"`
	Ops  []string `json:"ops"`
}

// schemaFields adapts query.Schema() (the parser's own field/operator matrix)
// to the JSON shape, so the autocomplete vocabulary and the parser can never
// drift — adding a field to internal/query surfaces it here automatically.
func schemaFields() []searchFieldSchema {
	qs := query.Schema()
	out := make([]searchFieldSchema, 0, len(qs))
	for _, f := range qs {
		out = append(out, searchFieldSchema{Name: f.Name, Ops: f.Ops})
	}
	return out
}

// handleSearchSchema serves the autocomplete vocabulary for the query language:
// the fields with their allowed operators, the distinct values present in the
// index for the enumerable fields, and the relative/ISO hints for `created`.
// It is recomputed per request from the in-memory index (archived excluded);
// the work is cheap because entries are already in memory.
func (s *Server) handleSearchSchema(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	values := map[string][]string{
		// status is the static enum in stable order, not derived from the index.
		"status":  {"draft", "awaiting-review", "answered", "done"},
		"project": distinctValues(s, func(e store.Entry) string { return e.Project }),
		"kind":    distinctValues(s, func(e store.Entry) string { return e.Kind }),
		"harness": distinctValues(s, func(e store.Entry) string { return e.Harness }),
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"fields":        schemaFields(),
		"values":        values,
		"created_hints": []string{"-24h", "-7d", "-2w", "YYYY-MM-DD"},
	})
}

// distinctValues collects the distinct non-empty values of one entry field
// across the non-archived index, sorted. It backs the autocomplete value lists
// for project/kind/harness.
func distinctValues(s *Server, pick func(store.Entry) string) []string {
	seen := map[string]struct{}{}
	for _, e := range s.store.Entries() {
		if e.Archived {
			continue
		}
		v := pick(e)
		if v == "" {
			continue
		}
		seen[v] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
