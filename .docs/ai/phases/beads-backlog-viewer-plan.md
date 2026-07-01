# Beads Backlog Viewer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a read-only **Backlog** view to the harness-deck dashboard that reads `bd` (beads) issues from every `.beads/`-enabled repo and visualizes the priority-sorted ready queue + a per-repo dependency graph with drill-in.

**Architecture:** A live server-side view mirroring `internal/usage` — an `internal/beads` adapter shells `bd … --json`, a `beads.Monitor` caches a `Snapshot` on its own goroutine and fires an `OnChange` callback, the server exposes `GET /api/beads` + `GET /api/beads/{project}/{id}` and broadcasts an SSE `beads` event, and `aggregator.js` gains a 4th view that renders lists + a hand-rolled inline-SVG DAG. Not a report block.

**Tech Stack:** Go stdlib only (no `require` block); vanilla JS/CSS under `internal/assets` (no npm/bundler); `bd` CLI 1.0.5.

**Full design:** `.docs/ai/phases/beads-backlog-viewer-spec.md` (read it — this plan implements it).

## Global Constraints

- **Stdlib-only Go.** No `go get`; `go.mod` stays with **no `require` block**.
- **Never read `.beads/` files** (binary Dolt DB). Only `bd -C <root> … --json`.
- **Every `bd` subprocess gets `cmd.Stdin = nil`** (TUI-hang guard) and a context timeout.
- **Graceful degradation:** `bd` absent / repo error / no `.beads/` → empty state or per-repo error, never a crash.
- **Discover by `.beads/`, not `.docs/ai`** (tga + portfolio-new have `.beads/` and no `.docs/ai`).
- **Frontend:** vanilla JS via `HDDom.el` (no innerHTML); SVG via a new `svgEl` (createElementNS) + `textContent`.
- `gofmt` clean; `go build ./...` + `go test ./...` green.
- Feature is **opt-in** via `beads.enabled` (default false).
- Reference patterns (read on `main` before editing — line numbers drift): `internal/usage/{usage,opencode}.go`, `internal/projects/projects.go` (`discover`), `internal/server/{server,sse}.go`, `internal/assets/aggregator.js` (`VIEWS`/`BUILDERS`/`HDKeys.chord`/`connectEvents`), `internal/assets/hd-dom.js`.

---

### Task 1: Beads data model + parsing

**Files:**
- Create: `internal/beads/types.go` — the structs (schema).
- Create: `internal/beads/parse.go` — `bd --json` → structs + edge derivation.
- Test: `internal/beads/parse_test.go`.

**Interfaces:**
- Produces: `Issue`, `Edge`, `Counts`, `RepoSnapshot`, `Snapshot` structs; `parseIssues([]byte) ([]Issue, error)`, `parseBlocked([]byte) ([]Issue, error)` (fills `BlockedBy`), `parseStatus([]byte) (Counts, error)`, `deriveEdges(all, blocked []Issue) []Edge`.

**Notes:** `bd` wraps list-style output as a bare JSON array (verified: `ready`/`blocked`/`list` return `[…]`; `status` returns `{summary:{…}}`). `blocked --json` items carry `blocked_by[]`; `list`/`show` items carry `parent`. Unknown JSON fields are ignored (lenient, forward-compat).

- [ ] **Step 1: Write failing tests** for `parseIssues`, `parseBlocked`, `parseStatus`, `deriveEdges` using captured real fixtures.

