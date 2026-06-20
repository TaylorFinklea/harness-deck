package manifest

// Block type identifiers. These are the values of a block's "type" field.
const (
	TypeProse           = "prose"
	TypeMetrics         = "metrics"
	TypeRisks           = "risks"
	TypeDiff            = "diff"
	TypeTimeline        = "timeline"
	TypeCompare         = "compare"
	TypeRecommendations = "recommendations"
	TypeCallout         = "callout"
	TypeBarchart        = "barchart"
	TypeTable           = "table"
	TypeHTML            = "html"
	TypeCardGrid        = "card-grid"
	// Interactive blocks — these pose a question the user answers in the
	// dashboard; the answer is written to responses.json.
	TypeAsk      = "ask"
	TypeDecision = "decision"
	TypeApproval = "approval"
)

// InteractiveTypes is the set of block types that record a user response.
var InteractiveTypes = map[string]bool{
	TypeAsk: true, TypeDecision: true, TypeApproval: true,
}

// blockHead holds fields common to every block. It is embedded in each body
// struct so a strict JSON decode accepts the shared "type" and "pills" keys.
type blockHead struct {
	Type  string `json:"type"`
	Title string `json:"title,omitempty"` // panel heading; renderer supplies a default if empty
	Pills []Pill `json:"pills,omitempty"` // optional right-aligned annotations in the panel head
}

// Pill is a small annotation shown in a panel header (e.g. "decision needed").
type Pill struct {
	Text  string `json:"text"`
	Level string `json:"level,omitempty"` // "" | ok | warn | err
}

// ProseBlock renders a panel of Markdown prose.
type ProseBlock struct {
	blockHead
	Markdown string `json:"markdown"`
}

func (ProseBlock) kind() string { return TypeProse }

// Metric is one cell of the metric grid.
type Metric struct {
	Label string    `json:"label"`
	Value string    `json:"value"`
	Unit  string    `json:"unit,omitempty"`
	Delta string    `json:"delta,omitempty"`
	Trend string    `json:"trend,omitempty"` // "" | pos | neg
	Spark []float64 `json:"spark,omitempty"` // sparkline points, rendered as an SVG path
	Color string    `json:"color,omitempty"` // "" | ok | warn | err | info
}

// Bar is one row of a horizontal bar breakdown.
type Bar struct {
	Label string  `json:"label"`
	Pct   float64 `json:"pct"`             // 0..100
	Color string  `json:"color,omitempty"` // "" | cyan | magenta | yellow | green | red
}

// MetricsBlock renders a metric grid, optionally with a bar breakdown below it.
type MetricsBlock struct {
	blockHead
	Metrics []Metric `json:"metrics"`
	Bars    []Bar    `json:"bars,omitempty"`
}

func (MetricsBlock) kind() string { return TypeMetrics }

// Risk is one row of the risk register.
type Risk struct {
	Severity string  `json:"severity"` // crit | high | med | low
	Label    string  `json:"label"`
	Pct      float64 `json:"pct"` // 0..100
}

// RisksBlock renders a risk register, optionally followed by callouts.
type RisksBlock struct {
	blockHead
	Risks    []Risk    `json:"risks"`
	Callouts []Callout `json:"callouts,omitempty"`
}

func (RisksBlock) kind() string { return TypeRisks }

// Callout is an info/warn/err aside. Used standalone and inside a risks block.
type Callout struct {
	Level    string `json:"level"` // info | warn | err
	Tag      string `json:"tag,omitempty"`
	Markdown string `json:"markdown"`
}

// CalloutBlock renders a single standalone callout.
type CalloutBlock struct {
	blockHead
	Callout
}

func (CalloutBlock) kind() string { return TypeCallout }

// DiffLine is one line of a diff hunk.
type DiffLine struct {
	Kind string `json:"kind"`          // ctx | add | del | hunk
	Old  string `json:"old,omitempty"` // old line number (blank for add/hunk)
	New  string `json:"new,omitempty"` // new line number (blank for del/hunk)
	Text string `json:"text"`
}

// DiffFile is one file's worth of diff lines.
type DiffFile struct {
	Path  string     `json:"path"`
	Lang  string     `json:"lang,omitempty"`
	Lines []DiffLine `json:"lines"`
}

