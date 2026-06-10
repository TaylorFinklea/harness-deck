// Package projects discovers project roots under configured scan roots and
// tracks which ones the user keeps visible. Discovery is depth-1: a direct
// child of a scan root that contains a .docs/ai directory is a project.
// Enabled/disabled state lives in an app-owned JSON file (projects.json);
// newly discovered projects are enabled by default, so the user unchecks to
// hide rather than checks to show.
package projects

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/TaylorFinklea/harness-deck/internal/config"
	"github.com/TaylorFinklea/harness-deck/internal/jsonfile"
)

// Project is one discovered project root.
type Project struct {
	Name    string `json:"name"`    // directory basename
	Path    string `json:"path"`    // absolute path to the project root
	Enabled bool   `json:"enabled"` // false when the user has hidden it
}

// Manager discovers projects and persists which ones are hidden.
type Manager struct {
	scanRoots []string
	explicit  []string
	statePath string
	mu        sync.Mutex
}

// StatePath is the default location of the project-state file. It sits
// beside the config file, so the HARNESS_DECK_CONFIG override keeps state
// and config together (handy for the fixture-based manual test setup).
func StatePath() string {
	if c := os.Getenv("HARNESS_DECK_CONFIG"); c != "" {
		return filepath.Join(filepath.Dir(c), "projects.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "projects.json"
	}
	return filepath.Join(home, ".config", "harness-deck", "projects.json")
}

// NewManager builds a Manager. scanRoots are directories scanned depth-1 for
// projects; explicit are project roots included unconditionally; statePath is
// the JSON file recording hidden projects.
func NewManager(scanRoots, explicit []string, statePath string) *Manager {
	return &Manager{scanRoots: scanRoots, explicit: explicit, statePath: statePath}
}

// Discovered returns every project found, sorted by name, with Enabled
// reflecting the persisted hidden set.
func (m *Manager) Discovered() []Project {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.discovered()
}

// discovered is Discovered without locking, for callers already holding mu.
// Projects listed in the saved order come first (in that order); anything not
// in the order is appended alpha-sorted, so a fresh install keeps its
// alphabetical default and newly discovered projects don't jump the line.
func (m *Manager) discovered() []Project {
	disabled, order := m.loadState()
	all := discover(m.scanRoots, m.explicit)
	byName := make(map[string]Project, len(all))
	for _, p := range all {
		byName[p.Name] = p
	}
	out := make([]Project, 0, len(all))
	consumed := make(map[string]bool, len(order))
	for _, name := range order {
		p, ok := byName[name]
		if !ok || consumed[name] {
			continue
		}
		p.Enabled = !disabled[name]
		out = append(out, p)
		consumed[name] = true
	}
	for _, p := range all {
		if consumed[p.Name] {
			continue
		}
		p.Enabled = !disabled[p.Name]
		out = append(out, p)
	}
	return out
}

// Enabled returns only the projects the user has left visible.
func (m *Manager) Enabled() []Project {
	out := []Project{}
	for _, p := range m.Discovered() {
		if p.Enabled {
			out = append(out, p)
		}
	}
	return out
}

// Toggle flips the visibility of a discovered project and persists the
// change. It returns an error if name is not a discovered project.
func (m *Manager) Toggle(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	for _, p := range discover(m.scanRoots, m.explicit) {
		if p.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("projects: unknown project %q", name)
	}
	disabled, order := m.loadState()
	if disabled[name] {
		delete(disabled, name)
	} else {
		disabled[name] = true
	}
	return m.saveState(disabled, order)
}

// docNames are the .docs/ai files surfaced in the projects view, also
// folded into Fingerprint so edits to them trigger a watcher refresh.
var docNames = []string{"roadmap.md", "current-state.md"}

// Fingerprint is a digest of discovered projects, their visibility, and the
// size and mtime of each project's .docs/ai docs. The watcher compares it
// between polls to decide when to push an SSE refresh.
func (m *Manager) Fingerprint() string {
	h := fnv.New64a()
	for _, p := range m.Discovered() {
		fmt.Fprintf(h, "%s\x00%t\x00", p.Name, p.Enabled)
		for _, doc := range docNames {
			if fi, err := os.Stat(filepath.Join(p.Path, ".docs", "ai", doc)); err == nil {
				fmt.Fprintf(h, "%s\x00%d\x00%d\x00", doc, fi.Size(), fi.ModTime().UnixNano())
			}
		}
	}
	return strconv.FormatUint(h.Sum64(), 16)
}

// Reorder records the user's preferred display order for discovered projects
// and persists it. Every name must match a currently discovered project so
// stale browser state can't smuggle ghost entries into the order. The hidden
// set is preserved untouched — visibility and order are independent.
func (m *Manager) Reorder(names []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	known := map[string]bool{}
	for _, p := range discover(m.scanRoots, m.explicit) {
		known[p.Name] = true
	}
	for _, n := range names {
		if !known[n] {
			return fmt.Errorf("projects: unknown project %q", n)
		}
	}
	disabled, _ := m.loadState()
	return m.saveState(disabled, names)
}

// stateFile is the on-disk shape of projects.json: the hidden names and the
// user's preferred display order. Recording only the exceptions means a
// newly discovered project is visible by default; a name no longer on disk
// drops out of both sets harmlessly.
type stateFile struct {
	Disabled []string `json:"disabled"`
	Order    []string `json:"order"`
}

// loadState reads the hidden set and the saved order. A missing or corrupt
// file yields empty values — harness-deck degrades to "everything visible,
// alphabetical" rather than failing.
func (m *Manager) loadState() (map[string]bool, []string) {
	set := map[string]bool{}
	data, err := os.ReadFile(m.statePath)
	if err != nil {
		return set, nil
	}
	var sf stateFile
	if json.Unmarshal(data, &sf) != nil {
		return set, nil
	}
	for _, n := range sf.Disabled {
		set[n] = true
	}
	return set, sf.Order
}

// saveState writes the hidden set and the saved order atomically (temp file +
// rename) so a crash mid-write cannot truncate projects.json.
func (m *Manager) saveState(set map[string]bool, order []string) error {
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	data, err := json.MarshalIndent(stateFile{Disabled: names, Order: order}, "", "  ")
	if err != nil {
		return err
	}
	return jsonfile.AtomicWrite(m.statePath, append(data, '\n'), 0o644)
}

// discover returns project roots: depth-1 children of each scan root that
// hold a .docs/ai directory, plus every explicit root. Results are deduped by
// name (first wins) and sorted. Every Project comes back enabled.
func discover(scanRoots, explicit []string) []Project {
	byName := map[string]Project{}
	add := func(path string) {
		abs, err := filepath.Abs(config.Expand(path))
		if err != nil {
			return
		}
		name := filepath.Base(abs)
		if _, seen := byName[name]; seen {
			return
		}
		byName[name] = Project{Name: name, Path: abs, Enabled: true}
	}
	for _, root := range scanRoots {
		dir := config.Expand(root)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			child := filepath.Join(dir, e.Name())
			if fi, err := os.Stat(filepath.Join(child, ".docs", "ai")); err == nil && fi.IsDir() {
				add(child)
			}
		}
	}
	for _, p := range explicit {
		add(p)
	}
	out := make([]Project, 0, len(byName))
	for _, p := range byName {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
