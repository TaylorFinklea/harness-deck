// Package manifest defines the harness-deck report manifest: the JSON document
// a harness writes to publish a report. A manifest is report-level metadata
// plus an ordered list of typed content blocks. This package owns parsing,
// the block type registry, and validation — it never produces HTML.
package manifest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Schema is the manifest schema identifier expected in a report's "schema" field.
const Schema = "harness-deck/report@1"

// Report is a single harness-deck report manifest.
type Report struct {
	Schema  string `json:"schema"`
	ID      string `json:"id"`
	Project string `json:"project"`
	Harness string `json:"harness"`         // claude-code | pi-mono | opencode | …
	Agent   string `json:"agent,omitempty"` // model identifier
	Title   string `json:"title"`
	Scope   string `json:"scope,omitempty"`   // short prefix highlighted in the banner
	Kind    string `json:"kind,omitempty"`    // audit | progress | idea | ask | …
	Status  string `json:"status"`            // draft | awaiting-review | answered | done
	Created string `json:"created"`           // RFC3339 timestamp
	Verdict string `json:"verdict,omitempty"` // free-text headline conclusion
	Meta    []KV   `json:"meta,omitempty"`    // ordered run metadata (tokens, cost, …)
	// Archived is a soft-delete flag: true hides the report from every
	// default view but leaves report.json + responses.json untouched on
	// disk. Distinct from Status so archiving an awaiting-review report
	// does not lose its open-ask state when restored.
	Archived bool `json:"archived,omitempty"`
	// Live carries optional in-flight telemetry: the harness updates the
	// fields while a run is active so the dashboard shows a pulse, the
	// current step, elapsed time, tokens, etc. The renderer treats Live
	// as "live" only when Updated is within liveWindow of now; older data
	// still displays but as static, not pulsing.
	Live   *LiveStatus `json:"live,omitempty"`
	Blocks []Block     `json:"blocks"`
}

// LiveStatus is the in-flight telemetry attached to an active run. Every
// field except Updated is optional — a harness can publish just the step
// label, or just token count, or any combination. Cost is a string to
// preserve whatever precision the harness chose (cents, mills, whatever)
// without introducing float-rounding surprises.
type LiveStatus struct {
	Updated   string  `json:"updated"`              // RFC3339 — required if Live is set
	Step      string  `json:"step,omitempty"`       // short human description of the current step
	ElapsedMs int64   `json:"elapsed_ms,omitempty"` // milliseconds since the run started
	Tokens    int64   `json:"tokens,omitempty"`     // cumulative token count
	CostUSD   string  `json:"cost_usd,omitempty"`   // free-form dollar string ("0.42", "$1.84")
	Progress  float64 `json:"progress,omitempty"`   // 0..1 progress fraction (optional)
}

// KV is an ordered key/value pair. Manifests use an ordered list rather than a
// map so the renderer shows run metadata in a stable, author-chosen order.
type KV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// BlockBody is the decoded payload of a block. The interface is closed: only
// this package's block structs implement it (kind is unexported).
type BlockBody interface {
	kind() string
	// PanelTitle is the author-supplied panel heading, or "" for the renderer
	// default. Promoted from the embedded blockHead.
	PanelTitle() string
	// PanelPills are optional annotations shown in the panel header.
	PanelPills() []Pill
}

// PanelTitle reports the author-supplied panel heading ("" means use a default).
func (h blockHead) PanelTitle() string { return h.Title }

// PanelPills reports the author-supplied panel-header annotations.
func (h blockHead) PanelPills() []Pill { return h.Pills }

// Block is one content block in a report. Type is always set; Body is the
// decoded payload, or nil when Type is not a known block type. Raw retains the
// original bytes so validation can re-decode strictly and unknown blocks can
// still be inspected.
type Block struct {
	Type string
	Raw  json.RawMessage
	Body BlockBody
}

// registry maps each known block type to a constructor for its body struct.
var registry = map[string]func() BlockBody{
	TypeProse:           func() BlockBody { return &ProseBlock{} },
	TypeMetrics:         func() BlockBody { return &MetricsBlock{} },
	TypeRisks:           func() BlockBody { return &RisksBlock{} },
	TypeDiff:            func() BlockBody { return &DiffBlock{} },
	TypeTimeline:        func() BlockBody { return &TimelineBlock{} },
	TypeCompare:         func() BlockBody { return &CompareBlock{} },
	TypeRecommendations: func() BlockBody { return &RecommendationsBlock{} },
	TypeCallout:         func() BlockBody { return &CalloutBlock{} },
	TypeBarchart:        func() BlockBody { return &BarchartBlock{} },
	TypeTable:           func() BlockBody { return &TableBlock{} },
	TypeHTML:            func() BlockBody { return &HTMLBlock{} },
	TypeAsk:             func() BlockBody { return &AskBlock{} },
	TypeDecision:        func() BlockBody { return &DecisionBlock{} },
	TypeApproval:        func() BlockBody { return &ApprovalBlock{} },
}