```go
package beads

import "testing"

const readyFixture = `[
 {"id":"harness-deck-i8t","title":"Review and merge","status":"open","priority":0,"issue_type":"task","dependent_count":1,"labels":["merge"]},
 {"id":"harness-deck-5ph.1","title":"Beads viewer Phase 1","status":"open","priority":2,"issue_type":"feature","parent":"harness-deck-5ph"}
]`

const blockedFixture = `[
 {"id":"harness-deck-7ne","title":"Verify herdr e2e","status":"open","priority":1,"issue_type":"task","blocked_by":["harness-deck-eoz"]},
 {"id":"harness-deck-eoz","title":"Release v0.2.13","status":"open","priority":1,"issue_type":"task","blocked_by":["harness-deck-i8t"]}
]`

const statusFixture = `{"schema_version":1,"summary":{"open_issues":13,"ready_issues":10,"blocked_issues":3,"in_progress_issues":0,"total_issues":13}}`

func TestParseIssues(t *testing.T) {
	got, err := parseIssues([]byte(readyFixture))
	if err != nil { t.Fatal(err) }
	if len(got) != 2 { t.Fatalf("want 2, got %d", len(got)) }
	if got[0].ID != "harness-deck-i8t" || got[0].Priority != 0 { t.Errorf("bad first: %+v", got[0]) }
	if got[1].Parent != "harness-deck-5ph" { t.Errorf("want parent, got %q", got[1].Parent) }
}

func TestParseBlockedFillsBlockedBy(t *testing.T) {
	got, err := parseBlocked([]byte(blockedFixture))
	if err != nil { t.Fatal(err) }
	if len(got[0].BlockedBy) != 1 || got[0].BlockedBy[0] != "harness-deck-eoz" {
		t.Errorf("want blocked_by eoz, got %+v", got[0].BlockedBy)
	}
}

func TestParseStatus(t *testing.T) {
	c, err := parseStatus([]byte(statusFixture))
	if err != nil { t.Fatal(err) }
	if c.Ready != 10 || c.Blocked != 3 || c.Open != 13 { t.Errorf("bad counts: %+v", c) }
}

func TestDeriveEdges(t *testing.T) {
	all, _ := parseIssues([]byte(readyFixture))
	blk, _ := parseBlocked([]byte(blockedFixture))
	edges := deriveEdges(all, blk)
	// blocks: eoz->7ne, i8t->eoz ; parent: 5ph->5ph.1
	has := func(from, to, kind string) bool {
		for _, e := range edges { if e.From == from && e.To == to && e.Kind == kind { return true } }
		return false
	}
	if !has("harness-deck-eoz", "harness-deck-7ne", "blocks") { t.Error("missing blocks eoz->7ne") }
	if !has("harness-deck-i8t", "harness-deck-eoz", "blocks") { t.Error("missing blocks i8t->eoz") }
	if !has("harness-deck-5ph", "harness-deck-5ph.1", "parent") { t.Error("missing parent 5ph->5ph.1") }
}

func TestParseIssuesEmptyAndGarbage(t *testing.T) {
	if got, err := parseIssues([]byte(`[]`)); err != nil || len(got) != 0 { t.Errorf("empty: %v %v", got, err) }
	if _, err := parseIssues([]byte(`not json`)); err == nil { t.Error("want error on garbage") }
}
```

- [ ] **Step 2: Run — expect FAIL** (undefined). `go test ./internal/beads/ -run 'Parse|Derive' -v`

- [ ] **Step 3: Write `types.go`** exactly as the spec's data model:

```go
package beads

// Issue is one bd issue (fields we consume; unknown JSON is ignored).
type Issue struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Description     string   `json:"description,omitempty"`
	Status          string   `json:"status"`
	IssueType       string   `json:"issue_type"`
	Owner           string   `json:"owner,omitempty"`
	Priority        int      `json:"priority"`
	Labels          []string `json:"labels,omitempty"`
	DependencyCount int      `json:"dependency_count"`
	DependentCount  int      `json:"dependent_count"`
	CommentCount    int      `json:"comment_count"`
	Created         string   `json:"created_at,omitempty"`
	Updated         string   `json:"updated_at,omitempty"`
	Parent          string   `json:"parent,omitempty"`
	BlockedBy       []string `json:"blocked_by,omitempty"`
}

// Edge is a dependency-graph edge. Kind is "blocks" or "parent".
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

type Counts struct {
	Open, Ready, Blocked, InProgress, Total int
}

// RepoSnapshot is one repo's beads state; Err isolates a failing repo.
type RepoSnapshot struct {
	Name    string  `json:"name"`
	Root    string  `json:"root"`
	Ready   []Issue `json:"ready"`
	Blocked []Issue `json:"blocked"`
	All     []Issue `json:"all"`
	Edges   []Edge  `json:"edges"`
	Counts  Counts  `json:"counts"`
	Err     string  `json:"err,omitempty"`
}

type Snapshot struct {
	Repos     []RepoSnapshot `json:"repos"`
	Updated   string         `json:"updated"`
	Available bool           `json:"available"`
}
```