// DiffBlock renders one or more file diffs. Add/remove counts are derived.
type DiffBlock struct {
	blockHead
	Files []DiffFile `json:"files"`
}

func (DiffBlock) kind() string { return TypeDiff }

// Event is one entry on a timeline.
type Event struct {
	Time     string `json:"time"`
	Kind     string `json:"kind,omitempty"` // "" | ok | crit
	Markdown string `json:"markdown"`
}

// TimelineBlock renders a vertical event timeline.
type TimelineBlock struct {
	blockHead
	Events []Event `json:"events"`
}

func (TimelineBlock) kind() string { return TypeTimeline }

// CompareItem is one bullet in a comparison column.
type CompareItem struct {
	Kind string `json:"kind"` // pro | con | neu
	Text string `json:"text"`
}

// CompareSide is one column of an A/B comparison.
type CompareSide struct {
	Tag   string        `json:"tag"` // short label, e.g. "A"
	Title string        `json:"title"`
	Items []CompareItem `json:"items"`
}

// CompareBlock renders a two-column A/B comparison.
type CompareBlock struct {
	blockHead
	A CompareSide `json:"a"`
	B CompareSide `json:"b"`
}

func (CompareBlock) kind() string { return TypeCompare }

// Recommendation is one numbered recommendation row.
type Recommendation struct {
	ID       string `json:"id,omitempty"`
	Owner    string `json:"owner,omitempty"` // free text; eng/dba/ops get accent colors
	Markdown string `json:"markdown"`
}

// RecommendationsBlock renders a list of numbered recommendations.
type RecommendationsBlock struct {
	blockHead
	Items []Recommendation `json:"items"`
}

func (RecommendationsBlock) kind() string { return TypeRecommendations }

// BarchartBlock renders a standalone horizontal bar chart.
type BarchartBlock struct {
	blockHead
	Bars []Bar `json:"bars"`
}

func (BarchartBlock) kind() string { return TypeBarchart }

// TableBlock renders a simple columnar table.
type TableBlock struct {
	blockHead
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

func (TableBlock) kind() string { return TypeTable }

// HTMLBlock is the escape hatch: raw HTML rendered inside the standard panel
// chrome, with the theme's CSS variables available. Use when the typed block
// vocabulary does not cover what the report needs.
type HTMLBlock struct {
	blockHead
	HTML string `json:"html"`
}

func (HTMLBlock) kind() string { return TypeHTML }

// Card is one cell in a card-grid. Title is required; Markdown and Pills are
// optional additional content shown in the card body.
type Card struct {
	Title    string `json:"title"`
	Markdown string `json:"markdown,omitempty"`
	Pills    []Pill `json:"pills,omitempty"`
}

// CardGridBlock renders a responsive grid of titled cards.
type CardGridBlock struct {
	blockHead
	Cards []Card `json:"cards"`
}

func (CardGridBlock) kind() string { return TypeCardGrid }

// AskBlock poses a question to the user. ID keys the response in responses.json.
type AskBlock struct {
	blockHead
	ID      string   `json:"id"`
	Prompt  string   `json:"prompt"`            // the question, Markdown
	Mode    string   `json:"mode,omitempty"`    // choice | yesno | text | multi (default: choice if options, else text)
	Options []string `json:"options,omitempty"` // for mode=choice
}

func (AskBlock) kind() string { return TypeAsk }

// ResolvedMode returns the effective input mode, applying the default.
func (a AskBlock) ResolvedMode() string {
	if a.Mode != "" {
		return a.Mode
	}
	if len(a.Options) > 0 {
		return "choice"
	}
	return "text"
}

// DecisionBlock asks the user to pick between two paths. It reuses the A/B
// comparison layout and records the chosen tag.
type DecisionBlock struct {
	blockHead
	ID     string      `json:"id"`
	Prompt string      `json:"prompt,omitempty"`
	A      CompareSide `json:"a"`
	B      CompareSide `json:"b"`
}

func (DecisionBlock) kind() string { return TypeDecision }

// ApprovalBlock asks the user to sign off on something.
type ApprovalBlock struct {
	blockHead
	ID     string `json:"id"`
	Prompt string `json:"prompt,omitempty"`
}

func (ApprovalBlock) kind() string { return TypeApproval }
