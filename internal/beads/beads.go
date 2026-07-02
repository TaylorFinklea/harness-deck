package beads

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// callTimeout bounds every bd invocation so a wedged repo can't stall a refresh.
const callTimeout = 10 * time.Second

// Client shells the resolved bd binary. All reads go through it; the .beads/
// directory is never touched directly. writeMu serializes mutations because
// bd's embedded Dolt is single-writer per repo; reads stay unlocked.
type Client struct {
	bin     string
	writeMu sync.Mutex
}

// New resolves the bd binary. ok is false when bd is not installed, in which
// case the beads feature stays dark (graceful degradation).
func New() (*Client, bool) {
	fb := []string{"/opt/homebrew/bin/bd", "/usr/local/bin/bd"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		fb = append(fb, filepath.Join(home, ".local", "bin", "bd"))
	}
	bin, ok := resolveBin("bd", fb)
	if !ok {
		return nil, false
	}
	return &Client{bin: bin}, true
}

// resolveBin finds an executable: $PATH first, then the given fallback paths.
// The dashboard often runs under launchd/systemd with a minimal PATH that omits
// Homebrew and ~/.local/bin, so LookPath alone is not enough.
func resolveBin(name string, fallbacks []string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil {
		return p, true
	}
	for _, p := range fallbacks {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p, true
		}
	}
	return "", false
}

// flagLike reports whether s would be parsed as a flag by bd's non-POSIX CLI
// (any '-'-leading token). Used to reject hostile drill-in ids at the boundary.
func flagLike(s string) bool { return strings.HasPrefix(s, "-") }

