// Package manifest defines the harness-deck report manifest: the JSON document
// a harness writes to publish a report. A manifest is report-level metadata
// plus an ordered list of typed content blocks. This package owns parsing,
// the block type registry, and validation — it never produces HTML.
package manifest

import (
	"encoding/json"
	"fmt"
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
	Archived bool    `json:"archived,omitempty"`
	Blocks   []Block `json:"blocks"`
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
		return fmt.Errorf("block %q: %w", head.Type, err)
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
// call Report.Validate for that.
func Parse(data []byte) (*Report, error) {
	var r Report
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}
