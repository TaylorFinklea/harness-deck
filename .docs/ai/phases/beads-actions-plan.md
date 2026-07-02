# Beads Actions (Phase 2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add claim / close / create actions to the read-only beads Backlog view, gated behind a new `beads.writable` flag.

**Architecture:** New `beads.Client` write methods (mutex-serialized for Dolt's single-writer constraint) → three `POST /api/beads/...` handlers (validate → `Show` re-check → write → `Monitor.RefreshNow` + SSE) → writable-only UI (detail-panel Claim/Close + a create form + keys `c`/`x`/`n`), gated by `window.HD_BEADS_WRITABLE`.

**Tech Stack:** Go stdlib only (no `require`); vanilla JS/CSS under `internal/assets`; `bd` CLI.

**Full design:** `.docs/ai/phases/beads-actions-spec.md`.

## Global Constraints

- Stdlib-only Go, **no `go.mod` require block**; vanilla JS (no npm).
- Writes require `beads.enabled && beads.writable`; enforced server-side (403), not just in JS.
- All `bd` string values via `--flag=value` **equals form**; `id` via `beads.ValidID`; `type`/`priority` are validated enums.
- Writes are **mutex-serialized** on `beads.Client` (Dolt single-writer). Reads stay unlocked.
- `bd create --silent` prints only the new id; every `bd` call keeps a ctx timeout + nil stdin.
- `gofmt` clean; `go build ./...` + `go test ./...` green.

---

### Task 1: config flag + beads write methods

**Files:**
- Modify: `internal/config/config.go` — add `Writable bool` to `BeadsConfig`.
- Modify: `internal/beads/beads.go` — add `mu sync.Mutex` to `Client`; `Claim`/`Close`/`Create`; `ValidType`/`ValidPriority`.
- Test: `internal/beads/beads_test.go` (enum validators) + `internal/config/config_test.go` (writable parses).

**Interfaces:**
- Produces: `config.BeadsConfig.Writable`; `(*beads.Client) Claim(ctx,root,id) error`, `Close(ctx,root,id,reason) error`, `Create(ctx,root,title,itype,priority,description) (string,error)`; `beads.ValidType(string) bool`, `beads.ValidPriority(string) bool`.

- [ ] **Step 1: Write failing tests** for the enum validators + writable config.

```go
// in internal/beads/beads_test.go
func TestValidTypeAndPriority(t *testing.T) {
	for _, ty := range []string{"bug", "feature", "task", "epic", "chore"} {
		if !ValidType(ty) {
			t.Errorf("%q should be a valid type", ty)
		}
	}
	for _, ty := range []string{"", "epi", "event", "Task", "-x"} {
		if ValidType(ty) {
			t.Errorf("%q should be invalid type", ty)
		}
	}
	for _, p := range []string{"0", "1", "2", "3", "4"} {
		if !ValidPriority(p) {
			t.Errorf("%q should be valid priority", p)
		}
	}
	for _, p := range []string{"", "5", "-1", "P2", "10"} {
		if ValidPriority(p) {
			t.Errorf("%q should be invalid priority", p)
		}
	}
}
```
```go
// in internal/config/config_test.go
func TestLoadBeadsWritable(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	os.WriteFile(p, []byte(`{"beads":{"enabled":true,"writable":true}}`), 0o644)
	t.Setenv("HARNESS_DECK_CONFIG", p)
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !c.Beads.Writable {
		t.Errorf("Beads.Writable = false, want true")
	}
}
```

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/beads/ ./internal/config/ -run 'ValidType|BeadsWritable'`

- [ ] **Step 3: Add `Writable` to `BeadsConfig`** (below `RefreshSec` in config.go):
```go
	// Writable enables mutations (claim/close/create) from the Backlog view.
	// Off by default so the view is read-only even when Enabled. Restart to apply.
	Writable bool `json:"writable,omitempty"`
```

- [ ] **Step 4: Add the write methods + validators to beads.go.** Add `mu sync.Mutex` to the `Client` struct. Mirror the existing `run` helper.
```go
var beadTypes = map[string]bool{"bug": true, "feature": true, "task": true, "epic": true, "chore": true}

// ValidType / ValidPriority guard create input before it reaches bd argv.
func ValidType(t string) bool { return beadTypes[t] }
func ValidPriority(p string) bool { return len(p) == 1 && p[0] >= '0' && p[0] <= '4' }

// Claim sets the issue in_progress + assigned to the caller (idempotent).
func (c *Client) Claim(ctx context.Context, root, id string) error {
	if !ValidID(id) {
		return os.ErrInvalid
	}
	c.mu.Lock()
	defer c.mu.Unlock()
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
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.run(ctx, root, args...)
	return err
}

// Create makes a new issue and returns its id (bd --silent prints only the id).
func (c *Client) Create(ctx context.Context, root, title, itype, priority, description string) (string, error) {
	args := []string{"create", "--silent", "--title=" + title, "--type=" + itype, "--priority=" + priority}
	if description != "" {
		args = append(args, "--description="+description)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out, err := c.run(ctx, root, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
```
(`strings` is already imported in beads.go.)

- [ ] **Step 5: Run — expect PASS.** `go test ./internal/beads/ ./internal/config/`
- [ ] **Step 6: gofmt + commit** — `feat(beads): write methods (claim/close/create) + beads.writable gate`.

---

### Task 2: server endpoints + wiring + freshness

**Files:**
- Modify: `internal/beads/monitor.go` — export `RefreshNow(ctx)`.
- Modify: `internal/server/beads.go` — `beadsMutator` interface, `handleBeadsClaim/Close/Create`, `beadsWriteGate`, `beadsShowStatus` (extract from handleBeadsIssue), `beadsRefreshAndBroadcast`.
- Modify: `internal/server/server.go` — `beadsMutator` field + construction; route registration; `HD_BEADS_WRITABLE` in `handleShell`.
- Modify: `internal/server/shell.html.tmpl` — `window.HD_BEADS_WRITABLE`.
- Test: `internal/server/beads_test.go`.

**Interfaces:**
- Consumes: `beads.Client` write methods, `beads.Monitor.RefreshNow`, `s.beadsClient.Show`, `s.hub.broadcastEvent`.
- Produces: `POST /api/beads/{project}/{id}/claim`, `.../{id}/close`, `/api/beads/{project}/create`.

- [ ] **Step 1: Write failing handler tests** with a fake implementing both `beadsDetailer` (Show) and `beadsMutator`.

```go
type fakeBeadsRW struct {
	issue     beads.Issue
	showErr   error
	claimErr  error
	closeErr  error
	createID  string
	createErr error
	closedIDs []string
}

func (f *fakeBeadsRW) Show(context.Context, string, string) (beads.Issue, error) {
	return f.issue, f.showErr
}
func (f *fakeBeadsRW) DepList(context.Context, string, string) (string, error)      { return "", nil }
func (f *fakeBeadsRW) DepTree(context.Context, string, string, string) (string, error) { return "", nil }
func (f *fakeBeadsRW) Comments(context.Context, string, string) (string, error)     { return "", nil }
func (f *fakeBeadsRW) Claim(context.Context, string, string) error                  { return f.claimErr }
func (f *fakeBeadsRW) Close(_ context.Context, _, id, _ string) error {
	f.closedIDs = append(f.closedIDs, id)
	return f.closeErr
}
func (f *fakeBeadsRW) Create(context.Context, string, string, string, string, string) (string, error) {
	return f.createID, f.createErr
}

func newRWServer(t *testing.T, f *fakeBeadsRW, writable bool) (*Server, string) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "myrepo", ".beads"), 0o755)
	return &Server{
		cfg:         config.Config{ScanRoots: []string{root}, Beads: config.BeadsConfig{Enabled: true, Writable: writable}},
		beadsClient: f, beadsMutator: f, hub: newHub(),
	}, root
}

func TestClaimHappyPath(t *testing.T) {
	f := &fakeBeadsRW{issue: beads.Issue{ID: "myrepo-a", Status: "open"}}
	s, _ := newRWServer(t, f, true)
	rr := httptest.NewRecorder()
	s.handleBeadsClaim(rr, beadsIssueReq("myrepo", "myrepo-a"))
	if rr.Code != 200 {
		t.Fatalf("want 200, got %d (%s)", rr.Code, rr.Body)
	}
}

func TestClaimForbiddenWhenNotWritable(t *testing.T) {
	f := &fakeBeadsRW{issue: beads.Issue{ID: "myrepo-a", Status: "open"}}
	s, _ := newRWServer(t, f, false) // writable=false
	rr := httptest.NewRecorder()
	s.handleBeadsClaim(rr, beadsIssueReq("myrepo", "myrepo-a"))
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d", rr.Code)
	}
}

