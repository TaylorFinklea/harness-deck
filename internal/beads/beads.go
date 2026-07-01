package beads

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/TaylorFinklea/harness-deck/internal/config"
)

// callTimeout bounds every bd invocation so a wedged repo can't stall a refresh.
const callTimeout = 10 * time.Second

// Client shells the resolved bd binary. All reads go through it; the .beads/
// directory is never touched directly.
type Client struct{ bin string }

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

// List returns all open issues (source of node attributes + parent links).
func (c *Client) List(ctx context.Context, root string) ([]Issue, error) {
	b, err := c.run(ctx, root, "list", "--status", "open", "--json")
	if err != nil {
		return nil, err
	}
	return parseIssues(b)
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
