// Package store discovers report manifests on disk and keeps an in-memory
// index of them. Reports are found in two places: the central reports
// directory and the .harness/ directory of each project root Scan is given.
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
	"github.com/TaylorFinklea/harness-deck/internal/respond"
)

// Entry is the indexed summary of one report. The full manifest is loaded
// on demand via Get.
type Entry struct {
	Project     string    `json:"project"`
	Run         string    `json:"run"`
	Harness     string    `json:"harness"`
	Agent       string    `json:"agent"`
	Title       string    `json:"title"`
	Kind        string    `json:"kind"`
	Status      string    `json:"status"`
	Created     string    `json:"created"`
	Verdict     string    `json:"verdict"`
	Source      string    `json:"source"` // "central" or "project"
	Blocks      int       `json:"blocks"`
	OpenAsks    int       `json:"open_asks"`
	Archived    bool      `json:"archived"` // soft-deleted; hidden from default views
	Dir         string    `json:"-"`        // run directory
	Path        string    `json:"-"`        // report.json path
	ModTime     time.Time `json:"-"`        // report.json modification time
	RespModTime time.Time `json:"-"`        // responses.json modification time, zero if absent
	// Live carries in-flight telemetry from the manifest's optional
	// `live` field. The index keeps it shallow so inbox-rendering decisions
	// (pulse vs. static, "X in flight" counts) don't need to re-read the
	// full manifest. nil means the report has no live block.
	Live *manifest.LiveStatus `json:"live,omitempty"`
}

// Sig returns a stable per-entry fingerprint. Changes whenever the
// report.json or its sibling responses.json is rewritten — which is the
// trigger for the report page's live-reload behavior.
func (e Entry) Sig() string {
	return fmt.Sprintf("%d-%d-%t", e.ModTime.UnixNano(), e.RespModTime.UnixNano(), e.Archived)
}

// Store holds the discovered report index. It is safe for concurrent use.
type Store struct {
	cfg     config.Config
	scanMu  sync.Mutex // serializes Scan so walk+commit is atomic across callers
	mu      sync.RWMutex
	entries []Entry
	errs    []string // paths that failed to parse, with the reason
	sig     string   // fingerprint of the indexed files; changes when they do
}

// New returns an empty Store; call Scan to populate it.
func New(cfg config.Config) *Store { return &Store{cfg: cfg} }

// Scan rebuilds the index from disk: the central directory plus the
// .harness/ directory of each project root it is given. The caller decides
// which projects count — typically the enabled set from the projects
// package.
func (s *Store) Scan(projectRoots []string) {
	// Serialize scans so a slower/partial walk can never commit over a
	// complete one (the walk runs unlocked; only the commit takes s.mu).
	s.scanMu.Lock()
	defer s.scanMu.Unlock()

	var entries []Entry
	var errs []string
	seen := map[string]string{} // project\x00run -> first-seen Path; central wins ties (scanned first)

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
			e, warn, perr := loadEntry(path, source)
			if perr != nil {
				errs = append(errs, path+": "+perr.Error())
				return nil
			}
			if warn != "" {
				// Soft problem (e.g. corrupt responses.json): the report
				// stays indexed, but the issue is visible instead of
				// silently flipping answered asks back to open.
				errs = append(errs, warn)
			}
			key := e.Project + "\x00" + e.Run
			if first, ok := seen[key]; ok {
				if first != e.Path {
					// Two reports claim the same (project,run) from
					// different files. The first wins; surface the
					// collision instead of silently shadowing.
					errs = append(errs, fmt.Sprintf("%s: duplicate (project,run) %s/%s already indexed from %s", e.Path, e.Project, e.Run, first))
				}
				return nil
			}
			seen[key] = e.Path
			entries = append(entries, e)
			return nil
		})
	}

	collect(config.Expand(s.cfg.CentralDir), "central")
	for _, p := range projectRoots {
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

// signature hashes each report's path, modification time, and the mtime
// of its responses.json sibling. Including responses.json means the
// watcher fires on cross-device answers, which the live-reload behavior
// on the report page relies on.
func signature(entries []Entry) string {
	h := fnv.New64a()
	for _, e := range entries {
		fmt.Fprintf(h, "%s|%d|%d|%t\n", e.Path, e.ModTime.UnixNano(), e.RespModTime.UnixNano(), e.Archived)
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

// loadEntry reads one report.json into an index Entry. The warn return
// carries soft problems that should surface in scan errors without
// dropping the report from the index.
func loadEntry(path, source string) (Entry, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, "", err
	}
	rep, err := manifest.Parse(data)
	if err != nil {
		return Entry{}, "", err
	}
	e := Entry{
		Project: rep.Project, Run: rep.ID, Harness: rep.Harness, Agent: rep.Agent,
		Title: rep.Title, Kind: rep.Kind, Status: rep.Status, Created: rep.Created,
		Verdict: rep.Verdict, Source: source, Blocks: len(rep.Blocks),
		Archived: rep.Archived, Live: rep.Live,
		Dir: filepath.Dir(path), Path: path,
	}
	if fi, statErr := os.Stat(path); statErr == nil {
		e.ModTime = fi.ModTime()
	}
	if fi, statErr := os.Stat(filepath.Join(e.Dir, "responses.json")); statErr == nil {
		e.RespModTime = fi.ModTime()
	}
	// OpenAsks is the count of interactive blocks not yet answered in
	// responses.json — the "needs you" signal for the inbox. A corrupt
	// responses.json can't tell us what was answered, so every ask counts
	// as open — but the problem is reported, not swallowed.
	warn := ""
	answers, aerr := respond.Load(e.Dir)
	if aerr != nil {
		warn = filepath.Join(e.Dir, respond.FileName) + ": " + aerr.Error()
	}
	for _, b := range rep.Blocks {
		if !manifest.InteractiveTypes[b.Type] {
			continue
		}
		if id := manifest.InteractiveID(b); id != "" {
			if _, answered := answers.Responses[id]; answered {
				continue
			}
		}
		e.OpenAsks++
	}
	// A draft report is "not ready for the user yet" (the status lifecycle in
	// docs/PUBLISHING.md), so its asks must not surface as open asks or fire a
	// push — a freshly scaffolded `new --template decision` shouldn't notify
	// for placeholder content. The asks still render and stay answerable on the
	// report page; they just don't count until the author flips status off
	// draft. (Push and the inbox/projects/MCP counters all key off OpenAsks.)
	if e.Status == "draft" {
		e.OpenAsks = 0
	}
	if e.Project == "" {
		e.Project = "(unknown)"
	}
	if e.Run == "" {
		e.Run = filepath.Base(e.Dir)
	}
	return e, warn, nil
}
