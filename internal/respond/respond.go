// Package respond manages responses.json — the file harness-deck writes into a
// run directory to record the user's answers, decisions, and approvals so the
// harness can pick them up on its next session.
package respond

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/jsonfile"
)

// recordMu serializes Record's read-modify-write so two concurrent answers
// (phone + desktop, two tabs) can't load the same snapshot and erase each
// other. A single process-wide mutex is plenty: answers are human-paced.
var recordMu sync.Mutex

// FileName is the response file written alongside a report.json.
const FileName = "responses.json"

// responsesVersion is the current schema version stamped on File.Version.
const responsesVersion = 1

// maxPrior is the maximum number of superseded answers retained per block.
const maxPrior = 20

// PriorAnswer is one superseded answer, kept in Response.Prior (newest-first).
// It intentionally has no Prior field of its own — history entries never nest.
type PriorAnswer struct {
	Value  string   `json:"value"`
	Values []string `json:"values,omitempty"`
	Note   string   `json:"note,omitempty"`
	At     string   `json:"at"`
}

// Response is one recorded answer to an interactive block.
type Response struct {
	Block  string        `json:"block"`            // the interactive block's id
	Value  string        `json:"value"`            // chosen option / answer text / approval verdict; for multi, the joined selection
	Values []string      `json:"values,omitempty"` // for mode=multi: the individual selected values
	Note   string        `json:"note,omitempty"`
	At     string        `json:"at"`              // RFC3339 timestamp
	Prior  []PriorAnswer `json:"prior,omitempty"` // superseded answers, newest-first, capped at maxPrior
}

// File is the on-disk responses.json for one run.
type File struct {
	Version   int                 `json:"version"`
	Run       string              `json:"run"`
	Project   string              `json:"project"`
	Updated   string              `json:"updated"`
	Responses map[string]Response `json:"responses"`
}

// Load reads responses.json from a run directory. A missing file is not an
// error — it returns an empty File ready to record into.
func Load(dir string) (File, error) {
	f := File{Responses: map[string]Response{}}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return f, err
	}
	if f.Responses == nil {
		f.Responses = map[string]Response{}
	}
	return f, nil
}

// Record merges one response into a run directory's responses.json, creating
// the file if needed, and returns the updated File. The write is atomic
// (unique temp + rename) — this file holds the user's answers, the one
// artifact a crash mid-write must never truncate.
func Record(dir, project, run string, r Response) (File, error) {
	recordMu.Lock()
	defer recordMu.Unlock()
	f, err := Load(dir)
	if err != nil {
		return f, err
	}
	if r.At == "" {
		r.At = time.Now().UTC().Format(time.RFC3339)
	}
	f.Run, f.Project, f.Updated = run, project, r.At

	// Build prior chain: prepend the old answer (if any) before overwriting.
	if old, ok := f.Responses[r.Block]; ok {
		r.Prior = append([]PriorAnswer{{Value: old.Value, Values: old.Values, Note: old.Note, At: old.At}}, old.Prior...)
		if len(r.Prior) > maxPrior {
			r.Prior = r.Prior[:maxPrior]
		}
	}

	f.Version = responsesVersion
	f.Responses[r.Block] = r

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return f, err
	}
	return f, jsonfile.AtomicWrite(filepath.Join(dir, FileName), append(data, '\n'), 0o644)
}