- [ ] **Step 4: Write `parse.go`.** `parseIssues`/`parseBlocked` unmarshal a `[]Issue`; `parseStatus` unmarshals `{summary:{…}}`; `deriveEdges` walks blocked→blocked_by (blocks) and all→parent (parent). Node-attribute source is `all`. Keep it pure/testable.

```go
package beads

import "encoding/json"

func parseIssues(b []byte) ([]Issue, error) {
	var xs []Issue
	if err := json.Unmarshal(b, &xs); err != nil { return nil, err }
	return xs, nil
}

// parseBlocked reuses the Issue shape (blocked_by populated by bd).
func parseBlocked(b []byte) ([]Issue, error) { return parseIssues(b) }

func parseStatus(b []byte) (Counts, error) {
	var s struct {
		Summary struct {
			Open       int `json:"open_issues"`
			Ready      int `json:"ready_issues"`
			Blocked    int `json:"blocked_issues"`
			InProgress int `json:"in_progress_issues"`
			Total      int `json:"total_issues"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(b, &s); err != nil { return Counts{}, err }
	return Counts{s.Summary.Open, s.Summary.Ready, s.Summary.Blocked, s.Summary.InProgress, s.Summary.Total}, nil
}

func deriveEdges(all, blocked []Issue) []Edge {
	var edges []Edge
	seen := map[string]bool{}
	add := func(from, to, kind string) {
		k := kind + "|" + from + "|" + to
		if from == "" || to == "" || seen[k] { return }
		seen[k] = true
		edges = append(edges, Edge{From: from, To: to, Kind: kind})
	}
	for _, b := range blocked {
		for _, blocker := range b.BlockedBy { add(blocker, b.ID, "blocks") }
	}
	for _, i := range all {
		if i.Parent != "" { add(i.Parent, i.ID, "parent") }
	}
	return edges
}
```

- [ ] **Step 5: Run — expect PASS.** `go test ./internal/beads/ -run 'Parse|Derive' -v`
- [ ] **Step 6: `gofmt -w internal/beads/ && commit`** — `feat(beads): issue model + bd --json parsing`.

---

### Task 2: Beads client (bin resolution, shelling, argv guard) + discovery

**Files:**
- Create: `internal/beads/beads.go` — `Client`, `New`, `resolveBin`, `flagLike`, shelling methods, `Repo`, `Discover`.
- Test: `internal/beads/beads_test.go`.

**Interfaces:**
- Produces: `New() (*Client, bool)`; `(*Client) Ready/Blocked/List(ctx, root) ([]Issue, error)`, `Status(ctx, root) (Counts, error)`, `Show(ctx, root, id) (Issue, error)`, `DepList/Comments(ctx, root, id) (string, error)`, `DepTree(ctx, root, id, dir) (string, error)`; `Repo{Name, Root}`; `Discover(scanRoots, explicit []string) []Repo`; `flagLike(string) bool`.

**Notes:** Mirror `usage.opencodeBin` for `resolveBin` (PATH then `/opt/homebrew/bin`, `/usr/local/bin`, `~/.local/bin`). Mirror `projects.discover` for `Discover` but stat `.beads` instead of `.docs/ai`. `flagLike` = leading `-`; also constrain drill-in ids to `[A-Za-z0-9._-]`. Shelling methods can't be unit-tested without `bd`; test the deterministic pieces (`resolveBin` fallback, `flagLike`, `Discover`) — the shell methods are exercised in Task 5's fake and in manual verify.

- [ ] **Step 1: Write failing tests** (`resolveBin` probes fallback dir; `flagLike`; `Discover` finds `.beads` and ignores `.docs/ai`-only).

```go
package beads

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBinFallback(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "bd")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil { t.Fatal(err) }
	got, ok := resolveBin("definitely-not-on-path-xyz", []string{bin})
	if !ok || got != bin { t.Fatalf("want %s, got %q ok=%v", bin, got, ok) }
}

func TestFlagLike(t *testing.T) {
	for _, s := range []string{"-rf", "--force", "-1"} {
		if !flagLike(s) { t.Errorf("%q should be flag-like", s) }
	}
	for _, s := range []string{"harness-deck-5ph.1", "abc"} {
		if flagLike(s) { t.Errorf("%q should NOT be flag-like", s) }
	}
}