// ValidID reports whether id is a safe bd issue id to pass as an argv value:
// non-empty, not flag-like, and limited to the bd id charset [A-Za-z0-9._-].
// Handlers use it to reject a hostile path param with 400 before shelling bd.
func ValidID(id string) bool {
	if id == "" || flagLike(id) {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}

// run executes `bd -C <root> <args...>` and returns stdout. Stdin is left nil
// (child gets /dev/null) so a bd that probes the terminal can't hang.
func (c *Client) run(ctx context.Context, root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	full := append([]string{"-C", root}, args...)
	cmd := exec.CommandContext(ctx, c.bin, full...)
	cmd.Stdin = nil
	return cmd.Output()
}

// Ready returns the priority-sorted, dependency-clear queue.
func (c *Client) Ready(ctx context.Context, root string) ([]Issue, error) {
	b, err := c.run(ctx, root, "ready", "--json")
	if err != nil {
		return nil, err
	}
	return parseIssues(b)
}

// Blocked returns open issues with an open blocker (blocked_by populated).
func (c *Client) Blocked(ctx context.Context, root string) ([]Issue, error) {
	b, err := c.run(ctx, root, "blocked", "--json")
	if err != nil {
		return nil, err
	}
	return parseBlocked(b)
}

// List returns all non-closed issues (open, in_progress, blocked, deferred) —
// the source of graph node attributes + parent links. `bd list` returns every
// issue including closed; we drop closed so the graph doesn't sprout resolved
// nodes, but keep in_progress (a claimed issue must still render with its real
// title/priority, not as a stub).
func (c *Client) List(ctx context.Context, root string) ([]Issue, error) {
	b, err := c.run(ctx, root, "list", "--json")
	if err != nil {
		return nil, err
	}
	xs, err := parseIssues(b)
	if err != nil {
		return nil, err
	}
	return nonClosed(xs), nil
}

// nonClosed keeps every issue whose status is not "closed".
func nonClosed(xs []Issue) []Issue {
	out := make([]Issue, 0, len(xs))
	for _, i := range xs {
		if i.Status != "closed" {
			out = append(out, i)
		}
	}
	return out
}

// Status returns the per-repo counts summary.
func (c *Client) Status(ctx context.Context, root string) (Counts, error) {
	b, err := c.run(ctx, root, "status", "--json")
	if err != nil {
		return Counts{}, err
	}
	return parseStatus(b)
}

// Show returns one issue's fields (no edges/comments — fetch those separately).
func (c *Client) Show(ctx context.Context, root, id string) (Issue, error) {
	if flagLike(id) {
		return Issue{}, os.ErrInvalid
	}
	b, err := c.run(ctx, root, "show", id, "--json")
	if err != nil {
		return Issue{}, err
	}
	xs, err := parseIssues(b)
	if err != nil {
		return Issue{}, err
	}
	if len(xs) == 0 {
		return Issue{}, os.ErrNotExist
	}
	return xs[0], nil
}

// DepList returns the textual list of this issue's blockers.
func (c *Client) DepList(ctx context.Context, root, id string) (string, error) {
	if flagLike(id) {
		return "", os.ErrInvalid
	}
	b, err := c.run(ctx, root, "dep", "list", id)
	return string(b), err
}

// DepTree returns a mermaid flowchart of the dependency chain. dir is
// "down" (blockers), "up" (dependents), or "both".
func (c *Client) DepTree(ctx context.Context, root, id, dir string) (string, error) {
	if flagLike(id) {
		return "", os.ErrInvalid
	}
	b, err := c.run(ctx, root, "dep", "tree", id, "--direction="+dir, "--format=mermaid")
	return string(b), err
}

// Comments returns this issue's comments (textual).
func (c *Client) Comments(ctx context.Context, root, id string) (string, error) {
	if flagLike(id) {
		return "", os.ErrInvalid
	}
	b, err := c.run(ctx, root, "comments", id)
	return string(b), err
}

// --- writes (mutations). Serialized by writeMu for Dolt's single-writer DB. ---

var beadTypes = map[string]bool{"bug": true, "feature": true, "task": true, "epic": true, "chore": true}

// ValidType reports whether t is a bd issue type accepted by create.
func ValidType(t string) bool { return beadTypes[t] }

// ValidPriority reports whether p is a single digit 0..4.
func ValidPriority(p string) bool { return len(p) == 1 && p[0] >= '0' && p[0] <= '4' }

// Claim sets the issue in_progress + assigned to the caller (bd --claim is
// idempotent).
func (c *Client) Claim(ctx context.Context, root, id string) error {
	if !ValidID(id) {
		return os.ErrInvalid
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := c.run(ctx, root, "update", id, "--claim")
	return err
}

// Close closes the issue, recording an optional reason.
func (c *Client) Close(ctx context.Context, root, id, reason string) error {
	if !ValidID(id) {
		return os.ErrInvalid
	}
	args := []string{"close", id}
	if reason != "" {
		args = append(args, "--reason="+reason)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_, err := c.run(ctx, root, args...)
	return err
}

// Create makes a new issue and returns its id. bd --silent prints only the id,
// so all values pass as --flag=value (equals form, argv-safe under exec.Command).
func (c *Client) Create(ctx context.Context, root, title, itype, priority, description string) (string, error) {
	args := []string{"create", "--silent", "--title=" + title, "--type=" + itype, "--priority=" + priority}
	if description != "" {
		args = append(args, "--description="+description)
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	out, err := c.run(ctx, root, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Repo is a discovered beads-enabled repository root.
type Repo struct {
	Name string `json:"name"`
	Root string `json:"root"`
}

// Discover finds beads repos: depth-1 children of scanRoots that hold a .beads/
// directory, plus every explicit root. Deduped by absolute path. Keys on
// .beads/ (not .docs/ai) because greenfield repos have the former without the
// latter.
func Discover(scanRoots, explicit []string) []Repo {
	var out []Repo
	seen := map[string]bool{}
	add := func(path string) {
		abs, err := filepath.Abs(config.Expand(path))
		if err != nil || seen[abs] {
			return
		}
		if fi, err := os.Stat(filepath.Join(abs, ".beads")); err != nil || !fi.IsDir() {
			return
		}
		seen[abs] = true
		out = append(out, Repo{Name: filepath.Base(abs), Root: abs})
	}
	for _, r := range explicit {
		add(r)
	}
	for _, sr := range scanRoots {
		dir := config.Expand(sr)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
				add(filepath.Join(dir, e.Name()))
			}
		}
	}
	return out
}