// KnownType reports whether t is a registered block type.
func KnownType(t string) bool {
	_, ok := registry[t]
	return ok
}

// Types returns the sorted list of every registered block type. The order is
// deterministic (sorted) so tests that iterate the list produce stable output.
func Types() []string {
	out := make([]string, 0, len(registry))
	for t := range registry {
		out = append(out, t)
	}
	// Insertion order of map keys is random; sort for stability.
	sort.Strings(out)
	return out
}

// BlockPrompt returns the human-readable question text for an interactive
// block, falling back to the block title if no prompt is set. It is the
// canonical helper for building push-notification bodies and fanout messages.
func BlockPrompt(b Block) string {
	switch body := b.Body.(type) {
	case *AskBlock:
		if body.Prompt != "" {
			return body.Prompt
		}
	case *DecisionBlock:
		if body.Prompt != "" {
			return body.Prompt
		}
	case *ApprovalBlock:
		if body.Prompt != "" {
			return body.Prompt
		}
	}
	if b.Body != nil {
		if t := b.Body.PanelTitle(); t != "" {
			return t
		}
	}
	return b.Type
}

// BlockText concatenates every block's plain-text content for full-text search.
// Skips the html block (raw markup is rarely what a user is searching for, and
// the cost of stripping tags isn't worth it). It is the canonical helper for
// building search indexes.
func BlockText(rep *Report) string {
	var b strings.Builder
	for _, blk := range rep.Blocks {
		switch body := blk.Body.(type) {
		case *ProseBlock:
			b.WriteString(body.Markdown)
			b.WriteByte('\n')
		case *RecommendationsBlock:
			for _, item := range body.Items {
				b.WriteString(item.Markdown)
				b.WriteByte('\n')
			}
		case *CalloutBlock:
			b.WriteString(body.Markdown)
			b.WriteByte('\n')
		case *TimelineBlock:
			for _, ev := range body.Events {
				b.WriteString(ev.Markdown)
				b.WriteByte('\n')
			}
		case *AskBlock:
			b.WriteString(body.Prompt)
			b.WriteByte('\n')
			for _, opt := range body.Options {
				b.WriteString(opt)
				b.WriteByte('\n')
			}
		case *DecisionBlock:
			b.WriteString(body.Prompt)
			b.WriteByte('\n')
			b.WriteString(body.A.Title)
			b.WriteByte('\n')
			b.WriteString(body.B.Title)
			b.WriteByte('\n')
		case *ApprovalBlock:
			b.WriteString(body.Prompt)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// InteractiveID returns an interactive block's response id, or "" if the block
// is not interactive (or has no id).
func InteractiveID(b Block) string {
	switch body := b.Body.(type) {
	case *AskBlock:
		return body.ID
	case *DecisionBlock:
		return body.ID
	case *ApprovalBlock:
		return body.ID
	}
	return ""
}

// UnmarshalJSON decodes a block, dispatching on its "type" field. Decoding is
// lenient: unknown fields are ignored and an unknown type leaves Body nil
// rather than erroring, so a stale renderer degrades gracefully. Strict
// checking is the job of Validate.
func (b *Block) UnmarshalJSON(data []byte) error {
	b.Raw = append(json.RawMessage(nil), data...)
	var head struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return fmt.Errorf("block: %w", err)
	}
	b.Type = head.Type
	ctor, ok := registry[head.Type]
	if !ok {
		return nil // unknown type: Body stays nil
	}
	body := ctor()
	if err := json.Unmarshal(data, body); err != nil {
		// Known type, mistyped field: degrade exactly like an unknown type.
		// Body stays nil so the renderer shows the fallback panel, the rest
		// of the report — and its dashboard index entry — survives, and
		// Validate's independent strict pass reports the actual problem.
		return nil
	}
	b.Body = body
	return nil
}

// MarshalJSON emits the block's original bytes.
func (b Block) MarshalJSON() ([]byte, error) {
	if b.Raw != nil {
		return b.Raw, nil
	}
	return []byte("null"), nil
}

// Parse decodes a report manifest from JSON. It does not validate semantics;
// call Report.Validate for that. Parse is lenient on schema version: a higher
// version of the canonical family still parses (unknown blocks degrade to
// fallback panels). A completely different schema family is an error.
func Parse(data []byte) (*Report, error) {
	// Peek at the schema field before full decode so we can reject an alien
	// family without silently ignoring it.
	var head struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, err
	}
	if err := checkSchemaLenient(head.Schema); err != nil {
		return nil, err
	}
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
