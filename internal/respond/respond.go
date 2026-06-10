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

// Response is one recorded answer to an interactive block.
type Response struct {
	Block string `json:"block"` // the interactive block's id
	Value string `json:"value"` // chosen option / answer text / approval verdict
	Note  string `json:"note,omitempty"`
	At    string `json:"at"` // RFC3339 timestamp
}

// File is the on-disk responses.json for one run.
type File struct {
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
	f.Responses[r.Block] = r

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return f, err
	}
	return f, jsonfile.AtomicWrite(filepath.Join(dir, FileName), append(data, '\n'), 0o644)
}