func TestCloseAlreadyClosed409(t *testing.T) {
	f := &fakeBeadsRW{issue: beads.Issue{ID: "myrepo-a", Status: "closed"}}
	s, _ := newRWServer(t, f, true)
	rr := httptest.NewRecorder()
	req := beadsIssueReq("myrepo", "myrepo-a")
	req.Body = io.NopCloser(strings.NewReader(`{"reason":"done"}`))
	s.handleBeadsClose(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", rr.Code)
	}
	if len(f.closedIDs) != 0 {
		t.Errorf("must not call Close on an already-closed issue")
	}
}

func TestCreateValidatesInput(t *testing.T) {
	f := &fakeBeadsRW{createID: "myrepo-new"}
	s, root := newRWServer(t, f, true)
	_ = root
	body := func(j string) *http.Request {
		r := httptest.NewRequest("POST", "/api/beads/myrepo/create", strings.NewReader(j))
		r.SetPathValue("project", "myrepo")
		return r
	}
	// bad type
	rr := httptest.NewRecorder()
	s.handleBeadsCreate(rr, body(`{"title":"x","type":"nope","priority":"2"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad type want 400, got %d", rr.Code)
	}
	// empty title
	rr = httptest.NewRecorder()
	s.handleBeadsCreate(rr, body(`{"title":"  ","type":"task","priority":"2"}`))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("empty title want 400, got %d", rr.Code)
	}
	// happy path returns id
	rr = httptest.NewRecorder()
	s.handleBeadsCreate(rr, body(`{"title":"hi","type":"task","priority":"2"}`))
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "myrepo-new") {
		t.Fatalf("want 200 + id, got %d %s", rr.Code, rr.Body)
	}
}
```
(Add `"io"` to the test imports.)

- [ ] **Step 2: Run — expect FAIL.** `go test ./internal/server/ -run 'Claim|Close|Create'`

- [ ] **Step 3: `Monitor.RefreshNow`** (monitor.go):
```go
// RefreshNow forces an immediate refresh so a mutation shows without waiting for
// the next tick. Safe on a nil Monitor.
func (m *Monitor) RefreshNow(ctx context.Context) {
	if m == nil {
		return
	}
	m.refreshOnce(ctx)
}
```

- [ ] **Step 4: Implement the handlers + helpers in server/beads.go.** Extract the Phase-1 Show-error mapping into `beadsShowStatus(err) int` (404 for `os.ErrNotExist`, else 502) and reuse it in `handleBeadsIssue`. Add:
```go
type beadsMutator interface {
	Claim(ctx context.Context, root, id string) error
	Close(ctx context.Context, root, id, reason string) error
	Create(ctx context.Context, root, title, itype, priority, description string) (string, error)
}

// beadsWriteGate enforces the mutation preconditions and resolves the repo root.
func (s *Server) beadsWriteGate(w http.ResponseWriter, project string) (string, bool) {
	if s.beadsMutator == nil {
		http.Error(w, "beads disabled", http.StatusServiceUnavailable)
		return "", false
	}
	if !s.cfg.Beads.Writable {
		http.Error(w, "beads is read-only (set beads.writable)", http.StatusForbidden)
		return "", false
	}
	root, ok := s.beadsRepoRoot(project)
	if !ok {
		http.Error(w, "unknown project", http.StatusNotFound)
		return "", false
	}
	return root, true
}

func (s *Server) beadsRefreshAndBroadcast(ctx context.Context) {
	if s.beadsMonitor != nil {
		s.beadsMonitor.RefreshNow(ctx)
	}
	s.hub.broadcastEvent("beads", "changed")
}

func (s *Server) handleBeadsClaim(w http.ResponseWriter, r *http.Request) {
	project, id := r.PathValue("project"), r.PathValue("id")
	if !beads.ValidID(id) {
		http.Error(w, "bad issue id", http.StatusBadRequest)
		return
	}
	root, ok := s.beadsWriteGate(w, project)
	if !ok {
		return
	}
	ctx := r.Context()
	issue, err := s.beadsClient.Show(ctx, root, id)
	if err != nil {
		http.Error(w, "show: "+err.Error(), s.beadsShowStatus(err))
		return
	}
	if issue.Status == "closed" {
		http.Error(w, "issue is closed", http.StatusConflict)
		return
	}
	if err := s.beadsMutator.Claim(ctx, root, id); err != nil {
		http.Error(w, "bd: "+err.Error(), http.StatusBadGateway)
		return
	}
	s.beadsRefreshAndBroadcast(ctx)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}
```
`handleBeadsClose` mirrors claim but decodes `{"reason":...}` (tolerate an empty body), re-checks closed→409, calls `Close(ctx,root,id,reason)`.
`handleBeadsCreate` calls `beadsWriteGate` (no id), decodes `{title,type,priority,description}`, trims+validates title/`beads.ValidType`/`beads.ValidPriority` (400 each), calls `Create`, returns `{"ok":true,"id":"<id>"}`.

- [ ] **Step 5: Wire server.go** — add `beadsMutator beadsMutator` field; in `New()` set `s.beadsMutator = bc` next to `s.beadsClient = bc`; register the three `mux.HandleFunc("POST /api/beads/...")` routes; add `BeadsWritable bool` to the `handleShell` struct = `s.beads != nil && s.cfg.Beads.Writable`.
- [ ] **Step 6: shell.html.tmpl** — add `<script>window.HD_BEADS_WRITABLE = {{.BeadsWritable}};</script>` next to the `HD_BEADS` line.
- [ ] **Step 7: Run — expect PASS + full server suite.** `go test ./internal/server/`
- [ ] **Step 8: gofmt + commit** — `feat(server): beads claim/close/create endpoints + writable gate + immediate refresh`.

---

### Task 3: Backlog view actions (frontend)

**Files:**
- Modify: `internal/assets/aggregator-backlog.js` — Claim/Close in the detail panel; a create form; POST helpers; `claimFocused`/`closeFocused`/`newFocusedRepo` for keys.
- Modify: `internal/assets/aggregator.js` — `c`/`x`/`n` in the backlog keydown block (writable only).
- Modify: `internal/assets/aggregator.css` — `.bk-act*` / `.bk-form*` styles.
- Verify: browser (chrome-devtools), against a throwaway issue.

**Interfaces:**
- Consumes: `POST /api/beads/{project}/{id}/claim|close`, `POST /api/beads/{project}/create`, `window.HD_BEADS_WRITABLE`.
- Produces: `HDBacklog.claimFocused()`, `HDBacklog.closeFocused()`, `HDBacklog.newFocusedRepo()`.

- [ ] **Step 1: POST helper + detail-panel actions** in aggregator-backlog.js. A `postAction(url, body)` → `fetch(POST json)` → on ok `refreshBeads()` (+ re-open detail for claim/close); on error show the response text inline in the panel. In `paintDetail`, when `window.HD_BEADS_WRITABLE`: append a Claim button (hidden if `issue.status === 'closed'`) and a Close control (reason `<input>` + button). Build with `HDDom.el` (no innerHTML).
- [ ] **Step 2: Create form.** A `+` button in the `repoCard` header (writable only) toggles a `.bk-form` panel (title input, type `<select>` bug/feature/task/epic/chore, priority `<select>` 0–4, description `<textarea>`, Create/Cancel). Submit → `postAction('/api/beads/'+enc(project)+'/create', {...})`.
- [ ] **Step 3: Keyboard.** Expose `claimFocused/closeFocused/newFocusedRepo` (act on `focusedRow()`); in aggregator.js's backlog keydown block add — only when `window.HD_BEADS_WRITABLE` — `case 'c'`, `case 'x'`, `case 'n'` calling them + `consume()`.
- [ ] **Step 4: CSS** — `.bk-actions`, `.bk-btn`, `.bk-reason`, `.bk-form`, `.bk-form select/textarea`, `.bk-err` in aggregator.css, reusing `--tn-*` tokens.
- [ ] **Step 5: Browser-verify** (config with `beads.enabled:true, beads.writable:true`): create a throwaway issue in a repo → it appears; claim it → status flips; close it with a reason → it leaves the ready/blocked lists. Then flip `writable:false`, reload: **no action affordances**, and a direct `curl -X POST .../claim` returns 403. Zero console errors. Leave the throwaway issue **closed** (note its id in the verify log).
- [ ] **Step 6: commit** — `feat(ui): beads actions — claim/close in detail, create form, c/x/n keys`.

---

### Task 4: docs + close-out

- [ ] **Step 1** `docs/SETUP.md` §9 — document `beads.writable` (default false; required for claim/close/create; restart to apply).
- [ ] **Step 2** `.docs/ai/decisions.md` — ADR "Beads actions (Phase 2)": two-flag gate rationale, argv equals-form, write mutex, RefreshNow, status re-check.
- [ ] **Step 3** roadmap/current-state routing; verify `go build ./... && go test ./...` green, `gofmt -l .` empty, `grep -c require go.mod` = 0.
- [ ] **Step 4** `bd -C ~/git/harness-deck close harness-deck-5ph.2 --reason "…"`.
- [ ] **Step 5: commit** — `docs(beads): Phase 2 setup + ADR + roadmap`.

---

## Self-Review

- **Spec coverage:** two-flag gate (T1 config + T2 server 403 + T2/T3 HD_BEADS_WRITABLE); 3 endpoints (T2); argv equals-form + enums (T1); write mutex (T1); RefreshNow freshness (T2); status re-check 409 (T2); UI claim/close/create + keys (T3); tests every guard (T2); docs (T4). ✔
- **Type consistency:** `beadsMutator` (Claim/Close/Create) defined T2, satisfied by `*beads.Client` (T1) + `fakeBeadsRW` (T2 test); re-check via existing `beadsDetailer.Show`. `ValidType`/`ValidPriority` defined T1, used T2. ✔
- **Placeholders:** none — greenfield code + tests are concrete; integration edits name exact files + the pattern to mirror.