func TestDiscoverKeysOnBeads(t *testing.T) {
	root := t.TempDir()
	mk := func(name string, subs ...string) string {
		p := filepath.Join(root, name)
		for _, s := range subs { os.MkdirAll(filepath.Join(p, s), 0o755) }
		return p
	}
	beadsRepo := mk("tga", ".beads")          // .beads, no .docs/ai (the landmine case)
	bothRepo := mk("harness-deck", ".beads", ".docs/ai")
	mk("chezmoi-config", ".docs/ai")          // .docs/ai only → excluded
	mk("plain")                               // neither → excluded

	repos := Discover([]string{root}, nil)
	names := map[string]bool{}
	for _, r := range repos { names[r.Name] = true }
	if !names["tga"] { t.Error("tga (.beads, no .docs/ai) must be discovered") }
	if !names["harness-deck"] { t.Error("harness-deck (.beads) must be discovered") }
	if names["chezmoi-config"] { t.Error(".docs/ai-only must NOT be discovered") }
	if names["plain"] { t.Error("neither must NOT be discovered") }
	_ = beadsRepo; _ = bothRepo
}
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/beads/ -run 'Resolve|Flag|Discover' -v`

- [ ] **Step 3: Write `beads.go`.** Read `internal/usage/opencode.go` for `opencodeBin` and `internal/projects/projects.go` for `discover`, then mirror. Concrete skeleton:

```go
package beads

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const callTimeout = 10 * time.Second

type Client struct{ bin string }

func New() (*Client, bool) {
	home, _ := os.UserHomeDir()
	fb := []string{"/opt/homebrew/bin/bd", "/usr/local/bin/bd"}
	if home != "" { fb = append(fb, filepath.Join(home, ".local", "bin", "bd")) }
	bin, ok := resolveBin("bd", fb)
	if !ok { return nil, false }
	return &Client{bin: bin}, true
}

func resolveBin(name string, fallbacks []string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil { return p, true }
	for _, p := range fallbacks {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() { return p, true }
	}
	return "", false
}

func flagLike(s string) bool { return strings.HasPrefix(s, "-") }

func (c *Client) run(ctx context.Context, root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	full := append([]string{"-C", root}, args...)
	cmd := exec.CommandContext(ctx, c.bin, full...)
	cmd.Stdin = nil // child gets /dev/null — TUI-hang guard
	return cmd.Output()
}

func (c *Client) Ready(ctx context.Context, root string) ([]Issue, error) {
	b, err := c.run(ctx, root, "ready", "--json"); if err != nil { return nil, err }
	return parseIssues(b)
}
func (c *Client) Blocked(ctx context.Context, root string) ([]Issue, error) {
	b, err := c.run(ctx, root, "blocked", "--json"); if err != nil { return nil, err }
	return parseBlocked(b)
}
func (c *Client) List(ctx context.Context, root string) ([]Issue, error) {
	b, err := c.run(ctx, root, "list", "--json"); if err != nil { return nil, err }
	return parseIssues(b)
}
func (c *Client) Status(ctx context.Context, root string) (Counts, error) {
	b, err := c.run(ctx, root, "status", "--json"); if err != nil { return Counts{}, err }
	return parseStatus(b)
}
func (c *Client) Show(ctx context.Context, root, id string) (Issue, error) {
	if flagLike(id) { return Issue{}, os.ErrInvalid }
	b, err := c.run(ctx, root, "show", id, "--json"); if err != nil { return Issue{}, err }
	xs, err := parseIssues(b); if err != nil { return Issue{}, err }
	if len(xs) == 0 { return Issue{}, os.ErrNotExist }
	return xs[0], nil
}
func (c *Client) DepList(ctx context.Context, root, id string) (string, error) {
	if flagLike(id) { return "", os.ErrInvalid }
	b, err := c.run(ctx, root, "dep", "list", id); return string(b), err
}
func (c *Client) DepTree(ctx context.Context, root, id, dir string) (string, error) {
	if flagLike(id) { return "", os.ErrInvalid }
	b, err := c.run(ctx, root, "dep", "tree", id, "--direction="+dir, "--format=mermaid"); return string(b), err
}
func (c *Client) Comments(ctx context.Context, root, id string) (string, error) {
	if flagLike(id) { return "", os.ErrInvalid }
	b, err := c.run(ctx, root, "comments", id); return string(b), err
}

type Repo struct{ Name, Root string }

