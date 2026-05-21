// Package store discovers report manifests on disk and keeps an in-memory
// index of them. Reports are found in two places: the central reports
// directory and the .harness/ directory of each registered project root.
package store

import (
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/manifest"
)

// interactiveTypes are block types that pose a question to the user. Counting
// them gives a cheap "needs you" signal even before Phase 4 wires up responses.
var interactiveTypes = map[string]bool{"ask": true, "decision": true, "approval": true}

// Entry is the indexed summary of one report. The full manifest is loaded
// on demand via Get.
type Entry struct {
	Project  string `json:"project"`
	Run      string `json:"run"`
	Harness  string `json:"harness"`
	Agent    string `json:"agent"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Status   string `json:"status"`
	Created  string `json:"created"`
	Verdict  string `json:"verdict"`
	Source   string `json:"source"` // "central" or "project"
	Blocks   int       `json:"blocks"`
	OpenAsks int       `json:"open_asks"`
	Dir      string    `json:"-"` // run directory
	Path     string    `json:"-"` // report.json path
	ModTime  time.Time `json:"-"` // report.json modification time
}

// Store holds the discovered report index. It is safe for concurrent use.
type Store struct {
	cfg     config.Config
	mu      sync.RWMutex
	entries []Entry
	errs    []string // paths that failed to parse, with the reason
	sig     string   // fingerprint of the indexed files; changes when they do
}

// New returns an empty Store; call Scan to populate it.
func New(cfg config.Config) *Store { return &Store{cfg: cfg} }

// Scan rebuilds the index from disk: the central directory plus every
// registered project's .harness/ directory.
func (s *Store) Scan() {
	var entries []Entry
	var errs []string
	seen := map[string]bool{} // project\x00run — central wins ties (scanned first)

	collect := func(root, source string) {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // unreadable dir/file — skip quietly
			}
			if d.IsDir() {
				if n := d.Name(); n == ".git" || n == "node_modules" {
					return fs.SkipDir
				}
				return nil
			}
			if d.Name() != "report.json" {
				return nil
			}
			e, perr := loadEntry(path, source)
			if perr != nil {
				errs = append(errs, path+": "+perr.Error())
				return nil
			}
			key := e.Project + "\x00" + e.Run
			if seen[key] {
				return nil
			}
			seen[key] = true
			entries = append(entries, e)
			return nil
		})
	}

	collect(config.Expand(s.cfg.CentralDir), "central")
	for _, p := range s.cfg.Projects {
		collect(filepath.Join(config.Expand(p), ".harness"), "project")
	}
	// Newest first. RFC3339 timestamps sort correctly as strings.
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Created > entries[j].Created })

	s.mu.Lock()
	s.entries, s.errs, s.sig = entries, errs, signature(entries)
	s.mu.Unlock()
}

// Signature is a fingerprint of the indexed report files. It changes whenever
// a report is added, removed, or modified — the live-update watcher polls it.
func (s *Store) Signature() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.sig
}

// signature hashes each report's path and modification time.
func signature(entries []Entry) string {
	h := fnv.New64a()
	for _, e := range entries {
		fmt.Fprintf(h, "%s|%d\n", e.Path, e.ModTime.UnixNano())
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// Entries returns a snapshot of the indexed reports, newest first.
func (s *Store) Entries() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]Entry(nil), s.entries...)
}

// Errors returns the list of report files that failed to parse during the
// last scan.
func (s *Store) Errors() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]string(nil), s.errs...)
}

// Get loads and parses the full manifest for one indexed report.
func (s *Store) Get(project, run string) (*manifest.Report, Entry, error) {
	s.mu.RLock()
	var found *Entry
	for i := range s.entries {
		if s.entries[i].Project == project && s.entries[i].Run == run {
			e := s.entries[i]
			found = &e
			break
		}
	}
	s.mu.RUnlock()
	if found == nil {
		return nil, Entry{}, fmt.Errorf("no report %s/%s", project, run)
	}
	data, err := os.ReadFile(found.Path)
	if err != nil {
		return nil, *found, err
	}
	rep, err := manifest.Parse(data)
	return rep, *found, err
}

// loadEntry reads one report.json into an index Entry.
func loadEntry(path, source string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, err
	}
	rep, err := manifest.Parse(data)
	if err != nil {
		return Entry{}, err
	}
	e := Entry{
		Project: rep.Project, Run: rep.ID, Harness: rep.Harness, Agent: rep.Agent,
		Title: rep.Title, Kind: rep.Kind, Status: rep.Status, Created: rep.Created,
		Verdict: rep.Verdict, Source: source, Blocks: len(rep.Blocks),
		Dir: filepath.Dir(path), Path: path,
	}
	if fi, statErr := os.Stat(path); statErr == nil {
		e.ModTime = fi.ModTime()
	}
	for _, b := range rep.Blocks {
		if interactiveTypes[b.Type] {
			e.OpenAsks++
		}
	}
	if e.Project == "" {
		e.Project = "(unknown)"
	}
	if e.Run == "" {
		e.Run = filepath.Base(e.Dir)
	}
	return e, nil
}