func Discover(scanRoots, explicit []string) []Repo {
	var out []Repo
	seen := map[string]bool{}
	add := func(root string) {
		abs, err := filepath.Abs(root); if err != nil { return }
		if seen[abs] { return }
		if fi, err := os.Stat(filepath.Join(abs, ".beads")); err != nil || !fi.IsDir() { return }
		seen[abs] = true
		out = append(out, Repo{Name: filepath.Base(abs), Root: abs})
	}
	for _, r := range explicit { add(r) }
	for _, sr := range scanRoots {
		expanded := sr
		if strings.HasPrefix(sr, "~") { if home, e := os.UserHomeDir(); e == nil { expanded = filepath.Join(home, sr[1:]) } }
		entries, err := os.ReadDir(expanded); if err != nil { continue }
		for _, e := range entries {
			if e.IsDir() && !strings.HasPrefix(e.Name(), ".") { add(filepath.Join(expanded, e.Name())) }
		}
	}
	return out
}
```

- [ ] **Step 4: Run — expect PASS.** `go test ./internal/beads/ -run 'Resolve|Flag|Discover' -v`
- [ ] **Step 5: `gofmt -w` + full package test + commit** — `go test ./internal/beads/`; `feat(beads): bd client + .beads discovery`.

---

### Task 3: Snapshot Monitor

**Files:**
- Create: `internal/beads/monitor.go` — `Monitor`, `NewMonitor`, `Start`, `Snapshot()`, refresh, fingerprint, `fetchRepo`.
- Test: `internal/beads/monitor_test.go`.

**Interfaces:**
- Consumes: the `Client` methods (via a small `fetcher` interface so tests inject a fake).
- Produces: `NewMonitor(f fetcher, repos func() []Repo, interval time.Duration, onChange func()) *Monitor`; `(*Monitor) Start(ctx)`, `Snapshot() Snapshot` (nil-safe), `refreshOnce(ctx)`.

**Notes:** Mirror `usage.Monitor` (RWMutex cache, `Start` launches a goroutine that refreshes immediately then on a ticker). `fetcher` = the subset of `Client` the monitor needs: `Ready/Blocked/List/Status`. `repos` is a thunk so re-discovery happens each tick. Fingerprint = FNV over each repo's issue id+status+updated; `onChange` fires only when it differs from the previous refresh.

- [ ] **Step 1: Write failing tests** — snapshot caches; onChange fires once per real change, not on identical refresh.

```go
package beads

import (
	"context"
	"testing"
)

type fakeFetcher struct{ ready, blocked, all []Issue; counts Counts; err error }
func (f *fakeFetcher) Ready(_ context.Context, _ string) ([]Issue, error)   { return f.ready, f.err }
func (f *fakeFetcher) Blocked(_ context.Context, _ string) ([]Issue, error) { return f.blocked, f.err }
func (f *fakeFetcher) List(_ context.Context, _ string) ([]Issue, error)    { return f.all, f.err }
func (f *fakeFetcher) Status(_ context.Context, _ string) (Counts, error)   { return f.counts, f.err }

func TestMonitorRefreshCachesAndFiresOnce(t *testing.T) {
	f := &fakeFetcher{ready: []Issue{{ID: "a", Status: "open"}}, counts: Counts{Ready: 1, Open: 1}}
	repos := func() []Repo { return []Repo{{Name: "r", Root: "/r"}} }
	fires := 0
	m := NewMonitor(f, repos, 0, func() { fires++ })
	m.refreshOnce(context.Background())
	snap := m.Snapshot()
	if !snap.Available || len(snap.Repos) != 1 || len(snap.Repos[0].Ready) != 1 {
		t.Fatalf("bad snapshot: %+v", snap)
	}
	if fires != 1 { t.Fatalf("want 1 onChange, got %d", fires) }
	m.refreshOnce(context.Background()) // identical → no new fire
	if fires != 1 { t.Fatalf("identical refresh must not fire, got %d", fires) }
	f.ready = []Issue{{ID: "a", Status: "closed"}} // change
	m.refreshOnce(context.Background())
	if fires != 2 { t.Fatalf("changed refresh must fire, got %d", fires) }
}

func TestMonitorRepoErrorIsolated(t *testing.T) {
	f := &fakeFetcher{err: context.DeadlineExceeded}
	m := NewMonitor(f, func() []Repo { return []Repo{{Name: "r", Root: "/r"}} }, 0, nil)
	m.refreshOnce(context.Background())
	snap := m.Snapshot()
	if len(snap.Repos) != 1 || snap.Repos[0].Err == "" { t.Fatalf("want per-repo Err, got %+v", snap) }
}
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/beads/ -run Monitor -v`

- [ ] **Step 3: Write `monitor.go`.** Read `internal/usage/usage.go` (`Monitor`/`Start`/`Samples`) and mirror the locking/goroutine shape. `nowUTC` indirected for tests (mirror usage). `refreshOnce` builds each `RepoSnapshot` via `fetchRepo`, computes the fingerprint, swaps the cache, fires `onChange` on diff. Guard `nil` receiver in `Snapshot`.

- [ ] **Step 4: Run — expect PASS.** `go test ./internal/beads/ -run Monitor -v`
- [ ] **Step 5: `gofmt -w` + `go test ./internal/beads/` + commit** — `feat(beads): snapshot monitor with change fingerprint`.

---

### Task 4: Config gate

**Files:**
- Modify: `internal/config/config.go` — add `BeadsConfig` + `Config.Beads` (model on `UsageConfig`).
- Test: `internal/config/config_test.go` (add a case; follow existing table tests).

**Interfaces:**
- Produces: `config.BeadsConfig{ Enabled bool; RefreshSec int }`; `Config.Beads BeadsConfig`.

- [ ] **Step 1: Write failing test** — a config JSON with `"beads":{"enabled":true,"refresh_sec":20}` loads those values; absent → zero/false default.

```go
func TestLoadBeadsConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"beads":{"enabled":true,"refresh_sec":20}}`), 0o644)
	t.Setenv("HARNESS_DECK_CONFIG", path)
	c, err := Load()
	if err != nil { t.Fatal(err) }
	if !c.Beads.Enabled || c.Beads.RefreshSec != 20 { t.Fatalf("bad beads cfg: %+v", c.Beads) }
}
```
*(Adjust to the file's actual `Load`/env idiom — read `config.go` first; the env var is `HARNESS_DECK_CONFIG`.)*

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/config/ -run Beads -v`
- [ ] **Step 3: Add the struct + field.**

```go
// BeadsConfig gates the read-only Backlog view (beads/bd issues). Opt-in.
type BeadsConfig struct {
	Enabled    bool `json:"enabled,omitempty"`
	RefreshSec int  `json:"refresh_sec,omitempty"` // default 15 when <= 0
}
```
Add to `Config`: `Beads BeadsConfig `json:"beads,omitempty"`` next to `Usage`.

- [ ] **Step 4: Run — expect PASS.** `go test ./internal/config/ -v`
- [ ] **Step 5: commit** — `feat(config): beads.enabled/refresh_sec gate`.

---

### Task 5: Server endpoints + wiring + SSE

**Files:**
- Create: `internal/server/beads.go` — `beadsSource` interface, `handleBeads`, `handleBeadsIssue`, detail assembly.
- Modify: `internal/server/server.go` — struct field, Monitor construction (gated), route registration, `Start`.
- Test: `internal/server/beads_test.go`.

**Interfaces:**
- Consumes: `beads.Monitor` (`Snapshot()`), `beads.Client` (drill-in shelling), `beads.Discover`, `s.hub.broadcastEvent`.
- Produces: `GET /api/beads` → `beads.Snapshot` JSON; `GET /api/beads/{project}/{id}` → `{issue, blockers, dependents, comments}` JSON (400 bad id, 404 unknown, 503 disabled).

**Notes:** Read `server.go`'s `New()` route block + `handleUsage` + how `s.usage` is built/started; mirror. Construct the Monitor only when `cfg.Beads.Enabled && beads.New() ok`, wiring `onChange = func(){ s.hub.broadcastEvent("beads","changed") }`; `Start` it in `Serve()`. `handleBeads` serves `s.beads.Snapshot()` (nil-safe → `{repos:[],available:false}`). `handleBeadsIssue` validates the id (`beads` guard), maps `{project}`→root via discovered repos, shells detail on demand. Define a `beadsSource` interface (`Snapshot() beads.Snapshot`) so tests inject a fake without a real monitor.

- [ ] **Step 1: Write failing tests** — build a `Server` with an injected fake snapshot; assert `/api/beads` JSON and the `{id}` guards.

```go
func TestHandleBeadsJSON(t *testing.T) {
	s := &Server{beads: fakeBeadsSource{snap: beads.Snapshot{Available: true, Repos: []beads.RepoSnapshot{{Name: "harness-deck", Counts: beads.Counts{Ready: 10}}}}}}
	req := httptest.NewRequest("GET", "/api/beads", nil)
	rr := httptest.NewRecorder()
	s.handleBeads(rr, req)
	if rr.Code != 200 { t.Fatalf("code %d", rr.Code) }
	var got beads.Snapshot
	json.Unmarshal(rr.Body.Bytes(), &got)
	if !got.Available || got.Repos[0].Name != "harness-deck" { t.Fatalf("bad body: %s", rr.Body) }
}

func TestHandleBeadsDisabledEmpty(t *testing.T) {
	s := &Server{} // no beads source
	rr := httptest.NewRecorder()
	s.handleBeads(rr, httptest.NewRequest("GET", "/api/beads", nil))
	if rr.Code != 200 { t.Fatalf("code %d", rr.Code) }
	if !strings.Contains(rr.Body.String(), `"available":false`) { t.Fatalf("want available:false, got %s", rr.Body) }
}

func TestHandleBeadsIssueBadID(t *testing.T) {
	s := &Server{beads: fakeBeadsSource{}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/beads/harness-deck/-rf", nil)
	req.SetPathValue("project", "harness-deck"); req.SetPathValue("id", "-rf")
	s.handleBeadsIssue(rr, req)
	if rr.Code != 400 { t.Fatalf("want 400 for flag-like id, got %d", rr.Code) }
}
```
*(Match `Server` struct construction to the real zero-value-friendly fields; if `New()` is required, add a test constructor. Read the existing `*_test.go` in `internal/server` for the idiom.)*

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/server/ -run Beads -v`
- [ ] **Step 3: Implement `beads.go` handlers + `beadsSource` interface + fake.** JSON via `json.NewEncoder`. Drill-in: `beads` id guard → 400; unknown project/id → 404; nil source → 503 (for `{id}`) / `{available:false}` (for list).
- [ ] **Step 4: Wire `server.go`** — add `beads beadsSource` + `beadsClient *beads.Client` fields; construct gated in `New()`; register `GET /api/beads` and `GET /api/beads/{project}/{id}`; `Start` the monitor in `Serve()`.
- [ ] **Step 5: Run — expect PASS + full server suite.** `go test ./internal/server/ -v`
- [ ] **Step 6: commit** — `feat(server): /api/beads endpoints + monitor wiring + SSE`.

---

### Task 6: Backlog view (frontend)

**Files:**
- Create: `internal/assets/aggregator-backlog.js` — `window.HDBacklog` (graph layout, list/detail render, `svgEl`).
- Modify: `internal/assets/aggregator.js` — `VIEWS`, `BUILDERS`, `viewBacklog`, `g b` chord, `:backlog` command, `beadsData`, `refreshBeads`, `connectEvents` `beads` listener, cursor wiring.
- Modify: `internal/assets/aggregator.css` — `/* backlog */` section (+ `mobile.css` collapse rule).
- Modify: `internal/assets/assets.go` — embed + bundle `aggregator-backlog.js` (mirror another `aggregator-*` module's embed/bundle-order).
- Verify: browser (chrome-devtools MCP / live dashboard).

**Interfaces:**
- Consumes: `GET /api/beads` (`beads.Snapshot` shape from Task 5), `HDDom.el`, `HDKeys.chord`, `VimNav.addCommand`, the inbox cursor helpers.
- Produces: a `backlog` view; `HDBacklog.repoCard(repo)`, `HDBacklog.graphSVG(repo)`, `HDBacklog.detail(project,id)`.

**Notes:** Read `aggregator.js` (`VIEWS`/`BUILDERS`/`viewInbox` cursor model/`connectEvents`/`registerCommands`) + `hd-dom.js` (`el`) + one `aggregator-*` module + `assets.go` bundle order first. `g b` is free. SVG must use `document.createElementNS` (a `svgEl` helper) — `el()` can't create SVG nodes. Layout = longest-path layering per the spec; match the approved mockup (report `20260630-beads-backlog-mockups`, Option A) for colors/shape.

- [ ] **Step 1: `aggregator-backlog.js`** — `svgEl(tag, attrs, kids)`; `layerNodes(nodes, edges)` (longest-path, visited-guard, depth cap); `graphSVG(repo)` (rects+labels+arrowhead marker, status/priority colors via `--tn-*`); `repoCard(repo)` (header strip + READY/BLOCKED columns via `el`); `detail(project,id)` (fetch `/api/beads/{project}/{id}`, render panel). Expose on `window.HDBacklog`.
- [ ] **Step 2: `aggregator.js` registration** — add `{id:'backlog',label:'backlog'}` to `VIEWS`; `backlog: viewBacklog` to `BUILDERS`; `function viewBacklog()` maps `beadsData.repos` → `HDBacklog.repoCard`; `var beadsData={repos:[],available:false}`; `function refreshBeads()` (fetch, tolerant, re-render if active); call on view-switch + startup; `HDKeys.chord('g','b',…showView('backlog'))`; `VimNav.addCommand('backlog',…)`; `es.addEventListener('beads', refreshBeads)` in `connectEvents`; wire j/k cursor + Enter (`HDBacklog.detail`) + Esc mirroring `viewInbox`.
- [ ] **Step 3: CSS** — `aggregator.css` `.backlog-*` (reuse `.panel`/`.metric-*` tokens); `mobile.css` collapse graph ≤720px.
- [ ] **Step 4: `assets.go`** — embed + include `aggregator-backlog.js` in the `AggregatorJS` bundle (correct order: it defines `HDBacklog` used by core `viewBacklog`, so prepend before core like `aggregator-tree.js`, or append if only called from deferred callbacks — match the seam).
- [ ] **Step 5: Build + browser-verify.** `go build ./... && ./harness-deck serve` (with `beads.enabled:true` in a test config). Check: view lists all `.beads` repos incl. tga/portfolio-new; the harness-deck `i8t→eoz→7ne` graph draws; blocked items read blocked; `g b`/`:backlog`/j/k/Enter/Esc work; SSE live-refresh; **0 console errors**.
- [ ] **Step 6: commit** — `feat(ui): beads Backlog view — lists + inline-SVG graph + drill-in`.

---

### Task 7: Docs + close-out

**Files:**
- Modify: `docs/SETUP.md` — a beads-config section (mirror the usage/agents §).
- Modify: `.docs/ai/decisions.md` — ADR "Beads backlog viewer".
- Modify: `.docs/ai/roadmap.md` + `.docs/ai/current-state.md` — route per the Session-End table.

- [ ] **Step 1** SETUP.md beads section (`beads.enabled`, `refresh_sec`, what the view shows).
- [ ] **Step 2** decisions.md ADR: live-view-not-block, structured-edges-not-mermaid, 15s monitor + SSE, discovery-by-`.beads/`, opt-in.
- [ ] **Step 3** roadmap/current-state routing; `gofmt -l .` (empty), `go build ./... && go test ./...` green, `grep -n require go.mod` (none).
- [ ] **Step 4** `bd -C ~/git/harness-deck close harness-deck-5ph.1 --reason "Phase 1 beads Backlog view shipped + verified"`.
- [ ] **Step 5: commit** — `docs(beads): setup + ADR + roadmap for the Backlog view`.

---

## Self-Review

- **Spec coverage:** adapter+parse (T1), client+discovery (T2), monitor (T3), config (T4), endpoints+SSE (T5), view+graph+drill-in+keyboard (T6), degradation (T1/T3/T5 err paths + T6 empty states), tests (each task), docs+acceptance (T7). Graph = inline SVG (T6). Discovery-by-`.beads/` (T2). ✔ every spec section maps to a task.
- **Type consistency:** `Issue/Edge/Counts/RepoSnapshot/Snapshot` defined once (T1) and consumed unchanged (T3/T5/T6); `fetcher` (T3) ⊂ `Client` (T2); `beadsSource.Snapshot()` (T5) returns `beads.Snapshot` (T1). ✔
- **Placeholder scan:** integration-edit steps intentionally say "read X; mirror it" (per AGENTS.md — codebase-derived idioms must be read, not guessed), not "TODO". Greenfield code is concrete. Test code is complete. ✔
